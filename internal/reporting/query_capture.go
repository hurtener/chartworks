package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// CaptureFromQueries supplies the existing query service's private, source-backed
// authoring handoff. The adapter performs no model or warehouse work of its own.
func CaptureFromQueries(query *nlqexec.Service, ruleReaders ...RuleReader) QueryCapture {
	if query == nil || len(ruleReaders) > 1 || len(ruleReaders) == 1 && nilValue(ruleReaders[0]) {
		return nil
	}
	var rules RuleReader
	if len(ruleReaders) == 1 {
		rules = ruleReaders[0]
	}
	return queryBlockCapture{query: query, rules: rules}
}

// queryBlockCapture translates a private existing-query handoff. No transport
// accepts SQL, schema, template provenance or topic digests in place of it.
type queryBlockCapture struct {
	query *nlqexec.Service
	rules RuleReader
}

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
	if len(captured.RuleVersions) > 0 && len(captured.RuleVersions) != len(captured.Publications) {
		return Capture{}, ErrStale
	}
	for i, version := range captured.RuleVersions {
		if version == "" {
			continue
		}
		if a.rules == nil {
			return Capture{}, ErrUnavailable
		}
		publication := captured.Publications[i]
		rules, err := a.rules.Read(ctx, e, publication.Definition.Topic, version)
		if err != nil {
			return Capture{}, err
		}
		if !rules.State.Active || rules.State.Version != version || rules.Definition.Topic != publication.Definition.Topic || rules.Definition.TopicVersion != publication.Definition.Version || rules.Definition.PackDigest != publication.Digest {
			return Capture{}, ErrStale
		}
		out.Rules = append(out.Rules, RulePin{Topic: publication.Definition.Topic, TopicVersion: publication.Definition.Version, PackDigest: publication.Digest, RuleVersion: version, RuleDigest: rules.Digest})
	}
	if len(captured.Templates) > 1 {
		return Capture{}, ErrInvalid
	}
	if len(captured.Templates) == 1 {
		selection := captured.Templates[0]
		matched := false
		for _, pin := range out.Rules {
			if pin.Topic == selection.Topic && pin.TopicVersion == selection.TopicVersion && pin.PackDigest == selection.PackDigest && pin.RuleVersion == selection.RuleVersion && pin.RuleDigest == selection.RuleDigest {
				matched = true
				break
			}
		}
		if !matched {
			return Capture{}, ErrStale
		}
		out.Template = &TemplatePin{ID: selection.ID, Version: selection.RuleVersion, Digest: selection.RuleDigest}
	}
	return out, nil
}
