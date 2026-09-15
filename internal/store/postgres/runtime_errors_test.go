package postgres

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestRuntimeDomainErrorsRemainTypedAndSanitized(t *testing.T) {
	for _, sentinel := range []error{reporting.ErrBudget, reporting.ErrExpired, reporting.ErrIncomplete, reporting.ErrStale, engineering.ErrProposalReview, engineering.ErrProposalDrift, engineering.ErrProposalConflict, engineering.ErrCompensationBlocked} {
		// Driver diagnostics must never survive even when wrapping a safe domain error.
		got := safe(fmt.Errorf("synthetic-private-diagnostic: %w", sentinel))
		if got != sentinel || !errors.Is(got, sentinel) {
			t.Fatalf("lost domain classification: %v", sentinel)
		}
	}
	if safe(errors.New("synthetic-driver-secret")) != store.ErrUnavailable {
		t.Fatal("unknown driver diagnostic escaped")
	}
}
