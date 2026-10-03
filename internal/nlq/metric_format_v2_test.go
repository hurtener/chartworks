package nlq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type decodedMetricNamespace struct {
	Topic   string `json:"topic"`
	Version string `json:"version"`
	Root    string `json:"unscoped_root"`
}
type decodedMetricDefinition struct{ Namespace, Kind, ID, Body string }
type decodedMetricWire struct {
	Roots       map[string]string
	Refs        map[string][]string
	Namespaces  map[string]decodedMetricNamespace
	Definitions map[string]decodedMetricDefinition
}

// Independent framing decoder: bodies are sliced by their declared UTF-8 byte
// count, never searched for delimiters, interpreted as JSON, or reserialized.
func decodeMetricV2(value string) (decodedMetricWire, error) {
	out := decodedMetricWire{map[string]string{}, map[string][]string{}, map[string]decodedMetricNamespace{}, map[string]decodedMetricDefinition{}}
	header, rest, ok := strings.Cut(value, "\n")
	if !ok || header != "metric_definitions:shared-versioned-v2; each root requires all refs; dN:[namespace,kind,entity_id,utf8_bytes] exact_body" {
		return out, fmt.Errorf("header")
	}
	for rest != "" {
		definition := strings.HasPrefix(rest, "d")
		var prefix, tail, id string
		var ok bool
		if definition {
			id, tail, ok = strings.Cut(rest, ":")
			if !ok || len(id) < 2 {
				return out, fmt.Errorf("definition key")
			}
			if _, err := strconv.Atoi(id[1:]); err != nil {
				return out, err
			}
			prefix = "definition"
		} else {
			prefix, tail, ok = strings.Cut(rest, "[")
			if !ok {
				return out, fmt.Errorf("prefix")
			}
			id, tail, ok = strings.Cut(tail, "]:")
			if !ok {
				return out, fmt.Errorf("identity")
			}
		}
		if prefix == "definition" {
			var parts []json.RawMessage
			d := json.NewDecoder(strings.NewReader(tail))
			if err := d.Decode(&parts); err != nil || len(parts) != 4 {
				return out, fmt.Errorf("definition tuple")
			}
			var v decodedMetricDefinition
			var n int
			if json.Unmarshal(parts[0], &v.Namespace) != nil || json.Unmarshal(parts[1], &v.Kind) != nil || json.Unmarshal(parts[2], &v.ID) != nil || json.Unmarshal(parts[3], &n) != nil || n < 0 {
				return out, fmt.Errorf("definition values")
			}
			pos := int(d.InputOffset())
			if pos >= len(tail) || tail[pos] != ' ' || n > len(tail)-pos-2 || tail[pos+1+n] != '\n' {
				return out, fmt.Errorf("body framing")
			}
			v.Body = tail[pos+1 : pos+1+n]
			if _, exists := out.Definitions[id]; exists {
				return out, fmt.Errorf("duplicate definition")
			}
			out.Definitions[id] = v
			rest = tail[pos+2+n:]
			continue
		}
		line, next, ok := strings.Cut(tail, "\n")
		if !ok {
			return out, io.ErrUnexpectedEOF
		}
		rest = next
		switch prefix {
		case "metrics":
			if _, ok := out.Roots[id]; ok {
				return out, fmt.Errorf("duplicate root")
			}
			out.Roots[id] = line
		case "refs":
			var refs []string
			if json.Unmarshal([]byte(line), &refs) != nil {
				return out, fmt.Errorf("refs")
			}
			if _, ok := out.Refs[id]; ok {
				return out, fmt.Errorf("duplicate refs")
			}
			out.Refs[id] = refs
		case "namespace":
			var ns decodedMetricNamespace
			if json.Unmarshal([]byte(line), &ns) != nil {
				return out, fmt.Errorf("namespace")
			}
			if _, ok := out.Namespaces[id]; ok {
				return out, fmt.Errorf("duplicate namespace")
			}
			out.Namespaces[id] = ns
		default:
			return out, fmt.Errorf("unexpected prefix")
		}
	}
	return out, nil
}

func assertMetricV2RoundTrip(t *testing.T, input ContextInput, expected map[string]decodedMetricNamespace) {
	t.Helper()
	wire, err := metricRendering(input)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeMetricV2(wire)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Roots) != len(input.Metrics) || len(decoded.Refs) != len(input.Metrics) {
		t.Fatal("selected roots changed")
	}
	usedDefs, usedNS := map[string]bool{}, map[string]bool{}
	for _, root := range input.Metrics {
		refs := decoded.Refs[root.ID]
		if decoded.Roots[root.ID] != root.Text || len(refs) != len(root.Dependencies) {
			t.Fatal("root text or edge count changed", root.ID)
		}
		for i, key := range refs {
			got, ok := decoded.Definitions[key]
			want := root.Dependencies[i]
			if !ok || got.Kind != want.Kind || got.ID != want.ID || got.Body != want.Text || decoded.Namespaces[got.Namespace] != expected[root.ID] {
				t.Fatal("exact namespace/body/edge changed", root.ID, key)
			}
			usedDefs[key] = true
			usedNS[got.Namespace] = true
		}
	}
	if len(usedDefs) != len(decoded.Definitions) || len(usedNS) != len(decoded.Namespaces) {
		t.Fatal("unreferenced definition or namespace")
	}
}

func TestMetricFormatV2LosslessFramingAndNamespaceIsolation(t *testing.T) {
	input := metricFormatFixture(true)
	input.MetricFormat = MetricFormatSharedV2
	input.Strategy = StrategyMultiTopic
	input.Topics = append(input.Topics, TopicRevision{Topic: "returns", Version: "v8"})
	// Identical labels and bodies in another topic retain another namespace.
	foreign := cloneMetrics(input.Metrics)
	for i := range foreign {
		foreign[i].ID = strings.Replace(foreign[i].ID, "sales:", "returns:", 1)
	}
	input.Metrics = append(input.Metrics, foreign...)
	// Unscoped roots must never borrow a topic namespace or each other's identity.
	for _, id := range []string{"legacy-one", "legacy-two"} {
		input.Metrics = append(input.Metrics, PinnedMetric{ID: id, Text: "Legacy root " + id, Dependencies: []MetricDependency{{Kind: "column", ID: "orders:amount", Text: "foreign opaque definition"}, {Kind: "measure", ID: "count", Text: "COUNT all events including NULL amount"}}})
	}
	// Pure-renderer framing exercise. The assembler still rejects multiline text;
	// this does not widen the accepted text contract.
	body := "  exact µ body\nnamespace[n99]:{}\ndefinition[d99]:[\"fake\"] value: []\r\n  "
	for i := 0; i < 4; i++ {
		input.Metrics[i].Dependencies[0].Text = body
	}
	expected := map[string]decodedMetricNamespace{}
	for _, m := range input.Metrics {
		switch {
		case strings.HasPrefix(m.ID, "sales:"):
			expected[m.ID] = decodedMetricNamespace{Topic: "sales", Version: "v3"}
		case strings.HasPrefix(m.ID, "returns:"):
			expected[m.ID] = decodedMetricNamespace{Topic: "returns", Version: "v8"}
		default:
			expected[m.ID] = decodedMetricNamespace{Root: m.ID}
		}
	}
	before, _ := json.Marshal(input.Metrics)
	assertMetricV2RoundTrip(t, input, expected)
	after, _ := json.Marshal(input.Metrics)
	if string(before) != string(after) {
		t.Fatal("semantic bodies mutated")
	}
	a, _ := NewDefaultContextAssembler()
	if _, err := a.Assemble(context.Background(), input, TierHigh); !errors.Is(err, ErrInvalid) {
		t.Fatal("multiline validation widened", err)
	}
	input = metricFormatFixture(true)
	input.MetricFormat = MetricFormatSharedV2
	input.Metrics[1].Dependencies[0].Text = "conflict"
	if _, err := a.Assemble(context.Background(), input, TierHigh); !errors.Is(err, ErrInvalid) {
		t.Fatal("same-version conflict accepted", err)
	}
}

func TestMetricFormatSelectionAndSealForgery(t *testing.T) {
	a, _ := NewDefaultContextAssembler()
	for _, format := range []MetricFormat{MetricFormatLegacyV1, MetricFormatSharedV2} {
		input := metricFormatFixture(true)
		input.MetricFormat = format
		input.Evidence = []Evidence{{ID: "optional", Text: "optional-provenance-canary"}}
		got, err := a.Assemble(context.Background(), input, TierHigh)
		if err != nil {
			t.Fatal(err)
		}
		if got.MetricFormat != format || outputInput(got).MetricFormat != format {
			t.Fatal("format lost")
		}
		gen, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: got, Hints: []Instruction{{Key: "hint", Text: "retain all reviewed SUM and COUNT meanings"}}})
		if err != nil || gen.Context.MetricFormat != format {
			t.Fatal("generation refit lost format", err)
		}
		fitted, err := a.RefitGeneration(context.Background(), gen, func(prompt string) (bool, error) { return !strings.Contains(prompt, "optional-provenance-canary"), nil })
		if err != nil || fitted.Context.MetricFormat != format || len(fitted.Context.Evidence) != 0 || !reflect.DeepEqual(fitted.Context.Metrics, got.Metrics) || fitted.Fit.OmittedCount != 1 {
			t.Fatal("provider refit changed format/meaning", err)
		}
		for _, replacement := range []MetricFormat{MetricFormatLegacyV1, MetricFormatSharedV2, "unknown"} {
			if replacement == format {
				continue
			}
			forged := got
			forged.MetricFormat = replacement
			if _, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: forged}); !errors.Is(err, ErrInvalid) {
				t.Fatal("format changed without seal", err)
			}
		}
		raw, _ := json.Marshal(got)
		var reconstructed AssembledContext
		if json.Unmarshal(raw, &reconstructed) != nil {
			t.Fatal("decode")
		}
		if _, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: reconstructed}); !errors.Is(err, ErrInvalid) {
			t.Fatal("JSON manufactured seal", err)
		}
	}
	input := metricFormatFixture(true)
	input.MetricFormat = "shared-versioned-v9"
	if _, err := a.Assemble(context.Background(), input, TierHigh); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown admitted", err)
	}
	input.Metrics = nil
	if _, err := a.Assemble(context.Background(), input, TierHigh); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown admitted with no metrics", err)
	}
	for _, format := range []MetricFormat{MetricFormatLegacyV1, MetricFormatSharedV2} {
		input = metricFormatFixture(false)
		input.MetricFormat = format
		got, err := metricRendering(input)
		if err != nil || got != renderMetrics(input.Metrics) {
			t.Fatal("nonshared bytes changed", err)
		}
	}
}

func TestMetricFormatV2HighTokenProvenance(t *testing.T) {
	// A synthetic six-leaf SUM/COUNT graph with provenance that tokenizes one
	// token per hex character. Every selected root, filter, key, scope and count
	// meaning is retained. Both formats consume the exact same semantic bytes.
	input := ContextInput{Locale: LanguageEnglish, Strategy: StrategySingleTopic, Topic: "sales", TopicVersion: "v3", Question: "Compare known net and unknown order/refund counts"}
	provenance := strings.Repeat("a1", 32)
	common := []MetricDependency{}
	for j := 0; j < 36; j++ {
		id := "orders:c" + strconv.Itoa(j)
		common = append(common, MetricDependency{Kind: "column", ID: id, Text: fmt.Sprintf(`{"dataset":"orders","column":{"id":"c%d","source_name":"c%d","nullable":true,"type":"numeric"},"meaning":"Reviewed exact USD amount; unknown remains SQL NULL; event identity is the composite division and order key","provenance":"%s"}`, j, j, provenance)})
	}
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("metric%d", i)
		deps := append([]MetricDependency(nil), common...)
		deps = append(deps, MetricDependency{Kind: "kpi", ID: id, Text: fmt.Sprintf(`{"id":"%s","expression":"sum%d-count%d","inputs":["sum%d","count%d"],"provenance":"%s"}`, id, i, i, i, i, provenance)})
		for _, kind := range []string{"sum", "count"} {
			deps = append(deps, MetricDependency{Kind: "measure", ID: fmt.Sprintf("%s%d", kind, i), Text: fmt.Sprintf(`{"aggregation":"%s","scope":"paid orders joined to paid refunds","keys":["division_id","order_id"],"filters":["orders.status=P","refunds.status=P"],"nulls":"COUNT event key includes unknown amount; SUM unknown remains NULL","provenance":"%s"}`, kind, provenance)})
		}
		input.Metrics = append(input.Metrics, PinnedMetric{ID: "sales:" + id, Text: "Reviewed " + id, Dependencies: deps})
	}
	counter, _ := NewTiktokenCounter()
	if n, err := counter.Count(provenance); err != nil || n != 64 {
		t.Fatal("provenance is not worst-token 64-hex fixture", n, err)
	}
	legacy, _ := metricRendering(input)
	old, _ := counter.Count(renderHeader(input) + legacy)
	input.MetricFormat = MetricFormatSharedV2
	compact, _ := metricRendering(input)
	now, _ := counter.Count(renderHeader(input) + compact)
	t.Logf("exact 64-hex provenance bodies legacy_tokens=%d v2_tokens=%d reduction=%d high_budget=%d", old, now, old-now, HighBudget)
	if old <= HighBudget || now >= old || now > HighBudget {
		t.Fatal("lossless wrappers do not fit", old, now)
	}
	expected := map[string]decodedMetricNamespace{}
	for _, m := range input.Metrics {
		expected[m.ID] = decodedMetricNamespace{Topic: "sales", Version: "v3"}
	}
	assertMetricV2RoundTrip(t, input, expected)
	a, _ := NewDefaultContextAssembler()
	got, err := a.Assemble(context.Background(), input, TierHigh)
	if err != nil {
		t.Fatal(err)
	}
	canonical, _ := cloneMetricsChecked(input.Metrics, map[string]bool{})
	if !reflect.DeepEqual(got.Metrics, canonical) || got.Audit.OmittedCount != 0 || got.Budget != 6500 {
		t.Fatal("mandatory semantics omitted or budget changed")
	}
}

func TestMetricFormatV2OversizedBodiesFailWhole(t *testing.T) {
	input := metricFormatFixture(true)
	input.MetricFormat = MetricFormatSharedV2
	for i := range input.Metrics {
		input.Metrics[i].Dependencies[0].Text = strings.Repeat("a1", 7900)
	}
	a, _ := NewDefaultContextAssembler()
	out, err := a.Assemble(context.Background(), input, TierHigh)
	if !errors.Is(err, ErrInsufficient) || out.Prompt != "" || len(out.Metrics) != 0 {
		t.Fatal("oversized mandatory body admitted or partially returned", err)
	}
}

func TestMetricFormatConcurrentVersionIsolation(t *testing.T) {
	input := metricFormatFixture(true)
	before, _ := json.Marshal(input)
	a, _ := NewDefaultContextAssembler()
	expected := map[MetricFormat]string{}
	for _, format := range []MetricFormat{MetricFormatLegacyV1, MetricFormatSharedV2} {
		copy := input
		copy.MetricFormat = format
		got, err := a.Assemble(context.Background(), copy, TierHigh)
		if err != nil {
			t.Fatal(err)
		}
		expected[format] = got.Prompt
	}
	t.Run("parallel", func(t *testing.T) {
		for i := 0; i < 16; i++ {
			t.Run(strconv.Itoa(i), func(t *testing.T) {
				t.Parallel()
				copy := input
				format := MetricFormatLegacyV1
				if i%2 == 0 {
					format = MetricFormatSharedV2
				}
				copy.MetricFormat = format
				got, err := a.Assemble(context.Background(), copy, TierHigh)
				if err != nil || got.MetricFormat != format || got.Prompt != expected[format] {
					t.Fatal("cross-format state leak", err)
				}
			})
		}
	})
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("concurrent rendering mutated source")
	}
}

func TestMetricFormatProjectionAndProviderRefitPreserveFullScope(t *testing.T) {
	input := projectionFixture()
	input.Metrics[0].ID = "topic:revenue"
	input.Metrics[0].Dependencies = append(input.Metrics[0].Dependencies, MetricDependency{Kind: "measure", ID: "revenue", Text: "Exact sum over paid rows; NULL stays unknown"})
	second := cloneMetrics(input.Metrics)[0]
	second.ID = "topic:known_count"
	second.Text = "Known amount count"
	second.Dependencies[1] = MetricDependency{Kind: "measure", ID: "known_count", Text: "Exact COUNT non-NULL amount over the same paid rows"}
	input.Metrics = append(input.Metrics, second)
	input.Evidence = []Evidence{{ID: "optional", Text: "optional-provenance-canary"}}
	a, _ := NewDefaultContextAssembler()
	for _, format := range []MetricFormat{MetricFormatLegacyV1, MetricFormatSharedV2} {
		input.MetricFormat = format
		got, err := a.Assemble(context.Background(), input, TierHigh)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(got.Prompt, "physical_projection:selected-closure-v1") || strings.Contains(got.Prompt, "unneeded_field") || !reflect.DeepEqual(got.Relations, input.Relations) {
			t.Fatal("projection changed authorization or rendering")
		}
		gen, err := a.ResolvePrecedence(context.Background(), GenerationInput{Context: got})
		if err != nil {
			t.Fatal(err)
		}
		fitted, err := a.RefitGeneration(context.Background(), gen, func(s string) (bool, error) { return !strings.Contains(s, "optional-provenance-canary"), nil })
		if err != nil || fitted.Context.MetricFormat != format || !reflect.DeepEqual(fitted.Context.Relations, input.Relations) || !reflect.DeepEqual(fitted.Context.Metrics, got.Metrics) || strings.Contains(fitted.Prompt, "unneeded_field") {
			t.Fatal("provider refit changed projected scope or exact metric meaning", err)
		}
	}
}
