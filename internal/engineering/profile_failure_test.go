package engineering

import (
	"context"
	"errors"
	"fmt"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestProfileFailurePreservesSealedReadReason(t *testing.T) {
	for _, tc := range []struct {
		status string
		code   string
		want   error
		public string
	}{
		{"failed", "limit_exceeded", readexec.ErrLimit, "limit_exceeded"},
		{"failed", "context_changed", readexec.ErrBinding, "context_changed"},
		{"failed", "unsupported", readexec.ErrUnsupported, "unsupported"},
		{"failed", "result_type_unsupported", readexec.ErrType, "result_type_unsupported"},
		{"failed", "invalid_result", readexec.ErrType, "result_type_unsupported"},
		{"cancelled", "cancelled", context.Canceled, "cancelled"},
		{"timed_out", "timed_out", context.DeadlineExceeded, "timed_out"},
		{"uncertain", "remote_outcome_unknown", ErrState, "reconciliation_required"},
		{"interrupted", "result_not_retained", ErrState, "reconciliation_required"},
		{"failed", "PRIVATE_DRIVER_MESSAGE", ErrUnavailable, "dependency_unavailable"},
		{"succeeded", "limit_exceeded", ErrUnavailable, "dependency_unavailable"},
		{"", "", ErrUnavailable, "dependency_unavailable"},
	} {
		t.Run(tc.status+"-"+tc.code, func(t *testing.T) {
			err := profileReadFailure(readexec.Attempt{Status: tc.status, Code: tc.code})
			if !errors.Is(err, tc.want) || profileFailureCode(fmt.Errorf("private wrapper: %w", err)) != tc.public {
				t.Fatal("sealed read reason was lost or driver text escaped", err)
			}
		})
	}
	if profileFailureCode(readexec.ErrUncertain) != "reconciliation_required" || profileFailureCode(readexec.ErrCancelled) != "cancelled" {
		t.Fatal("direct executor errors lost their cancellation/uncertainty semantics")
	}
	if profileFailureCode(errors.New("PRIVATE_DRIVER_MESSAGE")) != "dependency_unavailable" {
		t.Fatal("unknown driver error became a public diagnostic")
	}
}
