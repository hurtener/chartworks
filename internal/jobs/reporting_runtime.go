package jobs

import (
	"context"
	"errors"
)

// ErrReportingBudget is a refused durable reservation, not observed provider use.
var ErrReportingBudget = errors.New("jobs: reporting occurrence budget exhausted")

// ErrReportingAttention is terminal for this accepted occurrence. Replacing a
// schedule may fix future occurrences; it must not replace an accepted pin.
var ErrReportingAttention = errors.New("jobs: reporting occurrence requires attention")

// ReportingInput binds the resolved publication, output selection, declared
// budgets and original half-open period to this one accepted operation. It is
// computed before the outer manifest hash, avoiding a self-referential digest.
func ReportingInput(j Job) RequestInput {
	if j.Reporting == nil {
		return RequestInput{}
	}
	d := j.Reporting
	return RequestInput{Kind: d.Target.InputKind(), Target: d.Target.ID,
		InputHash: requestDigest([]any{"chartworks-scheduled-reporting-v1", j.ID,
			j.DueAt.UTC(), j.WindowStart.UTC(), j.WindowEnd.UTC(), d.Target,
			d.Revision, d.Digest, d.Pins, d.Blocked})}
}

// ReportingCharge is reserved before a protected physical query or SDK model
// attempt. Unknown outcomes stay charged; retries cannot reset these counters.
type ReportingCharge struct {
	Queries     int `json:"queries"`
	ModelCalls  int `json:"model_calls"`
	ModelTokens int `json:"model_tokens"`
}

// Valid prevents arbitrary signed changes or token-only/negative reservations.
func (c ReportingCharge) Valid() bool {
	return c.Queries == 1 && c.ModelCalls == 0 && c.ModelTokens == 0 ||
		c.Queries == 0 && c.ModelCalls == 1 && c.ModelTokens > 0 && c.ModelTokens <= 16<<20
}

// ReportingUsageRepository is the existing operation store's effect boundary.
// The opaque invocation and its live database fence are both required. A target
// ID, recipient, serialized lease or bearer alone cannot reserve work here.
type ReportingUsageRepository interface {
	ReserveReportingUsage(context.Context, Invocation, ReportingCharge) error
}

// ReportingReceipt separates query outcome, value retention, catalog delivery
// and notification evidence. This phase supports catalog pull delivery only;
// recipients never mean that a notification was requested or an email was sent.
// Reservations are pessimistic ceilings consumed, not a fabricated model bill.
type ReportingReceipt struct {
	Query        string          `json:"query"`
	Artifact     string          `json:"artifact"`
	Catalog      string          `json:"catalog"`
	Notification string          `json:"notification"`
	ArtifactID   string          `json:"artifact_id,omitempty"`
	Kind         string          `json:"kind"`
	Reserved     ReportingCharge `json:"reserved"`
}
