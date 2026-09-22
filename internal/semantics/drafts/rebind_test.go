package drafts

import (
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestRebindColumnPreservesSensitivityOnlyForExactEvidenceIdentity(t *testing.T) {
	old := semantics.Column{ID: "region", SourceName: "region_code", Name: "Region", Sensitivity: semantics.LiteralNonSensitive}
	physical := readexec.Column{Name: "region_code", NativeType: "text", Category: "text", Safe: true}
	prior := semantics.SourceReference{Source: "warehouse", Context: "context"}
	target := semantics.SourceReference{Source: "warehouse", Context: "context"}
	if got := rebindColumn(old, physical, prior, target); got.Sensitivity != semantics.LiteralNonSensitive || got.ID != old.ID {
		t.Fatalf("exact evidence identity lost reviewed classification: %#v", got)
	}
	physical.Name = "new_region_code"
	if got := rebindColumn(old, physical, prior, target); got.Sensitivity != "" || got.ID != old.ID {
		t.Fatalf("renamed destination inherited stale classification: %#v", got)
	}
	physical.Name = old.SourceName
	target.Context = "other_context"
	if got := rebindColumn(old, physical, prior, target); got.Sensitivity != "" {
		t.Fatalf("cross-context destination inherited stale classification: %#v", got)
	}
}
