package exec

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hurtener/chartworks/internal/identity"
	pgquery "github.com/wasilibs/go-pgquery"
)

// Admission is shared across independent learning consumers, just like the
// warehouse structural parser. A valid plan cannot request unbounded WASM work.
var learningDomainNativeSlots = make(chan struct{}, 2)

// LearningParameterDomains derives value-free input domains only from an opaque,
// current native-validated plan. Neither retained JSON nor supplied domain labels
// can construct this proof. Parameter values are deliberately never consulted.
// This is a learning-eligibility proof, not permission to execute different SQL.
func (p Plan) LearningParameterDomains(ctx context.Context, e identity.Envelope, current Binding) ([]string, error) {
	if ctx == nil {
		return nil, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sql, parameters, err := p.SQL(e, current)
	if err != nil {
		return nil, err
	}
	if len(parameters) == 0 {
		return nil, nil
	}
	if len(sql) > 32768 || len(parameters) > 64 {
		return nil, ErrLimit
	}
	proof := learningDomainProof{ctx: ctx, binding: current, domains: make([]string, len(parameters)), seen: make([]bool, len(parameters)), kinds: make([]string, len(parameters))}
	for i, parameter := range parameters {
		proof.kinds[i] = parameter.Kind
	}
	switch current.Dialect {
	case "postgres":
		select {
		case learningDomainNativeSlots <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		raw, err := pgquery.ParseToJSON(sql)
		<-learningDomainNativeSlots
		if err != nil {
			return nil, ErrUnsupported
		}
		var root map[string]any
		if json.Unmarshal([]byte(raw), &root) != nil {
			return nil, ErrUnsupported
		}
		statements := array(root["stmts"])
		if len(statements) != 1 {
			return nil, ErrUnsupported
		}
		err = proof.pgWalk(object(statements[0])["stmt"], nil, 0)
		if err != nil {
			return nil, err
		}
	case "mysql", "sqlserver", "bigquery", "snowflake", "databricks":
		if err := proof.warehouseSQL(sql); err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnsupported
	}
	for i, domain := range proof.domains {
		if !proof.seen[i] || domain != "" && parameters[i].Kind != "text" {
			return nil, ErrUnsupported
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]string(nil), proof.domains...), nil
}

type learningDomainProof struct {
	ctx               context.Context
	binding           Binding
	domains           []string
	seen              []bool
	kinds             []string
	nodes             int
	warehousePosition int
	warehouseCTEs     map[string]bool
}
type learningDomainScope map[string]*Relation

func (p *learningDomainProof) note(position int, domain string) error {
	if position < 1 || position > len(p.domains) {
		return ErrBinding
	}
	i := position - 1
	if p.kinds[i] != "text" {
		domain = ""
	}
	if p.seen[i] && p.domains[i] != domain {
		return ErrUnsupported
	}
	p.seen[i] = true
	p.domains[i] = domain
	return nil
}
func (p *learningDomainProof) bound(depth int) error {
	if err := p.ctx.Err(); err != nil {
		return err
	}
	p.nodes++
	if depth > 64 || p.nodes > 10000 {
		return ErrLimit
	}
	return nil
}
func learningNativeDomain(dialect, typ string) string {
	typ = strings.ToLower(strings.TrimSpace(typ))
	switch dialect {
	case "postgres":
		switch typ {
		case "date":
			return "date"
		case "timestamp", "timestamp without time zone":
			return "timestamp"
		case "timestamptz", "timestamp with time zone":
			return "timestamptz"
		case "uuid", "interval", "json", "jsonb":
			return typ
		case "time", "time without time zone":
			return "time"
		case "timetz", "time with time zone":
			return "timetz"
		}

	case "mysql", "sqlserver", "bigquery", "snowflake", "databricks":
		base, precision, modified := strings.Cut(typ, "(")
		if modified {
			max := byte('6')
			if dialect == "sqlserver" {
				max = '7'
			}
			if dialect == "snowflake" {
				max = '9'
			}
			if len(precision) != 2 || precision[1] != ')' || precision[0] < '0' || precision[0] > max {
				return ""
			}
		}
		if base == "date" && !modified {
			return "date"
		}
		switch dialect {
		case "mysql":
			if base == "time" {
				return "time"
			}
			if base == "json" && !modified {
				return "json"
			}
			if base == "datetime" || base == "timestamp" {
				return "timestamp"
			}
		case "sqlserver":
			if base == "time" {
				return "time"
			}
			if base == "datetime2" || base == "datetime" && !modified || base == "smalldatetime" && !modified {
				return "timestamp"
			}
			if base == "datetimeoffset" {
				return "timestamptz"
			}
			if base == "uniqueidentifier" && !modified {
				return "uuid"
			}
			// SQL Server TIMESTAMP/ROWVERSION is binary, never a temporal domain.
		case "bigquery":
			if !modified {
				if base == "time" {
					return "time"
				}
				if base == "datetime" {
					return "timestamp"
				}
				if base == "timestamp" {
					return "timestamptz"
				}
			}
		case "snowflake":
			if base == "time" {
				return "time"
			}
			if base == "datetime" || base == "timestamp_ntz" {
				return "timestamp"
			}
			if base == "timestamp_tz" || base == "timestamp_ltz" {
				return "timestamptz"
			}
			// Unqualified TIMESTAMP is a session-selected alias, not type evidence.
		case "databricks":
			if !modified {
				if base == "timestamp_ntz" {
					return "timestamp"
				}
				if base == "timestamp" || base == "timestamp_ltz" {
					return "timestamptz"
				}
			}
		}
	}
	return ""
}
func (p *learningDomainProof) relation(schema, name string) *Relation {
	var found *Relation
	for i := range p.binding.Relations {
		r := &p.binding.Relations[i]
		if r.Name == name && (schema == "" || r.Schema == schema) {
			if found != nil {
				return nil
			}
			found = r
		}
	}
	return found
}
func (p *learningDomainProof) columnDomain(parts []string, scope learningDomainScope) string {
	if len(parts) < 1 || len(parts) > 2 {
		return ""
	}
	name := parts[len(parts)-1]
	matches := 0
	domain := ""
	for alias, r := range scope {
		if len(parts) == 2 && parts[0] != alias {
			continue
		}
		if r == nil {
			if len(parts) == 1 {
				return ""
			}
			continue
		}
		for _, c := range r.Columns {
			if c.Name == name && c.Safe {
				matches++
				domain = learningNativeDomain(p.binding.Dialect, c.NativeType)
			}
		}
	}
	if matches != 1 {
		return ""
	}
	return domain
}
func pgLearningParameter(v any) (int, bool) {
	m := object(v)
	if len(m) != 1 || m["ParamRef"] == nil {
		return 0, false
	}
	n, ok := object(m["ParamRef"])["number"].(float64)
	return int(n), ok && n == float64(int(n))
}
func pgLearningCastDomain(v any) string {
	m := object(v)
	if len(m) != 1 || m["TypeCast"] == nil {
		return ""
	}
	typ := fieldObject(object(m["TypeCast"])["typeName"], "TypeName")
	if !only(typ, "names", "typmods", "typemod") {
		return ""
	}
	ns, ok := names(typ["names"])
	if !ok || len(ns) < 1 || len(ns) > 2 || len(ns) == 2 && ns[0] != "pg_catalog" {
		return ""
	}
	domain := learningNativeDomain("postgres", ns[len(ns)-1])
	modifiers := array(typ["typmods"])
	if len(modifiers) != 0 {
		if domain == "interval" {
			// Native interval range/precision modifiers are integer constants. Their
			// exact semantics remain in the SQL; a domain never claims equivalence.
			if len(modifiers) > 2 {
				return ""
			}
			for _, modifier := range modifiers {
				value, ok := analyticalIntegerConstant(modifier)
				if !ok || value < 0 || value > 65535 {
					return ""
				}
			}
		} else {
			if len(modifiers) != 1 || (domain != "timestamp" && domain != "timestamptz" && domain != "time" && domain != "timetz") {
				return ""
			}
			precision, ok := analyticalIntegerConstant(modifiers[0])
			if !ok || precision < 0 || precision > 6 {
				return ""
			}
		}
	}
	return domain
}
func (p *learningDomainProof) pgExprDomain(v any, scope learningDomainScope) string {
	if d := pgLearningCastDomain(v); d != "" {
		return d
	}
	m := object(v)
	if len(m) == 1 && m["ColumnRef"] != nil {
		ns, ok := names(object(m["ColumnRef"])["fields"])
		if ok {
			return p.columnDomain(ns, scope)
		}
	}
	return ""
}
func (p *learningDomainProof) pgSources(v any, scope learningDomainScope) {
	m := object(v)
	if r := object(m["RangeVar"]); r != nil {
		alias := text(fieldObject(r["alias"], "Alias")["aliasname"])
		if alias == "" {
			alias = text(r["relname"])
		}
		relation := p.relation(text(r["schemaname"]), text(r["relname"]))
		if text(r["schemaname"]) == "" {
			relation = nil
		}
		if r["alias"] != nil && len(array(fieldObject(r["alias"], "Alias")["colnames"])) > 0 {
			relation = nil
		}
		if _, ok := scope[alias]; ok {
			scope[alias] = nil
		} else {
			scope[alias] = relation
		}
		return
	}
	if j := object(m["JoinExpr"]); j != nil {
		if j["alias"] != nil {
			scope[text(fieldObject(j["alias"], "Alias")["aliasname"])] = nil
			return
		}
		p.pgSources(j["larg"], scope)
		p.pgSources(j["rarg"], scope)
		return
	}
	if d := object(m["RangeSubselect"]); d != nil {
		scope[text(fieldObject(d["alias"], "Alias")["aliasname"])] = nil
	}
}
func (p *learningDomainProof) pgWalk(v any, scope learningDomainScope, depth int) error {
	if err := p.bound(depth); err != nil {
		return err
	}
	switch node := v.(type) {
	case []any:
		for _, item := range node {
			if err := p.pgWalk(item, scope, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		if n, ok := pgLearningParameter(node); ok {
			return p.note(n, "")
		}
		if s := object(node["SelectStmt"]); len(node) == 1 && s != nil {
			local := learningDomainScope{}
			for _, from := range array(s["fromClause"]) {
				p.pgSources(from, local)
			}
			// Unqualified CTE/derived sources never borrow physical catalog types.
			for key, item := range s {
				if (key == "larg" || key == "rarg") && item != nil {
					item = map[string]any{"SelectStmt": item}
				}
				if err := p.pgWalk(item, local, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
		if c := object(node["TypeCast"]); len(node) == 1 && c != nil {
			if n, ok := pgLearningParameter(c["arg"]); ok {
				return p.note(n, pgLearningCastDomain(node))
			}
		}
		if a := object(node["A_Expr"]); len(node) == 1 && a != nil && text(a["kind"]) == "AEXPR_OP" {
			op, ok := names(a["name"])
			if ok && len(op) == 1 && (op[0] == "=" || op[0] == "<>" || op[0] == "<" || op[0] == ">" || op[0] == "<=" || op[0] == ">=") {
				for _, sides := range [][2]string{{"lexpr", "rexpr"}, {"rexpr", "lexpr"}} {
					if n, ok := pgLearningParameter(a[sides[0]]); ok {
						if err := p.note(n, p.pgExprDomain(a[sides[1]], scope)); err != nil {
							return err
						}
						return p.pgWalk(a[sides[1]], scope, depth+1)
					}
				}
			}
		}
		for _, item := range node {
			if err := p.pgWalk(item, scope, depth+1); err != nil {
				return err
			}
		}
	}
	return nil
}
