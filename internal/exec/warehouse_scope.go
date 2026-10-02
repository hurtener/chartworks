package exec

import (
	"context"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
)

// This resolver binds structural column occurrences, not SQL text or read
// safety. The primary native inspection and signature pass must already have
// accepted the same tree. Every logical output is backed by a fully checked
// child query; it never excuses an unchecked physical occurrence elsewhere.
type warehouseScopeResolver struct {
	ctx               context.Context
	full, allowed     Binding
	parameters, nodes int
	dependencies      map[string]bool
}
type warehouseReadScope struct {
	sources map[string]warehouseScopeSource
	ctes    map[string][]string
	outputs map[string]int
	parent  *warehouseReadScope
}
type warehouseScopeSource struct {
	columns     map[string]int
	safe        map[string]bool
	nativeTypes map[string]string
}
type warehouseQueryScope struct {
	names []string
	scope *warehouseReadScope
}

func warehouseResolveScope(ctx context.Context, root map[string]any, full, allowed Binding, parameters int) ([]string, []string, error) {
	r := warehouseScopeResolver{ctx: ctx, full: full, allowed: allowed, parameters: parameters, dependencies: map[string]bool{}}
	out, err := r.query(root, nil, nil, 0)
	if err != nil {
		return nil, nil, err
	}
	deps := make([]string, 0, len(r.dependencies))
	for id := range r.dependencies {
		deps = append(deps, id)
	}
	sort.Strings(deps)
	return deps, out.names, nil
}
func (r *warehouseScopeResolver) tick(depth int) error {
	r.nodes++
	if r.nodes > 100000 || depth > 256 {
		return ErrUnsupported
	}
	return r.ctx.Err()
}
func warehouseScopeFields(m map[string]any, allowed ...string) bool {
	if m == nil {
		return false
	}
	for key, value := range m {
		if strings.HasSuffix(key, "comments") || key == "span" || key == "inferred_type" {
			continue
		}
		found := false
		for _, a := range allowed {
			if key == a {
				found = true
				break
			}
		}
		if !found && !warehouseScopeEmpty(value) {
			return false
		}
	}
	return true
}
func (r *warehouseScopeResolver) identifier(v any) (string, error) {
	name := text(object(v)["name"])
	if !SQLIdentifierForDialect(r.full.Dialect, name) {
		return "", ErrUnsupported
	}
	return name, nil
}
func warehouseCopyCTEs(in map[string][]string) map[string][]string {
	out := map[string][]string{}
	for key, value := range in {
		out[key] = append([]string(nil), value...)
	}
	return out
}
func (r *warehouseScopeResolver) renamed(names []string, aliases any) ([]string, error) {
	a := array(aliases)
	if len(a) > len(names) {
		return nil, ErrUnsafe
	}
	out := append([]string(nil), names...)
	for i, node := range a {
		name, err := r.identifier(node)
		if err != nil {
			return nil, err
		}
		out[i] = name
	}
	return out, nil
}
func (r *warehouseScopeResolver) query(root map[string]any, inherited map[string][]string, parent *warehouseReadScope, depth int) (warehouseQueryScope, error) {
	fail := warehouseQueryScope{}
	if err := r.tick(depth); err != nil {
		return fail, err
	}
	if len(root) != 1 {
		return fail, ErrUnsupported
	}
	s := &warehouseReadScope{sources: map[string]warehouseScopeSource{}, ctes: warehouseCopyCTEs(inherited), outputs: map[string]int{}, parent: parent}
	var kind string
	var body map[string]any
	for k, v := range root {
		kind = k
		body = object(v)
	}
	if body == nil {
		return fail, ErrUnsupported
	}
	if w := object(body["with"]); w != nil {
		if !warehouseScopeFields(w, "ctes", "recursive") || truth(w["recursive"]) {
			return fail, ErrUnsupported
		}
		local := map[string]bool{}
		for _, node := range array(w["ctes"]) {
			c := object(node)
			if !warehouseScopeFields(c, "alias", "this", "columns", "materialized", "alias_first") {
				return fail, ErrUnsupported
			}
			name, err := r.identifier(c["alias"])
			if err != nil {
				return fail, err
			}
			if local[name] {
				return fail, ErrUnsafe
			}
			q, err := r.query(object(c["this"]), s.ctes, nil, depth+1)
			if err != nil {
				return fail, err
			}
			names, err := r.renamed(q.names, c["columns"])
			if err != nil {
				return fail, err
			}
			s.ctes[name] = names
			local[name] = true
		}
	}
	if kind == "union" || kind == "intersect" || kind == "except" {
		left, err := r.query(object(body["left"]), s.ctes, parent, depth+1)
		if err != nil {
			return fail, err
		}
		right, err := r.query(object(body["right"]), s.ctes, parent, depth+1)
		if err != nil {
			return fail, err
		}
		if len(left.names) != len(right.names) {
			return fail, ErrUnsafe
		}
		for _, name := range left.names {
			s.outputs[name]++
		}
		if err := r.order(body["order_by"], s, depth+1); err != nil {
			return fail, err
		}
		for key, value := range body {
			switch key {
			case "left", "right", "with", "order_by":
				continue
			}
			if err := r.expr(value, s, false, depth+1); err != nil {
				return fail, err
			}
		}
		return warehouseQueryScope{left.names, s}, nil
	}
	if kind != "select" {
		return fail, ErrUnsupported
	}
	if from := object(body["from"]); from != nil {
		if !warehouseScopeFields(from, "expressions") {
			return fail, ErrUnsupported
		}
		for _, node := range array(from["expressions"]) {
			if err := r.from(node, s, depth+1); err != nil {
				return fail, err
			}
		}
	}
	for _, join := range array(body["joins"]) {
		if err := r.join(object(join), s, depth+1); err != nil {
			return fail, err
		}
	}
	names := []string{}
	for _, node := range array(body["expressions"]) {
		expr := node
		name := ""
		if alias := object(object(node)["alias"]); alias != nil {
			if !warehouseScopeFields(alias, "this", "alias") {
				return fail, ErrUnsupported
			}
			var err error
			name, err = r.identifier(alias["alias"])
			if err != nil {
				return fail, err
			}
			expr = alias["this"]
		} else if column := object(object(node)["column"]); column != nil {
			name = text(object(column["name"])["name"])
		}
		if err := r.expr(expr, s, false, depth+1); err != nil {
			return fail, err
		}
		names = append(names, name)
		if name != "" {
			s.outputs[name]++
		}
	}
	if len(names) == 0 {
		return fail, ErrUnsafe
	}
	if err := r.order(body["order_by"], s, depth+1); err != nil {
		return fail, err
	}
	for key, value := range body {
		switch key {
		case "with", "from", "joins", "expressions", "order_by":
			continue
		}
		if err := r.expr(value, s, false, depth+1); err != nil {
			return fail, err
		}
	}
	return warehouseQueryScope{names, s}, nil
}
func (r *warehouseScopeResolver) add(s *warehouseReadScope, label string, names []string, safe map[string]bool) error {
	if !SQLIdentifierForDialect(r.full.Dialect, label) {
		return ErrUnsupported
	}
	for prior := range s.sources {
		if strings.EqualFold(prior, label) {
			return ErrUnsafe
		}
	}
	source := warehouseScopeSource{columns: map[string]int{}, safe: map[string]bool{}}
	for _, name := range names {
		if name != "" {
			source.columns[name]++
			source.safe[name] = safe == nil || safe[name]
		}
	}
	s.sources[label] = source
	return nil
}
func (r *warehouseScopeResolver) from(v any, s *warehouseReadScope, depth int) error {
	if err := r.tick(depth); err != nil {
		return err
	}
	m := object(v)
	if len(m) != 1 {
		return ErrUnsupported
	}
	if table := object(m["table"]); table != nil {
		if !warehouseScopeFields(table, "name", "schema", "catalog", "alias", "alias_explicit_as", "column_aliases") {
			return ErrUnsupported
		}
		name := text(object(table["name"])["name"])
		schema := text(object(table["schema"])["name"])
		catalog := text(object(table["catalog"])["name"])
		label := name
		if table["alias"] != nil {
			var err error
			label, err = r.identifier(table["alias"])
			if err != nil {
				return err
			}
		}
		if schema == "" && catalog == "" {
			if names, ok := s.ctes[name]; ok {
				renamed, err := r.renamed(names, table["column_aliases"])
				if err != nil {
					return err
				}
				return r.add(s, label, renamed, nil)
			}
			// Only BigQuery gives a quoted whole path qualification semantics. Other
			// engines treat a quoted dotted name as one identifier, not schema.table.
			if r.full.Dialect != "bigquery" || !truth(object(table["name"])["quoted"]) || !strings.Contains(name, ".") {
				return ErrUnsafe
			}
		}
		if len(array(table["column_aliases"])) > 0 {
			return ErrUnsupported
		}
		qualified := name
		if schema != "" {
			qualified = schema + "." + qualified
		}
		if catalog != "" {
			if schema == "" {
				return ErrUnsafe
			}
			qualified = catalog + "." + qualified
		}
		var physical Relation
		matches := 0
		for _, relation := range r.full.Relations {
			if warehouseRelationMatches(r.full, relation, qualified, false) {
				physical = relation
				matches++
			}
		}
		if matches != 1 {
			return ErrUnsafe
		}
		allowed := map[string]bool{}
		permitted := false
		for _, relation := range r.allowed.Relations {
			if relation.ID == physical.ID && relation.Schema == physical.Schema && relation.Name == physical.Name {
				permitted = true
				for _, column := range relation.Columns {
					allowed[column.Name] = column.Safe
				}
			}
		}
		if !permitted {
			return ErrUnsafe
		}
		columns := make([]string, 0, len(physical.Columns))
		for _, column := range physical.Columns {
			columns = append(columns, column.Name)
			allowed[column.Name] = allowed[column.Name] && column.Safe
		}
		if table["alias"] == nil {
			label = physical.Name
		}
		r.dependencies[physical.ID] = true
		if err := r.add(s, label, columns, allowed); err != nil {
			return err
		}
		source := s.sources[label]
		source.nativeTypes = map[string]string{}
		for _, column := range physical.Columns {
			source.nativeTypes[column.Name] = column.NativeType
		}
		s.sources[label] = source
		return nil
	}
	if sub := object(m["subquery"]); sub != nil {
		if !warehouseScopeFields(sub, "this", "alias", "column_aliases", "order_by", "limit", "offset", "modifiers_inside") || sub["alias"] == nil {
			return ErrUnsupported
		}
		label, err := r.identifier(sub["alias"])
		if err != nil {
			return err
		}
		q, err := r.query(object(sub["this"]), s.ctes, nil, depth+1)
		if err != nil {
			return err
		}
		if err = r.order(sub["order_by"], q.scope, depth+1); err != nil {
			return err
		}
		for _, key := range []string{"limit", "offset"} {
			if err = r.expr(sub[key], q.scope, false, depth+1); err != nil {
				return err
			}
		}
		names, err := r.renamed(q.names, sub["column_aliases"])
		if err != nil {
			return err
		}
		return r.add(s, label, names, nil)
	}
	if group := object(m["joined_table"]); group != nil {
		if !warehouseScopeFields(group, "left", "joins") {
			return ErrUnsupported
		}
		if err := r.from(group["left"], s, depth+1); err != nil {
			return err
		}
		for _, join := range array(group["joins"]) {
			if err := r.join(object(join), s, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrUnsupported
}
func (r *warehouseScopeResolver) join(j map[string]any, s *warehouseReadScope, depth int) error {
	if !warehouseScopeFields(j, "this", "on", "kind", "use_inner_keyword", "use_outer_keyword") {
		return ErrUnsupported
	}
	switch text(j["kind"]) {
	case "Inner", "Left", "Right", "Full", "Cross":
	default:
		return ErrUnsupported
	}
	if err := r.from(j["this"], s, depth+1); err != nil {
		return err
	}
	return r.expr(j["on"], s, false, depth+1)
}
func (r *warehouseScopeResolver) column(c map[string]any, s *warehouseReadScope) error {
	name := text(object(c["name"])["name"])
	table := text(object(c["table"])["name"])
	if (r.full.Dialect == "bigquery" || r.full.Dialect == "sqlserver") && table == "" && !truth(object(c["name"])["quoted"]) && strings.HasPrefix(name, "@") {
		n, err := businessParameterIndex(name, r.full.Dialect, 0)
		if err != nil || n > r.parameters {
			return ErrBinding
		}
		return nil
	}
	if !SQLIdentifierForDialect(r.full.Dialect, name) {
		return ErrUnsupported
	}
	for current := s; current != nil; current = current.parent {
		count := 0
		safe := true
		foundTable := false
		for label, source := range current.sources {
			if table != "" && label != table {
				if strings.EqualFold(label, table) {
					return ErrUnsafe
				}
				continue
			}
			if table != "" {
				foundTable = true
			}
			for column, n := range source.columns {
				if column == name {
					count += n
					safe = safe && source.safe[column]
				} else if strings.EqualFold(column, name) {
					return ErrUnsafe
				}
			}
		}
		if count > 0 {
			if count != 1 || !safe {
				return ErrUnsafe
			}
			return nil
		}
		if foundTable {
			return ErrUnsafe
		}
		// An omitted/hidden local field must never borrow an allowed outer field.
		// Bare correlation is provable only when there is no local input relation.
		if table == "" && len(current.sources) > 0 {
			return ErrUnsafe
		}
	}
	return ErrUnsafe
}
func (r *warehouseScopeResolver) order(v any, s *warehouseReadScope, depth int) error {
	if v == nil {
		return nil
	}
	o := object(v)
	if !warehouseScopeFields(o, "expressions") {
		return ErrUnsupported
	}
	for _, value := range array(o["expressions"]) {
		item := object(value)
		expr := item["this"]
		aliasResolved := false
		if column := object(object(expr)["column"]); column != nil && column["table"] == nil {
			name := text(object(column["name"])["name"])
			if n := s.outputs[name]; n > 0 {
				if n != 1 {
					return ErrUnsafe
				}
				aliasResolved = true
			}
			for alias := range s.outputs {
				if alias != name && strings.EqualFold(alias, name) {
					return ErrUnsafe
				}
			}
		}
		if !aliasResolved {
			if err := r.expr(expr, s, false, depth+1); err != nil {
				return err
			}
		}
		for key, child := range item {
			if key != "this" {
				if err := r.expr(child, s, false, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func (r *warehouseScopeResolver) expr(v any, s *warehouseReadScope, star bool, depth int) error {
	if err := r.tick(depth); err != nil {
		return err
	}
	if list, ok := v.([]any); ok {
		for _, item := range list {
			if err := r.expr(item, s, false, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	if len(m) == 1 {
		if cast := object(m["cast"]); cast != nil && object(object(cast["this"])["at_time_zone"]) != nil {
			return r.mysqlUTCInstant(cast, s)
		}
		if m["at_time_zone"] != nil {
			return ErrUnsupported
		}
		if column := object(m["column"]); column != nil {
			return r.column(column, s)
		}
		if _, ok := m["star"]; ok {
			if star {
				return nil
			}
			return ErrUnsupported
		}
		for _, kind := range []string{"select", "union", "intersect", "except"} {
			if m[kind] != nil {
				_, err := r.query(m, s.ctes, s, depth+1)
				return err
			}
		}
		if sub := object(m["subquery"]); sub != nil {
			if !warehouseScopeFields(sub, "this", "order_by", "limit", "offset", "modifiers_inside") {
				return ErrUnsupported
			}
			q, err := r.query(object(sub["this"]), s.ctes, s, depth+1)
			if err != nil {
				return err
			}
			if err = r.order(sub["order_by"], q.scope, depth+1); err != nil {
				return err
			}
			for _, key := range []string{"limit", "offset"} {
				if err = r.expr(sub[key], q.scope, false, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if m["table"] != nil {
			return ErrUnsupported
		}
		if name, args, body, call := warehouseCall(m); call {
			for i, arg := range args {
				if _, unit := sqlpolicy.CalendarArgument(r.full.Dialect, name, i); unit {
					if _, ok := warehouseCalendarUnit(r.full.Dialect, name, i, arg); !ok {
						return ErrUnsupported
					}
					continue
				}
				if err := r.expr(arg, s, name == "count", depth+1); err != nil {
					return err
				}
			}
			for key, child := range body {
				switch key {
				case "args", "this", "expression", "expressions", "decimals", "field":
					continue
				}
				if err := r.expr(child, s, false, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
	}
	for _, child := range m {
		if err := r.expr(child, s, false, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func warehouseScopeEmpty(v any) bool {
	if v == nil {
		return true
	}
	switch x := v.(type) {
	case bool:
		return !x
	case string:
		return x == ""
	case []any:
		return len(x) == 0
	case float64:
		return x == 0
	}
	return false
}
