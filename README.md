# fhir-quality-platform

A population-health platform that ingests FHIR resources, computes
**CMS electronic Clinical Quality Measures (eCQMs)** over a provider
panel, and surfaces care gaps in a clinician dashboard. Built as a
working slice of the same product space that CMS contractors (BCDA,
QPP / MIPS reporting tooling, ACO quality platforms) ship into.

## What it does

Out of the box, `docker compose up -d` brings up the whole stack and
the dashboard at **http://localhost:4200** computes four real CMS
measures against a deterministic 100-patient synthetic cohort:

| Measure | Title | Direction |
| --- | --- | --- |
| **CMS122** | Diabetes: Hemoglobin A1c Poor Control (> 9 %) | lower is better |
| **CMS125** | Breast Cancer Screening (Mammography 50-74 ♀) | higher is better |
| **CMS165** | Controlling High Blood Pressure | higher is better |
| **CMS117** | Childhood Immunization Status (combo-10 by age 2) | higher is better |

The dashboard renders a **provider × measure heatmap** with a calibrated
red → amber → green gradient (inverted for lower-is-better measures).
Clicking any cell opens the **gap-patient drill-down** — the list of
patients pulling the measure away from target, each with the
contributing clinical detail (most recent HbA1c, blood pressure,
mammography date, missing vaccines). The detail panel also shows a
**12-month sparkline trend** for the measure and links to the eCQI
specification, with the code sets (ICD-10, LOINC, CPT, CVX) the
measure logic applies.

The cohort prevalence rates are calibrated to published US adult
epidemiology so the resulting measure scores are plausible:

- Diabetes ≈ 25 % (CDC adult prevalence, age-skewed)
- Hypertension ≈ 45 % (AHA 2024 stats)
- Mammography in last 27 mo ≈ 72 % for women 50-74 (NHIS 2022)
- Combo-10 series complete at age 2 ≈ 70 % (CDC NIS 2023)

Default seed = 42; numbers are reproducible across runs. Tune
prevalence, poor-control rates, and cohort size via env vars
(`SYNTH_PATIENT_COUNT`, `SYNTH_SEED`, …).

## Architecture

```
                    ┌─────────────────────────────────────┐
                    │   Angular Dashboard (TypeScript)    │
                    │   provider × measure heatmap        │
                    │   gap-patient drill-down + trend    │
                    └────────────────┬────────────────────┘
                                     │  same-origin via nginx
                    ┌────────────────▼────────────────────┐
                    │  Node + TypeScript (Fastify) BFF    │
                    │  /api/measures · /api/scorecard     │
                    │  /api/measures/{id}/history         │
                    └────┬────────────────┬───────────────┘
                         │                │
                         ▼                ▼
       ┌─────────────────────────┐   ┌─────────────────────────────┐
       │  Spring Boot — Core API │   │  Python — Analytics         │
       │  /measures (catalog,    │   │  CMS122 / 125 / 165 / 117   │
       │   citations, code sets) │   │  provider attribution       │
       │                         │   │  per-measure history        │
       └─────────────────────────┘   └────────────▲────────────────┘
                                                  │ NATS subjects
       ┌──────────────────────────────────────────┴───────────────┐
       │  Go — Ingestion                                          │
       │  Synthetic cohort generator (default)                    │
       │  · FHIR Bulk Data $export client (SMART Backend Services)│
       │  · Plain FHIR REST source (Patient by id)                │
       │  · MinIO archive of every NDJSON byte ingested           │
       └─────────────────▲─────────────────────────────────────────┘
                         │
              ┌──────────┴───────────┐
              │  External EHRs       │  (Epic, Cerner, Athena, …)
              └──────────────────────┘
```

Five services, each in the language that fits its job:

- **Go** for the ingestion pipeline (concurrent I/O, NDJSON streaming,
  S3 archive, RS384 JWT signing for SMART Backend Services).
- **Python** for the measure compute layer (value-set lookups, per-
  patient state, eCQM math, future ML + NLP).
- **Java + Spring Boot** for the enterprise core (measure catalog,
  citation links, future OAuth2 / audit / Postgres-backed config).
- **Node + TypeScript (Fastify)** for the SMART/CDS Hooks edge and
  the BFF the SPA talks to.
- **Angular + TypeScript** for the dashboard (standalone components,
  inline-SVG sparklines, signal-free reactive UI).

Glue: **NATS** for the async pub/sub spine, **MinIO** for the audit-
trail archive of every NDJSON file ever ingested, **Postgres** +
**TimescaleDB** provisioned (wired in for the next slice), one
`docker compose up -d` for the whole thing.

## Service boundaries

| Service | Owns | Talks to |
| --- | --- | --- |
| `services/ingestion-go` | FHIR ingestion (synthetic / Bulk Data $export / REST) + MinIO archive | EHRs, NATS, MinIO |
| `services/analytics-py` | eCQM logic, patient state, provider attribution, trend | NATS, TimescaleDB (planned) |
| `services/core-spring` | Measure catalog, citation links (future: OAuth2 + audit) | Postgres |
| `services/edge-node` | BFF aggregation, SMART / CDS Hooks (planned), nginx upstream | core, analytics |
| `apps/dashboard-angular` | Clinician + admin UI | edge-node only |

Design rules: no service has more than two outbound dependencies, the
frontend only ever talks to one origin (nginx proxies `/api/*` to the
BFF), Go is the only service that touches external EHRs, the event
bus is the only async coupling.

## URLs once the stack is up

| What | URL |
| --- | --- |
| Dashboard | http://localhost:4200 |
| BFF — scorecard | http://localhost:4200/api/scorecard |
| BFF — measure detail | http://localhost:4200/api/measures/CMS122 |
| Spring measure catalog | http://localhost:8083/measures |
| Python analytics raw | http://localhost:8082/measures |
| Go ingestion health | http://localhost:8081/healthz |
| MinIO console | http://localhost:9001 (`fqp` / `fqp-local-dev`) |
| NATS monitoring | http://localhost:8222 |

## Switching the FHIR source

`SOURCE_MODE` is comma-separated, so the synthetic cohort and a real
EHR pull can run side-by-side. Available modes:

- **`SYNTHETIC`** (default) — the deterministic 100-patient cohort. No
  external dependencies, no creds. Drives the demo numbers.
- **`FIXTURES`** — the three hand-built patients shipped at
  `services/ingestion-go/fixtures/`. Tiny but useful for unit-style
  iteration.
- **`EPIC_REST`** — pulls configured `Patient` resources from a real
  SMART Backend Services FHIR REST endpoint by id. Uses RS384 JWT
  `client_assertion`. Default patient is Camila Lopez
  (`erXuFYUfucBZaryVksYEcMg3`), the well-known Epic R4 sandbox patient.
- **`BULK_EXPORT`** — full FHIR Bulk Data `$export` against a compliant
  server: discovery, token exchange, async kickoff, status polling,
  manifest download, MinIO archive, NATS publish.

```bash
cp .env.example .env       # fill in EPIC_FHIR_BASE / EPIC_CLIENT_ID / EPIC_PRIVATE_KEY_PEM
# SOURCE_MODE=SYNTHETIC,EPIC_REST       → synthetic + one real patient
# SOURCE_MODE=BULK_EXPORT               → live $export only
docker compose up -d --force-recreate ingestion-go
```

`.env`, `*.pem`, and `*.key` are all gitignored.

**Epic R4 sandbox caveat.** Epic's sandbox doesn't expose Bulk Data on
this Backend Services client (system-level and patient-level both 404).
`EPIC_REST` mode therefore pulls Patient resources only and archives
them to MinIO under `raw/epic-rest/<jobId>/Patient.ndjson` — the
clinical chart needed for the eCQM math (`Observation`, `Encounter`)
isn't in the scope grant. The dashboard score stays driven by the
synthetic cohort; the Epic data shows up in the MinIO console as
visible proof that the integration round-trips.

## Honest scope

This is a working slice, not a production deployment:

- The synthetic cohort generator is deterministic and prevalence-
  calibrated, but it isn't Synthea. Real Synthea integration is a
  one-shot data-load swap that doesn't change the rest of the stack.
- The 12-month trend is currently synthesised from the live score (the
  sparkline anchors at the current value with seeded noise). The wire
  contract is the same one TimescaleDB-backed per-period rollups will
  serve; that swap lives behind the `/measures/{id}/history` endpoint.
- TimescaleDB and Postgres are up and provisioned but not yet
  consumed — they're staged for the next slices (measure-result time
  series, audit log, org / user / OAuth2).
- The Spring Boot OAuth2 Authorization Server is a stub; the only
  surface today is the measure catalog + citation links.
- No tests beyond compile + `up -d` + assert on `/healthz`. Unit
  testing is wired but sparse.

What's real:

- Five services in five languages running clean from one compose file.
- Four CMS eCQMs computing correctly against the cohort, with value-
  set logic against ICD-10, LOINC, CPT, and CVX code systems.
- Provider attribution + scorecard heatmap + drill-down + sparkline.
- SMART Backend Services JWT auth against the real Epic R4 sandbox,
  with the audit-trail MinIO archive verifying every NDJSON byte
  ingested.

## License

MIT. Code is intended as a portfolio piece and a reference for the
shape this kind of platform takes. PRs welcome.
