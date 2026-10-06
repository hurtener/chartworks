package acceptance

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

func TestReportAppEffectDependencyDiscovery(t *testing.T) {
	f, s, author, prepare, _, _, _ := reportDatasetFixture(t, "effect-chart")
	ctx := t.Context()
	actor := func(tenant, user, session string, scopes []string) identity.Envelope {
		t.Helper()
		claims := f.f.f.token.claims(tenant, user, scopes)
		claims["session"] = session
		e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	scoped := func(scopes []string) identity.Envelope {
		return actor(author.Tenant(), author.User(), author.Session(), scopes)
	}
	prepared, err := s.PrepareDatasetChart(ctx, author, prepare)
	if err != nil {
		t.Fatal(err)
	}
	chart, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: prepare.NewBlock, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	seed := []string{"reporting.discover", "reporting.preview", "cw.block.read:" + prepare.NewBlock, "cw.block.preview:" + prepare.NewBlock}
	request := reporting.EffectDependencyRequest{Operation: "block_validate", ID: prepare.NewBlock, Revision: 1}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	metadata, err := s.EffectDependencies(ctx, scoped(seed), request)
	if err != nil {
		t.Fatal(err)
	}
	projected := func(m reporting.EffectDependencyManifest) []string {
		t.Helper()
		scopes := slices.Clone(m.Actions)
		for _, r := range m.References {
			scopes = append(scopes, "cw."+r.Kind+"."+r.Permission+":"+r.ID)
		}
		if m.Operation == "view" {
			scopes = append(scopes, "cw.run.read:"+m.Run)
		}
		slices.Sort(scopes)
		return slices.Compact(scopes)
	}
	validate := reporting.AuthoringBlockValidateRequest{Block: prepare.NewBlock, ExpectedVersion: chart.Block.State.Version, Revision: 1, Digest: chart.Block.Digest, Arguments: []reporting.Argument{}, Resolution: reporting.Resolution{At: time.Now().UTC(), Timezone: "UTC"}}
	if _, err := s.ValidateBlock(ctx, scoped(seed), validate); err == nil {
		t.Fatal("metadata seed validated source")
	}
	scopes := projected(metadata)
	for _, missing := range []string{"sources.query", "cw.source.query:" + chart.Block.Source, "cw.execution_context.use:" + chart.Block.Context, "cw.block.write:" + prepare.NewBlock} {
		denied := slices.DeleteFunc(slices.Clone(scopes), func(v string) bool { return v == missing })
		if _, err := s.ValidateBlock(ctx, scoped(denied), validate); err == nil {
			t.Fatal("missing validation requirement admitted", missing)
		}
	}
	if f.attemptCount(t) != before {
		t.Fatal("denial/discovery executed source")
	}
	if _, err := s.ValidateBlock(ctx, scoped(scopes), validate); err != nil {
		t.Fatal("projected validation", err)
	}
	if f.attemptCount(t) != before+1 {
		t.Fatal("validation did not perform exactly one read")
	}
	const report = "effect-report"
	createScopes := []string{"reporting.read", "reporting.write", "reporting.preview", "cw.tenant.write:" + author.Tenant(), "cw.report.write:" + report, "cw.report.read:" + report, "cw.report.preview:" + report}
	createScopes = append(createScopes, scopes...)
	slices.Sort(createScopes)
	createScopes = slices.Compact(createScopes)
	doc := phase29Text("Retained effect report")
	doc.SchemaVersion = reporting.PagedDocumentVersion
	doc.Widgets = nil
	doc.ReportPages = []reporting.ReportPage{{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{{ID: "amount", Kind: "block", Grid: reporting.GridCell{Width: 6, Height: 3}, Block: &reporting.BlockWidget{Block: prepare.NewBlock, Revision: 1, Digest: chart.Block.Digest, Policy: "private_preview", Outputs: []string{"chart"}}}}}, {ID: "notes", Title: "Notes", Widgets: []reporting.Widget{{ID: "note", Kind: "text", Grid: reporting.GridCell{Width: 12, Height: 1}, Text: &reporting.TextWidget{Format: "plain", Text: "Retained notes"}}}}}
	if _, err := s.Create(ctx, scoped(createScopes), reporting.AuthoringCreateRequest{ID: report, Definition: doc}); err != nil {
		t.Fatal("report setup", err)
	}
	seed = []string{"reporting.discover", "reporting.preview", "cw.report.read:" + report, "cw.report.preview:" + report}
	request = reporting.EffectDependencyRequest{Operation: "preview", ID: report, Revision: 1}
	metadata, err = s.EffectDependencies(ctx, scoped(seed), request)
	if err != nil {
		t.Fatal(err)
	}
	scopes = projected(metadata)
	run, err := s.Preview(ctx, scoped(scopes), reporting.AuthoringPreviewRequest{Report: report, Revision: 1, Key: "effect-preview", Resolution: validate.Resolution, Pages: []reporting.PageInput{}})
	if err != nil || !run.Private {
		t.Fatal("projected preview", run, err)
	}
	if f.attemptCount(t) != before+1 {
		t.Fatal("preview admission executed source")
	}
	seed = []string{"reporting.discover", "cw.run.read:" + run.ID}
	request = reporting.EffectDependencyRequest{Operation: "execute", ID: run.ID}
	metadata, err = s.EffectDependencies(ctx, scoped(seed), request)
	if err != nil || metadata.ID != report || metadata.Run != run.ID {
		t.Fatal(metadata, err)
	}
	scopes = projected(metadata)
	complete, err := s.Execute(ctx, scoped(scopes), reporting.AuthoringExecuteRequest{Run: run.ID})
	if err != nil || !complete.Complete {
		t.Fatal("projected execute", complete, err)
	}
	if f.attemptCount(t) != before+2 {
		t.Fatal("execution did not perform exactly one read")
	}
	request.Operation = "view"
	metadata, err = s.EffectDependencies(ctx, scoped(seed), request)
	if err != nil {
		t.Fatal(err)
	}
	scopes = projected(metadata)
	if slices.Contains(scopes, "sources.query") || slices.Contains(scopes, "reporting.execute") || slices.Contains(scopes, "cw.source.query:"+chart.Block.Source) {
		t.Fatal("retained read gained execution")
	}
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, f.limits.Viewer)
	if err != nil {
		t.Fatal(err)
	}
	output, err := delivery.View(ctx, scoped(scopes), reporting.DeliveryViewRequest{Kind: "report", Run: run.ID, Page: "analysis", Widget: "amount", Output: "chart", Limit: 100})
	if err != nil || output.Output == nil || output.Output.Chart == nil || len(output.Output.Chart.Points) != 1 || output.Output.Chart.Points[0].Value.Exact == "" {
		t.Fatal("projected retained values", output, err)
	}
	if _, err := delivery.View(ctx, scoped(seed), reporting.DeliveryViewRequest{Kind: "report", Run: run.ID}); err == nil {
		t.Fatal("discovery seed read artifact")
	}
	for _, who := range [][3]string{{"foreign", author.User(), author.Session()}, {author.Tenant(), "other", author.Session()}, {author.Tenant(), author.User(), "other-login"}} {
		if got, err := s.EffectDependencies(ctx, actor(who[0], who[1], who[2], seed), request); err == nil || got.Version != "" {
			t.Fatal("foreign custody metadata", got, err)
		}
	}
	for _, bad := range []reporting.EffectDependencyRequest{{Operation: "view", ID: "guessed"}, {Operation: "execute", ID: run.ID, Revision: 1}, {Operation: "publish", ID: run.ID}} {
		if _, err := s.EffectDependencies(ctx, scoped(seed), bad); err == nil {
			t.Fatal("invalid run selection")
		}
	}
	registry, err := reportingapi.DependencyRegistry()
	if err != nil {
		t.Fatal(err)
	}
	handler := assertRegisteredWireSchemas(t, registry, reportingapi.DependencyHandler(f.f.f.token.verifier, s, http.NotFoundHandler()))
	claims := f.f.f.token.claims(author.Tenant(), author.User(), seed)
	claims["session"] = author.Session()
	bearer := f.f.f.token.sign(t, claims, nil)
	body, _ := json.Marshal(request)
	response := callProtected(t, handler, "POST", reportingapi.EffectDependencyDiscoveryPath, bearer, string(body), map[string]string{"Content-Type": "application/json"})
	if response.Code != 200 {
		t.Fatal(response.Code, response.Body.String())
	}
	for _, field := range []string{"references", "scopes", "session"} {
		bad := strings.TrimSuffix(string(body), "}") + `,"` + field + `":[]}`
		if got := callProtected(t, handler, "POST", reportingapi.EffectDependencyDiscoveryPath, bearer, bad, map[string]string{"Content-Type": "application/json"}); got.Code != 400 {
			t.Fatal("caller supplied metadata accepted", field, got.Code)
		}
	}
	if f.attemptCount(t) != before+2 || f.f.model.requests.Load() != models {
		t.Fatal("retained read/discovery reran source/model")
	}
	t.Run("published_consumer_and_original_run_dependencies", func(t *testing.T) {
		publicationScopes := append(slices.Clone(createScopes), "reporting.publish", "cw.block.publish:"+prepare.NewBlock, "cw.report.publish:"+report)
		publisher := scoped(publicationScopes)
		inspected, err := s.InspectLifecycle(ctx, publisher, reporting.AuthoringLifecycleRequest{Report: report, Revision: 1})
		if err != nil {
			t.Fatal(err)
		}
		b := inspected.Blocks[0].Block
		if _, err := s.PublishBlock(ctx, publisher, reporting.AuthoringBlockPublishRequest{Block: prepare.NewBlock, ExpectedVersion: b.State.Version, Revision: 1, Digest: b.Digest, Evidence: b.Evidence.ID}); err != nil {
			t.Fatal(err)
		}
		state, err := s.RebindPublished(ctx, publisher, reporting.AuthoringRebindPublishedRequest{Report: report, ExpectedVersion: inspected.Report.State.Version, Revision: 1, Digest: inspected.Report.Digest, Widgets: []reporting.AuthoringPublishedWidget{{Widget: "amount", Block: prepare.NewBlock, Revision: 1, Digest: b.Digest}}})
		if err != nil {
			t.Fatal(err)
		}
		state, err = s.TransitionReport(ctx, publisher, reporting.AuthoringReportTransitionRequest{Report: report, ExpectedVersion: state.Version, Revision: state.DraftRevision, Operation: "review"})
		if err != nil {
			t.Fatal(err)
		}
		state, err = s.TransitionReport(ctx, publisher, reporting.AuthoringReportTransitionRequest{Report: report, ExpectedVersion: state.Version, Revision: state.ReviewRevision, Operation: "publish"})
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"report", "block"} {
			id, revision := report, state.PublishedRevision
			if kind == "block" {
				id, revision = prepare.NewBlock, 1
			}
			metadataSeed := []string{"reporting.discover", "cw." + kind + ".read:" + id}
			req := reporting.EffectDependencyRequest{Operation: kind + "_run", ID: id, Revision: revision}
			m, err := s.EffectDependencies(ctx, scoped(metadataSeed), req)
			if err != nil {
				t.Fatal(kind, "public execution discovery", err)
			}
			effectScopes := projected(m)
			for _, scope := range effectScopes {
				if strings.Contains(scope, "preview") || strings.Contains(scope, "write") || strings.Contains(scope, "publish") {
					t.Fatal("public execution acquired editing/private authority", scope)
				}
			}
			runInput := reporting.DeliveryRunRequest{Target: reporting.DeliveryTarget{Kind: kind, ID: id, Revision: revision}, Key: "published-" + kind, Timezone: "UTC", Locale: "en-US", PartialFailure: "fail_closed"}
			if kind == "block" {
				runInput.Policy = "published"
				runInput.PartialFailure = "fail"
				runInput.Outputs = []string{"chart"}
			}
			queries := f.attemptCount(t)
			denied := slices.DeleteFunc(slices.Clone(effectScopes), func(scope string) bool { return scope == "cw.execution_context.use:"+chart.Block.Context })
			if _, err := delivery.Run(ctx, scoped(denied), runInput); err == nil {
				t.Fatal("public run missing original context admitted")
			}
			if f.attemptCount(t) != queries {
				t.Fatal("denied public run queried source")
			}
			execution, err := delivery.Run(ctx, scoped(effectScopes), runInput)
			if err != nil {
				t.Fatal(kind, "public execution", err)
			}
			viewOp := "view"
			if kind == "block" {
				viewOp = "block_view"
			}
			readerSeed := []string{"reporting.discover", "cw.run.read:" + execution.Run}
			reader := func(scopes []string) identity.Envelope {
				return actor(author.Tenant(), "independent-reader", "independent-login", scopes)
			}
			retained, err := s.EffectDependencies(ctx, reader(readerSeed), reporting.EffectDependencyRequest{Operation: viewOp, ID: execution.Run})
			if err != nil || retained.Private {
				t.Fatal(kind, "public retained dependencies", err)
			}
			readScopes := projected(retained)
			if kind == "block" {
				readScopes = append(readScopes, "cw.run.read:"+execution.Run)
			}
			for _, scope := range readScopes {
				if scope == "sources.query" || strings.Contains(scope, "execute") || strings.Contains(scope, "preview") || strings.Contains(scope, "write") {
					t.Fatal("reader gained effect", scope)
				}
			}
			beforeRead := f.attemptCount(t)
			candidates, err := s.RunCandidates(ctx, reader(metadataSeed), reporting.RunCandidateRequest{Kind: kind, Resource: id, Limit: 32})
			if err != nil || !slices.Contains(candidates.Runs, execution.Run) || slices.Contains(candidates.Runs, run.ID) {
				t.Fatal("public candidates exposed private or omitted public", candidates, err)
			}
			exact, err := delivery.Runs(ctx, reader(readScopes), reporting.DeliveryRunsRequest{Kind: kind, Resource: id, Run: execution.Run, Limit: 1})
			if err != nil || len(exact.Items) != 1 || exact.Items[0].Private || exact.Items[0].Run != execution.Run {
				t.Fatal("exact run summary", exact, err)
			}
			read := reporting.DeliveryViewRequest{Kind: kind, Run: execution.Run, Output: "chart", Limit: 100}
			if kind == "report" {
				read.Page, read.Widget = "analysis", "amount"
			}
			output, err := delivery.View(ctx, reader(readScopes), read)
			if err != nil || output.Output == nil || output.Output.Chart == nil || output.Output.Chart.Points[0].Value.Exact != "9007199254740998.625" {
				t.Fatal(kind, "consumer exact values", err)
			}
			if kind == "report" {
				read.Page, read.Widget, read.Output = "notes", "note", ""
				notes, err := delivery.View(ctx, reader(readScopes), read)
				if err != nil || notes.Text == nil || notes.Text.Text != "Retained notes" {
					t.Fatal("consumer sibling page", err)
				}
			}
			missing := slices.DeleteFunc(slices.Clone(readScopes), func(scope string) bool { return scope == "cw.execution_context.use:"+chart.Block.Context })
			read.Output = "chart"
			if kind == "report" {
				read.Page, read.Widget = "analysis", "amount"
			}
			if got, err := delivery.View(ctx, reader(missing), read); err == nil && got.Output != nil {
				t.Fatal("withdrawn context returned values")
			}
			if _, err := s.EffectDependencies(ctx, reader([]string{"reporting.discover", "cw.run.read:" + run.ID}), reporting.EffectDependencyRequest{Operation: "view", ID: run.ID}); err == nil {
				t.Fatal("publication declassified prior private run")
			}
			if f.attemptCount(t) != beforeRead || f.f.model.requests.Load() != models {
				t.Fatal("consumer/discovery queried source or model")
			}
		}
	})

}
