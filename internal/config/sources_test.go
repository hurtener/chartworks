package config

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func sourceConfigFixture() Sources {
	out := DefaultSources()
	out.Enabled = true
	out.Connections = []SourceConnection{{Tenant: "tenant", ID: "warehouse", Version: "v1", ReadDSN: "env:READ_DSN", WriteDSN: "env:WRITE_DSN", Relations: []SourceRelation{{Schema: "analytics", Name: "sales", Columns: []string{"id", "amount"}}}}}
	return out
}
func TestSourceConfiguration(t *testing.T) {
	settings := sourceConfigFixture()
	if err := ValidateSources(settings); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Sources){
		func(s *Sources) { s.MaxConns = 0 }, func(s *Sources) { s.MaxConns = 17 }, func(s *Sources) { s.MaxRows = 1001 }, func(s *Sources) { s.MaxBytes = 1 }, func(s *Sources) { s.ConnectTimeout = 0 }, func(s *Sources) { s.QueryTimeout = Duration(5 * time.Second) },
		func(s *Sources) { s.Connections = nil }, func(s *Sources) { s.Connections = append(s.Connections, s.Connections[0]) }, func(s *Sources) { s.Connections[0].Tenant = "" }, func(s *Sources) { s.Connections[0].ID = "bad/" }, func(s *Sources) { s.Connections[0].Version = "" }, func(s *Sources) { s.Connections[0].ReadDSN = "inline-secret" }, func(s *Sources) { s.Connections[0].WriteDSN = s.Connections[0].ReadDSN }, func(s *Sources) { s.Connections[0].WriteDSN = "inline-secret" }, func(s *Sources) { s.Connections[0].Relations = nil }, func(s *Sources) { s.Connections[0].Relations[0].Schema = "pg_catalog" }, func(s *Sources) { s.Connections[0].Relations[0].Name = "" }, func(s *Sources) { s.Connections[0].Relations[0].Columns = []string{"id", "id"} }, func(s *Sources) { s.Connections[0].Relations[0].Columns = []string{"bad/"} }, func(s *Sources) {
			s.Connections[0].Relations = append(s.Connections[0].Relations, s.Connections[0].Relations[0])
		},
	} {
		bad := settings.Clone()
		mutate(&bad)
		if ValidateSources(bad) == nil {
			t.Fatal("invalid source configuration accepted")
		}
	}
	data, err := json.Marshal(map[string]any{"auth": good().Auth, "sources": DefaultSources()})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Load(bytes.NewReader(data), func(string) (string, bool) { return "synthetic", true }, Overrides{})
	if err != nil {
		t.Fatal("default configuration roundtrip", err)
	}
	if parsed.Values().Sources.Connections == nil {
		t.Fatal("default aliases serialized as null")
	}
	data, err = json.Marshal(map[string]any{"auth": good().Auth, "sources": settings})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err = Load(bytes.NewReader(data), func(string) (string, bool) { return "synthetic", true }, Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	copy := parsed.Values()
	copy.Sources.Connections[0].Relations[0].Columns[0] = "changed"
	if parsed.Values().Sources.Connections[0].Relations[0].Columns[0] != "id" {
		t.Fatal("caller mutated compiled source config")
	}
	for _, mutate := range []func(*ReadValidation){func(v *ReadValidation) { v.MaxSQLBytes = 1 }, func(v *ReadValidation) { v.MaxParameters = 65 }, func(v *ReadValidation) { v.MaxASTDepth = 65 }, func(v *ReadValidation) { v.MaxASTNodes = 1 }, func(v *ReadValidation) { v.Concurrency = 0 }} {
		bad := DefaultReadValidation()
		mutate(&bad)
		if ValidateReadValidation(bad) == nil {
			t.Fatal("invalid parser bounds")
		}
	}
}
