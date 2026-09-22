package migrationapi

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/migration"
)

func TestRegistryMatchesPublishedManifest(t *testing.T) {
	registry, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../docs/contracts/chartworks-migration-operations.json")
	if err != nil {
		t.Fatal(err)
	}
	var published []api.Operation
	if json.Unmarshal(raw, &published) != nil || !reflect.DeepEqual(published, registry.Operations()) {
		t.Fatal("published migration inventory drifted from executable registry")
	}
	if len(registry.Definitions()) != 7 {
		t.Fatal("missing migration operations")
	}
}

func TestMCPBindingsUseExecutableRegistry(t *testing.T) {
	adapter := migration.AdapterFuncs{
		ValidateFunc: func(context.Context, identity.Envelope, migration.Object, migration.Mapping) error { return nil },
		ApplyFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
			return o.ExternalRef, nil
		},
	}
	service, err := migration.New(migration.NewMemoryRepository(nil), map[migration.Kind]migration.Adapter{migration.KindSource: adapter}, nil)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := MCPBindings(service)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := mcpserver.NewRegistry(bindings)
	if err != nil || len(registry.Manifest()) != 7 {
		t.Fatal("missing typed migration bindings", err)
	}
	absent, err := MCPBindings(nil)
	if err != nil || len(absent) != 0 {
		t.Fatal("unavailable migration service advertised", err)
	}
}
