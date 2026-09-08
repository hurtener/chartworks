package semantics

import (
	"sort"
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
}

// EvaluateConstraints applies only closed hard constraints. Advisory text and
// clarification patterns are deliberately outside this bounded evaluator.
func EvaluateConstraints(subject RuleSubject, model RuleModel, refs []Reference) (ConstraintEvaluation, error) {
	if subject.digest == "" || model.digest == "" || len(refs) < 1 || len(refs) > 256 {
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
		if rule.Constraint == nil || !ruleApplies(rule.Scope, present) {
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
	out.Allowed = len(out.Violations) == 0
	return out, nil
}

func ruleApplies(scope RuleScope, present map[Reference]bool) bool {
	if scope.Kind == RuleScopeTopic {
		return true
	}
	for _, target := range scope.Targets {
		if present[target] {
			return true
		}
	}
	return false
}
