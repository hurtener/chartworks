package semantics

// CloneRuleSetDefinition detaches the entire versioned definition, including
// conditions, effect dictionaries, defaults and dependent question ordering.
// It is a copy operation, not compilation, approval or authority.
func CloneRuleSetDefinition(definition RuleSetDefinition) RuleSetDefinition {
	return cloneRules(definition)
}
