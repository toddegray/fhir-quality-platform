# edge-node

**Language / runtime:** Node.js 20+ with TypeScript (strict mode)
**Role:** The edge tier in front of the dashboard. Hosts the SMART on FHIR
launch endpoints and the CDS Hooks service surface, and acts as the
backend-for-frontend (BFF) for the Angular dashboard.

## Why Node + TypeScript

SMART on FHIR and CDS Hooks are JSON-heavy specs with rapidly evolving
shapes. Node + TS lets us model the wire format with strict schemas (Zod
or `io-ts`), iterate fast, and keep latency on the hot interactive paths
low. The CDS Hooks service surface is essentially a JSON-in / JSON-out
function — exactly Node's sweet spot.

## Responsibilities

- SMART on FHIR endpoints:
  - `GET /smart/launch` (EHR-initiated + standalone)
  - `GET /smart/callback`
  - Session minting + cookie / JWT issuance to the dashboard.
- CDS Hooks service endpoints:
  - `GET /cds-services` (discovery)
  - `POST /cds-services/{serviceId}` (e.g. `quality-gaps`)
  - Calls Python analytics for the actual measure data.
- BFF for the Angular dashboard:
  - Aggregates calls to Spring (config / auth) and Python (results) into
    dashboard-shaped responses.
- Strict request validation (Zod / OpenAPI).

## Outbound dependencies

- Spring core API (`CORE_API_URL`)
- Python analytics API (`ANALYTICS_API_URL`)

## Inbound interfaces

- Public HTTPS:
  - `/smart/**`, `/cds-services/**`, `/api/**` (dashboard BFF),
    `/healthz`

## Status

Stub — directory exists; no `package.json` yet.
