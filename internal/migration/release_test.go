package migration

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
)

func TestCurrentReleaseSourceRequiresActiveVerifiedCutover(t *testing.T) {
	adapter := &adapterTest{}
	service, _, actor := serviceTest(t, adapter)
	manifest := manifestTest("release")
	if _, err := service.CurrentReleaseSource(t.Context(), actor, manifest.Cohort, "source"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing cutover: %v", err)
	}
	batch, err := service.Import(t.Context(), actor, ImportRequest{Manifest: manifest})
	if err != nil || batch.State != "complete" {
		t.Fatalf("import: %v, %v", err, batch.State)
	}
	if _, err := service.Cutover(t.Context(), actor, CutoverRequest{Batch: batch.ID, Route: "release", PreviousRoute: "old", Expected: 0, OperatorRef: "operator"}); err != nil {
		t.Fatal(err)
	}
	got, err := service.CurrentReleaseSource(t.Context(), actor, manifest.Cohort, "source")
	if err != nil || got.SourceID != "source" || got.SourceRevision != 1 || got.ContextID != "source:v1" || got.Snapshot != manifest.SourceSnapshot || got.ManifestDigest != batch.Digest || got.Generation != 1 {
		t.Fatalf("active source projection: %v, %+v", err, got)
	}
	if _, err := service.CurrentReleaseSource(t.Context(), actor, manifest.Cohort, "missing"); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("unreachable source: %v", err)
	}
	noContext := actorTest(t, "migration.read", "sources.read", "cw.source.read:source")
	if _, err := service.CurrentReleaseSource(t.Context(), noContext, manifest.Cohort, "source"); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("unsigned context: %v", err)
	}
	if _, err := service.Rollback(t.Context(), actor, RollbackRequest{Cohort: manifest.Cohort, Expected: 1, OperatorRef: "operator", Effects: []string{"noted"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CurrentReleaseSource(t.Context(), actor, manifest.Cohort, "source"); !errors.Is(err, ErrNotReady) {
		t.Fatalf("rolled back cutover: %v", err)
	}
}
