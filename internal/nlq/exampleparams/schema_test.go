package exampleparams

import (
	"encoding/json"
	"errors"
	"reflect"
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
	for _, s := range []*Schema{{}, {Version: Version}, {Version: "future", Slots: []Slot{{1, "integer"}}}, {Version: Version, Slots: []Slot{{0, "integer"}}}, {Version: Version, Slots: []Slot{{2, "integer"}}}, {Version: Version, Slots: []Slot{{1, "text"}, {1, "integer"}}}, {Version: Version, Slots: []Slot{{1, "secret-kind"}}}, {Version: Version, Slots: make([]Slot, 65)}} {
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
		s := &Schema{Version: version, Slots: []Slot{{position, kind}}}
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
