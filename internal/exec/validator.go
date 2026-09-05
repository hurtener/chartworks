package exec

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

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
	adapter ReadAdapter
	limits  config.ReadValidation
	slots   chan struct{}
}

// NewValidator uses the pinned native PostgreSQL parser compiled to WASM, not a keyword filter.
func NewValidator(adapter ReadAdapter, limits config.ReadValidation) (*Validator, error) {
	if adapter == nil || config.ValidateReadValidation(limits) != nil {
		return nil, ErrBinding
	}
	return &Validator{adapter: adapter, limits: limits, slots: make(chan struct{}, limits.Concurrency)}, nil
}

// Validate constructs the only nonzero executable Plan after authority, whole-tree
// resolution and constrained native EXPLAIN have all succeeded.
func (v *Validator) Validate(ctx context.Context, e identity.Envelope, r Request) (Plan, error) {
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
		return Plan{}, ErrUnsupported
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
