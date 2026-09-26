package semantics

// MayRequireSourceBinding is request-local preflight information, not retained
// resolution evidence. A currently reachable reference choice can activate a
// typed predicate after the answer. The service must pin that source binding
// before it issues the answer context, not change the context after the answer.
// This never resolves a choice, selects a metric, or authorizes source access.
func (e ClarificationEvaluation) MayRequireSourceBinding() bool {
	return e.mayRequireSourceBinding
}

// possibleClarificationBinding over-approximates the branches reachable from
// the INITIAL question/facts. Only reviewed choice targets are traversed;
// unrelated or unseeded reference cycles stay inactive. Authored rules bound
// the graph to 128 patterns, 32 targets per pattern and 16 slots per pattern.
// Input/canonical answer validation has already completed in the evaluator.
func possibleClarificationBinding(d RuleSetDefinition, in ClarificationInput, answers map[clarificationKey]ClarificationAnswer) bool {
	facts := make(map[Reference]bool, len(in.References))
	for _, r := range in.References {
		facts[r] = true
	}
	visited := make(map[string]bool, len(d.Patterns))
	for pass := 0; pass <= len(d.Patterns); pass++ {
		changed := false
		for _, p := range d.Patterns {
			if visited[p.ID] || p.Policy != nil && p.Policy.Disabled {
				continue
			}
			active := false
			if p.Policy != nil {
				// The term matcher is the same as actual applicability; reference lookup
				// uses the bounded graph rather than promoting a hypothetical selection.
				active, _ = matchClarificationPattern(p, in.Question, nil)
				for _, r := range p.Policy.When.AnyReferences {
					active = active || facts[r]
				}
			} else {
				for _, s := range p.Slots {
					_, supplied := answers[clarificationKey{p.ID, s.ID}]
					active = active || supplied && s.Kind == SlotChoice
				}
			}
			if !active {
				continue
			}
			visited[p.ID] = true
			changed = true
			for _, s := range p.Slots {
				if s.Effect != nil {
					return true
				}
				if s.Kind != SlotChoice {
					continue
				}
				for _, c := range s.Choices {
					if c.Target != nil {
						facts[*c.Target] = true
					}
				}
			}
		}
		if !changed {
			return false
		}
	}
	return false // Finite positive graph: every changing pass visited a pattern.
}
