package reporting

import "github.com/hurtener/chartworks/internal/exec"

// nextFrozenQueryNumber classifies already authorized journal receipts. A queue
// retry before SQL must still use physical attempt one. Lost successful values
// and indeterminate native work are never silently replaced with another query.
func nextFrozenQueryNumber(attempts []exec.Attempt) (int, error) {
	if len(attempts) == 0 {
		return 1, nil
	}
	if len(attempts) > 3 {
		return 0, ErrBudget
	}
	seen := map[string]bool{}
	for index, a := range attempts {
		if a.Number != index+1 || len(a.ID) != 32 || seen[a.ID] {
			return 0, ErrInvalid
		}
		seen[a.ID] = true
	}
	last := attempts[len(attempts)-1]
	if successful(last.Status) {
		return 0, ErrIncomplete
	}
	if last.Finished == nil || last.RemoteState != "stopped" && last.RemoteState != "not_issued" {
		return 0, exec.ErrUncertain
	}
	switch last.Status {
	case "failed", "cancelled", "timed_out", "interrupted":
		if last.Number >= 3 {
			return 0, ErrBudget
		}
		return last.Number + 1, nil
	default:
		return 0, exec.ErrUncertain
	}
}
