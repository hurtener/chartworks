package acceptance

import (
	"context"
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/vindex"
)

// Add a real published topic whose second source partition is not part of the
// engineering proposal. Target-ID reach alone must not reveal that dependency.
func phase26ImpactTopic(t *testing.T, f *phase26Fixture, p engineering.AutopilotProposal) string {
	t.Helper()
	ctx := context.Background()
	first := sources.Source{ID: p.Material.Binding.Source, ContextID: p.Material.Binding.Context, Revision: p.Material.Binding.Revision}
	second := f.create(t, "p26-other-context")
	pack := semantics.TopicPack{SchemaVersion: 1, Topic: "p26-cross-context-topic", Version: "v1", Name: "Reviewed impact fixture", Description: "Synthetic two-source dependency"}
	for i, source := range []sources.Source{first, second} {
		id := []string{"p26-impact-first", "p26-impact-second"}[i]
		profile := f.profile(t, f.profileSpec(t, source, id, []string{"id"}, "")).Profile.Profile
		dataset := semantics.Dataset{ID: profile.Dataset, Name: id, Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: profile.Dataset, SourceRevision: source.Revision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
		for _, column := range profile.Schema {
			if column.Name == "id" {
				dataset.Columns = append(dataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
			}
		}
		pack.Datasets = append(pack.Datasets, dataset)
	}
	draftService, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	provider := newGatewayFixture(t, func(cfg *config.Gateway) {
		role := cfg.Roles["embedding"]
		role.MaxBatchItems = 32
		role.MaxBatchBytes = 64 << 10
		cfg.Roles["embedding"] = role
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	topicService, err := topics.New(f.db, f.s, index, provider.engine)
	if err != nil {
		t.Fatal(err)
	}
	scopes := append(topicScopes(f.author.Tenant()), phase26Scopes()...)
	slices.Sort(scopes)
	scopes = slices.Compact(scopes)
	actor := f.token.envelope(t, f.author.Tenant(), f.author.User(), scopes...)
	draft, err := draftService.Save(ctx, actor, drafts.SaveRequest{Pack: pack, Change: "Reviewed impact fixture"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := topicService.Review(ctx, actor, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Reviewed fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = topicService.Publish(ctx, actor, pack.Topic, topics.PublishRequest{Review: review.ID}); err != nil {
		t.Fatal(err)
	}
	return pack.Topic
}

func phase26CheckImpactReach(t *testing.T, f *phase26Fixture, p engineering.AutopilotProposal, topic string) {
	t.Helper()
	scopes := append(phase26Scopes(), "topics.read", "reporting.read")
	all := f.token.envelope(t, f.author.Tenant(), f.author.User(), scopes...)
	impact, err := f.auto.DetectDrift(context.Background(), all, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	contains := func(v engineering.AutopilotDrift) bool {
		return slices.ContainsFunc(v.Impacts, func(i engineering.ProposalImpact) bool { return i.Kind == "topic" && i.ID == topic })
	}
	if !contains(impact) {
		t.Fatal("authorized dependency impact missing", impact)
	}
	for _, omit := range []string{"cw.execution_context.use:*", "topics.read"} {
		narrow := slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == omit })
		if omit == "cw.execution_context.use:*" {
			narrow = append(narrow, "cw.execution_context.use:"+p.Material.Binding.Context)
		}
		actor := f.token.envelope(t, f.author.Tenant(), f.author.User(), narrow...)
		projected, err := f.auto.DetectDrift(context.Background(), actor, p.ID)
		if err != nil || contains(projected) || projected.ID != impact.ID {
			t.Fatal("impact exposed missing dependency/action or lost deduplication", omit, projected, err)
		}
	}
}
