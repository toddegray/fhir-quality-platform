// Command ingestion-go is the entry-point binary for the FHIR Bulk
// Data ingestion service. It assembles a pipeline of
//
//	Source  →  MinIOArchive  →  Publisher (NATS)
//
// for each ingestion run. The Source is selected by env var:
//
//	SOURCE_MODE=FIXTURES     (default) read NDJSON from FIXTURES_DIR
//	SOURCE_MODE=BULK_EXPORT  call $export against EPIC_FHIR_BASE
//
// Both modes archive every NDJSON file under
// raw/<source>/<jobId>/<ResourceType>.ndjson in MinIO, then publish
// one NATS message per JSON line on fhir.resource.<ResourceType>.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	defaultListenAddr   = ":8080"
	defaultFixturesDir  = "/data/fixtures"
	defaultNatsURL      = nats.DefaultURL
	defaultMinIOBucket  = "fqp-raw"
	defaultMinIOEndpoint = "minio:9000"
	defaultMinIOAccess  = "fqp"
	defaultMinIOSecret  = "fqp-local-dev"
	shutdownGracePeriod = 10 * time.Second
	readHeaderTimeout   = 5 * time.Second
	readBodyTimeout     = 30 * time.Second
	idleTimeout         = 60 * time.Second
)

var (
	published   atomic.Int64
	archived    atomic.Int64
	lastJobID   atomic.Value // string
	lastMode    atomic.Value // string
	lastError   atomic.Value // string
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := envOr("LISTEN_ADDR", defaultListenAddr)
	natsURL := envOr("NATS_URL", defaultNatsURL)

	nc, err := connectNATS(natsURL, log)
	if err != nil {
		log.Error("nats connect failed", "err", err, "url", natsURL)
		os.Exit(1)
	}
	defer nc.Drain() //nolint:errcheck

	archive, err := buildArchive(log)
	if err != nil {
		log.Error("minio init failed", "err", err)
		os.Exit(1)
	}

	pub := NewPublisher(nc, log)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthHandler())

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readBodyTimeout,
		IdleTimeout:       idleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("ingestion-go ready", "addr", addr, "nats", natsURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	go runIngestion(log, archive, pub)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case err := <-serverErr:
		log.Error("listen failed", "err", err)
		os.Exit(1)
	case sig := <-stop:
		log.Info("shutdown signal received", "signal", sig.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	log.Info("ingestion-go stopped")
}

// runIngestion runs one ingestion pass per configured source. SOURCE_MODE
// is comma-separated — e.g. "FIXTURES,EPIC_REST" loads the synthetic
// chart and pulls real Epic patient data side-by-side, archiving both
// under separate prefixes in MinIO. A failure in one source is logged
// and reported via /healthz but doesn't abort the others.
func runIngestion(log *slog.Logger, archive *MinIOArchive, pub *Publisher) {
	// Vanilla NATS does not persist — wait long enough for the analytics
	// subscriber to register before publishing.
	time.Sleep(5 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	modes := splitCSV(envOr("SOURCE_MODE", "FIXTURES"))
	var sourceNames []string
	var anySuccess bool
	for _, mode := range modes {
		jobID := time.Now().UTC().Format("20060102T150405Z") + "-" + strings.ToLower(mode)
		if err := runOneSource(ctx, mode, jobID, archive, pub, log); err != nil {
			log.Error("source pipeline failed", "mode", mode, "err", err)
			lastError.Store(mode + ": " + err.Error())
			continue
		}
		sourceNames = append(sourceNames, mode)
		anySuccess = true
		lastJobID.Store(jobID)
	}
	if anySuccess {
		lastMode.Store(strings.Join(sourceNames, ","))
	}
	if err := pub.conn.Flush(); err != nil {
		log.Error("nats flush failed", "err", err)
		lastError.Store(err.Error())
	}
	log.Info("ingestion complete", "sources", sourceNames, "lines", published.Load(), "files", archived.Load())
}

func runOneSource(ctx context.Context, mode, jobID string, archive *MinIOArchive, pub *Publisher, log *slog.Logger) error {
	source, err := buildSourceForMode(ctx, mode, log)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}
	log.Info("ingestion run starting", "source", source.Name(), "job_id", jobID)
	files, err := source.Files(ctx)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	for _, f := range files {
		if _, err := archive.Put(ctx, jobID, f); err != nil {
			return fmt.Errorf("archive %s: %w", f.ResourceType, err)
		}
		archived.Add(1)
		n, err := pub.Publish(f)
		if err != nil {
			return fmt.Errorf("publish %s: %w", f.ResourceType, err)
		}
		published.Add(int64(n))
		log.Info("ingested file", "source", source.Name(), "resource", f.ResourceType, "lines", n)
	}
	return nil
}

func buildSourceForMode(ctx context.Context, mode string, log *slog.Logger) (Source, error) {
	switch strings.ToUpper(mode) {
	case "FIXTURES":
		return &FixtureSource{
			Dir:   envOr("FIXTURES_DIR", defaultFixturesDir),
			Label: envOr("FIXTURES_LABEL", "fixtures"),
		}, nil
	case "BULK_EXPORT":
		return buildBulkExportSource(ctx, log)
	case "EPIC_REST":
		return buildEpicRestSource(ctx, log)
	default:
		return nil, fmt.Errorf("unknown source mode %q (want FIXTURES, BULK_EXPORT, or EPIC_REST)", mode)
	}
}

func buildBulkExportSource(ctx context.Context, log *slog.Logger) (*BulkExportSource, error) {
	fhirBase := os.Getenv("EPIC_FHIR_BASE")
	clientID := os.Getenv("EPIC_CLIENT_ID")
	keyID := envOr("EPIC_KEY_ID", "ingestion-go-1")
	keyPEM := os.Getenv("EPIC_PRIVATE_KEY_PEM")
	scopes := splitCSV(envOr("EPIC_SCOPES",
		"system/Patient.read system/Condition.read system/Observation.read system/Encounter.read"))
	resourceTypes := splitCSV(envOr("EPIC_RESOURCE_TYPES",
		"Patient,Condition,Observation,Encounter"))

	if fhirBase == "" || clientID == "" || keyPEM == "" {
		return nil, errors.New("BULK_EXPORT mode requires EPIC_FHIR_BASE, EPIC_CLIENT_ID, EPIC_PRIVATE_KEY_PEM")
	}

	tokenEndpoint, err := DiscoverTokenEndpoint(ctx, fhirBase)
	if err != nil {
		return nil, fmt.Errorf("smart configuration: %w", err)
	}
	log.Info("bulk: discovered token endpoint", "endpoint", tokenEndpoint)

	return NewBulkExportSource(BulkExportConfig{
		FhirBase:       fhirBase,
		ClientID:       clientID,
		TokenEndpoint:  tokenEndpoint,
		PrivateKeyPEM:  []byte(strings.ReplaceAll(keyPEM, `\n`, "\n")),
		KeyID:          keyID,
		Scopes:         scopes,
		ResourceTypes:  resourceTypes,
		ExportScope:    envOr("EPIC_EXPORT_SCOPE", "patient"),
		GroupID:        os.Getenv("EPIC_GROUP_ID"),
		PollInterval:   parseDurationOr("EPIC_POLL_INTERVAL", 5*time.Second),
		PollMaxRetries: parseIntOr("EPIC_POLL_MAX_RETRIES", 60),
	}, log)
}

func buildEpicRestSource(ctx context.Context, log *slog.Logger) (*EpicRestSource, error) {
	fhirBase := os.Getenv("EPIC_FHIR_BASE")
	clientID := os.Getenv("EPIC_CLIENT_ID")
	keyID := envOr("EPIC_KEY_ID", "ingestion-go-1")
	keyPEM := os.Getenv("EPIC_PRIVATE_KEY_PEM")
	scopes := splitCSV(envOr("EPIC_SCOPES",
		"system/Patient.read system/Patient.search system/Condition.read"))
	patientIDs := splitCSV(envOr("EPIC_PATIENT_IDS", "erXuFYUfucBZaryVksYEcMg3"))

	if fhirBase == "" || clientID == "" || keyPEM == "" {
		return nil, errors.New("EPIC_REST mode requires EPIC_FHIR_BASE, EPIC_CLIENT_ID, EPIC_PRIVATE_KEY_PEM")
	}
	tokenEndpoint, err := DiscoverTokenEndpoint(ctx, fhirBase)
	if err != nil {
		return nil, fmt.Errorf("smart configuration: %w", err)
	}
	log.Info("epic-rest: discovered token endpoint", "endpoint", tokenEndpoint)

	return NewEpicRestSource(EpicRestConfig{
		FhirBase:      fhirBase,
		ClientID:      clientID,
		TokenEndpoint: tokenEndpoint,
		PrivateKeyPEM: []byte(strings.ReplaceAll(keyPEM, `\n`, "\n")),
		KeyID:         keyID,
		Scopes:        scopes,
		PatientIDs:    patientIDs,
	}, log)
}

func buildArchive(log *slog.Logger) (*MinIOArchive, error) {
	endpoint := envOr("MINIO_ENDPOINT", defaultMinIOEndpoint)
	access := envOr("MINIO_ACCESS_KEY", defaultMinIOAccess)
	secret := envOr("MINIO_SECRET_KEY", defaultMinIOSecret)
	bucket := envOr("MINIO_BUCKET", defaultMinIOBucket)
	secure, _ := strconv.ParseBool(envOr("MINIO_SECURE", "false"))
	return NewMinIOArchive(context.Background(), endpoint, access, secret, bucket, secure, log)
}

func connectNATS(url string, log *slog.Logger) (*nats.Conn, error) {
	return nats.Connect(url,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(60),
		nats.ReconnectWait(time.Second),
		nats.Name("ingestion-go"),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Warn("nats disconnected", "err", err)
		}),
		nats.ReconnectHandler(func(c *nats.Conn) {
			log.Info("nats reconnected", "url", c.ConnectedUrl())
		}),
	)
}

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":           "ok",
			"service":          "ingestion-go",
			"published":        published.Load(),
			"archived":         archived.Load(),
			"last_job_id":      asString(lastJobID.Load()),
			"last_source_mode": asString(lastMode.Load()),
			"last_error":       asString(lastError.Load()),
		})
	}
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	// Allow both comma and space separation since SMART scope lists are
	// space-delimited but resource-type lists are conventionally commas.
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseDurationOr(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func parseIntOr(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
