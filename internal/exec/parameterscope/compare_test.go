package parameterscope

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type node = map[string]any

// These are synthetic native-AST shapes for the dependency-free comparator.
// Parser-to-JSON compatibility and SQL results have separate exec/acceptance
// tests; passing this suite is not a substitute for running those native gates.
func col(name string) node {
	return node{"ColumnRef": node{"fields": []any{node{"String": node{"sval": name}}}}}
}
func param(n int) node { return node{"ParamRef": node{"number": n, "location": 12}} }
func target(value any) node {
	return node{"ResTarget": node{"name": "result", "val": value, "location": 7}}
}
func relation(name string) node {
	return node{"RangeVar": node{"schemaname": "analytics", "relname": name, "inh": true, "location": 9}}
}
func expr(left, right any) node {
	return node{"A_Expr": node{"kind": "AEXPR_OP", "name": []any{node{"String": node{"sval": ">"}}}, "lexpr": left, "rexpr": right, "location": 4}}
}
func base() node {
	return node{"op": "SETOP_NONE", "targetList": []any{target(col("id"))}, "fromClause": []any{relation("sales")}, "whereClause": expr(col("amount"), param(1)), "limitOption": "LIMIT_OPTION_DEFAULT"}
}
func nativeJSON(q node) string {
	raw, _ := json.Marshal(node{"version": 170004, "stmts": []any{node{"stmt": node{"SelectStmt": q}, "stmt_location": 0, "stmt_len": 84}}})
	return string(raw)
}
func clone(q node) node {
	raw, _ := json.Marshal(q)
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var out node
	_ = dec.Decode(&out)
	return out
}
func with(inner node) node {
	q := base()
	delete(q, "whereClause")
	q["fromClause"] = []any{node{"RangeVar": node{"relname": "filtered"}}}
	q["withClause"] = node{"ctes": []any{node{"CommonTableExpr": node{"ctename": "filtered", "ctematerialized": "CTEMaterializeDefault", "ctequery": node{"SelectStmt": inner}}}}}
	return q
}
func checkPair(t *testing.T, a, b node, count int, want error) {
	t.Helper()
	err := Check(context.Background(), nativeJSON(a), nativeJSON(b), count)
	if !errors.Is(err, want) {
		t.Fatalf("want %v, got %v", want, err)
	}
}

func TestSQLRecoveryParameterScopeCoreImmutableGraphs(t *testing.T) {
	graphs := map[string]node{}
	graphs["flat"] = base()
	graphs["cte"] = with(base())
	derived := base()
	delete(derived, "whereClause")
	derived["fromClause"] = []any{node{"RangeSubselect": node{"subquery": node{"SelectStmt": base()}, "alias": node{"aliasname": "filtered"}}}}
	graphs["derived"] = derived
	join := base()
	join["fromClause"] = []any{node{"JoinExpr": node{"jointype": "JOIN_INNER", "larg": relation("sales"), "rarg": relation("items"), "quals": expr(col("id"), param(1))}}}
	delete(join, "whereClause")
	graphs["join"] = join
	sublink := base()
	sublink["whereClause"] = node{"SubLink": node{"subLinkType": "ANY_SUBLINK", "testexpr": col("id"), "subselect": node{"SelectStmt": base()}}}
	graphs["sublink"] = sublink
	set := node{"op": "SETOP_UNION", "all": true, "larg": base(), "rarg": base(), "limitOption": "LIMIT_OPTION_DEFAULT"}
	graphs["union"] = set
	win := base()
	delete(win, "whereClause")
	win["windowClause"] = []any{node{"WindowDef": node{"name": "frame", "partitionClause": []any{col("id")}, "startOffset": param(1), "frameOptions": json.Number("123")}}}
	graphs["named_window"] = win
	inline := base()
	delete(inline, "whereClause")
	inline["targetList"] = []any{target(node{"FuncCall": node{"funcname": []any{node{"String": node{"sval": "sum"}}}, "args": []any{col("amount")}, "over": node{"WindowDef": node{"startOffset": param(1)}}}})}
	graphs["inline_window"] = inline
	scalar := base()
	delete(scalar, "whereClause")
	scalar["targetList"] = []any{target(node{"SubLink": node{"subLinkType": "EXPR_SUBLINK", "subselect": node{"SelectStmt": base()}}})}
	graphs["scalar"] = scalar
	for name, q := range graphs {
		t.Run(name, func(t *testing.T) {
			before := nativeJSON(q)
			edited := clone(q)
			edited["limitCount"] = node{"A_Const": node{"ival": node{"ival": 2}}}
			checkPair(t, q, edited, 1, nil)
			if name != "named_window" {
				edited["sortClause"] = []any{node{"SortBy": node{"node": col("id"), "sortby_dir": "SORTBY_DESC"}}}
				checkPair(t, q, edited, 1, nil)
			}
			if nativeJSON(q) != before {
				t.Fatal("comparison mutated caller AST")
			}
		})
	}
}

func TestSQLRecoveryParameterScopeCoreRejectsRebinding(t *testing.T) {
	for name, change := range map[string]func(node){
		"slot":       func(q node) { q["whereClause"] = expr(col("amount"), param(2)) },
		"column":     func(q node) { q["whereClause"] = expr(col("id"), param(1)) },
		"source":     func(q node) { q["fromClause"] = []any{relation("other")} },
		"repetition": func(q node) { q["targetList"] = []any{target(param(1))} },
		"omission":   func(q node) { delete(q, "whereClause") },
		"alias":      func(q node) { object(array(q["fromClause"])[0])["alias"] = node{"aliasname": "changed"} },
		"boolean": func(q node) {
			q["whereClause"] = node{"BoolExpr": node{"boolop": "OR_EXPR", "args": []any{q["whereClause"], node{"A_Const": node{"boolval": node{"boolval": true}}}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			q := base()
			changed := clone(q)
			change(changed)
			checkPair(t, q, changed, 1, ErrBinding)
		})
	}
	q := with(base())
	changed := clone(q)
	withClause := object(changed["withClause"])
	cte := fieldObject(array(withClause["ctes"])[0], "CommonTableExpr")
	cte["ctematerialized"] = "CTEMaterializeAlways"
	checkPair(t, q, changed, 1, ErrBinding)
	changed = clone(q)
	withClause = object(changed["withClause"])
	cte = fieldObject(array(withClause["ctes"])[0], "CommonTableExpr")
	cte["ctename"] = "shadow"
	checkPair(t, q, changed, 1, ErrBinding)
	// All namespaces matter even when their own clauses do not contain a slot.
	q = base()
	q["withClause"] = node{"ctes": []any{node{"CommonTableExpr": node{"ctename": "u", "ctequery": node{"SelectStmt": node{"op": "SETOP_NONE", "targetList": []any{target(col("id"))}}}}}}}
	changed = clone(q)
	changed["withClause"] = nil
	checkPair(t, q, changed, 1, ErrBinding)
}

func TestSQLRecoveryParameterScopeCoreSetAndJoinNodes(t *testing.T) {
	q := base()
	q["fromClause"] = []any{node{"JoinExpr": node{"larg": relation("sales"), "rarg": relation("items"), "jointype": "JOIN_LEFT", "quals": expr(col("id"), col("id"))}}}
	edited := clone(q)
	edited["targetList"] = []any{target(col("amount"))}
	checkPair(t, q, edited, 1, nil)
	fieldObject(array(edited["fromClause"])[0], "JoinExpr")["jointype"] = "JOIN_INNER"
	checkPair(t, q, edited, 1, ErrBinding)
	q = node{"op": "SETOP_UNION", "larg": base(), "rarg": base(), "all": true}
	edited = clone(q)
	edited["all"] = false
	checkPair(t, q, edited, 1, ErrBinding)
	for _, bad := range []node{
		{"op": "SETOP_UNKNOWN", "whereClause": expr(col("id"), param(1))},
		{"op": "SETOP_NONE", "larg": base()},
		{"op": "SETOP_NONE", "all": true},
		{"op": "SETOP_UNION", "larg": base()},
		{"op": "SETOP_UNION", "larg": base(), "rarg": base(), "whereClause": param(1)},
		{"op": "SETOP_UNION", "larg": node{"RangeVar": node{"relname": "sales"}}, "rarg": base()},
	} {
		checkPair(t, bad, bad, 1, ErrUnsupported)
	}
	for _, op := range []string{"SETOP_INTERSECT", "SETOP_EXCEPT"} {
		q = node{"op": op, "larg": base(), "rarg": base()}
		edited = clone(q)
		edited["sortClause"] = []any{col("id")}
		checkPair(t, q, edited, 1, nil)
	}
}

func TestSQLRecoveryParameterScopeCoreCommandsStayUnsupported(t *testing.T) {
	recursive := with(base())
	object(recursive["withClause"])["recursive"] = true
	// This catches top-level raw WithClause as well as a nested wrapped form.
	for _, q := range []node{recursive, with(recursive)} {
		checkPair(t, q, q, 1, ErrUnsupported)
	}
	for _, field := range []string{"lockingClause", "intoClause", "valuesLists"} {
		q := base()
		q[field] = []any{node{"invalid": true}}
		checkPair(t, q, q, 1, ErrUnsupported)
	}
	for _, command := range []string{"InsertStmt", "UpdateStmt", "DeleteStmt", "MergeStmt", "CallStmt", "CopyStmt", "RangeFunction"} {
		q := base()
		q["targetList"] = []any{target(node{command: node{"arg": param(1)}})}
		checkPair(t, q, q, 1, ErrUnsupported)
	}
	q := with(base())
	object(q["withClause"])["unexpected"] = "field"
	checkPair(t, q, q, 1, ErrUnsupported)
	q = with(base())
	object(q["withClause"])["ctes"] = []any{node{"CommonTableExpr": node{"ctequery": node{"DeleteStmt": node{"whereClause": param(1)}}}}}
	checkPair(t, q, q, 1, ErrUnsupported)
}

func TestSQLRecoveryParameterScopeCoreOutputDependencies(t *testing.T) {
	for name, build := range map[string]func() node{
		"order":    func() node { q := base(); q["sortClause"] = []any{col("id"), param(1)}; return q },
		"group":    func() node { q := base(); q["groupClause"] = []any{col("id"), param(1)}; return q },
		"distinct": func() node { q := base(); q["distinctClause"] = []any{col("id"), param(1)}; return q },
		"window": func() node {
			q := base()
			q["windowClause"] = []any{node{"WindowDef": node{"name": "w", "startOffset": param(1)}}}
			return q
		},
	} {
		t.Run(name, func(t *testing.T) {
			q := build()
			changed := clone(q)
			changed["targetList"] = []any{target(col("name"))}
			checkPair(t, q, changed, 1, ErrBinding)
			if name == "window" || name == "distinct" {
				changed = clone(q)
				changed["sortClause"] = []any{col("name")}
				checkPair(t, q, changed, 1, ErrBinding)
			}
			if name == "group" {
				changed = clone(q)
				changed["groupDistinct"] = true
				checkPair(t, q, changed, 1, ErrBinding)
			}
		})
	}
	for _, limit := range []string{"limitCount", "limitOffset"} {
		q := base()
		q[limit] = param(1)
		changed := clone(q)
		changed["limitOption"] = "LIMIT_OPTION_WITH_TIES"
		checkPair(t, q, changed, 1, ErrBinding)
	}
}

func TestSQLRecoveryParameterScopeCoreExactLiteralsAndLocations(t *testing.T) {
	q := base()
	object(object(q["whereClause"])["A_Expr"])["literal"] = json.Number("9007199254740993")
	changed := clone(q)
	object(object(changed["whereClause"])["A_Expr"])["literal"] = json.Number("9007199254740992")
	checkPair(t, q, changed, 1, ErrBinding)
	changed = clone(q)
	object(object(changed["whereClause"])["A_Expr"])["location"] = json.Number("999")
	checkPair(t, q, changed, 1, nil)
	for _, pair := range [][2]string{{"A B", "A  B"}, {"Product", "product"}, {"$1", "$2"}, {"0.50", "0.5"}, {"秘密", "机密"}} {
		q = base()
		object(object(q["whereClause"])["A_Expr"])["literal"] = pair[0]
		changed = clone(q)
		object(object(changed["whereClause"])["A_Expr"])["literal"] = pair[1]
		checkPair(t, q, changed, 1, ErrBinding)
	}
	q = base()
	q["targetList"] = []any{target(node{"A_Const": node{"sval": node{"sval": "not a binding $64"}}})}
	checkPair(t, q, q, 1, nil)
	// Adjacent slots with swapped identities must not collapse into a multiset.
	q = base()
	q["whereClause"] = expr(param(1), param(2))
	changed = clone(q)
	changed["whereClause"] = expr(param(2), param(1))
	checkPair(t, q, changed, 2, ErrBinding)
}

func TestSQLRecoveryParameterScopeCoreMalformedAndBounded(t *testing.T) {
	good := nativeJSON(base())
	for _, count := range []int{-1, 0, 65, 2} {
		if err := Check(context.Background(), good, good, count); !errors.Is(err, ErrBinding) {
			t.Fatal("bad slots", count, err)
		}
	}
	for _, raw := range []string{"", strings.Repeat("x", (4<<20)+1)} {
		if err := Check(context.Background(), raw, good, 1); !errors.Is(err, ErrLimit) {
			t.Fatal("byte bound", err)
		}
	}
	for _, raw := range []string{`{`, good + good, string([]byte{0xff})} {
		if err := Check(context.Background(), raw, good, 1); !errors.Is(err, ErrBinding) {
			t.Fatal("malformed native output", err)
		}
	}
	for _, raw := range []string{`{"stmts":[]}`, `{"stmts":[{},{}]}`, `{"stmts":[{"stmt":{"DeleteStmt":{}}}]}`, `{"stmts":[{"stmt":{"SelectStmt":{}}}]}`} {
		if err := Check(context.Background(), raw, good, 1); err == nil {
			t.Fatal("not one bound select")
		}
	}
	for _, number := range []any{0, 2, "1", nil, 1.5} {
		q := base()
		q["whereClause"] = node{"ParamRef": node{"number": number}}
		checkPair(t, q, q, 1, ErrBinding)
	}
	if err := Check(nil, good, good, 1); !errors.Is(err, ErrBinding) {
		t.Fatal("nil context", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Check(canceled, good, good, 1); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation", err)
	}
	q := base()
	var deep any = param(1)
	for i := 0; i < 130; i++ {
		deep = node{"x": deep}
	}
	q["whereClause"] = deep
	checkPair(t, q, q, 1, ErrLimit)
	q = base()
	values := make([]any, 33000)
	for i := range values {
		values[i] = 1
	}
	q["targetList"] = values
	checkPair(t, q, q, 1, ErrLimit)
}

func TestSQLRecoveryParameterScopeCoreConcurrentDetached(t *testing.T) {
	q := base()
	raw := nativeJSON(q)
	want, err := signature(context.Background(), raw, 1)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := signature(context.Background(), raw, 1)
			if err != nil || !reflect.DeepEqual(want, got) {
				t.Error("concurrent signature differs")
			}
			delete(got, "fromClause")
		}()
	}
	wg.Wait()
	current, err := signature(context.Background(), raw, 1)
	if err != nil || !reflect.DeepEqual(want, current) || nativeJSON(q) != raw {
		t.Fatal("signature shared mutable state")
	}
	scan := parameterClauseScan{ctx: context.Background(), count: 1, seen: map[int]bool{}}
	direct := node{"SelectStmt": node{"whereClause": node{"ParamRef": node{"number": json.Number("1"), "location": json.Number("9")}}, "op": "SETOP_NONE"}}
	before, _ := json.Marshal(direct)
	normalized, bound, err := scan.visit(direct, 0)
	after, _ := json.Marshal(direct)
	if err != nil || !bound || string(before) != string(after) || reflect.DeepEqual(normalized, direct) {
		t.Fatal("walk did not detach/strip only location", err)
	}
	scan.nodes = 32768
	if _, _, err := scan.visit(direct, 0); !errors.Is(err, ErrLimit) {
		t.Fatal("node budget", err)
	}
	scan.nodes = 0
	if _, _, err := scan.visit(direct, 129); !errors.Is(err, ErrLimit) {
		t.Fatal("depth budget", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	scan.ctx = canceled
	if _, _, err := scan.visit(direct, 0); !errors.Is(err, context.Canceled) {
		t.Fatal("walk cancellation", err)
	}
}

func FuzzParameterScopeImmutableInputs(f *testing.F) {
	for _, seed := range []string{"North", "A  B", "9007199254740993", "秘密", "$64", "a\\b\n\"c"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, literal string) {
		if len(literal) > 4096 {
			return
		}
		q := with(base())
		cte := fieldObject(array(object(q["withClause"])["ctes"])[0], "CommonTableExpr")
		inner := fieldObject(cte["ctequery"], "SelectStmt")
		object(object(inner["whereClause"])["A_Expr"])["literal"] = literal
		edited := clone(q)
		edited["targetList"] = []any{target(col("name"))}
		checkPair(t, q, edited, 1, nil)
		edited = clone(q)
		object(edited["withClause"])["recursive"] = true
		checkPair(t, q, edited, 1, ErrUnsupported)
		edited = clone(q)
		newCTE := fieldObject(array(object(edited["withClause"])["ctes"])[0], "CommonTableExpr")
		newCTE["ctename"] = literal + "different"
		checkPair(t, q, edited, 1, ErrBinding)
	})
}

func FuzzParameterScopeDecoder(f *testing.F) {
	good := nativeJSON(base())
	f.Add(good, good, uint8(1))
	f.Add(`{"stmts":[]}`, `{`, uint8(1))
	f.Add(good, good+good, uint8(65))
	f.Fuzz(func(t *testing.T, left, right string, count uint8) {
		if len(left) > 8192 || len(right) > 8192 {
			return
		}
		err := Check(context.Background(), left, right, int(count))
		if err != nil && !errors.Is(err, ErrBinding) && !errors.Is(err, ErrUnsupported) && !errors.Is(err, ErrLimit) {
			t.Fatal("non-closed error type")
		}
		if err == nil {
			if reverse := Check(context.Background(), right, left, int(count)); reverse != nil {
				t.Fatal("non-symmetric equivalence")
			}
			if same := Check(context.Background(), left, left, int(count)); same != nil {
				t.Fatal("accepted candidate cannot match itself")
			}
		}
	})
}

func TestSQLRecoveryParameterScopeCoreIndirectValueConsumers(t *testing.T) {
	inner := base()
	delete(inner, "whereClause")
	inner["targetList"] = []any{target(param(1))}
	q := with(inner)
	q["whereClause"] = expr(col("amount"), col("result"))
	changed := clone(q)
	changed["whereClause"] = expr(col("id"), col("result"))
	checkPair(t, q, changed, 1, ErrBinding)
	changed = clone(q)
	changed["sortClause"] = []any{col("id")}
	checkPair(t, q, changed, 1, ErrBinding)
	changed = clone(q)
	changed["location"] = json.Number("200")
	checkPair(t, q, changed, 1, nil)
	// A nested named frame can also export a parameter-dependent value through
	// a column; no ParamRef is present where the outer filter consumes it.
	inner = base()
	delete(inner, "whereClause")
	inner["windowClause"] = []any{node{"WindowDef": node{"name": "w", "startOffset": param(1)}}}
	q = with(inner)
	q["whereClause"] = expr(col("amount"), col("result"))
	changed = clone(q)
	changed["whereClause"] = expr(col("id"), col("result"))
	checkPair(t, q, changed, 1, ErrBinding)
	// Parameters limited to an inner predicate do not export a scalar binding.
	q = with(base())
	changed = clone(q)
	changed["targetList"] = []any{target(col("amount"))}
	checkPair(t, q, changed, 1, nil)
}
