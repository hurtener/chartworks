package exec

import (
	"sort"
	"strings"
)

// AnalyticalGroupedPopulationsVersion keeps grouped alignment out of retained v6.
const AnalyticalGroupedPopulationsVersion = "analytical-metrics-v7"

// AnalyticalGroupedPopulationPolicy preserves the union of group keys, matches
// NULL keys as a group, and leaves missing aggregate values NULL.
const AnalyticalGroupedPopulationPolicy = "union-null-equal-preserve-missing-v1"

// AnalyticalGroupedPopulations is compiled only from reviewed population policy.
// Each lane is independently aggregated before any cross-population alignment.
type AnalyticalGroupedPopulations struct {
	Policy string                  `json:"policy"`
	Lanes  []AnalyticalGroupedLane `json:"lanes"`
}

type AnalyticalGroupedLane struct {
	Domain  string           `json:"domain,omitempty"`
	Dataset string           `json:"dataset"`
	Joins   []AnalyticalJoin `json:"joins,omitempty"`
}

func analyticalColumnDataset(base, name string) string {
	if parts := strings.SplitN(name, "/", 2); len(parts) == 2 {
		return parts[0]
	}
	return base
}

func rebaseGroupedColumn(base, target, name string) string {
	if name == "" {
		return ""
	}
	id := base
	if parts := strings.SplitN(name, "/", 2); len(parts) == 2 {
		id, name = parts[0], parts[1]
	}
	return AnalyticalColumnName(target, id, name)
}

func rebaseGroupedExpression(e AnalyticalExpression, base, target string) AnalyticalExpression {
	e.Column = rebaseGroupedColumn(base, target, e.Column)
	e.Filters = append([]AnalyticalFilter(nil), e.Filters...)
	for i := range e.Filters {
		e.Filters[i].Column = rebaseGroupedColumn(base, target, e.Filters[i].Column)
	}
	e.Args = append([]AnalyticalExpression(nil), e.Args...)
	for i := range e.Args {
		e.Args[i] = rebaseGroupedExpression(e.Args[i], base, target)
	}
	return e
}

func groupedLeaves(c AnalyticalContract, dataset string) []AnalyticalExpression {
	var leaves []AnalyticalExpression
	nodes := 0
	var visit func(AnalyticalExpression, int) bool
	visit = func(e AnalyticalExpression, depth int) bool {
		nodes++
		if depth > 32 || nodes > 1024 {
			return false
		}
		if e.Column != "" && analyticalColumnDataset(c.Dataset, e.Column) == dataset {
			leaves = append(leaves, e)
		}
		for _, arg := range e.Args {
			if !visit(arg, depth+1) {
				return false
			}
		}
		return true
	}
	for _, m := range c.Metrics {
		if !visit(m.Expression, 0) {
			return nil
		}
	}
	return leaves
}

// analyticalGroupedRelation verifies per-lane physical keys before making a
// detached contract-wide namespace. It never validates a raw cross-fact join.
func analyticalGroupedRelation(c AnalyticalContract, b Binding, base Relation) (Relation, error) {
	g := c.GroupedPopulations
	if (c.Version != AnalyticalGroupedPopulationsVersion && c.Version != AnalyticalGroupedProgramsVersion) || (b.Dialect != "postgres" && !(b.Dialect == "mysql" && c.Version == AnalyticalGroupedProgramsVersion)) || g == nil || g.Policy != AnalyticalGroupedPopulationPolicy || (len(g.Lanes) < 2 || len(g.Lanes) > 4) || len(c.Joins)+len(c.Populations) != 0 || c.Grain == nil || len(c.Grain.Columns)+len(c.Grain.Buckets) < 1 || c.Version == AnalyticalGroupedPopulationsVersion && len(c.Grain.Buckets) != 0 || c.QueryPopulation != nil && len(c.QueryPopulation.Constraints) > 0 {
		return Relation{}, ErrBinding
	}
	all := map[string]Column{}
	hasBase := false
	facts := map[string]bool{}
	for i, lane := range g.Lanes {
		if lane.Dataset == "" || i > 0 && g.Lanes[i-1].Dataset >= lane.Dataset {
			return Relation{}, ErrBinding
		}
		facts[lane.Dataset] = true
	}
	for _, lane := range g.Lanes {
		var root Relation
		for _, r := range b.Relations {
			if r.ID == lane.Dataset {
				root = r
			}
		}
		if root.ID == "" {
			return Relation{}, ErrBinding
		}
		hasBase = hasBase || root.ID == base.ID
		if c.Version == AnalyticalGroupedPopulationsVersion && lane.Domain != "" || lane.Domain != "" && lane.Domain != AnalyticalGroupDomainRaw && lane.Domain != AnalyticalGroupDomainQualifying {
			return Relation{}, ErrBinding
		}
		leaves := groupedLeaves(c, lane.Dataset)
		if len(leaves) == 0 {
			return Relation{}, ErrBinding
		}
		local := AnalyticalContract{Version: AnalyticalIntentVersion, Dataset: lane.Dataset, Joins: lane.Joins}
		for i, leaf := range leaves {
			local.Metrics = append(local.Metrics, AnalyticalMetric{ID: string(rune('a' + i)), Expression: rebaseGroupedExpression(leaf, c.Dataset, lane.Dataset)})
		}

		joined, err := analyticalJoinRelation(local, b, root)
		if err != nil {
			return Relation{}, err
		}
		available := map[string]bool{}
		for _, col := range joined.Columns {
			col.Name = rebaseGroupedColumn(lane.Dataset, c.Dataset, col.Name)
			available[col.Name] = true
			old, ok := all[col.Name]
			if ok && (old.NativeType != col.NativeType || old.Category != col.Category) {
				return Relation{}, ErrBinding
			}
			col.Nullable = col.Nullable || old.Nullable
			all[col.Name] = col
		}
		for _, column := range c.Grain.Columns {
			if !available[column] {
				return Relation{}, analyticalFailure("analytical_grain_mismatch", false)
			}
		}
		for _, bucket := range c.Grain.Buckets {
			if !available[bucket.Column] {
				return Relation{}, analyticalFailure("analytical_grain_mismatch", false)
			}
		}
		for _, leaf := range leaves {
			for _, filter := range leaf.Filters {
				if !available[filter.Column] {
					return Relation{}, ErrBinding
				}
			}
		}
	}
	if !hasBase {
		return Relation{}, ErrBinding
	}
	var visit func(AnalyticalExpression) bool
	visit = func(e AnalyticalExpression) bool {
		if e.Column != "" && !facts[analyticalColumnDataset(c.Dataset, e.Column)] {
			return false
		}
		for _, arg := range e.Args {
			if !visit(arg) {
				return false
			}
		}
		return true
	}
	for _, m := range c.Metrics {
		if !visit(m.Expression) {
			return Relation{}, ErrBinding
		}
	}
	out := base
	out.Columns, out.UniqueKeys = nil, nil
	for _, col := range all {
		out.Columns = append(out.Columns, col)
	}
	sort.Slice(out.Columns, func(i, j int) bool { return out.Columns[i].Name < out.Columns[j].Name })
	return out, nil
}

// ValidateAnalyticalGroupedPopulations checks the compiler's source-backed lanes.
func ValidateAnalyticalGroupedPopulations(c AnalyticalContract, b Binding) error {
	for _, r := range b.Relations {
		if r.ID == c.Dataset {
			_, err := analyticalGroupedRelation(c, b, r)
			return err
		}
	}
	return ErrBinding
}

type analyticalGroupedOutput struct {
	dataset string
	terms   map[string]analyticalTerm
	groups  map[string]analyticalTerm // SQL output name -> exact reviewed grouping term
}

func (a *analyticalChecker) queryGroupedPopulations(q map[string]any, expected map[string]int, policy *AnalyticalGroupedPopulations) error {
	if a.groupedExtensions {
		if handled, err := a.groupedFinalProjection(q, expected, policy); handled {
			return err
		}
	}
	fail := func() error { return analyticalFailure("analytical_population_mismatch", false) }
	if !only(q, "targetList", "fromClause", "withClause", "sortClause", "limitOffset", "limitCount", "limitOption", "op") || text(q["op"]) != "" && text(q["op"]) != "SETOP_NONE" {
		return fail()
	}
	var inlineSpine map[string]any
	if a.groupedExtensions {
		sources := array(q["fromClause"])
		if len(sources) == 1 {
			raw := sources[0]
			for depth := 0; depth < 4; depth++ {
				j := object(object(raw)["JoinExpr"])
				if j == nil {
					break
				}
				raw = j["larg"]
			}
			if query, _, ok := groupedDerivedSource(raw); ok {
				inlineSpine = query
			}
		}
	}
	cteCount := len(policy.Lanes) + 1
	if inlineSpine != nil {
		cteCount--
	}
	with := object(q["withClause"])
	if with == nil || truth(with["recursive"]) || !only(with, "ctes", "recursive", "location") || len(array(with["ctes"])) != cteCount {
		return fail()
	}
	ctes := map[string]map[string]any{}
	var spine string
	if inlineSpine != nil {
		spine = "\x00spine"
	}
	lanes := map[string]analyticalGroupedOutput{}
	used := map[string]bool{}
	for _, raw := range array(with["ctes"]) {
		cte := fieldObject(raw, "CommonTableExpr")
		name := text(cte["ctename"])
		if name == "" || ctes[name] != nil || !only(cte, "ctename", "ctematerialized", "ctequery", "location") {
			return fail()
		}
		query := fieldObject(cte["ctequery"], "SelectStmt")
		if query == nil {
			return fail()
		}
		ctes[name] = query
		if text(query["op"]) == "SETOP_UNION" {
			if spine != "" {
				return fail()
			}
			spine = name
			continue
		}
		out, err := a.groupedLane(query, policy, used)
		if err != nil {
			return err
		}
		lanes[name] = out
	}
	if spine == "" || len(lanes) != len(policy.Lanes) || len(used) != len(policy.Lanes) {
		return fail()
	}
	spineQuery := ctes[spine]
	if inlineSpine != nil {
		spineQuery = inlineSpine
	}
	keys, err := a.groupedSpine(spineQuery, lanes)
	if err != nil {
		return err
	}
	// Exact left-deep spine LEFT JOIN lane LEFT JOIN lane. Requiring the spine
	// key on each edge prevents a missing first lane from discarding later groups.
	from := array(q["fromClause"])
	if len(from) != 1 {
		return fail()
	}
	aliases := map[string]string{}
	joined := map[string]bool{}
	var read func(any) error
	read = func(raw any) error {
		if inlineSpine != nil && len(aliases) == 0 {
			if query, alias, ok := groupedDerivedSource(raw); ok {
				if Hash(query) != Hash(inlineSpine) {
					return fail()
				}
				aliases[alias] = spine
				return nil
			}
		}
		if rv := object(object(raw)["RangeVar"]); rv != nil {
			name, alias, ok := groupedCTERef(rv)
			if !ok || name != spine || len(aliases) != 0 {
				return fail()
			}
			aliases[alias] = spine
			return nil
		}
		j := object(object(raw)["JoinExpr"])
		if j == nil || text(j["jointype"]) != "JOIN_LEFT" || !only(j, "jointype", "larg", "rarg", "quals", "rtindex") {
			return fail()
		}
		if err := read(j["larg"]); err != nil {
			return err
		}
		rv := object(object(j["rarg"])["RangeVar"])
		name, alias, ok := groupedCTERef(rv)
		lane, present := lanes[name]
		if !ok || !present || joined[name] || aliases[alias] != "" {
			return fail()
		}
		aliases[alias] = name
		joined[name] = true
		conditions := analyticalConjuncts(j["quals"])
		if len(conditions) != len(keys) {
			return fail()
		}
		matched := map[string]bool{}
		for _, condition := range conditions {
			expr := fieldObject(condition, "A_Expr")
			ops, yes := names(expr["name"])
			if !yes || len(ops) != 1 || ops[0] != "=" || text(expr["kind"]) != "AEXPR_NOT_DISTINCT" {
				return fail()
			}
			left, lok := groupedQualifiedColumn(expr["lexpr"])
			right, rok := groupedQualifiedColumn(expr["rexpr"])
			if !lok || !rok {
				return fail()
			}
			if aliases[right[0]] == spine {
				left, right = right, left
			}
			key := keys[left[1]].groupKey()
			if aliases[left[0]] != spine || right[0] != alias || key == "" || lane.groups[right[1]].groupKey() != key || matched[key] {
				return fail()
			}
			matched[key] = true
		}
		return nil
	}
	if err := read(from[0]); err != nil {
		return err
	}
	if len(joined) != len(policy.Lanes) {
		return fail()
	}
	outer := *a
	outer.derivedTerms = map[string]analyticalTerm{}
	outer.joinAliases = nil
	// No base-relation reference can escape its aggregate lane into the outer SELECT.
	outer.relation.Columns = nil
	for alias, name := range aliases {
		if name == spine {
			for name, term := range keys {
				if !a.groupedExtensions {
					term = analyticalTerm{column: term.column}
				}
				outer.derivedTerms[alias+"."+name] = term
			}
			continue
		}
		for name, term := range lanes[name].terms {
			if term.aggregate {
				outer.derivedTerms[alias+"."+name] = term
			}
		}
	}
	targets := array(q["targetList"])
	if len(targets) < 1 || len(targets) > 48 {
		return ErrLimit
	}
	terms := make([]analyticalTerm, len(targets))
	outputs := map[string]analyticalTerm{}
	matched := map[string]bool{}
	groups := map[string]bool{}
	for i, raw := range targets {
		t := fieldObject(raw, "ResTarget")
		term, err := outer.term(t["val"], 0)
		if err != nil {
			return err
		}
		if term.guarded {
			return fail()
		}
		if term.aggregate {
			if expected[term.key] == 0 {
				return analyticalFailure("analytical_metric_mismatch", false)
			}
			matched[term.key] = true
		} else if term.groupKey() != "" {
			groups[term.groupKey()] = true
		} else {
			return fail()
		}
		terms[i] = term
		name := text(t["name"])
		if a.groupedExtensions {
			name = groupedOutputName(t)
		}
		if name != "" {
			if _, exists := outputs[name]; exists {
				return fail()
			}
			outputs[name] = term
		}
	}
	for key := range expected {
		if !matched[key] {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
	}
	if err := outer.checkGrain(groups, terms); err != nil {
		return err
	}
	if err := outer.checkIntent(q, terms, outputs); err != nil {
		return err
	}
	a.groupedOutput = outputs
	return a.ctx.Err()
}

func groupedQualifiedColumn(node any) ([2]string, bool) {
	parts, ok := names(fieldObject(node, "ColumnRef")["fields"])
	if !ok || len(parts) != 2 {
		return [2]string{}, false
	}
	return [2]string{parts[0], parts[1]}, true
}
func groupedCTERef(rv map[string]any) (string, string, bool) {
	if rv == nil || text(rv["schemaname"]) != "" || !truth(rv["inh"]) || !only(rv, "relname", "inh", "relpersistence", "alias", "location") {
		return "", "", false
	}
	name := text(rv["relname"])
	alias := name
	if av := object(rv["alias"]); av != nil {
		v := fieldObject(av, "Alias")
		if !only(v, "aliasname") {
			return "", "", false
		}
		alias = text(v["aliasname"])
	}
	return name, alias, name != "" && alias != ""
}

func (a *analyticalChecker) groupedLane(q map[string]any, policy *AnalyticalGroupedPopulations, used map[string]bool) (analyticalGroupedOutput, error) {
	if a.groupedExtensions {
		if proof, handled, err := a.groupedLaneProjection(q, policy, used); handled {
			return proof, err
		}
	}
	fail := func() (analyticalGroupedOutput, error) {
		return analyticalGroupedOutput{}, analyticalFailure("analytical_population_mismatch", false)
	}
	if !only(q, "targetList", "fromClause", "whereClause", "groupClause", "limitOption", "op") || text(q["limitOption"]) != "" && text(q["limitOption"]) != "LIMIT_OPTION_DEFAULT" {
		return fail()
	}
	from := array(q["fromClause"])
	if len(from) != 1 {
		return fail()
	}
	raw := from[0]
	for depth := 0; depth < 4; depth++ {
		j := object(object(raw)["JoinExpr"])
		if j == nil {
			break
		}
		raw = j["larg"]
	}
	rv := fieldObject(raw, "RangeVar")
	id := ""
	for _, r := range a.binding.Relations {
		if r.Schema == text(rv["schemaname"]) && r.Name == text(rv["relname"]) {
			id = r.ID
		}
	}
	var lane *AnalyticalGroupedLane
	for i := range policy.Lanes {
		if policy.Lanes[i].Dataset == id {
			lane = &policy.Lanes[i]
		}
	}
	if lane == nil || used[id] {
		return fail()
	}
	used[id] = true
	local := *a
	local.derivedTerms = nil
	local.leaves = nil
	local.intent = nil
	local.nodes = 0
	local.groupedLaneProof = true
	local.groupedDomain = lane.Domain
	local.groupedDomainProved = false
	local.joins = lane.Joins
	local.joinAliases = map[string]string{}
	local.joinUsed = map[int]bool{}
	local.populationSource = id
	local.alias = text(rv["relname"])
	if av := object(rv["alias"]); av != nil {
		local.alias = text(fieldObject(av, "Alias")["aliasname"])
	}
	local.relation.Schema = text(rv["schemaname"])
	local.relation.Name = text(rv["relname"])
	local.queryPopulation = &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy}
	if len(lane.Joins) == 0 {
		alias := text(rv["relname"])
		if av := object(rv["alias"]); av != nil {
			alias = text(fieldObject(av, "Alias")["aliasname"])
		}
		local.joinAliases[alias] = id
	}
	want := map[string]int{}
	for _, leaf := range a.leaves {
		if analyticalColumnDataset(a.relation.ID, leaf.Column) == id {
			key, err := local.expected(leaf, 0)
			if err != nil {
				return analyticalGroupedOutput{}, err
			}
			want[key]++
		}
	}
	if len(want) == 0 {
		return fail()
	}
	if err := local.query(q, want); err != nil {
		return analyticalGroupedOutput{}, err
	}
	out := analyticalGroupedOutput{dataset: id, terms: map[string]analyticalTerm{}, groups: map[string]analyticalTerm{}}
	for _, raw := range array(q["targetList"]) {
		target := fieldObject(raw, "ResTarget")
		name := text(target["name"])
		if name == "" {
			return fail()
		}
		if _, exists := out.terms[name]; exists {
			return fail()
		}
		term, err := local.term(target["val"], 0)
		if err != nil {
			return analyticalGroupedOutput{}, err
		}
		out.terms[name] = term
		if term.groupKey() != "" {
			out.groups[name] = term
		}
	}
	return out, nil
}

func (a *analyticalChecker) groupedSpine(q map[string]any, lanes map[string]analyticalGroupedOutput) (map[string]analyticalTerm, error) {
	fail := func() (map[string]analyticalTerm, error) {
		return nil, analyticalFailure("analytical_population_mismatch", false)
	}
	if text(q["op"]) != "SETOP_UNION" || truth(q["all"]) || !only(q, "op", "all", "larg", "rarg", "limitOption") {
		return fail()
	}
	var ordered []string
	outputs := map[string]analyticalTerm{}
	seen := map[string]bool{}
	var branches []map[string]any
	var flatten func(map[string]any, int) bool
	flatten = func(node map[string]any, depth int) bool {
		if depth > 4 {
			return false
		}
		if text(node["op"]) == "SETOP_UNION" {
			if truth(node["all"]) || !only(node, "op", "all", "larg", "rarg", "limitOption") {
				return false
			}
			return flatten(object(node["larg"]), depth+1) && flatten(object(node["rarg"]), depth+1)
		}
		branches = append(branches, node)
		return true
	}
	if !flatten(q, 0) || len(branches) != len(lanes) {
		return fail()
	}
	for branch, query := range branches {
		if !only(query, "targetList", "fromClause", "op", "limitOption") || text(query["op"]) != "" && text(query["op"]) != "SETOP_NONE" {
			return fail()
		}
		from := array(query["fromClause"])
		if len(from) != 1 {
			return fail()
		}
		name, alias, ok := groupedCTERef(fieldObject(from[0], "RangeVar"))
		lane, present := lanes[name]
		if !ok || !present || seen[name] {
			return fail()
		}
		seen[name] = true
		targets := array(query["targetList"])
		if len(targets) != len(a.grain.Columns)+len(a.grain.Buckets) {
			return fail()
		}
		selected := map[string]bool{}
		for i, raw := range targets {
			target := fieldObject(raw, "ResTarget")
			parts, ok := names(fieldObject(target["val"], "ColumnRef")["fields"])
			if !ok || len(parts) < 1 || len(parts) > 2 || len(parts) == 2 && parts[0] != alias {
				return fail()
			}
			term := lane.groups[parts[len(parts)-1]]
			key := term.groupKey()
			if key == "" || selected[key] {
				return fail()
			}
			selected[key] = true
			if branch == 0 {
				ordered = append(ordered, key)
				outName := text(target["name"])
				if outName == "" {
					outName = parts[len(parts)-1]
				}
				if outputs[outName].groupKey() != "" {
					return fail()
				}
				outputs[outName] = term
			} else if ordered[i] != key {
				return fail()
			}
		}
	}
	return outputs, nil
}
