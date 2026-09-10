package reporting

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestFrozenQueryAttemptClassification(t *testing.T) {
	now := time.Now()
	makeAttempt := func(number int, status, remote string, finished bool) exec.Attempt {
		a := exec.Attempt{ID: fmt.Sprintf("%032x", number), Number: number, Status: status, RemoteState: remote}
		if finished {
			a.Finished = &now
		}
		return a
	}
	for _, test := range []struct {
		name     string
		attempts []exec.Attempt
		want     int
		failure  error
	}{
		{"queue_retry_before_query", nil, 1, nil},
		{"failed_stopped", []exec.Attempt{makeAttempt(1, "failed", "stopped", true)}, 2, nil},
		{"failed_not_issued", []exec.Attempt{makeAttempt(1, "failed", "not_issued", true)}, 2, nil},
		{"cancelled_stopped", []exec.Attempt{makeAttempt(1, "cancelled", "stopped", true)}, 2, nil},
		{"timeout_stopped", []exec.Attempt{makeAttempt(1, "timed_out", "stopped", true)}, 2, nil},
		{"reconciled_interruption", []exec.Attempt{makeAttempt(1, "interrupted", "stopped", true)}, 2, nil},
		{"lost_successful_values", []exec.Attempt{makeAttempt(1, "succeeded", "stopped", true)}, 0, ErrIncomplete},
		{"lost_empty_values", []exec.Attempt{makeAttempt(1, "empty", "stopped", true)}, 0, ErrIncomplete},
		{"lost_truncated_values", []exec.Attempt{makeAttempt(1, "truncated", "stopped", true)}, 0, ErrIncomplete},
		{"remote_unknown", []exec.Attempt{makeAttempt(1, "uncertain", "unknown", true)}, 0, exec.ErrUncertain},
		{"cancel_not_termination", []exec.Attempt{makeAttempt(1, "cancelled", "running", true)}, 0, exec.ErrUncertain},
		{"accepted_not_terminal", []exec.Attempt{makeAttempt(1, "accepted", "not_issued", false)}, 0, exec.ErrUncertain},
		{"missing_first_attempt", []exec.Attempt{makeAttempt(2, "failed", "stopped", true)}, 0, ErrInvalid},
		{"duplicate_number", []exec.Attempt{makeAttempt(1, "failed", "stopped", true), makeAttempt(1, "failed", "stopped", true)}, 0, ErrInvalid},
		{"next_physical_attempt", []exec.Attempt{makeAttempt(1, "failed", "stopped", true), makeAttempt(2, "failed", "stopped", true)}, 3, nil},
		{"physical_budget", []exec.Attempt{makeAttempt(1, "failed", "stopped", true), makeAttempt(2, "failed", "stopped", true), makeAttempt(3, "failed", "stopped", true)}, 0, ErrBudget},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := nextFrozenQueryNumber(test.attempts)
			if got != test.want || !errors.Is(err, test.failure) {
				t.Fatalf("got (%d,%v), want (%d,%v)", got, err, test.want, test.failure)
			}
		})
	}
}
