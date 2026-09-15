package reporting

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

func (s *Compositions) checkpoint(ctx context.Context, inv jobs.Invocation, w CompositionWrite) (CompositionRecord, error) {
	proof, err := prepareCompositionWrite(inv, w)
	if err != nil {
		return CompositionRecord{}, err
	}
	return s.repo.CheckpointComposition(ctx, inv, proof)
}

func failedGroup(g CompositionGroup, code string) GroupResult {
	out := GroupResult{Group: g.ID, Kind: g.Kind, State: "failed", Code: code, Outputs: []RetainedOutput{}}
	out.Digest = GroupResultDigest(out)
	return out
}

func strictCompositionOmission(m CompositionManifest) bool {
	if m.Policy != "fail_closed" {
		return false
	}
	if m.Redacted {
		return true
	}
	for _, page := range m.Pages {
		for _, widget := range page.Widgets {
			if widget.Code != "" {
				return true
			}
		}
	}
	return false
}

func compositionExecutionCode(err error) string {
	if errors.Is(err, exec.ErrUncertain) {
		return "query_indeterminate"
	}
	return compositionFailure(err)
}

// Run performs one explicit bounded attempt with current signed authority.
// Completed checkpoints are read back, not regenerated on a replay.
func (s *Compositions) Run(ctx context.Context, e identity.Envelope, id string, resume bool) (CompositionView, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) {
		return CompositionView{}, ErrInvalid
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	record, err := s.repo.ReadComposition(ctx, e, id)
	if err != nil {
		return CompositionView{}, err
	}
	view := SummarizeComposition(record)
	if slices.Contains([]string{"completed", "partial"}, record.State) {
		return view, nil
	}
	if record.State == "failed" || record.State == "cancelled" {
		return view, ErrIncomplete
	}
	if record.State == "expired" || !time.Now().Before(record.Manifest.Expires) {
		return view, ErrExpired
	}
	if err := RequireComposition(e, record.Manifest); err != nil {
		return CompositionView{}, err
	}
	task, err := s.runner.Inspect(ctx, e, id)
	if err != nil {
		return view, err
	}
	if resume {
		task, err = s.runner.Resume(ctx, e, id)
		if err != nil {
			return view, err
		}
	}
	timeout := min(time.Duration(record.Manifest.Limits.Timeout), time.Duration(s.documents.limits.Composition.Timeout))
	_, runErr := s.runner.Run(ctx, e, task, timeout, func(work context.Context, inv jobs.Invocation) error {
		return s.continueComposition(work, e, inv, record)
	})
	// A bounded authorized read resolves a lost commit reply. It never supplies
	// fresh execution authority or extends the verified envelope's deadline.
	recovery, finish := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	defer finish()
	current, readErr := s.repo.ReadComposition(recovery, e, id)
	if readErr != nil {
		if runErr != nil {
			return view, runErr
		}
		return view, readErr
	}
	view = SummarizeComposition(current)
	if slices.Contains([]string{"completed", "partial"}, current.State) {
		return view, nil
	}
	if runErr != nil {
		return view, runErr
	}
	return view, ErrIncomplete
}

func (s *Compositions) continueComposition(ctx context.Context, e identity.Envelope, inv jobs.Invocation, record CompositionRecord) error {
	m := record.Manifest
	omit := strictCompositionOmission(m)
	completed := map[string]bool{}
	for _, result := range record.Results {
		completed[result.Group] = true
		omit = omit || m.Policy == "fail_closed" && result.State != "completed"
	}
	for _, group := range m.Groups {
		if completed[group.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := inv.Current(m.Kind+".run", m.Document, m.RequestHash); err != nil {
			return err
		}
		currentLimits := s.documents.limits.Composition
		var result GroupResult
		var err error
		switch {
		case omit:
			result = failedGroup(group, "strict_omission")
		case len(completed) >= currentLimits.MaxQueries:
			result = failedGroup(group, "budget_exhausted")
		case group.Kind == "query" && !currentLimits.LiveQueries:
			result = failedGroup(group, "live_queries_disabled")
		case group.Kind == "query" && group.Query.Durability == "session_bound" && !currentLimits.SessionBound:
			result = failedGroup(group, "session_bound_disabled")
		case group.Kind == "block":
			result, err = s.executeBlock(ctx, e, inv, m, group)
		default:
			result, record, err = s.executeQuery(ctx, e, inv, record, group)
		}
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if !e.Valid() {
				return access.ErrUnauthenticated
			}
			// A lost child checkpoint reply or metadata outage is not proof of
			// a failed query. Preserve the manifest and durable child evidence
			// for explicit resume instead of sealing a permanent failed group.
			if errors.Is(err, store.ErrUnavailable) {
				return err
			}
			result = failedGroup(group, compositionExecutionCode(err))
		}
		result.Digest = GroupResultDigest(result)
		updated, err := s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "group", Group: group.ID, Result: &result})
		if errors.Is(err, ErrBudget) {
			// The refused value transaction rolled back. Its reserved error space
			// remains available; never silently publish an empty successful widget.
			result = failedGroup(group, "budget_exhausted")
			updated, err = s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "group", Group: group.ID, Result: &result})
		}
		if err != nil {
			return err
		}
		record = updated
		completed[group.ID] = true
		omit = omit || m.Policy == "fail_closed" && result.State != "completed"
	}
	state, code, _, err := CompositionCompletion(m, record.Results)
	if err != nil {
		return err
	}
	_, err = s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "complete", Outcome: state, Code: code})
	return err
}

func (s *Compositions) executeBlock(ctx context.Context, e identity.Envelope, inv jobs.Invocation, m CompositionManifest, g CompositionGroup) (GroupResult, error) {
	if s.runs == nil || s.documents.blocks == nil {
		return GroupResult{}, ErrUnavailable
	}
	snapshot, err := s.documents.blocks.repo.ReadBlock(ctx, e, g.Block, Reference{Revision: g.Revision}, Execute)
	if err != nil {
		return GroupResult{}, err
	}
	if err := CheckCompositionBlock(e, g, snapshot); err != nil {
		return GroupResult{}, err
	}
	policy := g.Policy
	if m.Private {
		policy = "private_preview"
	}
	child, err := s.runs.admit(ctx, e, g.Block, RunRequest{Key: "composition:" + m.ID + ":" + g.ID,
		Reference: Reference{Revision: g.Revision}, Arguments: g.Arguments, Resolution: g.Resolution,
		Outputs: g.Outputs, Policy: policy, Locale: g.Locale, Narrative: g.Narrative, PartialPolicy: "allow_partial"}, &inv)
	if err != nil {
		return GroupResult{}, err
	}
	if child.RevisionDigest != g.Definition || child.PartitionDigest != exec.Hash(g.Binding) || child.Private != m.Private {
		return GroupResult{}, ErrStale
	}
	resume := child.Attempts > 0 && !slices.Contains([]string{"succeeded", "partial", "failed", "expired"}, child.State)
	if _, err := s.runs.run(ctx, e, child.ID, resume, &inv); err != nil {
		return GroupResult{}, err
	}
	retained, err := s.runs.repo.ReadFrozenRun(ctx, e, child.ID, true)
	if err != nil {
		return GroupResult{}, err
	}
	if retained.Manifest == nil || retained.Result == nil || retained.View.Observed == nil || retained.Manifest.Revision.Digest != g.Definition || retained.Manifest.Binding.Context != g.Binding.Context || exec.Hash(retained.Manifest.Binding) != exec.Hash(g.Binding) || digest(retained.Manifest.Resolved.Parameters) != digest(g.Resolved.Parameters) || retained.View.Private != m.Private {
		return GroupResult{}, ErrStale
	}
	// The original widget policy remains checked even for a private child run.
	snapshot, err = s.documents.blocks.repo.ReadBlock(ctx, e, g.Block, Reference{Revision: g.Revision}, Execute)
	if err != nil {
		return GroupResult{}, err
	}
	if err := CheckCompositionBlock(e, g, snapshot); err != nil {
		return GroupResult{}, err
	}
	out := GroupResult{Group: g.ID, Kind: "block", State: "completed", ChildRun: child.ID,
		Block: &retained.View, Outputs: clone(retained.Outputs), Observed: clone(retained.View.Observed)}
	for _, output := range out.Outputs {
		if output.State != "succeeded" {
			out.State, out.Code = "partial", "output_failed"
		}
	}
	if retained.Result.Outcome == "truncated" {
		out.State, out.Code = "partial", "query_truncated"
	}
	out.Digest = GroupResultDigest(out)
	return out, CheckCompositionResult(m, g, out)
}

func (s *Compositions) executeQuery(ctx context.Context, e identity.Envelope, inv jobs.Invocation, record CompositionRecord, g CompositionGroup) (GroupResult, CompositionRecord, error) {
	m := record.Manifest
	if s.queries == nil || g.Query == nil || g.Origin == nil {
		return GroupResult{}, record, ErrUnavailable
	}
	plan, planned := record.Plans[g.ID]
	if !planned {
		var err error
		operation := "composition:" + m.ID + ":" + g.ID
		if record.Started[g.ID] {
			recovery, ok := s.queries.(DocumentQueryRecovery)
			if !ok {
				return GroupResult{}, record, exec.ErrUncertain
			}
			// A prior owner may have spent the generation budget. Recovery is
			// strictly metadata-only and cannot silently start another model call.
			plan, err = recovery.RecoverDocumentQuery(ctx, e, *g.Query, *g.Origin, operation)
			if err != nil {
				return GroupResult{}, record, errors.Join(exec.ErrUncertain, err)
			}
		} else {
			updated, startErr := s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "query_start", Group: g.ID})
			if startErr != nil {
				// Only this invocation attempted the marker; no provider call has
				// occurred yet. Confirm its durable receipt before dispatching.
				updated, err = s.repo.ReadComposition(ctx, e, m.ID)
				if err != nil || !updated.Started[g.ID] {
					return GroupResult{}, record, startErr
				}
			}
			record = updated
			plan, err = s.queries.PrepareDocumentQuery(ctx, e, *g.Query, *g.Origin, operation, g.Locale)
			if err != nil {
				return GroupResult{}, record, err
			}
		}
		if plan.BindingDigest != exec.Hash(g.Binding) {
			return GroupResult{}, record, ErrStale
		}
		updated, err := s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "plan", Group: g.ID, Plan: &plan})
		if err != nil {
			updated, readErr := s.repo.ReadComposition(ctx, e, m.ID)
			if readErr != nil || updated.Plans[g.ID] != plan {
				return GroupResult{}, record, err
			}
			record = updated
		} else {
			record = updated
		}
	}
	available := int64(m.Limits.MaxRetainedBytes) - CompositionRetainedBytes(record) - int64(len(m.Groups)-len(record.Results))*1024
	if available < 1024 {
		return GroupResult{}, record, ErrBudget
	}
	rows := min(m.ArtifactLimits.MaxRows, s.documents.limits.Execution.MaxRows)
	bytes := min(m.ArtifactLimits.MaxResultBytes, s.documents.limits.Execution.MaxResultBytes, int(available/2))
	result, err := s.queries.RunDocumentQuery(ctx, e, *g.Query, *g.Origin, plan, rows, bytes, m.Private)
	if err != nil {
		return GroupResult{}, record, err
	}
	if result.Execution.Result == nil || result.Execution.Attempt.Finished == nil || result.Partition != exec.Hash(g.Binding) {
		return GroupResult{}, record, ErrIncomplete
	}
	out := GroupResult{Group: g.ID, Kind: "query", State: "completed", Outputs: []RetainedOutput{},
		Query: &result, QueryPlan: &plan, Observed: clone(result.Execution.Attempt.Finished)}
	if result.Execution.Result.Outcome == "truncated" {
		out.State, out.Code = "partial", "query_truncated"
	}
	if result.EvidenceStale {
		out.State, out.Code = "partial", "dependency_stale"
	}
	out.Digest = GroupResultDigest(out)
	return out, record, CheckCompositionResult(m, g, out)
}
