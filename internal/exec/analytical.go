package exec

import (
	"context"
	"errors"
	"log/slog"
	"math/big"
	"sort"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
)

// AnalyticalVersion identifies the closed, scoped metric proof and its policy.
const AnalyticalVersion = "analytical-metrics-v1"

// AnalyticalGrainVersion adds an optional exact, server-selected grouping proof.
// v1 remains a distinct retained policy and must never be upgraded on replay.
const AnalyticalGrainVersion = "analytical-metrics-v2"

// AnalyticalGroupingVersion pins explicit replacement/inherited grouping intent.
const AnalyticalGroupingVersion = "analytical-metrics-v5"

// AnalyticalGroupingPolicy permits a deliberately empty grouping (scalar total).
const AnalyticalGroupingPolicy = "reviewed-grouping-v1"

// AnalyticalTotalScope distinguishes a proved total from unmeasured grouping.
const AnalyticalTotalScope = "selected_metric_expression_population_and_scalar_total;single_base_relation"

const (
	// AnalyticalMetricScope does not certify the query-wide population or grain.
	AnalyticalMetricScope = "selected_metric_expression_and_population;single_base_relation"
	// AnalyticalGrainScope additionally proves the exact selected grouping columns.
	AnalyticalGrainScope = "selected_metric_expression_population_and_grouping;single_base_relation"
)

var (
	// ErrAnalyticalMismatch means a selected metric's checked definition differs.
	ErrAnalyticalMismatch = errors.New("exec: analytical metric mismatch")
	// ErrAnalyticalUnsupported means no proof is implemented for this shape.
	ErrAnalyticalUnsupported = errors.New("exec: analytical shape unsupported")
)

// AnalyticalError is deliberately closed and value-free, including in repair.
type AnalyticalError struct {
	Code        string
	Unsupported bool
}

func (e *AnalyticalError) Error() string { return "exec: " + e.Code }
func (e *AnalyticalError) Unwrap() error {
	if e.Unsupported {
		return ErrAnalyticalUnsupported
	}
	return ErrAnalyticalMismatch
}
func analyticalFailure(code string, unsupported bool) error {
	return &AnalyticalError{Code: code, Unsupported: unsupported}
}

// AnalyticalFilter is a reviewed metric population, not a query-wide user filter.
// Column uses the exact physical name. Values have the column's canonical type.
type AnalyticalFilter struct {
	Column string   `json:"column"`
	Kind   string   `json:"kind"`
	Values []string `json:"values,omitempty"`
}

// AnalyticalExpression is a closed aggregate/arithmetic tree. No SQL is accepted.
// Operators are sum/avg/min/max/count/distinct_count, number, and + - * /.
type AnalyticalExpression struct {
	Op      string                 `json:"op"`
	Column  string                 `json:"column,omitempty"`
	Value   string                 `json:"value,omitempty"`
	Filters []AnalyticalFilter     `json:"filters,omitempty"`
	Args    []AnalyticalExpression `json:"args,omitempty"`
}

// AnalyticalMetric is one selected output, not an incidental KPI ingredient.
type AnalyticalMetric struct {
	ID         string               `json:"id"`
	Expression AnalyticalExpression `json:"expression"`
}

// AnalyticalContract is server-derived from exact admitted semantic revisions.
// v1 proves selected metric expressions/populations over one base relation. It
// does NOT certify question interpretation or query-wide filters. v2 can also
// prove an explicitly compiled direct-column grain; nil grain remains unmeasured.
type AnalyticalContract struct {
	GroupSelection     *AnalyticalQueryPopulation    `json:"group_selection,omitempty"`
	Completeness       *AnalyticalCompleteness       `json:"completeness,omitempty"`
	ScalarPopulations  *AnalyticalScalarPopulations  `json:"scalar_populations,omitempty"`
	GroupDomain        *AnalyticalGroupDomain        `json:"group_domain,omitempty"`
	GroupedPopulations *AnalyticalGroupedPopulations `json:"grouped_populations,omitempty"`
	Populations        []string                      `json:"populations,omitempty"`
	Joins              []AnalyticalJoin              `json:"joins,omitempty"`
	Version            string                        `json:"version"`
	Binding            string                        `json:"binding"`
	Semantics          string                        `json:"semantics"`
	Dataset            string                        `json:"dataset"`
	Metrics            []AnalyticalMetric            `json:"metrics"`
	Grain              *AnalyticalGrain              `json:"grain,omitempty"`
	QueryPopulation    *AnalyticalQueryPopulation    `json:"query_population,omitempty"`
	Intent             *AnalyticalIntent             `json:"intent,omitempty"`
}

func (AnalyticalContract) String() string         { return "analytical-contract(redacted)" }
func (c AnalyticalContract) GoString() string     { return c.String() }
func (c AnalyticalContract) LogValue() slog.Value { return slog.StringValue(c.String()) }

// AnalyticalReceipt is bounded non-executable evidence of the specified checks.
// Native validation and business approval remain independent requirements.
type AnalyticalReceipt struct {
	Completeness    *AnalyticalCompleteness `json:"completeness,omitempty"`
	Outputs         []AnalyticalOutput      `json:"outputs,omitempty"`
	Version         string                  `json:"version"`
	Scope           string                  `json:"scope"`
	Contract        string                  `json:"contract"`
	Query           string                  `json:"query"`
	Metrics         []string                `json:"metrics"`
	Grouping        []string                `json:"grouping,omitempty"`
	QueryPopulation string                  `json:"query_population,omitempty"`
	Intent          string                  `json:"intent,omitempty"`
}

// CheckAnalyticalPlan checks an already native-validated opaque plan. It neither
// issues plans nor widens the admitted binding, and does no source/model work.
// Unsupported syntax is not labeled a passed analytical result.
func CheckAnalyticalPlan(ctx context.Context, p Plan, c AnalyticalContract) (*AnalyticalReceipt, error) {
	if err := validateAnalyticalCapabilities(c, p.candidate.binding); err != nil {
		return nil, err
	}
	standard := analyticalStandardPolicy(c)
	if ctx == nil || !p.nativeChecked || !p.candidate.checked || !p.candidate.owner.Valid() || (standard.Version != AnalyticalVersion && standard.Version != AnalyticalGrainVersion && standard.Version != AnalyticalCalendarVersion && standard.Version != AnalyticalQueryPopulationVersion && standard.Version != AnalyticalGroupingVersion && standard.Version != AnalyticalIntentVersion && (standard.Version != AnalyticalGroupedPopulationsVersion && standard.Version != AnalyticalGroupedProgramsVersion)) || c.Binding != Hash(p.candidate.binding) || len(c.Semantics) != 64 || len(c.Metrics) == 0 || len(c.Metrics) > 32 {
		return nil, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if (standard.Version == AnalyticalGroupedPopulationsVersion || standard.Version == AnalyticalGroupedProgramsVersion) && (c.Intent == nil || c.QueryPopulation == nil) {
		return nil, ErrBinding
	}
	_, knownDialect := sqlpolicy.NativeDialect(p.candidate.binding.Dialect)
	if p.candidate.binding.Dialect != "postgres" && (standard.Version != AnalyticalIntentVersion && (standard.Version != AnalyticalGroupedPopulationsVersion && standard.Version != AnalyticalGroupedProgramsVersion) || !knownDialect) {
		return nil, analyticalFailure("analytical_dialect_unsupported", true)
	}
	proofBinding := p.candidate.binding
	if proofBinding.Dialect != "postgres" {
		proofBinding.Relations = append([]Relation(nil), proofBinding.Relations...)
		for i, r := range proofBinding.Relations {
			proofBinding.Relations[i] = warehouseAnalyticalRelation(proofBinding.Dialect, r)
		}
	}
	var relation Relation
	matches := 0
	for _, r := range proofBinding.Relations {
		if r.ID == c.Dataset {
			relation = r
			matches++
		}
	}
	if matches != 1 {
		return nil, ErrBinding
	}
	var joinErr error
	if c.ScalarPopulations != nil {
		relation, joinErr = analyticalScalarRelation(ctx, c, proofBinding, relation)
	} else if c.GroupedPopulations != nil {
		relation, joinErr = analyticalGroupedRelation(c, proofBinding, relation)
	} else if len(c.Populations) > 0 {
		relation, joinErr = analyticalPopulationRelation(standard, proofBinding, relation)
	} else {
		relation, joinErr = analyticalJoinRelation(standard, proofBinding, relation)
	}
	if joinErr != nil {
		return nil, joinErr
	}
	if err := validateAnalyticalGroupDomain(standard); err != nil {
		return nil, err
	}
	if err := validateAnalyticalGrain(standard, relation); err != nil {
		return nil, err
	}
	if err := validateAnalyticalQueryPopulation(ctx, standard, p.candidate.binding); err != nil {
		return nil, err
	}
	if err := validateAnalyticalIntent(standard); err != nil {
		return nil, err
	}
	if c.Intent != nil {
		for _, o := range c.Intent.Order {
			if o.Column != "" {
				found := false
				for _, col := range relation.Columns {
					if col.Name == o.Column && col.Safe {
						found = true
					}
				}
				if !found {
					return nil, ErrBinding
				}
			}
		}
	}
	checker, expected, ids, err := prepareAnalyticalProgram(ctx, p.candidate.binding, c, relation, p.candidate.parameters)
	if err != nil {
		return nil, err
	}
	// Plan validation already bounded this SQL/AST; retain a separate hard bound
	// before re-entering the pinned native parser for the narrower semantic proof.
	sql := p.candidate.statement
	if len(sql) == 0 || len(sql) > 32<<10 || strings.Count(sql, "(") > 256 {
		return nil, ErrLimit
	}
	q, err := analyticalSQLTree(ctx, sql, p.candidate.binding)
	if err != nil {
		return nil, err
	}
	if c.GroupedPopulations != nil {
		err = checker.queryGroupedPopulations(q, expected, c.GroupedPopulations)
	} else if c.ScalarPopulations != nil || len(c.Populations) > 0 {
		err = checker.queryIndependent(q, expected)
	} else {
		err = checker.query(q, expected)
	}
	if err != nil {
		return nil, err
	}
	sort.Strings(ids)
	receipt := &AnalyticalReceipt{Version: c.Version, Scope: AnalyticalMetricScope, Contract: Hash(c), Query: AnalyticalQueryDigest(sql, p.candidate.parameters), Metrics: ids}
	if c.Grain != nil {
		receipt.Scope = AnalyticalGrainScope
		if c.Grain.Policy == AnalyticalGroupingPolicy && len(c.Grain.Dimensions) == 0 {
			receipt.Scope = AnalyticalTotalScope
		}
		if len(c.Grain.Buckets) > 0 {
			receipt.Scope = AnalyticalCalendarScope
		}
		receipt.Grouping = append([]string(nil), c.Grain.Dimensions...)
	}
	if c.QueryPopulation != nil {
		receipt.QueryPopulation = AnalyticalQueryPopulationPolicy
	}
	if c.Intent != nil {
		receipt.Intent = AnalyticalIntentPolicy
	}
	if len(c.Joins) > 0 {
		receipt.Scope = analyticalJoinScope(receipt.Scope)
	}
	if len(c.Populations) > 0 {
		receipt.Scope = strings.ReplaceAll(receipt.Scope, "single_base_relation", "independent_singleton_populations")
	}
	if c.ScalarPopulations != nil {
		receipt.Scope = strings.ReplaceAll(receipt.Scope, "single_base_relation", "independent_scoped_singleton_populations")
	}
	if c.GroupedPopulations != nil {
		receipt.Scope = strings.ReplaceAll(receipt.Scope, "single_base_relation", "independent_grouped_populations")
	}
	if c.Version == AnalyticalGroupedOwnedPopulationsVersion {
		receipt.Scope = strings.ReplaceAll(receipt.Scope, "independent_grouped_populations", "independent_owned_grouped_populations")
	}
	if c.Version == AnalyticalGroupedSelectionVersion {
		receipt.Scope = groupedSelectionScope(receipt.Scope)
	}
	if c.Version == AnalyticalGroupedFactsVersion {
		receipt.Scope = strings.ReplaceAll(receipt.Scope, "independent_grouped_populations", "independent_filtered_grouped_populations")
	}
	if c.Completeness != nil {
		receipt.Completeness = CloneAnalyticalCompleteness(c.Completeness)
		receipt.Scope += AnalyticalCompletenessScope
	}
	if c.Version == AnalyticalScopedPopulationsVersion || c.Version == AnalyticalGroupedOwnedPopulationsVersion || c.Version == AnalyticalGroupedSelectionVersion || c.Version == AnalyticalGroupedFactsVersion {
		receipt.Outputs, err = checker.provedOutputs(ids)
		if err != nil {
			return nil, err
		}
	}
	return receipt, nil
}

// prepareAnalyticalProgram builds only a bounded semantic checker. It issues no
// executable Plan; callers still require the native and authority boundary.
func prepareAnalyticalProgram(ctx context.Context, binding Binding, c AnalyticalContract, relation Relation, parameters []Parameter) (analyticalChecker, map[string]int, []string, error) {
	standard := analyticalStandardPolicy(c)
	checker := analyticalChecker{groupedOwned: c.Version == AnalyticalGroupedOwnedPopulationsVersion || c.Version == AnalyticalGroupedSelectionVersion || c.Version == AnalyticalGroupedFactsVersion, groupSelection: c.GroupSelection, scalarPopulations: c.ScalarPopulations, ordinaryGroupProof: standard.Version == AnalyticalGroupedProgramsVersion && c.ScalarPopulations == nil && c.GroupedPopulations == nil && len(c.Populations) == 0, ordinaryGroupDomain: c.GroupDomain, groupedExtensions: standard.Version == AnalyticalGroupedProgramsVersion, reviewedNullPolicy: (standard.Version == AnalyticalGroupedPopulationsVersion || standard.Version == AnalyticalGroupedProgramsVersion), populations: c.Populations, expandedExpressions: standard.Version == AnalyticalIntentVersion || (standard.Version == AnalyticalGroupedPopulationsVersion || standard.Version == AnalyticalGroupedProgramsVersion), joins: c.Joins, joinAliases: map[string]string{}, joinUsed: map[int]bool{}, intent: c.Intent, ctx: ctx, relation: relation, parameters: parameters, grain: c.Grain, binding: binding, queryPopulation: c.QueryPopulation}
	if c.ScalarPopulations != nil {
		for _, lane := range c.ScalarPopulations.Lanes {
			checker.populations = append(checker.populations, lane.Dataset)
		}
	}
	if c.GroupedPopulations != nil {
		for _, lane := range c.GroupedPopulations.Lanes {
			checker.populations = append(checker.populations, lane.Dataset)
		}
	}
	checker.intentMetricKeys = map[string]string{}
	expected := map[string]int{}
	ids := make([]string, 0, len(c.Metrics))
	seen := map[string]bool{}
	for _, metric := range c.Metrics {
		if metric.ID == "" || len(metric.ID) > 256 || seen[metric.ID] {
			return analyticalChecker{}, nil, nil, ErrBinding
		}
		seen[metric.ID] = true
		before := len(checker.leaves)
		key, err := checker.expected(metric.Expression, 0)
		if err != nil {
			return analyticalChecker{}, nil, nil, err
		}
		if len(checker.leaves) == before {
			return analyticalChecker{}, nil, nil, analyticalFailure("analytical_expression_unsupported", true)
		}
		checker.intentMetricKeys[metric.ID] = key
		expected[key]++
		ids = append(ids, metric.ID)
	}
	return checker, expected, ids, nil
}

type analyticalChecker struct {
	groupSelection     *AnalyticalQueryPopulation
	groupSelectionKeys map[string][2]string

	groupedOwned         bool
	finalTerms           []analyticalTerm
	scalarPopulations    *AnalyticalScalarPopulations
	ordinaryGroupProof   bool
	ordinaryGroupDomain  *AnalyticalGroupDomain
	ordinaryDomainProved bool
	groupedLaneProof     bool
	groupedDomain        string
	groupedDomainProved  bool
	groupedExtensions    bool
	groupedDepth         int
	groupedOutput        map[string]analyticalTerm
	reviewedNullPolicy   bool
	populationSource     string
	populations          []string
	derivedTerms         map[string]analyticalTerm
	expandedExpressions  bool
	joins                []AnalyticalJoin
	joinAliases          map[string]string
	joinUsed             map[int]bool
	ctx                  context.Context
	relation             Relation
	alias                string
	parameters           []Parameter
	nodes                int
	leaves               []AnalyticalExpression
	grain                *AnalyticalGrain
	binding              Binding
	queryPopulation      *AnalyticalQueryPopulation
	intent               *AnalyticalIntent
	intentMetricKeys     map[string]string
	common               map[string]bool
	global               map[string]bool
}

// Only bounded base-ten literals are normalized. Do not let leading zeroes
// acquire Go's integer base inference or an exponent allocate enormous integers.
func analyticalNumber(value string) (string, bool) {
	if len(value) == 0 || len(value) > 128 {
		return "", false
	}
	negative := false
	if value[0] == '-' || value[0] == '+' {
		negative = value[0] == '-'
		value = value[1:]
	}
	if len(value) == 0 {
		return "", false
	}
	fraction, digits, dot := 0, "", false
	for _, ch := range value {
		if ch == '.' && !dot {
			dot = true
			continue
		}
		if ch < '0' || ch > '9' {
			return "", false
		}
		digits += string(ch)
		if dot {
			fraction++
		}
	}
	if len(digits) == 0 {
		return "", false
	}
	numerator, ok := new(big.Int).SetString(digits, 10)
	if !ok {
		return "", false
	}
	if negative {
		numerator.Neg(numerator)
	}
	denominator := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(fraction)), nil)
	return new(big.Rat).SetFrac(numerator, denominator).RatString(), true
}
func (a *analyticalChecker) column(name string) (Column, bool) {
	for _, c := range a.relation.Columns {
		if c.Name == name && c.Safe {
			return c, true
		}
	}
	return Column{}, false
}
func analyticalFilterKey(f AnalyticalFilter) string {
	values := append([]string(nil), f.Values...)
	sort.Strings(values)
	return Hash([]any{f.Column, f.Kind, values})
}
func analyticalExprKey(op, column, value string, filters []AnalyticalFilter, args []string) string {
	keys := make([]string, 0, len(filters))
	for _, f := range filters {
		keys = append(keys, analyticalFilterKey(f))
	}
	sort.Strings(keys)
	if (op == "+" || op == "*") && len(args) == 2 && args[1] < args[0] {
		args = []string{args[1], args[0]}
	}
	return Hash([]any{op, column, value, keys, args})
}
func (a *analyticalChecker) expected(e AnalyticalExpression, depth int) (string, error) {
	a.nodes++
	if depth > 32 || a.nodes > 1024 {
		return "", ErrLimit
	}
	if err := a.ctx.Err(); err != nil {
		return "", err
	}
	switch e.Op {
	case "coalesce":
		if !a.reviewedNullPolicy {
			return "", analyticalFailure("analytical_expression_unsupported", true)
		}
		return a.expectedNullPolicy(e, depth)
	case "+", "-", "*", "/":
		if len(e.Args) != 2 || e.Column != "" || e.Value != "" || len(e.Filters) > 0 {
			return "", ErrBinding
		}
		x, err := a.expected(e.Args[0], depth+1)
		if err != nil {
			return "", err
		}
		y, err := a.expected(e.Args[1], depth+1)
		if err != nil {
			return "", err
		}
		return analyticalExprKey(e.Op, "", "", nil, []string{x, y}), nil
	case "number":
		value, ok := analyticalNumber(e.Value)
		if !ok || e.Column != "" || len(e.Args)+len(e.Filters) != 0 {
			return "", ErrBinding
		}
		return analyticalExprKey("number", "", value, nil, nil), nil
	case "sum", "avg", "min", "max", "count", "distinct_count":
		column, ok := a.column(e.Column)
		if !ok || len(e.Args) != 0 || e.Value != "" || len(e.Filters) > 32 {
			return "", ErrBinding
		}
		if (e.Op == "sum" || e.Op == "avg") && analyticalNumericKind(column) == "" {
			return "", analyticalFailure("analytical_type_unsupported", true)
		}
		e.Filters = append([]AnalyticalFilter(nil), e.Filters...)
		seen := map[string]bool{}
		for i, f := range e.Filters {
			col, ok := a.column(f.Column)
			if !ok {
				return "", ErrBinding
			}
			if f.Kind != "eq" && f.Kind != "in" && f.Kind != "not_null" || f.Kind == "eq" && len(f.Values) != 1 || f.Kind == "in" && (len(f.Values) < 1 || len(f.Values) > 32) || f.Kind == "not_null" && len(f.Values) != 0 {
				return "", ErrBinding
			}
			f.Values = append([]string(nil), f.Values...)
			for j, value := range f.Values {
				normalized, ok := analyticalScalar(col, value)
				if !ok {
					return "", analyticalFailure("analytical_filter_type_unsupported", true)
				}
				f.Values[j] = normalized
			}
			f = canonicalAnalyticalFilter(f)
			e.Filters[i] = f
			key := analyticalFilterKey(f)
			if seen[key] {
				return "", ErrBinding
			}
			seen[key] = true
		}
		a.leaves = append(a.leaves, e)
		return a.aggregateKey(e.Op, e.Column, e.Filters), nil
	default:
		return "", analyticalFailure("analytical_expression_unsupported", true)
	}
}

// PostgreSQL discovery deliberately labels integers, decimals, floats and money
// as the same "numeric" family. Only the verified native type can establish
// exact arithmetic or whether division truncates. Never infer that distinction
// from a category or a sampled value. Floating/money/domain types remain outside
// this first exact-arithmetic proof, even when their broad family is numeric.
func analyticalNumericKind(column Column) string {
	switch column.Category {
	case "numeric", "integer", "decimal", "number":
	default:
		return ""
	}
	native := strings.TrimPrefix(column.NativeType, "pg_catalog.")
	switch native {
	case "int2", "int4", "int8", "smallint", "integer", "bigint":
		return "integer"
	case "numeric", "decimal":
		return "decimal"
	}
	base, modifiers, ok := strings.Cut(native, "(")
	if !ok || (base != "numeric" && base != "decimal") || len(modifiers) > 32 || !strings.HasSuffix(modifiers, ")") {
		return ""
	}
	parts := strings.Split(strings.TrimSuffix(modifiers, ")"), ",")
	if len(parts) < 1 || len(parts) > 2 {
		return ""
	}
	precision, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil || precision < 1 || precision > 1000 {
		return ""
	}
	if len(parts) == 2 {
		scale, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || scale < -1000 || scale > 1000 {
			return ""
		}
	}
	return "decimal"
}

// Text comparison preserves exact spelling. Numeric comparison is exact rational
// normalization, never float64. Null/temporal/coercion inference is unsupported.
func analyticalScalar(column Column, value string) (string, bool) {
	if len(value) > 512 {
		return "", false
	}
	switch column.Category {
	case "numeric", "integer", "decimal", "number":
		// A type-free synthetic decimal is used only to parse literal AST nodes.
		// Source comparisons must instead have a supported exact native type.
		if column.NativeType != "" && analyticalNumericKind(column) == "" || column.Category == "numeric" && column.NativeType == "" {
			return "", false
		}
		return analyticalNumber(value)
	case "boolean":
		if value == "true" || value == "false" {
			return value, true
		}
	case "text":
		return value, true
	}
	return "", false
}

// AnalyticalQueryDigest canonicalizes an empty parameter list across JSON and
// database round trips. It is a content hash, not authentication or execution.
func AnalyticalQueryDigest(sql string, parameters []Parameter) string {
	values := append([]Parameter{}, parameters...)
	return Hash([]any{sql, values})
}
