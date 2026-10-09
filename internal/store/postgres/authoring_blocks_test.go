package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestAuthoringCopyCustodyGuardsBeforeDatabaseAccess(t *testing.T) {
	db := &DB{}
	actor := func(scopes ...string) identity.Envelope {
		now := time.Now()
		e, err := identity.FromVerified("tenant", "actor", "session", scopes, now.Add(time.Hour), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	for _, scopes := range [][]string{{"reporting.read", "cw.block.read:block"}, {"reporting.preview", "cw.block.preview:block"}, {"reporting.read", "reporting.preview", "cw.block.read:other", "cw.block.preview:block"}} {
		if _, err := db.ReadBlockForAuthoringCopy(t.Context(), actor(scopes...), "block", reporting.Reference{Revision: 1}); err == nil {
			t.Fatal("copy custody admitted missing authority")
		}
	}
	if _, err := db.ReadBlockForAuthoringCopy(t.Context(), identity.Envelope{}, "block", reporting.Reference{Revision: 1}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
	for _, ref := range []reporting.Reference{{}, {Revision: 257}, {Revision: 1, Draft: true}} {
		if _, err := db.ReadBlockForAuthoringCopy(t.Context(), identity.Envelope{}, "block", ref); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal(err)
		}
	}
}
