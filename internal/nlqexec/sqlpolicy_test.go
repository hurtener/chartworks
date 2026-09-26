package nlqexec

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlq"
)

func TestSQLRecoveryVocabularyGenerationAndRepair(t *testing.T) {
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		t.Run(dialect, func(t *testing.T) {
			e := testEnvelope(t)
			a, g, call, budget := testGeneration(t, e)
			a.binding.Dialect = dialect
			model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT wrong FROM analytics.sales", nil, "sqlgen"), recoveryCandidate("SELECT id FROM analytics.sales", nil, "sqlfix")}}}
			service := &Service{engine: model, validator: &sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}}
			result, fixes, _, _, err := service.generateAndValidate(context.Background(), e, a, g, call, budget, "")
			if err != nil || fixes != 1 || len(model.requests) != 2 || result.SQL != "SELECT id FROM analytics.sales" {
				t.Fatal("bounded validation flow", err)
			}
			guidance, _ := sqlpolicy.Guidance(dialect)
			for i, request := range model.requests {
				if strings.Count(request.System, guidance) != 1 || !strings.Contains(request.Prompt, "dialect:"+dialect) || len(request.Schema) == 0 {
					t.Fatal("effective request omitted/duplicated policy", i)
				}
			}
			if model.requests[0].Role != "sqlgen" || model.requests[1].Role != "sqlfix" {
				t.Fatal("policy selected a new model role")
			}
		})
	}
}

func TestSQLRecoveryVocabularyUnknownAndCancelledBeforeModel(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	model := &recoveryCapture{}
	s := &Service{engine: model}
	a.binding.Dialect = "private-unsupported"
	if _, receipt, err := s.generate(context.Background(), e, a, g, call, budget, "sqlgen", ""); !errors.Is(err, exec.ErrUnsupported) || len(receipt.Calls) != 0 || len(model.requests) != 0 || strings.Contains(err.Error(), a.binding.Dialect) {
		t.Fatal("unknown profile invoked model or leaked input", err)
	}
	a.binding.Dialect = "postgres"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := s.generate(ctx, e, a, g, call, budget, "sqlgen", ""); !errors.Is(err, context.Canceled) || len(model.requests) != 0 {
		t.Fatal("cancelled generation prepared a model call", err)
	}
}

func TestSQLRecoveryVocabularyIncludedBeforeEnvelopeFit(t *testing.T) {
	e := testEnvelope(t)
	a, g, call, budget := testGeneration(t, e)
	// Capture the exact mandatory prompt plus policy, then set a byte bound just
	// below that effective request. Omitting the policy would incorrectly fit.
	model := &envelopeCapture{maxBytes: 1 << 16, recoveryCapture: recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{recoveryCandidate("SELECT id FROM analytics.sales", nil, "sqlgen")}}}}
	s := &Service{engine: model}
	if _, _, err := s.generate(context.Background(), e, a, g, call, budget, "sqlgen", ""); err != nil {
		t.Fatal(err)
	}
	request := model.requests[0]
	guidance, _ := sqlpolicy.Guidance("postgres")
	if !strings.Contains(request.System, guidance) {
		t.Fatal("missing policy")
	}
	envelope, err := gateway.NewPromptEnvelope("synthetic", "recorded", request.System, "", generationSchema, 1<<16, 0, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	full, _, err := envelope.Measure(request.Prompt)
	if err != nil {
		t.Fatal(err)
	}
	smaller, err := gateway.NewPromptEnvelope("synthetic", "recorded", strings.Replace(request.System, guidance, "", 1), "", generationSchema, full.RequestBytes-1, 0, 1024, 256)
	if err != nil {
		t.Fatal(err)
	}
	if _, fits, err := smaller.Measure(request.Prompt); err != nil || !fits {
		t.Fatal("fixture must fit without policy", err)
	}
	model.maxBytes = full.RequestBytes - 1
	model.requests = nil
	if _, receipt, err := s.generate(context.Background(), e, a, g, call, budget, "sqlgen", ""); !errors.Is(err, nlq.ErrEnvelopeBudget) || len(receipt.Calls) != 0 || len(model.requests) != 0 {
		t.Fatal("policy was appended after request admission", err)
	}
}
