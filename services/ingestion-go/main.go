// Command ingestion-go is the entry-point binary for the FHIR Bulk Data
// ingestion service.
//
// In the MVP it reads NDJSON fixture files from FIXTURES_DIR (one file
// per FHIR ResourceType, e.g. Patient.ndjson) and publishes each line to
// NATS JetStream on subject "fhir.resource.<ResourceType>". A real
// $export → poll → download path replaces the fixture-loader in later
// iterations.
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
	"path/filepath"
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
	shutdownGracePeriod = 10 * time.Second
	readHeaderTimeout   = 5 * time.Second
	readBodyTimeout     = 30 * time.Second
	idleTimeout         = 60 * time.Second
)

// published is incremented atomically as resources are published; the
// /healthz handler reports it so we can observe ingestion completion
// without scraping logs.
var published atomic.Int64

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := envOr("LISTEN_ADDR", defaultListenAddr)
	natsURL := envOr("NATS_URL", defaultNatsURL)
	fixturesDir := envOr("FIXTURES_DIR", defaultFixturesDir)

	nc, err := connectNATS(natsURL, log)
	if err != nil {
		log.Error("nats connect failed", "err", err, "url", natsURL)
		os.Exit(1)
	}
	defer nc.Drain() //nolint:errcheck

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
		log.Info("ingestion-go ready", "addr", addr, "nats", natsURL, "fixtures", fixturesDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	go func() {
		// Give the analytics subscriber time to register before we
		// publish. Vanilla NATS does not persist messages, so a
		// consumer that joins after the publish misses the data —
		// kept short and explicit for the MVP. Replace with NATS
		// JetStream + a durable consumer when replay matters.
		time.Sleep(5 * time.Second)
		if err := ingestFixtures(fixturesDir, nc, log); err != nil {
			log.Error("fixture ingestion failed", "err", err)
		}
	}()

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

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// connectNATS retries connection until the daemon becomes reachable.
// docker-compose may schedule ingestion-go ahead of nats fully being
// ready even with depends_on, so we wait rather than crash-loop.
func connectNATS(url string, log *slog.Logger) (*nats.Conn, error) {
	opts := []nats.Option{
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
	}
	return nats.Connect(url, opts...)
}

// ingestFixtures reads each *.ndjson file under dir and publishes one
// NATS message per line. The filename (minus extension) is used as the
// FHIR ResourceType in the subject; the line itself is the message
// payload so downstream consumers don't have to re-parse the envelope.
func ingestFixtures(dir string, nc *nats.Conn, log *slog.Logger) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read fixtures dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ndjson") {
			continue
		}
		resourceType := strings.TrimSuffix(e.Name(), ".ndjson")
		path := filepath.Join(dir, e.Name())
		count, err := publishFile(path, resourceType, nc, log)
		if err != nil {
			return fmt.Errorf("publish %s: %w", path, err)
		}
		log.Info("published fixture file", "file", e.Name(), "count", count)
	}
	if err := nc.Flush(); err != nil {
		return fmt.Errorf("nats flush: %w", err)
	}
	log.Info("ingestion complete", "total", published.Load())
	return nil
}

func publishFile(path, resourceType string, nc *nats.Conn, log *slog.Logger) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	subject := "fhir.resource." + resourceType
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	count := 0
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Validate the line is JSON before publishing so a malformed
		// fixture file doesn't pollute the stream with garbage.
		if !json.Valid([]byte(line)) {
			log.Warn("invalid JSON line skipped", "file", path, "line", i+1)
			continue
		}
		if err := nc.Publish(subject, []byte(line)); err != nil {
			return count, err
		}
		count++
		published.Add(1)
	}
	return count, nil
}

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"service":   "ingestion-go",
			"published": published.Load(),
		})
	}
}
