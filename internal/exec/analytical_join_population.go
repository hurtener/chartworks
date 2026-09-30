package exec

import "strings"

// Reconstruct only the already-proved relation graph, using the native query's
// resolved aliases. The ordinary binder owns values and positions. This statement
// is parsed for predicate comparison only; it is never executed or model input.
func (a *analyticalChecker) populationBaseSQL() (string, error) {
	quote := func(s string) string { return businessQuote(a.binding.Dialect, s) }
	if len(a.joins) == 0 {
		return "SELECT count(*) FROM " + quote(a.relation.Schema) + "." + quote(a.relation.Name) + " AS " + quote(a.alias), nil
	}
	aliases := map[string]string{}
	for alias, id := range a.joinAliases {
		if aliases[id] != "" {
			return "", ErrBinding
		}
		aliases[id] = alias
	}
	relation := func(id string) (string, error) {
		if aliases[id] == "" {
			return "", ErrBinding
		}
		for _, r := range a.binding.Relations {
			if r.ID == id {
				return quote(r.Schema) + "." + quote(r.Name) + " AS " + quote(aliases[id]), nil
			}
		}
		return "", ErrBinding
	}
	base, err := relation(a.joins[0].Left)
	if err != nil {
		return "", err
	}
	seen := map[string]bool{a.joins[0].Left: true}
	var sql strings.Builder
	sql.WriteString("SELECT count(*) FROM ")
	sql.WriteString(base)
	for _, j := range a.joins {
		next := j.Right
		if !seen[j.Left] && seen[j.Right] && j.Type == "inner" {
			next = j.Left
		} else if !seen[j.Left] || seen[j.Right] {
			return "", analyticalFailure("analytical_join_unsupported", true)
		}
		target, err := relation(next)
		if err != nil {
			return "", err
		}
		switch j.Type {
		case "inner":
			sql.WriteString(" INNER JOIN ")
		case "left":
			sql.WriteString(" LEFT JOIN ")
		default:
			return "", ErrBinding
		}
		sql.WriteString(target)
		sql.WriteString(" ON ")
		if len(j.LeftColumns) == 0 || len(j.LeftColumns) != len(j.RightColumns) {
			return "", ErrBinding
		}
		for i, left := range j.LeftColumns {
			if i > 0 {
				sql.WriteString(" AND ")
			}
			sql.WriteString(quote(aliases[j.Left]) + "." + quote(left) + "=" + quote(aliases[j.Right]) + "." + quote(j.RightColumns[i]))
		}
		seen[next] = true
	}
	if sql.Len() > 32<<10 {
		return "", ErrLimit
	}
	return sql.String(), nil
}
