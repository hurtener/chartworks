package acceptance

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

type cw11ActorLabels struct{}

func (cw11ActorLabels) ResolveActorLabels(_ context.Context, _ identity.Envelope, ids []string, locale string) (map[string]reporting.ActorPresentation, error) {
	out := map[string]reporting.ActorPresentation{}
	for _, id := range ids {
		if id != "" {
			out[id] = reporting.ActorPresentation{Label: "Reviewed analyst (" + locale + ")", Kind: "person", Known: true}
		}
	}
	return out, nil
}

// This gap-wave test is intentionally cumulative across phases 29 and 30: the
// delete transaction must fence accepted runs and retire only matching schedules.
func TestCW11ReportingLifecycleAndCatalog(t *testing.T) {
	f := newPhase30Fixture(t, false)
	ctx := t.Context()
	state := f.domain.report(t, "cw11-lifecycle", phase29Text("Lifecycle report"), true)
	dashboardDefinition := phase29Text("Lifecycle dashboard")
	dashboardDefinition.Widgets = nil
	dashboardDefinition.Pages = []reporting.DocumentPage{{ID: "main", Title: "Lifecycle page", Report: state.ID, Revision: state.PublishedRevision}}
	dashboard, err := f.domain.documents.Create(ctx, f.domain.author, "dashboard", "cw11-dashboard", dashboardDefinition)
	if err != nil {
		t.Fatal("create dashboard", err)
	}
	dashboard = phase29Publish(t, f.domain.documents, f.domain.author, dashboard)
	run, err := f.domain.compositions.Admit(ctx, f.domain.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw11-sealed", Reference: reporting.DocumentReference{Revision: state.PublishedRevision}})
	if err != nil {
		t.Fatal("seal retained composition", err)
	}
	schedule := f.schedule(t, "cw11-matching-schedule", phase30Manual(phase30Target("report", state.ID)))
	unrelatedReport := f.domain.report(t, "cw11-unrelated", phase29Text("Unrelated report"), true)
	unrelated := f.schedule(t, "cw11-unrelated-schedule", phase30Manual(phase30Target("report", "cw11-unrelated")))
	unauthorizedDelete := reporting.DocumentDeleteRequest{ExpectedVersion: unrelatedReport.Version, Key: "cw11-no-schedule-reach", Reason: "Must remain atomic"}
	if _, err := f.domain.documents.Delete(ctx, f.domain.author, "report", unrelatedReport.ID, unauthorizedDelete); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("schedule dependency authority was not enforced atomically", err)
	}
	if _, err := f.domain.documents.Read(ctx, f.domain.author, "report", unrelatedReport.ID, reporting.DocumentReference{}); err != nil {
		t.Fatal("failed deletion changed report state", err)
	}

	scopes := append(slices.Clone(phase29AuthorScopes(f.actor.Tenant())),
		"scheduling.read", "scheduling.write", "cw.schedule.read:*", "cw.schedule.write:*")
	deleter := phase27Actor(t, f.domain.f, f.actor.User(), scopes)
	catalogDocuments := f.domain.documents.WithActorLabels(cw11ActorLabels{})
	page, err := catalogDocuments.List(ctx, deleter, "report", "", 100)
	if err != nil {
		t.Fatal("catalog", err)
	}
	var summary *reporting.DocumentSummary
	for i := range page.Items {
		if page.Items[i].ID == state.ID {
			summary = &page.Items[i]
		}
	}
	if summary == nil || !summary.Creator.Known || summary.Creator.Label != "Reviewed analyst (en-US)" || !slices.Equal(summary.Relationships.Schedules, []string{schedule.ID}) {
		t.Fatal("catalog labels or bounded delivery relationship", summary)
	}

	impact, err := f.domain.documents.PreviewDelete(ctx, deleter, "report", state.ID)
	if err != nil || impact.Version != state.Version || impact.RevisionCount != 1 || impact.RetainedRuns != 1 || !slices.Equal(impact.MatchingSchedules, []string{schedule.ID}) {
		t.Fatal("delete impact", impact, err)
	}
	request := reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "cw11-delete", Reason: "Owner approved live-data erasure"}
	deleted, err := f.domain.documents.Delete(ctx, deleter, "report", state.ID, request)
	if err != nil || deleted.DeletedVersion != state.Version+1 || deleted.ErasedRevisions != 1 || deleted.ErasedRuns != 1 || !slices.Equal(deleted.RetiredSchedules, []string{schedule.ID}) {
		t.Fatal("delete", deleted, err)
	}
	replayed, err := f.domain.documents.Delete(ctx, deleter, "report", state.ID, request)
	if err != nil || !replayed.DeletedAt.Equal(deleted.DeletedAt) {
		t.Fatal("idempotent replay", replayed, err)
	}
	changed := request
	changed.Key = "cw11-delete-changed"
	if _, err := f.domain.documents.Delete(ctx, deleter, "report", state.ID, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed replay did not conflict", err)
	}
	if _, err := f.domain.documents.Read(ctx, deleter, "report", state.ID, reporting.DocumentReference{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("deleted report remained readable", err)
	}
	dashboardView, err := f.domain.documents.Read(ctx, deleter, "dashboard", dashboard.ID, reporting.DocumentReference{})
	if err != nil || len(dashboardView.Definition.Pages) != 0 {
		t.Fatal("dashboard history was not preserved with dead page omitted", dashboardView, err)
	}
	if _, err := f.domain.compositions.Run(ctx, f.domain.execute, run.ID, false); err == nil {
		t.Fatal("stale worker completed after deletion")
	}
	retired, err := f.queue.GetSchedule(ctx, f.controls(t), schedule.ID)
	if err != nil || !retired.Retired || retired.Enabled {
		t.Fatal("matching schedule not retired", retired, err)
	}
	untouched, err := f.queue.GetSchedule(ctx, f.controls(t), unrelated.ID)
	if err != nil || untouched.Retired || !untouched.Enabled {
		t.Fatal("unrelated schedule changed", untouched, err)
	}
	var definitions, payloads int
	raw := support.Raw(t, f.domain.f.f.dsn)
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.document_revisions WHERE tenant_id=$1 AND kind='report' AND document_id=$2 AND definition<>jsonb_build_object('deleted',true)`, deleter.Tenant(), state.ID).Scan(&definitions); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`, deleter.Tenant(), run.ID).Scan(&payloads); err != nil {
		t.Fatal(err)
	}
	if definitions != 0 || payloads != 0 {
		t.Fatal("live payload erasure incomplete", definitions, payloads)
	}
	dashboardDelete := reporting.DocumentDeleteRequest{ExpectedVersion: dashboard.Version, Key: "cw11-delete-dashboard", Reason: "Owner approved dashboard erasure"}
	if _, err := f.domain.documents.Delete(ctx, deleter, "dashboard", dashboard.ID, dashboardDelete); err != nil {
		t.Fatal("delete non-owning dashboard", err)
	}
	if _, err := f.domain.documents.Read(ctx, deleter, "dashboard", dashboard.ID, reporting.DocumentReference{}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("deleted dashboard remained readable", err)
	}
	concurrent := f.domain.report(t, "cw11-concurrent-delete", phase29Text("Concurrent deletion"), true)
	concurrentRequest := reporting.DocumentDeleteRequest{ExpectedVersion: concurrent.Version, Key: "cw11-concurrent-key", Reason: "Concurrent replay proof"}
	results := make([]reporting.DocumentDeletion, 2)
	errs := make([]error, 2)
	var group sync.WaitGroup
	for i := range results {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			results[index], errs[index] = f.domain.documents.Delete(ctx, deleter, "report", concurrent.ID, concurrentRequest)
		}(i)
	}
	group.Wait()
	if errs[0] != nil || errs[1] != nil || !results[0].DeletedAt.Equal(results[1].DeletedAt) || results[0].DeletedVersion != results[1].DeletedVersion {
		t.Fatal("concurrent deletion did not converge on one tombstone", results, errs)
	}
}
