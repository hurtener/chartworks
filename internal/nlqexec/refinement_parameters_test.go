package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
)

func TestSQLRecoveryRefinementBaseSeparatesOwnedParameters(t *testing.T) {
	q := QueryRecord{SQL: "SELECT id FROM analytics.sales WHERE id=$1", Parameters: []exec.Parameter{{Kind: "integer", Value: "1"}}}
	sql, params, err := refinementSQLBase(q)
	if err != nil || sql != q.SQL || !parametersEqual(params, q.Parameters) {
		t.Fatal("raw base", err)
	}
	params[0].Value = "2"
	if q.Parameters[0].Value != "1" {
		t.Fatal("shared parameter memory")
	}
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, Changes: []ClarificationChange{{Action: "removed"}}}
	sql, _, err = refinementSQLBase(q)
	if err != nil || sql != q.SQL {
		t.Fatal("change-only receipt erased SQL")
	}
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: "SELECT id FROM analytics.sales WHERE name<>$1", BaseParameters: []exec.Parameter{{Kind: "text", Value: "model-private"}}, Binding: exec.BusinessBindingReceipt{SchemaVersion: 1}}
	sql, params, err = refinementSQLBase(q)
	if err != nil || sql != q.Clarification.BaseSQL || len(params) != 1 || params[0].Value != "model-private" {
		t.Fatal("used bound parameters", err)
	}
	q.Clarification.BaseSQL = ""
	if _, _, err = refinementSQLBase(q); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("broken owned base admitted", err)
	}
}
func TestSQLRecoveryRefinementSlotsPrivateAndFenced(t *testing.T) {
	old := QueryRecord{ID: "parent", Revision: 4, SQL: "SELECT id FROM analytics.sales WHERE name<>$1", Parameters: []exec.Parameter{{Kind: "text", Value: "private-983"}}}
	q := QuestionRequest{EditBase: []nlq.Instruction{{Key: "previous_sql", Text: old.SQL}}}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, old.Parameters)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(q)
	if strings.Contains(string(raw), "private-983") || !strings.Contains(string(raw), "parameter position") {
		t.Fatal("missing slots or exposed values")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", refinementParameterState(ctx), refinementParameterState(ctx)), "private-983") {
		t.Fatal("state log exposed values")
	}
	var decoded QuestionRequest
	if json.Unmarshal(raw, &decoded) != nil || refinementParameterState(context.Background()) != nil {
		t.Fatal("JSON reconstructed trusted bindings")
	}
	if err := refinementParameterState(ctx).verifyParent(&old); err != nil {
		t.Fatal(err)
	}
	old.Revision++
	if err := refinementParameterState(ctx).verifyParent(&old); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("stale parent fence", err)
	}
	if err := refinementParameterState(ctx).verifyParent(nil); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("parent-free state", err)
	}
}
func TestSQLRecoveryRefinementRestoresValuesAndRejectsSlotDrift(t *testing.T) {
	old := QueryRecord{ID: "parent", Revision: 1, SQL: "SELECT id FROM analytics.sales WHERE amount>$1"}
	q := QuestionRequest{}
	values := []exec.Parameter{{Kind: "number", Value: "9007199254740993.125"}}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, values)
	if err != nil {
		t.Fatal(err)
	}
	values[0].Value = "changed caller"
	a := admission{binding: exec.Binding{Dialect: "postgres"}, refinementParameters: refinementParameterState(ctx)}
	c := generatedCandidate{SQL: "SELECT amount,id FROM analytics.sales WHERE amount>$1 ORDER BY id", Parameters: []exec.Parameter{{Kind: "number", Value: "0"}}}
	got, err := restoreRefinementParameters(context.Background(), a, c)
	if err != nil || got.Parameters[0].Value != "9007199254740993.125" || c.Parameters[0].Value != "0" {
		t.Fatal("private value restoration", err)
	}
	got.Parameters[0].Value = "mutated output"
	if refinementParameterState(ctx).values[0].Value != "9007199254740993.125" {
		t.Fatal("output aliases state")
	}
	for _, bad := range []generatedCandidate{
		{SQL: c.SQL}, {SQL: c.SQL, Parameters: []exec.Parameter{{Kind: "text", Value: "x"}}},
		{SQL: strings.ReplaceAll(c.SQL, "amount>$1", "id>$1"), Parameters: c.Parameters},
		{SQL: strings.ReplaceAll(c.SQL, "amount>$1", "amount>0"), Parameters: c.Parameters},
	} {
		if _, err := restoreRefinementParameters(context.Background(), a, bad); err == nil {
			t.Fatal("parameter edit silently accepted")
		}
	}
}
func TestSQLRecoveryRefinementParametersReachValidationNotProvider(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	old := QueryRecord{ID: "parent", Revision: 1, SQL: "SELECT id FROM analytics.sales WHERE id>$1"}
	q := QuestionRequest{EditBase: []nlq.Instruction{{Key: "previous_sql", Text: old.SQL}}}
	values := []exec.Parameter{{Kind: "integer", Value: "7913543"}}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, values)
	if err != nil {
		t.Fatal(err)
	}
	assembler, _ := nlq.NewDefaultContextAssembler()
	g, err = assembler.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: g.Context, EditBase: q.EditBase})
	if err != nil {
		t.Fatal(err)
	}
	a.refinementParameters = refinementParameterState(ctx)
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate(old.SQL+" ORDER BY id", []exec.Parameter{{Kind: "integer", Value: "0"}}, "sqlgen")}}}
	v := &refinementValidator{}
	s := &Service{engine: model, validator: v}
	candidate, fixes, _, _, err := s.generateAndValidate(context.Background(), e, a, g, call, budget, "")
	if err != nil || fixes != 0 || len(v.requests) != 1 || !parametersEqual(candidate.Parameters, values) || !parametersEqual(v.requests[0].Parameters, values) {
		t.Fatal("bindings lost before validation", err)
	}
	if len(model.requests) != 1 || strings.Contains(model.requests[0].Prompt, values[0].Value) || !strings.Contains(model.requests[0].Prompt, "parameter_slots") && !strings.Contains(model.requests[0].Prompt, "Slot metadata") {
		t.Fatal("provider value leak/missing slot guidance")
	}
}

type refinementValidator struct {
	sequenceValidator
	requests []exec.Request
}

func (v *refinementValidator) ValidateWithin(ctx context.Context, e identity.Envelope, r exec.Request, scope []exec.RelationScope) (exec.Plan, error) {
	v.requests = append(v.requests, r)
	return v.sequenceValidator.ValidateWithin(ctx, e, r, scope)
}

func TestSQLRecoveryRefinementCorrectionKeepsOriginalBindings(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	old := QueryRecord{ID: "parent", Revision: 1, SQL: "SELECT id FROM analytics.sales WHERE id>$1"}
	q := QuestionRequest{}
	values := []exec.Parameter{{Kind: "integer", Value: "71531"}}
	ctx, err := retainRefinementParameters(context.Background(), &q, old, old.SQL, values)
	if err != nil {
		t.Fatal(err)
	}
	a.refinementParameters = refinementParameterState(ctx)
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{
		recoveryCandidate("SELECT missing FROM analytics.sales WHERE id>$1", []exec.Parameter{{Kind: "integer", Value: "1"}}, "sqlgen"),
		recoveryCandidate(old.SQL, []exec.Parameter{{Kind: "integer", Value: "2"}}, "sqlfix"),
	}}}
	v := &refinementValidator{sequenceValidator: sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}}
	s := &Service{engine: model, validator: v}
	got, fixes, _, _, err := s.generateAndValidate(context.Background(), e, a, g, call, budget, "")
	if err != nil || fixes != 1 || len(v.requests) != 2 || !reflect.DeepEqual(got.Parameters, values) {
		t.Fatal("repair lost private parent values", err)
	}
	for _, r := range v.requests {
		if !parametersEqual(r.Parameters, values) {
			t.Fatal("placeholder reached native validation")
		}
	}
	for _, r := range model.requests {
		if strings.Contains(r.Prompt, values[0].Value) {
			t.Fatal("repair leaked retained scalar")
		}
	}
}
