package nlqexec

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
)

func scalarEntailmentReceiptFixture(t *testing.T) (admission, []exec.BusinessConstraint, *exec.AnalyticalContract, QueryRecord) {
	t.Helper()
	a, constraints := scalarPredicateCharacterization(t, false)
	c, err := compileAnalyticalVersion(t.Context(), a, 13, constraints)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{AnalyticalVersion: 13, Route: a.route, SQL: "SELECT retained scalar proof"}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: strings.ReplaceAll(exec.AnalyticalMetricScope, "single_base_relation", "independent_entailed_scoped_singleton_populations"), Contract: exec.Hash(c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Intent: exec.AnalyticalIntentPolicy, QueryPopulation: exec.AnalyticalQueryPopulationPolicy, ScalarEntailment: exec.AnalyticalScalarEntailmentPolicy, ScalarEntailmentCoverage: exec.Hash(c.ScalarEntailment)}
	for _, m := range c.Metrics {
		q.Analytical.Metrics = append(q.Analytical.Metrics, m.ID)
		q.Analytical.Outputs = append(q.Analytical.Outputs, exec.AnalyticalOutput{Metric: m.ID, Column: 0})
	}
	b := exec.BusinessBindingReceipt{SchemaVersion: 6, PopulationPolicy: exec.AnalyticalScalarEntailmentPolicy, SourceBinding: exec.Hash(a.binding), Constraints: exec.Hash([]any{c.ScalarPopulations, c.ScalarEntailment}), Statement: exec.Hash("statement"), Validation: &exec.Receipt{Validated: true}}
	for _, lane := range c.ScalarPopulations.Lanes {
		period := lane.QueryPopulation.Constraints[0]
		first := len(q.Parameters) + 1
		q.Parameters = append(q.Parameters, exec.Parameter{Kind: "text", Value: period.Value}, exec.Parameter{Kind: "text", Value: period.Upper})
		b.Bindings = append(b.Bindings, exec.BusinessParameterBinding{Population: lane.Dataset, Resolution: period.Resolution, Dataset: period.Dataset, Column: period.Column, Kind: period.Kind, Operator: period.Operator, Nulls: period.Nulls, Parameters: []int{first, first + 1}})
	}
	b.Entailments = []exec.BusinessScalarPredicateEffect{{Kind: "entailed_scalar_predicate", Resolution: constraints[0].Resolution, Coverage: exec.Hash([]any{c.Binding, c.Semantics, c.Dataset, c.Metrics, c.ScalarPopulations, constraints[0], c.ScalarEntailment.Witnesses}), Occurrences: len(c.ScalarEntailment.Witnesses), Parameters: []int{}}}
	q.Analytical.Query = exec.AnalyticalQueryDigest(q.SQL, q.Parameters)
	q.Clarification = &ClarificationEvidence{SchemaVersion: 1, BaseSQL: "SELECT original reviewed base", Binding: b}
	return a, constraints, c, q
}

func TestScalarEntailmentRetainedSemanticIdentity(t *testing.T) {
	a, constraints, c, q := scalarEntailmentReceiptFixture(t)
	if !AnalyticalRecordValid(q) || !clarificationBindingSchemaValid(q) || analyticalVersionForReceipt(q.Analytical) != 13 {
		t.Fatal("valid closed retained shape refused")
	}
	if _, err := expectedAnalytical(t.Context(), q, a, constraints); err != nil {
		t.Fatal("exact reconstructed semantic receipt", err)
	}
	for name, mutate := range map[string]func(*exec.AnalyticalContract){
		"same_physical_edge_wrong_relationship_id": func(c *exec.AnalyticalContract) {
			for i := range c.ScalarEntailment.Witnesses {
				if c.ScalarEntailment.Witnesses[i].Relationship != "" {
					c.ScalarEntailment.Witnesses[i].Relationship = "same-shape-other-reviewed-edge"
					c.ScalarEntailment.Witnesses[i].RelationshipDigest = exec.Hash("other reviewed relationship")
				}
			}
		},
		"cross_topic_same_measure_id": func(c *exec.AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Topic = "other_topic" },
		"publication": func(c *exec.AnalyticalContract) {
			c.ScalarEntailment.Witnesses[0].Publication = exec.Hash("other publication")
		},
		"topic_version": func(c *exec.AnalyticalContract) { c.ScalarEntailment.Witnesses[0].TopicVersion = "v2" },
		"measure_id": func(c *exec.AnalyticalContract) {
			c.ScalarEntailment.Witnesses[0].Measure = "same_expression_other_measure"
			c.ScalarEntailment.Witnesses[0].FilterOwner = "same_expression_other_measure"
		},
		"filter_owner": func(c *exec.AnalyticalContract) {
			c.ScalarEntailment.Witnesses[0].FilterOwnerKind = "kpi"
			c.ScalarEntailment.Witnesses[0].FilterOwner = "other_inherited_context"
		},
		"filter_id":          func(c *exec.AnalyticalContract) { c.ScalarEntailment.Witnesses[0].Filter = "other_filter" },
		"missing_occurrence": func(c *exec.AnalyticalContract) { c.ScalarEntailment.Witnesses = c.ScalarEntailment.Witnesses[:1] },
		"changed_request":    func(c *exec.AnalyticalContract) { c.ScalarEntailment.Constraints[0].Value = "unpaid" },
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(c)
			var forged exec.AnalyticalContract
			if json.Unmarshal(raw, &forged) != nil {
				t.Fatal("fixture clone")
			}
			mutate(&forged)
			bad := q
			bad.Analytical = cloneAnalyticalReceipt(q.Analytical)
			// The attacker recomputes both complete contract and coverage digests.
			// Physical Binding cannot authenticate semantic IDs; reconstruction can.
			bad.Analytical.Contract = exec.Hash(forged)
			bad.Analytical.ScalarEntailmentCoverage = exec.Hash(forged.ScalarEntailment)
			if _, err := expectedAnalytical(t.Context(), bad, a, constraints); err == nil {
				t.Fatal("recomputed digests replaced immutable semantic identity")
			}
		})
	}
	if _, err := (&Service{}).expectedAnalytical(t.Context(), testEnvelope(t), q, a); err == nil {
		t.Fatal("stored route JSON replaced authenticated replay provider")
	}
	base, parameters, err := refinementSQLBase(q)
	if err != nil || base != q.Clarification.BaseSQL || len(parameters) != 0 {
		t.Fatal("bound period values became refinement base", err)
	}
	if learningPolicyForBinding(6) != "" {
		t.Fatal("new proof borrowed unqualified learning policy")
	}
}

func TestScalarEntailmentReceiptFamilyAndEffects(t *testing.T) {
	_, _, _, q := scalarEntailmentReceiptFixture(t)
	clone := func() QueryRecord {
		out := q
		out.Analytical = cloneAnalyticalReceipt(q.Analytical)
		raw, _ := json.Marshal(q.Clarification)
		var evidence ClarificationEvidence
		_ = json.Unmarshal(raw, &evidence)
		out.Clarification = &evidence
		return out
	}
	for name, mutate := range map[string]func(*QueryRecord){
		"wrong_binding_schema":      func(q *QueryRecord) { q.Clarification.Binding.SchemaVersion = 2 },
		"wrong_binding_policy":      func(q *QueryRecord) { q.Clarification.Binding.PopulationPolicy = exec.AnalyticalScalarPopulationPolicy },
		"no_entailments":            func(q *QueryRecord) { q.Clarification.Binding.Entailments = nil },
		"empty_entailments":         func(q *QueryRecord) { q.Clarification.Binding.Entailments = []exec.BusinessScalarPredicateEffect{} },
		"unknown_effect_kind":       func(q *QueryRecord) { q.Clarification.Binding.Entailments[0].Kind = "ignored_constraint" },
		"nonempty_parameter_effect": func(q *QueryRecord) { q.Clarification.Binding.Entailments[0].Parameters = []int{1} },
		"omitted_parameter_effect":  func(q *QueryRecord) { q.Clarification.Binding.Entailments[0].Parameters = nil },
		"period_effect_collision": func(q *QueryRecord) {
			q.Clarification.Binding.Entailments[0].Resolution = q.Clarification.Binding.Bindings[0].Resolution
		},
		"duplicate_effect": func(q *QueryRecord) {
			q.Clarification.Binding.Entailments = append(q.Clarification.Binding.Entailments, q.Clarification.Binding.Entailments[0])
		},
		"zero_occurrences":      func(q *QueryRecord) { q.Clarification.Binding.Entailments[0].Occurrences = 0 },
		"unbounded_occurrences": func(q *QueryRecord) { q.Clarification.Binding.Entailments[0].Occurrences = 513 },
		"coverage_shape":        func(q *QueryRecord) { q.Clarification.Binding.Entailments[0].Coverage = "invented" },
		"period_omitted":        func(q *QueryRecord) { q.Clarification.Binding.Bindings = q.Clarification.Binding.Bindings[:1] },
		"period_parameter_omitted": func(q *QueryRecord) {
			q.Clarification.Binding.Bindings[0].Parameters = q.Clarification.Binding.Bindings[0].Parameters[:1]
		},
		"period_parameters_swapped_to_duplicate": func(q *QueryRecord) {
			q.Clarification.Binding.Bindings[1].Parameters = q.Clarification.Binding.Bindings[0].Parameters
		},
	} {
		t.Run(name, func(t *testing.T) {
			bad := clone()
			mutate(&bad)
			if clarificationBindingSchemaValid(bad) || AnalyticalRecordValid(bad) {
				t.Fatal("invalid closed effect family accepted")
			}
		})
	}
	for version := 0; version <= 12; version++ {
		bad := clone()
		bad.AnalyticalVersion = version
		if AnalyticalRecordValid(bad) || clarificationBindingSchemaValid(bad) {
			t.Fatal("schema6 downgraded", version)
		}
	}
	for _, newFields := range []bool{true, false} {
		bad := clone()
		bad.AnalyticalVersion = 9
		bad.Analytical.Version = exec.AnalyticalScopedPopulationsVersion
		bad.Analytical.Scope = strings.ReplaceAll(bad.Analytical.Scope, "independent_entailed_scoped_singleton_populations", "independent_scoped_singleton_populations")
		bad.Clarification.Binding.SchemaVersion = 2
		bad.Clarification.Binding.PopulationPolicy = exec.AnalyticalScalarPopulationPolicy
		if newFields {
			bad.Clarification.Binding.Entailments = nil
		} else {
			bad.Analytical.ScalarEntailment, bad.Analytical.ScalarEntailmentCoverage = "", ""
			bad.Clarification.Binding.Entailments = []exec.BusinessScalarPredicateEffect{}
		}
		if AnalyticalRecordValid(bad) {
			t.Fatal("old version accepted new-only fields", newFields)
		}
	}
	public := publicClarificationBinding(q.Clarification)
	wire, _ := json.Marshal(public)
	for _, private := range []string{"2026-01-01T05:00:00Z", "2027-01-01T05:00:00Z", "SELECT original reviewed base", "\"paid\""} {
		if strings.Contains(string(wire), private) {
			t.Fatal("public effect leaked protected value")
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", q.Clarification), "SELECT") {
		t.Fatal("ordinary logging exposed protected base")
	}
}

func TestScalarEntailmentFieldsRejectedByEveryLegacyReceipt(t *testing.T) {
	for version := 1; version <= 12; version++ {
		r := &exec.AnalyticalReceipt{Version: fmt.Sprintf("analytical-metrics-v%d", version), Scope: exec.AnalyticalMetricScope, Metrics: []string{"topic:measure:amount"}}
		if version >= 4 {
			r.QueryPopulation = exec.AnalyticalQueryPopulationPolicy
		}
		if version >= 6 {
			r.Intent = exec.AnalyticalIntentPolicy
		}
		if version >= 9 {
			r.Outputs = []exec.AnalyticalOutput{{Metric: r.Metrics[0], Column: 0}}
		}
		if version == 9 {
			r.Scope = strings.ReplaceAll(exec.AnalyticalMetricScope, "single_base_relation", "independent_scoped_singleton_populations")
		}
		if version >= 10 {
			r.Grouping = []string{"topic:dimension:region"}
			r.Scope = strings.ReplaceAll(exec.AnalyticalGrainScope, "single_base_relation", map[int]string{10: "independent_owned_grouped_populations", 11: "independent_selected_grouped_populations", 12: "independent_filtered_grouped_populations"}[version])
		}
		if !analyticalReceiptScopeValid(r) {
			t.Fatal("invalid legacy receipt control", version)
		}
		for _, fields := range []string{"policy", "coverage", "both"} {
			bad := *r
			if fields != "coverage" {
				bad.ScalarEntailment = exec.AnalyticalScalarEntailmentPolicy
			}
			if fields != "policy" {
				bad.ScalarEntailmentCoverage = exec.Hash("invented-proof")
			}
			if analyticalReceiptScopeValid(&bad) || exec.AnalyticalOutputsValid(&bad) {
				t.Fatal("legacy receipt accepted new-only fields", version, fields)
			}
		}
	}
}
