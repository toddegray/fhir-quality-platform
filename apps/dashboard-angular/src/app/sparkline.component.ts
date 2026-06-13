import { CommonModule } from '@angular/common';
import { Component, Input } from '@angular/core';

export interface HistoryPoint {
  periodEnd: string;
  percentage: number;
  numerator: number;
  denominator: number;
}

/**
 * Inline-SVG sparkline. Single polyline + start/end markers, no axes —
 * pure trend communication. The endpoint label shows the latest score
 * so the eye doesn't have to chase the marker.
 */
@Component({
  selector: 'fqp-sparkline',
  standalone: true,
  imports: [CommonModule],
  template: `
    @if (points.length > 0) {
      <div class="wrap">
        <svg [attr.viewBox]="viewBox" preserveAspectRatio="none" class="spark">
          <polyline
            [attr.points]="line"
            fill="none"
            stroke="currentColor"
            stroke-width="1.5"
            stroke-linejoin="round"
            stroke-linecap="round"
          ></polyline>
          <circle [attr.cx]="lastX" [attr.cy]="lastY" r="2.4" fill="currentColor"></circle>
        </svg>
        <div class="endpoints">
          <span class="lo">{{ points[0].periodEnd | slice:0:7 }}</span>
          <span class="hi">latest {{ points[points.length - 1].percentage | number:'1.1-1' }}%</span>
        </div>
      </div>
    }
  `,
  styles: [`
    .wrap { display: flex; flex-direction: column; gap: .15rem; }
    .spark { width: 100%; height: 36px; color: #1170d2; }
    .endpoints { display: flex; justify-content: space-between;
                 font-size: .68rem; color: #86868b; }
    .endpoints .hi { color: #515154; font-weight: 600; }
  `],
})
export class SparklineComponent {
  @Input() set data(value: HistoryPoint[]) {
    this.points = value ?? [];
    this.recompute();
  }
  points: HistoryPoint[] = [];
  viewBox = '0 0 100 30';
  line = '';
  lastX = 0;
  lastY = 0;

  private recompute(): void {
    if (this.points.length === 0) return;
    const minPct = Math.min(...this.points.map(p => p.percentage));
    const maxPct = Math.max(...this.points.map(p => p.percentage));
    const range = Math.max(maxPct - minPct, 4); // floor to keep horizontals visible
    const xStep = 100 / Math.max(this.points.length - 1, 1);
    const coords = this.points.map((p, i) => {
      const x = i * xStep;
      const y = 28 - ((p.percentage - minPct) / range) * 24;
      return { x, y };
    });
    this.line = coords.map(c => `${c.x.toFixed(2)},${c.y.toFixed(2)}`).join(' ');
    const last = coords[coords.length - 1];
    if (last) {
      this.lastX = last.x;
      this.lastY = last.y;
    }
  }
}
