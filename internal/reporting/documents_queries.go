package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// DocumentQueries composes the existing NLQ service. Its own retained evidence
// is independent of block publication and certification.
type DocumentQueries interface {
	DocumentQueryCatalog
	PrepareDocumentQuery(context.Context, identity.Envelope, QueryWidget, QueryOrigin, string, string) (nlqexec.SavedPlan, error)
	RunDocumentQuery(context.Context, identity.Envelope, QueryWidget, QueryOrigin, nlqexec.SavedPlan, int, int) (nlqexec.SavedResult, error)
}

type documentQueries struct{ service *nlqexec.Service }

// DocumentsFromQueries installs the actual query service, not another planner.
func DocumentsFromQueries(service *nlqexec.Service) DocumentQueries {
	if service == nil {
		return nil
	}
	return documentQueries{service: service}
}

func savedQuestion(q QueryWidget) nlqexec.SavedQuestion {
	out := nlqexec.SavedQuestion{Durability: q.Durability, Context: q.Context, Question: q.Question, Query: q.Query, Topics: []nlqexec.SavedTopic{}}
	for _, pin := range q.Topics {
		out.Topics = append(out.Topics, nlqexec.SavedTopic{Topic: pin.Topic, Version: pin.Version, Digest: pin.Digest})
	}
	return out
}

func queryOrigin(e nlqexec.SavedEvidence) QueryOrigin {
	out := QueryOrigin{Query: e.Query, Actor: e.Actor, Session: e.Session, Source: e.Source, Context: e.Context, Topics: []TopicPin{}, SemanticDigest: e.SemanticDigest, QueryDigest: e.QueryDigest}
	for _, pin := range e.Topics {
		out.Topics = append(out.Topics, TopicPin{Topic: pin.Topic, Version: pin.Version, Digest: pin.Digest})
	}
	return out
}

// InspectDocumentQuery is metadata-only and never returns query SQL or rows.
func (a documentQueries) InspectDocumentQuery(ctx context.Context, e identity.Envelope, q QueryWidget) (QueryOrigin, error) {
	out, err := a.service.InspectSaved(ctx, e, savedQuestion(q))
	if err != nil {
		return QueryOrigin{}, err
	}
	return queryOrigin(out), nil
}

func (a documentQueries) evidence(ctx context.Context, e identity.Envelope, q QueryWidget, expected QueryOrigin) (nlqexec.SavedEvidence, error) {
	actual, err := a.service.InspectSaved(ctx, e, savedQuestion(q))
	if err != nil {
		return nlqexec.SavedEvidence{}, err
	}
	origin := queryOrigin(actual)
	origin.Widget = expected.Widget
	if digest(origin) != digest(expected) {
		return nlqexec.SavedEvidence{}, ErrStale
	}
	return actual, nil
}

// PrepareDocumentQuery delegates bounded generation or exact session cloning.
func (a documentQueries) PrepareDocumentQuery(ctx context.Context, e identity.Envelope, q QueryWidget, expected QueryOrigin, operation, locale string) (nlqexec.SavedPlan, error) {
	evidence, err := a.evidence(ctx, e, q, expected)
	if err != nil {
		return nlqexec.SavedPlan{}, err
	}
	return a.service.PrepareSaved(ctx, e, savedQuestion(q), evidence, operation, locale)
}

// RunDocumentQuery uses the ordinary validator/executor and returns its receipt.
func (a documentQueries) RunDocumentQuery(ctx context.Context, e identity.Envelope, q QueryWidget, expected QueryOrigin, plan nlqexec.SavedPlan, rows, bytes int) (nlqexec.SavedResult, error) {
	evidence, err := a.evidence(ctx, e, q, expected)
	if err != nil {
		return nlqexec.SavedResult{}, err
	}
	return a.service.RunSaved(ctx, e, savedQuestion(q), evidence, plan, rows, bytes)
}
