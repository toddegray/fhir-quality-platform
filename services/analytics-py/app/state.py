"""In-memory patient state assembled from inbound FHIR resources.

The MVP keeps every patient in a single in-process dict; later iterations
replace this with TimescaleDB-backed materialized state.
"""
from __future__ import annotations

from dataclasses import dataclass, field
from datetime import date, datetime
from threading import RLock


def parse_date(value: str | None) -> date | None:
    """Parse a FHIR date / dateTime string into a calendar date.

    Accepts both ``YYYY-MM-DD`` and full ISO-8601 timestamps. Returns
    ``None`` for empty or malformed input rather than raising — measure
    logic treats missing dates as absent data.
    """
    if not value:
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00")).date()
    except ValueError:
        try:
            return date.fromisoformat(value[:10])
        except ValueError:
            return None


@dataclass
class HbA1cReading:
    effective_date: date
    value_percent: float


@dataclass
class PatientState:
    patient_id: str
    birth_date: date | None = None
    gender: str | None = None
    has_diabetes: bool = False
    encounter_dates: list[date] = field(default_factory=list)
    hba1c_readings: list[HbA1cReading] = field(default_factory=list)

    def age_at(self, reference: date) -> int | None:
        if self.birth_date is None:
            return None
        years = reference.year - self.birth_date.year
        if (reference.month, reference.day) < (self.birth_date.month, self.birth_date.day):
            years -= 1
        return years

    def latest_hba1c(self) -> HbA1cReading | None:
        return max(self.hba1c_readings, key=lambda r: r.effective_date, default=None)

    def had_encounter_in(self, start: date, end: date) -> bool:
        return any(start <= d <= end for d in self.encounter_dates)


class PatientStore:
    """Thread-safe collection of patient state, keyed on Patient.id."""

    def __init__(self) -> None:
        self._patients: dict[str, PatientState] = {}
        self._lock = RLock()

    def get_or_create(self, patient_id: str) -> PatientState:
        with self._lock:
            if patient_id not in self._patients:
                self._patients[patient_id] = PatientState(patient_id=patient_id)
            return self._patients[patient_id]

    def snapshot(self) -> list[PatientState]:
        with self._lock:
            return list(self._patients.values())

    def __len__(self) -> int:
        with self._lock:
            return len(self._patients)
