package acceptance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestSQLRecoveryParameterEditLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			e := phase18Envelope(t, f.phase17Fixture, f.e.User(), f.e.Session(), false)
			question := "List sales records"
			if locale == nlq.LanguageSpanish {
				question = "Listar registros de ventas"
			}
			base := `SELECT id,amount FROM analytics.sales WHERE amount > $1 ORDER BY id`
			original := []readexec.Parameter{{Kind: "number", Value: "5.25"}}
			f.model.mode.Store(parameterResponse(t, base, original))
			parentPlan, err := f.query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: f.question(question, locale)})
			if err != nil || parentPlan.QueryID == "" || parentPlan.SQL != "" {
				t.Fatal("parent plan/inspection boundary", err)
			}
			scope, _ := store.NewScope(e.Tenant(), e.User())
			parent, err := f.f.db.ReadQuery(ctx, scope, parentPlan.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			childSQL := `SELECT id FROM analytics.sales WHERE amount > $1 ORDER BY id DESC`
			f.model.mode.Store(parameterResponse(t, childSQL, []readexec.Parameter{{Kind: "number", Value: "0"}}))
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			replacement := readexec.Parameter{Kind: "number", Value: "9007199254740993.12"}
			in := nlqexec.RefineRequest{QueryID: parentPlan.QueryID, ParameterEdits: []nlqexec.ParameterEdit{{Position: 1, Replacement: replacement}}, QuestionRequest: nlqexec.QuestionRequest{EditBase: []nlq.Instruction{{Key: "projection", Text: "Return IDs only, in descending order. Preserve positional parameter roles."}}}}
			child, err := f.query.Refine(ctx, e, in)
			if err != nil || child.QueryID == "" || child.QueryID == parentPlan.QueryID || child.SQL != "" {
				t.Fatal("typed child plan", err)
			}
			stored, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
			if err != nil || !reflect.DeepEqual(stored.Parameters, []readexec.Parameter{replacement}) || stored.Parent != parent.ID || stored.ParentRevision != parent.Revision || stored.ParentDigest != nlqexec.QueryLineageDigest(parent) {
				t.Fatal("typed replacement/lineage not retained", err)
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			run, err := f.query.Run(ctx, e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil || run.SQL != "" {
				t.Fatal("typed child execution", err)
			}
			requireParameterIDs(t, run, "1") // Strict > compares exact numeric values, not float-rounded cutoffs.
			saved, err := f.f.db.ReadSavedQuery(ctx, e, child.QueryID, false)
			if err != nil || saved.Result != nil || !reflect.DeepEqual(saved.Parameters, []readexec.Parameter{replacement}) {
				t.Fatal("saved projection lost value/privacy", err)
			}
			metadata := support.Raw(t, f.f.dsn)
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			calls := f.model.requests.Load()
			replay, err := f.query.Run(ctx, e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("replay regenerated bindings", err)
			}
			requireParameterIDs(t, replay, "1")
			grandchild, err := f.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: child.QueryID, ParameterEdits: []nlqexec.ParameterEdit{{Position: 1, Replacement: readexec.Parameter{Kind: "number", Value: "5.125"}}}})
			if err != nil {
				t.Fatal("second typed edit", err)
			}
			out, err := f.query.Run(ctx, e, nlqexec.RunRequest{QueryID: grandchild.QueryID, Operation: grandchild.QueryID + "-run"})
			if err != nil {
				t.Fatal(err)
			}
			requireParameterIDs(t, out, "2", "1")
			unchanged, err := f.f.db.ReadQuery(ctx, scope, parent.ID)
			if err != nil || nlqexec.QueryLineageDigest(unchanged) != nlqexec.QueryLineageDigest(parent) {
				t.Fatal("typed edit rewrote ancestor", err)
			}
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			for _, private := range []string{original[0].Value, replacement.Value, "5.125"} {
				if strings.Contains(wire, private) {
					t.Fatal("private typed value entered a provider request")
				}
			}
			if !strings.Contains(wire, "retained_parameter_slots") {
				t.Fatal("lost positional editing guidance")
			}
		})
	}
}

func TestSQLRecoveryParameterEditBoundariesAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	base := `SELECT id FROM analytics.sales WHERE amount > $1 ORDER BY id`
	original := []readexec.Parameter{{Kind: "number", Value: "5.25"}}
	f.model.mode.Store(parameterResponse(t, base, original))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("List sales records", nlq.LanguageEnglish)})
	if err != nil {
		t.Fatal(err)
	}
	edit := nlqexec.ParameterEdit{Position: 1, Replacement: readexec.Parameter{Kind: "number", Value: "5.125"}}
	metadata := support.Raw(t, f.f.dsn)
	t.Run("invalid edit fails before model or execution", func(t *testing.T) {
		for _, edits := range [][]nlqexec.ParameterEdit{{{Position: 2, Replacement: edit.Replacement}}, {edit, edit}, {{Position: 1, Replacement: readexec.Parameter{Kind: "integer", Value: "7"}}}} {
			calls := f.model.requests.Load()
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			out, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: edits})
			if !errors.Is(err, nlqexec.ErrInvalid) || out.QueryID != "" || f.model.requests.Load() != calls || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("invalid edit consumed generation or executed", err)
			}
		}
		calls := f.model.requests.Load()
		out, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Now use different date filters"}, ParameterEdits: []nlqexec.ParameterEdit{edit}})
		if !errors.Is(err, readexec.ErrUnsupported) || out.QueryID != "" || f.model.requests.Load() != calls {
			t.Fatal("typed replacement inferred changed free text", err)
		}
	})
	t.Run("parameter-free parent cannot acquire arbitrary slots", func(t *testing.T) {
		f.model.mode.Store(parameterResponse(t, "SELECT id FROM analytics.sales ORDER BY id", []readexec.Parameter{}))
		unbound, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("List sales records", nlq.LanguageEnglish)})
		if err != nil {
			t.Fatal(err)
		}
		before := f.model.requests.Load()
		result, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: unbound.QueryID, ParameterEdits: []nlqexec.ParameterEdit{edit}})
		if !errors.Is(err, nlqexec.ErrInvalid) || result.QueryID != "" || f.model.requests.Load() != before {
			t.Fatal("created a nonexistent slot", err)
		}
	})
	t.Run("native-invalid projection is corrected with edited value", func(t *testing.T) {
		f.model.mu.Lock()
		f.model.chatSequence = []string{parameterResponse(t, `SELECT missing FROM analytics.sales WHERE amount > $1 ORDER BY id`, []readexec.Parameter{{Kind: "number", Value: "0"}}), parameterResponse(t, base, []readexec.Parameter{{Kind: "number", Value: "99"}})}
		start := len(f.model.requestBodies)
		f.model.mu.Unlock()
		child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{edit}})
		if err != nil || child.ValidationFixes != 1 {
			t.Fatal("typed repair", err)
		}
		out, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
		if err != nil {
			t.Fatal(err)
		}
		requireParameterIDs(t, out, "1", "2")
		scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
		stored, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
		if err != nil || !reflect.DeepEqual(stored.Parameters, []readexec.Parameter{edit.Replacement}) {
			t.Fatal("repair restored old or placeholder value", err)
		}
		f.model.mu.Lock()
		wire := strings.Join(f.model.requestBodies[start:], "\n")
		f.model.mu.Unlock()
		if strings.Contains(wire, edit.Replacement.Value) || strings.Contains(wire, original[0].Value) {
			t.Fatal("correction exposed private values")
		}
	})
	t.Run("typed replacement cannot change clause roles", func(t *testing.T) {
		for _, sql := range []string{`SELECT id FROM analytics.sales WHERE id > $1`, `SELECT id FROM analytics.sales WHERE amount < $1`, `SELECT id FROM analytics.sales WHERE amount > $1 OR true`} {
			f.model.mode.Store(parameterResponse(t, sql, []readexec.Parameter{{Kind: "number", Value: "0"}}))
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			out, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{edit}})
			if err == nil || out.QueryID != "" || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != attempts {
				t.Fatal("unsafe role edit planned/executed", err)
			}
		}
	})
	t.Run("foreign actor session and missing permission cannot edit", func(t *testing.T) {
		permissions := []string{}
		for _, permission := range f.e.Scopes() {
			if permission != "query.execute" {
				permissions = append(permissions, permission)
			}
		}
		denied, err := identity.FromVerified(f.e.Tenant(), f.e.User(), f.e.Session(), permissions, f.e.Deadline(), nil)
		if err != nil {
			t.Fatal(err)
		}
		before := f.model.requests.Load()
		result, err := f.query.Refine(ctx, denied, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{edit}})
		if err == nil || result.QueryID != "" || f.model.requests.Load() != before {
			t.Fatal("edit acted as execution authority", err)
		}
		for _, who := range []struct{ user, session string }{{f.e.User(), "foreign-edit-session"}, {"other-edit-user", f.e.Session()}} {
			e := phase18Envelope(t, f.phase17Fixture, who.user, who.session, false)
			calls := f.model.requests.Load()
			out, err := f.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{edit}})
			if err == nil || out.QueryID != "" || f.model.requests.Load() != calls {
				t.Fatal("foreign edit reached generator", err)
			}
		}
	})
}

func TestSQLRecoveryParameterEditOwnedAnswerAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	q := f.question("List named sales", nlq.LanguageEnglish)
	pending := f.preflight(t, q)
	if pending.Route.Clarification == nil || pending.Route.Clarification.Reason != "required_answers" {
		t.Fatal("owned fixture did not activate policy")
	}
	q.ClarificationQuery = pending.QueryID
	q.AnswerContext = pending.Route.AnswerContext
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("primero"))}
	base := `SELECT id,amount FROM analytics.sales WHERE name <> $1 ORDER BY id`
	original := []readexec.Parameter{{Kind: "text", Value: "excluded-original-841"}}
	f.model.mode.Store(parameterResponse(t, base, original))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil || p.Bindings == nil {
		t.Fatal("mixed-owned parent", err)
	}
	calls := f.model.requests.Load()
	bad, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{{Position: 2, Replacement: readexec.Parameter{Kind: "text", Value: "forged-owned"}}}})
	if !errors.Is(err, nlqexec.ErrInvalid) || bad.QueryID != "" || f.model.requests.Load() != calls {
		t.Fatal("typed edit selected owned parameter", err)
	}
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "text", Value: "model-placeholder"}}))
	replacement := readexec.Parameter{Kind: "text", Value: "excluded-replacement-927"}
	f.model.mu.Lock()
	start := len(f.model.requestBodies)
	f.model.mu.Unlock()
	child, err := f.query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, ParameterEdits: []nlqexec.ParameterEdit{{Position: 1, Replacement: replacement}}, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{f.answer(t, "customer", cw01Text("segundo"))}}})
	if err != nil || child.Bindings == nil {
		t.Fatal("simultaneous owned-answer and model-slot edit", err)
	}
	scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
	stored, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
	if err != nil || stored.Clarification == nil || !reflect.DeepEqual(stored.Clarification.BaseParameters, []readexec.Parameter{replacement}) {
		t.Fatal("owned/model custody merged", err)
	}
	f.query, _ = newPhase18Service(t, f.phase17Fixture)
	run, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, run, "2")
	f.model.mu.Lock()
	wire := strings.Join(f.model.requestBodies[start:], "\n")
	f.model.mu.Unlock()
	for _, secret := range []string{original[0].Value, replacement.Value, "cw-alpha-731", "cw-beta-731", "alias-secret-731"} {
		if strings.Contains(wire, secret) {
			t.Fatal("mixed edit copied private value into model input")
		}
	}
}
