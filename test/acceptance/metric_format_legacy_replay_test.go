package acceptance

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// This is explicitly synthetic persisted compatibility evidence, not a recovered
// historical artifact. Before the first insert only, the wrapper runs the real
// legacy presentation owner on an otherwise normally validated query. PostgreSQL
// constraints, source admission, SQL/parameters, proof and publication stay real.
type syntheticLegacyMetricWriter struct{ *postgres.DB }

func legacyMetricContext(ctx context.Context, a *nlq.ContextAssembler, input nlq.AssembledContext) (nlq.AssembledContext, error) {
	return a.Assemble(ctx, nlq.ContextInput{MetricFormat: nlq.MetricFormatLegacyV1, Locale: input.Locale, Strategy: input.Strategy, Topic: input.Topic, TopicVersion: input.TopicVersion, Topics: input.Topics, Question: input.Question, Relations: input.Relations, Evidence: input.Evidence, Constraints: input.Constraints, Metrics: input.Metrics, Advisory: input.Advisory, Examples: input.Examples}, input.Tier)
}

func (r syntheticLegacyMetricWriter) CreateQuery(ctx context.Context, scope store.Scope, q nlqexec.QueryRecord) error {
	protected := func(q nlqexec.QueryRecord) string {
		return exec.Hash([]any{q.SQL, q.Parameters, q.Analytical, q.AnalyticalVersion, q.Clarification, q.Route.Selection, q.Route.Request, q.Route.Resolutions, q.Route.Interpretation, q.Route.AnswerContext, q.Route.SourceBindingDigest, q.Topic, q.Topics, q.TopicVersions, q.RuleVersions, q.Templates, q.RelationScope, q.PlanOperation, q.PlanRequestDigest})
	}
	before := protected(q)
	a, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		return err
	}
	if q.Route.Context == nil {
		return fmt.Errorf("synthetic legacy fixture requires route context")
	}
	raw, err := json.Marshal(q.Route.Context)
	if err != nil {
		return err
	}
	var routeInput nlq.AssembledContext
	if err = json.Unmarshal(raw, &routeInput); err != nil {
		return err
	}
	oldRoute, err := legacyMetricContext(ctx, a, routeInput)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(oldRoute.Metrics, routeInput.Metrics) || !reflect.DeepEqual(oldRoute.Constraints, routeInput.Constraints) || !reflect.DeepEqual(oldRoute.Relations, routeInput.Relations) {
		return fmt.Errorf("synthetic legacy conversion changed required semantics")
	}
	raw, err = json.Marshal(oldRoute)
	if err != nil {
		return err
	}
	view := new(nlqroute.ContextView)
	if err = json.Unmarshal(raw, view); err != nil {
		return err
	}
	q.Route.Context = view
	q.Route.Audit = oldRoute.Audit
	oldGeneration, err := legacyMetricContext(ctx, a, q.Generation.Context)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(oldGeneration.Metrics, q.Generation.Context.Metrics) || !reflect.DeepEqual(oldGeneration.Constraints, q.Generation.Context.Constraints) || !reflect.DeepEqual(oldGeneration.Relations, q.Generation.Context.Relations) {
		return fmt.Errorf("synthetic legacy generation changed required semantics")
	}
	genInput := nlq.GenerationInput{Context: oldGeneration}
	switch q.Generation.Strategy {
	case nlq.GenerationEditBase:
		genInput.EditBase = q.Generation.Selected
	case nlq.GenerationHints:
		genInput.Hints = q.Generation.Selected
	case nlq.GenerationExamples:
		genInput.Examples = q.Generation.Selected
	case nlq.GenerationDefault:
		genInput.Default = q.Generation.Selected
	default:
		return fmt.Errorf("unsupported synthetic legacy generation strategy")
	}
	q.Generation, err = a.ResolvePrecedence(ctx, genInput)
	if err != nil {
		return err
	}
	if protected(q) != before {
		return fmt.Errorf("synthetic legacy conversion changed protected query semantics")
	}
	return r.DB.CreateQuery(ctx, scope, q)
}

func TestMetricFormatLegacyStoredReplayAcceptance(t *testing.T) {
	h, model, current, _ := generateAdversarialPublished(t, true)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	question := ""
	for _, c := range cases {
		if c.ID == "cohort-known-net" {
			question = c.Question
		}
	}
	if question == "" {
		t.Fatal("missing cohort fixture question")
	}
	request := nlqexec.PlanRequest{Operation: "synthetic-legacy-metric-plan", QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{"known_cohort_net", "unknown_order_amounts", "unknown_cohort_refund_amounts"}, Kinds: []string{"kpi", "measure", "dimension"}, LimitPerKind: 8}}
	model.mode.Store(phase18RawResponse(t, scopedNetSQL))
	writer := scalarEntailmentService(t, h, model, syntheticLegacyMetricWriter{h.f.db})
	client := scalarEntailmentClient(t, h, writer, h.queryActor, phase18Scopes(h.queryActor.Tenant(), true))
	plan, err := client.PlanNLQ(t.Context(), request)
	if err != nil {
		t.Fatal("synthetic legacy Plan/insert", err)
	}
	scope, _ := store.NewScope(h.queryActor.Tenant(), h.queryActor.User())
	read := func() nlqexec.QueryRecord {
		t.Helper()
		q, err := h.f.db.ReadQuery(t.Context(), scope, plan.QueryID)
		if err != nil {
			t.Fatal(err)
		}
		return q
	}
	legacy := read()
	if legacy.Route.Context.MetricFormat != nlq.MetricFormatLegacyV1 || legacy.Generation.Context.MetricFormat != nlq.MetricFormatLegacyV1 || !strings.Contains(legacy.Route.Context.Prompt, "metric_definitions:shared-versioned-v1") || !strings.Contains(legacy.Generation.Context.Prompt, "metric_definitions:shared-versioned-v1") {
		t.Fatal("fixture is not production shared legacy format")
	}
	prompt := legacy.Route.Context.Prompt
	generationPrompt := legacy.Generation.Context.Prompt
	selection := exec.Hash(legacy.Route.Selection)
	metrics := exec.Hash(legacy.Route.Context.Metrics)
	beforeRestart := nlqexec.QueryLineageDigest(legacy)
	beforeJSON, _ := json.Marshal(legacy)
	restarted := scalarEntailmentService(t, h, model, h.f.db)
	restartedClient := scalarEntailmentClient(t, h, restarted, h.queryActor, phase18Scopes(h.queryActor.Tenant(), true))
	check := read()
	afterJSON, _ := json.Marshal(check)
	if string(beforeJSON) != string(afterJSON) || beforeRestart != nlqexec.QueryLineageDigest(check) {
		t.Fatal("restart changed persisted legacy JSON/lineage")
	}
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	run := nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "synthetic-legacy-metric-run"}
	result, err := restartedClient.RunNLQ(t.Context(), run)
	if err != nil {
		t.Fatal("legacy SDK source run", err)
	}
	scalarEntailmentAssertRows(t, result, scalarPredicateOracle(t, warehouse, false, false))
	retained := read()
	retainedJSON, _ := json.Marshal(retained)
	digest := nlqexec.QueryLineageDigest(retained)
	metadata := support.Raw(t, h.f.dsn)
	reads := func() int64 { return count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) }

	calls, attempts := model.requests.Load(), reads()
	replay, err := restartedClient.RunNLQ(t.Context(), run)
	if err != nil {
		t.Fatal("legacy SDK terminal replay", err)
	}
	scalarEntailmentAssertRows(t, replay, scalarPredicateOracle(t, warehouse, false, false))
	repeated, err := restartedClient.PlanNLQ(t.Context(), request)
	if err != nil || repeated.QueryID != plan.QueryID {
		t.Fatal("legacy SDK original Plan replay", err)
	}
	check = read()
	afterJSON, _ = json.Marshal(check)
	if string(retainedJSON) != string(afterJSON) || digest != nlqexec.QueryLineageDigest(check) || model.requests.Load() != calls || reads() != attempts {
		t.Fatal("terminal legacy replay mutated row/lineage or repeated model/source work")
	}
	child, err := restartedClient.RefineNLQ(t.Context(), nlqexec.RefineRequest{QueryID: plan.QueryID})
	if err != nil {
		t.Fatal("legacy parent refine", err)
	}
	fresh, err := h.f.db.ReadQuery(t.Context(), scope, child.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Route.Context.MetricFormat != nlq.MetricFormatSharedV2 || fresh.Generation.Context.MetricFormat != nlq.MetricFormatSharedV2 || fresh.Parent != plan.QueryID || fresh.ParentDigest != digest {
		t.Fatal("fresh child lost v2 or exact legacy parent lineage")
	}
	check = read()
	afterJSON, _ = json.Marshal(check)
	if string(retainedJSON) != string(afterJSON) || check.Route.Context.Prompt != prompt || check.Generation.Context.Prompt != generationPrompt || exec.Hash(check.Route.Selection) != selection || exec.Hash(check.Route.Context.Metrics) != metrics {
		t.Fatal("refinement rewrote legacy parent context")
	}
	childRun, err := restartedClient.RunNLQ(t.Context(), nlqexec.RunRequest{QueryID: child.QueryID, Operation: "synthetic-v2-child-run"})
	if err != nil {
		t.Fatal("v2 child source run", err)
	}
	scalarEntailmentAssertRows(t, childRun, scalarPredicateOracle(t, warehouse, false, false))
	t.Logf("synthetic stored production-v1 shared context tokens=%d; restart/SDK source run/terminal replay/Plan replay preserved; fresh v2 child tokens=%d; replay model/source work=0", legacy.Generation.Context.Tokens, fresh.Generation.Context.Tokens)
}
