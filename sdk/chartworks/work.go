package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// JobTarget is a closed implemented target. BindingID refers to Pengui-owned execution authority.
type JobTarget struct {
	Kind      string `json:"kind"`
	BindingID string `json:"binding_id"`
}

// Recurrence is a bounded cron/interval/manual schedule, never executable custom code.
type Recurrence struct {
	Type            string    `json:"type"`
	Cron            string    `json:"cron,omitempty"`
	Timezone        string    `json:"timezone"`
	IntervalSeconds int64     `json:"interval_seconds,omitempty"`
	Anchor          time.Time `json:"anchor,omitempty"`
	Missed          string    `json:"missed"`
	MaxCatchUp      int       `json:"max_catch_up,omitempty"`
	Overlap         string    `json:"overlap"`
}

// ScheduleRequest pairs a supported fixed target with a bounded recurrence definition.
type ScheduleRequest struct {
	Target JobTarget  `json:"target"`
	Spec   Recurrence `json:"spec"`
}

// Schedule is the retained, revisioned definition and its durable occurrence cursor.
type Schedule struct {
	ID               string          `json:"id"`
	Revision         int64           `json:"revision"`
	Enabled          bool            `json:"enabled"`
	Tenant           string          `json:"tenant"`
	Initiator        string          `json:"initiator"`
	InitiatorSession string          `json:"initiator_session"`
	Request          ScheduleRequest `json:"request"`
	NextDue          *time.Time      `json:"next_due,omitempty"`
	PreviousDue      *time.Time      `json:"previous_due,omitempty"`
}

// Job records accepted intent and actual execution outcome separately. It contains no tokens.
type Job struct {
	ID                string    `json:"id"`
	Tenant            string    `json:"tenant"`
	Kind              string    `json:"kind"`
	BindingID         string    `json:"binding_id"`
	Executor          string    `json:"executor"`
	Initiator         string    `json:"initiator"`
	InitiatorSession  string    `json:"initiator_session"`
	State             string    `json:"state"`
	ErrorCode         string    `json:"error_code,omitempty"`
	PolicyRevision    int64     `json:"policy_revision"`
	DueAt             time.Time `json:"due_at"`
	WindowStart       time.Time `json:"window_start"`
	WindowEnd         time.Time `json:"window_end"`
	Cutoff            time.Time `json:"cutoff"`
	Batch             int       `json:"batch"`
	Attempts          int       `json:"attempts"`
	MaxAttempts       int       `json:"max_attempts"`
	ScheduleID        string    `json:"schedule_id,omitempty"`
	ScheduleRevision  int64     `json:"schedule_revision,omitempty"`
	ManifestHash      string    `json:"manifest_hash"`
	DeletedEvents     int64     `json:"deleted_events"`
	DeletedOperations int64     `json:"deleted_operations"`
}

// GatewayProbe is a fixed synthetic-input paid remote role check, not a general prompt endpoint.
type GatewayProbe struct {
	Role       string          `json:"role"`
	OK         bool            `json:"ok"`
	Space      string          `json:"space,omitempty"`
	Dimensions int             `json:"dimensions,omitempty"`
	Receipt    json.RawMessage `json:"receipt"`
}

// ProbeGateway calls the fixed synthetic-input model probe; it cannot send arbitrary prompts.
func (c *Client) ProbeGateway(ctx context.Context, role string) (GatewayProbe, error) {
	var out GatewayProbe
	err := c.call(ctx, "POST", "/v1/gateway/probes", "", struct {
		Role string `json:"role"`
	}{role}, &out)
	return out, err
}

// SubmitJob submits a fixed target with an explicit logical operation key.
func (c *Client) SubmitJob(ctx context.Context, key string, target JobTarget) (Job, error) {
	var out Job
	if !wireID(key) {
		return out, errors.New("chartworks: invalid idempotency key")
	}
	err := c.call(ctx, "POST", "/v1/jobs", key, target, &out)
	return out, err
}

// Jobs reads only the server-authorized retained job list.
func (c *Client) Jobs(ctx context.Context) ([]Job, error) {
	var out []Job
	err := c.call(ctx, "GET", "/v1/jobs", "", nil, &out)
	return out, err
}

// Job reads one retained job by its validated opaque ID.
func (c *Client) Job(ctx context.Context, id string) (Job, error) {
	var out Job
	if !wireID(id) {
		return out, errors.New("chartworks: invalid job ID")
	}
	err := c.call(ctx, "GET", "/v1/jobs/"+id, "", nil, &out)
	return out, err
}

// CancelJob cancels pending work or a live attempt using the tenant-scoped operation row.
func (c *Client) CancelJob(ctx context.Context, id string) (Job, error) {
	var out Job
	if !wireID(id) {
		return out, errors.New("chartworks: invalid job ID")
	}
	err := c.call(ctx, "POST", "/v1/jobs/"+id+"/cancel", "", struct{}{}, &out)
	return out, err
}

// CreateSchedule stores or replays a fixed target and validated recurrence under tenant scope.
func (c *Client) CreateSchedule(ctx context.Context, key string, request ScheduleRequest) (Schedule, error) {
	var out Schedule
	if !wireID(key) {
		return out, errors.New("chartworks: invalid idempotency key")
	}
	err := c.call(ctx, "POST", "/v1/schedules", key, request, &out)
	return out, err
}

// Schedule is the retained, revisioned definition and its durable occurrence cursor.
func (c *Client) Schedule(ctx context.Context, id string) (Schedule, error) {
	var out Schedule
	if !wireID(id) {
		return out, errors.New("chartworks: invalid schedule ID")
	}
	err := c.call(ctx, "GET", "/v1/schedules/"+id, "", nil, &out)
	return out, err
}

// SetScheduleState updates pause/resume with an explicit expected revision.
func (c *Client) SetScheduleState(ctx context.Context, id string, expected int64, enabled bool) (Schedule, error) {
	var out Schedule
	if !wireID(id) {
		return out, errors.New("chartworks: invalid schedule ID")
	}
	body := struct {
		Expected int64 `json:"expected_revision"`
		Enabled  bool  `json:"enabled"`
	}{expected, enabled}
	err := c.call(ctx, "PUT", "/v1/schedules/"+id+"/state", "", body, &out)
	return out, err
}

// FireSchedule admits a replay-safe manual occurrence under the same overlap policy as scheduled work.
func (c *Client) FireSchedule(ctx context.Context, id, key string) (Job, error) {
	var out Job
	if !wireID(id) || !wireID(key) {
		return out, errors.New("chartworks: invalid schedule or operation ID")
	}
	err := c.call(ctx, "POST", "/v1/schedules/"+id+"/runs", key, struct{}{}, &out)
	return out, err
}
func wireID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for _, r := range id {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' && r != ':' {
			return false
		}
	}
	return true
}
