package reporting

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

func (s *Runs) pinnedPlan(ctx context.Context, e identity.Envelope, m RunManifest) (exec.Plan, error) {
	if !s.blocks.CanValidate() {
		return exec.Plan{}, ErrUnavailable
	}
	if err := RequireRunManifest(e, m); err != nil {
		return exec.Plan{}, err
	}
	snapshot, err := s.blocks.repo.ReadBlock(ctx, e, m.Block, Reference{Revision: m.Revision.Number}, Execute)
	if err != nil {
		return exec.Plan{}, err
	}
	if snapshot.Revision.Digest != m.Revision.Digest {
		return exec.Plan{}, ErrStale
	}
	if err = runEligibility(e, snapshot, m.Policy, time.Now()); err != nil {
		return exec.Plan{}, err
	}
	d := m.Revision.Definition
	definitions, _, err := s.blocks.resolveDefinitions(ctx, e, d, true)
	if err != nil {
		return exec.Plan{}, err
	}
	binding, err := s.blocks.sources.ContextBinding(ctx, e, d.Source, d.Context)
	if err != nil {
		return exec.Plan{}, err
	}
	if exec.Hash(binding) != exec.Hash(m.Binding) {
		return exec.Plan{}, ErrStale
	}
	scope, err := validationScope(binding, definitions)
	if err != nil {
		return exec.Plan{}, err
	}
	plan, err := s.blocks.validator.ValidateWithin(ctx, e, exec.Request{Source: d.Source, Context: d.Context, SQL: d.SQL, Parameters: m.Resolved.Parameters}, scope)
	if err != nil {
		return exec.Plan{}, err
	}
	statement, parameters, err := plan.SQL(e, binding)
	if err != nil || statement != d.SQL || parameterDigest(parameters) != parameterDigest(m.Resolved.Parameters) {
		return exec.Plan{}, ErrStale
	}
	dependencies, err := deriveDependencies(binding, scope, plan.Receipt().Dependencies)
	if err != nil {
		return exec.Plan{}, err
	}
	if DependencyDigest(dependencies, d.Topics) != DependencyDigest(m.Dependencies, d.Topics) {
		return exec.Plan{}, ErrStale
	}
	return plan, nil
}

func (s *Runs) writeRun(ctx context.Context, e identity.Envelope, inv jobs.Invocation, w RunWrite) (RunRecord, error) {
	proof, err := prepareRunWrite(e, w)
	if err != nil {
		return RunRecord{}, err
	}
	return s.repo.CheckpointFrozenRun(ctx, inv, proof)
}

func frozenOutcome(outputs []RetainedOutput, policy string) (string, string) {
	failed := false
	for _, output := range outputs {
		failed = failed || output.State != "succeeded"
	}
	if !failed {
		return "succeeded", ""
	}
	if policy == "allow_partial" {
		return "partial", "output_failed"
	}
	return "failed", "output_failed"
}

func (s *Runs) queryFrozen(ctx context.Context, e identity.Envelope, inv jobs.Invocation, m RunManifest) (RunRecord, error) {
	current, err := s.repo.ReadFrozenRun(ctx, e, m.ID, true)
	if err != nil {
		return RunRecord{}, err
	}
	number, err := nextFrozenQueryNumber(current.View.QueryAttempts)
	if err != nil {
		return RunRecord{}, err
	}
	caps, err := RuntimeQueryLimits(m, s.limits)
	if err != nil {
		return RunRecord{}, err
	}
	if s.queryAttempts > 0 {
		caps.QueryAttempts = min(caps.QueryAttempts, s.queryAttempts)
	}
	if number > caps.QueryAttempts {
		return RunRecord{}, ErrBudget
	}
	queryCtx, stop := context.WithTimeout(ctx, time.Duration(caps.TimeoutMillis)*time.Millisecond)
	defer stop()
	plan, err := s.pinnedPlan(queryCtx, e, m)
	if err != nil {
		return RunRecord{}, err
	}
	report, executeErr := s.blocks.executor.Execute(queryCtx, e, plan, exec.Options{Operation: m.ID, Number: number,
		Preview: m.Private, Rows: caps.MaxRows, Bytes: caps.MaxBytes})
	if report.Attempt.ID != "" {
		if _, err = s.writeRun(ctx, e, inv, RunWrite{Kind: "attempt", Manifest: m, Attempt: &report.Attempt}); err != nil {
			return RunRecord{}, err
		}
	}
	if executeErr != nil {
		return RunRecord{}, executeErr
	}
	if report.Result == nil || !successful(report.Attempt.Status) {
		if report.Attempt.Status == "uncertain" || report.Attempt.RemoteState == "unknown" || report.Attempt.RemoteState == "running" {
			return RunRecord{}, exec.ErrUncertain
		}
		return RunRecord{}, ErrIncomplete
	}
	if report.Attempt.Number != number || report.Attempt.Manifest.Receipt.Manifest != plan.Receipt().Manifest {
		return RunRecord{}, ErrInvalid
	}
	if err = CheckFrozenResult(ctx, m, *report.Result, report.Attempt); err != nil {
		return RunRecord{}, err
	}
	return s.writeRun(ctx, e, inv, RunWrite{Kind: "result", Manifest: m, Result: report.Result, Attempt: &report.Attempt})
}

func (s *Runs) makeOutput(ctx context.Context, e identity.Envelope, inv jobs.Invocation, m RunManifest, result exec.Result, saved Output, reservedCalls, reservedTokens int) (RetainedOutput, error) {
	out := RetainedOutput{ID: saved.ID, Kind: saved.Kind, State: "succeeded", Intent: clone(saved.Intent)}
	if saved.Intent != nil && !saved.Intent.Enabled {
		return out, selectionError("output_disabled")
	}
	if m.Selection != nil {
		out.ResultPolicy = clone(m.ResultPolicy)
	}
	if saved.Narrative != nil && m.Selection != nil {
		out.EvidencePolicy = narrativePolicy(m.ResultPolicy, *saved.Narrative)
	}
	if saved.Kind != "narrative" {
		if saved.Mapping == nil {
			return out, ErrInvalid
		}
		chart, err := s.buildRetainedChart(ctx, m, result, *saved.Mapping)
		if err != nil {
			out.State, out.Code = "failed", "output_failed"
		} else {
			out.Chart = &chart
		}
	} else {
		n := saved.Narrative
		available := n != nil && s.model != nil && m.Model == s.modelVersion && n.ModelVersion == s.modelVersion && n.SchemaVersion == "grounded-narrative-v1"
		versioned := m.Revision.Definition.SchemaVersion == CurrentSchemaVersion || n != nil && n.PolicyVersion != ""
		if n == nil || !available && !versioned {
			// Preserve the legacy unavailable receipt. Versioned policies first
			// resolve deterministic evidence exclusions, even without a model.
			out.State, out.Code = "failed", "narrative_unavailable"
		} else {
			prepared, err := prepareNarrative(m, result, *n)
			if err != nil {
				out.State, out.Code = "failed", "narrative_evidence_unavailable"
				if errors.Is(err, ErrBudget) {
					out.Code = "narrative_budget_exhausted"
				}
				if errors.Is(err, ErrNarrativePolicy) {
					out.Code = "narrative_policy_unsupported"
				}
				out.Digest = out.ContentDigest()
				return out, nil
			}
			if !available {
				out.State, out.Code = "failed", "narrative_unavailable"
				out.Digest = out.ContentDigest()
				return out, nil
			}
			// Check current and originally accepted cumulative limits only for
			// eligible evidence. An irrelevant offline model or smaller model
			// budget cannot change a deterministic no-evidence disposition.
			if reservedCalls+n.MaxCalls > min(m.Limits.NarrativeCalls, s.limits.NarrativeCalls) || reservedTokens+n.MaxTokens > min(m.Limits.NarrativeTokens, s.limits.NarrativeTokens) {
				return out, ErrBudget
			}
			// Persist reservations before the SDK may accept a request. A crash
			// here leaves an honest indeterminate output, not free retry budget.
			start := RetainedOutput{ResultPolicy: clone(out.ResultPolicy), ID: saved.ID, Kind: "narrative", Intent: clone(saved.Intent), EvidencePolicy: clone(out.EvidencePolicy), State: "indeterminate", Code: "narrative_indeterminate", ReservedCalls: n.MaxCalls, ReservedTokens: n.MaxTokens}
			if _, err := s.writeRun(ctx, e, inv, RunWrite{Kind: "output_start", Manifest: m, Output: &start}); err != nil {
				return out, err
			}
			out.ReservedCalls, out.ReservedTokens = start.ReservedCalls, start.ReservedTokens
			narrative, err := s.generatePreparedNarrative(ctx, e, m, saved.ID, *n, prepared)
			if err != nil {
				out.State, out.Code = "failed", "narrative_failed"
				out.Narrative = &narrative
			} else {
				out.Narrative = &narrative
			}
		}
	}
	out.Digest = digest(out)
	return out, nil
}

func (s *Runs) continueFrozen(ctx context.Context, e identity.Envelope, inv jobs.Invocation, initial RunRecord) error {
	if initial.Manifest == nil {
		return ErrExpired
	}
	m := *initial.Manifest
	if err := RequireRunManifest(e, m); err != nil {
		return err
	}
	if !time.Now().Before(m.Expires) {
		return ErrExpired
	}
	snapshot, err := s.blocks.repo.ReadBlock(ctx, e, m.Block, Reference{Revision: m.Revision.Number}, Execute)
	if err != nil {
		return err
	}
	if snapshot.Revision.Digest != m.Revision.Digest {
		return ErrStale
	}
	if err = runEligibility(e, snapshot, m.Policy, time.Now()); err != nil {
		return err
	}
	current, err := s.repo.ReadFrozenRun(ctx, e, m.ID, true)
	if err != nil {
		return err
	}
	if current.Result == nil && m.ReuseMaxAge > 0 && len(current.View.QueryAttempts) == 0 {
		// The repository rechecks present pins and reach without warehouse/model I/O.
		var reused bool
		current, reused, err = s.repo.ReuseFrozenRun(ctx, inv, m.ID, s.limits)
		if err != nil {
			return err
		}
		if reused {
			return nil
		}
	}
	if current.Result == nil {
		current, err = s.queryFrozen(ctx, e, inv, m)
		if err != nil {
			return err
		}
	}
	if current.Result == nil {
		return ErrIncomplete
	}
	if err := CheckRuntimeResult(m, s.limits, *current.Result); err != nil {
		return err
	}
	byID := make(map[string]RetainedOutput, len(current.Outputs))
	for _, output := range current.Outputs {
		byID[output.ID] = output
	}
	for _, saved := range m.Outputs {
		if err = ctx.Err(); err != nil {
			return err
		}
		if existing, ok := byID[saved.ID]; ok {
			if existing.State != "indeterminate" {
				continue
			}
			// The previous SDK attempt may have been accepted remotely. Do not
			// silently regenerate its text on a later request/worker attempt.
			existing.State, existing.Code, existing.Digest = "failed", "narrative_indeterminate", ""
			existing.Digest = digest(existing)
			if _, err = s.writeRun(ctx, e, inv, RunWrite{Kind: "output", Manifest: m, Output: &existing}); err != nil {
				return err
			}
			byID[saved.ID] = existing
			continue
		}
		calls, tokens := 0, 0
		for _, previous := range byID {
			calls += previous.ReservedCalls
			tokens += previous.ReservedTokens
		}
		output, buildErr := s.makeOutput(ctx, e, inv, m, *current.Result, saved, calls, tokens)
		if buildErr != nil {
			return buildErr
		}
		if _, err = s.writeRun(ctx, e, inv, RunWrite{Kind: "output", Manifest: m, Output: &output}); err != nil {
			return err
		}
		byID[saved.ID] = output
	}
	ordered := make([]RetainedOutput, 0, len(m.Outputs))
	for _, saved := range m.Outputs {
		ordered = append(ordered, byID[saved.ID])
	}
	state, code := frozenOutcome(ordered, m.PartialPolicy)
	_, err = s.writeRun(ctx, e, inv, RunWrite{Kind: "complete", Manifest: m, Outcome: state, Code: code})
	return err
}

// Run performs one explicit, bounded attempt with a fresh supplied Pengui JWT.
// It uses the common leased operation runner; it does not persist bearer tokens
// or create an unattended reporting worker ahead of the scheduling consumer.
func (s *Runs) Run(ctx context.Context, e identity.Envelope, id string, resume bool) (RunView, error) {
	return s.run(ctx, e, id, resume, nil)
}

func (s *Runs) run(ctx context.Context, e identity.Envelope, id string, resume bool, parent *jobs.Invocation) (RunView, error) {
	if s == nil || ctx == nil {
		return RunView{}, ErrInvalid
	}
	r, err := s.repo.ReadFrozenRun(ctx, e, id, true)
	if err != nil {
		return RunView{}, err
	}
	switch r.View.State {
	case "expired":
		return r.View, ErrExpired
	case "succeeded", "partial":
		return r.View, nil
	case "failed":
		return r.View, ErrIncomplete
	}
	if r.Manifest == nil {
		return r.View, ErrIncomplete
	}
	if err = RequireRunManifest(e, *r.Manifest); err != nil {
		return RunView{}, err
	}
	task, err := s.runner.Inspect(ctx, e, id)
	if err != nil {
		return r.View, err
	}
	if resume {
		task, err = s.runner.Resume(ctx, e, id)
		if err != nil {
			return r.View, err
		}
	}
	handler := func(work context.Context, inv jobs.Invocation) error { return s.continueFrozen(work, e, inv, r) }
	timeout := min(time.Duration(r.Manifest.Limits.Timeout), time.Duration(s.limits.Timeout))
	var runErr error
	if parent == nil {
		_, runErr = s.runner.Run(ctx, e, task, timeout, handler)
	} else {
		_, runErr = s.runner.RunNested(ctx, *parent, task, timeout, handler)
	}
	if runErr != nil && parent != nil && ctx.Err() == nil {
		if _, cancelErr := s.runner.Cancel(ctx, e, task.ID); cancelErr != nil && !errors.Is(cancelErr, store.ErrConflict) {
			return r.View, errors.Join(runErr, cancelErr)
		}
	}
	current, readErr := s.repo.ReadFrozenRun(ctx, e, id, true)
	if readErr != nil {
		if runErr != nil {
			return r.View, runErr
		}
		return r.View, readErr
	}
	if runErr != nil {
		return current.View, runErr
	}
	if current.View.State == "failed" {
		return current.View, ErrIncomplete
	}
	if current.View.State != "succeeded" && current.View.State != "partial" {
		return current.View, ErrIncomplete
	}
	return current.View, nil
}

// RebuildOutput exercises the deterministic rendition seam using retained data
// only. Narratives are returned verbatim; no inference or SQL path is reachable.
func (s *Runs) RebuildOutput(ctx context.Context, e identity.Envelope, id, output string) (RetainedOutput, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) || !identity.Identifier(output) {
		return RetainedOutput{}, ErrInvalid
	}
	r, err := s.repo.ReadFrozenRun(ctx, e, id, false)
	if err != nil {
		return RetainedOutput{}, err
	}
	if r.View.State == "expired" {
		return RetainedOutput{}, ErrExpired
	}
	if r.Manifest == nil || r.Result == nil || r.View.State != "succeeded" && r.View.State != "partial" {
		return RetainedOutput{}, ErrIncomplete
	}
	if err := retainedSelectionError(r, output); err != nil {
		return RetainedOutput{}, err
	}
	var retained *RetainedOutput
	for i := range r.Outputs {
		if r.Outputs[i].ID == output {
			retained = &r.Outputs[i]
		}
	}
	if retained == nil {
		return RetainedOutput{}, store.ErrNotFound
	}
	if retained.Kind == "narrative" || retained.State != "succeeded" {
		return clone(*retained), nil
	}
	for _, saved := range r.Manifest.Outputs {
		if saved.ID != output {
			continue
		}
		chart, err := s.buildRetainedChart(ctx, *r.Manifest, *r.Result, *saved.Mapping)
		if err != nil {
			return RetainedOutput{}, err
		}
		out := clone(*retained)
		out.Chart, out.Digest = &chart, ""
		out.Digest = digest(out)
		if out.Digest != retained.Digest {
			return RetainedOutput{}, ErrStale
		}
		return out, nil
	}
	return RetainedOutput{}, errors.Join(ErrInvalid, store.ErrNotFound)
}
