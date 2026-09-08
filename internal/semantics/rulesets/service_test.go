package rulesets

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
)

func TestServiceRejectsInvalidBoundariesBeforeDependencies(t *testing.T) {
	if _, err := New(nil, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil dependencies", err)
	}
	s := &Service{}
	ctx := context.Background()
	e := identity.Envelope{}
	definition := semantics.RuleSetDefinition{Topic: "commerce"}
	if _, err := s.Save(nil, e, SaveRequest{Definition: definition, Change: "change"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil save context", err)
	}
	if _, err := s.Review(ctx, e, "bad/topic", ReviewRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid review", err)
	}
	if _, err := s.Publish(ctx, e, "bad/topic", PublishRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid publication", err)
	}
	if _, err := s.Read(ctx, e, "bad/topic", ""); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid read", err)
	}
	if _, err := s.Retire(ctx, e, "bad/topic", RetireRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid retirement", err)
	}
	if _, err := s.Evaluate(ctx, e, "bad/topic", EvaluateRequest{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("invalid evaluation", err)
	}
}
