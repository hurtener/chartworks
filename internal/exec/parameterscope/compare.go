// Package parameterscope compares parameter roles in bounded PostgreSQL parser
// output. It does not parse SQL or establish native execution authority. The exec
// owner supplies pinned-parser JSON and performs normal whole-query validation.
package parameterscope

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	// ErrBinding denotes missing, malformed or changed positional roles.
	ErrBinding = errors.New("parameter-scope: invalid or changed binding")
	// ErrUnsupported denotes a SELECT graph not admitted by this custody policy.
	ErrUnsupported = errors.New("parameter-scope: unsupported input shape")
	// ErrLimit denotes an exceeded input or traversal bound.
	ErrLimit = errors.New("parameter-scope: work limit exceeded")
)

// Check compares two outputs from the pinned PostgreSQL parser. Values remain
// outside this package: only native syntax and slot positions are considered.
// Passing this check does not certify SQL safety, answer correctness or values.
func Check(ctx context.Context, beforeJSON, afterJSON string, count int) error {
	if ctx == nil || count < 1 || count > 64 {
		return ErrBinding
	}
	for _, raw := range []string{beforeJSON, afterJSON} {
		if len(raw) == 0 || len(raw) > 4<<20 {
			return ErrLimit
		}
		if !utf8.ValidString(raw) {
			return ErrBinding
		}
	}
	original, err := signature(ctx, beforeJSON, count)
	if err != nil {
		return err
	}
	candidate, err := signature(ctx, afterJSON, count)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(original, candidate) {
		return ErrBinding
	}
	return nil
}

func signature(ctx context.Context, raw string, count int) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(raw) > 4<<20 {
		return nil, ErrLimit
	}
	var tree map[string]any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&tree) != nil || decoder.Decode(new(any)) != io.EOF {
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
	if err := parameterSelectShape(selectNode); err != nil {
		return nil, err
	}
	scan := parameterClauseScan{ctx: ctx, count: count, seen: map[int]bool{}}
	result := map[string]any{}
	normalizedClauses := map[string]any{}
	boundClauses := map[string]bool{}
	for name, value := range selectNode {
		if name == "location" || name == "stmt_location" || name == "stmt_len" {
			continue
		}
		if name == "larg" || name == "rarg" {
			// At the root these are unwrapped set-operation SELECT bodies.
			if err := parameterSelectShape(object(value)); err != nil {
				return nil, err
			}
		}
		normalized, hasParameter, err := scan.visit(value, 0)
		if err != nil {
			return nil, err
		}
		normalizedClauses[name] = normalized
		boundClauses[name] = hasParameter
		if hasParameter || parameterNamespaceClause(name) {
			result[name] = normalized
		}
	}
	if len(scan.seen) != count {
		return nil, ErrBinding
	}
	// An inner SELECT can export a slot as a column, e.g. SELECT $1 AS bound.
	// A changed outer WHERE may then repurpose it without adding any ParamRef.
	// Freeze all outer consumers until a narrower typed dataflow proof exists.
	if scan.exportsParameter {
		return normalizedClauses, nil
	}
	// FETCH WITH TIES and ordinary LIMIT assign a different meaning to the same
	// bound limit; include its companion option when either limit carries a slot.
	if boundClauses["limitCount"] || boundClauses["limitOffset"] {
		result["limitOption"] = normalizedClauses["limitOption"]
	}
	// ORDER BY, GROUP BY and DISTINCT ON can resolve output names/ordinals.
	// Keeping the bound clause alone would let a changed target alias redirect
	// the same private slot to a different column or expression.
	if boundClauses["sortClause"] || boundClauses["groupClause"] || boundClauses["distinctClause"] || boundClauses["windowClause"] {
		result["targetList"] = normalizedClauses["targetList"]
	}
	if boundClauses["groupClause"] {
		result["groupDistinct"] = normalizedClauses["groupDistinct"]
	}
	// Named windows can carry slots without a ParamRef at their use site. Freeze
	// their output/order consumers rather than guess a transitive dependency.
	if boundClauses["windowClause"] || boundClauses["distinctClause"] {
		result["sortClause"] = normalizedClauses["sortClause"]
	}
	return result, nil
}

type parameterClauseScan struct {
	ctx              context.Context
	count, nodes     int
	seen             map[int]bool
	exportsParameter bool
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
			case "SelectStmt":
				if err := parameterSelectShape(object(item)); err != nil {
					return nil, false, err
				}
			case "larg", "rarg":
				// JoinExpr uses the same child field names, but not SELECT bodies.
				// Only a set-operation SelectStmt's operands are raw SelectStmts.
				if op := text(node["op"]); op != "" && op != "SETOP_NONE" {
					if err := parameterSelectShape(object(item)); err != nil {
						return nil, false, err
					}
				}
			case "withClause", "WithClause":
				with := fieldObject(item, "WithClause")
				if !only(with, "ctes", "recursive") || truth(with["recursive"]) {
					return nil, false, ErrUnsupported
				}
			case "RangeFunction", "InsertStmt", "UpdateStmt", "DeleteStmt", "MergeStmt", "CallStmt", "CopyStmt":
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
			if has && (key == "targetList" || key == "windowClause") {
				s.exportsParameter = true
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

// These namespaces can be referenced indirectly from a bound clause. Comparing
// the complete subtrees prevents CTE shadowing, changed derived-column aliases,
// changed join predicates, or a replaced named window from reassigning a slot.
func parameterNamespaceClause(name string) bool {
	switch name {
	case "fromClause", "withClause", "windowClause", "larg", "rarg", "op", "all":
		return true
	}
	return false
}

func parameterSelectShape(node map[string]any) error {
	if !only(node, "targetList", "fromClause", "whereClause", "groupClause", "groupDistinct", "havingClause", "windowClause", "withClause", "sortClause", "limitOffset", "limitCount", "limitOption", "distinctClause", "op", "all", "larg", "rarg") {
		return ErrUnsupported
	}
	if value := node["withClause"]; value != nil {
		with := fieldObject(value, "WithClause")
		if !only(with, "ctes", "recursive") || truth(with["recursive"]) {
			return ErrUnsupported
		}
	}
	switch text(node["op"]) {
	case "", "SETOP_NONE":
		if node["larg"] != nil || node["rarg"] != nil || truth(node["all"]) {
			return ErrUnsupported
		}
	case "SETOP_UNION", "SETOP_INTERSECT", "SETOP_EXCEPT":
		if node["larg"] == nil || node["rarg"] == nil || len(array(node["targetList"])) != 0 || len(array(node["fromClause"])) != 0 || node["whereClause"] != nil || node["havingClause"] != nil || len(array(node["groupClause"])) != 0 {
			return ErrUnsupported
		}
	default:
		return ErrUnsupported
	}
	return nil
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func text(v any) string           { s, _ := v.(string); return s }
func truth(v any) bool            { b, _ := v.(bool); return b }
func fieldObject(v any, kind string) map[string]any {
	m := object(v)
	if wrapped, ok := m[kind]; ok && len(m) == 1 {
		return object(wrapped)
	}
	return m
}
func only(m map[string]any, allowed ...string) bool {
	if m == nil {
		return false
	}
	for key := range m {
		if key == "location" {
			continue
		}
		found := false
		for _, a := range allowed {
			if key == a {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
