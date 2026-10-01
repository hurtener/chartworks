package exec

import (
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/exec/signatureparser"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
)

// Warehouse evidence is consumed directly from the native tree, not the analytical
// normalizer: normalization may intentionally erase original cast details.
func (p *learningDomainProof) warehouseSQL(sql string) error {
	native, ok := sqlpolicy.NativeDialect(p.binding.Dialect)
	if !ok {
		return ErrUnsupported
	}
	root, err := signatureparser.Inspect(p.ctx, sql, native, 10000, 64)
	if err != nil {
		if p.ctx.Err() != nil {
			return p.ctx.Err()
		}
		return ErrUnsupported
	}

	if err := p.warehouseSelect(root, 0); err != nil {
		return err
	}
	if !p.namedMarkers() && p.warehousePosition != len(p.domains) {
		return ErrUnsupported
	}
	return nil
}
func (p *learningDomainProof) warehouseSelect(root map[string]any, depth int) error {
	if err := p.bound(depth); err != nil {
		return err
	}
	if len(root) != 1 {
		return ErrUnsupported
	}
	var kind string
	var s map[string]any
	for key, value := range root {
		kind = key
		s = object(value)
	}
	old := p.warehouseCTEs
	p.warehouseCTEs = map[string]bool{}
	for name, yes := range old {
		p.warehouseCTEs[name] = yes
	}
	defer func() { p.warehouseCTEs = old }()
	if with := object(s["with"]); with != nil {
		if !warehouseAnalyticalFields(with, "ctes", "recursive") || truth(with["recursive"]) {
			return ErrUnsupported
		}
		for _, raw := range array(with["ctes"]) {
			cte := object(raw)
			if !warehouseAnalyticalFields(cte, "alias", "this", "columns", "materialized", "alias_first") {
				return ErrUnsupported
			}
			if err := p.warehouseSelect(object(cte["this"]), depth+1); err != nil {
				return err
			}
			p.warehouseCTEs[text(object(cte["alias"])["name"])] = true
		}
	}
	if kind == "union" || kind == "intersect" || kind == "except" {
		if !warehouseAnalyticalFields(s, "left", "right", "distinct", "with", "order_by", "limit", "offset") {
			return ErrUnsupported
		}
		if err := p.warehouseSelect(object(s["left"]), depth+1); err != nil {
			return err
		}
		if err := p.warehouseSelect(object(s["right"]), depth+1); err != nil {
			return err
		}
		if !p.namedMarkers() && s["limit"] != nil && s["offset"] != nil {
			return ErrUnsupported
		}
		for _, key := range []string{"order_by", "limit", "offset"} {
			if err := p.warehouseWalk(s[key], nil, depth+1, ""); err != nil {
				return err
			}
		}
		return nil
	}
	if kind != "select" || !warehouseAnalyticalFields(s, "with", "expressions", "from", "joins", "where_clause", "group_by", "having", "order_by", "limit", "offset", "distinct", "top", "fetch") {
		return ErrUnsupported
	}
	if !p.namedMarkers() && s["limit"] != nil && s["offset"] != nil {
		return ErrUnsupported
	}
	scope := learningDomainScope{}
	for _, raw := range array(object(s["from"])["expressions"]) {
		if err := p.warehouseSource(raw, scope); err != nil {
			return err
		}
	}
	for _, raw := range array(s["joins"]) {
		if err := p.warehouseSource(object(raw)["this"], scope); err != nil {
			return err
		}
	}
	// This follows original SQL order, including WITH and SELECT TOP. Named slots
	// are matched by exact ordinal instead; map iteration never assigns markers.
	for _, key := range []string{"top", "expressions", "from", "joins", "where_clause", "group_by", "having", "order_by", "limit", "offset", "fetch"} {
		if err := p.warehouseWalk(s[key], scope, depth+1, ""); err != nil {
			return err
		}
	}
	return nil
}

func (p *learningDomainProof) warehouseSource(v any, scope learningDomainScope) error {
	t := object(object(v)["table"])
	if t == nil {
		if sq := object(object(v)["subquery"]); sq != nil {
			name := text(object(sq["alias"])["name"])
			if name == "" {
				return ErrUnsupported
			}
			scope[name] = nil
			return nil
		}
		return ErrUnsupported
	}
	name := text(object(t["name"])["name"])
	schema := text(object(t["schema"])["name"])
	alias := text(object(t["alias"])["name"])
	if alias == "" {
		alias = name
	}
	coordinate := name
	if schema != "" {
		coordinate = schema + "." + coordinate
	}
	if catalog := text(object(t["catalog"])["name"]); catalog != "" {
		coordinate = catalog + "." + coordinate
	}
	var r *Relation
	for i := range p.binding.Relations {
		if warehouseRelationMatches(p.binding, p.binding.Relations[i], coordinate, false) {
			if r != nil {
				return ErrUnsupported
			}
			r = &p.binding.Relations[i]
		}
	}
	if p.warehouseCTEs[name] && schema == "" && t["catalog"] == nil || len(array(t["column_aliases"])) > 0 {
		r = nil
	}
	if _, exists := scope[alias]; exists {
		scope[alias] = nil
	} else {
		scope[alias] = r
	}
	return nil
}
func (p *learningDomainProof) warehouseLearningCastDomain(v any) string {
	c := object(object(v)["cast"])
	if c == nil || !warehouseAnalyticalFields(c, "this", "to", "double_colon_syntax") {
		return ""
	}
	typ := object(c["to"])
	name := text(typ["data_type"])
	if name == "custom" {
		if !warehouseAnalyticalFields(typ, "data_type", "name") {
			return ""
		}
		name = strings.ToLower(text(typ["name"]))
	} else if !warehouseAnalyticalFields(typ, "data_type", "precision", "timezone") {
		return ""
	}
	if value, ok := typ["precision"].(float64); ok {
		max := 6
		if p.binding.Dialect == "sqlserver" {
			max = 7
		}
		if p.binding.Dialect == "snowflake" {
			max = 9
		}
		if value < 0 || value > float64(max) || value != float64(int(value)) {
			return ""
		}
		if name == "date" {
			return ""
		}
	}
	if truth(typ["timezone"]) {
		return ""
	}
	return learningNativeDomain(p.binding.Dialect, name)
}
func (p *learningDomainProof) warehouseExprDomain(v any, scope learningDomainScope) string {
	if domain := p.warehouseLearningCastDomain(v); domain != "" {
		return domain
	}
	c := object(object(v)["column"])
	if c == nil {
		return ""
	}
	name := text(object(c["name"])["name"])
	table := text(object(c["table"])["name"])
	parts := []string{name}
	if table != "" {
		parts = []string{table, name}
	}
	return p.columnDomain(parts, scope)
}
func (p *learningDomainProof) warehouseLearningHasParameter(v any) bool {
	switch x := v.(type) {
	case []any:
		for _, a := range x {
			if p.warehouseLearningHasParameter(a) {
				return true
			}
		}
	case map[string]any:
		if _, ok := p.warehouseNamedParameter(x); ok {
			return true
		}
		if _, ok := x["parameter"]; ok {
			return true
		}
		if _, ok := x["placeholder"]; ok {
			return true
		}
		for _, a := range x {
			if p.warehouseLearningHasParameter(a) {
				return true
			}
		}
	}
	return false
}
func (p *learningDomainProof) warehouseWalk(v any, scope learningDomainScope, depth int, domain string) error {
	if err := p.bound(depth); err != nil {
		return err
	}
	if !p.warehouseLearningHasParameter(v) {
		return nil
	}
	switch node := v.(type) {
	case []any:
		for _, x := range node {
			if err := p.warehouseWalk(x, scope, depth+1, ""); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		if index, ok := p.warehouseNamedParameter(node); ok {
			return p.note(index, domain)
		}
		if ph, ok := node["placeholder"]; ok && len(node) == 1 {
			if p.namedMarkers() || object(ph)["index"] != nil {
				return ErrUnsupported
			}
			p.warehousePosition++
			return p.note(p.warehousePosition, domain)
		}
		if len(node) == 1 {
			for key, value := range node {
				b := object(value)
				switch key {
				case "subquery":
					if !warehouseAnalyticalFields(b, "this", "alias") {
						return ErrUnsupported
					}
					return p.warehouseSelect(object(b["this"]), depth+1)
				case "select", "union", "intersect", "except":
					return p.warehouseSelect(node, depth+1)
				case "cast":
					return p.warehouseWalk(b["this"], scope, depth+1, p.warehouseLearningCastDomain(node))
				case "eq", "neq", "gt", "gte", "lt", "lte":
					if !warehouseAnalyticalFields(b, "left", "right") {
						return ErrUnsupported
					}
					if err := p.warehouseWalk(b["left"], scope, depth+1, p.warehouseExprDomain(b["right"], scope)); err != nil {
						return err
					}
					return p.warehouseWalk(b["right"], scope, depth+1, p.warehouseExprDomain(b["left"], scope))
				case "and", "or", "add", "sub", "mul", "div":
					if !warehouseAnalyticalFields(b, "left", "right") {
						return ErrUnsupported
					}
					if err := p.warehouseWalk(b["left"], scope, depth+1, ""); err != nil {
						return err
					}
					return p.warehouseWalk(b["right"], scope, depth+1, "")
				case "alias":
					if !warehouseAnalyticalFields(b, "this", "alias", "column_aliases", "pre_alias_comments") {
						return ErrUnsupported
					}
					return p.warehouseWalk(b["this"], scope, depth+1, "")
				case "is_null":
					if !warehouseAnalyticalFields(b, "this", "not") {
						return ErrUnsupported
					}
					return p.warehouseWalk(b["this"], scope, depth+1, "")
				case "paren":
					return p.warehouseWalk(b["this"], scope, depth+1, domain)
				}
			}
		}
		// Statement wrappers have exact lexical child order. Unsupported expressions
		// with positional parameters never borrow map iteration order as slot custody.
		if warehouseAnalyticalFields(node, "this", "parenthesized") && node["this"] != nil {
			return p.warehouseWalk(node["this"], scope, depth+1, "")
		}
		if warehouseAnalyticalFields(node, "expressions") && node["expressions"] != nil {
			return p.warehouseWalk(node["expressions"], scope, depth+1, "")
		}
		if warehouseAnalyticalFields(node, "this", "on", "kind", "use_inner_keyword", "use_outer_keyword") && node["on"] != nil {
			if err := p.warehouseWalk(node["this"], scope, depth+1, ""); err != nil {
				return err
			}
			return p.warehouseWalk(node["on"], scope, depth+1, "")
		}
	}
	return ErrUnsupported
}

func (p *learningDomainProof) namedMarkers() bool {
	return p.binding.Dialect == "sqlserver" || p.binding.Dialect == "bigquery" || p.binding.Dialect == "databricks"
}
func (p *learningDomainProof) warehouseNamedParameter(v any) (int, bool) {
	if !p.namedMarkers() {
		return 0, false
	}
	m := object(v)
	if len(m) != 1 {
		return 0, false
	}
	name := ""
	prefix, style := "@p", "At"
	if p.binding.Dialect == "databricks" {
		prefix, style = ":p", "Colon"
	}
	if c := object(m["column"]); c != nil {
		n := object(c["name"])
		if c["table"] != nil || truth(n["quoted"]) || truth(c["join_mark"]) {
			return 0, false
		}
		name = text(n["name"])
	} else if c := object(m["parameter"]); c != nil {
		if !warehouseAnalyticalFields(c, "name", "index", "style") || text(c["style"]) != style || c["index"] != nil {
			return 0, false
		}
		name = prefix[:1] + text(c["name"])
	}
	if !strings.HasPrefix(name, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(name, prefix))
	if err != nil || n < 1 || n > 64 || name != prefix+strconv.Itoa(n) {
		return 0, false
	}
	return n, true
}
