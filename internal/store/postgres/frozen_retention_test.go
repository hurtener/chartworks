package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type retentionFailureRows struct {
	pgx.Rows
	remaining           bool
	closed              bool
	scanErr, errorAfter error
}

func (r *retentionFailureRows) Next() bool {
	if r.remaining {
		r.remaining = false
		return true
	}
	return false
}
func (r *retentionFailureRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	*dest[0].(*string) = "run"
	return nil
}
func (r *retentionFailureRows) Close()     { r.closed = true }
func (r *retentionFailureRows) Err() error { return r.errorAfter }

type retentionFailureTx struct {
	pgx.Tx
	rows                 *retentionFailureRows
	queryErr, errorWrite error
	failAt, calls        int
}

func (tx *retentionFailureTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.calls++
	if tx.calls == tx.failAt {
		return pgconn.CommandTag{}, tx.errorWrite
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *retentionFailureTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return tx.rows, tx.queryErr
}

// Real PostgreSQL tests prove transaction rollback. This checks propagation and
// cursor closure at each read/delete boundary without manufacturing stored data.
func TestFrozenRetentionStopsAtStorageFailure(t *testing.T) {
	marker := errors.New("synthetic retention storage failure")
	scope, err := store.NewScope("tenant", "actor")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                   string
		failAt                 int
		query, scan, iteration bool
	}{
		{"quota lock", 1, false, false, false},
		{"selection", 0, true, false, false},
		{"row decode", 0, false, true, false},
		{"iteration", 0, false, false, true},
		{"cancel intent", 2, false, false, false},
		{"derived output deletion", 3, false, false, false},
		{"payload deletion", 4, false, false, false},
		{"tombstone update", 5, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := &retentionFailureRows{remaining: true}
			tx := &retentionFailureTx{rows: rows, failAt: tc.failAt, errorWrite: marker}
			if tc.query {
				tx.queryErr = marker
			}
			if tc.scan {
				rows.scanErr = marker
			}
			if tc.iteration {
				rows.errorAfter = marker
			}
			count, err := expireFrozenRows(context.Background(), tx, scope, time.Now(), 10)
			if !errors.Is(err, marker) || count != 0 {
				t.Fatal("retention swallowed storage failure", count, err)
			}
			expectedCalls := max(1, tc.failAt)
			if tx.calls != expectedCalls {
				t.Fatal("retention continued after storage failure", tx.calls, expectedCalls)
			}
			if tc.failAt != 1 && !tc.query && !rows.closed {
				t.Fatal("retention leaked selection cursor")
			}
		})
	}
}
