//go:build chartworks_live_fixture

package acceptance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/sources"
)

// This opt-in fixture activates actual CSV uploads through the engineering core.
// The external reference service receives the persisted source catalog, not a
// topic, fake schema, generated bearer or preselected analytical question.
func TestReportAppUploadedLiveFixture(t *testing.T) {
	dir := os.Getenv("CHARTWORKS_LIVE_FIXTURE_OUTPUT")
	if !filepath.IsAbs(dir) {
		t.Fatal("explicit absolute fixture output required")
	}
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal("fresh private output required", err)
	}
	f := newEngineeringFixture(t, nil, nil)
	write := func(name string, value any) {
		t.Helper()
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	datasets := []any{}
	refs := []reporting.ResourceReference{}
	for _, input := range []struct {
		id, name, csv string
		columns       []engineering.UploadColumn
	}{
		{"observation-upload", "Observation log", "sample_id,device,observed_at,reading,elapsed_ms,accepted\n1,North sensor,2026-03-08T05:00:00Z,12.25,130,true\n2,South sensor,2026-03-08T06:00:00Z,8.75,95,true\n3,North sensor,2026-03-08T07:00:00Z,15.5,160,false\n4,West sensor,2026-03-09T04:00:00Z,11.5,120,true\n5,South sensor,2026-03-10T14:00:00Z,9.5,105,true\n6,West sensor,2026-03-11T14:00:00Z,14,150,false\n", []engineering.UploadColumn{{Name: "sample_id", Type: "integer"}, {Name: "device", Type: "text"}, {Name: "observed_at", Type: "timestamp"}, {Name: "reading", Type: "number"}, {Name: "elapsed_ms", Type: "integer"}, {Name: "accepted", Type: "boolean"}}},
		{"request-upload", "Request log", "ticket,queue,opened_on,duration,closed\nA-1,Intake,2026-03-08,30,true\nA-2,Research,2026-03-08,45,false\nA-3,Intake,2026-03-09,20,true\nA-4,Research,2026-03-10,60,true\n", []engineering.UploadColumn{{Name: "ticket", Type: "text"}, {Name: "queue", Type: "text"}, {Name: "opened_on", Type: "date"}, {Name: "duration", Type: "number"}, {Name: "closed", Type: "boolean"}}},
	} {
		raw := []byte(input.csv)
		spec := engineeringSpec(input.id, "csv", raw, input.columns)
		spec.Name = input.name
		loaded := f.load(t, spec, raw)
		source := *loaded.Upload.Source
		binding, err := f.s.Binding(t.Context(), f.e, source.ID, source.ContextID)
		if err != nil || len(binding.Relations) != 1 {
			t.Fatal("uploaded relation", err)
		}
		physical, err := f.s.DescribeDataset(t.Context(), f.e, sources.DatasetDescribeRequest{Source: source.ID, Context: source.ContextID, Dataset: binding.Relations[0].ID})
		if err != nil {
			t.Fatal(err)
		}
		pin := reporting.SourceDatasetPin{Source: physical.Source, Context: physical.Context, Dataset: physical.Relation.ID, SourceRevision: physical.Revision, SchemaDigest: physical.SchemaDigest}
		datasets = append(datasets, map[string]any{"name": input.name, "source_dataset": pin, "columns": physical.Relation.Columns, "rows": loaded.Upload.Rows, "csv": input.csv})
		refs = append(refs, reporting.ResourceReference{Kind: "source", ID: source.ID, Permission: "read"}, reporting.ResourceReference{Kind: "source", ID: source.ID, Permission: "query"}, reporting.ResourceReference{Kind: "dataset", ID: pin.Dataset, Permission: "query"}, reporting.ResourceReference{Kind: "execution_context", ID: source.ContextID, Permission: "use"})
	}
	writer, ok := f.lookup("CHARTWORKS_SOURCE_WRITE")
	if !ok {
		t.Fatal("fixture writer missing")
	}
	write("private.json", map[string]any{"store_url": f.dsn, "read_dsn": f.readDSN(), "write_dsn": writer, "sources": f.cfg.Clone()})
	write("public.json", map[string]any{"tenant": f.e.Tenant(), "datasets": datasets, "references": refs, "model_mode": "disabled during live journey", "upload_path": "reserve/stage/load through engineering.Service"})
	write("ready.json", map[string]any{"ready": true, "uploaded_datasets": len(datasets)})
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	deadline := time.NewTimer(2 * time.Hour)
	defer deadline.Stop()
	for {
		select {
		case <-t.Context().Done():
			t.Fatal("fixture interrupted")
		case <-deadline.C:
			t.Fatal("fixture lifetime exhausted")
		case <-tick.C:
			if _, err := os.Stat(filepath.Join(dir, "stop")); err == nil {
				write("completed.json", map[string]any{"stopped": true})
				return
			}
		}
	}
}
