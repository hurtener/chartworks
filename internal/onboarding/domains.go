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

func (d *Domains) ResolveRunAuthority(ctx context.Context, e identity.Envelope, r Run) ([]RunAuthority, error) {
	source, err := d.sources.Get(ctx, e, r.Input.Source)
	if err != nil {
		return nil, err
	}
	if source.ID != r.Input.Source || !identity.Identifier(source.ContextID) || source.Status != "registered" {
		return nil, store.ErrConflict
	}
	out := []RunAuthority{{Source: source.ID, Context: source.ContextID}}
	seen := map[string]bool{source.ID: true}
	for _, ref := range r.References {
		if ref.Source == "" || ref.Source == source.ID || seen[ref.Source] {
			continue
		}
		current, readErr := d.sources.Get(ctx, e, ref.Source)
		if readErr != nil {
			return nil, readErr
		}
		if current.ID != ref.Source || current.Status != "registered" || !identity.Identifier(current.ContextID) {
			return nil, store.ErrConflict
		}
		out = append(out, RunAuthority{Source: current.ID, Context: current.ContextID})
		seen[current.ID] = true
	}
	return out, nil
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
	return StepResult{References: []Reference{{Kind: "source", ID: s.ID, Revision: s.Revision, Source: s.ID, Context: s.ContextID}}, Evidence: []Evidence{{Entity: s.ID, Kind: "connectivity", Basis: []string{"registered_source", "exact_execution_context"}, Confidence: "observed"}}}, nil
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
	columns := []string{}
	entities := 0
	var evidence []Evidence
	for _, rel := range o.Relations {
		if rel.ID == r.Input.Dataset {
			found = true
			for _, col := range rel.Columns {
				columns = append(columns, col.Name)
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
	sort.Strings(columns)
	return StepResult{References: []Reference{{Kind: "dataset", ID: r.Input.Dataset, Revision: o.Revision, Source: r.Input.Source, Context: o.ContextID, Dataset: r.Input.Dataset, Columns: columns}}, Evidence: evidence}, nil
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
		if len(discovery.Relations) != 1 {
			return StepResult{}, store.ErrConflict
		}
		outputRelation := discovery.Relations[0]
		expectedColumns := make([]string, 0, len(last.Columns))
		for _, column := range last.Columns {
			expectedColumns = append(expectedColumns, column.Name)
		}
		actualColumns := make([]string, 0, len(outputRelation.Columns))
		for _, column := range outputRelation.Columns {
			actualColumns = append(actualColumns, column.Name)
		}
		sort.Strings(expectedColumns)
		sort.Strings(actualColumns)
		found := len(expectedColumns) == len(actualColumns)
		if found {
			for i := range expectedColumns {
				found = found && expectedColumns[i] == actualColumns[i]
			}
		}
		if !found || output.ID != effect.Target || output.ContextID != discovery.ContextID || output.Revision != effect.Version || output.Revision != discovery.Revision {
			return StepResult{}, store.ErrConflict
		}
		source, contextID, dataset = output.ID, output.ContextID, outputRelation.ID
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
	profileColumns := make([]string, 0, len(p.Columns))
	questions := []Question{}
	for _, col := range p.Columns {
		profileColumns = append(profileColumns, col.Name)
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
	sort.Strings(profileColumns)
	if transformation != nil {
		proposalKey := "engineering_proposal:" + transformation.ID
		return StepResult{References: []Reference{{Kind: "engineering_proposal", ID: transformation.ID, Revision: transformation.Revision, Digest: transformation.Digest, Private: true, Source: source, Context: contextID, Dataset: dataset, DependsOn: []string{"dataset:" + r.Input.Dataset}}, {Kind: "profile", ID: p.Version, Revision: p.SourceRevision, Private: true, Source: source, Context: contextID, Dataset: dataset, Columns: profileColumns, DependsOn: []string{proposalKey}}}, Evidence: append(evidence, Evidence{Entity: transformation.ID, Kind: "managed_transformation", Basis: []string{"reviewed_engineering_applied", "output_source:" + source, "output_dataset:" + dataset}, Confidence: "observed"}), Questions: questions}, nil
	}
	return StepResult{References: []Reference{{Kind: "profile", ID: p.Version, Revision: p.SourceRevision, Private: true, Source: source, Context: contextID, Dataset: dataset, Columns: profileColumns, DependsOn: []string{"dataset:" + dataset}}}, Evidence: evidence, Questions: questions}, nil
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
	semanticColumns := []string{}
	semanticSource, semanticContext, semanticDataset := "", "", ""
	for _, ds := range v.Pack.Datasets {
		if ds.Source.ProfileVersion == r.Input.Profile {
			semanticSource, semanticContext, semanticDataset = ds.Source.Source, ds.Source.Context, ds.ID
			for _, col := range ds.Columns {
				name := col.SourceName
				if name == "" {
					name = col.ID
				}
				semanticColumns = append(semanticColumns, name)
			}
		}
	}
	if semanticSource == "" || semanticContext == "" || semanticDataset == "" {
		return StepResult{}, store.ErrConflict
	}
	sort.Strings(semanticColumns)
	return StepResult{References: []Reference{{Kind: "topic_draft", ID: v.Metadata.Topic, Revision: v.Metadata.Revision, Digest: v.Metadata.Digest, Private: true, Source: semanticSource, Context: semanticContext, Dataset: semanticDataset, Columns: semanticColumns, DependsOn: []string{"profile:" + r.Input.Profile}}}, Evidence: evidence, Questions: questions}, nil
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
		return publicationStep(p, review, r)
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
	return publicationStep(p, review, r)
}

func publicationStep(p topics.Published, review ReviewReference, r Run) (StepResult, error) {
	coordinate := Reference{}
	for _, ref := range r.References {
		if ref.Kind == "topic_draft" && ref.ID == p.State.Topic {
			coordinate = ref
		}
	}
	if coordinate.Source == "" || coordinate.Context == "" || coordinate.Dataset == "" || len(coordinate.Columns) == 0 {
		return StepResult{}, store.ErrConflict
	}
	return StepResult{References: []Reference{{Kind: "topic", ID: p.State.Topic, Revision: p.State.Revision, Digest: p.Digest, Source: coordinate.Source, Context: coordinate.Context, Dataset: coordinate.Dataset, Columns: append([]string(nil), coordinate.Columns...), DependsOn: []string{"topic_draft:" + p.State.Topic}}}, Evidence: []Evidence{{Entity: p.State.Topic, Kind: "publication", Basis: []string{"independent_review:" + review.ID}, Confidence: "observed"}}, Receipt: p.Receipt}, nil
}

func (d *Domains) ProposeQueriesBlocksReports(_ context.Context, _ identity.Envelope, r Run, _ string) (StepResult, error) {
	basis := []string{"published_topic:" + r.Input.Topic, "human_review_required"}
	dependency := []string{"topic:" + r.Input.Topic}
	topic := Reference{}
	for _, ref := range r.References {
		if ref.Kind == "topic" && ref.ID == r.Input.Topic {
			topic = ref
		}
	}
	if topic.Source == "" || topic.Context == "" || topic.Dataset == "" {
		return StepResult{}, store.ErrConflict
	}
	digest := readexec.Hash([]any{topic.Source, topic.Context, topic.Dataset, r.Input.Topic})
	coordinate := func(kind, id string) Reference {
		return Reference{Kind: kind, ID: id, Digest: digest, Private: true, Source: topic.Source, Context: topic.Context, Dataset: topic.Dataset, DependsOn: dependency}
	}
	return StepResult{References: []Reference{coordinate("onboarding_query_intent", r.ID+"-query"), coordinate("onboarding_block_intent", r.Input.Block), coordinate("onboarding_report_intent", r.Input.Report)}, Evidence: []Evidence{{Entity: r.Input.Block, Kind: "proposal", Basis: basis, Confidence: "unresolved", Uncertainty: "run-owned intent only; ordinary authoring, SQL, output selection, publication and certification remain separate reviewed operations"}}}, nil
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
	changed := map[string]bool{}
	for _, name := range changes {
		changed[name] = true
	}
	refKey := func(ref Reference) string { return ref.Kind + ":" + ref.ID }
	affectedKeys := map[string]bool{}
	impact := []ImpactEvidence{}
	affected := []Reference{}
	add := func(ref Reference, basis []string, conservative bool) {
		key := refKey(ref)
		if affectedKeys[key] {
			return
		}
		affectedKeys[key] = true
		affected = append(affected, ref)
		impact = append(impact, ImpactEvidence{Kind: ref.Kind, ID: ref.ID, Basis: basis, Conservative: conservative})
	}
	for _, ref := range r.References {
		if ref.Source != source.ID || ref.Dataset != r.Input.Dataset {
			continue
		}
		if observation == "binding_changed" {
			add(ref, []string{"exact_source:" + source.ID, "execution_context_changed"}, false)
			continue
		}
		matched := []string{}
		for _, column := range ref.Columns {
			if changed[column] {
				matched = append(matched, "column:"+column)
			}
		}
		if len(matched) > 0 {
			sort.Strings(matched)
			add(ref, matched, false)
		}
	}
	// Close over stable run-owned dependency coordinates. Query/block/report
	// intents are conservative because they do not yet contain executable
	// definitions; that uncertainty is retained explicitly instead of calling
	// them directly column-affected.
	for changedClosure := true; changedClosure; {
		changedClosure = false
		for _, ref := range r.References {
			if affectedKeys[refKey(ref)] {
				continue
			}
			for _, dependency := range ref.DependsOn {
				if affectedKeys[dependency] {
					conservative := strings.HasPrefix(ref.Kind, "onboarding_")
					add(ref, []string{"depends_on:" + dependency}, conservative)
					changedClosure = true
					break
				}
			}
		}
	}
	if len(affected) == 0 {
		return Amendment{}, store.ErrConflict
	}
	digest := shortID(source.ID + "\x00" + source.ContextID + "\x00" + fmt.Sprint(source.Revision) + "\x00" + strings.Join(changes, "\x00"))
	return Amendment{Run: r.ID, Observation: observation, Source: source.ID, Context: source.ContextID, SourceRevision: source.Revision, Changes: changes, Affected: affected, ImpactEvidence: impact, Proposal: Reference{Kind: "topic_amendment", ID: r.Input.Topic + "-amend-" + digest, Digest: readexec.Hash(changes), Private: true, Source: source.ID, Context: source.ContextID, Dataset: r.Input.Dataset}, RequiredAction: "review_amendment", ExistingIntact: true, CreatedAt: time.Now().UTC()}, nil
}

func shortID(v string) string { s := sha256.Sum256([]byte(v)); return hex.EncodeToString(s[:6]) }
func localized(locale, en, es string) string {
	if locale == "es" {
		return es
	}
	return en
}
