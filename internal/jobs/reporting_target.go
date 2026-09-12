package jobs

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
)

// ReportingKind uses the existing durable occurrence/lease engine. The target
// names a reviewed definition, never caller-provided SQL, code or credentials.
const ReportingKind = "reporting.scheduled"

// ReportingPeriod is the closed scheduling wire form of a reporting period.
// Its calendar semantics are interpreted only by the reporting parameter core.
type ReportingPeriod struct {
	Mode string `json:"mode" jsonschema:"enum=explicit,enum=from_date,enum=previous,enum=rolling,enum=schedule_window"`
	Unit string `json:"unit,omitempty" jsonschema:"enum=hour,enum=day,enum=week,enum=month,enum=quarter,enum=year"`
	Count int `json:"count,omitempty"`
	Start string `json:"start,omitempty"`
	End string `json:"end,omitempty"`
	FromDate string `json:"from_date,omitempty"`
	FirstOccurrence string `json:"first_occurrence,omitempty" jsonschema:"enum=reject,enum=from_date,enum=previous"`
	DSTPolicy string `json:"dst_policy" jsonschema:"enum=reject,enum=earlier"`
	MonthPolicy string `json:"month_policy" jsonschema:"enum=clamp,enum=reject"`
}

// ReportingValue does not carry SQL fragments, source locations or authority.
type ReportingValue struct {
	Literal string `json:"literal,omitempty"`
	Period *ReportingPeriod `json:"period,omitempty"`
}

// ReportingArgument assigns a value to an existing block parameter or report
// filter. Reporting validates the declared type before accepting a schedule.
type ReportingArgument struct {
	Name string `json:"name"`
	Value ReportingValue `json:"value"`
}

// ReportingBudget is an explicit ceiling, not an estimate of incurred usage.
// Zero model calls/tokens is a deterministic-only execution policy.
type ReportingBudget struct {
	TimeoutMillis int `json:"timeout_ms"`
	MaxRows int `json:"max_rows"`
	MaxBytes int `json:"max_bytes"`
	QueryAttempts int `json:"query_attempts"`
	ModelCalls int `json:"model_calls"`
	ModelTokens int `json:"model_tokens"`
}

// Valid applies hard bounds independently of the narrower deployment settings.
func (b ReportingBudget) Valid() bool {
	return b.TimeoutMillis >= 1000 && b.TimeoutMillis <= 60000 && b.MaxRows >= 1 && b.MaxRows <= 10000 &&
		b.MaxBytes >= 1024 && b.MaxBytes <= 4<<20 && b.QueryAttempts >= 1 && b.QueryAttempts <= 8 &&
		b.ModelCalls >= 0 && b.ModelCalls <= 8 && b.ModelTokens >= 0 && b.ModelTokens <= 128<<10 &&
		(b.ModelCalls == 0 && b.ModelTokens == 0 || b.ModelCalls > 0 && b.ModelTokens >= 64)
}

// ReportingTarget references existing governed authoring objects. saved_sql
// uses a reviewed published SQL-bearing block without claiming certification;
// block requires an exact certified revision and selected outputs. A saved
// question selects one explicitly dynamic replayable widget of an existing
// published report; report executes the full publication. No hidden report or
// alternative authoring/security model is created by scheduling.
type ReportingTarget struct {
	Type string `json:"type" jsonschema:"enum=saved_sql,enum=saved_question,enum=block,enum=report"`
	ID string `json:"id"`
	Revision int64 `json:"revision"`
	LatestPublished bool `json:"latest_published"`
	Widget string `json:"widget,omitempty"`
	Outputs []string `json:"outputs"`
	Arguments []ReportingArgument `json:"arguments"`
	Locale string `json:"locale"`
	Timezone string `json:"timezone"`
	Dynamic bool `json:"dynamic"`
	Narrative bool `json:"narrative"`
	Budget ReportingBudget `json:"budget"`
}

// ResourceKind selects the existing signed resource namespace.
func (t ReportingTarget) ResourceKind() string {
	if t.Type == "saved_sql" || t.Type == "block" { return "block" }
	if t.Type == "saved_question" || t.Type == "report" { return "report" }
	return ""
}

// InputKind identifies the existing frozen or composition execution consumer.
func (t ReportingTarget) InputKind() string {
	if t.ResourceKind() == "block" { return "reporting.run" }
	if t.ResourceKind() == "report" { return "report.run" }
	return ""
}

// Valid rejects unsupported kinds and implicit dynamic/model execution.
func (t ReportingTarget) Valid() bool {
	if t.ResourceKind() == "" || !identity.Identifier(t.ID) || t.Revision < 0 || t.Revision > 256 ||
		(t.LatestPublished && t.Revision != 0) || (!t.LatestPublished && t.Revision == 0) ||
		!t.Budget.Valid() || len(t.Outputs) > 32 || len(t.Arguments) > 64 ||
		!slices.Contains([]string{"en", "en-US", "es", "es-AR", "es-ES"}, t.Locale) ||
		len(t.Timezone) == 0 || len(t.Timezone) > 128 || t.Timezone == "Local" || strings.Contains(t.Timezone, "..") ||
		strings.HasPrefix(t.Timezone, "/") || strings.ContainsAny(t.Timezone, "\\\x00\r\n") {
		return false
	}
	if t.Type == "block" && t.LatestPublished || t.ResourceKind() == "block" && (t.Widget != "" || t.Dynamic || len(t.Outputs) == 0) ||
		t.ResourceKind() == "report" && len(t.Outputs) != 0 ||
		t.Type == "saved_question" && (!t.Dynamic || !identity.Identifier(t.Widget)) ||
		t.Type != "saved_question" && t.Widget != "" ||
		(t.Dynamic || t.Narrative) && t.Budget.ModelCalls == 0 || (!t.Dynamic && !t.Narrative) && t.Budget.ModelCalls != 0 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range t.Outputs {
		if !identity.Identifier(id) || seen[id] { return false }
		seen[id] = true
	}
	seen = map[string]bool{}
	for _, a := range t.Arguments {
		if !identity.Identifier(a.Name) || seen[a.Name] || len(a.Value.Literal) > 4096 || !utf8.ValidString(a.Value.Literal) || strings.ContainsRune(a.Value.Literal, 0) {
			return false
		}
		seen[a.Name] = true
		if p := a.Value.Period; p != nil {
			if a.Value.Literal != "" || p.Count < 0 || p.Count > 3660 || len(p.Start) > 64 || len(p.End) > 64 || len(p.FromDate) > 64 ||
				!slices.Contains([]string{"explicit", "from_date", "previous", "rolling", "schedule_window"}, p.Mode) ||
				!slices.Contains([]string{"reject", "earlier"}, p.DSTPolicy) || !slices.Contains([]string{"clamp", "reject"}, p.MonthPolicy) {
				return false
			}
		}
	}
	return true
}

// Require never derives permission from a creator, recipient or binding name.
func (t ReportingTarget) Require(e identity.Envelope) error {
	if !t.Valid() { return ErrInvalid }
	return access.Require(e, "reporting.execute", access.Resource{Tenant: e.Tenant(), Kind: t.ResourceKind(), Permission: "execute", ID: t.ID})
}

// ReportingPin is resolved by the repository while accepting the occurrence.
// It is not a client write shape and cannot float during retry or dispatch.
type ReportingPin struct {
	ID string `json:"id"`
	Revision int64 `json:"revision"`
	Digest string `json:"digest"`
}

// ReportingDispatch records only definition coordinates and declared inputs.
// Source SQL, data, issuer/provider credentials and bearer tokens are absent.
// Blocked resolution is retained explicitly rather than silently repinning.
type ReportingDispatch struct {
	Target ReportingTarget `json:"target"`
	Revision int64 `json:"revision"`
	Digest string `json:"digest"`
	Pins []ReportingPin `json:"pins"`
	Input RequestInput `json:"input"`
	Blocked string `json:"blocked,omitempty"`
}

func reportingHash(s string) bool { return len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" }

// Valid also protects deserialization of accepted operational metadata.
func (d ReportingDispatch) Valid() bool {
	if !d.Target.Valid() || !d.Input.Valid() || d.Input.Kind != d.Target.InputKind() || d.Input.Target != d.Target.ID || d.Input.Context != "" || len(d.Pins) > 100 {
		return false
	}
	if d.Blocked != "" {
		return d.Blocked == "dependency_unavailable" && d.Revision == 0 && d.Digest == "" && len(d.Pins) == 0
	}
	if d.Revision < 1 || d.Revision > 256 || !reportingHash(d.Digest) || !d.Target.LatestPublished && d.Revision != d.Target.Revision { return false }
	seen := map[string]bool{}
	for _, p := range d.Pins {
		if !identity.Identifier(p.ID) || p.Revision < 1 || p.Revision > 256 || !reportingHash(p.Digest) || seen[p.ID] { return false }
		seen[p.ID] = true
	}
	return true
}

// ReportingExecutor is implemented by the real reporting service, not a
// second scheduler or token broker. Validation performs no source/model work.
type ReportingExecutor interface {
	ValidateScheduledReporting(context.Context, identity.Envelope, ReportingTarget) error
	ExecuteScheduledReporting(context.Context, Lease, auth.Execution) error
}

// NewWithReporting composes both real target families before workers start.
func NewWithReporting(repo Repository, authority Authority, limits Limits, pipeline PipelineExecutor, reporting ReportingExecutor) (*Service, error) {
	if reporting == nil { return nil, ErrInvalid }
	s, err := New(repo, authority, limits)
	if err != nil { return nil, err }
	s.pipeline, s.reporting = pipeline, reporting
	return s, nil
}

// ReportingInvocation reuses the occurrence's already acquired lease and fresh
// opaque Pengui proof. No inner root queue admission or token replay is allowed.
func ReportingInvocation(proof auth.Execution, lease Lease, task RequestTask) (Invocation, error) {
	if AssertExecution(proof, lease.Job) != nil || lease.Job.Kind != ReportingKind || task.Dispatch == nil || !task.Valid() ||
		task.Dispatch.ManifestHash != lease.Job.ManifestHash || task.Attempts != lease.Attempt {
		return Invocation{}, ErrAuthority
	}
	inv := Invocation{lease: RequestLease{Task: task, Owner: lease.Owner, Fence: lease.Fence, Attempt: lease.Attempt, Until: lease.Until}, authority: proof.Envelope(), owned: true}
	if !inv.Valid() { return Invocation{}, ErrAuthority }
	return inv, nil
}
