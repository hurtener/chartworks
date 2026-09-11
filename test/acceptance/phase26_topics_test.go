package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func testPhase26TopicApply(t *testing.T) {
	f := newPhase26Fixture(t)
	ctx := context.Background()
	source := sources.Source{ID: f.goal.Source, ContextID: f.goal.Context, Revision: 1}
	profile := f.profile(t, f.profileSpec(t, source, "p26-topic-profile", []string{"id"}, "")).Profile.Profile
	f.goal.Topic = &engineering.AutopilotTopicGoal{Topic: "p26-reviewed-topic", Profile: profile.Version, Version: "v1", Name: "Reviewed topic", Description: "Profile-backed reviewed topic"}
	service, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	p := f.propose(t)
	if p.Material.Topic == nil || len(p.Material.Objects) != 3 {
		t.Fatal("topic review material missing", p)
	}
	if _, err = service.Read(ctx, f.author, f.goal.Topic.Topic, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("proposal created a topic before apply", err)
	}
	pack := *p.Material.Topic
	pack.Name = "Edited review material"
	p, err = f.auto.Edit(ctx, f.author, p.ID, engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Topic: &pack, Reason: "Human semantic edit"})
	if err != nil || p.Review != nil || p.Material.Topic.Name != pack.Name {
		t.Fatal("topic edit failed", p, err)
	}
	applied := f.apply(t, f.approve(t, p))
	saved, err := service.Read(ctx, f.author, f.goal.Topic.Topic, 0)
	if err != nil || saved.Metadata.Revision != 1 || saved.Pack.Name != pack.Name {
		t.Fatal("reviewed topic did not use normal private save", saved, err)
	}
	if _, err = f.db.ReadPublishedTopic(ctx, f.author, f.goal.Topic.Topic, "", drafts.Read); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("engineering approval published business meaning", err)
	}
	replay := f.apply(t, applied)
	if replay.Version != applied.Version {
		t.Fatal("topic replay repeated effects", replay)
	}
	if _, err = f.auto.Compensate(ctx, f.author, applied.ID, phase26ApplyRequest(applied)); !errors.Is(err, engineering.ErrCompensationBlocked) {
		t.Fatal("multi-object apply claimed automatic reversal", err)
	}
	raw := support.Raw(t, f.dsn)
	if _, err = raw.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET applied_at=clock_timestamp()-interval '2 minutes' WHERE tenant_id=$1 AND proposal_id=$2`, f.author.Tenant(), applied.ID); err != nil {
		t.Fatal(err)
	}
	drift, err := f.auto.DetectDrift(ctx, f.author, applied.ID)
	if err != nil {
		t.Fatal(err)
	}
	amendment, err := f.client.AmendEngineeringProposal(ctx, applied.ID, sdk.EngineeringProposalAmend{Drift: drift.ID})
	if err != nil || amendment.Material.Topic == nil || amendment.Material.Topic.Name != pack.Name || amendment.Material.Request.Topic.ExpectedRevision != 1 || amendment.Review != nil {
		t.Fatal("topic amendment lost reviewed material or actual head", amendment, err)
	}
	f.apply(t, f.approve(t, amendment))
	updated, err := service.Read(ctx, f.author, f.goal.Topic.Topic, 0)
	if err != nil || updated.Metadata.Revision != 2 || updated.Pack.Name != pack.Name {
		t.Fatal("reviewed topic amendment failed ordinary CAS update", updated, err)
	}

}
