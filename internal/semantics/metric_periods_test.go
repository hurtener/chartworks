package semantics

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func metricPeriodPack() TopicPack {
	p := testPack()
	p.KPIs = p.KPIs[:1]
	for i := range p.Datasets {
		p.Datasets[i].Columns = append(p.Datasets[i].Columns, Column{ID: "event_at", SourceName: "event_at", Name: "Event time", NativeType: "timestamptz", Category: "timestamp"})
		p.Dimensions = append(p.Dimensions, Dimension{ID: p.Datasets[i].ID + "_time", Name: p.Datasets[i].Name + " time", Field: Reference{Kind: KindColumn, Dataset: p.Datasets[i].ID, ID: "event_at"}, Role: DimensionTemporal, Temporal: &TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []TimeGrain{GrainMonth, GrainYear}}})
	}
	p.KPIs[0].Periods = &MetricPeriodBindings{Policy: MetricPeriodBindingsPolicy, Bindings: []MetricPeriodBinding{{Measure: Reference{Kind: KindMeasure, ID: "revenue"}, Dimension: Reference{Kind: KindDimension, ID: "orders_time"}}, {Measure: Reference{Kind: KindMeasure, ID: "order_count"}, Dimension: Reference{Kind: KindDimension, ID: "orders_time"}}}}
	return p
}
func TestMetricPeriodsExactCoverageHashCloneAndReach(t *testing.T) {
	p := metricPeriodPack()
	model, err := Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	p.KPIs[0].Periods.Bindings[0].Dimension.ID = "changed"
	if model.Pack().KPIs[0].Periods.Bindings[0].Dimension.ID == "changed" {
		t.Fatal("input periods alias model")
	}
	again := model.Pack()
	slices.Reverse(again.KPIs[0].Periods.Bindings)
	same, err := Compile(again)
	if err != nil || same.Digest() != model.Digest() {
		t.Fatal("period order changes meaning", err)
	}
	out := model.Pack()
	out.KPIs[0].Periods.Bindings[0].Dimension.ID = "changed"
	if model.Pack().KPIs[0].Periods.Bindings[0].Dimension.ID == "changed" {
		t.Fatal("output periods alias model")
	}
	crossed := model.Pack()
	for i := range crossed.KPIs[0].Periods.Bindings {
		crossed.KPIs[0].Periods.Bindings[i].Dimension.ID = "customers_time"
	}
	different, err := Compile(crossed)
	if err != nil || different.Digest() == model.Digest() {
		t.Fatal("reviewed parent period did not affect digest", err)
	}
	graph := dependencyGraphPack(model.Pack())
	if !slices.Contains(graph[Reference{Kind: KindKPI, ID: model.Pack().KPIs[0].ID}], Reference{Kind: KindDimension, ID: "orders_time"}) {
		t.Fatal("period excluded from rule dependency graph")
	}
	if _, err := MutateEntities(model, "deleted-time", []EntityMutation{{Kind: KindDimension, ID: "orders_time", Operation: "delete"}}); err == nil {
		t.Fatal("deletion stranded reviewed period")
	}
}
func TestMetricPeriodsRejectMissingConflictingAndUnprovedMeaning(t *testing.T) {
	for name, mutate := range map[string]func(*TopicPack){
		"policy":          func(p *TopicPack) { p.KPIs[0].Periods.Policy = "other" },
		"missing":         func(p *TopicPack) { p.KPIs[0].Periods.Bindings = p.KPIs[0].Periods.Bindings[:1] },
		"duplicate":       func(p *TopicPack) { p.KPIs[0].Periods.Bindings[1] = p.KPIs[0].Periods.Bindings[0] },
		"foreign measure": func(p *TopicPack) { p.KPIs[0].Periods.Bindings[0].Measure.ID = "foreign" },
		"raw column": func(p *TopicPack) {
			p.KPIs[0].Periods.Bindings[0].Dimension = Reference{Kind: KindColumn, Dataset: "orders", ID: "event_at"}
		},
		"categorical":    func(p *TopicPack) { p.KPIs[0].Periods.Bindings[0].Dimension.ID = "customer_region" },
		"same fact axes": func(p *TopicPack) { p.KPIs[0].Periods.Bindings[0].Dimension.ID = "customers_time" },
		"unconfirmed reach": func(p *TopicPack) {
			p.Joins = nil
			for i := range p.KPIs[0].Periods.Bindings {
				p.KPIs[0].Periods.Bindings[i].Dimension.ID = "customers_time"
			}
		},
		"multiplying reach": func(p *TopicPack) {
			p.Joins[0].Cardinality = CardinalityOneToMany
			for i := range p.KPIs[0].Periods.Bindings {
				p.KPIs[0].Periods.Bindings[i].Dimension.ID = "customers_time"
			}
		},
		"budget": func(p *TopicPack) { p.KPIs[0].Periods.Bindings = make([]MetricPeriodBinding, 5) },
		"nested override": func(p *TopicPack) {
			child := p.KPIs[0]
			parent := KPI{ID: "parent", Name: "Parent", Expression: "a", Inputs: []Reference{{Kind: KindKPI, ID: child.ID}}, Periods: cloneMetricPeriods(child.Periods)}
			for i := range parent.Periods.Bindings {
				parent.Periods.Bindings[i].Dimension.ID = "customers_time"
			}
			p.KPIs = append(p.KPIs, parent)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := metricPeriodPack()
			mutate(&p)
			if _, err := Compile(p); err == nil {
				t.Fatal("invalid period metadata admitted")
			}
		})
	}
	// Nil means legacy/unresolved time basis, not permission to guess one.
	p := metricPeriodPack()
	p.KPIs[0].Periods = nil
	if _, err := Compile(p); err != nil {
		t.Fatal("legacy optional periods rejected", err)
	}
}
func TestMetricPeriodsPortableRoundTrip(t *testing.T) {
	model, err := Compile(metricPeriodPack())
	if err != nil {
		t.Fatal(err)
	}
	mapping := []ExportDatasetSlots{}
	bindings := DraftBindings{Topic: "imported", Version: "v1"}
	for _, d := range model.Pack().Datasets {
		m := ExportDatasetSlots{Dataset: d.ID, Slot: d.ID + "_slot"}
		b := ImportDatasetBinding{Slot: m.Slot, Source: SourceReference{Source: "new-source", Context: "new-context", Dataset: d.ID + "_new", ProfileVersion: d.ID + "_profile", ProfileDigest: strings.Repeat("c", 64), SourceRevision: 1}}
		for _, c := range d.Columns {
			m.Columns = append(m.Columns, ExportColumnSlot{Column: c.ID, Slot: c.ID})
			b.Columns = append(b.Columns, ImportColumnBinding{Slot: c.ID, ID: c.ID, SourceName: c.SourceName, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable})
		}
		mapping = append(mapping, m)
		bindings.Datasets = append(bindings.Datasets, b)
	}
	portable, err := ExportPortable(model, mapping)
	if err != nil {
		t.Fatal(err)
	}
	imported, err := ImportDraftCandidate(portable, bindings)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported.Pack().KPIs[0].Periods, model.Pack().KPIs[0].Periods) {
		t.Fatal("period entity references lost on import")
	}
	portable.KPIs[0].Periods.Bindings[0].Dimension.ID = "changed"
	if imported.Pack().KPIs[0].Periods.Bindings[0].Dimension.ID == "changed" {
		t.Fatal("portable periods shared state")
	}
}
