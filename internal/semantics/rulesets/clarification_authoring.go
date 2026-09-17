package rulesets

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
)

// ClarificationDisposition explains how a retained pattern is interpreted.
// Imported dispositions are checked against the definition, never trusted.
type ClarificationDisposition struct {
	Pattern     string `json:"pattern"`
	Version     string `json:"version"`
	Status      string `json:"status"`
	Explanation string `json:"explanation"`
}

// PortableClarifications carries exact reviewed references, not SQL mappings or
// authority. Importing is preview-only until the ordinary save/review/publish
// lifecycle explicitly approves a new version.
type PortableClarifications struct {
	SchemaVersion int                         `json:"schema_version"`
	Definition    semantics.RuleSetDefinition `json:"definition"`
	RuleDigest    string                      `json:"rule_digest"`
	Dispositions  []ClarificationDisposition  `json:"dispositions"`
}

type ClarificationExportRequest struct {
	Version string `json:"version"`
}

// ClarificationImportRequest intentionally supports an exact semantic-topic
// roundtrip. A foreign topic/digest requires explicit semantic remapping and a
// new reviewed authoring draft; display labels never provide an implicit map.
type ClarificationImportRequest struct {
	Pack              PortableClarifications         `json:"pack"`
	Version           string                         `json:"version"`
	LegacyDisposition string                         `json:"legacy_disposition,omitempty"`
	Cases             []semantics.ClarificationInput `json:"cases"`
}

type ClarificationImportPreview struct {
	Definition     semantics.RuleSetDefinition `json:"definition"`
	Preview        ClarificationPreview        `json:"preview"`
	Dispositions   []ClarificationDisposition  `json:"dispositions"`
	ReviewRequired bool                        `json:"review_required"`
}

// ClarificationImportError is content-free but carries an actionable mapping or
// legacy-disposition reason. It never echoes an imported definition or answer.
type ClarificationImportError struct {
	Code string `json:"code"`
}

func (e *ClarificationImportError) Error() string {
	return "rulesets: clarification import requires review"
}
func (e *ClarificationImportError) Unwrap() error { return store.ErrConflict }

func clarificationDispositions(definition semantics.RuleSetDefinition) []ClarificationDisposition {
	out := make([]ClarificationDisposition, 0, len(definition.Patterns))
	for _, pattern := range definition.Patterns {
		item := ClarificationDisposition{Pattern: pattern.ID, Version: pattern.Version, Status: semantics.ClarificationMigrationDisposition(pattern)}
		switch item.Status {
		case "conditional_v1":
			item.Explanation = "Reviewed conditions determine applicability; accepted answers have typed effects."
		case "reviewed_disabled":
			item.Explanation = "Explicitly disabled by the reviewed policy; no automatic blocking or default."
		default:
			item.Explanation = "Legacy reference choices remain explicit selections only; conditional activation and non-reference effects require a reviewed rewrite."
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pattern < out[j].Pattern })
	return out
}

// ExportClarifications checks export reach on the exact retained topic before
// loading the rule payload. It performs no provider or warehouse query.
func (s *Service) ExportClarifications(ctx context.Context, e identity.Envelope, topic string, in ClarificationExportRequest) (PortableClarifications, error) {
	if ctx == nil || !identity.Identifier(topic) || !identity.Identifier(in.Version) {
		return PortableClarifications{}, store.ErrInvalid
	}
	pin, err := s.repo.RuleVersionPin(ctx, e, topic, in.Version, drafts.Read)
	if err != nil {
		return PortableClarifications{}, err
	}
	publication, err := s.topics.ReadPublishedTopic(ctx, e, topic, pin.TopicVersion, drafts.Export)
	if err != nil {
		return PortableClarifications{}, err
	}
	published, err := s.repo.ReadPublishedRules(ctx, e, topic, pin.RuleVersion, drafts.Read, false)
	if err != nil {
		return PortableClarifications{}, err
	}
	subject, err := publishedSubject(publication)
	if err != nil {
		return PortableClarifications{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, published.Definition)
	if err != nil {
		return PortableClarifications{}, err
	}
	if publication.Digest != pin.PackDigest || model.Digest() != published.Digest {
		return PortableClarifications{}, store.ErrConflict
	}
	definition := model.Definition()
	return PortableClarifications{SchemaVersion: 1, Definition: definition, RuleDigest: model.Digest(), Dispositions: clarificationDispositions(definition)}, nil
}

// PreviewClarificationImport validates the complete pack and explicitly refuses
// automatic legacy activation. The result is a proposal, not a saved or approved
// rule version. The existing rule-draft endpoint supplies the actual save.
func (s *Service) PreviewClarificationImport(ctx context.Context, e identity.Envelope, topic string, in ClarificationImportRequest) (ClarificationImportPreview, error) {
	if ctx == nil || in.Pack.SchemaVersion != 1 || !identity.Identifier(topic) || !identity.Identifier(in.Version) || in.Version == in.Pack.Definition.Version || len(in.Cases) < 1 || len(in.Cases) > 16 {
		return ClarificationImportPreview{}, store.ErrInvalid
	}
	definition := semantics.CloneRuleSetDefinition(in.Pack.Definition)
	if definition.Topic != topic {
		return ClarificationImportPreview{}, &ClarificationImportError{Code: "semantic_mapping_required"}
	}
	publication, err := s.topics.ReadPublishedTopic(ctx, e, topic, definition.TopicVersion, drafts.Write)
	if err != nil {
		return ClarificationImportPreview{}, err
	}
	if !publication.State.Active || publication.Digest != definition.PackDigest {
		return ClarificationImportPreview{}, &ClarificationImportError{Code: "semantic_mapping_required"}
	}
	subject, err := publishedSubject(publication)
	if err != nil {
		return ClarificationImportPreview{}, err
	}
	model, err := semantics.CompilePublishedRules(subject, definition)
	if err != nil {
		return ClarificationImportPreview{}, err
	}
	dispositions := clarificationDispositions(model.Definition())
	if model.Digest() != in.Pack.RuleDigest || !reflect.DeepEqual(dispositions, in.Pack.Dispositions) {
		return ClarificationImportPreview{}, &ClarificationImportError{Code: "portable_definition_changed"}
	}
	if in.LegacyDisposition != "" && in.LegacyDisposition != "preserve_reference_only" {
		return ClarificationImportPreview{}, store.ErrInvalid
	}
	for _, pattern := range definition.Patterns {
		if pattern.Policy == nil && in.LegacyDisposition != "preserve_reference_only" {
			return ClarificationImportPreview{}, &ClarificationImportError{Code: "legacy_disposition_required"}
		}
	}
	definition.Version = in.Version
	for i := range definition.Patterns {
		definition.Patterns[i].Provenance = semantics.RuleProvenance{Kind: semantics.ProvenanceImport, Evidence: "portable-" + in.Pack.RuleDigest}
	}
	preview, err := s.PreviewClarifications(ctx, e, ClarificationPreviewRequest{Definition: definition, Cases: in.Cases})
	if err != nil {
		return ClarificationImportPreview{}, err
	}
	return ClarificationImportPreview{Definition: definition, Preview: preview, Dispositions: clarificationDispositions(definition), ReviewRequired: true}, nil
}

func evaluateClarificationCases(ctx context.Context, model semantics.RuleModel, cases []semantics.ClarificationInput) ([]semantics.ClarificationEvaluation, error) {
	if len(cases) > 16 {
		return nil, store.ErrInvalid
	}
	if len(cases) == 0 {
		return nil, nil
	}
	out := make([]semantics.ClarificationEvaluation, 0, len(cases))
	for _, input := range cases {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Raw case questions and submitted spellings are not stored in Comparison.
		// Only reviewed sensitivity labels, canonical resolutions and outcome data
		// leave this pure evaluator for protected evidence persistence.
		out = append(out, semantics.ResolveClarifications(model, input))
	}
	raw, err := json.Marshal(out)
	if err != nil || len(raw) > 1<<20 {
		return nil, store.ErrInvalid
	}
	return out, nil
}

func sameClarificationCases(a, b []semantics.ClarificationEvaluation) bool {
	normalize := func(cases []semantics.ClarificationEvaluation) []any {
		out := make([]any, 0, len(cases))
		for _, evaluation := range cases {
			slots := make([]any, 0, len(evaluation.Slots))
			for _, slot := range evaluation.Slots {
				slots = append(slots, []any{slot.Pattern, slot.Slot, slot.Outcome, slot.Reason, slot.Defaulted, slot.Prompt, slot.Why, slot.Choices, slot.Effect})
			}
			resolutions := make([]any, 0, len(evaluation.Resolutions))
			for _, r := range evaluation.Resolutions {
				resolutions = append(resolutions, []any{r.Pattern, r.Slot, r.Provenance, r.Reference, r.Effect, r.Time, r.Value, r.Upper, r.Null})
			}
			codes := make([]any, 0, len(evaluation.Errors))
			for _, field := range evaluation.Errors {
				codes = append(codes, []any{field.Pattern, field.Slot, field.Field, field.Code})
			}
			out = append(out, []any{evaluation.Outcome, slots, resolutions, codes})
		}
		return out
	}
	left, le := json.Marshal(normalize(a))
	right, re := json.Marshal(normalize(b))
	return le == nil && re == nil && string(left) == string(right)
}

// IsClarificationImportError exposes a stable repair code without importing
// arbitrary payload text into a transport error.
func IsClarificationImportError(err error) string {
	var failure *ClarificationImportError
	if errors.As(err, &failure) {
		return failure.Code
	}
	return ""
}
