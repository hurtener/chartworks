package exec

import (
	"sort"
	"strings"
)

// AnalyticalJoin records reviewed equality/population semantics. Physical source
// keys, rather than reviewed cardinality labels, prove absence of multiplication.
type AnalyticalJoin struct {
	Left         string   `json:"left"`
	Right        string   `json:"right"`
	Type         string   `json:"type"`
	LeftColumns  []string `json:"left_columns"`
	RightColumns []string `json:"right_columns"`
}

func AnalyticalColumnName(base, dataset, column string) string {
	if base == dataset {
		return column
	}
	return dataset + "/" + column
}

// analyticalJoinRelation builds an immutable local namespace, not a data-access
// binding. All native validation still uses the original restrictive source scope.
func analyticalJoinRelation(c AnalyticalContract, b Binding, base Relation) (Relation, error) {
	if len(c.Joins) == 0 {
		return base, nil
	}
	if (c.Version != AnalyticalIntentVersion && c.Version != AnalyticalGroupedPopulationsVersion) || len(c.Joins) > 3 {
		return Relation{}, ErrBinding
	}
	relations := map[string]Relation{}
	for _, r := range b.Relations {
		relations[r.ID] = r
	}
	joined := map[string]bool{base.ID: true}
	seen := map[string]bool{}
	nullable := map[string]bool{}
	for _, j := range c.Joins {
		left, lok := relations[j.Left]
		right, rok := relations[j.Right]
		if !lok || !rok || j.Left == j.Right || (j.Type != "inner" && j.Type != "left") || len(j.LeftColumns) == 0 || len(j.LeftColumns) > 16 || len(j.LeftColumns) != len(j.RightColumns) {
			return Relation{}, ErrBinding
		}
		key := j.Left + "\x00" + j.Right
		if seen[key] {
			return Relation{}, ErrBinding
		}
		seen[key] = true
		if joined[j.Left] == joined[j.Right] {
			return Relation{}, analyticalFailure("analytical_join_unsupported", true)
		}
		leftMetrics, rightMetrics := analyticalJoinMetricSides(c, j)
		if leftMetrics && !right.HasUniqueKey(j.RightColumns) || rightMetrics && !left.HasUniqueKey(j.LeftColumns) {
			return Relation{}, analyticalFailure("analytical_join_cardinality_unproven", true)
		}
		for i, name := range j.LeftColumns {
			lc, ok := analyticalRelationColumn(left, name)
			rc, ok2 := analyticalRelationColumn(right, j.RightColumns[i])
			if !ok || !ok2 || lc.NativeType != rc.NativeType || lc.Category != rc.Category {
				return Relation{}, ErrBinding
			}
		}
		joined[j.Left], joined[j.Right] = true, true
		if j.Type == "left" {
			nullable[j.Right] = true
		}
	}
	result := base
	result.Columns = nil
	result.UniqueKeys = nil
	ids := make([]string, 0, len(joined))
	for id := range joined {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		for _, col := range relations[id].Columns {
			col.Name = AnalyticalColumnName(base.ID, id, col.Name)
			col.Nullable = col.Nullable || nullable[id]
			result.Columns = append(result.Columns, col)
		}
	}
	return result, nil
}

func analyticalRelationColumn(r Relation, name string) (Column, bool) {
	for _, c := range r.Columns {
		if c.Name == name && c.Safe {
			return c, true
		}
	}
	return Column{}, false
}

func (a *analyticalChecker) joinSources(node any) (map[string]bool, error) {
	m := object(node)
	if rv := object(m["RangeVar"]); rv != nil {
		if !truth(rv["inh"]) {
			return nil, analyticalFailure("analytical_relation_mismatch", false)
		}
		var found Relation
		for _, r := range a.binding.Relations {
			if r.Schema == text(rv["schemaname"]) && r.Name == text(rv["relname"]) {
				found = r
			}
		}
		if found.ID == "" {
			return nil, ErrBinding
		}
		alias := found.Name
		if raw := object(rv["alias"]); raw != nil {
			v := fieldObject(raw, "Alias")
			if len(array(v["colnames"])) > 0 {
				return nil, analyticalFailure("analytical_join_unsupported", true)
			}
			alias = text(v["aliasname"])
		}
		if alias == "" || a.joinAliases[alias] != "" {
			return nil, analyticalFailure("analytical_join_unsupported", true)
		}
		for _, id := range a.joinAliases {
			if id == found.ID {
				return nil, analyticalFailure("analytical_join_unsupported", true)
			}
		}
		a.joinAliases[alias] = found.ID
		if found.ID == a.relation.ID {
			a.alias = alias
		}
		return map[string]bool{found.ID: true}, nil
	}
	j := object(m["JoinExpr"])
	if j == nil || object(object(j["rarg"])["RangeVar"]) == nil || !only(j, "jointype", "larg", "rarg", "quals", "rtindex") {
		return nil, analyticalFailure("analytical_join_unsupported", true)
	}
	left, err := a.joinSources(j["larg"])
	if err != nil {
		return nil, err
	}
	right, err := a.joinSources(j["rarg"])
	if err != nil {
		return nil, err
	}
	kind := ""
	switch text(j["jointype"]) {
	case "JOIN_INNER":
		kind = "inner"
	case "JOIN_LEFT":
		kind = "left"
	default:
		return nil, analyticalFailure("analytical_join_unsupported", true)
	}
	conditions := analyticalConjuncts(j["quals"])
	matched := -1
	for i, want := range a.joins {
		if a.joinUsed[i] || want.Type != kind {
			continue
		}
		forward := left[want.Left] && right[want.Right]
		reverse := kind == "inner" && left[want.Right] && right[want.Left]
		if (!forward && !reverse) || len(conditions) != len(want.LeftColumns) {
			continue
		}
		pairs := map[string]bool{}
		valid := true
		for _, condition := range conditions {
			op := fieldObject(condition, "A_Expr")
			names, ok := names(op["name"])
			if !ok || len(names) != 1 || names[0] != "=" || text(op["kind"]) != "AEXPR_OP" {
				valid = false
				break
			}
			ld, lc, lok := a.joinColumn(op["lexpr"])
			rd, rc, rok := a.joinColumn(op["rexpr"])
			if !lok || !rok {
				valid = false
				break
			}
			if ld == want.Right && rd == want.Left {
				ld, rd, lc, rc = rd, ld, rc, lc
			}
			if ld != want.Left || rd != want.Right {
				valid = false
				break
			}
			pair := lc + "\x00" + rc
			if pairs[pair] {
				valid = false
				break
			}
			pairs[pair] = true
		}
		for k := range want.LeftColumns {
			valid = valid && pairs[want.LeftColumns[k]+"\x00"+want.RightColumns[k]]
		}
		if valid {
			if matched >= 0 {
				return nil, ErrBinding
			}
			matched = i
		}
	}
	if matched < 0 || matched != len(a.joinUsed) {
		return nil, analyticalFailure("analytical_join_mismatch", false)
	}
	a.joinUsed[matched] = true
	for id := range right {
		if left[id] {
			return nil, ErrBinding
		}
		left[id] = true
	}
	return left, nil
}

func (a *analyticalChecker) joinColumn(node any) (string, string, bool) {
	parts, ok := names(fieldObject(node, "ColumnRef")["fields"])
	if !ok || len(parts) != 2 {
		return "", "", false
	}
	id := a.joinAliases[parts[0]]
	if id == "" {
		return "", "", false
	}
	for _, r := range a.binding.Relations {
		if r.ID == id {
			_, ok = analyticalRelationColumn(r, parts[1])
			return id, parts[1], ok
		}
	}
	return "", "", false
}

func (a *analyticalChecker) joinedField(node any) (Column, bool) {
	parts, ok := names(fieldObject(node, "ColumnRef")["fields"])
	if !ok {
		return Column{}, false
	}
	if len(parts) == 2 {
		id := a.joinAliases[parts[0]]
		if id == "" {
			return Column{}, false
		}
		return a.column(AnalyticalColumnName(a.relation.ID, id, parts[1]))
	}
	if len(parts) != 1 {
		return Column{}, false
	}
	var found Column
	count := 0
	for alias, id := range a.joinAliases {
		_ = alias
		if col, yes := a.column(AnalyticalColumnName(a.relation.ID, id, parts[0])); yes {
			found = col
			count++
		}
	}
	return found, count == 1
}

func analyticalJoinScope(scope string) string {
	return strings.ReplaceAll(scope, "single_base_relation", "physically_unique_reviewed_joins")
}

// ValidateAnalyticalJoins checks source-bound fan-out evidence before generation.
func ValidateAnalyticalJoins(c AnalyticalContract, b Binding) error {
	for _, r := range b.Relations {
		if r.ID == c.Dataset {
			_, err := analyticalJoinRelation(c, b, r)
			return err
		}
	}
	return ErrBinding
}

// An aggregate's input rows must never be duplicated by crossing an edge.
// Which endpoint needs a physical unique key depends on the actual metric
// inputs, not an untrusted many-to-one label or the SQL author's join direction.
func analyticalJoinMetricSides(c AnalyticalContract, edge AnalyticalJoin) (bool, bool) {
	left := map[string]bool{edge.Left: true}
	for round := 0; round < len(c.Joins); round++ {
		for _, j := range c.Joins {
			if j.Left == edge.Left && j.Right == edge.Right {
				continue
			}
			if left[j.Left] || left[j.Right] {
				left[j.Left], left[j.Right] = true, true
			}
		}
	}
	l, r := false, false
	var visit func(AnalyticalExpression)
	visit = func(e AnalyticalExpression) {
		if e.Column != "" {
			id := c.Dataset
			if parts := strings.SplitN(e.Column, "/", 2); len(parts) == 2 {
				id = parts[0]
			}
			if left[id] {
				l = true
			} else {
				r = true
			}
		}
		for _, a := range e.Args {
			visit(a)
		}
	}
	for _, m := range c.Metrics {
		visit(m.Expression)
	}
	if c.QueryPopulation != nil {
		for _, constraint := range c.QueryPopulation.Constraints {
			if constraint.Aggregation != "" {
				if left[constraint.Dataset] {
					l = true
				} else {
					r = true
				}
			}
		}
	}
	return l, r
}
