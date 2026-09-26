package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"math/big"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestSQLRecoveryParameterScopesLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			question := "List sales records"
			if locale == nlq.LanguageSpanish {
				question = "Listar registros de ventas"
			}
			cases := []struct{ name, base, child, kind, value string }{
				{"cte", `WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id,amount FROM filtered ORDER BY id`, `WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id FROM filtered ORDER BY id DESC LIMIT 1`, "number", "5.25"},
				{"derived", `SELECT s.id,s.amount FROM (SELECT id,amount FROM analytics.sales WHERE amount>$1) s ORDER BY s.id`, `SELECT s.id FROM (SELECT id,amount FROM analytics.sales WHERE amount>$1) s ORDER BY s.id DESC LIMIT 1`, "number", "5.25"},
				{"join", `SELECT s.id,s.amount FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id AND t.amount>$1 ORDER BY s.id`, `SELECT s.id FROM analytics.sales s JOIN analytics.sales t ON s.id=t.id AND t.amount>$1 ORDER BY s.id DESC LIMIT 1`, "number", "5.25"},
				{"sublink", `SELECT id,amount FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales WHERE amount>$1) ORDER BY id`, `SELECT id FROM analytics.sales WHERE id IN (SELECT id FROM analytics.sales WHERE amount>$1) ORDER BY id DESC LIMIT 1`, "number", "5.25"},
				{"scalar", `SELECT id,(SELECT max(amount) FROM analytics.sales WHERE amount>$1) AS largest FROM analytics.sales ORDER BY id`, `SELECT id,(SELECT max(amount) FROM analytics.sales WHERE amount>$1) AS largest FROM analytics.sales ORDER BY id DESC LIMIT 1`, "number", "5.25"},
				{"inline_window", `SELECT id,sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) AS rolling FROM analytics.sales ORDER BY id`, `SELECT id,sum(amount) OVER (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) AS rolling FROM analytics.sales ORDER BY id DESC LIMIT 1`, "integer", "1"},
				{"named_window", `SELECT id,sum(amount) OVER w AS rolling FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) ORDER BY id DESC`, `SELECT id,sum(amount) OVER w AS rolling FROM analytics.sales WINDOW w AS (ORDER BY id ROWS BETWEEN $1 PRECEDING AND CURRENT ROW) ORDER BY id DESC LIMIT 1`, "integer", "1"},
				{"set_operation", `SELECT id,amount FROM analytics.sales WHERE amount>$1 UNION ALL SELECT id,amount FROM analytics.sales WHERE amount<=$1 ORDER BY 1`, `SELECT id,amount FROM analytics.sales WHERE amount>$1 UNION ALL SELECT id,amount FROM analytics.sales WHERE amount<=$1 ORDER BY 1 DESC LIMIT 1`, "number", "5.25"},
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					original := []readexec.Parameter{{Kind: tc.kind, Value: tc.value}}
					f.model.mode.Store(parameterResponse(t, tc.base, original))
					p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
					if err != nil || p.QueryID == "" {
						t.Fatal("native parameterized parent", err)
					}
					sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
					parent, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
					if err != nil {
						t.Fatal(err)
					}
					f.query, _ = newPhase18Service(t, f.phase17Fixture)
					f.model.mu.Lock()
					start := len(f.model.requestBodies)
					f.model.mu.Unlock()
					f.model.mode.Store(parameterResponse(t, tc.child, []readexec.Parameter{{Kind: tc.kind, Value: "999999"}}))
					child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{EditBase: []nlq.Instruction{{Key: "view", Text: "Keep the protected source and parameter clauses; return only the final record by descending id."}}}})
					if err != nil || child.QueryID == p.QueryID {
						t.Fatal("supported outer edit", err)
					}
					saved, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
					if err != nil || !reflect.DeepEqual(saved.Parameters, original) || saved.ParentDigest != nlqexec.QueryLineageDigest(parent) || saved.ParentRevision != parent.Revision {
						t.Fatal("private values/lineage changed", err)
					}
					out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
					if err != nil {
						t.Fatal(err)
					}
					requireParameterIDs(t, out, "2")
					if strings.Contains(tc.name, "window") {
						requireWindowValue(t, out, "9007199254740998.625")
					}
					metadata := support.Raw(t, f.f.dsn)
					calls := f.model.requests.Load()
					attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
					replay, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
					if err != nil || calls != f.model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
						t.Fatal("terminal scope replay did work", err)
					}
					requireParameterIDs(t, replay, "2")
					projected, err := f.f.db.ReadSavedQuery(ctx, f.e, child.QueryID, false)
					if err != nil || projected.Result != nil || !reflect.DeepEqual(projected.Parameters, original) {
						t.Fatal("saved parameter projection", err)
					}
					unchanged, err := f.f.db.ReadQuery(ctx, sc, p.QueryID)
					if err != nil || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(parent) {
						t.Fatal("parent mutated", err)
					}
					f.model.mu.Lock()
					wire := strings.Join(f.model.requestBodies[start:], "\n")
					f.model.mu.Unlock()
					if tc.kind == "number" && strings.Contains(wire, tc.value) {
						t.Fatal("private numeric binding leaked into provider input")
					}
					if !strings.Contains(wire, "retained_parameter_slots") {
						t.Fatal("missing private-slot editing contract")
					}
					if strings.Contains(tc.name, "window") {
						f.model.mode.Store(parameterResponse(t, tc.child, []readexec.Parameter{{Kind: "integer", Value: "999999"}}))
						changed, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{{Position: 1, Replacement: readexec.Parameter{Kind: "integer", Value: "0"}}}})
						if err != nil {
							t.Fatal("typed window-frame replacement", err)
						}
						result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: changed.QueryID, Operation: changed.QueryID + "-run"})
						if err != nil {
							t.Fatal(err)
						}
						requireParameterIDs(t, result, "2")
						requireWindowValue(t, result, "5.5")
					}
				})
			}
		})
	}
}

func requireWindowValue(t *testing.T, out nlqexec.RunResult, expected string) {
	t.Helper()
	if out.Execution.Result == nil || len(out.Execution.Result.Rows) != 1 || len(out.Execution.Result.Rows[0]) != 2 {
		t.Fatal("window row shape")
	}
	var value string
	if json.Unmarshal(out.Execution.Result.Rows[0][1], &value) != nil {
		t.Fatal("lost decimal string")
	}
	got, ok := new(big.Rat).SetString(value)
	want, _ := new(big.Rat).SetString(expected)
	if !ok || got.Cmp(want) != 0 {
		t.Fatal("window binding changed exact running aggregate", value)
	}
}

func TestSQLRecoveryParameterScopesRejectionAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	question := f.question("List sales records", nlq.LanguageEnglish)
	base := `WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT id FROM filtered ORDER BY id`
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "number", Value: "5.25"}}))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	metadata := support.Raw(t, f.f.dsn)
	attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	for _, bad := range []string{
		`WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE id>$1) SELECT id FROM filtered ORDER BY id`,
		`WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1 OR true) SELECT id FROM filtered ORDER BY id`,
		`WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT $1 FROM filtered`,
		`WITH filtered AS (SELECT id,amount FROM analytics.sales WHERE amount>$1) SELECT pg_read_file('/etc/passwd') FROM filtered`,
	} {
		f.model.mode.Store(parameterResponse(t, bad, []readexec.Parameter{{Kind: "number", Value: "1"}}))
		child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID})
		if err == nil || child.QueryID != "" || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) {
			t.Fatal("unsafe/changed scope produced executable child", err)
		}
	}
	f.model.mode.Store(parameterResponse(t, base+" LIMIT 1", []readexec.Parameter{{Kind: "number", Value: "1"}}))
	other := phase18Envelope(t, f.phase17Fixture, f.e.User(), "different-scope-session", true)
	calls := f.model.requests.Load()
	if _, err := f.query.Refine(ctx, other, nlqexec.RefineRequest{QueryID: p.QueryID}); err == nil || f.model.requests.Load() != calls {
		t.Fatal("scope extension bypassed session admission")
	}
	// A projection-only native correction must still retain the complete CTE and
	// its private slots, even though neither new output contains a parameter.
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.chatSequence = []string{parameterResponse(t, strings.Replace(base, "SELECT id FROM filtered", "SELECT missing FROM filtered", 1), []readexec.Parameter{{Kind: "number", Value: "0"}}), parameterResponse(t, base+" LIMIT 1", []readexec.Parameter{{Kind: "number", Value: "0"}})}
	f.model.mu.Unlock()
	fixed, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID})
	if err != nil || fixed.ValidationFixes != 1 {
		t.Fatal("nested parameter custody through correction", err)
	}
	out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: fixed.QueryID, Operation: fixed.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "1")
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	if strings.Contains(wire, "5.25") {
		t.Fatal("retained nested value leaked into repair input")
	}
	// A projected slot can be referenced by column alias without another
	// ParamRef. Those outer consumers are immutable, but an explicit same-kind
	// value replacement still uses the unchanged, fully validated statement.
	projected := `WITH valueset AS (SELECT id,amount,$1::numeric AS cutoff FROM analytics.sales) SELECT id FROM valueset WHERE amount>cutoff ORDER BY id`
	f.model.mode.Store(parameterResponse(t, projected, []readexec.Parameter{{Kind: "number", Value: "5.25"}}))
	aliasParent, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal("projected private slot parent", err)
	}
	before := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	f.model.mode.Store(parameterResponse(t, strings.Replace(projected, "amount>cutoff", "amount<cutoff", 1), []readexec.Parameter{{Kind: "number", Value: "0"}}))
	if invalid, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: aliasParent.QueryID}); err == nil || invalid.QueryID != "" || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != before {
		t.Fatal("projected binding was repurposed through a column alias", err)
	}
	f.model.mode.Store(parameterResponse(t, projected, []readexec.Parameter{{Kind: "number", Value: "999999"}}))
	replaced, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: aliasParent.QueryID, ParameterEdits: []nlqexec.ParameterEdit{{Position: 1, Replacement: readexec.Parameter{Kind: "number", Value: "10"}}}})
	if err != nil {
		t.Fatal("unchanged projected-slot replacement", err)
	}
	out, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: replaced.QueryID, Operation: replaced.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "1")
	// The public custody function is not executable SQL authority.
	if !errors.Is(readexec.CheckParameterContinuity(ctx, "mysql", base, base+" LIMIT 1", 1), readexec.ErrUnsupported) {
		t.Fatal("non-PostgreSQL proof guessed")
	}
}
