package acceptance

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
)

// Independent manual authoring. This consumes only the profile scaffold and
// predeclared business vocabulary, never the recorded B response builder.
func pairedManualPack(t *testing.T, scaffold semantics.TopicPack, ids map[string]string, values []drafts.AuthoringValue) semantics.TopicPack {
	t.Helper()
	raw, _ := json.Marshal(scaffold)
	var p semantics.TopicPack
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	p.Topic, p.Version = "paired-manual-commerce", "manual-v1"
	p.GroupDomain = &semantics.GroupDomainPolicy{Policy: semantics.MetricGroupDomainPolicy, Domain: "qualifying_population"}
	p.Unresolved = nil
	col := func(table, field string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: ids[table], ID: field}
	}
	measure := func(table, field string) string {
		return semantics.GeneratedEntityID(semantics.EnhancementMeasure, ids[table], field)
	}
	dimension := func(table, field string) string {
		return semantics.GeneratedEntityID(semantics.EnhancementDimension, ids[table], field)
	}
	counter := func(table, field string) string { return drafts.GeneratedCountMeasureID(ids[table], field) }
	filter := func(table, name, relation string) semantics.SemanticFilter {
		return semantics.SemanticFilter{ID: name, Field: col(table, "status_code"), Operator: "eq", Values: []string{"P"}, Relationship: relation}
	}
	for di := range p.Datasets {
		d := &p.Datasets[di]
		table := ""
		for name, id := range ids {
			if id == d.ID {
				table = name
			}
		}
		for ci := range d.Columns {
			c := &d.Columns[ci]
			ref := col(table, c.ID)
			if c.ID == "private_note" {
				p.Unresolved = append(p.Unresolved, semantics.UnresolvedSemantic{ID: "private-note", Dataset: d.ID, Column: c.ID, Reason: "Sensitive free text is not an analytical field"})
				continue
			}
			c.SemanticRole = semantics.SemanticRoleAttribute
			if c.ID == "misleading_net_total_usd" || c.ID == "amount_usd" || c.ID == "line_total_usd" {
				c.SemanticRole = semantics.SemanticRoleMeasureInput
				m := semantics.Measure{ID: measure(table, c.ID), Name: table + " known amount", Description: "Sum known USD amounts at the source fact grain; unknown amounts remain unknown", Field: ref, Aggregation: semantics.AggregationSum, Unit: "USD"}
				if table == "orders" {
					m.Name = "Known paid booked amount"
					m.Aliases = []string{"importe bruto conocido de pedidos pagados"}
					c.Aliases = append([]string(nil), m.Aliases...)
					m.Filters = []semantics.SemanticFilter{filter("orders", "orders_population", "")}
					m.Completeness = &semantics.KnownAmountCompleteness{Policy: semantics.KnownAmountCompletenessPolicy, UnknownCount: semantics.Reference{Kind: semantics.KindKPI, ID: "unknown_order_amounts_in_scope"}}
				}
				if table == "refunds" {
					m.Filters = []semantics.SemanticFilter{filter("refunds", "refunds_population", ""), filter("orders", "paid_parent", "confirmed_refunds_to_orders")}
				}
				p.Measures = append(p.Measures, m)
			} else {
				d := semantics.Dimension{ID: dimension(table, c.ID), Name: table + " " + c.ID, Description: "Reviewed " + table + " " + c.ID, Field: ref, Role: semantics.DimensionCategorical}
				if strings.HasSuffix(c.ID, "_id") {
					c.SemanticRole = semantics.SemanticRoleFactKey
					d.Role = semantics.DimensionIdentifier
				}
				if c.ID == "ordered_at" || c.ID == "refunded_at" {
					c.SemanticRole = semantics.SemanticRoleEventTime
					d.Name = table + " event time"
					d.Role = semantics.DimensionTemporal
					d.Temporal = &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainQuarter, semantics.GrainYear}}
				}
				for _, v := range values {
					if v.Field == ref {
						d.Values = append(d.Values, semantics.GovernedValue{ID: v.ID, Value: v.Value, Aliases: v.Aliases, Sensitivity: v.Sensitivity, Provenance: semantics.ValueProvenance{Kind: "author_input", Evidence: readexec.Hash(v), Policy: "non_sensitive_text_exclude_null"}})
					}
				}
				p.Dimensions = append(p.Dimensions, d)
			}
		}
	}
	for _, spec := range []struct{ table, field, name, unit string }{{"orders", "order_id", "Paid order count", "orders"}, {"orders", "misleading_net_total_usd", "Known paid amount count", "orders"}, {"refunds", "refund_id", "Posted refund event count", "refunds"}, {"refunds", "amount_usd", "Known posted refund amount count", "refunds"}} {
		m := semantics.Measure{ID: counter(spec.table, spec.field), Name: spec.name, Description: "Count non-NULL values once per source fact row without identifier deduplication", Field: col(spec.table, spec.field), Aggregation: semantics.AggregationCount, Unit: spec.unit}
		if spec.table == "orders" {
			m.Filters = []semantics.SemanticFilter{filter("orders", "orders_paid_count", "")}
		} else {
			m.Filters = []semantics.SemanticFilter{filter("refunds", "posted_refund_count", ""), filter("orders", "paid_parent_count", "confirmed_refunds_to_orders")}
		}
		p.Measures = append(p.Measures, m)
	}
	for _, j := range []struct {
		left, key, right, rkey string
		kind                   semantics.JoinType
	}{{"orders", "customer_id", "customers", "customer_id", semantics.JoinLeft}, {"refunds", "order_id", "orders", "order_id", semantics.JoinInner}, {"order_lines", "order_id", "orders", "order_id", semantics.JoinLeft}} {
		name := j.left + "_to_" + j.right
		p.Joins = append(p.Joins, semantics.Join{ID: "confirmed_" + name, Name: name, Left: col(j.left, j.key), Right: col(j.right, j.rkey), AdditionalKeys: []semantics.JoinKeyPair{{Left: col(j.left, "division_id"), Right: col(j.right, "division_id")}}, Type: j.kind, Cardinality: semantics.CardinalityManyToOne, Evidence: semantics.RelationshipEvidence{ID: j.left + "_composite", LeftGrain: j.left + " composite identifier", RightGrain: j.right + " composite identifier", Provenance: "manual_author_business_input"}})
	}
	addKPI := func(id, name, left, right, leftAxis, rightAxis string) {
		k := semantics.KPI{ID: id, Name: name, Description: "Known-only arithmetic under the reviewed fact populations, with explicit unknown amount companions", Expression: left + " - " + right, Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: left}, {Kind: semantics.KindMeasure, ID: right}}}
		if leftAxis != "" {
			k.Periods = &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{{Measure: k.Inputs[0], Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: leftAxis}}, {Measure: k.Inputs[1], Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: rightAxis}}}}
		}
		p.KPIs = append(p.KPIs, k)
	}
	ot, rt := dimension("orders", "ordered_at"), dimension("refunds", "refunded_at")
	addKPI("known_cohort_net", "Order-cohort net known value", measure("orders", "misleading_net_total_usd"), measure("refunds", "amount_usd"), ot, ot)
	addKPI("known_activity_net", "Refund-activity net known value", measure("orders", "misleading_net_total_usd"), measure("refunds", "amount_usd"), ot, rt)
	addKPI("unknown_order_amounts_in_scope", "Unknown paid order amounts in the current gross scope", counter("orders", "order_id"), counter("orders", "misleading_net_total_usd"), "", "")
	addKPI("unknown_order_amounts", "Unknown paid order amount count", counter("orders", "order_id"), counter("orders", "misleading_net_total_usd"), ot, ot)
	addKPI("unknown_cohort_refund_amounts", "Unknown cohort refund amount count", counter("refunds", "refund_id"), counter("refunds", "amount_usd"), ot, ot)
	addKPI("unknown_activity_refund_amounts", "Unknown activity refund amount count", counter("refunds", "refund_id"), counter("refunds", "amount_usd"), rt, rt)
	reviewGeneratedAdversarialNet(t, p, ids)
	return p
}

// Drop prose and lifecycle identity only. Every enforced physical, population,
// temporal, completeness, vocabulary, relationship and KPI binding remains.
func pairedMeaning(p semantics.TopicPack) semantics.TopicPack {
	raw, _ := json.Marshal(p)
	_ = json.Unmarshal(raw, &p)
	p.Topic, p.Version, p.Name, p.Description = "equivalent", "v1", "same meaning", ""
	for i := range p.Unresolved {
		p.Unresolved[i].ID, p.Unresolved[i].Reason = "", ""
	}
	sort.Slice(p.Unresolved, func(i, j int) bool {
		if p.Unresolved[i].Dataset != p.Unresolved[j].Dataset {
			return p.Unresolved[i].Dataset < p.Unresolved[j].Dataset
		}
		return p.Unresolved[i].Column < p.Unresolved[j].Column
	})
	for i := range p.Measures {
		m := &p.Measures[i]
		m.Name, m.Description = "", ""
		m.Aliases = nil
		sort.Slice(m.Filters, func(i, j int) bool { return m.Filters[i].ID < m.Filters[j].ID })
	}
	for i := range p.Dimensions {
		d := &p.Dimensions[i]
		d.Name, d.Description = "", ""
		d.Aliases = nil
		sort.Slice(d.Values, func(i, j int) bool { return d.Values[i].ID < d.Values[j].ID })
	}
	for i := range p.KPIs {
		p.KPIs[i].Name, p.KPIs[i].Description = "", ""
		p.KPIs[i].Aliases = nil
	}
	for i := range p.Joins {
		p.Joins[i].Evidence.Provenance = "reviewed independently"
	}
	sort.Slice(p.Measures, func(i, j int) bool { return p.Measures[i].ID < p.Measures[j].ID })
	sort.Slice(p.Dimensions, func(i, j int) bool { return p.Dimensions[i].ID < p.Dimensions[j].ID })
	sort.Slice(p.KPIs, func(i, j int) bool { return p.KPIs[i].ID < p.KPIs[j].ID })
	sort.Slice(p.Joins, func(i, j int) bool { return p.Joins[i].ID < p.Joins[j].ID })
	return p
}

func pairedMeaningDigest(p semantics.TopicPack) string { return readexec.Hash(pairedMeaning(p)) }

func TestPairedMeaningComparisonControls(t *testing.T) {
	p := semantics.TopicPack{Unresolved: []semantics.UnresolvedSemantic{{ID: "old-id", Dataset: "orders", Column: "private_note", Reason: "private"}}, Measures: []semantics.Measure{{ID: "gross", Name: "Gross", Aggregation: semantics.AggregationSum}}}
	base := pairedMeaningDigest(p)
	copyPack := func() semantics.TopicPack {
		raw, _ := json.Marshal(p)
		var x semantics.TopicPack
		_ = json.Unmarshal(raw, &x)
		return x
	}
	prose := copyPack()
	prose.Unresolved[0].ID = "new-id"
	prose.Unresolved[0].Reason = "different explanation"
	prose.Measures[0].Name = "Other wording"
	if pairedMeaningDigest(prose) != base {
		t.Fatal("prose became enforcement equivalence")
	}
	different := copyPack()
	different.Unresolved[0].Column = "status_code"
	if pairedMeaningDigest(different) == base {
		t.Fatal("unresolved target drift ignored")
	}
	different = copyPack()
	different.Unresolved = nil
	if pairedMeaningDigest(different) == base {
		t.Fatal("unresolved coverage removal ignored")
	}
	different = copyPack()
	different.Measures[0].Aggregation = semantics.AggregationAverage
	if pairedMeaningDigest(different) == base {
		t.Fatal("aggregation change ignored")
	}
}
