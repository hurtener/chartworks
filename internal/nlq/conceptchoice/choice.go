// Package conceptchoice validates a bounded model choice against a caller-owned
// reviewed candidate set. It contains no SQL, source authorization or confidence
// calibration. Its proof records only IDs and spans of already redacted input.
package conceptchoice

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	Version          = "grounded-concept-choice-v1"
	MaxCandidates    = 64
	MaxSelected      = 8
	MaxQuestionBytes = 16 << 10
)

var ErrChoice = errors.New("conceptchoice: invalid grounded selection")

// Proposal is a model response, not an executable selection or an access grant.
type Proposal struct {
	Decision     string      `json:"decision"`
	Selected     []Selection `json:"selected"`
	Alternatives []string    `json:"alternatives"`
}

// Selection connects one candidate to an exact, unambiguous excerpt of the
// redacted question. Multiple independent choices require disjoint excerpts.
type Selection struct {
	ID    string `json:"id"`
	Quote string `json:"quote"`
}

// Span records byte coordinates, not an additional retained private literal.
type Span struct {
	ID    string `json:"id"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// Proof is structural grounding evidence, not model confidence or semantic truth.
type Proof struct {
	Version      string   `json:"version"`
	Question     string   `json:"question_digest"`
	Candidates   string   `json:"candidate_digest"`
	Decision     string   `json:"decision"`
	Selected     []Span   `json:"selected,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"`
	Digest       string   `json:"digest"`
}

func digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func validID(id string) bool {
	if len(id) != 64 {
		return false
	}
	for _, r := range id {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
func inputs(ctx context.Context, question string, candidates []string) (map[string]bool, error) {
	if ctx == nil {
		return nil, ErrChoice
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(question) == 0 || len(question) > MaxQuestionBytes || !utf8.ValidString(question) || strings.ContainsRune(question, 0) || len(candidates) == 0 || len(candidates) > MaxCandidates {
		return nil, ErrChoice
	}
	seen := map[string]bool{}
	for _, id := range candidates {
		if !validID(id) || seen[id] {
			return nil, ErrChoice
		}
		seen[id] = true
	}
	return seen, nil
}

// Resolve checks a structured proposal with exact candidate and quote membership.
// A quote proves only which words a model attached to a concept; it cannot prove
// that the semantic interpretation is right. Uncertainty is an explicit outcome.
func Resolve(ctx context.Context, question string, candidates []string, proposal Proposal) (Proof, error) {
	ids, err := inputs(ctx, question, candidates)
	if err != nil {
		return Proof{}, err
	}
	if len(proposal.Selected) > MaxSelected || len(proposal.Alternatives) > MaxSelected {
		return Proof{}, ErrChoice
	}
	p := Proof{Version: Version, Question: digest(question), Candidates: digest(candidates), Decision: proposal.Decision}
	seen := map[string]bool{}
	switch proposal.Decision {
	case "select":
		if len(proposal.Selected) == 0 || len(proposal.Alternatives) != 0 {
			return Proof{}, ErrChoice
		}
		for _, s := range proposal.Selected {
			if err := ctx.Err(); err != nil {
				return Proof{}, err
			}
			if !ids[s.ID] || seen[s.ID] || len(s.Quote) == 0 || len(s.Quote) > 1024 || strings.TrimSpace(s.Quote) != s.Quote || !utf8.ValidString(s.Quote) || strings.Contains(s.Quote, "[redacted answer]") || strings.Count(question, s.Quote) != 1 {
				return Proof{}, ErrChoice
			}
			start := strings.Index(question, s.Quote)
			end := start + len(s.Quote)
			if overlapsRedaction(question, start, end) || !utf8.ValidString(question[:start]) || !utf8.ValidString(question[:end]) {
				return Proof{}, ErrChoice
			}
			for _, old := range p.Selected {
				if start < old.End && old.Start < end {
					return Proof{}, ErrChoice
				}
			}
			seen[s.ID] = true
			p.Selected = append(p.Selected, Span{ID: s.ID, Start: start, End: end})
		}
		sort.Slice(p.Selected, func(i, j int) bool { return p.Selected[i].ID < p.Selected[j].ID })
	case "clarify":
		if len(proposal.Selected) != 0 || len(proposal.Alternatives) < 2 {
			return Proof{}, ErrChoice
		}
		for _, id := range proposal.Alternatives {
			if !ids[id] || seen[id] {
				return Proof{}, ErrChoice
			}
			seen[id] = true
			p.Alternatives = append(p.Alternatives, id)
		}
		sort.Strings(p.Alternatives)
	case "no_match":
		if len(proposal.Selected)+len(proposal.Alternatives) != 0 {
			return Proof{}, ErrChoice
		}
	default:
		return Proof{}, ErrChoice
	}
	p.Digest = digest(p)
	return p, nil
}

// Verify reconstructs the same bounded proof without a model call. Callers must
// reconstruct the reviewed candidates and redacted question from protected state.
func Verify(ctx context.Context, question string, candidates []string, p Proof) error {
	if _, err := inputs(ctx, question, candidates); err != nil {
		return err
	}
	if len(p.Selected) > MaxSelected || len(p.Alternatives) > MaxSelected {
		return ErrChoice
	}
	proposal := Proposal{Decision: p.Decision, Alternatives: append([]string(nil), p.Alternatives...)}
	for _, s := range p.Selected {
		if s.Start < 0 || s.End <= s.Start || s.End > len(question) {
			return ErrChoice
		}
		proposal.Selected = append(proposal.Selected, Selection{ID: s.ID, Quote: question[s.Start:s.End]})
	}
	expected, err := Resolve(ctx, question, candidates, proposal)
	if err != nil {
		return err
	}
	if digest(expected) != digest(p) {
		return ErrChoice
	}
	return nil
}

// Clone returns detached evidence suitable for protected snapshots.
func (p Proof) Clone() Proof {
	p.Selected = append([]Span(nil), p.Selected...)
	p.Alternatives = append([]string(nil), p.Alternatives...)
	return p
}

// A partial quote such as "answer" must not launder a redaction marker into
// evidence. Byte coordinates are relative to the exact sanitized question.
func overlapsRedaction(question string, start, end int) bool {
	const marker = "[redacted answer]"
	for offset := 0; offset < len(question); {
		index := strings.Index(question[offset:], marker)
		if index < 0 {
			return false
		}
		index += offset
		if start < index+len(marker) && index < end {
			return true
		}
		offset = index + len(marker)
	}
	return false
}
