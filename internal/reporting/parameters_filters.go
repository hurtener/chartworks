package reporting

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// DimensionSetCapacity is a fixed physical bind width. Padding is safe only in
// the positive WHERE IN placement proved by validateBoundedFilterSQL.
const DimensionSetCapacity = 16

func boundedFilterParameter(p Parameter) bool {
	return p.Type == "dimension_set" || p.Type == "date_range" || columnFilterParameter(p)
}

func validateBoundedFilterDeclaration(p Parameter) error {
	if columnFilterParameter(p) {
		return validateColumnFilterDeclaration(p)
	}
	if p.Column != nil || !boundedFilterParameter(p) || !p.Required || p.Default == nil || p.ListLength != 0 || p.Min != "" || p.Max != "" || len(p.Enum) != 0 || p.Dimension == nil || !identity.Identifier(p.Dimension.Topic) || !identity.Identifier(p.Dimension.Version) || !identity.Identifier(p.Dimension.Dimension) {
		return ErrInvalid
	}
	_, err := resolveBoundedFilter(p, *p.Default)
	return err
}

func resolveBoundedFilter(p Parameter, value Value) ([]exec.Parameter, error) {
	if columnFilterParameter(p) {
		return resolveColumnFilter(p, value)
	}
	if value.Range != nil || value.Period != nil || value.Literal != "" {
		return nil, ErrInvalid
	}
	switch p.Type {
	case "dimension_set":
		if value.DateRange != nil || len(value.Items) < 1 || len(value.Items) > DimensionSetCapacity {
			return nil, ErrInvalid
		}
		items := slices.Clone(value.Items)
		slices.Sort(items)
		out := make([]exec.Parameter, DimensionSetCapacity)
		for i := range out {
			out[i].Kind = "null"
		}
		for i, item := range items {
			if i > 0 && item == items[i-1] {
				return nil, ErrInvalid
			}
			bound, err := scalar(Parameter{Type: "dimension_value"}, item)
			if err != nil {
				return nil, err
			}
			out[i] = bound
		}
		return out, nil
	case "date_range":
		if value.DateRange == nil || len(value.Items) != 0 {
			return nil, ErrInvalid
		}
		r := value.DateRange
		start, err := time.Parse("2006-01-02", r.Start)
		end, endErr := time.Parse("2006-01-02", r.EndExclusive)
		if err != nil || endErr != nil || start.Year() < 1 || end.Year() > 9999 || start.Format("2006-01-02") != r.Start || end.Format("2006-01-02") != r.EndExclusive || !start.Before(end) || end.Sub(start) > 36625*24*time.Hour {
			return nil, ErrInvalid
		}
		return []exec.Parameter{{Kind: "text", Value: r.Start}, {Kind: "text", Value: r.EndExclusive}}, nil
	default:
		return nil, ErrInvalid
	}
}

// validateBoundedFilterSQL proves the meaning of the new bind shapes even for
// arbitrary native block authoring/import. This never replaces read validation.
// Every protected slot occurs exactly once in a root WHERE conjunction: a set
// occupies one positive IN list, and a date range is >= start AND < end on the
// same direct column. No negation, OR, nested/projection reuse, casts or partial
// lists can change NULL padding into another predicate's business meaning.
func validateBoundedFilterSQL(ctx context.Context, sql string, parameters []Parameter) error {
	type constraint struct {
		kind         string
		first, width int
	}
	constraints := []constraint{}
	protected := map[int]bool{}
	slot := 1
	for _, p := range parameters {
		width := scalarSlots([]Parameter{p})
		if boundedFilterParameter(p) {
			constraints = append(constraints, constraint{p.Type, slot, width})
			for n := slot; n < slot+width; n++ {
				protected[n] = true
			}
		}
		slot += width
	}
	if len(constraints) == 0 {
		return nil
	}
	document, root, err := parseAssistance(ctx, sql)
	if err != nil {
		return err
	}
	counts := map[int]int{}
	selects := 0
	budget := 20000
	if err := walkAST(document, 0, &budget, func(node map[string]any) error {
		if node["SelectStmt"] != nil {
			selects++
			if selects > 1 {
				return unsupportedPreparation("filter_nested_query_unsupported")
			}
		}
		if node["SubLink"] != nil || node["WithClause"] != nil || node["CommonTableExpr"] != nil {
			return unsupportedPreparation("filter_nested_query_unsupported")
		}
		if p := astObject(node["ParamRef"]); p != nil {
			n, ok := astInteger(p["number"])
			if !ok {
				return ErrInvalid
			}
			if protected[n] {
				counts[n]++
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for n := range protected {
		if counts[n] != 1 {
			return ErrInvalid
		}
	}
	leaves := []map[string]any{}
	var conjunct func(any) error
	conjunct = func(value any) error {
		node := astObject(value)
		if b := astObject(node["BoolExpr"]); b != nil {
			if astString(b["boolop"]) != "AND_EXPR" {
				return ErrInvalid
			}
			for _, child := range astArray(b["args"]) {
				if err := conjunct(child); err != nil {
					return err
				}
			}
			return nil
		}
		if node == nil {
			return ErrInvalid
		}
		leaves = append(leaves, node)
		return nil
	}
	if err := conjunct(root["whereClause"]); err != nil {
		return err
	}
	parameterNumber := func(node any) int {
		m := astObject(node)
		if len(m) != 1 {
			return 0
		}
		n, ok := astInteger(astObject(m["ParamRef"])["number"])
		if !ok {
			return 0
		}
		return n
	}
	operator := func(expression map[string]any) string {
		names := astArray(expression["name"])
		if len(names) != 1 {
			return ""
		}
		return astString(astObject(astObject(names[0])["String"])["sval"])
	}
	for _, c := range constraints {
		found := false
		startColumn, endColumn := "", ""
		for _, leaf := range leaves {
			expression := astObject(leaf["A_Expr"])
			column, ok := columnNames(expression["lexpr"])
			if !ok {
				continue
			}
			key := strings.Join(column, "\x00")
			if c.kind == "column_value" && astString(expression["kind"]) == "AEXPR_OP" && operator(expression) == "=" && parameterNumber(expression["rexpr"]) == c.first {
				found = true
			}
			if (c.kind == "dimension_set" || c.kind == "column_set") && astString(expression["kind"]) == "AEXPR_IN" && operator(expression) == "=" {
				items := astArray(astObject(astObject(expression["rexpr"])["List"])["items"])
				if len(items) != c.width {
					continue
				}
				match := true
				for i, item := range items {
					if parameterNumber(item) != c.first+i {
						match = false
					}
				}
				if match {
					found = true
				}
			}
			if (c.kind == "date_range" || c.kind == "column_range") && astString(expression["kind"]) == "AEXPR_OP" {
				n := parameterNumber(expression["rexpr"])
				if operator(expression) == ">=" && n == c.first {
					startColumn = key
				}
				if operator(expression) == "<" && n == c.first+1 {
					endColumn = key
				}
			}
		}
		if c.kind == "date_range" || c.kind == "column_range" {
			found = startColumn != "" && startColumn == endColumn
		}
		if !found {
			return ErrInvalid
		}
	}
	return ctx.Err()
}

func validateBoundedFilterBinding(ctx context.Context, d Definition, binding exec.Binding) error {
	for _, p := range d.Parameters {
		if boundedFilterParameter(p) && binding.Dialect != "postgres" {
			return ErrInvalid
		}
	}
	return validateBoundedFilterSQL(ctx, d.SQL, d.Parameters)
}

// validateBoundedFilterSemantics binds the parser-proved column to the exact
// reviewed dimension and source relation. Native authors cannot relabel another
// column as this dimension. Only one direct relation is supported for new types.
func validateBoundedFilterSemantics(ctx context.Context, d Definition, binding exec.Binding, publications []topics.Published) error {
	if err := validateBoundedFilterBinding(ctx, d, binding); err != nil {
		return err
	}
	_, root, err := parseAssistance(ctx, d.SQL)
	if err != nil {
		return err
	}
	from := astArray(root["fromClause"])
	if len(from) != 1 {
		return unsupportedPreparation("filter_relation_shape_unsupported")
	}
	relationNode := astObject(astObject(from[0])["RangeVar"])
	if relationNode == nil || astString(relationNode["catalogname"]) != "" {
		return unsupportedPreparation("filter_relation_shape_unsupported")
	}
	schema, name := astString(relationNode["schemaname"]), astString(relationNode["relname"])
	aliasNode := astObject(relationNode["alias"])
	alias := astString(aliasNode["aliasname"])
	if len(astArray(aliasNode["colnames"])) != 0 {
		return unsupportedPreparation("filter_relation_shape_unsupported")
	}
	var physical *exec.Relation
	for i := range binding.Relations {
		r := &binding.Relations[i]
		if r.Schema == schema && r.Name == name {
			if physical != nil {
				return ErrInvalid
			}
			physical = r
		}
	}
	if physical == nil {
		return ErrStale
	}
	slot := 1
	for _, parameter := range d.Parameters {
		first := slot
		slot += scalarSlots([]Parameter{parameter})
		if !boundedFilterParameter(parameter) {
			continue
		}
		if parameter.Dimension == nil && !columnFilterParameter(parameter) {
			return ErrInvalid
		}
		var dimension *semantics.Dimension
		var column *semantics.Column
		if columnFilterParameter(parameter) {
			resolved, err := resolveFilterColumn(d, parameter, binding, publications)
			if err != nil {
				return err
			}
			if parameter.Column.SourceDataset.Dataset != physical.ID {
				return ErrInvalid
			}
			column = &resolved
		} else {
			for _, publication := range publications {
				if publication.Definition.Topic != parameter.Dimension.Topic || publication.Definition.Version != parameter.Dimension.Version {
					continue
				}
				for i := range publication.Definition.Dimensions {
					candidate := &publication.Definition.Dimensions[i]
					if candidate.ID == parameter.Dimension.Dimension {
						if dimension != nil {
							return ErrInvalid
						}
						dimension = candidate
					}
				}
				if dimension != nil {
					for _, dataset := range publication.Definition.Datasets {
						if dataset.ID == dimension.Field.Dataset {
							if dataset.Source.Source != binding.Source || dataset.Source.Context != binding.Context || dataset.Source.SourceRevision != binding.Revision {
								return ErrStale
							}
							for i := range dataset.Columns {
								candidate := &dataset.Columns[i]
								if candidate.ID == dimension.Field.ID {
									if column != nil {
										return ErrInvalid
									}
									column = candidate
								}
							}
						}
					}
				}
			}
			if dimension == nil || column == nil || dimension.Field.Kind != semantics.KindColumn || dimension.Field.Dataset != physical.ID || len(dimension.Filters) != 0 || dimension.Temporal != nil {
				return unsupportedPreparation("filter_dimension_unsupported")
			}
			if parameter.Type == "dimension_set" && !authoringTextColumn(*column) || parameter.Type == "date_range" && (!authoringDateColumn(*column) || dimension.Role != semantics.DimensionTemporal) {
				return unsupportedPreparation("filter_type_unsupported")
			}
		}
		found := false
		for _, actual := range physical.Columns {
			if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
				found = true
			}
		}
		if !found {
			return ErrStale
		}
		var used []string
		budget := 20000
		err = walkAST(root["whereClause"], 0, &budget, func(node map[string]any) error {
			expression := astObject(node["A_Expr"])
			if expression == nil {
				return nil
			}
			right := expression["rexpr"]
			if parameter.Type == "dimension_set" || parameter.Type == "column_set" {
				items := astArray(astObject(astObject(right)["List"])["items"])
				if len(items) == 0 {
					return nil
				}
				right = items[0]
			}
			n, ok := astInteger(astObject(astObject(right)["ParamRef"])["number"])
			if ok && n == first {
				used, _ = columnNames(expression["lexpr"])
			}
			return nil
		})
		if err != nil {
			return err
		}
		matches := len(used) == 1 && used[0] == column.SourceName
		if len(used) == 2 {
			qualifier := name
			if alias != "" {
				qualifier = alias
			}
			matches = used[0] == qualifier && used[1] == column.SourceName
		}
		if len(used) == 3 {
			matches = alias == "" && used[0] == schema && used[1] == name && used[2] == column.SourceName
		}
		if !matches {
			return unsupportedPreparation("filter_dimension_mismatch")
		}
	}
	return nil
}
