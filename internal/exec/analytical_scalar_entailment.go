package exec

import (
	"context"
	"encoding/hex"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/identity"
)

const AnalyticalScalarEntailmentVersion = "analytical-metrics-v13"
const AnalyticalScalarEntailmentPolicy = "entailed-scalar-predicates-v1"

const scalarEntailmentMaxCoverage = 512

// AnalyticalScalarEntailment holds protected current request values and the
// complete occurrence-level witnesses reconstructed by the semantic compiler.
// It is evidence, never source authority or an executable plan.
type AnalyticalScalarEntailment struct {
	Policy      string                             `json:"policy"`
	Constraints []BusinessConstraint               `json:"constraints"`
	Witnesses   []AnalyticalScalarPredicateWitness `json:"witnesses"`
}

// AnalyticalScalarPredicateWitness identifies one selected aggregate occurrence.
// The compiler authenticates semantic IDs against immutable publications and the
// current route. Exec independently checks the compiled occurrence and source
// evidence; a public hash cannot authenticate semantic identity.
type AnalyticalScalarPredicateWitness struct {
	Resolution         string `json:"resolution"`
	Metric             string `json:"metric"`
	Path               []int  `json:"path"`
	Topic              string `json:"topic"`
	TopicVersion       string `json:"topic_version"`
	Publication        string `json:"publication"`
	Measure            string `json:"measure"`
	Filter             string `json:"filter"`
	FilterOwnerKind    string `json:"filter_owner_kind"`
	FilterOwner        string `json:"filter_owner"`
	LeafDigest         string `json:"leaf_digest"`
	FilterDigest       string `json:"filter_digest"`
	Relationship       string `json:"relationship,omitempty"`
	RelationshipDigest string `json:"relationship_digest,omitempty"`
	JoinDigest         string `json:"join_digest,omitempty"`
}

func (AnalyticalScalarEntailment) String() string         { return "analytical-scalar-entailment(redacted)" }
func (p AnalyticalScalarEntailment) GoString() string     { return p.String() }
func (p AnalyticalScalarEntailment) LogValue() slog.Value { return slog.StringValue(p.String()) }

// NewAnalyticalScalarEntailment canonicalizes and detaches the bounded proof.
// Inputs must come from current authenticated semantic/route reconstruction.
func NewAnalyticalScalarEntailment(ctx context.Context, b Binding, c AnalyticalContract, constraints []BusinessConstraint, witnesses []AnalyticalScalarPredicateWitness) (*AnalyticalScalarEntailment, error) {
	if ctx == nil || c.ScalarEntailment != nil || c.Version != AnalyticalScopedPopulationsVersion && c.Version != AnalyticalScalarEntailmentVersion {
		return nil, ErrBinding
	}
	if _, err := scalarEntailmentCoverageBound(ctx, c, len(constraints)); err != nil {
		return nil, err
	}
	if len(witnesses) > scalarEntailmentMaxCoverage {
		return nil, ErrLimit
	}
	for _, w := range witnesses {
		if len(w.Path) > 32 {
			return nil, ErrLimit
		}
		if !scalarWitnessIdentityValid(w) {
			return nil, ErrBinding
		}
	}
	p := &AnalyticalScalarEntailment{Policy: AnalyticalScalarEntailmentPolicy, Constraints: append([]BusinessConstraint(nil), constraints...), Witnesses: make([]AnalyticalScalarPredicateWitness, len(witnesses))}
	sort.Slice(p.Constraints, func(i, j int) bool { return p.Constraints[i].Resolution < p.Constraints[j].Resolution })
	for i, w := range witnesses {
		p.Witnesses[i] = w
		p.Witnesses[i].Path = append(make([]int, 0, len(w.Path)), w.Path...)
	}
	sort.Slice(p.Witnesses, func(i, j int) bool { return compareScalarWitness(p.Witnesses[i], p.Witnesses[j]) < 0 })
	c.Version, c.ScalarEntailment = AnalyticalScalarEntailmentVersion, p
	if err := ValidateAnalyticalScalarEntailment(ctx, c, b); err != nil {
		return nil, err
	}
	return p, nil
}

// Bound all witness strings before sorting or hashing caller-controlled fields.
func scalarWitnessIdentityValid(w AnalyticalScalarPredicateWitness) bool {
	if !scalarEntailmentDigest(w.Resolution) || w.Metric == "" || len(w.Metric) > 256 || !identity.Identifier(w.Topic) || !identity.Identifier(w.TopicVersion) || !scalarEntailmentDigest(w.Publication) || !identity.Identifier(w.Measure) || !identity.Identifier(w.Filter) || !identity.Identifier(w.FilterOwner) || w.FilterOwnerKind != "measure" && w.FilterOwnerKind != "kpi" || w.FilterOwnerKind == "measure" && w.FilterOwner != w.Measure || !scalarEntailmentDigest(w.LeafDigest) || !scalarEntailmentDigest(w.FilterDigest) {
		return false
	}
	if w.Relationship != "" && !identity.Identifier(w.Relationship) || w.RelationshipDigest != "" && !scalarEntailmentDigest(w.RelationshipDigest) || w.JoinDigest != "" && !scalarEntailmentDigest(w.JoinDigest) {
		return false
	}
	for _, index := range w.Path {
		if index < 0 || index > 1 {
			return false
		}
	}
	return true
}

func compareScalarWitness(a, b AnalyticalScalarPredicateWitness) int {
	if n := strings.Compare(a.Resolution, b.Resolution); n != 0 {
		return n
	}
	if n := strings.Compare(a.Metric, b.Metric); n != 0 {
		return n
	}
	return slices.Compare(a.Path, b.Path)
}

// Count without materializing paths, leaves or a constraint-by-leaf product.
// Validate the expression shape while traversing so malformed trees cannot hide
// unbounded branches behind nominal aggregate or constant nodes.
func scalarEntailmentCoverageBound(ctx context.Context, c AnalyticalContract, constraints int) (int, error) {
	if ctx == nil || constraints < 1 || len(c.Metrics) < 1 || len(c.Metrics) > 32 {
		return 0, ErrBinding
	}
	if constraints > 64 {
		return 0, ErrLimit
	}
	nodes, leaves := 0, 0
	var visit func(AnalyticalExpression, int) error
	visit = func(e AnalyticalExpression, depth int) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		nodes++
		if depth > 32 || nodes > 1024 {
			return ErrLimit
		}
		switch e.Op {
		case "+", "-", "*", "/":
			if len(e.Args) != 2 || e.Column != "" || e.Value != "" || len(e.Filters) != 0 {
				return ErrBinding
			}
			for _, arg := range e.Args {
				if err := visit(arg, depth+1); err != nil {
					return err
				}
			}
		case "number":
			if _, ok := analyticalNumber(e.Value); !ok || e.Column != "" || len(e.Args)+len(e.Filters) != 0 {
				return ErrBinding
			}
		case "sum", "count":
			if e.Column == "" || e.Value != "" || len(e.Args) != 0 || len(e.Filters) > 32 {
				return ErrBinding
			}
			leaves++
			if leaves > scalarEntailmentMaxCoverage/constraints {
				return ErrLimit
			}
		default:
			return analyticalFailure("analytical_scalar_entailment_unsupported", true)
		}
		return nil
	}
	seen := map[string]bool{}
	for _, metric := range c.Metrics {
		if metric.ID == "" || len(metric.ID) > 256 || seen[metric.ID] {
			return 0, ErrBinding
		}
		seen[metric.ID] = true
		before := leaves
		if err := visit(metric.Expression, 0); err != nil {
			return 0, err
		}
		if before == leaves {
			return 0, ErrBinding
		}
	}
	return leaves * constraints, nil
}

type scalarEntailmentLeaf struct {
	metric     string
	path       []int
	expression AnalyticalExpression
}

func scalarEntailmentLeaves(c AnalyticalContract) []scalarEntailmentLeaf {
	var out []scalarEntailmentLeaf
	var visit func(string, AnalyticalExpression, []int)
	visit = func(metric string, e AnalyticalExpression, path []int) {
		if e.Op == "sum" || e.Op == "count" {
			out = append(out, scalarEntailmentLeaf{metric, append(make([]int, 0, len(path)), path...), e})
			return
		}
		for i, arg := range e.Args {
			visit(metric, arg, append(path, i))
		}
	}
	for _, metric := range c.Metrics {
		visit(metric.ID, metric.Expression, nil)
	}
	return out
}

func scalarEntailmentDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

// PostgreSQL's broad text category also includes UUID/name/domain-like types.
// This increment admits only native text and varying-character columns.
func scalarEntailmentTextColumn(c Column) bool {
	if !c.Safe || c.Category != "text" {
		return false
	}
	native := strings.TrimPrefix(c.NativeType, "pg_catalog.")
	if native == "text" || native == "varchar" || native == "character varying" {
		return true
	}
	base, modifier, ok := strings.Cut(native, "(")
	if !ok || base != "varchar" && base != "character varying" || !strings.HasSuffix(modifier, ")") {
		return false
	}
	size, err := strconv.Atoi(strings.TrimSuffix(modifier, ")"))
	return err == nil && size > 0 && size <= 10485760
}

// ValidateAnalyticalScalarEntailment proves exact canonical request predicates
// against every selected SUM/COUNT occurrence and its current physical lane.
func ValidateAnalyticalScalarEntailment(ctx context.Context, c AnalyticalContract, b Binding) error {
	p := c.ScalarEntailment
	if ctx == nil || c.Version != AnalyticalScalarEntailmentVersion || p == nil || p.Policy != AnalyticalScalarEntailmentPolicy || b.Dialect != "postgres" || !b.Valid() || c.Binding != Hash(b) || !scalarEntailmentDigest(c.Semantics) || c.ScalarPopulations == nil || len(c.ScalarPopulations.Lanes) < 2 || len(c.ScalarPopulations.Lanes) > 4 || c.GroupedPopulations != nil || c.GroupSelection != nil || c.GroupDomain != nil || c.Completeness != nil || c.Intent == nil {
		return ErrBinding
	}
	product, err := scalarEntailmentCoverageBound(ctx, c, len(p.Constraints))
	if err != nil {
		return err
	}
	if len(p.Witnesses) != product {
		return ErrBinding
	}
	if err := ValidateBusinessConstraints(b, p.Constraints); err != nil {
		return err
	}
	relations := map[string]Relation{}
	for _, r := range b.Relations {
		if _, exists := relations[r.ID]; exists {
			return ErrBinding
		}
		relations[r.ID] = r
	}
	periods := map[string]bool{}
	for _, lane := range c.ScalarPopulations.Lanes {
		if lane.QueryPopulation == nil || len(lane.QueryPopulation.Constraints) != 1 || len(lane.Joins) > 3 {
			return ErrBinding
		}
		periods[lane.QueryPopulation.Constraints[0].Resolution] = true
	}
	for i, constraint := range p.Constraints {
		if i > 0 && p.Constraints[i-1].Resolution >= constraint.Resolution || periods[constraint.Resolution] || constraint.Kind != "text" || constraint.Operator != "eq" || constraint.Nulls != "exclude" || constraint.Null || constraint.Aggregation != "" || constraint.Upper != "" || constraint.Unit != "" || constraint.Precision != 0 || constraint.Scale != 0 || constraint.Bounds != "" || constraint.TemporalType != "" || constraint.Calendar != "" || constraint.TimeZone != "" || constraint.Grain != "" {
			return ErrBinding
		}
		col, ok := analyticalRelationColumn(relations[constraint.Dataset], constraint.Column)
		if !ok || !scalarEntailmentTextColumn(col) {
			return analyticalFailure("analytical_scalar_entailment_unsupported", true)
		}
	}
	base := scalarEntailmentBase(c)
	root, ok := relations[c.Dataset]
	if !ok {
		return ErrBinding
	}
	relation, err := analyticalScalarRelation(ctx, base, b, root)
	if err != nil {
		return err
	}
	standard := analyticalStandardPolicy(base)
	if err := validateAnalyticalGrain(standard, relation); err != nil {
		return err
	}
	if err := validateAnalyticalIntent(standard); err != nil {
		return err
	}
	if _, _, _, err := prepareAnalyticalProgram(ctx, b, base, relation, nil); err != nil {
		return err
	}
	leaves := scalarEntailmentLeaves(c)
	byOccurrence := map[string]scalarEntailmentLeaf{}
	for _, leaf := range leaves {
		byOccurrence[Hash([]any{leaf.metric, leaf.path})] = leaf
	}
	constraints := map[string]BusinessConstraint{}
	for _, constraint := range p.Constraints {
		constraints[constraint.Resolution] = constraint
	}
	lanes := map[string]AnalyticalScalarLane{}
	for _, lane := range c.ScalarPopulations.Lanes {
		lanes[lane.Dataset] = lane
	}
	for i, w := range p.Witnesses {
		if err := ctx.Err(); err != nil {
			return err
		}
		if w.Path == nil || len(w.Path) > 32 || !scalarWitnessIdentityValid(w) || i > 0 && compareScalarWitness(p.Witnesses[i-1], w) >= 0 {
			return ErrBinding
		}
		constraint, exists := constraints[w.Resolution]
		if !exists {
			return ErrBinding
		}
		leaf, exists := byOccurrence[Hash([]any{w.Metric, w.Path})]
		if !exists || w.LeafDigest != Hash(leaf.expression) {
			return ErrBinding
		}
		matched := 0
		for _, f := range leaf.expression.Filters {
			if f.Kind == "eq" && f.Column == AnalyticalColumnName(c.Dataset, constraint.Dataset, constraint.Column) && len(f.Values) == 1 && f.Values[0] == constraint.Value && w.FilterDigest == Hash(f) {
				matched++
			}
		}
		if matched != 1 {
			return analyticalFailure("analytical_scalar_entailment_mismatch", false)
		}
		fact := analyticalColumnDataset(c.Dataset, leaf.expression.Column)
		lane, exists := lanes[fact]
		if !exists {
			return ErrBinding
		}
		if fact == constraint.Dataset {
			if w.Relationship != "" || w.RelationshipDigest != "" || w.JoinDigest != "" {
				return ErrBinding
			}
			continue
		}
		if w.FilterOwnerKind != "measure" || w.FilterOwner != w.Measure || !identity.Identifier(w.Relationship) || !scalarEntailmentDigest(w.RelationshipDigest) || !scalarEntailmentDigest(w.JoinDigest) {
			return ErrBinding
		}
		joined := 0
		for _, j := range lane.Joins {
			if j.Type != "inner" || Hash(j) != w.JoinDigest {
				continue
			}
			if j.Left == fact && j.Right == constraint.Dataset && relations[j.Right].HasUniqueKey(j.RightColumns) || j.Right == fact && j.Left == constraint.Dataset && relations[j.Left].HasUniqueKey(j.LeftColumns) {
				joined++
			}
		}
		if joined != 1 {
			return analyticalFailure("analytical_scalar_entailment_mismatch", false)
		}
	}
	return nil
}

// Only validated v13 entrypoints use this detached legacy-policy view. It is
// never returned as a retained contract and never changes the original digest.
func scalarEntailmentBase(c AnalyticalContract) AnalyticalContract {
	c.Version = AnalyticalScopedPopulationsVersion
	c.ScalarEntailment = nil
	return c
}
