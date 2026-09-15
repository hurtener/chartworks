package gateway

import (
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

func TestReservationMatchesExactCurrentIdentity(t *testing.T) {
	now := time.Now()
	envelope := func(tenant, user, session string) identity.Envelope {
		t.Helper()
		e, err := identity.FromVerified(tenant, user, session, []string{"reporting.execute", "cw.block.execute:block"}, now.Add(time.Minute), func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	original := envelope("tenant", "svc:reporting", "occurrence")
	call, err := Authorize(original, "reporting.execute", "partition", access.Resource{Tenant: "tenant", Kind: "block", Permission: "execute", ID: "block"})
	if err != nil {
		t.Fatal(err)
	}
	if !call.MatchesIdentity(original) || !call.MatchesIdentity(envelope("tenant", "svc:reporting", "occurrence")) {
		t.Fatal("exact current principal rejected")
	}
	for _, other := range []identity.Envelope{envelope("other", "svc:reporting", "occurrence"), envelope("tenant", "other", "occurrence"), envelope("tenant", "svc:reporting", "other"), {}} {
		if call.MatchesIdentity(other) {
			t.Fatal("reservation identity widened")
		}
	}
	if (Call{}).MatchesIdentity(original) {
		t.Fatal("zero call accepted")
	}
	now = now.Add(2 * time.Minute)
	if call.MatchesIdentity(original) {
		t.Fatal("expired principal accepted")
	}
}
