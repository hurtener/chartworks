package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func TestAuthoringPreparationStorageRejectsUnsealedShapesBeforeSQL(t *testing.T) {
	db := &DB{}
	if _, _, err := db.ReserveAuthoringPreparation(t.Context(), identity.Envelope{}, reporting.AuthoringPreparationRecord{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if _, err := db.ReadAuthoringPreparation(t.Context(), identity.Envelope{}, "preparation"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if err := db.FinishAuthoringPreparation(t.Context(), identity.Envelope{}, reporting.AuthoringPreparationRecord{}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	if err := authoringPreparationShape(reporting.AuthoringPreparationRecord{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := authoringPreparationFence(context.Background(), nil, identity.Envelope{}, reporting.AuthoringPreparationRecord{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := preparationAttempt(context.Background(), nil, identity.Envelope{}, reporting.AuthoringPreparationRecord{}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal(err)
	}
	if err := consumeAuthoringPreparation(context.Background(), nil, identity.Envelope{}, reporting.Mutation{}); err != nil {
		t.Fatal("ordinary create changed", err)
	}
}
