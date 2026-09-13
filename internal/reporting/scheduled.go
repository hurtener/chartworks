package reporting

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

// Scheduled consumes the existing queue's accepted occurrence and fresh Pengui
// proof. It owns no worker, token broker, query engine or alternate result store.
type Scheduled struct {
	delivery *Delivery
	usage    jobs.ReportingUsageRepository
}

// ScheduledRunRepository seals a frozen artifact under the already owned queue
// fence. A serialized task or a freshly supplied user token is insufficient.
type ScheduledRunRepository interface {
	SealScheduledFrozenRun(context.Context, jobs.Invocation, PreparedRun) (RunRecord, error)
}

// ScheduledCompositionRepository is the corresponding composition seam.
type ScheduledCompositionRepository interface {
	SealScheduledComposition(context.Context, jobs.Invocation, PreparedComposition) (CompositionRecord, error)
}

// NewScheduled installs real persistence consumers before a worker starts.
func NewScheduled(delivery *Delivery, usage jobs.ReportingUsageRepository) (*Scheduled, error) {
	if delivery == nil || nilValue(usage) {
		return nil, ErrInvalid
	}
	if _, ok := delivery.runs.repo.(ScheduledRunRepository); !ok {
		return nil, ErrInvalid
	}
	if _, ok := delivery.compositions.repo.(ScheduledCompositionRepository); !ok {
		return nil, ErrInvalid
	}
	return &Scheduled{delivery: delivery, usage: usage}, nil
}

func scheduledArguments(in []jobs.ReportingArgument) []Argument {
	out := make([]Argument, len(in))
	for i, a := range in {
		out[i] = Argument{Name: a.Name, Value: Value{Literal: a.Value.Literal}}
		if p := a.Value.Period; p != nil {
			out[i].Value.Period = &Period{Mode: p.Mode, Unit: p.Unit, Count: p.Count,
				Start: p.Start, End: p.End, FromDate: p.FromDate, FirstOccurrence: p.FirstOccurrence,
				DSTPolicy: p.DSTPolicy, MonthPolicy: p.MonthPolicy}
		}
	}
	return out
}

func scheduledResolution(j jobs.Job) Resolution {
	out := Resolution{At: j.DueAt, Timezone: j.Reporting.Target.Timezone}
	if j.WindowStart.Before(j.WindowEnd) {
		out.ScheduleWindow = &Window{Start: j.WindowStart, End: j.WindowEnd}
	}
	return out
}

func scheduledFrozenRequest(j jobs.Job) RunRequest {
	t := j.Reporting.Target
	policy, partial := "published", "fail"
	if t.Type == "block" {
		policy = "certified_only"
	}
	if t.PartialFailure == "allow_partial" {
		partial = "allow_partial"
	}
	return RunRequest{Key: "occurrence:" + j.ID, Reference: Reference{Revision: j.Reporting.Revision},
		Arguments: scheduledArguments(t.Arguments), Resolution: scheduledResolution(j), Outputs: slices.Clone(t.Outputs),
		Policy: policy, Locale: t.Locale, Narrative: t.Narrative, PartialPolicy: partial}
}

func scheduledCompositionRequest(j jobs.Job) CompositionRequest {
	t := j.Reporting.Target
	policy := t.PartialFailure
	if policy == "" {
		policy = "fail_closed"
	}
	return CompositionRequest{Key: "occurrence:" + j.ID, Reference: DocumentReference{Revision: j.Reporting.Revision},
		Resolution: scheduledResolution(j), PartialFailure: policy,
		Pages: []PageInput{{Page: "main", Filters: scheduledArguments(t.Arguments)}}}
}

func scheduledExecutionLimits(original config.ReportingExecution, b jobs.ReportingBudget) config.ReportingExecution {
	out := original
	out.Timeout = min(out.Timeout, config.Duration(time.Duration(b.TimeoutMillis)*time.Millisecond))
	out.MaxRows = min(out.MaxRows, b.MaxRows)
	out.PageRows = min(out.PageRows, out.MaxRows)
	out.MaxResultBytes = min(out.MaxResultBytes, b.MaxBytes)
	// Zero model allowance is enforced at every real SDK attempt. Keep valid
	// positive type-system bounds here; the target separately prohibits narrative.
	if b.ModelCalls > 0 {
		out.NarrativeCalls = min(out.NarrativeCalls, b.ModelCalls)
		out.NarrativeTokens = min(out.NarrativeTokens, b.ModelTokens)
	}
	out.NarrativeTimeout = min(out.NarrativeTimeout, out.Timeout)
	return out
}

func (s *Scheduled) bounded(t jobs.ReportingTarget) (*Runs, *Compositions) {
	runs := *s.delivery.runs
	runs.limits = scheduledExecutionLimits(runs.limits, t.Budget)
	documents := *s.delivery.documents
	documents.limits.Execution = runs.limits
	documents.limits.Composition.Timeout = min(documents.limits.Composition.Timeout, runs.limits.Timeout)
	documents.limits.Composition.MaxQueries = min(documents.limits.Composition.MaxQueries, t.Budget.QueryAttempts)
	documents.limits.Composition.LiveQueries = documents.limits.Composition.LiveQueries && t.Dynamic
	documents.limits.Composition.SessionBound = false
	compositions := *s.delivery.compositions
	compositions.documents, compositions.runs = &documents, &runs
	return &runs, &compositions
}

// scheduledDefinition selects only the explicitly named saved-question widget
// or the full existing report. It applies occurrence-pinned block revisions to
// a detached in-memory projection, never edits or creates a report definition.
func scheduledDefinition(d DocumentDefinition, dispatch *jobs.ReportingDispatch) (DocumentDefinition, error) {
	if dispatch == nil {
		return d, nil
	}
	t := dispatch.Target
	if dispatch.Blocked != "" || t.ResourceKind() != "report" || d.Timezone != t.Timezone || d.Locale != t.Locale {
		return DocumentDefinition{}, ErrStale
	}
	d = clone(d)
	pins := map[string]jobs.ReportingPin{}
	for _, p := range dispatch.Pins {
		pins[p.ID] = p
	}
	widgets := make([]Widget, 0, len(d.Widgets))
	for _, w := range d.Widgets {
		if t.Type == "saved_question" && w.ID != t.Widget {
			continue
		}
		if t.Type == "saved_question" && w.Kind != "query" {
			return DocumentDefinition{}, ErrStale
		}
		if w.Kind == "query" && (!t.Dynamic || w.Query == nil || w.Query.Durability != "replayable") {
			return DocumentDefinition{}, ErrUnavailable
		}
		if w.Kind == "block" {
			p, found := pins[w.ID]
			if !found || w.Block == nil || p.Block != w.Block.Block || w.Block.Revision > 0 && w.Block.Revision != p.Revision || w.Block.Narrative && !t.Narrative {
				return DocumentDefinition{}, ErrStale
			}
			w.Block.Revision = p.Revision
			delete(pins, w.ID)
		}
		widgets = append(widgets, w)
	}
	if len(widgets) == 0 || len(pins) != 0 {
		return DocumentDefinition{}, ErrStale
	}
	d.Widgets = widgets
	return d, nil
}

func (s *Scheduled) inspectBlock(ctx context.Context, e identity.Envelope, runs *Runs, id string, revision int64, policy string, outputs []string, arguments []Argument, resolution Resolution, narrative bool) error {
	if !runs.blocks.CanValidate() {
		return ErrUnavailable
	}
	if err := Require(e, id, Execute); err != nil {
		return err
	}
	snapshot, err := runs.blocks.repo.ReadBlock(ctx, e, id, Reference{Revision: revision}, Execute)
	if err != nil {
		return err
	}
	if err := runEligibility(e, snapshot, policy, time.Now()); err != nil {
		return err
	}
	if _, err := runs.selectRunOutputs(snapshot.Revision.Definition, RunRequest{Outputs: outputs, Narrative: narrative, PartialPolicy: "allow_partial"}); err != nil {
		return err
	}
	if _, err := ResolveParameters(snapshot.Revision.Definition.Parameters, arguments, resolution); err != nil {
		return err
	}
	_, _, err = runs.blocks.resolveDefinitions(ctx, e, snapshot.Revision.Definition, true)
	if err != nil {
		return err
	}
	binding, err := runs.blocks.sources.ContextBinding(ctx, e, snapshot.Revision.Definition.Source, snapshot.Revision.Definition.Context)
	if err != nil {
		return err
	}
	if snapshot.Validation == nil || exec.Hash(binding) != snapshot.Validation.BindingDigest {
		return ErrStale
	}
	return nil
}

// ValidateScheduledReporting checks the creator's supplied target and actual
// dependency reach, with zero model/query execution. The sample window here is
// only shape validation; actual windows come from accepted occurrence instants.
func (s *Scheduled) ValidateScheduledReporting(ctx context.Context, e identity.Envelope, target jobs.ReportingTarget) error {
	if s == nil || ctx == nil || !target.Valid() {
		return ErrInvalid
	}
	if err := target.Require(e); err != nil {
		return err
	}
	if _, err := namedZone(target.Timezone); err != nil {
		return err
	}
	ctx, stop := context.WithDeadline(ctx, e.Deadline())
	defer stop()
	runs, _ := s.bounded(target)
	at := time.Now().UTC().Truncate(time.Microsecond)
	resolution := Resolution{At: at, Timezone: target.Timezone, ScheduleWindow: &Window{Start: at.Add(-time.Hour), End: at}}
	if target.ResourceKind() == "block" {
		policy := "published"
		if target.Type == "block" {
			policy = "certified_only"
		}
		return s.inspectBlock(ctx, e, runs, target.ID, target.Revision, policy, target.Outputs, scheduledArguments(target.Arguments), resolution, target.Narrative)
	}
	snapshot, err := s.delivery.documents.repo.ReadDocument(ctx, e, "report", target.ID, DocumentReference{Revision: target.Revision}, Execute, false)
	if err != nil {
		return err
	}
	if snapshot.PublishedAt == nil || snapshot.State.Archived {
		return ErrStale
	}
	d, err := ProjectStoredDocument(snapshot.Revision.Raw, "report")
	if err != nil {
		return err
	}
	if d.Locale != target.Locale || d.Timezone != target.Timezone || target.Type == "saved_question" && len(target.Arguments) != 0 {
		return ErrInvalid
	}
	if target.PartialFailure == "allow_partial" && d.PartialFailure != "allow_partial" {
		return ErrInvalid
	}
	if _, err := resolveReportFilters(d.Filters, scheduledArguments(target.Arguments)); err != nil {
		return err
	}
	found := target.Type != "saved_question"
	for _, w := range d.Widgets {
		if target.Type == "saved_question" && w.ID != target.Widget {
			continue
		}
		found = true
		switch w.Kind {
		case "query":
			if !target.Dynamic || !s.delivery.documents.limits.Composition.LiveQueries || s.delivery.documents.queries == nil || w.Query.Durability != "replayable" {
				return ErrUnavailable
			}
			if _, err := s.delivery.documents.queries.InspectDocumentQuery(ctx, e, *w.Query); err != nil {
				return err
			}
		case "block":
			if target.Type == "saved_question" || w.Block.Narrative && !target.Narrative {
				return ErrInvalid
			}
			block, err := runs.blocks.repo.ReadBlock(ctx, e, w.Block.Block, Reference{Revision: w.Block.Revision}, Execute)
			if err != nil {
				return err
			}
			arguments, _, err := widgetArguments(block.Revision.Definition.Parameters, d, w, scheduledArguments(target.Arguments), nil, resolution)
			if err != nil {
				return err
			}
			policy := w.Block.Policy
			if policy == "" {
				policy = "published"
			}
			if err := s.inspectBlock(ctx, e, runs, w.Block.Block, block.Revision.Number, policy, w.Block.Outputs, arguments, resolution, w.Block.Narrative); err != nil {
				return err
			}
		case "text":
			if target.Type == "saved_question" {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	}
	if !found {
		return ErrInvalid
	}
	return ctx.Err()
}

func scheduledError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, store.ErrUnavailable) {
		return err
	}
	if errors.Is(err, jobs.ErrReportingBudget) || errors.Is(err, gateway.ErrBudget) || errors.Is(err, ErrBudget) {
		return errors.Join(jobs.ErrReportingBudget, err)
	}
	if errors.Is(err, access.ErrUnauthenticated) || errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) || errors.Is(err, jobs.ErrAuthority) {
		return errors.Join(jobs.ErrAuthority, err)
	}
	return errors.Join(jobs.ErrReportingAttention, err)
}

// ExecuteScheduledReporting enters the real frozen/composition consumers with
// the queue's original lease. No inner root task, ambient token, hidden report,
// regenerated result, or floating-publication lookup is used on retry.
func (s *Scheduled) ExecuteScheduledReporting(ctx context.Context, lease jobs.Lease, proof auth.Execution) (err error) {
	if s == nil || ctx == nil || jobs.AssertExecution(proof, lease.Job) != nil || lease.Job.Kind != jobs.ReportingKind {
		return jobs.ErrAuthority
	}
	defer func() { err = scheduledError(err) }()
	j := lease.Job
	if j.Reporting.Blocked != "" {
		return jobs.ErrReportingAttention
	}
	e := proof.Envelope()
	work, cancel := context.WithTimeout(ctx, time.Duration(j.Reporting.Target.Budget.TimeoutMillis)*time.Millisecond)
	defer cancel()
	task, err := s.delivery.runs.runner.Inspect(work, e, j.ID)
	if err != nil {
		return err
	}
	inv, err := jobs.ReportingInvocation(proof, lease, task)
	if err != nil {
		return err
	}
	work, err = gateway.WithAttemptReservation(work, func(callCtx context.Context, call gateway.Call, tokens int) error {
		if !call.Valid() || call.Tenant() != j.Tenant {
			return jobs.ErrAuthority
		}
		return s.usage.ReserveReportingUsage(callCtx, inv, jobs.ReportingCharge{ModelCalls: 1, ModelTokens: tokens})
	})
	if err != nil {
		return err
	}
	work, err = exec.WithAttemptReservation(work, func(callCtx context.Context, current identity.Envelope, _ exec.Options) error {
		if !current.Valid() || current.Tenant() != j.Tenant || current.User() != j.Executor || current.Session() != j.ID {
			return jobs.ErrAuthority
		}
		return s.usage.ReserveReportingUsage(callCtx, inv, jobs.ReportingCharge{Queries: 1})
	})
	if err != nil {
		return err
	}
	runs, compositions := s.bounded(j.Reporting.Target)
	if j.Reporting.Target.ResourceKind() == "block" {
		input, err := normalizedRunRequest(scheduledFrozenRequest(j))
		if err != nil {
			return err
		}
		if _, err := runs.seal(work, e, j.Reporting.Target.ID, input, task.Input.InputHash, task, &inv); err != nil {
			return err
		}
		record, err := runs.repo.ReadFrozenRun(work, e, j.ID, true)
		if err != nil {
			return err
		}
		if record.Manifest == nil || record.Manifest.Revision.Digest != j.Reporting.Digest {
			return ErrStale
		}
		return runs.continueFrozen(work, e, inv, record)
	}
	record, err := compositions.repo.ReadComposition(work, e, j.ID)
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, access.ErrNotFound) {
		input, normalizeErr := normalizeCompositionRequest(scheduledCompositionRequest(j))
		if normalizeErr != nil {
			return normalizeErr
		}
		manifest, resolveErr := compositions.resolve(work, e, task, "report", j.Reporting.Target.ID, task.Input.InputHash, input)
		if resolveErr != nil {
			return resolveErr
		}
		prepared, prepareErr := prepareComposition(e, manifest)
		if prepareErr != nil {
			return prepareErr
		}
		record, err = compositions.repo.(ScheduledCompositionRepository).SealScheduledComposition(work, inv, prepared)
	}
	if err != nil {
		return err
	}
	if record.Manifest.Digest != j.Reporting.Digest || record.Manifest.RequestHash != task.Input.InputHash {
		return ErrStale
	}
	if err := RequireComposition(e, record.Manifest); err != nil {
		return err
	}
	return compositions.continueComposition(work, e, inv, record)
}

var _ jobs.ReportingExecutor = (*Scheduled)(nil)
