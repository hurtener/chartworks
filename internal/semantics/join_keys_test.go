package semantics

import (
	"encoding/json"
	"reflect"
	"testing"
)

func compositePack() TopicPack {
	p := testPack()
	for i := range p.Datasets {
		p.Datasets[i].Columns = append(p.Datasets[i].Columns, Column{ID: "realm", SourceName: "realm_id", Name: "Realm", NativeType: "bigint", Category: "integer"})
	}
	p.Joins[0].AdditionalKeys = []JoinKeyPair{{Left: Reference{Kind: KindColumn, Dataset: "orders", ID: "realm"}, Right: Reference{Kind: KindColumn, Dataset: "customers", ID: "realm"}}}
	return p
}
func TestSQLRecoveryCompositeJoinCatalog(t *testing.T) {
	p := compositePack()
	m, e := Compile(p)
	if e != nil {
		t.Fatal(e)
	}
	reordered := compositePack()
	j := &reordered.Joins[0]
	first := JoinKeyPair{Left: j.Left, Right: j.Right}
	j.Left, j.Right = j.AdditionalKeys[0].Left, j.AdditionalKeys[0].Right
	j.AdditionalKeys[0] = first
	other, err := Compile(reordered)
	if err != nil || other.Digest() != m.Digest() {
		t.Fatal("conjunction order changed reviewed meaning", err)
	}
	if len(m.Pack().Joins[0].References()) != 4 {
		t.Fatal("missing closure")
	}
	p.Joins[0].AdditionalKeys[0].Left.ID = "changed"
	if m.Pack().Joins[0].AdditionalKeys[0].Left.ID != "realm" {
		t.Fatal("caller alias")
	}
	got := m.Pack()
	got.Joins[0].AdditionalKeys[0].Right.ID = "changed"
	if m.Pack().Joins[0].AdditionalKeys[0].Right.ID != "realm" {
		t.Fatal("returned alias")
	}
	for _, kind := range []string{"duplicate", "foreign", "missing", "oversized", "partial_duplicate"} {
		t.Run(kind, func(t *testing.T) {
			p := compositePack()
			switch kind {
			case "duplicate":
				p.Joins[0].AdditionalKeys[0] = JoinKeyPair{Left: p.Joins[0].Left, Right: p.Joins[0].Right}
			case "foreign":
				p.Joins[0].AdditionalKeys[0].Left.Dataset = "customers"
			case "missing":
				p.Joins[0].AdditionalKeys[0].Left.ID = "unknown"
			case "oversized":
				p.Joins[0].AdditionalKeys = make([]JoinKeyPair, 16)
			case "partial_duplicate":
				p.Joins[0].AdditionalKeys[0].Left = p.Joins[0].Left
			}
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid key accepted")
			}
		})
	}
	old, e := Compile(testPack())
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(old.Pack())
	var round TopicPack
	if json.Unmarshal(raw, &round) != nil {
		t.Fatal("wire")
	}
	again, e := Compile(round)
	if e != nil || old.Digest() != again.Digest() {
		t.Fatal("legacy identity changed", e)
	}
}
func TestSQLRecoveryCompositeJoinPortable(t *testing.T) {
	_, mapping, _, bindings := portableFixture(t)
	p := compositePack()
	m, e := Compile(p)
	if e != nil {
		t.Fatal(e)
	}
	for i := range mapping {
		mapping[i].Columns = append(mapping[i].Columns, ExportColumnSlot{Column: "realm", Slot: "scope"})
	}
	for i := range bindings.Datasets {
		bindings.Datasets[i].Columns = append(bindings.Datasets[i].Columns, ImportColumnBinding{Slot: "scope", ID: "realm_key", SourceName: "realm", NativeType: "bigint", Category: "integer"})
	}
	portable, e := ExportPortable(m, mapping)
	if e != nil {
		t.Fatal(e)
	}
	candidate, e := ImportDraftCandidate(portable, bindings)
	if e != nil {
		t.Fatal(e)
	}
	keys := candidate.Pack().Joins[0].AdditionalKeys
	want := []JoinKeyPair{{Left: Reference{Kind: KindColumn, Dataset: "purchases_data", ID: "realm_key"}, Right: Reference{Kind: KindColumn, Dataset: "buyers_data", ID: "realm_key"}}}
	if !reflect.DeepEqual(keys, want) {
		t.Fatal("partial remapping", keys)
	}
	portable.Joins[0].AdditionalKeys[0].Left.ID = "missing"
	if _, e := ImportDraftCandidate(portable, bindings); e == nil {
		t.Fatal("missing additional binding accepted")
	}
}

func TestSQLRecoveryCompositeJoinReplacementAndRuleClosure(t *testing.T) {
	m, e := Compile(compositePack())
	if e != nil {
		t.Fatal(e)
	}
	p := m.Pack()
	var old Dataset
	for _, d := range p.Datasets {
		if d.ID == "orders" {
			old = d
		}
	}
	source := old.Source
	source.Dataset = "new_orders"
	source.ProfileVersion = "new-profile"
	updated, e := ReplaceDataset(m, "v2", "orders", DatasetReplacement{Dataset: "new_orders", Source: source, Columns: old.Columns})
	if e != nil {
		t.Fatal(e)
	}
	j := updated.Pack().Joins[0]
	if j.AdditionalKeys[0].Left.Dataset != "new_orders" {
		t.Fatal("additional reference not rebound")
	}
	graph := dependencyGraphPack(updated.Pack())
	if len(graph[Reference{Kind: KindJoin, ID: j.ID}]) != 4 {
		t.Fatal("rule closure lost composite key")
	}
}
