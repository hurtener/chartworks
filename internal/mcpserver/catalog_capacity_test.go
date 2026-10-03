package mcpserver

import (
	"fmt"
	"testing"
)

func TestRegisteredCatalogCapacity(t *testing.T) {
	base := testRegistry(t, fixtureCall).bindings[0]
	entries := func(count int) []Binding {
		out := make([]Binding, count)
		for i := range out {
			out[i] = base
			out[i].name = fmt.Sprintf("bounded_%d", i)
			out[i].definition.ID = fmt.Sprintf("bounded%d", i)
			out[i].resource = ""
		}
		return out
	}
	for _, count := range []int{64, 74, 79, 96} {
		registry, err := NewRegistry(entries(count))
		if err != nil || len(registry.Manifest()) != count {
			t.Fatal("bounded registered inventory rejected or dropped", count, err)
		}
	}
	if _, err := NewRegistry(entries(97)); err == nil {
		t.Fatal("unbounded catalog admitted")
	}
	duplicate := entries(96)
	duplicate[95].name = duplicate[0].name
	if _, err := NewRegistry(duplicate); err == nil {
		t.Fatal("duplicate name admitted at bound")
	}
	duplicate = entries(96)
	duplicate[95].definition.ID = duplicate[0].definition.ID
	if _, err := NewRegistry(duplicate); err == nil {
		t.Fatal("duplicate operation admitted at bound")
	}
	bad := entries(96)
	bad[95].name = "invalid-name"
	if _, err := NewRegistry(bad); err == nil {
		t.Fatal("invalid name admitted at bound")
	}
}
