package exec

import (
	"context"
	"errors"
	"testing"
)

func analyticalJoinFixture(t *testing.T, sql string) (Plan, AnalyticalContract) {
	p, c := analyticalFixture(t, sql, analyticalMetrics(analyticalMeasure("sum", "amount")))
	p.candidate.binding.Relations = p.candidate.binding.Relations[:1]
	p.candidate.binding.Relations[0].UniqueKeys = [][]string{{"id"}}
	p.candidate.binding.Relations = append(p.candidate.binding.Relations, Relation{ID: "customers", Schema: "analytics", Name: "customers", Columns: []Column{{Name: "id", NativeType: p.candidate.binding.Relations[0].Columns[0].NativeType, Category: "numeric", Safe: true}, {Name: "region", NativeType: "text", Category: "text", Safe: true}}, UniqueKeys: [][]string{{"id"}}})
	// Locate the actual id type rather than depending on fixture column order.
	for _, col := range p.candidate.binding.Relations[0].Columns {
		if col.Name == "id" {
			p.candidate.binding.Relations[1].Columns[0].NativeType = col.NativeType
		}
	}
	c.Version = AnalyticalIntentVersion
	c.Binding = Hash(p.candidate.binding)
	c.Joins = []AnalyticalJoin{{Left: "sales", Right: "customers", Type: "left", LeftColumns: []string{"id"}, RightColumns: []string{"id"}}}
	c.Grain = &AnalyticalGrain{Policy: AnalyticalGroupingPolicy, Columns: []string{"customers/region"}, Dimensions: []string{"customer-region"}}
	return p, c
}

func TestSQLRecoveryAnalyticalPhysicalJoins(t *testing.T) {
	for _, tc := range []struct {
		sql  string
		pass bool
	}{
		{`SELECT c.region,sum(s.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`, true},
		{`SELECT c.region,sum(s.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON c.id=s.id GROUP BY c.region`, true},
		{`SELECT c.region,sum(s.amount) FROM analytics.sales s JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`, false},
		{`SELECT c.region,sum(s.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id AND s.active GROUP BY c.region`, false},
		{`SELECT c.region,sum(s.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id IS NOT DISTINCT FROM c.id GROUP BY c.region`, false},
		{`SELECT c.region,sum(s.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.amount=c.id GROUP BY c.region`, false},
		{`SELECT c.region,sum(amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`, true},
		{`SELECT c.region,sum(id) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`, false},
	} {
		t.Run(tc.sql, func(t *testing.T) {
			p, c := analyticalJoinFixture(t, tc.sql)
			receipt, err := CheckAnalyticalPlan(context.Background(), p, c)
			if (err == nil) != tc.pass {
				t.Fatalf("want pass=%v got %v", tc.pass, err)
			}
			if tc.pass && receipt.Scope == AnalyticalGrainScope {
				t.Fatal("single-relation scope on join")
			}
		})
	}
}

func TestSQLRecoveryAnalyticalJoinRequiresPhysicalKeys(t *testing.T) {
	p, c := analyticalJoinFixture(t, `SELECT c.region,sum(s.amount) FROM analytics.sales s LEFT JOIN analytics.customers c ON s.id=c.id GROUP BY c.region`)
	p.candidate.binding.Relations[1].UniqueKeys = nil
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalUnsupported) {
		t.Fatal("unproven key", err)
	}
	p.candidate.binding.Relations[1].UniqueKeys = [][]string{{"id", "region"}}
	c.Binding = Hash(p.candidate.binding)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); !errors.Is(err, ErrAnalyticalUnsupported) {
		t.Fatal("partial composite key", err)
	}
}

func TestUniqueKeyBindingBoundsAndDetach(t *testing.T) {
	b := parserBinding()
	b.Relations[0].UniqueKeys = [][]string{{"id"}}
	if !b.Valid() {
		t.Fatal("valid key rejected")
	}
	copy := b.Clone()
	copy.Relations[0].UniqueKeys[0][0] = "amount"
	if b.Relations[0].UniqueKeys[0][0] != "id" {
		t.Fatal("shared key")
	}
	for _, keys := range [][][]string{{{}}, {{"absent"}}, {{"id", "id"}}, {{"name", "id"}}, {{"id"}, {"id"}}} {
		copy = b.Clone()
		copy.Relations[0].UniqueKeys = keys
		if copy.Valid() {
			t.Fatal("invalid keys accepted", keys)
		}
	}
	r := Relation{UniqueKeys: [][]string{{"a", "b"}}}
	if r.HasUniqueKey([]string{"a"}) || !r.HasUniqueKey([]string{"b", "a"}) || r.HasUniqueKey([]string{"a", "a", "b"}) {
		t.Fatal("composite matching")
	}
}
