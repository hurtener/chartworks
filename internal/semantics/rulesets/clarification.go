package rulesets

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// ResolvePublishedClarifications consumes already-admitted immutable publications.
// It does not load authority, execute a query, or approve an authoring draft.
// The routing consumer must first admit every requested topic and source context.
func ResolvePublishedClarifications(topic topics.Published, published Published, input semantics.ClarificationInput) (semantics.ClarificationEvaluation, error) {
	if !topic.State.Active || topic.State.Archived || !published.State.Active || published.State.Retired || topic.State.Topic != published.State.Topic || topic.Definition.Topic != published.Definition.Topic || topic.State.Version != published.Definition.TopicVersion || published.State.Version != published.Definition.Version || topic.Digest != published.Definition.PackDigest {
		return semantics.ClarificationEvaluation{}, store.ErrConflict
	}
	subject, err := publishedSubject(topic)
	if err != nil {
		return semantics.ClarificationEvaluation{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, published.Definition)
	if err != nil {
		return semantics.ClarificationEvaluation{}, err
	}
	if model.Digest() != published.Digest {
		return semantics.ClarificationEvaluation{}, store.ErrConflict
	}
	return semantics.ResolveClarifications(model, input), nil
}

// ClarificationPreviewRequest is a bounded synthetic authoring preview. A draft
// is evaluated, never made applicable to production routing by this operation.
type ClarificationPreviewRequest struct {
	Definition semantics.RuleSetDefinition    `json:"definition"`
	Cases      []semantics.ClarificationInput `json:"cases"`
}

// ClarificationPreview pins deterministic preview evidence to the exact candidate
// and currently readable topic. Publication still requires a separate review.
type ClarificationPreview struct {
	Topic        string                              `json:"topic"`
	TopicVersion string                              `json:"topic_version"`
	PackDigest   string                              `json:"pack_digest"`
	RuleDigest   string                              `json:"rule_digest"`
	Cases        []semantics.ClarificationEvaluation `json:"cases"`
}

// PreviewClarifications uses the same compiler and evaluator as production. It
// requires draft-write reach; preview results have no route or executable seal.
func (s *Service) PreviewClarifications(ctx context.Context, e identity.Envelope, in ClarificationPreviewRequest) (ClarificationPreview, error) {
	if ctx == nil || len(in.Cases) < 1 || len(in.Cases) > 16 {
		return ClarificationPreview{}, store.ErrInvalid
	}
	published, err := s.topics.ReadPublishedTopic(ctx, e, in.Definition.Topic, in.Definition.TopicVersion, drafts.Write)
	if err != nil {
		return ClarificationPreview{}, err
	}
	if !published.State.Active || published.State.Version != in.Definition.TopicVersion || published.Digest != in.Definition.PackDigest {
		return ClarificationPreview{}, store.ErrConflict
	}
	subject, err := publishedSubject(published)
	if err != nil {
		return ClarificationPreview{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, in.Definition)
	if err != nil {
		return ClarificationPreview{}, err
	}
	out := ClarificationPreview{Topic: in.Definition.Topic, TopicVersion: published.State.Version, PackDigest: published.Digest, RuleDigest: model.Digest(), Cases: make([]semantics.ClarificationEvaluation, 0, len(in.Cases))}
	for _, item := range in.Cases {
		if err := ctx.Err(); err != nil {
			return ClarificationPreview{}, err
		}
		out.Cases = append(out.Cases, semantics.ResolveClarifications(model, item))
	}
	return out, nil
}

// ClarificationColumn resolves an exact reviewed semantic target to its stable
// dataset/column coordinates. Display names are never interpreted as SQL.
func ClarificationColumn(definition topics.Definition, target semantics.Reference) (string, semantics.Column, error) {
	if !target.Valid() {
		return "", semantics.Column{}, store.ErrInvalid
	}
	if target.Kind == semantics.KindDimension {
		found := false
		for _, dimension := range definition.Dimensions {
			if dimension.ID == target.ID {
				target, found = dimension.Field, true
				break
			}
		}
		if !found {
			return "", semantics.Column{}, store.ErrConflict
		}
	} else if target.Kind == semantics.KindMeasure {
		found := false
		for _, measure := range definition.Measures {
			if measure.ID == target.ID {
				target, found = measure.Field, true
				break
			}
		}
		if !found {
			return "", semantics.Column{}, store.ErrConflict
		}
	}
	if target.Kind != semantics.KindColumn {
		return "", semantics.Column{}, store.ErrInvalid
	}
	for _, dataset := range definition.Datasets {
		if dataset.ID != target.Dataset {
			continue
		}
		for _, column := range dataset.Columns {
			if column.ID == target.ID && column.SourceName != "" {
				return dataset.ID, column, nil
			}
		}
	}
	return "", semantics.Column{}, store.ErrConflict
}
