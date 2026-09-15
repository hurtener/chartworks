package nlqexec

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/store"
)

// SavedTopic pins an immutable published semantic definition, not certified SQL.
type SavedTopic struct {
	Topic   string `json:"topic"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// SavedQuestion distinguishes replayable questions from private session references.
type SavedQuestion struct {
	Durability string           `json:"durability"`
	Context    string           `json:"context"`
	Topics     []SavedTopic     `json:"topics"`
	Question   string           `json:"question,omitempty"`
	Query      string           `json:"query,omitempty"`
	Selections *SavedSelections `json:"selections,omitempty"`
}

// SavedEvidence is metadata-only. It grants no authority over the named source.
type SavedEvidence struct {
	Query          string       `json:"query,omitempty"`
	Actor          string       `json:"actor,omitempty"`
	Session        string       `json:"session,omitempty"`
	Source         string       `json:"source"`
	Context        string       `json:"context"`
	Topics         []SavedTopic `json:"topics"`
	Datasets       []string     `json:"datasets"`
	SemanticDigest string       `json:"semantic_digest"`
	QueryDigest    string       `json:"query_digest"`
}

// SavedQueryReader projects protected definitions without fetching raw results.
type SavedQueryReader interface {
	ReadSavedQuery(context.Context, identity.Envelope, string, bool) (QueryRecord, error)
}

// SavedAttemptReader reads the actual source journal when a terminal NLQ result
// is replayed. It cannot manufacture a successful attempt from retained rows.
type SavedAttemptReader interface {
	ReadSavedAttempt(context.Context, identity.Envelope, string) (exec.Attempt, error)
}

// SavedPlan identifies an ordinary persisted plan, never a serialized credential.
type SavedPlan struct {
	Query         string `json:"query"`
	Operation     string `json:"operation"`
	InputDigest   string `json:"input_digest"`
	QueryDigest   string `json:"query_digest"`
	BindingDigest string `json:"binding_digest"`
}

// SavedResult retains the real read receipt and actual gateway usage. A dynamic
// query never acquires a block certificate from an enclosing report publication.
type SavedResult struct {
	Query          string               `json:"query"`
	QueryDigest    string               `json:"query_digest"`
	SemanticDigest string               `json:"semantic_digest"`
	Partition      string               `json:"partition_digest"`
	Execution      exec.ExecutionReport `json:"execution"`
	Receipt        gateway.Receipt      `json:"receipt"`
	EvidenceStale  bool                 `json:"evidence_stale"`
}

func savedQuestionValid(q SavedQuestion) bool {
	if !identity.Identifier(q.Context) || len(q.Topics) < 1 || len(q.Topics) > 4 || !savedSelectionsValid(q) {
		return false
	}
	seen := map[string]bool{}
	for _, pin := range q.Topics {
		if !identity.Identifier(pin.Topic) || !identity.Identifier(pin.Version) || len(pin.Digest) != 64 || strings.Trim(pin.Digest, "0123456789abcdef") != "" || seen[pin.Topic] {
			return false
		}
		seen[pin.Topic] = true
	}
	return q.Durability == "replayable" && q.Query == "" && strings.TrimSpace(q.Question) != "" && len(q.Question) <= 4096 && utf8.ValidString(q.Question) && !strings.ContainsRune(q.Question, 0) ||
		q.Durability == "session_bound" && q.Question == "" && identity.Identifier(q.Query)
}

func savedQueryDigest(q QueryRecord) string {
	return exec.Hash([]any{"saved-query-definition-v1", q.SQL, q.Parameters, q.Context, q.Topics, q.TopicVersions})
}

func savedRecordMatches(q QueryRecord, in SavedQuestion) bool {
	if q.Context != in.Context || len(q.Topics) != len(in.Topics) || len(q.TopicVersions) != len(in.Topics) || strings.TrimSpace(q.SQL) == "" || !savedSelectionsMatch(q, in) {
		return false
	}
	for i, pin := range in.Topics {
		if q.Topics[i] != pin.Topic || q.TopicVersions[i] != pin.Version {
			return false
		}
	}
	return true
}

func (s *Service) savedRecord(ctx context.Context, e identity.Envelope, id string, operation bool) (QueryRecord, error) {
	r, ok := s.repo.(SavedQueryReader)
	if !ok {
		return QueryRecord{}, store.ErrUnavailable
	}
	return r.ReadSavedQuery(ctx, e, id, operation)
}

// InspectSaved checks exact retained semantics and signed query reach without
// opening a warehouse connection or invoking a model provider.
func (s *Service) InspectSaved(ctx context.Context, e identity.Envelope, in SavedQuestion) (SavedEvidence, error) {
	if s == nil || ctx == nil || !savedQuestionValid(in) {
		return SavedEvidence{}, ErrInvalid
	}
	if err := access.Require(e, "query.execute", access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: in.Context}); err != nil {
		return SavedEvidence{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	out := SavedEvidence{Context: in.Context, Topics: append([]SavedTopic(nil), in.Topics...), Datasets: []string{}, SemanticDigest: exec.Hash(in.Topics), QueryDigest: exec.Hash(in)}
	seen := map[string]bool{}
	for _, pin := range in.Topics {
		if err := access.Require(e, "query.execute", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: pin.Topic}); err != nil {
			return SavedEvidence{}, err
		}
		contract, err := s.topics.RetainedContract(ctx, e, pin.Topic, pin.Version)
		if err != nil {
			return SavedEvidence{}, err
		}
		p := contract.Publication
		if p.Digest != pin.Digest || p.State.Archived || p.Definition.Topic != pin.Topic || p.Definition.Version != pin.Version {
			return SavedEvidence{}, ErrNoPlan
		}
		for _, dataset := range p.Definition.Datasets {
			binding := dataset.Source
			if binding.Context != in.Context || out.Source != "" && out.Source != binding.Source {
				return SavedEvidence{}, ErrInvalid
			}
			out.Source = binding.Source
			if err := access.Require(e, "query.execute",
				access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: binding.Source},
				access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset.ID}); err != nil {
				return SavedEvidence{}, err
			}
			if !seen[dataset.ID] {
				seen[dataset.ID] = true
				out.Datasets = append(out.Datasets, dataset.ID)
			}
		}
	}
	if out.Source == "" || len(out.Datasets) == 0 || len(out.Datasets) > 32 {
		return SavedEvidence{}, ErrInvalid
	}
	slices.Sort(out.Datasets)
	if in.Durability == "session_bound" {
		q, err := s.savedRecord(ctx, e, in.Query, false)
		if err != nil {
			return SavedEvidence{}, err
		}
		if q.Session != e.Session() || !savedRecordMatches(q, in) || q.EvidenceStale || !slices.Contains([]string{"planned", "succeeded", "empty", "truncated"}, q.Status) {
			return SavedEvidence{}, ErrNoPlan
		}
		out.Query, out.Actor, out.Session, out.QueryDigest = in.Query, e.User(), e.Session(), savedQueryDigest(q)
	}
	if !e.Valid() {
		return SavedEvidence{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}

func (s *Service) savedPlan(ctx context.Context, e identity.Envelope, in SavedQuestion, evidence SavedEvidence, q QueryRecord, operation string) (SavedPlan, error) {
	if q.Session != e.Session() || q.Operation != operation || !savedRecordMatches(q, in) || in.Durability == "replayable" && q.Question != in.Question || in.Durability == "session_bound" && q.Parent != in.Query {
		return SavedPlan{}, store.ErrConflict
	}
	// Only the ordinary execution service may have made a validated equivalent
	// correction after planning. Nonterminal session clones must remain exact.
	if in.Durability == "session_bound" && !terminalQueryStatus(q.Status) && savedQueryDigest(q) != evidence.QueryDigest {
		return SavedPlan{}, ErrNoPlan
	}
	binding, err := s.sources.Binding(ctx, e, evidence.Source, evidence.Context)
	if err != nil {
		return SavedPlan{}, err
	}
	return SavedPlan{Query: q.ID, Operation: operation, InputDigest: exec.Hash([]any{in, evidence}), QueryDigest: savedQueryDigest(q), BindingDigest: exec.Hash(binding)}, nil
}

// RecoverSaved is metadata-only recovery after a durable pre-generation marker.
// Missing plan evidence remains uncertain; it never triggers fresh generation.
func (s *Service) RecoverSaved(ctx context.Context, e identity.Envelope, in SavedQuestion, expected SavedEvidence, operation string) (SavedPlan, error) {
	if !identity.Identifier(operation) {
		return SavedPlan{}, ErrInvalid
	}
	evidence, err := s.InspectSaved(ctx, e, in)
	if err != nil || exec.Hash(evidence) != exec.Hash(expected) {
		if err == nil {
			err = ErrNoPlan
		}
		return SavedPlan{}, err
	}
	q, err := s.savedRecord(ctx, e, operation, true)
	if err != nil {
		return SavedPlan{}, err
	}
	return s.savedPlan(ctx, e, in, evidence, q, operation)
}

// PrepareSaved uses the existing NLQ planner or copies an exact session-bound
// definition into a new operation. The caller reserves its generation once first.
func (s *Service) PrepareSaved(ctx context.Context, e identity.Envelope, in SavedQuestion, expected SavedEvidence, operation, locale string) (SavedPlan, error) {
	if !identity.Identifier(operation) {
		return SavedPlan{}, ErrInvalid
	}
	evidence, err := s.InspectSaved(ctx, e, in)
	if err != nil {
		return SavedPlan{}, err
	}
	if exec.Hash(evidence) != exec.Hash(expected) {
		return SavedPlan{}, ErrNoPlan
	}
	if previous, err := s.savedRecord(ctx, e, operation, true); err == nil {
		return s.savedPlan(ctx, e, in, evidence, previous, operation)
	} else if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		return SavedPlan{}, err
	}
	if in.Durability == "session_bound" {
		original, err := s.savedRecord(ctx, e, in.Query, false)
		if err != nil {
			return SavedPlan{}, err
		}
		if original.Session != e.Session() || savedQueryDigest(original) != evidence.QueryDigest {
			return SavedPlan{}, ErrForeignSession
		}
		id, err := newID()
		if err != nil {
			return SavedPlan{}, err
		}
		sc, err := scope(e)
		if err != nil {
			return SavedPlan{}, err
		}
		original.ID, original.Parent, original.Operation = id, in.Query, operation
		original.Status, original.Result, original.Revision = "planned", nil, 1
		original.Created, original.Updated, original.ExecutionFixes = time.Now().UTC(), time.Now().UTC(), 0
		if err := s.repo.CreateQuery(ctx, sc, original); err != nil && !errors.Is(err, store.ErrConflict) {
			return SavedPlan{}, err
		}
	} else {
		for _, pin := range in.Topics {
			current, err := s.topics.Contract(ctx, e, pin.Topic)
			if err != nil {
				return SavedPlan{}, err
			}
			if current.Publication.Digest != pin.Digest || current.Publication.Definition.Version != pin.Version {
				return SavedPlan{}, ErrNoPlan
			}
		}
		language := nlq.LanguageEnglish
		if strings.HasPrefix(locale, "es") {
			language = nlq.LanguageSpanish
		} else if !strings.HasPrefix(locale, "en") {
			return SavedPlan{}, ErrInvalid
		}
		result, err := s.Plan(ctx, e, PlanRequest{Operation: operation, QuestionRequest: savedRouting(in, language)})
		if err != nil {
			return SavedPlan{}, err
		}
		if result.Status != "planned" {
			return SavedPlan{}, ErrNoPlan
		}
	}
	// Post-plan pin checks catch routing/publication races before source execution.
	q, err := s.savedRecord(ctx, e, operation, true)
	if err != nil {
		return SavedPlan{}, err
	}
	return s.savedPlan(ctx, e, in, evidence, q, operation)
}

// RunSaved delegates all SQL validation, execution and equivalent-correction
// bounds to Service.Run. Terminal replay recovers the real read-journal receipt.
func (s *Service) RunSaved(ctx context.Context, e identity.Envelope, in SavedQuestion, expected SavedEvidence, plan SavedPlan, rows, bytes int, preview bool) (SavedResult, error) {
	if plan.InputDigest != exec.Hash([]any{in, expected}) || !identity.Identifier(plan.Query) || !identity.Identifier(plan.Operation) || rows < 1 || rows > 100000 || bytes < 128 || bytes > 16<<20 {
		return SavedResult{}, ErrInvalid
	}
	evidence, err := s.InspectSaved(ctx, e, in)
	if err != nil || exec.Hash(evidence) != exec.Hash(expected) {
		if err == nil {
			err = ErrNoPlan
		}
		return SavedResult{}, err
	}
	q, err := s.savedRecord(ctx, e, plan.Query, false)
	if err != nil {
		return SavedResult{}, err
	}
	checked, err := s.savedPlan(ctx, e, in, evidence, q, plan.Operation)
	if err != nil {
		return SavedResult{}, err
	}
	if checked != plan {
		// A terminal ordinary query may retain a mechanically equivalent
		// correction. Nothing is reexecuted; identity, pins and partition must
		// still match the originally sealed plan and current signed authority.
		if !terminalQueryStatus(q.Status) || checked.Query != plan.Query || checked.Operation != plan.Operation || checked.InputDigest != plan.InputDigest || checked.BindingDigest != plan.BindingDigest {
			return SavedResult{}, ErrNoPlan
		}
	}
	run, err := s.Run(ctx, e, RunRequest{QueryID: plan.Query, Operation: plan.Operation, Rows: rows, Bytes: bytes, Preview: preview})
	if err != nil {
		return SavedResult{}, err
	}
	binding, err := s.sources.Binding(ctx, e, evidence.Source, evidence.Context)
	if err != nil || exec.Hash(binding) != plan.BindingDigest {
		if err == nil {
			err = ErrNoPlan
		}
		return SavedResult{}, err
	}
	actual, err := s.savedRecord(ctx, e, plan.Query, false)
	if err != nil {
		return SavedResult{}, err
	}
	if actual.Session != e.Session() || !savedRecordMatches(actual, in) || !e.Valid() {
		return SavedResult{}, ErrForeignSession
	}
	if run.Execution.Attempt.ID == "" {
		journal, ok := s.repo.(SavedAttemptReader)
		if !ok {
			return SavedResult{}, store.ErrUnavailable
		}
		run.Execution.Attempt, err = journal.ReadSavedAttempt(ctx, e, plan.Operation)
		if err != nil {
			return SavedResult{}, err
		}
	}
	a := run.Execution.Attempt
	if a.Manifest.Operation != plan.Operation || a.Manifest.Session != e.Session() || a.Manifest.Preview != preview || a.Manifest.Receipt.Source != evidence.Source || a.Manifest.Receipt.Context != evidence.Context || a.RemoteState != "stopped" || a.Finished == nil || run.Execution.Result == nil {
		return SavedResult{}, ErrNoPlan
	}
	return SavedResult{Query: plan.Query, QueryDigest: savedQueryDigest(actual), SemanticDigest: evidence.SemanticDigest, Partition: plan.BindingDigest, Execution: run.Execution, Receipt: actual.Receipt, EvidenceStale: run.EvidenceStale}, nil
}
