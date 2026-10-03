package nlqexec

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

const analyticalScalarEntailmentRecordVersion = 13

type scalarFilterOrigin struct {
	Kind   semantics.Kind
	Owner  string
	Filter semantics.SemanticFilter
}

type scalarExpressionOccurrence struct {
	Path []int
	Leaf exec.AnalyticalExpression
}

// The retained compiler is unchanged. A separate constructor proves that every
// current non-period constraint is already mandatory at every selected leaf.
func compileAnalyticalScalarEntailment(ctx context.Context, a admission, supplied [][]exec.BusinessConstraint) (*exec.AnalyticalContract, error) {
	if ctx == nil || len(supplied) != 1 || len(supplied[0]) == 0 || !hasActiveBusinessEvidence(a.route) || a.route.SourceBindingDigest != exec.Hash(a.binding) {
		return nil, exec.ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	base, err := compileAnalyticalVersion(ctx, a, analyticalGroupedOwnedRecordVersion, []exec.BusinessConstraint{})
	if err != nil || base == nil || base.ScalarPopulations == nil {
		// Preserve current grouped/ordinary dispatch, including every supplied
		// constraint. This path never returns a v13 receipt or partial discharge.
		return compileAnalyticalVersion(ctx, a, analyticalGroupedFactsRecordVersion, supplied[0])
	}
	if base.Version != exec.AnalyticalScopedPopulationsVersion || a.binding.Dialect != "postgres" || selectedKnownAmountCompleteness(a) {
		return nil, analyticalUnsupported("analytical_scalar_predicate_unsupported")
	}
	var datasets []string
	for _, scope := range a.relationScope {
		datasets = append(datasets, scope.Dataset)
	}
	canonical, err := exec.NewAnalyticalQueryPopulationWithin(ctx, a.binding, datasets, supplied[0])
	if err != nil {
		return nil, err
	}
	// Bound the complete selected occurrence set before constructing the product.
	occurrences := map[string][]scalarExpressionOccurrence{}
	nodes, leaves := 0, 0
	var collect func(string, exec.AnalyticalExpression, []int, int) error
	collect = func(metric string, e exec.AnalyticalExpression, path []int, depth int) error {
		nodes++
		if nodes > 1024 || depth > 32 {
			return exec.ErrLimit
		}
		if e.Column != "" {
			if (e.Op != "sum" && e.Op != "count") || len(e.Args) != 0 {
				return analyticalUnsupported("analytical_scalar_predicate_unsupported")
			}
			leaves++
			if leaves > 512/len(canonical.Constraints) {
				return exec.ErrLimit
			}
			occurrences[metric] = append(occurrences[metric], scalarExpressionOccurrence{Path: append([]int{}, path...), Leaf: e})
		}
		for i, arg := range e.Args {
			if err := collect(metric, arg, append(append([]int{}, path...), i), depth+1); err != nil {
				return err
			}
		}
		return ctx.Err()
	}
	for _, metric := range base.Metrics {
		if _, duplicate := occurrences[metric.ID]; duplicate {
			return nil, exec.ErrBinding
		}
		if err := collect(metric.ID, metric.Expression, []int{}, 0); err != nil {
			return nil, err
		}
	}
	if leaves == 0 {
		return nil, exec.ErrBinding
	}
	var witnesses []exec.AnalyticalScalarPredicateWitness
	seen := map[string]bool{}
	for i, pick := range a.route.Selection.Topics {
		pub := a.publications[i]
		for _, root := range pick.Roots {
			if root.Reason == "required_rule" || root.Reference.Kind != semantics.KindMeasure && root.Reference.Kind != semantics.KindKPI {
				continue
			}
			metricID := pick.Topic + ":" + string(root.Reference.Kind) + ":" + root.Reference.ID
			expected := occurrences[metricID]
			if len(expected) == 0 || seen[metricID] {
				return nil, exec.ErrBinding
			}
			seen[metricID] = true
			index := 0
			compiler := analyticalCompiler{scopedPopulations: true, reviewedNullPolicy: true, expandedExpressions: true, joins: true, ctx: ctx, definition: pub.Definition, binding: a.binding, visiting: map[semantics.Reference]bool{}}
			compiler.scalarCapture = func(measure semantics.Measure, origins []scalarFilterOrigin, leaf exec.AnalyticalExpression, dataset string) error {
				leaf = rebaseAnalyticalExpression(leaf, dataset, base.Dataset)
				if index >= len(expected) || exec.Hash(leaf) != exec.Hash(expected[index].Leaf) {
					return exec.ErrBinding
				}
				occurrence := expected[index]
				index++
				for _, constraint := range canonical.Constraints {
					witness, err := scalarSemanticPredicateWitness(ctx, a, pub, *base, measure, origins, constraint, metricID, occurrence)
					if err != nil {
						return err
					}
					witnesses = append(witnesses, witness)
				}
				return nil
			}
			expression, err := compiler.metric(root.Reference, nil, 0)
			if err != nil {
				return nil, err
			}
			expression = rebaseAnalyticalExpression(expression, compiler.dataset, base.Dataset)
			matched := false
			for _, metric := range base.Metrics {
				matched = matched || metric.ID == metricID && exec.Hash(metric.Expression) == exec.Hash(expression)
			}
			if !matched || index != len(expected) {
				return nil, exec.ErrBinding
			}
		}
	}
	if len(seen) != len(base.Metrics) {
		return nil, exec.ErrBinding
	}
	proof, err := exec.NewAnalyticalScalarEntailment(ctx, a.binding, *base, canonical.Constraints, witnesses)
	if err != nil {
		return nil, err
	}
	base.Version = exec.AnalyticalScalarEntailmentVersion
	base.ScalarEntailment = proof
	if err := exec.ValidateAnalyticalScalarEntailment(ctx, *base, a.binding); err != nil {
		return nil, err
	}
	return base, nil
}

func scalarSemanticPredicateWitness(ctx context.Context, a admission, pub topics.Published, c exec.AnalyticalContract, m semantics.Measure, origins []scalarFilterOrigin, constraint exec.BusinessConstraint, metric string, occurrence scalarExpressionOccurrence) (exec.AnalyticalScalarPredicateWitness, error) {
	bad := exec.AnalyticalScalarPredicateWitness{}
	var candidates []exec.AnalyticalScalarPredicateWitness
	for _, origin := range origins {
		f := origin.Filter
		if f.ID == "" || f.Operator != "eq" || len(f.Values) != 1 || f.Values[0] != constraint.Value || f.Field.Dataset != constraint.Dataset {
			continue
		}
		compiler := analyticalCompiler{ctx: ctx, definition: pub.Definition, binding: a.binding, joins: true, dataset: c.Dataset}
		column, err := compiler.column(f.Field)
		if err != nil {
			return bad, err
		}
		target := exec.AnalyticalColumnName(c.Dataset, constraint.Dataset, constraint.Column)
		if column.SourceName != target {
			continue
		}
		filter := exec.AnalyticalFilter{Column: target, Kind: "eq", Values: []string{constraint.Value}}
		matched := false
		for _, candidate := range occurrence.Leaf.Filters {
			matched = matched || exec.Hash(candidate) == exec.Hash(filter)
		}
		if !matched {
			continue
		}
		witness := exec.AnalyticalScalarPredicateWitness{Resolution: constraint.Resolution, Metric: metric, Path: append([]int{}, occurrence.Path...), Topic: pub.Definition.Topic, TopicVersion: pub.Definition.Version, Publication: pub.Digest, Measure: m.ID, Filter: f.ID, FilterOwnerKind: string(origin.Kind), FilterOwner: origin.Owner, LeafDigest: exec.Hash(occurrence.Leaf), FilterDigest: exec.Hash(filter)}
		if m.Field.Dataset == constraint.Dataset {
			if f.Relationship != "" {
				continue
			}
		} else {
			// The semantic publication contract permits relationship-bound filters
			// on measures only. Inherited KPI predicates do not acquire parent reach.
			if origin.Kind != semantics.KindMeasure || origin.Owner != m.ID {
				continue
			}
			relationship, ok := semantics.PopulationRelationship(semantics.TopicPack{Joins: pub.Definition.Joins}, m.Field.Dataset, constraint.Dataset, f.Relationship)
			if !ok {
				continue
			}
			permitted := len(a.route.Request.JoinChoices) == 0
			for _, choice := range a.route.Request.JoinChoices {
				permitted = permitted || choice.Topic == pub.Definition.Topic && choice.JoinID == relationship.ID
			}
			if !permitted {
				continue
			}
			physical, err := scalarPhysicalRelationship(ctx, a, pub.Definition, relationship)
			if err != nil {
				return bad, err
			}
			present := false
			for _, lane := range c.ScalarPopulations.Lanes {
				if lane.Dataset != m.Field.Dataset {
					continue
				}
				for _, join := range lane.Joins {
					present = present || exec.Hash(join) == exec.Hash(physical)
				}
			}
			if !present {
				continue
			}
			witness.Relationship, witness.RelationshipDigest, witness.JoinDigest = relationship.ID, exec.Hash(relationship), exec.Hash(physical)
		}
		candidates = append(candidates, witness)
	}
	if len(candidates) == 0 {
		return bad, analyticalUnsupported("analytical_scalar_predicate_not_entailed")
	}
	// Multiple identical mandatory predicates may have distinct reviewed owners.
	// Pick one canonical witness without collapsing separate leaf occurrences.
	sort.Slice(candidates, func(i, j int) bool { return exec.Hash(candidates[i]) < exec.Hash(candidates[j]) })
	return candidates[0], nil
}

func scalarPhysicalRelationship(ctx context.Context, a admission, def topics.Definition, j semantics.Join) (exec.AnalyticalJoin, error) {
	out := exec.AnalyticalJoin{Left: j.Left.Dataset, Right: j.Right.Dataset, Type: string(j.Type)}
	compiler := analyticalCompiler{ctx: ctx, definition: def, binding: a.binding, joins: true}
	for _, pair := range j.KeyPairs() {
		compiler.dataset = ""
		left, err := compiler.column(pair.Left)
		if err != nil {
			return out, err
		}
		compiler.dataset = ""
		right, err := compiler.column(pair.Right)
		if err != nil {
			return out, err
		}
		out.LeftColumns = append(out.LeftColumns, left.SourceName)
		out.RightColumns = append(out.RightColumns, right.SourceName)
	}
	if out.Type != "inner" || len(out.LeftColumns) == 0 {
		return out, exec.ErrBinding
	}
	if out.Left > out.Right {
		out.Left, out.Right = out.Right, out.Left
		out.LeftColumns, out.RightColumns = out.RightColumns, out.LeftColumns
	}
	indices := make([]int, len(out.LeftColumns))
	for i := range indices {
		indices[i] = i
	}
	sort.Slice(indices, func(i, j int) bool {
		a, b := indices[i], indices[j]
		if out.LeftColumns[a] != out.LeftColumns[b] {
			return out.LeftColumns[a] < out.LeftColumns[b]
		}
		return out.RightColumns[a] < out.RightColumns[b]
	})
	lefts, rights := make([]string, len(indices)), make([]string, len(indices))
	for i, index := range indices {
		lefts[i], rights[i] = out.LeftColumns[index], out.RightColumns[index]
	}
	out.LeftColumns, out.RightColumns = lefts, rights
	return out, nil
}
