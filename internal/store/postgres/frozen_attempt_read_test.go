package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

type frozenAttemptRows struct {
	pgx.Rows
	next, closed          bool
	body                  []byte
	scanErr, iterationErr error
}

func (r *frozenAttemptRows) Next() bool { v := r.next; r.next = false; return v }
func (r *frozenAttemptRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	*dest[0].(*[]byte) = r.body
	return nil
}
func (r *frozenAttemptRows) Err() error { return r.iterationErr }
func (r *frozenAttemptRows) Close()     { r.closed = true }

type frozenAttemptReadTx struct {
	pgx.Tx
	rows     [2]*frozenAttemptRows
	failures [2]error
	calls    int
}

func (tx *frozenAttemptReadTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	index := tx.calls
	tx.calls++
	return tx.rows[index], tx.failures[index]
}

// The first stream is the frozen receipt journal; the second is the execution
// journal. Neither an unreadable stream nor an unbound receipt can become evidence.
func TestFrozenAttemptReadRejectsIncompleteJournals(t *testing.T) {
	marker := errors.New("synthetic attempt journal failure")
	for _, name := range []string{"first query", "receipt decode", "receipt scan", "unbound receipt", "first iteration", "second query", "second scan", "second iteration"} {
		t.Run(name, func(t *testing.T) {
			first, second := &frozenAttemptRows{}, &frozenAttemptRows{}
			tx := &frozenAttemptReadTx{rows: [2]*frozenAttemptRows{first, second}}
			want := marker
			switch name {
			case "first query":
				tx.failures[0] = marker
			case "receipt decode":
				first.next = true
				first.body = []byte(`{`)
				want = store.ErrInvalid
			case "receipt scan":
				first.next = true
				first.scanErr = marker
				want = store.ErrInvalid
			case "unbound receipt":
				first.next = true
				first.body = []byte(`{}`)
				want = store.ErrInvalid
			case "first iteration":
				first.iterationErr = marker
			case "second query":
				tx.failures[1] = marker
			case "second scan":
				second.next = true
				second.scanErr = marker
			case "second iteration":
				second.iterationErr = marker
			}
			out, err := frozenAttemptsTx(context.Background(), tx, "tenant", frozenHead{})
			if !errors.Is(err, want) || len(out) != 0 {
				t.Fatal("incomplete journal exposed evidence", out, err)
			}
			if tx.failures[0] == nil && !first.closed {
				t.Fatal("first journal cursor leaked")
			}
			if tx.calls == 2 && tx.failures[1] == nil && !second.closed {
				t.Fatal("execution journal cursor leaked")
			}
		})
	}
}
