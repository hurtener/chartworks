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

func adversarialChatCount(f *cw01Fixture) int {
	f.model.mu.Lock()
	defer f.model.mu.Unlock()
	n := 0
	for _, model := range f.model.models {
		if model == f.model.cfg.Roles["sqlgen"].Model || model == f.model.cfg.Roles["sqlfix"].Model {
			n++
		}
	}
	return n
}
func adversarialExactResult(t *testing.T, r nlqexec.RunResult, want string) {
	t.Helper()
	if r.Execution.Result == nil || len(r.Execution.Result.Rows) != 1 || len(r.Execution.Result.Rows[0]) != 1 {
		t.Fatal("wrong exact-result shape")
	}
	var text string
	if json.Unmarshal(r.Execution.Result.Rows[0][0], &text) != nil {
		t.Fatal("numeric result not lossless text")
	}
	value, ok := new(big.Rat).SetString(text)
	expected, _ := new(big.Rat).SetString(want)
	if !ok || value.Cmp(expected) != 0 {
		t.Fatalf("actual PostgreSQL result %s, expected %s", text, want)
	}
}

// ONLY changes the population, but the current native validator already rejects
// that shape. Preserve that rejection and demonstrate the real result difference.
// The analytical-only regression is defense in depth, not a reachable native bypass.
func TestSQLRecoveryAdversarialOnlyPopulationAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET amount=CASE id WHEN 1 THEN 10 ELSE 20 END;
 CREATE TABLE analytics.sales_child () INHERITS (analytics.sales);
 INSERT INTO analytics.sales_child(id,amount) VALUES(3,100)`); err != nil {
		t.Fatal("inheritance source fixture", err)
	}
	bad, good := `SELECT sum(amount) AS revenue FROM ONLY analytics.sales`, `SELECT sum(amount) AS revenue FROM analytics.sales`
	var only, total string
	if err := f.f.admin.QueryRow(ctx, `SELECT (SELECT sum(amount)::text FROM ONLY analytics.sales),(SELECT sum(amount)::text FROM analytics.sales)`).Scan(&only, &total); err != nil || only != "30.000" || total != "130.000" {
		t.Fatal("independent ONLY counterexample", only, total, err)
	}
	if _, err := f.f.validator.Validate(ctx, f.e, readexec.Request{Source: f.pack.Datasets[0].Source.Source, Context: f.context, SQL: bad}); !errors.Is(err, readexec.ErrUnsupported) {
		t.Fatal("native ONLY rejection must remain intact", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	calls := adversarialChatCount(f)
	f.model.mode.Store(phase18RawResponse(t, bad))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
	if p.QueryID != "" || !errors.Is(err, readexec.ErrUnsupported) || adversarialChatCount(f)-calls != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
		t.Fatal("ONLY got an executable analytical plan", err)
	}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
	f.model.mu.Unlock()
	p, err = f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Revenue", nlq.LanguageEnglish)})
	if err != nil || p.Analytical == nil || p.ValidationFixes != 1 || p.SQL != good {
		t.Fatal("bounded ONLY repair", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil {
		t.Fatal("normal descendant-inclusive execution", err)
	}
	adversarialExactResult(t, out, "130")
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if !strings.Contains(wire, "validation_unsupported") || !strings.Contains(wire, "rejected_sql") {
		t.Fatal("repair missing closed diagnostic/unbound candidate")
	}
	calls = adversarialChatCount(f)
	before = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil || adversarialChatCount(f) != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
		t.Fatal("terminal replay introduced execution", err)
	}
	adversarialExactResult(t, replay, "130")
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	saved, err := f.f.db.ReadSavedQuery(ctx, f.e, p.QueryID, false)
	if err != nil || saved.Result != nil || saved.SQL != good || readexec.Hash(saved.Analytical) != readexec.Hash(p.Analytical) {
		t.Fatal("saved proof/result projection", err)
	}
	original, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-fix retained selected-metric receipt, without altering the
	// immutable original row. Its hashes alone must not bypass fresh checking.
	original.ID = readexec.Hash([]string{p.QueryID, "retained-only"})[:32]
	original.Operation = ""
	original.Status = "planned"
	original.Result = nil
	original.SQL = bad
	original.Analytical.Query = readexec.AnalyticalQueryDigest(bad, original.Parameters)
	if err = f.f.db.CreateQuery(ctx, sc, original); err != nil {
		t.Fatal("retained proof fixture", err)
	}
	_, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: original.ID, Operation: original.ID + "-run"})
	if !errors.Is(err, readexec.ErrUnsupported) || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
		t.Fatal("retained hash bypassed fresh native validation", err)
	}
}

func TestSQLRecoveryAdversarialIntegerDivisionAcceptance(t *testing.T) {
	f := newCW01FixtureWithPack(t, func(pack *semantics.TopicPack) {
		ref := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}
		pack.Measures = append(pack.Measures, semantics.Measure{ID: "order_count", Name: "Order count", Field: ref, Aggregation: semantics.AggregationCount})
		pack.KPIs = append(pack.KPIs, semantics.KPI{ID: "half_orders", Name: "Half orders", Expression: "order_count / 2", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: "order_count"}}})
	})
	ctx := context.Background()
	if _, err := f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount) VALUES(3,1)`); err != nil {
		t.Fatal(err)
	}
	var quoted, bigintDivision, widened string
	if err := f.f.admin.QueryRow(ctx, `SELECT (count(id)/'2')::text,(count(id)/2147483648)::text,(count(id)::numeric/2)::text FROM analytics.sales`).Scan(&quoted, &bigintDivision, &widened); err != nil || quoted != "1" || bigintDivision != "0" {
		t.Fatal("independent integer-division counterexample", quoted, bigintDivision, err)
	}
	value, ok := new(big.Rat).SetString(widened)
	if !ok || value.Cmp(big.NewRat(3, 2)) != 0 {
		t.Fatal("numeric positive control", widened)
	}
	question := f.question("Compute the reviewed result", nlq.LanguageEnglish)
	question.MetricIDs = []string{"half_orders"}
	bad, good := `SELECT count(id)/'2' AS result FROM analytics.sales`, `SELECT count(id)::numeric/2 AS result FROM analytics.sales`
	if _, err := f.f.validator.Validate(ctx, f.e, readexec.Request{Source: f.pack.Datasets[0].Source.Source, Context: f.context, SQL: bad}); err != nil {
		t.Fatal("unknown literal fixture must be native-safe", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	for _, sql := range []string{bad, `SELECT (count(id)/'2')::numeric AS result FROM analytics.sales`, `SELECT count(id)/2 AS result FROM analytics.sales`} {
		f.model.mode.Store(phase18RawResponse(t, sql))
		calls := adversarialChatCount(f)
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
		if p.QueryID != "" || err == nil || !errors.Is(err, readexec.ErrAnalyticalMismatch) && !errors.Is(err, readexec.ErrAnalyticalUnsupported) || adversarialChatCount(f)-calls != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
			t.Fatal("truncated integer division planned/executed", err)
		}
	}
	f.model.mu.Lock()
	f.model.chatSequence = []string{phase18RawResponse(t, bad), phase18RawResponse(t, good)}
	f.model.mu.Unlock()
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil || p.Analytical == nil || p.ValidationFixes != 1 {
		t.Fatal("numeric correction failed", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	adversarialExactResult(t, out, "1.5")
	calls := adversarialChatCount(f)
	attempts = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	out, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil || calls != adversarialChatCount(f) || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
		t.Fatal("ratio replay changed work or result", err)
	}
	adversarialExactResult(t, out, "1.5")
}

func TestSQLRecoveryAdversarialRepairFinalizationAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	statement := `SELECT id,amount/(id-id) AS amount FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, statement))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil || p.QueryID == "" || p.Analytical != nil {
		t.Fatal("parameter-free native-only fixture", err)
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	// New row emulates old generation metadata that no longer fits the current
	// context-owner contract; accepted SQL/source scope remain unchanged.
	q.ID = readexec.Hash([]string{p.QueryID, "old-context"})[:32]
	q.Operation = ""
	q.AnalyticalVersion = 0
	q.Analytical = nil
	q.Generation.Context.Tier = nlq.Tier("old-generation-tier")
	if err = f.f.db.CreateQuery(ctx, sc, q); err != nil {
		t.Fatal("legacy context fixture", err)
	}
	calls := adversarialChatCount(f)
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: q.ID, Operation: q.ID + "-run"})
	if err == nil || !errors.Is(err, nlqexec.ErrExecutionBudget) || out.Status != "failed" || out.ExecutionFixes != 0 || adversarialChatCount(f) != calls {
		t.Fatal("repair setup failure did not finalize query", err)
	}
	stored, err := f.f.db.ReadQuery(ctx, sc, q.ID)
	if err != nil || stored.Status != "failed" || stored.SQL != statement {
		t.Fatal("terminal record missing", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	replayCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	replay, err := f.query.Run(replayCtx, f.e, nlqexec.RunRequest{QueryID: q.ID, Operation: q.ID + "-run"})
	if err == nil || errors.Is(err, context.DeadlineExceeded) || replay.Status != "failed" || adversarialChatCount(f) != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
		t.Fatal("replay waited or re-executed after terminal preparation failure", err)
	}
}

func TestSQLRecoveryAdversarialDurableQueryFailureReachesCorrection(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	// The SQL is native-safe/EXPLAIN-safe but fails on real row values. A
	// semantics-preserving correction cannot invent a different denominator.
	// It gets one attempt, then persists the same explicit failure for replay.
	statement := `SELECT id,amount/(id-id) AS amount FROM analytics.sales`
	f.model.mode.Store(phase18RawResponse(t, statement))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("Show reviewed records", nlq.LanguageEnglish)})
	if err != nil || p.QueryID == "" || p.Analytical != nil {
		t.Fatal("native-safe runtime failure fixture", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	calls := adversarialChatCount(f)
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err == nil || !errors.Is(err, nlqexec.ErrExecutionBudget) || out.Status != "failed" || out.ExecutionFixes != 1 || adversarialChatCount(f) != calls+1 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before+2 {
		t.Fatal("durable query failure did not receive one bounded correction", err)
	}
	if out.Execution.Attempt.Code != "query_error" || out.Execution.Attempt.RemoteState != "stopped" || out.Execution.Attempt.Finished == nil || out.Execution.Result != nil {
		t.Fatal("unconfirmed physical failure treated as terminal correction")
	}
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	q, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
	if err != nil || q.Status != "failed" || q.ExecutionFixes != 1 || q.SQL != statement {
		t.Fatal("accepted SQL/failure not retained", err)
	}
	out, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err == nil || out.Status != "failed" || adversarialChatCount(f) != calls+1 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before+2 {
		t.Fatal("terminal retry attempted correction/execution again", err)
	}
}
