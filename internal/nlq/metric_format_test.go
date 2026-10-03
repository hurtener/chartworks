package nlq

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func metricFormatFixture(shared bool) ContextInput {
	input := ContextInput{Locale: LanguageEnglish, Strategy: StrategySingleTopic, Topic: "sales", TopicVersion: "v3", Question: "Compare reviewed sum and unknown count", Topics: []TopicRevision{{Topic: "sales", Version: "v3"}}}
	for _, root := range []string{"total", "unknown"} {
		sharedID := "orders:amount"
		if !shared {
			sharedID += "_" + root
		}
		input.Metrics = append(input.Metrics, PinnedMetric{ID: "sales:" + root, Text: "Reviewed " + root, Dependencies: []MetricDependency{
			{Kind: "column", ID: sharedID, Text: `{"dataset":"orders","column":{"id":"amount","source_name":"amount_usd","nullable":true},"meaning":"USD; unknown remains NULL"}`},
			{Kind: "measure", ID: root, Text: `{"id":"` + root + `","aggregation":"` + map[string]string{"total": "sum", "unknown": "count"}[root] + `","scope":"paid orders","keys":["division","order_id"],"filter":"status = P","meaning":"` + root + `"}`},
		}})
	}
	return input
}

// These synthetic goldens were captured with the unmodified production v1
// renderer at 41fe296, before adding MetricFormat. They are not recovered data.
func TestMetricFormatLegacyGoldenCompatibility(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	seals := map[string]string{"nonshared": "0ac064c7c655a030204cd86b331fb0bd4f6b47a9ac0006873d217d8ebfedd1f8", "shared": "8a2adbe2585c8e629bb164f5cdb570fe0e64b8f2f6560de742674a2ca38b271a"}
	for _, shared := range []bool{false, true} {
		kind := "nonshared"
		if shared {
			kind = "shared"
		}
		got, err := a.Assemble(context.Background(), metricFormatFixture(shared), TierHigh)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(got)
		before, err := os.ReadFile("testdata/metric-format/legacy-" + kind + ".json")
		if err != nil {
			t.Fatal(err)
		}
		prompt, err := os.ReadFile("testdata/metric-format/legacy-" + kind + ".prompt")
		if err != nil {
			t.Fatal(err)
		}
		if string(raw) != string(before) || got.Prompt != string(prompt) || fmt.Sprintf("%x", got.seal) != seals[kind] {
			t.Fatal("historical bytes or seal changed", kind)
		}
		if got.MetricFormat != MetricFormatLegacyV1 {
			t.Fatal("omitted format upgraded")
		}
		t.Logf("legacy %s JSON SHA256=%x prompt SHA256=%x tokens=%d", kind, sha256.Sum256(raw), sha256.Sum256(prompt), got.Tokens)
	}
}
