package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

func (s *Compositions) checkpoint(ctx context.Context, inv jobs.Invocation, write CompositionWrite) (CompositionRecord, error) {
	proof, err := prepareCompositionWrite(inv, write)
	if err != nil {
		return CompositionRecord{}, err
	}
	return s.repo.CheckpointComposition(ctx, inv, proof)
}

func failedGroup(g CompositionGroup, code string) GroupResult {
	r := GroupResult{Group: g.ID, Kind: g.Kind, State: "failed", Code: code, Outputs: []RetainedOutput{}}
	r.Digest = GroupResultDigest(r)
	return r
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

// Run executes or explicitly resumes one attempt through the existing lease
// runner. A retained terminal artifact is returned without warehouse/model work.
func (s *Compositions) Run(ctx context.Context, e identity.Envelope, id string, resume bool) (CompositionView, error) {
	if s == nil || ctx == nil || !identity.Identifier(id) {
		return CompositionView{}, ErrInvalid
	}
	r, err := s.repo.ReadComposition(ctx, e, id)
	if err != nil {
		return CompositionView{}, err
	}
	if r.State == "expired" || !time.Now().Before(r.Manifest.Expires) {
		return SummarizeComposition(r), ErrExpired
	}
	if slices.Contains([]string{"completed", "partial", "failed"}, r.State) {
		if r.State == "failed" {
			return SummarizeComposition(r), ErrIncomplete
		}
		return SummarizeComposition(r), nil
	}
	if err := RequireComposition(e, r.Manifest); err != nil {
		return CompositionView{}, err
	}
	task, err := s.runner.Inspect(ctx, e, id)
	if err != nil {
		return SummarizeComposition(r), err
	}
	if resume {
		task, err = s.runner.Resume(ctx, e, id)
		if err != nil {
			return SummarizeComposition(r), err
		}
	}
	timeout := min(time.Duration(r.Manifest.Limits.Timeout), time.Duration(s.documents.limits.Composition.Timeout))
	_, runErr := s.runner.Run(ctx, e, task, timeout, func(work context.Context, inv jobs.Invocation) error {
		return s.continueComposition(work, inv, r)
	})
	current, readErr := s.repo.ReadComposition(ctx, e, id)
	if readErr != nil {
		if runErr != nil {
			return SummarizeComposition(r), runErr
		}
		return SummarizeComposition(r), readErr
	}
	view := SummarizeComposition(current)
	if runErr != nil {
		return view, runErr
	}
	if current.State != "completed" && current.State != "partial" {
		return view, ErrIncomplete
	}
	return view, nil
}

func (s *Compositions) continueComposition(ctx context.Context, inv jobs.Invocation, record CompositionRecord) error {
	m := record.Manifest
	e, err := inv.Current(m.Kind+".run", m.Document, m.RequestHash)
	if err != nil {
		return err
	}
	if err := RequireComposition(e, m); err != nil {
		return err
	}
	done := map[string]bool{}
	retained := 0
	for _, result := range record.Results {
		done[result.Group] = true
		body, err := json.Marshal(result)
		if err != nil {
			return ErrInvalid
		}
		retained += len(body)
	}
	strictOmission := strictCompositionOmission(m)
	for index, group := range m.Groups {
		if done[group.ID] {
			continue
		}
		if _, err := inv.Current(m.Kind+".run", m.Document, m.RequestHash); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var result GroupResult
		switch {
		case strictOmission:
			result = failedGroup(group, "strict_omission")
		case index >= min(m.Limits.MaxQueries, s.documents.limits.Composition.MaxQueries):
			result = failedGroup(group, "budget_exhausted")
		case group.Kind == "query" && !s.documents.limits.Composition.LiveQueries:
			result = failedGroup(group, "live_queries_disabled")
		case group.Kind == "query" && group.Query.Durability == "session_bound" && !s.documents.limits.Composition.SessionBound:
			result = failedGroup(group, "session_bound_disabled")
		case group.Kind == "block":
			result, err = s.executeBlock(ctx, e, m, group)
		case group.Kind == "query":
			var updated CompositionRecord
			result, updated, err = s.executeQuery(ctx, e, inv, record, group)
			if err == nil && updated.Manifest.ID != "" {
				record = updated
			}
		default:
			return ErrInvalid
		}
		if err != nil {
			if ctx.Err() != nil || !e.Valid() {
				return err
			}
			result = failedGroup(group, compositionFailure(err))
		}
		result.Digest = GroupResultDigest(result)
		body, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			return ErrInvalid
		}
		if retained+len(body) > min(m.Limits.MaxRetainedBytes, s.documents.limits.Composition.MaxRetainedBytes) {
			result = failedGroup(group, "budget_exhausted")
			body, marshalErr = json.Marshal(result)
			if marshalErr != nil {
				return ErrInvalid
			}
		}
		record, err = s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "group", Group: group.ID, Result: &result})
		if err != nil {
			return err
		}
		retained += len(body)
	}
	outcome, code, _, err := CompositionCompletion(m, record.Results)
	if err != nil {
		return err
	}
	_, err = s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "complete", Outcome: outcome, Code: code})
	return err
}

func (s *Compositions) executeBlock(ctx context.Context, e identity.Envelope, m CompositionManifest, g CompositionGroup) (GroupResult, error) {
	if s.runs == nil || s.documents.blocks == nil {
		return GroupResult{}, ErrUnavailable
	}
	snapshot, err := s.documents.blocks.repo.ReadBlock(ctx, e, g.Block, Reference{Revision: g.Revision}, Execute)
	if err != nil {
		return GroupResult{}, err
	}
	if err := runEligibility(e, snapshot, g.Policy, time.Now()); err != nil {
		return GroupResult{}, err
	}
	if snapshot.Revision.Digest != g.Definition || snapshot.Revision.ExecutionDigest != g.Execution || snapshot.Validation.BindingDigest != exec.Hash(g.Binding) {
		return GroupResult{}, ErrStale
	}
	policy := g.Policy
	if m.Private {
		policy = "private_preview"
	}
	child, err := s.runs.Admit(ctx, e, g.Block, RunRequest{Key: "composition:" + m.ID + ":" + g.ID, Reference: Reference{Revision: g.Revision}, Arguments: clone(g.Arguments),
		Resolution: clone(g.Resolution), Outputs: clone(g.Outputs), Policy: policy, Locale: g.Locale, Narrative: g.Narrative, PartialPolicy: "allow_partial"})
	if err != nil {
		return GroupResult{}, err
	}
	if child.RevisionDigest != g.Definition || child.PartitionDigest != exec.Hash(g.Binding) || child.Private != g.Private {
		return GroupResult{}, ErrStale
	}
	if _, err := s.runs.Run(ctx, e, child.ID, false); err != nil {
		return GroupResult{}, err
	}
	retained, err := s.runs.repo.ReadFrozenRun(ctx, e, child.ID, true)
	if err != nil {
		return GroupResult{}, err
	}
	if retained.Manifest == nil || digest(retained.Manifest.Resolved.Parameters) != digest(g.Resolved.Parameters) || retained.View.RevisionDigest != g.Definition || retained.View.PartitionDigest != exec.Hash(g.Binding) {
		return GroupResult{}, ErrStale
	}
	// A private child run preserves privacy but does not bypass the report's
	// original certified-only or explicit-stale eligibility requirement.
	current, err := s.documents.blocks.repo.ReadBlock(ctx, e, g.Block, Reference{Revision: g.Revision}, Execute)
	if err != nil {
		return GroupResult{}, err
	}
	if err := runEligibility(e, current, g.Policy, time.Now()); err != nil {
		return GroupResult{}, err
	}
	out := GroupResult{Group: g.ID, Kind: "block", State: "completed", ChildRun: child.ID, Block: &retained.View, Outputs: []RetainedOutput{}, Observed: retained.View.Observed}
	byID := map[string]RetainedOutput{}
	for _, output := range retained.Outputs {
		byID[output.ID] = output
	}
	for _, id := range g.Outputs {
		output, ok := byID[id]
		if !ok {
			return GroupResult{}, ErrIncomplete
		}
		out.Outputs = append(out.Outputs, clone(output))
		if output.State != "succeeded" {
			out.State, out.Code = "partial", "output_failed"
		}
	}
	if retained.Result == nil {
		return GroupResult{}, ErrIncomplete
	}
	if retained.Result.Truncated {
		out.State, out.Code = "partial", "query_truncated"
	}
	out.Digest = GroupResultDigest(out)
	return out, nil
}

func (s *Compositions) executeQuery(ctx context.Context, e identity.Envelope, inv jobs.Invocation, record CompositionRecord, group CompositionGroup) (GroupResult, CompositionRecord, error) {
	if s.queries == nil || group.Query == nil || group.Origin == nil {
		return GroupResult{}, record, ErrUnavailable
	}
	m := record.Manifest
	plan, found := record.Plans[group.ID]
	if !found {
		var err error
		plan, err = s.queries.PrepareDocumentQuery(ctx, e, *group.Query, *group.Origin, "composition:"+m.ID+":"+group.ID, group.Locale)
		if err != nil {
			return GroupResult{}, record, err
		}
		if plan.BindingDigest != exec.Hash(group.Binding) {
			return GroupResult{}, record, ErrStale
		}
		record, err = s.checkpoint(ctx, inv, CompositionWrite{Manifest: m, Kind: "plan", Group: group.ID, Plan: &plan})
		if err != nil {
			return GroupResult{}, record, err
		}
	}
	query, err := s.queries.RunDocumentQuery(ctx, e, *group.Query, *group.Origin, plan,
		min(m.ArtifactLimits.MaxRows, s.documents.limits.Execution.MaxRows), min(m.ArtifactLimits.MaxResultBytes, s.documents.limits.Execution.MaxResultBytes))
	if err != nil {
		return GroupResult{}, record, err
	}
	observed := time.Now().UTC()
	result := GroupResult{Group: group.ID, Kind: "query", State: "completed", Query: &query, QueryPlan: &plan, Outputs: []RetainedOutput{}, Observed: &observed}
	if query.Execution.Result == nil {
		return GroupResult{}, record, ErrIncomplete
	}
	if query.EvidenceStale || query.Execution.Result.Truncated {
		result.State, result.Code = "partial", "query_truncated"
	}
	result.Digest = GroupResultDigest(result)
	return result, record, nil
}

// SummarizeComposition projects already-authorized execution state. Retained
// read surfaces apply page redaction in SQL before returning this projection.
func SummarizeComposition(record CompositionRecord) CompositionView {
	m := record.Manifest
	view := CompositionView{ID: m.ID, Kind: m.Kind, Document: m.Document, Revision: m.Revision, Manifest: m.ManifestDigest(), State: record.State, Code: record.Code,
		Private: m.Private, Redacted: m.Redacted, Created: m.Created, Expires: m.Expires, Pages: []CompositionPageSummary{}, QueryGroups: len(m.Groups)}
	results := map[string]GroupResult{}
	for _, r := range record.Results {
		results[r.Group] = r
		if raw, err := json.Marshal(r); err == nil {
			view.RetainedBytes += int64(len(raw))
		}
	}
	for _, p := range m.Pages {
		page := CompositionPageSummary{ID: p.ID, Report: p.Report, Revision: p.Revision, Title: p.Title, Widgets: []CompositionWidgetSummary{}}
		for _, widget := range p.Widgets {
			w := CompositionWidgetSummary{ID: widget.Definition.ID, Kind: widget.Definition.Kind, State: "pending", Code: widget.Code, Grid: widget.Definition.Grid,
				Presentation: clone(widget.Definition.Presentation), Parameters: clone(widget.Parameters), Outputs: []string{}}
			if w.Kind == "text" {
				w.State = "completed"
			} else if w.Code != "" {
				w.State = "omitted"
			} else if result, found := results[widget.Group]; found {
				w.State, w.Code, w.Observed = result.State, result.Code, result.Observed
				if result.Block != nil {
					trust := clone(result.Block.Trust)
					w.Trust = &trust
				}
				if result.Query != nil {
					w.QueryDigest, w.SemanticDigest = result.Query.QueryDigest, result.Query.SemanticDigest
				}
			}
			if widget.Definition.Block != nil {
				w.Outputs = clone(widget.Definition.Block.Outputs)
			}
			if widget.Definition.Query != nil {
				w.Durability = widget.Definition.Query.Durability
			}
			page.Widgets = append(page.Widgets, w)
		}
		view.Pages = append(view.Pages, page)
	}
	if state, _, mixed, err := CompositionCompletion(m, record.Results); err == nil {
		view.Complete = record.State == "completed" && state == "completed"
		view.MixedFreshness = mixed
	}
	return view
}

// CompositionResultJSON is the bounded immutable group storage representation.
func CompositionResultJSON(result GroupResult) ([]byte, error) {
	body, err := json.Marshal(result)
	if err != nil || len(body) > 16<<20 {
		return nil, ErrBudget
	}
	return body, nil
}

// DecodeCompositionResult rejects corrupt or substituted persisted group data.
func DecodeCompositionResult(body []byte, manifest CompositionManifest, group string) (GroupResult, error) {
	var result GroupResult
	g, ok := compositionGroup(manifest, group)
	if !ok || len(body) == 0 || len(body) > 16<<20 || json.Unmarshal(body, &result) != nil || CheckCompositionResult(manifest, g, result) != nil {
		return GroupResult{}, ErrInvalid
	}
	return result, nil
}

// DecodeCompositionManifest is a validation boundary, not an authority issuer.
func DecodeCompositionManifest(body []byte, expected string) (CompositionManifest, error) {
	var m CompositionManifest
	if len(body) == 0 || len(body) > 16<<20 || json.Unmarshal(body, &m) != nil || !validComposition(m) || m.ManifestDigest() != expected {
		return CompositionManifest{}, ErrInvalid
	}
	return m, nil
}

// Suppress unused-import drift while keeping error identity explicit in tests.
var _ = errors.Is
var _ nlqexec.SavedPlan
