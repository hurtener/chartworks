package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

const generatedOrdersBusiness = "All-order gross value is the sum of total_usd in USD at one row per order_id. It includes both paid and cancelled orders; status is descriptive, not a population restriction. ordered_at is the order's calendar date, using the Gregorian calendar and UTC; month, quarter and year groupings are meaningful. order_id is an identifier. Produce no net-revenue KPI or relationships. Missing business meaning must remain unresolved."

type generatedTopicReport struct {
	RecordingID             string                 `json:"recording_id,omitempty"`
	RecordingDigest         string                 `json:"recording_digest,omitempty"`
	InterpretationPolicy    string                 `json:"interpretation_policy,omitempty"`
	FixtureID               string                 `json:"fixture_id,omitempty"`
	FixtureDigest           string                 `json:"fixture_digest,omitempty"`
	PrivacyAnnotationDigest string                 `json:"privacy_annotation_digest,omitempty"`
	InputMode               string                 `json:"input_mode"`
	Stage                   string                 `json:"stage"`
	Complete                bool                   `json:"complete"`
	GeneratedOnly           bool                   `json:"generated_only"`
	ProfileVersion          string                 `json:"profile_version"`
	ProfileDigest           string                 `json:"profile_digest"`
	ScaffoldDigest          string                 `json:"scaffold_digest"`
	Steps                   []drafts.EnhanceResult `json:"generation_steps"`
	PublicationDigest       string                 `json:"publication_digest,omitempty"`
	BusinessReview          string                 `json:"business_review"`
	Queries                 []liveReceipt          `json:"queries"`
	ModelUsage              []gateway.Usage        `json:"model_usage"`
	Unsupported             []string               `json:"unsupported_semantics"`
}
type generatedTopicHarness struct {
	drafts             *drafts.Service
	f                  *engineeringFixture
	author, queryActor identity.Envelope
	client             *sdk.Client
	topics             *topics.Service
	query              *nlqexec.Service
	scaffold           sdk.TopicDraft
}

// Uses the real warehouse and profile service. No semantic entities are inserted
// by this helper: the sole initial draft comes from OnboardProfile.
func newGeneratedTopicHarness(t *testing.T, engine gateway.Engine, business string, report *generatedTopicReport) *generatedTopicHarness {
	t.Helper()
	return newNamedGeneratedTopicHarness(t, engine, business, report, "generated-order-value", "All-order gross value")
}

func newNamedGeneratedTopicHarness(t *testing.T, engine gateway.Engine, business string, report *generatedTopicReport, topic, name string) *generatedTopicHarness {
	t.Helper()
	ctx := t.Context()
	report.Stage = "source_profile"
	if report.InputMode == "" {
		report.InputMode = "schema_profile_and_business_description"
	}
	f := liveCommerceSource(t)
	cfg := f.cfg.Clone()
	cfg.Connections[0].Relations = []config.SourceRelation{{Schema: "analytics", Name: "orders", Columns: []string{"order_id", "ordered_at", "total_usd", "status"}}}
	sourceService, err := sources.New(f.db, cfg, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sourceService.Close)
	validator, err := readexec.NewValidator(sourceService, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	executor, err := readexec.NewExecutor(sourceService, f.db, f.values.Exec)
	if err != nil {
		t.Fatal(err)
	}
	values := f.values
	values.Sources = cfg
	profiles, err := engineering.New(f.db, sourceService, validator, executor, nil, values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(profiles.Close)
	f.s, f.validator, f.executor, f.service, f.cfg = sourceService, validator, executor, profiles, cfg
	source := f.create(t, "generated-commerce")
	binding, err := sourceService.Binding(ctx, f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	dataset := ""
	for _, relation := range binding.Relations {
		if relation.Name == "orders" {
			dataset = relation.ID
		}
	}
	if dataset == "" {
		t.Fatal("orders missing from real source catalog")
	}
	profile := f.profile(t, engineering.ProfileSpec{ID: "generated-orders-profile", Source: source.ID, Context: source.ContextID, Dataset: dataset, Columns: []string{"order_id", "ordered_at", "total_usd", "status"}, SkipLLM: true}).Profile.Profile
	if profile == nil {
		t.Fatal("real profile was not retained")
	}
	report.ProfileVersion, report.ProfileDigest = profile.Version, profile.DeterministicHash()
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	draftService, err := drafts.NewWithEngine(f.db, sourceService, profiles, engine)
	if err != nil {
		t.Fatal(err)
	}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	topicService, err := topics.New(f.db, sourceService, index, engine)
	if err != nil {
		t.Fatal(err)
	}
	client := publicationClient(t, f, draftService, topicService, topicScopes(f.e.Tenant()))
	report.Stage = "profile_onboarding"
	scaffold, err := client.OnboardTopicProfile(ctx, sdk.OnboardTopicProfileRequest{Topic: topic, Version: "v1", Name: name, Description: business, Profile: profile.Version, Change: "Create unresolved scaffold from real profile"})
	if err != nil {
		t.Fatal("profile onboarding", err)
	}
	if len(scaffold.Pack.Measures)+len(scaffold.Pack.Dimensions)+len(scaffold.Pack.KPIs)+len(scaffold.Pack.Joins) != 0 {
		t.Fatal("onboarding injected semantic entities")
	}
	report.ScaffoldDigest = scaffold.Metadata.Digest
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topicService, rules, index, engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, topicService, sourceService, validator, executor, engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	return &generatedTopicHarness{drafts: draftService, f: f, author: author, queryActor: f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...), client: client, topics: topicService, query: query, scaffold: scaffold}
}

func (h *generatedTopicHarness) generate(t *testing.T, report *generatedTopicReport, prepare func([]semantics.Reference, bool), vocabulary ...[]drafts.AuthoringValue) drafts.Version {
	t.Helper()
	report.Stage = "paginated_generation"
	model, err := semantics.Compile(h.scaffold.Pack)
	if err != nil {
		t.Fatal(err)
	}
	columns := semantics.GenerationColumns(model)
	current := drafts.Version{Metadata: drafts.Revision{Topic: h.scaffold.Metadata.Topic, Revision: h.scaffold.Metadata.Revision, Digest: h.scaffold.Metadata.Digest}, Pack: h.scaffold.Pack}
	for cursor := 0; cursor < len(columns); {
		end := min(cursor+2, len(columns))
		if prepare != nil {
			prepare(columns[cursor:end], end == len(columns))
		}
		request := sdk.EnhanceTopicRequest{Expected: current.Metadata.Revision, Version: fmt.Sprintf("v%d", current.Metadata.Revision+1), Cursor: cursor, Limit: 2, Change: "Generate bounded profile-backed semantics"}
		if cursor == 0 && len(vocabulary) > 0 {
			request.Vocabulary = vocabulary[0]
			report.InputMode = "schema_profile_business_description_and_explicit_vocabulary"
		}
		step, err := h.client.EnhanceTopicDraft(t.Context(), current.Pack.Topic, request)
		if err != nil {
			t.Fatalf("generation cursor %d: %T %#v", cursor, err, err)
		}
		report.Steps = append(report.Steps, step)
		current = step.Draft
		if step.NextCursor != end || step.Complete != (end == len(columns)) {
			t.Fatal("generation cursor mismatch")
		}
		cursor = step.NextCursor
	}
	report.Stage = "whole_candidate_review"
	compiled, err := semantics.Compile(current.Pack)
	if err != nil {
		t.Fatal(err)
	}
	if current.Quality == nil || !current.Quality.ValidFor(compiled) {
		t.Fatal("generated candidate has no exact complete advisory")
	}
	reloaded, err := h.client.TopicDraftVersion(t.Context(), current.Pack.Topic, current.Metadata.Revision)
	if err != nil || reloaded.Quality == nil || reloaded.Quality.CandidateDigest != current.Metadata.Digest {
		t.Fatal("advisory did not survive API reload", err)
	}
	return current
}

// This is a synthetic operator's explicit review gate, not an automatic approval
// of arbitrary model output. The harness stops instead of repairing the candidate.
func generatedBusinessReview(p semantics.TopicPack, paidOnly bool) string {
	if len(p.Datasets) != 1 || len(p.Joins) != 0 || len(p.KPIs) != 0 {
		return "unexpected_generated_structure"
	}
	var gross *semantics.Measure
	for i := range p.Measures {
		m := &p.Measures[i]
		if m.Field.ID == "total_usd" {
			if gross != nil {
				return "ambiguous_generated_gross"
			}
			gross = m
		}
	}
	if gross == nil || gross.Aggregation != semantics.AggregationSum || gross.Unit != "USD" {
		return "generated_gross_definition_missing"
	}
	if paidOnly {
		if p.GroupDomain == nil || p.GroupDomain.Policy != "metric-group-domain-v1" || p.GroupDomain.Domain != "qualifying_population" {
			return "generated_group_domain_missing"
		}
		paid := false
		for _, f := range gross.Filters {
			if f.Field.Dataset == p.Datasets[0].ID && f.Field.ID == "status" && f.Operator == "eq" && len(f.Values) == 1 && f.Values[0] == "paid" {
				paid = true
			}
		}
		if !paid {
			return "generated_paid_population_missing"
		}
	} else if len(gross.Filters) != 0 {
		return "invented_generated_population"
	}
	temporal := false
	for _, d := range p.Dimensions {
		if d.Field.ID == "ordered_at" && d.Role == semantics.DimensionTemporal && d.Temporal != nil && d.Temporal.Calendar == "gregorian" && d.Temporal.Timezone == "UTC" {
			for _, g := range d.Temporal.Grains {
				if g == semantics.GrainMonth {
					temporal = true
				}
			}
		}
	}
	if !temporal {
		return "generated_calendar_policy_missing"
	}
	if len(p.Unresolved) != 0 {
		return "generated_semantics_unresolved"
	}
	return "accepted_synthetic_business_contract"
}

func (h *generatedTopicHarness) publish(t *testing.T, current drafts.Version, report *generatedTopicReport, paid ...bool) {
	t.Helper()
	h.publishReviewed(t, current, report, generatedBusinessReview(current.Pack, len(paid) > 0 && paid[0]), "Synthetic operator checked all-order gross and Gregorian UTC calendar business requirements against this exact generated candidate")
}

// Both fixture lanes stop on findings. Recorded advisories are contract fixtures,
// never permission to waive a finding in a live candidate.
func generatedPublicationBlockReason(current drafts.Version, businessReview string) string {
	if businessReview != "accepted_synthetic_business_contract" {
		return businessReview
	}
	if current.Quality == nil || current.Quality.Status != "no_findings" {
		return "model_advisory_requires_adjudication"
	}
	return ""
}

func (h *generatedTopicHarness) publishReviewed(t *testing.T, current drafts.Version, report *generatedTopicReport, businessReview, note string) {
	t.Helper()
	report.Stage = "explicit_operator_review"
	report.BusinessReview = businessReview
	if reason := generatedPublicationBlockReason(current, businessReview); reason != "" {
		t.Fatalf("generated candidate not patched or published: %s", reason)
	}
	review, err := h.client.ReviewTopic(t.Context(), current.Pack.Topic, sdk.TopicReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: note})
	if err != nil {
		t.Fatal("explicit generated-topic review", err)
	}
	report.Stage = "publication"
	published, err := h.client.PublishTopic(t.Context(), current.Pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil {
		t.Fatal("generated-topic publication", err)
	}
	if published.Digest != current.Metadata.Digest {
		t.Fatal("publication differs from reviewed generated topic")
	}
	report.PublicationDigest = published.Digest
}

type generatedTopicQuery struct {
	id, question, sql string
	monthly           bool
	checkPlan         func(*testing.T, nlqexec.PlanResult)
}

func generatedQueryCases(paid ...bool) []generatedTopicQuery {
	if len(paid) > 0 && paid[0] {
		return []generatedTopicQuery{{"paid-gross", "What is paid-order gross revenue in USD?", "SELECT SUM(total_usd) AS paid_gross FROM analytics.orders WHERE status='paid'", false, nil}, {"paid-calendar-month", "What is paid-order gross revenue by month in 2026?", "SELECT date_trunc('month',CAST(ordered_at AS timestamp without time zone)) AS order_month, SUM(total_usd) AS paid_gross FROM analytics.orders WHERE status='paid' GROUP BY date_trunc('month',CAST(ordered_at AS timestamp without time zone)) ORDER BY order_month", true, nil}}
	}
	return []generatedTopicQuery{
		{"all-order-gross", "What is the all-order gross value in USD across every order, including paid and cancelled orders?", "SELECT SUM(total_usd) AS all_order_gross FROM analytics.orders", false, nil},
		{"calendar-month", "What is all-order gross value by month in 2026?", "SELECT date_trunc('month',CAST(ordered_at AS timestamp without time zone)) AS order_month, SUM(total_usd) AS all_order_gross FROM analytics.orders GROUP BY date_trunc('month',CAST(ordered_at AS timestamp without time zone)) ORDER BY order_month", true, nil},
	}
}

func (h *generatedTopicHarness) runQueries(t *testing.T, ctx context.Context, current drafts.Version, report *generatedTopicReport, prepare func(string), paid ...bool) {
	t.Helper()
	paidOnly := len(paid) > 0 && paid[0]
	expectedTotal, expectedMonths := generatedIndependentTotals(t, h.f, paidOnly)
	h.runQueryCases(t, ctx, current, report, prepare, generatedQueryCases(paidOnly), expectedTotal, expectedMonths)
}

func (h *generatedTopicHarness) runQueryCases(t *testing.T, ctx context.Context, current drafts.Version, report *generatedTopicReport, prepare func(string), cases []generatedTopicQuery, expectedTotal string, expectedMonths map[string]string, policies ...string) {
	t.Helper()
	policy := ""
	if len(policies) > 1 {
		t.Fatal("ambiguous generated-query interpretation policy")
	}
	if len(policies) == 1 {
		policy = policies[0]
	}
	report.InterpretationPolicy = policy
	report.Stage = "nlq_execution"
	metric := semantics.GeneratedEntityID(semantics.EnhancementMeasure, current.Pack.Datasets[0].ID, "total_usd")
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			if prepare != nil {
				prepare(tc.sql)
			}
			receipt := liveReceipt{Case: tc.id, Status: "not_completed"}
			report.Queries = append(report.Queries, receipt)
			slot := len(report.Queries) - 1
			planned, err := h.query.Plan(ctx, h.queryActor, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{InterpretationPolicy: policy, Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.LanguageEnglish, Question: tc.question, MetricIDs: []string{metric}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5, Rerank: true}})
			if err != nil {
				report.Queries[slot].Reason = livePlanFailureReason(err)
				t.Fatalf("generated-topic plan: %s", report.Queries[slot].Reason)
			}
			if tc.checkPlan != nil {
				tc.checkPlan(t, planned)
			}
			run, err := h.query.Run(ctx, h.queryActor, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "generated-" + tc.id, Rows: 20, Bytes: 65536})
			if err != nil || run.Execution.Result == nil {
				t.Fatal("generated-topic execution", err, run.Status)
			}
			if !tc.monthly && !liveSingleNumericEquals(run.Execution.Result.Rows, expectedTotal) {
				t.Fatal("generated amount differs from independently calculated fixture result")
			}
			if tc.monthly && !generatedCalendarCorrect(run.Execution.Result.Rows, expectedMonths) {
				t.Fatal("generated calendar totals differ from independently calculated per-month results")
			}
			report.Queries[slot] = liveReceipt{Case: tc.id, Status: run.Status, QueryID: planned.QueryID, RowCount: len(run.Execution.Result.Rows), ModelCalls: len(planned.Receipt.Calls), ModelCallsKnown: true, ModelUsage: planned.Receipt.Calls, RouteOutcome: string(planned.Route.Outcome), SourceStatus: run.Execution.Attempt.Status}
		})
	}
	report.Complete = !t.Failed()
	if report.Complete {
		report.Stage = "complete"
	}
}

func generatedCalendarCorrect(rows [][]json.RawMessage, expected map[string]string) bool {
	want := map[string]string{}
	for key, value := range expected {
		want[key] = value
	}
	if len(rows) != len(want) {
		return false
	}
	for _, row := range rows {
		if len(row) != 2 {
			return false
		}
		var values [2]string
		for i := range row {
			if json.Unmarshal(row[i], &values[i]) != nil {
				values[i] = string(row[i])
			}
		}
		key, value := values[0], values[1]
		if !strings.HasPrefix(key, "2026-") {
			key, value = value, key
		}
		if len(key) < 7 {
			return false
		}
		key = key[:7]
		expected, ok := want[key]
		if !ok || !liveNumberEquals(value, expected) {
			return false
		}
		delete(want, key)
	}
	return len(want) == 0
}

func recordedGeneratedStep(t *testing.T, columns []semantics.Reference, calendars ...string) string {
	t.Helper()
	calendar := "gregorian"
	if len(calendars) > 1 {
		t.Fatal("ambiguous recorded calendar")
	}
	if len(calendars) == 1 {
		calendar = calendars[0]
	}
	items := []any{}
	for _, ref := range columns {
		common := map[string]any{"dataset": ref.Dataset, "column": ref.ID, "aliases": []string{}}
		switch ref.ID {
		case "total_usd":
			common["kind"] = "measure"
			common["name"] = "All-order gross value"
			common["description"] = "Sum total_usd across every order, including paid and cancelled orders, at order grain"
			common["aggregation"] = "sum"
			common["unit"] = "USD"
			common["semantic_role"] = "measure_input"
		case "ordered_at":
			common["kind"] = "dimension"
			common["name"] = "Order date"
			common["description"] = "Gregorian UTC order calendar date"
			common["role"] = "temporal"
			common["semantic_role"] = "event_time"
			common["temporal"] = map[string]any{"calendar": calendar, "timezone": "UTC", "grains": []string{"month", "quarter", "year"}}
			common["aliases"] = []string{"order month"}
		case "order_id":
			common["kind"] = "dimension"
			common["name"] = "Order identifier"
			common["description"] = "One order row identifier"
			common["role"] = "identifier"
			common["semantic_role"] = "fact_key"
			common["temporal"] = nil
		case "status":
			common["kind"] = "dimension"
			common["name"] = "Order status"
			common["description"] = "Descriptive order state, not a population filter"
			common["role"] = "categorical"
			common["semantic_role"] = "attribute"
			common["temporal"] = nil
		default:
			t.Fatalf("unexpected profiled column %s", ref.ID)
		}
		items = append(items, common)
	}
	body, _ := json.Marshal(map[string]any{"results": items})
	wire, _ := json.Marshal(map[string]any{"id": "recorded-generated-topic", "object": "chat.completion", "model": "model-enhance", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(body)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}})
	return "chat_raw:" + string(wire)
}

func TestGeneratedTopicPipelineRecorded(t *testing.T) {
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 65536
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	report := &generatedTopicReport{GeneratedOnly: true, Unsupported: []string{"paid-only population requires structured reviewed filter input; prose alone is insufficient"}}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		responses := []string{recordedGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	})
	if reason := generatedBusinessReview(current.Pack, true); reason != "generated_paid_population_missing" && reason != "generated_group_domain_missing" {
		t.Fatal("paid-only meaning was silently fabricated", reason)
	}
	h.publish(t, current, report)
	h.runQueries(t, t.Context(), current, report, func(sql string) { model.mode.Store(phase18RawResponse(t, sql)) })
	if !report.Complete || len(report.Steps) != 2 || len(report.Queries) != 2 {
		t.Fatal("generated pipeline did not complete")
	}
}

func TestGeneratedTopicProfileHeadFreshness(t *testing.T) {
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 65536
		c.Roles["embedding"] = r
	})
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	prepare := func(columns []semantics.Reference, last bool) {
		responses := []string{recordedGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	}
	current := h.generate(t, report, prepare)
	review, err := h.topics.Review(t.Context(), h.author, current.Pack.Topic, topics.ReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Synthetic human checked exact first-profile candidate"})
	if err != nil {
		t.Fatal(err)
	}
	originalQuality := current.Quality.ContextDigest
	origin := current.Pack.Datasets[0].Source
	var replacement *engineering.Profile
	index, err := vindex.New(h.f.db)
	if err != nil {
		t.Fatal(err)
	}
	paused := &generatedProfileReplacementEngine{Engine: model.engine, before: func() {
		if _, insertErr := h.f.admin.Exec(t.Context(), `INSERT INTO analytics.orders(order_id,customer_id,ordered_at,total_usd,status,internal_code) VALUES(999,1,'2026-04-01',15,'paid','synthetic')`); insertErr != nil {
			t.Fatal(insertErr)
		}
		replacement = h.f.profile(t, engineering.ProfileSpec{ID: "generated-orders-profile-2", Previous: origin.ProfileVersion, Source: origin.Source, Context: origin.Context, Dataset: origin.Dataset, Columns: []string{"order_id", "ordered_at", "total_usd", "status"}, SkipLLM: true}).Profile.Profile
	}}
	midPublish, err := topics.New(h.f.db, h.f.s, index, paused)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = midPublish.Publish(t.Context(), h.author, current.Pack.Topic, topics.PublishRequest{Review: review.ID}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("profile replacement during embedding escaped commit fence", err)
	}
	if replacement == nil || replacement.SourceRevision != origin.SourceRevision || replacement.DeterministicHash() == origin.ProfileDigest || replacement.Sampling.Rows != 7 {
		t.Fatal("replacement did not change aggregate evidence at unchanged source revision")
	}
	if _, err = h.topics.Read(t.Context(), h.author, current.Pack.Topic, ""); err == nil {
		t.Fatal("stale candidate became published during replacement race")
	}
	before := model.requests.Load()
	_, err = h.topics.Review(t.Context(), h.author, current.Pack.Topic, topics.ReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Attempt stale profile review"})
	if !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("changed profile head did not invalidate generated approval", err)
	}
	if _, err = h.topics.Publish(t.Context(), h.author, current.Pack.Topic, topics.PublishRequest{Review: review.ID}); !errors.Is(err, readexec.ErrBinding) || model.requests.Load() != before {
		t.Fatal("stale generated publication reached model or committed", err)
	}
	retained, err := h.client.TopicDraftVersion(t.Context(), current.Pack.Topic, current.Metadata.Revision)
	if err != nil || retained.Quality == nil || retained.Quality.ContextDigest != originalQuality || retained.Pack.Datasets[0].Source.ProfileVersion != origin.ProfileVersion {
		t.Fatal("historical advisory was silently rewritten", err)
	}
	columns := []drafts.ColumnRebinding{}
	for _, c := range current.Pack.Datasets[0].Columns {
		columns = append(columns, drafts.ColumnRebinding{Column: c.ID, SourceName: c.SourceName})
	}
	rebound, err := h.client.RebindTopicDataset(t.Context(), current.Pack.Topic, sdk.RebindTopicDatasetRequest{Expected: current.Metadata.Revision, Version: "fresh-profile", Dataset: origin.Dataset, Profile: replacement.Version, Columns: columns, Change: "Explicitly bind replacement profile before fresh generation review"})
	if err != nil || rebound.Quality != nil {
		t.Fatal("rebind reused stale quality report", err)
	}
	h.scaffold = rebound
	fresh := h.generate(t, report, prepare)
	if fresh.Quality.ContextDigest == originalQuality || fresh.Pack.Datasets[0].Source.ProfileVersion != replacement.Version {
		t.Fatal("fresh generation did not bind replacement evidence")
	}
	h.publish(t, fresh, report)
}

// Replaces the active profile after publication preflight but before its commit.
type generatedProfileReplacementEngine struct {
	gateway.Engine
	once   sync.Once
	before func()
}

func (e *generatedProfileReplacementEngine) Embed(ctx context.Context, call gateway.Call, budget *gateway.Budget, space string, input []string) (gateway.Embedded, error) {
	e.once.Do(e.before)
	return e.Engine.Embed(ctx, call, budget, space, input)
}

// The oracle reads fixture rows and computes rational sums in Go; it does not
// reuse the model's SQL, date_trunc expression, topic measures or prompt examples.
func generatedIndependentTotals(t *testing.T, f *engineeringFixture, paid bool) (string, map[string]string) {
	t.Helper()
	rows, err := f.admin.Query(t.Context(), "SELECT ordered_at::text,total_usd::text,status FROM analytics.orders")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	total := new(big.Rat)
	months := map[string]*big.Rat{}
	for rows.Next() {
		var date, amount, state string
		if err := rows.Scan(&date, &amount, &state); err != nil {
			t.Fatal(err)
		}
		if paid && state != "paid" {
			continue
		}
		value, ok := new(big.Rat).SetString(amount)
		if !ok {
			t.Fatal("invalid synthetic money")
		}
		total.Add(total, value)
		if strings.HasPrefix(date, "2026-") {
			key := date[:7]
			if months[key] == nil {
				months[key] = new(big.Rat)
			}
			months[key].Add(months[key], value)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for key, value := range months {
		out[key] = value.FloatString(2)
	}
	return total.FloatString(2), out
}

const generatedPaidBusiness = "Paid-order gross revenue is the sum of total_usd in USD at one row per order_id, restricted to the paid state from the explicitly supplied status vocabulary. Cancelled orders and NULL or unmapped status do not count. Only calendar months with at least one qualifying paid order should appear. ordered_at is a Gregorian UTC calendar date; month, quarter and year grouping are supported. order_id is an identifier. No refund or net-revenue definition is supplied. If the paid mapping is unavailable, keep the population unresolved."

func generatedPaidVocabulary(p semantics.TopicPack) []drafts.AuthoringValue {
	origin := p.Datasets[0].Source
	field := semantics.Reference{Kind: semantics.KindColumn, Dataset: p.Datasets[0].ID, ID: "status"}
	return []drafts.AuthoringValue{{ID: "paid_status", Field: field, Origin: origin, Kind: "text", Value: "paid", Aliases: []string{"pagado"}, Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"}, {ID: "cancelled_status", Field: field, Origin: origin, Kind: "text", Value: "cancelled", Aliases: []string{"cancelado"}, Sensitivity: semantics.LiteralNonSensitive, Nulls: "exclude"}}
}

func recordedPaidGeneratedStep(t *testing.T, columns []semantics.Reference) string {
	t.Helper()
	var envelope map[string]any
	if json.Unmarshal([]byte(strings.TrimPrefix(recordedGeneratedStep(t, columns), "chat_raw:")), &envelope) != nil {
		t.Fatal("recorded step envelope")
	}
	message := envelope["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
	var body map[string]any
	if json.Unmarshal([]byte(message["content"].(string)), &body) != nil {
		t.Fatal("recorded step body")
	}
	for _, item := range body["results"].([]any) {
		result := item.(map[string]any)
		switch result["column"] {
		case "total_usd":
			result["name"] = "Paid-order gross revenue"
			result["description"] = "Sum order totals only for the explicit paid-state mapping; exclude NULL and cancelled status"
			body["filter_proposals"] = []any{map[string]any{"measure": semantics.GeneratedEntityID(semantics.EnhancementMeasure, result["dataset"].(string), "total_usd"), "id": "paid_only", "operator": "eq", "nulls": "exclude", "vocabulary_ids": []string{"paid_status"}}}
			body["group_domain"] = map[string]any{"policy": "metric-group-domain-v1", "domain": "qualifying_population"}
		case "status":
			body["value_proposals"] = []any{map[string]any{"dataset": result["dataset"], "column": "status", "vocabulary_ids": []string{"paid_status", "cancelled_status"}}}
		}
	}
	raw, _ := json.Marshal(body)
	message["content"] = string(raw)
	raw, _ = json.Marshal(envelope)
	return "chat_raw:" + string(raw)
}

func TestGeneratedTopicPaidVocabularyPipelineRecorded(t *testing.T) {
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 65536
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedPaidBusiness, report)
	classifyGeneratedStatusVocabulary(t, h, report)
	vocabulary := generatedPaidVocabulary(h.scaffold.Pack)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		if last {
			head, err := h.client.TopicDraft(t.Context(), h.scaffold.Pack.Topic)
			if err != nil {
				t.Fatal(err)
			}
			changed := append([]drafts.AuthoringValue(nil), vocabulary...)
			changed[0].Value = "different-paid-state"
			before := model.requests.Load()
			if _, err = h.client.EnhanceTopicDraft(t.Context(), head.Pack.Topic, sdk.EnhanceTopicRequest{Expected: head.Metadata.Revision, Version: "catalog-change", Cursor: 2, Limit: 2, Change: "Changed catalog must not resume", Vocabulary: changed}); err == nil || model.requests.Load() != before {
				t.Fatal("changed vocabulary resumed or reached model", err)
			}
		}
		responses := []string{recordedPaidGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	}, vocabulary)
	checkpoint, ok, err := h.f.db.TopicGenerationCheckpoint(t.Context(), h.author, current.Pack.Topic, current.Metadata.Revision)
	if err != nil || !ok || len(checkpoint.Vocabulary) != 2 || checkpoint.Vocabulary[0].Origin != vocabulary[0].Origin {
		t.Fatal("vocabulary was not inherited and sealed", err)
	}
	h.publish(t, current, report, true)
	h.runQueries(t, t.Context(), current, report, func(sql string) { model.mode.Store(phase18RawResponse(t, sql)) }, true)
	if !report.Complete || report.InputMode != "schema_profile_business_description_and_explicit_vocabulary" {
		t.Fatal("paid generated pipeline incomplete")
	}
}

func TestGeneratedTopicMissingVocabularyDeniesPopulation(t *testing.T) {
	model := newGatewayFixture(t, nil)
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedPaidBusiness, report)
	compiled, err := semantics.Compile(h.scaffold.Pack)
	if err != nil {
		t.Fatal(err)
	}
	model.mode.Store(recordedPaidGeneratedStep(t, semantics.GenerationColumns(compiled)))
	before := model.requests.Load()
	if _, err = h.client.EnhanceTopicDraft(t.Context(), h.scaffold.Pack.Topic, sdk.EnhanceTopicRequest{Expected: h.scaffold.Metadata.Revision, Version: "no-vocabulary", Limit: 4, Change: "No authorized literal mapping"}); err == nil {
		t.Fatal("paid population invented without authorized mapping")
	}
	head, err := h.client.TopicDraft(t.Context(), h.scaffold.Pack.Topic)
	if err != nil || head.Metadata.Revision != h.scaffold.Metadata.Revision || model.requests.Load() != before+1 {
		t.Fatal("invalid population changed draft or retried model", err)
	}
}

// Explicit privacy annotation is normal caller input, never a model-inferred
// downgrade. It does not insert a target measure, dimension, filter or join.
func classifyGeneratedStatusVocabulary(t *testing.T, h *generatedTopicHarness, report *generatedTopicReport) {
	t.Helper()
	model, err := semantics.Compile(h.scaffold.Pack)
	if err != nil {
		t.Fatal(err)
	}
	pack := model.Pack()
	if len(pack.Measures)+len(pack.Dimensions)+len(pack.KPIs)+len(pack.Joins) != 0 {
		t.Fatal("privacy annotation must precede generated semantics")
	}
	found := false
	for i := range pack.Datasets {
		for j := range pack.Datasets[i].Columns {
			c := &pack.Datasets[i].Columns[j]
			if c.ID == "status" {
				if c.Sensitivity == semantics.LiteralSensitive {
					t.Fatal("sensitive classification cannot be overridden")
				}
				c.Sensitivity = semantics.LiteralNonSensitive
				found = true
			}
		}
	}
	if !found {
		t.Fatal("status field missing")
	}
	pack.Version = fmt.Sprintf("privacy-v%d", h.scaffold.Metadata.Revision+1)
	annotated, err := h.client.SaveTopicDraft(t.Context(), sdk.SaveTopicDraftRequest{Expected: h.scaffold.Metadata.Revision, Pack: pack, Change: "Explicit synthetic author privacy annotation: status is non-sensitive; no target semantics supplied"})
	if err != nil {
		t.Fatal("explicit field privacy annotation", err)
	}
	h.scaffold = annotated
	report.PrivacyAnnotationDigest = annotated.Metadata.Digest
}
