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

// runIngestion picks a Source, runs the pipeline, and records the result
// in the atomics so /healthz can report it. It deliberately catches and
// records every failure rather than panicking — the HTTP server should
// keep serving so docker-compose, the BFF, and the dashboard can all
// observe the failure mode.
func runIngestion(log *slog.Logger, archive *MinIOArchive, pub *Publisher) {
	// Vanilla NATS does not persist — wait long enough for the analytics
	// subscriber to register before publishing.
	time.Sleep(5 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	source, err := buildSource(ctx, log)
	if err != nil {
		log.Error("source init failed", "err", err)
		lastError.Store(err.Error())
		return
	}
	lastMode.Store(source.Name())
	jobID := time.Now().UTC().Format("20060102T150405Z")
	lastJobID.Store(jobID)

	log.Info("ingestion run starting", "source", source.Name(), "job_id", jobID)
	files, err := source.Files(ctx)
	if err != nil {
		log.Error("source fetch failed", "err", err)
		lastError.Store(err.Error())
		return
	}

	for _, f := range files {
		if _, err := archive.Put(ctx, jobID, f); err != nil {
			log.Error("archive put failed", "err", err, "resource", f.ResourceType)
			lastError.Store(err.Error())
			return
		}
		archived.Add(1)
		n, err := pub.Publish(f)
		if err != nil {
			log.Error("publish failed", "err", err, "resource", f.ResourceType)
			lastError.Store(err.Error())
			return
		}
		published.Add(int64(n))
		log.Info("ingested file", "resource", f.ResourceType, "lines", n)
	}

	if err := pub.conn.Flush(); err != nil {
		log.Error("nats flush failed", "err", err)
		lastError.Store(err.Error())
		return
	}
	log.Info("ingestion run complete", "files", len(files), "lines", published.Load())
}

func buildSource(ctx context.Context, log *slog.Logger) (Source, error) {
	mode := strings.ToUpper(envOr("SOURCE_MODE", "FIXTURES"))
	switch mode {
	case "FIXTURES":
		return &FixtureSource{
			Dir:   envOr("FIXTURES_DIR", defaultFixturesDir),
			Label: envOr("FIXTURES_LABEL", "fixtures"),
		}, nil
	case "BULK_EXPORT":
		return buildBulkExportSource(ctx, log)
	default:
		return nil, fmt.Errorf("unknown SOURCE_MODE %q (want FIXTURES or BULK_EXPORT)", mode)
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
		PollInterval:   parseDurationOr("EPIC_POLL_INTERVAL", 5*time.Second),
		PollMaxRetries: parseIntOr("EPIC_POLL_MAX_RETRIES", 60),
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
