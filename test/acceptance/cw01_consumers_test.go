package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/topicapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func cw01HTTPClient(t *testing.T, f *cw01Fixture) (*sdk.Client, *httptest.Server, string) {
	t.Helper()
	_, published := newPhase18Service(t, f.phase17Fixture)
	handler := nlqapi.ExecutionHandler(f.model.token.verifier, f.query, topicapi.Handler(f.model.token.verifier, nil, published, f.rules, http.NotFoundHandler()))
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	token := phase18Token(t, f.phase17Fixture, f.e.User(), f.e.Session(), true)
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	return client, server, token
}

func cw01ExportDenials(t *testing.T, f *cw01Fixture, server *httptest.Server) {
	t.Helper()
	ctx := context.Background()
	before := f.model.requests.Load()
	for _, denied := range []struct {
		name, remove string
		wantError    error
		wantStatus   int
	}{
		{"missing-export-action", "topics.export", access.ErrForbidden, http.StatusForbidden},
		{"missing-export-resource", "cw.topic.export:*", access.ErrNotFound, http.StatusNotFound},
	} {
		t.Run(denied.name, func(t *testing.T) {
			var scopes []string
			removed := false
			for _, scope := range phase18Scopes(f.e.Tenant(), true) {
				if scope == denied.remove {
					removed = true
					continue
				}
				scopes = append(scopes, scope)
			}
			if !removed {
				t.Fatal("denial fixture did not remove the expected export reach")
			}
			claims := f.model.token.claims(f.e.Tenant(), f.e.User(), scopes)
			claims["session"] = f.e.Session()
			token := f.model.token.sign(t, claims, nil)
			envelope, err := f.model.token.verifier.Verify(ctx, token, auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			out, err := f.rules.ExportClarifications(ctx, envelope, f.pack.Topic, rulesets.ClarificationExportRequest{Version: f.definition.Version})
			if !errors.Is(err, denied.wantError) || out.RuleDigest != "" || out.Definition.Topic != "" {
				t.Fatalf("direct export did not preserve the nondisclosing authority contract: %T %v", err, err)
			}
			client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
			if err != nil {
				t.Fatal(err)
			}
			out, err = client.ExportClarifications(ctx, f.pack.Topic, sdk.ClarificationExportRequest{Version: f.definition.Version})
			var status *sdk.StatusError
			if !errors.As(err, &status) || status.Status != denied.wantStatus || out.RuleDigest != "" || out.Definition.Topic != "" {
				t.Fatalf("HTTP export did not preserve the nondisclosing authority contract: %T %v", err, err)
			}
		})
	}
	if f.model.requests.Load() != before {
		t.Fatal("denied export invoked a provider")
	}
}

func cw01AuthoringAcceptance(t *testing.T) {
	t.Helper()
	f := newCW01Fixture(t)
	ctx := context.Background()
	client, server, token := cw01HTTPClient(t, f)
	cases := []semantics.ClarificationInput{{Locale: "en", Question: "Show large sales"}, {Locale: "es", Question: "Mostrá el total de ventas"}, {Locale: "en", Question: "Show turnover review"}}
	before := f.model.requests.Load()
	preview, err := client.PreviewClarifications(ctx, f.pack.Topic, sdk.ClarificationPreviewRequest{Definition: f.definition, Cases: cases})
	if err != nil || len(preview.Cases) != 3 || preview.Cases[0].Outcome != semantics.ClarificationMissing || preview.Cases[1].Outcome != semantics.ClarificationNotApplicable {
		t.Fatal("HTTP draft preview did not evaluate matching/nonmatching cases", err)
	}
	detached, err := f.rules.Patterns(ctx, f.e, f.pack.Topic, f.definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	detached[0].Policy.Why = "mutated read"
	detached[0].Slots[0].Effect.Unit = "mutated-unit"
	again, err := f.rules.Patterns(ctx, f.e, f.pack.Topic, f.definition.Version)
	if err != nil || again[0].Policy.Why == "mutated read" || again[0].Slots[0].Effect.Unit == "mutated-unit" {
		t.Fatal("published policy nested pointers are shared", err)
	}
	cw01ExportDenials(t, f, server)
	portable, err := client.ExportClarifications(ctx, f.pack.Topic, sdk.ClarificationExportRequest{Version: f.definition.Version})
	if err != nil || portable.RuleDigest != preview.RuleDigest || len(portable.Dispositions) != len(f.definition.Patterns) {
		t.Fatal("versioned export lost exact definition", err)
	}
	imported, err := client.PreviewClarificationImport(ctx, f.pack.Topic, sdk.ClarificationImportRequest{Pack: portable, Version: "rules-imported", Cases: cases})
	if err != nil || !imported.ReviewRequired || imported.Definition.Version != "rules-imported" {
		t.Fatal("portable import preview failed", err)
	}
	current, err := f.rules.Read(ctx, f.e, f.pack.Topic, "")
	if err != nil || current.State.Version != f.definition.Version {
		t.Fatal("preview changed active policy", err)
	}
	for i := range imported.Definition.Patterns {
		if imported.Definition.Patterns[i].ID == "amount-required" {
			imported.Definition.Patterns[i].Policy.When.AnyTerms = append(imported.Definition.Patterns[i].Policy.When.AnyTerms, "turnover review")
		}
	}
	f.publishRules(t, imported.Definition, 1)
	refs := []semantics.Reference{{Kind: semantics.KindDataset, ID: f.pack.Datasets[0].ID}}
	replay, err := f.rules.Replay(ctx, f.e, f.pack.Topic, rulesets.ReplayRequest{TopicVersion: f.published.State.Version, RuleVersion: f.definition.Version, References: refs, ClarificationCases: cases})
	if err != nil || len(replay.BaselineClarifications) != 3 {
		t.Fatal("retained clarification replay failed", err)
	}
	shadow, err := f.rules.Shadow(ctx, f.e, f.pack.Topic, rulesets.ShadowRequest{TopicVersion: f.published.State.Version, BaselineRuleVersion: f.definition.Version, CandidateRuleVersion: imported.Definition.Version, References: refs, ClarificationCases: cases})
	if err != nil || !shadow.Changed || len(shadow.CandidateClarifications) != 3 || shadow.BaselineClarifications[2].Outcome != semantics.ClarificationNotApplicable || shadow.CandidateClarifications[2].Outcome != semantics.ClarificationMissing {
		t.Fatal("shadow missed conditional behavior change", err)
	}
	metadata := support.Raw(t, f.f.dsn)
	var baseline, candidate []byte
	err = metadata.QueryRow(ctx, `SELECT baseline_clarification_result,candidate_clarification_result FROM chartworks.topic_rule_comparison_evidence WHERE tenant_id=$1 AND comparison_id=$2`, f.e.Tenant(), shadow.ID).Scan(&baseline, &candidate)
	var storedBaseline, storedCandidate []semantics.ClarificationEvaluation
	if err != nil || json.Unmarshal(baseline, &storedBaseline) != nil || json.Unmarshal(candidate, &storedCandidate) != nil || len(storedBaseline) != 3 || len(storedCandidate) != 3 || storedCandidate[2].Outcome != semantics.ClarificationMissing {
		t.Fatal("typed shadow evidence not persisted", err)
	}
	if _, err = metadata.Exec(ctx, `UPDATE chartworks.topic_rule_comparison_evidence SET baseline_clarification_result='[]'::jsonb WHERE tenant_id=$1 AND comparison_id=$2`, f.e.Tenant(), shadow.ID); err == nil {
		t.Fatal("retained comparison evidence mutated")
	}
	// Unknown executable matcher fields must fail the actual closed HTTP decoder.
	wire, _ := json.Marshal(sdk.ClarificationPreviewRequest{Definition: imported.Definition, Cases: cases})
	var unknown map[string]any
	_ = json.Unmarshal(wire, &unknown)
	definition := unknown["definition"].(map[string]any)
	policy := definition["patterns"].([]any)[0].(map[string]any)["policy"].(map[string]any)
	policy["when"].(map[string]any)["regex"] = ".*"
	raw, _ := json.Marshal(unknown)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/topics/"+f.pack.Topic+"/clarifications/preview", bytes.NewReader(raw))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 400 {
		t.Fatal("ordinary authoring accepted an arbitrary executable matcher")
	}
	if f.model.requests.Load() != before {
		t.Fatal("authoring/replay/shadow invoked provider")
	}
	// Legacy imports require a deliberate reference-only disposition and cannot
	// acquire automatic blocking merely because the field was once marked required.
	legacy := semantics.CloneRuleSetDefinition(imported.Definition)
	legacy.Version = "legacy-rules"
	pattern := cw01Pattern(t, legacy, "metric")
	pattern.Policy = nil
	legacy.Patterns = []semantics.ClarificationPattern{pattern}
	f.publishRules(t, legacy, 2)
	exported, err := f.rules.ExportClarifications(ctx, f.e, f.pack.Topic, rulesets.ClarificationExportRequest{Version: legacy.Version})
	if err != nil {
		t.Fatal(err)
	}
	migration := rulesets.ClarificationImportRequest{Pack: exported, Version: "legacy-reviewed-copy", Cases: []semantics.ClarificationInput{{Locale: "en", Question: "Choose sales"}}}
	if _, err = f.rules.PreviewClarificationImport(ctx, f.e, f.pack.Topic, migration); rulesets.IsClarificationImportError(err) != "legacy_disposition_required" {
		t.Fatal("legacy imported without a safe disposition", err)
	}
	migration.LegacyDisposition = "preserve_reference_only"
	compatible, err := f.rules.PreviewClarificationImport(ctx, f.e, f.pack.Topic, migration)
	if err != nil || compatible.Preview.Cases[0].Outcome != semantics.ClarificationNotApplicable {
		t.Fatal("safe legacy migration introduced blocking", err)
	}
	if _, err = f.rules.Retire(ctx, f.e, f.pack.Topic, rulesets.RetireRequest{Expected: 3, Note: "Retire synthetic clarification policy"}); err != nil {
		t.Fatal(err)
	}
	retained, err := f.rules.Replay(ctx, f.e, f.pack.Topic, rulesets.ReplayRequest{TopicVersion: f.published.State.Version, RuleVersion: legacy.Version, References: refs, ClarificationCases: migration.Cases})
	if err != nil || retained.BaselineClarifications[0].Outcome != semantics.ClarificationNotApplicable {
		t.Fatal("retirement destroyed exact retained replay", err)
	}
}

func cw01ConsumerAcceptance(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	f := newCW01Fixture(t)
	client, _, _ := cw01HTTPClient(t, f)
	question := f.question("Show optional sales", nlq.LanguageSpanish)
	pending, err := client.PreflightNLQ(ctx, sdk.NLQPreflightRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	question.AnswerContext = pending.Route.AnswerContext
	question.ClarificationQuery = pending.QueryID
	question.Answers = []semantics.ClarificationAnswer{f.answer(t, "amount-optional", cw01Number("10"))}
	plan, err := client.PlanNLQ(ctx, sdk.NLQPlanRequest{QuestionRequest: question})
	if err != nil || plan.Bindings == nil || plan.Bindings.Validation == nil {
		t.Fatal("HTTP/SDK typed plan roundtrip failed", err)
	}
	run, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-http"})
	if err != nil || run.Execution.Result == nil || len(run.Execution.Result.Rows) != 1 {
		t.Fatal("HTTP answer did not affect actual rows", err)
	}
	corrected := f.answer(t, "amount-optional", cw01Number("1"))
	child, err := client.RefineNLQ(ctx, sdk.NLQRefineRequest{QueryID: plan.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{corrected}}})
	if err != nil || len(child.AnswerChanges) != 1 {
		t.Fatal("HTTP correction lineage missing", err)
	}
	changed, err := client.RunNLQ(ctx, sdk.NLQRunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-http"})
	if err != nil || changed.Execution.Result == nil || len(changed.Execution.Result.Rows) != 2 {
		t.Fatal("HTTP correction retained stale filter", err)
	}
	invalid := f.answer(t, "amount-optional", cw01Number("not-a-number"))
	before := f.model.requests.Load()
	_, err = client.RefineNLQ(ctx, sdk.NLQRefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{invalid}}})
	var status *sdk.StatusError
	if !errors.As(err, &status) || status.Clarification == nil || len(status.Clarification.Fields) == 0 || !strings.Contains(status.Clarification.Fields[0].Message, "Usá") || f.model.requests.Load() != before {
		t.Fatal("HTTP localized repair lost or provider called", err)
	}
	bindings, err := nlqapi.ExecutionMCPBindings(f.query)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	settings := config.DefaultMCP()
	server, err := mcpserver.New(f.model.token.verifier, registry, settings, []string{"https://console.example"})
	if err != nil {
		t.Fatal(err)
	}
	transport, err := mcpserver.HTTPRegistry(settings)
	if err != nil {
		t.Fatal(err)
	}
	network := httptest.NewServer(api.Guard(f.model.token.verifier, transport, server.Handler()))
	defer network.Close()
	claims := f.model.token.claims(f.e.Tenant(), f.e.User(), append(phase18Scopes(f.e.Tenant(), true), "mcp.use"))
	claims["session"] = f.e.Session()
	claims["aud"] = f.model.token.cfg.MCPAudience()
	token := f.model.token.sign(t, claims, nil)
	mcpClient, err := sdk.New(network.URL, network.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	// The same HTTP-created retained query is refined by the actual MCP/SDK
	// transport under a newly supplied bearer, not copied to another executor.
	corrected.Value = nil
	corrected.Remove = true
	removed := phase22Call[nlqexec.PlanResult](t, mcpClient, "refine_question", nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{corrected}}})
	if len(removed.Route.Resolutions) != 0 || len(removed.AnswerChanges) != 1 || removed.AnswerChanges[0].Action != "removed" {
		t.Fatal("MCP removal contract lost")
	}
	clean := phase22Call[nlqexec.RunResult](t, mcpClient, "run_question", nlqexec.RunRequest{QueryID: removed.QueryID, Operation: removed.QueryID + "-mcp"})
	if clean.Execution.Result == nil || len(clean.Execution.Result.Rows) != 2 {
		t.Fatal("MCP retained stale filter")
	}
	bad := phase22RawTool(t, mcpClient, "refine_question", nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Answers: []semantics.ClarificationAnswer{invalid}}})
	failure := phase22Fault(t, bad)
	if !bad.IsError || failure.Clarification == nil || failure.Clarification.Outcome != semantics.ClarificationInvalid {
		t.Fatal("MCP typed field repair lost")
	}
}
