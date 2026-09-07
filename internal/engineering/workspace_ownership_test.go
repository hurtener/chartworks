package engineering

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// This protocol fixture checks proof ordering. Real workspace/DDL regressions
// live in test/acceptance and are not replaced by this unit test.
type ownershipRow func(...any) error

func (r ownershipRow) Scan(dest ...any) error { return r(dest...) }

type ownershipTransaction struct {
	pgx.Tx
	t         *testing.T
	rows      []pgx.Row
	calls     []string
	lockError error
}

func (x *ownershipTransaction) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	x.t.Helper()
	if len(args) != 2 || args[0] != "cw_schema" || args[1] != "cw_table" || len(x.rows) == 0 {
		x.t.Fatal("unexpected ownership query", args)
	}
	x.calls = append(x.calls, "probe")
	row := x.rows[0]
	x.rows = x.rows[1:]
	return row
}

func (x *ownershipTransaction) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	x.t.Helper()
	if sql != `LOCK TABLE "cw_schema"."cw_table" IN ACCESS EXCLUSIVE MODE` {
		x.t.Fatal("unexpected statement at ownership boundary", sql)
	}
	x.calls = append(x.calls, "lock")
	return pgconn.NewCommandTag("LOCK TABLE"), x.lockError
}

func ownershipSnapshot(oid int64, kind string, owned, unsafe bool) pgx.Row {
	return ownershipRow(func(dest ...any) error {
		*dest[0].(*int64) = oid
		*dest[1].(*string) = kind
		*dest[2].(*bool) = owned
		*dest[3].(*bool) = unsafe
		return nil
	})
}

func TestWorkspaceOwnershipRecheckedUnderDDLLock(t *testing.T) {
	good := func() pgx.Row { return ownershipSnapshot(41, "r", true, false) }
	for name, after := range map[string]pgx.Row{
		"replacement": ownershipSnapshot(42, "r", true, false),
		"owner":       ownershipSnapshot(41, "r", false, false),
		"unsafe":      ownershipSnapshot(41, "r", true, true),
		"view":        ownershipSnapshot(41, "v", true, false),
	} {
		t.Run(name, func(t *testing.T) {
			x := &ownershipTransaction{t: t, rows: []pgx.Row{good(), after}}
			if oid, err := ownedTable(context.Background(), x, "cw_schema", "cw_table", 41); oid != 0 || !errors.Is(err, ErrOwnership) {
				t.Fatal("changed object accepted after lock", oid, err)
			}
			if strings.Join(x.calls, ",") != "probe,lock,probe" {
				t.Fatal("ownership proof was not held under the DDL lock", x.calls)
			}
		})
	}
	x := &ownershipTransaction{t: t, rows: []pgx.Row{good(), good()}}
	if oid, err := ownedTable(context.Background(), x, "cw_schema", "cw_table", 0); err != nil || oid != 41 || strings.Join(x.calls, ",") != "probe,lock,probe" {
		t.Fatal("valid object not locked and re-proven", oid, err, x.calls)
	}
	x = &ownershipTransaction{t: t, rows: []pgx.Row{good()}}
	if _, err := ownedTable(context.Background(), x, "cw_schema", "cw_table", 42); !errors.Is(err, ErrOwnership) || len(x.calls) != 1 {
		t.Fatal("foreign object locked despite failed initial proof", err, x.calls)
	}
	x = &ownershipTransaction{t: t, rows: []pgx.Row{good()}, lockError: context.DeadlineExceeded}
	if _, err := ownedTable(context.Background(), x, "cw_schema", "cw_table", 41); !errors.Is(err, context.DeadlineExceeded) || strings.Join(x.calls, ",") != "probe,lock" {
		t.Fatal("failed lock did not stop ownership proof", err, x.calls)
	}
}
