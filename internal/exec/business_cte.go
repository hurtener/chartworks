package exec

import (
	"context"
	"strings"
)

// businessBindingLayout admits only independent, flat PostgreSQL CTEs. Every
// declared CTE must feed the final SELECT exactly once. This is deliberately a
// smaller language than the read validator: the binder must prove where a row
// predicate belongs before the validator can prove the resulting query safe.
func businessBindingLayout(ctx context.Context, statement string, tokens []businessToken, binding Binding, constraints []BusinessConstraint) (businessSQLLayout, error) {
	if len(tokens) == 0 || !tokens[0].word("with") {
		return businessLayout(statement, tokens, binding)
	}
	bad := businessSQLLayout{}
	if binding.Dialect != "postgres" || len(tokens) > 4096 {
		return bad, businessSQLFailure("unsupported_select_shape")
	}
	i := 1
	names := map[string]bool{}
	uses := map[string]int{}
	var scopes []businessSQLLayout
	for {
		if err := ctx.Err(); err != nil {
			return bad, err
		}
		if i+4 >= len(tokens) || tokens[i].depth != 0 || tokens[i].kind != 'w' {
			return bad, businessSQLFailure("unsupported_select_shape")
		}
		name, ok := businessName(tokens[i], binding.Dialect)
		if !ok || names[name] || !tokens[i+1].word("as") || tokens[i+2].text != "(" || tokens[i+2].depth != 0 {
			return bad, businessSQLFailure("unsupported_select_shape")
		}
		for _, relation := range binding.Relations {
			if strings.EqualFold(name, relation.Name) {
				return bad, businessSQLFailure("unsupported_select_shape")
			}
		}
		open := i + 2
		close := open + 1
		for close < len(tokens) && !(tokens[close].text == ")" && tokens[close].depth == 0) {
			close++
		}
		if close >= len(tokens) || close == open+1 || !tokens[open+1].word("select") || tokens[open+1].depth != 1 {
			return bad, businessSQLFailure("unsupported_select_shape")
		}
		start, end := tokens[open+1].start, tokens[close].start
		body := statement[start:end]
		bodyTokens, err := businessScan(ctx, body, false)
		if err != nil {
			return bad, err
		}
		scope, err := businessLayoutWithVirtual(body, bodyTokens, binding, names)
		if err != nil {
			return bad, err
		}
		localUses := map[string]bool{}
		for _, source := range scope.ranges {
			if strings.HasPrefix(source.relation.ID, "__cte_") {
				dependency := strings.TrimPrefix(source.relation.ID, "__cte_")
				if localUses[dependency] {
					return bad, businessSQLFailure("unsupported_select_shape")
				}
				localUses[dependency] = true
				uses[dependency]++
			}
		}
		names[name] = true
		scope.where, scope.whereEnd = shiftedClause(scope.where, start), shiftedClause(scope.whereEnd, start)
		scope.having, scope.havingEnd = shiftedClause(scope.having, start), shiftedClause(scope.havingEnd, start)
		scope.rowInsert += start
		scope.aggregateInsert += start
		scopes = append(scopes, scope)
		if len(scopes) > 4 {
			return bad, businessSQLFailure("unsupported_select_shape")
		}
		i = close + 1
		if i < len(tokens) && tokens[i].text == "," && tokens[i].depth == 0 {
			i++
			continue
		}
		break
	}
	if i >= len(tokens) || !tokens[i].word("select") || tokens[i].depth != 0 {
		return bad, businessSQLFailure("unsupported_select_shape")
	}
	// The final SELECT may read only the declared CTEs, each once. In
	// particular, an unused filter-bearing CTE cannot create binding evidence.
	seen := map[string]bool{}
	from := false
	for j := i; j < len(tokens); j++ {
		t := tokens[j]
		if j > i && t.word("select") || t.word("with") || t.word("union") || t.word("intersect") || t.word("except") || t.word("recursive") || t.word("into") || t.word("lateral") || t.text == ";" && j != len(tokens)-1 {
			return bad, businessSQLFailure("unsupported_select_shape")
		}
		if t.depth != 0 {
			continue
		}
		if t.word("from") || t.word("join") || t.text == "," && from {
			if j+1 >= len(tokens) || tokens[j+1].depth != 0 {
				return bad, businessSQLFailure("unsupported_select_shape")
			}
			name, ok := businessName(tokens[j+1], binding.Dialect)
			if !ok || !names[name] || seen[name] {
				return bad, businessSQLFailure("unsupported_select_shape")
			}
			seen[name] = true
			uses[name]++
			from = true
		}
		if t.word("where") || t.word("group") || t.word("having") || t.word("order") || t.word("limit") || t.word("offset") || t.word("fetch") {
			from = false
		}
	}
	for name := range names {
		if uses[name] < 1 {
			return bad, businessSQLFailure("unsupported_select_shape")
		}
	}
	var target businessSQLLayout
	matchCount := 0
	targetScope := -1
	for scopeIndex, scope := range scopes {
		for _, source := range scope.ranges {
			for _, constraint := range constraints {
				if source.relation.ID == constraint.Dataset {
					if targetScope >= 0 && targetScope != scopeIndex {
						return bad, businessSQLFailure("unsupported_missing_or_ambiguous_target")
					}
					targetScope = scopeIndex
					matchCount++
					target = scope
				}
			}
		}
	}
	if matchCount != len(constraints) || len(constraints) == 0 {
		return bad, businessSQLFailure("unsupported_missing_or_ambiguous_target")
	}
	return target, nil
}

func shiftedClause(position, offset int) int {
	if position < 0 {
		return position
	}
	return position + offset
}
