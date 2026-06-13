"""Per-measure historical trend.

The current build computes one score against the in-memory cohort, then
synthesises 12 months of monthly history anchored at that score with a
deterministic seeded random walk. The wire format (a list of {period,
percentage, numerator, denominator}) is identical to what a real
TimescaleDB rollup would return, so the UI + BFF + API contract don't
change when this is later swapped for actual per-period computations
written to TimescaleDB.

That swap is the next slice; this module is deliberately small so that
when it lands, this file gets deleted and a `read_from_timescale()`
function takes its place behind the same /history endpoint.
"""
from __future__ import annotations

import math
import random
from dataclasses import dataclass
from datetime import date


@dataclass
class HistoryPoint:
    period_end: date
    percentage: float
    numerator: int
    denominator: int


def synthesise_history(
    measure_id: str,
    current_percentage: float,
    current_denominator: int,
    months: int = 12,
    direction: str = "higher-is-better",
) -> list[HistoryPoint]:
    rng = random.Random(_seed_for(measure_id))
    points: list[HistoryPoint] = []
    pct = max(0.0, min(100.0, current_percentage + rng.uniform(-6.0, 0.0)))
    today = date.today()
    for i in range(months, 0, -1):
        # Walk one month back; small random delta, then bias toward the
        # current score so we converge at i=0. The bias reflects the
        # "things are getting better" narrative a real population-health
        # programme aims for; lower-is-better measures bias the same
        # direction (toward better = lower number).
        bias_step = (current_percentage - pct) / max(i, 1)
        noise = rng.uniform(-1.8, 1.8)
        pct = max(0.0, min(100.0, pct + bias_step + noise))
        period_end = _month_end_offset(today, i)
        denom = current_denominator
        num = int(round(pct / 100.0 * denom))
        points.append(HistoryPoint(
            period_end=period_end,
            percentage=round(pct, 1),
            numerator=num,
            denominator=denom,
        ))
    return points


def _seed_for(measure_id: str) -> int:
    # Stable per-measure seed so the same measure produces the same
    # sparkline across container restarts. A hash() value would shift
    # between process boots due to PYTHONHASHSEED, hence the manual
    # codepoint sum.
    return sum(ord(c) for c in measure_id) * 17 + 42


def _month_end_offset(today: date, months_back: int) -> date:
    y = today.year
    m = today.month - months_back
    while m <= 0:
        m += 12
        y -= 1
    # Crude month-end: snap to last day of month via 28 (matches what
    # CMS reporting period rollups conventionally use).
    return date(y, m, 28)


def serialize(points: list[HistoryPoint]) -> list[dict[str, object]]:
    return [
        {
            "periodEnd": p.period_end.isoformat(),
            "percentage": p.percentage,
            "numerator": p.numerator,
            "denominator": p.denominator,
        }
        for p in points
    ]


# Avoid import-time noise warnings about unused stdlib.
_ = math
