import { CommonModule } from '@angular/common';
import { Component, EventEmitter, Input, Output } from '@angular/core';
import { cellBackground, cellForeground } from './cell-color';
import type { Scorecard, ScorecardRow } from './measure.service';

/**
 * Provider × measure heatmap. Each row is one eCQM, each column one
 * provider; cell shade encodes the score relative to the measure's
 * direction (lower-is-better measures invert the gradient). Click a
 * cell to bubble the (measure, provider) pair so the parent can show
 * the drill-down.
 */
@Component({
  selector: 'fqp-scorecard',
  standalone: true,
  imports: [CommonModule],
  template: `
    @if (data) {
      <div class="scorecard">
        <table>
          <thead>
            <tr>
              <th class="rowhead">Measure</th>
              <th class="overall">Overall</th>
              @for (p of data.providers; track p) {
                <th class="provider">{{ p }}</th>
              }
            </tr>
          </thead>
          <tbody>
            @for (m of data.measures; track m.measureId) {
              <tr>
                <th class="rowhead">
                  <span class="mid">{{ m.measureId }}</span>
                  <span class="mtitle">{{ m.title }}</span>
                </th>
                <td
                  class="cell overall"
                  [style.background]="bg(m.overallPercentage, m.direction, m.overallDenominator)"
                  [style.color]="fg(m.overallPercentage, m.direction, m.overallDenominator)"
                  (click)="selectMeasure(m, null)"
                >
                  @if (m.overallDenominator > 0) {
                    <strong>{{ m.overallPercentage | number:'1.1-1' }}%</strong>
                    <small>{{ m.overallNumerator }}/{{ m.overallDenominator }}</small>
                  } @else {
                    <span class="empty">—</span>
                  }
                </td>
                @for (c of m.cells; track c.providerId) {
                  <td
                    class="cell"
                    [style.background]="bg(c.percentage, m.direction, c.denominator)"
                    [style.color]="fg(c.percentage, m.direction, c.denominator)"
                    (click)="selectMeasure(m, c.providerId)"
                  >
                    @if (c.denominator > 0) {
                      <strong>{{ c.percentage | number:'1.0-0' }}%</strong>
                      <small>{{ c.numerator }}/{{ c.denominator }}</small>
                    } @else {
                      <span class="empty">—</span>
                    }
                  </td>
                }
              </tr>
            }
          </tbody>
        </table>
        <p class="legend">
          <span class="swatch good"></span> better &nbsp;
          <span class="swatch mid"></span> watch &nbsp;
          <span class="swatch bad"></span> needs attention &nbsp;
          <span class="swatch empty"></span> no denominator
          <span class="hint">click a cell for the gap list</span>
        </p>
      </div>
    }
  `,
  styles: [`
    .scorecard { background: #fff; border-radius: 16px; padding: 1rem 1.25rem 1.5rem;
                 box-shadow: 0 1px 2px rgba(0,0,0,.04); }
    table { width: 100%; border-collapse: separate; border-spacing: .35rem; table-layout: fixed; }
    th.rowhead { text-align: left; font-weight: 500; color: #1d1d1f; width: 40%;
                 padding: .55rem .25rem; vertical-align: top; font-size: .92rem; }
    th.rowhead .mid { display: inline-block; font-family: ui-monospace, Menlo, monospace;
                      font-size: .72rem; background: #f2f2f5; color: #515154;
                      padding: .15rem .45rem; border-radius: 4px; margin-right: .5rem; }
    th.rowhead .mtitle { color: #515154; }
    th.provider, th.overall { font-size: .7rem; font-weight: 600; text-transform: uppercase;
                              color: #86868b; letter-spacing: .04em; text-align: center;
                              padding: .35rem 0; }
    th.overall { font-weight: 700; color: #1d1d1f; }
    td.cell { border-radius: 8px; padding: .55rem .35rem; text-align: center; cursor: pointer;
              transition: transform .1s ease; min-width: 4.5rem; }
    td.cell:hover { transform: scale(1.04); }
    td.cell strong { display: block; font-size: 1rem; line-height: 1.2; }
    td.cell small { display: block; font-size: .68rem; color: rgba(0,0,0,.55); margin-top: .15rem; }
    td.cell.overall { font-weight: 700; }
    td.cell .empty { color: #86868b; font-weight: 600; }
    .legend { color: #86868b; font-size: .8rem; margin: 1rem 0 0; display: flex; flex-wrap: wrap; gap: .75rem; align-items: center; }
    .swatch { display: inline-block; width: 14px; height: 14px; border-radius: 4px;
              vertical-align: middle; margin-right: .25rem; }
    .swatch.good  { background: #dff0e5; }
    .swatch.mid   { background: #fee6c4; }
    .swatch.bad   { background: #fddcdc; }
    .swatch.empty { background: #f5f5f7; border: 1px solid #e5e5e7; }
    .hint { margin-left: auto; color: #1170d2; }
  `],
})
export class ScorecardComponent {
  @Input() data: Scorecard | null = null;
  @Output() selected = new EventEmitter<{ measureId: string; providerId: string | null }>();

  bg(p: number, d: ScorecardRow['direction'], denom: number): string {
    return cellBackground(p, d, denom > 0);
  }
  fg(p: number, d: ScorecardRow['direction'], denom: number): string {
    return cellForeground(p, d, denom > 0);
  }

  selectMeasure(m: ScorecardRow, providerId: string | null): void {
    this.selected.emit({ measureId: m.measureId, providerId });
  }
}
