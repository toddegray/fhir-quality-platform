package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// BulkExportConfig describes one SMART Backend Services + Bulk Data
// integration. ResourceTypes is the FHIR _type filter sent to $export.
type BulkExportConfig struct {
	FhirBase       string   // e.g. https://fhir.epic.com/interconnect-fhir-oauth/api/FHIR/R4
	ClientID       string   // SMART Backend Services client id
	TokenEndpoint  string   // discovered via .well-known/smart-configuration
	PrivateKeyPEM  []byte   // RSA private key, RS384 signing
	KeyID          string   // kid published in our JWKS
	Scopes         []string // e.g. ["system/Patient.read", ...]
	ResourceTypes  []string // _type query param on $export
	PollInterval   time.Duration
	PollMaxRetries int
}

// BulkExportSource pulls FHIR resources from a SMART Backend Services
// endpoint via the FHIR Bulk Data Access API ($export). The MVP issues
// a system-level $export; group/patient scope can be parameterised
// later.
type BulkExportSource struct {
	cfg    BulkExportConfig
	http   *http.Client
	log    *slog.Logger
	signer rsaSigner
}

func NewBulkExportSource(cfg BulkExportConfig, log *slog.Logger) (*BulkExportSource, error) {
	key, err := parseRSAPrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return &BulkExportSource{
		cfg:    cfg,
		http:   &http.Client{Timeout: 60 * time.Second},
		log:    log,
		signer: rsaSigner{key: key, keyID: cfg.KeyID},
	}, nil
}

func (s *BulkExportSource) Name() string { return "epic-bulk(" + s.cfg.FhirBase + ")" }

// Files orchestrates the full SMART Backend Services + Bulk Data dance:
// token exchange, $export init, status polling, and parallel download
// of each output URL into the returned ResourceFile slice.
func (s *BulkExportSource) Files(ctx context.Context) ([]ResourceFile, error) {
	tok, err := s.requestAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	s.log.Info("bulk: access token acquired", "scope", tok.Scope, "expires_in", tok.ExpiresIn)

	statusURL, err := s.initExport(ctx, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("$export init: %w", err)
	}
	s.log.Info("bulk: export job started", "status_url", statusURL)

	manifest, err := s.pollUntilComplete(ctx, statusURL, tok.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("poll status: %w", err)
	}
	s.log.Info("bulk: export complete", "files", len(manifest.Output))

	out := make([]ResourceFile, 0, len(manifest.Output))
	for _, f := range manifest.Output {
		data, err := s.download(ctx, f.URL, tok.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", f.URL, err)
		}
		out = append(out, ResourceFile{
			ResourceType: f.Type,
			Content:      data,
			SourceLabel:  "epic-bulk",
		})
	}
	return out, nil
}

// tokenResponse mirrors a standard OAuth2 / SMART client-credentials
// reply. Epic returns these fields plus a token_type of "bearer".
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// requestAccessToken signs a JWT assertion, exchanges it for an access
// token at the SMART token endpoint, and returns the parsed response.
func (s *BulkExportSource) requestAccessToken(ctx context.Context) (*tokenResponse, error) {
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

// initExport POSTs $export against the FHIR base. The response status
// is 202 Accepted with a Content-Location header pointing at the job
// status endpoint.
func (s *BulkExportSource) initExport(ctx context.Context, accessToken string) (string, error) {
	u, err := url.Parse(strings.TrimRight(s.cfg.FhirBase, "/") + "/$export")
	if err != nil {
		return "", err
	}
	if len(s.cfg.ResourceTypes) > 0 {
		q := u.Query()
		q.Set("_type", strings.Join(s.cfg.ResourceTypes, ","))
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/fhir+json")
	req.Header.Set("Prefer", "respond-async")

	resp, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("$export HTTP %d: %s", resp.StatusCode, body)
	}
	statusURL := resp.Header.Get("Content-Location")
	if statusURL == "" {
		return "", errors.New("$export: missing Content-Location header")
	}
	return statusURL, nil
}

// bulkManifest is the JSON body returned when an $export job reaches
// status 200. Output is the list of per-resource-type NDJSON file URLs.
type bulkManifest struct {
	TransactionTime string         `json:"transactionTime"`
	Request         string         `json:"request"`
	Output          []bulkOutFile  `json:"output"`
	Error           []bulkOutFile  `json:"error"`
}

type bulkOutFile struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// pollUntilComplete polls statusURL until the server returns 200 OK
// with a manifest body. 202 means "still processing"; anything else
// is a failure.
func (s *BulkExportSource) pollUntilComplete(ctx context.Context, statusURL, accessToken string) (*bulkManifest, error) {
	interval := s.cfg.PollInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	maxRetries := s.cfg.PollMaxRetries
	if maxRetries <= 0 {
		maxRetries = 60 // 5 minutes at 5s interval
	}

	for i := 0; i < maxRetries; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, statusURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Accept", "application/json")
		resp, err := s.http.Do(req)
		if err != nil {
			return nil, err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck

		switch resp.StatusCode {
		case http.StatusOK:
			var m bulkManifest
			if err := json.Unmarshal(body, &m); err != nil {
				return nil, fmt.Errorf("manifest decode: %w", err)
			}
			return &m, nil
		case http.StatusAccepted:
			s.log.Info("bulk: still processing",
				"progress", resp.Header.Get("X-Progress"), "attempt", i+1)
			time.Sleep(interval)
		default:
			return nil, fmt.Errorf("status HTTP %d: %s", resp.StatusCode, body)
		}
	}
	return nil, fmt.Errorf("status polling exhausted after %d attempts", maxRetries)
}

func (s *BulkExportSource) download(ctx context.Context, fileURL, accessToken string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/fhir+ndjson")
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("file HTTP %d: %s", resp.StatusCode, body)
	}
	return io.ReadAll(resp.Body)
}

// ---------------------- helpers --------------------------------------

// rsaSigner produces the SMART Backend Services client_assertion JWT.
type rsaSigner struct {
	key   *rsa.PrivateKey
	keyID string
}

func (s rsaSigner) signAssertion(clientID, tokenEndpoint string, ttl time.Duration) (string, error) {
	jti, err := randomJTI()
	if err != nil {
		return "", fmt.Errorf("jti: %w", err)
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss": clientID,
		"sub": clientID,
		"aud": tokenEndpoint,
		"jti": jti,
		"iat": now.Unix(),
		"exp": now.Add(ttl).Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodRS384, claims)
	t.Header["kid"] = s.keyID
	t.Header["typ"] = "JWT"
	return t.SignedString(s.key)
}

func parseRSAPrivateKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block found")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA (got %T)", key)
		}
		return rsaKey, nil
	default:
		return nil, fmt.Errorf("unsupported PEM block type %q", block.Type)
	}
}

func randomJTI() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", n), nil
}

// DiscoverTokenEndpoint fetches the SMART configuration document and
// returns its token_endpoint. Exposed so the caller can populate the
// BulkExportConfig before constructing the source.
func DiscoverTokenEndpoint(ctx context.Context, fhirBase string) (string, error) {
	wellKnown := strings.TrimRight(fhirBase, "/") + "/.well-known/smart-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, wellKnown, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("smart-configuration HTTP %d", resp.StatusCode)
	}
	var cfg struct {
		TokenEndpoint string `json:"token_endpoint"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return "", err
	}
	if cfg.TokenEndpoint == "" {
		return "", errors.New("smart-configuration missing token_endpoint")
	}
	return cfg.TokenEndpoint, nil
}
