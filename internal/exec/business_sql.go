package exec

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// BusinessParameterBinding is value-free effect evidence. Parameter indexes are
// one-based positions in the exact final parameter vector, not option IDs.
type BusinessParameterBinding struct {
	Resolution  string `json:"resolution"`
	Dataset     string `json:"dataset"`
	Column      string `json:"column"`
	Aggregation string `json:"aggregation,omitempty"`
	Kind        string `json:"kind"`
	Operator    string `json:"operator"`
	Nulls       string `json:"nulls"`
	Parameters  []int  `json:"parameters"`
}

// BusinessBindingReceipt is provenance, not execution authority. The final read
// receipt is attached only after the existing validator issues an opaque Plan.
type BusinessBindingReceipt struct {
	SchemaVersion int                        `json:"schema_version"`
	SourceBinding string                     `json:"source_binding"`
	Constraints   string                     `json:"constraints"`
	Statement     string                     `json:"statement"`
	Bindings      []BusinessParameterBinding `json:"bindings"`
	Validation    *Receipt                   `json:"validation,omitempty"`
}

// BusinessBoundQuery carries protected SQL and parameters pending ordinary read validation.
type BusinessBoundQuery struct {
	SQL        string                 `json:"-"`
	Parameters []Parameter            `json:"-"`
	Receipt    BusinessBindingReceipt `json:"receipt"`
}

// String omits SQL and parameter values from ordinary formatted logs.
func (BusinessBoundQuery) String() string { return "business-bound-query(redacted)" }

// GoString preserves protected-query redaction for Go-syntax formatting.
func (b BusinessBoundQuery) GoString() string { return b.String() }

type businessRange struct {
	relation Relation
	alias    string
	virtual  string
}

type businessSQLLayout struct {
	ranges                     []businessRange
	where, whereEnd            int
	having, havingEnd          int
	rowInsert, aggregateInsert int
	hasAggregate               bool
	hasOuterJoin               bool
}

type businessEdit struct {
	start, end int
	text       string
}

type businessScalar struct {
	parameter  Parameter
	resolution string
}

// BindBusinessConstraints applies closed business predicates to the admitted
// source coordinates. A bounded single SELECT with qualified base relations is
// supported, including joins, WHERE, GROUP BY, HAVING and ordering/limits. A
// flat PostgreSQL WITH of independent SELECTs is also supported. Other nested
// SELECTs, set operations and ambiguous self-join targets return explicit
// insufficiency rather than dropping a constraint or guessing its meaning.
// The resulting SQL is NEVER executable without the existing full validator.
func BindBusinessConstraints(ctx context.Context, binding Binding, statement string, parameters []Parameter, constraints []BusinessConstraint) (BusinessBoundQuery, error) {
	if ctx == nil || len(statement) == 0 || len(statement) > 32<<10 || len(parameters) > 64 {
		return BusinessBoundQuery{}, businessSQLFailure("unsupported_statement_size")
	}
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	if err := ValidateBusinessConstraints(binding, constraints); err != nil {
		return BusinessBoundQuery{}, err
	}
	for _, parameter := range parameters {
		if !parameter.Valid() {
			return BusinessBoundQuery{}, businessSQLFailure("invalid_parameter_value")
		}
	}
	if len(constraints) == 0 {
		if err := ctx.Err(); err != nil {
			return BusinessBoundQuery{}, err
		}
		return BusinessBoundQuery{SQL: statement, Parameters: append([]Parameter(nil), parameters...)}, nil
	}
	constraints = append([]BusinessConstraint(nil), constraints...)
	sort.Slice(constraints, func(i, j int) bool { return constraints[i].Resolution < constraints[j].Resolution })
	tokens, err := businessScan(ctx, statement, false)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	layout, err := businessBindingLayout(ctx, statement, tokens, binding, constraints)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	var rows, aggregates []string
	var scalars []businessScalar
	bindings := make([]BusinessParameterBinding, 0, len(constraints))
	for _, constraint := range constraints {
		if err := ctx.Err(); err != nil {
			return BusinessBoundQuery{}, err
		}
		alias, matches := "", 0
		for _, source := range layout.ranges {
			if source.relation.ID == constraint.Dataset {
				alias, matches = source.alias, matches+1
			}
		}
		if matches != 1 {
			return BusinessBoundQuery{}, businessError(constraint, "target", "unsupported_missing_or_ambiguous_target")
		}
		// A WHERE predicate on a nullable side after an outer join removes
		// unmatched preserved rows. The binder does not rewrite JOIN inputs or
		// ON expressions, so refuse the whole target scope conservatively.
		if layout.hasOuterJoin {
			return BusinessBoundQuery{}, businessError(constraint, "target", "unsupported_outer_join_target")
		}
		column := businessQuote(binding.Dialect, alias) + "." + businessQuote(binding.Dialect, constraint.Column)
		if constraint.Aggregation != "" {
			if !layout.hasAggregate {
				return BusinessBoundQuery{}, businessError(constraint, "target", "unsupported_aggregate_query")
			}
			column, err = businessAggregate(constraint.Aggregation, column)
			if err != nil {
				return BusinessBoundQuery{}, err
			}
		}
		predicate, err := businessPredicate(binding.Dialect, column, constraint, &scalars)
		if err != nil {
			return BusinessBoundQuery{}, err
		}
		if constraint.Aggregation != "" {
			aggregates = append(aggregates, predicate)
		} else {
			rows = append(rows, predicate)
		}
		bindings = append(bindings, BusinessParameterBinding{Resolution: constraint.Resolution, Dataset: constraint.Dataset, Column: constraint.Column, Aggregation: constraint.Aggregation, Kind: constraint.Kind, Operator: constraint.Operator, Nulls: constraint.Nulls, Parameters: []int{}})
	}
	var edits []businessEdit
	if len(rows) > 0 {
		predicate := strings.Join(rows, " AND ")
		if layout.where >= 0 {
			edits = append(edits, businessEdit{layout.where, layout.whereEnd, "WHERE (" + strings.TrimSpace(statement[layout.where+len("WHERE"):layout.whereEnd]) + ") AND (" + predicate + ") "})
		} else {
			edits = append(edits, businessEdit{layout.rowInsert, layout.rowInsert, " WHERE " + predicate + " "})
		}
	}
	if len(aggregates) > 0 {
		predicate := strings.Join(aggregates, " AND ")
		if layout.having >= 0 {
			edits = append(edits, businessEdit{layout.having, layout.havingEnd, "HAVING (" + strings.TrimSpace(statement[layout.having+len("HAVING"):layout.havingEnd]) + ") AND (" + predicate + ") "})
		} else {
			edits = append(edits, businessEdit{layout.aggregateInsert, layout.aggregateInsert, " HAVING " + predicate + " "})
		}
	}
	bound, err := businessApplyEdits(statement, edits)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	boundTokens, err := businessScan(ctx, bound, true)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	var parameterEdits []businessEdit
	outParams := make([]Parameter, 0, len(parameters)+len(scalars))
	used := make([]bool, len(parameters))
	positional := 0
	for _, token := range boundTokens {
		if token.kind != 'p' && token.kind != 'm' {
			continue
		}
		var value Parameter
		resolution := ""
		if token.kind == 'm' {
			index, err := strconv.Atoi(token.text)
			if err != nil || index < 0 || index >= len(scalars) {
				return BusinessBoundQuery{}, businessSQLFailure("invalid_parameter_marker")
			}
			value, resolution = scalars[index].parameter, scalars[index].resolution
		} else {
			positional++
			index, err := businessParameterIndex(token.text, binding.Dialect, positional)
			if err != nil || index < 1 || index > len(parameters) {
				return BusinessBoundQuery{}, businessSQLFailure("invalid_parameter_index")
			}
			used[index-1], value = true, parameters[index-1]
		}
		if len(outParams) >= 64 {
			return BusinessBoundQuery{}, businessSQLFailure("unsupported_parameter_budget")
		}
		outParams = append(outParams, value)
		parameterEdits = append(parameterEdits, businessEdit{token.start, token.end, businessPlaceholder(binding.Dialect, len(outParams))})
		if resolution != "" {
			for i := range bindings {
				if bindings[i].Resolution == resolution {
					bindings[i].Parameters = append(bindings[i].Parameters, len(outParams))
				}
			}
		}
	}
	for _, found := range used {
		if !found {
			return BusinessBoundQuery{}, businessSQLFailure("unused_parameter")
		}
	}
	bound, err = businessApplyEdits(bound, parameterEdits)
	if err != nil {
		return BusinessBoundQuery{}, err
	}
	if len(bound) > 32<<10 || strings.ContainsRune(bound, 0x1f) {
		return BusinessBoundQuery{}, businessSQLFailure("unsupported_statement_size")
	}
	if err := ctx.Err(); err != nil {
		return BusinessBoundQuery{}, err
	}
	return BusinessBoundQuery{SQL: bound, Parameters: outParams, Receipt: BusinessBindingReceipt{SchemaVersion: 1, SourceBinding: Hash(binding), Constraints: Hash(constraints), Statement: Hash([]any{bound, outParams}), Bindings: bindings}}, nil
}

func businessAggregate(aggregation, column string) (string, error) {
	switch aggregation {
	case "sum":
		return "SUM(" + column + ")", nil
	case "average":
		return "AVG(" + column + ")", nil
	case "minimum":
		return "MIN(" + column + ")", nil
	case "maximum":
		return "MAX(" + column + ")", nil
	case "count":
		return "COUNT(" + column + ")", nil
	case "distinct_count":
		return "COUNT(DISTINCT " + column + ")", nil
	default:
		return "", businessSQLFailure("unsupported_aggregation")
	}
}

func businessPredicate(dialect, column string, c BusinessConstraint, scalars *[]businessScalar) (string, error) {
	if c.Null {
		return "(" + column + " IS NULL)", nil
	}
	appendValue := func(value string) (string, error) {
		kind := "text"
		if c.Kind == "boolean" {
			kind = "boolean"
		}
		index := len(*scalars)
		*scalars = append(*scalars, businessScalar{Parameter{Kind: kind, Value: value}, c.Resolution})
		marker := "\x1f" + strconv.Itoa(index) + "\x1f"
		switch c.Kind {
		case "number":
			typeName := "DECIMAL(" + strconv.Itoa(c.Precision) + "," + strconv.Itoa(c.Scale) + ")"
			if dialect == "bigquery" {
				typeName = "NUMERIC"
				if c.Precision-c.Scale > 29 || c.Scale > 9 {
					typeName = "BIGNUMERIC"
				}
			}
			return "CAST(" + marker + " AS " + typeName + ")", nil
		case "time_window":
			typeName := "DATE"
			switch c.TemporalType {
			case "timestamp":
				switch dialect {
				case "sqlserver":
					typeName = "DATETIME2"
				case "mysql", "bigquery":
					typeName = "DATETIME"
				case "snowflake", "databricks":
					typeName = "TIMESTAMP_NTZ"
				default:
					typeName = "TIMESTAMP"
				}
			case "timestamptz":
				switch dialect {
				case "postgres":
					typeName = "TIMESTAMPTZ"
				case "sqlserver":
					typeName = "DATETIMEOFFSET"
				case "snowflake":
					typeName = "TIMESTAMP_TZ"
				case "bigquery", "databricks":
					typeName = "TIMESTAMP"
				default:
					return "", businessError(c, "temporal_type", "unsupported_temporal_type")
				}
			}
			return "CAST(" + marker + " AS " + typeName + ")", nil
		default:
			return marker, nil
		}
	}
	lower, err := appendValue(c.Value)
	if err != nil {
		return "", err
	}
	var predicate string
	if c.Operator == "range" {
		upper, err := appendValue(c.Upper)
		if err != nil {
			return "", err
		}
		lowerOp, upperOp := ">", "<"
		if strings.HasPrefix(c.Bounds, "[") {
			lowerOp = ">="
		}
		if strings.HasSuffix(c.Bounds, "]") {
			upperOp = "<="
		}
		predicate = column + " " + lowerOp + " " + lower + " AND " + column + " " + upperOp + " " + upper
	} else {
		op := map[string]string{"eq": "=", "ne": "<>", "lt": "<", "lte": "<=", "gt": ">", "gte": ">="}[c.Operator]
		if op == "" {
			return "", businessError(c, "operator", "unsupported_operator")
		}
		predicate = column + " " + op + " " + lower
	}
	if c.Nulls == "include" {
		predicate = "(" + predicate + ") OR " + column + " IS NULL"
	}
	return "(" + predicate + ")", nil
}

func businessApplyEdits(statement string, edits []businessEdit) (string, error) {
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out strings.Builder
	position := 0
	for _, edit := range edits {
		if edit.start < position || edit.end < edit.start || edit.end > len(statement) {
			return "", businessSQLFailure("unsupported_overlapping_clause")
		}
		out.WriteString(statement[position:edit.start])
		out.WriteString(edit.text)
		position = edit.end
	}
	out.WriteString(statement[position:])
	return out.String(), nil
}

func businessLayout(statement string, tokens []businessToken, binding Binding) (businessSQLLayout, error) {
	return businessLayoutWithVirtual(statement, tokens, binding, nil)
}

func businessLayoutWithVirtual(statement string, tokens []businessToken, binding Binding, virtual map[string]bool) (businessSQLLayout, error) {
	out := businessSQLLayout{where: -1, having: -1, whereEnd: len(statement), havingEnd: len(statement), rowInsert: len(statement), aggregateInsert: len(statement)}
	if len(tokens) < 4 || !tokens[0].word("select") || tokens[0].depth != 0 {
		return out, businessSQLFailure("unsupported_select_shape")
	}
	from, fromEnd := -1, len(tokens)
	for i, token := range tokens {
		if i > 0 && token.word("select") {
			return out, businessSQLFailure("unsupported_nested_select")
		}
		for _, forbidden := range []string{"with", "union", "intersect", "except", "into", "values", "returning", "for", "qualify", "tablesample", "pivot", "unpivot", "connect", "match_recognize"} {
			if token.word(forbidden) {
				return out, businessSQLFailure("unsupported_select_shape")
			}
		}
		if token.text == ";" {
			if i != len(tokens)-1 || token.depth != 0 {
				return out, businessSQLFailure("unsupported_multiple_statements")
			}
			out.whereEnd, out.havingEnd, out.rowInsert, out.aggregateInsert = token.start, token.start, token.start, token.start
		}
		if token.depth != 0 {
			continue
		}
		if token.word("from") {
			if from >= 0 {
				return out, businessSQLFailure("unsupported_multiple_from")
			}
			from = i
		}
		if token.word("sum") || token.word("avg") || token.word("min") || token.word("max") || token.word("count") {
			out.hasAggregate = true
		}
		if from < 0 {
			continue
		}
		if token.word("where") || token.word("group") || token.word("having") || token.word("order") || token.word("limit") || token.word("offset") || token.word("fetch") {
			if fromEnd == len(tokens) {
				fromEnd = i
			}
		}
		if token.word("where") {
			if out.where >= 0 {
				return out, businessSQLFailure("unsupported_multiple_where")
			}
			out.where = token.start
		}
		if token.word("having") {
			if out.having >= 0 {
				return out, businessSQLFailure("unsupported_multiple_having")
			}
			out.having = token.start
		}
		if token.word("group") {
			out.hasAggregate = true
		}
	}
	if from < 0 || from+1 >= fromEnd {
		return out, businessSQLFailure("unsupported_missing_from")
	}
	for i := from + 1; i < fromEnd; i++ {
		if tokens[i].depth != 0 || !tokens[i].word("left") && !tokens[i].word("right") && !tokens[i].word("full") {
			continue
		}
		j := i + 1
		if j < fromEnd && tokens[j].word("outer") {
			j++
		}
		if j < fromEnd && tokens[j].depth == 0 && tokens[j].word("join") {
			out.hasOuterJoin = true
		}
	}
	for _, token := range tokens {
		if token.depth != 0 {
			continue
		}
		if token.word("group") || token.word("having") || token.word("order") || token.word("limit") || token.word("offset") || token.word("fetch") {
			if token.start < out.rowInsert {
				out.rowInsert = token.start
			}
			if out.where >= 0 && token.start > out.where && token.start < out.whereEnd {
				out.whereEnd = token.start
			}
		}
		if token.word("order") || token.word("limit") || token.word("offset") || token.word("fetch") {
			if token.start < out.aggregateInsert {
				out.aggregateInsert = token.start
			}
			if out.having >= 0 && token.start > out.having && token.start < out.havingEnd {
				out.havingEnd = token.start
			}
		}
	}
	aliases := map[string]bool{}
	for i := from + 1; i < fromEnd; i++ {
		if i != from+1 && (tokens[i-1].depth != 0 || !tokens[i-1].word("join") && tokens[i-1].text != ",") {
			continue
		}
		if tokens[i].depth != 0 {
			return out, businessSQLFailure("unsupported_derived_relation")
		}
		source, next, err := businessReadRange(tokens, i, fromEnd, binding, virtual)
		if err != nil {
			return out, err
		}
		if aliases[source.alias] {
			return out, businessSQLFailure("unsupported_duplicate_alias")
		}
		aliases[source.alias] = true
		out.ranges = append(out.ranges, source)
		i = next - 1
	}
	if len(out.ranges) < 1 || len(out.ranges) > 32 {
		return out, businessSQLFailure("unsupported_relation_count")
	}
	return out, nil
}

func businessReadRange(tokens []businessToken, start, end int, binding Binding, virtual map[string]bool) (businessRange, int, error) {
	var parts []string
	index := start
	for index < end {
		token := tokens[index]
		if token.kind == 'q' && binding.Dialect == "bigquery" && strings.Contains(token.text, ".") {
			for _, part := range strings.Split(token.text, ".") {
				if !SQLIdentifierForDialect(binding.Dialect, part) {
					return businessRange{}, index, businessSQLFailure("unsupported_relation_name")
				}
				parts = append(parts, part)
			}
		} else {
			name, ok := businessName(token, binding.Dialect)
			if !ok {
				return businessRange{}, index, businessSQLFailure("unsupported_relation_name")
			}
			parts = append(parts, name)
		}
		index++
		if index < end && tokens[index].text == "." {
			index++
			continue
		}
		break
	}
	if len(parts) == 1 && virtual[parts[0]] {
		alias := parts[0]
		if index < end && tokens[index].word("as") {
			index++
			if index >= end || tokens[index].kind != 'w' && tokens[index].kind != 'q' {
				return businessRange{}, index, businessSQLFailure("invalid_relation_alias")
			}
		}
		if index < end && (tokens[index].kind == 'w' || tokens[index].kind == 'q') {
			reserved := false
			for _, word := range []string{"join", "inner", "outer", "left", "right", "full", "cross", "natural", "on", "using"} {
				reserved = reserved || tokens[index].word(word)
			}
			if !reserved {
				var ok bool
				alias, ok = businessName(tokens[index], binding.Dialect)
				if !ok {
					return businessRange{}, index, businessSQLFailure("invalid_relation_alias")
				}
				index++
			}
		}
		return businessRange{alias: alias, virtual: parts[0]}, index, nil
	}
	if len(parts) < 2 || len(parts) > 3 || len(parts) == 3 && parts[0] != binding.Catalog {
		return businessRange{}, index, businessSQLFailure("unsupported_unqualified_or_foreign_relation")
	}
	var relation Relation
	matches := 0
	for _, candidate := range binding.Relations {
		if candidate.Schema == parts[len(parts)-2] && candidate.Name == parts[len(parts)-1] {
			relation, matches = candidate, matches+1
		}
	}
	if matches != 1 {
		return businessRange{}, index, businessSQLFailure("foreign_relation")
	}
	alias := relation.Name
	if index < end && tokens[index].word("as") {
		index++
		if index >= end {
			return businessRange{}, index, businessSQLFailure("invalid_relation_alias")
		}
		name, ok := businessName(tokens[index], binding.Dialect)
		if !ok {
			return businessRange{}, index, businessSQLFailure("invalid_relation_alias")
		}
		alias = name
		index++
	} else if index < end {
		token := tokens[index]
		reserved := false
		for _, word := range []string{"join", "inner", "outer", "left", "right", "full", "cross", "natural", "on", "using"} {
			reserved = reserved || token.word(word)
		}
		if !reserved && (token.kind == 'w' || token.kind == 'q') {
			name, ok := businessName(token, binding.Dialect)
			if !ok {
				return businessRange{}, index, businessSQLFailure("invalid_relation_alias")
			}
			alias = name
			index++
		}
	}
	return businessRange{relation: relation, alias: alias}, index, nil
}
