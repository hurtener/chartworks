# Foundation implementation decisions — 2026-09-05

### D-056 — Phase 01–02 exposes only an honest foundation until authority enforcement lands

The application composes configuration, public-key dependency health and metadata persistence now. It binds only an explicit loopback IP and exposes health/readiness plus a non-sensitive implemented-capability manifest. No business/MCP/metric/audit/maintenance endpoint is exposed before the Pengui JWT enforcement phases. MCP command dispatch reports unsupported with a nonzero exit rather than simulating a server. This is a scoped implementation boundary, not removal of any later capability.

The key-health probe is not a token verifier or issuer. Phase 03 owns actual JWT acceptance and will wire its verifier/cache into the dependency checker. Authentication settings needed by that phase are validated now without pretending to enforce a token. No local authentication mode, privileged bootstrap or new issuer is introduced.

### D-057 — Store isolation coordinates are not authentication; retention is the first consumer

`store.Scope` is a nonzero, validated internal tenant/actor coordinate. Constructing it grants no authority and it is never accepted directly from a remote caller. Verified authority will be projected into it by the later protected service boundary. All repository queries retain tenant predicates and composite references independently of that future boundary.

The first real business consumer of revision/CAS/audit/idempotency/lease primitives is the bounded internal maintenance service. Its operational retention settings are not sharing or identity policy. Only the five necessary foundation relations ship, rather than preallocating every analytics domain. Future queue/reporting domains reuse/extend the operation semantics through forward migrations; no second work engine is implied.

Expired operation keys retain minimal tombstones. Payload retention must not turn an old retry key into a new execution. A changed retention policy prevents a previously accepted destructive manifest from committing; the caller must explicitly accept a new operation. Local effects/audit/result are transactional; no cross-system exactly-once claim is made.

### D-058 — Typed JSON configuration and real cross-package coverage

The first concrete configuration format is closed JSON, generated defaults and explicit environment references, with duplicate/null/unknown/retired rejection. No parallel YAML/config-alias surface is implemented in these phases. A later format addition must preserve the identical typed semantics and tests.

Package coverage is measured over one instrumented full race-enabled suite, including actual PostgreSQL acceptance callers. The previous per-package-only runner omitted those calls. Thresholds are unchanged: 85% store, 80% other internal code and 70% CLI. Missing tooling, packages, profiles or bands fail rather than report SKIP. Planning checks remain distinct from actual phase acceptance and all-product release.

Operator backup/restore uses PostgreSQL tools, explicit environment connection fields and a private atomic archive; restore requires a new empty target and a trusted archive. No tenant-facing arbitrary SQL/backup endpoint or identity bootstrap is added.
