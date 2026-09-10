package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// CaptureFromQueries supplies the existing query service's private, source-backed
// authoring handoff. The adapter performs no model or warehouse work of its own.
func CaptureFromQueries(query *nlqexec.Service) QueryCapture {
	if query == nil {
		return nil
	}
	return queryBlockCapture{query: query}
}

// queryBlockCapture translates a private existing-query handoff. No transport
// accepts SQL, schema, template provenance or topic digests in place of it.
type queryBlockCapture struct{ query *nlqexec.Service }

// Capture delegates to the owning private-query handoff without generating or executing SQL.
func (a queryBlockCapture) Capture(ctx context.Context, e identity.Envelope, id string) (Capture, error) {
	if a.query == nil {
		return Capture{}, ErrUnavailable
	}
	captured, err := a.query.CaptureDefinition(ctx, e, id)
	if err != nil {
		return Capture{}, err
	}
	out := Capture{SQL: captured.SQL, Parameters: captured.Parameters, Schema: captured.Schema, Source: captured.Source, Context: captured.Context, Question: captured.Question, Topics: []TopicPin{}}
	for _, publication := range captured.Publications {
		out.Topics = append(out.Topics, TopicPin{Topic: publication.Definition.Topic, Version: publication.Definition.Version, Digest: publication.Digest})
	}
	return out, nil
}
