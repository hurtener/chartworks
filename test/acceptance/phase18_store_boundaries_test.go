package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func phase18StoreID(char string) string { return strings.Repeat(char, 32) }

func phase18StoreSession(id, tenant, actor, contextID, topic string, now time.Time) nlqexec.SessionRecord {
	return nlqexec.SessionRecord{
		ID: id, Tenant: tenant, Actor: actor, Context: contextID, Topics: []string{topic},
		Locale: nlq.LanguageEnglish, Created: now, Updated: now,
	}
}

func phase18StoreQuery(id, session, topic, contextID, operation string, ruleVersions []string, now time.Time) nlqexec.QueryRecord {
	return nlqexec.QueryRecord{
		ID: id, Session: session, Operation: operation, Topic: topic,
		Topics: []string{topic}, TopicVersions: []string{"v1"}, RuleVersions: ruleVersions,
		Context: contextID, Locale: nlq.LanguageEnglish, Question: "What is revenue?",
		Route: nlqroute.RouteResult{
			Outcome: nlq.StrategySingleTopic, Topic: topic, Topics: []string{topic},
			TopicVersions: []string{"v1"}, Confidence: 1,
		},
		Generation: nlq.GenerationContext{Strategy: nlq.GenerationDefault, Prompt: "synthetic", Tokens: 1, Budget: nlq.LowBudget},
		Status:     "planned", Revision: 1, Created: now, Updated: now,
	}
}

func TestPhase18PostgresRuntimeBoundaries(t *testing.T) {
	fixture := newPhase18Fixture(t)
	ctx := context.Background()
	scope, err := store.NewScope(fixture.f.e.Tenant(), fixture.f.e.User())
	if err != nil {
		t.Fatal("store scope", err)
	}
	metadata := support.Raw(t, fixture.f.dsn)
	now := time.Now().UTC()
	topic := fixture.pack.Topic
	contextID := fixture.context

	t.Run("invalid input is rejected before durable access", func(t *testing.T) {
		invalidSession := phase18StoreSession("", scope.Tenant(), scope.Actor(), contextID, topic, now)
		if err := fixture.f.db.CreateSession(ctx, scope, invalidSession); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("malformed session reached the store: %v", err)
		}
		if _, err := fixture.f.db.ReadSession(ctx, scope, ""); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("empty session key was accepted: %v", err)
		}
		invalidQuery := phase18StoreQuery("", "phase18-store-session", topic, contextID, "phase18-invalid", nil, now)
		if err := fixture.f.db.CreateQuery(ctx, scope, invalidQuery); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("malformed query reached the store: %v", err)
		}
		if _, err := fixture.f.db.ReadQuery(ctx, scope, ""); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("empty query key was accepted: %v", err)
		}
		if _, err := fixture.f.db.ReadOperation(ctx, scope, ""); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("empty operation key was accepted: %v", err)
		}
		if err := fixture.f.db.UpdateQuery(ctx, scope, phase18StoreQuery(phase18StoreID("9"), "phase18-store-session", topic, contextID, "phase18-invalid-update", nil, now), 0); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("invalid query CAS was accepted: %v", err)
		}
		if err := fixture.f.db.RecordFeedback(ctx, scope, nlqexec.FeedbackRecord{ID: phase18StoreID("9"), QueryID: phase18StoreID("9"), Session: "phase18-store-session", Verdict: "unknown", Created: now}); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("invalid feedback verdict was accepted: %v", err)
		}
		if _, err := fixture.f.db.ReadExample(ctx, scope, ""); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("empty example key was accepted: %v", err)
		}
	})

	t.Run("session_scope_duplicate_and_malformed_json", func(t *testing.T) {
		session := phase18StoreSession("phase18-store-session", scope.Tenant(), scope.Actor(), contextID, topic, now)
		if err := fixture.f.db.CreateSession(ctx, scope, session); err != nil {
			t.Fatal("create session", err)
		}
		if err := fixture.f.db.CreateSession(ctx, scope, session); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate session was not rejected as a conflict: %v", err)
		}
		read, err := fixture.f.db.ReadSession(ctx, scope, session.ID)
		if err != nil || read.ID != session.ID || read.Context != contextID || len(read.Topics) != 1 || read.Topics[0] != topic {
			t.Fatalf("session round trip changed durable anchor: %#v %v", read, err)
		}
		otherActor, err := store.NewScope(scope.Tenant(), "phase18-other-actor")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.f.db.ReadSession(ctx, otherActor, session.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign actor read disclosed or accepted the session: %v", err)
		}
		otherTenant, err := store.NewScope("phase18-other-tenant", scope.Actor())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.f.db.ReadSession(ctx, otherTenant, session.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign tenant read disclosed or accepted the session: %v", err)
		}

		corruptID := "phase18-corrupt-session"
		if _, err = metadata.Exec(ctx, `INSERT INTO chartworks.nlq_sessions(tenant_id,actor_id,session_id,context_id,topics,locale,created_at,updated_at) VALUES($1,$2,$3,$4,$5::jsonb,'en',$6,$6)`, scope.Tenant(), scope.Actor(), corruptID, contextID, "[1]", now); err != nil {
			t.Fatal("insert malformed retained session", err)
		}
		if _, err = fixture.f.db.ReadSession(ctx, scope, corruptID); !errors.Is(err, store.ErrMigration) {
			t.Fatalf("malformed retained session was not rejected safely: %v", err)
		}
	})

	t.Run("query_scope_conflict_cas_and_malformed_json", func(t *testing.T) {
		sessionID := "phase18-store-session"
		query := phase18StoreQuery(phase18StoreID("a"), sessionID, topic, contextID, "phase18-store-operation", nil, now)
		if err := fixture.f.db.CreateQuery(ctx, scope, query); err != nil {
			t.Fatal("create query", err)
		}
		if err := fixture.f.db.CreateQuery(ctx, scope, query); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate query identity was not rejected as a conflict: %v", err)
		}
		read, err := fixture.f.db.ReadQuery(ctx, scope, query.ID)
		if err != nil || read.ID != query.ID || read.Operation != query.Operation || read.Revision != 1 {
			t.Fatalf("query round trip changed durable metadata: %#v %v", read, err)
		}
		operation, err := fixture.f.db.ReadOperation(ctx, scope, query.Operation)
		if err != nil || operation.ID != query.ID {
			t.Fatalf("operation lookup did not use the scoped durable key: %#v %v", operation, err)
		}
		otherActor, err := store.NewScope(scope.Tenant(), "phase18-query-actor")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.f.db.ReadQuery(ctx, otherActor, query.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign actor read disclosed or accepted the query: %v", err)
		}
		if _, err = fixture.f.db.ReadOperation(ctx, otherActor, query.Operation); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign actor operation read disclosed or accepted the query: %v", err)
		}
		otherTenant, err := store.NewScope("phase18-query-tenant", scope.Actor())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.f.db.ReadQuery(ctx, otherTenant, query.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign tenant read disclosed or accepted the query: %v", err)
		}

		updated := read
		updated.Operation = "phase18-store-operation-updated"
		updated.Status = "succeeded"
		updated.Revision = 2
		if err = fixture.f.db.UpdateQuery(ctx, scope, updated, 1); err != nil {
			t.Fatal("update query", err)
		}
		if err = fixture.f.db.UpdateQuery(ctx, scope, updated, 1); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("stale query revision was not rejected: %v", err)
		}

		resultQuery := phase18StoreQuery(phase18StoreID("7"), sessionID, topic, contextID, "phase18-store-result", nil, now)
		resultQuery.Result = &readexec.Result{
			Schema: []readexec.Field{{Name: "amount", Type: "integer", Encoding: "text", NativeType: "int8"}},
			Rows:   [][]json.RawMessage{{json.RawMessage("1")}}, Outcome: "complete", Truncation: "none", Bytes: 1,
		}
		if err = fixture.f.db.CreateQuery(ctx, scope, resultQuery); err != nil {
			t.Fatal("create result query", err)
		}
		resultRead, err := fixture.f.db.ReadQuery(ctx, scope, resultQuery.ID)
		if err != nil || resultRead.Result == nil || len(resultRead.Topics) != 1 || len(resultRead.TopicVersions) != 1 {
			t.Fatalf("optional query JSON did not round trip safely: %#v %v", resultRead, err)
		}

		malformedGeneration := phase18StoreQuery(phase18StoreID("b"), sessionID, topic, contextID, "phase18-store-malformed-generation", nil, now)
		if err = fixture.f.db.CreateQuery(ctx, scope, malformedGeneration); err != nil {
			t.Fatal("create malformed-generation query", err)
		}
		if _, err = metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET generation=$4::jsonb,revision=revision+1 WHERE tenant_id=$1 AND actor_id=$2 AND query_id=$3`, scope.Tenant(), scope.Actor(), malformedGeneration.ID, `{"selected":"not-an-array"}`); err != nil {
			t.Fatal("corrupt generation fixture", err)
		}
		if _, err = fixture.f.db.ReadQuery(ctx, scope, malformedGeneration.ID); !errors.Is(err, store.ErrMigration) {
			t.Fatalf("malformed retained generation was not rejected safely: %v", err)
		}

		malformedResult := phase18StoreQuery(phase18StoreID("c"), sessionID, topic, contextID, "phase18-store-malformed-result", nil, now)
		if err = fixture.f.db.CreateQuery(ctx, scope, malformedResult); err != nil {
			t.Fatal("create malformed-result query", err)
		}
		if _, err = metadata.Exec(ctx, `UPDATE chartworks.nlq_queries SET result=$4::jsonb,revision=revision+1 WHERE tenant_id=$1 AND actor_id=$2 AND query_id=$3`, scope.Tenant(), scope.Actor(), malformedResult.ID, `{"schema":"not-an-array"}`); err != nil {
			t.Fatal("corrupt result fixture", err)
		}
		if _, err = fixture.f.db.ReadQuery(ctx, scope, malformedResult.ID); !errors.Is(err, store.ErrMigration) {
			t.Fatalf("malformed retained result was not rejected safely: %v", err)
		}
	})

	t.Run("feedback_atomicity_and_example_lifecycle", func(t *testing.T) {
		feedbackQuery := phase18StoreQuery(phase18StoreID("d"), "phase18-store-session", topic, contextID, "phase18-store-feedback", nil, now)
		if err := fixture.f.db.CreateQuery(ctx, scope, feedbackQuery); err != nil {
			t.Fatal("create feedback query", err)
		}
		feedback := nlqexec.FeedbackRecord{ID: phase18StoreID("e"), QueryID: feedbackQuery.ID, Session: feedbackQuery.Session, Verdict: "positive", Note: "durable boundary", Provenance: "phase18-store-test", Created: now}
		if err := fixture.f.db.RecordFeedback(ctx, scope, feedback); err != nil {
			t.Fatal("record feedback", err)
		}
		if err := fixture.f.db.RecordFeedback(ctx, scope, feedback); !errors.Is(err, store.ErrConflict) {
			t.Fatalf("duplicate feedback was not rejected as a conflict: %v", err)
		}

		failureQuery := phase18StoreQuery(phase18StoreID("f"), "phase18-store-session", topic, contextID, "phase18-store-feedback-failure", nil, now)
		if err := fixture.f.db.CreateQuery(ctx, scope, failureQuery); err != nil {
			t.Fatal("create feedback failure query", err)
		}
		if _, err := metadata.Exec(ctx, `CREATE FUNCTION chartworks.reject_phase18_feedback_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='nlq.feedback_recorded' THEN RAISE EXCEPTION 'phase18 feedback audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_phase18_feedback_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_phase18_feedback_audit()`); err != nil {
			t.Fatal("install feedback failure fixture", err)
		}
		t.Cleanup(func() {
			_, _ = metadata.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_phase18_feedback_audit ON chartworks.audit_events; DROP FUNCTION IF EXISTS chartworks.reject_phase18_feedback_audit()`)
		})
		if err := fixture.f.db.RecordFeedback(ctx, scope, nlqexec.FeedbackRecord{ID: phase18StoreID("f"), QueryID: failureQuery.ID, Session: failureQuery.Session, Verdict: "negative", Provenance: "phase18-store-failure", Created: now}); err == nil {
			t.Fatal("audit failure did not abort feedback transaction")
		}
		var feedbackRows int
		if err := metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.nlq_feedback WHERE tenant_id=$1 AND feedback_id=$2`, scope.Tenant(), phase18StoreID("f")).Scan(&feedbackRows); err != nil || feedbackRows != 0 {
			t.Fatalf("failed feedback transaction left durable state: rows=%d err=%v", feedbackRows, err)
		}
		if _, err := metadata.Exec(ctx, `DROP TRIGGER reject_phase18_feedback_audit ON chartworks.audit_events; DROP FUNCTION chartworks.reject_phase18_feedback_audit()`); err != nil {
			t.Fatal("remove feedback failure fixture", err)
		}

		example := nlqexec.ExampleRecord{ID: phase18StoreID("1"), Topic: topic, Question: "What is revenue?", SQL: "SELECT amount FROM analytics.sales", Digest: strings.Repeat("1", 64), State: "candidate", Weight: 0.5, EvidenceCount: 1, Provenance: "phase18-store-example", Created: now, Updated: now}
		stored, err := fixture.f.db.UpsertExample(ctx, scope, example)
		if err != nil || stored.ID != example.ID || stored.EvidenceCount != 1 {
			t.Fatalf("initial example was not retained: %#v %v", stored, err)
		}
		stored, err = fixture.f.db.UpsertExample(ctx, scope, nlqexec.ExampleRecord{ID: phase18StoreID("2"), Topic: topic, Question: example.Question, SQL: example.SQL, Digest: example.Digest, State: "candidate", Weight: 0.1, EvidenceCount: 1, Provenance: "phase18-store-example-duplicate", Created: now, Updated: now})
		if err != nil || stored.ID != example.ID || stored.EvidenceCount != 2 || stored.Weight < 0.55 {
			t.Fatalf("duplicate example was not DB-first/deduplicated: %#v %v", stored, err)
		}
		listed, err := fixture.f.db.ListExamples(ctx, scope, topic, 8)
		if err != nil || len(listed) != 1 || listed[0].ID != example.ID {
			t.Fatalf("candidate example list was not bounded and scoped: %#v %v", listed, err)
		}
		read, err := fixture.f.db.ReadExample(ctx, scope, example.ID)
		if err != nil || read.ID != example.ID {
			t.Fatalf("exact example read failed: %#v %v", read, err)
		}
		otherTenant, err := store.NewScope(scope.Tenant()+"-example-tenant", scope.Actor())
		if err != nil {
			t.Fatal(err)
		}
		if _, err = fixture.f.db.ReadExample(ctx, otherTenant, example.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("foreign tenant example read disclosed or accepted the row: %v", err)
		}
		if _, err = fixture.f.db.ListExamples(ctx, scope, topic, 0); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("invalid example limit was accepted: %v", err)
		}
		if _, err = fixture.f.db.UpsertExample(ctx, scope, nlqexec.ExampleRecord{ID: phase18StoreID("3"), Topic: topic, State: "candidate"}); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("malformed example was accepted: %v", err)
		}
		if _, err = fixture.f.db.SetExampleState(ctx, scope, example.ID, "unsupported"); !errors.Is(err, store.ErrInvalid) {
			t.Fatalf("unsupported example state was accepted: %v", err)
		}
		active, err := fixture.f.db.SetExampleState(ctx, scope, example.ID, "active")
		if err != nil || active.State != "active" {
			t.Fatalf("candidate activation failed: %#v %v", active, err)
		}
		retired, err := fixture.f.db.SetExampleState(ctx, scope, example.ID, "retired")
		if err != nil || retired.State != "retired" {
			t.Fatalf("active retirement failed: %#v %v", retired, err)
		}
		if _, err = fixture.f.db.ReadExample(ctx, scope, "missing-example"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("missing example read was not non-disclosing: %v", err)
		}
		if _, err = fixture.f.db.SetExampleState(ctx, scope, "missing-example", "active"); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("missing example state change was not non-disclosing: %v", err)
		}

		atomicExample := nlqexec.ExampleRecord{ID: phase18StoreID("4"), Topic: topic, Question: "What is cash?", SQL: "SELECT cash FROM analytics.sales", Digest: strings.Repeat("2", 64), State: "candidate", Weight: 0.4, EvidenceCount: 1, Provenance: "phase18-store-atomic", Created: now, Updated: now}
		if _, err = fixture.f.db.UpsertExample(ctx, scope, atomicExample); err != nil {
			t.Fatal("create atomic example", err)
		}
		if _, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.reject_phase18_example_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='nlq.example_changed' THEN RAISE EXCEPTION 'phase18 example audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_phase18_example_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_phase18_example_audit()`); err != nil {
			t.Fatal("install example failure fixture", err)
		}
		t.Cleanup(func() {
			_, _ = metadata.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_phase18_example_audit ON chartworks.audit_events; DROP FUNCTION IF EXISTS chartworks.reject_phase18_example_audit()`)
		})
		if _, err = fixture.f.db.SetExampleState(ctx, scope, atomicExample.ID, "active"); err == nil {
			t.Fatal("audit failure did not abort example state transaction")
		}
		unchanged, err := fixture.f.db.ReadExample(ctx, scope, atomicExample.ID)
		if err != nil || unchanged.State != "candidate" {
			t.Fatalf("failed example state transaction changed durable state: %#v %v", unchanged, err)
		}
		if _, err = metadata.Exec(ctx, `DROP TRIGGER reject_phase18_example_audit ON chartworks.audit_events; DROP FUNCTION chartworks.reject_phase18_example_audit()`); err != nil {
			t.Fatal("remove example failure fixture", err)
		}
	})
}
