# Changelog

## Unreleased — verified authority foundation

Pengui JWT verification, exact signed resource/action enforcement, current-envelope query constraints, protected real operational consumers and public SDK parity. Shared bounded JWKS rotation/freshness, issuer-shaped fixtures, adversarial tests and compiled JWT-to-PostgreSQL lifecycle coverage. No new IAM tables or local issuer.

## Unreleased

### Added — phases 01 and 02

- Go 1.26.4 foundation binary, strict redacted typed configuration, loopback-only health/readiness/capability routes, bounded lifecycle and content-free metrics.
- PostgreSQL/pgx metadata with forward-only checksum-verified migrations, tenant-composite references, immutable operational revisions, CAS/audit atomicity, operation keys and lease fences.
- Real internal retention consumer, expired-key tombstones, trusted-operator private backup/empty-target restore, and recovery tests.
- Twelve named phase acceptance criteria backed by real PostgreSQL, race tests, fuzz seeds, strict package coverage, compiled-process smoke and pre-PR adversarial review.
- Getting-started/configuration/verification documentation and read-only CI with actual Go cross-builds.

### Preserved boundaries

Pengui remains the sole authority issuer. No local IAM, authentication bypass, inference, business API, MCP server, reporting, renderer or scheduler dispatcher is claimed by this foundation. Future production inference remains Bifrost SDK-only to remote providers. Later phases retain their separate acceptance criteria.

## Planning baseline

The earlier commits contain the governed-reporting analysis and actionable 34-phase implementation plan. Their historical detailed notes remain under `docs/archive/`; planning checks are not runtime acceptance.

## Phase 10 read execution / phase 09 continuation

- One plan-only PostgreSQL cursor core with exact ordered results and honest empty/truncated/error states.
- Content-free explicit attempt journal, cancellation intent and observed reconciliation; no hidden retries or result cache.
- Source-revision fence through bounded execution/cleanup; API/SDK parity and real adversarial regressions.
- Forward migration 006; applied migrations 001–005 and existing phase-09 criteria preserved.
