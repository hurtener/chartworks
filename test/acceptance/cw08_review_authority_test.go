package acceptance

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
)

func TestCW08FeedbackOnlyReviewAuthority(t *testing.T) {
	fixture := newPhase18Fixture(t)
	service, _ := newPhase18Service(t, fixture)
	ctx := context.Background()
	full := phase18Envelope(t, fixture, fixture.f.e.User(), "cw08-review-seed", true)
	binding, err := fixture.f.s.Binding(ctx, full, fixture.pack.Datasets[0].Source.Source, fixture.context)
	if err != nil {
		t.Fatal("seed binding", err)
	}
	scope, err := store.NewScope(full.Tenant(), full.User())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	example := nlqexec.ExampleRecord{
		ID: "cw08-review-authority", Topic: fixture.pack.Topic,
		Question: "What is revenue?", SQL: "SELECT id, amount FROM analytics.sales ORDER BY id",
		Digest: strings.Repeat("8", 64), State: "candidate", Weight: 2.0 / 3.0,
		Uncertainty: 1 / math.Sqrt(3), EvidenceCount: 1, PositiveEvidence: 1, EvidenceOutcome: "positive",
		Origin:  nlqexec.ExampleOrigin{SchemaVersion: 1, Locale: nlq.LanguageEnglish, TopicVersion: fixture.pack.Version, Context: fixture.context, SourceBindingDigest: readexec.Hash(binding), RuleVersions: []string{""}},
		Version: 1, Provenance: "cw08-review-authority", Created: now, Updated: now,
	}
	if _, err = fixture.f.db.UpsertExample(ctx, scope, example); err != nil {
		t.Fatal("seed review candidate", err)
	}

	reviewScopes := []string{
		"feedback.write",
		"cw.topic.read:" + fixture.pack.Topic,
		"cw.source.query:" + fixture.pack.Datasets[0].Source.Source,
		"cw.dataset.query:" + fixture.pack.Datasets[0].ID,
		"cw.execution_context.use:" + fixture.context,
	}
	reviewer := cw08ReviewEnvelope(t, fixture, reviewScopes)
	crossScopes := append([]string(nil), reviewScopes[:len(reviewScopes)-1]...)
	crossScopes = append(crossScopes, "cw.execution_context.use:other-context")
	if _, err = service.ExampleState(ctx, cw08ReviewEnvelope(t, fixture, crossScopes), nlqexec.ExampleStateRequest{ExampleID: example.ID, State: "active", ReviewNote: "reviewed positive evidence"}); !errors.Is(err, access.ErrNotFound) {
		t.Fatalf("review crossed signed context reach: %v", err)
	}
	if _, err = service.ExampleState(ctx, cw08ReviewEnvelope(t, fixture, reviewScopes[1:]), nlqexec.ExampleStateRequest{ExampleID: example.ID, State: "active", ReviewNote: "reviewed positive evidence"}); !errors.Is(err, access.ErrForbidden) {
		t.Fatalf("review accepted missing feedback action: %v", err)
	}
	active, err := service.ExampleState(ctx, reviewer, nlqexec.ExampleStateRequest{ExampleID: example.ID, State: "active", ReviewNote: "reviewed positive evidence"})
	if err != nil || active.State != "active" {
		t.Fatalf("feedback-only reviewer could not activate: %#v %v", active, err)
	}
}

func cw08ReviewEnvelope(t *testing.T, fixture *phase17Fixture, scopes []string) identity.Envelope {
	t.Helper()
	claims := fixture.model.token.claims(fixture.f.e.Tenant(), fixture.f.e.User(), scopes)
	claims["session"] = "cw08-reviewer"
	token := fixture.model.token.sign(t, claims, nil)
	e, err := fixture.model.token.verifier.Verify(context.Background(), token, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
