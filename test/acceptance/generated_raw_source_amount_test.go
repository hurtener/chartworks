package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

// This independently versioned test input does not reinterpret the original
// gross fixture or establish the business components of orders.total_usd.
const generatedRawSourceAmountV1SHA256 = "be9f3d0e29aab3002bba6aac5745c0c9ee718463f0c19e8bd13226a5b225796c"
const generatedRawSourceAmountV2SHA256 = "b0a348f50e4e71603b6a6a5dbe18203093f0221d093c110918420689bc9f2831"
const generatedRawSourceAmountV2SQLV2SHA256 = "ebf1b1b1a18b4dffa88f63e1e1c95e88ed52ac7abcfe98dac9bca5f0fe1888ab"
const generatedRawSourceAmountDescription = "Sum the stored total_usd source values across all order rows at one row per order_id"
const generatedRawSourceAmountDisclosure = "Gross revenue, net revenue, tax, discount and refund treatment are not defined."
const generatedRawSourceAmountMonetaryDescription = "Sum the stored monetary values in total_usd across all order rows at one row per order_id. " + generatedRawSourceAmountDisclosure

type generatedRawSourceAmountFixture struct {
	ID                  string            `json:"id"`
	Topic               string            `json:"topic"`
	Name                string            `json:"name"`
	Business            string            `json:"business"`
	SourceFixtureSHA256 string            `json:"source_fixture_sha256"`
	ExpectedTotal       string            `json:"expected_total"`
	ExpectedMonths      map[string]string `json:"expected_months"`
	Queries             []struct {
		ID       string `json:"id"`
		Question string `json:"question"`
		SQL      string `json:"recorded_sql"`
		Monthly  bool   `json:"monthly"`
	} `json:"queries"`
	digest string
}

func loadGeneratedRawSourceAmountV1(t *testing.T) *generatedRawSourceAmountFixture {
	t.Helper()
	return loadGeneratedRawSourceAmountFixture(t, "v1", generatedRawSourceAmountV1SHA256)
}

func loadGeneratedRawSourceAmountV2(t *testing.T) *generatedRawSourceAmountFixture {
	t.Helper()
	return loadGeneratedRawSourceAmountFixture(t, "v2", generatedRawSourceAmountV2SHA256)
}

func loadGeneratedRawSourceAmountFixture(t *testing.T, version, expectedDigest string) *generatedRawSourceAmountFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/generated_raw_source_amount_" + version + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture generatedRawSourceAmountFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	fixture.digest = hex.EncodeToString(digest[:])
	if fixture.digest != expectedDigest {
		t.Fatal("raw source-amount input/question identity changed; version the new fixture independently")
	}
	source, err := os.ReadFile("testdata/live_commerce.sql")
	if err != nil {
		t.Fatal(err)
	}
	digest = sha256.Sum256(source)
	if hex.EncodeToString(digest[:]) != fixture.SourceFixtureSHA256 {
		t.Fatal("raw source-amount source fixture changed; version the new input independently")
	}
	return &fixture
}

func (f *generatedRawSourceAmountFixture) queryCases() []generatedTopicQuery {
	cases := make([]generatedTopicQuery, 0, len(f.Queries))
	for _, q := range f.Queries {
		tc := generatedTopicQuery{id: q.ID, question: q.Question, sql: q.SQL, monthly: q.Monthly}
		if q.Monthly && f.ID == "generated-raw-source-amount-v2" {
			tc.checkPlan = requireGeneratedRawSourceAmountYear
		}
		cases = append(cases, tc)
	}
	return cases
}

// Keep the failed initial recording in the frozen input fixture. This separately
// versioned provider recording leaves the exact owned period to the service.
func recordedRawSourceAmountV2Cases(t *testing.T, f *generatedRawSourceAmountFixture, report *generatedTopicReport) []generatedTopicQuery {
	t.Helper()
	raw, err := os.ReadFile("testdata/generated_raw_source_amount_v2_sql_v2.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != generatedRawSourceAmountV2SQLV2SHA256 {
		t.Fatal("raw source-amount SQL recording identity changed")
	}
	var recording struct {
		ID            string `json:"id"`
		Fixture       string `json:"input_fixture_id"`
		FixtureDigest string `json:"input_fixture_sha256"`
		Case          string `json:"case_id"`
		SQL           string `json:"sql"`
	}
	if err := json.Unmarshal(raw, &recording); err != nil {
		t.Fatal(err)
	}
	if recording.ID != "generated-raw-source-amount-v2-sql-v2" || recording.Fixture != f.ID || recording.FixtureDigest != f.digest {
		t.Fatal("SQL recording changed input/question identity")
	}
	cases := f.queryCases()
	matched := 0
	for i := range cases {
		if cases[i].id == recording.Case {
			if !cases[i].monthly {
				t.Fatal("owned period recording targets a scalar case")
			}
			cases[i].sql = recording.SQL
			matched++
		}
	}
	if matched != 1 {
		t.Fatal("SQL recording must replace exactly one named case")
	}
	report.RecordingID, report.RecordingDigest = recording.ID, generatedRawSourceAmountV2SQLV2SHA256
	return cases
}

func requireGeneratedRawSourceAmountYear(t *testing.T, planned nlqexec.PlanResult) {
	t.Helper()
	if planned.Route.Interpretation == nil || len(planned.Route.Interpretation.Temporal) != 1 || planned.Bindings == nil || planned.Bindings.Validation == nil || !planned.Bindings.Validation.Validated || len(planned.Bindings.Bindings) != 1 {
		t.Fatal("raw source-amount year has no validated owned binding")
	}
	span := planned.Route.Interpretation.Temporal[0]
	binding := planned.Bindings.Bindings[0]
	if span.Column != "ordered_at" || span.Start != "2026-01-01" || span.End != "2027-01-01" || span.Calendar != "gregorian" || span.TimeZone != "UTC" || span.TemporalType != "date" || binding.Resolution != span.ID || binding.Dataset != span.Dataset || binding.Column != span.Column || binding.Kind != "time_window" || binding.Operator != "range" || !reflect.DeepEqual(binding.Parameters, []int{1, 2}) {
		t.Fatal("raw source-amount query changed the exact owned UTC 2026 period")
	}
	if !strings.Contains(planned.SQL, "CAST($1 AS DATE)") || !strings.Contains(planned.SQL, "CAST($2 AS DATE)") {
		t.Fatal("final SQL did not retain parameterized service-owned date bounds")
	}
}

func (f *generatedRawSourceAmountFixture) report() *generatedTopicReport {
	return &generatedTopicReport{FixtureID: f.ID, FixtureDigest: f.digest, InterpretationPolicy: nlqroute.GroundedCalendarPolicy, GeneratedOnly: true, Stage: "not_started", Unsupported: []string{"Gross/net revenue and tax/discount/refund components are undefined; this fixture establishes only the sum of stored source amounts across all orders"}}
}

func (f *generatedRawSourceAmountFixture) independentTotals(t *testing.T, h *generatedTopicHarness) (string, map[string]string) {
	t.Helper()
	// Compute from source rows in Go, then also pin to fixed independent values.
	// Neither the generated candidate nor recorded/model SQL defines this oracle.
	total, months := generatedIndependentTotals(t, h.f, false)
	if total != f.ExpectedTotal || !reflect.DeepEqual(months, f.ExpectedMonths) {
		t.Fatal("raw source-amount rows no longer match the independent fixed numeric oracle")
	}
	return total, months
}

func generatedRawSourceAmountReview(p semantics.TopicPack) string {
	if reason := generatedBusinessReview(p, false); reason != "accepted_synthetic_business_contract" {
		return reason
	}
	if len(p.Measures) != 1 || len(p.Dimensions) != 3 || p.GroupDomain != nil || p.GroupedPopulation != nil || len(p.RelationshipDecisions) != 0 || len(p.CanonicalEntities) != 0 {
		return "unexpected_raw_amount_structure"
	}
	m := p.Measures[0]
	if m.Field.Dataset != p.Datasets[0].ID || m.Completeness != nil {
		return "unexpected_raw_amount_definition"
	}
	// Admit only bounded raw-contract descriptions, including truthful missing-
	// meaning disclosure. Unknown wording requires review; this is not a general
	// language classifier. The exact advisory remains required independently.
	switch m.Description {
	case generatedRawSourceAmountDescription, generatedRawSourceAmountDescription + ". " + generatedRawSourceAmountDisclosure, generatedRawSourceAmountMonetaryDescription:
	default:
		return "raw_amount_description_requires_review"
	}
	rawLabel := func(label string) bool {
		switch strings.ToLower(label) {
		case "raw order source amount", "raw source amount", "order source amount", "sum of orders.total_usd", "source amount", "total_usd":
			return true
		default:
			return false
		}
	}
	if !rawLabel(m.Name) {
		return "invented_raw_amount_business_meaning"
	}
	for _, alias := range m.Aliases {
		if !rawLabel(alias) {
			return "invented_raw_amount_business_meaning"
		}
	}
	identifier := false
	for _, d := range p.Dimensions {
		if d.Field.Dataset != p.Datasets[0].ID || len(d.Filters) != 0 {
			return "unexpected_raw_amount_dimension"
		}
		switch d.Field.ID {
		case "order_id":
			identifier = d.Role == semantics.DimensionIdentifier
		case "ordered_at":
			if d.Role != semantics.DimensionTemporal {
				return "unexpected_raw_amount_dimension"
			}
		case "status":
			if d.Role != semantics.DimensionCategorical {
				return "unexpected_raw_amount_dimension"
			}
		default:
			return "unexpected_raw_amount_dimension"
		}
	}
	if !identifier {
		return "raw_amount_order_grain_missing"
	}
	return "accepted_synthetic_business_contract"
}

// Recorded provider output enters the ordinary enhancement decoder. No generated
// candidate is edited after decoding, advisory review or publication.
func recordedRawSourceAmountStep(t *testing.T, columns []semantics.Reference) string {
	t.Helper()
	items := []any{}
	for _, ref := range columns {
		item := map[string]any{"dataset": ref.Dataset, "column": ref.ID, "aliases": []string{}}
		switch ref.ID {
		case "total_usd":
			item["kind"], item["name"] = "measure", "Raw order source amount"
			item["description"] = generatedRawSourceAmountDescription + ". " + generatedRawSourceAmountDisclosure
			item["aggregation"], item["unit"], item["semantic_role"] = "sum", "USD", "measure_input"
		case "ordered_at":
			item["kind"], item["name"], item["description"] = "dimension", "Order date", "Gregorian UTC order calendar date"
			item["role"], item["semantic_role"] = "temporal", "event_time"
			item["temporal"] = map[string]any{"calendar": "gregorian", "timezone": "UTC", "grains": []string{"month", "quarter", "year"}}
		case "order_id":
			item["kind"], item["name"], item["description"] = "dimension", "Order identifier", "One order row identifier"
			item["role"], item["semantic_role"], item["temporal"] = "identifier", "fact_key", nil
		case "status":
			item["kind"], item["name"], item["description"] = "dimension", "Order status", "Descriptive order state without a population restriction"
			item["role"], item["semantic_role"], item["temporal"] = "categorical", "attribute", nil
		default:
			t.Fatalf("unexpected raw source-amount column %s", ref.ID)
		}
		items = append(items, item)
	}
	body, _ := json.Marshal(map[string]any{"results": items})
	wire, _ := json.Marshal(map[string]any{"id": "recorded-raw-source-amount", "object": "chat.completion", "model": "model-enhance", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(body)}, "finish_reason": "stop"}}, "usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 20, "total_tokens": 30}})
	return "chat_raw:" + string(wire)
}

func TestGeneratedRawSourceAmountV1Contract(t *testing.T) {
	fixture := loadGeneratedRawSourceAmountV1(t)
	if fixture.ID != "generated-raw-source-amount-v1" || fixture.Topic != fixture.ID || fixture.Business == generatedOrdersBusiness || fixture.Business == generatedPaidBusiness {
		t.Fatal("raw source-amount input lost its separate identity")
	}
	if fixture.ExpectedTotal != "700.00" || !reflect.DeepEqual(fixture.ExpectedMonths, map[string]string{"2026-01": "320.00", "2026-02": "240.00", "2026-03": "140.00"}) || len(fixture.Queries) != 2 {
		t.Fatal("v1 fixed numeric oracle drifted")
	}
	for _, q := range fixture.Queries {
		if !strings.HasPrefix(q.ID, "raw-source-amount-") || !strings.Contains(q.Question, "raw order source amount") || strings.Contains(q.Question, "gross") {
			t.Fatal("raw source-amount question lost its separate identity")
		}
		for _, original := range generatedQueryCases() {
			if q.ID == original.id || q.Question == original.question {
				t.Fatal("original gross question was relabeled")
			}
		}
	}
}

func TestGeneratedRawSourceAmountV2PipelineRecorded(t *testing.T) {
	fixture := loadGeneratedRawSourceAmountV2(t)
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		// Match the existing generated-topic recorded fixture, unchanged.
		r := c.Roles["embedding"]
		r.MaxBatchItems, r.MaxBatchBytes = 64, 65536
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	report := fixture.report()
	h := newNamedGeneratedTopicHarness(t, model.engine, fixture.Business, report, fixture.Topic, fixture.Name)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		responses := []string{recordedRawSourceAmountStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_echo")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	})
	if current.Pack.Topic != fixture.Topic || current.Pack.Description != fixture.Business {
		t.Fatal("generated candidate lost the admitted raw source-amount input")
	}
	h.publishReviewed(t, current, report, generatedRawSourceAmountReview(current.Pack), "Synthetic operator checked only SUM(orders.total_usd), all rows, USD, order_id grain and Gregorian UTC calendar against this exact candidate; no source amount components are defined")
	total, months := fixture.independentTotals(t, h)
	h.runQueryCases(t, t.Context(), current, report, func(sql string) { model.mode.Store(phase18RawResponse(t, sql)) }, recordedRawSourceAmountV2Cases(t, fixture, report), total, months, report.InterpretationPolicy)
	if report.InputMode != "schema_profile_and_business_description" || report.InterpretationPolicy != nlqroute.GroundedCalendarPolicy || report.RecordingID != "generated-raw-source-amount-v2-sql-v2" || report.RecordingDigest != generatedRawSourceAmountV2SQLV2SHA256 {
		t.Fatal("raw source-amount receipt misstates the admitted input or interpretation policy")
	}
	if !report.Complete || len(report.Steps) != 2 || len(report.Queries) != 2 || report.FixtureID != fixture.ID || report.FixtureDigest != fixture.digest {
		t.Fatal("recorded raw source-amount pipeline or identity incomplete")
	}
	assertRecordedAuthoringEnvelopes(t, report)
	model.mu.Lock()
	bodies := strings.Join(model.requestBodies, "\n")
	requestedModels := append([]string(nil), model.models...)
	model.mu.Unlock()
	seenModels := map[string]bool{}
	for _, model := range requestedModels {
		seenModels[model] = true
	}
	for _, required := range []string{"model-enhance", "model-topic_review", "embedding-model", "model-rerank", "model-sqlgen"} {
		if !seenModels[required] {
			t.Fatal("recorded raw source-amount pipeline missed a required gateway stage", required)
		}
	}
	for _, input := range []string{fixture.Business, fixture.Queries[0].Question, fixture.Queries[1].Question} {
		if !strings.Contains(bodies, input) {
			t.Fatal("exact versioned input/question did not reach the recorded gateway")
		}
	}
	// The review gate rejects a new business claim without changing any candidate
	// used by the pipeline. This copy is only an isolated negative assertion.
	invented := current.Pack
	invented.Measures = append([]semantics.Measure(nil), current.Pack.Measures...)
	invented.Measures[0].Description = generatedRawSourceAmountMonetaryDescription
	if reason := generatedRawSourceAmountReview(invented); reason != "accepted_synthetic_business_contract" {
		t.Fatal("truthful missing-meaning disclosure was rejected", reason)
	}
	invented.Measures[0].Description = "Gross revenue including tax before discounts/refunds"
	if reason := generatedRawSourceAmountReview(invented); reason != "raw_amount_description_requires_review" {
		t.Fatal("positive business claim with a raw label was accepted", reason)
	}
	invented.Measures[0].Description = current.Pack.Measures[0].Description
	invented.Measures[0].Name = "Gross revenue"
	if generatedRawSourceAmountReview(invented) != "invented_raw_amount_business_meaning" {
		t.Fatal("raw amount review accepted invented gross meaning")
	}
}

// A recorded control for the same refusal class, not a replacement or rerun of
// the preserved live advisory evidence in the reconstruction qualification note.
func TestGeneratedOriginalGrossRefusalRecorded(t *testing.T) {
	model := newGatewayFixture(t, recordedLiveChatCaps)
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		responses := []string{recordedGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_ambiguous_amount")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	})
	if current.Pack.Description != generatedOrdersBusiness || current.Quality.Status != "needs_review" || len(current.Quality.Findings) != 1 || current.Quality.Findings[0].Code != "ambiguous_meaning" {
		t.Fatal("original gross input or recorded ambiguous-amount refusal changed")
	}
	if reason := generatedPublicationBlockReason(current, generatedBusinessReview(current.Pack, false)); reason != "model_advisory_requires_adjudication" {
		t.Fatal("original gross advisory no longer blocks the live harness publication gate", reason)
	}
	if _, err := h.topics.Read(t.Context(), h.author, current.Pack.Topic, ""); err == nil {
		t.Fatal("refused original gross candidate became published")
	}
	if report.PublicationDigest != "" || report.Complete || len(report.Queries) != 0 {
		t.Fatal("recorded refusal reached publication or query claims")
	}
	if reason := generatedRawSourceAmountReview(current.Pack); reason != "raw_amount_description_requires_review" {
		t.Fatal("original gross candidate was accepted as raw source amount", reason)
	}
}

func TestGeneratedRawSourceAmountV2Contract(t *testing.T) {
	previous := loadGeneratedRawSourceAmountV1(t)
	fixture := loadGeneratedRawSourceAmountV2(t)
	if fixture.ID != "generated-raw-source-amount-v2" || fixture.Topic != fixture.ID || fixture.Business != previous.Business || fixture.digest == previous.digest {
		t.Fatal("v2 lost its independent identity or changed the raw amount meaning")
	}
	if fixture.ExpectedTotal != previous.ExpectedTotal || !reflect.DeepEqual(fixture.ExpectedMonths, previous.ExpectedMonths) || len(fixture.Queries) != len(previous.Queries) {
		t.Fatal("v2 changed the fixed numeric oracle")
	}
	for i, q := range fixture.Queries {
		if q.ID == previous.Queries[i].ID || q.SQL != previous.Queries[i].SQL || q.Monthly != previous.Queries[i].Monthly {
			t.Fatal("v2 lost query identity or changed the recorded calculation")
		}
	}
	if fixture.Queries[1].Question == previous.Queries[1].Question || !strings.Contains(fixture.Queries[1].Question, "by UTC calendar month in 2026") {
		t.Fatal("v2 did not independently name supported UTC calendar wording")
	}
}
