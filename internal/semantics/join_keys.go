package semantics

import (
	"sort"
	"strings"
)

// JoinKeyPair is one reviewed equality in the same ordered relationship.
// Additional pairs never authorize another relation or arbitrary SQL text.
type JoinKeyPair struct {
	Left  Reference `json:"left"`
	Right Reference `json:"right"`
}

func joinKeyPairs(left, right Reference, additional []JoinKeyPair) []JoinKeyPair {
	if len(additional) > 15 {
		return nil
	}
	out := make([]JoinKeyPair, 1, len(additional)+1)
	out[0] = JoinKeyPair{Left: left, Right: right}
	return append(out, additional...)
}
func (j Join) KeyPairs() []JoinKeyPair { return joinKeyPairs(j.Left, j.Right, j.AdditionalKeys) }
func (j RelationshipDecision) KeyPairs() []JoinKeyPair {
	return joinKeyPairs(j.Left, j.Right, j.AdditionalKeys)
}
func joinKeyReferences(pairs []JoinKeyPair) []Reference {
	out := make([]Reference, 0, 2*len(pairs))
	for _, p := range pairs {
		out = append(out, p.Left, p.Right)
	}
	return out
}
func (j Join) References() []Reference                 { return joinKeyReferences(j.KeyPairs()) }
func (j RelationshipDecision) References() []Reference { return joinKeyReferences(j.KeyPairs()) }
func validJoinKeys(left, right Reference, additional []JoinKeyPair) bool {
	if len(additional) > 15 {
		return false
	}
	seenLeft, seenRight := map[Reference]bool{}, map[Reference]bool{}
	for _, p := range joinKeyPairs(left, right, additional) {
		if !p.Left.Valid() || !p.Right.Valid() || p.Left.Kind != KindColumn || p.Right.Kind != KindColumn || p.Left.Dataset != left.Dataset || p.Right.Dataset != right.Dataset || seenLeft[p.Left] || seenRight[p.Right] {
			return false
		}
		seenLeft[p.Left], seenRight[p.Right] = true, true
	}
	return true
}
func joinPairIdentity(pairs []JoinKeyPair, reverse bool) string {
	// Sorting the complete pair list prevents a matching first key from hiding
	// extra terms and treats reordered equality conjunctions consistently.
	keys := make([]string, 0, len(pairs))
	for _, p := range pairs {
		l, r := p.Left, p.Right
		if reverse {
			l, r = r, l
		}
		keys = append(keys, l.key()+"\x01"+r.key())
	}
	sort.Strings(keys)
	return strings.Join(keys, "\x02")
}

func canonicalJoinKeys(left, right *Reference, additional *[]JoinKeyPair) {
	if len(*additional) == 0 {
		return
	}
	pairs := joinKeyPairs(*left, *right, *additional)
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].Left.key()+"\x01"+pairs[i].Right.key() < pairs[j].Left.key()+"\x01"+pairs[j].Right.key()
	})
	*left, *right = pairs[0].Left, pairs[0].Right
	*additional = append([]JoinKeyPair(nil), pairs[1:]...)
}
