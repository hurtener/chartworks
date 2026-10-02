package sources

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

func diagnosticPG(code string) *pgconn.PgError {
	return &pgconn.PgError{Code: code, Message: "PRIVATE_MESSAGE", Detail: "PRIVATE_DETAIL", Hint: "PRIVATE_HINT", Where: "PRIVATE_WHERE", SchemaName: "PRIVATE_SCHEMA", TableName: "PRIVATE_TABLE", ColumnName: "PRIVATE_COLUMN", ConstraintName: "PRIVATE_CONSTRAINT", InternalQuery: "PRIVATE_SQL", File: "PRIVATE_FILE", Routine: "PRIVATE_ROUTINE"}
}
func TestSQLRecoveryNativeDiagnosticBoundaries(t *testing.T) {
	for _, tc := range []struct{ state, code string }{{"22012", "query_division_by_zero"}, {"22003", "query_numeric_range"}, {"22P02", "query_invalid_text"}, {"22007", "query_invalid_datetime"}, {"22008", "query_datetime_range"}, {"21000", "query_cardinality"}, {"42883", "query_function_signature"}, {"42725", "query_function_signature"}, {"42804", "query_type_mismatch"}, {"42P18", "query_type_mismatch"}, {"42803", "query_grouping"}, {"42P20", "query_windowing"}} {
		for _, classify := range []func(context.Context, error) error{postgresQueryRejection, readFailure} {
			got := classify(context.Background(), fmt.Errorf("PRIVATE_WRAP: %w", diagnosticPG(tc.state)))
			if !errors.Is(got, readexec.ErrQuery) || readexec.QueryRejectionCode(got) != tc.code {
				t.Fatal("native diagnostic lost", tc.state, got)
			}
			got = safe(fmt.Errorf("PRIVATE_WRAP: %w", got))
			if readexec.QueryRejectionCode(got) != tc.code || strings.Contains(fmt.Sprintf("%v %#v", got, got), "PRIVATE_") {
				t.Fatal("metadata boundary leaked/discarded diagnostic")
			}
		}
	}
	// The same raw driver exception at a metadata boundary must not authorize SQL
	// repair. Only read/EXPLAIN sites call the native diagnostic classifier.
	if safe(diagnosticPG("42803")) != store.ErrUnavailable {
		t.Fatal("metadata error promoted into query repair")
	}
	if !errors.Is(readFailure(context.Background(), diagnosticPG("22023")), readexec.ErrQuery) || readexec.QueryRejectionCode(readFailure(context.Background(), diagnosticPG("22023"))) != "query_error" {
		t.Fatal("legacy data-error category changed")
	}
}
func TestSQLRecoveryNativeDiagnosticsNeverPromoteTerminal(t *testing.T) {
	for _, terminal := range []error{readexec.ErrUncertain, readexec.ErrCancelled, readexec.ErrTimeout, readexec.ErrBinding, readexec.ErrType, readexec.ErrLimit, readexec.ErrUnsupported, readexec.ErrUnsafe, readexec.ErrReplay, store.ErrUnavailable, store.ErrConflict, store.ErrInvalid, store.ErrNotFound, access.ErrForbidden, access.ErrUnauthenticated, access.ErrNotFound, context.Canceled, context.DeadlineExceeded} {
		for _, classify := range []func(context.Context, error) error{postgresQueryRejection, readFailure} {
			err := classify(context.Background(), errors.Join(diagnosticPG("42803"), terminal))
			if errors.Is(err, readexec.ErrQuery) || strings.Contains(err.Error(), "PRIVATE_") {
				t.Fatal("terminal cause promoted/leaked", terminal, err)
			}
		}
		if errors.Is(safe(errors.Join(readexec.QueryRejection("query_grouping"), terminal)), readexec.ErrQuery) {
			t.Fatal("safe discarded terminal evidence")
		}
	}
	for _, state := range []string{"42501", "42P01", "42703", "40001", "40P01", "08006", "53200", "53300", "57P01", "unknown"} {
		if err := postgresQueryRejection(context.Background(), diagnosticPG(state)); err != store.ErrUnavailable {
			t.Fatal("non-query failure reclassified", state, err)
		}
	}
	for _, state := range []string{"57014", "25P03", "25P04"} {
		if err := postgresQueryRejection(context.Background(), diagnosticPG(state)); err != readexec.ErrTimeout {
			t.Fatal("native timeout code lost", err)
		}
	}
	for _, ctx := range []context.Context{nil, func() context.Context { c, cancel := context.WithCancel(context.Background()); cancel(); return c }(), func() context.Context {
		c, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		cancel()
		return c
	}()} {
		if err := postgresQueryRejection(ctx, diagnosticPG("22012")); err == nil || errors.Is(err, readexec.ErrQuery) {
			t.Fatal("context failure ignored", err)
		}
	}
	if postgresQueryRejection(context.Background(), nil) != nil {
		t.Fatal("nil error acquired diagnostic")
	}
	if errors.Is(postgresQueryRejection(context.Background(), errors.New("division by zero 22012")), readexec.ErrQuery) {
		t.Fatal("driver text inspected")
	}
}
