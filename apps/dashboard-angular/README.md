# dashboard-angular

**Language / runtime:** Angular 17+ (standalone components) + TypeScript
(strict mode)
**Role:** The single-page application clinicians and admins use. Talks
exclusively to the `edge-node` BFF; never makes cross-origin calls to the
analytics, core, or ingestion services.

## Why Angular

Angular's opinionated structure (DI, modules → standalone components,
reactive forms, RxJS) fits the multi-screen, role-aware enterprise UX
this platform needs: nested routes for dashboards / drill-downs / admin,
typed reactive forms for measure-config screens, schedulers, etc.

## Responsibilities

- **Population dashboard** — quality scores per measure / per provider /
  per org, with heat maps, trend lines, comparison views.
- **Patient drill-down** — for any gap surfaced in the dashboard, show
  the contributing FHIR resources, the rule logic that fired, and a
  link to the originating EHR if available.
- **Measure library admin** — manage which eCQMs are active per org,
  edit value-set bindings, schedule attestation periods.
- **Bulk-data job monitor** — visibility into ingestion-go job status.
- **Report builder** — custom report definitions, export to PDF / XLSX
  for MIPS / MACRA submissions.

## Outbound dependencies

- `edge-node` BFF only (single origin)

## Inbound interfaces

- Browser only.

## Status

Stub — directory exists; no Angular workspace generated yet
(`ng new dashboard-angular` will live here).
