package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func TestCapturedQueryVariantReportLifecycle(t *testing.T) {
	f := newPhase27CatalogFixture(t)
	ctx := context.Background()
	e := f.actor
	query, topics := newPhase18Service(t, f.phase17Fixture)
	limits := config.DefaultReporting()
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), limits)
	if err != nil {
		t.Fatal(err)
	}
	originalSQL := "SELECT sum(amount) AS amount FROM analytics.sales"
	base := phase27Definition(t, f.phase17Fixture, e, originalSQL)
	base.Outputs = base.Outputs[:1]
	f.model.embeddingMode.Store("fixed")
	f.model.rerankMode.Store("fixed")
	f.model.mode.Store(phase18RawResponse(t, originalSQL))
	planned, err := query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(f.phase17Fixture, nlq.LanguageEnglish, f.pack.Topic), Operation: "variant-origin"})
	if err != nil {
		t.Fatal("plan origin", err)
	}
	capture := reporting.CaptureRequest{ID: "captured-variant", Query: planned.QueryID, Metadata: base.Metadata, Outputs: base.Outputs}
	if _, err = blocks.CaptureQuery(ctx, e, capture); err == nil {
		t.Fatal("planned query captured")
	}
	result, err := query.Run(ctx, e, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "variant-origin", Rows: 10, Bytes: 65536})
	if err != nil || result.Execution.Result == nil {
		t.Fatal("run origin", err)
	}
	wrong, err := f.f.token.verifier.Verify(ctx, phase27Token(t, f.phase17Fixture, e.User(), "foreign-session", phase27Scopes(e.Tenant())), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocks.CaptureQuery(ctx, wrong, capture); !errors.Is(err, nlqexec.ErrForeignSession) {
		t.Fatal("foreign capture", err)
	}
	noSQL := phase27Actor(t, f.phase17Fixture, e.User(), slices.DeleteFunc(phase27Scopes(e.Tenant()), func(s string) bool { return s == "reporting.sql.read" }))
	if _, err = blocks.CaptureQuery(ctx, noSQL, capture); !errors.Is(err, nlqexec.ErrInspectionRequired) {
		t.Fatal("capture without SQL authority", err)
	}
	captured, err := blocks.CaptureQuery(ctx, e, capture)
	if err != nil {
		t.Fatal("capture", err)
	}
	if _, err = blocks.PrepareQueryVariant(ctx, e, capture.ID, reporting.QueryVariantRequest{Revision: 1}); err == nil {
		t.Fatal("unpublished variant prepared")
	}
	// A separately authored reporting amendment establishes the filter before
	// parameterization. The query and capture revision remain immutable.
	authored := phase27Copy(t, base)
	authored.SQL = "SELECT sum(amount) AS amount FROM analytics.sales WHERE created_at >= '2026-01-01' AND created_at < '2026-02-01'"
	captured, err = blocks.Edit(ctx, e, capture.ID, reporting.EditRequest{ExpectedVersion: captured.State.Version, Definition: authored})
	if err != nil {
		t.Fatal("explicit period authoring", err)
	}
	parameter := reporting.Parameter{Name: "period", Type: "relative_period", Required: true, Default: phase27Period("2026-01-01", "2026-02-01")}
	proposal, err := blocks.ProposeParameterization(ctx, e, capture.ID, reporting.ParameterizationProposalRequest{DefinitionDigest: captured.Digest, Column: []string{"created_at"}, Parameter: parameter})
	if err != nil {
		t.Fatal("parameter proposal", err)
	}
	amended, err := blocks.Parameterize(ctx, e, capture.ID, reporting.ParameterizeRequest{ExpectedVersion: captured.State.Version, DefinitionDigest: captured.Digest, Column: []string{"created_at"}, Parameter: parameter, Note: "Review selected date range slots", ProposalDigest: proposal.ProposalDigest, OriginalQuestion: base.Metadata[0].Question, QuestionDisposition: "preserved", TemplateDisposition: "not_applicable", ParaphraseDisposition: "preserved"})
	if err != nil {
		t.Fatal("parameterize", err)
	}
	state, _ := phase27ValidatePublish(t, blocks, e, amended)
	runs := phase28RunService(t, f.phase17Fixture, blocks, f.f.db, nil, limits.Execution)
	documents, err := reporting.NewDocuments(f.f.db, blocks, reporting.DocumentsFromQueries(query), limits)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, f.f.db, runs, reporting.DocumentsFromQueries(query), runner)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(blocks, runs, documents, compositions, f.f.db, limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(reportingapi.DeliveryHandler(f.f.token.verifier, delivery, true, http.NotFoundHandler()))
	defer server.Close()
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		return phase27Token(t, f.phase17Fixture, e.User(), e.Session(), phase27Scopes(e.Tenant())), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := client.PrepareCapturedQueryVariant(ctx, sdk.ReportingQueryVariantRequest{Block: capture.ID, Revision: state.PublishedRevision, Outputs: []string{"table-main"}})
	if err != nil {
		_, domainErr := delivery.PrepareQueryVariant(ctx, e, reporting.QueryVariantRequest{Block: capture.ID, Revision: state.PublishedRevision, Outputs: []string{"table-main"}})
		t.Fatalf("prepare transport: %#v; domain=%v", err, domainErr)
	}
	readDescriptor, err := blocks.PrepareQueryVariant(ctx, noSQL, capture.ID, reporting.QueryVariantRequest{Revision: state.PublishedRevision, Outputs: []string{"table-main"}})
	if err != nil || readDescriptor.Query.Variant.Digest != descriptor.Query.Variant.Digest {
		t.Fatal("metadata variant required SQL inspection", err)
	}
	protected, err := f.f.db.ReadBlock(ctx, noSQL, capture.ID, reporting.Reference{Revision: state.PublishedRevision}, reporting.Read)
	if err != nil || protected.Revision.Definition.SQL != "" || protected.Revision.Provenance.Query != "" || protected.Revision.Provenance.OriginalQuestion != "" || protected.Revision.Provenance.CaptureDigest == "" {
		t.Fatal("capture metadata disclosed private provenance", err)
	}
	if _, err = client.PrepareCapturedQueryVariant(ctx, sdk.ReportingQueryVariantRequest{Block: capture.ID, Revision: state.PublishedRevision, Outputs: []string{}}); err == nil {
		t.Fatal("explicit empty outputs became defaults")
	}
	raw, _ := json.Marshal(descriptor)
	if strings.Contains(string(raw), originalSQL) || strings.Contains(string(raw), planned.QueryID) || len(descriptor.Parameters) != 1 {
		t.Fatal("private provenance or missing parameter catalog")
	}
	author := phase27Actor(t, f.phase17Fixture, e.User(), phase29AuthorScopes(e.Tenant()))
	execute := phase27Actor(t, f.phase17Fixture, e.User(), phase29RuntimeScopes(e.Tenant()))
	definition := phase29Text("Captured query variant")
	definition.Filters = []reporting.ReportFilter{{Label: "Period", Parameter: parameter}}
	widget := reporting.Widget{ID: "variant", Kind: "query", Grid: reporting.GridCell{Row: 1, Width: 12, Height: 2}, Query: &descriptor.Query, Bindings: []reporting.FilterBinding{{Filter: "period", Parameter: "period"}}}
	definition.Widgets = append(definition.Widgets, widget)
	for _, mutate := range []func(*reporting.DocumentDefinition){
		func(d *reporting.DocumentDefinition) { d.Widgets[1].Query.Variant.Digest = strings.Repeat("0", 64) },
		func(d *reporting.DocumentDefinition) {
			d.Widgets[1].Query.Variant.CaptureDigest = strings.Repeat("0", 64)
		},
		func(d *reporting.DocumentDefinition) { d.Widgets[1].Query.Variant.Block = "foreign-block" },
		func(d *reporting.DocumentDefinition) {
			d.Filters[0].Parameter = reporting.Parameter{Name: "period", Type: "integer", Default: &reporting.Value{Literal: "1"}}
		},
	} {
		bad := phase27Copy(t, definition)
		mutate(&bad)
		if _, err = documents.Create(ctx, author, "report", "invalid-variant-report", bad); err == nil {
			t.Fatal("invalid variant admitted")
		}
	}
	doc, err := documents.Create(ctx, author, "report", "variant-report", definition)
	if err != nil {
		t.Fatal("create report", err)
	}
	doc = phase29Publish(t, documents, author, doc)
	description, err := delivery.Describe(ctx, execute, reporting.DeliveryDescribeRequest{Target: reporting.DeliveryTarget{Kind: "report", ID: doc.ID, Revision: doc.PublishedRevision}, Locale: "en-US"})
	if err != nil || description.Dynamic {
		t.Fatal("variant described as dynamic model query", err)
	}
	before := f.model.requests.Load()
	for i, period := range []*reporting.Value{phase27Period("2026-01-01", "2026-02-01"), phase27Period("2026-02-01", "2026-03-01")} {
		key := []string{"january", "february"}[i]
		admitted, err := compositions.Admit(ctx, execute, "report", doc.ID, reporting.CompositionRequest{Key: key, Pages: []reporting.PageInput{{Page: "main", Filters: []reporting.Argument{{Name: "period", Value: *period}}}}})
		if err != nil {
			t.Fatal("admit", err)
		}
		completed, err := compositions.Run(ctx, execute, admitted.ID, false)
		if err != nil || !completed.Complete || completed.State != "completed" {
			t.Fatal("run variant", completed, err)
		}
		record, err := f.f.db.ReadComposition(ctx, execute, admitted.ID)
		if err != nil || len(record.Manifest.Groups) != 1 || record.Manifest.Groups[0].Variant == nil || record.Manifest.Groups[0].Kind != "block" {
			t.Fatal("variant execution custody", err)
		}
		payload, err := compositions.Widget(ctx, execute, admitted.ID, "main", "variant")
		if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].Chart == nil {
			t.Fatal("variant result", err)
		}
		count := len(payload.Outputs[0].Chart.Rows)
		if count != 1 || len(payload.Outputs[0].Chart.Rows[0]) != 1 || (i == 0 && payload.Outputs[0].Chart.Rows[0][0].Value != "9007199254740998.625") || (i == 1 && !payload.Outputs[0].Chart.Rows[0][0].Null) {
			t.Fatal("period did not change result", i, count)
		}
		replay, err := compositions.Admit(ctx, execute, "report", doc.ID, reporting.CompositionRequest{Key: key, Pages: []reporting.PageInput{{Page: "main", Filters: []reporting.Argument{{Name: "period", Value: *period}}}}})
		if err != nil || replay.ID != admitted.ID {
			t.Fatal("variant idempotency", err)
		}
	}
	if f.model.requests.Load() != before {
		t.Fatal("frozen variant called model")
	}
	denied := phase27Actor(t, f.phase17Fixture, e.User(), slices.DeleteFunc(phase29RuntimeScopes(e.Tenant()), func(s string) bool { return s == "cw.block.execute:*" }))
	deniedRun, deniedErr := compositions.Admit(ctx, denied, "report", doc.ID, reporting.CompositionRequest{Key: "denied"})
	if deniedErr == nil && deniedRun.QueryGroups != 0 {
		t.Fatal("variant inherited query authority")
	}
	source := f.pack.Datasets[0].Source
	if _, err = f.f.s.Rotate(ctx, f.f.e, source.Source, source.SourceRevision); err != nil {
		t.Fatal(err)
	}
	stale, staleErr := compositions.Admit(ctx, execute, "report", doc.ID, reporting.CompositionRequest{Key: "source-drift"})
	if staleErr == nil && stale.QueryGroups != 0 {
		t.Fatal("source drift accepted by variant")
	}

	unchanged, err := blocks.SQL(ctx, e, capture.ID, reporting.Reference{Revision: 1})
	if err != nil || unchanged.SQL != originalSQL {
		t.Fatal("original capture was rewritten", err)
	}
}
