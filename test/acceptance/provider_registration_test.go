package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/securityapi"
)

func TestProviderRegistrationManifest(t *testing.T) {
	data, err := os.ReadFile("../../docs/contracts/chartworks-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var declared []securityapi.Operation
	if json.Unmarshal(data, &declared) != nil || !reflect.DeepEqual(declared, securityapi.Operations()) {
		t.Fatal("operator manifest drift from real registered operations")
	}
	example, err := os.ReadFile("../../examples/pengui-chartworks-scopes.json")
	if err != nil {
		t.Fatal(err)
	}
	var request struct {
		Scopes []string `json:"scopes"`
	}
	if json.Unmarshal(example, &request) != nil {
		t.Fatal("operator example invalid")
	}
	f := newTokenFixture(t)
	token := f.sign(t, f.claims("tenant_demo", "demo-user", request.Scopes), nil)
	e, err := f.verifier.Verify(context.Background(), token, auth.HTTP)
	if err != nil {
		t.Fatal("Pengui-shaped scope example incompatible", err)
	}
	if access.Require(e, "ops.read", access.Tenant(e, "read")) != nil {
		t.Fatal("read-only provider example denied")
	}
	if access.Require(e, "ops.write", access.Tenant(e, "write")) == nil {
		t.Fatal("read example implies write")
	}
}
func TestSelectionExpiresWithVerifiedEnvelope(t *testing.T) {
	f := newTokenFixture(t)
	e := f.envelope(t, "tenant", "user", "query.execute", "cw.source.query:s1")
	s, err := access.Constrain(e, "query.execute", "source", "query")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Contains("tenant", "s1") {
		t.Fatal("valid selection denied")
	}
	f.clock.Add(400)
	if s.Contains("tenant", "s1") || s.Tenant() != "" || s.All() || len(s.IDs()) != 0 {
		t.Fatal("expired selection retained usable authority")
	}
	if _, err = access.Constrain(e, "query.execute", "source", "query"); err == nil {
		t.Fatal("expired authority built new selection")
	}
}
