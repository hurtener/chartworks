package exec

import (
	"context"
	"encoding/json"
	pgquery "github.com/wasilibs/go-pgquery"
)

func analyticalSQLTree(ctx context.Context, sql string, binding Binding) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if binding.Dialect != "postgres" {
		return warehouseAnalyticalAST(ctx, sql, binding)
	}
	raw, err := pgquery.ParseToJSON(sql)
	if err != nil {
		return nil, analyticalFailure("analytical_shape_unsupported", true)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var tree map[string]any
	if json.Unmarshal([]byte(raw), &tree) != nil {
		return nil, ErrBinding
	}
	stmts := array(tree["stmts"])
	if len(stmts) != 1 {
		return nil, ErrBinding
	}
	stmt := object(object(stmts[0])["stmt"])
	if len(stmt) != 1 || stmt["SelectStmt"] == nil {
		return nil, ErrBinding
	}
	return object(stmt["SelectStmt"]), nil
}
