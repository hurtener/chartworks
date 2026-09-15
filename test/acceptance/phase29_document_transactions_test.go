package acceptance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestDocumentListBoundaries(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	e := phase27Actor(t, f, f.f.e.User(), phase29DocumentScopes())
	s, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"list-a", "list-b", "list-private", "list-retired"} {
		v, err := s.Create(ctx, e, "report", id, phase29Text(id))
		if err != nil {
			t.Fatal(err)
		}
		if id == "list-private" {
			continue
		}
		v = phase29Publish(t, s, e, v)
		if id == "list-retired" {
			if _, err := s.Transition(ctx, e, "report", id, v.Version, 1, "archive", "Retired fixture"); err != nil {
				t.Fatal(err)
			}
		}
	}
	beforeSource, beforeModel := f.f.lookups.Load(), f.model.requests.Load()
	first, err := s.List(ctx, e, "report", "", 1)
	if err != nil || len(first.Items) != 1 || first.Items[0].ID != "list-a" || first.Next != "list-a" {
		t.Fatal("bounded first page", first, err)
	}
	last, err := s.List(ctx, e, "report", first.Next, 1)
	if err != nil || len(last.Items) != 1 || last.Items[0].ID != "list-b" || last.Next != "" {
		t.Fatal("cursor lost order or exposed private/archived state", last, err)
	}
	limited := phase27Actor(t, f, "limited-document-reader", []string{"reporting.read", "cw.report.read:list-b"})
	list, err := s.List(ctx, limited, "report", "", 100)
	if err != nil || len(list.Items) != 1 || list.Items[0].ID != "list-b" || list.Next != "" {
		t.Fatal("list widened resource selection", list, err)
	}
	for _, tc := range []struct {
		kind, after string
		limit       int
	}{{"report", "", 0}, {"report", "", 101}, {"unknown", "", 1}, {"report", "invalid/cursor", 1}} {
		if _, err := f.f.db.ListDocuments(ctx, e, tc.kind, tc.after, tc.limit); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("store list bounds", tc, err)
		}
	}
	withoutAction := phase27Actor(t, f, "no-action", []string{"cw.report.read:*"})
	if _, err := f.f.db.ListDocuments(ctx, withoutAction, "report", "", 1); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("list action bypass", err)
	}
	withoutReach := phase27Actor(t, f, "no-reach", []string{"reporting.read"})
	if _, err := f.f.db.ListDocuments(ctx, withoutReach, "report", "", 1); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("list existence disclosure", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := f.f.db.ListDocuments(cancelled, e, "report", "", 1); !errors.Is(err, context.Canceled) {
		t.Fatal("list ignored cancellation", err)
	}
	for _, ref := range []reporting.DocumentReference{{Revision: -1}, {Revision: 257}, {Stage: "unknown"}, {Revision: 1, Stage: "draft"}} {
		if _, err := f.f.db.ReadDocument(ctx, e, "report", "list-a", ref, reporting.Read, false); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("store reference bounds", ref, err)
		}
	}
	if _, err := f.f.db.ReadDocument(cancelled, e, "report", "list-a", reporting.DocumentReference{}, reporting.Read, false); !errors.Is(err, context.Canceled) {
		t.Fatal("document read ignored cancellation", err)
	}
	limits := config.DefaultReporting()
	limits.MaxBlocks = 4
	bounded, err := reporting.NewDocuments(f.f.db, nil, nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bounded.Create(ctx, e, "report", "over-document-quota", phase29Text("Denied by actual stored inventory")); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("document quota was not enforced", err)
	}
	if _, err := s.Create(ctx, e, "report", "list-a", phase29Text("Duplicate cannot replace a publication")); !errors.Is(err, store.ErrConflict) {
		t.Fatal("duplicate head was replaced", err)
	}
	if _, err := s.Transition(ctx, e, "report", "missing-document", 1, 1, "archive", "No such document"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("transition invented a head", err)
	}
	if _, err := s.Transition(ctx, e, "report", "list-a", first.Items[0].Version, 2, "archive", "No such revision"); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("transition invented a revision", err)
	}
	if f.f.lookups.Load() != beforeSource || f.model.requests.Load() != beforeModel {
		t.Fatal("document metadata used a source/model service")
	}
}

func TestDocumentAuditRollback(t *testing.T) {
	f := newPhase18Fixture(t)
	ctx := context.Background()
	e := phase27Actor(t, f, f.f.e.User(), phase29DocumentScopes())
	s, err := reporting.NewDocuments(f.f.db, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.dsn)
	// Fail the actual audit insert after the domain writes, not a fake repository.
	sql(t, raw, `CREATE FUNCTION chartworks.reject_document_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action LIKE 'document.%' THEN RAISE EXCEPTION 'SYNTHETIC_DOCUMENT_AUDIT_CANARY'; END IF; RETURN NEW; END; $$`)
	for _, operation := range []string{"create", "edit", "review", "publish", "reject", "archive"} {
		t.Run(operation, func(t *testing.T) {
			id := "atomic-" + operation
			var before reporting.DocumentState
			if operation != "create" {
				before, err = s.Create(ctx, e, "report", id, phase29Text("Original"))
				if err != nil {
					t.Fatal(err)
				}
				if operation == "publish" || operation == "reject" {
					before, err = s.Transition(ctx, e, "report", id, before.Version, 1, "review", "Ready")
					if err != nil {
						t.Fatal(err)
					}
				}
			}
			sql(t, raw, `CREATE TRIGGER reject_document_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_document_audit()`)
			t.Cleanup(func() { sql(t, raw, `DROP TRIGGER IF EXISTS reject_document_audit ON chartworks.audit_events`) })
			var got reporting.DocumentState
			switch operation {
			case "create":
				got, err = s.Create(ctx, e, "report", id, phase29Text("Must roll back"))
			case "edit":
				got, err = s.Edit(ctx, e, "report", id, before.Version, reporting.DocumentReference{Revision: 1}, phase29Text("Must roll back"))
			default:
				got, err = s.Transition(ctx, e, "report", id, before.Version, 1, operation, "Must roll back")
			}
			if !errors.Is(err, store.ErrUnavailable) || strings.Contains(err.Error(), "CANARY") || got.ID != "" {
				t.Fatal("audit failure leaked or committed partial state", got, err)
			}
			var heads, revisions, events, publications int
			if err := raw.QueryRow(ctx, `SELECT (SELECT count(*) FROM chartworks.document_heads WHERE tenant_id=$1 AND document_id=$2), (SELECT count(*) FROM chartworks.document_revisions WHERE tenant_id=$1 AND document_id=$2), (SELECT count(*) FROM chartworks.document_events WHERE tenant_id=$1 AND document_id=$2), (SELECT count(*) FROM chartworks.document_publications WHERE tenant_id=$1 AND document_id=$2)`, e.Tenant(), id).Scan(&heads, &revisions, &events, &publications); err != nil {
				t.Fatal(err)
			}
			if operation == "create" {
				if heads != 0 || revisions != 0 || events != 0 || publications != 0 {
					t.Fatal("failed create left identity/content/events", heads, revisions, events, publications)
				}
			} else {
				view, err := f.f.db.ReadDocument(ctx, e, "report", id, reporting.DocumentReference{Revision: 1}, reporting.Write, false)
				if err != nil || !reflect.DeepEqual(view.State, before) || heads != 1 || revisions != 1 || events != int(before.Version) || publications != 0 {
					t.Fatal("failed mutation moved pointers or published content", view.State, before, heads, revisions, events, publications, err)
				}
			}
		})
	}
}
