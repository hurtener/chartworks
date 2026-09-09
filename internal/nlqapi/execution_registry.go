package nlqapi

import (
	"net/http"
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// ExampleListRequest selects bounded, tenant-scoped learning examples for one topic.
// The service accepts either query.plan or feedback.write for this read.
type ExampleListRequest struct {
	Topic string `json:"topic"`
	Limit int    `json:"limit"`
}

// FeedbackResult confirms that feedback was durably accepted.
type FeedbackResult struct {
	Accepted bool `json:"accepted"`
}

// ExecutionRegistry returns the public Phase 18 generation, execution and
// learning operations. It is composed only when the corresponding service is
// enabled, so OpenAPI cannot advertise an unavailable executor.
func ExecutionRegistry() (*api.Registry, error) {
	errors := executionErrors()
	definitions := []struct {
		method, path, action, effect, id, summary, loader string
		request, response                                 reflect.Type
	}{
		{http.MethodPost, "/v1/nlq/preflight", "query.preflight", "nlq_routing_and_preflight_commit", "preflightNLQ", "Admit a question and return bounded routing evidence", "nlqexec.Service.Preflight", reflect.TypeFor[nlqexec.PreflightRequest](), reflect.TypeFor[nlqexec.PreflightResult]()},
		{http.MethodPost, "/v1/nlq/plans", "query.plan", "nlq_generation_and_plan_commit", "planNLQ", "Generate and validate one governed read plan", "nlqexec.Service.Plan", reflect.TypeFor[nlqexec.PlanRequest](), reflect.TypeFor[nlqexec.PlanResult]()},
		{http.MethodPost, "/v1/nlq/runs", "query.execute", "nlq_validated_read_execution", "runNLQ", "Execute one previously validated NLQ plan", "nlqexec.Service.Run", reflect.TypeFor[nlqexec.RunRequest](), reflect.TypeFor[nlqexec.RunResult]()},
		{http.MethodPost, "/v1/nlq/refinements", "query.execute", "nlq_refine_generation_and_plan_commit", "refineNLQ", "Refine a plan within its signed session", "nlqexec.Service.Refine", reflect.TypeFor[nlqexec.RefineRequest](), reflect.TypeFor[nlqexec.PlanResult]()},
		{http.MethodPost, "/v1/nlq/feedback", "feedback.write", "nlq_feedback_commit", "feedbackNLQ", "Record bounded feedback for a governed query", "nlqexec.Service.Feedback", reflect.TypeFor[nlqexec.FeedbackRequest](), reflect.TypeFor[FeedbackResult]()},
		{http.MethodPost, "/v1/nlq/examples/state", "feedback.write", "nlq_example_state_commit", "exampleStateNLQ", "Advance one reviewed NLQ example state", "nlqexec.Service.ExampleState", reflect.TypeFor[nlqexec.ExampleStateRequest](), reflect.TypeFor[nlqexec.ExampleRecord]()},
		{http.MethodPost, "/v1/nlq/examples/read", "query.plan", "nlq_examples_read", "examplesNLQ", "Read bounded tenant-scoped NLQ examples", "nlqexec.Service.Examples", reflect.TypeFor[ExampleListRequest](), reflect.TypeFor[[]nlqexec.ExampleRecord]()},
	}
	out := make([]api.Definition, 0, len(definitions))
	for _, item := range definitions {
		request, err := api.SchemaFor(item.id+"Request", item.request, false, api.OptionalJSONFields)
		if err != nil {
			return nil, err
		}
		response, err := api.SchemaFor(item.id+"Response", item.response, true)
		if err != nil {
			return nil, err
		}
		out = append(out, api.Definition{
			Operation:      api.Operation{Method: item.method, Path: item.path, Action: item.action, Effect: item.effect},
			ID:             item.id,
			Summary:        item.summary,
			ResourceLoader: item.loader,
			Audit:          "query lifecycle audit",
			MaxBodyBytes:   MaxBodyBytes,
			Request:        request,
			Response:       response,
			Errors:         errors,
		})
	}
	return api.New(out)
}

func executionErrors() []api.ErrorResponse {
	return []api.ErrorResponse{
		{Status: http.StatusBadRequest, Code: "invalid_request"},
		{Status: http.StatusUnauthorized, Code: "unauthenticated"},
		{Status: http.StatusUnauthorized, Code: "unauthorized"},
		{Status: http.StatusForbidden, Code: "forbidden"},
		{Status: http.StatusNotFound, Code: "not_found"},
		{Status: http.StatusConflict, Code: "conflict"},
		{Status: http.StatusConflict, Code: "context_changed"},
		{Status: http.StatusConflict, Code: "foreign_session"},
		{Status: http.StatusConflict, Code: "no_plan"},
		{Status: http.StatusRequestEntityTooLarge, Code: "limit_exceeded"},
		{Status: http.StatusUnprocessableEntity, Code: "insufficient_context"},
		{Status: http.StatusUnprocessableEntity, Code: "sql_unsafe"},
		{Status: http.StatusUnprocessableEntity, Code: "unsupported"},
		{Status: http.StatusUnprocessableEntity, Code: "validation_budget_exhausted"},
		{Status: http.StatusUnprocessableEntity, Code: "execution_budget_exhausted"},
		{Status: http.StatusBadGateway, Code: "execution_failed"},
		{Status: http.StatusServiceUnavailable, Code: "generation_failed"},
		{Status: http.StatusServiceUnavailable, Code: "unavailable"},
		{Status: http.StatusGatewayTimeout, Code: "cancelled_or_timed_out"},
	}
}
