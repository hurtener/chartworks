package exec

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"

	pgquery "github.com/wasilibs/go-pgquery"
)

// AnalyticalQueryPopulationVersion adds optional, server-owned query predicate
// conformance. Retained versions keep their original, narrower proof.
const AnalyticalQueryPopulationVersion = "analytical-metrics-v4"

// AnalyticalQueryPopulationPolicy is exact predicate provenance, not a claim
// that every natural-language restriction was recognized.
const AnalyticalQueryPopulationPolicy = "owned-query-predicates-v1"

// AnalyticalQueryPopulation contains protected typed values. Only its policy
// marker and the enclosing contract digest may enter a public receipt. It is
// never a model instruction, authority token or executable plan.
type AnalyticalQueryPopulation struct {
	Policy      string               `json:"policy"`
	Constraints []BusinessConstraint `json:"constraints"`
}

func (AnalyticalQueryPopulation) String() string         { return "analytical-query-population(redacted)" }
func (p AnalyticalQueryPopulation) GoString() string     { return p.String() }
func (p AnalyticalQueryPopulation) LogValue() slog.Value { return slog.StringValue(p.String()) }

// NewAnalyticalQueryPopulation detaches verified constraints from the caller.
// The caller must obtain them from current sealed routing or authenticated replay.
// This check validates coordinates/types; it does not establish authority.
func NewAnalyticalQueryPopulation(ctx context.Context, binding Binding, dataset string, constraints []BusinessConstraint) (*AnalyticalQueryPopulation, error) {
	if ctx == nil {
		return nil, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if binding.Dialect != "postgres" || dataset == "" || len(constraints) < 1 || len(constraints) > 64 {
		return nil, ErrBinding
	}
	if err := ValidateBusinessConstraints(binding, constraints); err != nil {
		return nil, err
	}
	for _, c := range constraints {
		if c.Dataset != dataset {
			return nil, analyticalFailure("analytical_query_population_unsupported", true)
		}
	}
	out := &AnalyticalQueryPopulation{Policy: AnalyticalQueryPopulationPolicy, Constraints: append([]BusinessConstraint(nil), constraints...)}
	sort.Slice(out.Constraints, func(i, j int) bool { return out.Constraints[i].Resolution < out.Constraints[j].Resolution })
	return out, nil
}

func validateAnalyticalQueryPopulation(ctx context.Context, c AnalyticalContract, binding Binding) error {
	if c.QueryPopulation == nil {
		return nil
	}
	if c.Version != AnalyticalQueryPopulationVersion || c.QueryPopulation.Policy != AnalyticalQueryPopulationPolicy {
		return ErrBinding
	}
	canonical, err := NewAnalyticalQueryPopulation(ctx, binding, c.Dataset, c.QueryPopulation.Constraints)
	if err != nil {
		return err
	}
	if Hash(canonical) != Hash(c.QueryPopulation) {
		return ErrBinding
	}
	return nil
}

// checkQueryPopulation requires every WHERE/HAVING conjunct to be accounted for.
// Existing per-metric population checks still run independently. A global filter
// shared by every requested metric may remain in WHERE; a private/user predicate
// may not be replaced by a narrower generated condition or hidden in HAVING.
func (a *analyticalChecker) checkQueryPopulation(q map[string]any) error {
	if a.queryPopulation == nil {
		return nil
	}
	base := "SELECT count(*) FROM " + businessQuote("postgres", a.relation.Schema) + "." + businessQuote("postgres", a.relation.Name) + " AS " + businessQuote("postgres", a.alias)
	bound, err := BindBusinessConstraints(a.ctx, a.binding, base, nil, a.queryPopulation.Constraints)
	if err != nil {
		return err
	}
	if len(bound.SQL) > 32<<10 {
		return ErrLimit
	}
	raw, err := pgquery.ParseToJSON(bound.SQL)
	if err != nil {
		return analyticalFailure("analytical_query_population_unsupported", true)
	}
	if err := a.ctx.Err(); err != nil {
		return err
	}
	var tree map[string]any
	if json.Unmarshal([]byte(raw), &tree) != nil || len(array(tree["stmts"])) != 1 {
		return ErrBinding
	}
	stmt := object(object(array(tree["stmts"])[0])["stmt"])
	if len(stmt) != 1 || object(stmt["SelectStmt"]) == nil {
		return ErrBinding
	}
	expected := object(stmt["SelectStmt"])
	for _, clause := range []string{"whereClause", "havingClause"} {
		if err := a.matchPopulationClause(q[clause], expected[clause], bound.Parameters, clause == "whereClause"); err != nil {
			return err
		}
	}
	return a.ctx.Err()
}

func (a *analyticalChecker) matchPopulationClause(actual, expected any, expectedParameters []Parameter, allowCommonMetric bool) error {
	want := map[string]bool{}
	for _, node := range analyticalConjuncts(expected) {
		key, err := a.populationNodeKey(node, expectedParameters)
		if err != nil {
			return err
		}
		want[key] = false
	}
	for _, node := range analyticalConjuncts(actual) {
		key, err := a.populationNodeKey(node, a.parameters)
		if err != nil {
			return err
		}
		if _, exists := want[key]; exists {
			want[key] = true
			continue
		}
		if allowCommonMetric {
			filter, filterErr := a.predicate(node)
			if filterErr == nil && a.common[analyticalFilterKey(filter)] {
				continue
			}
		}
		return analyticalFailure("analytical_query_population_mismatch", false)
	}
	for _, present := range want {
		if !present {
			return analyticalFailure("analytical_query_population_mismatch", false)
		}
	}
	return nil
}

func (a *analyticalChecker) populationNodeKey(node any, parameters []Parameter) (string, error) {
	nodes := 0
	v, err := a.populationNode(node, parameters, 0, &nodes)
	if err != nil {
		return "", err
	}
	return Hash(v), nil
}

// Structural equality intentionally does not infer cast/literal equivalence.
// Only parser locations, resolved qualification and parameter numbering differ.
// Concrete parameter kinds/values and every cast/operator/Boolean node remain
// in the private comparison. Never use a SQL fingerprint that erases literals.
func (a *analyticalChecker) populationNode(node any, parameters []Parameter, depth int, nodes *int) (any, error) {
	*nodes++
	if *nodes > 4096 || depth > 64 {
		return nil, ErrLimit
	}
	if err := a.ctx.Err(); err != nil {
		return nil, err
	}
	switch v := node.(type) {
	case map[string]any:
		if len(v) == 1 && v["ColumnRef"] != nil {
			column, ok := a.field(v)
			if !ok {
				return nil, analyticalFailure("analytical_query_population_unsupported", true)
			}
			return map[string]any{"resolved_column": column.Name}, nil
		}
		if len(v) == 1 && v["ParamRef"] != nil {
			n, ok := object(v["ParamRef"])["number"].(float64)
			if !ok || n < 1 || n > float64(len(parameters)) || n != float64(int(n)) || !parameters[int(n)-1].Valid() {
				return nil, ErrBinding
			}
			return map[string]any{"bound_parameter": parameters[int(n)-1]}, nil
		}
		out := make(map[string]any, len(v))
		for key, value := range v {
			if key == "location" {
				continue
			}
			normalized, err := a.populationNode(value, parameters, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, value := range v {
			normalized, err := a.populationNode(value, parameters, depth+1, nodes)
			if err != nil {
				return nil, err
			}
			out[i] = normalized
		}
		return out, nil
	case nil, bool, string, float64:
		return v, nil
	default:
		return nil, ErrBinding
	}
}
