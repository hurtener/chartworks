package exampleparams

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestSQLRecoveryExampleParameterSchema(t *testing.T) {
	kinds := []string{"text", "integer", "number", "boolean", "null"}
	s, err := New(kinds)
	if err != nil {
		t.Fatal(err)
	}
	values, err := s.ProbeValues()
	if err != nil || !reflect.DeepEqual(values, []string{"example", "1", "1", "true", ""}) {
		t.Fatal("public probes", values, err)
	}
	kinds[0] = "mutation"
	if s.Slots[0].Kind != "text" {
		t.Fatal("caller alias")
	}
	clone := s.Clone()
	clone.Slots[0].Kind = "null"
	if s.Slots[0].Kind != "text" {
		t.Fatal("clone alias")
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var round Schema
	if err = json.Unmarshal(raw, &round); err != nil || round.Validate() != nil || !reflect.DeepEqual(s, &round) {
		t.Fatal("round trip", err)
	}
	var shape map[string]any
	_ = json.Unmarshal(raw, &shape)
	for _, item := range shape["slots"].([]any) {
		if len(item.(map[string]any)) != 2 {
			t.Fatal("slot contains value/default content")
		}
	}
}
func TestSQLRecoveryExampleParameterSchemaRejectsInvalid(t *testing.T) {
	for _, s := range []*Schema{{}, {Version: Version}, {Version: "future", Slots: []Slot{{Position: 1, Kind: "integer"}}}, {Version: Version, Slots: []Slot{{Position: 0, Kind: "integer"}}}, {Version: Version, Slots: []Slot{{Position: 2, Kind: "integer"}}}, {Version: Version, Slots: []Slot{{Position: 1, Kind: "text"}, {Position: 1, Kind: "integer"}}}, {Version: Version, Slots: []Slot{{Position: 1, Kind: "secret-kind"}}}, {Version: Version, Slots: make([]Slot, 65)}} {
		if !errors.Is(s.Validate(), ErrInvalid) {
			t.Fatal("invalid schema accepted")
		}
		if v, err := s.ProbeValues(); v != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal("partial probes from invalid schema")
		}
	}
	for _, kinds := range [][]string{nil, {}, make([]string, 65), {"unknown"}} {
		if s, err := New(kinds); s != nil || !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid constructor")
		}
	}
	var legacy *Schema
	if legacy.Validate() != nil || legacy.Clone() != nil {
		t.Fatal("legacy nil changed")
	}
	if values, err := legacy.ProbeValues(); err != nil || values != nil {
		t.Fatal("legacy probes changed")
	}
}
func TestSQLRecoveryExampleParameterSchemaConcurrent(t *testing.T) {
	s, _ := New([]string{"text", "number"})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, err := s.ProbeValues()
			if err != nil || v[1] != "1" {
				t.Error("shared schema changed")
			}
			v[0] = "not a default"
			c := s.Clone()
			c.Slots[0].Kind = "null"
		}()
	}
	wg.Wait()
	if s.Slots[0].Kind != "text" {
		t.Fatal("concurrent mutation")
	}
}
func FuzzSQLRecoveryExampleParameterSchema(f *testing.F) {
	f.Add("text", 1, Version)
	f.Add("null", 0, "future")
	f.Fuzz(func(t *testing.T, kind string, position int, version string) {
		s := &Schema{Version: version, Slots: []Slot{{Position: position, Kind: kind}}}
		v, err := s.ProbeValues()
		if err != nil {
			if v != nil {
				t.Fatal("partial probe")
			}
			return
		}
		if s.Validate() != nil || len(v) != 1 || position != 1 || version != Version {
			t.Fatal("invalid accepted schema")
		}
	})
}

func TestSQLRecoveryExampleParameterDomains(t *testing.T) {
	base, _ := New([]string{"text", "text", "text", "text", "number"})
	schema, err := base.WithDomains([]string{"date", "timestamp", "timestamptz", "uuid", ""})
	if err != nil || schema.Version != DomainVersion {
		t.Fatal(schema, err)
	}
	probes, err := schema.ProbeValues()
	if err != nil || !reflect.DeepEqual(probes, []string{"2000-01-02", "2000-01-02 03:04:05", "2000-01-02T03:04:05Z", "00000000-0000-4000-8000-000000000001", "1"}) {
		t.Fatal(probes, err)
	}
	if base.Version != Version || base.Slots[0].Domain != "" {
		t.Fatal("rewrote historical schema")
	}
	unchanged, err := base.WithDomains(make([]string, 5))
	if err != nil || !reflect.DeepEqual(base, unchanged) {
		t.Fatal("legacy schema changed", err)
	}
	for _, mutate := range []func(*Schema){func(s *Schema) { s.Version = Version }, func(s *Schema) { s.Slots[0].Kind = "integer" }, func(s *Schema) { s.Slots[0].Domain = "caller-defined" }, func(s *Schema) {
		for i := range s.Slots {
			s.Slots[i].Domain = ""
		}
	}} {
		bad := schema.Clone()
		mutate(bad)
		if bad.Validate() == nil {
			t.Fatal("invalid domain schema")
		}
	}
	if _, err := base.WithDomains([]string{"date"}); err == nil {
		t.Fatal("partial proof accepted")
	}
	raw, _ := json.Marshal(schema)
	if strings.Contains(string(raw), "2000") || strings.Contains(string(raw), "value") || strings.Contains(string(raw), "default") {
		t.Fatal("value-bearing schema")
	}
}

func TestSQLRecoveryExampleParameterDomainInventory(t *testing.T) {
	domains := []string{"date", "timestamp", "timestamptz", "uuid", "time", "timetz", "interval", "json", "jsonb"}
	kinds := make([]string, len(domains))
	for i := range kinds {
		kinds[i] = "text"
	}
	base, _ := New(kinds)
	schema, err := base.WithDomains(domains)
	if err != nil {
		t.Fatal(err)
	}
	probes, err := schema.ProbeValues()
	want := []string{"2000-01-02", "2000-01-02 03:04:05", "2000-01-02T03:04:05Z", "00000000-0000-4000-8000-000000000001", "03:04:05", "03:04:05+00:00", "1 day", "{}", "{}"}
	if err != nil || !reflect.DeepEqual(probes, want) {
		t.Fatal("canonical public probes", probes, err)
	}
	raw, _ := json.Marshal(schema)
	for _, value := range probes {
		if strings.Contains(string(raw), value) {
			t.Fatal("probe retained in schema")
		}
	}
	for _, domain := range []string{"enum", "vendor.json", "user-defined", "array", "sql-expression"} {
		if _, err := base.WithDomains(append([]string{domain}, domains[1:]...)); err == nil {
			t.Fatal("arbitrary domain", domain)
		}
	}
}
