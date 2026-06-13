package io.github.toddegray.fqp.core;

import java.util.List;
import java.util.Map;

import org.springframework.web.bind.annotation.GetMapping;
import org.springframework.web.bind.annotation.PathVariable;
import org.springframework.web.bind.annotation.RequestMapping;
import org.springframework.web.bind.annotation.RestController;

/**
 * Serves measure-library metadata. Today only CMS122 is wired; future
 * iterations back this with the Postgres measure-library table the
 * Spring Data JPA layer manages.
 */
@RestController
@RequestMapping("/measures")
public class MeasureController {

    private static final Map<String, Measure> LIBRARY = Map.of(
        "CMS122", new Measure(
            "CMS122",
            "Diabetes: Hemoglobin A1c (HbA1c) Poor Control (> 9 %)",
            "Percentage of patients 18-75 years of age with diabetes who had hemoglobin A1c > 9.0% during the measurement period.",
            "lower-is-better",
            List.of("E10.*", "E11.*", "E13.*"),
            List.of("LOINC 4548-4", "LOINC 4549-2", "LOINC 17856-6", "LOINC 59261-8"),
            "https://ecqi.healthit.gov/ecqm/ec/2024/cms122v12"
        ),
        "CMS125", new Measure(
            "CMS125",
            "Breast Cancer Screening (Mammography)",
            "Percentage of women 50-74 with one or more mammograms during the measurement period or the 15 months prior to the measurement period.",
            "higher-is-better",
            List.of(),
            List.of("CPT 77067"),
            "https://ecqi.healthit.gov/ecqm/ec/2024/cms125v12"
        ),
        "CMS165", new Measure(
            "CMS165",
            "Controlling High Blood Pressure",
            "Percentage of patients 18-85 with hypertension whose blood pressure was controlled (< 140/90 mmHg) during the measurement period.",
            "higher-is-better",
            List.of("I10"),
            List.of("LOINC 85354-9", "LOINC 8480-6", "LOINC 8462-4"),
            "https://ecqi.healthit.gov/ecqm/ec/2024/cms165v12"
        ),
        "CMS117", new Measure(
            "CMS117",
            "Childhood Immunization Status",
            "Percentage of children 2 years of age who had the combo-10 vaccine series by their second birthday.",
            "higher-is-better",
            List.of(),
            List.of("CVX 08", "CVX 10", "CVX 20", "CVX 94", "CVX 115"),
            "https://ecqi.healthit.gov/ecqm/ec/2024/cms117v12"
        )
    );

    @GetMapping("/{id}")
    public Measure get(@PathVariable("id") String id) {
        var measure = LIBRARY.get(id.toUpperCase());
        if (measure == null) {
            throw new MeasureNotFoundException(id);
        }
        return measure;
    }

    @GetMapping
    public List<Measure> list() {
        return List.copyOf(LIBRARY.values());
    }

    public record Measure(
        String id,
        String title,
        String description,
        String direction,
        List<String> diagnosisCodes,
        List<String> labCodes,
        String reference
    ) {}
}
