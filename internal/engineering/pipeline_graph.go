package engineering

import (
	"encoding/json"
	"strings"

	pgquery "github.com/wasilibs/go-pgquery"
)

// validatePipelineDerivedSQL is only a provisional graph check. Execution still
// requires the source service's actual registered-stage/native validation proof.
func validatePipelineDerivedSQL(step PipelineStep) error {
	sql := step.SQL
	if len(sql) > 1<<20 || strings.Count(sql, "(") > 256 {
		return ErrInvalid
	}
	allowed := map[string]bool{}
	for _, id := range step.FromSteps {
		allowed[id] = true
		sql = strings.ReplaceAll(sql, "{{step."+id+"}}", quotePipelineRelation("cw_dependency", id))
	}
	raw, err := pgquery.ParseToJSON(sql)
	if err != nil {
		return ErrInvalid
	}
	var doc struct {
		Stmts []struct {
			Stmt map[string]any `json:"stmt"`
		} `json:"stmts"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil || len(doc.Stmts) != 1 || len(doc.Stmts[0].Stmt) != 1 || doc.Stmts[0].Stmt["SelectStmt"] == nil {
		return ErrInvalid
	}
	ctes := map[string]bool{}
	used := map[string]bool{}
	valid := true
	var collect func(any)
	collect = func(value any) {
		switch node := value.(type) {
		case []any:
			for _, v := range node {
				collect(v)
			}
		case map[string]any:
			if cte, ok := node["CommonTableExpr"].(map[string]any); ok {
				name, _ := cte["ctename"].(string)
				ctes[name] = true
			}
			for _, v := range node {
				collect(v)
			}
		}
	}
	collect(doc.Stmts[0].Stmt)
	var inspect func(any)
	inspect = func(value any) {
		switch node := value.(type) {
		case []any:
			for _, v := range node {
				inspect(v)
			}
		case map[string]any:
			if relation, ok := node["RangeVar"].(map[string]any); ok {
				schema, _ := relation["schemaname"].(string)
				name, _ := relation["relname"].(string)
				if schema == "cw_dependency" && allowed[name] {
					used[name] = true
				} else if schema != "" || !ctes[name] {
					valid = false
				}
			}
			if _, ok := node["InsertStmt"]; ok {
				valid = false
			}
			if _, ok := node["UpdateStmt"]; ok {
				valid = false
			}
			if _, ok := node["DeleteStmt"]; ok {
				valid = false
			}
			if _, ok := node["intoClause"]; ok {
				valid = false
			}
			for _, v := range node {
				inspect(v)
			}
		}
	}
	inspect(doc.Stmts[0].Stmt)
	if !valid || len(used) != len(allowed) {
		return ErrInvalid
	}
	return nil
}
