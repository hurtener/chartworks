package nlqapi

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// ExecutionMCPBindings binds only the installed service, never an unavailable placeholder.
func ExecutionMCPBindings(service *nlqexec.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := ExecutionRegistry()
	if err != nil {
		return nil, err
	}
	var bindings []mcpserver.Binding
	mapper := func(err error) mcpserver.Fault {
		_, code := classify(err)
		return mcpserver.Fault{Code: code, Clarification: clarificationProblem(err)}
	}
	b0, err := mcpserver.Bind(registry, "preflightNLQ", "preflight_question", "query", "Admit a question using authorized semantic routing and persist bounded session evidence. May incur remote model cost; preflight is not a pure read or an executable plan.", service.Preflight, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b0)
	b1, err := mcpserver.Bind(registry, "planNLQ", "plan_question", "query", "Generate and validate one governed read plan using authorized semantics. Persists a plan and may incur model cost; no result query is executed by planning.", service.Plan, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b1)
	b2, err := mcpserver.Bind(registry, "runNLQ", "run_question", "query", "Execute a previously validated plan under current signed session and dependency authority. Persists attempt evidence and may incur source or bounded correction cost. Inspect the outcome before retrying.", service.Run, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b2)
	b3, err := mcpserver.Bind(registry, "refineNLQ", "refine_question", "query", "Refine an existing plan within its original signed session and current authorized semantic context. Generates and persists a new validated plan; may incur model cost.", service.Refine, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b3)
	b4, err := mcpserver.Bind(registry, "feedbackNLQ", "submit_feedback", "query", "Record feedback for a governed query in the current signed session. A corrected SQL candidate is validated, not executed or automatically published. Persists review and possible learning evidence.", func(ctx context.Context, e identity.Envelope, in nlqexec.FeedbackRequest) (FeedbackResult, error) {
		err := service.Feedback(ctx, e, in)
		return FeedbackResult{Accepted: err == nil}, err
	}, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b4)
	b5, err := mcpserver.Bind(registry, "exampleStateNLQ", "review_example", "query", "Review and advance one versioned learning example under current topic and dependency authority. Activation requires bounded positive evidence and an explicit review note; it never publishes rules or semantics.", service.ExampleState, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b5)
	b6, err := mcpserver.Bind(registry, "examplesNLQ", "list_examples", "query", "Inspect bounded versioned learning examples for one currently authorized topic. Protected SQL remains redacted without separate inspection authority.", func(ctx context.Context, e identity.Envelope, in ExampleListRequest) ([]nlqexec.ExampleRecord, error) {
		return service.Examples(ctx, e, in.Topic, in.Limit)
	}, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b6)
	b7, err := mcpserver.Bind(registry, "exportExamplesNLQ", "export_examples", "query", "Export a bounded protected learning bundle under current topic, dependency, feedback and SQL-inspection authority. It performs no source or model work.", service.ExportExamples, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b7)
	b8, err := mcpserver.Bind(registry, "importExampleNLQ", "import_example", "query", "Revalidate one portable example against current routing, authority, exact origin and native SQL safety, then persist it as an unreviewed candidate. It never activates imported evidence.", service.ImportExample, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b8)
	return bindings, nil
}

// BYOMCPBindings binds only the installed service, never an unavailable placeholder.
func BYOMCPBindings(service *nlqbyo.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := BYORegistry(service.CanCreate())
	if err != nil {
		return nil, err
	}
	var bindings []mcpserver.Binding
	mapper := func(err error) mcpserver.Fault {
		_, code := classify(err)
		return mcpserver.Fault{Code: code, Clarification: clarificationProblem(err)}
	}
	b0, err := mcpserver.Bind(registry, "readQueryContext", "read_query_context", "byo", "Read a previously created opaque context reference with current authority and exact version checks. Returns content-free step receipts, not retained SQL result values; never re-executes a submitted step.", service.Lookup, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b0)
	b1, err := mcpserver.Bind(registry, "submitSQL", "submit_sql", "byo", "Submit an explicit SQL step against an opaque authorized context. Uses the ordinary validator and read executor, persists an idempotent receipt, and never reruns a previously accepted step on retry.", service.Submit, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b1)
	if service.CanCreate() {
		b2, err := mcpserver.Bind(registry, "getQueryContext", "get_query_context", "byo", "Create an opaque expiring semantic context for external SQL analysis. May incur routing model cost and persists exact pins. The reference grants no authority and does not execute SQL.", service.Create, mapper)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, b2)
	}
	return bindings, nil
}
