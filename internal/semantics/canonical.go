package semantics

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/identity"
)

// CanonicalMeaning is the tenant-wide part of a canonical entity revision.
// Topic-local physical keys are deliberately excluded: imports may remap those
// keys without changing the stable business identity or its exact revision.
type CanonicalMeaning struct {
	ID       string   `json:"id"`
	Revision int64    `json:"revision"`
	Name     string   `json:"name"`
	Aliases  []string `json:"aliases"`
}

// Meaning removes topic-local physical key mappings from an entity revision.
func (e CanonicalEntity) Meaning() CanonicalMeaning {
	return CanonicalMeaning{ID: e.ID, Revision: e.Revision, Name: e.Name, Aliases: append([]string(nil), e.Aliases...)}
}

// CanonicalMeanings returns detached, compiler-canonical registry proposals.
func (m Model) CanonicalMeanings() []CanonicalMeaning {
	out := make([]CanonicalMeaning, 0, len(m.pack.CanonicalEntities))
	for _, entity := range m.pack.CanonicalEntities {
		out = append(out, entity.Meaning())
	}
	return out
}

// Digest identifies exact global meaning independently of topic-local key maps.
func (m CanonicalMeaning) Digest() string {
	if !m.Valid() {
		return ""
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Terms returns the normalized name and aliases in canonical order.
func (m CanonicalMeaning) Terms() []string {
	if !identity.Identifier(m.ID) || m.Revision < 1 || m.Revision >= 1<<62 || !validLine(m.Name, 256) || len(m.Aliases) > 32 {
		return nil
	}
	out := make([]string, 0, len(m.Aliases)+1)
	seen := map[string]bool{}
	name, ok := normalizeTerm(m.Name)
	if !ok {
		return nil
	}
	out = append(out, name)
	seen[name] = true
	for _, alias := range m.Aliases {
		term, valid := normalizeTerm(alias)
		if !valid || seen[term] {
			return nil
		}
		out = append(out, term)
		seen[term] = true
	}
	return out
}

func (m CanonicalMeaning) Valid() bool { return len(m.Terms()) != 0 }
