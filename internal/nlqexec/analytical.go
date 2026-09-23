package nlqexec

import (
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// The persisted revision is independent of selection and native validation.
// Zero means retained legacy evidence, not a claim of analytical correctness.
const analyticalRecordVersion = 2

func analyticalUnsupported(code string) error {
	return &exec.AnalyticalError{Code: code, Unsupported: true}
}

// compileAnalytical consumes exact admitted definitions, never model claims,
// retrieved prose or serialized prompt fragments. Required-rule dependencies are
// still authoritative context but cannot masquerade as a user-selected output.
func compileAnalytical(ctx context.Context, a admission) (*exec.AnalyticalContract, error) {
	return compileAnalyticalVersion(ctx, a, analyticalRecordVersion)
}

func compileAnalyticalVersion(ctx context.Context, a admission, version int) (*exec.AnalyticalContract, error) {
	if version != 1 && version != analyticalRecordVersion {
		return nil, exec.ErrBinding
	}
	proofVersion := exec.AnalyticalVersion
	if version == analyticalRecordVersion {
		proofVersion = exec.AnalyticalGrainVersion
	}
	if ctx == nil {
		return nil, exec.ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selected := a.route.Selection
	if selected == nil {
		return nil, nil
	} // Existing adapters without selected intent.
	copy := *selected
	copy.Digest = ""
	if selected.Version != "catalog-selection-v1" || selected.Digest != exec.Hash(copy) || len(selected.Topics) != len(a.publications) || len(selected.Topics) > 4 {
		return nil, exec.ErrBinding
	}
	var out *exec.AnalyticalContract
	for i, pick := range selected.Topics {
		publication := a.publications[i]
		def := publication.Definition
		if pick.Topic != def.Topic || pick.TopicVersion != def.Version || pick.PackDigest != publication.Digest || !topics.DigestValid(publication.Digest) || len(pick.Roots) > 128 {
			return nil, exec.ErrBinding
		}
		compiler := &analyticalCompiler{ctx: ctx, definition: def, binding: a.binding, visiting: map[semantics.Reference]bool{}}
		for _, root := range pick.Roots {
			if root.Reference.Kind != semantics.KindMeasure && root.Reference.Kind != semantics.KindKPI || root.Reason == "required_rule" {
				continue
			}
			if !root.Reference.Valid() {
				return nil, exec.ErrBinding
			}
			if out == nil {
				if a.binding.Dialect != "postgres" {
					return nil, analyticalUnsupported("analytical_dialect_unsupported")
				}
				out = &exec.AnalyticalContract{Version: proofVersion, Binding: exec.Hash(a.binding), Semantics: selected.Digest}
			}
			expression, err := compiler.metric(root.Reference, nil, 0)
			if err != nil {
				return nil, err
			}

			if out.Dataset != "" && out.Dataset != compiler.dataset {
				return nil, analyticalUnsupported("analytical_join_unsupported")
			}
			out.Dataset = compiler.dataset
			out.Metrics = append(out.Metrics, exec.AnalyticalMetric{ID: pick.Topic + ":" + string(root.Reference.Kind) + ":" + root.Reference.ID, Expression: expression})
			if len(out.Metrics) > 32 {
				return nil, exec.ErrLimit
			}
		}
	}
	if out != nil {
		sort.Slice(out.Metrics, func(i, j int) bool { return out.Metrics[i].ID < out.Metrics[j].ID })
		if version == analyticalRecordVersion {
			var err error
			out.Grain, err = compileAnalyticalGrain(ctx, a, *out)
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

type analyticalCompiler struct {
	ctx        context.Context
	definition topics.Definition
	binding    exec.Binding
	dataset    string
	visiting   map[semantics.Reference]bool
	nodes      int
}

func (c *analyticalCompiler) column(ref semantics.Reference) (semantics.Column, error) {
	if !ref.Valid() || ref.Kind != semantics.KindColumn {
		return semantics.Column{}, exec.ErrBinding
	}
	var column semantics.Column
	found := 0
	for _, d := range c.definition.Datasets {
		if d.ID != ref.Dataset {
			continue
		}
		if d.Source.Source != c.binding.Source || d.Source.Context != c.binding.Context || d.Source.SourceRevision != c.binding.Revision {
			return column, exec.ErrBinding
		}
		for _, candidate := range d.Columns {
			if candidate.ID == ref.ID {
				column = candidate
				found++
			}
		}
	}
	if found != 1 || column.SourceName == "" {
		return column, exec.ErrBinding
	}
	if c.dataset != "" && c.dataset != ref.Dataset {
		return column, analyticalUnsupported("analytical_join_unsupported")
	}
	c.dataset = ref.Dataset
	found = 0
	for _, relation := range c.binding.Relations {
		if relation.ID != c.dataset {
			continue
		}
		for _, actual := range relation.Columns {
			if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
				found++
			}
		}
	}
	if found != 1 {
		return column, exec.ErrBinding
	}
	return column, nil
}

func (c *analyticalCompiler) filters(in []semantics.SemanticFilter) ([]exec.AnalyticalFilter, error) {
	if len(in) > 32 {
		return nil, exec.ErrLimit
	}
	var out []exec.AnalyticalFilter
	seen := map[string]bool{}
	for _, f := range in {
		col, err := c.column(f.Field)
		if err != nil {
			return nil, err
		}
		if f.Operator != "eq" && f.Operator != "in" && f.Operator != "not_null" || f.Operator == "eq" && len(f.Values) != 1 || f.Operator == "in" && (len(f.Values) < 1 || len(f.Values) > 32) || f.Operator == "not_null" && len(f.Values) != 0 {
			return nil, analyticalUnsupported("analytical_population_unsupported")
		}
		// Publication already governs disclosure. Recheck literal-bearing definitions
		// before using them as local constraints; no sensitive value is sent to a model.
		if len(f.Values) > 0 && col.Sensitivity != semantics.LiteralNonSensitive {
			return nil, exec.ErrBinding
		}
		filter := exec.AnalyticalFilter{Column: col.SourceName, Kind: f.Operator, Values: append([]string(nil), f.Values...)}
		sort.Strings(filter.Values)
		key := exec.Hash(filter)
		if !seen[key] {
			out = append(out, filter)
			seen[key] = true
		}
	}
	return out, nil
}

func (c *analyticalCompiler) metric(ref semantics.Reference, inherited []semantics.SemanticFilter, depth int) (exec.AnalyticalExpression, error) {
	bad := exec.AnalyticalExpression{}
	c.nodes++
	if depth > 32 || c.nodes > 1024 {
		return bad, exec.ErrLimit
	}
	if err := c.ctx.Err(); err != nil {
		return bad, err
	}
	if !ref.Valid() || c.visiting[ref] {
		return bad, exec.ErrBinding
	}
	c.visiting[ref] = true
	defer delete(c.visiting, ref)
	switch ref.Kind {
	case semantics.KindMeasure:
		for _, m := range c.definition.Measures {
			if m.ID != ref.ID {
				continue
			}
			col, err := c.column(m.Field)
			if err != nil {
				return bad, err
			}
			filters, err := c.filters(append(append([]semantics.SemanticFilter(nil), inherited...), m.Filters...))
			if err != nil {
				return bad, err
			}
			ops := map[semantics.Aggregation]string{semantics.AggregationSum: "sum", semantics.AggregationAverage: "avg", semantics.AggregationMinimum: "min", semantics.AggregationMaximum: "max", semantics.AggregationCount: "count", semantics.AggregationDistinctCount: "distinct_count"}
			op := ops[m.Aggregation]
			if op == "" {
				return bad, exec.ErrBinding
			}
			return exec.AnalyticalExpression{Op: op, Column: col.SourceName, Filters: filters}, nil
		}
	case semantics.KindKPI:
		for _, k := range c.definition.KPIs {
			if k.ID != ref.ID {
				continue
			}
			if len(k.Expression) == 0 || len(k.Expression) > 4096 || len(k.Inputs) < 1 || len(k.Inputs) > 32 {
				return bad, exec.ErrLimit
			}
			inputs := map[string]semantics.Reference{}
			for _, input := range k.Inputs {
				if _, exists := inputs[input.ID]; exists || input.Kind != semantics.KindMeasure && input.Kind != semantics.KindKPI {
					return bad, exec.ErrBinding
				}
				inputs[input.ID] = input
			}
			expr, err := parser.ParseExpr(k.Expression)
			if err != nil {
				return bad, analyticalUnsupported("analytical_expression_unsupported")
			}
			used := map[string]bool{}
			inherited = append(append([]semantics.SemanticFilter(nil), inherited...), k.Filters...)
			result, err := c.expression(expr, inputs, used, inherited, depth+1)
			if err != nil {
				return bad, err
			}
			if len(used) != len(inputs) {
				return bad, analyticalUnsupported("analytical_expression_unsupported")
			}
			return result, nil
		}
	}
	return bad, exec.ErrBinding
}

func (c *analyticalCompiler) expression(node ast.Expr, inputs map[string]semantics.Reference, used map[string]bool, filters []semantics.SemanticFilter, depth int) (exec.AnalyticalExpression, error) {
	bad := exec.AnalyticalExpression{}
	c.nodes++
	if depth > 32 || c.nodes > 1024 {
		return bad, exec.ErrLimit
	}
	if err := c.ctx.Err(); err != nil {
		return bad, err
	}
	switch n := node.(type) {
	case *ast.ParenExpr:
		return c.expression(n.X, inputs, used, filters, depth+1)
	case *ast.Ident:
		ref, ok := inputs[n.Name]
		if !ok {
			return bad, analyticalUnsupported("analytical_expression_unsupported")
		}
		used[n.Name] = true
		return c.metric(ref, filters, depth+1)
	case *ast.BasicLit:
		if n.Kind != token.INT && n.Kind != token.FLOAT || len(n.Value) > 128 || strings.ContainsAny(n.Value, "_xXoObBeEpPiI") {
			return bad, analyticalUnsupported("analytical_expression_unsupported")
		}
		return exec.AnalyticalExpression{Op: "number", Value: n.Value}, nil
	case *ast.BinaryExpr:
		if n.Op != token.ADD && n.Op != token.SUB && n.Op != token.MUL && n.Op != token.QUO {
			return bad, analyticalUnsupported("analytical_expression_unsupported")
		}
		x, err := c.expression(n.X, inputs, used, filters, depth+1)
		if err != nil {
			return bad, err
		}
		y, err := c.expression(n.Y, inputs, used, filters, depth+1)
		if err != nil {
			return bad, err
		}
		return exec.AnalyticalExpression{Op: n.Op.String(), Args: []exec.AnalyticalExpression{x, y}}, nil
	}
	return bad, analyticalUnsupported("analytical_expression_unsupported")
}

func cloneAnalyticalReceipt(r *exec.AnalyticalReceipt) *exec.AnalyticalReceipt {
	if r == nil {
		return nil
	}
	out := *r
	out.Metrics = append([]string(nil), r.Metrics...)
	out.Grouping = append([]string(nil), r.Grouping...)
	return &out
}

// expectedAnalytical checks durable metadata before native planning or replay.
// A legacy row stays explicitly unmeasured, and a new row cannot silently drop a
// receipt, change a contract or substitute another query's proof.
func expectedAnalytical(ctx context.Context, q QueryRecord, a admission) (*exec.AnalyticalContract, error) {
	if q.AnalyticalVersion == 0 {
		if q.Analytical != nil {
			return nil, exec.ErrBinding
		}
		return nil, nil
	}
	if q.AnalyticalVersion != 1 && q.AnalyticalVersion != analyticalRecordVersion {
		return nil, exec.ErrBinding
	}
	contract, err := compileAnalyticalVersion(ctx, a, q.AnalyticalVersion)
	if err != nil {
		return nil, err
	}
	if contract == nil {
		if q.Analytical != nil {
			return nil, exec.ErrBinding
		}
		return nil, nil
	}
	want := &exec.AnalyticalReceipt{Version: contract.Version, Scope: exec.AnalyticalMetricScope, Contract: exec.Hash(*contract), Query: exec.AnalyticalQueryDigest(q.SQL, q.Parameters)}
	for _, m := range contract.Metrics {
		want.Metrics = append(want.Metrics, m.ID)
	}
	sort.Strings(want.Metrics)
	if contract.Grain != nil {
		want.Scope = exec.AnalyticalGrainScope
		want.Grouping = append([]string(nil), contract.Grain.Dimensions...)
	}
	if q.Analytical == nil || exec.Hash(want) != exec.Hash(q.Analytical) {
		return nil, exec.ErrBinding
	}
	return contract, nil
}

func (s *Service) validateAnalyticalCandidate(ctx context.Context, e identity.Envelope, a admission, c generatedCandidate) (exec.Plan, *exec.AnalyticalReceipt, error) {
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: c.SQL, Parameters: c.Parameters}, a.relationScope)
	if err != nil {
		return exec.Plan{}, nil, err
	}
	if a.analytical == nil {
		return plan, nil, nil
	}
	receipt, err := exec.CheckAnalyticalPlan(ctx, plan, *a.analytical)
	if err != nil {
		return exec.Plan{}, nil, err
	}
	return plan, receipt, nil
}

func analyticalDiagnostic(err error) string {
	// Only these closed categories may enter the correction prompt. An arbitrary
	// error Code, parser body, query literal or native error message never does.
	var e *exec.AnalyticalError
	if !errors.As(err, &e) {
		return ""
	}
	switch e.Code {
	case "analytical_grain_mismatch", "analytical_metric_mismatch", "analytical_population_mismatch", "analytical_relation_mismatch", "analytical_integer_division", "analytical_zero_policy":
		return e.Code
	default:
		return "analytical_shape_unsupported"
	}
}

// AnalyticalRecordValid validates only durable evidence shape/integrity. It does
// not authorize access or replace native/semantic proof reconstruction on Run.
func AnalyticalRecordValid(q QueryRecord) bool {
	if q.AnalyticalVersion == 0 {
		return q.Analytical == nil
	}
	if q.AnalyticalVersion != 1 && q.AnalyticalVersion != analyticalRecordVersion {
		return false
	}
	selected := false
	if q.Route.Selection != nil {
		if len(q.Route.Selection.Topics) > 4 {
			return false
		}
		for _, topic := range q.Route.Selection.Topics {
			if len(topic.Roots) > 128 {
				return false
			}
			for _, root := range topic.Roots {
				if root.Reason != "required_rule" && (root.Reference.Kind == semantics.KindMeasure || root.Reference.Kind == semantics.KindKPI) {
					selected = true
				}
			}
		}
	}
	if !selected {
		return q.Analytical == nil
	}
	r := q.Analytical
	version := exec.AnalyticalVersion
	if q.AnalyticalVersion == analyticalRecordVersion {
		version = exec.AnalyticalGrainVersion
	}
	if r == nil || q.SQL == "" || r.Version != version || !analyticalReceiptScopeValid(r) || !topics.DigestValid(r.Contract) || !topics.DigestValid(r.Query) || r.Query != exec.AnalyticalQueryDigest(q.SQL, q.Parameters) || len(r.Metrics) == 0 || len(r.Metrics) > 32 {
		return false
	}
	for i, id := range r.Metrics {
		if len(id) == 0 || len(id) > 256 || i > 0 && r.Metrics[i-1] >= id {
			return false
		}
	}
	return true
}

func analyticalReceiptScopeValid(r *exec.AnalyticalReceipt) bool {
	if r.Scope == exec.AnalyticalMetricScope {
		return len(r.Grouping) == 0
	}
	if r.Scope != exec.AnalyticalGrainScope || r.Version != exec.AnalyticalGrainVersion || len(r.Grouping) < 1 || len(r.Grouping) > 16 {
		return false
	}
	for i, id := range r.Grouping {
		if len(id) == 0 || len(id) > 256 || i > 0 && r.Grouping[i-1] >= id {
			return false
		}
	}
	return true
}
