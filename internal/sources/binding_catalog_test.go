package sources

import (
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestObservedBindingCatalogCompatibility(t *testing.T) {
	stored := readexec.Binding{
		Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1,
		Dialect: "snowflake", Contract: "contract", Fingerprint: readexec.Hash("native-catalog-evidence"),
		Relations: []readexec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []readexec.Column{{Name: "id", NativeType: "number", Safe: true}}}},
	}
	observed := stored.Clone()
	observed.Catalog = "database"
	if !observedBindingMatches(stored, observed) {
		t.Fatal("legacy binding did not tolerate the additive catalog field")
	}
	changedEvidence := observed.Clone()
	changedEvidence.Fingerprint = readexec.Hash("different-native-catalog-evidence")
	if observedBindingMatches(stored, changedEvidence) {
		t.Fatal("legacy binding ignored changed native catalog evidence")
	}
	stored.Catalog = "database"
	changedCatalog := observed.Clone()
	changedCatalog.Catalog = "foreign"
	if observedBindingMatches(stored, changedCatalog) {
		t.Fatal("catalog-bearing binding accepted a different catalog")
	}
}
