import { CommonModule } from '@angular/common';
import { Component, OnInit, inject } from '@angular/core';
import { GapListComponent } from './gap-list.component';
import { HistoryEntry, Measure, MeasureService, Scorecard } from './measure.service';
import { ScorecardComponent } from './scorecard.component';

@Component({
  selector: 'fqp-root',
  standalone: true,
  imports: [CommonModule, ScorecardComponent, GapListComponent],
  template: `
    <header class="page">
      <div class="page-inner">
        <h1>FHIR Quality Platform</h1>
        <p class="subline">
          Population eCQM scorecard over a deterministic 100-patient cohort, computed end-to-end
          from FHIR resources: Go ingests + archives, NATS routes, Python computes, Spring serves
          metadata, Node aggregates the BFF, Angular renders.
        </p>
      </div>
    </header>

    <main class="layout">
      @if (loading) {
        <section class="card placeholder">Loading measures…</section>
      }
      @if (error) {
        <section class="card error">
          <h2>Couldn't load scorecard</h2>
          <p>{{ error }}</p>
        </section>
      }

      @if (scorecard && !error) {
        <fqp-scorecard [data]="scorecard" (selected)="onCellSelected($event)"></fqp-scorecard>
      }

      @if (selectedMeasure) {
        <fqp-gap-list
          [measure]="selectedMeasure"
          [providerFilter]="selectedProvider"
          [history]="selectedHistory"
          (closed)="closeDetail()"
        ></fqp-gap-list>
      } @else if (scorecard) {
        <section class="hint card">
          <strong>Click any cell</strong> in the scorecard above to drill into the measure detail
          and the underlying gap-patient list. Click the row's "Overall" column to see every gap,
          or any provider column to filter to that provider's panel.
        </section>
      }
    </main>
  `,
  styles: [`
    .page { background: linear-gradient(180deg, #fff 0%, #f5f5f7 100%);
            border-bottom: 1px solid #e5e5e7; }
    .page-inner { max-width: 1180px; margin: 0 auto; padding: 2rem 1.5rem 1.5rem; }
    h1 { font-size: 1.85rem; letter-spacing: -.02em; margin: 0 0 .35rem; }
    .subline { color: #515154; margin: 0; max-width: 80ch; font-size: .92rem; line-height: 1.5; }
    .layout { max-width: 1180px; margin: 1.5rem auto 3rem; padding: 0 1.5rem;
              display: grid; gap: 1rem; }
    .card { background: #fff; border-radius: 16px; padding: 1rem 1.25rem;
            box-shadow: 0 1px 2px rgba(0,0,0,.04); }
    .placeholder { color: #86868b; }
    .error { background: #fde8e8; color: #d92121; }
    .hint { color: #515154; font-size: .92rem; line-height: 1.5; }
    .hint strong { color: #1d1d1f; }
  `],
})
export class AppComponent implements OnInit {
  private readonly measures = inject(MeasureService);

  scorecard: Scorecard | null = null;
  selectedMeasure: Measure | null = null;
  selectedProvider: string | null = null;
  selectedHistory: HistoryEntry[] = [];
  loading = true;
  error: string | null = null;

  ngOnInit(): void {
    this.measures.getScorecard().subscribe({
      next: s => {
        this.scorecard = s;
        this.loading = false;
      },
      error: (err: Error) => {
        this.error = err.message ?? 'Unknown error';
        this.loading = false;
      },
    });
  }

  onCellSelected(event: { measureId: string; providerId: string | null }): void {
    this.selectedProvider = event.providerId;
    this.selectedHistory = [];
    this.measures.getMeasure(event.measureId).subscribe({
      next: m => { this.selectedMeasure = m; },
      error: (err: Error) => { this.error = err.message ?? 'Unknown error'; },
    });
    this.measures.getHistory(event.measureId).subscribe({
      next: h => { this.selectedHistory = h.points; },
      error: () => { /* sparkline is optional; ignore */ },
    });
  }

  closeDetail(): void {
    this.selectedMeasure = null;
    this.selectedProvider = null;
    this.selectedHistory = [];
  }
}
