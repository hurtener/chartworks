package postgres

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec/querydiagnostic"
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
