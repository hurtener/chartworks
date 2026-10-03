package nlq

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSharedMetricDefinitionsPreserveExactCoverageAndBudget(t *testing.T) {
	input := ContextInput{Locale: LanguageEnglish, Strategy: StrategySingleTopic, Topic: "sales", TopicVersion: "v3", Question: "Compare three reviewed metrics"}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("metric%d", i)
		metric := PinnedMetric{ID: "sales:" + id, Text: "Reviewed metric " + id, Dependencies: []MetricDependency{{Kind: "kpi", ID: id, Text: "exact root formula " + id}}}
		for j := 0; j < 10; j++ {
			metric.Dependencies = append(metric.Dependencies, MetricDependency{Kind: "column", ID: fmt.Sprintf("orders:column%d", j), Text: fmt.Sprintf("DEFINITION_%d ", j) + strings.Repeat("reviewed exact population calendar relationship meaning ", 55)})
		}
		input.Metrics = append(input.Metrics, metric)
	}
	before, _ := json.Marshal(input)
	beforeHash := sha256.Sum256(before)
	legacy := renderMetrics(input.Metrics)
	shared, err := metricRendering(input)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if strings.Count(shared, fmt.Sprintf("DEFINITION_%d ", i)) != 1 {
			t.Fatal("missing or repeated exact definition", i)
		}
	}
	assertMetricRenderingClosure(t, input, shared)
	if strings.Count(shared, "metric_dependency_definition[") != 13 {
		t.Fatal("typed dependency identity lost")
	}
	if !strings.Contains(shared, `"topic":"sales","version":"v3"`) {
		t.Fatal("definition version absent")
	}
	after, _ := json.Marshal(input)
	if sha256.Sum256(after) != beforeHash {
		t.Fatal("render changed source evidence digest")
	}
	counter, err := NewTiktokenCounter()
	if err != nil {
		t.Fatal(err)
	}
	oldTokens, _ := counter.Count(legacy)
	newTokens, _ := counter.Count(shared)
	t.Logf("mandatory metric bodies: old_bytes=%d old_tokens=%d shared_bytes=%d shared_tokens=%d high_budget=%d", len(legacy), oldTokens, len(shared), newTokens, HighBudget)
	if oldTokens <= HighBudget || newTokens >= HighBudget || newTokens >= oldTokens {
		t.Fatal("bounded body sharing did not resolve duplicate-only overflow")
	}
	assembler, _ := NewDefaultContextAssembler()
	assembled, err := assembler.Assemble(context.Background(), input, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := cloneMetricsChecked(input.Metrics, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(assembled.Metrics, canonical) || assembled.Budget != HighBudget {
		t.Fatal("semantic edges changed or tier widened")
	}
}

func TestSharedMetricDefinitionsNeverMergeForeignOrConflictingIdentity(t *testing.T) {
	root := func(topic, id, body string) PinnedMetric {
		return PinnedMetric{ID: topic + ":" + id, Text: id, Dependencies: []MetricDependency{{Kind: "kpi", ID: id, Text: "root " + id}, {Kind: "measure", ID: "shared", Text: body}}}
	}
	input := ContextInput{Locale: LanguageEnglish, Strategy: StrategyMultiTopic, Topic: "sales", TopicVersion: "v1", Topics: []TopicRevision{{Topic: "sales", Version: "v1"}, {Topic: "returns", Version: "v2"}}, Question: "Compare", Metrics: []PinnedMetric{root("sales", "one", "SALES_BODY"), root("returns", "two", "RETURNS_BODY")}}
	rendered, err := metricRendering(input)
	if err != nil || !strings.Contains(rendered, "SALES_BODY") || !strings.Contains(rendered, "RETURNS_BODY") {
		t.Fatal("foreign same-ID meaning suppressed", err)
	}
	input.Strategy = StrategySingleTopic
	input.Topics = nil
	input.Metrics = []PinnedMetric{root("sales", "one", "FIRST_BODY"), root("sales", "two", "CONFLICTING_BODY")}
	assembler, _ := NewDefaultContextAssembler()
	if _, err := assembler.Assemble(context.Background(), input, TierHigh); !errors.Is(err, ErrInvalid) {
		t.Fatal("conflicting same-version dependency admitted", err)
	}
	input.Metrics[1].Dependencies[1].Text = "FIRST_BODY"
	rendered, err = metricRendering(input)
	if err != nil || strings.Count(rendered, "FIRST_BODY") != 1 {
		t.Fatal("equal body did not share", err)
	}
	unshared := input
	unshared.Metrics = []PinnedMetric{root("sales", "one", "ONE"), root("sales", "two", "TWO")}
	unshared.Metrics[0].Dependencies[1].ID = "one-only"
	unshared.Metrics[1].Dependencies[1].ID = "two-only"
	if rendered, err := metricRendering(unshared); err != nil || rendered != renderMetrics(unshared.Metrics) {
		t.Fatal("multiple unshared roots changed historical rendering", err)
	}
	input.Metrics = input.Metrics[:1]
	rendered, err = metricRendering(input)
	if err != nil || rendered != renderMetrics(input.Metrics) {
		t.Fatal("single-root legacy rendering changed", err)
	}
}

// Decode the compact wire representation back to each exact root closure. This
// detects missing, swapped, foreign-version or spurious references rather than
// treating presence of a reference line as sufficient coverage.
func assertMetricRenderingClosure(t *testing.T, input ContextInput, rendered string) {
	t.Helper()
	type namespace struct {
		Topic   string `json:"topic"`
		Version string `json:"version"`
		Root    string `json:"unscoped_root"`
	}
	type definition struct {
		identity []string
		body     string
	}
	namespaces := map[string]namespace{}
	definitions := map[string]definition{}
	refs := map[string][]string{}
	for _, line := range strings.Split(rendered, "\n") {
		prefix, rest, ok := strings.Cut(line, "[")
		if !ok {
			continue
		}
		id, suffix, ok := strings.Cut(rest, "]")
		if !ok {
			t.Fatal("invalid rendered identity")
		}
		switch prefix {
		case "metric_namespace":
			var n namespace
			if json.Unmarshal([]byte(strings.TrimPrefix(suffix, ":")), &n) != nil {
				t.Fatal("invalid namespace")
			}
			if _, exists := namespaces[id]; exists {
				t.Fatal("duplicate namespace")
			}
			namespaces[id] = n
		case "metric_dependency_definition":
			identity, body, ok := strings.Cut(strings.TrimPrefix(suffix, " identity:"), " value:")
			var parts []string
			if !ok || json.Unmarshal([]byte(identity), &parts) != nil || len(parts) != 3 {
				t.Fatal("invalid definition identity")
			}
			if _, exists := definitions[id]; exists {
				t.Fatal("duplicate definition")
			}
			definitions[id] = definition{parts, body}
		case "metric_dependency_refs":
			var values []string
			if json.Unmarshal([]byte(strings.TrimPrefix(suffix, ":")), &values) != nil {
				t.Fatal("invalid root references")
			}
			if _, exists := refs[id]; exists {
				t.Fatal("duplicate root reference list")
			}
			refs[id] = values
		}
	}
	if len(refs) != len(input.Metrics) {
		t.Fatal("root coverage changed")
	}
	used := map[string]bool{}
	for _, root := range input.Metrics {
		topic, version := metricDefinitionScope(input, root)
		actual := refs[root.ID]
		if len(actual) != len(root.Dependencies) {
			t.Fatal("root dependency count changed", root.ID)
		}
		for i, id := range actual {
			d, ok := definitions[id]
			if !ok {
				t.Fatal("dangling root dependency", id)
			}
			n, ok := namespaces[d.identity[0]]
			want := root.Dependencies[i]
			if !ok || n.Topic != topic || n.Version != version || topic == "" && n.Root != root.ID || d.identity[1] != want.Kind || d.identity[2] != want.ID || d.body != want.Text {
				t.Fatal("exact root-to-versioned-definition edge changed", root.ID, id)
			}
			used[id] = true
		}
	}
	if len(used) != len(definitions) {
		t.Fatal("unreferenced extra definition")
	}
}
