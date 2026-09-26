package exec

import (
	"context"
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	bruinsql "github.com/bruin-data/bruin/pkg/sqlparser"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec/sqlpolicy"
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

func validationAuthority(adapter ReadAdapter, e identity.Envelope, binding Binding, dependencies []string) (PrivatePipelineValidationAdapter, string, error) {
	private, ok := adapter.(PrivatePipelineValidationAdapter)
	if !ok {
		return nil, "", Require(e, binding, dependencies)
	}
	proof, err := private.AuthorizePrivatePipeline(e, binding, dependencies)
	if err != nil {
		return nil, "", err
	}
	if !validPrivatePipelineProof(proof) {
		return nil, "", ErrBinding
	}
	return private, proof, nil
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
	return v.validate(ctx, e, r, nil)
}

func (v *Validator) validate(ctx context.Context, e identity.Envelope, r Request, scope []RelationScope) (Plan, error) {
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
	scoped, err := narrowBinding(binding, scope)
	if err != nil {
		return Plan{}, err
	}
	scopeDigest := ""
	if scope != nil {
		scopeDigest = Hash(scoped.Relations)
	}
	if binding.Dialect != "postgres" {
		return v.validateWarehouse(ctx, e, r, binding, scoped, scopeDigest)
	}
	_, initialProof, err := validationAuthority(v.adapter, e, binding, nil)
	if err != nil {
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
	resolver := sqlResolver{binding: scoped, dependencies: map[string]bool{}, parameters: map[int]bool{}, parameterCount: len(r.Parameters)}
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
	private, proof, err := validationAuthority(v.adapter, e, binding, deps)
	if err != nil {
		return Plan{}, err
	}
	if proof != initialProof {
		return Plan{}, ErrBinding
	}
	candidate := Candidate{binding: binding.Clone(), semanticScope: scopeDigest, owner: e, authority: authority(e), private: private, privateProof: proof, statement: r.SQL, parameters: append([]Parameter(nil), r.Parameters...), dependencies: deps, columns: append([]string(nil), columns...), checked: true}
	if err = v.adapter.Explain(ctx, e, candidate); err != nil {
		return Plan{}, err
	}
	if !e.Valid() {
		return Plan{}, ErrBinding
	}
	return Plan{candidate: candidate, nativeChecked: true}, nil
}

func (v *Validator) validateWarehouse(ctx context.Context, e identity.Envelope, r Request, binding, scoped Binding, scopeDigest string) (Plan, error) {
	dialect, ok := sqlpolicy.NativeDialect(binding.Dialect)
	if !ok || binding.Dialect == "postgres" {
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
	parameters := inspection.Parameters
	if parameters != len(r.Parameters) {
		// The fallback compensates for omitted markers; it cannot erase markers
		// already reported by the native parser. In particular, an unsupported
		// fallback spelling must not turn a missing binding into a zero count.
		parameters, err = warehouseParameterCount(ctx, r.SQL, binding.Dialect)
		if parameters < inspection.Parameters {
			return Plan{}, ErrBinding
		}
	}
	if err != nil || parameters != len(r.Parameters) || len(inspection.Outputs) < 1 || len(inspection.Outputs) > 256 {
		return Plan{}, ErrBinding
	}
	dependencies := make([]string, 0, len(inspection.Tables))
	selected := make([]Relation, 0, len(inspection.Tables))
	for _, table := range inspection.Tables {
		var relation Relation
		matches := 0
		for _, candidate := range scoped.Relations {
			if warehouseRelationMatches(binding, candidate, table, false) {
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
			if column.Table != "" && !warehouseRelationMatches(binding, relation, column.Table, true) {
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
	for _, function := range inspection.Functions {
		if !sqlpolicy.AllowsFunction(binding.Dialect, []string{function}) {
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
	candidate := Candidate{binding: binding.Clone(), semanticScope: scopeDigest, owner: e, authority: authority(e), statement: r.SQL, parameters: append([]Parameter(nil), r.Parameters...), dependencies: dependencies, columns: append([]string(nil), inspection.Outputs...), checked: true}
	if err = v.adapter.Explain(ctx, e, candidate); err != nil {
		return Plan{}, err
	}
	return Plan{candidate: candidate, nativeChecked: true}, nil
}

// warehouseParameterCount compensates for omitted markers in otherwise parsed
// read trees (such as LIKE ... ESCAPE). Its result cannot reduce the native
// inspection count and never authorizes SQL on its own.
func warehouseParameterCount(ctx context.Context, statement, dialect string) (int, error) {
	tokens, err := businessScan(ctx, statement, true)
	if err != nil {
		return 0, err
	}
	seen := map[int]bool{}
	positional := 0
	for _, token := range tokens {
		if token.kind != 'p' {
			continue
		}
		positional++
		index, err := businessParameterIndex(token.text, dialect, positional)
		if err != nil || index < 1 || index > 64 {
			return 0, ErrBinding
		}
		seen[index] = true
	}
	for i := 1; i <= len(seen); i++ {
		if !seen[i] {
			return 0, ErrBinding
		}
	}
	return len(seen), nil
}

func warehouseRelationMatches(binding Binding, relation Relation, name string, allowBare bool) bool {
	parts := strings.Split(name, ".")
	switch len(parts) {
	case 1:
		return allowBare && parts[0] == relation.Name
	case 2:
		return parts[0] == relation.Schema && parts[1] == relation.Name
	case 3:
		if binding.Catalog == "" {
			return false
		}
		switch binding.Dialect {
		case "sqlserver", "bigquery", "snowflake", "databricks":
			return parts[0] == binding.Catalog && parts[1] == relation.Schema && parts[2] == relation.Name
		}
	}
	return false
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
