"""Measure registry. Every eCQM module here exposes a uniform
:class:`MeasureResult` plus a metadata block, and the registry below
lets the FastAPI layer iterate over the catalog without knowing
each measure's internal logic.
"""
from __future__ import annotations

import math
from dataclasses import dataclass
from datetime import date
from typing import Callable

from .cms122 import compute as compute_cms122
from .cms122 import Cms122Result
from .state import PatientStore
from .value_sets import CMS117_CVX_CODES


# ----- shared result shape ---------------------------------------------------


@dataclass
class GapPatient:
    patient_id: str
    age: int
    provider_id: str | None
    detail: str


@dataclass
class MeasureScore:
    measure_id: str
    title: str
    description: str
    direction: str          # "lower-is-better" or "higher-is-better"
    measurement_period: tuple[date, date]
    denominator: int
    numerator: int
    percentage: float       # rounded to 1 decimal
    gap_patients: list[GapPatient]
    provider_breakdown: dict[str, tuple[int, int]]  # provider_id -> (numerator, denominator)


# ----- CMS122 adapter (wraps the existing module) ----------------------------


def _cms122_score(store: PatientStore) -> MeasureScore:
    raw = compute_cms122(store)
    return _build_score_from_cms122(store, raw)


def _build_score_from_cms122(store: PatientStore, raw: Cms122Result) -> MeasureScore:
    gaps: list[GapPatient] = []
    by_provider: dict[str, list[int]] = {}
    for p in store.snapshot():
        age = p.age_at(raw.measurement_period_start)
        if age is None or age < 18 or age > 75 or not p.has_diabetes:
            continue
        if not p.had_encounter_in(raw.measurement_period_start, raw.measurement_period_end):
            continue
        slot = by_provider.setdefault(p.provider_id or "unassigned", [0, 0])
        slot[1] += 1
        in_period = [r for r in p.hba1c_readings if raw.measurement_period_start <= r.effective_date <= raw.measurement_period_end]
        if in_period:
            latest = max(in_period, key=lambda r: r.effective_date)
            if latest.value_percent > 9.0:
                slot[0] += 1
                gaps.append(GapPatient(
                    patient_id=p.patient_id, age=age, provider_id=p.provider_id,
                    detail=f"Most recent HbA1c {latest.value_percent:.1f}% on {latest.effective_date.isoformat()}"
                ))
        else:
            slot[0] += 1
            gaps.append(GapPatient(
                patient_id=p.patient_id, age=age, provider_id=p.provider_id,
                detail="No HbA1c lab recorded during measurement period"
            ))
    return MeasureScore(
        measure_id="CMS122",
        title="Diabetes: Hemoglobin A1c (HbA1c) Poor Control (> 9 %)",
        description="Percentage of patients aged 18-75 with diabetes whose most recent HbA1c is greater than 9 %.",
        direction="lower-is-better",
        measurement_period=(raw.measurement_period_start, raw.measurement_period_end),
        denominator=raw.denominator,
        numerator=raw.numerator,
        percentage=raw.percentage,
        gap_patients=gaps,
        provider_breakdown={pid: (n, d) for pid, (n, d) in by_provider.items()},
    )


# ----- CMS125 — Breast Cancer Screening (Mammography) ------------------------


def _cms125_score(store: PatientStore) -> MeasureScore:
    period_start = date(2025, 1, 1)
    period_end = date(2025, 12, 31)
    # CMS125 looks back 27 months from the end of the measurement period.
    lookback_start = date(period_end.year - 2, period_end.month, 1)
    denominator = 0
    numerator = 0
    gaps: list[GapPatient] = []
    by_provider: dict[str, list[int]] = {}
    for p in store.snapshot():
        age = p.age_at(period_start)
        if age is None or p.gender != "female" or age < 50 or age > 74:
            continue
        if not p.had_encounter_in(period_start, period_end):
            continue
        denominator += 1
        slot = by_provider.setdefault(p.provider_id or "unassigned", [0, 0])
        slot[1] += 1
        if p.had_mammography_in(lookback_start, period_end):
            numerator += 1
            slot[0] += 1
        else:
            gaps.append(GapPatient(
                patient_id=p.patient_id, age=age, provider_id=p.provider_id,
                detail="No screening mammography in the last 27 months",
            ))
    return MeasureScore(
        measure_id="CMS125",
        title="Breast Cancer Screening (Mammography)",
        description="Percentage of women 50-74 with one or more mammograms in the last 27 months.",
        direction="higher-is-better",
        measurement_period=(period_start, period_end),
        denominator=denominator,
        numerator=numerator,
        percentage=round((numerator / denominator * 100.0) if denominator else 0.0, 1),
        gap_patients=gaps,
        provider_breakdown={pid: (n, d) for pid, (n, d) in by_provider.items()},
    )


# ----- CMS165 — Controlling High Blood Pressure ------------------------------


def _cms165_score(store: PatientStore) -> MeasureScore:
    period_start = date(2025, 1, 1)
    period_end = date(2025, 12, 31)
    denominator = 0
    numerator = 0
    gaps: list[GapPatient] = []
    by_provider: dict[str, list[int]] = {}
    for p in store.snapshot():
        age = p.age_at(period_start)
        if age is None or age < 18 or age > 85 or not p.has_hypertension:
            continue
        if not p.had_encounter_in(period_start, period_end):
            continue
        denominator += 1
        slot = by_provider.setdefault(p.provider_id or "unassigned", [0, 0])
        slot[1] += 1
        in_period = [r for r in p.bp_readings if period_start <= r.effective_date <= period_end]
        latest = max(in_period, key=lambda r: r.effective_date, default=None)
        if latest is not None and latest.controlled():
            numerator += 1
            slot[0] += 1
        else:
            if latest is None:
                detail = "No blood-pressure reading recorded during measurement period"
            else:
                detail = (
                    f"Most recent BP {int(latest.systolic_mmhg)}/{int(latest.diastolic_mmhg)} mmHg on "
                    f"{latest.effective_date.isoformat()}"
                )
            gaps.append(GapPatient(
                patient_id=p.patient_id, age=age, provider_id=p.provider_id, detail=detail,
            ))
    return MeasureScore(
        measure_id="CMS165",
        title="Controlling High Blood Pressure",
        description="Percentage of hypertensive patients 18-85 whose most recent BP is < 140/90 mmHg.",
        direction="higher-is-better",
        measurement_period=(period_start, period_end),
        denominator=denominator,
        numerator=numerator,
        percentage=round((numerator / denominator * 100.0) if denominator else 0.0, 1),
        gap_patients=gaps,
        provider_breakdown={pid: (n, d) for pid, (n, d) in by_provider.items()},
    )


# ----- CMS117 — Childhood Immunization Status --------------------------------


def _cms117_score(store: PatientStore) -> MeasureScore:
    period_start = date(2025, 1, 1)
    period_end = date(2025, 12, 31)
    # CMS117 denominator = children who turn 2 during the measurement
    # period. That means birth_date sits two calendar years before the
    # period — Jan 2023 babies turn 2 in Jan 2025, Dec 2023 babies turn 2
    # in Dec 2025. Working off birth_date is unambiguous; age_at()
    # bucketing at period_start systematically misses Q1-born kids.
    birth_start = date(period_start.year - 2, period_start.month, period_start.day)
    birth_end = date(period_end.year - 2, period_end.month, period_end.day)
    required = CMS117_CVX_CODES
    denominator = 0
    numerator = 0
    gaps: list[GapPatient] = []
    by_provider: dict[str, list[int]] = {}
    for p in store.snapshot():
        if p.birth_date is None or not (birth_start <= p.birth_date <= birth_end):
            continue
        age = p.age_at(period_end) or 0
        denominator += 1
        slot = by_provider.setdefault(p.provider_id or "unassigned", [0, 0])
        slot[1] += 1
        missing = sorted(required - p.immunization_codes)
        if not missing:
            numerator += 1
            slot[0] += 1
        else:
            gaps.append(GapPatient(
                patient_id=p.patient_id, age=age, provider_id=p.provider_id,
                detail="Missing CVX " + ", ".join(missing),
            ))
    return MeasureScore(
        measure_id="CMS117",
        title="Childhood Immunization Status",
        description="Percentage of 2-year-olds with the combo-10 vaccine series complete by age two.",
        direction="higher-is-better",
        measurement_period=(period_start, period_end),
        denominator=denominator,
        numerator=numerator,
        percentage=round((numerator / denominator * 100.0) if denominator else 0.0, 1),
        gap_patients=gaps,
        provider_breakdown={pid: (n, d) for pid, (n, d) in by_provider.items()},
    )


# ----- registry --------------------------------------------------------------


MEASURES: dict[str, Callable[[PatientStore], MeasureScore]] = {
    "CMS122": _cms122_score,
    "CMS125": _cms125_score,
    "CMS165": _cms165_score,
    "CMS117": _cms117_score,
}


def compute_all(store: PatientStore) -> list[MeasureScore]:
    return [fn(store) for _, fn in MEASURES.items()]


def serialize(score: MeasureScore) -> dict[str, object]:
    return {
        "measureId": score.measure_id,
        "title": score.title,
        "description": score.description,
        "direction": score.direction,
        "measurementPeriod": {
            "start": score.measurement_period[0].isoformat(),
            "end": score.measurement_period[1].isoformat(),
        },
        "denominator": score.denominator,
        "numerator": score.numerator,
        "percentage": score.percentage,
        "gapPatients": [
            {
                "patientId": g.patient_id,
                "age": g.age,
                "providerId": g.provider_id,
                "detail": g.detail,
            }
            for g in score.gap_patients
        ],
        "providerBreakdown": [
            {
                "providerId": pid,
                "numerator": n,
                "denominator": d,
                "percentage": round((n / d * 100.0) if d else 0.0, 1),
            }
            for pid, (n, d) in sorted(score.provider_breakdown.items())
        ],
    }


def _coerce(value: float) -> float:
    """Convert NaN to 0.0 in case downstream JSON serialisation chokes."""
    return 0.0 if math.isnan(value) else value
