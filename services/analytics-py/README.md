# analytics-py

**Language / runtime:** Python 3.12+
**Role:** Consumes the normalized FHIR resource stream from NATS, computes
eCQM measures (starting with CMS122), runs risk-stratification ML models,
and applies NLP over clinical notes. Writes derived results to TimescaleDB.

## Why Python

Every piece of this service's job is a first-class Python ecosystem
strength: numerical work (NumPy / Pandas), terminology lookups
(SQLAlchemy + cached lookup tables), ML (scikit-learn / XGBoost),
NLP over clinical notes (spaCy / scispaCy / MedSpaCy). Doing this in any
other language on this list would be uphill.

## Responsibilities

- Subscribe to `fhir.resource.>` on NATS JetStream (durable consumer).
- Maintain a per-patient working set keyed on `Patient.id`.
- Apply value sets (RxNorm, LOINC, SNOMED CT, ICD-10) to normalize codes.
- Compute eCQM measures (Phase 1: **CMS122 — HbA1c poor control**).
- Risk stratification (Phase 2 — e.g., 30-day readmission risk).
- NLP over clinical notes for unstructured BP / smoking status / etc.
- Write measure results + risk scores as time-series rows in
  TimescaleDB.

## Outbound dependencies

- NATS JetStream (subscribe)
- TimescaleDB (write)

## Inbound interfaces

- HTTP query API for the Node edge:
  - `GET /measures/{measureId}/results?org=<id>&period=<yyyy>`
  - `GET /patients/{patientId}/gaps`
  - `GET /healthz`

## Status

Stub — directory exists; no Python package or virtualenv yet.
