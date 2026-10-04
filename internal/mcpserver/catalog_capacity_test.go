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
	for _, count := range []int{64, 74, 77, 78, 79, 82, 83, 88, 96} {
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

func TestManualChartMutationEffects(t *testing.T) {
	for _, name := range []string{"block_draft_cas_commit", "private_block_copy_commit", "private_block_preparation_consume"} {
		got, ok := effectFor(name)
		if !ok || !got.persists || got.readOnly || got.idempotent || got.paid || got.openWorld || got.destructive {
			t.Fatalf("invalid metadata mutation effect %s: %+v", name, got)
		}
	}
	got, ok := effectFor("explicit_bounded_source_read_private_evidence")
	if !ok || !got.persists || !got.paid || !got.openWorld || got.readOnly || got.idempotent || got.destructive {
		t.Fatalf("validation source cost/replay semantics hidden: %+v", got)
	}
}

func TestManualPreparationEffects(t *testing.T) {
	source, ok := effectFor("bounded_source_read_private_preparation")
	if !ok || !source.openWorld || !source.persists || !source.paid || source.readOnly || source.idempotent {
		t.Fatal("preparation source effect", source)
	}
	control, ok := effectFor("existing_source_attempt_control")
	if !ok || !control.openWorld || !control.persists || control.readOnly || control.idempotent {
		t.Fatal("attempt control marked read-only or replayable", control)
	}
	status, ok := effectFor("retained_metadata_read")
	if !ok || !status.readOnly || !status.idempotent || status.openWorld || status.persists || status.paid {
		t.Fatal("status unexpectedly controls source", status)
	}
}
