package reporting

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

func TestScheduledFailureClassification(t *testing.T) {
	for _, tc := range []struct{ input, want error }{
		{nil, nil}, {context.Canceled, context.Canceled},
		{context.DeadlineExceeded, context.DeadlineExceeded},
		{store.ErrUnavailable, store.ErrUnavailable}, {store.ErrInvalid, jobs.ErrReportingAttention},
		{ErrInvalid, jobs.ErrReportingAttention}, {ErrStale, jobs.ErrReportingAttention},
		{ErrIncomplete, jobs.ErrReportingAttention},
		{ErrBudget, jobs.ErrReportingBudget}, {gateway.ErrBudget, jobs.ErrReportingBudget},
		{jobs.ErrReportingBudget, jobs.ErrReportingBudget}, {access.ErrForbidden, jobs.ErrAuthority},
	} {
		got := scheduledError(tc.input)
		if !errors.Is(got, tc.want) {
			t.Fatalf("%v: got %v, want %v", tc.input, got, tc.want)
		}
	}
}
