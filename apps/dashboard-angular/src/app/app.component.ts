import { CommonModule } from '@angular/common';
import { Component, OnInit, inject } from '@angular/core';
import { MeasureResult, MeasureService } from './measure.service';

@Component({
  selector: 'fqp-root',
  standalone: true,
  imports: [CommonModule],
  template: `
    <main class="dashboard">
      <header>
        <h1>FHIR Quality Platform</h1>
        <p class="subline">Polyglot eCQM demo — Go ingest → NATS → Python compute → Spring metadata → Node BFF → this dashboard.</p>
      </header>

      @if (loading) {
        <section class="card placeholder">Loading measure…</section>
      }

      @if (error) {
        <section class="card error">
          <h2>Couldn't load CMS122</h2>
          <p>{{ error }}</p>
        </section>
      }

      @if (measure) {
        <section class="card">
          <div class="measure-id">{{ measure.id }} · lower is better</div>
          <h2>{{ measure.title }}</h2>
          <p class="description">{{ measure.description }}</p>

          <div class="score">
            <div class="percentage">{{ measure.percentage | number:'1.1-1' }}<span>%</span></div>
            <div class="fraction">{{ measure.numerator }} / {{ measure.denominator }}</div>
            <div class="fraction-label">poor-control patients / eligible diabetics</div>
          </div>

          <div class="period">
            Measurement period {{ measure.measurementPeriod.start }} – {{ measure.measurementPeriod.end }}
          </div>

          @if (measure.gapPatients.length > 0) {
            <h3>Patients with care gaps</h3>
            <ul class="gaps">
              @for (gap of measure.gapPatients; track gap.patientId) {
                <li>
                  <span class="pid">{{ gap.patientId }}</span>
                  <span class="meta">age {{ gap.age }}</span>
                  <span class="hba1c">
                    HbA1c
                    @if (gap.latestHbA1c !== null) {
                      <strong>{{ gap.latestHbA1c | number:'1.1-1' }}%</strong>
                      <small>({{ gap.latestHbA1cDate }})</small>
                    } @else {
                      <strong>missing</strong>
                    }
                  </span>
                </li>
              }
            </ul>
          }

          <a class="spec" [href]="measure.reference" target="_blank" rel="noopener">eCQI measure specification ↗</a>
        </section>
      }
    </main>
  `,
  styles: [`
    .dashboard { max-width: 760px; margin: 3rem auto; padding: 0 1.5rem; }
    header h1 { font-size: 1.85rem; letter-spacing: -0.02em; margin: 0 0 .35rem; }
    .subline { color: #515154; margin: 0 0 2rem; }
    .card { background: #fff; border-radius: 16px; padding: 1.5rem 1.75rem;
            box-shadow: 0 1px 2px rgba(0,0,0,.05); margin-bottom: 1rem; }
    .card.placeholder, .card.error { color: #86868b; }
    .card.error { background: #fde8e8; color: #d92121; }
    .measure-id { font-size: .72rem; text-transform: uppercase; letter-spacing: .06em;
                  color: #86868b; font-weight: 600; margin-bottom: .35rem; }
    h2 { font-size: 1.2rem; letter-spacing: -0.01em; margin: 0 0 .35rem; }
    .description { color: #515154; margin: 0 0 1.25rem; font-size: .95rem; line-height: 1.5; }
    .score { display: flex; align-items: baseline; gap: 1.25rem; flex-wrap: wrap;
             padding: 1rem 0; border-top: 1px solid #e5e5e7; border-bottom: 1px solid #e5e5e7; }
    .percentage { font-size: 3rem; font-weight: 700; letter-spacing: -.04em; color: #d92121; }
    .percentage span { font-size: 1.25rem; color: #86868b; font-weight: 500; margin-left: .15rem; }
    .fraction { font-size: 1.1rem; font-weight: 600; }
    .fraction-label { font-size: .8rem; color: #86868b; }
    .period { font-size: .85rem; color: #86868b; margin: 1rem 0 0; }
    h3 { font-size: .9rem; text-transform: uppercase; letter-spacing: .05em;
         color: #515154; margin: 1.5rem 0 .5rem; }
    .gaps { list-style: none; margin: 0; padding: 0; }
    .gaps li { display: flex; align-items: baseline; gap: .9rem;
               padding: .55rem 0; border-bottom: 1px solid #f0f0f3; font-size: .92rem; }
    .gaps li:last-child { border-bottom: 0; }
    .pid { font-family: ui-monospace, Menlo, monospace; color: #1d1d1f; font-weight: 600; }
    .meta { color: #86868b; font-size: .85rem; }
    .hba1c { margin-left: auto; color: #515154; }
    .hba1c strong { color: #d92121; margin: 0 .15rem 0 .25rem; }
    .hba1c small { color: #86868b; }
    .spec { display: inline-block; margin-top: 1.25rem; color: #1170d2; text-decoration: none;
            font-size: .9rem; font-weight: 500; }
    .spec:hover { text-decoration: underline; }
  `],
})
export class AppComponent implements OnInit {
  private readonly measures = inject(MeasureService);
  measure: MeasureResult | null = null;
  loading = true;
  error: string | null = null;

  ngOnInit(): void {
    this.measures.getCms122().subscribe({
      next: (m) => {
        this.measure = m;
        this.loading = false;
      },
      error: (err: Error) => {
        this.error = err.message ?? 'Unknown error';
        this.loading = false;
      },
    });
  }
}
