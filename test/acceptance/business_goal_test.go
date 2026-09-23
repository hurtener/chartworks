package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

// This is an additive real PG17 journey over registered source metadata,
// actor-private profile heads, draft CAS and a current published topic contract.
func TestBusinessGoalAuthoringPG17(t *testing.T) {
	f, draftService, topicService, model, pack := publicationFixture(t)
	goal, err := onboarding.NewGoalService(f.s, f.service, topicService, draftService)
	if err != nil {
		t.Fatal(err)
	}
	service, err := onboarding.NewWithGoal(f.db, newPhase33FailureAdapter(), onboarding.DefaultLimits(), goal)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(onboardingapi.Handler(f.token.verifier, service, http.NotFoundHandler()))
	t.Cleanup(server.Close)
	scopes := append(topicScopes(f.e.Tenant()), "onboarding.read", "onboarding.write", "cw.tenant.read:"+f.e.Tenant())
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), scopes...)
	claims := f.token.claims(e.Tenant(), e.User(), scopes)
	claims["session"] = e.Session()
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return f.token.sign(t, claims, nil), nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	beforeSearch := model.requests.Load()
	for _, test := range []struct{ locale, goal string }{{"en", "Review sales amount"}, {"es", "Revisar ventas y monto"}} {
		found, err := client.SearchBusinessGoal(ctx, onboarding.GoalSearchRequest{Goal: test.goal, Locale: test.locale})
		if err != nil || found.Policy != "authorized-lexical-v1" || len(found.Candidates) == 0 || len(found.Unresolved) == 0 {
			t.Fatal("goal search", test.locale, found, err)
		}
		profile := false
		for _, candidate := range found.Candidates {
			if candidate.Kind == "profile" && candidate.Profile == pack.Datasets[0].Source.ProfileVersion && candidate.Context == pack.Datasets[0].Source.Context {
				profile = true
			}
		}
		if !profile {
			t.Fatal("active private profile omitted", test.locale, found)
		}
	}
	if model.requests.Load() != beforeSearch {
		t.Fatal("goal search called gateway")
	}
	ref := pack.Datasets[0].Source
	choice := onboarding.GoalChoiceRequest{Kind: "new_draft", Topic: "goal-commerce", Version: "v1", Source: ref.Source, Context: ref.Context, Dataset: pack.Datasets[0].ID, Profile: ref.ProfileVersion, SourceRevision: ref.SourceRevision, Name: "Commerce draft", Description: "Unresolved synthetic business goal"}
	created, err := client.ChooseBusinessGoal(ctx, choice)
	if err != nil || created.Kind != "new_draft" || created.Draft == nil || created.Draft.Metadata.Revision != 1 || created.NextAction != "review_business_meaning" {
		t.Fatal("private draft", created, err)
	}
	if _, err = topicService.Read(ctx, e, choice.Topic, ""); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("private draft became published", err)
	}
	stale := choice
	stale.SourceRevision++
	stale.Topic = "stale-goal-topic"
	if _, err = service.ChooseGoal(ctx, e, stale); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale source accepted", err)
	}

	published := phase17PublishTopic(t, draftService, topicService, e, pack)
	beforePublishedSearch := model.requests.Load()
	found, err := client.SearchBusinessGoal(ctx, onboarding.GoalSearchRequest{Goal: "Commerce revenue", Locale: "en"})
	if err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, candidate := range found.Candidates {
		if candidate.Kind == "topic" && candidate.Topic == published.State.Topic && candidate.Digest == published.Digest {
			seen = true
		}
	}
	if !seen {
		t.Fatal("current reviewed topic missing", found)
	}
	if model.requests.Load() != beforePublishedSearch {
		t.Fatal("goal search called gateway after publication")
	}
	reuse := onboarding.GoalChoiceRequest{Kind: "reuse", Topic: published.State.Topic, Version: published.State.Version, Revision: published.State.Revision, Digest: published.Digest, Context: ref.Context}
	chosen, err := client.ChooseBusinessGoal(ctx, reuse)
	if err != nil || chosen.Kind != "reuse" || chosen.Draft != nil {
		t.Fatal("reuse", chosen, err)
	}
	reuse.Revision++
	if _, err = service.ChooseGoal(ctx, e, reuse); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale topic accepted", err)
	}
	narrow := f.token.envelope(t, e.Tenant(), e.User(), "onboarding.read", "sources.read", "topics.read", "engineering.read", "cw.tenant.read:"+e.Tenant(), "cw.source.read:*", "cw.dataset.query:*", "cw.topic.read:*", "cw.execution_context.use:other-context")
	if result, err := service.SearchGoal(ctx, narrow, onboarding.GoalSearchRequest{Goal: "revenue", Locale: "en"}); err == nil && len(result.Candidates) != 0 {
		t.Fatal("cross-context search leaked candidates", result)
	} else if err != nil && !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
		t.Fatal("unexpected cross-context result", err)
	}
	other := f.token.envelope(t, "other-tenant", e.User(), scopes...)
	if result, err := service.SearchGoal(ctx, other, onboarding.GoalSearchRequest{Goal: "revenue", Locale: "en"}); err == nil && len(result.Candidates) != 0 {
		t.Fatal("cross-tenant candidate leak", result)
	}
}
