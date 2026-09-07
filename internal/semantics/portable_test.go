package semantics

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func portableFixture(t *testing.T) (Model, []ExportDatasetSlots, PortablePack, DraftBindings) {
	t.Helper()
	model, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	mapping := []ExportDatasetSlots{
		{Dataset: "orders", Slot: "purchases", Columns: []ExportColumnSlot{{Column: "customer_key", Slot: "buyer_key"}, {Column: "amount", Slot: "value"}}},
		{Dataset: "customers", Slot: "buyers", Columns: []ExportColumnSlot{{Column: "customer_key", Slot: "key"}, {Column: "region", Slot: "area"}}},
	}
	portable, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	bindings := DraftBindings{Topic: "commerce", Version: "commerce:v2", Datasets: []ImportDatasetBinding{
		{Slot: "purchases", Source: SourceReference{Source: "destination", Context: "destination:v7", Dataset: "purchases_data", ProfileVersion: "purchases_profile:v1", ProfileDigest: strings.Repeat("c", 64), SourceRevision: 7}, Columns: []ImportColumnBinding{
			{Slot: "buyer_key", ID: "buyer_identifier", SourceName: "buyer_id", NativeType: "int64", Category: "integer"},
			{Slot: "value", ID: "total", SourceName: "transaction_total", NativeType: "decimal(12,2)", Category: "decimal", Nullable: true},
		}},
		{Slot: "buyers", Source: SourceReference{Source: "destination", Context: "destination:v7", Dataset: "buyers_data", ProfileVersion: "buyers_profile:v1", ProfileDigest: strings.Repeat("d", 64), SourceRevision: 7}, Columns: []ImportColumnBinding{
			{Slot: "key", ID: "buyer_identifier", SourceName: "buyer_id", NativeType: "int64", Category: "integer"},
			{Slot: "area", ID: "area_name", SourceName: "area", NativeType: "varchar(80)", Category: "text", Nullable: true},
		}},
	}}
	return model, mapping, portable, bindings
}

func TestPortableProjectionStructurallyExcludesInstallationCoordinates(t *testing.T) {
	model, mapping, portable, _ := portableFixture(t)
	raw, err := json.Marshal(portable)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"source"`, `"context"`, `"profile_version"`, `"profile_digest"`, `"source_revision"`, `"source_name"`, `"native_type"`, `"topic"`, `"version"`, `"actor"`, `"session"`, `"credentials"`, `"warehouse"`, `"warehouse:v3"`, `"customer_id"`, strings.Repeat("a", 64)} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("installation coordinate appeared in portable DTO: %s", forbidden)
		}
	}
	if portable.Datasets[0].Slot != "buyers" || portable.Measures[1].Field != (Reference{Kind: KindColumn, Dataset: "purchases", ID: "value"}) || portable.CanonicalEntities[0].Revision != 2 {
		t.Fatalf("logical mapping or canonical pin changed: %#v", portable)
	}
	slices.Reverse(mapping)
	slices.Reverse(mapping[0].Columns)
	again, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	againBytes, _ := json.Marshal(again)
	if string(raw) != string(againBytes) {
		t.Fatal("export depends on mapping input order")
	}
	portable.Measures[0].Name = "changed"
	portable.KPIs[0].Inputs[0].ID = "changed"
	portable.CanonicalEntities[0].Keys[0].ID = "changed"
	if model.Pack().Measures[0].Name == "changed" || model.Pack().KPIs[0].Inputs[0].ID == "changed" || model.Pack().CanonicalEntities[0].Keys[0].ID == "changed" {
		t.Fatal("export shares mutable model state")
	}
}

func TestPortableDraftRoundTripRemapsEveryColumnReference(t *testing.T) {
	_, _, portable, bindings := portableFixture(t)
	candidate, err := ImportDraftCandidate(portable, bindings)
	if err != nil {
		t.Fatal(err)
	}
	pack := candidate.Pack()
	if pack.Version != bindings.Version || pack.Datasets[0].Source.Source != "destination" || pack.Measures[1].Field != (Reference{Kind: KindColumn, Dataset: "purchases_data", ID: "total"}) || pack.Dimensions[0].Field != (Reference{Kind: KindColumn, Dataset: "buyers_data", ID: "area_name"}) {
		t.Fatalf("destination mapping was not complete: %#v", pack)
	}
	if pack.Joins[0].Left.Dataset != "purchases_data" || pack.Joins[0].Right.Dataset != "buyers_data" || pack.CanonicalEntities[0].Keys[0].Dataset != "purchases_data" || pack.CanonicalEntities[0].Keys[1].Dataset != "buyers_data" || pack.CanonicalEntities[0].Revision != 2 {
		t.Fatal("join/canonical references or key order changed")
	}
	if portable.Measures[1].Field.Dataset != "purchases" || portable.CanonicalEntities[0].Keys[0].Dataset != "purchases" {
		t.Fatal("import mutated the portable input")
	}
	bindings.Datasets[0].Columns[0].SourceName = "mutated"
	pack.Datasets[0].Columns[0].SourceName = "mutated"
	pack.CanonicalEntities[0].Keys[0].ID = "mutated"
	if candidate.Pack().Datasets[0].Columns[0].SourceName == "mutated" || candidate.Pack().CanonicalEntities[0].Keys[0].ID == "mutated" {
		t.Fatal("draft candidate shares caller-owned data")
	}

	compiled, err := Compile(candidate.Pack())
	if err != nil || compiled.Digest() != candidate.Digest() {
		t.Fatalf("candidate bypassed the semantic compiler: %v", err)
	}
	again, err := ExportPortable(compiled, []ExportDatasetSlots{
		{Dataset: "purchases_data", Slot: "purchases", Columns: []ExportColumnSlot{{Column: "buyer_identifier", Slot: "buyer_key"}, {Column: "total", Slot: "value"}}},
		{Dataset: "buyers_data", Slot: "buyers", Columns: []ExportColumnSlot{{Column: "buyer_identifier", Slot: "key"}, {Column: "area_name", Slot: "area"}}},
	})
	if err != nil || !reflect.DeepEqual(portable, again) {
		t.Fatalf("semantic meaning changed across mapped round trip: %v", err)
	}
}

func TestPortableExportRequiresCompleteExplicitMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func([]ExportDatasetSlots) []ExportDatasetSlots
	}{
		{"missing_dataset", func(m []ExportDatasetSlots) []ExportDatasetSlots { return m[:1] }},
		{"unknown_dataset", func(m []ExportDatasetSlots) []ExportDatasetSlots { m[0].Dataset = "unknown"; return m }},
		{"duplicate_dataset", func(m []ExportDatasetSlots) []ExportDatasetSlots { m[1].Dataset = m[0].Dataset; return m }},
		{"duplicate_dataset_slot", func(m []ExportDatasetSlots) []ExportDatasetSlots { m[1].Slot = m[0].Slot; return m }},
		{"invalid_slot", func(m []ExportDatasetSlots) []ExportDatasetSlots { m[0].Slot = "../invalid"; return m }},
		{"missing_column", func(m []ExportDatasetSlots) []ExportDatasetSlots { m[0].Columns = m[0].Columns[:1]; return m }},
		{"column_name_fallback", func(m []ExportDatasetSlots) []ExportDatasetSlots { m[0].Columns[0].Column = "customer_id"; return m }},
		{"duplicate_column", func(m []ExportDatasetSlots) []ExportDatasetSlots {
			m[0].Columns[1].Column = m[0].Columns[0].Column
			return m
		}},
		{"duplicate_column_slot", func(m []ExportDatasetSlots) []ExportDatasetSlots {
			m[0].Columns[1].Slot = m[0].Columns[0].Slot
			return m
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, mapping, _, _ := portableFixture(t)
			_, err := ExportPortable(model, tc.edit(mapping))
			validationCode(t, err)
		})
	}
	if _, err := ExportPortable(Model{}, nil); validationCode(t, err) != CodeEvidenceMismatch {
		t.Fatal(err)
	}
}

func TestPortableImportRejectsInexactBindingsAndInvalidSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*PortablePack, *DraftBindings)
	}{
		{"schema", func(p *PortablePack, b *DraftBindings) { p.SchemaVersion++ }},
		{"oversized_name", func(p *PortablePack, b *DraftBindings) { p.Name = strings.Repeat("x", 257) }},
		{"dataset_limit", func(p *PortablePack, b *DraftBindings) { p.Datasets = make([]PortableDataset, 33) }},
		{"column_limit", func(p *PortablePack, b *DraftBindings) { p.Datasets[0].Columns = make([]PortableColumn, 257) }},
		{"column_slot", func(p *PortablePack, b *DraftBindings) { p.Datasets[0].Columns[0].Slot = "../bad" }},
		{"dataset_name", func(p *PortablePack, b *DraftBindings) { p.Datasets[0].Name = "" }},
		{"empty_destination", func(p *PortablePack, b *DraftBindings) { b.Topic = "" }},
		{"missing_dataset_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets = b.Datasets[:1] }},
		{"missing_profile", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Source.ProfileVersion = "" }},
		{"duplicate_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets[1].Slot = b.Datasets[0].Slot }},
		{"duplicate_portable_dataset", func(p *PortablePack, b *DraftBindings) { p.Datasets[1] = p.Datasets[0] }},
		{"unknown_slot_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Slot = "unknown" }},
		{"missing_column_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Columns = b.Datasets[0].Columns[:1] }},
		{"invalid_column_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Columns[0].ID = "" }},
		{"duplicate_column_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Columns[1].Slot = b.Datasets[0].Columns[0].Slot }},
		{"duplicate_column_slot", func(p *PortablePack, b *DraftBindings) { p.Datasets[0].Columns[1] = p.Datasets[0].Columns[0] }},
		{"unknown_column_binding", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Columns[0].Slot = "missing" }},
		{"category_mismatch", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Columns[0].Category = "text" }},
		{"nullability_mismatch", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Columns[0].Nullable = true }},
		{"cross_context_join", func(p *PortablePack, b *DraftBindings) { b.Datasets[0].Source.Context = "destination:v8" }},
		{"duplicate_destination_dataset", func(p *PortablePack, b *DraftBindings) { b.Datasets[1].Source.Dataset = b.Datasets[0].Source.Dataset }},
		{"physical_name_duplicate", func(p *PortablePack, b *DraftBindings) {
			b.Datasets[0].Columns[1].SourceName = b.Datasets[0].Columns[0].SourceName
		}},
		{"unmapped_measure", func(p *PortablePack, b *DraftBindings) { p.Measures[0].Field.ID = "missing" }},
		{"unmapped_dimension", func(p *PortablePack, b *DraftBindings) { p.Dimensions[0].Field.ID = "missing" }},
		{"unmapped_join_left", func(p *PortablePack, b *DraftBindings) { p.Joins[0].Left.ID = "missing" }},
		{"unmapped_join_right", func(p *PortablePack, b *DraftBindings) { p.Joins[0].Right.ID = "missing" }},
		{"unmapped_canonical_key", func(p *PortablePack, b *DraftBindings) { p.CanonicalEntities[0].Keys[0].ID = "missing" }},
		{"invalid_reference", func(p *PortablePack, b *DraftBindings) { p.Measures[0].Field.Revision = 1 }},
		{"kpi_column_dependency", func(p *PortablePack, b *DraftBindings) {
			p.KPIs[0].Inputs[0] = Reference{Kind: KindColumn, Dataset: "purchases", ID: "value"}
		}},
		{"unmapped_kpi_column", func(p *PortablePack, b *DraftBindings) {
			p.KPIs[0].Inputs[0] = Reference{Kind: KindColumn, Dataset: "purchases", ID: "missing"}
		}},
		{"cyclic_kpi", func(p *PortablePack, b *DraftBindings) {
			p.KPIs[0].Inputs = []Reference{{Kind: KindKPI, ID: p.KPIs[0].ID}}
		}},
		{"invalid_canonical_revision", func(p *PortablePack, b *DraftBindings) { p.CanonicalEntities[0].Revision = 0 }},
		{"duplicate_canonical_revision", func(p *PortablePack, b *DraftBindings) {
			p.CanonicalEntities = append(p.CanonicalEntities, p.CanonicalEntities[0])
			p.CanonicalEntities[1].Revision++
		}},
		{"kpi_inputs_limit", func(p *PortablePack, b *DraftBindings) { p.KPIs[0].Inputs = make([]Reference, 33) }},
		{"canonical_keys_limit", func(p *PortablePack, b *DraftBindings) { p.CanonicalEntities[0].Keys = make([]Reference, 33) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, p, bindings := portableFixture(t)
			tc.edit(&p, &bindings)
			candidate, err := ImportDraftCandidate(p, bindings)
			validationCode(t, err)
			if candidate.Digest() != "" {
				t.Fatal("failed import exposed a compiled candidate")
			}
		})
	}
}

func TestPortableMappingConcurrentReuse(t *testing.T) {
	model, mapping, portable, bindings := portableFixture(t)
	candidate, err := ImportDraftCandidate(portable, bindings)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 20; j++ {
				exported, exportErr := ExportPortable(model, mapping)
				imported, importErr := ImportDraftCandidate(portable, bindings)
				if exportErr != nil || importErr != nil || imported.Digest() != candidate.Digest() {
					t.Errorf("concurrent mapping failed: %v / %v", exportErr, importErr)
					return
				}
				exported.Measures[0].Name = "mutated"
				pack := imported.Pack()
				pack.CanonicalEntities[0].Keys[0].ID = "mutated"
			}
		}()
	}
	wg.Wait()
}

func TestPortableImportBoundsSerializedAuthoringContent(t *testing.T) {
	_, _, portable, bindings := portableFixture(t)
	for i := 0; i < 256; i++ {
		portable.Measures = append(portable.Measures, Measure{ID: "measure_" + itoa(i), Name: "Measure", Description: strings.Repeat("a", 4096), Field: Reference{Kind: KindColumn, Dataset: "purchases", ID: "value"}, Aggregation: AggregationSum})
	}
	if _, err := ImportDraftCandidate(portable, bindings); validationCode(t, err) != CodeLimit {
		t.Fatalf("portable byte limit: %v", err)
	}
	_, _, portable, bindings = portableFixture(t)
	portable.Measures[0].Field.ID = strings.Repeat("x", 1024)
	if _, err := ImportDraftCandidate(portable, bindings); validationCode(t, err) != CodeInvalidReference {
		t.Fatalf("unbounded reference reached serialization: %v", err)
	}
}
