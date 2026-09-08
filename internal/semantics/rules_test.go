package semantics

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
)

func testRules(t *testing.T) (Model, RuleSetDefinition) {
	t.Helper()
	model, err := Compile(testPack())
	if err != nil {
		t.Fatal(err)
	}
	measure := Reference{Kind: KindMeasure, ID: "revenue"}
	kpi := Reference{Kind: KindKPI, ID: "average_order_value"}
	canonical := model.pack.CanonicalEntities[0].Reference()
	return model, RuleSetDefinition{
		SchemaVersion: SchemaVersion, ID: "commerce_rules", Version: "rules:v1", Topic: model.pack.Topic, TopicVersion: model.pack.Version, PackDigest: model.Digest(),
		Rules: []RuleDefinition{
			{ID: "revenue_required", Version: "rule:v1", Category: RuleComputation, Class: RuleExecutionConstraint, Scope: RuleScope{Kind: RuleScopeEntities, Targets: []Reference{measure}}, Priority: 100, Provenance: RuleProvenance{Kind: ProvenanceHuman, Evidence: "review_note:1"}, Constraint: &Constraint{Kind: ConstraintRequireReference, Target: measure}},
			{ID: "revenue_explanation", Version: "rule:v2", Category: RuleSemantic, Class: RuleAdvisoryContext, Scope: RuleScope{Kind: RuleScopeTopic}, Priority: 1, Provenance: RuleProvenance{Kind: ProvenanceFeedback, Evidence: "feedback:1"}, Guidance: &AdvisoryGuidance{Text: "Describe the metric and its unit. Explicar la métrica y su unidad.", Sensitivity: LiteralNonSensitive}},
		},
		Patterns: []ClarificationPattern{{ID: "metric_choice", Version: "pattern:v1", Targets: []Reference{measure, kpi, canonical}, Provenance: RuleProvenance{Kind: ProvenanceModel, Evidence: "candidate:1"}, Slots: []ClarificationSlot{
			{ID: "metric", Prompt: "Choose a metric / Elegí una métrica", Required: true, Kind: SlotChoice, Sensitivity: LiteralNonSensitive, Choices: []ClarificationChoice{{ID: "total", Label: "Total revenue", Target: &measure}, {ID: "average", Label: "Average order value", Target: &kpi}}},
			{ID: "reference", Prompt: "Reference / Referencia", Required: false, Kind: SlotText, Sensitivity: LiteralSensitive},
		}}},
	}
}

func TestRuleCompilationPinsSemanticsAndDetachesAllNestedState(t *testing.T) {
	model, input := testRules(t)
	compiled, err := CompileRules(model, input)
	if err != nil {
		t.Fatal(err)
	}
	if len(compiled.Digest()) != 64 || compiled.Definition().PackDigest != model.Digest() {
		t.Fatal("missing exact semantic pin")
	}
	before := compiled.Definition()
	input.Rules[0].Constraint.Target.ID = "changed"
	input.Rules[0].Scope.Targets[0].ID = "changed"
	input.Rules[1].Guidance.Text = "changed"
	input.Patterns[0].Targets[0].ID = "changed"
	input.Patterns[0].Slots[0].Choices[0].Target.ID = "changed"
	input.Patterns[0].Slots[0].Prompt = "changed"
	if compiled.Definition().Rules[1].Constraint.Target.ID != "revenue" || compiled.Definition().Patterns[0].Slots[0].Choices[0].Target.ID != "revenue" || compiled.Definition().Rules[0].Guidance.Text == "changed" {
		t.Fatal("compiler retained mutable caller state")
	}
	before.Rules[1].Constraint.Target.ID = "changed"
	before.Rules[1].Scope.Targets[0].ID = "changed"
	before.Rules[0].Guidance.Text = "changed"
	before.Patterns[0].Targets[0].ID = "changed"
	before.Patterns[0].Slots[0].Choices[0].Target.ID = "changed"
	if compiled.Definition().Rules[1].Constraint.Target.ID != "revenue" || compiled.Definition().Patterns[0].Slots[0].Choices[0].Target.ID != "revenue" || compiled.Definition().Rules[0].Guidance.Text == "changed" {
		t.Fatal("definition returned shared mutable state")
	}

	_, reordered := testRules(t)
	slices.Reverse(reordered.Rules)
	slices.Reverse(reordered.Patterns[0].Targets)
	again, err := CompileRules(model, reordered)
	if err != nil || again.Digest() != compiled.Digest() {
		t.Fatalf("unordered definition changed digest: %v", err)
	}
	slices.Reverse(reordered.Patterns[0].Slots[0].Choices)
	changed, err := CompileRules(model, reordered)
	if err != nil || changed.Digest() == compiled.Digest() {
		t.Fatalf("choice presentation order was lost: %v", err)
	}
	_, reordered = testRules(t)
	reordered.Rules[0].Priority++
	changed, err = CompileRules(model, reordered)
	if err != nil || changed.Digest() == compiled.Digest() {
		t.Fatalf("priority was not retained in revision: %v", err)
	}
}

func TestPublishedRuleSubjectAndDeterministicConstraintEvaluation(t *testing.T) {
	model, definition := testRules(t)
	pack := model.Pack()
	for i := range pack.Datasets {
		pack.Datasets[i].Source = SourceReference{}
	}
	subject, err := NewRuleSubject(pack, model.Digest())
	if err != nil {
		t.Fatal(err)
	}
	revenue := Reference{Kind: KindMeasure, ID: "revenue"}
	orders := Reference{Kind: KindDataset, ID: "orders"}
	definition.Patterns = nil
	definition.Rules = []RuleDefinition{{
		ID: "require_revenue", Version: "v1", Category: RuleComputation, Class: RuleExecutionConstraint,
		Scope: RuleScope{Kind: RuleScopeTopic}, Provenance: RuleProvenance{Kind: ProvenanceHuman, Evidence: "review:1"},
		Constraint: &Constraint{Kind: ConstraintRequireReference, Target: revenue},
	}}
	compiled, err := CompilePublishedRules(subject, definition)
	if err != nil {
		t.Fatal(err)
	}
	missing, err := EvaluateConstraints(subject, compiled, []Reference{orders})
	if err != nil || missing.Allowed || len(missing.Violations) != 1 || missing.Violations[0].Kind != ViolationMissingRequired || !slices.Equal(missing.Required, []Reference{revenue}) {
		t.Fatal("missing requirement", missing, err)
	}
	allowed, err := EvaluateConstraints(subject, compiled, []Reference{revenue})
	if err != nil || !allowed.Allowed || len(allowed.Violations) != 0 {
		t.Fatal("allowed references", allowed, err)
	}
	for _, refs := range [][]Reference{nil, {revenue, revenue}, {{Kind: KindMeasure, ID: "unknown"}}} {
		if _, err = EvaluateConstraints(subject, compiled, refs); err == nil {
			t.Fatal("invalid candidate references accepted", refs)
		}
	}
	if _, err = NewRuleSubject(pack, strings.Repeat("A", 64)); err == nil {
		t.Fatal("noncanonical topic digest accepted")
	}
}

func TestRuleCompilationRejectsStaleAndMalformedDefinitions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*RuleSetDefinition)
		code ValidationCode
	}{
		{"wrong_topic", func(p *RuleSetDefinition) { p.Topic = "other" }, CodeEvidenceMismatch},
		{"stale_topic_version", func(p *RuleSetDefinition) { p.TopicVersion = "commerce:v0" }, CodeEvidenceMismatch},
		{"stale_digest", func(p *RuleSetDefinition) { p.PackDigest = strings.Repeat("0", 64) }, CodeEvidenceMismatch},
		{"schema", func(p *RuleSetDefinition) { p.SchemaVersion++ }, CodeInvalidValue},
		{"rules_limit", func(p *RuleSetDefinition) { p.Rules = make([]RuleDefinition, 257) }, CodeLimit},
		{"patterns_limit", func(p *RuleSetDefinition) { p.Patterns = make([]ClarificationPattern, 129) }, CodeLimit},
		{"priority_bound", func(p *RuleSetDefinition) { p.Rules[0].Priority = 1001 }, CodeInvalidValue},
		{"category", func(p *RuleSetDefinition) { p.Rules[0].Category = "custom" }, CodeInvalidValue},
		{"provenance", func(p *RuleSetDefinition) { p.Rules[0].Provenance.Kind = "approved" }, CodeInvalidValue},
		{"provenance_evidence", func(p *RuleSetDefinition) { p.Rules[0].Provenance.Evidence = "" }, CodeInvalidValue},
		{"open_scope", func(p *RuleSetDefinition) { p.Rules[0].Scope.Kind = "all_tenants" }, CodeInvalidValue},
		{"topic_scope_targets", func(p *RuleSetDefinition) { p.Rules[0].Scope.Kind = RuleScopeTopic }, CodeInvalidValue},
		{"empty_entities", func(p *RuleSetDefinition) { p.Rules[0].Scope.Targets = nil }, CodeInvalidValue},
		{"missing_reference", func(p *RuleSetDefinition) { p.Rules[0].Scope.Targets[0].ID = "missing" }, CodeMissingReference},
		{"duplicate_reference", func(p *RuleSetDefinition) {
			p.Rules[0].Scope.Targets = append(p.Rules[0].Scope.Targets, p.Rules[0].Scope.Targets[0])
		}, CodeInvalidReference},
		{"wrong_column_coordinates", func(p *RuleSetDefinition) { p.Rules[0].Scope.Targets[0] = Reference{Kind: KindColumn, ID: "amount"} }, CodeInvalidReference},
		{"stale_canonical_pin", func(p *RuleSetDefinition) { p.Patterns[0].Targets[2].Revision++ }, CodeMissingReference},
		{"constraint_outside_scope", func(p *RuleSetDefinition) { p.Rules[0].Constraint.Target.ID = "order_count" }, CodeInvalidReference},
		{"constraint_missing", func(p *RuleSetDefinition) { p.Rules[0].Constraint = nil }, CodeInvalidValue},
		{"both_rule_payloads", func(p *RuleSetDefinition) { p.Rules[0].Guidance = p.Rules[1].Guidance }, CodeInvalidValue},
		{"arbitrary_constraint", func(p *RuleSetDefinition) { p.Rules[0].Constraint.Kind = "sql_predicate" }, CodeInvalidValue},
		{"rule_class", func(p *RuleSetDefinition) { p.Rules[0].Class = "custom" }, CodeInvalidValue},
		{"unmarked_guidance", func(p *RuleSetDefinition) { p.Rules[1].Guidance.Sensitivity = "" }, CodeInvalidValue},
		{"blank_guidance", func(p *RuleSetDefinition) { p.Rules[1].Guidance.Text = " \n " }, CodeInvalidValue},
		{"duplicate_rule_id_new_version", func(p *RuleSetDefinition) { p.Rules[1].ID = p.Rules[0].ID }, CodeDuplicateID},
		{"pattern_provenance", func(p *RuleSetDefinition) { p.Patterns[0].Provenance.Evidence = "" }, CodeInvalidValue},
		{"duplicate_pattern", func(p *RuleSetDefinition) { p.Patterns = append(p.Patterns, p.Patterns[0]) }, CodeDuplicateID},
		{"duplicate_slot", func(p *RuleSetDefinition) { p.Patterns[0].Slots[1].ID = p.Patterns[0].Slots[0].ID }, CodeDuplicateID},
		{"slot_kind", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Kind = "script" }, CodeInvalidValue},
		{"unmarked_slot", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Sensitivity = "" }, CodeInvalidValue},
		{"missing_choices", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Choices = nil }, CodeLimit},
		{"text_with_choices", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Kind = SlotText }, CodeInvalidValue},
		{"invalid_choice", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Choices[0].Label = "" }, CodeInvalidValue},
		{"duplicate_choice", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Choices[1].ID = "total" }, CodeDuplicateID},
		{"choice_outside_targets", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Choices[0].Target.ID = "order_count" }, CodeInvalidReference},
		{"choice_missing_reference", func(p *RuleSetDefinition) { p.Patterns[0].Slots[0].Choices[0].Target.ID = "missing" }, CodeMissingReference},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			model, p := testRules(t)
			tc.edit(&p)
			_, err := CompileRules(model, p)
			if got := validationCode(t, err); got != tc.code {
				t.Fatalf("code=%s want %s, err=%v", got, tc.code, err)
			}
		})
	}
	_, p := testRules(t)
	if _, err := CompileRules(Model{}, p); validationCode(t, err) != CodeEvidenceMismatch {
		t.Fatalf("zero semantic model: %v", err)
	}
}

func TestRuleConflictsFollowRequiredSemanticDependencies(t *testing.T) {
	for _, tc := range []struct {
		name               string
		required, excluded Reference
	}{
		{"direct", Reference{Kind: KindMeasure, ID: "revenue"}, Reference{Kind: KindMeasure, ID: "revenue"}},
		{"measure_column", Reference{Kind: KindMeasure, ID: "revenue"}, Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}},
		{"measure_dataset", Reference{Kind: KindMeasure, ID: "revenue"}, Reference{Kind: KindDataset, ID: "orders"}},
		{"dimension", Reference{Kind: KindDimension, ID: "customer_region"}, Reference{Kind: KindDataset, ID: "customers"}},
		{"transitive_kpi", Reference{Kind: KindKPI, ID: "indexed_order_value"}, Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}},
		{"join", Reference{Kind: KindJoin, ID: "orders_customers"}, Reference{Kind: KindDataset, ID: "customers"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model, p := testRules(t)
			p.Rules = []RuleDefinition{
				{ID: "required", Version: "v1", Category: RuleStructural, Class: RuleExecutionConstraint, Scope: RuleScope{Kind: RuleScopeTopic}, Priority: -1000, Provenance: RuleProvenance{Kind: ProvenanceHuman, Evidence: "e1"}, Constraint: &Constraint{Kind: ConstraintRequireReference, Target: tc.required}},
				{ID: "excluded", Version: "v1", Category: RuleStructural, Class: RuleExecutionConstraint, Scope: RuleScope{Kind: RuleScopeTopic}, Priority: 1000, Provenance: RuleProvenance{Kind: ProvenanceImport, Evidence: "e2"}, Constraint: &Constraint{Kind: ConstraintExcludeReference, Target: tc.excluded}},
			}
			_, err := CompileRules(model, p)
			var conflict *RuleConflictError
			if !errors.As(err, &conflict) || validationCode(t, err) != CodeRuleConflict || conflict.RequiredBy != "required" || conflict.ExcludedBy != "excluded" || conflict.Target != tc.excluded {
				t.Fatalf("missing dependency conflict: %#v, %v", conflict, err)
			}
			if strings.Contains(err.Error(), tc.excluded.ID) {
				t.Fatal("error message exposed definition content")
			}
			slices.Reverse(p.Rules)
			_, err = CompileRules(model, p)
			var reordered *RuleConflictError
			if !errors.As(err, &reordered) || *reordered != *conflict {
				t.Fatalf("conflict changed with input order: %v", err)
			}
		})
	}
}

func TestRuleCompilerKeepsUnrelatedConstraintsAndSensitiveDeclarations(t *testing.T) {
	model, p := testRules(t)
	p.Rules = append(p.Rules, RuleDefinition{ID: "exclude_region", Version: "v1", Category: RuleStructural, Class: RuleExecutionConstraint, Scope: RuleScope{Kind: RuleScopeTopic}, Provenance: RuleProvenance{Kind: ProvenanceImport, Evidence: "e1"}, Constraint: &Constraint{Kind: ConstraintExcludeReference, Target: Reference{Kind: KindDimension, ID: "customer_region"}}})
	p.Rules[1].Guidance.Sensitivity = LiteralSensitive
	compiled, err := CompileRules(model, p)
	if err != nil || len(compiled.Definition().Rules) != 3 {
		t.Fatalf("dropped or rejected unrelated constraints: %v", err)
	}
	if compiled.Definition().Rules[1].Guidance.Sensitivity != LiteralSensitive || compiled.Definition().Patterns[0].Slots[1].Sensitivity != LiteralSensitive {
		t.Fatal("sensitivity declarations lost")
	}
	// Canonical registry keys are not one executable dependency requirement.
	p.Rules[0].Scope = RuleScope{Kind: RuleScopeTopic}
	p.Rules[0].Constraint.Target = model.pack.CanonicalEntities[0].Reference()
	if _, err := CompileRules(model, p); validationCode(t, err) != CodeInvalidReference {
		t.Fatalf("canonical registry became executable dependency: %v", err)
	}
}

func TestRuleCompilerConcurrentReuse(t *testing.T) {
	model, p := testRules(t)
	compiled, err := CompileRules(model, p)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				copy := compiled.Definition()
				copy.Rules[1].Constraint.Target.ID = "changed"
				copy.Patterns[0].Slots[0].Choices[0].Target.ID = "changed"
				again, err := CompileRules(model, p)
				if err != nil || again.Digest() != compiled.Digest() {
					t.Errorf("concurrent compiler state drifted: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestRuleCompilerEnforcesSerializedLimitAndClosedSlotShapes(t *testing.T) {
	model, p := testRules(t)
	p.Rules = nil
	p.Patterns = nil
	for i := 0; i < 256; i++ {
		p.Rules = append(p.Rules, RuleDefinition{ID: "advice_" + itoa(i), Version: "v1", Category: RuleSemantic, Class: RuleAdvisoryContext, Scope: RuleScope{Kind: RuleScopeTopic}, Provenance: RuleProvenance{Kind: ProvenanceHuman, Evidence: "e1"}, Guidance: &AdvisoryGuidance{Text: strings.Repeat("a", 4096), Sensitivity: LiteralNonSensitive}})
	}
	if _, err := CompileRules(model, p); validationCode(t, err) != CodeLimit {
		t.Fatalf("serialized definition limit: %v", err)
	}

	model, p = testRules(t)
	for _, kind := range []SlotKind{SlotNumber, SlotBoolean, SlotDate} {
		p.Patterns[0].Slots = append(p.Patterns[0].Slots, ClarificationSlot{ID: string(kind), Prompt: "Supply the requested value", Required: true, Kind: kind, Sensitivity: LiteralNonSensitive})
	}
	// An unbound choice is a named literal choice, not a reference guessed from its label.
	p.Patterns[0].Slots[0].Choices[0].Target = nil
	compiled, err := CompileRules(model, p)
	if err != nil || len(compiled.Definition().Patterns[0].Slots) != 5 || compiled.Definition().Patterns[0].Slots[0].Choices[0].Target != nil {
		t.Fatalf("closed slot definitions lost: %v", err)
	}
	p.Patterns[0].Slots = make([]ClarificationSlot, 17)
	if _, err := CompileRules(model, p); validationCode(t, err) != CodeInvalidValue {
		t.Fatalf("slot count bound: %v", err)
	}
}
