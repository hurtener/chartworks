"""Apply an explicit, bounded AP-03B2 source checkpoint; never edit live data."""
from pathlib import Path

def edit(path, old, new, count=1):
    p = Path(path)
    text = p.read_text()
    if text.count(old) != count:
        raise SystemExit(f'Unexpected source for {path}: {text.count(old)} != {count}')
    p.write_text(text.replace(old, new))

def put(path, text):
    p = Path(path)
    if p.exists():
        raise SystemExit(f'New checkpoint file already exists: {path}')
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(text.lstrip('\n'))

put('internal/exec/analytical_calendar.go', r'''
package exec

import (
    "strings"
    "time"
)

// AnalyticalCalendarVersion preserves v1/v2 replay while adding reviewed buckets.
const AnalyticalCalendarVersion = "analytical-metrics-v3"
const AnalyticalCalendarPolicy = "reviewed-calendar-suffix-v1"
const AnalyticalCalendarScope = "selected_metric_expression_population_and_calendar_grouping;single_base_relation"

// AnalyticalBucket specifies a partition, not display formatting or query SQL.
// Timezone is mandatory only for instant-valued fields; civil values do not shift.
type AnalyticalBucket struct {
    Column string `json:"column"`
    Grain string `json:"grain"`
    Calendar string `json:"calendar"`
    Timezone string `json:"timezone,omitempty"`
}

// AnalyticalCalendarKind recognizes exact PostgreSQL native types, not a broad
// connector category. A temporal category alone cannot establish zone semantics.
func AnalyticalCalendarKind(nativeType, category string) string {
    if category != "temporal" { return "" }
    switch strings.ToLower(strings.TrimSpace(nativeType)) {
    case "date": return "date"
    case "timestamp", "timestamp without time zone": return "civil"
    case "timestamptz", "timestamp with time zone": return "instant"
    default: return ""
    }
}

func validAnalyticalBucket(b AnalyticalBucket, col Column) bool {
    if !col.Safe || b.Column != col.Name || b.Calendar != "gregorian" { return false }
    switch b.Grain { case "day", "month", "quarter", "year": default: return false }
    switch AnalyticalCalendarKind(col.NativeType, col.Category) {
    case "date", "civil": return b.Timezone == ""
    case "instant":
        if b.Timezone == "" || b.Timezone == "Local" || len(b.Timezone) > 128 { return false }
        _, err := time.LoadLocation(b.Timezone)
        return err == nil
    default: return false
    }
}

func analyticalBucketKey(b AnalyticalBucket) string { return "bucket:" + Hash(b) }
func (t analyticalTerm) groupKey() string {
    if t.bucket != "" { return t.bucket }
    if t.column != "" { return "column:" + t.column }
    return ""
}

func calendarCast(node any, name string) (any, bool) {
    cast := object(object(node)["TypeCast"])
    if cast == nil { return nil, false }
    typ := fieldObject(cast["typeName"], "TypeName")
    parts, ok := names(typ["names"])
    if len(parts) == 2 && parts[0] == "pg_catalog" { parts = parts[1:] }
    if !ok || len(parts) != 1 || parts[0] != name || len(array(typ["typmods"])) > 0 || len(array(typ["arrayBounds"])) > 0 || truth(typ["setof"]) { return nil, false }
    return cast["arg"], true
}
func calendarLiteral(node any) (string, bool) {
    c := object(object(node)["A_Const"])
    value := object(c["sval"])
    if c == nil || truth(c["isnull"]) || value == nil { return "", false }
    return text(value["sval"]), true
}

// calendarTerm recognizes only partitions for which field type, unit and zone
// are checked. EXTRACT(month), display strings and session-zone coercions do not
// acquire bucket equivalence. It runs only under the new versioned grain policy.
func (a *analyticalChecker) calendarTerm(node any, depth int) (analyticalTerm, bool, error) {
    bad := func() (analyticalTerm, bool, error) {
        return analyticalTerm{}, true, analyticalFailure("analytical_grain_unsupported", true)
    }
    if arg, ok := calendarCast(node, "date"); ok {
        t, err := a.term(arg, depth+1)
        if err != nil { return analyticalTerm{}, true, err }
        // Casting a zoned midnight to date consults the session timezone.
        if t.bucket == "" || t.zoned { return bad() }
        return t, true, nil
    }
    f := object(object(node)["FuncCall"])
    parts, ok := names(f["funcname"])
    if len(parts) == 2 && parts[0] == "pg_catalog" { parts = parts[1:] }
    if !ok || len(parts) != 1 || parts[0] != "date_trunc" { return analyticalTerm{}, false, nil }
    if !only(f, "funcname", "args", "location", "funcformat") { return bad() }
    args := array(f["args"])
    if len(args) != 2 && len(args) != 3 { return bad() }
    grain, ok := calendarLiteral(args[0])
    if !ok { return bad() }
    source := args[1]
    casted := false
    if arg, ok := calendarCast(source, "timestamp"); ok { source, casted = arg, true }
    col, ok := a.field(source)
    if !ok { return bad() }
    kind := AnalyticalCalendarKind(col.NativeType, col.Category)
    if kind == "date" && !casted || kind == "instant" && casted || kind == "" { return bad() }
    b := AnalyticalBucket{Column: col.Name, Grain: strings.ToLower(grain), Calendar: "gregorian"}
    if len(args) == 3 {
        if kind != "instant" { return bad() }
        b.Timezone, ok = calendarLiteral(args[2])
        if !ok { return bad() }
    }
    if !validAnalyticalBucket(b, col) { return bad() }
    return analyticalTerm{bucket: analyticalBucketKey(b), zoned: kind == "instant"}, true, nil
}
''')

edit('internal/exec/analytical.go', 'c.Version != AnalyticalVersion && c.Version != AnalyticalGrainVersion', 'c.Version != AnalyticalVersion && c.Version != AnalyticalGrainVersion && c.Version != AnalyticalCalendarVersion')
edit('internal/exec/analytical.go', 'receipt.Scope = AnalyticalGrainScope', 'receipt.Scope = AnalyticalGrainScope\n\t\tif len(c.Grain.Buckets) > 0 { receipt.Scope = AnalyticalCalendarScope }')
edit('internal/exec/analytical_grain.go', 'Dimensions []string `json:"dimensions"`', 'Dimensions []string `json:"dimensions"`\n\tBuckets []AnalyticalBucket `json:"buckets,omitempty"`')
edit('internal/exec/analytical_grain.go', 'if c.Version != AnalyticalGrainVersion || g.Policy != AnalyticalGrainPolicy || len(g.Columns) < 1 || len(g.Columns) > 16 || len(g.Dimensions) < 1 || len(g.Dimensions) > 16 {', '''validPolicy := c.Version == AnalyticalGrainVersion && g.Policy == AnalyticalGrainPolicy && len(g.Buckets) == 0 || c.Version == AnalyticalCalendarVersion && g.Policy == AnalyticalCalendarPolicy
    if !validPolicy || len(g.Columns)+len(g.Buckets) < 1 || len(g.Columns)+len(g.Buckets) > 16 || len(g.Dimensions) < 1 || len(g.Dimensions) > 16 {''')
edit('internal/exec/analytical_grain.go', '\tfor i, id := range g.Dimensions {', '''    for i, bucket := range g.Buckets {
        if i > 0 && analyticalBucketKey(g.Buckets[i-1]) >= analyticalBucketKey(bucket) { return ErrBinding }
        matches := 0
        for _, col := range relation.Columns { if validAnalyticalBucket(bucket, col) { matches++ } }
        if matches != 1 { return ErrBinding }
    }
\tfor i, id := range g.Dimensions {''')
p = Path('internal/exec/analytical_grain.go')
s = p.read_text(); pos = s.index('func (a *analyticalChecker) checkGrain(')
p.write_text(s[:pos] + r'''func (a *analyticalChecker) checkGrain(groups map[string]bool, terms []analyticalTerm) error {
    if a.grain == nil { return nil }
    expected := map[string]bool{}
    for _, column := range a.grain.Columns { expected["column:"+column] = true }
    for _, bucket := range a.grain.Buckets { expected[analyticalBucketKey(bucket)] = true }
    mismatch := func() error { return analyticalFailure("analytical_grain_mismatch", false) }
    if len(groups) != len(expected) { return mismatch() }
    projected := map[string]bool{}
    for _, term := range terms { if key := term.groupKey(); key != "" { projected[key] = true } }
    if len(projected) != len(expected) { return mismatch() }
    for key := range expected { if !groups[key] || !projected[key] { return mismatch() } }
    return nil
}
''')
edit('internal/exec/analytical_pg.go', '\tconstant                           string', '\tconstant                           string\n\tbucket string\n\tzoned bool')
edit('internal/exec/analytical_pg.go', '} else if term.column == "" {', '} else if term.groupKey() == "" {')
edit('internal/exec/analytical_pg.go', 'if term.column == "" || term.aggregate || term.guarded {', 'if term.groupKey() == "" || term.aggregate || term.guarded {')
edit('internal/exec/analytical_pg.go', 'groups[term.column] = true', 'groups[term.groupKey()] = true')
edit('internal/exec/analytical_pg.go', 'if term.column != "" && !groups[term.column] {', 'if term.groupKey() != "" && !groups[term.groupKey()] {')
edit('internal/exec/analytical_pg.go', '\tif cast := object(root["TypeCast"]); cast != nil {', '''    if a.grain != nil && a.grain.Policy == AnalyticalCalendarPolicy {
        if term, handled, err := a.calendarTerm(node, depth); handled { return term, err }
    }
\tif cast := object(root["TypeCast"]); cast != nil {''')

put('internal/nlqexec/analytical_calendar.go', r'''
package nlqexec

import (
    "time"
    "github.com/hurtener/chartworks/internal/exec"
    "github.com/hurtener/chartworks/internal/semantics"
)
func calendarGrainWord(s string) string {
    switch s {
    case "day", "día", "dia": return "day"
    case "month", "mes": return "month"
    case "quarter", "trimestre": return "quarter"
    case "year", "año", "ano": return "year"
    default: return ""
    }
}
func compileCalendarBucket(d grainDimension, col semantics.Column, grain string) (exec.AnalyticalBucket, error) {
    bad := func() (exec.AnalyticalBucket, error) { return exec.AnalyticalBucket{}, analyticalUnsupported("analytical_grain_unsupported") }
    if d.role != semantics.DimensionTemporal || d.temporal == nil || d.temporal.Calendar != "gregorian" || len(d.filters) != 0 { return bad() }
    allowed := false
    for _, g := range d.temporal.Grains { if string(g) == grain { allowed = true } }
    if !allowed { return bad() }
    b := exec.AnalyticalBucket{Column: col.SourceName, Grain: grain, Calendar: "gregorian"}
    switch exec.AnalyticalCalendarKind(col.NativeType, col.Category) {
    case "date", "civil":
    case "instant":
        b.Timezone = d.temporal.Timezone
        if b.Timezone == "" || b.Timezone == "Local" || len(b.Timezone) > 128 { return bad() }
        if _, err := time.LoadLocation(b.Timezone); err != nil { return bad() }
    default: return bad()
    }
    return b, nil
}
''')
edit('internal/nlqexec/analytical_grain.go', '\tfilters []semantics.SemanticFilter', '\tfilters []semantics.SemanticFilter\n\ttemporal *semantics.TemporalPolicy')
edit('internal/nlqexec/analytical_grain.go', 'func compileAnalyticalGrain(ctx context.Context, a admission, contract exec.AnalyticalContract) (*exec.AnalyticalGrain, error) {', '''func compileAnalyticalGrain(ctx context.Context, a admission, contract exec.AnalyticalContract) (*exec.AnalyticalGrain, error) {
    return compileAnalyticalGrainPolicy(ctx, a, contract, false)
}
func compileAnalyticalGrainPolicy(ctx context.Context, a admission, contract exec.AnalyticalContract, calendar bool) (*exec.AnalyticalGrain, error) {''')
edit('internal/nlqexec/analytical_grain.go', 'role: d.Role, filters: d.Filters}', 'role: d.Role, filters: d.Filters, temporal: d.Temporal}')
edit('internal/nlqexec/analytical_grain.go', '\tcolumns, dimensions := map[string]bool{}, map[string]bool{}', '''    if calendar { result.Policy = exec.AnalyticalCalendarPolicy }
    buckets := map[string]exec.AnalyticalBucket{}
\tcolumns, dimensions := map[string]bool{}, map[string]bool{}''')
edit('internal/nlqexec/analytical_grain.go', '\t\tif len(choices) == 0 && len(dimensions) == 0 {', '''        grain := ""
        if calendar && len(choices) == 0 && start+1 < len(words) && (words[start+1] == "of" || words[start+1] == "de") {
            grain = calendarGrainWord(words[start])
            if grain != "" {
                start += 2
                width = min(maxWords, len(words)-start)
                for ; width > 0; width-- {
                    if values := terms[strings.Join(words[start:start+width], " ")]; len(values) > 0 { choices = values; break }
                }
                if len(choices) == 0 { return nil, analyticalUnsupported("analytical_grain_unsupported") }
            }
        }
\t\tif len(choices) == 0 && len(dimensions) == 0 {''')
edit('internal/nlqexec/analytical_grain.go', 'if chosen.role == semantics.DimensionTemporal || len(chosen.filters) > 0 {', 'if chosen.role == semantics.DimensionTemporal && grain == "" || grain != "" && chosen.role != semantics.DimensionTemporal || len(chosen.filters) > 0 {')
edit('internal/nlqexec/analytical_grain.go', '\t\tcolumns[col.SourceName], dimensions[chosen.id] = true, true', '''        dimensions[chosen.id] = true
        if grain == "" { columns[col.SourceName] = true } else {
            bucket, err := compileCalendarBucket(chosen, col, grain)
            if err != nil { return nil, err }
            buckets[exec.Hash(bucket)] = bucket
        }
        if len(columns)+len(buckets) > 16 { return nil, exec.ErrLimit }''')
edit('internal/nlqexec/analytical_grain.go', '\tsort.Strings(result.Columns)', '''    for _, bucket := range buckets { result.Buckets = append(result.Buckets, bucket) }
    sort.Slice(result.Buckets, func(i,j int) bool { return exec.Hash(result.Buckets[i]) < exec.Hash(result.Buckets[j]) })
\tsort.Strings(result.Columns)''')
edit('internal/nlqexec/analytical_grain.go', '\treturn " The exact selected grouping is enforced:', '''    if len(contract.Grain.Buckets) > 0 {
        return " The exact selected grouping is enforced. Project and GROUP BY every direct column and calendar bucket; preserve year and NULL groups. Use date_trunc with the exact literal unit. For date inputs explicitly cast the field to timestamp without time zone first; civil timestamps stay unzoned. For timestamptz inputs use the exact reviewed timezone as date_trunc's third argument; never rely on session timezone or cast its result to date. Do not replace buckets with EXTRACT(month), formatted strings or scalar totals. Grouping contract: " + string(raw)
    }
\treturn " The exact selected grouping is enforced:''')

edit('internal/nlqexec/analytical.go', 'const analyticalRecordVersion = 2', 'const analyticalRecordVersion = 3')
edit('internal/nlqexec/analytical.go', 'if version != 1 && version != analyticalRecordVersion {', 'if version < 1 || version > analyticalRecordVersion {')
edit('internal/nlqexec/analytical.go', '''if version == analyticalRecordVersion {
		proofVersion = exec.AnalyticalGrainVersion
	}''', '''if version == 2 { proofVersion = exec.AnalyticalGrainVersion }
    if version == 3 { proofVersion = exec.AnalyticalCalendarVersion }''')
edit('internal/nlqexec/analytical.go', '''if version == analyticalRecordVersion {
			var err error
			out.Grain, err = compileAnalyticalGrain(ctx, a, *out)''', '''if version >= 2 {
            var err error
            out.Grain, err = compileAnalyticalGrainPolicy(ctx, a, *out, version == 3)''')
edit('internal/nlqexec/analytical.go', 'if q.AnalyticalVersion != 1 && q.AnalyticalVersion != analyticalRecordVersion {', 'if q.AnalyticalVersion < 1 || q.AnalyticalVersion > analyticalRecordVersion {', 2)
edit('internal/nlqexec/analytical.go', 'want.Scope = exec.AnalyticalGrainScope', 'want.Scope = exec.AnalyticalGrainScope\n\t\tif len(contract.Grain.Buckets) > 0 { want.Scope = exec.AnalyticalCalendarScope }')
edit('internal/nlqexec/analytical.go', '''if q.AnalyticalVersion == analyticalRecordVersion {
		version = exec.AnalyticalGrainVersion
	}''', '''if q.AnalyticalVersion == 2 { version = exec.AnalyticalGrainVersion }
    if q.AnalyticalVersion == 3 { version = exec.AnalyticalCalendarVersion }''')
edit('internal/nlqexec/analytical.go', 'if r.Scope != exec.AnalyticalGrainScope || r.Version != exec.AnalyticalGrainVersion || len(r.Grouping) < 1 || len(r.Grouping) > 16 {', '''validScope := r.Scope == exec.AnalyticalGrainScope && (r.Version == exec.AnalyticalGrainVersion || r.Version == exec.AnalyticalCalendarVersion) || r.Scope == exec.AnalyticalCalendarScope && r.Version == exec.AnalyticalCalendarVersion
    if !validScope || len(r.Grouping) < 1 || len(r.Grouping) > 16 {''')

migration = Path('internal/store/postgres/migrations/054_nlq_analytical_grain.sql').read_text()
migration = migration.replace('-- Keep the v1 receipt and version-zero unknown rows untouched. New authoring\n-- chooses v2; replay must reconstruct the policy selected by the original row.', '-- Keep all v0/v1/v2 records untouched. New authoring selects v3; retained replay\n-- reconstructs the original policy, never a newer calendar interpretation.')
migration = migration.replace('IN (0,1,2)', 'IN (0,1,2,3)').replace('IN (1,2)', 'IN (1,2,3)')
migration = migration.replace("(analytical_version=2 AND analytical->>'version'='analytical-metrics-v2' AND (", "(((analytical_version=2 AND analytical->>'version'='analytical-metrics-v2') OR (analytical_version=3 AND analytical->>'version'='analytical-metrics-v3')) AND (")
migration = migration.replace("    ))\n   ), false)))", "    )) OR\n    (analytical_version=3 AND analytical->>'version'='analytical-metrics-v3'\n     AND analytical->>'scope'='selected_metric_expression_population_and_calendar_grouping;single_base_relation'\n     AND jsonb_typeof(analytical->'grouping')='array'\n     AND jsonb_array_length(analytical->'grouping') BETWEEN 1 AND 16)\n   ), false)))")
put('internal/store/postgres/migrations/055_nlq_analytical_calendar.sql', migration)
# Update new-authoring expectations only. Explicit retained v1/v2 compiler tests
# and previously shipped migration files retain their original policy.
for path in ['internal/nlqexec/analytical_grain_test.go', 'test/acceptance/sql_analytical_grain_test.go']:
    p = Path(path); s = p.read_text()
    s = s.replace('Version != exec.AnalyticalGrainVersion', 'Version != exec.AnalyticalCalendarVersion')
    s = s.replace('AnalyticalVersion != 2', 'AnalyticalVersion != 3')
    if path.startswith('test/'):
        s = s.replace('reviewed-dimension-suffix-v1', 'reviewed-calendar-suffix-v1')
    p.write_text(s)
p = Path('test/acceptance/sql_analytical_test.go'); p.write_text(p.read_text().replace('AnalyticalVersion != 2', 'AnalyticalVersion != 3'))

put('internal/exec/analytical_calendar_test.go', r'''
package exec
import (
    "context"
    "errors"
    "testing"
)
func calendarPlan(t *testing.T, sql, native, zone string) (Plan, AnalyticalContract) {
    t.Helper()
    p,c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
    p.candidate.binding.Relations[0].Columns = append(p.candidate.binding.Relations[0].Columns, Column{Name:"event_at", NativeType:native, Category:"temporal", Nullable:true, Safe:true})
    c.Binding, c.Version = Hash(p.candidate.binding), AnalyticalCalendarVersion
    c.Grain = &AnalyticalGrain{Policy:AnalyticalCalendarPolicy, Dimensions:[]string{"sales:dimension:event"}, Buckets:[]AnalyticalBucket{{Column:"event_at", Grain:"month", Calendar:"gregorian", Timezone:zone}}}
    return p,c
}
func TestSQLRecoveryCalendarNativePartitions(t *testing.T) {
    for _, tc := range []struct{ native, expression, zone string }{
        {"date", "date_trunc('month', event_at::timestamp)", ""},
        {"timestamp without time zone", "date_trunc('month', event_at)::date", ""},
        {"timestamp", "pg_catalog.date_trunc('MONTH', event_at)", ""},
        {"timestamptz", "date_trunc('month', event_at, 'America/New_York')", "America/New_York"},
        {"timestamp with time zone", "date_trunc('month', event_at, 'UTC')", "UTC"},
    } {
        t.Run(tc.native, func(t *testing.T) {
            for _, group := range []string{"1", "period", tc.expression} {
                p,c := calendarPlan(t, "SELECT "+tc.expression+" AS period, sum(amount) FROM analytics.sales GROUP BY "+group, tc.native, tc.zone)
                before := Hash(c)
                r,err := CheckAnalyticalPlan(context.Background(), p,c)
                if err != nil || r == nil || r.Scope != AnalyticalCalendarScope || r.Version != AnalyticalCalendarVersion || Hash(c) != before { t.Fatalf("calendar proof: %v %+v",err,r) }
            }
        })
    }
}
func TestSQLRecoveryCalendarWrongAndAmbientPartitions(t *testing.T) {
    for _, expression := range []string{
        "date_trunc('year', event_at, 'America/New_York')",
        "date_trunc('month', event_at)",
        "date_trunc('month', event_at, 'UTC')",
        "date_trunc('month', event_at, 'America/New_York')::date",
        "date_trunc('month', event_at::timestamp)",
        "date_trunc($1, event_at, 'America/New_York')",
        "date_trunc('month', event_at + interval '1 day', 'America/New_York')",
        "extract(month from event_at)", "event_at",
    } {
        t.Run(expression, func(t *testing.T) {
            p,c := calendarPlan(t,"SELECT "+expression+" AS period, sum(amount) FROM analytics.sales GROUP BY 1","timestamptz","America/New_York")
            if _,err := CheckAnalyticalPlan(context.Background(),p,c); err == nil { t.Fatal("unproved partition accepted") }
        })
    }
    p,c := calendarPlan(t,"SELECT date_trunc('month',event_at,'America/New_York'),sum(amount) FROM analytics.sales GROUP BY 1,id","timestamptz","America/New_York")
    if _,err := CheckAnalyticalPlan(context.Background(),p,c); !errors.Is(err,ErrAnalyticalMismatch) { t.Fatal("hidden group",err) }
    p,c = calendarPlan(t,"SELECT date_trunc('month',event_at),sum(amount) FROM analytics.sales GROUP BY 1","date","")
    if _,err := CheckAnalyticalPlan(context.Background(),p,c); err == nil { t.Fatal("date implicitly chose zoned overload") }
}
func TestSQLRecoveryCalendarProofBoundaries(t *testing.T) {
    statement := "SELECT date_trunc('month',event_at,'America/New_York'),sum(amount) FROM analytics.sales GROUP BY 1"
    for _, mutate := range []func(*AnalyticalContract){
        func(c *AnalyticalContract){c.Version=AnalyticalGrainVersion},
        func(c *AnalyticalContract){c.Grain.Policy=AnalyticalGrainPolicy},
        func(c *AnalyticalContract){c.Grain.Buckets[0].Calendar="fiscal"},
        func(c *AnalyticalContract){c.Grain.Buckets[0].Timezone="Local"},
        func(c *AnalyticalContract){c.Grain.Buckets[0].Timezone=""},
        func(c *AnalyticalContract){c.Grain.Buckets[0].Column="foreign"},
        func(c *AnalyticalContract){c.Grain.Buckets[0].Grain="week"},
        func(c *AnalyticalContract){c.Grain.Buckets=append(c.Grain.Buckets,c.Grain.Buckets[0])},
    } {
        p,c := calendarPlan(t,statement,"timestamptz","America/New_York"); mutate(&c)
        if _,err := CheckAnalyticalPlan(context.Background(),p,c); !errors.Is(err,ErrBinding) {t.Fatal("contract",err)}
    }
    p,c := calendarPlan(t,statement,"timestamptz","America/New_York"); p.nativeChecked=false
    if _,err := CheckAnalyticalPlan(context.Background(),p,c); !errors.Is(err,ErrBinding) { t.Fatal("native gate bypassed",err) }
}
func TestSQLRecoveryCalendarExactTypes(t *testing.T) {
    for _, tc := range []struct{native,category,want string}{
        {"date","temporal","date"},{"timestamp","temporal","civil"},{"timestamptz","temporal","instant"},
        {"timestamp(3)","temporal",""},{"time","temporal",""},{"text","temporal",""},{"date","text",""},
    } {if got:=AnalyticalCalendarKind(tc.native,tc.category);got!=tc.want{t.Fatalf("%+v = %q",tc,got)}}
}
''')

put('docs/contracts/analytical-calendar-v3.md', '''# Analytical calendar grain v3

New authoring selects analytical record version 3. Versions 0, 1 and 2 remain
unchanged and rebuild their original policies on execution and replay.

This adds a scoped PostgreSQL single-base calendar partition proof, not general
natural-language correctness, query-wide filters, ordering, joins or approval.
The complete terminal by/per/por clause may name day/month/quarter/year of an
already selected reviewed temporal dimension, or día/mes/trimestre/año de it.
Exact direct dimension labels take precedence. Generic “by month” does not guess
a date column. The existing quoted/private/negative-clause exclusions remain.

The selected dimension must review Gregorian calendar and the requested grain.
Exact native date, timestamp without time zone, and timestamp with time zone
mean different things. A date needs an explicit unzoned timestamp cast before
date_trunc; a civil timestamp needs no zone. An instant requires the exact
reviewed IANA timezone as the third date_trunc argument. Session-zone coercions,
EXTRACT(month), formatted labels, arithmetic source fields and unknown native
types cannot acquire partition equivalence. Civil midnight buckets may be cast
to date; zoned bucket-to-date casts are not proven. Week/hour/fiscal grains,
filtered dimensions and multi-relation shapes remain unsupported.

Native validation still runs first and is the sole issuer of executable plans.
The analytical check compares the complete GROUP BY and projected partition
sets while retaining the aggregate/population/zero-division checks. Wrong grain
uses the existing bounded validation correction; private scalar values remain
server-owned. No model calls are added to frozen report refresh.

Migration 055 accepts v3 without rewriting prior rows or removing the immutable
proof trigger. The contract digest pins grain/calendar/zone/field metadata;
receipt grouping IDs remain reviewed dimension identities. Unknown grouping
remains unmeasured, not a grand-total assertion.

Qualification: synthetic/native unit and real PostgreSQL/recorded-provider
acceptance are required. Live interpretation accuracy, warehouse parity and
query-wide business correctness are not implied by these scoped tests.
''')
for path in ['docs/reviews/sql-context-recovery.md','docs/plans/phase-18-nlq-generation-execution.md']:
    p=Path(path)
    p.write_text(p.read_text()+'''\n\n## AP-03B2 reviewed calendar partitions\n\nThe [calendar grain contract](../contracts/analytical-calendar-v3.md) extends\nnew authoring with explicit reviewed Gregorian day/month/quarter/year buckets\nwhile retaining original v1/v2 replay. Exact native temporal typing and explicit\ninstant timezone are required; the complete grouping/output partition set is\nchecked after native validation. This is not query-wide population, join or\nordering conformance. Final runtime qualification is recorded in PR #62; code\npresence alone is not a passing acceptance result.\n''')
print('Explicit calendar checkpoint applied; runtime qualification still required.')
