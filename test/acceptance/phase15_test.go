package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	semantictopics "github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase15(t *testing.T) {
	t.Run("AC01", TestTopicPublicationRaceFailureAndContextTransition)
	t.Run("AC02", TestTopicPublicationAPIAndAtomicLifecycle)
	t.Run("AC03", TestTopicDraftOnboardingEntityMutationAndRebind)
	t.Run("AC04", testPhase15HealthRecheck)
	t.Run("AC05", testPhase15ResumableEnhancement)
	t.Run("AC06", TestTopicDraftAPIAndSDK)
}

func testPhase15HealthRecheck(t *testing.T) {
	f, draftsService, service, _, pack := publicationFixture(t)
	ctx := context.Background()
	client := publicationClient(t, f, draftsService, service, topicScopes(f.e.Tenant()))
	draft, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Health candidate"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Approve health candidate"})
	if err != nil {
		t.Fatal(err)
	}
	published, err := client.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil {
		t.Fatal(err)
	}
	health, err := client.HealthTopic(ctx, pack.Topic)
	if err != nil || !health.Healthy || health.Revision != published.State.Revision || len(health.Issues) != 0 {
		t.Fatal("initial retained health", health, err)
	}
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	metadata := support.Raw(t, f.dsn)
	injectedIssue := []semantictopics.HealthIssue{{Source: pack.Datasets[0].Source.Source, Context: pack.Datasets[0].Source.Context, Dataset: pack.Datasets[0].ID, Code: "schema_changed"}}
	if _, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.reject_health_write_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected health write failure'; END; $$; CREATE TRIGGER reject_health_write_test BEFORE INSERT OR UPDATE ON chartworks.topic_health FOR EACH ROW EXECUTE FUNCTION chartworks.reject_health_write_test()`); err != nil {
		t.Fatal("install health write failure", err)
	}
	if _, err = f.db.SaveTopicHealth(ctx, e, published, injectedIssue); err == nil {
		t.Fatal("injected health write failure committed")
	}
	retained, err := client.HealthTopic(ctx, pack.Topic)
	if err != nil || !retained.Healthy || retained.ObservedAt != health.ObservedAt {
		t.Fatal("failed health write changed retained state", retained, err)
	}
	if _, err = metadata.Exec(ctx, `DROP TRIGGER reject_health_write_test ON chartworks.topic_health; DROP FUNCTION chartworks.reject_health_write_test()`); err != nil {
		t.Fatal("remove health write failure", err)
	}
	if _, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.reject_health_audit_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='topic.health_rechecked' THEN RAISE EXCEPTION 'injected health audit failure'; END IF; RETURN NEW; END; $$; CREATE TRIGGER reject_health_audit_test BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_health_audit_test()`); err != nil {
		t.Fatal("install health audit failure", err)
	}
	t.Cleanup(func() {
		_, _ = metadata.Exec(context.Background(), `DROP TRIGGER IF EXISTS reject_health_audit_test ON chartworks.audit_events; DROP FUNCTION IF EXISTS chartworks.reject_health_audit_test()`)
	})
	if _, err = f.db.SaveTopicHealth(ctx, e, published, injectedIssue); err == nil {
		t.Fatal("injected audit failure committed health")
	}
	retained, err = client.HealthTopic(ctx, pack.Topic)
	if err != nil || !retained.Healthy || retained.ObservedAt != health.ObservedAt {
		t.Fatal("failed health transaction changed retained state", retained, err)
	}
	if _, err = metadata.Exec(ctx, `DROP TRIGGER reject_health_audit_test ON chartworks.audit_events; DROP FUNCTION chartworks.reject_health_audit_test()`); err != nil {
		t.Fatal("remove health audit failure", err)
	}
	if _, err = f.s.Rotate(ctx, f.e, pack.Datasets[0].Source.Source, pack.Datasets[0].Source.SourceRevision); err != nil {
		t.Fatal("rotate source", err)
	}
	// Retained health is independent of private profiles and live source work.
	retained, err = client.HealthTopic(ctx, pack.Topic)
	if err != nil || !retained.Healthy || retained.ObservedAt != health.ObservedAt {
		t.Fatal("retained health changed without recheck", retained, err)
	}
	rechecked, err := client.RecheckTopic(ctx, pack.Topic)
	if err != nil || rechecked.Healthy || len(rechecked.Issues) != 1 || rechecked.Issues[0].Code != "source_revision_changed" {
		t.Fatal("source drift not retained", rechecked, err)
	}
	if _, err = client.TopicContract(ctx, pack.Topic); err == nil {
		t.Fatal("unhealthy source exposed query contract")
	}
	if _, err = f.db.SaveTopicHealth(ctx, e, published, nil); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unverified healthy result cleared issues", err)
	}
	retained, err = client.HealthTopic(ctx, pack.Topic)
	if err != nil || retained.Healthy || len(retained.Issues) != 1 {
		t.Fatal("failed clear changed retained issues", retained, err)
	}
	var stored bool
	if err = metadata.QueryRow(ctx, `SELECT healthy FROM chartworks.topic_health WHERE tenant_id=$1 AND topic_id=$2`, f.e.Tenant(), pack.Topic).Scan(&stored); err != nil || stored {
		t.Fatal("health observation not committed", err)
	}
	for _, issues := range [][]semantictopics.HealthIssue{
		{{Source: "bad/source", Context: pack.Datasets[0].Source.Context, Dataset: pack.Datasets[0].ID, Code: "schema_changed"}},
		{injectedIssue[0], injectedIssue[0]},
		{{Source: pack.Datasets[0].Source.Source, Context: pack.Datasets[0].Source.Context, Dataset: pack.Datasets[0].ID, Code: "unknown"}},
	} {
		if _, err = f.db.SaveTopicHealth(ctx, e, published, issues); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("malformed health evidence accepted", err)
		}
	}
	if _, err = f.db.SaveTopicHealth(ctx, identity.Envelope{}, published, injectedIssue); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("unauthorized health write accepted", err)
	}
	if _, err = f.db.ReadTopicHealth(ctx, identity.Envelope{}, published); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("unauthorized health read accepted", err)
	}
	stale := published
	stale.State.Revision++
	if _, err = f.db.SaveTopicHealth(ctx, e, stale, injectedIssue); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale publication health write accepted", err)
	}
	if _, err = f.db.ReadTopicHealth(ctx, e, stale); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("missing publication health read accepted", err)
	}
	var originalIssues []byte
	if err = metadata.QueryRow(ctx, `SELECT issues FROM chartworks.topic_health WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic).Scan(&originalIssues); err != nil {
		t.Fatal("read retained health payload", err)
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_health SET issues='not-json'::jsonb WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic); err == nil {
		t.Fatal("PostgreSQL accepted malformed JSON")
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_health SET healthy=true WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic); err != nil {
		t.Fatal("corrupt retained health consistency", err)
	}
	if _, err = f.db.ReadTopicHealth(ctx, e, published); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("inconsistent retained health accepted", err)
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_health SET healthy=false,issues=$3 WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic, originalIssues); err != nil {
		t.Fatal("restore retained health", err)
	}
	if _, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.reject_health_delete_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected health delete failure'; END; $$; CREATE TRIGGER reject_health_delete_test BEFORE DELETE ON chartworks.topic_health FOR EACH ROW EXECUTE FUNCTION chartworks.reject_health_delete_test()`); err != nil {
		t.Fatal("install health delete failure", err)
	}
	if _, err = client.ArchiveTopic(ctx, pack.Topic, sdk.ArchiveTopicRequest{Expected: published.State.Revision, Note: "Archive with injected failure"}); err == nil {
		t.Fatal("injected health delete archived topic")
	}
	if retained, err = client.HealthTopic(ctx, pack.Topic); err != nil || retained.Healthy || len(retained.Issues) != 1 {
		t.Fatal("failed archive changed retained health", retained, err)
	}
	if _, err = metadata.Exec(ctx, `DROP TRIGGER reject_health_delete_test ON chartworks.topic_health; DROP FUNCTION chartworks.reject_health_delete_test()`); err != nil {
		t.Fatal("remove health delete failure", err)
	}
	if _, err = client.ArchiveTopic(ctx, pack.Topic, sdk.ArchiveTopicRequest{Expected: published.State.Revision, Note: "Archive unhealthy topic"}); err != nil {
		t.Fatal("archive unhealthy topic", err)
	}
	if _, err = client.HealthTopic(ctx, pack.Topic); err == nil {
		t.Fatal("archived topic exposed health contract")
	}
	var retainedRows int
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.topic_health WHERE tenant_id=$1 AND topic_id=$2`, f.e.Tenant(), pack.Topic).Scan(&retainedRows); err != nil || retainedRows != 0 {
		t.Fatal("archived health observation retained", retainedRows, err)
	}
}

func enhancementResponse(t *testing.T, result any) string {
	t.Helper()
	content, err := json.Marshal(map[string]any{"results": []any{result}})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(map[string]any{"id": "recorded-enhance", "object": "chat.completion", "model": "model-enhance", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}})
	if err != nil {
		t.Fatal(err)
	}
	return "raw:" + string(wire)
}

func testPhase15ResumableEnhancement(t *testing.T) {
	f, _, e, pack := topicFixture(t)
	gatewayFixture := newGatewayFixture(t, nil)
	service, err := drafts.NewWithEngine(f.db, f.s, f.service, gatewayFixture.engine)
	if err != nil {
		t.Fatal(err)
	}
	client, _ := topicClient(t, f, service, e)
	ctx := context.Background()
	first, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Generation scaffold"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.db.TopicGenerationCheckpoint(ctx, e, pack.Topic, 0); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("zero checkpoint revision accepted", err)
	}
	if _, err = client.EnhanceTopicDraft(ctx, pack.Topic, sdk.EnhanceTopicRequest{Expected: first.Metadata.Revision, Version: "skip", Cursor: 1, Limit: 1, Change: "Skip initial column"}); err == nil || gatewayFixture.requests.Load() != 0 {
		t.Fatal("initial generation cursor skipped", err)
	}
	columns := []semantics.Reference{{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "amount"}, {Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}}
	gatewayFixture.mode.Store(enhancementResponse(t, map[string]any{"dataset": columns[0].Dataset, "column": columns[0].ID, "kind": "unresolved", "reason": "Aggregation needs review"}))
	step1, err := client.EnhanceTopicDraft(ctx, pack.Topic, sdk.EnhanceTopicRequest{Expected: first.Metadata.Revision, Version: "v2", Cursor: 0, Limit: 1, Change: "First bounded enhancement"})
	if err != nil || step1.NextCursor != 1 || step1.Complete || len(step1.Draft.Pack.Unresolved) != 1 {
		t.Fatal("first generation checkpoint", step1.NextCursor, err)
	}
	unresolvedID := step1.Draft.Pack.Unresolved[0].ID
	if unresolvedID != semantics.GeneratedEntityID(semantics.EnhancementUnresolved, columns[0].Dataset, columns[0].ID) {
		t.Fatal("unstable unresolved ID", unresolvedID)
	}
	metadata := support.Raw(t, f.dsn)
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_generation_checkpoints SET receipt='{}'::jsonb WHERE tenant_id=$1 AND topic_id=$2 AND actor_id=$3 AND session_id=$4 AND draft_revision=$5`, e.Tenant(), pack.Topic, e.User(), e.Session(), step1.Draft.Metadata.Revision); err == nil {
		t.Fatal("immutable checkpoint receipt mutated")
	}
	if _, err = client.EnhanceTopicDraft(ctx, pack.Topic, sdk.EnhanceTopicRequest{Expected: step1.Draft.Metadata.Revision, Version: "rewind", Cursor: 0, Limit: 1, Change: "Rewind generation cursor"}); err == nil || gatewayFixture.requests.Load() != 1 {
		t.Fatal("generation cursor rewind reached gateway", err)
	}
	gatewayFixture.mode.Store(enhancementResponse(t, map[string]any{"dataset": columns[1].Dataset, "column": columns[1].ID, "kind": "dimension", "name": "Identifier", "role": "identifier"}))
	step2, err := client.EnhanceTopicDraft(ctx, pack.Topic, sdk.EnhanceTopicRequest{Expected: step1.Draft.Metadata.Revision, Version: "v3", Cursor: step1.NextCursor, Limit: 1, Change: "Resume bounded enhancement"})
	if err != nil || !step2.Complete || step2.NextCursor != 2 || len(step2.Draft.Pack.Unresolved) != 1 {
		t.Fatal("resumed generation", step2.NextCursor, err)
	}
	if _, err = client.EnhanceTopicDraft(ctx, pack.Topic, sdk.EnhanceTopicRequest{Expected: first.Metadata.Revision, Version: "stale", Cursor: 1, Limit: 1, Change: "Stale resume"}); err == nil {
		t.Fatal("stale generation checkpoint accepted")
	}
	var checkpoints int
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.topic_generation_checkpoints WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic).Scan(&checkpoints); err != nil || checkpoints != 2 {
		t.Fatal("durable checkpoints", checkpoints, err)
	}
	if gatewayFixture.requests.Load() != 2 {
		t.Fatalf("unexpected gateway requests: %d", gatewayFixture.requests.Load())
	}
	projected := semantictopics.Project(step2.Draft.Pack)
	if len(projected.Unresolved) != 1 || projected.Unresolved[0].ID != unresolvedID || len(projected.Dimensions) == 0 || projected.Dimensions[0].ID != semantics.GeneratedEntityID(semantics.EnhancementDimension, columns[1].Dataset, columns[1].ID) {
		t.Fatal("compact stable generation projection")
	}
	if _, err = f.db.ReadTopicDraft(ctx, e, pack.Topic, step1.Draft.Metadata.Revision, drafts.Read); err != nil {
		t.Fatal("resumable immutable checkpoint", err)
	}
}
