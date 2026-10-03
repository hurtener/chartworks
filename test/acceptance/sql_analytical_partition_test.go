package acceptance

import (
	"context"
	"encoding/json"
	"math/big"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
)

// Repeated category values exercise aggregation within a partition, rather than
// grouping only by unique row IDs. Keep the NULL category distinct and preserve
// the exact decimal sum above float64's integral precision.
func TestSQLRecoveryAnalyticalGrainPartitions(t *testing.T) {
	f := newCW01FixtureWithPack(t, func(pack *semantics.TopicPack) {
		pack.Dimensions = []semantics.Dimension{{ID: "state", Name: "State", Description: "Reviewed synthetic categorical grouping", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "active"}, Role: semantics.DimensionCategorical}}
	})
	ctx := context.Background()
	if _, err := f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,active) VALUES (3,2.875,true),(4,NULL,false),(5,7.25,NULL)`); err != nil {
		t.Fatal("partition fixture", err)
	}
	f.model.mode.Store(phase18RawResponse(t, `SELECT active,sum(amount) AS revenue FROM analytics.sales GROUP BY active ORDER BY active NULLS LAST`))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue by State", nlq.LanguageEnglish)})
	if err != nil || p.Analytical == nil || p.Analytical.Scope != readexec.AnalyticalGrainScope || len(p.Analytical.Grouping) != 1 || p.Analytical.Grouping[0] != f.pack.Topic+":dimension:state" {
		t.Fatal("categorical grouping proof", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 3 || readexec.Hash(out.Analytical) != readexec.Hash(p.Analytical) {
		t.Fatal("categorical grouping result", err)
	}
	want := map[string]string{"false": "5.5", "true": "9007199254740996", "null": "7.25"}
	seen := map[string]bool{}
	for _, row := range out.Execution.Result.Rows {
		if len(row) != 2 {
			t.Fatal("grouping result arity")
		}
		key := string(row[0])
		expected, ok := new(big.Rat).SetString(want[key])
		if !ok || seen[key] {
			t.Fatal("missing, repeated or substituted grouping key")
		}
		seen[key] = true
		var scalar string
		if json.Unmarshal(row[1], &scalar) != nil {
			t.Fatal("aggregate lost exact decimal encoding")
		}
		actual, ok := new(big.Rat).SetString(scalar)
		if !ok || actual.Cmp(expected) != 0 {
			t.Fatal("group aggregate differs from exact expected population")
		}
	}
}
