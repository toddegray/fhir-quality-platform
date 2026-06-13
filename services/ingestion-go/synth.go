package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
)

// SyntheticCohortSource generates a deterministic FHIR R4 patient cohort
// sized large enough to drive realistic eCQM denominators (default 100
// patients). Prevalence rates and clinical distributions are calibrated
// against published US adult epidemiology so the resulting measure
// scores are plausible:
//
//	Diabetes:      ~25% of adults (CDC 11% national avg; older cohort skew)
//	Hypertension:  ~45% of adults (AHA 2024 stats: 48% of US adults)
//	Mammography:   ~72% of women 50-74 had one in last 2y (NHIS 2022)
//	Immunizations: ~70% of 2yo complete the standard series (CDC NIS 2023)
//
// Within each disease cohort the generator picks a sub-rate of "poor
// control" — for diabetes that's HbA1c >9 (CMS122 numerator), for HTN
// it's BP ≥140/90 (CMS165 numerator inverse). Both are tuned to land
// the resulting measure score in a credible window (CMS122 ~ 30-40%,
// CMS165 ~ 60-75% controlled) so the dashboard reads as real.
type SyntheticCohortSource struct {
	Seed             uint64
	PatientCount     int
	ProviderCount    int
	MeasurementYear  int
	DiabetesRate     float64
	DiabetesPoorCtrl float64
	HypertensionRate float64
	HtnControlled    float64
	MammographyRate  float64
	ChildImmRate     float64
}

func NewSyntheticCohortSource() *SyntheticCohortSource {
	return &SyntheticCohortSource{
		Seed:             42,
		PatientCount:     100,
		ProviderCount:    6,
		MeasurementYear:  2025,
		DiabetesRate:     0.25,
		DiabetesPoorCtrl: 0.22,
		HypertensionRate: 0.45,
		HtnControlled:    0.68,
		MammographyRate:  0.72,
		ChildImmRate:     0.70,
	}
}

func (s *SyntheticCohortSource) Name() string {
	return fmt.Sprintf("synthetic(n=%d, providers=%d, seed=%d, year=%d)",
		s.PatientCount, s.ProviderCount, s.Seed, s.MeasurementYear)
}

// firstNames + lastNames seed a deterministic name pool so the same
// seed produces the same chart strings across runs. Kept short and
// neutral; not optimised for diversity of representation.
var firstNamesF = []string{"Anita", "Camila", "Sofia", "Mei", "Linh", "Aisha", "Priya", "Olivia", "Emma", "Maria", "Ana", "Yuki", "Hiroko", "Fatima", "Zara"}
var firstNamesM = []string{"Ravi", "Marco", "Jin", "Hassan", "Diego", "Leo", "Mateo", "Omar", "Hugo", "Liam", "Noah", "Aarav", "Kai", "Idris", "Bjorn"}
var lastNames = []string{"Carter", "Singh", "Nguyen", "Garcia", "Tanaka", "Hassan", "Patel", "Okonkwo", "Kim", "Reyes", "Diaz", "Brown", "Cohen", "Vasquez", "Lopez", "Mueller", "Olsson", "Hassan", "Park", "Ito"}

type genPatient struct {
	id            string
	gender        string
	birthDate     string
	age           int
	familyName    string
	givenName     string
	providerID    string
	hasDiabetes   bool
	hasHTN        bool
	hadMammography bool
	childImmComplete bool
}

// Files runs the generator and returns one ResourceFile per FHIR
// ResourceType. The generator is fully deterministic given the
// configured Seed.
func (s *SyntheticCohortSource) Files(_ context.Context) ([]ResourceFile, error) {
	rng := rand.New(rand.NewPCG(s.Seed, s.Seed^0xdeadbeef))
	patients := s.generatePatients(rng)

	var (
		patientLines      []string
		conditionLines    []string
		observationLines  []string
		encounterLines    []string
		procedureLines    []string
		immunizationLines []string
	)

	for _, p := range patients {
		patientLines = append(patientLines, marshalLine(buildPatientResource(p)))
		// Every patient has at least one encounter in the measurement
		// period — measure denominators require it.
		for i := 0; i < 1+rng.IntN(3); i++ {
			eDate := dateInYear(rng, s.MeasurementYear)
			encounterLines = append(encounterLines, marshalLine(buildEncounter(p, i, eDate)))
		}

		if p.hasDiabetes {
			conditionLines = append(conditionLines, marshalLine(buildDiabetesCondition(p)))
			// 1-3 HbA1c labs; pick a baseline value per patient that
			// determines control status, then jitter for each reading.
			baseline := 6.5 + rng.Float64()*2.0 // 6.5-8.5 normal
			if rng.Float64() < s.DiabetesPoorCtrl {
				baseline = 9.0 + rng.Float64()*3.0 // 9.0-12.0 poor
			}
			reads := 1 + rng.IntN(3)
			for i := 0; i < reads; i++ {
				eDate := dateInYear(rng, s.MeasurementYear)
				value := baseline + (rng.Float64()-0.5)*0.6
				observationLines = append(observationLines, marshalLine(buildHbA1c(p, i, eDate, value)))
			}
		}

		if p.hasHTN {
			conditionLines = append(conditionLines, marshalLine(buildHypertensionCondition(p)))
			controlled := rng.Float64() < s.HtnControlled
			eDate := dateInYear(rng, s.MeasurementYear)
			var sys, dia float64
			if controlled {
				sys = 115 + rng.Float64()*20  // 115-135
				dia = 70 + rng.Float64()*15   // 70-85
			} else {
				sys = 142 + rng.Float64()*25  // 142-167
				dia = 92 + rng.Float64()*15   // 92-107
			}
			observationLines = append(observationLines, marshalLine(buildBloodPressure(p, eDate, sys, dia)))
		}

		if p.hadMammography {
			procedureLines = append(procedureLines, marshalLine(buildMammography(p, dateInYear(rng, s.MeasurementYear-rng.IntN(2)))))
		}

		if p.age == 2 && p.childImmComplete {
			for _, code := range []string{"08", "10", "20", "94", "115"} {
				eDate := dateInYear(rng, s.MeasurementYear-1)
				immunizationLines = append(immunizationLines, marshalLine(buildImmunization(p, code, eDate)))
			}
		}
	}

	files := []ResourceFile{
		{ResourceType: "Patient", Content: linesToBytes(patientLines), SourceLabel: "synthetic"},
		{ResourceType: "Condition", Content: linesToBytes(conditionLines), SourceLabel: "synthetic"},
		{ResourceType: "Observation", Content: linesToBytes(observationLines), SourceLabel: "synthetic"},
		{ResourceType: "Encounter", Content: linesToBytes(encounterLines), SourceLabel: "synthetic"},
		{ResourceType: "Procedure", Content: linesToBytes(procedureLines), SourceLabel: "synthetic"},
		{ResourceType: "Immunization", Content: linesToBytes(immunizationLines), SourceLabel: "synthetic"},
	}
	return files, nil
}

// generatePatients lays out the cohort demographics. Age distribution
// is weighted toward adults so we hit the typical eCQM denominator age
// windows (18-75 for CMS122, 50-74 for CMS125, etc.) plus a small slice
// of 2-year-olds for CMS117.
func (s *SyntheticCohortSource) generatePatients(rng *rand.Rand) []genPatient {
	out := make([]genPatient, 0, s.PatientCount)
	for i := 0; i < s.PatientCount; i++ {
		isFemale := rng.Float64() < 0.52
		// Age distribution: 10% two-year-olds (CMS117 cohort), 90% adults 18-85.
		// We slightly over-weight infants relative to the US under-2 population
		// so the CMS117 denominator has 8-12 patients — enough for the heatmap
		// to show real per-provider variation.
		var age int
		if rng.Float64() < 0.10 {
			age = 2
		} else {
			age = 18 + rng.IntN(68) // 18-85
		}
		var gender, givenName string
		if isFemale {
			gender = "female"
			givenName = firstNamesF[rng.IntN(len(firstNamesF))]
		} else {
			gender = "male"
			givenName = firstNamesM[rng.IntN(len(firstNamesM))]
		}
		familyName := lastNames[rng.IntN(len(lastNames))]
		birthYear := s.MeasurementYear - age
		birthMonth := 1 + rng.IntN(12)
		birthDay := 1 + rng.IntN(28)

		// Prevalence: diabetes scales up with age (very low under 30, high over 50).
		diabetesProb := s.DiabetesRate
		switch {
		case age < 30:
			diabetesProb *= 0.15
		case age < 50:
			diabetesProb *= 0.6
		case age >= 65:
			diabetesProb *= 1.4
		}
		hasDiabetes := age >= 18 && age <= 75 && rng.Float64() < diabetesProb

		htnProb := s.HypertensionRate
		switch {
		case age < 30:
			htnProb *= 0.15
		case age < 50:
			htnProb *= 0.7
		case age >= 65:
			htnProb *= 1.3
		}
		hasHTN := age >= 18 && rng.Float64() < htnProb

		hadMammography := gender == "female" && age >= 50 && age <= 74 && rng.Float64() < s.MammographyRate
		childImmComplete := age == 2 && rng.Float64() < s.ChildImmRate

		out = append(out, genPatient{
			id:               "synth-" + strconv.Itoa(i+1),
			gender:           gender,
			birthDate:        fmt.Sprintf("%04d-%02d-%02d", birthYear, birthMonth, birthDay),
			age:              age,
			familyName:       familyName,
			givenName:        givenName,
			providerID:       fmt.Sprintf("prov-%02d", 1+(i%s.ProviderCount)),
			hasDiabetes:      hasDiabetes,
			hasHTN:           hasHTN,
			hadMammography:   hadMammography,
			childImmComplete: childImmComplete,
		})
	}
	return out
}

// ----- FHIR resource builders -----------------------------------------------

func buildPatientResource(p genPatient) map[string]any {
	return map[string]any{
		"resourceType": "Patient",
		"id":           p.id,
		"gender":       p.gender,
		"birthDate":    p.birthDate,
		"name": []map[string]any{
			{"family": p.familyName, "given": []string{p.givenName}},
		},
		"generalPractitioner": []map[string]any{
			{"reference": "Practitioner/" + p.providerID, "display": "Provider " + p.providerID},
		},
	}
}

func buildEncounter(p genPatient, idx int, date string) map[string]any {
	return map[string]any{
		"resourceType": "Encounter",
		"id":           fmt.Sprintf("enc-%s-%d", p.id, idx),
		"status":       "finished",
		"subject":      map[string]any{"reference": "Patient/" + p.id},
		"class": map[string]any{
			"system":  "http://terminology.hl7.org/CodeSystem/v3-ActCode",
			"code":    "AMB",
			"display": "ambulatory",
		},
		"period": map[string]any{"start": date, "end": date},
	}
}

func buildDiabetesCondition(p genPatient) map[string]any {
	return map[string]any{
		"resourceType": "Condition",
		"id":           "cond-dm-" + p.id,
		"subject":      map[string]any{"reference": "Patient/" + p.id},
		"clinicalStatus": map[string]any{
			"coding": []map[string]any{
				{"system": "http://terminology.hl7.org/CodeSystem/condition-clinical", "code": "active"},
			},
		},
		"code": map[string]any{
			"coding": []map[string]any{
				{"system": "http://hl7.org/fhir/sid/icd-10-cm", "code": "E11.9", "display": "Type 2 diabetes mellitus without complications"},
			},
		},
		"onsetDateTime": "2015-01-15",
	}
}

func buildHypertensionCondition(p genPatient) map[string]any {
	return map[string]any{
		"resourceType": "Condition",
		"id":           "cond-htn-" + p.id,
		"subject":      map[string]any{"reference": "Patient/" + p.id},
		"clinicalStatus": map[string]any{
			"coding": []map[string]any{
				{"system": "http://terminology.hl7.org/CodeSystem/condition-clinical", "code": "active"},
			},
		},
		"code": map[string]any{
			"coding": []map[string]any{
				{"system": "http://hl7.org/fhir/sid/icd-10-cm", "code": "I10", "display": "Essential (primary) hypertension"},
			},
		},
		"onsetDateTime": "2018-06-01",
	}
}

func buildHbA1c(p genPatient, idx int, date string, valuePct float64) map[string]any {
	return map[string]any{
		"resourceType": "Observation",
		"id":           fmt.Sprintf("obs-a1c-%s-%d", p.id, idx),
		"status":       "final",
		"subject":      map[string]any{"reference": "Patient/" + p.id},
		"code": map[string]any{
			"coding": []map[string]any{
				{"system": "http://loinc.org", "code": "4548-4", "display": "Hemoglobin A1c/Hemoglobin.total in Blood"},
			},
		},
		"valueQuantity": map[string]any{
			"value": roundFloat(valuePct, 1), "unit": "%", "system": "http://unitsofmeasure.org", "code": "%",
		},
		"effectiveDateTime": date,
	}
}

func buildBloodPressure(p genPatient, date string, systolic, diastolic float64) map[string]any {
	return map[string]any{
		"resourceType": "Observation",
		"id":           "obs-bp-" + p.id,
		"status":       "final",
		"subject":      map[string]any{"reference": "Patient/" + p.id},
		"code": map[string]any{
			"coding": []map[string]any{
				{"system": "http://loinc.org", "code": "85354-9", "display": "Blood pressure panel"},
			},
		},
		"effectiveDateTime": date,
		"component": []map[string]any{
			{
				"code": map[string]any{
					"coding": []map[string]any{
						{"system": "http://loinc.org", "code": "8480-6", "display": "Systolic blood pressure"},
					},
				},
				"valueQuantity": map[string]any{"value": roundFloat(systolic, 0), "unit": "mm[Hg]", "system": "http://unitsofmeasure.org", "code": "mm[Hg]"},
			},
			{
				"code": map[string]any{
					"coding": []map[string]any{
						{"system": "http://loinc.org", "code": "8462-4", "display": "Diastolic blood pressure"},
					},
				},
				"valueQuantity": map[string]any{"value": roundFloat(diastolic, 0), "unit": "mm[Hg]", "system": "http://unitsofmeasure.org", "code": "mm[Hg]"},
			},
		},
	}
}

func buildMammography(p genPatient, date string) map[string]any {
	return map[string]any{
		"resourceType": "Procedure",
		"id":           "proc-mam-" + p.id,
		"status":       "completed",
		"subject":      map[string]any{"reference": "Patient/" + p.id},
		"code": map[string]any{
			"coding": []map[string]any{
				{"system": "http://www.ama-assn.org/go/cpt", "code": "77067", "display": "Screening mammography, bilateral"},
			},
		},
		"performedDateTime": date,
	}
}

func buildImmunization(p genPatient, vaccineCode, date string) map[string]any {
	return map[string]any{
		"resourceType": "Immunization",
		"id":           fmt.Sprintf("imm-%s-%s", p.id, vaccineCode),
		"status":       "completed",
		"patient":      map[string]any{"reference": "Patient/" + p.id},
		"vaccineCode": map[string]any{
			"coding": []map[string]any{
				{"system": "http://hl7.org/fhir/sid/cvx", "code": vaccineCode},
			},
		},
		"occurrenceDateTime": date,
	}
}

// ----- helpers --------------------------------------------------------------

func dateInYear(rng *rand.Rand, year int) string {
	month := 1 + rng.IntN(12)
	day := 1 + rng.IntN(28)
	return fmt.Sprintf("%04d-%02d-%02d", year, month, day)
}

func roundFloat(v float64, decimals int) float64 {
	mult := 1.0
	for i := 0; i < decimals; i++ {
		mult *= 10
	}
	return float64(int(v*mult+0.5)) / mult
}

func marshalLine(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}

func linesToBytes(lines []string) []byte {
	if len(lines) == 0 {
		return []byte{}
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

