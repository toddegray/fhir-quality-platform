"""FHIR resource consumers — update :class:`PatientStore` from inbound
NATS messages or direct calls (for unit tests).

One handler per ResourceType keeps each rule small and lets us add more
resource types without growing a single fan-out switch.
"""
from __future__ import annotations

from .state import BloodPressureReading, HbA1cReading, PatientStore, parse_date
from .value_sets import (
    coding_has_diabetes,
    coding_is_bp_panel,
    coding_is_cms117_vaccine,
    coding_is_hba1c,
    coding_is_hypertension,
    coding_is_mammography,
)


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

    # Honor generalPractitioner[0] reference for provider attribution
    # so the heatmap can group patients by provider. Real deployments
    # would use careTeam or recent-encounter provider; this is enough
    # for the synthetic cohort.
    gps = resource.get("generalPractitioner") or []
    if gps:
        ref = (gps[0] or {}).get("reference", "")
        if ref.startswith("Practitioner/"):
            state.provider_id = ref.split("/", 1)[1]


def handle_condition(store: PatientStore, resource: dict) -> None:
    subject = resource.get("subject") or {}
    patient_id = patient_id_from_reference(subject.get("reference"))
    if not patient_id:
        return
    coding = (resource.get("code") or {}).get("coding") or []
    state = store.get_or_create(patient_id)
    if coding_has_diabetes(coding):
        state.has_diabetes = True
    if coding_is_hypertension(coding):
        state.has_hypertension = True


def handle_observation(store: PatientStore, resource: dict) -> None:
    subject = resource.get("subject") or {}
    patient_id = patient_id_from_reference(subject.get("reference"))
    if not patient_id:
        return
    coding = (resource.get("code") or {}).get("coding") or []
    effective = parse_date(resource.get("effectiveDateTime"))
    if effective is None:
        return

    if coding_is_hba1c(coding):
        value = (resource.get("valueQuantity") or {}).get("value")
        if isinstance(value, (int, float)):
            store.get_or_create(patient_id).hba1c_readings.append(
                HbA1cReading(effective_date=effective, value_percent=float(value))
            )
        return

    if coding_is_bp_panel(coding):
        systolic, diastolic = _extract_bp_components(resource.get("component") or [])
        if systolic is not None and diastolic is not None:
            store.get_or_create(patient_id).bp_readings.append(
                BloodPressureReading(
                    effective_date=effective,
                    systolic_mmhg=systolic,
                    diastolic_mmhg=diastolic,
                )
            )
        return


def _extract_bp_components(components: list[dict]) -> tuple[float | None, float | None]:
    """Pull systolic + diastolic mmHg from an Observation.component array."""
    from .value_sets import DIASTOLIC_BP_LOINC, HBA1C_LOINC_SYSTEM, SYSTOLIC_BP_LOINC
    systolic = diastolic = None
    for comp in components:
        codings = (comp.get("code") or {}).get("coding") or []
        value = (comp.get("valueQuantity") or {}).get("value")
        if not isinstance(value, (int, float)):
            continue
        for coding in codings:
            if coding.get("system") != HBA1C_LOINC_SYSTEM:
                continue
            if coding.get("code") == SYSTOLIC_BP_LOINC:
                systolic = float(value)
            elif coding.get("code") == DIASTOLIC_BP_LOINC:
                diastolic = float(value)
    return systolic, diastolic


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


def handle_procedure(store: PatientStore, resource: dict) -> None:
    subject = resource.get("subject") or {}
    patient_id = patient_id_from_reference(subject.get("reference"))
    if not patient_id:
        return
    coding = (resource.get("code") or {}).get("coding") or []
    if not coding_is_mammography(coding):
        return
    performed = parse_date(resource.get("performedDateTime"))
    if performed is None:
        return
    store.get_or_create(patient_id).mammography_dates.append(performed)


def handle_immunization(store: PatientStore, resource: dict) -> None:
    patient_ref = (resource.get("patient") or {}).get("reference", "")
    patient_id = patient_id_from_reference(patient_ref)
    if not patient_id:
        return
    coding = (resource.get("vaccineCode") or {}).get("coding") or []
    code = coding_is_cms117_vaccine(coding)
    if code is None:
        return
    store.get_or_create(patient_id).immunization_codes.add(code)


HANDLERS = {
    "Patient": handle_patient,
    "Condition": handle_condition,
    "Observation": handle_observation,
    "Encounter": handle_encounter,
    "Procedure": handle_procedure,
    "Immunization": handle_immunization,
}


def dispatch(store: PatientStore, resource_type: str, resource: dict) -> None:
    """Route a resource to the handler matching its ResourceType, if any."""
    handler = HANDLERS.get(resource_type)
    if handler is not None:
        handler(store, resource)
