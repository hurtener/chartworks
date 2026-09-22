package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// Domains is the production adapter over the existing source, profile, draft and
// publication services. Proposal references remain private suggestions until the
// ordinary query/block/report authoring APIs create and review their definitions.
type sourceDomains interface {
	Get(context.Context, identity.Envelope, string) (sources.Source, error)
	Discover(context.Context, identity.Envelope, string) (sources.Discovery, error)
}
type profileDomains interface {
	InspectUpload(context.Context, identity.Envelope, string) (engineering.UploadStatus, error)
	InspectProfile(context.Context, identity.Envelope, string) (engineering.ProfileStatus, error)
	Build(context.Context, identity.Envelope, engineering.ProfileSpec, string, bool) (engineering.ProfileRun, error)
}
type draftDomains interface {
	Read(context.Context, identity.Envelope, string, int64) (drafts.Version, error)
	OnboardProfile(context.Context, identity.Envelope, drafts.OnboardRequest) (drafts.Version, error)
}
type topicDomains interface {
	Publish(context.Context, identity.Envelope, string, topics.PublishRequest) (topics.Published, error)
	Read(context.Context, identity.Envelope, string, string) (topics.Published, error)
}
type autopilotDomains interface {
	Get(context.Context, identity.Envelope, string) (engineering.AutopilotProposal, error)
}

type Domains struct {
	sources   sourceDomains
	profiles  profileDomains
	drafts    draftDomains
	topics    topicDomains
	autopilot autopilotDomains
}

func NewDomains(s sourceDomains, p profileDomains, d draftDomains, t topicDomains, a autopilotDomains) (*Domains, error) {
	if s == nil || p == nil || d == nil || t == nil || a == nil {
		return nil, ErrInvalid
	}
	return &Domains{sources: s, profiles: p, drafts: d, topics: t, autopilot: a}, nil
}

func (d *Domains) Connect(ctx context.Context, e identity.Envelope, in StartRequest, _ string) (StepResult, error) {
	if in.Mode == ModeUpload {
		u, err := d.profiles.InspectUpload(ctx, e, in.Upload)
		if err != nil {
			return StepResult{}, err
		}
		if u.State != "active" || u.Source == nil || u.Source.ID != in.Source {
			return StepResult{}, store.ErrConflict
		}
	}
	s, err := d.sources.Get(ctx, e, in.Source)
	if err != nil {
		return StepResult{}, err
	}
	if s.ContextID != in.Context || s.Status != "registered" {
		return StepResult{}, store.ErrConflict
	}
	return StepResult{References: []Reference{{Kind: "source", ID: s.ID, Revision: s.Revision}}, Evidence: []Evidence{{Entity: s.ID, Kind: "connectivity", Basis: []string{"registered_source", "exact_execution_context"}, Confidence: "observed"}}, Usage: Usage{Entities: 1}}, nil
}

func (d *Domains) Inspect(ctx context.Context, e identity.Envelope, r Run, _ string) (StepResult, error) {
	o, err := d.sources.Discover(ctx, e, r.Input.Source)
	if err != nil {
		return StepResult{}, err
	}
	if o.ContextID != r.Input.Context {
		return StepResult{}, store.ErrConflict
	}
	found := false
	entities := 0
	var evidence []Evidence
	for _, rel := range o.Relations {
		if rel.ID == r.Input.Dataset {
			found = true
			for _, col := range rel.Columns {
				confidence := "observed"
				uncertainty := ""
				if !col.Safe {
					confidence = "unresolved"
					uncertainty = "column excluded by source safety policy"
				}
				evidence = append(evidence, Evidence{Entity: col.Name, Kind: "column", Basis: []string{"source_catalog", "revision:" + fmt.Sprint(o.Revision)}, Confidence: confidence, Uncertainty: uncertainty, Sensitive: !col.Safe})
				entities++
			}
		}
	}
	if !found {
		return StepResult{}, store.ErrNotFound
	}
	return StepResult{References: []Reference{{Kind: "dataset", ID: r.Input.Dataset, Revision: o.Revision}}, Evidence: evidence, Usage: Usage{Entities: entities}}, nil
}

func (d *Domains) Profile(ctx context.Context, e identity.Envelope, r Run, key string) (StepResult, error) {
	_, inspectErr := d.profiles.InspectProfile(ctx, e, r.Input.Profile)
	resume := inspectErr == nil
	if inspectErr != nil && !errors.Is(inspectErr, store.ErrNotFound) {
		return StepResult{}, inspectErr
	}
	run, err := d.profiles.Build(ctx, e, engineering.ProfileSpec{ID: r.Input.Profile, Source: r.Input.Source, Context: r.Input.Context, Dataset: r.Input.Dataset, SkipLLM: false}, key, resume)
	if err != nil {
		return StepResult{}, err
	}
	if run.Profile.State != "complete" || run.Profile.Profile == nil {
		return StepResult{}, store.ErrUnavailable
	}
	p := run.Profile.Profile
	evidence := make([]Evidence, 0, len(p.Columns))
	questions := []Question{}
	for _, col := range p.Columns {
		basis := []string{"bounded_profile", "sample_rows:" + fmt.Sprint(p.Sampling.Rows)}
		confidence := "observed"
		uncertainty := ""
		if col.Nullable && col.Nulls > 0 {
			uncertainty = "observed null values require reviewed semantics"
		}
		evidence = append(evidence, Evidence{Entity: col.Name, Kind: "profile_column", Basis: basis, Confidence: confidence, Uncertainty: uncertainty})
		qid := "null_" + shortID(col.Name)
		if uncertainty != "" && answerValue(r.Answers, qid) == "" {
			questions = append(questions, Question{ID: qid, Prompt: localized(r.Locale, "How should null values be interpreted for "+col.Name+"?", "¿Cómo deben interpretarse los valores nulos de "+col.Name+"?"), Evidence: basis, Required: true})
		}
	}
	if r.Input.Transformation {
		proposal, proposalErr := d.autopilot.Get(ctx, e, r.Input.TransformationProposal)
		if proposalErr != nil {
			return StepResult{}, proposalErr
		}
		if proposal.State != "applied" {
			questions = append(questions, Question{ID: "approve_transformation", Prompt: localized(r.Locale, "Review and apply the managed transformation proposal through the reviewed engineering workflow.", "Revise y aplique la propuesta de transformación administrada mediante el flujo de ingeniería revisado."), Evidence: []string{"managed_write_review_required", "proposal:" + proposal.ID}, Required: true})
		} else {
			return StepResult{References: []Reference{{Kind: "profile", ID: p.Version, Revision: p.SourceRevision, Private: true}, {Kind: "engineering_proposal", ID: proposal.ID, Revision: proposal.Revision, Digest: proposal.Digest, Private: true}}, Evidence: append(evidence, Evidence{Entity: proposal.ID, Kind: "managed_transformation", Basis: []string{"reviewed_engineering_applied"}, Confidence: "observed"}), Questions: questions, Usage: Usage{Entities: len(evidence) + 1}}, nil
		}
	}
	return StepResult{References: []Reference{{Kind: "profile", ID: p.Version, Revision: p.SourceRevision, Private: true}}, Evidence: evidence, Questions: questions, Usage: Usage{Entities: len(evidence)}}, nil
}

func (d *Domains) DraftSemantics(ctx context.Context, e identity.Envelope, r Run, _ string) (StepResult, error) {
	v, err := d.drafts.Read(ctx, e, r.Input.Topic, 0)
	if errors.Is(err, store.ErrNotFound) {
		v, err = d.drafts.OnboardProfile(ctx, e, drafts.OnboardRequest{Topic: r.Input.Topic, Version: r.Input.TopicVersion, Name: r.Input.Topic, Description: localized(r.Locale, "Guided semantic draft for review", "Borrador semántico guiado para revisión"), Profile: r.Input.Profile, Change: "Guided onboarding profile scaffold"})
	}
	if err != nil {
		return StepResult{}, err
	}
	if v.Pack.Version != r.Input.TopicVersion {
		return StepResult{}, store.ErrConflict
	}
	if len(v.Pack.Datasets) == 0 {
		return StepResult{}, store.ErrInvalid
	}
	evidence := []Evidence{}
	questions := []Question{}
	for _, ds := range v.Pack.Datasets {
		for _, c := range ds.Columns {
			uncertainty := "business role, units, null semantics and sensitivity require review"
			evidence = append(evidence, Evidence{Entity: c.ID, Kind: "semantic_column", Basis: []string{"profile:" + r.Input.Profile, "dataset:" + ds.ID}, Confidence: "unresolved", Uncertainty: uncertainty, Sensitive: c.Sensitivity != "non_sensitive"})
			qid := "meaning_" + shortID(ds.ID+"_"+c.ID)
			if answerValue(r.Answers, qid) == "" {
				questions = append(questions, Question{ID: qid, Prompt: localized(r.Locale, "Define the business role, unit, null meaning and sensitivity for "+c.Name+".", "Defina el rol de negocio, la unidad, el significado de nulos y la sensibilidad de "+c.Name+"."), Evidence: []string{"profile:" + r.Input.Profile}, Required: false})
			}
		}
	}
	if answerValue(r.Answers, "grain") == "" {
		questions = append(questions, Question{ID: "grain", Prompt: localized(r.Locale, "Confirm dataset grain and candidate key evidence.", "Confirme el grano del conjunto y la evidencia de claves candidatas."), Evidence: []string{"dataset:" + r.Input.Dataset}, Required: true})
	}
	if answerValue(r.Answers, "joins") == "" {
		questions = append(questions, Question{ID: "joins", Prompt: localized(r.Locale, "Confirm join cardinality or explicitly retain it as unresolved.", "Confirme la cardinalidad de uniones o consérvela explícitamente como no resuelta."), Evidence: []string{"source_revision:" + fmt.Sprint(v.Pack.Datasets[0].Source.SourceRevision)}, Required: true})
	}
	if answerValue(r.Answers, "kpis") == "" {
		questions = append(questions, Question{ID: "kpis", Prompt: localized(r.Locale, "Define reviewed measures and KPIs, including units or currency, or leave them unresolved.", "Defina medidas y KPI revisados, incluidas unidades o moneda, o déjelos sin resolver."), Evidence: []string{"topic_draft:" + v.Metadata.Digest}, Required: true})
	}
	return StepResult{References: []Reference{{Kind: "topic_draft", ID: v.Metadata.Topic, Revision: v.Metadata.Revision, Digest: v.Metadata.Digest, Private: true}}, Evidence: evidence, Questions: questions, Usage: Usage{Entities: len(evidence)}}, nil
}

func (d *Domains) PublishReviewed(ctx context.Context, e identity.Envelope, r Run, review ReviewReference, _ string) (StepResult, error) {
	if review.Digest == "" {
		return StepResult{}, ErrInvalid
	}
	draft, err := d.drafts.Read(ctx, e, r.Input.Topic, review.Revision)
	if err != nil {
		return StepResult{}, err
	}
	if draft.Metadata.Revision != review.Revision || draft.Metadata.Digest != review.Digest || draft.Pack.Version != r.Input.TopicVersion {
		return StepResult{}, store.ErrConflict
	}
	p, err := d.topics.Publish(ctx, e, r.Input.Topic, topics.PublishRequest{Review: review.ID, Expected: 0})
	if errors.Is(err, store.ErrConflict) {
		// A publication may have committed before the onboarding CAS did. Read
		// the immutable version and accept only the exact reviewed digest.
		p, err = d.topics.Read(ctx, e, r.Input.Topic, r.Input.TopicVersion)
	}
	if err != nil {
		return StepResult{}, err
	}
	if p.Digest != review.Digest || p.State.Version != r.Input.TopicVersion {
		return StepResult{}, store.ErrConflict
	}
	return StepResult{References: []Reference{{Kind: "topic", ID: p.State.Topic, Revision: p.State.Revision, Digest: p.Digest}}, Evidence: []Evidence{{Entity: p.State.Topic, Kind: "publication", Basis: []string{"independent_review:" + review.ID}, Confidence: "observed"}}, Usage: Usage{Entities: 1}}, nil
}

func (d *Domains) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, r Run, _ string) (StepResult, error) {
	basis := []string{"published_topic:" + r.Input.Topic, "human_review_required"}
	return StepResult{References: []Reference{{Kind: "query_proposal", ID: r.ID + "-query", Private: true}, {Kind: "block_proposal", ID: r.Input.Block, Private: true}, {Kind: "report_proposal", ID: r.Input.Report, Private: true}}, Evidence: []Evidence{{Entity: r.Input.Block, Kind: "proposal", Basis: basis, Confidence: "unresolved", Uncertainty: "SQL, output selection, publication and certification remain separate reviewed operations"}}, Usage: Usage{Entities: 3}}, nil
}

func (d *Domains) ProposeDriftAmendment(_ context.Context, _ identity.Envelope, r Run, in DriftRequest, _ string) (Amendment, error) {
	affected := []Reference{}
	for _, ref := range r.References {
		if ref.Kind == "profile" || ref.Kind == "topic" || ref.Kind == "block_proposal" || ref.Kind == "report_proposal" {
			affected = append(affected, ref)
		}
	}
	return Amendment{Run: r.ID, SourceRevision: in.SourceRevision, Affected: affected, Proposal: Reference{Kind: "topic_amendment", ID: r.Input.Topic + "-amend-" + shortID(in.Observation), Private: true}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

func shortID(v string) string { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:6]) }
func localized(locale, en, es string) string {
	if locale == "es" {
		return es
	}
	return en
}
