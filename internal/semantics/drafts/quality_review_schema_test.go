package drafts

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
)

func TestQualityReviewSchemaBindsEveryDigestAndEntity(t *testing.T) {
	model, profiles := authoringFixture(t)
	material, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	coverage := qualityCoverage(model.Pack())
	domain, err := qualityReviewSchema(model.Digest(), material.Digest, coverage)
	if err != nil {
		t.Fatal(err)
	}
	strict, err := gateway.NewStrictSchema(domain)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := gateway.NewSchema("former_review", []byte(qualitySchemaTemplate))
	if err != nil {
		t.Fatal(err)
	}
	answer := qualityWire{CandidateDigest: model.Digest(), ContextDigest: material.Digest, CoverageDigest: readexec.Hash(coverage), Status: "needs_review", Findings: []QualityFinding{{Code: "incomplete_evidence", Entities: append([]string(nil), coverage[:min(32, len(coverage))]...), Detail: "A declared grain is not a physical uniqueness guarantee."}}}
	raw, _ := json.Marshal(answer)
	normalized, err := strict.Normalize(raw, 65536)
	if err != nil {
		t.Fatal("exact request references rejected", err)
	}
	var actual qualityWire
	if json.Unmarshal(normalized, &actual) != nil || !reflect.DeepEqual(answer, actual) {
		t.Fatal("valid advisory changed")
	}
	for name, alter := range map[string]func(*qualityWire){
		"candidate_drift": func(q *qualityWire) { q.CandidateDigest = strings.Repeat("a", 64) },
		"context_drift":   func(q *qualityWire) { q.ContextDigest = strings.Repeat("b", 64) },
		"coverage_drift":  func(q *qualityWire) { q.CoverageDigest = strings.Repeat("c", 64) },
		"bare_measure":    func(q *qualityWire) { q.Findings[0].Entities = []string{model.Pack().Measures[0].ID} },
		"column_shorthand": func(q *qualityWire) {
			d := model.Pack().Datasets[0]
			q.Findings[0].Entities = []string{d.ID + "." + d.Columns[0].ID}
		},
		"unknown_qualified_entity": func(q *qualityWire) { q.Findings[0].Entities = []string{"dataset:other_context"} },
	} {
		t.Run(name, func(t *testing.T) {
			var changed qualityWire
			_ = json.Unmarshal(raw, &changed)
			alter(&changed)
			bad, _ := json.Marshal(changed)
			if legacy.Validate(bad, 65536) != nil {
				t.Fatal("regression does not reproduce the former static-schema gap")
			}
			if !errors.Is(domain.Validate(bad, 65536), gateway.ErrOutput) {
				t.Fatal("domain schema admitted drift")
			}
			if _, err := strict.Normalize(bad, 65536); !errors.Is(err, gateway.ErrOutput) {
				t.Fatal("provider projection admitted drift", err)
			}
		})
	}
	// A valid advisory must still pass the unchanged domain consumer; it is not an
	// approval and its findings must survive augmentation intact.
	service := Service{engine: qualityTestEngine{generate: func(_ string, sent *gateway.Schema) (gateway.Generated, error) {
		transport, err := newQualityReviewTransport(model.Digest(), material.Digest, coverage)
		if err != nil || string(sent.Document()) != string(transport.schema.Document()) {
			t.Fatal("consumer sent a different transport schema", err)
		}
		return gateway.Generated{JSON: raw}, nil
	}}}
	review, _, err := service.reviewAuthoringCandidate(t.Context(), gateway.Call{}, nil, model, material)
	if err != nil || review == nil || !review.ValidFor(model) || review.Status != "needs_review" || !reflect.DeepEqual(review.Findings[0], answer.Findings[0]) {
		t.Fatal("valid needs-review advisory lost", review, err)
	}
}

func TestQualityReviewSchemaConcurrentRequestIsolation(t *testing.T) {
	model, profiles := authoringFixture(t)
	material, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	coverage := qualityCoverage(model.Pack())
	schema, err := qualityReviewSchema(model.Digest(), material.Digest, coverage)
	if err != nil {
		t.Fatal(err)
	}
	before := string(schema.Document())
	errs := make(chan error, 16)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scope := []string{fmt.Sprintf("topic:request_%d", i)}
			domain, e := qualityReviewSchema(model.Digest(), material.Digest, scope)
			if e != nil {
				errs <- e
				return
			}
			scope[0] = "topic:mutated"
			q := qualityWire{CandidateDigest: model.Digest(), ContextDigest: material.Digest, CoverageDigest: readexec.Hash([]string{fmt.Sprintf("topic:request_%d", i)}), Status: "needs_review", Findings: []QualityFinding{{Code: "missing_description", Entities: []string{fmt.Sprintf("topic:request_%d", i)}, Detail: "Missing declared meaning."}}}
			raw, _ := json.Marshal(q)
			if e = domain.Validate(raw, 65536); e != nil {
				errs <- e
				return
			}
			if schema.Validate(raw, 65536) == nil {
				errs <- errors.New("other request admitted")
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if string(schema.Document()) != before {
		t.Fatal("shared schema mutated")
	}
}

func TestQualityReviewSchemaCompleteEnvelopeAndBounds(t *testing.T) {
	model, profiles := authoringFixture(t)
	material, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	coverage := qualityCoverage(model.Pack())
	schema, err := qualityReviewSchema(model.Digest(), material.Digest, coverage)
	if err != nil {
		t.Fatal(err)
	}
	transport, err := newQualityReviewTransport(model.Digest(), material.Digest, coverage)
	if err != nil {
		t.Fatal(err)
	}
	if string(transport.domain.Document()) != string(schema.Document()) {
		t.Fatal("canonical schema drift")
	}
	input, _ := json.Marshal(map[string]any{"context": material, "coverage": coverage, "coverage_digest": readexec.Hash(coverage), "review_reference_encoding": "request-local-handles-v1", "canonical_review_schema_digest": transport.domainDigest(), "entity_references": transport.references})
	envelope, err := gateway.NewPromptEnvelope("openrouter", "test/pinned-model", qualityReviewInstructions, "", transport.schema, 65536, 65536, 1024, 4096)
	if err != nil {
		t.Fatal(err)
	}
	usage, fits, err := envelope.Measure(string(input))
	if err != nil || !fits {
		t.Fatal("complete request-bound review does not fit", usage, err)
	}
	t.Logf("complete review reservation=%d request=%d protocol=%d output=%d", usage.InputUpperBound+usage.OutputReserve, usage.RequestBytes, usage.ProtocolReserve, usage.OutputReserve)
	loose, _ := gateway.NewSchema("topic_quality_review", []byte(qualitySchemaTemplate))
	old, _ := gateway.NewPromptEnvelope("openrouter", "test/pinned-model", qualityReviewInstructions, "", loose, 65536, 65536, 1024, 4096)
	oldUsage, _, _ := old.Measure(string(input))
	if usage.RequestBytes <= oldUsage.RequestBytes || usage.DomainSchemaDigest == oldUsage.DomainSchemaDigest || usage.WireSchemaDigest == oldUsage.WireSchemaDigest {
		t.Fatal("request bindings escaped envelope accounting")
	}
	oversized := make([]string, 1001)
	for i := range oversized {
		oversized[i] = fmt.Sprintf("column:%d", i)
	}
	bound, err := qualityReviewSchema(model.Digest(), material.Digest, oversized)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = gateway.NewStrictSchema(bound); !errors.Is(err, gateway.ErrInput) {
		t.Fatal("provider enum limit widened", err)
	}
	oversized = []string{strings.Repeat("x", 65536)}
	if _, err = qualityReviewSchema(model.Digest(), material.Digest, oversized); !errors.Is(err, gateway.ErrBudget) {
		t.Fatal("oversized binding silently truncated", err)
	}
	for _, bad := range []string{"", strings.Repeat("F", 64), "not-a-digest"} {
		if _, err := qualityReviewSchema(bad, material.Digest, coverage); !errors.Is(err, gateway.ErrInput) {
			t.Fatal("invalid digest admitted")
		}
	}
	if _, err := qualityReviewSchema(model.Digest(), material.Digest, nil); !errors.Is(err, gateway.ErrInput) {
		t.Fatal("empty coverage admitted")
	}
}
