package reporting

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// Runs composes the existing block, read, queue and model services. Its retained
// read methods have no path to a warehouse or a model provider.
type Runs struct {
	blocks       *Service
	repo         RunRepository
	runner       *jobs.RequestRunner
	model        gateway.Engine
	modelVersion string
	limits       config.ReportingExecution
}

// NewRuns performs no source/model calls. Optional narrative availability does
// not determine the availability of frozen deterministic runs or artifact reads.
func NewRuns(blocks *Service, repo RunRepository, runner *jobs.RequestRunner, model gateway.Engine, modelVersion string, limits config.ReportingExecution) (*Runs, error) {
	if blocks == nil || nilValue(repo) || runner == nil || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	if nilValue(model) {
		model = nil
	}
	return &Runs{blocks: blocks, repo: repo, runner: runner, model: model, modelVersion: modelVersion, limits: limits}, nil
}

func normalizedRunRequest(in RunRequest) (RunRequest, error) {
	in = clone(in)
	if in.Policy == "" {
		in.Policy = "published"
	}
	if in.PartialPolicy == "" {
		in.PartialPolicy = "fail"
	}
	if in.Resolution.Timezone == "" {
		in.Resolution.Timezone = "UTC"
	}
	if !identity.Identifier(in.Key) || !slices.Contains([]string{"published", "certified_only", "explicit_stale", "private_preview"}, in.Policy) ||
		!slices.Contains([]string{"fail", "allow_partial"}, in.PartialPolicy) || in.Reference.Revision < 0 ||
		in.Policy != "private_preview" && in.Reference.Draft || in.Policy == "private_preview" && in.Reference.Revision < 1 ||
		in.ReuseMaxAgeSeconds < 0 || in.ReuseMaxAgeSeconds > 86400 || len(in.Outputs) > 32 || len(in.Arguments) > 64 || in.Locale != "" && !locale(in.Locale) {
		return RunRequest{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range in.Outputs {
		if !identity.Identifier(id) || seen[id] {
			return RunRequest{}, ErrInvalid
		}
		seen[id] = true
	}
	return in, nil
}

func runEligibility(e identity.Envelope, snapshot Snapshot, policy string, now time.Time) error {
	if snapshot.State.Archived || snapshot.Validation == nil {
		return ErrStale
	}
	private := snapshot.PublishedAt == nil
	if private {
		if policy != "private_preview" || snapshot.State.DraftState == "rejected" {
			return ErrStale
		}
		if err := RequirePrivate(e, snapshot.State.ID, snapshot.Revision.Actor, Execute); err != nil {
			return err
		}
	} else if policy == "private_preview" {
		// An explicitly private run of a published revision is still private. It
		// cannot be used as a way to read somebody else's original private draft.
		if err := Require(e, snapshot.State.ID, Preview); err != nil {
			return err
		}
	}
	v := snapshot.Validation
	if v.Evidence.DefinitionDigest != snapshot.Revision.Digest || v.Evidence.ExecutionDigest != snapshot.Revision.ExecutionDigest ||
		v.Evidence.RevisionID != snapshot.Revision.ID || !snapshot.Current || snapshot.Health.Status != "healthy" ||
		v.Evidence.DependencyDigest != snapshot.Health.DependencyDigest || !successful(v.Evidence.Attempt.Status) {
		return ErrStale
	}
	trust := project(snapshot, now).Trust
	if policy == "certified_only" && trust.Certification != "valid" {
		return ErrStale
	}
	if policy == "explicit_stale" && (snapshot.Attestation == nil || snapshot.Withdrawal != nil || trust.Certification == "unavailable") {
		return ErrStale
	}
	return nil
}

func (s *Runs) selectRunOutputs(d Definition, in RunRequest) ([]Output, error) {
	selected, err := SelectOutputs(d.Outputs, in.Outputs)
	if err != nil {
		return nil, err
	}
	// Authoring previews use definition order. An explicit frozen-run selection
	// seals the caller's output order, which is also the retained paging order.
	if len(in.Outputs) > 0 {
		byID := make(map[string]Output, len(selected))
		for _, output := range selected {
			byID[output.ID] = output
		}
		for i, id := range in.Outputs {
			selected[i] = byID[id]
		}
	}
	calls, tokens := 0, 0
	for _, output := range selected {
		if output.Kind != "narrative" {
			continue
		}
		n := output.Narrative
		if !in.Narrative || n == nil {
			return nil, ErrUnavailable
		}
		// Partial runs retain an explicit failed receipt for an unavailable
		// optional narrative instead of discarding independent table/chart
		// outputs. Opt-in and the full declared budget remain mandatory.
		if in.PartialPolicy != "allow_partial" && (s.model == nil || n.ModelVersion != s.modelVersion || n.SchemaVersion != "grounded-narrative-v1") {
			return nil, ErrUnavailable
		}
		calls += n.MaxCalls
		tokens += n.MaxTokens
	}
	if calls > s.limits.NarrativeCalls || tokens > s.limits.NarrativeTokens {
		return nil, ErrBudget
	}
	return selected, nil
}

// Admit first reserves the existing operation key/hash. Only the winning
// reservation may subsequently seal floating revision and logical-time inputs.
// A replay reads its original seal even after a new block publication.
func (s *Runs) Admit(ctx context.Context, e identity.Envelope, id string, input RunRequest) (RunView, error) {
	if s == nil || ctx == nil {
		return RunView{}, ErrInvalid
	}
	in, err := normalizedRunRequest(input)
	if err != nil {
		return RunView{}, err
	}
	if err = Require(e, id, Execute); err != nil {
		return RunView{}, err
	}
	if time.Duration(in.ReuseMaxAgeSeconds)*time.Second > time.Duration(s.limits.MaxReuseAge) {
		return RunView{}, ErrInvalid
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	requestHash := digest([]any{FrozenVersion, id, in})
	task, err := s.runner.Admit(ctx, e, in.Key, jobs.RequestInput{Kind: "reporting.run", Target: id, InputHash: requestHash})
	if err != nil {
		return RunView{}, err
	}
	existing, err := s.repo.ReadFrozenRun(ctx, e, task.ID, true)
	if err == nil {
		return existing.View, nil
	}
	if !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
		return RunView{}, err
	}
	if !time.Now().Before(task.Expires) {
		return RunView{}, ErrExpired
	}
	if !s.blocks.CanValidate() {
		return RunView{}, ErrUnavailable
	}
	snapshot, err := s.blocks.repo.ReadBlock(ctx, e, id, in.Reference, Execute)
	if err != nil {
		return RunView{}, err
	}
	if err = runEligibility(e, snapshot, in.Policy, time.Now()); err != nil {
		return RunView{}, err
	}
	d := snapshot.Revision.Definition
	selected, err := s.selectRunOutputs(d, in)
	if err != nil {
		return RunView{}, err
	}
	if in.Resolution.At.IsZero() {
		in.Resolution.At = task.Created
	}
	resolved, err := ResolveParameters(d.Parameters, in.Arguments, in.Resolution)
	if err != nil {
		return RunView{}, err
	}
	definitions, refs, err := s.blocks.resolveDefinitions(ctx, e, d, true)
	if err != nil {
		return RunView{}, err
	}
	binding, err := s.blocks.sources.ContextBinding(ctx, e, d.Source, d.Context)
	if err != nil {
		return RunView{}, err
	}
	if exec.Hash(binding) != snapshot.Validation.BindingDigest {
		return RunView{}, ErrStale
	}
	private := in.Policy == "private_preview"
	retention := s.limits.Retention
	if private {
		retention = s.limits.PreviewRetention
	}
	language := in.Locale
	if language == "" {
		language = d.Metadata[0].Locale
	}
	m := RunManifest{Version: FrozenVersion, ID: task.ID, Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Block: id,
		RequestHash: requestHash, TaskHash: task.ManifestHash, Revision: clone(snapshot.Revision), Outputs: selected,
		Resolved: resolved, Binding: binding.Clone(), Dependencies: clone(snapshot.Validation.Dependencies), References: refs,
		Trust: project(snapshot, task.Created).Trust, Private: private, Policy: in.Policy, PartialPolicy: in.PartialPolicy,
		Locale: language, Created: task.Created, Expires: task.Created.Add(time.Duration(retention)), Limits: s.limits,
		ReuseMaxAge: in.ReuseMaxAgeSeconds, Model: s.modelVersion, Definitions: []topics.Definition{}}
	for _, definition := range definitions {
		m.Definitions = append(m.Definitions, clone(definition.Definition))
	}
	privacyActor := ""
	if private {
		privacyActor = e.User()
	}
	m.ReuseKey = digest([]any{FrozenVersion, charts.Version, m.Tenant, m.Block, m.Revision.Digest,
		m.Outputs, m.Resolved.Parameters, m.Resolved.Timezone, m.Locale, exec.Hash(binding), m.Private, privacyActor,
		m.Policy, m.Trust, m.Model, m.Limits.MaxRows, m.Limits.MaxResultBytes})
	proof, err := prepareRun(e, m)
	if err != nil {
		return RunView{}, err
	}
	sealed, err := s.repo.SealFrozenRun(ctx, e, task, proof)
	return sealed.View, err
}

// Get reads only retained metadata. Current artifact/context reach is required,
// but neither sources.query nor reporting.execute is needed to open a result.
func (s *Runs) Get(ctx context.Context, e identity.Envelope, id string) (RunView, error) {
	if s == nil || ctx == nil {
		return RunView{}, ErrInvalid
	}
	r, err := s.repo.ReadFrozenRun(ctx, e, id, false)
	return r.View, err
}

// Inspect reads an actor/session-private execution receipt before an artifact is complete.
func (s *Runs) Inspect(ctx context.Context, e identity.Envelope, id string) (RunView, error) {
	if s == nil || ctx == nil {
		return RunView{}, ErrInvalid
	}
	r, err := s.repo.ReadFrozenRun(ctx, e, id, true)
	return r.View, err
}

// List returns only currently eligible artifact summaries with a bounded cursor.
func (s *Runs) List(ctx context.Context, e identity.Envelope, after string, limit int) (ArtifactList, error) {
	if s == nil || ctx == nil || limit < 1 || limit > 100 || after != "" && !identity.Identifier(after) {
		return ArtifactList{}, ErrInvalid
	}
	return s.repo.ListFrozenArtifacts(ctx, e, after, limit)
}

// Rows pages the one retained logical result without touching execution services.
func (s *Runs) Rows(ctx context.Context, e identity.Envelope, id string, offset, limit int) (ResultPage, error) {
	if s == nil || ctx == nil || offset < 0 || limit < 1 || limit > s.limits.PageRows {
		return ResultPage{}, ErrInvalid
	}
	r, err := s.repo.ReadFrozenRun(ctx, e, id, false)
	if err != nil {
		return ResultPage{}, err
	}
	if r.View.State == "expired" {
		return ResultPage{}, ErrExpired
	}
	if r.Result == nil || !slices.Contains([]string{"succeeded", "partial"}, r.View.State) {
		return ResultPage{}, ErrIncomplete
	}
	if offset > len(r.Result.Rows) {
		return ResultPage{}, ErrInvalid
	}
	end := min(len(r.Result.Rows), offset+limit)
	out := ResultPage{ID: id, Schema: clone(r.Result.Schema), Rows: clone(r.Result.Rows[offset:end]), Offset: offset,
		TotalRows: len(r.Result.Rows), Truncated: r.Result.Outcome == "truncated", Observed: clone(r.View.Observed)}
	if end < len(r.Result.Rows) {
		out.Next = &end
	}
	return out, nil
}

// Output returns the exact retained deterministic or narrative output. It never
// repairs missing text by regenerating it with a model.
func (s *Runs) Output(ctx context.Context, e identity.Envelope, id, output string) (RetainedOutput, error) {
	if s == nil || ctx == nil || !identity.Identifier(output) {
		return RetainedOutput{}, ErrInvalid
	}
	r, err := s.repo.ReadFrozenRun(ctx, e, id, false)
	if err != nil {
		return RetainedOutput{}, err
	}
	if r.View.State == "expired" {
		return RetainedOutput{}, ErrExpired
	}
	if !slices.Contains([]string{"succeeded", "partial"}, r.View.State) {
		return RetainedOutput{}, ErrIncomplete
	}
	for _, item := range r.Outputs {
		if item.ID == output {
			return clone(item), nil
		}
	}
	return RetainedOutput{}, store.ErrNotFound
}

// Cancel persists intent; a still-running native query is not claimed stopped.
func (s *Runs) Cancel(ctx context.Context, e identity.Envelope, id string) (RunView, error) {
	if s == nil || ctx == nil {
		return RunView{}, ErrInvalid
	}
	return s.repo.CancelFrozenRun(ctx, e, id)
}

// Expire performs an explicitly authorized bounded retention pass. Ordinary GET
// requests do not mutate storage, execute old queries or extend expiry.
func (s *Runs) Expire(ctx context.Context, e identity.Envelope, limit int) (int64, error) {
	if s == nil || ctx == nil || limit < 1 || limit > 1000 {
		return 0, ErrInvalid
	}
	return s.repo.ExpireFrozenArtifacts(ctx, e, limit)
}
