package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/releaseprofile"
)

// This is a bounded AC03 prerequisite check, not the missing final_stress
// acceptance subtest. It reads a real native PostgreSQL source through the
// governed validator and proves changed rows alter the release witness.
func TestPhase25ReleaseDatasetProbe(t *testing.T) {
	f := newSourceFixture(t, nil)
	source := f.create(t, "release-probe")
	binding, err := f.s.Binding(context.Background(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	var datasetID string
	for _, relation := range binding.Relations {
		if relation.Name == "sales" {
			datasetID = relation.ID
		}
	}
	if datasetID == "" {
		t.Fatal("registered sales relation absent")
	}
	probe, err := releaseprofile.NewNativeDatasetProbe(f.validator, f.s)
	if err != nil {
		t.Fatal(err)
	}
	first, err := probe.ObserveDataset(t.Context(), f.e, source, []string{datasetID})
	if err != nil || first.Rows != 2 || len(first.Digest) != 64 || first.SourceRevision != source.Revision {
		t.Fatalf("native dataset evidence: %v, %+v", err, first)
	}
	if _, err := f.admin.Exec(t.Context(), "UPDATE analytics.sales SET amount=amount+1 WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	changed, err := probe.ObserveDataset(t.Context(), f.e, source, []string{datasetID})
	if err != nil || changed.Rows != first.Rows || changed.Digest == first.Digest {
		t.Fatalf("changed native rows did not invalidate evidence: %v, %+v", err, changed)
	}
	if _, err := probe.ObserveDataset(t.Context(), f.e, source, []string{datasetID, datasetID}); err == nil {
		t.Fatal("multi-dataset snapshot claimed without a shared source transaction")
	}
	withoutDataset := f.token.envelope(t, f.e.Tenant(), "operator", "sources.read", "sources.query", "cw.source.read:"+source.ID, "cw.source.query:"+source.ID, "cw.execution_context.use:"+source.ContextID)
	if _, err := probe.ObserveDataset(t.Context(), withoutDataset, source, []string{datasetID}); err == nil {
		t.Fatal("missing signed dataset reach produced evidence")
	}
	otherTenant := f.actor(t, "source-b", "operator")
	if _, err := probe.ObserveDataset(t.Context(), otherTenant, source, []string{datasetID}); err == nil {
		t.Fatal("cross-tenant source produced evidence")
	}
}
