# core-spring

**Language / runtime:** Java 21 + Spring Boot 3
**Role:** The platform's enterprise core. Owns multi-tenant org / provider /
user data, the measure-library catalog, an OAuth2 identity provider for
partner apps (CDS Hooks consumers, SMART apps), and the audit log every
regulated healthcare deployment requires.

## Why Spring Boot

This service's surface is the classic enterprise-CRUD shape: transactional
persistence over Postgres, complex authorization rules, mature OAuth2
support (Spring Authorization Server), strict audit guarantees. Java's
typing + Spring's ecosystem make this kind of work straightforward — and
much of regulated healthcare runs on it, so the choice mirrors real
deployment patterns.

## Responsibilities

- Multi-tenant org / provider / clinician / admin user management.
- Measure-library catalog (which eCQMs are active per org, value-set
  references, attestation periods).
- OAuth2 Authorization Server: issues tokens for partner CDS Hooks
  consumers and downstream service-to-service auth.
- Append-only audit log (immutable, queryable by user, resource, period).
- Persistent storage on Postgres via Spring Data JPA + Flyway migrations.

## Outbound dependencies

- Postgres (read/write)

## Inbound interfaces

- REST API for the Node edge:
  - `/orgs/**`, `/providers/**`, `/users/**`, `/measures/**`,
    `/audit/**`
- OAuth2 endpoints (`/oauth2/authorize`, `/oauth2/token`,
  `/.well-known/openid-configuration`)
- `/actuator/health`

## Status

Stub — directory exists; no Gradle / Maven project initialized yet.
