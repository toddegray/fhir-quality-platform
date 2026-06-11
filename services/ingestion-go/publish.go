package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/nats-io/nats.go"
)

// Publisher splits a ResourceFile's NDJSON body into individual lines
// and emits each line as a NATS message on "fhir.resource.<Type>".
type Publisher struct {
	conn *nats.Conn
	log  *slog.Logger
}

func NewPublisher(conn *nats.Conn, log *slog.Logger) *Publisher {
	return &Publisher{conn: conn, log: log}
}

// Publish returns the number of lines successfully published. Malformed
// JSON lines are logged and skipped — one bad row should not stop the
// rest of the file.
func (p *Publisher) Publish(file ResourceFile) (int, error) {
	subject := "fhir.resource." + file.ResourceType
	body := strings.TrimRight(string(file.Content), "\n")
	if body == "" {
		return 0, nil
	}

	count := 0
	for i, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			p.log.Warn("invalid JSON line skipped",
				"resourceType", file.ResourceType, "line", i+1)
			continue
		}
		if err := p.conn.Publish(subject, []byte(line)); err != nil {
			return count, fmt.Errorf("nats publish %s line %d: %w", subject, i+1, err)
		}
		count++
	}
	return count, nil
}
