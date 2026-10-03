package acceptance

import (
	"context"
	"fmt"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
	"strings"
	"testing"
	"time"
)

func TestRetentionOriginalOperationAudit(t *testing.T) {
	for _, mode := range []string{"baseline", "later_run", "saved_descendant"} {
		t.Run(mode, func(t *testing.T) {
			f := newPhase29Execution(t, true)
			ctx := t.Context()
			definition := phase29Text("Synthetic retention origin audit")
			definition.Widgets = append(definition.Widgets, f.queryWidget())
			state := f.report(t, "retention-origin", definition, true)
			run, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "retention-origin-run"})
			if err != nil {
				t.Fatal(err)
			}
			run, err = f.compositions.Run(ctx, f.execute, run.ID, false)
			if err != nil || !run.Complete {
				t.Fatal("run composition", err)
			}
			record, err := f.f.f.db.ReadComposition(ctx, f.execute, run.ID)
			if err != nil || len(record.Results) != 1 || record.Results[0].Query == nil {
				t.Fatal("query checkpoint", err)
			}
			id := record.Results[0].Query.Query
			scope, _ := store.NewScope(f.execute.Tenant(), f.execute.User())
			before, err := f.f.f.db.ReadQuery(ctx, scope, id)
			if err != nil || before.PlanOperation == "" || before.Result == nil {
				t.Fatal("original custody", err)
			}
			if mode == "baseline" {
				captured, err := f.blocks.CaptureQuery(ctx, f.blockAuthor, reporting.CaptureRequest{ID: "independent-capture", Query: id, Metadata: f.base.Metadata, Outputs: f.base.Outputs[:1]})
				if err != nil {
					t.Fatal("independent capture", err)
				}
				phase27ValidatePublish(t, f.blocks, f.blockAuthor, captured)
			}
			childID, childRunID := "", ""
			if mode == "later_run" {
				if _, err = f.query.Run(ctx, f.execute, nlqexec.RunRequest{QueryID: id, Operation: "explicit-later-run", Rows: 20, Bytes: 65536}); err != nil {
					t.Fatal("explicit rerun", err)
				}
				after, err := f.f.f.db.ReadQuery(ctx, scope, id)
				if err != nil || after.Operation != "explicit-later-run" || after.PlanOperation != before.PlanOperation {
					t.Fatal("operation did not change exactly", err)
				}
				t.Logf("rerun changed mutable operation; immutable original remained: query=%s", id)
			}
			if mode == "saved_descendant" {
				childDefinition := phase29Text("Synthetic session descendant")
				widget := f.queryWidget()
				widget.Query.Durability, widget.Query.Question, widget.Query.Query = "session_bound", "", id
				widget.Query.Selections = nil
				childDefinition.Widgets = append(childDefinition.Widgets, widget)
				childState := f.report(t, "retention-child", childDefinition, false)
				childRun, err := f.compositions.Admit(ctx, f.execute, "report", childState.ID, reporting.CompositionRequest{Key: "child-run", Preview: true, Reference: reporting.DocumentReference{Revision: 1}})
				if err != nil {
					t.Fatal("admit child", err)
				}
				childRun, err = f.compositions.Run(ctx, f.execute, childRun.ID, false)
				if err != nil || !childRun.Complete {
					t.Fatal("execute child", err)
				}
				childRecord, err := f.f.f.db.ReadComposition(ctx, f.execute, childRun.ID)
				if err != nil || len(childRecord.Results) != 1 || childRecord.Results[0].Query == nil {
					t.Fatal("child checkpoint", err)
				}
				childID = childRecord.Results[0].Query.Query
				childRunID = childRun.ID
				child, err := f.f.f.db.ReadQuery(ctx, scope, childID)
				if err != nil || child.Parent != id || child.PlanOperation != "" {
					t.Fatal("child custody", err)
				}
			}
			deleted, deleteErr := f.documents.Delete(ctx, f.author, "report", state.ID, reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "retention-delete", Reason: "Synthetic retention audit"})
			raw := support.Raw(t, f.f.f.dsn)
			var retained, sqlValues, resultValues int
			if err := raw.QueryRow(ctx, `SELECT count(*),count(sql_text),count(result) FROM chartworks.nlq_queries WHERE tenant_id=$1 AND query_id=$2`, f.execute.Tenant(), id).Scan(&retained, &sqlValues, &resultValues); err != nil {
				t.Fatal(err)
			}
			t.Logf("delete_error=%v erased_queries=%d retained_origin=%d sql_values=%d result_values=%d descendant=%s", deleteErr, deleted.ErasedQueries, retained, sqlValues, resultValues, childID)
			if deleteErr != nil {
				t.Fatalf("owning document deletion failed: %v", deleteErr)
			}
			if retained != 0 {
				t.Fatalf("owning deletion left exact original query and retained material: rows=%d sql=%d results=%d", retained, sqlValues, resultValues)
			}
			if _, err := f.query.Run(ctx, f.execute, nlqexec.RunRequest{QueryID: id, Operation: "after-erasure", Rows: 20, Bytes: 65536}); err == nil {
				t.Fatal("erased query ran")
			}
			if mode == "baseline" {
				if _, err := f.blocks.Read(ctx, f.blockAuthor, "independent-capture", reporting.Reference{}); err != nil {
					t.Fatal("reviewed captured block erased with origin", err)
				}
				independent, err := f.runs.Admit(ctx, f.execute, "independent-capture", reporting.RunRequest{Key: "after-origin-delete", Outputs: []string{"table-main"}})
				if err != nil {
					t.Fatal("independent capture admission", err)
				}
				independent, err = f.runs.Run(ctx, f.execute, independent.ID, false)
				if err != nil || independent.State != "succeeded" {
					t.Fatal("independent capture execution", independent.State, err)
				}
			}
			if childID != "" {
				if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_queries WHERE tenant_id=$1 AND query_id=$2`, f.execute.Tenant(), childID).Scan(&retained); err != nil || retained != 0 {
					t.Fatal("saved copy retained", retained, err)
				}
				view, err := f.documents.Read(ctx, f.author, "report", "retention-child", reporting.DocumentReference{Revision: 1})
				if err != nil || len(view.UnavailableQueries) != 1 || view.UnavailableQueries[0] != "dynamic" {
					t.Fatal("independent definition missing explicit unavailable reference", view.UnavailableQueries, err)
				}
				if _, err := f.compositions.Widget(ctx, f.execute, childRunID, "main", "dynamic"); err == nil {
					t.Fatal("erased borrowed result served")
				}
				if next, err := f.compositions.Admit(ctx, f.execute, "report", "retention-child", reporting.CompositionRequest{Key: "reuse-deleted", Preview: true, Reference: reporting.DocumentReference{Revision: 1}}); err == nil {
					_, _ = f.compositions.Run(ctx, f.execute, next.ID, false)
					payload, err := f.compositions.Widget(ctx, f.execute, next.ID, "main", "dynamic")
					if err == nil && payload.Query != nil && payload.Query.Execution.Result != nil {
						t.Fatal("erased borrowed values recreated")
					}
				}
			}
		})
	}
}

// Queue actual mutations behind the same held transaction fence, with the
// deletion waiter first. Observe PostgreSQL lock waits rather than sleeping and
// assuming the goroutines reached the database.
func TestRetentionConcurrentMutationFences(t *testing.T) {
	for _, mode := range []string{"run", "saved_copy", "new_reference", "feedback"} {
		t.Run(mode, func(t *testing.T) {
			f := newPhase29Execution(t, true)
			ctx := t.Context()
			d := phase29Text("Concurrent synthetic retention")
			d.Widgets = append(d.Widgets, f.queryWidget())
			state := f.report(t, "concurrent-origin", d, true)
			run, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "origin-run"})
			if err != nil {
				t.Fatal(err)
			}
			run, err = f.compositions.Run(ctx, f.execute, run.ID, false)
			if err != nil || !run.Complete {
				t.Fatal(err)
			}
			record, err := f.f.f.db.ReadComposition(ctx, f.execute, run.ID)
			if err != nil || len(record.Results) != 1 || record.Results[0].Query == nil {
				t.Fatal(err)
			}
			id := record.Results[0].Query.Query
			saved := nlqexec.SavedQuestion{Durability: "session_bound", Query: id, Context: f.base.Context}
			for _, pin := range f.base.Topics {
				saved.Topics = append(saved.Topics, nlqexec.SavedTopic{Topic: pin.Topic, Version: pin.Version, Digest: pin.Digest})
			}
			evidence, err := f.query.InspectSaved(ctx, f.execute, saved)
			if err != nil {
				t.Fatal(err)
			}
			raw := support.Raw(t, f.f.f.dsn)
			observer := support.Raw(t, f.f.f.dsn)
			tx, err := raw.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,721415))`, f.execute.Tenant()); err != nil {
				t.Fatal(err)
			}
			deleted := make(chan error, 1)
			mutated := make(chan error, 1)
			go func() {
				_, e := f.documents.Delete(ctx, f.author, "report", state.ID, reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "delete", Reason: "Synthetic concurrent erasure"})
				deleted <- e
			}()
			waiters := func(want int) {
				t.Helper()
				deadline := time.Now().Add(10 * time.Second)
				for time.Now().Before(deadline) {
					var n int
					if err := observer.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory'`).Scan(&n); err != nil {
						t.Fatal(err)
					}
					if n >= want {
						return
					}
					time.Sleep(5 * time.Millisecond)
				}
				t.Fatal("mutation did not reach retention fence")
			}
			waiters(1)
			go func() {
				var e error
				switch mode {
				case "run":
					_, e = f.query.Run(ctx, f.execute, nlqexec.RunRequest{QueryID: id, Operation: "concurrent-run", Rows: 20, Bytes: 65536})
				case "saved_copy":
					_, e = f.query.PrepareSaved(ctx, f.execute, saved, evidence, "concurrent-copy", "en")
				case "feedback":
					e = f.query.Feedback(ctx, phase18Envelope(t, f.f, f.execute.User(), f.execute.Session(), true), nlqexec.FeedbackRequest{QueryID: id, Verdict: "positive"})
				case "new_reference":
					def := phase29Text("Independent authored reference")
					widget := f.queryWidget()
					widget.Query.Durability = "session_bound"
					widget.Query.Question = ""
					widget.Query.Selections = nil
					widget.Query.Query = id
					def.Widgets = append(def.Widgets, widget)
					_, e = f.documents.Create(ctx, f.author, "report", "concurrent-borrower", def)
				}
				mutated <- e
			}()
			waiters(2)
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-deleted:
				if err != nil {
					t.Fatal("delete", err)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("delete stuck")
			}
			select {
			case err := <-mutated:
				if err == nil {
					t.Fatal("mutation reused erased origin", mode)
				}
			case <-time.After(15 * time.Second):
				t.Fatal("mutation stuck")
			}
			var n int
			if err := observer.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_queries q JOIN chartworks.nlq_query_owners k USING(tenant_id,actor_id,session_id,query_id) WHERE k.tenant_id=$1 AND k.composition_root=$2`, f.execute.Tenant(), run.ID).Scan(&n); err != nil || n != 0 {
				t.Fatal("private material recreated", n, err)
			}
		})
	}
}

type retentionCommitBarrier struct {
	next    nlqexec.PlanExecutor
	reached chan struct{}
	release chan struct{}
}

func (b retentionCommitBarrier) Execute(ctx context.Context, e identity.Envelope, p exec.Plan, o exec.Options) (exec.ExecutionReport, error) {
	out, err := b.next.Execute(ctx, e, p, o)
	if err != nil {
		return out, err
	}
	close(b.reached)
	select {
	case <-b.release:
		return out, nil
	case <-ctx.Done():
		return exec.ExecutionReport{}, ctx.Err()
	}
}

func TestRetentionLateResultAndRendition(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := t.Context()
	def := phase29Text("Late commit origin")
	def.Widgets = append(def.Widgets, f.queryWidget())
	state := f.report(t, "late-origin", def, true)
	run, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "late-origin-run"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = f.compositions.Run(ctx, f.execute, run.ID, false)
	if err != nil || !run.Complete {
		t.Fatal(err)
	}
	record, err := f.f.f.db.ReadComposition(ctx, f.execute, run.ID)
	if err != nil || len(record.Results) != 1 || record.Results[0].Query == nil {
		t.Fatal(err)
	}
	id := record.Results[0].Query.Query
	// Retain an actually readable input before deletion. Persistence is delayed
	// below, so a stale rendering caller cannot recreate erased output.
	if payload, err := f.compositions.Widget(ctx, f.execute, run.ID, "main", "dynamic"); err != nil || payload.Query == nil {
		t.Fatal("previously readable view", err)
	}
	now := time.Now().UTC()
	rendition := rendering.Record{Tenant: f.execute.Tenant(), Actor: f.execute.User(), Session: f.execute.Session(), Request: rendering.Request{View: reporting.DeliveryViewRequest{Kind: "report", Run: run.ID}, Format: "html"}, Rendition: rendering.Rendition{ID: "rnd-" + strings.Repeat("1", 32), SourceDigest: strings.Repeat("a", 64), Digest: strings.Repeat("b", 64), WorkerVersion: "worker-v1", ThemeVersion: "theme-v1", Format: "html", Content: "synthetic private retained output", CreatedAt: now, ExpiresAt: now.Add(time.Minute)}}
	if _, err := f.f.f.db.PutRendition(ctx, rendition); err != nil {
		t.Fatal("initial rendition", err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	// A same-text ID in a different resource kind must not be erased by the
	// composition selector. This synthetic storage row isolates typed identity.
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.render_renditions SELECT x.* FROM chartworks.render_renditions r CROSS JOIN LATERAL jsonb_populate_record(NULL::chartworks.render_renditions,to_jsonb(r)||jsonb_build_object('rendition_id',$2::text,'request','\x'||encode(convert_to('{"view":{"kind":"block"}}','UTF8'),'hex'))) x WHERE r.tenant_id=$1 AND r.rendition_id=$3`, f.execute.Tenant(), "rnd-"+strings.Repeat("2", 32), rendition.Rendition.ID); err != nil {
		t.Fatal("typed rendition fixture", err)
	}
	_, topics := newPhase18Service(t, f.f)
	barrier := retentionCommitBarrier{next: f.f.f.executor, reached: make(chan struct{}), release: make(chan struct{})}
	service, err := nlqexec.New(f.f.service, topics, f.f.f.s, f.f.f.validator, barrier, f.f.model.engine, f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		out, e := service.Run(ctx, f.execute, nlqexec.RunRequest{QueryID: id, Operation: "late-physical-read", Rows: 20, Bytes: 65536})
		if e == nil || out.Execution.Result != nil {
			done <- fmt.Errorf("late result leaked: %v", e)
		} else {
			done <- nil
		}
	}()
	select {
	case <-barrier.reached:
	case <-time.After(15 * time.Second):
		t.Fatal("actual source did not reach commit barrier")
	}
	if _, err := f.documents.Delete(ctx, f.author, "report", state.ID, reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "erase", Reason: "Synthetic late commit test"}); err != nil {
		close(barrier.release)
		t.Fatal(err)
	}
	close(barrier.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("late commit blocked")
	}
	if _, err := f.f.f.db.PutRendition(ctx, rendition); err == nil {
		t.Fatal("late rendition recreated")
	}
	if _, err := f.f.f.db.ReadRendition(ctx, f.execute.Tenant(), rendition.Rendition.ID); err == nil {
		t.Fatal("erased rendition served")
	}
	var remaining int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.render_renditions WHERE tenant_id=$1 AND rendition_id=$2`, f.execute.Tenant(), "rnd-"+strings.Repeat("2", 32)).Scan(&remaining); err != nil || remaining != 1 {
		t.Fatal("different-kind rendition erased", remaining, err)
	}
}

func TestRetentionLegacyCheckpointAndExactGroupOwnership(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := t.Context()
	def := phase29Text("Legacy checkpoint identity")
	def.Widgets = append(def.Widgets, f.queryWidget())
	state := f.report(t, "legacy-owner", def, true)
	run, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "legacy-run"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = f.compositions.Run(ctx, f.execute, run.ID, false)
	if err != nil || !run.Complete {
		t.Fatal(err)
	}
	record, err := f.f.f.db.ReadComposition(ctx, f.execute, run.ID)
	if err != nil || len(record.Results) != 1 || record.Results[0].Query == nil {
		t.Fatal(err)
	}
	id := record.Results[0].Query.Query
	// This genuinely new authoring request deliberately resembles a root key,
	// but no persisted group owns it. Prefix resemblance grants no erase scope.
	independent, err := f.query.Plan(ctx, f.execute, nlqexec.PlanRequest{QuestionRequest: phase18Question(f.f, "en", f.f.pack.Topic), Operation: "composition:" + run.ID + ":independent-authoring"})
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	legacyID := strings.Repeat("e", 32)
	// Legacy-shaped metadata has no original Plan seal and its current operation
	// has moved. Recreate only the historical storage shape from real fixture data.
	var columns string
	if err := raw.QueryRow(ctx, `SELECT string_agg(quote_ident(attname),',' ORDER BY attnum) FROM pg_attribute WHERE attrelid='chartworks.nlq_queries'::regclass AND attnum>0 AND NOT attisdropped AND attgenerated=''`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.nlq_queries (`+columns+`) SELECT x.`+strings.ReplaceAll(columns, ",", ",x.")+` FROM chartworks.nlq_queries q CROSS JOIN LATERAL jsonb_populate_record(NULL::chartworks.nlq_queries,to_jsonb(q)||jsonb_build_object('query_id',$3::text,'operation','legacy-later-run','plan_operation',NULL,'plan_request_digest',NULL)) x WHERE q.tenant_id=$1 AND q.query_id=$2`, f.execute.Tenant(), id, legacyID); err != nil {
		t.Fatal("legacy query shape", err)
	}
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.composition_run_groups(tenant_id,operation_id,group_id,ordinal,kind,started,plan) VALUES($1,$2,'legacy-group',98,'query',true,convert_to(jsonb_build_object('query',$3::text,'operation','composition:'||$2::text||':legacy-group')::text,'UTF8'))`, f.execute.Tenant(), run.ID, legacyID); err != nil {
		t.Fatal("legacy authoritative checkpoint", err)
	}
	migrations, err := postgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sql := migrations[73].SQL
	start := strings.Index(sql, "INSERT INTO chartworks.nlq_query_owners\n")
	if start < 0 {
		t.Fatal("missing custody backfill")
	}
	end := strings.Index(sql[start:], "ON CONFLICT DO NOTHING;") + len("ON CONFLICT DO NOTHING;")
	if end < len("ON CONFLICT DO NOTHING;") {
		t.Fatal("backfill boundary")
	}
	if _, err := raw.Exec(ctx, sql[start:start+end]); err != nil {
		t.Fatal("actual legacy ownership backfill", err)
	}
	var owners int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_query_owners WHERE tenant_id=$1 AND query_id=$2 AND composition_root=$3`, f.execute.Tenant(), legacyID, run.ID).Scan(&owners); err != nil || owners != 1 {
		t.Fatal("legacy identity not recovered", owners, err)
	}
	if _, err := f.documents.Delete(ctx, f.author, "report", state.ID, reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "legacy-delete", Reason: "Synthetic legacy custody"}); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_queries WHERE tenant_id=$1 AND query_id=$2`, f.execute.Tenant(), legacyID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("legacy moved-operation query retained", remaining, err)
	}
	scope, _ := store.NewScope(f.execute.Tenant(), f.execute.User())
	if _, err := f.f.f.db.ReadQuery(ctx, scope, independent.QueryID); err != nil {
		t.Fatal("prefix-only authoring query erased", err)
	}
}

func TestRetentionLearningContributionCustody(t *testing.T) {
	for _, mode := range []string{"sole", "shared_last_origin_overwritten", "reviewed", "legacy_incomplete", "review_race", "review_wins", "requalified_unreviewed"} {
		t.Run(mode, func(t *testing.T) {
			f := newPhase29Execution(t, true)
			ctx := t.Context()
			canary := "ERASED_PRIVATE_EXAMPLE_CANARY"
			def := phase29Text("Feedback custody")
			widget := f.queryWidget()
			widget.Query.Question += " " + canary
			def.Widgets = append(def.Widgets, widget)
			state := f.report(t, "learning-owner", def, true)
			run, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "learning-run"})
			if err != nil {
				t.Fatal(err)
			}
			run, err = f.compositions.Run(ctx, f.execute, run.ID, false)
			if err != nil || !run.Complete {
				t.Fatal(err)
			}
			record, err := f.f.f.db.ReadComposition(ctx, f.execute, run.ID)
			if err != nil || len(record.Results) != 1 || record.Results[0].Query == nil {
				t.Fatal(err)
			}
			id := record.Results[0].Query.Query
			reviewer := phase18Envelope(t, f.f, f.execute.User(), f.execute.Session(), true)
			if err := f.query.Feedback(ctx, reviewer, nlqexec.FeedbackRequest{QueryID: id, Verdict: "positive"}); err != nil {
				t.Fatal("owned feedback", err)
			}
			raw := support.Raw(t, f.f.f.dsn)
			scope, _ := store.NewScope(f.execute.Tenant(), f.execute.User())
			var exampleID string
			if err := raw.QueryRow(ctx, `SELECT example_id FROM chartworks.nlq_example_contributions WHERE tenant_id=$1 AND query_id=$2`, f.execute.Tenant(), id).Scan(&exampleID); err != nil {
				t.Fatal("contribution origin", err)
			}
			candidate, err := f.f.f.db.ReadExample(ctx, scope, exampleID)
			if err != nil {
				t.Fatal(err)
			}
			if mode == "shared_last_origin_overwritten" {
				question := phase18Question(f.f, "en", f.f.pack.Topic)
				question.Question = widget.Query.Question
				other, err := f.query.Plan(ctx, f.execute, nlqexec.PlanRequest{QuestionRequest: question, Operation: "independent-contributor"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := f.query.Run(ctx, f.execute, nlqexec.RunRequest{QueryID: other.QueryID, Operation: "independent-contributor", Rows: 20, Bytes: 65536}); err != nil {
					t.Fatal(err)
				}
				if err := f.query.Feedback(ctx, reviewer, nlqexec.FeedbackRequest{QueryID: other.QueryID, Verdict: "positive"}); err != nil {
					t.Fatal(err)
				}
				var count int
				if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_example_contributions WHERE tenant_id=$1 AND example_id=$2`, f.execute.Tenant(), exampleID).Scan(&count); err != nil || count != 2 {
					t.Fatal("shared contribution did not aggregate", count, err)
				}
				current, err := f.f.f.db.ReadExample(ctx, scope, exampleID)
				if err != nil || current.Provenance == candidate.Provenance {
					t.Fatal("last provenance was not overwritten", err)
				}
			}
			requalifiedID := ""
			if mode == "requalified_unreviewed" {
				_, published := newPhase18Service(t, f.f)
				publisher := phase18Envelope(t, f.f, f.f.f.e.User(), f.f.f.e.Session(), true)
				draftService, err := drafts.New(f.f.f.db, f.f.f.s, f.f.f.service)
				if err != nil {
					t.Fatal(err)
				}
				current, err := draftService.Read(ctx, publisher, f.f.pack.Topic, 0)
				if err != nil {
					t.Fatal(err)
				}
				pack := cloneTopic(t, current.Pack)
				pack.Version = "retention-requal-v2"
				pack.Description = "Explicit synthetic requalification publication"
				next, err := draftService.Save(ctx, publisher, drafts.SaveRequest{Expected: current.Metadata.Revision, Pack: pack, Change: "Synthetic current semantic review"})
				if err != nil {
					t.Fatal(err)
				}
				review, err := published.Review(ctx, publisher, pack.Topic, topics.ReviewRequest{DraftRevision: next.Metadata.Revision, Digest: next.Metadata.Digest, Decision: "approve", Note: "Synthetic current semantic review"})
				if err != nil {
					t.Fatal(err)
				}
				previous, err := published.Read(ctx, publisher, pack.Topic, "")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := published.Publish(ctx, publisher, pack.Topic, topics.PublishRequest{Expected: previous.State.Revision, Review: review.ID}); err != nil {
					t.Fatal(err)
				}
				copy, err := f.query.RequalifyExample(ctx, reviewer, nlqexec.ExampleRequalificationRequest{ExampleID: exampleID, ExpectedVersion: candidate.Version, Anchor: phase18Question(f.f, "en", f.f.pack.Topic)})
				if err != nil {
					t.Fatal("explicit unreviewed requalification", err)
				}
				requalifiedID = copy.ID
				var contributions int
				if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_example_contributions WHERE tenant_id=$1 AND example_id=$2`, f.execute.Tenant(), copy.ID).Scan(&contributions); err != nil || contributions != 1 {
					t.Fatal("requalified copy lost contribution lineage", contributions, err)
				}
			}
			if mode == "reviewed" {
				if _, err := f.query.ExampleState(ctx, reviewer, nlqexec.ExampleStateRequest{ExampleID: exampleID, State: "active", ExpectedVersion: candidate.Version, ReviewNote: "Independent current-source review of synthetic example"}); err != nil {
					t.Fatal("review", err)
				}
			}
			if mode == "legacy_incomplete" {
				legacy := candidate
				legacy.ID = strings.Repeat("d", 32)
				legacy.EvidenceOutcome = "positive"
				legacy.Provenance = "legacy-aggregate"
				legacy.Version = 1
				if _, err := f.f.f.db.UpsertExample(ctx, scope, legacy); err != nil {
					t.Fatal("legacy aggregate shape", err)
				}
				migrations, err := postgres.Migrations()
				if err != nil {
					t.Fatal(err)
				}
				sql := migrations[73].SQL
				start := strings.Index(sql, "INSERT INTO chartworks.nlq_example_quarantine(tenant_id,example_id,reason)\n")
				stop := strings.Index(sql[start:], ";\n") + 2
				if start < 0 || stop < 2 {
					t.Fatal("legacy quarantine boundary")
				}
				if _, err := raw.Exec(ctx, sql[start:start+stop]); err != nil {
					t.Fatal("legacy quarantine", err)
				}
			}
			var deleted reporting.DocumentDeletion
			deleteRequest := reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "learning-delete", Reason: "Synthetic learning retention"}
			if mode == "review_race" || mode == "review_wins" {
				tx, e := raw.Begin(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer tx.Rollback(context.Background())
				if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,721415))`, f.execute.Tenant()); e != nil {
					t.Fatal(e)
				}
				observer := support.Raw(t, f.f.f.dsn)
				waiters := func(want int) {
					t.Helper()
					deadline := time.Now().Add(10 * time.Second)
					for time.Now().Before(deadline) {
						var n int
						if e := observer.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event='advisory'`).Scan(&n); e != nil {
							t.Fatal(e)
						}
						if n >= want {
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
					t.Fatal("review/delete did not reach fence")
				}
				type deletionResult struct {
					out reporting.DocumentDeletion
					err error
				}
				deletedCh := make(chan deletionResult, 1)
				reviewedCh := make(chan error, 1)
				startDelete := func() {
					go func() {
						out, e := f.documents.Delete(ctx, f.author, "report", state.ID, deleteRequest)
						deletedCh <- deletionResult{out, e}
					}()
				}
				startReview := func() {
					go func() {
						_, e := f.query.ExampleState(ctx, reviewer, nlqexec.ExampleStateRequest{ExampleID: exampleID, State: "active", ExpectedVersion: candidate.Version, ReviewNote: "Concurrent independent source review"})
						reviewedCh <- e
					}()
				}
				if mode == "review_race" {
					startDelete()
					waiters(1)
					startReview()
				} else {
					startReview()
					waiters(1)
					startDelete()
				}
				waiters(2)
				if e := tx.Commit(ctx); e != nil {
					t.Fatal(e)
				}
				select {
				case result := <-deletedCh:
					deleted, err = result.out, result.err
				case <-time.After(15 * time.Second):
					t.Fatal("delete stalled")
				}
				select {
				case e := <-reviewedCh:
					if (mode == "review_race" && e == nil) || (mode == "review_wins" && e != nil) {
						t.Fatal("review/delete ordering violated", mode, e)
					}
				case <-time.After(15 * time.Second):
					t.Fatal("review stalled")
				}
			} else {
				deleted, err = f.documents.Delete(ctx, f.author, "report", state.ID, deleteRequest)
			}
			if err != nil {
				t.Fatal(err)
			}
			if deleted.LearningRetention == "" {
				t.Fatal("missing retained-payload qualification")
			}
			remaining, readErr := f.f.f.db.ReadExample(ctx, scope, exampleID)
			switch mode {
			case "requalified_unreviewed":
				if readErr == nil || deleted.ErasedExamples != 2 {
					t.Fatal("unreviewed requalification escaped erasure", readErr, deleted)
				}
				if _, err := f.f.f.db.ReadExample(ctx, scope, requalifiedID); err == nil {
					t.Fatal("unreviewed requalified payload survived")
				}
			case "sole", "review_race":
				if readErr == nil || deleted.ErasedExamples != 1 {
					t.Fatal("sole unreviewed payload survived", readErr, deleted)
				}
			case "reviewed", "review_wins":
				if readErr != nil || remaining.State != "active" || deleted.ErasedExamples != 0 {
					t.Fatal("independently reviewed example erased", readErr, deleted)
				}
			default:
				if readErr != nil {
					t.Fatal("shared/unproved payload deleted", readErr)
				}
				var count int
				if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_example_quarantine WHERE tenant_id=$1 AND example_id=$2`, f.execute.Tenant(), exampleID).Scan(&count); err != nil || count != 1 {
					t.Fatal("candidate not quarantined", count, err)
				}
			}
			if mode != "reviewed" && mode != "review_wins" {
				examples, err := f.f.f.db.ListExamples(ctx, scope, f.f.pack.Topic, 64)
				if err != nil {
					t.Fatal(err)
				}
				for _, example := range examples {
					if example.ID == exampleID {
						t.Fatal("quarantined candidate served")
					}
				}
				f.f.model.mu.Lock()
				beforeBodies := len(f.f.model.requestBodies)
				f.f.model.mu.Unlock()
				planned, err := f.query.Plan(ctx, f.execute, nlqexec.PlanRequest{QuestionRequest: phase18Question(f.f, "en", f.f.pack.Topic), Operation: "post-erasure-generation"})
				if err != nil {
					t.Fatal(err)
				}
				query, err := f.f.f.db.ReadQuery(ctx, scope, planned.QueryID)
				if err != nil {
					t.Fatal(err)
				}
				for _, selected := range query.ExampleSelection.Selected {
					if selected.ExampleID == exampleID {
						t.Fatal("quarantined example selected")
					}
				}
				f.f.model.mu.Lock()
				bodies := append([]string(nil), f.f.model.requestBodies[beforeBodies:]...)
				f.f.model.mu.Unlock()
				for _, body := range bodies {
					if strings.Contains(body, canary) {
						t.Fatal("private example leaked to generation prompt")
					}
				}
			}
		})
	}
}

func TestRetentionExpiredLegacyOwnershipLimit(t *testing.T) {
	f := newPhase29Execution(t, true)
	ctx := t.Context()
	def := phase29Text("Expired ownership evidence")
	def.Widgets = append(def.Widgets, f.queryWidget())
	state := f.report(t, "expired-origin", def, true)
	limits := f.limits
	limits.Execution.Retention = config.Duration(time.Minute)
	limits.Execution.PreviewRetention = config.Duration(time.Minute)
	limits.Execution.MaxReuseAge = config.Duration(time.Second)
	compositions := f.withLimits(t, limits)
	run, err := compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "expiring-origin"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = compositions.Run(ctx, f.execute, run.ID, false)
	if err != nil || !run.Complete {
		t.Fatal(err)
	}
	record, err := f.f.f.db.ReadComposition(ctx, f.execute, run.ID)
	if err != nil || len(record.Results) != 1 || record.Results[0].Query == nil {
		t.Fatal(err)
	}
	id := record.Results[0].Query.Query
	if _, err := f.query.Run(ctx, f.execute, nlqexec.RunRequest{QueryID: id, Operation: "prospective-moved-key", Rows: 20, Bytes: 65536}); err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.f.dsn)
	legacyID := strings.Repeat("a", 32)
	var columns string
	if err := raw.QueryRow(ctx, `SELECT string_agg(quote_ident(attname),',' ORDER BY attnum) FROM pg_attribute WHERE attrelid='chartworks.nlq_queries'::regclass AND attnum>0 AND NOT attisdropped AND attgenerated=''`).Scan(&columns); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.nlq_queries (`+columns+`) SELECT x.`+strings.ReplaceAll(columns, ",", ",x.")+` FROM chartworks.nlq_queries q CROSS JOIN LATERAL jsonb_populate_record(NULL::chartworks.nlq_queries,to_jsonb(q)||jsonb_build_object('query_id',$3::text,'operation','historical-moved-key','plan_operation',NULL,'plan_request_digest',NULL)) x WHERE q.tenant_id=$1 AND q.query_id=$2`, f.execute.Tenant(), id, legacyID); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(ctx, `INSERT INTO chartworks.composition_run_groups(tenant_id,operation_id,group_id,ordinal,kind,started,plan) VALUES($1,$2,'legacy-expired-group',98,'query',true,convert_to(jsonb_build_object('query',$3::text,'operation','composition:'||$2::text||':legacy-expired-group')::text,'UTF8'))`, f.execute.Tenant(), run.ID, legacyID); err != nil {
		t.Fatal(err)
	}
	// Use the public retention boundary and its real clock, not trigger bypass.
	if delay := time.Until(run.Expires.Add(20 * time.Millisecond)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	eraser := phase27Actor(t, f.f, f.execute.User(), []string{"reporting.retention", "cw.tenant.erase:" + f.execute.Tenant()})
	if n, err := compositions.Expire(ctx, eraser, 10); err != nil || n != 1 {
		t.Fatal("actual composition expiry", n, err)
	}
	migrations, err := postgres.Migrations()
	if err != nil {
		t.Fatal(err)
	}
	sql := migrations[73].SQL
	start := strings.Index(sql, "INSERT INTO chartworks.nlq_query_owners\n")
	stop := strings.Index(sql[start:], "ON CONFLICT DO NOTHING;") + len("ON CONFLICT DO NOTHING;")
	if _, err := raw.Exec(ctx, sql[start:start+stop]); err != nil {
		t.Fatal(err)
	}
	var legacyOwners, prospectiveOwners int
	if err := raw.QueryRow(ctx, `SELECT count(*) FILTER(WHERE query_id=$2),count(*) FILTER(WHERE query_id=$3) FROM chartworks.nlq_query_owners WHERE tenant_id=$1 AND composition_root=$4`, f.execute.Tenant(), legacyID, id, run.ID).Scan(&legacyOwners, &prospectiveOwners); err != nil || legacyOwners != 0 || prospectiveOwners != 1 {
		t.Fatal("ownership evidence invented/lost", legacyOwners, prospectiveOwners, err)
	}
	deleted, err := f.documents.Delete(ctx, f.author, "report", state.ID, reporting.DocumentDeleteRequest{ExpectedVersion: state.Version, Key: "expired-delete", Reason: "Synthetic historical evidence limit"})
	if err != nil || deleted.QueryRetention != "legacy_expired_origins_unproven" || deleted.ErasedQueries != 1 {
		t.Fatal("missing explicit bounded deletion result", deleted, err)
	}
	var legacyRows, prospectiveRows int
	if err := raw.QueryRow(ctx, `SELECT count(*) FILTER(WHERE query_id=$2),count(*) FILTER(WHERE query_id=$3) FROM chartworks.nlq_queries WHERE tenant_id=$1`, f.execute.Tenant(), legacyID, id).Scan(&legacyRows, &prospectiveRows); err != nil || legacyRows != 1 || prospectiveRows != 0 {
		t.Fatal("erasure guessed legacy ownership or lost prospective custody", legacyRows, prospectiveRows, err)
	}
}
