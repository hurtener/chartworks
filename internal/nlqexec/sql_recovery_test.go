package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
)

// recoveryCapture is an in-memory synthetic boundary spy, never a production
// prompt logger or live-provider measurement. It records the exact core request
// sent to Engine; the existing Bifrost wire tests own provider serialization.
type recoveryCapture struct {
	sequenceGateway
	requests []recoveryRequest
}

type recoveryRequest struct {
	Role, System, Prompt string
	Schema               json.RawMessage
}

func (g *recoveryCapture) Generate(ctx context.Context, call gateway.Call, budget *gateway.Budget, role, system, prompt string, schema *gateway.Schema) (gateway.Generated, error) {
	g.requests = append(g.requests, recoveryRequest{Role: role, System: system, Prompt: prompt, Schema: schema.Document()})
	response, err := g.sequenceGateway.Generate(ctx, call, budget, role, system, prompt, schema)
	if err == nil {
		err = schema.Validate(response.JSON, 64<<10)
	}
	return response, err
}

func recoveryCandidate(sql string, parameters []exec.Parameter, role string) gateway.Generated {
	raw, err := json.Marshal(generatedCandidate{Decision: "ready", Questions: []string{}, SQL: sql, Parameters: append([]exec.Parameter{}, parameters...), Assumptions: []string{}, Ambiguities: []string{}})
	if err != nil {
		panic(err) // All callers supply synthetic strings and closed parameter DTOs.
	}
	return gateway.Generated{JSON: raw, Receipt: gateway.Receipt{Calls: []gateway.Usage{{Role: role}}}}
}

func TestSQLRecoveryValidationRepairCarriesRejectedCandidate(t *testing.T) {
	e := testEnvelope(t)
	a, generation, call, budget := testGeneration(t, e)
	bad := "SELECT missing_id FROM analytics.sales WHERE id = $1"
	good := "SELECT id FROM analytics.sales WHERE id = $1"
	parameters := []exec.Parameter{{Kind: "integer", Value: "93471928"}}
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{
		recoveryCandidate(bad, parameters, "sqlgen"),
		recoveryCandidate(good, []exec.Parameter{{Kind: "integer", Value: "0"}}, "sqlfix"),
	}}}
	validator := &sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}
	service := &Service{engine: model, validator: validator}
	candidate, fixes, receipt, _, err := service.generateAndValidate(context.Background(), e, a, generation, call, budget, "")
	if err != nil {
		t.Fatal(err)
	}
	if fixes != 1 || len(model.requests) != 2 || validator.calls != 2 || len(receipt.Calls) != 2 {
		t.Fatalf("bounded repair accounting: fixes=%d requests=%d validations=%d", fixes, len(model.requests), validator.calls)
	}
	first, repair := model.requests[0], model.requests[1]
	if first.Role != "sqlgen" || repair.Role != "sqlfix" || len(repair.Schema) == 0 || !strings.Contains(repair.System, "read-only") {
		t.Fatal("missing role/system/schema in captured engine request")
	}
	if !strings.Contains(repair.Prompt, bad) || !strings.Contains(repair.Prompt, "validation_unsafe") {
		t.Fatal("repair did not receive the rejected unbound candidate and safe diagnostic")
	}
	for _, request := range model.requests {
		if strings.Contains(request.Prompt, parameters[0].Value) || strings.Contains(request.System, parameters[0].Value) {
			t.Fatal("model parameter value was copied into a repair request")
		}
	}
	if candidate.SQL != good || !sameParameters(candidate.Parameters, parameters) {
		t.Fatal("repair changed the existing parameter bindings")
	}
}

func TestSQLRecoveryTerminalValidationFailuresDoNotCallRepair(t *testing.T) {
	for name, failure := range map[string]error{
		"forbidden": access.ErrForbidden,
		"binding":   exec.ErrBinding,
		"cancelled": context.Canceled,
		"deadline":  context.DeadlineExceeded,
		"uncertain": exec.ErrUncertain,
	} {
		t.Run(name, func(t *testing.T) {
			e := testEnvelope(t)
			a, generation, call, budget := testGeneration(t, e)
			model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT id FROM analytics.sales", nil, "sqlgen")}}}
			service := &Service{engine: model, validator: &sequenceValidator{errors: []error{failure}}}
			_, fixes, _, _, err := service.generateAndValidate(context.Background(), e, a, generation, call, budget, "")
			if !errors.Is(err, failure) || fixes != 0 || len(model.requests) != 1 {
				t.Fatalf("terminal validation failure spent a correction: fixes=%d calls=%d err=%v", fixes, len(model.requests), err)
			}
		})
	}
}

func TestSQLRecoveryRepairPacketIsUnboundAndValueFree(t *testing.T) {
	e := testEnvelope(t)
	_, generation, _, _ := testGeneration(t, e)
	before := generation.Prompt
	private := "protected-scalar-735194"
	candidate := generatedCandidate{
		SQL:         "SELECT id FROM analytics.sales WHERE id = $1",
		Parameters:  []exec.Parameter{{Kind: "text", Value: private}},
		Assumptions: []string{private}, Ambiguities: []string{private},
		clarification: &ClarificationEvidence{BaseSQL: private},
	}
	repair, err := validationRepairContext(context.Background(), generation, candidate,
		validationCode(errors.Join(exec.ErrUnsafe, errors.New(private)), candidate.SQL))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(repair.Prompt, private) || !strings.Contains(repair.Prompt, candidate.SQL) ||
		!strings.Contains(repair.Prompt, `"parameter_slots":[{"position":1,"kind":"text"}]`) {
		t.Fatal("repair packet lost its unbound SQL/slot contract or exposed private values")
	}
	if generation.Prompt != before || repair.Tokens > repair.Budget || !strings.Contains(repair.Prompt, "tenant_id is the signed tenant") {
		t.Fatal("repair mutated or bypassed the sealed mandatory context")
	}
}

func TestSQLRecoveryRejectsChangedRepairSlots(t *testing.T) {
	original := []exec.Parameter{{Kind: "integer", Value: "42"}}
	for _, parameters := range [][]exec.Parameter{nil, {{Kind: "text", Value: "42"}}, {{Kind: "integer", Value: "0"}, {Kind: "integer", Value: "0"}}} {
		if _, err := restoreValidationRepairParameters(generatedCandidate{SQL: "SELECT id FROM analytics.sales", Parameters: parameters}, original); !errors.Is(err, ErrUnsafeCorrection) {
			t.Fatal("repair changed parameter slot count or kinds")
		}
	}
	fixed, err := restoreValidationRepairParameters(generatedCandidate{SQL: "SELECT id FROM analytics.sales", Parameters: []exec.Parameter{{Kind: "integer", Value: "0"}}}, original)
	if err != nil || !sameParameters(fixed.Parameters, original) {
		t.Fatal("repair did not restore the original bindings")
	}
	fixed.Parameters[0].Value = "99"
	if original[0].Value != "42" {
		t.Fatal("restored parameter slice aliases the original")
	}
}

func TestSQLRecoveryOversizedRepairStopsBeforeSecondCall(t *testing.T) {
	e := testEnvelope(t)
	a, generation, call, budget := testGeneration(t, e)
	bad := "SELECT " + strings.Repeat("missing_name,", 1600) + "id FROM analytics.sales"
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate(bad, nil, "sqlgen")}}}
	service := &Service{engine: model, validator: &sequenceValidator{errors: []error{exec.ErrUnsafe}}}
	_, fixes, receipt, _, err := service.generateAndValidate(context.Background(), e, a, generation, call, budget, "")
	if !errors.Is(err, ErrValidationBudget) || fixes != 0 || len(model.requests) != 1 || len(receipt.Calls) != 1 {
		t.Fatal("oversized repair bypassed fitting or fabricated a second model attempt")
	}
}

func TestSQLRecoveryRepeatedInvalidCandidateStopsAfterOneRepair(t *testing.T) {
	e := testEnvelope(t)
	a, generation, call, budget := testGeneration(t, e)
	bad := recoveryCandidate("SELECT missing_id FROM analytics.sales", nil, "sqlgen")
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{bad, bad, bad}}}
	validator := &sequenceValidator{errors: []error{exec.ErrUnsafe, exec.ErrUnsafe}}
	service := &Service{engine: model, validator: validator}
	_, fixes, receipt, _, err := service.generateAndValidate(context.Background(), e, a, generation, call, budget, "")
	if !errors.Is(err, ErrValidationBudget) || fixes != 1 || len(model.requests) != 2 || validator.calls != 2 || len(receipt.Calls) != 2 {
		t.Fatal("invalid SQL was accepted or correction exceeded one bounded attempt")
	}
}
