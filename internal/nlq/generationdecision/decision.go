// Package generationdecision defines bounded generation readiness, not SQL
// authorization, source facts, approved choices or an analytical certificate.
package generationdecision

import (
	"encoding/hex"
	"errors"
	"github.com/hurtener/chartworks/internal/identity"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	Version          = "generation-decision-v1"
	Ready            = "ready"
	Clarify          = "clarify"
	Insufficient     = "insufficient_context"
	MaxQuestions     = 8
	MaxQuestionBytes = 512
)

// ErrInvalid never echoes untrusted decision prose.
var ErrInvalid = errors.New("nlq: invalid generation decision")

// Problem is the public, redacted non-executable outcome. The owner redacts
// private values before constructing it. Questions are model-authored requests,
// never authoritative catalog choices or instructions for automatic execution.
type Choice struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

type Problem struct {
	Resume        string    `json:"resume,omitempty"`
	QueryID       string    `json:"query_id,omitempty"`
	AnswerContext string    `json:"answer_context,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	Choices       []Choice  `json:"choices,omitempty"`
	Version       string    `json:"version"`
	Outcome       string    `json:"outcome"`
	Questions     []string  `json:"questions"`
}

// Check enforces the decision half of the response union. hasSQL and counts
// describe the actual candidate, not a model-provided readiness assertion alone.
// SQL safety and parameter validity remain separate required checks for Ready.
func Check(outcome string, questions []string, hasSQL bool, parameters, assumptions, ambiguities int) error {
	if parameters < 0 || assumptions < 0 || ambiguities < 0 {
		return ErrInvalid
	}
	switch outcome {
	case Ready:
		if !hasSQL || len(questions) != 0 {
			return ErrInvalid
		}
	case Clarify, Insufficient:
		if hasSQL || parameters+assumptions+ambiguities != 0 || !validQuestions(questions) {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}

func validQuestions(questions []string) bool {
	if len(questions) < 1 || len(questions) > MaxQuestions {
		return false
	}
	for _, q := range questions {
		if len(q) == 0 || len(q) > MaxQuestionBytes || strings.TrimSpace(q) != q || !utf8.ValidString(q) || strings.ContainsAny(q, "\x00\r\n") {
			return false
		}
	}
	return true
}

// Public returns a detached, valid error projection or nil. This validates
// structure; only the request-owning service can supply known-value redaction.
func Public(p Problem) *Problem {
	if p.Version != Version || p.Outcome != Clarify && p.Outcome != Insufficient || !validQuestions(p.Questions) {
		return nil
	}
	if (p.QueryID == "") != (p.AnswerContext == "") || len(p.Choices) > 128 {
		return nil
	}
	if p.QueryID == "" {
		if !p.ExpiresAt.IsZero() || len(p.Choices) != 0 || p.Resume != "" {
			return nil
		}
	} else {
		digest, err := hex.DecodeString(p.AnswerContext)
		if (p.Resume != "plan" && p.Resume != "refine") || !identity.Identifier(p.QueryID) || err != nil || len(digest) != 32 || strings.ToLower(p.AnswerContext) != p.AnswerContext || p.ExpiresAt.IsZero() {
			return nil
		}
	}
	seen := map[Choice]bool{}
	for _, c := range p.Choices {
		if !identity.Identifier(c.ID) || c.Kind != "measure" && c.Kind != "kpi" && c.Kind != "dimension" || seen[c] {
			return nil
		}
		seen[c] = true
	}
	p.Choices = append([]Choice(nil), p.Choices...)
	p.Questions = append([]string(nil), p.Questions...)
	return &p
}

// ErrorCode is a closed transport discriminator, never a model-selected code.
func ErrorCode(outcome string) string {
	switch outcome {
	case Clarify:
		return "generation_clarification_required"
	case Insufficient:
		return "generation_context_insufficient"
	}
	return ""
}

// MatchesCode keeps an unrelated HTTP/MCP error from carrying a valid but
// misleading generation problem. Its textual question content is not interpreted.
func MatchesCode(p *Problem, code string) bool {
	return p != nil && ErrorCode(p.Outcome) == code && Public(*p) != nil
}
