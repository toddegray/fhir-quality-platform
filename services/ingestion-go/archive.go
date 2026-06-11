package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"path"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOArchive uploads every ingested NDJSON file to an S3-compatible
// object store. The archive is the system-of-record for replay and
// audit: "what bytes did we see, when, from where."
type MinIOArchive struct {
	client *minio.Client
	bucket string
	prefix string // e.g. "raw" — keeps room for parallel "curated" buckets later
	log    *slog.Logger
}

func NewMinIOArchive(ctx context.Context, endpoint, accessKey, secretKey, bucket string, secure bool, log *slog.Logger) (*MinIOArchive, error) {
	cli, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: secure,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client: %w", err)
	}
	exists, err := cli.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("bucket exists check: %w", err)
	}
	if !exists {
		if err := cli.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %s: %w", bucket, err)
		}
		log.Info("created object bucket", "bucket", bucket)
	}
	return &MinIOArchive{
		client: cli,
		bucket: bucket,
		prefix: "raw",
		log:    log,
	}, nil
}

// Put archives one ResourceFile as a single object. The key layout
// (raw/<source>/<jobId>/<ResourceType>.ndjson) is deliberate: it
// stays grep-able in the console and groups all files from one
// ingestion run for easy replay.
func (a *MinIOArchive) Put(ctx context.Context, jobID string, file ResourceFile) (string, error) {
	key := path.Join(a.prefix, file.SourceLabel, jobID, file.ResourceType+".ndjson")
	reader := bytes.NewReader(file.Content)
	_, err := a.client.PutObject(ctx, a.bucket, key, reader, int64(len(file.Content)), minio.PutObjectOptions{
		ContentType: "application/fhir+ndjson",
		UserMetadata: map[string]string{
			"resource-type":  file.ResourceType,
			"source-label":   file.SourceLabel,
			"ingested-at-iso": time.Now().UTC().Format(time.RFC3339),
		},
	})
	if err != nil {
		return "", fmt.Errorf("put %s: %w", key, err)
	}
	a.log.Info("archived ndjson file", "bucket", a.bucket, "key", key, "bytes", len(file.Content))
	return key, nil
}
