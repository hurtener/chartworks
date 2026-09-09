package workapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/jobs"
)

const workRequestMaxBytes = 8192

// ScheduleStateRequest is the closed body accepted by schedule CAS updates.
type ScheduleStateRequest struct {
	Expected int64 `json:"expected_revision"`
	Enabled  bool  `json:"enabled"`
}

func workErrors(receipt bool) []api.ErrorResponse {
	errors := []api.ErrorResponse{
		{Status: 400, Code: "invalid_request"},
		{Status: 401, Code: "unauthenticated"},
		{Status: 401, Code: "unauthorized"},
		{Status: 403, Code: "forbidden"},
		{Status: 404, Code: "not_found"},
		{Status: 409, Code: "conflict"},
		{Status: 409, Code: "role_disabled"},
		{Status: 429, Code: "budget_exceeded"},
		{Status: 502, Code: "invalid_provider_output"},
		{Status: 503, Code: "unavailable"},
		{Status: 504, Code: "cancelled_or_timed_out"},
	}
	if receipt {
		for i := range errors {
			errors[i].Receipt = true
		}
	}
	return errors
}

// APIRegistry describes the actual gateway and durable-work routes selected by
// the supplied service capabilities. Disabled dispatch never advertises
// admission or resume operations that Handler cannot serve.
func APIRegistry(engine gateway.Engine, queue *jobs.Service) (*api.Registry, error) {
	probeRequest, err := api.SchemaFor("gatewayProbeRequest", reflect.TypeFor[ProbeRequest](), false)
	if err != nil {
		return nil, err
	}
	probeResponse, err := api.SchemaFor("gatewayProbeResponse", reflect.TypeFor[ProbeResult](), true)
	if err != nil {
		return nil, err
	}
	empty, err := api.SchemaFor("workEmptyRequest", reflect.TypeFor[struct{}](), false)
	if err != nil {
		return nil, err
	}
	submission, err := api.SchemaFor("jobSubmissionRequest", reflect.TypeFor[jobs.Submission](), false)
	if err != nil {
		return nil, err
	}
	schedule, err := api.SchemaFor("scheduleRequest", reflect.TypeFor[jobs.ScheduleRequest](), false)
	if err != nil {
		return nil, err
	}
	scheduleState, err := api.SchemaFor("scheduleStateRequest", reflect.TypeFor[ScheduleStateRequest](), false)
	if err != nil {
		return nil, err
	}
	jobResponse, err := api.SchemaFor("jobResponse", reflect.TypeFor[jobs.Job](), true)
	if err != nil {
		return nil, err
	}
	jobsResponse, err := api.SchemaFor("jobsResponse", reflect.TypeFor[[]jobs.Job](), true)
	if err != nil {
		return nil, err
	}
	scheduleResponse, err := api.SchemaFor("scheduleResponse", reflect.TypeFor[jobs.Schedule](), true)
	if err != nil {
		return nil, err
	}
	probeErrors := workErrors(true)
	workErrors := workErrors(false)
	definitions := make([]api.Definition, 0, 9)
	if engine != nil {
		definitions = append(definitions, api.Definition{
			Operation: api.Operation{Method: "POST", Path: "/v1/gateway/probes", Action: "ops.model", Effect: "paid_model_call"},
			ID:        "gatewayProbe", Summary: "Probe one configured remote model role", ResourceLoader: "workapi.Probe", Audit: "gateway.probe", Request: probeRequest, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: probeResponse, Errors: probeErrors,
		})
	}
	if queue == nil {
		if len(definitions) == 0 {
			return nil, nil
		}
		return api.New(definitions)
	}
	dispatch := queue.DispatchEnabled()
	appendDefinition := func(definition api.Definition, enabled bool) {
		if enabled {
			definitions = append(definitions, definition)
		}
	}
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "GET", Path: "/v1/jobs", Action: "scheduling.read", Effect: "metadata_read"},
		ID:        "listJobs", Summary: "List retained durable operation metadata", ResourceLoader: "jobs.Service.List", Audit: "read_only_no_domain_audit", Response: jobsResponse, Errors: workErrors,
	}, true)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "GET", Path: "/v1/jobs/{id}", Action: "scheduling.read", Effect: "metadata_read"},
		ID:        "getJob", Summary: "Read one retained durable operation", ResourceLoader: "jobs.Service.Get", Audit: "read_only_no_domain_audit", Response: jobResponse, Errors: workErrors,
	}, true)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "POST", Path: "/v1/jobs", Action: "scheduling.write", Effect: "durable_admission"},
		Replay:    "keyed", ID: "submitJob", Summary: "Admit one bounded durable operation", ResourceLoader: "jobs.Service.Submit", Audit: "job.admitted", Headers: []api.Parameter{{Name: "Idempotency-Key", In: "header", Description: "Stable key for the logical operation", Type: "string", Required: true, Max: 128, Pattern: "^[A-Za-z0-9_.:-]+$"}}, Request: submission, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: jobResponse, Errors: workErrors,
	}, dispatch)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "POST", Path: "/v1/jobs/{id}/cancel", Action: "scheduling.cancel", Effect: "durable_cancellation"},
		ID:        "cancelJob", Summary: "Record cancellation for one durable operation", ResourceLoader: "jobs.Service.Cancel", Audit: "job.cancelled", Request: empty, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: jobResponse, Errors: workErrors,
	}, true)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "GET", Path: "/v1/schedules/{id}", Action: "scheduling.read", Effect: "metadata_read"},
		ID:        "getSchedule", Summary: "Read one retained schedule", ResourceLoader: "jobs.Service.GetSchedule", Audit: "read_only_no_domain_audit", Response: scheduleResponse, Errors: workErrors,
	}, true)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "POST", Path: "/v1/schedules", Action: "scheduling.write", Effect: "schedule_creation"},
		Replay:    "keyed", ID: "createSchedule", Summary: "Create one bounded schedule", ResourceLoader: "jobs.Service.CreateSchedule", Audit: "schedule.created", Headers: []api.Parameter{{Name: "Idempotency-Key", In: "header", Description: "Stable key for the logical schedule", Type: "string", Required: true, Max: 128, Pattern: "^[A-Za-z0-9_.:-]+$"}}, Request: schedule, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: scheduleResponse, Errors: workErrors,
	}, dispatch)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "PUT", Path: "/v1/schedules/{id}/state", Action: "scheduling.write", Effect: "schedule_state"},
		ID:        "setScheduleState", Summary: "Change one schedule with revision CAS", ResourceLoader: "jobs.Service.SetSchedule", Audit: "schedule.state_changed", Request: scheduleState, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: scheduleResponse, Errors: workErrors,
	}, true)
	appendDefinition(api.Definition{
		Operation: api.Operation{Method: "POST", Path: "/v1/schedules/{id}/runs", Action: "scheduling.execute", Effect: "durable_admission"},
		Replay:    "keyed", ID: "fireSchedule", Summary: "Admit one manual schedule run", ResourceLoader: "jobs.Service.Fire", Audit: "schedule.run_requested", Headers: []api.Parameter{{Name: "Idempotency-Key", In: "header", Description: "Stable key for the logical schedule run", Type: "string", Required: true, Max: 128, Pattern: "^[A-Za-z0-9_.:-]+$"}}, Request: empty, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: jobResponse, Errors: workErrors,
	}, dispatch)
	if len(definitions) == 0 {
		return nil, nil
	}
	return api.New(definitions)
}
