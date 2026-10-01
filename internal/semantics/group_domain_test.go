package semantics

import (
	"reflect"
	"testing"
)

func TestSQLRecoveryOrdinaryGroupDomainSemanticContract(t *testing.T) {
	base, mapping, _, bindings := portableFixture(t)
	p := base.Pack()
	p.GroupDomain = &GroupDomainPolicy{Policy: MetricGroupDomainPolicy, Domain: "qualifying_population"}
	model, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	p.GroupDomain.Domain = "mutated"
	if model.Pack().GroupDomain.Domain != "qualifying_population" {
		t.Fatal("reviewed policy aliased")
	}
	diff, err := DiffModels(base, model)
	if err != nil || !diff.MetadataChanged {
		t.Fatal("domain invisible to review", err)
	}
	exported, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportDraftCandidate(exported, bindings)
	if err != nil || !reflect.DeepEqual(imported.Pack().GroupDomain, model.Pack().GroupDomain) {
		t.Fatal("portable meaning lost", err)
	}
	exported.GroupDomain.Domain = "raw_source_groups"
	if imported.Pack().GroupDomain.Domain != "qualifying_population" {
		t.Fatal("import aliases portable policy")
	}
	for _, bad := range []*GroupDomainPolicy{{}, {Policy: "unknown", Domain: "qualifying_population"}, {Policy: MetricGroupDomainPolicy, Domain: "zero_fill"}} {
		p = model.Pack()
		p.GroupDomain = bad
		if _, err := Compile(p); err == nil {
			t.Fatal("unknown domain accepted")
		}
	}
}
