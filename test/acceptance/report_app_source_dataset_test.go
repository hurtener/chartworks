package acceptance

import (
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func TestReportAppSourceDatasetNative(t *testing.T) {
	f, authoring, _, request, _, dataset, scopes := reportDatasetFixture(t, "source-chart")
	ctx := t.Context()
	scopes = slices.DeleteFunc(scopes, func(scope string) bool { return scope == "topics.read" || strings.HasPrefix(scope, "cw.topic.") })
	scopes = append(scopes, "reporting.publish", "cw.block.publish:source-chart")
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	physical, err := f.f.f.s.DescribeDataset(ctx, author, sources.DatasetDescribeRequest{Source: dataset.Source.Source, Context: dataset.Source.Context, Dataset: dataset.ID})
	if err != nil || physical.SchemaDigest == "" {
		t.Fatal("source metadata", err)
	}
	pin := &reporting.SourceDatasetPin{Source: physical.Source, Context: physical.Context, Dataset: physical.Relation.ID, SourceRevision: physical.Revision, SchemaDigest: physical.SchemaDigest}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	discovery := phase27Actor(t, f.f, author.User(), []string{"reporting.discover", "cw.source.read:" + pin.Source})
	requirements, err := authoring.DataDependencies(ctx, discovery, reporting.DataDependencyRequest{SourceDataset: pin, Dataset: pin.Dataset})
	if err != nil || !reflect.DeepEqual(requirements.SourceDataset, pin) || len(requirements.References) != 3 || len(requirements.QueryReferences) != 3 {
		t.Fatal("table dependency discovery", requirements, err)
	}
	if _, err := authoring.Dataset(ctx, discovery, reporting.AuthoringDatasetRequest{SourceDataset: pin, Dataset: pin.Dataset}); err == nil {
		t.Fatal("discovery seed admitted content")
	}
	for _, change := range []func(*reporting.SourceDatasetPin){func(p *reporting.SourceDatasetPin) { p.SourceRevision++ }, func(p *reporting.SourceDatasetPin) { p.SchemaDigest = strings.Repeat("0", 64) }, func(p *reporting.SourceDatasetPin) { p.Context = "different-context" }, func(p *reporting.SourceDatasetPin) { p.Dataset = "different-dataset" }} {
		bad := *pin
		change(&bad)
		if _, err := authoring.DataDependencies(ctx, discovery, reporting.DataDependencyRequest{SourceDataset: &bad, Dataset: bad.Dataset}); err == nil {
			t.Fatal("changed origin discovered")
		}
	}

	view, err := authoring.Dataset(ctx, author, reporting.AuthoringDatasetRequest{SourceDataset: pin, Dataset: pin.Dataset})
	if err != nil || view.SourceDataset == nil || view.Topic != (reporting.TopicPin{}) || view.Fields == nil || !view.Fields.Supported || len(view.Measures) != 0 || f.attemptCount(t) != before {
		t.Fatal("topic-free typed metadata", view, err)
	}
	registry, err := reportingapi.AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, reportingapi.AuthoringHandler(f.f.f.token.verifier, authoring, http.NotFoundHandler()))
	bearer := phase27Token(t, f.f, author.User(), author.Session(), scopes)
	metadataBody, _ := json.Marshal(map[string]any{"source_dataset": pin, "dataset": pin.Dataset})
	if response := callProtected(t, handler, "POST", "/v1/reporting/authoring/v1/dataset", bearer, string(metadataBody), map[string]string{"Content-Type": "application/json"}); response.Code != 200 {
		t.Fatal("source metadata HTTP", response.Code, response.Body.String())
	}
	var amount string
	for _, column := range view.Fields.Columns {
		if column.SourceName == "amount" {
			amount = column.ID
		}
	}
	if amount == "" {
		t.Fatal("actual physical column missing")
	}
	request.Intent = reporting.AuthoringDatasetIntent{SourceDataset: pin, Dataset: pin.Dataset, Fields: &reporting.AuthoringFieldSelection{Mode: "aggregate", Measures: []reporting.AuthoringMeasureSelection{{Kind: "column", Field: amount, Aggregation: "sum"}}}, Mapping: reporting.AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Value: "value_1"}, Options: charts.DefaultOptions()}}
	encoded, _ := json.Marshal(request)
	var body map[string]any
	_ = json.Unmarshal(encoded, &body)
	delete(body["intent"].(map[string]any), "topic")
	encoded, _ = json.Marshal(body)
	response := callProtected(t, handler, "POST", "/v1/reporting/authoring/v1/prepare_chart", bearer, string(encoded), map[string]string{"Content-Type": "application/json"})
	var prepared reporting.AuthoringPreparationView
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &prepared) != nil || prepared.Status != "prepared" {
		t.Fatal("source prepare HTTP", response.Code, response.Body.String())
	}
	for _, missing := range []string{"sources.query", "cw.source.read:" + pin.Source, "cw.dataset.query:" + pin.Dataset, "cw.execution_context.use:" + pin.Context} {
		narrowed := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool { return scope == missing }))
		if _, err := authoring.CreatePreparedChart(ctx, narrowed, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest}); err == nil {
			t.Fatal("consume accepted revoked reach", missing)
		}
	}
	other := phase27Actor(t, f.f, "different-table-author", scopes)
	if _, err := authoring.CreatePreparedChart(ctx, other, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest}); err == nil {
		t.Fatal("cross-author custody accepted")
	}
	base, err := f.f.f.db.ReadAuthoringPreparation(ctx, author, prepared.Preparation)
	if err != nil {
		t.Fatal(err)
	}
	privateDiscovery := phase27Actor(t, f.f, author.User(), []string{"reporting.discover", "reporting.preview", "cw.block.read:" + request.NewBlock, "cw.block.write:" + request.NewBlock, "cw.block.preview:" + request.NewBlock})
	checkCustody := func() {
		t.Helper()
		got, err := authoring.DataDependencies(ctx, privateDiscovery, reporting.DataDependencyRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation})
		if err != nil || !reflect.DeepEqual(got.SourceDataset, pin) || got.Topic != (reporting.TopicPin{}) || got.Preparation != prepared.Preparation || len(got.References) != 3 {
			t.Fatal("source custody", got, err)
		}
	}
	checkCustody()
	created, err := authoring.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil || created.Block.State.Topic != "" || created.Block.State.Source != pin.Source || created.Block.SourceDataset == nil || !created.Block.Private {
		t.Fatal("consume", created, err)
	}
	validated, err := f.blocks.Validate(ctx, author, request.NewBlock, reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision})
	if err != nil {
		t.Fatal("validate", err)
	}
	published, err := f.blocks.Publish(ctx, author, request.NewBlock, reporting.PublishRequest{ExpectedVersion: validated.State.Version, Evidence: validated.Evidence.ID})
	if err != nil || published.PublishedRevision != 1 {
		t.Fatal("publish", err)
	}
	readerScopes := slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool {
		return strings.Contains(scope, ".write") || strings.Contains(scope, ".publish") || strings.Contains(scope, ".preview") || scope == "reporting.validate" || scope == "sources.query" || strings.HasPrefix(scope, "cw.source.query:")
	})
	reader := phase27Actor(t, f.f, "ordinary-table-reader", readerScopes)
	read, err := f.blocks.Read(ctx, reader, request.NewBlock, reporting.Reference{})
	if err != nil || read.Private || read.SourceDataset == nil || read.State.Topic != "" {
		t.Fatal("published read", err)
	}
	denied := phase27Actor(t, f.f, reader.User(), slices.DeleteFunc(slices.Clone(readerScopes), func(scope string) bool { return strings.HasPrefix(scope, "cw.dataset.query:") }))
	if _, err := f.blocks.Read(ctx, denied, request.NewBlock, reporting.Reference{}); err == nil {
		t.Fatal("read without dataset reach")
	}
	executionScopes := append(slices.Clone(scopes), "reporting.execute", "cw.block.execute:"+request.NewBlock, "cw.run.read:*")
	executor := phase27Actor(t, f.f, author.User(), executionScopes)
	admitted, err := f.runs.Admit(ctx, executor, request.NewBlock, reporting.RunRequest{Key: "source-run"})
	if err != nil {
		t.Fatal("frozen admission", err)
	}
	completed, err := f.runs.Run(ctx, executor, admitted.ID, false)
	if err != nil || completed.State != "succeeded" {
		t.Fatal("frozen run", completed.State, err)
	}
	output, err := f.runs.Output(ctx, executor, admitted.ID, "chart")
	if err != nil || output.Chart == nil || len(output.Chart.Points) != 1 || output.Chart.Points[0].Value.Exact != "9007199254740998.625" {
		t.Fatal("exact retained aggregate", err)
	}
	reportScopes := append(slices.Clone(scopes), "cw.report.read:source-report", "cw.report.write:source-report", "cw.report.publish:source-report", "cw.report.preview:source-report")
	reportAuthor := phase27Actor(t, f.f, author.User(), reportScopes)
	document := phase29Text("Source dataset report")
	document.Widgets = append(document.Widgets, phase29BlockWidget("chart-widget", request.NewBlock, 1, "chart"))
	reportState, err := f.documents.Create(ctx, reportAuthor, "report", "source-report", document)
	if err != nil {
		t.Fatal("report create", err)
	}
	phase29Publish(t, f.documents, reportAuthor, reportState)
	runtimeScopes := slices.DeleteFunc(slices.Clone(reportScopes), func(scope string) bool {
		return strings.Contains(scope, ".write") || strings.Contains(scope, ".publish") || scope == "reporting.validate" || scope == "charts.bind"
	})
	runtimeScopes = append(runtimeScopes, "reporting.execute", "cw.block.execute:"+request.NewBlock, "cw.report.execute:source-report")
	runtime := phase27Actor(t, f.f, author.User(), runtimeScopes)
	composition, err := f.compositions.Admit(ctx, runtime, "report", "source-report", reporting.CompositionRequest{Key: "source-report-run"})
	if err != nil {
		t.Fatal("source composition admission", err)
	}
	composed, err := f.compositions.Run(ctx, runtime, composition.ID, false)
	if err != nil || !composed.Complete || composed.State != "completed" {
		t.Fatal("source composition", composed.State, err)
	}
	reportDiscovery := phase27Actor(t, f.f, author.User(), []string{"reporting.discover", "cw.report.read:source-report"})
	deps, err := authoring.Dependencies(ctx, reportDiscovery, reporting.DependencyRequest{Kind: "report", ID: "source-report"})
	if err != nil || len(deps.Blocks) != 1 || deps.Blocks[0].Source != pin.Source || deps.Blocks[0].Topic != "" {
		t.Fatal("source report dependencies", err)
	}
	amendment := phase27Copy(t, base.Revision.Definition)
	amendment.Metadata[0].Title = "Amended chart title"
	amended, err := f.blocks.Edit(ctx, author, request.NewBlock, reporting.EditRequest{ExpectedVersion: published.Version, Definition: amendment})
	if err != nil || amended.State.DraftRevision != 2 || amended.State.PublishedRevision != 1 {
		t.Fatal("source amendment", err)
	}
	if _, err := f.blocks.Read(ctx, reader, request.NewBlock, reporting.Reference{Revision: 2}); err == nil {
		t.Fatal("reader saw private amendment")
	}
	old, err := f.blocks.Read(ctx, reader, request.NewBlock, reporting.Reference{})
	if err != nil || old.Revision != 1 || old.Digest != created.Block.Digest {
		t.Fatal("publication changed with amendment", err)
	}
	// Cleanup must retain the source pin and original immutable Create receipt.
	raw := support.Raw(t, f.f.f.dsn)
	agePreparation(t, raw, author.Tenant(), base.ID, time.Now().Add(-25*time.Hour).UTC())
	next := preparationCopy(t, base, 600)
	if _, _, err := f.f.f.db.ReserveAuthoringPreparation(ctx, author, next); !errors.Is(err, store.ErrConflict) {
		t.Fatal("existing target", err)
	}
	if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1 AND preparation_id=$2`, author.Tenant(), base.ID) != 1 {
		t.Fatal("source receipt not compacted")
	}
	checkCustody()
	replay, err := authoring.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil || replay.Block.Digest != created.Block.Digest || !reflect.DeepEqual(replay.Block.SourceDataset, pin) {
		t.Fatal("compacted replay", err)
	}
	if f.attemptCount(t) != before+4 || f.f.model.requests.Load() != models {
		t.Fatal("unexpected source or model work")
	}
}
