import { CommonModule } from '@angular/common';
import { Component, EventEmitter, Input, Output } from '@angular/core';
import type { HistoryEntry, Measure } from './measure.service';
import { SparklineComponent } from './sparkline.component';

/**
 * Detail panel for one measure (optionally narrowed to one provider).
 * Renders the score banner, gap-patient list, and the citation strip
 * (code sets + eCQI specification link) the Spring core supplies.
 */
@Component({
  selector: 'fqp-gap-list',
  standalone: true,
  imports: [CommonModule, SparklineComponent],
  template: `
    @if (measure) {
      <section class="detail">
        <header>
          <button class="close" (click)="closed.emit()" title="Close">×</button>
          <div class="mid">{{ measure.measureId }}</div>
          <h2>{{ measure.title }}</h2>
          <p class="desc">{{ measure.description }}</p>
          @if (providerFilter) {
            <p class="scoped">Filtered to <strong>{{ providerFilter }}</strong></p>
          }
        </header>

        <div class="score-bar">
          <div class="num">
            {{ filteredPercentage() | number:'1.1-1' }}<span>%</span>
          </div>
          <div class="frac">{{ filteredNumerator() }} / {{ filteredDenominator() }}</div>
          <div class="dir">{{ measure.direction === 'lower-is-better' ? 'lower is better' : 'higher is better' }}</div>
        </div>

        @if (history && history.length > 0) {
          <div class="trend">
            <span class="trend-label">12-month trend</span>
            <fqp-sparkline [data]="history" class="spark"></fqp-sparkline>
          </div>
        }

        @if (filteredGaps().length > 0) {
          <h3>Care gaps · {{ filteredGaps().length }}</h3>
          <ul class="gaps">
            @for (g of filteredGaps(); track g.patientId) {
              <li>
                <span class="pid">{{ g.patientId }}</span>
                <span class="meta">age {{ g.age }} · {{ g.providerId || 'unassigned' }}</span>
                <span class="dl">{{ g.detail }}</span>
              </li>
            }
          </ul>
        } @else {
          <p class="empty">No open care gaps in this slice.</p>
        }

        <footer>
          @if (measure.diagnosisCodes && measure.diagnosisCodes.length > 0) {
            <div class="codes">
              <span class="codes-label">Diagnosis codes:</span>
              @for (c of measure.diagnosisCodes; track c) {
                <code>{{ c }}</code>
              }
            </div>
          }
          @if (measure.labCodes && measure.labCodes.length > 0) {
            <div class="codes">
              <span class="codes-label">Observation codes:</span>
              @for (c of measure.labCodes; track c) {
                <code>{{ c }}</code>
              }
            </div>
          }
          @if (measure.reference) {
            <a class="spec" [href]="measure.reference" target="_blank" rel="noopener">eCQI specification ↗</a>
          }
        </footer>
      </section>
    }
  `,
  styles: [`
    .detail { background: #fff; border-radius: 16px; padding: 1.25rem 1.5rem;
              box-shadow: 0 1px 2px rgba(0,0,0,.04); position: relative; }
    .close { position: absolute; top: .9rem; right: 1.1rem; background: transparent; border: 0;
             font-size: 1.4rem; color: #86868b; cursor: pointer; line-height: 1; }
    .close:hover { color: #1d1d1f; }
    .mid { display: inline-block; font-family: ui-monospace, Menlo, monospace;
           background: #f2f2f5; color: #515154; padding: .15rem .55rem; border-radius: 4px;
           font-size: .72rem; margin-bottom: .35rem; }
    h2 { font-size: 1.2rem; letter-spacing: -0.01em; margin: 0 0 .35rem; }
    .desc { color: #515154; margin: 0 0 .35rem; font-size: .92rem; line-height: 1.5; max-width: 60ch; }
    .scoped { color: #86868b; font-size: .82rem; margin: 0 0 .5rem; }
    .score-bar { display: flex; gap: 1.5rem; align-items: baseline; padding: 1rem 0;
                 border-top: 1px solid #e5e5e7; border-bottom: 1px solid #e5e5e7; margin: 0 0 1rem; }
    .num { font-size: 2.5rem; font-weight: 700; letter-spacing: -.04em; color: #1d1d1f; }
    .num span { font-size: 1.05rem; color: #86868b; font-weight: 500; margin-left: .1rem; }
    .frac { font-size: 1rem; color: #515154; font-weight: 600; }
    .dir { font-size: .78rem; color: #86868b; text-transform: uppercase; letter-spacing: .05em;
           margin-left: auto; }
    .trend { margin: 0 0 1rem; display: grid; grid-template-columns: 100px 1fr; align-items: center; gap: 1rem; }
    .trend-label { font-size: .72rem; text-transform: uppercase; color: #86868b;
                   letter-spacing: .05em; font-weight: 600; }
    .spark { display: block; }
    h3 { font-size: .82rem; text-transform: uppercase; color: #86868b; letter-spacing: .05em;
         font-weight: 600; margin: 0 0 .5rem; }
    .empty { color: #86868b; font-size: .9rem; }
    .gaps { list-style: none; padding: 0; margin: 0; }
    .gaps li { display: flex; align-items: baseline; gap: .9rem;
               padding: .55rem 0; border-bottom: 1px solid #f0f0f3; font-size: .9rem; }
    .gaps li:last-child { border-bottom: 0; }
    .pid { font-family: ui-monospace, Menlo, monospace; font-weight: 600; min-width: 5rem; }
    .meta { color: #86868b; min-width: 11rem; font-size: .82rem; }
    .dl { color: #515154; flex: 1; }
    footer { margin-top: 1rem; padding-top: 1rem; border-top: 1px solid #e5e5e7;
             display: flex; flex-wrap: wrap; align-items: center; gap: .75rem;
             font-size: .82rem; }
    .codes { color: #86868b; }
    .codes-label { color: #515154; font-weight: 600; margin-right: .35rem; }
    code { background: #f2f2f5; padding: .1rem .4rem; border-radius: 4px;
           font-family: ui-monospace, Menlo, monospace; font-size: .78rem; margin-right: .25rem; }
    .spec { color: #1170d2; text-decoration: none; font-weight: 500; margin-left: auto; }
    .spec:hover { text-decoration: underline; }
  `],
})
export class GapListComponent {
  @Input() measure: Measure | null = null;
  @Input() providerFilter: string | null = null;
  @Input() history: HistoryEntry[] = [];
  @Output() closed = new EventEmitter<void>();

  filteredGaps(): Measure['gapPatients'] {
    if (!this.measure) return [];
    if (!this.providerFilter) return this.measure.gapPatients;
    return this.measure.gapPatients.filter(g => g.providerId === this.providerFilter);
  }

  filteredNumerator(): number {
    if (!this.measure) return 0;
    if (!this.providerFilter) return this.measure.numerator;
    const row = this.measure.providerBreakdown.find(r => r.providerId === this.providerFilter);
    return row?.numerator ?? 0;
  }

  filteredDenominator(): number {
    if (!this.measure) return 0;
    if (!this.providerFilter) return this.measure.denominator;
    const row = this.measure.providerBreakdown.find(r => r.providerId === this.providerFilter);
    return row?.denominator ?? 0;
  }

  filteredPercentage(): number {
    const d = this.filteredDenominator();
    return d > 0 ? Math.round((this.filteredNumerator() / d) * 1000) / 10 : 0;
  }
}
