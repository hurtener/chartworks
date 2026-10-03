package acceptance

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway/bifrost"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
)

// This config helper reads only the checked-in public reference configuration.
// Credentials are resolved exclusively inside the explicitly opted-in test.
func liveGeneratedTopicConfig(t *testing.T) config.Gateway {
	t.Helper()
	g := liveGatewayConfig(t)
	for _, name := range []string{"enhance", "topic_review", "sqlgen", "sqlfix", "clarify"} {
		r := g.Roles[name]
		r.Enabled = true
		r.Provider = "openrouter"
		r.Model = "openai/gpt-6-luna"
		g.Roles[name] = r
	}
	return g
}

func TestLiveGeneratedTopicConfig(t *testing.T) {
	g := liveGeneratedTopicConfig(t)
	if err := config.ValidateGateway(g, true); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"enhance", "topic_review", "sqlgen"} {
		if !g.Roles[role].Enabled || g.Roles[role].Model != "openai/gpt-6-luna" {
			t.Fatal("generated-topic role is not explicitly admitted", role)
		}
	}
	if g.Roles["embedding"].Model != "perplexity/pplx-embed-v1-0.6b" || g.Roles["rerank"].Model != "cohere/rerank-4-fast" || g.Roles["rerank"].OnFailure != "fail" {
		t.Fatal("generated-topic retrieval roles drifted")
	}
}

// Parent-operated paid gate only. It is absent from ordinary phase/release suites
// and requires a second explicit opt-in in addition to the existing live gate.
// The generated branch never calls liveCommerceTopics or injects authored metrics.
func TestLiveGeneratedTopicGatewayE2E(t *testing.T) { runLiveGeneratedTopic(t, false) }

func TestLiveGeneratedTopicPaidVocabularyE2E(t *testing.T) { runLiveGeneratedTopic(t, true) }

func runLiveGeneratedTopic(t *testing.T, paid bool) {
	if os.Getenv("CHARTWORKS_LIVE_E2E") != "1" || os.Getenv("CHARTWORKS_LIVE_GENERATED_TOPICS") != "1" {
		t.Skip("parent budget runner must explicitly enable the generated-topic live gate")
	}
	artifactDir := liveArtifactDir(t)
	g := liveGeneratedTopicConfig(t)
	if err := config.ValidateGateway(g, true); err != nil {
		t.Fatal("pre-spend generated-topic config", err)
	}
	engine, err := bifrost.New(t.Context(), g, os.LookupEnv, bifrost.TransportOptions{EnvironmentProxy: os.Getenv("CHARTWORKS_LIVE_ENV_PROXY") == "1"})
	if err != nil {
		t.Fatal("generated-topic gateway setup", err)
	}
	t.Cleanup(engine.Close)
	if !engine.RoleEnabled("topic_review") {
		t.Fatal("topic review unavailable before generation admission")
	}
	observed := &liveReceiptEngine{Engine: engine, callLimit: 32, traceEnabled: os.Getenv("CHARTWORKS_LIVE_SYNTHETIC_TRACE") == "1"}
	report := &generatedTopicReport{GeneratedOnly: true, Stage: "not_started", Unsupported: []string{"Paid-only and refund populations require explicit admitted business vocabulary; no authored filters are injected by this baseline"}}
	defer func() {
		report.ModelUsage = observed.since(0)
		writeLiveJSON(t, artifactDir, "generated-topic-receipt.json", report)
		if observed.traceEnabled {
			writeLiveJSON(t, artifactDir, "generated-topic-trace.json", observed.traceSnapshot())
		}
	}()
	business := generatedOrdersBusiness
	if paid {
		business = generatedPaidBusiness
		report.Unsupported = []string{"Cross-dataset refund/net definitions require their own admitted meaning and reviewed relationships"}
	}
	h := newGeneratedTopicHarness(t, observed, business, report)
	var vocabulary []drafts.AuthoringValue
	if paid {
		classifyGeneratedStatusVocabulary(t, h, report)
		vocabulary = generatedPaidVocabulary(h.scaffold.Pack)
	}
	current := h.generate(t, report, nil, vocabulary)
	writeLiveJSON(t, artifactDir, "generated-topic-candidate.json", current)
	requireLiveModelCall(t, observed.since(0), "enhance", "openrouter", "openai/gpt-6-luna", "openai/gpt-6-luna", "gpt-6-luna")
	requireLiveModelCall(t, observed.since(0), "topic_review", "openrouter", "openai/gpt-6-luna", "openai/gpt-6-luna", "gpt-6-luna")
	h.publish(t, current, report, paid)
	requireLiveModelCall(t, observed.since(0), "embedding", "openrouter", "perplexity/pplx-embed-v1-0.6b", "pplx-embed-v1-0.6b", "perplexity/pplx-embed-v1-0.6b")
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	h.runQueries(t, ctx, current, report, nil, paid)
	requireLiveModelCall(t, observed.since(0), "sqlgen", "openrouter", "openai/gpt-6-luna", "openai/gpt-6-luna", "gpt-6-luna")
	requireLiveModelCall(t, observed.since(0), "rerank", "openrouter-rerank", "cohere/rerank-4-fast", "cohere/rerank-4-fast", "rerank-4-fast")
}
