//go:build cgo && (linux || darwin)

package sources

import (
	"fmt"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestSQLRecoveryMySQLOrdinaryGroupDomainLocal(t *testing.T) {
	f := newMySQLAnalyticalFixture(t)
	if _, err := f.admin.ExecContext(t.Context(), "UPDATE "+f.schema+".sales SET category=CASE WHEN id=4 THEN 'A' ELSE category END,created_at=CASE WHEN id<=2 THEN '2026-01-01' WHEN id=3 THEN '2026-02-01' ELSE '2026-03-01' END"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.ExecContext(t.Context(), "INSERT INTO "+f.schema+".sales(id,amount,category) VALUES(5,NULL,'A')"); err != nil {
		t.Fatal(err)
	}
	filter := readexec.AnalyticalFilter{Column: "category", Kind: "eq", Values: []string{"A"}}
	c := readexec.AnalyticalContract{Version: readexec.AnalyticalGroupedProgramsVersion, Binding: readexec.Hash(f.binding), Semantics: readexec.Hash("synthetic-reviewed-group-domain"), Dataset: f.dataset,
		Metrics: []readexec.AnalyticalMetric{{ID: "total", Expression: readexec.AnalyticalExpression{Op: "sum", Column: "amount", Filters: []readexec.AnalyticalFilter{filter}}}, {ID: "count", Expression: readexec.AnalyticalExpression{Op: "count", Column: "amount", Filters: []readexec.AnalyticalFilter{filter}}}},
		Grain:   &readexec.AnalyticalGrain{Policy: readexec.AnalyticalGroupingPolicy, Columns: []string{"created_at"}, Dimensions: []string{"created"}}, Intent: &readexec.AnalyticalIntent{Policy: readexec.AnalyticalIntentPolicy}, QueryPopulation: &readexec.AnalyticalQueryPopulation{Policy: readexec.AnalyticalQueryPopulationPolicy},
	}
	where := "SELECT created_at AS group_time,sum(amount) AS total,count(amount) AS value_count FROM " + f.schema + ".sales WHERE category='A' GROUP BY created_at"
	conditional := "SELECT created_at AS group_time,sum(CASE WHEN category='A' THEN amount END) AS total,count(CASE WHEN category='A' THEN amount END) AS value_count FROM " + f.schema + ".sales GROUP BY created_at"
	for i, domain := range []string{readexec.AnalyticalGroupDomainQualifying, readexec.AnalyticalGroupDomainRaw} {
		c.GroupDomain = &readexec.AnalyticalGroupDomain{Policy: readexec.AnalyticalGroupDomainPolicy, Domain: domain}
		good, bad := where, conditional
		if domain == readexec.AnalyticalGroupDomainRaw {
			good, bad = conditional, where
		}
		for _, statement := range []string{good, bad} {
			plan, err := f.validator.Validate(t.Context(), f.envelope, readexec.Request{Source: f.binding.Source, Context: f.binding.Context, SQL: statement})
			if err != nil {
				t.Fatal("native group domain", statement, err)
			}
			_, err = readexec.CheckAnalyticalPlan(t.Context(), plan, c)
			if statement == bad {
				if err == nil {
					t.Fatal("MySQL WHERE/CASE relocation certified")
				}
				continue
			}
			if err != nil {
				t.Fatal("reviewed MySQL domain", domain, err)
			}
			result, err := f.service.ExecuteRead(t.Context(), f.envelope, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: 10 * time.Second, CancelGrace: time.Second, PlannerCost: 1e6}, fmt.Sprintf("%032x", i+1301), &cloudObserverCapture{})
			if err != nil {
				t.Fatal(err)
			}
			got := map[string][2]string{}
			for _, row := range result.Result.Rows {
				got[string(row[0])] = [2]string{string(row[1]), string(row[2])}
			}
			want := map[string][2]string{`"2026-01-01 00:00:00"`: {`"20.00"`, `"2"`}, `"2026-03-01 00:00:00"`: {"null", `"0"`}, "null": {"null", `"0"`}}
			if domain == readexec.AnalyticalGroupDomainRaw {
				want[`"2026-02-01 00:00:00"`] = [2]string{"null", `"0"`}
			}
			if readexec.Hash(got) != readexec.Hash(want) {
				t.Fatal("MySQL phantom/NULL/duplicate domain result", domain, got)
			}
		}
	}
}
