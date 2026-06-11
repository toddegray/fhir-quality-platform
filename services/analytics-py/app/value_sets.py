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
