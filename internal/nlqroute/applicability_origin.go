package nlqroute

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// ApplicabilityEvidence is descriptive protected query metadata. It contains no
// question/term strings and cannot restore custody through JSON. Current routing
// and source admission remain mandatory after an authenticated origin read.
type ApplicabilityEvidence struct {
	Version          int                  `json:"version"`
	Query            string               `json:"query"`
	PriorQuery       string               `json:"prior_query,omitempty"`
	PriorDigest      string               `json:"prior_digest,omitempty"`
	ActorContext     string               `json:"actor_context"`
	AnswerContext    string               `json:"answer_context"`
	RequestDigest    string               `json:"request_digest"`
	ResolutionDigest string               `json:"resolution_digest"`
	RouteDigest      string               `json:"route_digest"`
	OriginalQuestion string               `json:"original_question_digest"`
	Pins             []ApplicabilityPin   `json:"pins"`
	Topics           []ApplicabilityTopic `json:"topics"`
}

// ApplicabilityPin is a tagged wire projection of the existing context pins.
// Its shape does not alter clarificationContext's established digest format.
type ApplicabilityPin struct {
	Topic         string           `json:"topic"`
	Version       string           `json:"version"`
	Digest        string           `json:"digest"`
	Revision      int64            `json:"revision"`
	RulesVersion  string           `json:"rules_version"`
	RulesDigest   string           `json:"rules_digest"`
	RulesRevision int64            `json:"rules_revision"`
	Sources       []topics.Binding `json:"sources"`
	BindingDigest string           `json:"binding_digest"`
}

func applicabilityPins(pins []clarificationTopicPin) []ApplicabilityPin {
	out := make([]ApplicabilityPin, 0, len(pins))
	for _, p := range pins {
		out = append(out, ApplicabilityPin{p.Topic, p.Version, p.Digest, p.Revision, p.RulesVersion, p.RulesDigest, p.RulesRevision, append([]topics.Binding(nil), p.Sources...), p.BindingDigest})
	}
	return out
}

type ApplicabilityTopic struct {
	Topic        string                             `json:"topic"`
	TopicVersion string                             `json:"topic_version"`
	PackDigest   string                             `json:"pack_digest"`
	RulesVersion string                             `json:"rules_version"`
	RulesDigest  string                             `json:"rules_digest"`
	Matches      []semantics.ClarificationTermMatch `json:"matches"`
}

func (ApplicabilityEvidence) String() string     { return "clarification-applicability(protected)" }
func (v ApplicabilityEvidence) GoString() string { return v.String() }

// ApplicabilityRecord is returned only by the configured authenticated repository
// reader. Public Route/Replay calls accept neither a reader nor a record payload.
type ApplicabilityRecord struct {
	Query, Tenant, Actor, Session, Context, Status string
	Revision                                       int64
	Route                                          RouteResult
}

type ApplicabilityReader interface {
	ReadClarificationApplicability(context.Context, identity.Envelope, string, string) (ApplicabilityRecord, error)
}

// WithApplicabilityReader returns a service-local composition. Only the trusted
// server constructor supplies this dependency; requests cannot configure it.
func (s *Service) WithApplicabilityReader(reader ApplicabilityReader) *Service {
	copy := *s
	copy.applicabilityReader = reader
	return &copy
}

type applicabilityKey struct{}
type applicabilityOrigin struct {
	evidence                  ApplicabilityEvidence
	actor, previous, expected string
	admitted                  bool
}

func applicabilityActor(tenant, actor, session string) string {
	return readexec.Hash([]string{tenant, actor, session})
}
func applicabilityRequest(in RouteRequest) string { return readexec.Hash(in) }

// The domain-separated commitment is descriptive until authenticated query
// custody is restored. It allows the exact original input, never mask equality.
func applicabilityOriginal(e identity.Envelope, in RouteRequest) string {
	return readexec.Hash([]any{"chartworks.clarification.original-question.v1", applicabilityActor(e.Tenant(), e.User(), e.Session()), in.Context, in.Locale, in.Question})
}

func hasHiddenApplicability(p *ApplicabilityEvidence) bool {
	for _, topic := range p.Topics {
		if len(topic.Matches) > 0 {
			return true
		}
	}
	return false
}
func applicabilityResolutions(r RouteResult) string {
	return readexec.Hash([]any{r.Request.Answers, r.Resolutions})
}
func applicabilityRoute(r RouteResult) string { r.Applicability = nil; return readexec.Hash(r) }
func applicabilitySeal(r RouteResult) string {
	return readexec.Hash([]any{r.Applicability, applicabilityRoute(r)})
}
func cloneApplicability(in *ApplicabilityEvidence) *ApplicabilityEvidence {
	if in == nil {
		return nil
	}
	raw, _ := json.Marshal(in)
	var out ApplicabilityEvidence
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return &out
}
func validApplicability(r RouteResult, tenant, actor, session string) bool {
	p := r.Applicability
	return p != nil && p.Version == 1 && p.OriginalQuestion != "" && len(p.Topics) > 0 && len(p.Topics) <= 4 && p.ActorContext == applicabilityActor(tenant, actor, session) && p.AnswerContext != "" && p.AnswerContext == r.AnswerContext && p.RequestDigest == applicabilityRequest(r.Request) && p.ResolutionDigest == applicabilityResolutions(r) && p.RouteDigest == applicabilityRoute(r)
}
func (r RouteResult) applicabilityProduced(e identity.Envelope) bool {
	return validApplicability(r, e.Tenant(), e.User(), e.Session()) && r.applicabilitySeal != "" && r.applicabilitySeal == applicabilitySeal(r)
}

// BindApplicabilityQuery binds an actually produced/replayed witness to its new
// immutable row identity. JSON-decoded evidence has no private producer seal.
func (r RouteResult) BindApplicabilityQuery(e identity.Envelope, query string) (RouteResult, error) {
	if r.Applicability == nil {
		return r, nil
	}
	if !e.Valid() || !identity.Identifier(query) || !r.applicabilityProduced(e) {
		return RouteResult{}, readexec.ErrBinding
	}
	r.Applicability = cloneApplicability(r.Applicability)
	r.Applicability.Query = query
	r.applicabilitySeal = applicabilitySeal(r)
	return r, nil
}

// ApplicabilityWriteValid is the guarded repository INSERT boundary. Descriptive
// fields without current private custody cannot be promoted by storage.
func (r RouteResult) ApplicabilityWriteValid(tenant, actor, session, query string) bool {
	if r.Applicability == nil {
		return true
	}
	return r.Applicability.Query == query && identity.Identifier(query) && validApplicability(r, tenant, actor, session) && r.applicabilitySeal != "" && r.applicabilitySeal == applicabilitySeal(r)
}

func (r *RouteResult) sealApplicability(e identity.Envelope, ctx context.Context, original RouteRequest) {
	if len(r.applicabilityTopics) == 0 {
		return
	}
	p := &ApplicabilityEvidence{OriginalQuestion: applicabilityOriginal(e, original), Version: 1, ActorContext: applicabilityActor(e.Tenant(), e.User(), e.Session()), AnswerContext: r.AnswerContext, RequestDigest: applicabilityRequest(r.Request), ResolutionDigest: applicabilityResolutions(*r), Pins: applicabilityPins(r.applicabilityPins), Topics: r.applicabilityTopics}
	if origin, ok := ctx.Value(applicabilityKey{}).(applicabilityOrigin); ok {
		p.OriginalQuestion = origin.evidence.OriginalQuestion
		p.PriorQuery = origin.evidence.Query
		p.PriorDigest = readexec.Hash(origin.evidence)
	}
	r.Applicability = p
	p.RouteDigest = applicabilityRoute(*r)
	r.applicabilitySeal = applicabilitySeal(*r)
}

func admitApplicabilityContext(ctx context.Context, e identity.Envelope, in RouteRequest) (context.Context, error) {
	origin, ok := ctx.Value(applicabilityKey{}).(applicabilityOrigin)
	if !ok {
		return ctx, nil
	}
	if origin.actor != applicabilityActor(e.Tenant(), e.User(), e.Session()) || origin.expected != applicabilityRequest(in) {
		return nil, readexec.ErrBinding
	}
	origin.admitted = true
	return context.WithValue(ctx, applicabilityKey{}, origin), nil
}

func replayApplicabilityContext(ctx context.Context, e identity.Envelope, previous RouteResult) (context.Context, error) {
	if previous.Applicability == nil {
		return ctx, nil
	}
	if !validApplicability(previous, e.Tenant(), e.User(), e.Session()) {
		return nil, readexec.ErrBinding
	}
	origin, ok := ctx.Value(applicabilityKey{}).(applicabilityOrigin)
	if previous.applicabilityProduced(e) {
		origin = applicabilityOrigin{evidence: *cloneApplicability(previous.Applicability), actor: applicabilityActor(e.Tenant(), e.User(), e.Session()), previous: readexec.Hash(previous), expected: applicabilityRequest(previous.Request)}
	} else if !ok || origin.previous != readexec.Hash(previous) || readexec.Hash(origin.evidence) != readexec.Hash(previous.Applicability) {
		// A fingerprint alone supplies no missing semantics. Ordinary serialized
		// replay can independently reconstruct public applicability, but gets no
		// custody for an original-question transition or a new protected write.
		if !ok && !hasHiddenApplicability(previous.Applicability) {
			return ctx, nil
		}
		return nil, readexec.ErrBinding
	}
	origin.expected = applicabilityRequest(previous.Request)
	ctx = context.WithValue(ctx, applicabilityKey{}, origin)
	return admitApplicabilityContext(ctx, e, previous.Request)
}

// WithQueryApplicability loads the exact protected record through the configured
// reader, replays it under current authority, then binds custody to one exact
// server-approved transition. It never accepts a caller-supplied match list.
func (s *Service) WithQueryApplicability(ctx context.Context, e identity.Envelope, query string, previous RouteResult, next RouteRequest, action, transition string) (context.Context, error) {
	if previous.Applicability == nil {
		return ctx, nil
	}
	if ctx == nil || !e.Valid() {
		return nil, access.ErrUnauthenticated
	}
	if s.applicabilityReader == nil || !identity.Identifier(query) {
		return nil, readexec.ErrBinding
	}
	if action != "query.preflight" && action != "query.plan" && action != "query.execute" {
		return nil, readexec.ErrBinding
	}
	record, err := s.applicabilityReader.ReadClarificationApplicability(ctx, e, query, action)
	if err != nil {
		return nil, err
	}
	if record.Query != query || record.Tenant != e.Tenant() || record.Actor != e.User() || record.Session != e.Session() || record.Revision < 1 || record.Context != previous.Request.Context || readexec.Hash(record.Route) != readexec.Hash(previous) || !validApplicability(record.Route, e.Tenant(), e.User(), e.Session()) || record.Route.Applicability.Query != query {
		return nil, readexec.ErrBinding
	}
	switch transition {
	case "replay", "saved_copy", "pending":
		if applicabilityRequest(next) != applicabilityRequest(previous.Request) {
			return nil, readexec.ErrBinding
		}
	case "reply", "refine":
		if next.Question != previous.Request.Question && applicabilityOriginal(e, next) != record.Route.Applicability.OriginalQuestion {
			return nil, clarificationFailure(next.Locale, "clarification_query", "clarification_question_mismatch")
		}
		if transition == "reply" && record.Status != "preflight" || next.Context != previous.Request.Context || next.Locale != previous.Request.Locale || next.AnswerContext != "" && next.AnswerContext != previous.AnswerContext {
			return nil, readexec.ErrBinding
		}
	default:
		return nil, readexec.ErrBinding
	}
	origin := applicabilityOrigin{evidence: *cloneApplicability(record.Route.Applicability), actor: applicabilityActor(e.Tenant(), e.User(), e.Session()), previous: readexec.Hash(previous), expected: applicabilityRequest(previous.Request)}
	replayContext := context.WithValue(ctx, applicabilityKey{}, origin)
	if _, _, err = s.ReplayClarifications(replayContext, e, previous); err != nil {
		return nil, err
	}
	origin.expected = applicabilityRequest(next)
	return context.WithValue(ctx, applicabilityKey{}, origin), nil
}

// ReissueApplicabilityForCopy is restricted to an authenticated exact stored
// parent and a successful current replay. The copy still needs a new row binding.
func (s *Service) ReissueApplicabilityForCopy(ctx context.Context, e identity.Envelope, query string, previous RouteResult) (RouteResult, error) {
	return s.reissueApplicability(ctx, e, query, previous, "saved_copy")
}

// ReissueApplicabilityForPending replays the actual immutable SQL parent before
// a correction decision can produce a new protected pending row.
func (s *Service) ReissueApplicabilityForPending(ctx context.Context, e identity.Envelope, query string, previous RouteResult) (RouteResult, error) {
	return s.reissueApplicability(ctx, e, query, previous, "pending")
}
func (s *Service) reissueApplicability(ctx context.Context, e identity.Envelope, query string, previous RouteResult, transition string) (RouteResult, error) {
	if previous.Applicability == nil {
		return previous, nil
	}
	if _, err := s.WithQueryApplicability(ctx, e, query, previous, previous.Request, "query.execute", transition); err != nil {
		return RouteResult{}, err
	}
	previous.Applicability = cloneApplicability(previous.Applicability)
	previous.Applicability.PriorDigest = readexec.Hash(previous.Applicability)
	previous.Applicability.PriorQuery = query
	previous.applicabilitySeal = applicabilitySeal(previous)
	return previous, nil
}

func applicabilityForTopic(ctx context.Context, item admittedTopic) ([]semantics.ClarificationTermMatch, error) {
	origin, ok := ctx.Value(applicabilityKey{}).(applicabilityOrigin)
	if !ok {
		return nil, nil
	}
	if !origin.admitted {
		return nil, readexec.ErrBinding
	}
	for _, topic := range origin.evidence.Topics {
		if topic.Topic != item.id {
			continue
		}
		if topic.TopicVersion != item.publication.State.Version || topic.PackDigest != item.publication.Digest || topic.RulesVersion != item.rules.State.Version || topic.RulesDigest != item.rules.Digest {
			return nil, readexec.ErrBinding
		}
		return append([]semantics.ClarificationTermMatch(nil), topic.Matches...), nil
	}
	return nil, nil
}

func captureApplicability(ctx context.Context, in RouteRequest, admitted []admittedTopic, result *RouteResult) error {
	origin, carried := ctx.Value(applicabilityKey{}).(applicabilityOrigin)
	pins := clarificationPins(admitted)
	if carried && (!origin.admitted || origin.evidence.AnswerContext != result.AnswerContext || readexec.Hash(origin.evidence.Pins) != readexec.Hash(applicabilityPins(pins))) {
		return readexec.ErrBinding
	}
	result.applicabilityTopics = nil
	for _, item := range admitted {
		if !item.hasRules {
			continue
		}
		matches := semantics.MatchClarificationTerms(item.rules.Definition, in.Question)
		seed, err := applicabilityForTopic(ctx, item)
		if err != nil {
			return err
		}
		for _, m := range seed {
			found := false
			for i, x := range matches {
				if x.Pattern == m.Pattern {
					found = true
					if m.Specificity > x.Specificity {
						matches[i] = m
					}
					break
				}
			}
			if !found {
				matches = append(matches, m)
			}
		}
		visible := semantics.MatchClarificationTerms(item.rules.Definition, result.Request.Question)
		var hidden []semantics.ClarificationTermMatch
		for _, m := range matches {
			score := 0
			for _, v := range visible {
				if v.Pattern == m.Pattern {
					score = v.Specificity
				}
			}
			if m.Specificity > score {
				hidden = append(hidden, m)
			}
		}
		if len(hidden) > 0 || in.Question != result.Request.Question || carried {
			result.applicabilityTopics = append(result.applicabilityTopics, ApplicabilityTopic{Topic: item.id, TopicVersion: item.publication.State.Version, PackDigest: item.publication.Digest, RulesVersion: item.rules.State.Version, RulesDigest: item.rules.Digest, Matches: hidden})
		}
	}
	result.applicabilityPins = pins
	return nil
}
