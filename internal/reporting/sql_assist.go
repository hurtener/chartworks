package reporting

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	pgquery "github.com/wasilibs/go-pgquery"
)

// SQL assistance edits only source spans identified by PostgreSQL's grammar.
// Comparing the full reparsed AST to the intended AST proves that unrelated
// filters, joins, aliases, literals and output definitions were not rewritten.
// This is not an execution validator: every resulting draft needs the existing
// native read validator and real source evidence before it can be published.
type sqlEdit struct {
	start, end int
	text       string
}

func astObject(v any) map[string]any { m, _ := v.(map[string]any); return m }
func astArray(v any) []any           { a, _ := v.([]any); return a }
func astString(v any) string         { s, _ := v.(string); return s }
func astInteger(v any) (int, bool) {
	n, ok := v.(float64)
	return int(n), ok && n >= 0 && n <= 1<<20 && float64(int(n)) == n
}

func walkAST(v any, depth int, budget *int, fn func(map[string]any) error) error {
	*budget--
	if depth > 64 || *budget < 0 {
		return ErrInvalid
	}
	switch value := v.(type) {
	case map[string]any:
		if err := fn(value); err != nil {
			return err
		}
		for _, child := range value {
			if err := walkAST(child, depth+1, budget, fn); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := walkAST(child, depth+1, budget, fn); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseAssistance(ctx context.Context, statement string) (map[string]any, map[string]any, error) {
	if ctx == nil || ctx.Err() != nil || statement == "" || len(statement) > 64<<10 {
		return nil, nil, ErrInvalid
	}
	raw, err := pgquery.ParseToJSON(statement)
	if err != nil || len(raw) > 4<<20 {
		return nil, nil, ErrInvalid
	}
	var document map[string]any
	if json.Unmarshal([]byte(raw), &document) != nil {
		return nil, nil, ErrInvalid
	}
	budget := 20000
	if err := walkAST(document, 0, &budget, func(map[string]any) error { return nil }); err != nil {
		return nil, nil, err
	}
	statements := astArray(document["stmts"])
	if len(statements) != 1 {
		return nil, nil, ErrInvalid
	}
	root := astObject(astObject(statements[0])["stmt"])
	selectNode := astObject(root["SelectStmt"])
	if len(root) != 1 || selectNode == nil {
		return nil, nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return document, selectNode, nil
}

func equivalentAST(a, b any) bool {
	left, right := clone(a), clone(b)
	strip := func(m map[string]any) error {
		delete(m, "location")
		delete(m, "stmt_location")
		delete(m, "stmt_len")
		return nil
	}
	budget := 20000
	if walkAST(left, 0, &budget, strip) != nil {
		return false
	}
	budget = 20000
	if walkAST(right, 0, &budget, strip) != nil {
		return false
	}
	return reflect.DeepEqual(left, right)
}

func applySQLEdits(ctx context.Context, original string, expected map[string]any, edits []sqlEdit) (string, error) {
	if len(edits) == 0 || len(edits) > 512 {
		return "", ErrInvalid
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })
	var out strings.Builder
	previous := 0
	for _, edit := range edits {
		if edit.start < previous || edit.start < 0 || edit.end < edit.start || edit.end > len(original) {
			return "", ErrInvalid
		}
		out.WriteString(original[previous:edit.start])
		out.WriteString(edit.text)
		previous = edit.end
	}
	out.WriteString(original[previous:])
	statement := out.String()
	actual, _, err := parseAssistance(ctx, statement)
	if err != nil || !equivalentAST(expected, actual) {
		return "", ErrInvalid
	}
	return statement, nil
}

func columnNames(node any) ([]string, bool) {
	column := astObject(astObject(node)["ColumnRef"])
	fields := astArray(column["fields"])
	if len(fields) == 0 || len(fields) > 4 {
		return nil, false
	}
	out := make([]string, len(fields))
	for i, field := range fields {
		name := astString(astObject(astObject(field)["String"])["sval"])
		if name == "" || !text(name, 128) {
			return nil, false
		}
		out[i] = name
	}
	return out, true
}

func literalNode(node any) map[string]any {
	m := astObject(node)
	if constant := astObject(m["A_Const"]); constant != nil {
		return m
	}
	if cast := astObject(m["TypeCast"]); cast != nil {
		return literalNode(cast["arg"])
	}
	return nil
}

func temporalLiteral(s string) bool {
	if len(s) > 64 {
		return false
	}
	if date, err := time.Parse("2006-01-02", s); err == nil {
		return date.Year() > 0 && date.Format("2006-01-02") == s
	}
	date, err := time.Parse(time.RFC3339Nano, s)
	return err == nil && date.Year() > 0 && date.Year() <= 9999
}

// quotedLiteralSpan rejects escape/dollar-prefixed and implicitly concatenated
// strings; those more complex forms remain ordinary manually amendable SQL.
func quotedLiteralSpan(sql string, start int, wanted string) (int, error) {
	if start < 0 || start >= len(sql) || sql[start] != '\'' {
		return 0, ErrInvalid
	}
	var value strings.Builder
	for i := start + 1; i < len(sql); i++ {
		if sql[i] != '\'' {
			value.WriteByte(sql[i])
			continue
		}
		if i+1 < len(sql) && sql[i+1] == '\'' {
			value.WriteByte('\'')
			i++
			continue
		}
		if value.String() != wanted {
			return 0, ErrInvalid
		}
		return i + 1, nil
	}
	return 0, ErrInvalid
}

// parameterizePeriod accepts exactly one half-open temporal range in a root
// conjunction. It never turns an OR, BETWEEN, nested query or an unrelated date
// literal into a parameter by pattern matching textual SQL.
func parameterizePeriod(ctx context.Context, sql string, column []string, slot int) (string, error) {
	if len(column) < 1 || len(column) > 4 || slot < 1 || slot > 63 {
		return "", ErrInvalid
	}
	for _, name := range column {
		if name == "" || !text(name, 128) {
			return "", ErrInvalid
		}
	}
	document, root, err := parseAssistance(ctx, sql)
	if err != nil {
		return "", err
	}
	predicates := []any{}
	var conjunct func(any) error
	conjunct = func(value any) error {
		m := astObject(value)
		if b := astObject(m["BoolExpr"]); b != nil {
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
		if len(predicates) >= 256 {
			return ErrInvalid
		}
		predicates = append(predicates, value)
		return nil
	}
	if err := conjunct(root["whereClause"]); err != nil {
		return "", err
	}
	seen := map[string]bool{}
	edits := []sqlEdit{}
	for _, predicate := range predicates {
		expression := astObject(astObject(predicate)["A_Expr"])
		if expression == nil || astString(expression["kind"]) != "AEXPR_OP" {
			continue
		}
		names := astArray(expression["name"])
		if len(names) != 1 {
			continue
		}
		op := astString(astObject(astObject(names[0])["String"])["sval"])
		if op != ">=" && op != "<" {
			continue
		}
		fields, ok := columnNames(expression["lexpr"])
		if !ok || !reflect.DeepEqual(fields, column) {
			continue
		}
		constantNode := literalNode(expression["rexpr"])
		constant := astObject(constantNode["A_Const"])
		literal := astString(astObject(constant["sval"])["sval"])
		start, ok := astInteger(constant["location"])
		if !ok || !temporalLiteral(literal) || seen[op] {
			return "", ErrInvalid
		}
		end, err := quotedLiteralSpan(sql, start, literal)
		if err != nil {
			return "", err
		}
		number := slot
		if op == "<" {
			number++
		}
		edits = append(edits, sqlEdit{start: start, end: end, text: "$" + strconv.Itoa(number)})
		delete(constantNode, "A_Const")
		constantNode["ParamRef"] = map[string]any{"number": float64(number), "location": float64(start)}
		seen[op] = true
	}
	if len(seen) != 2 {
		return "", ErrInvalid
	}
	return applySQLEdits(ctx, sql, document, edits)
}
