package nlqexec

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/exec"
)

// terminalRunStatus finalizes the logical query, not the physical attempt.
// The actual Executor can withhold its entire receipt after a journal failure.
// Such absence cannot leave a claimed operation planned forever or imply that
// a remote read stopped/succeeded. Control/reconciliation still own that proof.
func terminalRunStatus(report exec.ExecutionReport, runErr error) (string, error) {
	switch report.Attempt.Status {
	case "succeeded", "empty", "truncated":
		if runErr != nil || report.Result == nil {
			return "uncertain", errors.Join(exec.ErrUncertain, runErr)
		}
		return report.Attempt.Status, nil
	case "failed", "uncertain", "cancelled", "timed_out", "interrupted":
		return report.Attempt.Status, runErr
	}
	switch {
	case errors.Is(runErr, exec.ErrUncertain):
		return "uncertain", runErr
	case errors.Is(runErr, exec.ErrCancelled), errors.Is(runErr, context.Canceled):
		return "cancelled", runErr
	case errors.Is(runErr, exec.ErrTimeout), errors.Is(runErr, context.DeadlineExceeded):
		return "timed_out", runErr
	case runErr != nil:
		return "failed", runErr
	default:
		return "uncertain", exec.ErrUncertain
	}
}
