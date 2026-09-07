package postgres

import (
	"strings"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestComparableManifestMigratesLegacyPostgresOnly(t *testing.T) {
	base := readexec.Manifest{Operation: "operation", Session: "session", Receipt: readexec.Receipt{Validated: true, Source: "source", Context: "context", Contract: "contract", Columns: []string{"value"}, Manifest: strings.Repeat("a", 64)}, Limits: readexec.Limits{Rows: 1, Bytes: 128, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1}}
	current := base
	current.Receipt.Dialect = "postgres"
	if comparableManifest(base) != comparableManifest(current) {
		t.Fatal("legacy PostgreSQL receipt changed logical operation identity")
	}
	current.Receipt.Dialect = "mysql"
	if comparableManifest(base) == comparableManifest(current) {
		t.Fatal("cross-driver receipt shared logical operation identity")
	}
}
