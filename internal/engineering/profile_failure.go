package engineering

import (
	"context"
	"errors"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

// profileReadFailure translates the common executor's sealed receipt, not raw
// driver text. ExecuteCapped can return a failed receipt with a nil Go error;
// that is not evidence of a generic dependency outage or a successful profile.
func profileReadFailure(a readexec.Attempt) error {
	switch a.Status {
	case "uncertain", "interrupted":
		return ErrState
	case "cancelled":
		return context.Canceled
	case "timed_out":
		return context.DeadlineExceeded
	case "failed":
		switch a.Code {
		case "limit_exceeded":
			return readexec.ErrLimit
		case "context_changed":
			return readexec.ErrBinding
		case "unsupported":
			return readexec.ErrUnsupported
		case "result_type_unsupported", "invalid_result":
			return readexec.ErrType
		}
	}
	return ErrUnavailable
}

// profileFailureCode extends the shared engineering codes for read-only source
// failures. Unknown messages never become public codes or inferred successes.
func profileFailureCode(err error) string {
	switch {
	case errors.Is(err, readexec.ErrUncertain):
		return "reconciliation_required"
	case errors.Is(err, readexec.ErrCancelled):
		return "cancelled"
	case errors.Is(err, readexec.ErrUnsupported):
		return "unsupported"
	case errors.Is(err, readexec.ErrType):
		return "result_type_unsupported"
	default:
		return engineeringCode(err)
	}
}
