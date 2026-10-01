package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func groupedDomainFixture(t *testing.T, domain string, where bool) (Plan, AnalyticalContract) {
	sql := strings.Replace(groupedSQL, "sum(s.amount)", "sum(s.amount) FILTER (WHERE s.id=1)", 1)
	if where {
		sql = strings.Replace(groupedSQL, "GROUP BY d.region), i AS", "WHERE s.id=1 GROUP BY d.region), i AS", 1)
	}
	p, c := groupedProgramFixture(t, sql)
	c.Metrics[0].Expression.Args[0].Filters = []AnalyticalFilter{{Column: "id", Kind: "eq", Values: []string{"1"}}}
	for i := range c.GroupedPopulations.Lanes {
		if c.GroupedPopulations.Lanes[i].Dataset == "sales" {
			c.GroupedPopulations.Lanes[i].Domain = domain
		}
	}
	return p, c
}
func TestSQLRecoveryGroupedReviewedDomains(t *testing.T) {
	for _, domain := range []string{AnalyticalGroupDomainRaw, AnalyticalGroupDomainQualifying} {
		for _, where := range []bool{false, true} {
			p, c := groupedDomainFixture(t, domain, where)
			_, err := CheckAnalyticalPlan(context.Background(), p, c)
			want := where == (domain == AnalyticalGroupDomainQualifying)
			if (err == nil) != want {
				t.Fatal("group domain placement", domain, where, err)
			}
		}
	}
	for _, version := range []string{AnalyticalGroupedPopulationsVersion, AnalyticalGroupedProgramsVersion} {
		p, c := groupedDomainFixture(t, "", false)
		c.Version = version
		_, err := CheckAnalyticalPlan(context.Background(), p, c)
		var detail *AnalyticalError
		if !errors.As(err, &detail) || detail.Code != AnalyticalGroupDomainReviewCode {
			t.Fatal("missing explicit domain review disposition", version, err)
		}
	}
	// Retained v7's full common WHERE is unambiguous qualifying population.
	p, c := groupedDomainFixture(t, "", true)
	c.Version = AnalyticalGroupedPopulationsVersion
	before := Hash(c)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil || Hash(c) != before {
		t.Fatal("safe v7 replay changed", err)
	}
}
func TestSQLRecoveryGroupedQualifyingPopulationUnion(t *testing.T) {
	p, c := groupedDomainFixture(t, AnalyticalGroupDomainQualifying, false)
	one := c.Metrics[0].Expression.Args[0]
	two := one
	two.Filters = []AnalyticalFilter{{Column: "id", Kind: "eq", Values: []string{"2"}}}
	c.Metrics[0].Expression.Args[0] = AnalyticalExpression{Op: "+", Args: []AnalyticalExpression{one, two}}
	sql := strings.Replace(groupedSQL, "sum(s.amount) AS value", "sum(s.amount) FILTER (WHERE s.id=1) AS one,sum(s.amount) FILTER (WHERE s.id=2) AS two", 1)
	sql = strings.Replace(sql, "GROUP BY d.region), i AS", "WHERE (s.id=2 OR s.id=1) GROUP BY d.region), i AS", 1)
	sql = strings.Replace(sql, "s.value-i.value", "s.one+s.two-i.value", 1)
	p.candidate.statement = sql
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal("exact qualifying union rejected", err)
	}
	for _, replacement := range []string{"s.id=1", "s.id=1 AND s.id=2", "s.id=1 OR s.id=2 OR s.id=3"} {
		p.candidate.statement = strings.Replace(sql, "s.id=2 OR s.id=1", replacement, 1)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("changed group domain accepted", replacement)
		}
	}
}
func TestSQLRecoveryGroupDNFPositiveBooleanIdentity(t *testing.T) {
	a := analyticalChecker{ctx: context.Background(), relation: Relation{ID: "sales", Columns: []Column{{Name: "id", Category: "numeric", NativeType: "int4", Safe: true}, {Name: "amount", Category: "numeric", NativeType: "numeric", Nullable: true, Safe: true}}}, alias: "s"}
	var first string
	for _, where := range []string{"(s.id=1 OR s.id=2) AND s.amount IS NOT NULL", "(s.amount IS NOT NULL AND s.id=2) OR (s.id=1 AND s.amount IS NOT NULL)"} {
		q, err := analyticalSQLTree(context.Background(), "SELECT s.id FROM analytics.sales s WHERE "+where, Binding{Dialect: "postgres"})
		if err != nil {
			t.Fatal(err)
		}
		dnf, err := a.groupedWhereDNF(q["whereClause"], 0)
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = Hash(dnf)
		} else if first != Hash(dnf) {
			t.Fatal("positive DNF lost exact union")
		}
	}
}
