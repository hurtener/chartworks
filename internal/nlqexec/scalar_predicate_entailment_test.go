package nlqexec

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestScalarPredicateEntailmentExpectedSuccess(t *testing.T) {
	for _, activity := range []bool{false, true} {
		a, constraints := scalarPredicateCharacterization(t, activity)
		c, err := compileAnalyticalVersion(t.Context(), a, 13, constraints)
		if err != nil || c == nil || c.Version != exec.AnalyticalScalarEntailmentVersion || c.ScalarPopulations == nil || c.ScalarEntailment == nil || len(c.ScalarEntailment.Witnesses) != 2 {
			t.Fatal("exact paid predicate on every selected scalar leaf must compile", activity, err)
		}
		for _, w := range c.ScalarEntailment.Witnesses {
			if w.Metric != c.Metrics[0].ID || w.Topic != a.publications[0].Definition.Topic || w.TopicVersion != a.publications[0].Definition.Version || w.Publication != a.publications[0].Digest || w.FilterOwnerKind != "measure" || w.FilterOwner != w.Measure {
				t.Fatal("semantic occurrence witness lost identity")
			}
			if w.Measure == "refunds" && (w.Relationship != "refund-parent" || w.RelationshipDigest == "" || w.JoinDigest == "") {
				t.Fatal("parent relationship identity omitted")
			}
		}
		before := exec.Hash(c)
		constraints[0].Value = "mutated"
		a.publications[0].Definition.Measures[0].Filters[0].Values[0] = "mutated"
		if exec.Hash(c) != before {
			t.Fatal("compiler proof aliases caller values")
		}
		if strings.Contains(analyticalGrainGuidance(c), "paid") || strings.Contains(analyticalGrainGuidance(c), "scalar_entailment") {
			t.Fatal("entailment values or proof reached generation guidance")
		}
	}
}

func resealScalarEntailment(a *admission) {
	analyticalReseal(a)
	for i := range a.metricPeriods {
		a.metricPeriods[i].PackDigest = a.publications[0].Digest
		a.metricPeriods[i].MappingDigest = exec.Hash(a.publications[0].Definition.KPIs[0].Periods)
	}
}

func TestScalarPredicateEntailmentClosedCompilerBoundaries(t *testing.T) {
	for name, change := range map[string]func(*admission, []exec.BusinessConstraint){
		"nonentailed": func(_ *admission, c []exec.BusinessConstraint) { c[0].Value = "unpaid" },
		"one_leaf": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Measures[0].Filters = nil
			resealScalarEntailment(a)
		},
		"singleton_in": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Measures[0].Filters[0].Operator = "in"
			resealScalarEntailment(a)
		},
		"wrong_fact":          func(_ *admission, c []exec.BusinessConstraint) { c[0].Dataset = "refunds" },
		"wrong_column":        func(_ *admission, c []exec.BusinessConstraint) { c[0].Column = "id_native" },
		"source":              func(a *admission, _ []exec.BusinessConstraint) { a.binding.Source = "other" },
		"context":             func(a *admission, _ []exec.BusinessConstraint) { a.binding.Context = "other" },
		"revision":            func(a *admission, _ []exec.BusinessConstraint) { a.binding.Revision++ },
		"constraint_revision": func(_ *admission, c []exec.BusinessConstraint) { c[0].SourceRevision++ },
		"evidence":            func(a *admission, _ []exec.BusinessConstraint) { a.route.Resolutions = nil },
		"relationship": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Measures[2].Filters[0].Relationship = "wrong"
			resealScalarEntailment(a)
		},
		"same_edge_excluded_relationship": func(a *admission, _ []exec.BusinessConstraint) {
			j := a.publications[0].Definition.Joins[0]
			j.ID = "other-parent"
			a.publications[0].Definition.Joins = append(a.publications[0].Definition.Joins, j)
			a.route.Request.JoinChoices = []nlqroute.JoinChoice{{Topic: a.publications[0].Definition.Topic, JoinID: "other-parent"}}
			resealScalarEntailment(a)
		},
		"left": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Joins[0].Type = semantics.JoinLeft
			resealScalarEntailment(a)
		},
		"partial_key": func(a *admission, _ []exec.BusinessConstraint) {
			a.publications[0].Definition.Joins[0].AdditionalKeys = nil
			resealScalarEntailment(a)
		},
		"missing_unique_current_source": func(a *admission, _ []exec.BusinessConstraint) {
			a.binding.Relations[0].UniqueKeys = nil
			a.route.SourceBindingDigest = exec.Hash(a.binding)
			a.metricPeriods[0].SourceBindingDigest = exec.Hash(a.binding)
		},
		"period_collision": func(a *admission, c []exec.BusinessConstraint) {
			c[0].Resolution = a.metricPeriods[0].Bindings[0].Constraint.Resolution
		},
		"aggregate_metadata": func(_ *admission, c []exec.BusinessConstraint) { c[0].Aggregation = "count" },
		"null":               func(_ *admission, c []exec.BusinessConstraint) { c[0].Null = true },
		"null_include":       func(_ *admission, c []exec.BusinessConstraint) { c[0].Nulls = "include" },
		"null_only": func(_ *admission, c []exec.BusinessConstraint) {
			c[0].Nulls = "only"
			c[0].Null = true
			c[0].Value = ""
		},
		"upper":         func(_ *admission, c []exec.BusinessConstraint) { c[0].Upper = "paid" },
		"bounds":        func(_ *admission, c []exec.BusinessConstraint) { c[0].Bounds = "[]" },
		"unit":          func(_ *admission, c []exec.BusinessConstraint) { c[0].Unit = "USD" },
		"precision":     func(_ *admission, c []exec.BusinessConstraint) { c[0].Precision = 1 },
		"scale":         func(_ *admission, c []exec.BusinessConstraint) { c[0].Scale = 1 },
		"temporal_type": func(_ *admission, c []exec.BusinessConstraint) { c[0].TemporalType = "date" },
		"calendar":      func(_ *admission, c []exec.BusinessConstraint) { c[0].Calendar = "gregorian" },
		"timezone":      func(_ *admission, c []exec.BusinessConstraint) { c[0].TimeZone = "UTC" },
		"grain":         func(_ *admission, c []exec.BusinessConstraint) { c[0].Grain = "year" },
		"not_equal":     func(_ *admission, c []exec.BusinessConstraint) { c[0].Operator = "ne" },
		"kind_entity":   func(_ *admission, c []exec.BusinessConstraint) { c[0].Kind = "entity" },
	} {
		t.Run(name, func(t *testing.T) {
			a, c := scalarPredicateCharacterization(t, true)
			change(&a, c)
			if got, err := compileAnalyticalVersion(t.Context(), a, 13, c); got != nil || err == nil {
				t.Fatal("unproved predicate compiled", err)
			}
		})
	}
	a, c := scalarPredicateCharacterization(t, false)
	second := c[0]
	second.Resolution = exec.Hash("unsupported-second")
	second.Value = "unpaid"
	c = append(c, second)
	if _, err := compileAnalyticalVersion(t.Context(), a, 13, c); err == nil {
		t.Fatal("unaccounted second constraint dropped")
	}
	a, c = scalarPredicateCharacterization(t, false)
	raw, _ := json.Marshal(a.route)
	_ = json.Unmarshal(raw, &a.route)
	if _, err := compileCurrentAnalytical(t.Context(), a); err == nil {
		t.Fatal("JSON manufactured current routing/application seal")
	}
	for _, version := range []int{8, 9, 10, 11, 12} {
		if _, err := compileAnalyticalVersion(t.Context(), a, version, c); err == nil {
			t.Fatal("old receipt family acquired entailment", version)
		}
	}
}

// A measure reused by two KPI paths is two obligations. Its inherited filters
// are scoped to the actual path, not merged under a global measure ID.
func TestScalarPredicateEntailmentInheritedOccurrences(t *testing.T) {
	for _, paidSecond := range []bool{true, false} {
		a, c := scalarPredicateCharacterization(t, false)
		d := &a.publications[0].Definition
		revenue := d.Measures[0].ID
		d.Measures[0].Filters = nil
		for _, id := range []string{"first_paid", "second_paid"} {
			filters := []semantics.SemanticFilter{{ID: "inherited_paid", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "region"}, Operator: "eq", Values: []string{"paid"}}}
			if id == "second_paid" && !paidSecond {
				filters = nil
			}
			d.KPIs = append(d.KPIs, semantics.KPI{ID: id, Expression: revenue, Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: revenue}}, Filters: filters})
		}
		d.KPIs[0].Expression = "first_paid + second_paid - refunds"
		d.KPIs[0].Inputs = []semantics.Reference{{Kind: semantics.KindKPI, ID: "first_paid"}, {Kind: semantics.KindKPI, ID: "second_paid"}, {Kind: semantics.KindMeasure, ID: "refunds"}}
		resealScalarEntailment(&a)
		got, err := compileAnalyticalVersion(t.Context(), a, 13, c)
		if !paidSecond {
			if got != nil || err == nil {
				t.Fatal("one unentailed inherited occurrence accepted")
			}
			continue
		}
		if err != nil || got.ScalarEntailment == nil || len(got.ScalarEntailment.Witnesses) != 3 {
			t.Fatal("complete inherited occurrence proof", err)
		}
		owners := map[string]bool{}
		for _, w := range got.ScalarEntailment.Witnesses {
			if w.Measure == revenue {
				owners[w.FilterOwner] = true
				if w.FilterOwnerKind != "kpi" {
					t.Fatal("inherited owner lost")
				}
			}
		}
		if !owners["first_paid"] || !owners["second_paid"] {
			t.Fatal("repeated measure collapsed")
		}
	}
}

func TestScalarPredicateEntailmentSelectedCountCoverage(t *testing.T) {
	for _, hasFilter := range []bool{true, false} {
		a, constraints := scalarPredicateCharacterization(t, false)
		d := &a.publications[0].Definition
		count := d.Measures[0]
		count.ID = "paid_count"
		count.Aggregation = semantics.AggregationCount
		count.Field = semantics.Reference{Kind: semantics.KindColumn, Dataset: "sales", ID: "id"}
		if !hasFilter {
			count.Filters = nil
		}
		d.Measures = append(d.Measures, count)
		periods := &semantics.MetricPeriodBindings{Policy: semantics.MetricPeriodBindingsPolicy, Bindings: []semantics.MetricPeriodBinding{{Measure: semantics.Reference{Kind: semantics.KindMeasure, ID: count.ID}, Dimension: semantics.Reference{Kind: semantics.KindDimension, ID: "sales-period"}}}}
		d.KPIs = append(d.KPIs, semantics.KPI{ID: "selected_count", Expression: count.ID, Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: count.ID}}, Periods: periods})
		a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindKPI, ID: "selected_count"}, Reason: "explicit_metric"})
		analyticalReseal(&a)
		a.metricPeriods[0].PackDigest = a.publications[0].Digest
		application := a.metricPeriods[0]
		application.KPI = "selected_count"
		application.MappingDigest = exec.Hash(periods)
		application.Bindings = nil
		for _, binding := range a.metricPeriods[0].Bindings {
			if binding.Population == "sales" {
				binding.Measure = periods.Bindings[0].Measure
				application.Bindings = append(application.Bindings, binding)
			}
		}
		a.metricPeriods = append(a.metricPeriods, application)
		got, err := compileAnalyticalVersion(t.Context(), a, 13, constraints)
		if !hasFilter {
			if got != nil || err == nil {
				t.Fatal("selected COUNT leaf borrowed SUM predicate")
			}
			continue
		}
		if err != nil || len(got.Metrics) != 2 || len(got.ScalarEntailment.Witnesses) != 3 {
			t.Fatal("selected count occurrence missing", err)
		}
		found := false
		for _, w := range got.ScalarEntailment.Witnesses {
			if w.Measure == count.ID {
				found = true
				if len(w.Path) != 0 || w.Metric != d.Topic+":kpi:selected_count" {
					t.Fatal("COUNT root identity lost")
				}
			}
		}
		if !found {
			t.Fatal("COUNT proof omitted")
		}
	}
}

func TestScalarPredicateEntailmentDistinguishesSameNamedFactField(t *testing.T) {
	a, constraints := scalarPredicateCharacterization(t, false)
	var sourceColumn semantics.Column
	for _, dataset := range a.publications[0].Definition.Datasets {
		if dataset.ID == "sales" {
			for _, column := range dataset.Columns {
				if column.ID == "region" {
					sourceColumn = column
				}
			}
		}
	}
	if sourceColumn.SourceName == "" {
		t.Fatal("missing source field")
	}
	for i := range a.publications[0].Definition.Datasets {
		dataset := &a.publications[0].Definition.Datasets[i]
		if dataset.ID == "refunds" {
			for _, column := range dataset.Columns {
				if column.ID == sourceColumn.ID {
					t.Fatal("fixture already has target field")
				}
			}
			dataset.Columns = append(dataset.Columns, sourceColumn)
		}
	}
	for i := range a.binding.Relations {
		if a.binding.Relations[i].ID == "refunds" {
			a.binding.Relations[i].Columns = append(a.binding.Relations[i].Columns, exec.Column{Name: sourceColumn.SourceName, NativeType: sourceColumn.NativeType, Category: sourceColumn.Category, Nullable: sourceColumn.Nullable, Safe: true})
		}
	}
	for i := range a.relationScope {
		if a.relationScope[i].Dataset == "refunds" {
			a.relationScope[i].Columns = append(a.relationScope[i].Columns, sourceColumn.SourceName)
		}
	}
	a.binding.Fingerprint = exec.Hash("scalar-source-with-two-real-region-fields")
	a.route.SourceBindingDigest = exec.Hash(a.binding)
	a.metricPeriods[0].SourceBindingDigest = exec.Hash(a.binding)
	resealScalarEntailment(&a)
	if _, err := compileAnalyticalVersion(t.Context(), a, 13, constraints); err != nil {
		t.Fatal("original exact field no longer entailed", err)
	}
	constraints[0].Dataset = "refunds"
	if err := exec.ValidateBusinessConstraints(a.binding, constraints); err != nil {
		t.Fatal("negative must have valid physical coordinates and type", err)
	}
	got, err := compileAnalyticalVersion(t.Context(), a, 13, constraints)
	detail, ok := err.(*exec.AnalyticalError)
	if got != nil || !ok || detail.Code != "analytical_scalar_predicate_not_entailed" {
		t.Fatal("same physical field name on a different fact borrowed entailment", err)
	}
}
