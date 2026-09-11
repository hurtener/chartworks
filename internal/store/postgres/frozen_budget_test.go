package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SQL predicates are covered by PostgreSQL acceptance. This seam checks that
// accounting cannot accept a failed write or an update that matched no artifact.
type frozenBudgetTx struct {
	pgx.Tx
	tag   pgconn.CommandTag
	err   error
	calls int
}

func (tx *frozenBudgetTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.calls++
	return tx.tag, tx.err
}

func TestFrozenByteAccountingRejectsUncommittedCharge(t *testing.T) {
	marker := errors.New("synthetic storage failure")
	for _, tc := range []struct {
		name    string
		delta   int64
		tag     string
		failure error
		want    error
		calls   int
	}{
		{"exceeds reserved maximum", 61, "UPDATE 1", nil, reporting.ErrBudget, 0},
		{"negative retained bytes", -41, "UPDATE 1", nil, reporting.ErrBudget, 0},
		{"database failure", 1, "", marker, marker, 1},
		{"concurrent limit or missing artifact", 1, "UPDATE 0", nil, reporting.ErrBudget, 1},
		{"exact maximum", 60, "UPDATE 1", nil, nil, 1},
		{"release exact retained bytes", -40, "UPDATE 1", nil, nil, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &frozenBudgetTx{tag: pgconn.NewCommandTag(tc.tag), err: tc.failure}
			h := frozenHead{maximum: 100, view: reporting.RunView{ID: "run", RetainedBytes: 40}}
			err := frozenBytesTx(context.Background(), tx, identity.Envelope{}, h, tc.delta)
			if !errors.Is(err, tc.want) || tx.calls != tc.calls {
				t.Fatalf("charge result %v calls %d; want %v calls %d", err, tx.calls, tc.want, tc.calls)
			}
		})
	}
}
