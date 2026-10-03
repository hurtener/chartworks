package exec

// expectedNullPolicy records an explicitly reviewed fallback. It is never
// inferred from nullable metadata, an empty source, or a generated SQL default.
func (a *analyticalChecker) expectedNullPolicy(e AnalyticalExpression, depth int) (string, error) {
	if e.Column != "" || e.Value != "" || len(e.Filters) > 0 || len(e.Args) < 2 || len(e.Args) > 8 {
		return "", ErrBinding
	}
	keys := make([]string, len(e.Args))
	for i, arg := range e.Args {
		key, err := a.expected(arg, depth+1)
		if err != nil {
			return "", err
		}
		keys[i] = key
	}
	return analyticalExprKey("coalesce", "", "", nil, keys), nil
}
func (a *analyticalChecker) coalesceTerm(c map[string]any, depth int) (analyticalTerm, error) {
	if !only(c, "args", "location") || len(array(c["args"])) < 2 || len(array(c["args"])) > 8 {
		return analyticalTerm{}, analyticalFailure("analytical_expression_unsupported", true)
	}
	keys := []string{}
	out := analyticalTerm{exact: true}
	for _, arg := range array(c["args"]) {
		term, err := a.term(arg, depth+1)
		if err != nil {
			return analyticalTerm{}, err
		}
		if term.key == "" || !term.exact || term.guarded {
			return analyticalTerm{}, analyticalFailure("analytical_zero_policy", false)
		}
		keys = append(keys, term.key)
		out.aggregate = out.aggregate || term.aggregate
		out.numeric = out.numeric || term.numeric
	}
	out.key = analyticalExprKey("coalesce", "", "", nil, keys)
	return out, nil
}
