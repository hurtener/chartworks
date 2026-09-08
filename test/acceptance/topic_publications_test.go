package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/topicapi"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func publicationFixture(t *testing.T) (*engineeringFixture, *drafts.Service, *topics.Service, *gatewayFixture, semantics.TopicPack) {
	t.Helper()
	f, draftsService, _, pack := topicFixture(t)
	gatewayFixture := newGatewayFixture(t, func(cfg *config.Gateway) {
		embedding := cfg.Roles["embedding"]
		embedding.MaxBatchItems = 32
		embedding.MaxBatchBytes = 64 << 10
		cfg.Roles["embedding"] = embedding
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := topics.New(f.db, f.s, index, gatewayFixture.engine)
	if err != nil {
		t.Fatal(err)
	}
	return f, draftsService, service, gatewayFixture, pack
}

func publicationClient(t *testing.T, f *engineeringFixture, draftsService *drafts.Service, service *topics.Service, scopes []string) *sdk.Client {
	t.Helper()
	registry, err := topicapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, topicapi.Handler(f.token.verifier, draftsService, service, http.NotFoundHandler()))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return f.token.sign(t, f.token.claims(f.e.Tenant(), f.e.User(), scopes), nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestTopicPublicationAPIAndAtomicLifecycle(t *testing.T) {
	f, draftsService, service, gatewayFixture, pack := publicationFixture(t)
	ctx := context.Background()
	scopes := topicScopes(f.e.Tenant())
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), scopes...)
	client := publicationClient(t, f, draftsService, service, scopes)

	draft, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Publication candidate"})
	if err != nil {
		t.Fatal("save", err)
	}
	if _, err = service.Review(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: strings.ToUpper(draft.Metadata.Digest), Decision: "approve", Note: "Invalid digest"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("noncanonical review digest accepted", err)
	}
	rejected, err := service.Review(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "reject", Note: "Reject synthetic definition"})
	if err != nil {
		t.Fatal("reject review", err)
	}
	beforeRejected := gatewayFixture.requests.Load()
	if _, err = service.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: rejected.ID}); !errors.Is(err, store.ErrConflict) || gatewayFixture.requests.Load() != beforeRejected {
		t.Fatal("rejected review reached gateway or publication", err)
	}
	review, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Reviewed synthetic definition"})
	if err != nil || review.Digest != draft.Metadata.Digest || review.ID == "" {
		t.Fatal("review", review, err)
	}
	raw, _ := json.Marshal(review)
	if string(raw) == "" || containsAny(string(raw), "actor", "session", "profile") {
		t.Fatal("private provenance escaped review response", string(raw))
	}

	requests := gatewayFixture.requests.Load()
	narrow := f.token.envelope(t, f.e.Tenant(), f.e.User(), "topics.publish", "cw.topic.publish:"+pack.Topic, "cw.execution_context.use:"+pack.Datasets[0].Source.Context)
	if _, err = service.Publish(ctx, narrow, pack.Topic, topics.PublishRequest{Review: review.ID}); !errors.Is(err, access.ErrNotFound) || gatewayFixture.requests.Load() != requests {
		t.Fatal("dependency scope was not denied before gateway dispatch", err)
	}

	publishScopes := []string{"topics.publish", "sources.read", "cw.topic.publish:" + pack.Topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"}
	publishClient := publicationClient(t, f, draftsService, service, publishScopes)
	published, err := publishClient.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil || published.State.Revision != 1 || !published.State.Active || published.State.Archived || published.State.Version != pack.Version || len(published.State.Generations) != 1 {
		var status *sdk.StatusError
		_ = errors.As(err, &status)
		code := 0
		if status != nil {
			code = status.Status
		}
		t.Fatal("publish", published.State, err, code)
	}
	retained, err := client.PublishedTopic(ctx, pack.Topic)
	if err != nil || !reflect.DeepEqual(retained, published) {
		t.Fatal("retained read", err)
	}
	exact, err := client.PublishedTopicVersion(ctx, pack.Topic, pack.Version)
	if err != nil || !reflect.DeepEqual(exact, published) {
		t.Fatal("exact retained read", err)
	}
	contract, err := client.TopicContract(ctx, pack.Topic)
	if err != nil || contract.Publication.State.Version != pack.Version || contract.ObservedAt.IsZero() {
		t.Fatal("current contract", err)
	}

	index, _ := vindex.New(f.db)
	space := vindex.Space(gatewayFixture.engine.EmbeddingSpace())
	query := vindex.Query{ID: "published-search", Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Space: space, Vector: []float32{1, 2}, Kinds: []string{"topic", "entity", "measure"}, LimitPerKind: 10}
	results, err := index.Search(ctx, e, []vindex.Query{query})
	if err != nil || len(results) != 1 || len(results[0].Hits) == 0 || results[0].Publication.Version != pack.Version {
		t.Fatal("managed facet search", results, err)
	}
	scope := support.Scope(t, e.Tenant(), e.User())
	if _, err = f.db.SearchFacets(ctx, identity.Envelope{}, scope, []vindex.Query{query}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("raw coordinates accessed managed facets", err)
	}
	metadata := support.Raw(t, f.dsn)
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_published_versions SET receipt='{}' WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, e.Tenant(), pack.Topic, pack.Version); err == nil {
		t.Fatal("published version mutated")
	}
	extra, facets := vectorGeneration("extra-generation", "extra-context", 2, 1)
	extra.Topic, extra.Version, extra.SourceGeneration, extra.Space = pack.Topic, pack.Version, "extra-source", space
	if err = index.Begin(ctx, e, extra); err != nil {
		t.Fatal("stage extra generation", err)
	}
	if err = index.Upsert(ctx, e, extra, facets); err != nil {
		t.Fatal("stage extra facets", err)
	}
	manifest, _ := json.Marshal(extra)
	if _, err = metadata.Exec(ctx, `INSERT INTO chartworks.topic_published_generations(tenant_id,topic_id,version_id,context_id,generation_id,manifest) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), pack.Topic, pack.Version, extra.Context, extra.ID, manifest); err == nil {
		t.Fatal("active publication accepted unmatched context generation")
	}

	archived, err := client.ArchiveTopic(ctx, pack.Topic, sdk.ArchiveTopicRequest{Expected: 1, Note: "Archive synthetic topic"})
	if err != nil || archived.Revision != 2 || !archived.Archived || archived.Active {
		t.Fatal("archive", archived, err)
	}
	if _, err = service.Archive(ctx, e, pack.Topic, 2, "Repeated archive"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("no-op archive created evidence", err)
	}
	if _, err = index.Search(ctx, e, []vindex.Query{query}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("archived managed facets searchable", err)
	}
	beforeRollback := gatewayFixture.requests.Load()
	restored, err := client.RollbackTopic(ctx, pack.Topic, sdk.TopicTransitionRequest{Version: pack.Version, Expected: 2, Note: "Restore reviewed version"})
	if err != nil || restored.State.Revision != 3 || !restored.State.Active || gatewayFixture.requests.Load() != beforeRollback {
		t.Fatal("gateway-free rollback", restored.State, err)
	}
	if _, err = service.Rollback(ctx, e, pack.Topic, topics.TransitionRequest{Version: pack.Version, Expected: 3, Note: "Repeated rollback"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("no-op rollback created evidence", err)
	}
	if _, err = index.Archive(ctx, e, pack.Topic, pack.Datasets[0].Source.Context, 3); err == nil {
		t.Fatal("standalone vector archive desynchronized managed topic")
	}

	gatewayFixture.mode.Store("error")
	beforeRead := gatewayFixture.requests.Load()
	if _, err = client.PublishedTopic(ctx, pack.Topic); err != nil || gatewayFixture.requests.Load() != beforeRead {
		t.Fatal("retained read called gateway", err)
	}
	if _, err = f.s.Rotate(ctx, f.e, pack.Datasets[0].Source.Source, pack.Datasets[0].Source.SourceRevision); err != nil {
		t.Fatal("rotate", err)
	}
	if _, err = client.PublishedTopic(ctx, pack.Topic); err != nil {
		t.Fatal("retained read depended on current source", err)
	}
	if _, err = client.TopicContract(ctx, pack.Topic); err == nil {
		t.Fatal("stale source reported healthy")
	}
	if _, err = index.Search(ctx, e, []vindex.Query{query}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("managed search ignored current dependency fence", err)
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.sources SET deleted=true WHERE tenant_id=$1 AND source_id=$2`, e.Tenant(), pack.Datasets[0].Source.Source); err != nil {
		t.Fatal("delete source fixture", err)
	}
	if _, err = client.PublishedTopic(ctx, pack.Topic); err != nil {
		t.Fatal("retained read depended on deleted source", err)
	}
}

func TestTopicPublicationRaceFailureAndContextTransition(t *testing.T) {
	f, draftsService, service, gatewayFixture, pack := publicationFixture(t)
	ctx := context.Background()
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)

	secondSource := f.create(t, "topic-source-two")
	binding, err := f.s.Binding(ctx, f.e, secondSource.ID, secondSource.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	var itemDataset string
	for _, relation := range binding.Relations {
		if relation.Name == "items" {
			itemDataset = relation.ID
		}
	}
	if itemDataset == "" {
		t.Fatal("items relation missing")
	}
	secondProfile := f.profile(t, engineering.ProfileSpec{ID: "topic-profile-two", Source: secondSource.ID, Context: secondSource.ContextID, Dataset: itemDataset, Columns: []string{"sale_id", "quantity"}, SkipLLM: true})
	secondEvidence := secondProfile.Profile.Profile
	secondDataset := semantics.Dataset{ID: secondEvidence.Dataset, Name: "Items", Source: semantics.SourceReference{Source: secondSource.ID, Context: secondSource.ContextID, Dataset: secondEvidence.Dataset, SourceRevision: secondSource.Revision, ProfileVersion: secondEvidence.Version, ProfileDigest: secondEvidence.DeterministicHash()}}
	for _, column := range secondEvidence.Schema {
		if column.Name == "sale_id" || column.Name == "quantity" {
			secondDataset.Columns = append(secondDataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
		}
	}
	pack.Datasets = append(pack.Datasets, secondDataset)
	first, err := draftsService.Save(ctx, e, drafts.SaveRequest{Pack: pack, Change: "Two context publication"})
	if err != nil {
		t.Fatal("save first", err)
	}
	review, err := service.Review(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: 1, Digest: first.Metadata.Digest, Decision: "approve", Note: "Approve two contexts"})
	if err != nil {
		t.Fatal("review first", err)
	}

	results := make(chan error, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, err := service.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: review.ID})
			results <- err
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatal("publication CAS race", successes)
	}
	current, err := service.Read(ctx, e, pack.Topic, "")
	if err != nil || current.State.Revision != 1 || len(current.State.Generations) != 2 {
		t.Fatal("first publication", current.State, err)
	}

	pack.Version = "v2"
	pack.Datasets = pack.Datasets[:1]
	pack.Measures[0].Name = "Net revenue"
	second, err := draftsService.Save(ctx, e, drafts.SaveRequest{Expected: 1, Pack: pack, Change: "Remove secondary context"})
	if err != nil {
		t.Fatal("save second", err)
	}
	review2, err := service.Review(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: 2, Digest: second.Metadata.Digest, Decision: "approve", Note: "Approve context removal"})
	if err != nil {
		t.Fatal("review second", err)
	}
	gatewayFixture.mode.Store("error")
	if _, err = service.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: review2.ID, Expected: 1}); err == nil {
		t.Fatal("gateway failure published")
	}
	old, err := service.Read(ctx, e, pack.Topic, "")
	if err != nil || old.State.Version != "v1" || old.State.Revision != 1 || len(old.State.Generations) != 2 {
		t.Fatal("gateway failure replaced old publication", old.State, err)
	}
	gatewayFixture.mode.Store("normal")
	secondPublished, err := service.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: review2.ID, Expected: 1})
	if err != nil || secondPublished.State.Revision != 2 || len(secondPublished.State.Generations) != 1 {
		t.Fatal("second publication", secondPublished.State, err)
	}
	var oldArchived, keptArchived bool
	metadata := support.Raw(t, f.dsn)
	if err = metadata.QueryRow(ctx, `SELECT archived FROM chartworks.vector_heads WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3`, e.Tenant(), pack.Topic, secondSource.ContextID).Scan(&oldArchived); err != nil {
		t.Fatal(err)
	}
	if err = metadata.QueryRow(ctx, `SELECT archived FROM chartworks.vector_heads WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3`, e.Tenant(), pack.Topic, pack.Datasets[0].Source.Context).Scan(&keptArchived); err != nil {
		t.Fatal(err)
	}
	if !oldArchived || keptArchived {
		t.Fatal("context heads did not switch atomically", oldArchived, keptArchived)
	}
	restored, err := service.Rollback(ctx, e, pack.Topic, topics.TransitionRequest{Version: "v1", Expected: 2, Note: "Restore both contexts"})
	if err != nil || len(restored.State.Generations) != 2 || restored.State.Revision != 3 {
		t.Fatal("multi-context rollback", restored.State, err)
	}
	if err = metadata.QueryRow(ctx, `SELECT archived FROM chartworks.vector_heads WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3`, e.Tenant(), pack.Topic, secondSource.ContextID).Scan(&oldArchived); err != nil || oldArchived {
		t.Fatal("rollback did not restore removed context", oldArchived, err)
	}
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
