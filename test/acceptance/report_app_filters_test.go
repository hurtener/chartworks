package acceptance

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

func filteredDatasetFixture(t *testing.T) (*phase29ExecutionFixture, *reporting.Authoring, identity.Envelope, reporting.AuthoringPrepareRequest, topics.Published, []string) {
	t.Helper()
	f, s, _, request, _, _, _ := reportDatasetFixture(t, "filtered-chart")
	ctx := t.Context()
	// A fresh registered source and reviewed profile observe the actual date type;
	// no retained binding or column type is invented for the compiler.
	if _, err := f.f.f.admin.Exec(ctx, `ALTER TABLE analytics.sales ALTER COLUMN created_at TYPE date USING (created_at AT TIME ZONE 'UTC')::date;
 TRUNCATE analytics.sales;
 INSERT INTO analytics.sales(id,amount,created_at,name) VALUES
 (1,1.25,'2026-01-01','North'),(2,2.5,'2026-01-02','South'),
 (3,100,'2026-02-01','North'),(4,1000,'2026-01-02',NULL),
 (5,7,'2026-01-02','x'' OR true --'),(6,9,'2026-01-02',''),
 (7,3.25,'2026-01-31','East');`); err != nil {
		t.Fatal(err)
	}
	source := f.f.f.create(t, "filtered-source")
	profile := f.f.f.profile(t, f.f.f.profileSpec(t, source, "filtered-profile", []string{"id", "amount", "name", "created_at"}, "")).Profile.Profile
	dataset := semantics.Dataset{ID: profile.Dataset, Name: "Filtered sales", Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: profile.Dataset, SourceRevision: source.Revision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
	for _, c := range profile.Schema {
		if slices.Contains([]string{"id", "amount", "name", "created_at"}, c.Name) {
			dataset.Columns = append(dataset.Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Sensitivity: semantics.LiteralNonSensitive})
		}
	}
	pack := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: "filtered-sales", Version: "v1", Name: "Filtered sales", Description: "Synthetic reviewed filter fixture", Datasets: []semantics.Dataset{dataset}, Measures: []semantics.Measure{{ID: "revenue", Name: "Revenue", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: "amount"}, Aggregation: semantics.AggregationSum, Unit: "USD"}}, Dimensions: []semantics.Dimension{{ID: "region", Name: "Region", Role: semantics.DimensionCategorical, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: "name"}}, {ID: "day", Name: "Day", Role: semantics.DimensionTemporal, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: "created_at"}}}}
	draftService, err := drafts.New(f.f.f.db, f.f.f.s, f.f.f.service)
	if err != nil {
		t.Fatal(err)
	}
	_, topicService := newPhase18Service(t, f.f)
	reviewer := f.f.f.token.envelope(t, f.author.Tenant(), f.author.User(), topicScopes(f.author.Tenant())...)
	publication := phase17PublishTopic(t, draftService, topicService, reviewer, pack)
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "reporting.execute", "topics.read", "sources.read", "sources.query", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:filtered-chart", "cw.block.write:filtered-chart", "cw.block.preview:filtered-chart", "cw.block.execute:filtered-chart", "cw.topic.read:" + pack.Topic, "cw.topic.write:" + pack.Topic, "cw.source.read:" + source.ID, "cw.source.query:" + source.ID, "cw.dataset.query:" + dataset.ID, "cw.execution_context.use:" + source.ContextID, "cw.report.read:filtered-report", "cw.report.write:filtered-report", "cw.report.preview:filtered-report", "cw.report.execute:filtered-report"}
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	request.Intent.Topic = reporting.TopicPin{Topic: pack.Topic, Version: pack.Version, Digest: publication.Digest}
	request.Intent.Dataset = dataset.ID
	request.Intent.Filters = []reporting.AuthoringDatasetFilter{{Dimension: "region", Kind: "multi_select", Default: reporting.Value{Items: []string{"South", "North"}}}, {Dimension: "day", Kind: "date_range", Default: reporting.Value{DateRange: &reporting.DateRange{Start: "2026-01-01", EndExclusive: "2026-02-01"}}}}
	return f, s, author, request, publication, scopes
}

func TestReportAppTypedFilterJourney(t *testing.T) {
	f, s, author, request, publication, scopes := filteredDatasetFixture(t)
	ctx := t.Context()
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	prepared, err := s.PrepareDatasetChart(ctx, author, request)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal("prepare filtered", prepared, err)
	}
	replay, err := s.PrepareDatasetChart(ctx, author, request)
	if err != nil || replay.Digest != prepared.Digest || f.attemptCount(t) != before+1 {
		t.Fatal("filtered replay", err)
	}
	changed := phase27Copy(t, request)
	changed.Intent.Filters[0].Default.Items = []string{"North"}
	if _, err := s.PrepareDatasetChart(ctx, author, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed default reused custody", err)
	}
	created, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil || created.Block.SchemaVersion != reporting.CurrentSchemaVersion || len(created.Block.Parameters) != 2 || created.Block.Evidence != nil || created.Block.Outputs[0].Intent == nil {
		t.Fatal("filtered private draft", created, err)
	}
	originalParameters := phase27Copy(t, created.Block.Parameters)
	widget := phase29BlockWidget("filtered-widget", request.NewBlock, 0, "chart")
	widget.Block.Policy = "private_preview"
	widget.Block.Revision = created.Block.Revision
	widget.Block.Digest = created.Block.Digest
	page := reporting.ReportPage{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{widget}}
	for _, p := range created.Block.Parameters {
		filter := phase27Copy(t, p)
		filter.Name = p.Dimension.Dimension
		page.Filters = append(page.Filters, reporting.ReportFilter{Parameter: filter, Label: filter.Name})
		page.Widgets[0].Bindings = append(page.Widgets[0].Bindings, reporting.FilterBinding{Filter: filter.Name, Parameter: p.Name})
	}
	document := phase29Text("Filtered report")
	document.SchemaVersion = reporting.PagedDocumentVersion
	document.Widgets = nil
	document.ReportPages = []reporting.ReportPage{page, {ID: "empty", Title: "Empty", Widgets: []reporting.Widget{}}}
	state, err := s.Create(ctx, author, reporting.AuthoringCreateRequest{ID: "filtered-report", Definition: document})
	if err != nil {
		t.Fatal("save unvalidated filters", err)
	}
	saved, err := s.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil || f.attemptCount(t) != before+1 {
		t.Fatal("metadata caused a source read", err)
	}
	validated, err := s.ValidateBlock(ctx, author, reporting.AuthoringBlockValidateRequest{Block: request.NewBlock, ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision, Digest: created.Block.Digest, Arguments: []reporting.Argument{}})
	if err != nil {
		t.Fatal("validate filters", err)
	}
	if validated.State.DraftState != "validated" || f.attemptCount(t) != before+2 {
		t.Fatal("validation not separate")
	}
	run := func(key string, values []string, want string) {
		t.Helper()
		pages := []reporting.PageInput{}
		if values != nil {
			pages = []reporting.PageInput{{Page: "analysis", Filters: []reporting.Argument{{Name: "region", Value: reporting.Value{Items: values}}}, Overrides: []reporting.WidgetOverride{}}}
		}
		preview, err := s.Preview(ctx, author, reporting.AuthoringPreviewRequest{Report: state.ID, Revision: state.DraftRevision, Key: key, Pages: pages})
		if err != nil {
			t.Fatal("preview", err)
		}
		complete, err := s.Execute(ctx, author, reporting.AuthoringExecuteRequest{Run: preview.ID})
		if err != nil || !complete.Complete {
			t.Fatal("execute", complete, err)
		}
		reader := phase27Actor(t, f.f, author.User(), []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + state.ID, "cw.execution_context.use:" + created.Block.Context})
		payload, err := f.compositions.Widget(ctx, reader, preview.ID, "analysis", "filtered-widget")
		if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].Chart == nil || len(payload.Outputs[0].Chart.Points) != 1 || payload.Outputs[0].Chart.Points[0].Value.Exact != want {
			t.Fatal("exact filtered value", want, payload, err)
		}
	}
	run("filtered-defaults", nil, "3.750")
	run("filtered-one", []string{"North"}, "1.250")
	run("filtered-boundary", []string{"East"}, "3.250")
	run("filtered-injection", []string{"x' OR true --"}, "7.000")
	run("filtered-empty-text", []string{""}, "9.000")
	run("filtered-maximum", []string{"North", "South", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k", "l", "m", "n"}, "3.750")
	current, err := s.Read(ctx, author, reporting.AuthoringReadRequest{Report: state.ID})
	if err != nil || !reflect.DeepEqual(saved.Definition, current.Definition) || saved.Digest != current.Digest {
		t.Fatal("runtime selections changed saved defaults", err)
	}
	block, err := s.ReadBlock(ctx, author, reporting.AuthoringBlockReadRequest{Block: request.NewBlock, Revision: created.Block.Revision})
	if err != nil || !reflect.DeepEqual(originalParameters, block.Block.Parameters) {
		t.Fatal("runtime selections changed block defaults", err)
	}
	if f.attemptCount(t) != before+8 || f.f.model.requests.Load() != models {
		t.Fatal("unexpected hidden query or model work")
	}
	// Arbitrary native block input must satisfy the same new-type predicate proof.
	native, err := f.f.f.db.ReadBlock(ctx, author, request.NewBlock, reporting.Reference{Revision: created.Block.Revision}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	badActor := phase27Actor(t, f.f, author.User(), append(slices.Clone(scopes), "cw.block.write:bad-filter"))
	for _, sql := range []string{strings.Replace(native.Revision.Definition.SQL, " IN ", " NOT IN ", 1), strings.Replace(native.Revision.Definition.SQL, `"name" IN`, `"amount" IN`, 1)} {
		bad := phase27Copy(t, native.Revision.Definition)
		bad.SQL = sql
		if _, err := f.blocks.Create(ctx, badActor, reporting.CreateRequest{ID: "bad-filter", Definition: bad}); err == nil {
			t.Fatal("unsafe native filter definition accepted")
		}
	}
	if f.attemptCount(t) != before+8 {
		t.Fatal("rejected native SQL executed source")
	}
	pending, err := s.Preview(ctx, author, reporting.AuthoringPreviewRequest{Report: state.ID, Revision: state.DraftRevision, Key: "filter-rule-activation"})
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.f.f.db, f.f.f.db, f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	phase17PublishRules(t, rules, f.blockAuthor, publication)
	if _, err := s.ValidateBlock(ctx, author, reporting.AuthoringBlockValidateRequest{Block: request.NewBlock, ExpectedVersion: validated.State.Version, Revision: created.Block.Revision, Digest: created.Block.Digest}); err == nil {
		t.Fatal("filtered validation ignored active rules")
	}
	if _, err := s.Execute(ctx, author, reporting.AuthoringExecuteRequest{Run: pending.ID}); err == nil {
		t.Fatal("filtered pending run ignored active rules")
	}
	if f.attemptCount(t) != before+8 {
		t.Fatal("rules rejection executed a source query")
	}
}

func TestReportAppSelectFilterJourney(t *testing.T) {
	f, s, author, request, _, _ := filteredDatasetFixture(t)
	request.Intent.Filters = []reporting.AuthoringDatasetFilter{{Dimension: "region", Kind: "select", Default: reporting.Value{Literal: "North"}}}
	ctx := t.Context()
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	prepared, err := s.PrepareDatasetChart(ctx, author, request)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal(prepared, err)
	}
	created, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil || len(created.Block.Parameters) != 1 || created.Block.Parameters[0].Type != "dimension_value" {
		t.Fatal(created, err)
	}
	preview, err := f.blocks.Preview(ctx, author, request.NewBlock, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision}, Outputs: []string{"chart"}})
	if err != nil || len(preview.Result.Rows) != 1 || len(preview.Result.Rows[0]) != 1 || string(preview.Result.Rows[0][0]) != `"101.250"` {
		t.Fatal("actual equality predicate", preview.Result, err)
	}
	if f.attemptCount(t) != before+2 || f.f.model.requests.Load() != models {
		t.Fatal("select hidden work")
	}
}
