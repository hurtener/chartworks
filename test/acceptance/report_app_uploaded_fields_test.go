package acceptance

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/vindex"
)

func TestReportAppUploadedFieldsNative(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	raw := []byte("device,observed_at,reading,elapsed_ms,accepted\nSensor A,2026-03-08T05:00:00Z,12.25,130,true\nSensor A,2026-03-08T07:00:00Z,15.5,160,false\n")
	loaded := f.load(t, engineeringSpec("typed-upload", "csv", raw, []engineering.UploadColumn{{Name: "device", Type: "text"}, {Name: "observed_at", Type: "timestamp"}, {Name: "reading", Type: "number"}, {Name: "elapsed_ms", Type: "integer"}, {Name: "accepted", Type: "boolean"}}), raw)
	source := *loaded.Upload.Source
	binding, err := f.s.Binding(t.Context(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	physical, err := f.s.DescribeDataset(t.Context(), f.e, sources.DatasetDescribeRequest{Source: source.ID, Context: source.ContextID, Dataset: binding.Relations[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	pin := &reporting.SourceDatasetPin{Source: source.ID, Context: source.ContextID, Dataset: physical.Relation.ID, SourceRevision: physical.Revision, SchemaDigest: physical.SchemaDigest}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := topics.New(f.db, f.s, index, nil)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := reporting.New(f.db, topic, f.s, f.validator, f.executor, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	documents, err := reporting.NewDocuments(f.db, blocks, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewCompositions(documents, f.db, nil, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	authoring, err := reporting.NewAuthoring(documents, runs)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"sources.read", "sources.query", "charts.bind", "reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "cw.tenant.read:source-a", "cw.tenant.write:source-a", "cw.source.read:" + pin.Source, "cw.source.query:" + pin.Source, "cw.dataset.query:" + pin.Dataset, "cw.execution_context.use:" + pin.Context, "cw.block.read:upload-chart", "cw.block.write:upload-chart", "cw.block.preview:upload-chart"}
	author := f.token.envelope(t, "source-a", "upload-author", scopes...)
	meta, err := authoring.Dataset(t.Context(), author, reporting.AuthoringDatasetRequest{SourceDataset: pin, Dataset: pin.Dataset})
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]string{}
	for _, c := range meta.Fields.Columns {
		columns[c.SourceName] = c.ID
	}
	selected := []string{"group_1", "group_2", "group_3", "value_1", "value_2", "value_3"}
	table := &charts.TableOptions{PageSize: 20}
	for _, c := range selected {
		table.Columns = append(table.Columns, charts.TableColumnIntent{Column: c, Visible: true})
	}
	request := reporting.AuthoringPrepareRequest{NewBlock: "upload-chart", OperationVersion: reporting.AuthoringPreparationOperationVersion, Operation: "prepare:" + strconv.FormatInt(time.Now().Unix(), 10) + ":" + strings.Repeat("a", 32), Metadata: []reporting.Localized{{Locale: "en", Title: "Uploaded fields", Question: "Selected uploaded fields"}}, Intent: reporting.AuthoringDatasetIntent{SourceDataset: pin, Dataset: pin.Dataset, Fields: &reporting.AuthoringFieldSelection{Mode: "aggregate", Dimensions: []reporting.AuthoringGrouping{{Kind: "column", Field: columns["device"]}, {Kind: "column", Field: columns["accepted"]}, {Kind: "column", Field: columns["observed_at"], Grain: "day", Calendar: "gregorian", Timezone: "America/New_York"}}, Measures: []reporting.AuthoringMeasureSelection{{Kind: "column", Field: columns["reading"], Aggregation: "average"}, {Kind: "column", Field: columns["elapsed_ms"], Aggregation: "maximum"}, {Kind: "count"}}}, Mapping: reporting.AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: selected}, Options: charts.DefaultOptions(), Table: table}}}
	prepared, err := authoring.PrepareDatasetChart(t.Context(), author, request)
	if err != nil || prepared.Status != "prepared" || len(prepared.Schema) != 6 {
		t.Fatal("uploaded typed preparation", prepared, err)
	}
	created, err := authoring.CreatePreparedChart(t.Context(), author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal("consume", err)
	}
	for _, column := range created.Block.Outputs[0].Mapping.Columns {
		if column.ID == "value_1" && !column.Format.PreservePrecision {
			t.Fatal("new average silently rounds unknown client data")
		}
		if column.ID == "value_3" && column.Format.PreservePrecision {
			t.Fatal("row count no longer uses integer display")
		}
	}
	preview, err := blocks.Preview(t.Context(), author, request.NewBlock, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision}, Outputs: []string{"chart"}})
	if err != nil || len(preview.Result.Rows) != 2 || len(preview.Result.Schema) != 6 {
		t.Fatal("uploaded preview", err, preview.Result)
	}
	want := map[string]string{"15.5": "160", "12.25": "130"}
	for _, row := range preview.Result.Rows {
		if len(row) != 6 {
			t.Fatal("selected field count", len(row))
		}
		value, maximum := strings.Trim(string(row[3]), `"`), strings.Trim(string(row[4]), `"`)
		if want[value] != maximum || strings.Trim(string(row[5]), `"`) != "1" || !strings.Contains(string(row[2]), "2026-03-08") {
			t.Fatal("uploaded exact aggregates and explicit calendar", row)
		}
		delete(want, value)
	}
	if len(want) != 0 {
		t.Fatal("missing uploaded groups", want)
	}
}
