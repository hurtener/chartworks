package reporting

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func setParameter() Parameter {
	return Parameter{Name: "regions", Type: "dimension_set", Required: true, Default: &Value{Items: []string{"West"}}, Dimension: &DimensionReference{Topic: "sales", Version: "v1", Dimension: "region"}}
}
func setPredicate() string {
	v := []string{}
	for i := 1; i <= DimensionSetCapacity; i++ {
		v = append(v, "$"+strconv.Itoa(i))
	}
	return `"region" IN (` + strings.Join(v, ", ") + ")"
}
func TestBoundedFilterValues(t *testing.T) {
	p := setParameter()
	for _, items := range [][]string{{"West"}, {"", "x' OR true --", "West"}, {"a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n", "o", "p"}} {
		out, err := ResolveParameters([]Parameter{p}, []Argument{{Name: p.Name, Value: Value{Items: items}}}, testResolution())
		if err != nil || len(out.Parameters) != 16 {
			t.Fatal(out, err)
		}
		for i := len(items); i < 16; i++ {
			if out.Parameters[i].Kind != "null" {
				t.Fatal("missing padding")
			}
		}
	}
	a, err := ResolveParameters([]Parameter{p}, []Argument{{Name: p.Name, Value: Value{Items: []string{"b", "a"}}}}, testResolution())
	if err != nil {
		t.Fatal(err)
	}
	b, err := ResolveParameters([]Parameter{p}, []Argument{{Name: p.Name, Value: Value{Items: []string{"a", "b"}}}}, testResolution())
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("set order changed binds", err)
	}
	for _, v := range []Value{{}, {Items: []string{}}, {Items: []string{"a", "a"}}, {Items: make([]string, 17)}, {Literal: "NULL"}, {Items: []string{"x"}, DateRange: &DateRange{Start: "2026-01-01", EndExclusive: "2026-01-02"}}} {
		if _, err := ResolveParameters([]Parameter{p}, []Argument{{Name: p.Name, Value: v}}, testResolution()); err == nil {
			t.Fatal("invalid set", v)
		}
	}
	p.Type = "date_range"
	p.Default = &Value{DateRange: &DateRange{Start: "2024-02-29", EndExclusive: "2024-03-01"}}
	var baseline []exec.Parameter
	for _, zone := range []string{"UTC", "America/New_York", "Pacific/Apia"} {
		resolution := testResolution()
		resolution.Timezone = zone
		out, err := ResolveParameters([]Parameter{p}, nil, resolution)
		if err != nil || len(out.Parameters) != 2 || out.Parameters[0].Value != "2024-02-29" || out.Parameters[1].Value != "2024-03-01" {
			t.Fatal(zone, out, err)
		}
		if baseline != nil && !reflect.DeepEqual(baseline, out.Parameters) {
			t.Fatal("server zone changed dates")
		}
		baseline = out.Parameters
	}
	for _, r := range []DateRange{{"2023-02-29", "2023-03-01"}, {"2024-03-01", "2024-03-01"}, {"2024-03-02", "2024-03-01"}, {"2024-01-01T00:00:00Z", "2024-02-01"}, {"0000-01-01", "2024-01-01"}, {"9999-12-31", "10000-01-01"}, {"1900-01-01", "2026-01-01"}} {
		if _, err := ResolveParameters([]Parameter{p}, []Argument{{Name: p.Name, Value: Value{DateRange: &r}}}, testResolution()); err == nil {
			t.Fatal("invalid date range", r)
		}
	}
	legacy := Parameter{Name: "n", Type: "integer", Default: &Value{Literal: "1"}}
	raw, _ := json.Marshal(legacy)
	if strings.Contains(string(raw), "date_range") {
		t.Fatal("legacy digest surface changed")
	}
	if _, err := ResolveParameters([]Parameter{legacy}, []Argument{{Name: "n", Value: Value{Literal: "1", DateRange: p.Default.DateRange}}}, testResolution()); err == nil {
		t.Fatal("cross-type payload accepted")
	}
}

func TestBoundedFilterSQLPlacement(t *testing.T) {
	p := setParameter()
	good := `SELECT sum("amount") FROM "analytics"."sales" WHERE ` + setPredicate()
	if err := validateBoundedFilterSQL(t.Context(), good, []Parameter{p}); err != nil {
		t.Fatal("positive IN", err)
	}
	cases := map[string]string{
		"negative in":         strings.Replace(good, " IN ", " NOT IN ", 1),
		"outer negation":      strings.Replace(good, "WHERE ", "WHERE NOT (", 1) + ")",
		"or":                  good + " OR true",
		"is false":            "SELECT sum(amount) FROM analytics.sales WHERE (" + setPredicate() + ") IS FALSE",
		"projection reuse":    strings.Replace(good, "sum(\"amount\")", "sum(\"amount\"), $1", 1),
		"partial list":        strings.Replace(good, ", $16)", ")", 1),
		"duplicate slot":      strings.Replace(good, "$16)", "$15)", 1),
		"expression slot":     strings.Replace(good, "$1,", "lower($1),", 1),
		"different type cast": strings.Replace(good, "$1,", "$1::integer,", 1),
		"subquery":            "SELECT * FROM (" + good + ") nested",
		"projection subquery": strings.Replace(good, `sum("amount")`, `(SELECT sum(amount) FROM analytics.sales)`, 1),
		"where subquery":      good + ` AND EXISTS (SELECT 1 FROM analytics.sales)`,
		"cte":                 `WITH ignored AS (SELECT amount FROM analytics.sales) ` + good,
		"union":               good + ` UNION ALL SELECT sum(amount) FROM analytics.sales`,
		"having":              strings.Replace(good, "WHERE", "HAVING", 1),
	}
	for name, sql := range cases {
		t.Run(name, func(t *testing.T) {
			if validateBoundedFilterSQL(t.Context(), sql, []Parameter{p}) == nil {
				t.Fatal("unsafe placement accepted")
			}
		})
	}
	date := Parameter{Name: "days", Type: "date_range", Required: true, Dimension: p.Dimension, Default: &Value{DateRange: &DateRange{Start: "2026-01-01", EndExclusive: "2026-02-01"}}}
	for _, sql := range []string{`SELECT sum(amount) FROM analytics.sales WHERE day >= $1 AND day < $2`, `SELECT sum(amount) FROM analytics.sales s WHERE s.day >= $1 AND s.day < $2`} {
		if err := validateBoundedFilterSQL(t.Context(), sql, []Parameter{date}); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{`SELECT sum(amount) FROM analytics.sales WHERE day >= $1 AND another_day < $2`, `SELECT sum(amount) FROM analytics.sales WHERE day > $1 AND day <= $2`, `SELECT sum(amount) FROM analytics.sales WHERE day BETWEEN $1 AND $2`, `SELECT sum(amount) FROM analytics.sales WHERE NOT (day >= $1 AND day < $2)`} {
		if validateBoundedFilterSQL(t.Context(), sql, []Parameter{date}) == nil {
			t.Fatal("invalid range predicate")
		}
	}
	// Existing scalar/fixed-list SQL keeps its previous acceptance behavior.
	for _, kind := range []string{"dimension_value", "dimension_list"} {
		legacy := p
		legacy.Type = kind
		if validateBoundedFilterSQL(t.Context(), "SELECT unsupported_legacy_shape($1)", []Parameter{legacy}) != nil {
			t.Fatal("legacy rewrite")
		}
	}
}

func TestBoundedFilterReviewedColumnIdentity(t *testing.T) {
	_, publication, _, binding := preparationCompileFixture()
	p := setParameter()
	for _, prefix := range []string{"", `sales.`, `analytics.sales.`, `s.`} {
		sql := `SELECT sum(amount) FROM analytics.sales`
		if prefix == "s." {
			sql += " s"
		}
		sql += " WHERE " + strings.Replace(setPredicate(), `"region"`, prefix+`"region"`, 1)
		d := Definition{Source: binding.Source, Context: binding.Context, SQL: sql, Parameters: []Parameter{p}}
		if err := validateBoundedFilterSemantics(t.Context(), d, binding, []topics.Published{publication}); err != nil {
			t.Fatal(prefix, err)
		}
	}
	for _, sql := range []string{
		`SELECT sum(amount) FROM analytics.sales WHERE ` + strings.Replace(setPredicate(), `"region"`, `"amount"`, 1),
		`SELECT sum(amount) FROM analytics.sales s WHERE ` + strings.Replace(setPredicate(), `"region"`, `sales."region"`, 1),
		`SELECT sum(amount) FROM other.sales WHERE ` + setPredicate(),
		`SELECT sum(amount) FROM analytics.sales JOIN analytics.other USING(region) WHERE ` + setPredicate(),
		`SELECT sum(amount) FROM analytics.sales s(other) WHERE ` + setPredicate(),
	} {
		if validateBoundedFilterSemantics(t.Context(), Definition{SQL: sql, Parameters: []Parameter{p}}, binding, []topics.Published{publication}) == nil {
			t.Fatal("misdeclared semantic column admitted")
		}
	}
	publication.Definition.Dimensions[0].Filters = []semantics.SemanticFilter{{ID: "mandatory"}}
	if validateBoundedFilterSemantics(t.Context(), Definition{SQL: `SELECT sum(amount) FROM analytics.sales WHERE ` + setPredicate(), Parameters: []Parameter{p}}, binding, []topics.Published{publication}) == nil {
		t.Fatal("mandatory filter policy dropped")
	}
}
