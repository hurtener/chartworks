package workapi

import (
	"reflect"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/jobs"
)

// ScheduleRevisionRequest requires explicit consent to the currently shown
// definition. A concurrent edit cannot silently change a requested test run.
type ScheduleRevisionRequest struct {
	Expected int64 `json:"expected_revision"`
}

// ScheduleReplaceRequest replaces only future unaccepted occurrences.
type ScheduleReplaceRequest struct {
	Expected int64 `json:"expected_revision"`
	Request jobs.ScheduleRequest `json:"request"`
}

func scheduleLifecycleDefinitions(dispatch bool) ([]api.Definition, error) {
	revision, err := api.SchemaFor("scheduleRevisionRequest", reflect.TypeFor[ScheduleRevisionRequest](), false)
	if err != nil { return nil, err }
	replacement, err := api.SchemaFor("scheduleReplaceRequest", reflect.TypeFor[ScheduleReplaceRequest](), false, api.NullableCollections)
	if err != nil { return nil, err }
	historyRequest, err := api.SchemaFor("scheduleHistoryRequest", reflect.TypeFor[jobs.ScheduleHistoryRequest](), false)
	if err != nil { return nil, err }
	history, err := api.SchemaFor("scheduleHistoryResponse", reflect.TypeFor[jobs.ScheduleHistory](), true)
	if err != nil { return nil, err }
	schedule, err := api.SchemaFor("scheduleResponse", reflect.TypeFor[jobs.Schedule](), true)
	if err != nil { return nil, err }
	job, err := api.SchemaFor("jobResponse", reflect.TypeFor[jobs.Job](), true)
	if err != nil { return nil, err }
	definitions := []api.Definition{
		{
			Operation: api.Operation{Method: "POST", Path: "/v1/schedules/{id}/history", Action: "scheduling.read", Effect: "metadata_read"},
			ID: "scheduleHistory", Summary: "Read bounded immutable definition or occurrence history", ResourceLoader: "jobs.Service.History", Audit: "read_only_no_domain_audit", Request: historyRequest, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: history, Errors: workErrors(false),
		},
		{
			Operation: api.Operation{Method: "POST", Path: "/v1/schedules/{id}/retire", Action: "scheduling.write", Effect: "schedule_state"},
			ID: "retireSchedule", Summary: "Permanently retire future admissions with revision CAS", ResourceLoader: "jobs.Service.RetireSchedule", Audit: "schedule.updated", Request: revision, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: schedule, Errors: workErrors(false),
		},
	}
	if dispatch {
		header := []api.Parameter{{Name: "Idempotency-Key", In: "header", Description: "Stable key for the logical operation", Type: "string", Required: true, Max: 128, Pattern: "^[A-Za-z0-9_.:-]+$"}}
		definitions = append(definitions,
			api.Definition{
				Operation: api.Operation{Method: "PUT", Path: "/v1/schedules/{id}", Action: "scheduling.write", Effect: "schedule_state"},
				Replay: "keyed", ID: "replaceSchedule", Summary: "Replace future schedule intent with revision CAS", ResourceLoader: "jobs.Service.ReplaceSchedule", Audit: "schedule.updated", Headers: header, Request: replacement, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: schedule, Errors: workErrors(false),
			},
			api.Definition{
				Operation: api.Operation{Method: "POST", Path: "/v1/schedules/{id}/test", Action: "scheduling.execute", Effect: "durable_admission"},
				Replay: "keyed", ID: "testSchedule", Summary: "Admit an actual test run of an explicitly selected revision", ResourceLoader: "jobs.Service.TestSchedule", Audit: "job.accepted", Headers: header, Request: revision, RequestContentType: "application/json", MaxBodyBytes: workRequestMaxBytes, Response: job, Errors: workErrors(false),
			},
		)
	}
	return definitions, nil
}
