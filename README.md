# fhir-quality-platform

A polyglot population-health / eCQM platform built around FHIR. Each service is
written in the language that best fits its job: Go for high-concurrency
ingestion, Python for analytics, Spring Boot for enterprise core / auth,
Node + TypeScript for the SMART on FHIR + CDS Hooks edge, Angular for the
clinician dashboard.

The MVP measure is **CMS122 — Diabetes: Hemoglobin A1c (HbA1c) Poor Control
(> 9 %)**: percentage of patients aged 18-75 with diabetes whose most recent
HbA1c is above 9 %. Computing it end-to-end exercises every layer.

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │   Angular Dashboard (TypeScript)    │
                    │   Quality dashboards · drill-downs  │
                    │   Measure admin · report builder    │
                    └────────────────┬────────────────────┘
                                     │  HTTPS (BFF pattern)
                    ┌────────────────▼────────────────────┐
                    │  Node + TypeScript — SMART Edge     │
                    │  /smart/launch · /smart/callback    │
                    │  /cds-services/* (CDS Hooks)        │
                    └────┬────────────────┬───────────────┘
                         │ REST           │ REST
                         ▼                ▼
       ┌─────────────────────────┐   ┌─────────────────────────────┐
       │ Spring Boot — Core API  │   │ Python — Analytics Service  │
       │ Multi-tenant orgs/users │   │ eCQM measure logic          │
       │ OAuth2 IdP              │   │ Risk stratification         │
       │ Measure library / audit │   │ NLP on notes                │
       │ Postgres                │   │ TimescaleDB                 │
       └─────────────────────────┘   └────────────▲────────────────┘
                                                  │ NATS events
       ┌──────────────────────────────────────────┴───────────────┐
       │ Go — Bulk Ingestion Service                              │
       │ FHIR $export orchestration · per-tenant goroutine pools  │
       │ NDJSON streaming · object-store archive · retry/backoff  │
       └─────────────────▲─────────────────────────────────────────┘
                         │ FHIR Bulk Data API
              ┌──────────┴───────────┐
              │  External EHRs       │  (Epic, Cerner, Athena, …)
              └──────────────────────┘
```

## Service boundaries

| Service                    | Owns                                                       | Talks to                                |
| -------------------------- | ---------------------------------------------------------- | --------------------------------------- |
| `services/ingestion-go`    | FHIR `$export` job lifecycle + NDJSON streaming            | EHRs (outbound), NATS (publish), MinIO  |
| `services/analytics-py`    | eCQM computation, ML risk models, NLP, terminology mapping | NATS (subscribe), TimescaleDB           |
| `services/core-spring`     | Identity, orgs, measure config, audit, OAuth2 IdP          | Postgres, Node edge                     |
| `services/edge-node`       | SMART launch, CDS Hooks, frontend BFF                      | Spring (auth/config), Python (results)  |
| `apps/dashboard-angular`   | Clinician + admin UI                                       | Node edge (only)                        |

Design rules:

- No service has more than two outbound dependencies.
- The frontend only ever talks to the Node edge (single BFF surface).
- Spring owns *configured* state (orgs, measure defs, audit); Python owns
  *derived* state (measure results, predictions).
- Go is the only service that touches external EHRs.
- The event bus (NATS JetStream) is the only async coupling.

## Repo layout

```
fhir-quality-platform/
├── README.md                  ← this file
├── docker-compose.yml         ← brings up the whole stack locally
├── schemas/                   ← shared FHIR / CDS Hooks JSON Schemas
├── services/
│   ├── ingestion-go/          ← Go (Golang)
│   ├── analytics-py/          ← Python
│   ├── core-spring/           ← Java + Spring Boot
│   └── edge-node/             ← Node.js + TypeScript
└── apps/
    └── dashboard-angular/     ← Angular + TypeScript
```

## Local development

Prerequisites: Docker Desktop (or equivalent), then:

```bash
docker compose up -d
```

This boots Postgres, TimescaleDB, NATS JetStream, MinIO, and each service
container. Service-specific build/test commands live in each service's
`README.md`.

## Status

Repository skeleton only. Each service contains a README describing its role;
implementations are stubbed and will be filled in incrementally, MVP measure
first (CMS122 end-to-end against Synthea-generated diabetic cohorts and the
Epic R4 sandbox).
