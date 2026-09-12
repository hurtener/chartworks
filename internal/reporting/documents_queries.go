package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// DocumentQueries composes the existing NLQ service. Its evidence remains
// independent of block publication, certification and artifact access policy.
type DocumentQueries interface {
	DocumentQueryCatalog
	PrepareDocumentQuery(context.Context, identity.Envelope, QueryWidget, QueryOrigin, string, string) (nlqexec.SavedPlan, error)
	RunDocumentQuery(context.Context, identity.Envelope, QueryWidget, QueryOrigin, nlqexec.SavedPlan, int, int, bool) (nlqexec.SavedResult, error)
}

// DocumentQueryRecovery recovers a previously persisted ordinary plan without
// authorizing another model call after a durable generation-start marker.
type DocumentQueryRecovery interface {
	RecoverDocumentQuery(context.Context, identity.Envelope, QueryWidget, QueryOrigin, string) (nlqexec.SavedPlan, error)
}

type documentQueries struct{ service *nlqexec.Service }

// DocumentsFromQueries installs the actual query service, never a second planner.
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

// InspectDocumentQuery reads metadata only and never returns SQL or result rows.
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

// RecoverDocumentQuery cannot generate replacement SQL for missing evidence.
func (a documentQueries) RecoverDocumentQuery(ctx context.Context, e identity.Envelope, q QueryWidget, expected QueryOrigin, operation string) (nlqexec.SavedPlan, error) {
	evidence, err := a.evidence(ctx, e, q, expected)
	if err != nil {
		return nlqexec.SavedPlan{}, err
	}
	return a.service.RecoverSaved(ctx, e, savedQuestion(q), evidence, operation)
}

// RunDocumentQuery uses the ordinary validator/executor and actual read receipt.
func (a documentQueries) RunDocumentQuery(ctx context.Context, e identity.Envelope, q QueryWidget, expected QueryOrigin, plan nlqexec.SavedPlan, rows, bytes int, preview bool) (nlqexec.SavedResult, error) {
	evidence, err := a.evidence(ctx, e, q, expected)
	if err != nil {
		return nlqexec.SavedResult{}, err
	}
	return a.service.RunSaved(ctx, e, savedQuestion(q), evidence, plan, rows, bytes, preview)
}
