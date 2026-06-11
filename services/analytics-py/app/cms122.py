"""CMS122 — Diabetes: Hemoglobin A1c (HbA1c) Poor Control (>9%).

Computes the measure on demand from the current :class:`PatientStore`
state. The numerator/denominator semantics follow the CMS122 measure
specification at a level appropriate for the MVP:

* **Initial population / denominator** — patients aged 18 to 75 at the
  start of the measurement period who have an active diabetes diagnosis
  and at least one encounter during the period.
* **Numerator** — patients in the denominator whose most recent HbA1c
  result during the measurement period is greater than 9 %.

The score is the *percentage of poor control*: numerator / denominator
× 100. Lower is better.
"""
from __future__ import annotations

from dataclasses import dataclass
from datetime import date

from .state import PatientStore

CMS122_AGE_MIN = 18
CMS122_AGE_MAX = 75
CMS122_POOR_CONTROL_THRESHOLD = 9.0  # percent

DEFAULT_PERIOD_START = date(2025, 1, 1)
DEFAULT_PERIOD_END = date(2025, 12, 31)


@dataclass
class PatientGap:
    patient_id: str
    age: int
    latest_hba1c: float
    latest_hba1c_date: date


@dataclass
class Cms122Result:
    measurement_period_start: date
    measurement_period_end: date
    denominator: int
    numerator: int
    percentage: float
    gap_patients: list[PatientGap]


def compute(
    store: PatientStore,
    period_start: date = DEFAULT_PERIOD_START,
    period_end: date = DEFAULT_PERIOD_END,
) -> Cms122Result:
    denominator = 0
    numerator = 0
    gaps: list[PatientGap] = []

    for patient in store.snapshot():
        age = patient.age_at(period_start)
        if age is None or age < CMS122_AGE_MIN or age > CMS122_AGE_MAX:
            continue
        if not patient.has_diabetes:
            continue
        if not patient.had_encounter_in(period_start, period_end):
            continue

        denominator += 1

        # Use the most recent HbA1c within the measurement period; an
        # absent reading counts as poor control per CMS122 spec.
        in_period = [r for r in patient.hba1c_readings if period_start <= r.effective_date <= period_end]
        if in_period:
            latest = max(in_period, key=lambda r: r.effective_date)
            if latest.value_percent > CMS122_POOR_CONTROL_THRESHOLD:
                numerator += 1
                gaps.append(
                    PatientGap(
                        patient_id=patient.patient_id,
                        age=age,
                        latest_hba1c=latest.value_percent,
                        latest_hba1c_date=latest.effective_date,
                    )
                )
        else:
            numerator += 1  # missing recent HbA1c == poor control
            gaps.append(
                PatientGap(
                    patient_id=patient.patient_id,
                    age=age,
                    latest_hba1c=float("nan"),
                    latest_hba1c_date=period_start,
                )
            )

    percentage = (numerator / denominator * 100.0) if denominator > 0 else 0.0
    return Cms122Result(
        measurement_period_start=period_start,
        measurement_period_end=period_end,
        denominator=denominator,
        numerator=numerator,
        percentage=round(percentage, 2),
        gap_patients=gaps,
    )
