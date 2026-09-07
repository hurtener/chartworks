package config

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestEngineeringSnapshotPreservesEmptyArrays(t *testing.T) {
	v := Defaults()
	v.Sources.Connections = []SourceConnection{{Tenant: "tenant", ID: "workspace", ManagedSchema: "cw_test", Relations: []SourceRelation{}}}
	c := Config{values: v}
	for _, snapshot := range []Values{v, c.Values(), {Sources: v.Sources.Clone(), Profiling: v.Profiling.Clone()}} {
		// Test the concrete new fields separately from unrelated zero-value settings.
		for _, value := range []any{snapshot.Sources, snapshot.Profiling} {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if err = checkJSON(json.NewDecoder(bytes.NewReader(raw)), 0); err != nil {
				t.Fatalf("snapshot cannot pass the closed configuration decoder: %s: %v", raw, err)
			}
		}
	}
	raw, err := json.Marshal(c.Values())
	if err != nil {
		t.Fatal(err)
	}
	if err = checkJSON(json.NewDecoder(bytes.NewReader(raw)), 0); err != nil {
		t.Fatalf("complete snapshot contains invalid JSON: %s: %v", raw, err)
	}
	original := DefaultProfiling()
	original.Policies = []ProfilePolicy{{ID: "p", Tenant: "t", Source: "s", RangeColumns: []string{}}}
	copied := original.Clone()
	copied.Policies[0].RangeColumns = append(copied.Policies[0].RangeColumns, "amount")
	if len(original.Policies[0].RangeColumns) != 0 {
		t.Fatal("snapshot shares a policy column list")
	}
}
