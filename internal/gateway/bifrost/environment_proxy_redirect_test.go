package bifrost

import (
	"context"
	"encoding/pem"
	"errors"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// EnvProxy changes only SDK dialing. The same pinned SDK HTTP codec must never
// follow a provider redirect to a second destination with its bearer credential.
func TestEnvironmentProxyCodecDoesNotFollowRedirect(t *testing.T) {
	var escaped, first atomic.Int64
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first.Add(1)
		w.Header().Set("Location", target.URL+"/private")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	cfg := environmentProxyTestConfig(origin.URL)
	cfg.MaxAttemptsPerCall = 1
	r := cfg.Roles["rerank"]
	r.Enabled = true
	cfg.Roles["rerank"] = r
	ca := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: origin.Certificate().Raw}))
	engine, err := New(context.Background(), cfg, func(string) (string, bool) { return "SYNTHETIC_REDIRECT_KEY", true }, TransportOptions{AllowPrivateNetwork: true, CACertPEM: ca})
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	e, err := identity.FromVerified("test", "user", "session", []string{"gateway.use", "cw.tenant.use:test", "cw.topic.read:first"}, time.Now().Add(time.Minute), nil)
	if err != nil {
		t.Fatal(err)
	}
	call, err := gateway.Authorize(e, "gateway.use", "context", access.Tenant(e, "use"))
	if err != nil {
		t.Fatal(err)
	}
	candidates, err := gateway.AdmitCandidates(call, "gateway.use", []gateway.Candidate{{ID: "first", Text: "Synthetic", Resource: access.Resource{Tenant: "test", Kind: "topic", Permission: "read", ID: "first"}}})
	if err != nil {
		t.Fatal(err)
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 1, Tokens: 1024, Duration: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Rerank(context.Background(), call, budget, "Synthetic", candidates)
	if !errors.Is(err, gateway.ErrUnavailable) || first.Load() != 1 || escaped.Load() != 0 {
		t.Fatal("redirect escaped fixed destination", err, first.Load(), escaped.Load())
	}
}
