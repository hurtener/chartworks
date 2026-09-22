package reportingapi

import (
	"testing"

	"github.com/hurtener/chartworks/internal/api"
)

func TestDocumentAPIRegistry(t *testing.T) {
	registry, err := DocumentsRegistry(false)
	if err != nil {
		t.Fatal("closed document schemas", err)
	}
	if len(registry.Definitions()) != 30 {
		t.Fatal("document route inventory is incomplete", len(registry.Definitions()))
	}
	for _, route := range registry.Definitions() {
		if route.Public || route.Response == nil || route.ResourceLoader == "" || len(route.Errors) == 0 {
			t.Fatal("unprotected or incomplete route", route.ID)
		}
		if route.Method != "GET" && (route.Replay != "never" || route.Request == nil) {
			t.Fatal("mutation silently replayable or missing closed schema", route.ID)
		}
	}
	blocks, err := Registry(false, false, false)
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := RuntimeRegistry(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Compose(blocks, runtime, registry); err != nil {
		t.Fatal("document routes collide with existing phase 27/28 surfaces", err)
	}
}

func TestDocumentFilterOptionsRegistrationRequiresExecution(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		registry, err := DocumentsRegistry(enabled)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, definition := range registry.Definitions() {
			found = found || definition.ID == "report_filter_options"
		}
		if found != enabled {
			t.Fatal("document filter options capability mismatch", enabled, found)
		}
	}
}
