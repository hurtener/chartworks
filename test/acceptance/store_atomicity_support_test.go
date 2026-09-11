package acceptance

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// storeWriteFault fails an actual PostgreSQL statement, not a repository mock.
// A sequence is deliberately used as the observation counter: unlike table
// writes, nextval survives the transaction rollback that these tests verify.
// Each caller owns a disposable database and uses faults serially.
func storeWriteFault(t *testing.T, raw *pgx.Conn, table, operation, condition string) func() {
	t.Helper()
	if operation != "INSERT" && operation != "UPDATE" && operation != "DELETE" {
		t.Fatal("test fault must name one SQL operation")
	}
	ctx := context.Background()
	_, err := raw.Exec(ctx, `CREATE SEQUENCE chartworks.test_store_fault_count;
CREATE FUNCTION chartworks.test_store_fault() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 PERFORM nextval('chartworks.test_store_fault_count');
 RAISE EXCEPTION 'synthetic storage failure: PRIVATE_FAULT_CANARY' USING ERRCODE='P0001';
END; $$;`)
	if err != nil {
		t.Fatal("install database fault", err)
	}
	when := ""
	if condition != "" {
		// Conditions are fixed expressions in the test cases, never request data.
		when = " WHEN (" + condition + ")"
	}
	name := pgx.Identifier{"chartworks", table}.Sanitize()
	_, err = raw.Exec(ctx, "CREATE TRIGGER test_store_fault BEFORE "+operation+" ON "+name+" FOR EACH ROW"+when+" EXECUTE FUNCTION chartworks.test_store_fault()")
	if err != nil {
		t.Fatal("attach database fault", err)
	}
	removed := false
	remove := func(requireHit bool) {
		t.Helper()
		if removed {
			return
		}
		removed = true
		if requireHit {
			var hit bool
			if err := raw.QueryRow(ctx, `SELECT is_called FROM chartworks.test_store_fault_count`).Scan(&hit); err != nil || !hit {
				t.Errorf("storage failure did not reach %s %s: hit=%v error=%v", operation, table, hit, err)
			}
		}
		if _, err := raw.Exec(ctx, "DROP TRIGGER test_store_fault ON "+name+`; DROP FUNCTION chartworks.test_store_fault(); DROP SEQUENCE chartworks.test_store_fault_count;`); err != nil {
			t.Error("remove database fault", err)
		}
	}
	t.Cleanup(func() { remove(false) })
	return func() { remove(true) }
}

// storeTableSnapshot includes complete rows, not just row counts. It catches
// changed pointers, leaked reservations, overwritten evidence, and extra rows.
func storeTableSnapshot(t *testing.T, raw *pgx.Conn, tables ...string) string {
	t.Helper()
	parts := make([]string, 0, len(tables))
	for _, table := range tables {
		var value string
		query := "SELECT COALESCE(jsonb_agg(to_jsonb(r) ORDER BY to_jsonb(r)::text),'[]'::jsonb)::text FROM " + pgx.Identifier{"chartworks", table}.Sanitize() + " r"
		if err := raw.QueryRow(context.Background(), query).Scan(&value); err != nil {
			t.Fatal("snapshot isolated test table", table, err)
		}
		parts = append(parts, fmt.Sprintf("%s=%s", table, value))
	}
	return strings.Join(parts, "\n")
}

// storeHideTable simulates a missing/unavailable metadata relation while keeping
// every row, foreign key and immutable-history trigger intact. It is used only
// in an isolated database, and restores the original relation before assertions.
func storeHideTable(t *testing.T, raw *pgx.Conn, table string) func() {
	t.Helper()
	ctx := context.Background()
	name := pgx.Identifier{"chartworks", table}.Sanitize()
	hidden := pgx.Identifier{"chartworks", "test_unavailable_" + table}.Sanitize()
	if _, err := raw.Exec(ctx, "ALTER TABLE "+name+" RENAME TO "+pgx.Identifier{"test_unavailable_" + table}.Sanitize()); err != nil {
		t.Fatal("hide owned fixture table", err)
	}
	restored := false
	restore := func() {
		if restored {
			return
		}
		restored = true
		if _, err := raw.Exec(ctx, "ALTER TABLE "+hidden+" RENAME TO "+pgx.Identifier{table}.Sanitize()); err != nil {
			t.Error("restore owned fixture table", err)
		}
	}
	t.Cleanup(restore)
	return restore
}
