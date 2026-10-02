package exec

import (
	"context"
	"strings"
	"testing"
)

func groupedProgramFixture(t *testing.T, sql string) (Plan, AnalyticalContract) {
	p, c := groupedFixture(t, sql)
	c.Version = AnalyticalGroupedProgramsVersion
	return p, c
}
func groupedDerivedSpineSQL() string {
	sql := strings.Replace(groupedSQL, ", keys AS (SELECT region FROM s UNION SELECT region FROM i)", "", 1)
	return strings.Replace(sql, "FROM keys LEFT JOIN", "FROM (SELECT region FROM s UNION SELECT region FROM i) keys LEFT JOIN", 1)
}
func groupedWrappedLaneSQL() string {
	sql := strings.Replace(groupedSQL, "s AS (SELECT d.region", "s AS (SELECT w.region,w.value FROM (SELECT d.region", 1)
	return strings.Replace(sql, "GROUP BY d.region), i AS", "GROUP BY d.region) w), i AS", 1)
}
func TestSQLRecoveryGroupedDerivedProjections(t *testing.T) {
	for _, sql := range []string{groupedWrappedLaneSQL(), groupedDerivedSpineSQL(), "SELECT q.region,q.net FROM (" + groupedSQL + ") q", "SELECT q.net AS result,q.region AS area FROM (" + groupedWrappedLaneSQL() + ") q"} {
		p, c := groupedProgramFixture(t, sql)
		before := Hash(c)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
			t.Fatal(sql, err)
		}
		if Hash(c) != before {
			t.Fatal("wrapper proof mutated contract")
		}
		c.Version = AnalyticalGroupedPopulationsVersion
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("retained v7 acquired derived grammar")
		}
	}
}
func TestSQLRecoveryGroupedProjectionClauses(t *testing.T) {
	outer := "SELECT q.region,q.net FROM (" + groupedSQL + ") q"
	for _, sql := range []string{
		outer + " WHERE q.net>0",
		"SELECT q.region FROM (" + strings.Replace(groupedSQL, " AS net", "", 1) + ") q", strings.Replace(outer, "SELECT q.region", "SELECT DISTINCT q.region", 1), outer + " GROUP BY q.region,q.net",
		strings.Replace(outer, "q.region,q.net", "q.region", 1), strings.Replace(outer, "q.region,q.net", "q.region,q.net+1", 1),
		strings.Replace(outer, ") q", ") q(region,net)", 1), strings.Replace(outer, "FROM (", "FROM LATERAL (", 1),
		strings.Replace(groupedWrappedLaneSQL(), ") w), i", ") w WHERE w.value>0), i", 1),
		strings.Replace(groupedWrappedLaneSQL(), ") w), i", ") w LIMIT 1), i", 1),
		strings.Replace(groupedWrappedLaneSQL(), "SELECT w.region,w.value", "SELECT w.region", 1),
		strings.Replace(groupedDerivedSpineSQL(), "SELECT region FROM s UNION", "SELECT region FROM s WHERE region IS NOT NULL UNION", 1),
	} {
		p, c := groupedProgramFixture(t, sql)
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("wrapper clause silently stripped", sql)
		}
	}
	deep := groupedSQL
	for i := 0; i < 5; i++ {
		deep = "SELECT q.region,q.net FROM (" + deep + ") q"
	}
	p, c := groupedProgramFixture(t, deep)
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
		t.Fatal("unbounded projection depth")
	}
}
func TestSQLRecoveryGroupedFinalLayerIntent(t *testing.T) {
	outer := "SELECT q.region,q.net FROM (" + groupedSQL + ") q ORDER BY q.net DESC NULLS LAST LIMIT 2"
	p, c := groupedProgramFixture(t, outer)
	c.Intent = &AnalyticalIntent{Policy: AnalyticalIntentPolicy, Order: []AnalyticalOrder{{Metric: c.Metrics[0].ID, Descending: true, Nulls: "last"}}, Limit: 2}
	if _, err := CheckAnalyticalPlan(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		"SELECT q.region,q.net FROM (" + groupedSQL + " ORDER BY net DESC NULLS LAST LIMIT 2) q",
		"SELECT q.region,q.net FROM (" + groupedSQL + " LIMIT 2) q ORDER BY q.net DESC NULLS LAST LIMIT 2",
		"SELECT q.region,q.net FROM (" + groupedSQL + " ORDER BY net DESC NULLS LAST) q ORDER BY q.net DESC NULLS LAST LIMIT 2",
		strings.Replace(outer, "DESC NULLS LAST", "ASC NULLS LAST", 1),
	} {
		p.candidate.statement = sql
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("buried or changed final intent accepted", sql)
		}
	}
}

func TestSQLRecoveryGroupedV8MandatoryEvidence(t *testing.T) {
	for _, which := range []string{"intent", "population"} {
		p, c := groupedProgramFixture(t, "SELECT q.region,q.net FROM ("+groupedSQL+") q")
		if which == "intent" {
			c.Intent = nil
		} else {
			c.QueryPopulation = nil
		}
		if _, err := CheckAnalyticalPlan(context.Background(), p, c); err == nil {
			t.Fatal("v8 dropped mandatory evidence", which)
		}
	}
}
