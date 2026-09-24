from pathlib import Path
import re

ROOT = Path('scripts/sql-population')

def edit(path, old, new, count=1):
    p = Path(path)
    text = p.read_text()
    if text.count(old) != count:
        raise SystemExit(f'unexpected source {path}: {text.count(old)} != {count}: {old[:100]}')
    p.write_text(text.replace(old, new))

def add(path, text):
    p = Path(path)
    if p.exists():
        raise SystemExit(f'already exists: {path}')
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text)

for source, target in [('exec.go.txt','internal/exec/analytical_query_population.go'),('compiler.go.txt','internal/nlqexec/analytical_query_population.go'),('exec_test.go.txt','internal/exec/analytical_query_population_test.go')]:
    add(target, (ROOT/source).read_text())

p='internal/exec/analytical.go'
edit(p, '\tGrain     *AnalyticalGrain   `json:"grain,omitempty"`\n', '\tGrain     *AnalyticalGrain   `json:"grain,omitempty"`\n\tQueryPopulation *AnalyticalQueryPopulation `json:"query_population,omitempty"`\n')
edit(p, '\tGrouping []string `json:"grouping,omitempty"`\n', '\tGrouping []string `json:"grouping,omitempty"`\n\tQueryPopulation string `json:"query_population,omitempty"`\n')
edit(p, '&& c.Version != AnalyticalCalendarVersion)', '&& c.Version != AnalyticalCalendarVersion && c.Version != AnalyticalQueryPopulationVersion)')
edit(p, '\tchecker := analyticalChecker{ctx: ctx, relation: relation, parameters: p.candidate.parameters, grain: c.Grain}', '\tif err := validateAnalyticalQueryPopulation(ctx, c, p.candidate.binding); err != nil {\n\t\treturn nil, err\n\t}\n\tchecker := analyticalChecker{ctx: ctx, relation: relation, parameters: p.candidate.parameters, grain: c.Grain, binding: p.candidate.binding, queryPopulation: c.QueryPopulation}')
edit(p, '\treturn receipt, nil\n', '\tif c.QueryPopulation != nil {\n\t\treceipt.QueryPopulation = AnalyticalQueryPopulationPolicy\n\t}\n\treturn receipt, nil\n')
edit(p, '\tcommon     map[string]bool\n', '\tbinding Binding\n\tqueryPopulation *AnalyticalQueryPopulation\n\tcommon     map[string]bool\n')
p='internal/exec/analytical_grain.go'
edit(p, 'c.Version == AnalyticalCalendarVersion && g.Policy == AnalyticalCalendarPolicy', '(c.Version == AnalyticalCalendarVersion || c.Version == AnalyticalQueryPopulationVersion) && g.Policy == AnalyticalCalendarPolicy')
p='internal/exec/analytical_pg.go'
edit(p, '\ttargets := array(q["targetList"])', '\tif err := a.checkQueryPopulation(q); err != nil {\n\t\treturn err\n\t}\n\ttargets := array(q["targetList"])')
p='internal/nlqexec/analytical.go'
edit(p, 'const analyticalRecordVersion = 3', 'const analyticalRecordVersion = 4')
edit(p, 'func compileAnalyticalVersion(ctx context.Context, a admission, version int)', 'func compileAnalyticalVersion(ctx context.Context, a admission, version int, queryConstraints ...[]exec.BusinessConstraint)')
edit(p, '\tif version == 3 {\n\t\tproofVersion = exec.AnalyticalCalendarVersion\n\t}', '\tif version >= 3 {\n\t\tproofVersion = exec.AnalyticalCalendarVersion\n\t}\n\tif version == 4 {\n\t\tproofVersion = exec.AnalyticalQueryPopulationVersion\n\t}')
edit(p, 'compileAnalyticalGrainPolicy(ctx, a, *out, version == 3)', 'compileAnalyticalGrainPolicy(ctx, a, *out, version >= 3)')
edit(p, '\treturn out, nil\n}\n\ntype analyticalCompiler struct {', '\tif out != nil && version == 4 && len(queryConstraints) > 0 {\n\t\tif err := compileQueryPopulation(ctx, a, out, queryConstraints); err != nil { return nil, err }\n\t}\n\treturn out, nil\n}\n\ntype analyticalCompiler struct {')
edit(p, 'func expectedAnalytical(ctx context.Context, q QueryRecord, a admission)', 'func expectedAnalytical(ctx context.Context, q QueryRecord, a admission, queryConstraints ...[]exec.BusinessConstraint)')
edit(p, 'compileAnalyticalVersion(ctx, a, q.AnalyticalVersion)', 'compileAnalyticalVersion(ctx, a, q.AnalyticalVersion, queryConstraints...)')
edit(p, '\tif q.Analytical == nil || exec.Hash(want) != exec.Hash(q.Analytical) {', '\tif contract.QueryPopulation != nil {\n\t\twant.QueryPopulation = exec.AnalyticalQueryPopulationPolicy\n\t}\n\tif q.Analytical == nil || exec.Hash(want) != exec.Hash(q.Analytical) {')
edit(p, 'case "analytical_grain_mismatch", "analytical_metric_mismatch",', 'case "analytical_query_population_mismatch", "analytical_grain_mismatch", "analytical_metric_mismatch",')
edit(p, '\tif q.AnalyticalVersion == 3 {\n\t\tversion = exec.AnalyticalCalendarVersion\n\t}', '\tif q.AnalyticalVersion == 3 {\n\t\tversion = exec.AnalyticalCalendarVersion\n\t}\n\tif q.AnalyticalVersion == 4 {\n\t\tversion = exec.AnalyticalQueryPopulationVersion\n\t}')
edit(p, 'func analyticalReceiptScopeValid(r *exec.AnalyticalReceipt) bool {', 'func analyticalReceiptScopeValid(r *exec.AnalyticalReceipt) bool {\n\tif r.QueryPopulation != "" && (r.Version != exec.AnalyticalQueryPopulationVersion || r.QueryPopulation != exec.AnalyticalQueryPopulationPolicy) { return false }')
edit(p, 'r.Version == exec.AnalyticalCalendarVersion', '(r.Version == exec.AnalyticalCalendarVersion || r.Version == exec.AnalyticalQueryPopulationVersion)', 2)
p='internal/nlqexec/analytical_grain.go'
edit(p, 'func analyticalGrainGuidance(contract *exec.AnalyticalContract) string {', 'func analyticalGrainGuidanceOnly(contract *exec.AnalyticalContract) string {')
p='internal/nlqexec/analytical_query_population.go'
with Path(p).open('a') as f:
    f.write('\nfunc analyticalGrainGuidance(contract *exec.AnalyticalContract) string {\n\treturn analyticalGrainGuidanceOnly(contract) + analyticalPopulationGuidance(contract)\n}\n')

# Production current-plan and durable-run consumers use the authenticated variants.
p='internal/nlqexec/service.go'
text=Path(p).read_text()
n=text.count('compileAnalytical(ctx,')
if n != 1: raise SystemExit(f'current analytical consumer count: {n}')
edit(p, 'compileAnalytical(ctx,', 'compileCurrentAnalytical(ctx,')
edit(p, 'expectedAnalytical(ctx, record,', 's.expectedAnalytical(ctx, e, record,', 3)

# Current authoring chooses v4. Retained-policy fixtures keep explicit old versions.
for p in ['internal/nlqexec/analytical_grain_test.go','internal/nlqexec/analytical_calendar_test.go']:
    edit(p, 'c.Version != exec.AnalyticalCalendarVersion', 'c.Version != exec.AnalyticalQueryPopulationVersion')
p='internal/nlqexec/analytical_calendar_test.go'
edit(p, '\tc, err := compileAnalytical(context.Background(), a)\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\tq := QueryRecord{AnalyticalVersion: 3,', '\tc, err := compileAnalyticalVersion(context.Background(), a, 3)\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n\tq := QueryRecord{AnalyticalVersion: 3,')
for p in ['test/acceptance/sql_analytical_grain_test.go','test/acceptance/sql_analytical_calendar_test.go','test/acceptance/sql_analytical_test.go']:
    text=Path(p).read_text()
    text=text.replace('p.Analytical.Version != readexec.AnalyticalCalendarVersion','p.Analytical.Version != readexec.AnalyticalQueryPopulationVersion')
    text=text.replace('out.Analytical.Version != readexec.AnalyticalCalendarVersion','out.Analytical.Version != readexec.AnalyticalQueryPopulationVersion')
    text=text.replace('saved.AnalyticalVersion != 3','saved.AnalyticalVersion != 4')
    Path(p).write_text(text)

add('internal/store/postgres/migrations/056_nlq_query_population.sql', '''-- Forward-only: retained v0/v1/v2/v3 rows keep their original proof policy.
ALTER TABLE chartworks.nlq_queries
 DROP CONSTRAINT nlq_queries_analytical_version_check,
 DROP CONSTRAINT nlq_analytical_shape,
 ADD CONSTRAINT nlq_queries_analytical_version_check CHECK (analytical_version IN (0,1,2,3,4)),
 ADD CONSTRAINT nlq_analytical_shape CHECK (
  (analytical_version=0 AND analytical IS NULL) OR
  (analytical_version IN (1,2,3,4) AND (analytical IS NULL OR COALESCE(
   jsonb_typeof(analytical)='object' AND octet_length(analytical::text)<=16384
   AND analytical->>'contract' ~ '^[0-9a-f]{64}$'
   AND analytical->>'query' ~ '^[0-9a-f]{64}$'
   AND jsonb_typeof(analytical->'metrics')='array'
   AND jsonb_array_length(analytical->'metrics') BETWEEN 1 AND 32
   AND (NOT analytical ? 'query_population' OR
    (analytical_version=4 AND analytical->>'query_population'='owned-query-predicates-v1'))
   AND (
    (analytical_version=1 AND analytical->>'version'='analytical-metrics-v1'
     AND analytical->>'scope'='selected_metric_expression_and_population;single_base_relation'
     AND NOT analytical ? 'grouping')
    OR
    (analytical_version=2 AND analytical->>'version'='analytical-metrics-v2' AND (
     (analytical->>'scope'='selected_metric_expression_and_population;single_base_relation' AND NOT analytical ? 'grouping') OR
     (analytical->>'scope'='selected_metric_expression_population_and_grouping;single_base_relation'
      AND jsonb_typeof(analytical->'grouping')='array' AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)))
    OR
    (analytical_version IN (3,4) AND analytical->>'version'=CASE analytical_version WHEN 3 THEN 'analytical-metrics-v3' ELSE 'analytical-metrics-v4' END AND (
     (analytical->>'scope'='selected_metric_expression_and_population;single_base_relation' AND NOT analytical ? 'grouping') OR
     (analytical->>'scope' IN ('selected_metric_expression_population_and_grouping;single_base_relation','selected_metric_expression_population_and_calendar_grouping;single_base_relation')
      AND jsonb_typeof(analytical->'grouping')='array' AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)))
   ), false)))
 );
-- The existing immutable trigger also protects the optional query_population
-- marker; only a newly checked execution correction may refresh the query hash.
''')
p='internal/store/postgres/errors_test.go'
edit(p, 'SchemaVersion() != "55" || len(manifest) != 55', 'SchemaVersion() != "56" || len(manifest) != 56')
edit(p, '{54, "migrations/055_nlq_analytical_calendar.sql", "analytical-metrics-v3"},', '{54, "migrations/055_nlq_analytical_calendar.sql", "analytical-metrics-v3"},\n\t\t{55, "migrations/056_nlq_query_population.sql", "owned-query-predicates-v1"},')

add('docs/contracts/analytical-query-population-v4.md', '''# Analytical query population v4

AP-03C1 adds an optional owned-query-predicates-v1 certificate to the existing
native-first PostgreSQL single-base analytical proof. Current authoring chooses
record v4. Retained records 0 through 3 are rebuilt with their original policy.

The normal service obtains typed constraints from the in-process route seal.
Durable execution and terminal replay reconstruct them through the authenticated
clarification/interpretation replayer; saved resolution JSON alone is insufficient.
A pure metric compiler without those verified values cannot issue this certificate.

Every WHERE/HAVING conjunct must match a predicate produced by the existing typed
business binder, or (in WHERE only) a reviewed filter common to all selected metric
leaves. All required predicates must be present. Extra narrowing, changed operators,
changed parameter values, unexpected Boolean structure and hidden HAVING conditions
fail under the existing one-correction allowance. Equivalent-looking expressions
are not guessed: only locations, resolved qualification and parameter numbering
are normalized. Constants, casts, operators and concrete parameter kinds/values
remain exact. The matcher is bounded and does not execute the reference statement.

The separate metric checker continues to enforce aggregate-local populations;
filters for different metrics are not moved into one global intersection. Native
validation remains the only issuer of executable plans. Signed reach, relation
scope, exact source pins, current/retained authority and frozen refresh are unchanged.

The optional query_population receipt marker explicitly names the added policy.
Private constraints exist only in the local contract comparison; public receipts
contain the policy marker plus the existing contract/query digests, not values.
Provider guidance is fixed value-free text. The existing immutable-proof trigger
protects the marker and contract; migration 056 does not rewrite earlier rows.

Scope limits: this certifies predicate provenance/completeness relative to the
resolved typed constraints, not exhaustive natural-language comprehension, intended
ORDER BY/LIMIT, joins/fan-out or general SQL equivalence. With no selected metric or
no typed query constraints, no query-population certificate is added. Broad original
SQL support and other dialects remain separate qualifications, not implied passes.

Tests must cover typed/private values, inclusive/exclusive bounds, NULL policies,
aggregate predicates, independent metric filters, extra restrictions, missing or
changed predicates, immutable old-policy replay and actual PostgreSQL results.
Executed results belong in the PR checkpoint, not inferred from this inventory.
''')
for p in ['docs/plans/phase-18-nlq-generation-execution.md','docs/reviews/sql-context-recovery.md']:
    with Path(p).open('a') as f:
        f.write('\n\n## AP-03C1 owned query-predicate conformance\n\nThe [v4 population contract](../contracts/analytical-query-population-v4.md)\nchecks complete WHERE/HAVING predicate provenance against authenticated typed\nconstraints, retaining common metric filters and all existing native/metric/grain\nchecks. Private values do not enter provider guidance or public receipts. Record\nv4 is forward-only; older policy replay remains distinct. Query-wide natural\nlanguage completeness, ordering, joins and other dialects remain unqualified.\nTests and actual qualification are recorded in PR #62, not asserted by this plan.\n')
print('AP-03C1 source applied; no validation result is claimed by this authoring step')
