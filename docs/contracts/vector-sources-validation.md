# Vector generations, PostgreSQL sources and validated reads

Implemented by phases 07/08 and their phase-09 prerequisite on the existing phases
01–06 foundation. D-064 applies. Pengui remains the sole issuer and policy owner.
These contracts do not introduce local IAM, retained human JWTs or model clients.

## Source operations and custody

The executable operation inventory is [chartworks-source-operations.json](chartworks-source-operations.json).
The source HTTP handler and Go SDK use the same service checks. Create requires
`sources.write` and signed tenant write reach. Read/list require `sources.read`
and signed source read reach, applied before the database limit. Test/discovery
require their registered action, source query reach and use reach for the actual
immutable execution-context revision. Rotation requires source write reach.
Denials occur before credential resolution or warehouse work. Exact request fields
are closed; a body/header tenant, DSN, credential, relation list or context label
cannot override the registered binding. SQL is not accepted by the source HTTP API.

Operators configure a bounded tenant-bound connection alias and explicit `env:`
references using [chartworks.sources.json](../../examples/chartworks.sources.json).
The reader resolves only the read DSN. The optional independent write reference
must differ and is never resolved by this read adapter. It reserves separation for
later managed-engineering consumers, not a working write feature. Public source
read/list types contain no DSN, password, token or resolved credential field.
There is no new integration vault or source authentication service.

An execution context records the observed database/user, role capabilities,
relation/column/type/ACL evidence and configured exposure version. Its immutable
revision and fingerprint define the data partition. Creation/rotation probes the
actual credentials and rejects unproven exposure. Rotation uses a revision CAS and
atomic metadata/audit commit, refreshes the bounded pool, and invalidates stale
plans. Changing a secret without explicit rotation is not permission to replay an
old plan with a new data partition. Sessions use bounded read-only transactions,
fixed search path, timeout controls and cancellation. Shutdown joins pool users.

`registered` metadata is not a claim that a warehouse is currently healthy. Explicit
test/discovery performs current work; no success-returning periodic-test stub is
advertised. Disabling source connections retains authenticated metadata reads and
removes connection-dependent operations. A model or worker outage does not disable
public JWT verification or retained metadata.

## Qualified PostgreSQL source subset

The initial source safety contract is qualified for PostgreSQL **17**. The adapter
checks the actual server major before its catalog proof; unknown majors fail with
a typed unsupported result rather than inheriting an untested safety assumption.
Minor-version qualification is covered by the actual CI reference image, not an
assertion that every future engine behavior has been tested.

The executable exposure is a declared set of ordinary PostgreSQL heap relations
and permitted native columns. Superuser/bypass-RLS/unsafe role capabilities, views,
RLS, foreign/inherited relations, unproven table access methods, types and other
unsupported exposure are rejected by the probe. This is a deliberately closed
initial subset, not full RLS/view support or all-engine parity. PostgreSQL discovery
classifies numeric (including money), temporal, text, boolean, structured, binary
and unknown types; classification alone does not grant query authority.

## Validated-read prerequisite

`internal/exec` owns the acyclic read-adapter contract and the opaque nonzero plan.
Only the validator issues it. A plan binds source, immutable context, contract,
resolved dependencies, semantic version, parameters and verified authority; these
bindings are checked again at execution. Its zero value and forged/stale/narrowed
bindings fail. Parameters have explicit canonical types and bounded shapes.

The pinned `github.com/wasilibs/go-pgquery` module uses PostgreSQL grammar through
WASM while retaining the CGo-free shipping build. The exact pin remains in go.mod
and go.sum. Whole-tree positive AST checks resolve relation/column/function reach;
unknown syntax or dependency visibility fails closed. Parsing and native
`EXPLAIN (FORMAT JSON)` are complementary checks, never independent authorization.
Native planning uses constrained read-only transactions and never EXPLAIN ANALYZE.
There is no raw-string execution escape, skip-validation flag or SELECT-prefix proof.

Supported CTE, derived-table, window and set-operation cases are covered by the
positive corpus. Base-table positional column aliases are rejected because they
refer to physical columns, not the permitted projection. GROUP BY and DISTINCT ON
use input-only resolution in this subset. A bare ORDER BY output alias is allowed;
an output alias inside a function/operator/cast is not treated as blanket authority.
Ambiguous or unsupported cases fail explicitly rather than being silently rewritten.
The full phase-10 execution product and later engine qualification remain separate.

## Vector generations

`internal/vindex` is a pure PostgreSQL/pgvector storage boundary. It performs no
inference. Production Bifrost-to-generation assembly belongs to the phase-15
semantic consumer; deterministic vectors in storage tests are not a production
fallback model.

Each immutable staging manifest contains tenant/topic/version/context/source
provenance and the complete space descriptor: provider, route, endpoint, model,
revision, dimensions, preprocessing, input type and normalization. Same dimensions
are insufficient for compatibility. Expected facet IDs, kinds, source IDs and exact
input hashes preserve batch association. Invalid vectors, duplicates and foreign
origins fail before publication.

A complete generation is sealed before one revision-fenced publication pointer
switch. Incomplete/interrupted indexing cannot expose partial semantics or pair a
new publication with old vectors. Database constraints/triggers protect immutable
coordinates and ready facets. Archive and targeted topic/version/tenant deletion
invalidate the affected pointer without broadening cleanup to another tenant.

Search applies signed tenant/topic/context and active-generation restrictions in
SQL before ranking. Batch members observe one repeatable-read publication snapshot.
Per-kind limits and stable ties preserve scalar semantics and input association.
There is intentionally no evidence cache. Identical text or a tenant label alone
cannot authorize reuse. Oversized responses fail atomically without partial hits.

Current hard bounds: 4096 origins per generation; 64 facets and 4 MiB per upsert;
4096 bytes per facet text; 8 queries per search batch; 8 kinds and 10 hits per kind;
2 MiB combined response; 1–16000 vector dimensions. These are enforced initial
bounds, not a new configurable tuning interface. Retrieval is exact pgvector search
with partition indexes; no ANN/production-latency claim is inferred from fixtures.

## Operation and deployment

The reference metadata image is `pgvector/pgvector:0.8.2-pg17`. Operators must make
pgvector available before migrations; restricted roles may require operator
extension installation in the documented schema. Forward migrations 004/005 add
these consumers; applied migrations 001–003 remain unchanged. The warehouse fixture
is separate from metadata and uses actual restricted source credentials.

All 18 phase-07/08/09 criteria use real PostgreSQL/pgvector, the native parser and
actual service boundaries. Their source/API/SDK, alias, rollback, concurrency and
response-bound regressions are recorded in the [adversarial review](../reviews/phase-07-08-adversarial.md).
Final exact-head CI must pass the existing package gates and cumulative acceptance.
No paid model call, additional warehouse-engine support, deployment or full-product
release acceptance is claimed. Twenty-five later workstreams remain planned.
