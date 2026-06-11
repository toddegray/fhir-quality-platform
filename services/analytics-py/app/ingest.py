"""FHIR resource consumers — update :class:`PatientStore` from inbound
NATS messages or direct calls (for unit tests).

One handler per ResourceType keeps each rule small and lets us add more
resource types without growing a single fan-out switch.
"""
from __future__ import annotations

from .state import HbA1cReading, PatientStore, parse_date
from .value_sets import coding_has_diabetes, coding_is_hba1c


def patient_id_from_reference(reference: str | None) -> str | None:
    """Extract ``"p1"`` from ``"Patient/p1"`` (or accept a bare id)."""
    if not reference:
        return None
    return reference.split("/", 1)[1] if reference.startswith("Patient/") else reference


def handle_patient(store: PatientStore, resource: dict) -> None:
    patient_id = resource.get("id")
    if not patient_id:
        return
    state = store.get_or_create(patient_id)
    state.birth_date = parse_date(resource.get("birthDate"))
    state.gender = resource.get("gender")


def handle_condition(store: PatientStore, resource: dict) -> None:
    subject = resource.get("subject") or {}
    patient_id = patient_id_from_reference(subject.get("reference"))
    if not patient_id:
        return
    code = resource.get("code") or {}
    if coding_has_diabetes(code.get("coding") or []):
        store.get_or_create(patient_id).has_diabetes = True


def handle_observation(store: PatientStore, resource: dict) -> None:
    subject = resource.get("subject") or {}
    patient_id = patient_id_from_reference(subject.get("reference"))
    if not patient_id:
        return
    code = resource.get("code") or {}
    if not coding_is_hba1c(code.get("coding") or []):
        return
    value_quantity = resource.get("valueQuantity") or {}
    value = value_quantity.get("value")
    if not isinstance(value, (int, float)):
        return
    effective = parse_date(resource.get("effectiveDateTime"))
    if effective is None:
        return
    store.get_or_create(patient_id).hba1c_readings.append(
        HbA1cReading(effective_date=effective, value_percent=float(value))
    )


def handle_encounter(store: PatientStore, resource: dict) -> None:
    subject = resource.get("subject") or {}
    patient_id = patient_id_from_reference(subject.get("reference"))
    if not patient_id:
        return
    period = resource.get("period") or {}
    start = parse_date(period.get("start"))
    if start is None:
        return
    store.get_or_create(patient_id).encounter_dates.append(start)


HANDLERS = {
    "Patient": handle_patient,
    "Condition": handle_condition,
    "Observation": handle_observation,
    "Encounter": handle_encounter,
}


def dispatch(store: PatientStore, resource_type: str, resource: dict) -> None:
    """Route a resource to the handler matching its ResourceType, if any."""
    handler = HANDLERS.get(resource_type)
    if handler is not None:
        handler(store, resource)
