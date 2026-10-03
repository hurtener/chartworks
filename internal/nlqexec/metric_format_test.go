package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/exec"
	"os"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// Synthetic pre-change records exercise persisted JSON and lineage compatibility.
func TestMetricFormatLegacyQueryLineage(t *testing.T) {
	for kind, want := range map[string]string{"shared": "50fb5814eb339dd9bcbd3bd7aad38a1a1a00da69aed82d20dd3f72b6145e4858", "nonshared": "eaeb289b0c6c2f75a8cf81a6f34c0908a4423f2f3fda2de33769c8215c99bbc8"} {
		raw, err := os.ReadFile("testdata/metric-format/legacy-" + kind + "-query.json")
		if err != nil {
			t.Fatal(err)
		}
		var q QueryRecord
		if json.Unmarshal(raw, &q) != nil {
			t.Fatal("legacy query decode")
		}
		again, _ := json.Marshal(q)
		if string(raw) != string(again) || QueryLineageDigest(q) != want {
			t.Fatal("historical query bytes or lineage changed", kind)
		}
		if q.Route.Context.MetricFormat != nlq.MetricFormatLegacyV1 || q.Generation.Context.MetricFormat != nlq.MetricFormatLegacyV1 {
			t.Fatal("legacy query upgraded")
		}
	}
}

func TestMetricFormatRetainedResealPreservesVersionAndFallback(t *testing.T) {
	e := unitEnvelope(t)
	binding, err := retainedSourceReader{}.Binding(context.Background(), e, "source", "context")
	if err != nil {
		t.Fatal(err)
	}
	def := unitContract("topic", "v1", "source", "context", "dataset", true, false).Publication.Definition
	scopes, relations, err := reviewedProjection([]reviewedDataset{{topic: "topic", dataset: def.Datasets[0]}}, binding)
	if err != nil {
		t.Fatal(err)
	}
	current := admission{binding: binding, relationScope: scopes, relations: relations}
	a, _ := nlq.NewDefaultContextAssembler()
	for _, format := range []nlq.MetricFormat{nlq.MetricFormatLegacyV1, nlq.MetricFormatSharedV2} {
		q := unitQuery(e, "query-format", "topic", "v1", "context", false)
		input := nlq.ContextInput{MetricFormat: format, Locale: q.Locale, Strategy: q.Route.Outcome, Topic: q.Topic, TopicVersion: "v1", Topics: []nlq.TopicRevision{{Topic: q.Topic, Version: "v1"}}, Question: q.Question, Relations: relations}
		for _, root := range []string{"sum", "count"} {
			input.Metrics = append(input.Metrics, nlq.PinnedMetric{ID: "topic:" + root, Text: "Reviewed " + root, Dependencies: []nlq.MetricDependency{{Kind: "measure", ID: root, Text: "Exact " + root + " semantics"}, {Kind: "column", ID: "dataset:id", Text: "Exact reviewed primary key includes unknown amount"}}})
		}
		assembled, err := a.Assemble(context.Background(), input, nlq.TierHigh)
		if err != nil {
			t.Fatal(err)
		}
		q.Generation.Context = assembled
		raw, _ := json.Marshal(assembled)
		if json.Unmarshal(raw, q.Route.Context) != nil {
			t.Fatal("view decode")
		}
		raw, _ = json.Marshal(q)
		if json.Unmarshal(raw, &q) != nil {
			t.Fatal("stored query decode")
		}
		before := QueryLineageDigest(q)
		for _, fallback := range []bool{false, true} {
			candidate := q
			if fallback {
				candidate.Generation.Context = nlq.AssembledContext{}
			}
			got, err := (&Service{}).resealQueryContext(context.Background(), candidate, current)
			if err != nil || got.MetricFormat != format || got.Prompt != assembled.Prompt || !reflect.DeepEqual(got.Metrics, assembled.Metrics) {
				t.Fatal("retained renderer changed", format, fallback, err)
			}
			if _, err := a.ResolvePrecedence(context.Background(), nlq.GenerationInput{Context: got}); err != nil {
				t.Fatal("not resealed", err)
			}
		}
		if QueryLineageDigest(q) != before {
			t.Fatal("retained parent mutated")
		}
		for _, replacement := range []nlq.MetricFormat{nlq.MetricFormatLegacyV1, nlq.MetricFormatSharedV2, "unknown"} {
			if replacement == format {
				continue
			}
			changed := q
			changed.Generation.Context.MetricFormat = replacement
			if _, err := (&Service{}).resealQueryContext(context.Background(), changed, current); !errors.Is(err, exec.ErrBinding) {
				t.Fatal("generation disagreement accepted", replacement, err)
			}
			changed = q
			view := *q.Route.Context
			view.MetricFormat = replacement
			changed.Route.Context = &view
			if _, err := (&Service{}).resealQueryContext(context.Background(), changed, current); !errors.Is(err, exec.ErrBinding) {
				t.Fatal("route disagreement accepted", replacement, err)
			}
		}
		changed := q
		changed.Generation.Context.Prompt = "earlier opaque historical prompt"
		_, err = (&Service{}).resealQueryContext(context.Background(), changed, current)
		if format == nlq.MetricFormatLegacyV1 && err != nil {
			t.Fatal("historical structured reseal tightened", err)
		}
		if format == nlq.MetricFormatSharedV2 && !errors.Is(err, exec.ErrBinding) {
			t.Fatal("explicit canonical v2 prompt mismatch accepted", err)
		}
	}
}

func TestMetricFormatUnknownRetainedVersionStopsBeforeSourceOrModel(t *testing.T) {
	e := unitEnvelope(t)
	q := unitQuery(e, "unknown-format-query", "topic", "v1", "context", false)
	q.Route.Context.MetricFormat = "shared-versioned-unknown"
	repo := newUnitRepository()
	repo.queries[q.ID] = q
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}}
	executor := &unitExecutor{}
	engine := &sequenceGateway{}
	s := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: executor, engine: engine, repo: repo}
	if _, err := s.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "unknown-format-run"}); !errors.Is(err, exec.ErrBinding) || executor.calls != 0 || engine.calls != 0 {
		t.Fatal("unknown retained format reached source/model", err, executor.calls, engine.calls)
	}
}
