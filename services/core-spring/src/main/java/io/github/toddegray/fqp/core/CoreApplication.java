package io.github.toddegray.fqp.core;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

/**
 * Entry point for the FHIR quality platform's core API service. Today only
 * Spring Boot Actuator's /actuator/health is wired; future iterations
 * add multi-tenant org/user management, the measure-library, audit log,
 * and Spring Authorization Server OAuth2 endpoints.
 */
@SpringBootApplication
public class CoreApplication {
    public static void main(String[] args) {
        SpringApplication.run(CoreApplication.class, args);
    }
}
