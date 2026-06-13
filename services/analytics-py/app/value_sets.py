"""Code-system value sets used by CMS122.

Hardcoded here for the MVP — real deployments would resolve these via a
terminology server (e.g. ONC's VSAC FHIR endpoint) and cache per
measurement period.
"""

# Type 1 and Type 2 diabetes mellitus from ICD-10-CM. CMS122's official
# value set (VSAC OID 2.16.840.1.113883.3.464.1003.103.12.1001) is broader;
# this subset is enough to recognise our fixtures and the most common
# real-world codes.
DIABETES_ICD10_PREFIXES: tuple[str, ...] = ("E10", "E11", "E13")
DIABETES_ICD10_SYSTEM = "http://hl7.org/fhir/sid/icd-10-cm"

# Hemoglobin A1c laboratory result. LOINC codes from CMS122's HbA1c
# laboratory test value set (VSAC OID 2.16.840.1.113883.3.464.1003.198.12.1013).
HBA1C_LOINC_CODES: frozenset[str] = frozenset({
    "4548-4",   # Hemoglobin A1c/Hemoglobin.total in Blood
    "4549-2",   # Hemoglobin A1c/Hemoglobin.total in Blood by HPLC
    "17856-6",  # Hemoglobin A1c/Hemoglobin.total in Blood by calculation
    "59261-8",  # Hemoglobin A1c/Hemoglobin.total in Blood by IFCC protocol
})
HBA1C_LOINC_SYSTEM = "http://loinc.org"

# Essential hypertension from ICD-10-CM. CMS165's value set (VSAC OID
# 2.16.840.1.113883.3.464.1003.104.12.1011) is broader but I10 covers
# the most common encoded form.
HYPERTENSION_ICD10_CODES: frozenset[str] = frozenset({"I10"})

# Blood pressure measurements. CMS165 looks for the BP panel
# (LOINC 85354-9) with systolic + diastolic components, OR the
# individual systolic / diastolic codes.
BP_PANEL_LOINC = "85354-9"
SYSTOLIC_BP_LOINC = "8480-6"
DIASTOLIC_BP_LOINC = "8462-4"

# Mammography screening. CPT 77067 is the bilateral-screening code
# CMS125 looks for; production deployments would resolve VSAC OID
# 2.16.840.1.113883.3.464.1003.108.12.1018 dynamically.
MAMMOGRAPHY_CPT = "77067"
MAMMOGRAPHY_SYSTEM = "http://www.ama-assn.org/go/cpt"

# CMS117 — Childhood Immunization Status. CVX codes for the
# combo-10 vaccines used at 24 months of age. Tight subset for the
# demo; real measure runs against the full schedule.
CMS117_CVX_CODES: frozenset[str] = frozenset({
    "08",   # Hepatitis B, pediatric dose
    "10",   # IPV (polio)
    "20",   # DTaP
    "94",   # MMR
    "115",  # Tdap (or DTaP)
})
CVX_SYSTEM = "http://hl7.org/fhir/sid/cvx"


def coding_has_diabetes(codings: list[dict]) -> bool:
    """True if any coding identifies the patient as diabetic."""
    for coding in codings:
        system = coding.get("system", "")
        code = coding.get("code", "")
        if system == DIABETES_ICD10_SYSTEM and any(
            code.startswith(prefix) for prefix in DIABETES_ICD10_PREFIXES
        ):
            return True
    return False


def coding_is_hba1c(codings: list[dict]) -> bool:
    """True if any coding identifies the observation as an HbA1c lab."""
    for coding in codings:
        if coding.get("system") == HBA1C_LOINC_SYSTEM and coding.get("code") in HBA1C_LOINC_CODES:
            return True
    return False


def coding_is_hypertension(codings: list[dict]) -> bool:
    for coding in codings:
        if coding.get("system") == DIABETES_ICD10_SYSTEM and coding.get("code") in HYPERTENSION_ICD10_CODES:
            return True
    return False


def coding_is_bp_panel(codings: list[dict]) -> bool:
    for coding in codings:
        if coding.get("system") == HBA1C_LOINC_SYSTEM and coding.get("code") == BP_PANEL_LOINC:
            return True
    return False


def coding_is_mammography(codings: list[dict]) -> bool:
    for coding in codings:
        if coding.get("system") == MAMMOGRAPHY_SYSTEM and coding.get("code") == MAMMOGRAPHY_CPT:
            return True
    return False


def coding_is_cms117_vaccine(codings: list[dict]) -> str | None:
    """Return the CVX code if the coding is one of the CMS117 combo-10 vaccines."""
    for coding in codings:
        code = coding.get("code")
        if coding.get("system") == CVX_SYSTEM and code in CMS117_CVX_CODES:
            return code
    return None
