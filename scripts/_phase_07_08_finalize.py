"""One-use branch finalizer. The final commit removes this and both writer workflows."""
from pathlib import Path
import json

root = Path(__file__).resolve().parents[1]
def write(path, text):
    (root / path).parent.mkdir(parents=True, exist_ok=True)
    (root / path).write_text(text.rstrip() + '\n')
def edit(path, transform):
    p = root / path
    p.write_text(transform(p.read_text()))

# Qualify the actual catalog proof, not merely the parser or a user context label.
p = root / 'internal/sources/postgres.go'
s = p.read_text()
if 'func supportedPostgresVersion(' not in s:
    needle = "\tif _, err = tx.Exec(ctx, `SELECT set_config('search_path','pg_catalog',true)"
    assert s.count(needle) == 1, 'source probe changed; review instead of guessing'
    guard = '''\t// This catalog/type/exposure proof is qualified against PostgreSQL 17.
\t// Reject unknown majors before interpreting catalogs or locking targets.
\tvar serverVersion int
\tif err = tx.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&serverVersion); err != nil {
\t\treturn out, safe(err)
\t}
\tif !supportedPostgresVersion(serverVersion) {
\t\treturn out, readexec.ErrUnsupported
\t}
'''
    s = s.replace(needle, guard + needle, 1)
    s += '\n// supportedPostgresVersion pins the independently tested source safety contract.\nfunc supportedPostgresVersion(version int) bool { return version >= 170000 && version < 180000 }\n'
    p.write_text(s)
write('internal/sources/version_test.go', '''package sources

import "testing"

func TestQualifiedPostgresMajor(t *testing.T) {
    for _, version := range []int{0, 160000, 169999, 180000, 190001} {
        if supportedPostgresVersion(version) { t.Fatalf("unqualified major accepted: %d", version) }
    }
    for _, version := range []int{170000, 170010, 179999} {
        if !supportedPostgresVersion(version) { t.Fatalf("qualified major rejected: %d", version) }
    }
}
''')

write('docs/contracts/vector-sources-validation.md', '''# Vector generations, PostgreSQL sources and validated reads

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
''')

write('docs/decisions/2026-09-05-vindex-sources.md', '''# Vector/source/validation implementation decisions

## Initial qualified implementation

### D-064 — Atomic vector generations and qualified PostgreSQL source reads

Date: 2026-09-05. Status: accepted implementation decision.

Implement phases 07 and 08 with the existing hard dependency on the actual phase-09
validated-read contract. Preserve the graph and all individually numbered criteria.
Use the existing PostgreSQL store, authority verifier, service assembly and SDK.

The initial PostgreSQL source/catalog proof is explicitly qualified for major 17;
unqualified majors/exposures fail closed. Operator-owned tenant-bound aliases and
separate credential references define actual source contexts; request labels cannot
narrow broad credentials. Explicit testing reports current health rather than a
fake background-test stub. No issuer, integration vault or local grants are added.

Use native PostgreSQL grammar through the pinned WASM parser plus positive AST and
actual source-native checks. Preserve known-valid CTE/window/set operations while
rejecting unproven positional aliases and name-resolution ambiguity. The read plan
is opaque, nonzero, immutable and rechecked at execution. There is no raw-SQL escape.

Vector search uses full-space, exact-provenance generations, complete atomic
publication, stable bounded batches and no evidence cache. Keep enforced initial
bounds explicit; do not add speculative ANN tuning or a local inference path.
The production Bifrost semantic-generation consumer remains phase 15, the full read
execution product phase 10, and other warehouse drivers phase 14.

See [the implementation contract](../contracts/vector-sources-validation.md) and
[adversarial review](../reviews/phase-07-08-adversarial.md). Status and planning checks
are not substitutes for actual named acceptance, race or coverage evidence.
''')

write('docs/reviews/phase-07-08-adversarial.md', '''# Phases 07/08 and phase-09 prerequisite — adversarial self-review

Reviewed on top of the merged phases 01–06 baseline
`9ab8af135ac062d62731212da012ca0e589aa127`. This extends the existing implementation;
it is not a rewrite or an independent external security audit.

## Findings and executable regressions

| Finding | Correction and evidence |
|---|---|
| Base-table positional aliases expose hidden physical columns | Reject the base-table column alias list while retaining normal/derived/CTE aliases; `TestSQLAliasCannotExposeHiddenInputs` uses a real PostgreSQL canary oracle and asserts zero native-planning credential lookups on rejection. |
| GROUP BY input precedence can be confused with output aliases | Input-only resolution for this subset; real database grouping oracle and validator rejection. |
| Nested ORDER BY aliases can smuggle excluded inputs | Bare output alias only; functions/operators/casts resolve against input reach. Real database ordering oracle and positive bare-alias regression. |
| Unknown nested AST fields, window references and VALUES shapes can evade partial walking | Positive whole-node vocabulary and mutation/negative corpus in `TestSQLRejectsUnknownASTFields` and `TestSQLResolverNestedFailureBoundaries`. |
| Future PostgreSQL majors can invalidate the catalog proof | Probe actual major before interpreting catalogs; reject unqualified majors. `TestQualifiedPostgresMajor` plus real PostgreSQL 17 source acceptance. |
| Failed audit commit could leave a partial source/vector transition | Actual database fault-trigger rollback regressions in `TestSourceStoreRejectsUnscopedOrPartialRecords` and `TestVindexRepositoryBoundsAndAtomicFailure`. |
| An oversized batch could return partially authorized evidence | Atomic bounded response failure; `TestVindexBatchResponseCapIsAtomic` exceeds the real 2 MiB response limit. |
| Caller mutation, incomplete generations, rotation and narrowed authority could reuse stale state | Detached bindings/slices, sealed manifests, immutable context revisions, signed selections and concurrent publish/search/rotation negatives in phase acceptance. |

## Executed development evidence and final gates

The reviewed runtime snapshot `8fe346673500657ebda47dea67e4123e1e0cda84` has retained
source, acceptance logs and real coverage instrumentation in Actions run
`33994776854`. That development run passed the 18 phase-07/08/09 criteria and the
whole uncached race-enabled coverage suite. It is not substituted for verification
of later finalization changes. Final-head check/commit links belong in the PR
verification comment after the read-only permanent workflow completes.

The final suite retains all 40 preceding named criteria, for **58 across phases
01–09**, with no implemented-phase skips. Package gates remain 85% storage/security,
exec/vindex; 80% other internal/SDK packages; 70% CLI. No coverage band was lowered.
Planning, drift, mirror, dependency/format checks, build, vet, lint, lifecycle smoke,
SQL/authority fuzzing and Linux amd64/macOS arm64 CGo-free builds remain final gates.

The real query-plan fixture records PostgreSQL/pgvector versions, dataset and batch
shape. Its small synthetic measurement is not a production warehouse/model latency
claim. Recorded provider responses and deterministic vectors do not measure live
model quality or paid-service availability.

## Honest boundaries

Only the qualified PostgreSQL source subset is executable. Unknown engines, majors,
exposure and parser capabilities fail explicitly. There is no public raw-SQL bypass,
local IAM, retained human JWT, alternative model backend or universal SQL-safety
claim. The full execution product remains phase 10, other engine qualification phase
14 and the Bifrost semantic-to-generation consumer phase 15. Twenty-five future
workstreams remain planned. No paid model call, production deployment or merge is
part of this delivery. Already-issued authority retains its documented offline
revocation window; current supplied JWTs remain mandatory.
''')

write('examples/chartworks.sources.json', json.dumps({
    'sources': {'enabled': True, 'max_conns': 4, 'max_rows': 256, 'max_bytes': 1048576,
                'connect_timeout': '1s', 'query_timeout': '2s',
                'connections': [{'tenant': 'example-tenant', 'id': 'warehouse',
                    'version': 'operator-v1', 'read_dsn': 'env:CHARTWORKS_SOURCE_READ',
                    'write_dsn': 'env:CHARTWORKS_SOURCE_WRITE',
                    'relations': [{'schema': 'analytics', 'name': 'sales', 'columns': ['id', 'amount', 'created_at']}]}]},
    'exec': {'max_sql_bytes': 32768, 'max_parameters': 64, 'max_ast_depth': 64,
             'max_ast_nodes': 8192, 'concurrency': 2}}, indent=2))

phase_files = {'07': 'phase-07-vindex.md', '08': 'phase-08-sources-core.md', '09': 'phase-09-sql-validate-core.md'}
for number, name in phase_files.items():
    p = root / 'docs/plans' / name
    s = p.read_text().replace('Status: planned.', 'Status: shipped.').replace('Status: in_progress.', 'Status: shipped.')
    s = s.replace('No runtime completion is claimed.', 'Runtime implementation and named acceptance are supplied here; no production deployment is claimed.')
    marker = '\n## Implemented evidence and qualified scope\n'
    if marker not in s:
        s += marker + '\nD-064 and [the vector/source/read contract](../contracts/vector-sources-validation.md) pin the qualified scope. See the [adversarial review](../reviews/phase-07-08-adversarial.md). Existing hard dependencies and all six numbered criteria remain unchanged. Real PostgreSQL/pgvector and native-parser fixtures provide runtime evidence; final exact-head CI must pass before merge. The full execution product remains phase 10, additional engines phase 14 and the semantic generation consumer phase 15.\n'
    p.write_text(s)
p = root / 'docs/plans/phase-registry.json'
registry = json.loads(p.read_text())
for number in phase_files:
    registry['phases'][number]['status'] = 'shipped'
p.write_text(json.dumps(registry, indent=2) + '\n')
p = root / 'docs/plans/coverage.json'
coverage = json.loads(p.read_text())
for number in phase_files:
    coverage.setdefault('implementation_evidence', {})[number] = [
        'docs/contracts/vector-sources-validation.md', 'docs/reviews/phase-07-08-adversarial.md',
        f'test/acceptance/phase{number}_test.go']
p.write_text(json.dumps(coverage, indent=2) + '\n')

edit('docs/plans/README.md', lambda s: s.replace(
    'Current implementation status: phases 01–06 provide the Go/PostgreSQL foundation, verified operational access, remote Bifrost inference and durable maintenance work. Twenty-eight later workstreams remain planned.',
    'Current implementation status: phases 01–09 provide the existing foundation, authority, gateway and jobs plus pgvector generations, qualified PostgreSQL sources and validated-read contracts. Twenty-five later workstreams remain planned.').replace(
    'Phases01–06 are implemented; the remaining twenty-eight are planned.',
    'Phases01–09 are implemented; the remaining twenty-five are planned.').replace(
    'Phases01–06 now supply the foundation, gateway and queue.',
    'Phases01–09 now supply the foundation, gateway, queue, vector generations and qualified source/validation core.'))
edit('README.md', lambda s: s.replace('phases 01–06', 'phases 01–09').replace('Phases 01–06', 'Phases 01–09').replace('40 named', '58 named').replace('28 planned', '25 planned'))
edit('RFC-001-Chartworks.md', lambda s: s.replace('Design acceptance is not runtime completion; all 34 phases remain planned.', 'Phases 01–09 now have runtime implementations; 25 later workstreams remain planned. Named tests and exact-source verification, not design acceptance alone, establish completion.'))

for path, title, body in [
    ('GETTING-STARTED.md', 'Vector generations and qualified PostgreSQL sources',
     'The reference metadata image is `pgvector/pgvector:0.8.2-pg17`; install the extension before migrations when using a restricted migration role. Merge the source excerpt from `examples/chartworks.sources.json` into the existing configuration, replace the synthetic tenant/relation coordinates, and provide the read DSN through the referenced environment variable. Never put a resolved credential or human JWT in metadata. The optional write reference is not used by the reader.\n\nThe source adapter is qualified for PostgreSQL 17 and the documented ordinary-heap subset. Use explicit source test/discovery and revision-checked rotation; registered metadata is not a health promise. Obtain Pengui-issued action/resource/context reach for the actual operations. See `docs/contracts/vector-sources-validation.md` and the executable source operation inventory. No additional warehouse engine, public raw-SQL route or local model is enabled by this excerpt.'),
    ('docs/configuration.md', 'Source and validation configuration',
     'The typed `sources` block defaults to disabled. Bounds: max_conns 1–16 (default 4), max_rows 1–1000 (256), max_bytes 1 KiB–4 MiB (1 MiB), connect_timeout and query_timeout 1 ms–4 s (1 s and 2 s). Connection aliases are tenant-bound and carry version, declared relations/columns and independent env: read/write references; the reader never resolves write credentials.\n\nThe typed `exec` block bounds SQL bytes 128–65536 (32768), parameters 1–64 (64), AST depth 4–64 (64), AST nodes 32–16384 (8192) and concurrent validations 1–16 (2). Unknown/retired keys fail. These limits are not skip-validation settings. See `../examples/chartworks.sources.json` and `contracts/vector-sources-validation.md` for enforced fixed vector bounds, credential custody, PostgreSQL qualification and operational behavior.')]:
    p = root / path
    s = p.read_text()
    marker = '\n## ' + title + '\n'
    if marker not in s:
        p.write_text(s + marker + '\n' + body + '\n')

p = root / 'AGENTS.md'
s = p.read_text()
marker = '\n## Phases 07/08 and validated-read prerequisite\n'
if marker not in s:
    s += marker + '\nD-064 and docs/contracts/vector-sources-validation.md govern the actual PostgreSQL 17 source and native-parser subset. Do not bypass opaque plans, widen signed context reach, turn an operator alias into a public DSN field, or treat parsed SQL as authority. Vector generations require complete exact-origin manifests and atomic publication; do not add tenant-only evidence caches or local inference. Preserve the SQL alias, rollback, oversized-batch and concurrent generation regressions. Later phase 10/14/15 consumers retain their existing obligations. Final CI is read-only and must test committed source without preparation or repair scripts.\n'
write('AGENTS.md', s)
write('CLAUDE.md', s)
edit('docker-compose.yml', lambda s: s.replace('image: postgres:17', 'image: pgvector/pgvector:0.8.2-pg17'))

p = root / '.github/workflows/ci.yml'
s = p.read_text()
s = s.replace('feat/phase-05-06-gateway-jobs]', 'feat/phase-05-06-gateway-jobs, feat/phase-07-08-vindex-sources]')
s = s.replace('image: postgres:17', 'image: pgvector/pgvector:0.8.2-pg17')
s = s.replace('All forty implemented phase criteria', 'All fifty-eight implemented phase criteria')
needle = '          python3 scripts/run_phase_acceptance.py --phase 06\n'
if 'run_phase_acceptance.py --phase 07' not in s:
    assert needle in s
    s = s.replace(needle, needle + '          python3 scripts/run_phase_acceptance.py --phase 07\n          python3 scripts/run_phase_acceptance.py --phase 09\n          python3 scripts/run_phase_acceptance.py --phase 08\n')
s = s.replace("-o -name '_gateway_jobs*.py'", "-o -name '_gateway_jobs*.py' -o -name '_phase_07_08*'")
needle = "          go test -race ./test/acceptance -run '^$' -fuzz '^FuzzActualVerifier$' -fuzztime=5s -parallel=2\n"
if 'names=$(go test ./internal/exec' not in s:
    assert needle in s
    s = s.replace(needle, needle + '''          names=$(go test ./internal/exec -list '^Fuzz' | grep '^Fuzz')
          test -n "$names"
          for name in $names; do
            go test -race ./internal/exec -run '^$' -fuzz "^${name}$" -fuzztime=5s -parallel=2
          done
''')
if 'CHARTWORKS_COVERAGE_OUTPUT:' not in s:
    s = s.replace('      CGO_ENABLED: 1\n', '      CGO_ENABLED: 1\n      CHARTWORKS_COVERAGE_OUTPUT: ${{ runner.temp }}/chartworks-coverage.out\n', 1)
if 'name: Retain actual race coverage instrumentation' not in s:
    s = s.replace('      - name: Working tree must remain unchanged', '''      - name: Retain actual race coverage instrumentation
        if: always()
        uses: actions/upload-artifact@v4
        with:
          name: race-coverage
          path: ${{ runner.temp }}/chartworks-coverage.out
          retention-days: 14
      - name: Working tree must remain unchanged''')
p.write_text(s)

# Remove source writers and outdated handoffs atomically with the finalized source.
for p in (root / 'scripts').glob('_phase_07_08*'):
    if p.is_file():
        p.unlink()
for name in ['.github/workflows/phase-07-08-development.yml', '.github/workflows/phase-07-08-finalize.yml',
             'docs/reviews/phase-07-08-recovery-checkpoint.md', 'docs/reviews/phase-07-08-finalization-handoff.md']:
    (root / name).unlink(missing_ok=True)
