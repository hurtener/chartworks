package exec

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	bruinsql "github.com/bruin-data/bruin/pkg/sqlparser"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	pgquery "github.com/wasilibs/go-pgquery"
)

// Request contains the complete SQL and parameter input. The source resolves the
// actual context; neither SQL nor a request label can modify its restrictions.
type Request struct {
	Source     string      `json:"source"`
	Context    string      `json:"context"`
	SQL        string      `json:"sql"`
	Parameters []Parameter `json:"parameters"`
}

// Validator shares only immutable settings and a bounded parser admission semaphore.
type Validator struct {
	adapter         ReadAdapter
	limits          config.ReadValidation
	slots           chan struct{}
	warehouseParser *bruinsql.RustSQLParser
}

// NewValidator uses the pinned native PostgreSQL parser compiled to WASM, not a keyword filter.
func NewValidator(adapter ReadAdapter, limits config.ReadValidation) (*Validator, error) {
	if adapter == nil || reflect.ValueOf(adapter).Kind() == reflect.Pointer && reflect.ValueOf(adapter).IsNil() || config.ValidateReadValidation(limits) != nil {
		return nil, ErrBinding
	}
	// Initialize the pinned WASM parser at explicit capability construction, not
	// during the first user request. Disabled sources never initialize it.
	if _, err := pgquery.ParseToJSON("SELECT 1"); err != nil {
		return nil, ErrUnsupported
	}
	warehouseParser, err := bruinsql.NewRustSQLParserWithConfig(false, limits.MaxSQLBytes)
	if err != nil || warehouseParser.Start() != nil {
		return nil, ErrUnsupported
	}
	return &Validator{adapter: adapter, limits: limits, slots: make(chan struct{}, limits.Concurrency), warehouseParser: warehouseParser}, nil
}

// Validate constructs the only nonzero executable Plan after authority, whole-tree
// resolution and constrained native EXPLAIN have all succeeded.
func (v *Validator) Validate(ctx context.Context, e identity.Envelope, r Request) (Plan, error) {
	if ctx == nil {
		return Plan{}, ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return Plan{}, err
	}
	if !e.Valid() {
		return Plan{}, ErrBinding
	}
	if !identity.Identifier(r.Source) || !identity.Identifier(r.Context) || len(r.SQL) < 1 || len(r.SQL) > v.limits.MaxSQLBytes || len(r.Parameters) > v.limits.MaxParameters || strings.ContainsRune(r.SQL, 0) {
		return Plan{}, ErrLimit
	}
	for _, p := range r.Parameters {
		if !p.Valid() {
			return Plan{}, ErrBinding
		}
	}
	// This conservative lexical bound is allocation/stack admission only. It does
	// not authorize SQL; the native whole-tree parser below supplies that proof.
	if strings.Count(r.SQL, "(") > 256 {
		return Plan{}, ErrLimit
	}
	binding, err := v.adapter.Binding(ctx, e, r.Source, r.Context)
	if err != nil {
		return Plan{}, err
	}
	if !binding.Valid() || binding.Source != r.Source || binding.Context != r.Context {
		return Plan{}, ErrBinding
	}
	if binding.Dialect != "postgres" {
		return v.validateWarehouse(ctx, e, r, binding)
	}
	if err = Require(e, binding, nil); err != nil {
		return Plan{}, err
	}
	select {
	case v.slots <- struct{}{}:
	case <-ctx.Done():
		return Plan{}, ctx.Err()
	}
	raw, err := pgquery.ParseToJSON(r.SQL)
	<-v.slots
	if err != nil {
		return Plan{}, ErrUnsafe
	}
	if err = ctx.Err(); err != nil {
		return Plan{}, err
	}
	var tree map[string]any
	if json.Unmarshal([]byte(raw), &tree) != nil {
		return Plan{}, ErrUnsafe
	}
	nodes := 0
	if !boundedTree(tree, 0, &nodes, v.limits) {
		return Plan{}, ErrLimit
	}
	stmts := array(tree["stmts"])
	if len(stmts) != 1 {
		return Plan{}, ErrUnsafe
	}
	statement := object(stmts[0])["stmt"]
	resolver := sqlResolver{binding: binding.Clone(), dependencies: map[string]bool{}, parameters: map[int]bool{}, parameterCount: len(r.Parameters)}
	columns, err := resolver.selectStatement(statement, nil)
	if err != nil {
		return Plan{}, err
	}
	if len(resolver.parameters) != len(r.Parameters) {
		return Plan{}, ErrBinding
	}
	deps := make([]string, 0, len(resolver.dependencies))
	for id := range resolver.dependencies {
		deps = append(deps, id)
	}
	sort.Strings(deps)
	if err = Require(e, binding, deps); err != nil {
		return Plan{}, err
	}
	candidate := Candidate{binding: binding.Clone(), owner: e, authority: authority(e), statement: r.SQL, parameters: append([]Parameter(nil), r.Parameters...), dependencies: deps, columns: append([]string(nil), columns...), checked: true}
	if err = v.adapter.Explain(ctx, e, candidate); err != nil {
		return Plan{}, err
	}
	if !e.Valid() {
		return Plan{}, ErrBinding
	}
	return Plan{candidate: candidate, nativeChecked: true}, nil
}

func (v *Validator) validateWarehouse(ctx context.Context, e identity.Envelope, r Request, binding Binding) (Plan, error) {
	dialects := map[string]string{"mysql": "mysql", "sqlserver": "tsql", "bigquery": "bigquery", "snowflake": "snowflake", "databricks": "databricks"}
	dialect, ok := dialects[binding.Dialect]
	if !ok {
		return Plan{}, ErrUnsupported
	}
	select {
	case v.slots <- struct{}{}:
	case <-ctx.Done():
		return Plan{}, ctx.Err()
	}
	inspection, err := v.warehouseParser.InspectRead(r.SQL, dialect, v.limits.MaxASTNodes, v.limits.MaxASTDepth)
	<-v.slots
	if err != nil {
		return Plan{}, ErrUnsafe
	}
	if inspection.Parameters != len(r.Parameters) || len(inspection.Outputs) < 1 || len(inspection.Outputs) > 256 {
		return Plan{}, ErrBinding
	}
	dependencies := make([]string, 0, len(inspection.Tables))
	selected := make([]Relation, 0, len(inspection.Tables))
	for _, table := range inspection.Tables {
		var relation Relation
		matches := 0
		for _, candidate := range binding.Relations {
			qualified := candidate.Schema + "." + candidate.Name
			if table == qualified || strings.HasSuffix(table, "."+qualified) {
				relation, matches = candidate, matches+1
			}
		}
		if matches != 1 {
			return Plan{}, ErrUnsafe
		}
		dependencies = append(dependencies, relation.ID)
		selected = append(selected, relation)
	}
	for _, column := range inspection.Columns {
		if column.Name == "*" || !SQLIdentifier(strings.ToLower(column.Name)) {
			return Plan{}, ErrUnsupported
		}
		matches := 0
		for _, relation := range selected {
			if column.Table != "" && column.Table != relation.Name && column.Table != relation.Schema+"."+relation.Name {
				continue
			}
			for _, candidate := range relation.Columns {
				if candidate.Name == column.Name && candidate.Safe {
					matches++
				}
			}
		}
		if matches != 1 {
			return Plan{}, ErrUnsafe
		}
	}
	allowedFunctions := map[string]bool{"abs": true, "avg": true, "coalesce": true, "count": true, "length": true, "lower": true, "max": true, "min": true, "round": true, "sum": true, "upper": true}
	for _, function := range inspection.Functions {
		if !allowedFunctions[strings.ToLower(function)] {
			return Plan{}, ErrUnsupported
		}
	}
	for _, output := range inspection.Outputs {
		if !SQLIdentifier(strings.ToLower(output)) {
			return Plan{}, ErrUnsupported
		}
	}
	sort.Strings(dependencies)
	if err = Require(e, binding, dependencies); err != nil {
		return Plan{}, err
	}
	candidate := Candidate{binding: binding.Clone(), owner: e, authority: authority(e), statement: r.SQL, parameters: append([]Parameter(nil), r.Parameters...), dependencies: dependencies, columns: append([]string(nil), inspection.Outputs...), checked: true}
	if err = v.adapter.Explain(ctx, e, candidate); err != nil {
		return Plan{}, err
	}
	return Plan{candidate: candidate, nativeChecked: true}, nil
}

func boundedTree(v any, depth int, count *int, limits config.ReadValidation) bool {
	*count++
	if depth > limits.MaxASTDepth || *count > limits.MaxASTNodes {
		return false
	}
	switch x := v.(type) {
	case map[string]any:
		for _, item := range x {
			if !boundedTree(item, depth+1, count, limits) {
				return false
			}
		}
	case []any:
		for _, item := range x {
			if !boundedTree(item, depth+1, count, limits) {
				return false
			}
		}
	}
	return true
}
