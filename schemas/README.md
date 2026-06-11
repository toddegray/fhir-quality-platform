# schemas

Shared JSON Schemas for the cross-service wire format. Every service
generates / validates against these:

- **FHIR R4 resource schemas** — vendored from the official R4 schema
  bundle, used for resource validation in the Go ingestion path and
  request validation in the Node edge.
- **CDS Hooks request / response schemas** — the wire contract for
  `POST /cds-services/{serviceId}` in the Node edge.
- **Platform-internal event envelopes** — the shape of the messages Go
  publishes to NATS (`fhir.resource.<ResourceType>`) and Python
  consumes.

Each service has its own code-gen step:

- `services/ingestion-go` → `quicktype` → Go structs
- `services/analytics-py` → `datamodel-code-generator` → Pydantic models
- `services/core-spring` → `jsonschema2pojo` → Java records
- `services/edge-node` → `json-schema-to-typescript` (or Zod) → TS types

The contract lives here; the generated code lives in each service.
