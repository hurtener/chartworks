package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
)

func TestSQLRecoveryParameterEditsTypedDetachedValues(t *testing.T) {
	old := []exec.Parameter{{Kind: "number", Value: "9007199254740993.125"}, {Kind: "text", Value: "old customer"}, {Kind: "integer", Value: "8"}, {Kind: "boolean", Value: "true"}, {Kind: "null"}}
	edits := []ParameterEdit{{Position: 4, Replacement: exec.Parameter{Kind: "boolean", Value: "false"}}, {Position: 2, Replacement: exec.Parameter{Kind: "text", Value: "cliente nuevo — 東京"}}, {Position: 1, Replacement: exec.Parameter{Kind: "number", Value: "9007199254740993.126"}}, {Position: 3, Replacement: exec.Parameter{Kind: "integer", Value: "9223372036854775807"}}}
	before := exec.Hash([]any{old, edits})
	got, err := replaceRefinementParameters(context.Background(), old, edits)
	if err != nil || len(got) != len(old) || got[0].Value != "9007199254740993.126" || got[1].Value != "cliente nuevo — 東京" || got[2].Value != "9223372036854775807" || got[3].Value != "false" || got[4] != old[4] {
		t.Fatal("typed replacement lost exact value or slot", err)
	}
	got[0].Value = "mutation"
	if exec.Hash([]any{old, edits}) != before {
		t.Fatal("replacements mutated parent or request")
	}
	unchanged, err := replaceRefinementParameters(context.Background(), old, nil)
	if err != nil || !reflect.DeepEqual(unchanged, old) {
		t.Fatal("legacy no-edit path changed", err)
	}
	unchanged[1].Value = "changed"
	if exec.Hash([]any{old, edits}) != before {
		t.Fatal("no-edit result aliases parent")
	}
}

func TestSQLRecoveryParameterEditsInvalidAreAtomic(t *testing.T) {
	old := []exec.Parameter{{Kind: "number", Value: "7.5"}, {Kind: "text", Value: "private-old"}}
	good := ParameterEdit{Position: 1, Replacement: exec.Parameter{Kind: "number", Value: "9.5"}}
	for _, edits := range [][]ParameterEdit{
		{{Position: 0, Replacement: good.Replacement}}, {{Position: -1, Replacement: good.Replacement}}, {{Position: math.MinInt, Replacement: good.Replacement}}, {{Position: math.MaxInt, Replacement: good.Replacement}},
		{{Position: 3, Replacement: good.Replacement}}, {good, good},
		{good, {Position: 2, Replacement: exec.Parameter{Kind: "integer", Value: "4"}}},
		{{Position: 1, Replacement: exec.Parameter{Kind: "number", Value: "NaN"}}},
		{{Position: 1, Replacement: exec.Parameter{Kind: "number", Value: "1e99999"}}},
		{{Position: 1, Replacement: exec.Parameter{Kind: "null"}}},
		{{Position: 2}}, {{Position: 2, Replacement: exec.Parameter{Kind: "text", Value: "bad\x00value"}}},
		{{Position: 2, Replacement: exec.Parameter{Kind: "text", Value: string([]byte{0xff})}}},
		{{Position: 2, Replacement: exec.Parameter{Kind: "text", Value: strings.Repeat("a", 4097)}}},
		make([]ParameterEdit, 65),
	} {
		before := exec.Hash([]any{old, edits})
		got, err := replaceRefinementParameters(context.Background(), old, edits)
		if !errors.Is(err, ErrInvalid) || got != nil || exec.Hash([]any{old, edits}) != before {
			t.Fatal("partial/invalid replacement accepted or inputs changed", err)
		}
	}
	if _, err := replaceRefinementParameters(context.Background(), nil, []ParameterEdit{good}); !errors.Is(err, ErrInvalid) {
		t.Fatal("edit invented a slot", err)
	}
	if _, err := replaceRefinementParameters(context.Background(), []exec.Parameter{{Kind: "unknown"}}, nil); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("corrupt retained value admitted", err)
	}
	if _, err := replaceRefinementParameters(context.Background(), make([]exec.Parameter, 65), nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("unbounded parent accepted", err)
	}
}

func TestSQLRecoveryParameterEditsPrivateStateAndParentFence(t *testing.T) {
	old := QueryRecord{ID: "parameter-parent", Revision: 3, Question: "List records", SQL: "SELECT id FROM analytics.sales WHERE name<>$1", Parameters: []exec.Parameter{{Kind: "text", Value: "private-before-418"}}}
	q := QuestionRequest{Question: old.Question}
	edits := []ParameterEdit{{Position: 1, Replacement: exec.Parameter{Kind: "text", Value: "private-after-827"}}}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, old.Parameters, edits...)
	if err != nil {
		t.Fatal(err)
	}
	state := refinementParameterState(ctx)
	if state == nil || state.values[0].Value != edits[0].Replacement.Value || state.verifyParent(&old) != nil {
		t.Fatal("selected state or parent fence lost")
	}
	wire, _ := json.Marshal(q)
	logs := fmt.Sprintf("%v %#v %v %#v %v", state, state, edits, edits, edits[0].LogValue())
	for _, secret := range []string{old.Parameters[0].Value, edits[0].Replacement.Value} {
		if strings.Contains(string(wire)+logs, secret) {
			t.Fatal("private slot values copied into prompt/log")
		}
	}
	edits[0].Replacement.Value = "caller mutation"
	if state.values[0].Value != "private-after-827" || old.Parameters[0].Value != "private-before-418" {
		t.Fatal("caller/parent values aliased")
	}
	mutated := old
	mutated.Parameters = append([]exec.Parameter(nil), old.Parameters...)
	mutated.Parameters[0].Value = "changed storage"
	if !errors.Is(state.verifyParent(&mutated), exec.ErrBinding) {
		t.Fatal("edited state accepted a different parent")
	}
	var decoded RefineRequest
	raw, _ := json.Marshal(RefineRequest{QueryID: old.ID, ParameterEdits: edits})
	if json.Unmarshal(raw, &decoded) != nil || refinementParameterState(context.Background()) != nil {
		t.Fatal("request established trusted state")
	}
	q.Question = "Use a completely different filter"
	if _, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, old.Parameters, edits...); !errors.Is(err, exec.ErrUnsupported) {
		t.Fatal("typed value edit authorized a new question", err)
	}
}

func TestSQLRecoveryParameterEditsReboundThroughCorrection(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	old := QueryRecord{ID: "parent", Revision: 1, SQL: "SELECT id FROM analytics.sales WHERE amount>$1", Parameters: []exec.Parameter{{Kind: "number", Value: "671854.625"}}}
	q := QuestionRequest{EditBase: []nlq.Instruction{{Key: "previous_sql", Text: old.SQL}}}
	edit := ParameterEdit{Position: 1, Replacement: exec.Parameter{Kind: "number", Value: "9007199254740993.126"}}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, old.Parameters, edit)
	if err != nil {
		t.Fatal(err)
	}
	a.refinementParameters = refinementParameterState(ctx)
	assembler, _ := nlq.NewDefaultContextAssembler()
	g, err = assembler.ResolvePrecedence(ctx, nlq.GenerationInput{Context: g.Context, EditBase: q.EditBase})
	if err != nil {
		t.Fatal(err)
	}
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT missing FROM analytics.sales WHERE amount>$1", []exec.Parameter{{Kind: "number", Value: "0"}}, "sqlgen"), recoveryCandidate(old.SQL, []exec.Parameter{{Kind: "number", Value: "1"}}, "sqlfix")}}}
	v := &refinementValidator{sequenceValidator: sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}}
	svc := &Service{engine: model, validator: v}
	got, fixes, _, _, err := svc.generateAndValidate(ctx, e, a, g, call, budget, "")
	if err != nil || fixes != 1 || len(v.requests) != 2 || !parametersEqual(got.Parameters, []exec.Parameter{edit.Replacement}) {
		t.Fatal("corrected edit did not retain selected value", err)
	}
	for _, r := range v.requests {
		if !parametersEqual(r.Parameters, []exec.Parameter{edit.Replacement}) {
			t.Fatal("placeholder/old value reached validator")
		}
	}
	for _, r := range model.requests {
		if strings.Contains(r.Prompt, edit.Replacement.Value) || strings.Contains(r.Prompt, old.Parameters[0].Value) {
			t.Fatal("private replacement reached provider")
		}
	}
	got.Assumptions = []string{"Changed from " + old.Parameters[0].Value + " to " + edit.Replacement.Value}
	record := QueryRecord{}
	retainGenerationExplanations(&record, got, nil, &old)
	if strings.Contains(strings.Join(record.Assumptions, " "), "671854") || strings.Contains(strings.Join(record.Assumptions, " "), "9007199") {
		t.Fatal("old/new values escaped through explanation")
	}
}

func TestSQLRecoveryParameterEditsCannotReassignRolesOrOwnedSlots(t *testing.T) {
	old := QueryRecord{ID: "parent", Revision: 1, SQL: "SELECT id FROM analytics.sales WHERE amount>$1", Parameters: []exec.Parameter{{Kind: "number", Value: "2"}}}
	q := QuestionRequest{}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, old.Parameters, ParameterEdit{Position: 1, Replacement: exec.Parameter{Kind: "number", Value: "20"}})
	if err != nil {
		t.Fatal(err)
	}
	a := admission{binding: exec.Binding{Dialect: "postgres"}, refinementParameters: refinementParameterState(ctx)}
	for _, sql := range []string{"SELECT id FROM analytics.sales WHERE id>$1", "SELECT id FROM analytics.sales WHERE amount<$1", "SELECT id FROM analytics.sales WHERE amount>$1 OR true", "SELECT id FROM analytics.sales WHERE amount>20"} {
		if _, err := restoreRefinementParameters(ctx, a, generatedCandidate{SQL: sql, Parameters: []exec.Parameter{{Kind: "number", Value: "0"}}}); err == nil {
			t.Fatal("edit value authorized a changed slot role")
		}
	}
	owned := old
	owned.Parameters = append(append([]exec.Parameter(nil), old.Parameters...), exec.Parameter{Kind: "text", Value: "owned-private"})
	owned.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: old.SQL, BaseParameters: old.Parameters, Binding: exec.BusinessBindingReceipt{SchemaVersion: 1}}
	base, values, err := refinementSQLBase(owned)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = retainRefinementParameters(ctx, &q, owned, base, values, ParameterEdit{Position: 2, Replacement: exec.Parameter{Kind: "text", Value: "forged-owned"}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("edited service-owned slot", err)
	}
}

func TestSQLRecoveryParameterEditsConcurrentAndCancelled(t *testing.T) {
	values := []exec.Parameter{{Kind: "integer", Value: "8"}}
	edits := []ParameterEdit{{Position: 1, Replacement: exec.Parameter{Kind: "integer", Value: "9"}}}
	before := exec.Hash([]any{values, edits})
	done := make(chan bool, 16)
	for i := 0; i < cap(done); i++ {
		go func() {
			v, err := replaceRefinementParameters(context.Background(), values, edits)
			ok := err == nil && v[0].Value == "9"
			if err == nil {
				v[0].Value = "detached"
			}
			done <- ok
		}()
	}
	for i := 0; i < cap(done); i++ {
		if !<-done {
			t.Fatal("concurrent edit changed selection")
		}
	}
	if exec.Hash([]any{values, edits}) != before {
		t.Fatal("shared input mutated")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := replaceRefinementParameters(ctx, values, edits); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
	if _, err := replaceRefinementParameters(nil, values, edits); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil context", err)
	}
}
