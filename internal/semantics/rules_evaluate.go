package semantics

import (
	"sort"

	"github.com/hurtener/chartworks/internal/identity"
)

// RuleViolationKind identifies why a hard rule evaluation was not allowed.
type RuleViolationKind string

// ViolationMissingRequired and ViolationExcludedPresent identify the supported
// hard-constraint violations.
const (
	ViolationMissingRequired RuleViolationKind = "missing_required"
	ViolationExcludedPresent RuleViolationKind = "excluded_present"
)

// RuleViolation records one deterministic hard-constraint failure.
type RuleViolation struct {
	Rule   string            `json:"rule"`
	Kind   RuleViolationKind `json:"kind"`
	Target Reference         `json:"target"`
}

// ConstraintEvaluation is deterministic evidence about explicit semantic
// references. It is not an executable plan, SQL-safety proof, or authority.
type ConstraintEvaluation struct {
	Allowed    bool            `json:"allowed"`
	Required   []Reference     `json:"required"`
	Excluded   []Reference     `json:"excluded"`
	Violations []RuleViolation `json:"violations"`
	Selection  []RuleSelection `json:"selection"`
}

// RuleSelection explains deterministic scope admission without exposing rule
// text. It is stable replay/shadow evidence rather than an authority decision.
type RuleSelection struct {
	Rule    string `json:"rule"`
	Scope   string `json:"scope"`
	Applied bool   `json:"applied"`
	Reason  string `json:"reason"`
}

// RuleSelectionInput is the closed set of facts that may activate a rule.
// Template is an exact reviewed identifier, never SQL or a matching pattern.
type RuleSelectionInput struct {
	References []Reference `json:"references"`
	Template   string      `json:"template,omitempty"`
}

// EvaluateConstraints applies only closed hard constraints. Advisory text and
// clarification patterns are deliberately outside this bounded evaluator.
func EvaluateConstraints(subject RuleSubject, model RuleModel, refs []Reference) (ConstraintEvaluation, error) {
	return EvaluateSelectedConstraints(subject, model, RuleSelectionInput{References: refs})
}

// EvaluateSelectedConstraints applies closed scope selection and hard
// constraints. Compound scope is AND; entity scope preserves legacy ANY;
// template scope is exact identifier equality.
func EvaluateSelectedConstraints(subject RuleSubject, model RuleModel, input RuleSelectionInput) (ConstraintEvaluation, error) {
	refs := input.References
	if subject.digest == "" || model.digest == "" || len(refs) < 1 || len(refs) > 256 || input.Template != "" && !identity.Identifier(input.Template) {
		return ConstraintEvaluation{}, invalid(CodeInvalidValue, "evaluation.references")
	}
	present := map[Reference]bool{}
	for _, ref := range refs {
		if !ref.Valid() {
			return ConstraintEvaluation{}, invalid(CodeInvalidReference, "evaluation.references")
		}
		if _, ok := subject.refs[ref.key()]; !ok {
			return ConstraintEvaluation{}, invalid(CodeMissingReference, "evaluation.references")
		}
		if present[ref] {
			return ConstraintEvaluation{}, invalid(CodeInvalidReference, "evaluation.references")
		}
		present[ref] = true
	}
	out := ConstraintEvaluation{}
	required, excluded := map[Reference]bool{}, map[Reference]bool{}
	for _, rule := range model.definition.Rules {
		applied, reason := ruleApplies(rule.Scope, present, input.Template)
		out.Selection = append(out.Selection, RuleSelection{Rule: rule.ID, Scope: string(rule.Scope.Kind), Applied: applied, Reason: reason})
		if rule.Constraint == nil || !applied {
			continue
		}
		target := rule.Constraint.Target
		switch rule.Constraint.Kind {
		case ConstraintRequireReference:
			required[target] = true
			if !present[target] {
				out.Violations = append(out.Violations, RuleViolation{Rule: rule.ID, Kind: ViolationMissingRequired, Target: target})
			}
		case ConstraintExcludeReference:
			excluded[target] = true
			if present[target] {
				out.Violations = append(out.Violations, RuleViolation{Rule: rule.ID, Kind: ViolationExcludedPresent, Target: target})
			}
		}
	}
	for ref := range required {
		out.Required = append(out.Required, ref)
	}
	for ref := range excluded {
		out.Excluded = append(out.Excluded, ref)
	}
	sortReferences(out.Required)
	sortReferences(out.Excluded)
	sort.Slice(out.Violations, func(i, j int) bool {
		if out.Violations[i].Rule != out.Violations[j].Rule {
			return out.Violations[i].Rule < out.Violations[j].Rule
		}
		return out.Violations[i].Target.key() < out.Violations[j].Target.key()
	})
	sort.Slice(out.Selection, func(i, j int) bool { return out.Selection[i].Rule < out.Selection[j].Rule })
	out.Allowed = len(out.Violations) == 0
	return out, nil
}

func ruleApplies(scope RuleScope, present map[Reference]bool, template string) (bool, string) {
	if scope.Kind == RuleScopeTopic {
		return true, "topic"
	}
	if scope.Kind == RuleScopeTemplate {
		if template == scope.Template {
			return true, "template_match"
		}
		return false, "template_mismatch"
	}
	if scope.Kind == RuleScopeCompound {
		for _, target := range scope.Targets {
			if !present[target] {
				return false, "compound_incomplete"
			}
		}
		return true, "compound_complete"
	}
	for _, target := range scope.Targets {
		if present[target] {
			return true, "entity_present"
		}
	}
	return false, "entity_absent"
}
