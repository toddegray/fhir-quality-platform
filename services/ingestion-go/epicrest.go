package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EpicRestSource fetches resources via individual FHIR REST calls
// instead of the Bulk Data $export operation. We use this when the EHR
// can authenticate the client but does not grant Bulk Data access — the
// case for the Epic R4 sandbox with the spike's SMART Backend Services
// app, which is registered for `system/Patient.r + .s` plus a handful
// of read-by-id only resources (Condition, DiagnosticReport,
// MedicationRequest). Without `.s` on the others we can't enumerate a
// patient's chart, so this source produces just the Patient stream:
//
//	Patient.ndjson    — one Patient resource per configured id
//
// The CMS122 measure stays driven by the bundled fixtures (which carry
// the Encounter + Observation rows the Epic scope grant does not allow).
// MinIO therefore archives both: real Epic patient resources alongside
// the synthetic-but-complete clinical chart that drives the dashboard
// number.
type EpicRestSource struct {
	cfg    EpicRestConfig
	http   *http.Client
	log    *slog.Logger
	signer rsaSigner
}

type EpicRestConfig struct {
	FhirBase      string
	ClientID      string
	TokenEndpoint string
	PrivateKeyPEM []byte
	KeyID         string
	Scopes        []string
	PatientIDs    []string
}

func NewEpicRestSource(cfg EpicRestConfig, log *slog.Logger) (*EpicRestSource, error) {
	key, err := parseRSAPrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	if len(cfg.PatientIDs) == 0 {
		return nil, fmt.Errorf("EpicRestSource requires at least one patient id")
	}
	return &EpicRestSource{
		cfg:    cfg,
		http:   &http.Client{Timeout: 60 * time.Second},
		log:    log,
		signer: rsaSigner{key: key, keyID: cfg.KeyID},
	}, nil
}

func (s *EpicRestSource) Name() string {
	return fmt.Sprintf("epic-rest(%s, %d patient(s))", s.cfg.FhirBase, len(s.cfg.PatientIDs))
}

// Files acquires an access token and reads one Patient per configured
// id. Returns a single Patient.ndjson file with all of them
// concatenated.
func (s *EpicRestSource) Files(ctx context.Context) ([]ResourceFile, error) {
	tok, err := s.requestAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	s.log.Info("epic-rest: access token acquired",
		"scope", tok.Scope, "expires_in", tok.ExpiresIn, "patients", len(s.cfg.PatientIDs))

	var patientNDJSON strings.Builder
	for _, pid := range s.cfg.PatientIDs {
		patient, err := s.fetchPatient(ctx, tok.AccessToken, pid)
		if err != nil {
			return nil, fmt.Errorf("patient %s: %w", pid, err)
		}
		patientNDJSON.Write(patient)
		patientNDJSON.WriteByte('\n')
		s.log.Info("epic-rest: fetched patient", "id", pid, "bytes", len(patient))
	}

	return []ResourceFile{
		{
			ResourceType: "Patient",
			Content:      []byte(patientNDJSON.String()),
			SourceLabel:  "epic-rest",
		},
	}, nil
}

func (s *EpicRestSource) requestAccessToken(ctx context.Context) (*tokenResponse, error) {
	assertion, err := s.signer.signAssertion(s.cfg.ClientID, s.cfg.TokenEndpoint, 5*time.Minute)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer")
	form.Set("client_assertion", assertion)
	form.Set("scope", strings.Join(s.cfg.Scopes, " "))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("token HTTP %d: %s", resp.StatusCode, body)
	}
	var tok tokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("token decode: %w", err)
	}
	return &tok, nil
}

func (s *EpicRestSource) fetchPatient(ctx context.Context, accessToken, patientID string) ([]byte, error) {
	patientURL := strings.TrimRight(s.cfg.FhirBase, "/") + "/Patient/" + url.PathEscape(patientID)
	return s.getJSON(ctx, patientURL, accessToken)
}

// fetchConditions calls Condition.search filtered to problem-list-item.
// Epic's Backend Services scope grant ties the category to the full
// token form (system|code), so the search param must mirror it
// exactly or the response is 403. Pagination is followed via
// Bundle.link[rel=next] so the result reflects the full set.
func (s *EpicRestSource) fetchConditions(ctx context.Context, accessToken, patientID string) ([][]byte, error) {
	base := strings.TrimRight(s.cfg.FhirBase, "/") + "/Condition"
	q := url.Values{}
	q.Set("patient", patientID)
	q.Set("category", "http://terminology.hl7.org/CodeSystem/condition-category|problem-list-item")
	next := base + "?" + q.Encode()

	var out [][]byte
	for next != "" {
		body, err := s.getJSON(ctx, next, accessToken)
		if err != nil {
			return nil, err
		}
		var bundle struct {
			Entry []struct {
				Resource json.RawMessage `json:"resource"`
			} `json:"entry"`
			Link []struct {
				Relation string `json:"relation"`
				URL      string `json:"url"`
			} `json:"link"`
		}
		if err := json.Unmarshal(body, &bundle); err != nil {
			return nil, fmt.Errorf("bundle decode: %w", err)
		}
		for _, e := range bundle.Entry {
			if len(e.Resource) > 0 {
				out = append(out, append([]byte(nil), e.Resource...))
			}
		}
		next = ""
		for _, l := range bundle.Link {
			if strings.EqualFold(l.Relation, "next") {
				next = l.URL
				break
			}
		}
	}
	return out, nil
}

func (s *EpicRestSource) getJSON(ctx context.Context, fullURL, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/fhir+json")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s HTTP %d: %s", fullURL, resp.StatusCode, body)
	}
	return body, nil
}
