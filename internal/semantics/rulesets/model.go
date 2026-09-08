// Package rulesets owns reviewed rule lifecycle state and deterministic hard-
// constraint evaluation for published semantic topics.
package rulesets

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

type Draft struct {
	Topic      string                      `json:"topic"`
	Revision   int64                       `json:"revision"`
	Digest     string                      `json:"digest"`
	Definition semantics.RuleSetDefinition `json:"definition"`
	Change     string                      `json:"change"`
	CreatedAt  time.Time                   `json:"created_at"`
}

type SaveRequest struct {
	Expected   int64                       `json:"expected_revision"`
	Definition semantics.RuleSetDefinition `json:"definition"`
	Change     string                      `json:"change"`
}

type ReviewRequest struct {
	DraftRevision int64  `json:"draft_revision"`
	Digest        string `json:"digest"`
	Decision      string `json:"decision"`
	Note          string `json:"note"`
}

type Review struct {
	ID            string    `json:"id"`
	Topic         string    `json:"topic"`
	DraftRevision int64     `json:"draft_revision"`
	Digest        string    `json:"digest"`
	Decision      string    `json:"decision"`
	Note          string    `json:"note"`
	CreatedAt     time.Time `json:"created_at"`
}

type PublishRequest struct {
	Review   string `json:"review"`
	Expected int64  `json:"expected_revision"`
}

type State struct {
	Topic    string `json:"topic"`
	Revision int64  `json:"revision"`
	Version  string `json:"version"`
	Active   bool   `json:"active"`
	Retired  bool   `json:"retired"`
}

type Published struct {
	State       State                       `json:"state"`
	Definition  semantics.RuleSetDefinition `json:"definition"`
	Digest      string                      `json:"digest"`
	PublishedAt time.Time                   `json:"published_at"`
}

type RetireRequest struct {
	Expected int64  `json:"expected_revision"`
	Note     string `json:"note"`
}

type EvaluateRequest struct {
	References []semantics.Reference `json:"references"`
}

type Evaluation struct {
	Topic        string                         `json:"topic"`
	TopicVersion string                         `json:"topic_version"`
	RuleVersion  string                         `json:"rule_version"`
	RuleDigest   string                         `json:"rule_digest"`
	Result       semantics.ConstraintEvaluation `json:"result"`
	EvaluatedAt  time.Time                      `json:"evaluated_at"`
}

type Pin struct {
	RuleVersion  string
	TopicVersion string
	PackDigest   string
}

func digestValid(value string) bool {
	b, err := hex.DecodeString(value)
	return err == nil && len(b) == 32 && strings.ToLower(value) == value
}

func textValid(value string) bool {
	return len(value) >= 1 && len(value) <= 1024 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

type Repository interface {
	SaveRuleDraft(context.Context, identity.Envelope, topics.Published, semantics.RuleModel, int64, string) (Draft, error)
	ReviewRuleDraft(context.Context, identity.Envelope, topics.Published, string, ReviewRequest) (Review, error)
	PublishRules(context.Context, identity.Envelope, topics.Published, string, int64) (Published, error)
	RuleVersionPin(context.Context, identity.Envelope, string, string, drafts.Access) (Pin, error)
	ReadPublishedRules(context.Context, identity.Envelope, string, string, drafts.Access, bool) (Published, error)
	RetireRules(context.Context, identity.Envelope, topics.Published, string, int64) (State, error)
}

type TopicRepository interface {
	ReadPublishedTopic(context.Context, identity.Envelope, string, string, drafts.Access) (topics.Published, error)
}
