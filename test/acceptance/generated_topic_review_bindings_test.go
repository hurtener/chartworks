package acceptance

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func recordedQualityAnswer(t *testing.T, mode string, context, material map[string]any) map[string]any {
	t.Helper()
	answer := map[string]any{"candidate_digest": context["candidate_digest"], "context_digest": context["digest"], "coverage_digest": material["coverage_digest"], "status": "no_findings", "findings": []any{}}
	if mode == "topic_quality_echo" {
		return answer
	}
	coverage, _ := material["coverage"].([]any)
	for _, v := range coverage {
		entity, _ := v.(string)
		if !strings.HasPrefix(entity, "measure:") {
			continue
		}
		if mode == "topic_quality_unqualified" {
			entity = strings.TrimPrefix(entity, "measure:")
		} else {
			for _, raw := range material["entity_references"].([]any) {
				ref := raw.(map[string]any)
				if ref["entity"] == entity {
					entity = ref["handle"].(string)
					break
				}
			}
		}
		answer["status"] = "needs_review"
		answer["findings"] = []any{map[string]any{"code": "incomplete_evidence", "entities": []string{entity}, "detail": "An operator must verify that the declared all-order gross definition matches the intended business requirement."}}
		return answer
	}
	t.Error("recorded quality fixture lacks a measure reference")
	return answer
}

func TestGeneratedTopicReviewReferencesRecorded(t *testing.T) {
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems, r.MaxBatchBytes = 64, 65536
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	current := h.generate(t, report, func(columns []semantics.Reference, last bool) {
		responses := []string{recordedGeneratedStep(t, columns)}
		if last {
			responses = append(responses, "topic_quality_findings")
		}
		model.mu.Lock()
		model.chatSequence = responses
		model.mu.Unlock()
	})
	if current.Quality.Status != "needs_review" || len(current.Quality.Findings) != 1 {
		t.Fatal("provider advisory changed or vanished")
	}
	retained, err := h.client.TopicDraftVersion(t.Context(), current.Pack.Topic, current.Metadata.Revision)
	if err != nil || !reflect.DeepEqual(retained.Quality, current.Quality) {
		t.Fatal("exact advisory not retained", err)
	}
	if generatedBusinessReview(current.Pack, false) != "accepted_synthetic_business_contract" {
		t.Fatal("independent business oracle rejected candidate")
	}
	// This explicitly exercises the existing operator path. Model findings remain
	// unchanged; an advisory is neither automatic approval nor a publication veto.
	review, err := h.client.ReviewTopic(t.Context(), current.Pack.Topic, sdk.TopicReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Synthetic operator adjudicated the retained advisory against the independent all-order gross and UTC Gregorian oracle; candidate unchanged."})
	if err != nil {
		t.Fatal("explicit operator review", err)
	}
	published, err := h.client.PublishTopic(t.Context(), current.Pack.Topic, sdk.PublishTopicRequest{Review: review.ID})
	if err != nil || published.Digest != current.Metadata.Digest {
		t.Fatal("exact reviewed publication", err)
	}
	h.runQueries(t, t.Context(), current, report, func(sql string) { model.mode.Store(phase18RawResponse(t, sql)) })
	if !report.Complete || len(report.Queries) != 2 {
		t.Fatal("reviewed candidate did not reach independent result oracles")
	}
	retained, err = h.client.TopicDraftVersion(t.Context(), current.Pack.Topic, current.Metadata.Revision)
	if err != nil || !reflect.DeepEqual(retained.Quality, current.Quality) {
		t.Fatal("publication rewrote advisory", err)
	}
	model.mu.Lock()
	bodies := append([]string(nil), model.requestBodies...)
	model.mu.Unlock()
	reviews := 0
	for _, body := range bodies {
		if assertRecordedQualityBinding(t, body) {
			reviews++
		}
	}
	if reviews != 1 {
		t.Fatal("missing exact review wire", reviews)
	}
	assertRecordedAuthoringEnvelopes(t, report)
}

func TestGeneratedTopicInvalidReviewDoesNotSave(t *testing.T) {
	model := newGatewayFixture(t, recordedLiveChatCaps)
	report := &generatedTopicReport{GeneratedOnly: true}
	h := newGeneratedTopicHarness(t, model.engine, generatedOrdersBusiness, report)
	compiled, err := semantics.Compile(h.scaffold.Pack)
	if err != nil {
		t.Fatal(err)
	}
	columns := semantics.GenerationColumns(compiled)
	if len(columns) != 4 {
		t.Fatal("synthetic column count changed")
	}
	model.mode.Store(recordedGeneratedStep(t, columns[:2]))
	first, err := h.client.EnhanceTopicDraft(t.Context(), h.scaffold.Pack.Topic, sdk.EnhanceTopicRequest{Expected: h.scaffold.Metadata.Revision, Version: "first", Cursor: 0, Limit: 2, Change: "Synthetic first page"})
	if err != nil || first.Complete {
		t.Fatal("first checkpoint", err)
	}
	model.mu.Lock()
	model.chatSequence = []string{recordedGeneratedStep(t, columns[2:]), "topic_quality_unqualified"}
	model.mu.Unlock()
	before := model.requests.Load()
	_, err = h.client.EnhanceTopicDraft(t.Context(), h.scaffold.Pack.Topic, sdk.EnhanceTopicRequest{Expected: first.Draft.Metadata.Revision, Version: "invalid-review", Cursor: 2, Limit: 2, Change: "Synthetic unqualified review reference"})
	if err == nil || model.requests.Load() != before+2 {
		t.Fatal("invalid advisory admitted or retried", err)
	}
	head, err := h.client.TopicDraft(t.Context(), h.scaffold.Pack.Topic)
	if err != nil || head.Metadata.Revision != first.Draft.Metadata.Revision || head.Metadata.Digest != first.Draft.Metadata.Digest || head.Quality != nil {
		t.Fatal("invalid advisory changed durable checkpoint", err)
	}
}

func assertRecordedQualityBinding(t *testing.T, body string) bool {
	t.Helper()
	var wire map[string]any
	if json.Unmarshal([]byte(body), &wire) != nil {
		t.Fatal("recorded request decode")
	}
	format, _ := wire["response_format"].(map[string]any)
	wrapper, _ := format["json_schema"].(map[string]any)
	if wrapper["name"] != "topic_quality_review" {
		return false
	}
	root := wrapper["schema"].(map[string]any)
	props := root["properties"].(map[string]any)
	var input map[string]any
	for _, v := range wire["messages"].([]any) {
		m := v.(map[string]any)
		if m["role"] == "user" {
			if json.Unmarshal([]byte(m["content"].(string)), &input) != nil {
				t.Fatal("recorded review context")
			}
		}
	}
	context := input["context"].(map[string]any)
	for key, expected := range map[string]any{"candidate_digest": context["candidate_digest"], "context_digest": context["digest"], "coverage_digest": input["coverage_digest"]} {
		s := recordedStrictReferences(t, props[key].(map[string]any), root)
		if !reflect.DeepEqual(s["enum"], []any{expected}) {
			t.Fatal("wire digest is not request-bound", key)
		}
	}
	findings := recordedStrictReferences(t, props["findings"].(map[string]any), root)
	item := recordedStrictReferences(t, findings["items"].(map[string]any), root)
	entities := recordedStrictReferences(t, item["properties"].(map[string]any)["entities"].(map[string]any), root)
	entity := recordedStrictReferences(t, entities["items"].(map[string]any), root)
	handles := []any{}
	canonical := []any{}
	for _, raw := range input["entity_references"].([]any) {
		ref := raw.(map[string]any)
		handles = append(handles, ref["handle"])
		canonical = append(canonical, ref["entity"])
	}
	if !reflect.DeepEqual(entity["enum"], handles) || !reflect.DeepEqual(canonical, input["coverage"]) {
		t.Fatal("wire lost complete handle-to-coverage mapping")
	}
	if input["review_reference_encoding"] != "request-local-handles-v1" || len(input["canonical_review_schema_digest"].(string)) != 64 {
		t.Fatal("canonical binding omitted from measured request")
	}
	return true
}

// This 400 fixture is deliberately faithful to the observed provider limitation.
// A subsequent successful full role call must use a quote-free handle enum,
// rather than weakening strictness or rewriting canonical semantic identifiers.
func TestGeneratedTopicReviewQuotedEnumProviderContract(t *testing.T) {
	f := newGatewayFixture(t, recordedLiveChatCaps)
	f.mode.Store("topic_quality_echo")
	schema, err := gateway.NewSchema("topic_quality_review", []byte(`{"type":"object","additionalProperties":false,"required":["entity"],"properties":{"entity":{"type":"string","enum":["column:[\"orders\",\"amount\"]"]}}}`))
	if err != nil {
		t.Fatal(err)
	}
	before := f.requests.Load()
	out, err := f.engine.Generate(t.Context(), f.call, gatewayBudget(t, f.call, 1), "topic_review", "Synthetic strict enum contract", "Synthetic request", schema)
	if !errors.Is(err, gateway.ErrUnavailable) || len(out.JSON) != 0 || f.requests.Load() != before+1 {
		t.Fatal("quoted enum restriction was not reproduced without retries", err)
	}
}

func recordedQualityQuotedEnums(value any) bool {
	switch x := value.(type) {
	case map[string]any:
		for key, child := range x {
			if key == "enum" {
				if entries, ok := child.([]any); ok {
					for _, v := range entries {
						if s, ok := v.(string); ok && strings.ContainsRune(s, '"') {
							return true
						}
					}
				}
			}
			if recordedQualityQuotedEnums(child) {
				return true
			}
		}
	case []any:
		for _, child := range x {
			if recordedQualityQuotedEnums(child) {
				return true
			}
		}
	}
	return false
}
