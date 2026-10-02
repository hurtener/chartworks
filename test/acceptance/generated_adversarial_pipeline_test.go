package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/hurtener/chartworks/internal/nlq"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

// The scaffold is assembled exclusively from real PlanProfile outputs. Author
// input supplies wording, privacy and vocabulary, never measures or joins.
func newAdversarialTopicHarness(t *testing.T, engine gateway.Engine, netMode ...bool) (*generatedTopicHarness, map[string]string, []drafts.AuthoringValue) {
	t.Helper()
	f := newEngineeringFixture(t, nil, nil)
	seedGeneratedAdversarialWarehouse(t, f)
	var input adversarialAuthorInput
	readAdversarialJSON(t, "authoring_inputs.json", &input)
	if len(netMode) > 0 && netMode[0] {
		var netInput struct {
			Description string `json:"description"`
		}
		readAdversarialJSON(t, "net_business.json", &netInput)
		input.Description += " " + netInput.Description
	}
	cfg := f.cfg.Clone()
	names := []string{"customers", "orders", "refunds", "order_lines"}
	columns := map[string][]string{"customers": {"division_id", "customer_id", "segment"}, "orders": {"division_id", "order_id", "customer_id", "ordered_at", "misleading_net_total_usd", "status_code", "private_note"}, "refunds": {"division_id", "refund_id", "order_id", "refunded_at", "amount_usd", "status_code"}, "order_lines": {"division_id", "line_id", "order_id", "category", "line_total_usd"}}
	cfg.Connections[0].Relations = nil
	for _, name := range names {
		cfg.Connections[0].Relations = append(cfg.Connections[0].Relations, config.SourceRelation{Schema: "analytics", Name: "adv_" + name, Columns: columns[name]})
	}
	sourceService, err := sources.New(f.db, cfg, f.lookup)
	if err != nil {
		t.Fatal("adversarial setup line 39", err)
	}
	t.Cleanup(sourceService.Close)
	validator, err := readexec.NewValidator(sourceService, config.DefaultReadValidation())
	if err != nil {
		t.Fatal("adversarial setup line 44", err)
	}
	executor, err := readexec.NewExecutor(sourceService, f.db, f.values.Exec)
	if err != nil {
		t.Fatal("adversarial setup line 48", err)
	}
	values := f.values
	values.Sources = cfg
	profiles, err := engineering.New(f.db, sourceService, validator, executor, nil, values, f.lookup)
	if err != nil {
		t.Fatal("adversarial setup line 54", err)
	}
	t.Cleanup(profiles.Close)
	f.s, f.validator, f.executor, f.service, f.cfg = sourceService, validator, executor, profiles, cfg
	source := f.create(t, "adversarial-commerce")
	binding, err := sourceService.Binding(t.Context(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal("adversarial setup line 61", err)
	}
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	draftService, err := drafts.NewWithEngine(f.db, sourceService, profiles, engine)
	if err != nil {
		t.Fatal("adversarial setup line 66", err)
	}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal("adversarial setup line 70", err)
	}
	topicService, err := topics.New(f.db, sourceService, index, engine)
	if err != nil {
		t.Fatal("adversarial setup line 74", err)
	}
	client := publicationClient(t, f, draftService, topicService, topicScopes(f.e.Tenant()))
	var scaffold semantics.TopicPack
	datasets := map[string]string{}
	for _, name := range names {
		for _, relation := range binding.Relations {
			if relation.Name == "adv_"+name {
				datasets[name] = relation.ID
			}
		}
		if datasets[name] == "" {
			t.Fatal("missing scoped relation", name)
		}
		profile := f.profile(t, engineering.ProfileSpec{ID: "adversarial-" + name, Source: source.ID, Context: source.ContextID, Dataset: datasets[name], SkipLLM: true}).Profile.Profile
		if profile == nil {
			t.Fatal("missing profile", name)
		}
		planned, err := draftService.PlanProfile(t.Context(), author, drafts.OnboardRequest{Topic: "adversarial-commerce", Version: "v1", Name: input.Name, Description: input.Description, Profile: profile.Version, Change: "Profile-only structural scaffold"})
		if err != nil {
			t.Fatal("adversarial setup line 94", err)
		}
		if scaffold.Topic == "" {
			scaffold = planned
		} else {
			scaffold.Datasets = append(scaffold.Datasets, planned.Datasets...)
		}
	}
	if len(scaffold.Measures)+len(scaffold.Dimensions)+len(scaffold.KPIs)+len(scaffold.Joins) != 0 {
		t.Fatal("profile scaffold pre-authored semantics")
	}
	saved, err := client.SaveTopicDraft(t.Context(), sdk.SaveTopicDraftRequest{Pack: scaffold, Change: "Assemble four actual profile-only structural scaffolds"})
	if err != nil {
		t.Fatal("adversarial setup line 107", err)
	}
	annotated := saved.Pack
	annotated.Version = "privacy-v2"
	for i := range annotated.Datasets {
		for j := range annotated.Datasets[i].Columns {
			c := &annotated.Datasets[i].Columns[j]
			for table, id := range datasets {
				if id == annotated.Datasets[i].ID {
					if privacy := input.Privacy[table+"."+c.ID]; privacy != "" {
						c.Sensitivity = semantics.LiteralSensitivity(privacy)
					}
				}
			}
		}
	}
	saved, err = client.SaveTopicDraft(t.Context(), sdk.SaveTopicDraftRequest{Expected: saved.Metadata.Revision, Pack: annotated, Change: "Explicit author privacy classification only"})
	if err != nil {
		t.Fatal("adversarial setup line 125", err)
	}
	vocabulary := []drafts.AuthoringValue{}
	for _, entry := range input.Values {
		for _, d := range saved.Pack.Datasets {
			if d.ID == datasets[entry.Table] {
				vocabulary = append(vocabulary, drafts.AuthoringValue{ID: entry.Table + "_" + entry.Label, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: d.ID, ID: entry.Column}, Origin: d.Source, Kind: "text", Value: entry.Value, Aliases: entry.Aliases, Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"})
			}
		}
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal("adversarial setup line 137", err)
	}
	router, err := nlqroute.New(topicService, rules, index, engine)
	if err != nil {
		t.Fatal("adversarial setup line 141", err)
	}
	query, err := nlqexec.New(router, topicService, sourceService, validator, executor, engine, f.db)
	if err != nil {
		t.Fatal("adversarial setup line 145", err)
	}
	return &generatedTopicHarness{drafts: draftService, f: f, author: author, queryActor: f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...), client: client, topics: topicService, query: query, scaffold: saved}, datasets, vocabulary
}

// Recorded proposals exercise transport and validation; they are not a live
// measurement of model reasoning. They never read held_out.json or oracle.json.
func recordedAdversarialStep(t *testing.T, columns []semantics.Reference, datasets map[string]string, omitRelationships ...bool) string {
	t.Helper()
	results := []any{}
	filters := []any{}
	governed := []any{}
	relationships := []semantics.RelationshipDecision{}
	counts := []drafts.CountProposal{}
	for _, ref := range columns {
		table := ""
		for name, id := range datasets {
			if id == ref.Dataset {
				table = name
			}
		}
		item := map[string]any{"dataset": ref.Dataset, "column": ref.ID, "kind": "dimension", "name": table + " " + ref.ID, "description": "Profile-backed " + table + " " + ref.ID, "aliases": []string{}, "role": "categorical", "semantic_role": "attribute", "temporal": nil}
		switch ref.ID {
		case "misleading_net_total_usd", "amount_usd", "line_total_usd":
			item = map[string]any{"dataset": ref.Dataset, "column": ref.ID, "kind": "measure", "name": table + " known amount", "description": "Sum known USD amounts at the source fact grain; unknown amounts remain unknown", "aliases": []string{}, "aggregation": "sum", "unit": "USD", "semantic_role": "measure_input"}
			if table == "orders" {
				item["name"] = "Known paid booked amount"
				item["aliases"] = []string{"importe bruto conocido de pedidos pagados"}
			}
			if table == "orders" || table == "refunds" {
				label := "paid"
				if table == "refunds" {
					label = "posted"
				}
				filters = append(filters, map[string]any{"measure": semantics.GeneratedEntityID(semantics.EnhancementMeasure, ref.Dataset, ref.ID), "id": table + "_population", "operator": "eq", "nulls": "exclude", "vocabulary_ids": []string{table + "_" + label}})
			}
		case "ordered_at", "refunded_at":
			item["name"] = table + " event time"
			item["role"] = "temporal"
			item["semantic_role"] = "event_time"
			item["temporal"] = map[string]any{"calendar": "gregorian", "timezone": "America/New_York", "grains": []string{"month", "quarter", "year"}}
		case "private_note":
			item = map[string]any{"dataset": ref.Dataset, "column": ref.ID, "kind": "unresolved", "reason": "Sensitive free text is not an analytical field"}
		case "status_code":
			ids := []string{table + "_paid", table + "_cancelled"}
			if table == "refunds" {
				ids = []string{table + "_posted", table + "_void"}
			}
			governed = append(governed, map[string]any{"dataset": ref.Dataset, "column": ref.ID, "vocabulary_ids": ids})
		default:
			if strings.HasSuffix(ref.ID, "_id") {
				item["role"] = "identifier"
				item["semantic_role"] = "fact_key"
			}
		}
		results = append(results, item)
		if table == "orders" && (ref.ID == "order_id" || ref.ID == "misleading_net_total_usd") {
			name := "Paid order count"
			description := "Count the non-NULL order identifier once per source row; duplicate identifiers across divisions remain separate orders"
			if ref.ID == "misleading_net_total_usd" {
				name = "Known paid amount count"
				description = "Count only paid orders with a non-NULL amount; a known zero is included"
			}
			counts = append(counts, drafts.CountProposal{Dataset: ref.Dataset, Column: ref.ID, Name: name, Description: description, Aliases: []string{}, Unit: "orders"})
			filters = append(filters, map[string]any{"measure": drafts.GeneratedCountMeasureID(ref.Dataset, ref.ID), "id": "orders_paid_count", "operator": "eq", "nulls": "exclude", "vocabulary_ids": []string{"orders_paid"}})
		}
		if table == "orders" && ref.ID == "customer_id" || (table == "refunds" || table == "order_lines") && ref.ID == "order_id" {
			rightTable, rightID := "customers", "customer_id"
			if table != "orders" {
				rightTable, rightID = "orders", "order_id"
			}
			relationships = append(relationships, semantics.RelationshipDecision{ID: table + "_to_" + rightTable, Left: ref, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: datasets[rightTable], ID: rightID}, AdditionalKeys: []semantics.JoinKeyPair{{Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: ref.Dataset, ID: "division_id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: datasets[rightTable], ID: "division_id"}}}, Cardinality: semantics.CardinalityManyToOne, State: "candidate", Evidence: semantics.RelationshipEvidence{ID: table + "_composite", LeftGrain: table + " composite identifier", RightGrain: rightTable + " composite identifier", Provenance: "profile_and_author_business_input"}})
		}
	}
	if len(omitRelationships) > 0 && omitRelationships[0] {
		relationships = []semantics.RelationshipDecision{}
	}
	body, _ := json.Marshal(map[string]any{"results": results, "count_proposals": counts, "filter_proposals": filters, "value_proposals": governed, "relationships": relationships, "group_domain": map[string]any{"policy": "metric-group-domain-v1", "domain": "qualifying_population"}})
	wire, _ := json.Marshal(map[string]any{"id": "recorded-adversarial-authoring", "object": "chat.completion", "model": "model-enhance", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(body)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}})
	return "chat_raw:" + string(wire)
}

// Build the generated, explicitly reviewed publication used by the held-out
// contract tests. A successful lifecycle is not itself an answer-correctness pass.
func generateAdversarialPublished(t *testing.T, netMode ...bool) (*generatedTopicHarness, *gatewayFixture, drafts.Version, map[string]string) {
	t.Helper()
	model := newGatewayFixture(t, func(c *config.Gateway) {
		r := c.Roles["embedding"]
		r.MaxBatchItems = 128
		r.MaxBatchBytes = 128 << 10
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	h, datasets, vocabulary := newAdversarialTopicHarness(t, model.engine, netMode...)
	report := &generatedTopicReport{GeneratedOnly: true}
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		response := recordedAdversarialStep(t, columns, datasets)
		if len(netMode) > 0 && netMode[0] {
			response = recordedAdversarialNetStep(t, columns, datasets, false, false)
		}
		responses := []string{response}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	}, vocabulary)
	expectedMeasures := 5
	if len(netMode) > 0 && netMode[0] {
		expectedMeasures = 7
	}
	if len(current.Pack.Measures) != expectedMeasures || len(current.Pack.RelationshipDecisions) != 3 || len(current.Pack.Joins) != 0 {
		t.Fatal("candidate structure", len(current.Pack.Measures), len(current.Pack.RelationshipDecisions), len(current.Pack.Joins))
	}
	for _, decision := range current.Pack.RelationshipDecisions {
		if len(decision.KeyPairs()) != 2 || !strings.HasPrefix(decision.Evidence.Provenance, "authoring:") {
			t.Fatal("composite relationship evidence missing")
		}
	}
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	model.mu.Unlock()
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	for _, body := range bodies {
		if strings.Contains(body, warehouse.PrivateNote) {
			t.Fatal("private raw value reached model")
		}
	}
	t.Logf("recorded generated candidate: profiles=%d pages=%d measures=%d composite_candidates=%d advisory=%s", len(current.Pack.Datasets), len(report.Steps), len(current.Pack.Measures), len(current.Pack.RelationshipDecisions), current.Quality.Status)
	current = confirmAdversarialCandidate(t, h, model, current, datasets, vocabulary, report, netMode...)
	return h, model, current, datasets
}

func TestGeneratedAdversarialHeldOutRecorded(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t, true)
	evaluateAdversarialGrossCases(t, h, model, current, datasets)
	evaluateAdversarialAuthorityCases(t, h, model, current, datasets)
}

func confirmAdversarialCandidate(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, current drafts.Version, datasets map[string]string, vocabulary []drafts.AuthoringValue, report *generatedTopicReport, netMode ...bool) drafts.Version {
	t.Helper()
	// Explicit synthetic operator independently validates complete key membership
	// against the author-supplied grain contract before confirming proposals.
	pack := current.Pack
	expected := map[string][4]string{"orders_to_customers": {"orders", "customer_id", "customers", "customer_id"}, "refunds_to_orders": {"refunds", "order_id", "orders", "order_id"}, "order_lines_to_orders": {"order_lines", "order_id", "orders", "order_id"}}
	for _, decision := range pack.RelationshipDecisions {
		want, ok := expected[decision.ID]
		if !ok || decision.Cardinality != semantics.CardinalityManyToOne || len(decision.KeyPairs()) != 2 {
			t.Fatal("operator rejects unknown relationship")
		}
		actual := map[string]bool{}
		for _, pair := range decision.KeyPairs() {
			actual[pair.Left.Dataset+"/"+pair.Left.ID+"="+pair.Right.Dataset+"/"+pair.Right.ID] = true
		}
		for _, ids := range [][2]string{{want[1], want[3]}, {"division_id", "division_id"}} {
			if !actual[datasets[want[0]]+"/"+ids[0]+"="+datasets[want[2]]+"/"+ids[1]] {
				t.Fatal("operator rejects incomplete composite key")
			}
		}
		joinType := semantics.JoinLeft
		if len(netMode) > 0 && netMode[0] && decision.ID == "refunds_to_orders" {
			joinType = semantics.JoinInner
		}
		pack.Joins = append(pack.Joins, semantics.Join{ID: "confirmed_" + decision.ID, Name: decision.ID, Left: decision.Left, Right: decision.Right, AdditionalKeys: decision.AdditionalKeys, Type: joinType, Cardinality: decision.Cardinality, Evidence: decision.Evidence})
	}
	// Candidates are promoted, not duplicated; the prior immutable draft retains proposal evidence.
	pack.RelationshipDecisions = nil
	pack.Version = fmt.Sprintf("confirmed-v%d", current.Metadata.Revision+1)
	saved, err := h.client.SaveTopicDraft(t.Context(), sdk.SaveTopicDraftRequest{Expected: current.Metadata.Revision, Pack: pack, Change: "Synthetic operator confirms all three complete composite relationship proposals after checking declared grains"})
	if err != nil {
		t.Fatal("confirm relationships", err)
	}
	if saved.Quality != nil {
		t.Fatal("structural change inherited stale advisory")
	}
	h.scaffold = saved
	current = h.generate(t, report, func(columns []semantics.Reference, last bool) {
		response := recordedAdversarialStep(t, columns, datasets, true)
		if len(netMode) > 0 && netMode[0] {
			response = recordedAdversarialNetStep(t, columns, datasets, last)
		}
		responses := []string{response}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	}, vocabulary)
	if len(current.Pack.Joins) != 3 || current.Quality == nil {
		t.Fatal("confirmed structure lacks fresh whole review")
	}
	if len(netMode) > 0 && netMode[0] {
		reviewGeneratedAdversarialNet(t, current.Pack, datasets)
	}
	review, err := h.client.ReviewTopic(t.Context(), current.Pack.Topic, sdk.TopicReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Synthetic operator accepts known-only USD sums and explicit paid/posted mappings, complete composite joins and New York calendar. Private note stays unresolved and unavailable; only independently checked generated metric and period definitions are approved."})
	if err != nil {
		t.Fatal("operator review", err)
	}
	published, err := h.client.PublishTopic(t.Context(), current.Pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil {
		t.Fatal("publish reviewed corpus", err)
	}
	if published.Digest != current.Metadata.Digest {
		t.Fatal("published wrong candidate")
	}
	return current
}

func evaluateAdversarialGrossCases(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, current drafts.Version, datasets map[string]string) {
	t.Helper()
	// Evaluation begins only after publication. These files are never passed to
	// the authoring path, including recorded response construction above.
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	var data adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &data)
	oracle := adversarialOracle(t, data)
	metric := semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")
	for _, tc := range cases {
		if tc.ID == "stale-profile" || tc.ID == "sensitive-note" || tc.ID == "wrong-context" {
			continue
		}
		t.Run(tc.ID, func(t *testing.T) {
			result := adversarialCaseResult{ID: tc.ID, Expected: tc.Expected, Oracle: tc.Oracle, Actual: "not_completed", Stage: "plan"}
			defer func() {
				raw, _ := json.Marshal(result)
				t.Log(string(raw))
				if !result.Matches {
					t.Errorf("held-out contract mismatch: expected %s, actual %s", result.Expected, result.Actual)
				}
			}()
			sql := "SELECT SUM(misleading_net_total_usd) AS known_gross, COUNT(order_id)-COUNT(misleading_net_total_usd) AS unknown_amounts FROM analytics.adv_orders WHERE status_code='P'"
			monthly := strings.HasPrefix(tc.ID, "month")
			switch tc.ID {
			case "count-orders":
				sql = "SELECT COUNT(*) AS paid_orders FROM analytics.adv_orders WHERE status_code='P'"
			case "count-known":
				sql = "SELECT COUNT(misleading_net_total_usd) AS known_paid_amounts FROM analytics.adv_orders WHERE status_code='P'"
			case "unknown-status":
				sql = "SELECT SUM(misleading_net_total_usd) AS gross FROM analytics.adv_orders WHERE status_code='X'"
			case "single-key-join":
				sql = "SELECT SUM(o.misleading_net_total_usd) AS gross FROM analytics.adv_orders o JOIN analytics.adv_customers c ON c.customer_id=o.customer_id WHERE o.status_code='P'"
			case "child-fanout":
				sql = "SELECT SUM(o.misleading_net_total_usd) AS gross FROM analytics.adv_orders o JOIN analytics.adv_order_lines l ON (l.division_id,l.order_id)=(o.division_id,o.order_id) JOIN analytics.adv_refunds r ON (r.division_id,r.order_id)=(o.division_id,o.order_id) WHERE o.status_code='P'"
			}
			if monthly {
				sql = "SELECT date_trunc('month',ordered_at,'America/New_York') AS order_month,SUM(misleading_net_total_usd) AS known_gross,COUNT(order_id)-COUNT(misleading_net_total_usd) AS unknown_amounts FROM analytics.adv_orders WHERE status_code='P' GROUP BY date_trunc('month',ordered_at,'America/New_York') ORDER BY order_month"
			}
			model.mode.Store(phase18RawResponse(t, sql))
			locale := nlq.LanguageEnglish
			if strings.HasSuffix(tc.ID, "es") {
				locale = nlq.LanguageSpanish
			}
			metrics := []string{metric}
			if strings.Contains(tc.ID, "net") || tc.ID == "dst-fold" {
				metrics = nil
			}
			if tc.ID == "count-orders" {
				metrics = []string{drafts.GeneratedCountMeasureID(datasets["orders"], "order_id")}
			}
			if tc.ID == "count-known" {
				metrics = []string{drafts.GeneratedCountMeasureID(datasets["orders"], "misleading_net_total_usd")}
			}
			if tc.ID == "cohort-known-net" {
				metrics = []string{"known_cohort_net", "unknown_cohort_refund_amounts"}
				model.mode.Store(phase18RawResponse(t, adversarialNetSQL(false)))
			}
			if tc.ID == "activity-known-net" {
				metrics = []string{"known_activity_net", "unknown_order_amounts", "unknown_activity_refund_amounts"}
				model.mode.Store(phase18RawResponse(t, adversarialNetSQL(true)))
			}
			planned, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: locale, Question: tc.Question, MetricIDs: metrics, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}})
			if err != nil {
				result.Actual = "reject"
				result.Reason = livePlanFailureReason(err) + ": " + err.Error()
				var analytical *readexec.AnalyticalError
				if tc.Expected == "reject" && errors.As(err, &analytical) {
					result.Matches = true
				}
				var clarification *nlqroute.Clarification
				if errors.As(err, &clarification) {
					result.Actual = "clarify"
					raw, _ := json.Marshal(clarification)
					result.Reason = string(raw)
					required := map[string]string{"net-ambiguous-en": "ambiguous_metric_meaning", "net-ambiguous-es": "ambiguous_metric_meaning", "nullable-refund-net": "unknown_amount_policy_required", "definitive-gross-unknown": "unknown_amount_policy_required", "dst-fold": "ambiguous_local_time"}
					result.Matches = tc.Expected == "clarify" && required[tc.ID] != "" && clarification.Reason == required[tc.ID]
				}
				return
			}
			result.Stage = "run"
			run, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "adversarial-" + tc.ID, Rows: 20, Bytes: 65536})
			if err != nil || run.Execution.Result == nil {
				result.Actual = "execution_failed"
				if err != nil {
					result.Reason = err.Error()
				}
				return
			}
			result.Actual = "answer_mismatch"
			result.Result = run.Execution.Result.Rows
			result.AmountCompleteness = run.AmountCompleteness
			if strings.HasPrefix(tc.ID, "gross") {
				if complete, ok := adversarialAmountEvidence(run, current.Pack.Topic+":measure:"+metric); ok && len(run.Execution.Result.Rows) == 1 && complete.ValueColumn < len(run.Execution.Result.Rows[0]) {
					value := run.Execution.Result.Rows[0][complete.ValueColumn]
					if liveSingleNumericEquals([][]json.RawMessage{{value}}, oracle["known_gross_paid_local_2026"].(string)) && complete.Status == "incomplete" && len(complete.Rows) == 1 && complete.Rows[0].UnknownCount == fmt.Sprint(oracle["unknown_amount_paid_orders"]) {
						result.Actual = "answer"
						result.Matches = true
						result.Reason = "Known subtotal and proof-bound incomplete-result evidence match independent oracle"
					}
				}
			}
			if strings.HasPrefix(tc.ID, "count-") && tc.Oracle != nil && liveSingleNumericEquals(run.Execution.Result.Rows, fmt.Sprint(oracle[*tc.Oracle])) {
				result.Actual = "answer"
				result.Matches = true
			}
			if tc.ID == "cohort-known-net" || tc.ID == "activity-known-net" {
				expected := []string{oracle["known_cohort_net"].(string), fmt.Sprint(oracle["unknown_posted_refund_events_in_2026_paid_cohort"])}
				if tc.ID == "activity-known-net" {
					expected = []string{oracle["known_activity_net"].(string), fmt.Sprint(oracle["unknown_amount_paid_orders"]), fmt.Sprint(oracle["unknown_posted_refund_events_on_paid_orders_by_2026_activity"])}
				}
				rows := run.Execution.Result.Rows
				matches := len(rows) == 1 && len(rows[0]) == len(expected)
				if matches {
					for i, want := range expected {
						var got string
						if json.Unmarshal(rows[0][i], &got) != nil {
							got = string(rows[0][i])
						}
						matches = matches && liveNumberEquals(got, want)
					}
				}
				if matches {
					result.Actual = "answer"
					result.Matches = true
					result.Reason = "Net and every requested unknown-count companion match independent oracle"
				}
			}
			if monthly {
				if complete, ok := adversarialAmountEvidence(run, current.Pack.Topic+":measure:"+metric); ok {
					projected := [][]json.RawMessage{}
					for _, row := range run.Execution.Result.Rows {
						if len(row) > complete.ValueColumn {
							projected = append(projected, []json.RawMessage{row[0], row[complete.ValueColumn]})
						}
					}
					countsMatch := len(complete.Rows) == len(run.Execution.Result.Rows)
					for i, row := range run.Execution.Result.Rows {
						var instant string
						if len(row) == 0 || json.Unmarshal(row[0], &instant) != nil || len(instant) < 7 || i >= len(complete.Rows) {
							countsMatch = false
							continue
						}
						want := oracle["monthly_unknown_paid_amounts_local_2026"].(map[string]int)[instant[:7]]
						countsMatch = countsMatch && complete.Rows[i].Row == i && complete.Rows[i].UnknownCount == fmt.Sprint(want)
						status := "complete"
						if want > 0 {
							status = "incomplete"
						}
						countsMatch = countsMatch && complete.Rows[i].Status == status
					}
					if adversarialMonthsMatch(projected, oracle["monthly_qualifying_paid_local_2026"].(map[string]any)) && complete.Status == "incomplete" && countsMatch {
						result.Actual = "answer"
						result.Matches = true
						result.Reason = "Canonical instant month buckets preserve NULL/zero; proof-bound unknown counts retain incompleteness"
					}
				}
			}
		})
	}
}

func adversarialMonthsMatch(rows [][]json.RawMessage, want map[string]any) bool {
	if len(rows) != len(want) {
		return false
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if len(row) != 2 {
			return false
		}
		var key string
		if json.Unmarshal(row[0], &key) != nil || len(key) < 7 {
			return false
		}
		key = key[:7]
		expected, ok := want[key]
		if !ok || seen[key] {
			return false
		}
		seen[key] = true
		if expected == nil {
			if string(row[1]) != "null" {
				return false
			}
			continue
		}
		var amount string
		if json.Unmarshal(row[1], &amount) != nil {
			amount = string(row[1])
		}
		if !liveNumberEquals(amount, expected.(string)) {
			return false
		}
	}
	return true
}

func evaluateAdversarialAuthorityCases(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, current drafts.Version, datasets map[string]string) {
	t.Helper()
	record := func(id string, ok bool, reason string) {
		result := adversarialCaseResult{ID: id, Expected: "reject", Actual: "reject", Matches: ok, Stage: "authority", Reason: reason}
		if !ok {
			result.Actual = "assertion_failed"
			t.Error(reason)
		}
		raw, _ := json.Marshal(result)
		t.Log(string(raw))
	}
	var warehouse adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &warehouse)
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	model.mu.Unlock()
	safe := true
	for _, body := range bodies {
		if strings.Contains(body, warehouse.PrivateNote) {
			safe = false
		}
	}
	record("sensitive-note", safe, "Private raw note canary absent from every authoring, advisory and query gateway request")
	scopes := topicScopes(h.f.e.Tenant())
	for i, scope := range scopes {
		if strings.HasPrefix(scope, "cw.execution_context.use:") {
			scopes[i] = "cw.execution_context.use:unrelated:v1"
		}
	}
	wrong := h.f.token.envelope(t, h.f.e.Tenant(), h.f.e.User(), scopes...)
	if _, _, err := h.drafts.PrepareFeedbackVocabulary(t.Context(), h.author, current.Pack.Topic, current.Metadata.Revision, generatedVocabularyFromPack(current.Pack, datasets)); err != nil {
		t.Fatal("authorized vocabulary control", err)
	}
	before := model.requests.Load()
	_, _, err := h.drafts.PrepareFeedbackVocabulary(t.Context(), wrong, current.Pack.Topic, current.Metadata.Revision, generatedVocabularyFromPack(current.Pack, datasets))
	t.Logf("wrong-context typed error: %T %v, model before=%d after=%d", err, err, before, model.requests.Load())
	record("wrong-context", (errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound)) && model.requests.Load() == before, "Unrelated context denied before gateway dispatch")
	var origin semantics.SourceReference
	for _, dataset := range current.Pack.Datasets {
		if dataset.ID == datasets["orders"] {
			origin = dataset.Source
		}
	}
	if _, err := h.f.admin.Exec(t.Context(), `UPDATE analytics.adv_orders SET misleading_net_total_usd=misleading_net_total_usd+1 WHERE division_id='alpha' AND order_id=1`); err != nil {
		t.Fatal(err)
	}
	replacement := h.f.profile(t, engineering.ProfileSpec{ID: "adversarial-orders-new-head", Previous: origin.ProfileVersion, Source: origin.Source, Context: origin.Context, Dataset: origin.Dataset, SkipLLM: true}).Profile.Profile
	if replacement == nil || replacement.DeterministicHash() == origin.ProfileDigest {
		t.Fatal("new profile did not change evidence")
	}
	before = model.requests.Load()
	_, err = h.topics.Review(t.Context(), h.author, current.Pack.Topic, topics.ReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Attempt approval after changed active profile"})
	record("stale-profile", errors.Is(err, readexec.ErrBinding) && model.requests.Load() == before, "Changed active profile rejects generated approval before model work")
}

func generatedVocabularyFromPack(pack semantics.TopicPack, datasets map[string]string) []drafts.AuthoringValue {
	for _, dataset := range pack.Datasets {
		if dataset.ID == datasets["orders"] {
			return []drafts.AuthoringValue{{ID: "orders_paid", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: "status_code"}, Origin: dataset.Source, Kind: "text", Value: "P", Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"}}
		}
	}
	return nil
}

// Paraphrase controls isolate admitted count semantics from the separately owned
// annual-language parser. They use the same generated publication, not a manually
// authored measure. The original held-out questions remain unchanged.
func TestGeneratedAdversarialCountSemanticsRecorded(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t)
	var data adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &data)
	rows, known := 0, 0
	identities := map[string]bool{}
	for _, order := range data.Orders {
		if order.Status != nil && *order.Status == "P" {
			rows++
			if order.Amount != nil {
				known++
			}
			identities[fmt.Sprint(order.ID)] = true
		}
	}
	if rows == known || len(identities) == rows {
		t.Fatal("count fixture has no NULL/composite duplicate trap")
	}
	for _, tc := range []struct {
		id, question, column string
		expected             int
	}{{"rows", "How many paid orders are there across all dates?", "order_id", rows}, {"rows-paraphrase", "Count paid orders across every date, including orders whose amount is unknown.", "order_id", rows}, {"known", "How many paid orders have a known amount across all dates?", "misleading_net_total_usd", known}, {"known-paraphrase", "Count the non-null amounts of paid orders, including known zero amounts, across all dates.", "misleading_net_total_usd", known}} {
		t.Run(tc.id, func(t *testing.T) {
			metric := drafts.GeneratedCountMeasureID(datasets["orders"], tc.column)
			model.mode.Store(phase18RawResponse(t, "SELECT COUNT("+tc.column+") AS count_value FROM analytics.adv_orders WHERE status_code='P'"))
			request := nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: tc.question, MetricIDs: []string{metric}, Kinds: []string{"measure"}, LimitPerKind: 5, Rerank: true}}
			plan, err := h.query.Plan(t.Context(), h.queryActor, request)
			if err != nil {
				t.Fatal("generated count plan", err)
			}
			run, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: "count-control-" + tc.id, Rows: 10, Bytes: 65536})
			if err != nil || run.Execution.Result == nil || !liveSingleNumericEquals(run.Execution.Result.Rows, fmt.Sprint(tc.expected)) {
				t.Fatal("generated count differs from independent row/NULL oracle", err)
			}
			model.mode.Store(phase18RawResponse(t, "SELECT COUNT(DISTINCT "+tc.column+") AS count_value FROM analytics.adv_orders WHERE status_code='P'"))
			if _, err = h.query.Plan(t.Context(), h.queryActor, request); err == nil {
				t.Fatal("distinct shortcut admitted for generated count")
			}
		})
	}
}

func adversarialAmountEvidence(run nlqexec.RunResult, metric string) (nlqexec.AmountCompleteness, bool) {
	for _, evidence := range run.AmountCompleteness {
		if evidence.Metric == metric && evidence.Policy == nlqexec.AmountCompletenessPolicy && evidence.Scope == "returned_query_rows" && evidence.ValueColumn >= 0 && evidence.UnknownCountColumn >= 0 && evidence.ValueColumn != evidence.UnknownCountColumn && strings.HasSuffix(evidence.UnknownCountMetric, ":kpi:unknown_order_amounts_in_scope") {
			return evidence, true
		}
	}
	return nlqexec.AmountCompleteness{}, false
}

// Compatibility control retains the original civil-output spelling. Current
// reviewed instant buckets require date_trunc(unit, instant, zone); accepting a
// two-argument civil result would silently change the reviewed output type.
func TestGeneratedAdversarialCivilMonthCompatibilityRecorded(t *testing.T) {
	h, model, current, datasets := generateAdversarialPublished(t)
	var cases []adversarialCase
	readAdversarialJSON(t, "held_out.json", &cases)
	question := ""
	for _, c := range cases {
		if c.ID == "month-en" {
			question = c.Question
		}
	}
	model.mode.Store(phase18RawResponse(t, "SELECT date_trunc('month',ordered_at AT TIME ZONE 'America/New_York') AS order_month,SUM(misleading_net_total_usd) AS known_gross FROM analytics.adv_orders WHERE status_code='P' GROUP BY date_trunc('month',ordered_at AT TIME ZONE 'America/New_York') ORDER BY order_month"))
	_, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: question, MetricIDs: []string{semantics.GeneratedEntityID(semantics.EnhancementMeasure, datasets["orders"], "misleading_net_total_usd")}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}})
	if !errors.Is(err, readexec.ErrUnsafe) {
		t.Fatalf("civil-output spelling changed its explicit compatibility boundary: %T %v", err, err)
	}
	t.Log("Original AT TIME ZONE/two-argument civil-output form remains unsupported; canonical three-argument instant form is tested separately against unchanged month oracle")
}
