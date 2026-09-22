package acceptance

import (
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// This gap-wave test is intentionally cumulative across phases 29 and 30: the
// delete transaction must fence accepted runs and retire only matching schedules.
func TestCW11ReportingLifecycleAndCatalog(t *testing.T) {
	f := newPhase30Fixture(t, false)
	ctx := t.Context()
	blockDefinition, err := reporting.MigrateDefinition(f.domain.base)
	if err != nil {
		t.Fatal(err)
	}
	f.domain.block(t, "cw11-owned-block", blockDefinition)
	independent, err := f.domain.runs.Admit(ctx, f.domain.execute, "cw11-owned-block", reporting.RunRequest{Key: "cw11-independent-run", Outputs: []string{"table-main"}})
	if err != nil {
		t.Fatal("admit independent block run", err)
	}
	if independent, err = f.domain.runs.Run(ctx, f.domain.execute, independent.ID, false); err != nil || independent.State != "succeeded" {
		t.Fatal("complete independent block run", independent, err)
	}
	reportDefinition := phase29Text("Lifecycle report")
	reportDefinition.Widgets = []reporting.Widget{phase29BlockWidget("owned", "cw11-owned-block", 0, "table-main")}
	state := f.domain.report(t, "cw11-lifecycle", reportDefinition, true)
	dashboardDefinition := phase29Text("Lifecycle dashboard")
	dashboardDefinition.Widgets = nil
	dashboardDefinition.Pages = []reporting.DocumentPage{{ID: "main", Title: "Lifecycle page", Report: state.ID, Revision: state.PublishedRevision}}
	dashboard, err := f.domain.documents.Create(ctx, f.domain.author, "dashboard", "cw11-dashboard", dashboardDefinition)
	if err != nil {
		t.Fatal("create dashboard", err)
	}
	dashboard = phase29Publish(t, f.domain.documents, f.domain.author, dashboard)
	dashboardRun, err := f.domain.compositions.Admit(ctx, f.domain.execute, "dashboard", dashboard.ID, reporting.CompositionRequest{Key: "cw11-dashboard-page-run"})
	if err != nil {
		t.Fatal("seal dashboard page root", err)
	}
	if completed, err := f.domain.compositions.Run(ctx, f.domain.execute, dashboardRun.ID, false); err != nil || !completed.Complete {
		t.Fatal("complete dashboard page child", completed, err)
	}
	run, err := f.domain.compositions.Admit(ctx, f.domain.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw11-sealed", Reference: reporting.DocumentReference{Revision: state.PublishedRevision}})
	if err != nil {
		t.Fatal("seal retained composition", err)
	}
	if completed, err := f.domain.compositions.Run(ctx, f.domain.execute, run.ID, false); err != nil || !completed.Complete {
		t.Fatal("complete document-owned child", completed, err)
	}
	activeRun, err := f.domain.compositions.Admit(ctx, f.domain.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw11-active-root"})
	if err != nil {
		t.Fatal("seal active root", err)
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
	page, err := f.domain.documents.List(ctx, deleter, "report", "", 100)
	if err != nil {
		t.Fatal("catalog", err)
	}
	var summary *reporting.DocumentSummary
	for i := range page.Items {
		if page.Items[i].ID == state.ID {
			summary = &page.Items[i]
		}
	}
	if summary == nil || !summary.Creator.Known || summary.Creator.Label != "Current actor" || !slices.Equal(summary.Relationships.Schedules, []string{schedule.ID}) {
		t.Fatal("catalog labels or bounded delivery relationship", summary)
	}

	impact, err := f.domain.documents.PreviewDelete(ctx, deleter, "report", state.ID)
	if err != nil || impact.Version != state.Version || impact.RevisionCount != 1 || impact.RetainedRuns != 3 || !slices.Equal(impact.MatchingSchedules, []string{schedule.ID}) {
		t.Fatal("delete impact", impact, err)
	}
	request := reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "cw11-delete", Reason: "Owner approved live-data erasure"}
	deleted, err := f.domain.documents.Delete(ctx, deleter, "report", state.ID, request)
	if err != nil || deleted.DeletedVersion != state.Version+1 || deleted.ErasedRevisions != 1 || deleted.ErasedRuns != 3 || deleted.ErasedChildRuns != 2 || !slices.Equal(deleted.RetiredSchedules, []string{schedule.ID}) {
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
	if _, err := f.domain.compositions.Run(ctx, f.domain.execute, activeRun.ID, false); err == nil {
		t.Fatal("active root completed after deletion")
	}
	if preserved, err := f.domain.f.f.db.ReadFrozenRun(ctx, f.domain.execute, independent.ID, true); err != nil || preserved.View.State != "succeeded" || preserved.Result == nil {
		t.Fatal("independently owned retained artifact was erased", preserved.View, err)
	}
	retired, err := f.queue.GetSchedule(ctx, f.controls(t), schedule.ID)
	if err != nil || !retired.Retired || retired.Enabled {
		t.Fatal("matching schedule not retired", retired, err)
	}
	untouched, err := f.queue.GetSchedule(ctx, f.controls(t), unrelated.ID)
	if err != nil || untouched.Retired || !untouched.Enabled {
		t.Fatal("unrelated schedule changed", untouched, err)
	}
	var definitions, payloads, childPayloads int
	raw := support.Raw(t, f.domain.f.f.dsn)
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.document_revisions WHERE tenant_id=$1 AND kind='report' AND document_id=$2 AND definition<>jsonb_build_object('deleted',true)`, deleter.Tenant(), state.ID).Scan(&definitions); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`, deleter.Tenant(), run.ID).Scan(&payloads); err != nil {
		t.Fatal(err)
	}
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.frozen_run_payloads p JOIN chartworks.operations o USING(tenant_id,operation_id)
 WHERE p.tenant_id=$1 AND o.nested_parent=$2`, deleter.Tenant(), run.ID).Scan(&childPayloads); err != nil {
		t.Fatal(err)
	}
	if definitions != 0 || payloads != 0 || childPayloads != 0 {
		t.Fatal("live payload erasure incomplete", definitions, payloads, childPayloads)
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

func TestCW11DeletionErasesOwnedDynamicQueries(t *testing.T) {
	f := newPhase29Execution(t, true)
	definition := phase29Text("Dynamic owned query")
	definition.Widgets = append(definition.Widgets, f.queryWidget())
	state := f.report(t, "cw11-dynamic-owned", definition, true)
	run, err := f.compositions.Admit(t.Context(), f.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw11-dynamic-run"})
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	if _, err := raw.Exec(t.Context(), `INSERT INTO chartworks.nlq_sessions
 (tenant_id,actor_id,session_id,context_id,topics,locale) VALUES($1,$2,$3,$4,'["synthetic-topic"]','en')`,
		f.execute.Tenant(), f.execute.User(), f.execute.Session(), f.base.Context); err != nil {
		t.Fatal("seed owned query session", err)
	}
	if _, err := raw.Exec(t.Context(), `INSERT INTO chartworks.nlq_queries
 (tenant_id,actor_id,session_id,query_id,operation,topic_id,topics,topic_versions,rule_versions,template_selections,example_selection,
  context_id,locale,question,route,generation,parameters,receipt,status,assumptions,ambiguities,errors,validation_fixes,execution_fixes,revision)
 VALUES($1,$2,$3,'11111111111111111111111111111111',$4,'synthetic-topic','["synthetic-topic"]','[{"id":"synthetic-topic","version":1}]','[]','[]','{}',
  $5,'en','What are the retained values?','{}','{}','[]','{}','planned','[]','[]','[]',0,0,1)`,
		f.execute.Tenant(), f.execute.User(), f.execute.Session(), "composition:"+run.ID+":dynamic", f.base.Context); err != nil {
		t.Fatal("seed owned query", err)
	}
	deleted, err := f.documents.Delete(t.Context(), f.author, "report", state.ID, reporting.DocumentDeleteRequest{
		ExpectedVersion: state.Version, Key: "cw11-dynamic-delete", Reason: "Erase document-owned query material",
	})
	if err != nil || deleted.ErasedQueries != 1 {
		t.Fatal("delete dynamic query owner", deleted, err)
	}
	var remaining int
	if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.nlq_queries
 WHERE tenant_id=$1 AND operation LIKE $2`, f.author.Tenant(), "composition:"+run.ID+":%").Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("document-owned query retained", remaining, err)
	}
}

func TestCW11CatalogDoesNotLeakPrivateDraftEditor(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	published := f.report(t, "cw11-private-editor", phase29Text("Published metadata"), true)
	editor := phase27Actor(t, f.f, "draft-editor", phase29AuthorScopes(f.author.Tenant()))
	draftDefinition := phase29Text("Private draft metadata")
	if _, err := f.documents.Edit(ctx, editor, "report", published.ID, published.Version,
		reporting.DocumentReference{Revision: published.PublishedRevision}, draftDefinition); err != nil {
		t.Fatal("append private draft", err)
	}
	readerScopes := slices.DeleteFunc(phase29RuntimeScopes(f.author.Tenant()), func(scope string) bool {
		return scope == "reporting.preview" || strings.Contains(scope, ".preview:") || scope == "reporting.execute" || strings.Contains(scope, ".execute:") || scope == "jobs.cancel" || scope == "query.plan" || scope == "query.execute"
	})
	reader := phase27Actor(t, f.f, f.author.User(), readerScopes)
	page, err := f.documents.List(ctx, reader, "report", "", 20)
	if err != nil {
		t.Fatal("list published catalog", err)
	}
	for _, item := range page.Items {
		if item.ID == published.ID {
			if item.Revision != published.PublishedRevision || item.LastEditor.Label != "Current actor" || !item.LastEditor.Known {
				t.Fatal("private draft editor leaked through published catalog", item)
			}
			return
		}
	}
	t.Fatal("published report omitted from catalog")
}
