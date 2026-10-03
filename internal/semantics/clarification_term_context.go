package semantics

import "strings"

// ClarificationTermMatch describes one reviewed term match without retaining the
// term or question. It is business evidence, not an applicability grant or source
// authority. The route owns protected production and replay custody.
type ClarificationTermMatch struct {
	Pattern     string `json:"pattern"`
	Version     string `json:"pattern_version"`
	Term        int    `json:"term_index"`
	Specificity int    `json:"specificity"`
}

// MatchClarificationTerms uses exactly the evaluator's bounded term grammar.
// Callers must first admit the current immutable published definition.
func MatchClarificationTerms(definition RuleSetDefinition, question string) []ClarificationTermMatch {
	var out []ClarificationTermMatch
	question = clarificationTokens(question)
	for _, p := range definition.Patterns {
		if p.Policy == nil || p.Policy.Disabled {
			continue
		}
		selected := ClarificationTermMatch{Pattern: p.ID, Version: p.Version, Term: -1}
		for i, term := range p.Policy.When.AnyTerms {
			if containsClarificationTerm(question, term) && len(strings.Fields(term)) > selected.Specificity {
				selected.Term, selected.Specificity = i, len(strings.Fields(term))
			}
		}
		if selected.Term >= 0 {
			out = append(out, selected)
		}
	}
	return out
}

func validClarificationTermContext(definition RuleSetDefinition, matches []ClarificationTermMatch) bool {
	if len(matches) > len(definition.Patterns) {
		return false
	}
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m.Pattern] {
			return false
		}
		seen[m.Pattern] = true
		valid := false
		for _, p := range definition.Patterns {
			if p.ID == m.Pattern && p.Version == m.Version && p.Policy != nil && !p.Policy.Disabled && m.Term >= 0 && m.Term < len(p.Policy.When.AnyTerms) && m.Specificity == len(strings.Fields(p.Policy.When.AnyTerms[m.Term])) && m.Specificity > 0 {
				valid = true
				break
			}
		}
		if !valid {
			return false
		}
	}
	return true
}

func matchClarificationInputPattern(pattern ClarificationPattern, input ClarificationInput, refs []Reference, termContext []ClarificationTermMatch) (bool, int) {
	active, score := matchClarificationPattern(pattern, input.Question, refs)
	if pattern.Policy == nil || pattern.Policy.Disabled {
		return active, score
	}
	_, termScore := matchClarificationPattern(pattern, input.Question, nil)
	for _, m := range termContext {
		if m.Pattern == pattern.ID && m.Specificity > termScore {
			score += m.Specificity - termScore
			termScore = m.Specificity
			active = true
		}
	}
	return active, score
}
