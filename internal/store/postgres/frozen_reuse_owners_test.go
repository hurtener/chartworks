package postgres

import (
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestFrozenReuseSettlementRequiresDefiniteTerminalProof(t *testing.T) {
	finished := time.Now()
	for _, status := range []string{"accepted", "dispatching", "running", "uncertain", "succeeded", "empty", "truncated", "failed", "cancelled", "timed_out", "interrupted"} {
		for _, remote := range []string{"", "not_issued", "running", "unknown", "stopped"} {
			for _, terminal := range []bool{false, true} {
				a := readexec.Attempt{Status: status, RemoteState: remote}
				if terminal {
					a.Finished = &finished
				}
				want := "unresolved"
				if terminal {
					switch status {
					case "succeeded", "empty", "truncated":
						if remote == "stopped" {
							want = "successful"
						}
					case "failed", "cancelled", "timed_out", "interrupted":
						if remote == "stopped" || remote == "not_issued" {
							want = "retryable"
						}
					}
				}
				if got := frozenReuseAttemptSettlement(a); got != want {
					t.Fatalf("status=%s remote=%s terminal=%v: got %s want %s", status, remote, terminal, got, want)
				}
			}
		}
	}
}

func TestFrozenReuseEligibilityAndPrivatePartition(t *testing.T) {
	m := reporting.RunManifest{Tenant: "tenant", Actor: "actor", Session: "session-one", ReuseMaxAge: 60}
	m.ReuseKey = reporting.ReuseIdentity(m)
	if !frozenReuseEligible(m) || frozenReusePartition(m) != "" {
		t.Fatal("public canonical identity not eligible")
	}
	m.Private = true
	m.ReuseKey = reporting.ReuseIdentity(m)
	if !frozenReuseEligible(m) || frozenReusePartition(m) != "session-one" {
		t.Fatal("private ownership lacks session partition")
	}
	previous := m.ReuseKey
	m.Session = "session-two"
	if reporting.ReuseIdentity(m) != previous || frozenReusePartition(m) != "session-two" {
		t.Fatal("private sessions must use distinct custody despite equal completed-reuse keys")
	}
	m.ReuseKey = "caller-key"
	if frozenReuseEligible(m) {
		t.Fatal("caller key selected cold custody")
	}
	m.ReuseMaxAge = 0
	m.ReuseKey = reporting.ReuseIdentity(m)
	if frozenReuseEligible(m) {
		t.Fatal("disabled reuse elected shared owner")
	}
	m.ReuseMaxAge = 60
	m.Outputs = []reporting.Output{{Kind: "narrative"}}
	m.ReuseKey = reporting.ReuseIdentity(m)
	if frozenReuseEligible(m) {
		t.Fatal("legacy unpinned narrative elected shared owner")
	}
}
