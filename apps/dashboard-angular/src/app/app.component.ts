import { Component } from '@angular/core';

@Component({
  selector: 'fqp-root',
  standalone: true,
  template: `
    <main style="max-width: 720px; margin: 4rem auto; padding: 0 1.5rem;">
      <h1 style="font-size: 1.75rem; letter-spacing: -0.02em;">FHIR Quality Platform</h1>
      <p style="color: #515154;">
        Dashboard placeholder. The clinician + admin UI lands here — quality scores, gap drill-downs,
        measure-library admin, bulk-data job monitor, report builder.
      </p>
      <p style="color: #86868b; font-size: 0.9rem;">
        Backed by the <code>edge-node</code> BFF; never talks directly to the analytics, core, or
        ingestion services.
      </p>
    </main>
  `,
})
export class AppComponent {}
