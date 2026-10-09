//go:build chartworks_live_fixture

package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// TestReportAppLiveFixture is an explicit local-service fixture, excluded from
// ordinary acceptance runs. It publishes synthetic reviewed data through the
// existing domain services, then keeps the disposable databases alive for a
// separately running reference binary. Its test issuer and recorded gateway
// are setup-only: neither is exported to that binary or used by browser calls.
// Stop by creating OUTPUT/stop; all databases/roles are then cleaned up normally.
func TestReportAppLiveFixture(t *testing.T) {
	dir := os.Getenv("CHARTWORKS_LIVE_FIXTURE_OUTPUT")
	if !filepath.IsAbs(dir) {
		t.Fatal("explicit absolute CHARTWORKS_LIVE_FIXTURE_OUTPUT required")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal("fresh private output directory required", err)
	}
	var f *phase29ExecutionFixture
	var request reporting.AuthoringPrepareRequest
	var publication topics.Published
	expected := "9007199254740998.625"
	if os.Getenv("CHARTWORKS_LIVE_FIXTURE_BUSINESS") == "true" {
		f, request, publication, expected = businessDatasetFixture(t)
	} else if os.Getenv("CHARTWORKS_LIVE_FIXTURE_OPTIONS") == "true" {
		f, _, _, request, publication, _ = filteredDatasetFixture(t)
		expected = "3.750"
	} else {
		f, _, _, request, publication, _, _ = reportDatasetFixture(t, "fixture-unallocated")
	}
	base := f.f.f.sourceFixture
	write := func(name string, value any) {
		t.Helper()
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	connections := base.cfg.Clone()
	connections.Connections = []config.SourceConnection{connections.Connections[0]}
	connections.Connections[0].WriteDSN = ""
	write("private.json", map[string]any{
		"store_url": base.dsn, "read_dsn": base.readDSN(), "sources": connections,
	})
	refs := []reporting.ResourceReference{{Kind: "topic", ID: request.Intent.Topic.Topic, Permission: "read"}, {Kind: "topic", ID: request.Intent.Topic.Topic, Permission: "write"}}
	for _, dataset := range publication.Definition.Datasets {
		refs = append(refs, reporting.ResourceReference{Kind: "source", ID: dataset.Source.Source, Permission: "read"}, reporting.ResourceReference{Kind: "dataset", ID: dataset.ID, Permission: "query"}, reporting.ResourceReference{Kind: "execution_context", ID: dataset.Source.Context, Permission: "use"})
		if dataset.ID == request.Intent.Dataset {
			refs = append(refs, reporting.ResourceReference{Kind: "source", ID: dataset.Source.Source, Permission: "query"})
		}
	}
	write("public.json", map[string]any{
		"tenant": base.e.Tenant(), "topic_name": publication.Definition.Name,
		"request": request, "references": refs,
		"expected_value": expected, "model_mode": "disabled during live journey",
	})
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	write("ready.json", map[string]any{"ready": true, "setup_source_attempts": before})
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	deadline := time.NewTimer(2 * time.Hour)
	defer deadline.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("fixture interrupted")
		case <-deadline.C:
			t.Fatal("fixture lifetime exhausted")
		case <-ticker.C:
			if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
				write("completed.json", map[string]any{"live_source_attempts": f.attemptCount(t) - before, "live_fixture_model_calls": f.f.model.requests.Load() - models})
				return
			}
		}
	}
}
