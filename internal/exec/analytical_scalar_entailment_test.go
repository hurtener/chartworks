package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func scalarEntailmentFixture(t *testing.T) (Plan, AnalyticalContract, string) {
	t.Helper()
	p, c, sql := scopedPopulationFixture(t)
	constraint := BusinessConstraint{Resolution: Hash("explicit-paid-selection"), Dataset: "sales", Column: "name", SourceRevision: p.candidate.binding.Revision, Kind: "text", Operator: "eq", Nulls: "exclude", Value: "paid"}
	witnesses := []AnalyticalScalarPredicateWitness{}
	for i, leaf := range c.Metrics[0].Expression.Args {
		witness := AnalyticalScalarPredicateWitness{Resolution: constraint.Resolution, Metric: c.Metrics[0].ID, Path: []int{i}, Topic: "topic", TopicVersion: "v1", Publication: Hash("publication"), Measure: []string{"gross", "refunds"}[i], Filter: "paid", FilterOwnerKind: "measure", FilterOwner: []string{"gross", "refunds"}[i], LeafDigest: Hash(leaf), FilterDigest: Hash(leaf.Filters[0])}
		if i == 1 {
			witness.Relationship = "refund_parent"
			witness.RelationshipDigest = Hash("reviewed-refund-parent")
			witness.JoinDigest = Hash(c.ScalarPopulations.Lanes[0].Joins[0])
		}
		witnesses = append(witnesses, witness)
	}
	var err error
	c.ScalarEntailment, err = NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, c, []BusinessConstraint{constraint}, witnesses)
	if err != nil {
		t.Fatal(err)
	}
	c.Version = AnalyticalScalarEntailmentVersion
	return p, c, sql
}

func scalarEntailmentClone(t *testing.T, c AnalyticalContract) AnalyticalContract {
	t.Helper()
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out AnalyticalContract
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestScalarEntailmentBindingPreservesV9SQLAndParameters(t *testing.T) {
	p, c, sql := scalarEntailmentFixture(t)
	before := Hash(c)
	base := c
	base.Version = AnalyticalScopedPopulationsVersion
	base.ScalarEntailment = nil
	legacy, err := BindScalarPopulationConstraints(t.Context(), p.candidate.binding, sql, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindScalarEntailmentConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	if bound.SQL != legacy.SQL || !reflect.DeepEqual(bound.Parameters, legacy.Parameters) || !reflect.DeepEqual(bound.Receipt.Bindings, legacy.Receipt.Bindings) || Hash(c) != before {
		t.Fatal("entailed predicate changed period SQL, values, placement, or caller contract")
	}
	r := bound.Receipt
	if r.SchemaVersion != 6 || r.PopulationPolicy != AnalyticalScalarEntailmentPolicy || r.SourceBinding != Hash(p.candidate.binding) || r.Constraints != Hash([]any{c.ScalarPopulations, c.ScalarEntailment}) || r.Statement != Hash([]any{bound.SQL, bound.Parameters}) || r.Validation != nil || len(r.Entailments) != 1 {
		t.Fatal("invalid schema-six binding evidence")
	}
	effect := r.Entailments[0]
	constraint := c.ScalarEntailment.Constraints[0]
	want := Hash([]any{c.Binding, c.Semantics, c.Dataset, c.Metrics, c.ScalarPopulations, constraint, c.ScalarEntailment.Witnesses})
	if effect.Kind != "entailed_scalar_predicate" || effect.Resolution != constraint.Resolution || effect.Coverage != want || effect.Occurrences != 2 || effect.Parameters == nil || len(effect.Parameters) != 0 {
		t.Fatal("invalid closed discharge effect")
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	proof, err := CheckAnalyticalPlan(t.Context(), p, c)
	if err != nil {
		t.Fatal(err)
	}
	if proof.Version != AnalyticalScalarEntailmentVersion || proof.Contract != before || proof.ScalarEntailment != AnalyticalScalarEntailmentPolicy || proof.ScalarEntailmentCoverage != Hash(c.ScalarEntailment) || !strings.HasSuffix(proof.Scope, ";independent_entailed_scoped_singleton_populations") || !AnalyticalOutputsValid(proof) || len(proof.Outputs) != 1 || proof.Outputs[0].Column != 0 {
		t.Fatal("v13 receipt lost scope, coverage, or output ordinal")
	}
	for _, mutate := range []func(*Plan){
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "o.name='paid'", "o.name='unpaid'", 1)
		},
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "s.name='paid'", "s.name='unpaid'", 1)
		},
		func(p *Plan) {
			p.candidate.statement = strings.Replace(p.candidate.statement, "i.division=o.division AND ", "", 1)
		},
	} {
		bad := p
		mutate(&bad)
		if _, err := CheckAnalyticalPlan(t.Context(), bad, c); err == nil {
			t.Fatal("changed native population admitted")
		}
	}
}

func TestScalarEntailmentRejectsIncompleteOrInexactCoverage(t *testing.T) {
	p, original, _ := scalarEntailmentFixture(t)
	cases := map[string]func(*AnalyticalContract){
		"empty_proof":        func(c *AnalyticalContract) { c.ScalarEntailment = nil },
		"wrong_policy":       func(c *AnalyticalContract) { c.ScalarEntailment.Policy = "unreviewed" },
		"missing_constraint": func(c *AnalyticalContract) { c.ScalarEntailment.Constraints = nil },
		"duplicate_constraint": func(c *AnalyticalContract) {
			c.ScalarEntailment.Constraints = append(c.ScalarEntailment.Constraints, c.ScalarEntailment.Constraints[0])
		},
		"missing_witness": func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses = c.ScalarEntailment.Witnesses[:1] },
		"extra_witness": func(c *AnalyticalContract) {
			c.ScalarEntailment.Witnesses = append(c.ScalarEntailment.Witnesses, c.ScalarEntailment.Witnesses[0])
		},
		"duplicate_path": func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[1].Path = []int{0} },
		"reordered_witnesses": func(c *AnalyticalContract) {
			c.ScalarEntailment.Witnesses[0], c.ScalarEntailment.Witnesses[1] = c.ScalarEntailment.Witnesses[1], c.ScalarEntailment.Witnesses[0]
		},
		"foreign_metric":              func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Metric = "foreign" },
		"foreign_resolution":          func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Resolution = Hash("foreign") },
		"wrong_leaf_digest":           func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].LeafDigest = Hash("foreign") },
		"wrong_filter_digest":         func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].FilterDigest = Hash("foreign") },
		"missing_publication":         func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Publication = "" },
		"missing_owner":               func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].FilterOwner = "" },
		"invalid_owner_kind":          func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].FilterOwnerKind = "dimension" },
		"missing_relationship":        func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[1].Relationship = "" },
		"missing_relationship_digest": func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[1].RelationshipDigest = "" },
		"wrong_join_digest":           func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[1].JoinDigest = Hash("foreign") },
		"same_fact_relationship":      func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Relationship = "unrelated" },
		"one_leaf_unentailed": func(c *AnalyticalContract) {
			c.Metrics[0].Expression.Args[1].Filters = c.Metrics[0].Expression.Args[1].Filters[1:]
			c.ScalarEntailment.Witnesses[1].LeafDigest = Hash(c.Metrics[0].Expression.Args[1])
		},
		"singleton_in": func(c *AnalyticalContract) {
			c.Metrics[0].Expression.Args[0].Filters[0].Kind = "in"
			c.ScalarEntailment.Witnesses[0].LeafDigest = Hash(c.Metrics[0].Expression.Args[0])
			c.ScalarEntailment.Witnesses[0].FilterDigest = Hash(c.Metrics[0].Expression.Args[0].Filters[0])
		},
		"unsupported_aggregate": func(c *AnalyticalContract) {
			c.Metrics[0].Expression.Args[0].Op = "avg"
			c.ScalarEntailment.Witnesses[0].LeafDigest = Hash(c.Metrics[0].Expression.Args[0])
		},
		"period_resolution_overlap": func(c *AnalyticalContract) {
			c.ScalarPopulations.Lanes[0].QueryPopulation.Constraints[0].Resolution = c.ScalarEntailment.Constraints[0].Resolution
		},
		"grouped_domain": func(c *AnalyticalContract) {
			c.GroupDomain = &AnalyticalGroupDomain{Policy: AnalyticalGroupDomainPolicy, Domain: AnalyticalGroupDomainRaw}
		},
		"outer_join": func(c *AnalyticalContract) {
			c.ScalarPopulations.Lanes[0].Joins[0].Type = "left"
			c.ScalarEntailment.Witnesses[1].JoinDigest = Hash(c.ScalarPopulations.Lanes[0].Joins[0])
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			c := scalarEntailmentClone(t, original)
			mutate(&c)
			if err := ValidateAnalyticalScalarEntailment(t.Context(), c, p.candidate.binding); err == nil {
				t.Fatal("unproved coverage admitted")
			}
		})
	}
	for name, mutate := range map[string]func(*BusinessConstraint){
		"entity": func(c *BusinessConstraint) { c.Kind = "entity" }, "number": func(c *BusinessConstraint) { c.Kind = "number" }, "ne": func(c *BusinessConstraint) { c.Operator = "ne" }, "include_null": func(c *BusinessConstraint) { c.Nulls = "include" }, "null_only": func(c *BusinessConstraint) { c.Null = true; c.Nulls = "only"; c.Value = "" }, "inexact_value": func(c *BusinessConstraint) { c.Value = "PAID" }, "aggregation": func(c *BusinessConstraint) { c.Aggregation = "count" }, "unit": func(c *BusinessConstraint) { c.Unit = "tag" }, "precision": func(c *BusinessConstraint) { c.Precision = 1 }, "scale": func(c *BusinessConstraint) { c.Scale = 1 }, "upper": func(c *BusinessConstraint) { c.Upper = "z" }, "bounds": func(c *BusinessConstraint) { c.Bounds = "[]" }, "temporal_type": func(c *BusinessConstraint) { c.TemporalType = "text" }, "calendar": func(c *BusinessConstraint) { c.Calendar = "gregorian" }, "timezone": func(c *BusinessConstraint) { c.TimeZone = "UTC" }, "grain": func(c *BusinessConstraint) { c.Grain = "day" }, "stale_revision": func(c *BusinessConstraint) { c.SourceRevision++ },
	} {
		t.Run(name, func(t *testing.T) {
			c := scalarEntailmentClone(t, original)
			mutate(&c.ScalarEntailment.Constraints[0])
			if err := ValidateAnalyticalScalarEntailment(t.Context(), c, p.candidate.binding); err == nil {
				t.Fatal("noncanonical constraint admitted")
			}
		})
	}
}

func TestScalarEntailmentCurrentPhysicalEvidence(t *testing.T) {
	original, c, _ := scalarEntailmentFixture(t)
	for _, name := range []string{"no_unique_key", "partial_unique_key", "unsafe_filter", "foreign_context", "unsupported_dialect"} {
		t.Run(name, func(t *testing.T) {
			p := original
			data, _ := json.Marshal(p.candidate.binding)
			_ = json.Unmarshal(data, &p.candidate.binding)
			for i := range p.candidate.binding.Relations {
				r := &p.candidate.binding.Relations[i]
				if r.ID != "sales" {
					continue
				}
				switch name {
				case "no_unique_key":
					r.UniqueKeys = nil
				case "partial_unique_key":
					r.UniqueKeys = [][]string{{"division", "id", "amount"}}
				case "unsafe_filter":
					for j := range r.Columns {
						if r.Columns[j].Name == "name" {
							r.Columns[j].Safe = false
						}
					}
				}
			}
			if name == "foreign_context" {
				p.candidate.binding.Context = "foreign"
			}
			if name == "unsupported_dialect" {
				p.candidate.binding.Dialect = "mysql"
			}
			bad := scalarEntailmentClone(t, c)
			if name != "foreign_context" {
				bad.Binding = Hash(p.candidate.binding)
			}
			if err := ValidateAnalyticalScalarEntailment(t.Context(), bad, p.candidate.binding); err == nil {
				t.Fatal("lost physical evidence admitted")
			}
		})
	}
}

func TestScalarEntailmentConstructorDetachmentAndBounds(t *testing.T) {
	p, c, _ := scalarEntailmentFixture(t)
	base := c
	base.Version = AnalyticalScopedPopulationsVersion
	base.ScalarEntailment = nil
	constraints := append([]BusinessConstraint(nil), c.ScalarEntailment.Constraints...)
	witnesses := append([]AnalyticalScalarPredicateWitness(nil), c.ScalarEntailment.Witnesses...)
	witnesses[0].Path = append([]int(nil), witnesses[0].Path...)
	witnesses[0], witnesses[1] = witnesses[1], witnesses[0]
	proof, err := NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, base, constraints, witnesses)
	if err != nil || Hash(proof) != Hash(c.ScalarEntailment) {
		t.Fatal("constructor failed to canonicalize", err)
	}
	prior := Hash(proof)
	constraints[0].Value = "changed"
	witnesses[1].Path[0] = 5
	if Hash(proof) != prior {
		t.Fatal("constructor retained caller slices")
	}
	if _, err := NewAnalyticalScalarEntailment(nil, p.candidate.binding, base, c.ScalarEntailment.Constraints, c.ScalarEntailment.Witnesses); err == nil {
		t.Fatal("nil context admitted")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewAnalyticalScalarEntailment(ctx, p.candidate.binding, base, c.ScalarEntailment.Constraints, c.ScalarEntailment.Witnesses); err == nil {
		t.Fatal("cancelled constructor admitted")
	}
	many := make([]BusinessConstraint, 64)
	for i := range many {
		many[i] = c.ScalarEntailment.Constraints[0]
		many[i].Resolution = Hash(fmt.Sprint(i))
	}
	base.Metrics = nil
	for i := 0; i < 5; i++ {
		m := c.Metrics[0]
		m.ID = fmt.Sprintf("metric%d", i)
		base.Metrics = append(base.Metrics, m)
	}
	if _, err := NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, base, many, nil); err != ErrLimit {
		t.Fatal("constraint by occurrence bound not enforced before coverage allocation", err)
	}
}

func TestScalarEntailmentRepeatedOccurrenceAndRootPaths(t *testing.T) {
	p, c, _ := scalarEntailmentFixture(t)
	base := c
	base.Version = AnalyticalScopedPopulationsVersion
	base.ScalarEntailment = nil
	leaf := c.Metrics[0].Expression.Args[0]
	base.Metrics = append(base.Metrics, AnalyticalMetric{ID: "root", Expression: leaf}, AnalyticalMetric{ID: "repeated", Expression: AnalyticalExpression{Op: "+", Args: []AnalyticalExpression{leaf, leaf}}})
	witnesses := append([]AnalyticalScalarPredicateWitness(nil), c.ScalarEntailment.Witnesses...)
	root := witnesses[0]
	root.Metric = "root"
	root.Path = nil
	witnesses = append(witnesses, root)
	for i := 0; i < 2; i++ {
		w := root
		w.Metric = "repeated"
		w.Path = []int{i}
		witnesses = append(witnesses, w)
	}
	proof, err := NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, base, c.ScalarEntailment.Constraints, witnesses)
	if err != nil {
		t.Fatal(err)
	}
	if len(proof.Witnesses) != 5 {
		t.Fatal("repeated measure occurrences deduplicated")
	}
	for _, w := range proof.Witnesses {
		if w.Metric == "root" && (w.Path == nil || len(w.Path) != 0) {
			t.Fatal("root path is not canonical empty array")
		}
	}
	base.Version = AnalyticalScalarEntailmentVersion
	base.ScalarEntailment = proof
	if err := ValidateAnalyticalScalarEntailment(t.Context(), base, p.candidate.binding); err != nil {
		t.Fatal(err)
	}
	proof.Witnesses = proof.Witnesses[:4]
	if err := ValidateAnalyticalScalarEntailment(t.Context(), base, p.candidate.binding); err == nil {
		t.Fatal("missing repeated/root occurrence admitted")
	}
}

func TestScalarEntailmentLegacyFencesAndRedaction(t *testing.T) {
	p, c, sql := scalarEntailmentFixture(t)
	for _, version := range []string{AnalyticalVersion, AnalyticalGrainVersion, AnalyticalCalendarVersion, AnalyticalQueryPopulationVersion, AnalyticalGroupingVersion, AnalyticalIntentVersion, AnalyticalGroupedPopulationsVersion, AnalyticalGroupedProgramsVersion, AnalyticalScopedPopulationsVersion, AnalyticalGroupedOwnedPopulationsVersion, AnalyticalGroupedSelectionVersion, AnalyticalGroupedFactsVersion} {
		t.Run(version, func(t *testing.T) {
			bad := c
			bad.Version = version
			if _, err := CheckAnalyticalPlan(t.Context(), p, bad); err == nil {
				t.Fatal("new protected field admitted under old version")
			}
			if _, err := BindScalarPopulationConstraints(t.Context(), p.candidate.binding, sql, nil, bad); err == nil {
				t.Fatal("legacy binder detached new-only field")
			}
			if err := ValidateAnalyticalGroupedPopulations(bad, p.candidate.binding); err == nil {
				t.Fatal("grouped validator admitted new-only field")
			}
		})
	}
	bound, err := BindScalarEntailmentConstraints(t.Context(), p.candidate.binding, sql, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(bound.Receipt)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"paid"`) || strings.Contains(string(data), "SELECT") {
		t.Fatal("public receipt exposed protected value or SQL")
	}
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	logger.Info("proof", "proof", c.ScalarEntailment)
	if strings.Contains(fmt.Sprintf("%v %#v", c.ScalarEntailment, c.ScalarEntailment), "paid") || strings.Contains(buf.String(), "paid") {
		t.Fatal("ordinary logging exposed protected constraints")
	}
	receipt := &AnalyticalReceipt{Version: AnalyticalGroupedProgramsVersion, ScalarEntailment: AnalyticalScalarEntailmentPolicy, ScalarEntailmentCoverage: Hash(c.ScalarEntailment)}
	if AnalyticalOutputsValid(receipt) {
		t.Fatal("legacy receipt acquired entailment fields")
	}
}

func TestScalarEntailmentMultipleConstraintsAndOutputOrdinals(t *testing.T) {
	p, c, sql := scalarEntailmentFixture(t)
	base := c
	base.Version = AnalyticalScopedPopulationsVersion
	base.ScalarEntailment = nil
	second := c.ScalarEntailment.Constraints[0]
	second.Resolution = Hash("second-independent-paid-selection")
	constraints := []BusinessConstraint{second, c.ScalarEntailment.Constraints[0]}
	witnesses := append([]AnalyticalScalarPredicateWitness(nil), c.ScalarEntailment.Witnesses...)
	for _, old := range c.ScalarEntailment.Witnesses {
		w := old
		w.Resolution = second.Resolution
		witnesses = append(witnesses, w)
	}
	// Add direct selected amount outputs and deliberately permute their SQL targets.
	base.Metrics = append(base.Metrics, AnalyticalMetric{ID: "selected_gross", Expression: c.Metrics[0].Expression.Args[0]}, AnalyticalMetric{ID: "selected_refunds", Expression: c.Metrics[0].Expression.Args[1]})
	for _, constraint := range constraints {
		for i, id := range []string{"selected_gross", "selected_refunds"} {
			w := c.ScalarEntailment.Witnesses[i]
			w.Resolution = constraint.Resolution
			w.Metric = id
			w.Path = []int{}
			witnesses = append(witnesses, w)
		}
	}
	sql = strings.Replace(sql, "SELECT gross.total-refunds.total AS net", "SELECT refunds.total AS refunds, gross.total-refunds.total AS net, gross.total AS gross", 1)
	var err error
	base.ScalarEntailment, err = NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, base, constraints, witnesses)
	if err != nil {
		t.Fatal(err)
	}
	base.Version = AnalyticalScalarEntailmentVersion
	bound, err := BindScalarEntailmentConstraints(t.Context(), p.candidate.binding, sql, nil, base)
	if err != nil {
		t.Fatal(err)
	}
	if len(bound.Parameters) != 4 || len(bound.Receipt.Entailments) != 2 {
		t.Fatal("additional predicate changed period binding")
	}
	for _, effect := range bound.Receipt.Entailments {
		if effect.Occurrences != 4 || effect.Parameters == nil || len(effect.Parameters) != 0 {
			t.Fatal("lost complete repeated selected-root coverage")
		}
	}
	p.candidate.statement, p.candidate.parameters = bound.SQL, bound.Parameters
	receipt, err := CheckAnalyticalPlan(t.Context(), p, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, out := range receipt.Outputs {
		want := 1
		if out.Metric == "selected_gross" {
			want = 2
		}
		if out.Metric == "selected_refunds" {
			want = 0
		}
		if out.Column != want {
			t.Fatal("output proof inferred alias/order instead of final ordinal")
		}
	}
	for _, badSQL := range []string{strings.Replace(sql, "s.name='paid'", "s.name='unpaid'", 1), strings.Replace(sql, "o.name='paid'", "o.name='unpaid'", 1)} {
		if _, err := BindScalarEntailmentConstraints(t.Context(), p.candidate.binding, badSQL, nil, base); err == nil {
			t.Fatal("binder emitted discharge for unproved generated SQL")
		}
	}
}

func TestScalarEntailmentInheritedOwnerAndClosedTextTypes(t *testing.T) {
	p, c, _ := scalarEntailmentFixture(t)
	inherited := scalarEntailmentClone(t, c)
	inherited.ScalarEntailment.Witnesses[0].FilterOwnerKind = "kpi"
	inherited.ScalarEntailment.Witnesses[0].FilterOwner = "selected_net"
	if err := ValidateAnalyticalScalarEntailment(t.Context(), inherited, p.candidate.binding); err != nil {
		t.Fatal("same-fact inherited reviewed filter rejected", err)
	}
	inherited.ScalarEntailment.Witnesses[1].FilterOwnerKind = "kpi"
	inherited.ScalarEntailment.Witnesses[1].FilterOwner = "selected_net"
	if err := ValidateAnalyticalScalarEntailment(t.Context(), inherited, p.candidate.binding); err == nil {
		t.Fatal("unqualified cross-fact inherited relationship admitted")
	}
	for _, native := range []string{"text", "varchar", "varchar(64)", "character varying", "character varying(64)", "pg_catalog.text"} {
		if !scalarEntailmentTextColumn(Column{NativeType: native, Category: "text", Safe: true}) {
			t.Fatal("eligible text rejected", native)
		}
	}
	for _, native := range []string{"uuid", "name", "bpchar", "char(2)", "citext", "text_domain", "varchar(0)", "varchar(-1)", "varchar(99999999)", "varchar(1,2)"} {
		if scalarEntailmentTextColumn(Column{NativeType: native, Category: "text", Safe: true}) {
			t.Fatal("ineligible native type admitted", native)
		}
	}
}

func TestScalarEntailmentTraversalBoundsAndCanonicalPaths(t *testing.T) {
	p, c, _ := scalarEntailmentFixture(t)
	for _, mutate := range []func(*AnalyticalContract){
		func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Path = []int{-1} },
		func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Path = []int{0, 0} },
		func(c *AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Path = make([]int, 33) },
		func(c *AnalyticalContract) {
			c.Metrics[0].Expression.Args = append(c.Metrics[0].Expression.Args, c.Metrics[0].Expression.Args[0])
		},
		func(c *AnalyticalContract) {
			c.Metrics[0].Expression = AnalyticalExpression{Op: "coalesce", Args: []AnalyticalExpression{c.Metrics[0].Expression, {Op: "number", Value: "0"}}}
		},
		func(c *AnalyticalContract) {
			c.ScalarPopulations.Lanes = append(c.ScalarPopulations.Lanes, c.ScalarPopulations.Lanes...)
			c.ScalarPopulations.Lanes = append(c.ScalarPopulations.Lanes, c.ScalarPopulations.Lanes[0])
		},
	} {
		bad := scalarEntailmentClone(t, c)
		mutate(&bad)
		if err := ValidateAnalyticalScalarEntailment(t.Context(), bad, p.candidate.binding); err == nil {
			t.Fatal("unbounded or noncanonical shape admitted")
		}
	}
	base := c
	base.ScalarEntailment = nil
	base.Version = AnalyticalScopedPopulationsVersion
	expression := c.Metrics[0].Expression
	for i := 0; i < 33; i++ {
		expression = AnalyticalExpression{Op: "+", Args: []AnalyticalExpression{expression, {Op: "number", Value: "1"}}}
	}
	base.Metrics = []AnalyticalMetric{{ID: "deep", Expression: expression}}
	if _, err := NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, base, c.ScalarEntailment.Constraints, nil); err != ErrLimit {
		t.Fatal("deep tree not bounded", err)
	}
	expression = AnalyticalExpression{Op: "number", Value: "1"}
	for i := 0; i < 10; i++ {
		expression = AnalyticalExpression{Op: "+", Args: []AnalyticalExpression{expression, expression}}
	}
	base.Metrics = []AnalyticalMetric{{ID: "large", Expression: AnalyticalExpression{Op: "+", Args: []AnalyticalExpression{expression, c.Metrics[0].Expression}}}}
	if _, err := NewAnalyticalScalarEntailment(t.Context(), p.candidate.binding, base, c.ScalarEntailment.Constraints, nil); err != ErrLimit {
		t.Fatal("large constant tree not bounded", err)
	}
}

func TestScalarEntailmentConcurrentReuse(t *testing.T) {
	p, c, sql := scalarEntailmentFixture(t)
	before := Hash(c)
	failures := make(chan error, 4)
	for i := 0; i < 4; i++ {
		go func() {
			bound, err := BindScalarEntailmentConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			if err == nil {
				local := p
				local.candidate.statement, local.candidate.parameters = bound.SQL, bound.Parameters
				_, err = CheckAnalyticalPlan(t.Context(), local, c)
			}
			failures <- err
		}()
	}
	for i := 0; i < 4; i++ {
		if err := <-failures; err != nil {
			t.Fatal(err)
		}
	}
	if Hash(c) != before {
		t.Fatal("concurrent reuse mutated shared protected contract")
	}
}

func TestScalarEntailmentRejectsDetachedLegacyFamilyPaths(t *testing.T) {
	_, current, _ := scalarEntailmentFixture(t)
	for _, version := range []string{AnalyticalGroupedProgramsVersion, AnalyticalGroupedOwnedPopulationsVersion, AnalyticalGroupedSelectionVersion, AnalyticalGroupedFactsVersion} {
		t.Run(version, func(t *testing.T) {
			var p Plan
			var c AnalyticalContract
			var sql string
			switch version {
			case AnalyticalGroupedProgramsVersion:
				p, c, sql = groupedOwnedFixture(t, AnalyticalGroupDomainRaw, false)
				c.Version = version
				for i := range c.GroupedPopulations.Lanes {
					c.GroupedPopulations.Lanes[i].QueryPopulation = nil
				}
			case AnalyticalGroupedOwnedPopulationsVersion:
				p, c, sql = groupedOwnedFixture(t, AnalyticalGroupDomainRaw, false)
			case AnalyticalGroupedSelectionVersion:
				p, c, sql = groupSelectionFixture(t, AnalyticalGroupDomainRaw, true)
			case AnalyticalGroupedFactsVersion:
				p, c, sql = groupedFactsFixture(t, AnalyticalGroupDomainRaw, true, true)
				groupedFactsSet(t, p, &c, "sales", groupedFactConstraint("sales", "tag", "approved"))
			}
			if err := ValidateAnalyticalGroupedPopulations(c, p.candidate.binding); err != nil {
				t.Fatal("invalid legacy control", err)
			}
			c.ScalarEntailment = current.ScalarEntailment
			if err := ValidateAnalyticalGroupedPopulations(c, p.candidate.binding); err == nil {
				t.Fatal("new-only field reached detached legacy relation proof")
			}
			var err error
			switch version {
			case AnalyticalGroupedOwnedPopulationsVersion:
				_, err = BindGroupedPopulationConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			case AnalyticalGroupedSelectionVersion:
				_, err = BindGroupedSelectionConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			case AnalyticalGroupedFactsVersion:
				_, err = BindGroupedFactConstraints(t.Context(), p.candidate.binding, sql, nil, c)
			default:
				_, err = CheckAnalyticalPlan(t.Context(), p, c)
			}
			if err == nil {
				t.Fatal("new-only field detached by legacy binding/proof")
			}
		})
	}
}
