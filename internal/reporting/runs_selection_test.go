package reporting

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
)

// This engine is present only to distinguish configuration availability from
// permission to make a call. These pure admission/receipt tests must never use it.
type forbiddenOutputEngine struct{ t *testing.T }

func (e forbiddenOutputEngine) Generate(context.Context, gateway.Call, *gateway.Budget, string, string, string, *gateway.Schema) (gateway.Generated, error) {
	e.t.Error("output admission or unavailable receipt called a model")
	return gateway.Generated{}, gateway.ErrDisabled
}
func (e forbiddenOutputEngine) Embed(context.Context, gateway.Call, *gateway.Budget, string, []string) (gateway.Embedded, error) {
	e.t.Error("frozen output called embeddings")
	return gateway.Embedded{}, gateway.ErrDisabled
}
func (e forbiddenOutputEngine) Rerank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	e.t.Error("frozen output called reranking")
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (e forbiddenOutputEngine) VisualRank(context.Context, gateway.Call, *gateway.Budget, string, gateway.Candidates) (gateway.Ranked, error) {
	e.t.Error("frozen output called visual ranking")
	return gateway.Ranked{}, gateway.ErrDisabled
}
func (forbiddenOutputEngine) Space() string                          { return "selection-only" }
func (forbiddenOutputEngine) EmbeddingSpace() gateway.EmbeddingSpace { return gateway.EmbeddingSpace{} }
func (forbiddenOutputEngine) Close()                                 {}

func TestFrozenOutputSelectionAvailabilityAndConsent(t *testing.T) {
	for _, availability := range []string{"available", "missing-engine", "model-version", "schema-version", "missing-spec"} {
		t.Run(availability, func(t *testing.T) {
			s := &Runs{model: forbiddenOutputEngine{t}, modelVersion: "v1", limits: config.DefaultReportingExecution()}
			d := contractDefinition()
			n := contractNarrative()
			n.SchemaVersion = "grounded-narrative-v1"
			d.Outputs = append(d.Outputs, Output{ID: "narrative", Kind: "narrative", Narrative: &n})
			switch availability {
			case "missing-engine":
				s.model = nil
			case "model-version":
				n.ModelVersion = "other-model"
			case "schema-version":
				n.SchemaVersion = "unsupported-schema"
			case "missing-spec":
				d.Outputs[1].Narrative = nil
			}
			for _, policy := range []string{"fail", "allow_partial"} {
				request := RunRequest{PartialPolicy: policy, Outputs: []string{"narrative", "table"}}
				if _, err := s.selectRunOutputs(d, request); !errors.Is(err, ErrUnavailable) {
					t.Fatal("partial mode granted narrative consent", policy, err)
				}
				request.Narrative = true
				selected, err := s.selectRunOutputs(d, request)
				allowed := availability != "missing-spec" && (policy == "allow_partial" || availability == "available")
				if !allowed {
					if !errors.Is(err, ErrUnavailable) {
						t.Fatal("strict admission accepted unavailable configuration", policy, err)
					}
				} else if err != nil || len(selected) != 2 || selected[0].ID != "narrative" || selected[1].ID != "table" {
					t.Fatal("explicit partial selection or output order lost", policy, selected, err)
				}
				request.Outputs, request.Narrative = []string{"table"}, false
				selected, err = s.selectRunOutputs(d, request)
				if err != nil || len(selected) != 1 || selected[0].ID != "table" {
					t.Fatal("unselected narrative disabled deterministic work", policy, err)
				}
			}
		})
	}
}

func TestFrozenPartialNarrativeDeclaredBudgets(t *testing.T) {
	s := &Runs{modelVersion: "v1", limits: config.DefaultReportingExecution()}
	d := contractDefinition()
	n := contractNarrative()
	n.SchemaVersion = "grounded-narrative-v1"
	d.Outputs = append(d.Outputs, Output{ID: "narrative", Kind: "narrative", Narrative: &n})
	request := RunRequest{Narrative: true, PartialPolicy: "allow_partial"}
	for _, budget := range []string{"calls", "tokens"} {
		t.Run(budget, func(t *testing.T) {
			n.MaxCalls, n.MaxTokens = 1, 1024
			if budget == "calls" {
				n.MaxCalls = s.limits.NarrativeCalls + 1
			} else {
				n.MaxTokens = s.limits.NarrativeTokens + 1
			}
			if _, err := s.selectRunOutputs(d, request); !errors.Is(err, ErrBudget) {
				t.Fatal("unavailable narrative bypassed its declared budget", err)
			}
		})
	}
}

func TestFrozenUnavailableNarrativeReceipt(t *testing.T) {
	for _, availability := range []string{"missing-engine", "manifest-model", "output-model", "schema-version", "missing-spec"} {
		t.Run(availability, func(t *testing.T) {
			s := &Runs{model: forbiddenOutputEngine{t}, modelVersion: "v1", limits: config.DefaultReportingExecution()}
			n := contractNarrative()
			n.SchemaVersion = "grounded-narrative-v1"
			output := Output{ID: "narrative", Kind: "narrative", Narrative: &n}
			m := RunManifest{Model: "v1"}
			switch availability {
			case "missing-engine":
				s.model = nil
			case "manifest-model":
				m.Model = "other-model"
			case "output-model":
				n.ModelVersion = "other-model"
			case "schema-version":
				n.SchemaVersion = "unsupported-schema"
			case "missing-spec":
				output.Narrative = nil
			}
			m.Outputs = []Output{output}
			// No repository, authority or result is supplied: unavailable outputs
			// must return before any I/O, reservation or evidence access.
			receipt, err := s.makeOutput(context.Background(), identity.Envelope{}, jobs.Invocation{}, m, exec.Result{}, output)
			if err != nil || receipt.State != "failed" || receipt.Code != "narrative_unavailable" || receipt.Narrative != nil || receipt.Chart != nil || receipt.ReservedCalls != 0 || receipt.ReservedTokens != 0 || CheckFrozenOutput(m, receipt, false) != nil {
				t.Fatal("unavailable output fabricated content, spending or an invalid receipt", receipt, err)
			}
		})
	}
}
