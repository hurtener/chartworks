package topics

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

func TestPublicHealthBoundRejectsNilContextAndClassifiesAuthority(t *testing.T) {
	service := &Service{}
	//nolint:staticcheck // This boundary regression requires a literal nil context.
	if _, err := service.Contract(nil, identity.Envelope{}, "topic"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("nil contract context returned %v", err)
	}
	//nolint:staticcheck // This boundary regression requires a literal nil context.
	if _, err := service.Health(nil, identity.Envelope{}, "topic"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("nil health context returned %v", err)
	}
	//nolint:staticcheck // This boundary regression requires a literal nil context.
	if _, err := service.Recheck(nil, identity.Envelope{}, "topic"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("nil recheck context returned %v", err)
	}
	//nolint:staticcheck // This boundary regression requires a literal nil context.
	if _, err := service.Archive(nil, identity.Envelope{}, "topic", 1, "reviewed archive"); !errors.Is(err, store.ErrInvalid) {
		t.Fatalf("nil archive context returned %v", err)
	}
	if !healthAuthorization(access.ErrUnauthenticated) || !healthAuthorization(access.ErrForbidden) || healthAuthorization(store.ErrUnavailable) {
		t.Fatal("health source errors were misclassified")
	}
}
