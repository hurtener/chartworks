package nlqexec

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func recoveryReferenceQuery() QueryRecord {
	ref := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	return QueryRecord{Route: nlqroute.RouteResult{Resolutions: []semantics.ClarificationResolution{{ID: "resolved-revenue", Topic: "topic", Pattern: "metric", Slot: "choice", Reference: &ref}}}}
}

func TestSQLRecoveryReferenceOnlyStillReplaysBeforeExecution(t *testing.T) {
	binding := cw07ExecBinding(1)
	replayer := &cw07ReplayRouter{}
	service := &Service{router: replayer}
	query := recoveryReferenceQuery()
	if err := service.verifyQueryClarificationBinding(context.Background(), unitEnvelope(t), query, admission{binding: binding}); err != nil {
		t.Fatal("reference choice fabricated a predicate requirement", err)
	}
	replayer.err = exec.ErrBinding
	if err := service.verifyQueryClarificationBinding(context.Background(), unitEnvelope(t), query, admission{binding: binding}); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("reference-only evidence bypassed current rule replay")
	}
	replayer.err = nil
	query.Route.Resolutions[0].Effect = &semantics.ClarificationEffect{}
	if err := service.verifyQueryClarificationBinding(context.Background(), unitEnvelope(t), query, admission{binding: binding}); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("an executable effect with missing predicates was accepted")
	}
}

func TestSQLRecoveryReferenceOnlyRejectsFabricatedBindingAndChanges(t *testing.T) {
	binding := cw07ExecBinding(1)
	query := recoveryReferenceQuery()
	query.Clarification = &ClarificationEvidence{SchemaVersion: 1, Changes: clarificationChanges(nil, query.Route.Resolutions)}
	if err := validateReferenceOnlyEvidence(query, binding); err != nil {
		t.Fatal("valid reference change receipt rejected", err)
	}
	for _, damage := range []string{"binding", "sql", "parameters", "change", "missing_change", "source", "interpreted_filter"} {
		t.Run(damage, func(t *testing.T) {
			query := recoveryReferenceQuery()
			query.Clarification = &ClarificationEvidence{SchemaVersion: 1, Changes: clarificationChanges(nil, query.Route.Resolutions)}
			switch damage {
			case "binding":
				query.Clarification.Binding.SchemaVersion = 1
			case "sql":
				query.Clarification.BaseSQL = "SELECT id FROM analytics.sales"
			case "parameters":
				query.Clarification.BaseParameters = []exec.Parameter{{Kind: "integer", Value: "1"}}
			case "change":
				query.Clarification.Changes[0].Current = "unreviewed-choice"
			case "missing_change":
				query.Clarification.Changes = nil
			case "source":
				query.Route.SourceBindingDigest = exec.Hash("different-source")
			case "interpreted_filter":
				query.Route.Interpretation = &nlqroute.Interpretation{Values: []nlqroute.ValueInterpretation{{ID: "filter"}}}
			}
			if err := validateReferenceOnlyEvidence(query, binding); !errors.Is(err, exec.ErrBinding) {
				t.Fatal("reference-only path accepted fabricated SQL/filter evidence")
			}
		})
	}
}
