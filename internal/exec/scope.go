package exec

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/identity"
)

// RelationScope is a restrictive semantic projection of a real source binding.
// It can remove relations/columns, never add data access or construct a Plan.
type RelationScope struct {
	Dataset string   `json:"dataset"`
	Columns []string `json:"columns"`
}

// ValidateWithin applies the same whole-tree/native checks as Validate, with a
// smaller semantic allowlist. The sealed plan remains bound to the complete real
// source revision, so ordinary source/executor fences still apply unchanged.
func (v *Validator) ValidateWithin(ctx context.Context, e identity.Envelope, r Request, scope []RelationScope) (Plan, error) {
	if len(scope) == 0 {
		return Plan{}, ErrBinding
	}
	return v.validate(ctx, e, r, scope)
}

func narrowBinding(b Binding, scope []RelationScope) (Binding, error) {
	out := b.Clone()
	if scope == nil {
		return out, nil
	}
	if len(scope) == 0 || len(scope) > 32 {
		return Binding{}, ErrBinding
	}
	out.Relations = nil
	seen := map[string]bool{}
	for _, s := range scope {
		if seen[s.Dataset] || len(s.Columns) < 1 || len(s.Columns) > 256 {
			return Binding{}, ErrBinding
		}
		seen[s.Dataset] = true
		var selected *Relation
		for _, r := range b.Relations {
			if r.ID == s.Dataset {
				r.Columns = nil
				selected = &r
				break
			}
		}
		if selected == nil {
			return Binding{}, ErrBinding
		}
		columns := map[string]bool{}
		for _, name := range s.Columns {
			if columns[name] {
				return Binding{}, ErrBinding
			}
			columns[name] = true
			found := false
			for _, r := range b.Relations {
				if r.ID != s.Dataset {
					continue
				}
				for _, c := range r.Columns {
					if c.Name == name && c.Safe {
						selected.Columns = append(selected.Columns, c)
						found = true
					}
				}
			}
			if !found {
				return Binding{}, ErrBinding
			}
		}
		sort.Slice(selected.Columns, func(i, j int) bool { return selected.Columns[i].Name < selected.Columns[j].Name })
		out.Relations = append(out.Relations, *selected)
	}
	sort.Slice(out.Relations, func(i, j int) bool { return out.Relations[i].ID < out.Relations[j].ID })
	return out, nil
}
