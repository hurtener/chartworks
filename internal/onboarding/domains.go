package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
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
	PublishBounded(context.Context, identity.Envelope, string, topics.PublishRequest, gateway.Limits) (topics.Published, error)
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

type reconciliationKey struct{}

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
	return StepResult{References: []Reference{{Kind: "source", ID: s.ID, Revision: s.Revision}}, Evidence: []Evidence{{Entity: s.ID, Kind: "connectivity", Basis: []string{"registered_source", "exact_execution_context"}, Confidence: "observed"}}}, nil
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
				evidence = append(evidence, Evidence{Entity: col.Name, Kind: "column", Basis: []string{"source_catalog", "schema_digest:" + readexec.Hash(col)}, Confidence: confidence, Uncertainty: uncertainty, Sensitive: !col.Safe})
				entities++
			}
		}
	}
	if !found {
		return StepResult{}, store.ErrNotFound
	}
	_ = entities
	return StepResult{References: []Reference{{Kind: "dataset", ID: r.Input.Dataset, Revision: o.Revision}}, Evidence: evidence}, nil
}

func (d *Domains) Profile(ctx context.Context, e identity.Envelope, r Run, key string) (StepResult, error) {
	source, contextID, dataset := r.Input.Source, r.Input.Context, r.Input.Dataset
	var transformation *engineering.AutopilotProposal
	if r.Input.Transformation {
		proposal, err := d.autopilot.Get(ctx, e, r.Input.TransformationProposal)
		if err != nil {
			return StepResult{}, err
		}
		if proposal.ID != r.Input.TransformationProposal || proposal.State != "applied" || proposal.Digest != proposal.Material.Digest() || proposal.Material.Binding.Source != source || proposal.Material.Binding.Context != contextID || proposal.Material.Binding.Revision != r.SourceRevision || proposal.Material.Request.Source != source || proposal.Material.Request.Context != contextID || len(proposal.Material.Pipeline.Steps) == 0 {
			if proposal.State != "applied" {
				return StepResult{Questions: []Question{{ID: "approve_transformation", Prompt: localized(r.Locale, "Review and apply the exact managed transformation proposal through the reviewed engineering workflow.", "Revise y aplique la propuesta de transformación administrada exacta mediante el flujo de ingeniería revisado."), Evidence: []string{"managed_write_review_required", "proposal:" + proposal.ID}, Required: true}}}, nil
			}
			return StepResult{}, store.ErrConflict
		}
		last := proposal.Material.Pipeline.Steps[len(proposal.Material.Pipeline.Steps)-1]
		var effect *engineering.ProposalEffect
		pipelineCommitted := false
		for i := range proposal.Effects {
			candidate := &proposal.Effects[i]
			if candidate.Kind == "pipeline_run" && candidate.Target == proposal.Material.Pipeline.ID && candidate.State == "published" && candidate.Digest == readexec.Hash(proposal.Material.Pipeline) && candidate.Operation == proposal.Operation {
				pipelineCommitted = true
			}
			if candidate.Kind == "managed_step" && candidate.Target == proposal.Material.Pipeline.ID+"."+last.ID && candidate.State == "checked" && candidate.Version > 0 && candidate.Digest != "" && candidate.Operation == proposal.Operation {
				effect = candidate
			}
		}
		if effect == nil || !pipelineCommitted {
			return StepResult{}, store.ErrConflict
		}
		output, err := d.sources.Get(ctx, e, effect.Target)
		if err != nil {
			return StepResult{}, err
		}
		discovery, err := d.sources.Discover(ctx, e, output.ID)
		if err != nil {
			return StepResult{}, err
		}
		found := false
		for _, relation := range discovery.Relations {
			if relation.ID == last.ID {
				found = true
			}
		}
		if !found || output.ID != effect.Target || output.ContextID != discovery.ContextID || output.Revision != effect.Version || output.Revision != discovery.Revision {
			return StepResult{}, store.ErrConflict
		}
		source, contextID, dataset = output.ID, output.ContextID, last.ID
		transformation = &proposal
	}
	status, inspectErr := d.profiles.InspectProfile(ctx, e, r.Input.Profile)
	resume := inspectErr == nil
	if inspectErr != nil && !errors.Is(inspectErr, store.ErrNotFound) {
		return StepResult{}, inspectErr
	}
	var p *engineering.Profile
	if inspectErr == nil && status.State == "complete" && status.Profile != nil {
		if status.Profile.Source != source || status.Profile.Context != contextID || status.Profile.Dataset != dataset {
			return StepResult{}, store.ErrConflict
		}
		copy := *status.Profile
		p = &copy
	}
	if p == nil {
		run, err := d.profiles.Build(ctx, e, engineering.ProfileSpec{ID: r.Input.Profile, Source: source, Context: contextID, Dataset: dataset, SkipLLM: true}, key, resume)
		if err != nil {
			return StepResult{}, err
		}
		if run.Profile.State != "complete" || run.Profile.Profile == nil {
			return StepResult{}, store.ErrUnavailable
		}
		p = run.Profile.Profile
	}
	if p.Source != source || p.Context != contextID || p.Dataset != dataset || p.SourceRevision < 1 {
		return StepResult{}, store.ErrConflict
	}
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
	if transformation != nil {
		return StepResult{References: []Reference{{Kind: "profile", ID: p.Version, Revision: p.SourceRevision, Private: true}, {Kind: "engineering_proposal", ID: transformation.ID, Revision: transformation.Revision, Digest: transformation.Digest, Private: true}}, Evidence: append(evidence, Evidence{Entity: transformation.ID, Kind: "managed_transformation", Basis: []string{"reviewed_engineering_applied", "output_source:" + source, "output_dataset:" + dataset}, Confidence: "observed"}), Questions: questions}, nil
	}
	return StepResult{References: []Reference{{Kind: "profile", ID: p.Version, Revision: p.SourceRevision, Private: true}}, Evidence: evidence, Questions: questions}, nil
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
	return StepResult{References: []Reference{{Kind: "topic_draft", ID: v.Metadata.Topic, Revision: v.Metadata.Revision, Digest: v.Metadata.Digest, Private: true}}, Evidence: evidence, Questions: questions}, nil
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
	// Reconcile an exact immutable version first: publication can commit before
	// the onboarding ledger CAS. Otherwise bind publication to the current head.
	p, err := d.topics.Read(ctx, e, r.Input.Topic, r.Input.TopicVersion)
	if err == nil && p.Digest == review.Digest {
		return publicationStep(p, review), nil
	}
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return StepResult{}, err
	}
	if reconcileOnly, _ := ctx.Value(reconciliationKey{}).(bool); reconcileOnly {
		return StepResult{}, store.ErrUnavailable
	}
	expected := int64(0)
	current, currentErr := d.topics.Read(ctx, e, r.Input.Topic, "")
	if currentErr == nil {
		expected = current.State.Revision
	} else if !errors.Is(currentErr, store.ErrNotFound) {
		return StepResult{}, currentErr
	}
	if r.Lease == nil || r.Lease.ReservedCalls < 1 || r.Lease.ReservedTokens < 1 {
		return StepResult{}, ErrBudget
	}
	duration := time.Until(r.Deadline)
	if duration > 30*time.Second {
		duration = 30 * time.Second
	}
	p, err = d.topics.PublishBounded(ctx, e, r.Input.Topic, topics.PublishRequest{Review: review.ID, Expected: expected}, gateway.Limits{Calls: r.Lease.ReservedCalls, Tokens: r.Lease.ReservedTokens, Duration: duration})
	if errors.Is(err, store.ErrConflict) {
		p, err = d.topics.Read(ctx, e, r.Input.Topic, r.Input.TopicVersion)
	}
	if err != nil {
		return StepResult{}, err
	}
	if p.Digest != review.Digest || p.State.Version != r.Input.TopicVersion {
		return StepResult{}, store.ErrConflict
	}
	return publicationStep(p, review), nil
}

func publicationStep(p topics.Published, review ReviewReference) StepResult {
	return StepResult{References: []Reference{{Kind: "topic", ID: p.State.Topic, Revision: p.State.Revision, Digest: p.Digest}}, Evidence: []Evidence{{Entity: p.State.Topic, Kind: "publication", Basis: []string{"independent_review:" + review.ID}, Confidence: "observed"}}, Receipt: p.Receipt}
}

func (d *Domains) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, r Run, _ string) (StepResult, error) {
	basis := []string{"published_topic:" + r.Input.Topic, "human_review_required"}
	return StepResult{References: []Reference{{Kind: "onboarding_query_intent", ID: r.ID + "-query", Private: true}, {Kind: "onboarding_block_intent", ID: r.Input.Block, Private: true}, {Kind: "onboarding_report_intent", ID: r.Input.Report, Private: true}}, Evidence: []Evidence{{Entity: r.Input.Block, Kind: "proposal", Basis: basis, Confidence: "unresolved", Uncertainty: "run-owned intent only; ordinary authoring, SQL, output selection, publication and certification remain separate reviewed operations"}}}, nil
}

func (d *Domains) ProposeDriftAmendment(ctx context.Context, e identity.Envelope, r Run, _ DriftRequest, _ string) (Amendment, error) {
	source, err := d.sources.Get(ctx, e, r.Input.Source)
	if err != nil {
		return Amendment{}, err
	}
	if source.Revision <= r.SourceRevision {
		return Amendment{}, store.ErrConflict
	}
	discovery, err := d.sources.Discover(ctx, e, source.ID)
	if err != nil {
		return Amendment{}, err
	}
	if discovery.ContextID != source.ContextID || discovery.Revision != source.Revision {
		return Amendment{}, store.ErrConflict
	}
	old := map[string]string{}
	for _, ev := range r.Evidence {
		if ev.Kind == "column" {
			for _, basis := range ev.Basis {
				if strings.HasPrefix(basis, "schema_digest:") {
					old[ev.Entity] = strings.TrimPrefix(basis, "schema_digest:")
				}
			}
		}
	}
	current := map[string]string{}
	found := false
	for _, rel := range discovery.Relations {
		if rel.ID == r.Input.Dataset {
			found = true
			for _, col := range rel.Columns {
				current[col.Name] = readexec.Hash(col)
			}
		}
	}
	if !found {
		return Amendment{}, store.ErrNotFound
	}
	changes := []string{}
	for name, digest := range current {
		if old[name] != digest {
			changes = append(changes, name)
		}
	}
	for name := range old {
		if _, ok := current[name]; !ok {
			changes = append(changes, name)
		}
	}
	observation := "schema_changed"
	if source.ContextID != r.Input.Context {
		observation = "binding_changed"
		changes = append(changes, "execution_context")
	}
	if len(changes) == 0 {
		return Amendment{}, store.ErrConflict
	}
	sort.Strings(changes)
	affected := []Reference{}
	for _, ref := range r.References {
		if ref.Kind == "profile" || ref.Kind == "topic" || ref.Kind == "onboarding_block_intent" || ref.Kind == "onboarding_report_intent" {
			affected = append(affected, ref)
		}
	}
	if len(affected) == 0 {
		return Amendment{}, store.ErrConflict
	}
	digest := shortID(source.ID + "\x00" + source.ContextID + "\x00" + fmt.Sprint(source.Revision) + "\x00" + strings.Join(changes, "\x00"))
	return Amendment{Run: r.ID, Observation: observation, Source: source.ID, Context: source.ContextID, SourceRevision: source.Revision, Changes: changes, Affected: affected, Proposal: Reference{Kind: "topic_amendment", ID: r.Input.Topic + "-amend-" + digest, Digest: readexec.Hash(changes), Private: true}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

func shortID(v string) string { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:6]) }
func localized(locale, en, es string) string {
	if locale == "es" {
		return es
	}
	return en
}
