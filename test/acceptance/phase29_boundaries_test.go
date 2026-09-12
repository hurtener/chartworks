package acceptance

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestReportingCompositionBoundaries(t *testing.T) {
	t.Run("receipt-time-and-retained-context-isolation", testPhase29RetainedBoundaries)
	t.Run("document-reach-does-not-grant-query-actions", testPhase29QueryActions)
	t.Run("shared-retention-request-quota", func(t *testing.T) { testPhase29SharedQuota(t, false) })
	t.Run("shared-retention-byte-quota", func(t *testing.T) { testPhase29SharedQuota(t, true) })
	t.Run("independent-output-failure", testPhase29OutputFailure)
}

func testPhase29RetainedBoundaries(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "boundary-block", f.base)
	d := phase29Text("Context-isolated retained values")
	d.Widgets = append(d.Widgets, phase29BlockWidget("frozen", "boundary-block", 1, "table-main"))
	state := f.report(t, "boundary-report", d, true)
	admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "boundary-run"})
	if err != nil || admitted.Finished != nil {
		t.Fatal("pending composition has an invented completion time", admitted, err)
	}
	completed, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || !completed.Complete || completed.Finished == nil || completed.Finished.Before(completed.Created) {
		t.Fatal("execution receipt lost its persisted completion time", completed, err)
	}
	beforeSource, beforeModels, beforeQueries := f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)
	receipt, err := f.compositions.Inspect(ctx, f.execute, admitted.ID)
	if err != nil || receipt.Finished == nil || !receipt.Finished.Equal(*completed.Finished) {
		t.Fatal("receipt read changed the actual completion timestamp", receipt, err)
	}
	view, err := f.compositions.Get(ctx, f.execute, admitted.ID)
	if err != nil || view.Finished == nil || !view.Finished.Equal(*completed.Finished) {
		t.Fatal("metadata and execution receipts disagree", view, err)
	}
	replayed, err := f.compositions.Run(ctx, f.execute, admitted.ID, true)
	if err != nil || replayed.Finished == nil || !replayed.Finished.Equal(*completed.Finished) {
		t.Fatal("terminal replay invented a new completion time", replayed, err)
	}
	wrongContext := phase27Actor(t, f.f, "same-tenant-other-context", []string{"reporting.read", "cw.report.read:" + state.ID, "cw.execution_context.use:unrelated-context"})
	redacted, err := f.compositions.Get(ctx, wrongContext, admitted.ID)
	if err != nil || !redacted.Redacted || redacted.Complete || len(redacted.Pages) != 0 || redacted.RetainedBytes != 0 || redacted.QueryGroups != 0 || redacted.Finished != nil {
		t.Fatal("same-tenant reader learned inaccessible execution metadata", redacted, err)
	}
	if _, err := f.compositions.Widget(ctx, wrongContext, admitted.ID, "main", "frozen"); !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) {
		t.Fatal("report reach bypassed the retained execution-context partition", err)
	}
	foreign := f.f.f.token.envelope(t, "other-composition-tenant", f.execute.User(), phase29RuntimeScopes("other-composition-tenant")...)
	if _, err := f.compositions.Get(ctx, foreign, admitted.ID); !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-tenant manifest metadata disclosed", err)
	}
	if _, err := f.compositions.Widget(ctx, foreign, admitted.ID, "main", "frozen"); !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-tenant retained values disclosed", err)
	}
	if f.f.f.lookups.Load() != beforeSource || f.f.model.requests.Load() != beforeModels || f.attemptCount(t) != beforeQueries {
		t.Fatal("metadata, denial or terminal replay reached source/model execution")
	}
}

func testPhase29QueryActions(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := context.Background()
	d := phase29Text("Query actions remain independent")
	d.Widgets = append(d.Widgets, f.queryWidget())
	state := f.report(t, "query-action-report", d, true)
	for _, missing := range []string{"query.execute", "query.plan"} {
		scopes := slices.DeleteFunc(phase29RuntimeScopes(f.execute.Tenant()), func(value string) bool { return value == missing })
		actor := phase27Actor(t, f.f, f.execute.User(), scopes)
		beforeModels, beforeQueries := f.f.model.requests.Load(), f.attemptCount(t)
		admitted, err := f.compositions.Admit(ctx, actor, "report", state.ID, reporting.CompositionRequest{Key: "denied-" + missing})
		if err != nil {
			if !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) {
				t.Fatal("missing query action failed outside the authorization boundary", missing, err)
			}
		} else {
			finished, err := f.compositions.Run(ctx, actor, admitted.ID, false)
			if !errors.Is(err, reporting.ErrIncomplete) || finished.Complete || finished.State != "failed" {
				t.Fatal("enclosing report granted an absent query action", missing, finished, err)
			}
		}
		if f.f.model.requests.Load() != beforeModels || f.attemptCount(t) != beforeQueries {
			t.Fatal("denied query action still triggered model/source work", missing)
		}
	}
}

func testPhase29SharedQuota(t *testing.T, bytes bool) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	f.block(t, "quota-block", f.base)
	state := f.report(t, "quota-report", phase29Text("Shared storage reservation"), true)
	limits := f.limits
	if bytes {
		limits.Composition.MaxRetainedBytes = limits.Execution.MaxArtifactBytes
		limits.Execution.MaxTenantBytes = int64(limits.Composition.MaxRetainedBytes)
	} else {
		limits.Execution.MaxRequests = 1
	}
	compositions := f.withLimits(t, limits)
	runs := phase28RunService(t, f.f, f.blocks, f.f.f.db, nil, limits.Execution)
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	admitted, err := compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "reserve-composition-first"})
	if err != nil {
		t.Fatal("first bounded composition reservation", err)
	}
	if _, err := runs.Admit(ctx, f.execute, "quota-block", reporting.RunRequest{Key: "block-after-composition"}); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("frozen run ignored existing composition reservations", err)
	}
	if _, err := compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "second-composition"}); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("composition quota allowed another reservation", err)
	}
	replayed, err := compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "reserve-composition-first"})
	if err != nil || replayed.ID != admitted.ID || replayed.Manifest != admitted.Manifest {
		t.Fatal("full quota broke idempotent replay of an accepted reservation", replayed, err)
	}
	if f.attemptCount(t) != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("quota admission or rejection executed source/model work")
	}
}

func testPhase29OutputFailure(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	definition := phase27Definition(t, f.f, f.blockAuthor, f.base.SQL)
	f.block(t, "output-error-block", definition)
	d := phase29Text("Independent retained output errors")
	d.PartialFailure = "allow_partial"
	widget := phase29BlockWidget("mixed-outputs", "output-error-block", 1, "table-main", "narrative-main")
	widget.Block.Narrative = true
	d.Widgets = append(d.Widgets, widget)
	state := f.report(t, "output-error-report", d, true)
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	admitted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "output-error-run"})
	if err != nil {
		t.Fatal(err)
	}
	finished, err := f.compositions.Run(ctx, f.execute, admitted.ID, false)
	if err != nil || finished.State != "partial" || finished.Complete || f.attemptCount(t) != beforeQueries+1 {
		t.Fatal("one unavailable narrative erased good outputs or invented completeness", finished, err)
	}
	payload, err := f.compositions.Widget(ctx, f.execute, admitted.ID, "main", widget.ID)
	if err != nil || len(payload.Outputs) != 2 || payload.Outputs[0].State != "succeeded" || payload.Outputs[1].State != "failed" || payload.Outputs[1].Code != "narrative_unavailable" {
		t.Fatal("per-output failure receipt or successful table was lost", payload, err)
	}
	if f.f.model.requests.Load() != beforeModels {
		t.Fatal("unconfigured narrative lane invoked a provider")
	}
}
