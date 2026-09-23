package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestSQLRecoveryAnalyticalCalendarAcceptance(t *testing.T) {
	f := newCW01FixtureWithPack(t, func(pack *semantics.TopicPack) {
		pack.Dimensions = []semantics.Dimension{{ID: "order_date", Name: "Order date", Description: "Synthetic reviewed instant calendar", Aliases: []string{"fecha de pedido"}, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "created_at"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{"day", "month", "quarter", "year"}}}}
	})
	ctx := context.Background()
	sql(t, f.f.admin, `UPDATE analytics.sales SET created_at=CASE id WHEN 1 THEN '2025-01-15T12:00:00Z'::timestamptz ELSE '2026-01-15T12:00:00Z'::timestamptz END;
 INSERT INTO analytics.sales(id,amount,created_at,active,name) VALUES
 (3,2.875,'2025-02-01T04:00:00Z',true,'boundary'),
 (4,7.25,NULL,true,'unknown'),
 (5,10,'2025-03-09T06:59:59Z',true,'before-transition'),
 (6,20,'2025-03-09T07:00:00Z',true,'after-transition')`)
	metadata := support.Raw(t, f.f.dsn)
	good := `SELECT date_trunc('month',created_at,'America/New_York') AS period,sum(amount) AS revenue FROM analytics.sales GROUP BY 1 ORDER BY 1 NULLS LAST`
	plan := func(t *testing.T, text, statement string) nlqexec.PlanResult {
		t.Helper()
		f.model.mode.Store(phase18RawResponse(t, statement))
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(text, nlq.LanguageEnglish)})
		if err != nil || p.Analytical == nil || p.Analytical.Version != readexec.AnalyticalCalendarVersion || p.Analytical.Scope != readexec.AnalyticalCalendarScope || len(p.Analytical.Grouping) != 1 {
			t.Fatal("calendar plan", err)
		}
		return p
	}
	values := func(t *testing.T, result *readexec.Result) map[string]string {
		t.Helper()
		out := map[string]string{}
		for _, row := range result.Rows {
			if len(row) != 2 {
				t.Fatal("calendar output arity")
			}
			key := "null"
			if string(row[0]) != "null" {
				var s string
				if json.Unmarshal(row[0], &s) != nil {
					t.Fatal("non-string temporal output")
				}
				instant, err := time.Parse(time.RFC3339Nano, s)
				if err != nil {
					t.Fatal("temporal encoding", err)
				}
				key = instant.UTC().Format(time.RFC3339)
			}
			var value string
			if json.Unmarshal(row[1], &value) != nil {
				t.Fatal("lossy amount")
			}
			if _, duplicate := out[key]; duplicate {
				t.Fatal("duplicate bucket")
			}
			out[key] = value
		}
		return out
	}
	t.Run("year zone null partitions and immutable replay", func(t *testing.T) {
		p := plan(t, "Revenue by month of Order date", good)
		r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || r.Execution.Result == nil || len(r.Execution.Result.Rows) != 4 {
			t.Fatal("calendar execution", err)
		}
		got := values(t, r.Execution.Result)
		want := map[string]string{"2025-01-01T05:00:00Z": "9007199254740996", "2025-03-01T05:00:00Z": "30", "2026-01-01T05:00:00Z": "5.5", "null": "7.25"}
		for key, v := range want {
			actual, ok := new(big.Rat).SetString(got[key])
			expected, _ := new(big.Rat).SetString(v)
			if !ok || actual.Cmp(expected) != 0 {
				t.Fatalf("wrong exact partition %s: %s", key, got[key])
			}
		}
		sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
		saved, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
		if err != nil || saved.AnalyticalVersion != 3 || readexec.Hash(saved.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("calendar persistence", err)
		}
		projected, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
		if err != nil || projected.Result != nil || readexec.Hash(projected.Analytical) != readexec.Hash(p.Analytical) {
			t.Fatal("saved calendar proof", err)
		}
		for _, mutation := range []string{`analytical_version=2`, `analytical=NULL`, `analytical=analytical-'grouping'`, `analytical=jsonb_set(analytical,'{contract}','"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"')`} {
			if _, err := metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET `+mutation+` WHERE query_id=$1`, p.QueryID); err == nil {
				t.Fatal("calendar proof mutation allowed")
			}
		}
		calls, attempts := f.model.requests.Load(), count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || readexec.Hash(replay.Execution.Result.Rows) != readexec.Hash(r.Execution.Result.Rows) || calls != f.model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("calendar replay recomputed", err)
		}
	})
	t.Run("Spanish reviewed calendar", func(t *testing.T) {
		f.model.mode.Store(phase18RawResponse(t, good))
		q := f.question("Revenue por mes de fecha de pedido", nlq.LanguageSpanish)
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
		if err != nil || p.Analytical == nil || p.Analytical.Scope != readexec.AnalyticalCalendarScope {
			t.Fatal("Spanish calendar", err)
		}
	})
	t.Run("wrong zone corrected once wrong year never executes", func(t *testing.T) {
		f.model.mu.Lock()
		start := len(f.model.requestBodies)
		f.model.chatSequence = []string{phase18RawResponse(t, strings.ReplaceAll(good, "America/New_York", "UTC")), phase18RawResponse(t, good)}
		f.model.mu.Unlock()
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue by month of Order date", nlq.LanguageEnglish)})
		if err != nil || p.ValidationFixes != 1 || p.Analytical == nil || p.Analytical.Scope != readexec.AnalyticalCalendarScope {
			t.Fatal("calendar correction", err)
		}
		f.model.mu.Lock()
		wire := strings.Join(f.model.requestBodies[start:], "\n")
		f.model.mu.Unlock()
		if !strings.Contains(wire, "analytical_grain_mismatch") || !strings.Contains(wire, "rejected_sql") {
			t.Fatal("untargeted calendar correction")
		}
		f.model.mode.Store(phase18RawResponse(t, strings.ReplaceAll(good, "'month'", "'year'")))
		attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
		bad, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue by month of Order date", nlq.LanguageEnglish)})
		if bad.QueryID != "" || !errors.Is(err, readexec.ErrAnalyticalMismatch) || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("wrong year executed", err)
		}
	})
	t.Run("DST transition retains reviewed local day", func(t *testing.T) {
		p := plan(t, "Revenue by day of Order date", strings.ReplaceAll(good, "'month'", "'day'"))
		r, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
		if err != nil || r.Execution.Result == nil {
			t.Fatal("day result", err)
		}
		got := values(t, r.Execution.Result)
		value, ok := new(big.Rat).SetString(got["2025-03-09T05:00:00Z"])
		if !ok || value.Cmp(big.NewRat(30, 1)) != 0 {
			t.Fatal("one local DST day split", got)
		}
	})
}
