package exec

import (
	"context"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	pgquery "github.com/wasilibs/go-pgquery"
)

// CheckParameterContinuity checks that an edit has not repurposed retained
// positional bindings. It does not issue a native plan, grant source access or
// certify the rest of the query. The caller still restores the protected values
// and performs ordinary whole-query validation before any execution.
//
// PostgreSQL permits edits outside parameter-bearing top-level clauses of a
// single SELECT. The source/alias namespace and the complete parameter-bearing
// clauses remain identical (ignoring parser locations only). Nested scopes and
// set operations are deliberately unsupported. Other dialects require unchanged
// SQL until their native binding-role proof is implemented; no lexical guess is
// used to authorize a changed parameterized query.
func CheckParameterContinuity(ctx context.Context, dialect, before, after string, parameterCount int) error {
	if ctx == nil || parameterCount < 1 || parameterCount > 64 {
		return ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, sql := range []string{before, after} {
		if len(sql) == 0 || len(sql) > 32<<10 || !utf8.ValidString(sql) || strings.ContainsRune(sql, 0) || strings.Count(sql, "(") > 256 {
			return ErrLimit
		}
	}
	if dialect != "postgres" {
		if dialect == "" {
			return ErrBinding
		}
		if before != after {
			return ErrUnsupported
		}
		return nil
	}
	original, err := parameterClauses(ctx, before, parameterCount)
	if err != nil {
		return err
	}
	candidate, err := parameterClauses(ctx, after, parameterCount)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(original, candidate) {
		return ErrBinding
	}
	return nil
}

func parameterClauses(ctx context.Context, sql string, count int) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := pgquery.ParseToJSON(sql)
	if err != nil {
		return nil, ErrUnsupported
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, ErrLimit
	}
	var tree map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&tree) != nil {
		return nil, ErrBinding
	}
	stmts := array(tree["stmts"])
	if len(stmts) != 1 {
		return nil, ErrUnsupported
	}
	root := object(object(stmts[0])["stmt"])
	if len(root) != 1 || root["SelectStmt"] == nil {
		return nil, ErrUnsupported
	}
	selectNode := object(root["SelectStmt"])
	if !only(selectNode, "targetList", "fromClause", "whereClause", "groupClause", "groupDistinct", "havingClause", "sortClause", "limitOffset", "limitCount", "limitOption", "distinctClause", "op") || text(selectNode["op"]) != "" && text(selectNode["op"]) != "SETOP_NONE" {
		return nil, ErrUnsupported
	}
	scan := parameterClauseScan{ctx: ctx, count: count, seen: map[int]bool{}}
	result := map[string]any{}
	for name, value := range selectNode {
		normalized, hasParameter, err := scan.visit(value, 0)
		if err != nil {
			return nil, err
		}
		if hasParameter || name == "fromClause" {
			result[name] = normalized
		}
	}
	if len(scan.seen) != count {
		return nil, ErrBinding
	}
	// FETCH WITH TIES and ordinary LIMIT assign a different meaning to the same
	// bound limit; include its companion option when either limit carries a slot.
	if result["limitCount"] != nil || result["limitOffset"] != nil {
		result["limitOption"] = selectNode["limitOption"]
	}
	return result, nil
}

type parameterClauseScan struct {
	ctx          context.Context
	count, nodes int
	seen         map[int]bool
}

func (s *parameterClauseScan) visit(value any, depth int) (any, bool, error) {
	s.nodes++
	if s.nodes > 32768 || depth > 128 {
		return nil, false, ErrLimit
	}
	if err := s.ctx.Err(); err != nil {
		return nil, false, err
	}
	switch node := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		contains := false
		for key, item := range node {
			switch key {
			case "location", "stmt_location", "stmt_len":
				continue
			case "SelectStmt", "SubLink", "RangeSubselect", "RangeFunction", "JoinExpr", "WithClause", "WindowDef":
				return nil, false, ErrUnsupported
			case "ParamRef":
				p := object(item)
				number, ok := p["number"].(json.Number)
				n, err := strconv.Atoi(string(number))
				if !ok || err != nil || n < 1 || n > s.count {
					return nil, false, ErrBinding
				}
				s.seen[n] = true
				contains = true
			}
			n, has, err := s.visit(item, depth+1)
			if err != nil {
				return nil, false, err
			}
			out[key] = n
			contains = contains || has
		}
		return out, contains, nil
	case []any:
		out := make([]any, len(node))
		contains := false
		for i, item := range node {
			n, has, err := s.visit(item, depth+1)
			if err != nil {
				return nil, false, err
			}
			out[i] = n
			contains = contains || has
		}
		return out, contains, nil
	default:
		return value, false, nil
	}
}
