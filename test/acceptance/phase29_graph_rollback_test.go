package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// Late failures must roll back the entire imported reference graph. Real SQL
// constraints and typed domain proofs remain enabled throughout these faults.
func TestDocumentGraphRollback(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := context.Background()
	f.block(t, "atomic-graph-block", f.base)
	definition := phase29Text("Atomic graph")
	definition.Widgets = append(definition.Widgets, phase29BlockWidget("table", "atomic-graph-block", 1, "table-main"), f.queryWidget())
	published := f.report(t, "atomic-graph-child", definition, true)
	raw := support.Raw(t, f.f.f.dsn)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_document_graph() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'SYNTHETIC_GRAPH_FAILURE'; END; $$`)
	beforeQueries, beforeModels := f.attemptCount(t), f.f.model.requests.Load()
	for _, table := range []string{"document_heads", "document_revisions", "document_block_refs", "document_topic_refs", "document_references", "document_external_refs", "document_events", "document_page_refs", "document_quarantine", "audit_events"} {
		t.Run(table, func(t *testing.T) {
			kind, id := "report", "atomic-graph-"+table
			d := definition
			if table == "document_page_refs" {
				kind = "dashboard"
				d.Widgets = nil
				d.Pages = []reporting.DocumentPage{{ID: "main", Title: "Exact report", Report: published.ID, Revision: 1}}
			}
			body, err := json.Marshal(d)
			if err != nil {
				t.Fatal(err)
			}
			quarantined := table == "document_quarantine" || table == "audit_events"
			if quarantined {
				body = []byte(`{"schema_version":99}`)
			}
			target := pgx.Identifier{"chartworks", table}.Sanitize()
			predicate := ""
			if table == "audit_events" {
				predicate = " WHEN (NEW.action='document.quarantined')"
			}
			sql(t, raw, "CREATE TRIGGER reject_document_graph BEFORE INSERT ON "+target+" FOR EACH ROW"+predicate+" EXECUTE FUNCTION chartworks.reject_document_graph()")
			t.Cleanup(func() { sql(t, raw, "DROP TRIGGER IF EXISTS reject_document_graph ON "+target) })
			external := reporting.ExternalReference{System: "fixture", ID: id, Version: "v1"}
			out, err := f.documents.Import(ctx, f.author, kind, id, body, external)
			if !errors.Is(err, store.ErrUnavailable) || out.State != nil || out.Quarantine != "" {
				t.Fatal("late import fault exposed committed material", out, err)
			}
			var rows int
			if err := raw.QueryRow(ctx, `SELECT
    (SELECT count(*) FROM chartworks.document_heads WHERE tenant_id=$1 AND document_id=$2)+
    (SELECT count(*) FROM chartworks.document_revisions WHERE tenant_id=$1 AND document_id=$2)+
    (SELECT count(*) FROM chartworks.document_external_refs WHERE tenant_id=$1 AND document_id=$2)+
    (SELECT count(*) FROM chartworks.document_events WHERE tenant_id=$1 AND document_id=$2)+
    (SELECT count(*) FROM chartworks.document_quarantine WHERE tenant_id=$1 AND target_id=$2)`, f.author.Tenant(), id).Scan(&rows); err != nil || rows != 0 {
				t.Fatal("failed import left graph/quarantine rows", rows, err)
			}
			sql(t, raw, "DROP TRIGGER reject_document_graph ON "+target)
			retried, err := f.documents.Import(ctx, f.author, kind, id, body, external)
			if err != nil || (!quarantined && (retried.State == nil || retried.State.ID != id)) || (quarantined && retried.Quarantine == "") {
				t.Fatal("atomic import retry", retried, err)
			}
		})
	}
	// A publication insert failure must not clear review or set publication.
	review, err := f.documents.Create(ctx, f.author, "report", "atomic-publish", phase29Text("Publish rollback"))
	if err != nil {
		t.Fatal(err)
	}
	review, err = f.documents.Transition(ctx, f.author, "report", review.ID, review.Version, 1, "review", "Reviewed")
	if err != nil {
		t.Fatal(err)
	}
	sql(t, raw, `CREATE TRIGGER reject_document_graph BEFORE INSERT ON chartworks.document_publications FOR EACH ROW EXECUTE FUNCTION chartworks.reject_document_graph()`)
	if _, err := f.documents.Transition(ctx, f.author, "report", review.ID, review.Version, 1, "publish", "Publish"); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("publication failure", err)
	}
	read, err := f.documents.Read(ctx, f.author, "report", review.ID, reporting.DocumentReference{Stage: "review"})
	if err != nil || read.State.Version != review.Version || read.State.PublishedRevision != 0 || read.State.ReviewRevision != 1 {
		t.Fatal("failed publication moved pointers", read.State, err)
	}
	if f.attemptCount(t) != beforeQueries || f.f.model.requests.Load() != beforeModels {
		t.Fatal("authoring graph invoked source/model work")
	}
}
