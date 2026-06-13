/**
 * Heatmap cell colouring. The CMS quality world reports scores 0-100 %,
 * with the "good" direction varying by measure. This module returns a
 * background colour for a cell given the cell's percentage and the
 * measure's direction, plus a contrasting foreground colour for text.
 *
 * The palette is a calibrated three-stop gradient (red → amber → green)
 * tuned to land cleanly on the Apple-Health-style off-white surface
 * the dashboard sits on. Lower-is-better measures invert the gradient.
 */

export type MeasureDirection = 'lower-is-better' | 'higher-is-better';

const GOOD: [number, number, number] = [31, 138, 71];   // #1f8a47
const MID:  [number, number, number] = [194, 94, 4];    // #c25e04
const BAD:  [number, number, number] = [217, 33, 33];   // #d92121

const GOOD_SOFT: [number, number, number] = [223, 240, 229]; // very pale green
const MID_SOFT:  [number, number, number] = [254, 230, 196];
const BAD_SOFT:  [number, number, number] = [253, 220, 220];

function lerp(a: number, b: number, t: number): number {
  return a + (b - a) * t;
}

function mixRgb(
  c1: [number, number, number],
  c2: [number, number, number],
  t: number,
): [number, number, number] {
  return [lerp(c1[0], c2[0], t), lerp(c1[1], c2[1], t), lerp(c1[2], c2[2], t)];
}

function toCss(c: [number, number, number]): string {
  return `rgb(${Math.round(c[0])} ${Math.round(c[1])} ${Math.round(c[2])})`;
}

export function cellBackground(percent: number, direction: MeasureDirection, hasData: boolean): string {
  if (!hasData) return '#f5f5f7';
  // Normalise so 0 = bad, 1 = good regardless of direction.
  const goodness = direction === 'higher-is-better' ? percent / 100 : 1 - percent / 100;
  const t = Math.max(0, Math.min(1, goodness));
  const [r, g, b] = t < 0.5
    ? mixRgb(BAD_SOFT, MID_SOFT, t * 2)
    : mixRgb(MID_SOFT, GOOD_SOFT, (t - 0.5) * 2);
  return toCss([r, g, b]);
}

export function cellForeground(percent: number, direction: MeasureDirection, hasData: boolean): string {
  if (!hasData) return '#86868b';
  const goodness = direction === 'higher-is-better' ? percent / 100 : 1 - percent / 100;
  const t = Math.max(0, Math.min(1, goodness));
  const [r, g, b] = t < 0.5
    ? mixRgb(BAD, MID, t * 2)
    : mixRgb(MID, GOOD, (t - 0.5) * 2);
  return toCss([r, g, b]);
}
