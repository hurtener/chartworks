package chartworks

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/jobs"
)

// ScheduledReportingTarget names an existing reviewed publication, never SQL.
type ScheduledReportingTarget = jobs.ReportingTarget

// ScheduledReportingArgument supplies a declared parameter or report filter.
type ScheduledReportingArgument = jobs.ReportingArgument

// ScheduledReportingValue is a literal or a closed relative-period selection.
type ScheduledReportingValue = jobs.ReportingValue

// ScheduledReportingPeriod resolves against the accepted occurrence's window.
type ScheduledReportingPeriod = jobs.ReportingPeriod

// ScheduledReportingBudget bounds physical warehouse and model attempts.
type ScheduledReportingBudget = jobs.ReportingBudget

// ScheduledReportingDispatch is the server-resolved immutable accepted pin.
type ScheduledReportingDispatch = jobs.ReportingDispatch

// ScheduledReportingReceipt distinguishes execution, retention and delivery.
type ScheduledReportingReceipt = jobs.ReportingReceipt

// ScheduleHistoryRequest uses bounded, mutually exclusive history cursors.
type ScheduleHistoryRequest = jobs.ScheduleHistoryRequest

// ScheduleOccurrence retains actual accepted or skipped half-open windows.
type ScheduleOccurrence = jobs.ScheduleOccurrence

// ScheduleRevision records one immutable definition/state transition.
type ScheduleRevision struct {
	Revision int64 `json:"revision"`
	Enabled bool `json:"enabled"`
	Retired bool `json:"retired"`
	Request ScheduleRequest `json:"request"`
	RecordedAt time.Time `json:"recorded_at"`
}

// ScheduleHistory contains exactly one bounded revision or occurrence stream.
type ScheduleHistory struct {
	ScheduleID string `json:"schedule_id"`
	Kind string `json:"kind"`
	Revisions []ScheduleRevision `json:"revisions"`
	Occurrences []ScheduleOccurrence `json:"occurrences"`
	NextBeforeRevision int64 `json:"next_before_revision,omitempty"`
	NextBeforeDue *time.Time `json:"next_before_due,omitempty"`
}

func scheduleCoordinate(id string, expected int64) error {
	if !wireID(id) || expected < 1 || expected >= 1<<62 { return errors.New("chartworks: invalid schedule coordinate") }
	return nil
}

// ReplaceSchedule changes only future unaccepted work. The effect key and CAS
// revision let the caller safely reconcile an interrupted response.
func (c *Client) ReplaceSchedule(ctx context.Context, id string, expected int64, key string, request ScheduleRequest) (Schedule, error) {
	var out Schedule
	if err := scheduleCoordinate(id, expected); err != nil { return out, err }
	if !wireID(key) { return out, errors.New("chartworks: invalid idempotency key") }
	body := struct { Expected int64 `json:"expected_revision"`; Request ScheduleRequest `json:"request"` }{expected, request}
	err := c.call(ctx, "PUT", "/v1/schedules/"+id, key, body, &out)
	return out, err
}

// RetireSchedule permanently disables future admissions without changing jobs
// that were already accepted. Those jobs remain independently cancellable.
func (c *Client) RetireSchedule(ctx context.Context, id string, expected int64) (Schedule, error) {
	var out Schedule
	if err := scheduleCoordinate(id, expected); err != nil { return out, err }
	body := struct { Expected int64 `json:"expected_revision"` }{expected}
	err := c.call(ctx, "POST", "/v1/schedules/"+id+"/retire", "", body, &out)
	return out, err
}

// TestSchedule admits an actual cost-bearing test run under the normal shared
// queue and fresh authority checks, not a dry run or a claimed successful query.
func (c *Client) TestSchedule(ctx context.Context, id, key string, expected int64) (Job, error) {
	var out Job
	if err := scheduleCoordinate(id, expected); err != nil { return out, err }
	if !wireID(key) { return out, errors.New("chartworks: invalid idempotency key") }
	body := struct { Expected int64 `json:"expected_revision"` }{expected}
	err := c.call(ctx, "POST", "/v1/schedules/"+id+"/test", key, body, &out)
	return out, err
}

// ScheduleHistory reads immutable metadata. It cannot regenerate a report or
// grant access to the artifact referenced by a returned occurrence.
func (c *Client) ScheduleHistory(ctx context.Context, id string, request ScheduleHistoryRequest) (ScheduleHistory, error) {
	var out ScheduleHistory
	if !wireID(id) || !request.Valid() { return out, errors.New("chartworks: invalid schedule history request") }
	err := c.call(ctx, "POST", "/v1/schedules/"+id+"/history", "", request, &out)
	return out, err
}
