package semantics

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

func testPack() TopicPack {
	column := func(dataset, id string) Reference { return Reference{Kind: KindColumn, Dataset: dataset, ID: id} }
	return TopicPack{
		SchemaVersion: SchemaVersion,
		Topic:         "commerce",
		Version:       "commerce:v1",
		Name:          "Commerce",
		Description:   "Reviewed commerce definitions.",
		Datasets: []Dataset{
			{ID: "orders", Name: "Orders", Source: SourceReference{Source: "warehouse", Context: "warehouse:v3", Dataset: "orders", ProfileVersion: "orders-profile:v2", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 3}, Columns: []Column{
				{ID: "customer_key", SourceName: "customer_id", Name: "Customer", NativeType: "bigint", Category: "integer"},
				{ID: "amount", SourceName: "amount", Name: "Amount", NativeType: "numeric", Category: "decimal", Nullable: true},
			}},
			{ID: "customers", Name: "Customers", Source: SourceReference{Source: "warehouse", Context: "warehouse:v3", Dataset: "customers", ProfileVersion: "customers-profile:v1", ProfileDigest: strings.Repeat("b", 64), SourceRevision: 3}, Columns: []Column{
				{ID: "customer_key", SourceName: "id", Name: "Customer", NativeType: "bigint", Category: "integer"},
				{ID: "region", SourceName: "region", Name: "Region", NativeType: "text", Category: "text", Nullable: true},
			}},
		},
		Measures: []Measure{
			{ID: "order_count", Name: "Order count", Description: "Number of orders.", Field: column("orders", "customer_key"), Aggregation: AggregationCount, Unit: "orders"},
			{ID: "revenue", Name: "Revenue", Description: "Total order amount.", Field: column("orders", "amount"), Aggregation: AggregationSum, Unit: "currency"},
		},
		Dimensions: []Dimension{{ID: "customer_region", Name: "Customer region", Description: "Reviewed customer region.", Field: column("customers", "region"), Role: DimensionCategorical}},
		KPIs: []KPI{
			{ID: "average_order_value", Name: "Average order value", Description: "Revenue per order.", Expression: "revenue divided by order count", Inputs: []Reference{{Kind: KindMeasure, ID: "revenue"}, {Kind: KindMeasure, ID: "order_count"}}},
			{ID: "indexed_order_value", Name: "Indexed order value", Description: "Reviewed index over average order value.", Expression: "average order value divided by one hundred", Inputs: []Reference{{Kind: KindKPI, ID: "average_order_value"}}},
		},
		Joins:             []Join{{ID: "orders_customers", Name: "Orders to customers", Left: column("orders", "customer_key"), Right: column("customers", "customer_key"), Type: JoinInner, Cardinality: CardinalityManyToOne}},
		CanonicalEntities: []CanonicalEntity{{ID: "customer", Revision: 2, Name: "Customer", Aliases: []string{"Cliente", "Buyer"}, Keys: []Reference{column("orders", "customer_key"), column("customers", "customer_key")}}},
	}
}

func validationCode(t *testing.T, err error) ValidationCode {
	t.Helper()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid, got %v", err)
	}
	var typed *ValidationError
	if !errors.As(err, &typed) {
		t.Fatalf("expected typed validation error, got %T", err)
	}
	return typed.Code
}

func TestCompileCanonicalizesWithoutSharingCallerState(t *testing.T) {
	input := testPack()
	model, err := Compile(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.Digest()) != 64 || !model.Contains(Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}) {
		t.Fatalf("invalid compiled model: digest=%q", model.Digest())
	}
	if model.Contains(Reference{Kind: KindColumn, Dataset: "orders", ID: "customer_id"}) {
		t.Fatal("source/display name became a stable reference fallback")
	}

	input.Datasets[0].Columns[0].ID = "mutated"
	input.KPIs[0].Inputs[0].ID = "mutated"
	got := model.Pack()
	if got.Datasets[0].ID != "customers" || got.Datasets[1].Columns[0].ID != "amount" || got.KPIs[0].Inputs[0].ID != "order_count" {
		t.Fatalf("compile did not detach and sort input: %#v", got)
	}
	got.Datasets[0].Columns[0].ID = "also-mutated"
	if model.Pack().Datasets[0].Columns[0].ID == "also-mutated" {
		t.Fatal("Pack returned shared mutable state")
	}

	reordered := testPack()
	slices.Reverse(reordered.Datasets)
	slices.Reverse(reordered.Measures)
	slices.Reverse(reordered.KPIs[0].Inputs)
	slices.Reverse(reordered.CanonicalEntities[0].Aliases)
	again, err := Compile(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if again.Digest() != model.Digest() {
		t.Fatalf("canonical digest changed with input order: %s != %s", again.Digest(), model.Digest())
	}
	reordered = testPack()
	slices.Reverse(reordered.CanonicalEntities[0].Keys)
	different, err := Compile(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if different.Digest() == model.Digest() {
		t.Fatal("ordered composite canonical keys were treated as an unordered set")
	}
}

func TestCompileRejectsNonExactAndUnsafeReferences(t *testing.T) {
	tests := []struct {
		name string
		edit func(*TopicPack)
		code ValidationCode
	}{
		{"name_fallback", func(p *TopicPack) { p.Measures[0].Field.ID = "customer_id" }, CodeMissingReference},
		{"wrong_reference_kind", func(p *TopicPack) { p.Measures[0].Field = Reference{Kind: KindDataset, ID: "orders"} }, CodeInvalidReference},
		{"foreign_column", func(p *TopicPack) { p.Dimensions[0].Field.Dataset = "orders" }, CodeMissingReference},
		{"same_dataset_join", func(p *TopicPack) { p.Joins[0].Right = Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"} }, CodeInvalidReference},
		{"duplicate_column_id", func(p *TopicPack) { p.Datasets[0].Columns[1].ID = p.Datasets[0].Columns[0].ID }, CodeDuplicateID},
		{"duplicate_source_name", func(p *TopicPack) { p.Datasets[0].Columns[1].SourceName = p.Datasets[0].Columns[0].SourceName }, CodeDuplicateID},
		{"dataset_evidence_mismatch", func(p *TopicPack) { p.Datasets[0].Source.Dataset = "different" }, CodeEvidenceMismatch},
		{"missing_profile_digest", func(p *TopicPack) { p.Datasets[0].Source.ProfileDigest = "" }, CodeInvalidValue},
		{"cross_source_join", func(p *TopicPack) { p.Datasets[0].Source.Source = "other_warehouse" }, CodeEvidenceMismatch},
		{"cross_context_join", func(p *TopicPack) { p.Datasets[0].Source.Context = "warehouse:v4" }, CodeEvidenceMismatch},
		{"mixed_source_revision_join", func(p *TopicPack) { p.Datasets[0].Source.SourceRevision++ }, CodeEvidenceMismatch},
		{"duplicate_canonical_id_at_new_revision", func(p *TopicPack) {
			p.CanonicalEntities = append(p.CanonicalEntities, CanonicalEntity{ID: "customer", Revision: 3, Name: "Account", Keys: p.CanonicalEntities[0].Keys})
		}, CodeDuplicateID},
		{"duplicate_join_pair", func(p *TopicPack) {
			p.Joins = append(p.Joins, Join{ID: "duplicate_pair", Name: "Duplicate pair", Left: p.Joins[0].Right, Right: p.Joins[0].Left, Type: JoinLeft, Cardinality: CardinalityOneToMany})
		}, CodeInvalidReference},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pack := testPack()
			tc.edit(&pack)
			_, err := Compile(pack)
			if got := validationCode(t, err); got != tc.code {
				t.Fatalf("code=%q want %q", got, tc.code)
			}
		})
	}
}

func TestReferenceCoordinatesAreClosed(t *testing.T) {
	for _, ref := range []Reference{
		{},
		{Kind: "unknown", ID: "id"},
		{Kind: KindColumn, ID: "id"},
		{Kind: KindColumn, Dataset: "orders", ID: "id", Revision: 1},
		{Kind: KindMeasure, Dataset: "orders", ID: "id"},
		{Kind: KindMeasure, ID: "id", Revision: 1},
		{Kind: KindCanonicalEntity, ID: "id"},
		{Kind: KindCanonicalEntity, ID: "id", Revision: -1},
		{Kind: KindCanonicalEntity, ID: "id", Revision: 1 << 62},
		{Kind: KindCanonicalEntity, Dataset: "orders", ID: "id", Revision: 1},
	} {
		if ref.Valid() {
			t.Fatalf("accepted incomplete or ambiguous coordinates: %#v", ref)
		}
	}
}

func TestCompileBoundsAndOpaqueSourceNames(t *testing.T) {
	pack := testPack()
	pack.Datasets[0].Columns[0].SourceName = "Customer Account / Key"
	if _, err := Compile(pack); err != nil {
		t.Fatalf("compiler imposed a dialect-specific identifier grammar: %v", err)
	}

	pack = testPack()
	pack.Datasets = make([]Dataset, 33)
	if _, err := Compile(pack); validationCode(t, err) != CodeLimit {
		t.Fatalf("dataset bound: %v", err)
	}

	pack = testPack()
	pack.Measures = nil
	pack.KPIs = nil
	for i := 0; i < 300; i++ {
		pack.Measures = append(pack.Measures, Measure{ID: "measure_" + itoa(i), Name: "Measure", Description: strings.Repeat("a", 4096), Field: Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Aggregation: AggregationSum})
	}
	if _, err := Compile(pack); validationCode(t, err) != CodeLimit {
		t.Fatalf("serialized pack bound: %v", err)
	}
}

func TestCanonicalCompositeKeyOrderIsPreserved(t *testing.T) {
	pack := testPack()
	pack.CanonicalEntities[0].Keys = []Reference{
		{Kind: KindColumn, Dataset: "orders", ID: "customer_key"},
		{Kind: KindColumn, Dataset: "orders", ID: "amount"},
		{Kind: KindColumn, Dataset: "customers", ID: "customer_key"},
		{Kind: KindColumn, Dataset: "customers", ID: "region"},
	}
	model, err := Compile(pack)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := model.CanonicalKeys("customer", "orders")
	if err != nil || !slices.Equal(keys, pack.CanonicalEntities[0].Keys[:2]) {
		t.Fatalf("composite key order changed: %#v, %v", keys, err)
	}
	slices.Reverse(pack.CanonicalEntities[0].Keys[:2])
	changed, err := Compile(pack)
	if err != nil || changed.Digest() == model.Digest() {
		t.Fatalf("composite key reorder did not change digest: %v", err)
	}
}

func TestCompileRejectsKPIReferenceCycles(t *testing.T) {
	pack := testPack()
	pack.KPIs[0].Inputs = []Reference{{Kind: KindKPI, ID: "indexed_order_value"}}
	if got := validationCode(t, func() error { _, err := Compile(pack); return err }()); got != CodeReferenceCycle {
		t.Fatalf("code=%q", got)
	}

	pack = testPack()
	pack.KPIs[0].Inputs = []Reference{{Kind: KindKPI, ID: "missing"}}
	if got := validationCode(t, func() error { _, err := Compile(pack); return err }()); got != CodeMissingReference {
		t.Fatalf("code=%q", got)
	}
}

func TestCanonicalEntityResolutionUsesStableKeys(t *testing.T) {
	model, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	entity, err := model.ResolveCanonical("  CLIENTE  ")
	if err != nil || entity.ID != "customer" {
		t.Fatalf("resolution=%#v err=%v", entity, err)
	}
	if !model.Contains(entity.Reference()) || model.Contains(Reference{Kind: KindCanonicalEntity, ID: entity.ID, Revision: entity.Revision + 1}) {
		t.Fatal("canonical reference did not pin the exact registry revision")
	}
	keys, err := model.CanonicalKeys("buyer", "orders")
	if err != nil || len(keys) != 1 || keys[0] != (Reference{Kind: KindColumn, Dataset: "orders", ID: "customer_key"}) {
		t.Fatalf("keys=%#v err=%v", keys, err)
	}
	keys[0].ID = "mutated"
	again, err := model.CanonicalKeys("Customer", "orders")
	if err != nil || again[0].ID != "customer_key" {
		t.Fatal("canonical keys shared mutable state")
	}
	if _, err = model.ResolveCanonical("unknown"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown term error=%v", err)
	}

	pack := testPack()
	pack.CanonicalEntities = append(pack.CanonicalEntities, CanonicalEntity{ID: "buyer", Revision: 1, Name: "ＢＵＹＥＲ", Keys: []Reference{{Kind: KindColumn, Dataset: "orders", ID: "customer_key"}}})
	if got := validationCode(t, func() error { _, err := Compile(pack); return err }()); got != CodeAmbiguousTerm {
		t.Fatalf("code=%q", got)
	}
}

func TestCompiledModelConcurrentReuse(t *testing.T) {
	model, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	for i := 0; i < 32; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := 0; j < 100; j++ {
				pack := model.Pack()
				pack.Datasets[0].Columns[0].Name = "changed"
				entity, resolveErr := model.ResolveCanonical("customer")
				if resolveErr != nil || entity.ID != "customer" || model.Digest() == "" {
					t.Errorf("concurrent read failed: entity=%#v err=%v", entity, resolveErr)
					return
				}
			}
		}()
	}
	wait.Wait()
}
