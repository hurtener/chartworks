package drafts

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
)

func TestQualityReviewHandlesPreserveCanonicalReferences(t *testing.T) {
	coverage := []string{`column:["a:b","c"]`, `column:["a","b:c"]`, `topic:currency "quoted" 日本語`}
	candidate, context := strings.Repeat("a", 64), strings.Repeat("b", 64)
	transport, err := newQualityReviewTransport(candidate, context, coverage)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := gateway.NewStrictSchema(transport.schema)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	_ = json.Unmarshal(projection.Document(), &document)
	if quotedReviewEnum(document) {
		t.Fatal("observed provider quote restriction reproduced in wire schema")
	}
	if !quotedReviewEnumDocument(transport.domain.Document()) {
		t.Fatal("test lacks canonical quoted identities")
	}
	handles := make([]string, len(coverage))
	for i, r := range transport.references {
		handles[i] = r.Handle
		if r.Entity != coverage[i] {
			t.Fatal("mapping changed meaning")
		}
	}
	expected := qualityWire{CandidateDigest: candidate, ContextDigest: context, CoverageDigest: readexec.Hash(coverage), Status: "needs_review", Findings: []QualityFinding{{Code: "ambiguous_meaning", Entities: handles, Detail: "Preserve the advisory and exact identities."}}}
	raw, _ := json.Marshal(expected)
	normalized, err := projection.Normalize(raw, 65536)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := transport.decode(normalized)
	if err != nil {
		t.Fatal(err)
	}
	expected.Findings[0].Entities = coverage
	if !reflect.DeepEqual(restored, expected) {
		t.Fatal("handle translation changed advisory")
	}
	canonical, _ := json.Marshal(expected)
	if _, err := transport.decode(canonical); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("canonical IDs bypassed handle allowlist")
	}
	// Even a corrupted internal map cannot skip the original canonical schema.
	transport.entities[handles[0]] = "topic:outside"
	if _, err := transport.decode(raw); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("canonical domain schema was bypassed")
	}
}

func TestQualityReviewHandlesRejectUnknownAndStaleResponses(t *testing.T) {
	candidate, context := strings.Repeat("a", 64), strings.Repeat("b", 64)
	coverage := []string{`column:["orders","amount"]`}
	transport, err := newQualityReviewTransport(candidate, context, coverage)
	if err != nil {
		t.Fatal(err)
	}
	base := qualityWire{CandidateDigest: candidate, ContextDigest: context, CoverageDigest: readexec.Hash(coverage), Status: "needs_review", Findings: []QualityFinding{{Code: "ambiguous_meaning", Entities: []string{transport.references[0].Handle}, Detail: "Unresolved unit."}}}
	for name, alter := range map[string]func(*qualityWire){
		"unknown_handle": func(q *qualityWire) { q.Findings[0].Entities = []string{"e999999"} },
		"bare_column":    func(q *qualityWire) { q.Findings[0].Entities = []string{"orders.amount"} },
		"stale_context":  func(q *qualityWire) { q.ContextDigest = strings.Repeat("c", 64) },
		"stale_coverage": func(q *qualityWire) { q.CoverageDigest = strings.Repeat("d", 64) },
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var q qualityWire
			_ = json.Unmarshal(raw, &q)
			alter(&q)
			raw, _ = json.Marshal(q)
			if _, err := transport.decode(raw); !errors.Is(err, gateway.ErrOutput) {
				t.Fatal("invalid response accepted", err)
			}
		})
	}
	raw, _ := json.Marshal(base)
	other, err := newQualityReviewTransport(candidate, context, []string{`column:["private","amount"]`})
	if err != nil {
		t.Fatal(err)
	}
	if other.references[0].Handle != transport.references[0].Handle {
		t.Fatal("test must exercise colliding request-local handle")
	}
	if _, err := other.decode(raw); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("handle reused across a different bound request")
	}
	coverage[0] = "mutated caller"
	if _, err := transport.decode(raw); err != nil {
		t.Fatal("caller mutation changed request map", err)
	}
	if transport.domainDigest() == other.domainDigest() {
		t.Fatal("canonical schema identity does not bind mapping")
	}
	if _, err := transport.decode(append(raw, []byte(` {}`)...)); !errors.Is(err, gateway.ErrOutput) {
		t.Fatal("trailing response admitted")
	}
}

func quotedReviewEnumDocument(raw []byte) bool {
	var v any
	_ = json.Unmarshal(raw, &v)
	return quotedReviewEnum(v)
}
func quotedReviewEnum(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		for key, value := range x {
			if key == "enum" {
				for _, entry := range value.([]any) {
					if s, ok := entry.(string); ok && strings.ContainsRune(s, '"') {
						return true
					}
				}
			}
			if quotedReviewEnum(value) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if quotedReviewEnum(value) {
				return true
			}
		}
	}
	return false
}
