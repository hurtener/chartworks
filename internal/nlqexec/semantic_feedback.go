package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// ErrFeedbackRetention identifies erased proposal origin evidence.
var ErrFeedbackRetention = errors.New("nlqexec: semantic proposal origin was erased")

// SemanticFeedbackOrigin is content-free provenance for an explicit authoring
// request. Historical SQL, questions, notes, parameters and rows never cross it.
type SemanticFeedbackOrigin struct {
	QueryRevision       int64  `json:"query_revision"`
	FeedbackID          string `json:"feedback_id"`
	QueryID             string `json:"query_id"`
	Topic               string `json:"topic"`
	TopicVersion        string `json:"topic_version"`
	PublicationDigest   string `json:"publication_digest"`
	SourceBindingDigest string `json:"source_binding_digest"`
	EvidenceDigest      string `json:"evidence_digest"`
	Verdict             string `json:"verdict"`
	Corrected           bool   `json:"corrected"`
}

// SemanticFeedback is an ephemeral service-issued admission, not JSON authority.
type SemanticFeedback struct {
	origin                 SemanticFeedbackOrigin
	definition             topics.Definition
	tenant, actor, session string
	deadline               time.Time
}

// Inspect releases only current admitted metadata to the original authority.
func (p SemanticFeedback) Inspect(e identity.Envelope) (SemanticFeedbackOrigin, topics.Definition, error) {
	if !e.Valid() || p.origin.EvidenceDigest == "" || p.tenant != e.Tenant() || p.actor != e.User() || p.session != e.Session() || !time.Now().Before(p.deadline) {
		return SemanticFeedbackOrigin{}, topics.Definition{}, access.ErrUnauthenticated
	}
	// The definition is immutable service data; callers must not gain authority
	// from its contents. Requiring the same envelope remains mandatory.
	raw, err := json.Marshal(p.definition)
	if err != nil {
		return SemanticFeedbackOrigin{}, topics.Definition{}, store.ErrInvalid
	}
	var definition topics.Definition
	if json.Unmarshal(raw, &definition) != nil {
		return SemanticFeedbackOrigin{}, topics.Definition{}, store.ErrInvalid
	}
	return p.origin, definition, nil
}

type semanticFeedbackReader interface {
	ReadSemanticFeedback(context.Context, identity.Envelope, string) (FeedbackRecord, error)
}

// SemanticFeedbackEvidence reauthorizes a retained correction or failed-query
// review against the current publication and source without executing its SQL.
func (s *Service) SemanticFeedbackEvidence(ctx context.Context, e identity.Envelope, id string) (SemanticFeedback, error) {
	if ctx == nil || !identity.Identifier(id) {
		return SemanticFeedback{}, ErrInvalid
	}
	if !e.Has("feedback.write") || !e.Has("topics.write") {
		return SemanticFeedback{}, access.ErrForbidden
	}
	reader, ok := s.repo.(semanticFeedbackReader)
	if !ok {
		return SemanticFeedback{}, store.ErrUnavailable
	}
	f, err := reader.ReadSemanticFeedback(ctx, e, id)
	if err != nil {
		return SemanticFeedback{}, err
	}
	sc, err := scope(e)
	if err != nil {
		return SemanticFeedback{}, err
	}
	q, err := s.repo.ReadQuery(ctx, sc, f.QueryID)
	if err != nil {
		return SemanticFeedback{}, err
	}
	if f.ID != id || f.Session != e.Session() || q.Session != e.Session() {
		return SemanticFeedback{}, ErrForeignSession
	}
	if f.Verdict != "negative" && f.Correction == "" {
		return SemanticFeedback{}, ErrInvalid
	}
	if len(q.Topics) != 1 || len(q.TopicVersions) != 1 || q.Topics[0] != q.Topic {
		return SemanticFeedback{}, exec.ErrBinding
	}
	if err = access.Require(e, "topics.write", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "write", ID: q.Topic}); err != nil {
		return SemanticFeedback{}, err
	}
	a, err := s.currentAdmission(ctx, e, q)
	if err != nil {
		return SemanticFeedback{}, err
	}
	if err = gatewayRequirement(e, "feedback.write", a.resources); err != nil {
		return SemanticFeedback{}, err
	}
	digest := exec.Hash(a.binding)
	if q.Route.SourceBindingDigest != "" && q.Route.SourceBindingDigest != digest || len(a.publications) != 1 {
		return SemanticFeedback{}, exec.ErrBinding
	}
	origin := SemanticFeedbackOrigin{QueryRevision: q.Revision, FeedbackID: f.ID, QueryID: q.ID, Topic: q.Topic, TopicVersion: q.TopicVersions[0], PublicationDigest: a.publications[0].Digest, SourceBindingDigest: digest, Verdict: f.Verdict, Corrected: f.Correction != ""}
	origin.EvidenceDigest = exec.Hash([]any{f.ID, f.QueryID, f.Verdict, f.Correction, f.Note, f.Created, q.Revision, q.TopicVersions, q.RuleVersions, q.SQL, q.Parameters, digest})
	return SemanticFeedback{origin: origin, definition: a.publications[0].Definition, tenant: e.Tenant(), actor: e.User(), session: e.Session(), deadline: e.Deadline()}, nil
}

// SemanticFeedbackReference identifies selectable owned evidence without
// exposing historical question, correction SQL, note, parameters or results.
type SemanticFeedbackReference struct {
	ID        string    `json:"id"`
	Verdict   string    `json:"verdict"`
	Corrected bool      `json:"corrected"`
	Created   time.Time `json:"created_at"`
}
type semanticFeedbackLister interface {
	ListSemanticFeedback(context.Context, identity.Envelope, string) ([]SemanticFeedbackReference, error)
}

// SemanticFeedbackReferences supplies discoverable exact evidence IDs under
// current topic/source admission. A SQL plan is neither executed nor required.
func (s *Service) SemanticFeedbackReferences(ctx context.Context, e identity.Envelope, queryID string) ([]SemanticFeedbackReference, error) {
	if ctx == nil || !identity.Identifier(queryID) {
		return nil, ErrInvalid
	}
	if !e.Has("feedback.write") || !e.Has("topics.write") {
		return nil, access.ErrForbidden
	}
	sc, err := scope(e)
	if err != nil {
		return nil, err
	}
	q, err := s.repo.ReadQuery(ctx, sc, queryID)
	if err != nil {
		return nil, err
	}
	if q.Session != e.Session() {
		return nil, ErrForeignSession
	}
	if err = access.Require(e, "topics.write", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "write", ID: q.Topic}); err != nil {
		return nil, err
	}
	a, err := s.currentAdmission(ctx, e, q)
	if err != nil {
		return nil, err
	}
	if err = gatewayRequirement(e, "feedback.write", a.resources); err != nil {
		return nil, err
	}
	reader, ok := s.repo.(semanticFeedbackLister)
	if !ok {
		return nil, store.ErrUnavailable
	}
	return reader.ListSemanticFeedback(ctx, e, queryID)
}
