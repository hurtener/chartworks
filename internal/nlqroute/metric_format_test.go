package nlqroute

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
)

func TestMetricFormatFreshRouteAndRetainedClarificationReplay(t *testing.T) {
	p := recoveryPublication()
	hit := recoveryFacet(t, p, "kpi", "margin_pct", p.Definition.KPIs[1])
	service, _ := recoveryRouteService(t, p, hit)
	out, err := service.Route(context.Background(), cw07DiscoveryEnvelope(t), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Gross margin percentage", MetricIDs: []string{"margin_pct", "margin"}})
	if err != nil || out.Context == nil || out.Context.MetricFormat != nlq.MetricFormatSharedV2 {
		t.Fatal("fresh route lacks explicit format", err)
	}
	assembled, err := out.GenerationContext()
	if err != nil || assembled.MetricFormat != nlq.MetricFormatSharedV2 {
		t.Fatal("sealed context lacks format", err)
	}
	selectionDigest := readexec.Hash(out.Selection)
	for _, format := range []nlq.MetricFormat{nlq.MetricFormatLegacyV1, nlq.MetricFormatSharedV2} {
		raw, _ := json.Marshal(out)
		var retained RouteResult
		if json.Unmarshal(raw, &retained) != nil {
			t.Fatal("retained decode")
		}
		view := retained.Context
		rebuilt, err := service.assembler.Assemble(context.Background(), nlq.ContextInput{MetricFormat: format, Locale: view.Locale, Strategy: view.Strategy, Topic: view.Topic, TopicVersion: view.TopicVersion, Topics: view.Topics, Question: view.Question, Relations: view.Relations, Constraints: view.Constraints, Metrics: view.Metrics, Evidence: view.Evidence, Advisory: view.Advisory, Examples: view.Examples}, view.Tier)
		if err != nil {
			t.Fatal(err)
		}
		retained.Context = contextView(rebuilt)
		before, _ := json.Marshal(retained)
		_, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), retained)
		if err != nil {
			t.Fatal("retained semantic replay rejected", format, err)
		}
		after, _ := json.Marshal(retained)
		if string(before) != string(after) || retained.Context.MetricFormat != format || !reflect.DeepEqual(retained.Context.Metrics, out.Context.Metrics) || readexec.Hash(retained.Selection) != selectionDigest {
			t.Fatal("replay reinterpreted retained context")
		}
		retained.Context.MetricFormat = "unknown"
		if _, _, err = service.ReplayClarifications(context.Background(), cw07DiscoveryEnvelope(t), retained); err == nil {
			t.Fatal("unknown retained format accepted")
		}
	}
}

type metricFormatCounter struct {
	nlq.TokenCounter
	prompts []string
}

func (c *metricFormatCounter) Count(text string) (int, error) {
	c.prompts = append(c.prompts, text)
	return c.TokenCounter.Count(text)
}

func TestMetricFormatClarificationBudgetUsesExplicitFreshVersion(t *testing.T) {
	counter, _ := nlq.NewTiktokenCounter()
	spy := &metricFormatCounter{TokenCounter: counter}
	assembler, _ := nlq.NewContextAssembler(spy)
	s := &Service{assembler: assembler}
	p := recoveryPublication()
	admitted := []admittedTopic{{id: p.State.Topic, publication: p}}
	metrics, err := resolveMetrics(admitted, []string{"margin_pct", "margin"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.preflightClarificationBudget(context.Background(), RouteRequest{Locale: nlq.LanguageEnglish, Question: "Compare exact metrics"}, admitted, RouteResult{Outcome: nlq.StrategySingleTopic}, metrics)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, prompt := range spy.prompts {
		found = found || strings.Contains(prompt, "metric_definitions:shared-versioned-v2")
		if strings.Contains(prompt, "metric_definitions:shared-versioned-v1") {
			t.Fatal("fresh preflight used legacy format")
		}
	}
	if !found {
		t.Fatal("budget did not measure explicit v2 shared wrappers")
	}
}
