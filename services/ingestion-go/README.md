# ingestion-go

**Language / runtime:** Go (Golang) 1.22+
**Role:** Orchestrates FHIR Bulk Data (`$export`) jobs against external EHRs,
streams the resulting NDJSON into the event bus, and archives raw payloads
to object storage.

## Why Go

The Bulk Data workflow is heavy on I/O concurrency: many simultaneous
`$export` jobs (one per tenant × EHR), each polling job status, each
downloading large NDJSON streams. Goroutines + channels model that
naturally; the memory cost per in-flight connection is low.

## Responsibilities

- Discover and authenticate to upstream FHIR endpoints (SMART Backend
  Services, `private_key_jwt`).
- Kick off `$export` operations (group, system, or patient-scope).
- Poll the bulk job status endpoint with backoff; download NDJSON when
  complete.
- Parse NDJSON streamingly (no full-file buffering) and publish one
  resource per NATS message on `fhir.resource.<ResourceType>`.
- Archive every raw NDJSON file to MinIO under
  `s3://fqp/raw/<tenant>/<jobId>/<resourceType>.ndjson` for replay and
  audit.
- Per-tenant goroutine pools with rate-limit + retry.

## Outbound dependencies

- External EHR FHIR endpoints (HTTPS)
- NATS JetStream (publish)
- MinIO / S3 (PUT)

## Inbound interfaces

- HTTP admin API: `POST /jobs`, `GET /jobs/{id}`, `GET /healthz`.

## Status

Stub — directory exists; no Go module initialized yet.
