package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
)

type envelopeCapture struct {
	recoveryCapture
	maxBytes int
	roles    []string
}

func (g *envelopeCapture) GenerationEnvelope(ctx context.Context, role, system string, schema *gateway.Schema) (gateway.PromptEnvelope, error) {
	g.roles = append(g.roles, role)
	model, system, digest := gateway.ApplyRuntimeConfig(ctx, role, "recorded", system)
	return gateway.NewPromptEnvelope("synthetic", model, system, digest, schema, g.maxBytes, 0, 1024, 256)
}

func TestSQLRecoveryServiceRefitsBeforeGenerateAndRetainsActualPacket(t *testing.T) {
	e := testEnvelope(t)
	admitted, base, call, budget := testGeneration(t, e)
	a, _ := nlq.NewDefaultContextAssembler()
	g, err := a.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: base.Context, Examples: []nlq.Instruction{{Key: "demo", Text: strings.Repeat("optional-demonstration ", 140)}}, Default: []nlq.Instruction{{Key: "default", Text: "reviewed default"}}})
	if err != nil {
		t.Fatal(err)
	}
	model := &envelopeCapture{maxBytes: 2400, recoveryCapture: recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT id FROM analytics.sales", nil, "sqlgen")}}}}
	svc := &Service{engine: model, validator: &sequenceValidator{}}
	got, fixes, _, _, err := svc.generateAndValidate(context.Background(), e, admitted, g, call, budget, "")
	if err != nil {
		t.Fatal(err)
	}
	if fixes != 0 || len(model.requests) != 1 || got.generation == nil || got.generation.Strategy != nlq.GenerationDefault || strings.Contains(model.requests[0].Prompt, "optional-demonstration") || !strings.Contains(model.requests[0].Prompt, "reviewed default") {
		t.Fatal("effective packet not refitted/retained")
	}
	if got.generation.Fit.OmittedCount != 1 || got.generation.Fit.Omitted[0].Reason != "provider_budget" || !strings.Contains(model.requests[0].Prompt, "source_context:context") {
		t.Fatal("fit audit or suffix missing")
	}
	if !strings.Contains(g.Prompt, "optional-demonstration") {
		t.Fatal("mutated original generation")
	}
}

func TestSQLRecoveryServiceMandatoryEnvelopeFailureDoesNotGenerate(t *testing.T) {
	e := testEnvelope(t)
	admitted, g, call, budget := testGeneration(t, e)
	model := &envelopeCapture{maxBytes: 64}
	validator := &sequenceValidator{}
	svc := &Service{engine: model, validator: validator}
	_, fixes, receipt, _, err := svc.generateAndValidate(context.Background(), e, admitted, g, call, budget, "")
	if !errors.Is(err, nlq.ErrEnvelopeBudget) || fixes != 0 || len(receipt.Calls) != 0 || len(model.requests) != 0 || validator.calls != 0 {
		t.Fatal("over-budget mandatory packet reached provider/validator", err)
	}
	if calls, _ := budget.Used(); calls != 0 {
		t.Fatal("local fit spent provider budget")
	}
}

func TestSQLRecoveryRepairUsesOwnEnvelopeAndRetainsInitialUsage(t *testing.T) {
	e := testEnvelope(t)
	admitted, g, call, budget := testGeneration(t, e)
	model := &envelopeCapture{maxBytes: 8192, recoveryCapture: recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT wrong FROM analytics.sales", nil, "sqlgen"), recoveryCandidate("SELECT id FROM analytics.sales", nil, "sqlfix")}}}}
	svc := &Service{engine: model, validator: &sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}}
	candidate, fixes, _, _, err := svc.generateAndValidate(context.Background(), e, admitted, g, call, budget, "")
	if err != nil || fixes != 1 || len(model.roles) != 2 || model.roles[0] != "sqlgen" || model.roles[1] != "sqlfix" || candidate.generation == nil || strings.Contains(candidate.generation.Prompt, "validation_repair") {
		t.Fatal("repair envelope or initial usage lost", err)
	}
}
