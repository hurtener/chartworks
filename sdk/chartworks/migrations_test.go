package chartworks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestMigrationMethodsUseTypedRoutes(t *testing.T) {
	var mu sync.Mutex
	paths := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := New(server.URL, server.Client(), func(context.Context) (string, error) { return "token", nil })
	if err != nil {
		t.Fatal(err)
	}
	manifest := MigrationManifest{Batch: "batch"}
	if _, err = client.DryRunMigration(t.Context(), MigrationDryRunRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.ImportMigration(t.Context(), MigrationImportRequest{Manifest: manifest}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.ResumeMigration(t.Context(), MigrationResumeRequest{Batch: "batch", Expected: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.ExportMigration(t.Context(), MigrationExportRequest{Batch: "batch", Limit: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.CutoverMigration(t.Context(), MigrationCutoverRequest{Batch: "batch", Route: "route", OperatorRef: "drill"}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.RollbackMigration(t.Context(), MigrationRollbackRequest{Cohort: "cohort", Expected: 1, OperatorRef: "drill"}); err != nil {
		t.Fatal(err)
	}
	if _, err = client.EraseMigration(t.Context(), MigrationEraseRequest{Batch: "batch", Limit: 1}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/v1/migrations/dry-runs", "/v1/migrations/imports", "/v1/migrations/resume", "/v1/migrations/exports", "/v1/migrations/cutovers", "/v1/migrations/rollbacks", "/v1/migrations/erasures"}
	if len(paths) != len(want) {
		t.Fatal(paths)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatal(paths)
		}
	}
}

func TestMigrationMethodsRejectInvalidCoordinatesBeforeTransport(t *testing.T) {
	client, err := New("https://example.test", nil, func(context.Context) (string, error) { return "token", nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.DryRunMigration(t.Context(), MigrationDryRunRequest{}); err == nil {
		t.Fatal("empty manifest accepted")
	}
	if _, err = client.ImportMigration(t.Context(), MigrationImportRequest{Manifest: MigrationManifest{Batch: "batch"}, Expected: -1}); err == nil {
		t.Fatal("negative import revision accepted")
	}
	if _, err = client.ResumeMigration(t.Context(), MigrationResumeRequest{Batch: "batch"}); err == nil {
		t.Fatal("zero resume revision accepted")
	}
	if _, err = client.ExportMigration(t.Context(), MigrationExportRequest{Batch: "batch"}); err == nil {
		t.Fatal("zero export limit accepted")
	}
	if _, err = client.CutoverMigration(t.Context(), MigrationCutoverRequest{Batch: "batch"}); err == nil {
		t.Fatal("missing route accepted")
	}
	if _, err = client.RollbackMigration(t.Context(), MigrationRollbackRequest{Cohort: "cohort"}); err == nil {
		t.Fatal("zero rollback generation accepted")
	}
	if _, err = client.EraseMigration(t.Context(), MigrationEraseRequest{Batch: "batch", Limit: 1001}); err == nil {
		t.Fatal("oversized erase accepted")
	}
}
