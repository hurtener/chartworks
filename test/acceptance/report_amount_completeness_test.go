package acceptance

import (
	"context"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reportingapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
)

// Uses actual generated/reviewed topic semantics and a proved query whose aliases
// deliberately suggest the opposite roles. Reporting review creates a new,
// explicitly distinct declaration rather than inheriting analytical authority.
func TestCapturedAmountCompletenessLifecycle(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t, true)
	ctx := t.Context()
	actor := func(scopes []string) identity.Envelope {
		t.Helper()
		claims := h.f.token.claims(h.queryActor.Tenant(), h.queryActor.User(), scopes)
		claims["session"] = h.queryActor.Session()
		e, err := h.f.token.verifier.Verify(ctx, h.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	author := actor(phase27Scopes(h.queryActor.Tenant()))
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	question := ""
	for _, c := range cases {
		if c.ID == "gross-en" {
			question = c.Question
		}
	}
	if question == "" {
		t.Fatal("missing recorded question")
	}
	metric := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
	selectSQL := "SELECT COUNT(order_id)-COUNT(misleading_net_total_usd) AS looks_like_gross,SUM(misleading_net_total_usd) AS looks_like_unknown FROM analytics.adv_orders WHERE status_code='P'"
	model.mode.Store(phase18RawResponse(t, selectSQL))
	plan, err := h.query.Plan(ctx, h.queryActor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{metric}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}})
	if err != nil {
		t.Fatal("plan proved origin", err)
	}
	original, err := h.query.Run(ctx, h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "captured-completeness-origin", Rows: 20, Bytes: 65536})
	if err != nil || len(original.AmountCompleteness) != 1 || original.AmountCompleteness[0].ValueColumn != 1 || original.AmountCompleteness[0].UnknownCountColumn != 0 {
		t.Fatal("prove output identity", err, original.AmountCompleteness)
	}
	evidence, err := h.query.CaptureDefinition(ctx, author, plan.QueryID)
	if err != nil {
		t.Fatal("private current capture evidence", err)
	}
	var parameters []reporting.Parameter
	for i, p := range evidence.Parameters {
		if p.Kind != "text" {
			t.Fatal("unexpected recorded parameter", i, p.Kind)
		}
		name := []string{"from", "until"}[i]
		parameters = append(parameters, reporting.Parameter{Name: name, Type: "datetime", Required: true, Default: &reporting.Value{Literal: p.Value}})
	}
	columns := []charts.Column{{ID: "counter", Name: "looks_like_gross", Type: evidence.Schema[0].Type, Role: "measure", Provenance: charts.Provenance{Version: 1}}, {ID: "known", Name: "looks_like_unknown", Type: evidence.Schema[1].Type, Role: "measure", Format: charts.Format{Currency: "USD", FractionDigits: 2}, Provenance: charts.Provenance{Version: 1}}}
	table := charts.Mapping{Version: charts.Version, Kind: charts.Table, Columns: columns, Bindings: charts.Bindings{Columns: []string{"counter", "known"}}, Options: charts.DefaultOptions()}
	count := charts.Mapping{Version: charts.Version, Kind: charts.KPI, Columns: columns[:1], Bindings: charts.Bindings{Value: "counter"}, Options: charts.DefaultOptions()}
	limits := config.DefaultReporting()
	blocks, err := reporting.New(h.f.db, h.topics, h.f.s, h.f.validator, h.f.executor, reporting.CaptureFromQueries(h.query), limits)
	if err != nil {
		t.Fatal(err)
	}
	captured, err := blocks.CaptureQuery(ctx, author, reporting.CaptureRequest{ID: "known-amount-capture", Query: plan.QueryID, Metadata: []reporting.Localized{{Locale: "en-US", Title: "Known paid amounts", Question: "What known amount and missing-amount count were observed?"}}, Parameters: parameters, Outputs: []reporting.Output{{ID: "amounts", Kind: "table", Mapping: &table}, {ID: "unknown-count", Kind: "kpi", Mapping: &count}}})
	if err != nil {
		t.Fatal("capture proposed declaration", err)
	}
	if len(captured.AmountCompleteness) != 1 || captured.AmountCompleteness[0].ValueColumn != 1 || captured.AmountCompleteness[0].UnknownCountColumn != 0 || len(captured.Outputs[0].AmountCompleteness) != 2 || captured.Outputs[1].AmountCompleteness[0].Role != "unknown_count" {
		t.Fatal("capture guessed aliases or discarded roles")
	}
	sqlView, err := blocks.SQL(ctx, author, captured.State.ID, reporting.Reference{Draft: true})
	if err != nil || sqlView.Definition == nil {
		t.Fatal(err)
	}
	definition := *sqlView.Definition
	for name, change := range map[string]func(*reporting.Definition){
		"ordinal": func(d *reporting.Definition) { d.AmountCompleteness[0].ValueColumn = 0 },
		"alias":   func(d *reporting.Definition) { d.AmountCompleteness[0].UnknownCountField.Name = "looks_like_unknown" },
		"omit":    func(d *reporting.Definition) { d.Outputs[0].AmountCompleteness = nil },
		"erase": func(d *reporting.Definition) {
			d.AmountCompleteness = nil
			for i := range d.Outputs {
				d.Outputs[i].AmountCompleteness = nil
			}
		},
		"count currency": func(d *reporting.Definition) { d.Outputs[1].Mapping.Columns[0].Format.Currency = "USD" },
	} {
		bad := phase27Copy(t, definition)
		change(&bad)
		if _, err := blocks.Edit(ctx, author, captured.State.ID, reporting.EditRequest{ExpectedVersion: captured.State.Version, Definition: bad}); err == nil {
			t.Fatal("accepted disclosure tamper", name)
		}
	}
	// Explicit SQL authoring changes the period and invalidates the query proof.
	// The new publication is reviewed-definition evidence only.
	definition.SQL = selectSQL + " AND ordered_at >= '2026-01-01' AND ordered_at < '2026-02-01'"
	definition.Parameters = nil
	amended, err := blocks.Edit(ctx, author, captured.State.ID, reporting.EditRequest{ExpectedVersion: captured.State.Version, Definition: definition})
	if err != nil {
		t.Fatal("review period amendment", err)
	}
	parameter := reporting.Parameter{Name: "period", Type: "relative_period", Required: true, Default: phase27Period("2026-01-01", "2026-02-01")}
	proposal, err := blocks.ProposeParameterization(ctx, author, captured.State.ID, reporting.ParameterizationProposalRequest{DefinitionDigest: amended.Digest, Column: []string{"ordered_at"}, Parameter: parameter})
	if err != nil {
		t.Fatal(err)
	}
	amended, err = blocks.Parameterize(ctx, author, captured.State.ID, reporting.ParameterizeRequest{ExpectedVersion: amended.State.Version, DefinitionDigest: amended.Digest, Column: []string{"ordered_at"}, Parameter: parameter, Note: "Review declared known amount and count roles for each period", ProposalDigest: proposal.ProposalDigest, OriginalQuestion: definition.Metadata[0].Question, QuestionDisposition: "preserved", TemplateDisposition: "not_applicable", ParaphraseDisposition: "preserved"})
	if err != nil {
		t.Fatal(err)
	}
	published, _ := phase27ValidatePublish(t, blocks, author, amended)
	runner, err := jobs.NewRequestRunner(h.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewRuns(blocks, h.f.db, runner, nil, "none", limits.Execution)
	if err != nil {
		t.Fatal(err)
	}
	execute := actor(phase28Scopes(author.Tenant()))
	documents, err := reporting.NewDocuments(h.f.db, blocks, reporting.DocumentsFromQueries(h.query), limits)
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, h.f.db, runs, reporting.DocumentsFromQueries(h.query), runner)
	if err != nil {
		t.Fatal(err)
	}
	delivery, err := reporting.NewDelivery(blocks, runs, documents, compositions, h.f.db, limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	descriptor, err := blocks.PrepareQueryVariant(ctx, author, captured.State.ID, reporting.QueryVariantRequest{Revision: published.PublishedRevision, Outputs: []string{"amounts"}})
	if err != nil {
		t.Fatal(err)
	}
	reportDefinition := phase29Text("Known amounts with required disclosure")
	reportDefinition.Filters = []reporting.ReportFilter{{Label: "Period", Parameter: parameter}}
	reportDefinition.Widgets = append(reportDefinition.Widgets, reporting.Widget{ID: "known", Kind: "query", Grid: reporting.GridCell{Row: 1, Width: 12, Height: 2}, Query: &descriptor.Query, Bindings: []reporting.FilterBinding{{Filter: "period", Parameter: "period"}}})
	reportAuthor := actor(phase29AuthorScopes(author.Tenant()))
	reportActor := actor(phase29RuntimeScopes(author.Tenant()))
	report, err := documents.Create(ctx, reportAuthor, "report", "amount-disclosure-report", reportDefinition)
	if err != nil {
		t.Fatal(err)
	}
	report = phase29Publish(t, documents, reportAuthor, report)
	server := httptest.NewServer(reportingapi.DeliveryHandler(h.f.token.verifier, delivery, true, http.NotFoundHandler()))
	defer server.Close()
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		claims := h.f.token.claims(reportActor.Tenant(), reportActor.User(), phase29RuntimeScopes(author.Tenant()))
		claims["session"] = reportActor.Session()
		return h.f.token.sign(t, claims, nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := rendering.NewManaged(delivery, h.f.db, rendering.LocalProcessor{MaxBytes: 2 << 20}, 2<<20, phase32Options())
	if err != nil {
		t.Fatal(err)
	}
	renderActor := actor([]string{"reporting.read", "reporting.export", "cw.run.read:*", "cw.run.export:*", "cw.execution_context.use:" + definition.Context})
	before := model.requests.Load()
	for _, tc := range []struct{ key, start, end, status, count string }{{"january", "2026-01-01", "2026-02-01", "complete", "0"}, {"june", "2026-06-01", "2026-07-01", "incomplete", "1"}} {
		request := reporting.RunRequest{Key: "completeness-" + tc.key, Arguments: []reporting.Argument{{Name: "period", Value: *phase27Period(tc.start, tc.end)}}}
		admitted, err := runs.Admit(ctx, execute, captured.State.ID, request)
		if err != nil {
			t.Fatal("admit", tc.key, err)
		}
		done, err := runs.Run(ctx, execute, admitted.ID, false)
		if err != nil || done.State != "succeeded" {
			t.Fatal("execute", tc.key, err, done.State, done.Code)
		}
		out, err := runs.Output(ctx, execute, done.ID, "amounts")
		if err != nil {
			t.Fatal(err)
		}
		if tc.key == "june" && (out.Chart == nil || len(out.Chart.Rows) != 1 || !out.Chart.Rows[0][1].Null) {
			t.Fatal("missing known amount was coerced to zero")
		}
		if len(out.AmountCompleteness) != 2 {
			t.Fatal("retained disclosure absent")
		}
		for _, item := range out.AmountCompleteness {
			if item.Evidence != "reviewed_definition" || item.DefinitionDigest != amended.Digest || item.Result.Status != tc.status || len(item.Result.Rows) != 1 || item.Result.Rows[0].UnknownCount != tc.count {
				t.Fatal("incorrect reviewed disclosure", tc.key, item)
			}
		}
		countOut, err := runs.Output(ctx, execute, done.ID, "unknown-count")
		if err != nil || countOut.AmountCompleteness[0].Role != "unknown_count" || countOut.AmountCompleteness[0].Unit != "count" {
			t.Fatal("count KPI lost role", err)
		}

		view, err := client.ViewReporting(ctx, sdk.ReportingViewRequest{Kind: "block", Run: done.ID, Output: "amounts", Limit: 1})
		if err != nil || view.Output == nil || len(view.Output.AmountCompleteness) != 2 || view.Output.AmountCompleteness[0].RowsScope != "visible_source_rows" || view.Output.AmountCompleteness[0].Result.Rows[0].UnknownCount != tc.count || len(view.Output.Table.RowIndices) != 1 {
			t.Fatal("HTTP/SDK retained disclosure", err)
		}
		for _, format := range []string{"json", "csv", "html", "svg", "png"} {
			rendition, err := renderer.Generate(ctx, renderActor, rendering.Request{View: reporting.DeliveryViewRequest{Kind: "block", Run: done.ID, Output: "amounts", Limit: 1}, Format: format, Theme: "light", Width: 800, Height: 420})
			if err != nil || rendition.ID == "" {
				t.Fatal("retained rendition", format, err)
			}
			if format == "png" {
				if _, err := sdk.ReportingRenditionBytes(ctx, rendition, 2<<20); err != nil {
					t.Fatal("PNG disclosure bytes", err)
				}
				if len(rendition.Projection.AmountCoverage) == 0 || rendition.Projection.AmountCoverage[0].Status != tc.status || rendition.Projection.AmountCoverage[0].Evidence != "reviewed_definition" {
					t.Fatal("PNG disclosure identity lost")
				}
			}
			if (format == "html" || format == "svg") && (!strings.Contains(rendition.Content, "reviewed definition") || !strings.Contains(rendition.Content, "Unknown amount count (displayed rows): "+tc.count)) {
				t.Fatal("static export dropped disclosure", format)
			}
		}
		composed, err := compositions.Admit(ctx, reportActor, "report", report.ID, reporting.CompositionRequest{Key: "amount-report-" + tc.key, Pages: []reporting.PageInput{{Page: "main", Filters: request.Arguments}}})
		if err != nil {
			t.Fatal("report filter admission", err)
		}
		completed, err := compositions.Run(ctx, reportActor, composed.ID, false)
		if err != nil || completed.State != "completed" || !completed.Complete {
			t.Fatal("report execution", err, completed.State)
		}
		shown, err := client.ViewReporting(ctx, sdk.ReportingViewRequest{Kind: "report", Run: composed.ID, Page: "main", Widget: "known", Output: "amounts", Limit: 1})
		if err != nil || shown.Output == nil || shown.Output.AmountCompleteness[0].Result.Status != tc.status || shown.Output.AmountCompleteness[0].Result.Rows[0].UnknownCount != tc.count {
			t.Fatal("composed report disclosure", err)
		}
		reportRenderActor := actor(append(phase29RuntimeScopes(author.Tenant()), "reporting.export", "cw.run.export:*"))
		for _, format := range []string{"html", "svg"} {
			whole, err := renderer.Export(ctx, reportRenderActor, rendering.Request{View: reporting.DeliveryViewRequest{Kind: "report", Run: composed.ID, Page: "main", Limit: 1}, Format: format, Theme: "light", Width: 800, Height: 420, Full: true})
			if err != nil || !strings.Contains(whole.Content, "reviewed definition") || !strings.Contains(whole.Content, "incomplete") && tc.status == "incomplete" {
				t.Fatal("composed disclosure export", format, err)
			}
		}
		again, err := runs.Admit(ctx, execute, captured.State.ID, request)
		if err != nil || again.ID != done.ID {
			t.Fatal("replay", err)
		}
	}
	if model.requests.Load() != before {
		t.Fatal("reporting regenerated query/model evidence")
	}
	// Raw-report reach and model-evidence policy are independent. The model is
	// genuinely available in both cases; sensitive companions cannot hitchhike
	// through the new disclosure sidecar into a narrative prompt.
	privateSQL, err := blocks.SQL(ctx, author, captured.State.ID, reporting.Reference{Revision: published.PublishedRevision})
	if err != nil || privateSQL.Definition == nil {
		t.Fatal(err)
	}

	// A separate explicit operator publication narrows this topic to public
	// fixture columns. The sensitive private-note field stays outside this
	// scope; block-level labels never declassify it.
	privacyPack := phase27Copy(t, current.Pack)
	privacyPack.Version = "report-privacy-reviewed"
	removedPrivateColumn := ""
	for i := range privacyPack.Datasets {
		if privacyPack.Datasets[i].ID == datasets["orders"] {
			columns := privacyPack.Datasets[i].Columns[:0]
			for _, c := range privacyPack.Datasets[i].Columns {
				if c.SourceName == "private_note" {
					removedPrivateColumn = c.ID
					continue
				}
				c.Sensitivity = semantics.LiteralNonSensitive
				columns = append(columns, c)
			}
			privacyPack.Datasets[i].Columns = columns
		}
	}
	remainingGaps := privacyPack.Unresolved[:0]
	for _, gap := range privacyPack.Unresolved {
		if gap.Dataset != datasets["orders"] || gap.Column != removedPrivateColumn {
			remainingGaps = append(remainingGaps, gap)
		}
	}
	privacyPack.Unresolved = remainingGaps
	if _, err := semantics.Compile(privacyPack); err != nil {
		t.Fatal("public-only fixture scope", err)
	}
	oldPublication, err := h.client.PublishedTopic(ctx, current.Pack.Topic)
	if err != nil {
		t.Fatal(err)
	}
	privacyDraft, err := h.client.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: current.Metadata.Revision, Pack: privacyPack, Change: "Synthetic operator narrows the orders topic to reviewed public fixture fields, excluding private_note"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := h.client.ReviewTopic(ctx, current.Pack.Topic, sdk.TopicReviewRequest{DraftRevision: privacyDraft.Metadata.Revision, Digest: privacyDraft.Metadata.Digest, Decision: "approve", Note: "Reviewed public-only fixture scope; sensitive private_note remains excluded"})
	if err != nil {
		t.Fatal(err)
	}
	privacyPublication, err := h.client.PublishTopic(ctx, current.Pack.Topic, sdk.PublishTopicRequest{Review: review.ID, Expected: oldPublication.State.Revision})
	if err != nil {
		t.Fatal(err)
	}
	for i := range privateSQL.Definition.Topics {
		if privateSQL.Definition.Topics[i].Topic == current.Pack.Topic {
			privateSQL.Definition.Topics[i].Version = privacyPublication.Definition.Version
			privateSQL.Definition.Topics[i].Digest = privacyPublication.Digest
		}
	}
	narrativeModel := newGatewayFixture(t, nil)
	narrativeModel.mode.Store(phase28Chat(t, narrativeModel.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"value","evidence":["e1"]}]}`))
	narrativeRuns, err := reporting.NewRuns(blocks, h.f.db, runner, narrativeModel.engine, "policy-v1", limits.Execution)
	if err != nil {
		t.Fatal(err)
	}
	for _, policy := range []string{"allowed", "sensitive", "redacted"} {
		t.Run("companion_egress_"+policy, func(t *testing.T) {
			d := phase27Copy(t, *privateSQL.Definition)
			d.ResultPolicy = []reporting.ResultFieldPolicy{{Field: "looks_like_gross", Sensitivity: semantics.LiteralNonSensitive}}
			if policy == "sensitive" {
				d.ResultPolicy[0].Sensitivity = semantics.LiteralSensitive
			}
			if policy == "redacted" {
				d.ResultPolicy[0].Redacted = true
			}
			d.Outputs = append(d.Outputs, reporting.Output{ID: "count-narrative", Kind: "narrative", AmountCompleteness: []reporting.AmountOutputBinding{{Declaration: d.AmountCompleteness[0].ID, Role: "unknown_count"}}, Intent: &reporting.OutputIntent{Enabled: true, DefaultSelected: true, DisplayOrder: 2, Metadata: []reporting.OutputMetadata{{Locale: "en-US", DisplayName: "Unknown amount count"}}}, Narrative: &reporting.Narrative{PolicyVersion: reporting.NarrativePolicyVersion, Type: "summary", Instructions: "evidence_only", Fields: []string{"looks_like_gross"}, Reduction: "first_rows", MaxRows: 10, MaxBytes: 4096, MaxCharacters: 1000, MaxCalls: 1, MaxTokens: 8192, MaxClaims: 1, TimeoutMillis: 10000, PromptVersion: "summary-v1", ModelVersion: "policy-v1", SchemaVersion: "grounded-narrative-v1", Locale: "en-US", Tone: "neutral", RequireEvidence: true, RequireCaveats: true}})
			created, err := blocks.Create(ctx, author, reporting.CreateRequest{ID: "companion-policy-" + policy, Definition: d})
			if err != nil {
				t.Fatal(err)
			}
			phase27ValidatePublish(t, blocks, author, created)
			calls := narrativeModel.requests.Load()
			accepted, err := narrativeRuns.Admit(ctx, execute, created.State.ID, reporting.RunRequest{Key: "companion-egress-" + policy, Narrative: true, PartialPolicy: "allow_partial", Arguments: []reporting.Argument{{Name: "period", Value: *phase27Period("2026-06-01", "2026-07-01")}}})
			if err != nil {
				t.Fatal(err)
			}
			done, err := narrativeRuns.Run(ctx, execute, accepted.ID, false)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := narrativeRuns.Output(ctx, execute, done.ID, "amounts")
			if err != nil || raw.Chart == nil || raw.Chart.Rows[0][0].Value != "1" || raw.AmountCompleteness[0].Result.Rows[0].UnknownCount != "1" {
				t.Fatal("raw-report authority was confused with model egress", err)
			}
			out, err := narrativeRuns.Output(ctx, execute, done.ID, "count-narrative")
			if err != nil {
				t.Fatal(err)
			}
			if policy == "allowed" {
				if done.State != "succeeded" || out.Narrative == nil || len(out.Narrative.Evidence) != 1 || out.Narrative.Evidence[0].Field != "looks_like_gross" || narrativeModel.requests.Load() != calls+1 {
					t.Fatal("positive narrative control failed", done.State, out.Code, "field_policy", raw.ResultPolicy, "evidence_policy", out.EvidencePolicy)
				}
			} else if done.State != "partial" || out.Code != "narrative_evidence_unavailable" || out.Narrative != nil || len(out.AmountCompleteness) != 0 || narrativeModel.requests.Load() != calls || done.ReservedCalls != 0 {
				t.Fatal("sensitive companion reached model", policy, done.State, out.Code)
			}
			narrativeModel.mu.Lock()
			body := strings.Join(narrativeModel.requestBodies, "\n")
			narrativeModel.mu.Unlock()
			if strings.Contains(body, "amount_completeness") || strings.Contains(body, "reviewed-amount-completeness-v1") {
				t.Fatal("retained disclosure entered model payload")
			}
		})
	}
}
