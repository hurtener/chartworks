package exec

import (
	"context"
	"sort"
)

// AnalyticalScopedPopulationsVersion preserves v1-v8 replay while adding
// fact-owned scalar populations with separately proved joins and predicates.
const AnalyticalScopedPopulationsVersion = "analytical-metrics-v9"

const AnalyticalScalarPopulationPolicy = "scoped-singleton-populations-v1"

// AnalyticalScalarPopulations is protected compiler output, not model-selected
// placement. QueryPopulation contains current private values and must never be
// marshaled into generation guidance.
type AnalyticalScalarPopulations struct {
	Policy string                 `json:"policy"`
	Lanes  []AnalyticalScalarLane `json:"lanes"`
}

type AnalyticalScalarLane struct {
	Dataset         string                     `json:"dataset"`
	Joins           []AnalyticalJoin           `json:"joins,omitempty"`
	QueryPopulation *AnalyticalQueryPopulation `json:"query_population"`
}

func (AnalyticalScalarPopulations) String() string     { return "analytical-scalar-populations(redacted)" }
func (p AnalyticalScalarPopulations) GoString() string { return p.String() }

// Each fact is proved independently. A relation used only by a filter or a
// period remains a join dependency; it never creates another aggregate lane.
func analyticalScalarRelation(ctx context.Context, c AnalyticalContract, b Binding, base Relation) (Relation, error) {
	p := c.ScalarPopulations
	if c.Version != AnalyticalScopedPopulationsVersion || b.Dialect != "postgres" || p == nil || p.Policy != AnalyticalScalarPopulationPolicy || len(p.Lanes) < 2 || len(p.Lanes) > 4 || c.GroupedPopulations != nil || len(c.Joins)+len(c.Populations) != 0 || c.Grain != nil && len(c.Grain.Columns)+len(c.Grain.Buckets) != 0 || c.QueryPopulation == nil || c.QueryPopulation.Policy != AnalyticalQueryPopulationPolicy || len(c.QueryPopulation.Constraints) != 0 {
		return Relation{}, ErrBinding
	}
	facts := map[string]bool{}
	columns := map[string]Column{}
	for i, lane := range p.Lanes {
		if lane.Dataset == "" || i > 0 && p.Lanes[i-1].Dataset >= lane.Dataset || lane.QueryPopulation == nil || len(lane.QueryPopulation.Constraints) != 1 {
			return Relation{}, ErrBinding
		}
		for _, constraint := range lane.QueryPopulation.Constraints {
			if constraint.Kind != "time_window" || constraint.Aggregation != "" {
				return Relation{}, ErrBinding
			}
		}
		for _, join := range lane.Joins {
			if join.Type != "inner" {
				return Relation{}, analyticalFailure("analytical_shape_unsupported", true)
			}
		}
		facts[lane.Dataset] = true
		var root Relation
		for _, r := range b.Relations {
			if r.ID == lane.Dataset {
				root = r
			}
		}
		if root.ID == "" {
			return Relation{}, ErrBinding
		}
		leaves := groupedLeaves(c, lane.Dataset)
		if len(leaves) == 0 {
			return Relation{}, ErrBinding
		}
		local := AnalyticalContract{Version: AnalyticalGroupedProgramsVersion, Dataset: lane.Dataset, Joins: lane.Joins, QueryPopulation: lane.QueryPopulation}
		for i, leaf := range leaves {
			local.Metrics = append(local.Metrics, AnalyticalMetric{ID: string(rune('a' + i)), Expression: rebaseGroupedExpression(leaf, c.Dataset, lane.Dataset)})
		}
		joined, err := analyticalJoinRelation(local, b, root)
		if err != nil {
			return Relation{}, err
		}
		if err := validateAnalyticalQueryPopulation(ctx, local, b); err != nil {
			return Relation{}, err
		}
		for _, col := range joined.Columns {
			col.Name = rebaseGroupedColumn(lane.Dataset, c.Dataset, col.Name)
			prior, ok := columns[col.Name]
			if ok && (prior.NativeType != col.NativeType || prior.Category != col.Category) {
				return Relation{}, ErrBinding
			}
			col.Nullable = col.Nullable || prior.Nullable
			columns[col.Name] = col
		}
	}
	if !facts[c.Dataset] {
		return Relation{}, ErrBinding
	}
	var validLeaves func(AnalyticalExpression) bool
	validLeaves = func(e AnalyticalExpression) bool {
		if e.Column != "" && !facts[analyticalColumnDataset(c.Dataset, e.Column)] {
			return false
		}
		for _, arg := range e.Args {
			if !validLeaves(arg) {
				return false
			}
		}
		return true
	}
	for _, metric := range c.Metrics {
		if !validLeaves(metric.Expression) {
			return Relation{}, ErrBinding
		}
	}
	out := base
	out.Columns, out.UniqueKeys = nil, nil
	for _, col := range columns {
		out.Columns = append(out.Columns, col)
	}
	sort.Slice(out.Columns, func(i, j int) bool { return out.Columns[i].Name < out.Columns[j].Name })
	return out, nil
}

func (a *analyticalChecker) scopedSingletonLane(q map[string]any, alias string, used map[string]bool) error {
	if !only(q, "targetList", "fromClause", "whereClause", "limitOption", "op") || text(q["limitOption"]) != "" && text(q["limitOption"]) != "LIMIT_OPTION_DEFAULT" || text(q["op"]) != "" && text(q["op"]) != "SETOP_NONE" {
		return analyticalFailure("analytical_shape_unsupported", true)
	}
	from := array(q["fromClause"])
	if len(from) != 1 {
		return ErrBinding
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
	var lane *AnalyticalScalarLane
	for i := range a.scalarPopulations.Lanes {
		if a.scalarPopulations.Lanes[i].Dataset == id {
			lane = &a.scalarPopulations.Lanes[i]
		}
	}
	if lane == nil || used[id] {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	used[id] = true
	local := *a
	local.derivedTerms = nil
	local.leaves = nil
	local.grain = nil
	local.intent = nil
	local.ordinaryGroupProof = false
	local.groupedLaneProof = false
	local.nodes = 0
	local.joins = lane.Joins
	local.joinAliases = map[string]string{}
	local.joinUsed = map[int]bool{}
	local.queryPopulation = lane.QueryPopulation
	local.populationSource = id
	local.relation.Schema, local.relation.Name = text(rv["schemaname"]), text(rv["relname"])
	local.alias = local.relation.Name
	if av := object(rv["alias"]); av != nil {
		local.alias = text(fieldObject(av, "Alias")["aliasname"])
	}
	if len(lane.Joins) == 0 {
		local.joinAliases[local.alias] = id
	}
	want := map[string]int{}
	for _, leaf := range a.leaves {
		if analyticalColumnDataset(a.relation.ID, leaf.Column) == id {
			key, err := local.expected(leaf, 0)
			if err != nil {
				return err
			}
			want[key]++
		}
	}
	if len(want) == 0 {
		return analyticalFailure("analytical_population_mismatch", false)
	}
	if err := local.query(q, want); err != nil {
		return err
	}
	for _, raw := range array(q["targetList"]) {
		target := fieldObject(raw, "ResTarget")
		name := text(target["name"])
		if name == "" {
			return analyticalFailure("analytical_output_ambiguous", true)
		}
		term, err := local.term(target["val"], 0)
		if err != nil {
			return err
		}
		if !term.aggregate || term.guarded {
			return analyticalFailure("analytical_metric_mismatch", false)
		}
		key := alias + "." + name
		if _, ok := a.derivedTerms[key]; ok {
			return ErrBinding
		}
		a.derivedTerms[key] = term
	}
	return nil
}

// V9 and v10 compose unchanged v8 standard proofs with separately gated
// ownership proofs. Keep c itself intact for the original contract digest.
func analyticalStandardPolicy(c AnalyticalContract) AnalyticalContract {
	if c.Version == AnalyticalScopedPopulationsVersion || c.Version == AnalyticalGroupedOwnedPopulationsVersion {
		c.Version = AnalyticalGroupedProgramsVersion
	}
	return c
}

func ValidateAnalyticalScalarPopulations(ctx context.Context, c AnalyticalContract, b Binding) error {
	for _, r := range b.Relations {
		if r.ID == c.Dataset {
			_, err := analyticalScalarRelation(ctx, c, b, r)
			return err
		}
	}
	return ErrBinding
}
