package engineering

import (
	"encoding/json"
	"strings"

	readexec "github.com/hurtener/chartworks/internal/exec"
	pgquery "github.com/wasilibs/go-pgquery"
)

// validatePipelineRendered independently restricts every generated write target.
// Input SELECTs have already passed the ordinary validator against actual source
// bindings. No returned renderer string is itself an authorization receipt.
func validatePipelineRendered(statement, schema, table string) error {
	raw, err := pgquery.ParseToJSON(statement)
	if err != nil {
		return readexec.ErrUnsafe
	}
	var doc struct {
		Stmts []struct {
			Stmt map[string]json.RawMessage `json:"stmt"`
		} `json:"stmts"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil || len(doc.Stmts) == 0 || len(doc.Stmts) > 32 {
		return readexec.ErrUnsafe
	}
	temps := map[string]bool{}
	relation := func(raw json.RawMessage) (string, string, string) {
		var v struct {
			Schema      string `json:"schemaname"`
			Name        string `json:"relname"`
			Persistence string `json:"relpersistence"`
		}
		_ = json.Unmarshal(raw, &v)
		return v.Schema, v.Name, v.Persistence
	}
	for _, stmt := range doc.Stmts {
		if raw, ok := stmt.Stmt["CreateTableAsStmt"]; ok {
			var v struct {
				Into struct {
					Rel json.RawMessage `json:"rel"`
				} `json:"into"`
			}
			if json.Unmarshal(raw, &v) != nil {
				return readexec.ErrUnsafe
			}
			s, n, p := relation(v.Into.Rel)
			if p == "t" && s == "" && strings.HasPrefix(n, "__bruin_") {
				temps[n] = true
			}
		}
	}
	target := func(raw json.RawMessage) bool {
		s, n, _ := relation(raw)
		return s == schema && n == table || s == "" && temps[n]
	}
	for _, stmt := range doc.Stmts {
		if len(stmt.Stmt) != 1 {
			return readexec.ErrUnsafe
		}
		for kind, raw := range stmt.Stmt {
			switch kind {
			case "TransactionStmt":
				var v struct {
					Kind string `json:"kind"`
				}
				if json.Unmarshal(raw, &v) != nil || (v.Kind != "TRANS_STMT_BEGIN" && v.Kind != "TRANS_STMT_COMMIT") {
					return readexec.ErrUnsafe
				}
			case "VariableSetStmt":
				var v struct {
					Name  string `json:"name"`
					Local bool   `json:"is_local"`
					Args  []struct {
						AConst struct {
							Sval struct {
								Value string `json:"sval"`
							} `json:"sval"`
						} `json:"A_Const"`
					} `json:"args"`
				}
				if json.Unmarshal(raw, &v) != nil || v.Name != "timezone" || !v.Local || len(v.Args) != 1 || v.Args[0].AConst.Sval.Value != "UTC" {
					return readexec.ErrUnsafe
				}
			case "CreateTableAsStmt":
				var v struct {
					Into struct {
						Rel json.RawMessage `json:"rel"`
					} `json:"into"`
				}
				if json.Unmarshal(raw, &v) != nil || !target(v.Into.Rel) {
					return readexec.ErrUnsafe
				}
			case "InsertStmt", "DeleteStmt", "UpdateStmt", "MergeStmt":
				var v struct {
					Relation json.RawMessage `json:"relation"`
				}
				if json.Unmarshal(raw, &v) != nil || !target(v.Relation) {
					return readexec.ErrUnsafe
				}
			case "DropStmt":
				var v struct {
					RemoveType string `json:"removeType"`
					Behavior   string `json:"behavior"`
					Objects    []struct {
						List struct {
							Items []struct {
								String struct {
									Value string `json:"sval"`
								} `json:"String"`
							} `json:"items"`
						} `json:"List"`
					} `json:"objects"`
				}
				if json.Unmarshal(raw, &v) != nil || v.RemoveType != "OBJECT_TABLE" || v.Behavior == "DROP_CASCADE" || len(v.Objects) == 0 {
					return readexec.ErrUnsafe
				}
				for _, o := range v.Objects {
					parts := o.List.Items
					if len(parts) == 1 && temps[parts[0].String.Value] {
						continue
					}
					if len(parts) != 2 || parts[0].String.Value != schema || parts[1].String.Value != table {
						return readexec.ErrUnsafe
					}
				}
			default:
				return readexec.ErrUnsafe
			}
		}
	}
	return nil
}
