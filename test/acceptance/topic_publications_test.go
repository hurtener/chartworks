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
	"github.com/hurtener/chartworks/internal/gateway"
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
	handler := assertRegisteredWireSchemas(t, registry, topicapi.Handler(f.token.verifier, draftsService, service, nil, http.NotFoundHandler()))
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
	validDigest := strings.Repeat("0", 64)
	if _, err := f.db.ReviewTopic(ctx, e, pack.Topic, topics.ReviewRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid store review accepted", err)
	}
	unauthorized := f.token.envelope(t, f.e.Tenant(), f.e.User(), "topics.read", "cw.topic.read:"+pack.Topic)
	if _, err := f.db.ReviewTopic(ctx, unauthorized, pack.Topic, topics.ReviewRequest{DraftRevision: 1, Digest: validDigest, Decision: "approve", Note: "Valid note"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("store review action was not enforced", err)
	}
	if _, _, err := f.db.ReviewedTopic(ctx, e, pack.Topic, "bad/review"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid review identifier accepted", err)
	}
	if _, _, err := f.db.ReviewedTopic(ctx, identity.Envelope{}, pack.Topic, "valid-review"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("invalid reviewed-topic envelope accepted", err)
	}
	if _, err := f.db.ReadPublishedTopic(ctx, e, pack.Topic, "bad/version", drafts.Read); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid publication version accepted", err)
	}
	if _, err := f.db.ReadPublishedTopic(ctx, identity.Envelope{}, pack.Topic, "", drafts.Read); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("invalid publication envelope accepted", err)
	}
	if _, err := f.db.ReadPublishedTopic(ctx, e, pack.Topic, "", drafts.Export); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid publication access accepted", err)
	}
	if _, err := f.db.RollbackTopic(ctx, e, pack.Topic, topics.TransitionRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid store rollback accepted", err)
	}
	if _, err := f.db.RollbackTopic(ctx, identity.Envelope{}, pack.Topic, topics.TransitionRequest{Version: "valid-version", Expected: 1, Note: "Valid note"}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("invalid rollback envelope accepted", err)
	}
	if _, err := f.db.ArchiveTopic(ctx, e, pack.Topic, 0, "Valid note"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid store archive accepted", err)
	}
	if _, err := f.db.ArchiveTopic(ctx, identity.Envelope{}, pack.Topic, 1, "Valid note"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("invalid archive envelope accepted", err)
	}
	if _, err := f.db.ConfirmTopicContract(ctx, identity.Envelope{}, pack.Topic, 1); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("invalid contract envelope accepted", err)
	}
	if _, err := f.db.ConfirmTopicContract(ctx, e, "unknown-topic", 1); err == nil {
		t.Fatal("missing publication contract accepted")
	}
	if _, err := f.db.ReviewTopic(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: 1, Digest: validDigest, Decision: "approve", Note: "No draft"}); err == nil {
		t.Fatal("review without draft accepted")
	}

	draft, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Publication candidate"})
	if err != nil {
		t.Fatal("save", err)
	}
	if _, err = f.db.ReviewTopic(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: validDigest, Decision: "approve", Note: "Wrong digest"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("mismatched review digest accepted", err)
	}
	if _, _, err = f.db.ReviewedTopic(ctx, e, pack.Topic, "unknown-review"); err == nil {
		t.Fatal("missing review returned")
	}
	if _, err = f.db.ReadPublishedTopic(ctx, e, pack.Topic, "unknown-version", drafts.Read); err == nil {
		t.Fatal("missing publication returned")
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
	if _, _, err = f.db.ReviewedTopic(ctx, unauthorized, pack.Topic, review.ID); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("reviewed topic omitted publish action", err)
	}
	index, _ := vindex.New(f.db)
	space := vindex.Space(gatewayFixture.engine.EmbeddingSpace())
	query := vindex.Query{ID: "published-search", Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Space: space, Vector: []float32{1, 2}, Kinds: []string{"topic", "entity", "measure"}, LimitPerKind: 10}
	if _, err = index.Search(ctx, e, []vindex.Query{query}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("reviewed topic without publication became searchable", err)
	}
	if _, err = f.db.PublishTopic(ctx, e, topics.Prepared{}, nil, gateway.Receipt{}, -1); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid publication revision accepted", err)
	}
	if _, err = f.db.PublishTopic(ctx, e, topics.Prepared{}, nil, gateway.Receipt{}, 0); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("unprepared publication accepted", err)
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
	withoutSourceAction := []string{"topics.read", "topics.publish", "cw.topic.read:" + pack.Topic, "cw.topic.publish:" + pack.Topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"}
	noSourceEnvelope := f.token.envelope(t, f.e.Tenant(), f.e.User(), withoutSourceAction...)
	withoutTopicActions := f.token.envelope(t, f.e.Tenant(), f.e.User(), "sources.read", "cw.topic.read:"+pack.Topic, "cw.topic.publish:"+pack.Topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*")
	if _, err = service.Contract(ctx, noSourceEnvelope, pack.Topic); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("contract omitted secondary sources.read action", err)
	}
	if _, err = f.db.ReadPublishedTopic(ctx, withoutTopicActions, pack.Topic, "", drafts.Read); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("publication read omitted topic action", err)
	}
	if _, err = f.db.ConfirmTopicContract(ctx, e, pack.Topic, 99); !errors.Is(err, store.ErrConflict) {
		t.Fatal("contract revision conflict was not enforced", err)
	}
	if _, err = f.db.RollbackTopic(ctx, e, pack.Topic, topics.TransitionRequest{Version: pack.Version, Expected: 1, Note: "Already active"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("active target rollback accepted", err)
	}
	if _, err = f.db.RollbackTopic(ctx, e, pack.Topic, topics.TransitionRequest{Version: "unknown-version", Expected: 1, Note: "Missing target"}); err == nil {
		t.Fatal("missing rollback target accepted")
	}
	if _, err = f.db.RollbackTopic(ctx, e, "unknown-topic", topics.TransitionRequest{Version: pack.Version, Expected: 1, Note: "Missing topic"}); err == nil {
		t.Fatal("missing rollback topic accepted")
	}
	if _, err = f.db.RollbackTopic(ctx, withoutTopicActions, pack.Topic, topics.TransitionRequest{Version: pack.Version, Expected: 1, Note: "Missing topic action"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("rollback omitted topic action", err)
	}
	if _, err = f.db.ArchiveTopic(ctx, e, pack.Topic, 99, "Wrong revision"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("archive revision conflict was not enforced", err)
	}
	if _, err = f.db.ArchiveTopic(ctx, withoutTopicActions, pack.Topic, 1, "Missing topic action"); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("archive omitted topic action", err)
	}
	if _, err = f.db.ConfirmTopicContract(ctx, withoutTopicActions, pack.Topic, 1); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("contract store omitted topic action", err)
	}
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

	archiveClient := publicationClient(t, f, draftsService, service, []string{"topics.publish", "cw.topic.publish:" + pack.Topic, "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"})
	archived, err := archiveClient.ArchiveTopic(ctx, pack.Topic, sdk.ArchiveTopicRequest{Expected: 1, Note: "Archive synthetic topic"})
	if err != nil || archived.Revision != 2 || !archived.Archived || archived.Active {
		t.Fatal("archive", archived, err)
	}
	if _, err = service.Archive(ctx, e, pack.Topic, 2, "Repeated archive"); !errors.Is(err, store.ErrConflict) {
		t.Fatal("no-op archive created evidence", err)
	}
	if _, err = f.db.ConfirmTopicContract(ctx, e, pack.Topic, 2); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("archived publication reported healthy", err)
	}
	if _, err = index.Search(ctx, e, []vindex.Query{query}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("archived managed facets searchable", err)
	}
	beforeRollback := gatewayFixture.requests.Load()
	if _, err = service.Rollback(ctx, noSourceEnvelope, pack.Topic, topics.TransitionRequest{Version: pack.Version, Expected: 2, Note: "Missing source action"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("rollback omitted secondary sources.read action", err)
	}
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
	if _, err = f.db.ConfirmTopicContract(ctx, e, pack.Topic, 3); err == nil {
		t.Fatal("store contract ignored changed source revision", err)
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

func TestCanonicalRegistryReviewedPublicationAndCollisionFences(t *testing.T) {
	f, draftsService, service, gatewayFixture, pack := publicationFixture(t)
	ctx := context.Background()
	fullScopes := topicScopes(f.e.Tenant())
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), fullScopes...)
	client := publicationClient(t, f, draftsService, service, fullScopes)
	pack.CanonicalEntities = []semantics.CanonicalEntity{{
		ID: "customer", Revision: 1, Name: "Customer", Aliases: []string{"Buyer"},
		Keys: []semantics.Reference{{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}},
	}}

	draft, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Propose customer meaning"})
	if err != nil {
		t.Fatal("canonical draft proposal", err)
	}
	review, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Approve customer meaning"})
	if err != nil {
		t.Fatal("canonical review", err)
	}
	withoutTenantScopes := make([]string, 0, len(fullScopes))
	for _, scope := range fullScopes {
		if scope != "cw.tenant.write:"+f.e.Tenant() {
			withoutTenantScopes = append(withoutTenantScopes, scope)
		}
	}
	withoutTenant := f.token.envelope(t, f.e.Tenant(), f.e.User(), withoutTenantScopes...)
	before := gatewayFixture.requests.Load()
	if _, err = service.Publish(ctx, withoutTenant, pack.Topic, topics.PublishRequest{Review: review.ID}); !errors.Is(err, access.ErrNotFound) || gatewayFixture.requests.Load() != before {
		t.Fatal("registry-changing publication lacked early tenant-write fence", err)
	}
	published, err := client.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil || len(published.Definition.CanonicalEntities) != 1 || published.Definition.CanonicalEntities[0].Revision != 1 {
		t.Fatal("reviewed canonical publication", published.Definition.CanonicalEntities, err)
	}
	metadata := support.Raw(t, f.dsn)
	var revisions, terms int
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.canonical_entity_revisions WHERE tenant_id=$1 AND entity_id='customer'`, e.Tenant()).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.canonical_entity_terms WHERE tenant_id=$1 AND entity_id='customer'`, e.Tenant()).Scan(&terms); err != nil {
		t.Fatal(err)
	}
	if revisions != 1 || terms != 2 {
		t.Fatal("canonical meaning or normalized terms not retained")
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.canonical_entity_revisions SET meaning='{}' WHERE tenant_id=$1 AND entity_id='customer' AND revision=1`, e.Tenant()); err == nil {
		t.Fatal("canonical meaning revision mutated")
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.canonical_entity_heads SET current_revision=3 WHERE tenant_id=$1 AND entity_id='customer'`, e.Tenant()); err == nil {
		t.Fatal("canonical head skipped a revision")
	}

	for _, tc := range []struct {
		name    string
		meaning semantics.CanonicalMeaning
	}{
		{"same revision different meaning", semantics.CanonicalMeaning{ID: "customer", Revision: 1, Name: "Client"}},
		{"revision gap", semantics.CanonicalMeaning{ID: "customer", Revision: 3, Name: "Customer"}},
		{"reserved normalized term", semantics.CanonicalMeaning{ID: "account", Revision: 1, Name: "ＢＵＹＥＲ"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, checkErr := f.db.CheckCanonicalMeanings(ctx, e, pack.Topic, drafts.Publish, []semantics.CanonicalMeaning{tc.meaning}); !errors.Is(checkErr, store.ErrConflict) {
				t.Fatal("canonical collision accepted", checkErr)
			}
		})
	}
	if changes, checkErr := f.db.CheckCanonicalMeanings(ctx, e, pack.Topic, drafts.Publish, []semantics.CanonicalMeaning{pack.CanonicalEntities[0].Meaning()}); checkErr != nil || changes {
		t.Fatal("exact revision reuse treated as mutation", changes, checkErr)
	}
	otherTenant := f.token.envelope(t, "tenant-canonical-other", f.e.User(), "topics.publish", "cw.topic.publish:"+pack.Topic)
	if changes, checkErr := f.db.CheckCanonicalMeanings(ctx, otherTenant, pack.Topic, drafts.Publish, []semantics.CanonicalMeaning{pack.CanonicalEntities[0].Meaning()}); checkErr != nil || !changes {
		t.Fatal("tenant registry boundary changed", changes, checkErr)
	}

	// Remapping a physical key changes the topic digest but reuses exact global
	// meaning without tenant-write authority.
	pack.Version = "v2"
	pack.CanonicalEntities[0].Keys[0].ID = "amount"
	second, err := draftsService.Save(ctx, withoutTenant, drafts.SaveRequest{Expected: 1, Pack: pack, Change: "Remap local customer key"})
	if err != nil || second.Metadata.Digest == draft.Metadata.Digest {
		t.Fatal("topic-local key remap", second.Metadata.Digest, err)
	}
	review2, err := service.Review(ctx, withoutTenant, pack.Topic, topics.ReviewRequest{DraftRevision: 2, Digest: second.Metadata.Digest, Decision: "approve", Note: "Approve local remap"})
	if err != nil {
		t.Fatal("review exact reuse", err)
	}
	if _, err = service.Publish(ctx, withoutTenant, pack.Topic, topics.PublishRequest{Review: review2.ID, Expected: 1}); err != nil {
		t.Fatal("exact canonical revision reuse", err)
	}

	pack.Version = "v3"
	pack.CanonicalEntities[0].Revision = 2
	pack.CanonicalEntities[0].Name = "Customer account"
	pack.CanonicalEntities[0].Aliases = []string{"Buyer", "Customer"}
	third, err := draftsService.Save(ctx, withoutTenant, drafts.SaveRequest{Expected: 2, Pack: pack, Change: "Propose customer revision two"})
	if err != nil {
		t.Fatal("next revision proposal", err)
	}
	review3, err := service.Review(ctx, withoutTenant, pack.Topic, topics.ReviewRequest{DraftRevision: 3, Digest: third.Metadata.Digest, Decision: "approve", Note: "Approve customer revision two"})
	if err != nil {
		t.Fatal("review next revision", err)
	}
	if _, err = metadata.Exec(ctx, `CREATE FUNCTION chartworks.reject_canonical_test() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected canonical publication failure'; END; $$; CREATE TRIGGER reject_canonical_test BEFORE INSERT ON chartworks.topic_published_dependencies FOR EACH ROW EXECUTE FUNCTION chartworks.reject_canonical_test()`); err != nil {
		t.Fatal("install atomicity fixture", err)
	}
	if _, err = service.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: review3.ID, Expected: 2}); err == nil {
		t.Fatal("injected failure published canonical revision")
	}
	var registryHead, topicHead int64
	if err = metadata.QueryRow(ctx, `SELECT current_revision FROM chartworks.canonical_entity_heads WHERE tenant_id=$1 AND entity_id='customer'`, e.Tenant()).Scan(&registryHead); err != nil {
		t.Fatal(err)
	}
	if err = metadata.QueryRow(ctx, `SELECT revision FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic).Scan(&topicHead); err != nil {
		t.Fatal(err)
	}
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.canonical_entity_revisions WHERE tenant_id=$1 AND entity_id='customer' AND revision=2`, e.Tenant()).Scan(&revisions); err != nil {
		t.Fatal(err)
	}
	if registryHead != 1 || topicHead != 2 || revisions != 0 {
		t.Fatal("failed publication left canonical or topic state", registryHead, topicHead)
	}
	if _, err = metadata.Exec(ctx, `DROP TRIGGER reject_canonical_test ON chartworks.topic_published_dependencies; DROP FUNCTION chartworks.reject_canonical_test()`); err != nil {
		t.Fatal("remove atomicity fixture", err)
	}
	thirdPublished, err := service.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: review3.ID, Expected: 2})
	if err != nil || thirdPublished.Definition.CanonicalEntities[0].Revision != 2 {
		t.Fatal("retry canonical publication", thirdPublished.Definition.CanonicalEntities, err)
	}
	rolledBack, err := service.Rollback(ctx, withoutTenant, pack.Topic, topics.TransitionRequest{Version: "v1", Expected: 3, Note: "Restore exact canonical revision one"})
	if err != nil || rolledBack.Definition.CanonicalEntities[0].Revision != 1 {
		t.Fatal("exact canonical rollback", rolledBack.Definition.CanonicalEntities, err)
	}
}

func TestCanonicalPublicationFenceUsesExactEntityIDs(t *testing.T) {
	f, draftsService, service, gatewayFixture, pack := publicationFixture(t)
	ctx := context.Background()
	scopes := topicScopes(f.e.Tenant())
	client := publicationClient(t, f, draftsService, service, scopes)
	column := func(id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: id}
	}
	// PostgreSQL's en_US.utf8 ordering differs from Go byte ordering for this
	// mixed-case/punctuation set. Retained fences must therefore match by ID.
	pack.CanonicalEntities = []semantics.CanonicalEntity{
		{ID: "alpha", Revision: 1, Name: "Alpha meaning", Keys: []semantics.Reference{column("id")}},
		{ID: "Beta", Revision: 1, Name: "Beta meaning", Keys: []semantics.Reference{column("amount")}},
		{ID: "_lead", Revision: 1, Name: "Lead meaning", Keys: []semantics.Reference{column("id")}},
		{ID: "entity-2", Revision: 1, Name: "Entity two meaning", Keys: []semantics.Reference{column("amount")}},
	}
	draft, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: pack, Change: "Publish mixed canonical IDs"})
	if err != nil {
		t.Fatal("save mixed canonical draft", err)
	}
	review, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Approve mixed canonical IDs"})
	if err != nil {
		t.Fatal("review mixed canonical draft", err)
	}
	first, err := client.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil || first.State.Version != "v1" || len(first.Definition.CanonicalEntities) != 4 {
		t.Fatal("publish mixed canonical IDs", first.State, err)
	}
	retained, err := client.PublishedTopic(ctx, pack.Topic)
	if err != nil || retained.State.Version != "v1" || len(retained.Definition.CanonicalEntities) != 4 {
		t.Fatal("retained mixed canonical read", retained.State, err)
	}

	pack.Version = "v2"
	pack.Name = "Commerce v2"
	second, err := client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: 1, Pack: pack, Change: "Publish second mixed canonical version"})
	if err != nil {
		t.Fatal("save second mixed canonical draft", err)
	}
	review2, err := client.ReviewTopic(ctx, pack.Topic, sdk.TopicReviewRequest{DraftRevision: second.Metadata.Revision, Digest: second.Metadata.Digest, Decision: "approve", Note: "Approve second mixed canonical version"})
	if err != nil {
		t.Fatal("review second mixed canonical draft", err)
	}
	current, err := client.PublishTopic(ctx, pack.Topic, sdk.PublishTopicRequest{Review: review2.ID, Expected: 1})
	if err != nil || current.State.Version != "v2" || current.State.Revision != 2 {
		t.Fatal("publish second mixed canonical version", current.State, err)
	}
	beforeGateway := gatewayFixture.requests.Load()
	rolledBack, err := client.RollbackTopic(ctx, pack.Topic, sdk.TopicTransitionRequest{Version: "v1", Expected: 2, Note: "Restore mixed canonical version"})
	if err != nil || rolledBack.State.Version != "v1" || rolledBack.State.Revision != 3 || gatewayFixture.requests.Load() != beforeGateway {
		t.Fatal("rollback mixed canonical version", rolledBack.State, err)
	}
	exact, err := client.PublishedTopicVersion(ctx, pack.Topic, "v1")
	if err != nil || exact.State.Version != "v1" || len(exact.Definition.CanonicalEntities) != 4 {
		t.Fatal("exact mixed canonical read", exact.State, err)
	}
}

func TestCanonicalTermCompatibilityExpansionStopsBeforeGateway(t *testing.T) {
	f, draftsService, _, gatewayFixture, pack := publicationFixture(t)
	ctx := context.Background()
	// U+FDFA expands from three source bytes to 33 normalized bytes. The raw
	// name stays below the authoring field bound while the canonical term exceeds
	// the downstream 1024-byte limit.
	pack.CanonicalEntities = []semantics.CanonicalEntity{{
		ID: "expanded", Revision: 1, Name: strings.Repeat("\ufdfa", 64),
		Keys: []semantics.Reference{{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}},
	}}
	beforeGateway := gatewayFixture.requests.Load()
	beforeLookups := f.lookups.Load()
	_, err := draftsService.Save(ctx, f.e, drafts.SaveRequest{Pack: pack, Change: "Reject expanded canonical term"})
	if !errors.Is(err, semantics.ErrInvalid) {
		t.Fatalf("compatibility-expanded term accepted: %v", err)
	}
	if gatewayFixture.requests.Load() != beforeGateway || f.lookups.Load() != beforeLookups {
		t.Fatalf("invalid canonical term reached source/gateway: gateway=%d/%d lookups=%d/%d", gatewayFixture.requests.Load(), beforeGateway, f.lookups.Load(), beforeLookups)
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
