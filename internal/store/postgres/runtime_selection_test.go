package postgres

import (
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

func TestRuntimeSelectionRejectsInvalidIdentityAndActions(t *testing.T) {
	// Constructor use is confined to this unit test; production identity still
	// enters exclusively through the verifier. No database is touched here.
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"reporting.read", "engineering.autopilot.read", "cw.source.read:pipeline", "cw.run.read:run"}, time.Now().Add(time.Minute), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		check func() error
		want  error
	}{
		{"invalid run identifier", func() error { _, err := frozenArgs(e, "../run", false); return err }, store.ErrInvalid},
		{"reader cannot execute", func() error { _, err := frozenArgs(e, "run", true); return err }, access.ErrForbidden},
		{"invalid proposal identifier", func() error { _, err := proposalReadArgs(e, "../proposal", "engineering.autopilot.read"); return err }, store.ErrInvalid},
		{"unknown proposal action", func() error { _, err := proposalReadArgs(e, "proposal", "engineering.autopilot.publish"); return err }, access.ErrForbidden},
		{"reader cannot review", func() error { _, err := proposalReadArgs(e, "proposal", "engineering.autopilot.review"); return err }, access.ErrForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.check(); !errors.Is(err, tc.want) {
				t.Fatalf("selection %v; want %v", err, tc.want)
			}
		})
	}
}
