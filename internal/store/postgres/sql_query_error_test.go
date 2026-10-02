package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSQLRecoveryAdversarialSourceFailureSurvivesRevisionFence(t *testing.T) {
	// WithSource returns its driver callback through transactionDuration -> safe.
	// Only the already classified, detail-free source error may cross that fence.
	for _, err := range []error{readexec.ErrQuery, fmt.Errorf("private-source-canary: %w", readexec.ErrQuery)} {
		got := safe(err)
		if got != readexec.ErrQuery || safe(got) != got {
			t.Fatal("source failure lost its closed classification")
		}
	}
	// Metadata errors are not validated-source query failures and may not authorize
	// SQL correction simply because their driver code resembles a data error.
	for _, code := range []string{"22012", "22003", "21000", "XX000"} {
		got := safe(&pgconn.PgError{Code: code, Message: "private-driver-canary"})
		if errors.Is(got, readexec.ErrQuery) || got == nil || got.Error() == "private-driver-canary" {
			t.Fatal("raw metadata error authorized SQL correction")
		}
	}
	for _, err := range []error{context.Canceled, context.DeadlineExceeded, readexec.ErrUncertain, readexec.ErrBinding, store.ErrUnavailable} {
		if got := safe(errors.Join(err, readexec.ErrQuery)); got != err {
			t.Fatal("query error masked a stronger terminal condition", got)
		}
	}
}
