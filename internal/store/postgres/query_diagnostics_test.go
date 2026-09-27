package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestSQLRecoveryDiagnosticMigrationVocabulary(t *testing.T) {
	manifest, err := Migrations()
	if err != nil || len(manifest) < 60 {
		t.Fatal("missing forward migration", err)
	}
	sql := manifest[59].SQL
	for _, code := range querydiagnostic.Codes() {
		if !strings.Contains(sql, "'"+code+"'") {
			t.Fatal("diagnostic cannot be journaled", code)
		}
	}
	if !strings.Contains(sql, "read_query_diagnostic_outcome") || !strings.Contains(sql, "rows_returned=0") || !strings.Contains(sql, "finished_at IS NOT NULL") || !strings.Contains(sql, "remote_state IN ('stopped','not_issued')") {
		t.Fatal("incoherent detailed diagnostic receipt")
	}
	if strings.Contains(sql, "UPDATE ") || strings.Contains(sql, "DROP TRIGGER") || !strings.Contains(manifest[57].SQL, "'query_error'") {
		t.Fatal("historical rows/immutable guard rewritten")
	}
}

// The metadata repository wraps both native EXPLAIN and read callbacks. A
// source-package-only test misses this second sanitizer on the actual path.
func TestSQLRecoveryDiagnosticMetadataBoundary(t *testing.T) {
	for _, code := range querydiagnostic.Codes() {
		t.Run(code, func(t *testing.T) {
			diagnosed := exec.QueryRejection(code)
			got := safe(fmt.Errorf("PRIVATE_WRAPPER: %w", diagnosed))
			if !errors.Is(got, exec.ErrQuery) || exec.QueryRejectionCode(got) != code {
				t.Fatal("scoped transaction discarded the admitted source diagnosis", code, got)
			}
			payload, err := json.Marshal(got)
			if err != nil || strings.Contains(string(payload)+fmt.Sprintf("%v %#v", got, got), "PRIVATE_") {
				t.Fatal("metadata sanitizer retained private wrapper/body", err)
			}
			for _, terminal := range []error{context.Canceled, context.DeadlineExceeded, exec.ErrUncertain, exec.ErrCancelled, exec.ErrTimeout, exec.ErrBinding, exec.ErrType, exec.ErrUnsafe, exec.ErrUnsupported, exec.ErrReplay, exec.ErrLimit, store.ErrUnavailable, store.ErrConflict, store.ErrInvalid, store.ErrNotFound, store.ErrMigration, store.ErrScope, store.ErrExpired, access.ErrForbidden, access.ErrUnauthenticated, access.ErrNotFound} {
				result := safe(errors.Join(diagnosed, terminal))
				if !errors.Is(result, terminal) || exec.QueryRejectionCode(result) != "" {
					t.Fatal("source diagnosis displaced metadata/terminal evidence", terminal, result)
				}
			}
		})
	}
	// The code itself does not classify raw errors that arose in metadata work.
	// Keep existing store classification and never promote these into SQL repair.
	for _, state := range []string{"22012", "22003", "22P02", "22007", "22008", "21000", "42883", "42725", "42804", "42P18", "42803", "42P20"} {
		got := safe(&pgconn.PgError{Code: state, Message: "PRIVATE_METADATA_MESSAGE", Detail: "PRIVATE_DETAIL", InternalQuery: "PRIVATE_QUERY"})
		if exec.QueryRejectionCode(got) != "" || strings.Contains(fmt.Sprintf("%v %#v", got, got), "PRIVATE_") {
			t.Fatal("metadata SQLSTATE/body became a query diagnosis", state, got)
		}
	}
	if safe(nil) != nil {
		t.Fatal("nil acquired a diagnosis")
	}
}
