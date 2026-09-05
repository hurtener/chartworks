package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
)

func TestBrokerRejectsUntrustedResponses(t *testing.T) {
	q := newQueueFixture(t, nil)
	ctx := context.Background()
	j := q.submit(t, "broker-adversary")
	credential := broker.Credential{ClientID: "fixture", Secret: "SYNTHETIC_NEVER_PRINT"}
	if strings.Contains(fmt.Sprintf("%v %#v", credential, credential), credential.Secret) {
		t.Fatal("credential formatting leak")
	}
	b, err := json.Marshal(credential)
	if err != nil || strings.Contains(string(b), credential.Secret) {
		t.Fatal("credential JSON leak")
	}
	for _, endpoint := range []string{"http://host/exchange/execution-authority", "https://user:secret@host/exchange/execution-authority", "https://host/other", "https://host/exchange/execution-authority?tenant=foreign"} {
		if _, err := broker.New(endpoint, map[string]broker.Credential{j.Tenant: credential}, q.token.verifier, nil); err == nil {
			t.Fatal("unsafe broker URL")
		}
	}
	if _, err := broker.New("https://host/exchange/execution-authority", map[string]broker.Credential{"bad/tenant": credential}, q.token.verifier, nil); err == nil {
		t.Fatal("invalid partition")
	}
	if _, err := q.provider.Acquire(ctx, jobs.Job{}); err == nil {
		t.Fatal("invalid accepted manifest")
	}
	unknown := j
	unknown.Tenant = "unknown"
	unknown.ManifestHash = unknown.Digest()
	if _, err := q.provider.Acquire(ctx, unknown); err == nil {
		t.Fatal("fallback to another tenant credential")
	}
	for _, body := range []string{`{}`, `null`, `{"version":1,"version":1}`, strings.Repeat("x", 65537), `{"version":1,"access_token":"x","token_type":"Bearer","expires_in":30,"binding_id":"maintenance","WRONG_REVISION":1}`, `{"version":1,"access_token":"x","token_type":"Bearer","expires_in":999,"binding_id":"maintenance","binding_revision":1}`} {
		server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		p, err := broker.New(server.URL+"/exchange/execution-authority", map[string]broker.Credential{j.Tenant: credential}, q.token.verifier, server.Client())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := p.Acquire(ctx, j); err == nil {
			t.Fatal("invalid broker response accepted")
		}
		p.Close()
		server.Close()
	}
	var targetCalls int
	target := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetCalls++; w.WriteHeader(500) }))
	defer target.Close()
	redirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", target.URL)
		w.WriteHeader(307)
	}))
	defer redirect.Close()
	p, err := broker.New(redirect.URL+"/exchange/execution-authority", map[string]broker.Credential{j.Tenant: credential}, q.token.verifier, redirect.Client())
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	if _, err := p.Acquire(ctx, j); err == nil || targetCalls != 0 {
		t.Fatal("broker credential followed redirect")
	}
}
