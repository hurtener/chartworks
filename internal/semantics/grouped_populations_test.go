package semantics

import (
	"reflect"
	"testing"
)

func TestSQLRecoveryReviewedGroupedPopulationPolicy(t *testing.T) {
	p := testPack()
	p.GroupedPopulation = &GroupedPopulationPolicy{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"customers", "orders"}}
	model, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	p.GroupedPopulation.Datasets[0] = "mutated"
	if model.Pack().GroupedPopulation.Datasets[0] != "customers" {
		t.Fatal("policy not detached")
	}
	copy := model.Pack()
	copy.GroupedPopulation = nil
	before, err := Compile(copy)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := DiffModels(before, model)
	if err != nil || !diff.MetadataChanged {
		t.Fatal("policy change hidden from review", err)
	}
	for _, policy := range []*GroupedPopulationPolicy{
		{Policy: "zero-fill", Datasets: []string{"customers", "orders"}},
		{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"customers"}},
		{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"orders", "customers"}},
		{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"customers", "customers"}},
		{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"customers", "missing"}},
	} {
		p = model.Pack()
		p.GroupedPopulation = policy
		if _, err := Compile(p); err == nil {
			t.Fatal("invalid population policy accepted", policy)
		}
	}
}

func TestSQLRecoveryGroupedPolicyPortableRemapping(t *testing.T) {
	model, mapping, _, bindings := portableFixture(t)
	p := model.Pack()
	p.GroupedPopulation = &GroupedPopulationPolicy{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"customers", "orders"}}
	model, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	portable, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(portable.GroupedPopulation.Datasets, []string{"buyers", "purchases"}) {
		t.Fatal("source identifiers leaked")
	}
	imported, err := ImportDraftCandidate(portable, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported.Pack().GroupedPopulation.Datasets, []string{"buyers_data", "purchases_data"}) {
		t.Fatal("destination policy mapping lost")
	}
	if !reflect.DeepEqual(portable.GroupedPopulation.Datasets, []string{"buyers", "purchases"}) {
		t.Fatal("import mutated input")
	}
}

func TestSQLRecoveryGroupedDomainReviewAndPortability(t *testing.T) {
	base, mapping, _, bindings := portableFixture(t)
	p := base.Pack()
	p.GroupedPopulation = &GroupedPopulationPolicy{Policy: GroupedPopulationUnionPolicy, Datasets: []string{"customers", "orders"}, GroupDomains: []GroupedPopulationDomain{{Dataset: "customers", Domain: "raw_source_groups"}, {Dataset: "orders", Domain: "qualifying_population"}}}
	model, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	p.GroupedPopulation.GroupDomains[0].Domain = "mutated"
	if model.Pack().GroupedPopulation.GroupDomains[0].Domain != "raw_source_groups" {
		t.Fatal("domain alias mutated reviewed model")
	}
	exported, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportDraftCandidate(exported, bindings)
	if err != nil {
		t.Fatal(err)
	}
	want := []GroupedPopulationDomain{{Dataset: "buyers_data", Domain: "raw_source_groups"}, {Dataset: "purchases_data", Domain: "qualifying_population"}}
	if !reflect.DeepEqual(imported.Pack().GroupedPopulation.GroupDomains, want) {
		t.Fatal("domain mapping lost exact fact origin")
	}
	for _, domains := range [][]GroupedPopulationDomain{{{Dataset: "orders", Domain: "qualifying_population"}}, {{Dataset: "customers", Domain: "zero_fill"}, {Dataset: "orders", Domain: "qualifying_population"}}, {{Dataset: "customers", Domain: "raw_source_groups"}, {Dataset: "customers", Domain: "qualifying_population"}}} {
		p = model.Pack()
		p.GroupedPopulation.GroupDomains = domains
		if _, err := Compile(p); err == nil {
			t.Fatal("incomplete, foreign or unknown domain accepted")
		}
	}
}
