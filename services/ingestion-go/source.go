package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResourceFile is one NDJSON file's worth of FHIR resources of a single
// ResourceType. SourceLabel identifies the origin (e.g. "fixtures",
// "epic-bulk:<jobId>") and is used to namespace the archive object key.
type ResourceFile struct {
	ResourceType string
	Content      []byte
	SourceLabel  string
}

// Source produces a batch of ResourceFile values. Each call should
// return the full set of resources to ingest for one pass; the caller
// is responsible for archive + publish of every returned file.
type Source interface {
	Name() string
	Files(ctx context.Context) ([]ResourceFile, error)
}

// FixtureSource reads *.ndjson files from a directory on disk. The
// filename (minus extension) is the FHIR ResourceType.
type FixtureSource struct {
	Dir   string
	Label string
}

func (s *FixtureSource) Name() string { return "fixtures(" + s.Dir + ")" }

func (s *FixtureSource) Files(_ context.Context) ([]ResourceFile, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, fmt.Errorf("read fixtures dir %s: %w", s.Dir, err)
	}
	var out []ResourceFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".ndjson") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.Dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", e.Name(), err)
		}
		out = append(out, ResourceFile{
			ResourceType: strings.TrimSuffix(e.Name(), ".ndjson"),
			Content:      data,
			SourceLabel:  s.Label,
		})
	}
	return out, nil
}
