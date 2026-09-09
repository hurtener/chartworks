package foundation

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
)

// queryBlockCapture translates a private existing-query handoff. No transport
// accepts SQL, schema, template provenance or topic digests in place of it.
type queryBlockCapture struct { query *nlqexec.Service }

func (a queryBlockCapture) Capture(ctx context.Context, e identity.Envelope, id string) (reporting.Capture, error) {
	if a.query == nil { return reporting.Capture{}, reporting.ErrUnavailable }
	captured, err := a.query.CaptureDefinition(ctx, e, id)
	if err != nil { return reporting.Capture{}, err }
	out := reporting.Capture{SQL: captured.SQL, Parameters: captured.Parameters, Schema: captured.Schema, Source: captured.Source, Context: captured.Context, Question: captured.Question, Topics: []reporting.TopicPin{}}
	for _, publication := range captured.Publications {
		out.Topics = append(out.Topics, reporting.TopicPin{Topic: publication.Definition.Topic, Version: publication.Definition.Version, Digest: publication.Digest})
	}
	return out, nil
}
