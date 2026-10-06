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
}
