package acceptance

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/vindex"
)

// Both schemas are synthetic. All field bindings below come from a newly
// profiled actual source; neither a mocked result nor a caller schema is accepted.
func TestReportAppTypedFieldsNative(t *testing.T) {
	for _, domain := range []struct{ name, key, value, category, occurred string }{
		{"observations", "specimen", "reading", "instrument", "observed_at"},
		{"requests", "ticket", "duration", "queue", "opened_at"},
	} {
		t.Run(domain.name, func(t *testing.T) {
			f, _, _, request, _, _, _ := reportDatasetFixture(t, "typed-chart")
			ctx := t.Context()
			// Fixture-owned table only. The name is inherited from the base test
			// infrastructure; the new source's column structure and meaning differ.
			ddl := fmt.Sprintf(`TRUNCATE analytics.sales;
ALTER TABLE analytics.sales RENAME COLUMN id TO %s;
ALTER TABLE analytics.sales RENAME COLUMN amount TO %s;
ALTER TABLE analytics.sales RENAME COLUMN name TO %s;
ALTER TABLE analytics.sales RENAME COLUMN created_at TO %s;
INSERT INTO analytics.sales(%s,%s,%s,%s) VALUES
(1,2.5,'alpha','2026-01-02T00:30:00Z'),
(2,7.5,'alpha','2026-01-02T01:30:00Z'),
(3,12,'beta','2026-02-02T12:00:00Z');`, domain.key, domain.value, domain.category, domain.occurred, domain.key, domain.value, domain.category, domain.occurred)
			if _, err := f.f.f.admin.Exec(ctx, ddl); err != nil {
				t.Fatal(err)
			}
			// Register the renamed physical schema as explicit operator configuration.
			// This is a fresh adapter, not a mutation of a retained source binding.
			settings := f.f.f.cfg.Clone()
			settings.Connections[0].Relations = []config.SourceRelation{{Schema: "analytics", Name: "sales", Columns: []string{domain.key, domain.value, domain.category, domain.occurred}}}
			sourceService, err := sources.New(f.f.f.db, settings, f.f.f.lookup)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(sourceService.Close)
			validator, err := readexec.NewValidator(sourceService, config.DefaultReadValidation())
			if err != nil {
				t.Fatal(err)
			}
			executor, err := readexec.NewExecutor(sourceService, f.f.f.db, f.f.f.values.Exec)
			if err != nil {
				t.Fatal(err)
			}
			profileService, err := engineering.New(f.f.f.db, sourceService, validator, executor, nil, f.f.f.values, f.f.f.lookup)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(profileService.Close)
			source, err := sourceService.Create(ctx, f.f.f.e, sources.CreateRequest{ID: "typed-source", Name: domain.name, Connection: "warehouse"})
			if err != nil {
				t.Fatal(err)
			}
			binding, err := sourceService.Binding(ctx, f.f.f.e, source.ID, source.ContextID)
			if err != nil || len(binding.Relations) != 1 {
				t.Fatal(err)
			}
			profileRun, err := profileService.Build(ctx, f.f.f.e, engineering.ProfileSpec{ID: "typed-profile", Source: source.ID, Context: source.ContextID, Dataset: binding.Relations[0].ID, Columns: []string{domain.key, domain.value, domain.category, domain.occurred}, SkipLLM: true}, "typed-profile-build", false)
			if err != nil || profileRun.Profile.Profile == nil {
				t.Fatal("profile", err)
			}
			profile := profileRun.Profile.Profile
			dataset := semantics.Dataset{ID: profile.Dataset, Name: domain.name, Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: profile.Dataset, SourceRevision: source.Revision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
			for _, c := range profile.Schema {
				dataset.Columns = append(dataset.Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Sensitivity: semantics.LiteralNonSensitive})
			}
			pack := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: domain.name, Version: "v1", Name: domain.name, Description: "Synthetic typed field authoring acceptance", Datasets: []semantics.Dataset{dataset}, Measures: []semantics.Measure{{ID: "mean", Name: "Reviewed mean", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: domain.value}, Aggregation: semantics.AggregationAverage}}}
			draftService, err := drafts.New(f.f.f.db, sourceService, profileService)
			if err != nil {
				t.Fatal(err)
			}
			index, err := vindex.New(f.f.f.db)
			if err != nil {
				t.Fatal(err)
			}
			topicService, err := topics.New(f.f.f.db, sourceService, index, f.f.model.engine)
			if err != nil {
				t.Fatal(err)
			}
			blocks, err := reporting.New(f.f.f.db, topicService, sourceService, validator, executor, nil, config.DefaultReporting())
			if err != nil {
				t.Fatal(err)
			}
			documents, err := reporting.NewDocuments(f.f.f.db, blocks, nil, config.DefaultReporting())
			if err != nil {
				t.Fatal(err)
			}
			s, err := reporting.NewAuthoring(documents, f.compositions)
			if err != nil {
				t.Fatal(err)
			}
			reviewer := f.f.f.token.envelope(t, f.author.Tenant(), f.author.User(), topicScopes(f.author.Tenant())...)
			publication := phase17PublishTopic(t, draftService, topicService, reviewer, pack)
			scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "topics.read", "sources.read", "sources.query", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:typed-chart", "cw.block.write:typed-chart", "cw.block.preview:typed-chart", "cw.topic.read:" + pack.Topic, "cw.topic.write:" + pack.Topic, "cw.source.read:" + source.ID, "cw.source.query:" + source.ID, "cw.dataset.query:" + dataset.ID, "cw.execution_context.use:" + source.ContextID}
			author := phase27Actor(t, f.f, f.author.User(), scopes)
			request.Intent = reporting.AuthoringDatasetIntent{Topic: reporting.TopicPin{Topic: pack.Topic, Version: pack.Version, Digest: publication.Digest}, Dataset: dataset.ID, Dimensions: []string{}, Fields: &reporting.AuthoringFieldSelection{Mode: "aggregate", Dimensions: []reporting.AuthoringGrouping{{Kind: "column", Field: domain.key}, {Kind: "column", Field: domain.category}, {Kind: "column", Field: domain.occurred, Grain: "month", Calendar: "gregorian", Timezone: "UTC"}}, Measures: []reporting.AuthoringMeasureSelection{{Kind: "measure", Field: "mean"}, {Kind: "column", Field: domain.value, Aggregation: "maximum"}, {Kind: "count"}}}}
			bindings := []string{"group_1", "group_2", "group_3", "value_1", "value_2", "value_3"}
			table := &charts.TableOptions{PageSize: 20}
			for _, column := range bindings {
				table.Columns = append(table.Columns, charts.TableColumnIntent{Column: column, Visible: true})
			}
			request.Intent.Mapping = reporting.AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: bindings}, Options: charts.DefaultOptions(), Table: table}
			request.Metadata = []reporting.Localized{{Locale: "en", Title: domain.name, Question: domain.name, Aliases: []string{}}}
			before, models := f.attemptCount(t), f.f.model.requests.Load()
			catalog, err := s.Dataset(ctx, author, reporting.AuthoringDatasetRequest{Topic: request.Intent.Topic, Dataset: dataset.ID})
			if err != nil || catalog.Fields == nil || !catalog.Fields.Supported || len(catalog.Fields.Columns) != 4 || f.attemptCount(t) != before {
				t.Fatal("retained typed catalog", catalog, err)
			}
			prepared, err := s.PrepareDatasetChart(ctx, author, request)
			if err != nil || prepared.Status != "prepared" || len(prepared.Schema) != 6 {
				t.Fatal("typed preparation", prepared, err)
			}
			if replay, err := s.PrepareDatasetChart(ctx, author, request); err != nil || replay.Digest != prepared.Digest || f.attemptCount(t) != before+1 {
				t.Fatal("immutable preparation replay", replay, err)
			}
			revoked := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool { return strings.HasPrefix(scope, "cw.dataset.query:") }))
			if _, err := s.CreatePreparedChart(ctx, revoked, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest}); err == nil {
				t.Fatal("revoked data reach consumed typed preparation")
			}
			created, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: request.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
			if err != nil || !created.Block.Private || f.attemptCount(t) != before+1 {
				t.Fatal("private consume", err)
			}
			preview, err := blocks.Preview(ctx, author, request.NewBlock, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision}, Outputs: []string{"chart"}})
			if err != nil || len(preview.Result.Rows) != 3 || len(preview.Result.Schema) != 6 {
				t.Fatal("native typed preview", preview.Result, err)
			}
			for i, want := range []string{`"2.500"`, `"7.500"`, `"12.000"`} {
				if string(preview.Result.Rows[i][4]) != want || string(preview.Result.Rows[i][5]) != `"1"` {
					t.Fatal("exact aggregation", i, preview.Result.Rows[i])
				}
			}
			if f.attemptCount(t) != before+2 || f.f.model.requests.Load() != models {
				t.Fatal("unexpected source or model work")
			}
			for i, shape := range []string{"rows", "count", "calendar", "physical-filters"} {
				t.Run(shape, func(t *testing.T) {
					next := request
					next.NewBlock = "typed-" + shape
					next.Operation = "prepare:" + strconv.FormatInt(time.Now().Unix(), 10) + ":" + strings.Repeat(strconv.Itoa(i+2), 32)
					access := slices.Clone(scopes)
					for _, permission := range []string{"read", "write", "preview"} {
						access = append(access, "cw.block."+permission+":"+next.NewBlock)
					}
					actor := phase27Actor(t, f.f, author.User(), access)
					switch shape {
					case "rows":
						next.Intent.Fields = &reporting.AuthoringFieldSelection{Mode: "rows", Dimensions: []reporting.AuthoringGrouping{{Kind: "column", Field: domain.category}}}
						next.Intent.Mapping = reporting.AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"group_1"}}, Options: charts.DefaultOptions(), Table: &charts.TableOptions{PageSize: 20, Columns: []charts.TableColumnIntent{{Column: "group_1", Visible: true}}}}
					case "count", "physical-filters":
						next.Intent.Fields = &reporting.AuthoringFieldSelection{Mode: "aggregate", Measures: []reporting.AuthoringMeasureSelection{{Kind: "count"}}}
						next.Intent.Mapping = reporting.AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Value: "value_1"}, Options: charts.DefaultOptions()}
					case "calendar":
						next.Intent.Fields = &reporting.AuthoringFieldSelection{Mode: "aggregate", Dimensions: []reporting.AuthoringGrouping{{Kind: "column", Field: domain.occurred, Grain: "day", Calendar: "gregorian", Timezone: "America/Argentina/Buenos_Aires"}}, Measures: []reporting.AuthoringMeasureSelection{{Kind: "measure", Field: "mean"}, {Kind: "column", Field: domain.value, Aggregation: "maximum"}}}
						next.Intent.Mapping = reporting.AuthoringChartMapping{Kind: charts.ColumnChart, Bindings: charts.Bindings{Category: "group_1", Values: []string{"value_1", "value_2"}}, Options: charts.DefaultOptions()}
					}
					if shape == "physical-filters" {
						next.Intent.Filters = []reporting.AuthoringDatasetFilter{
							{Column: domain.category, Kind: "multi_select", Default: reporting.Value{Items: []string{"alpha"}}},
							{Column: domain.value, Kind: "range", Default: reporting.Value{Range: &reporting.ScalarRange{Start: "2", EndExclusive: "10"}}},
							{Column: domain.occurred, Kind: "range", Calendar: "gregorian", Timezone: "UTC", Default: reporting.Value{Range: &reporting.ScalarRange{Start: "2026-01-01T00:00:00", EndExclusive: "2026-02-01T00:00:00"}}},
						}
					}
					attempts := f.attemptCount(t)
					prepared, err := s.PrepareDatasetChart(ctx, actor, next)
					if err != nil || prepared.Status != "prepared" {
						t.Fatal("prepare", prepared, err)
					}
					created, err := s.CreatePreparedChart(ctx, actor, reporting.AuthoringCreatePreparedRequest{NewBlock: next.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
					if err != nil {
						t.Fatal("consume", err)
					}
					preview, err := blocks.Preview(ctx, actor, next.NewBlock, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision}, Outputs: []string{"chart"}})
					if err != nil || f.attemptCount(t) != attempts+2 || f.f.model.requests.Load() != models {
						t.Fatal("preview", err)
					}
					switch shape {
					case "rows":
						counts := map[string]int{}
						for _, row := range preview.Result.Rows {
							counts[string(row[0])]++
						}
						if len(preview.Result.Rows) != 3 || counts[`"alpha"`] != 2 || counts[`"beta"`] != 1 {
							t.Fatal("raw rows lost duplicates", counts)
						}
					case "physical-filters":
						if len(preview.Result.Rows) != 1 || string(preview.Result.Rows[0][0]) != `"2"` {
							t.Fatal("physical filter population", preview.Result.Rows)
						}
						if len(created.Block.Parameters) != 3 {
							t.Fatal("physical parameter retention")
						}
						for _, p := range created.Block.Parameters {
							if p.Column == nil || p.Dimension != nil {
								t.Fatal("invented reviewed dimension")
							}
						}
					case "count":
						if len(preview.Result.Rows) != 1 || string(preview.Result.Rows[0][0]) != `"3"` {
							t.Fatal("count", preview.Result.Rows)
						}
					case "calendar":
						if len(preview.Result.Rows) != 2 || string(preview.Result.Rows[0][0]) != `"2026-01-01 03:00:00+00"` || string(preview.Result.Rows[0][1]) != `"5.0000000000000000"` || string(preview.Result.Rows[0][2]) != `"7.500"` {
							t.Fatal("calendar boundary or multi-value aggregation", preview.Result.Rows)
						}
					}
				})
			}
		})
	}
}
