package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The provider is recorded; all admitted queries execute against PostgreSQL.
// Canceled-only groups distinguish FILTER from WHERE without inventing zero fill.
func TestSQLRecoveryOrdinaryGroupDomainAcceptance(t *testing.T) {
	f := liveCommerceSource(t)
	if _, err := f.admin.Exec(t.Context(), `ALTER TABLE analytics.customers ALTER COLUMN region DROP NOT NULL; ALTER TABLE analytics.customers DROP CONSTRAINT customers_region_check; ALTER TABLE analytics.orders ALTER COLUMN total_usd DROP NOT NULL; ALTER TABLE analytics.orders ALTER COLUMN customer_id DROP NOT NULL;
 INSERT INTO analytics.customers VALUES(5,'consumer','quiet'),(6,'consumer',NULL),(7,'business','all_null');
 INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status) VALUES(107,5,'2026-03-01',40,'cancelled'),(108,6,'2026-03-01',15,'paid'),(109,7,'2026-03-01',NULL,'paid'),(110,NULL,'2026-03-01',NULL,'paid');`); err != nil {
		t.Fatal(err)
	}
	pack, _ := liveCommerceTopics(t, f)
	pack.GroupDomain = nil // Explicit missing-policy review regression.
	pack.Dimensions = append(pack.Dimensions, semantics.Dimension{ID: "order_customer", Name: "Order customer", Role: semantics.DimensionIdentifier, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "customer_id"}})
	pack.Measures = append(pack.Measures, semantics.Measure{ID: "paid_values", Name: "Paid order values", Field: pack.Measures[0].Field, Aggregation: semantics.AggregationCount, Filters: append([]semantics.SemanticFilter(nil), pack.Measures[0].Filters...)})
	model := newGatewayFixture(t, func(cfg *config.Gateway) {
		r := cfg.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 4096
		cfg.Roles["embedding"] = r
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := topics.New(f.db, f.s, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	phase17PublishTopic(t, draft, topic, author, pack)
	for _, domain := range []string{exec.AnalyticalGroupDomainRaw, exec.AnalyticalGroupDomainQualifying} {
		p := pack
		p.Topic = pack.Topic + "-" + domain
		p.GroupDomain = &semantics.GroupDomainPolicy{Policy: semantics.MetricGroupDomainPolicy, Domain: domain}
		phase17PublishTopic(t, draft, topic, author, p)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topic, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	actor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	metadata := support.Raw(t, f.dsn)
	request := nlqexec.QuestionRequest{Topic: pack.Topic, Context: pack.Datasets[0].Source.Context, Question: "Gross revenue and Paid order values by Order customer", Locale: nlq.LanguageEnglish, MetricIDs: []string{"gross_revenue", "paid_values"}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5}
	model.mode.Store(phase18RawResponse(t, `SELECT o.customer_id,sum(o.total_usd),count(o.total_usd) FROM analytics.orders o WHERE o.status='paid' GROUP BY o.customer_id`))
	if p, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); p.QueryID != "" || !errors.Is(err, nlqexec.ErrGenerationContext) || nlqexec.GenerationProblem(err) == nil {
		t.Fatal("absent policy not explicit review-needed", err)
	}
	// Both public transports carry fixed localized review guidance without a
	// SQL generation/repair call or an executable pending plan.
	sqlCalls := func() int {
		model.mu.Lock()
		defer model.mu.Unlock()
		calls := 0
		for _, body := range model.requestBodies {
			var wire struct {
				Model string `json:"model"`
			}
			_ = json.Unmarshal([]byte(body), &wire)
			if wire.Model == model.cfg.Roles["sqlgen"].Model || wire.Model == model.cfg.Roles["sqlfix"].Model {
				calls++
			}
		}
		return calls
	}
	if sqlCalls() != 0 {
		t.Fatal("missing policy reached SQL model")
	}
	handler := nlqapi.ExecutionHandler(model.token.verifier, query, http.NotFoundHandler())
	claims := model.token.claims(actor.Tenant(), actor.User(), actor.Scopes())
	claims["session"] = actor.Session()
	httpToken := model.token.sign(t, claims, nil)
	bindings, err := nlqapi.ExecutionMCPBindings(query)
	if err != nil {
		t.Fatal(err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	mcpServer, err := mcpserver.New(model.token.verifier, mcpRegistry, config.DefaultMCP(), nil)
	if err != nil {
		t.Fatal(err)
	}
	mcpClaims := model.token.claims(actor.Tenant(), actor.User(), append(actor.Scopes(), "mcp.use"))
	mcpClaims["session"] = actor.Session()
	mcpClaims["aud"] = model.token.cfg.MCPAudience()
	mcpToken := model.token.sign(t, mcpClaims, nil)
	mcpClient, err := mcpServer.Client(func(context.Context) (string, error) { return mcpToken, nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		localized := request
		localized.Locale = locale
		payload, _ := json.Marshal(nlqexec.PlanRequest{QuestionRequest: localized})
		r := httptest.NewRequest(http.MethodPost, "/v1/nlq/plans", bytes.NewReader(payload))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Authorization", "Bearer "+httpToken)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, r)
		if response.Code != 422 || !strings.Contains(response.Body.String(), `"error":"generation_context_insufficient"`) || !strings.Contains(response.Body.String(), "group_domain") {
			t.Fatal("HTTP domain review disposition", response.Code, response.Body.String())
		}
		localizedWord := "topic owner"
		if locale == nlq.LanguageSpanish {
			localizedWord = "responsable del tema"
		}
		if !strings.Contains(response.Body.String(), localizedWord) {
			t.Fatal("HTTP domain review locale")
		}
		result, err := mcpClient.CallTool(t.Context(), "plan_question", payload)
		if err != nil || result == nil || !result.IsError || len(result.Content) == 0 {
			t.Fatal("MCP domain review", err)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok || !strings.Contains(content.Text, "generation_context_insufficient") || !strings.Contains(content.Text, localizedWord) || !strings.Contains(content.Text, "group_domain") {
			t.Fatal("MCP domain review public guidance")
		}
	}
	if sqlCalls() != 0 {
		t.Fatal("public review request generated SQL")
	}
	for _, joined := range []bool{false, true} {
		key, from, dimension := "o.customer_id", "analytics.orders o", "Order customer"
		want := map[string][]string{"1": {"200.00", "2"}, "2": {"200.00", "1"}, "3": {"90.00", "1"}, "4": {"150.00", "1"}, "6": {"15.00", "1"}, "7": {"NULL", "0"}, "NULL": {"NULL", "0"}}
		if joined {
			key, from, dimension = "c.region", "analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id", "Customer region"
			want = map[string][]string{"north": {"350.00", "3"}, "south": {"200.00", "1"}, "west": {"90.00", "1"}, "NULL": {"15.00", "1"}, "all_null": {"NULL", "0"}}
		}
		for _, domain := range []string{exec.AnalyticalGroupDomainQualifying, exec.AnalyticalGroupDomainRaw} {
			request.Topic = pack.Topic + "-" + domain
			request.Question = "Gross revenue and Paid order values by " + dimension
			whereSQL := "SELECT " + key + ",sum(o.total_usd),count(o.total_usd) FROM " + from + " WHERE o.status='paid' GROUP BY " + key
			filterSQL := "SELECT " + key + ",sum(o.total_usd) FILTER(WHERE o.status='paid'),count(o.total_usd) FILTER(WHERE o.status='paid') FROM " + from + " GROUP BY " + key
			good, bad := whereSQL, filterSQL
			if domain == exec.AnalyticalGroupDomainRaw {
				good, bad = filterSQL, whereSQL
				if joined {
					want["quiet"] = []string{"NULL", "0"}
				} else {
					want["5"] = []string{"NULL", "0"}
				}
			}
			model.mode.Store(phase18RawResponse(t, good))
			planned, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
			if err != nil {
				t.Fatal("ordinary domain Plan", joined, domain, err)
			}
			result, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: planned.QueryID + "-domain"})
			if err != nil || result.Execution.Result == nil {
				t.Fatal("ordinary domain Run", err)
			}
			actual := map[string][]string{}
			for _, row := range result.Execution.Result.Rows {
				values := []string{"NULL", "NULL", "NULL"}
				for i := range values {
					if string(row[i]) != "null" && json.Unmarshal(row[i], &values[i]) != nil {
						t.Fatal("unexpected typed value")
					}
				}
				if _, exists := actual[values[0]]; exists {
					t.Fatal("duplicate group")
				}
				actual[values[0]] = values[1:]
			}
			if exec.Hash(actual) != exec.Hash(want) {
				t.Fatal("phantom/NULL/all-NULL group result", joined, domain, actual)
			}
			attempts := count(t, metadata, "SELECT count(*) FROM chartworks.read_attempts")
			model.mode.Store(phase18RawResponse(t, bad))
			if p, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); p.QueryID != "" || err == nil || count(t, metadata, "SELECT count(*) FROM chartworks.read_attempts") != attempts {
				t.Fatal("WHERE/FILTER relocation obtained executable authority", joined, domain, err)
			}
		}
	}
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	model.mu.Unlock()
	seen := map[string]bool{}
	for _, body := range bodies {
		var wire struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if json.Unmarshal([]byte(body), &wire) != nil {
			t.Fatal("provider JSON")
		}
		if wire.Model != model.cfg.Roles["sqlgen"].Model && wire.Model != model.cfg.Roles["sqlfix"].Model {
			continue
		}
		for _, message := range wire.Messages {
			if message.Role != "system" {
				continue
			}
			for _, domain := range []string{exec.AnalyticalGroupDomainRaw, exec.AnalyticalGroupDomainQualifying} {
				if strings.Contains(message.Content, domain) {
					seen[domain] = true
				}
			}
			if !strings.Contains(message.Content, "Reviewed ordinary group-domain policy") || strings.Contains(message.Content, "only a reviewed filter shared by every selected metric may be moved to WHERE") {
				t.Fatal("provider domain instruction missing or contradictory")
			}
		}
	}
	if len(seen) != 2 {
		t.Fatal("both reviewed domain prompts not observed")
	}
	if _, err := f.admin.Exec(t.Context(), "DELETE FROM analytics.refunds; DELETE FROM analytics.order_items; DELETE FROM analytics.orders"); err != nil {
		t.Fatal(err)
	}
	model.mode.Store(phase18RawResponse(t, `SELECT c.region,sum(o.total_usd) FILTER(WHERE o.status='paid'),count(o.total_usd) FILTER(WHERE o.status='paid') FROM analytics.orders o JOIN analytics.customers c ON o.customer_id=c.customer_id GROUP BY c.region`))
	planned, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
	if err != nil {
		t.Fatal(err)
	}
	result, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: planned.QueryID + "-empty"})
	if err != nil || result.Execution.Result == nil || len(result.Execution.Result.Rows) != 0 {
		t.Fatal("empty population invented group", err)
	}
}
