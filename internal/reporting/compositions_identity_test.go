package reporting

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
)

func TestReportingCompositionGroupingBoundaries(t *testing.T) {
	base := CompositionGroup{Kind: "block", Block: "block", Revision: 1, Definition: "definition", Execution: "query", Locale: "en-US", Policy: "published"}
	base.Binding = exec.Binding{Source: "source", Context: "context-a"}
	base.Resolution = Resolution{At: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Timezone: "UTC"}
	key := groupIdentity(base)
	for _, test := range []struct {
		name   string
		mutate func(*CompositionGroup)
	}{
		{"context", func(g *CompositionGroup) { g.Binding.Context = "context-b" }},
		{"source", func(g *CompositionGroup) { g.Binding.Source = "different-source" }},
		{"private", func(g *CompositionGroup) { g.Private = true }},
		{"locale", func(g *CompositionGroup) { g.Locale = "es-AR" }},
		{"policy", func(g *CompositionGroup) { g.Policy = "certified_only" }},
		{"revision", func(g *CompositionGroup) { g.Revision = 2 }},
		{"definition", func(g *CompositionGroup) { g.Definition = "changed-definition" }},
		{"execution", func(g *CompositionGroup) { g.Execution = "changed-query" }},
		{"instant", func(g *CompositionGroup) { g.Resolution.At = g.Resolution.At.Add(time.Second) }},
		{"timezone", func(g *CompositionGroup) { g.Resolution.Timezone = "America/Argentina/Buenos_Aires" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			changed := clone(base)
			test.mutate(&changed)
			if groupIdentity(changed) == key {
				t.Fatal("distinct context or execution semantics were shared")
			}
		})
	}
	selected := clone(base)
	selected.Outputs, selected.Narrative = []string{"another-output"}, true
	if groupIdentity(selected) != key {
		t.Fatal("output selection incorrectly duplicated identical source queries")
	}
	dynamic := clone(base)
	dynamic.Kind = "query"
	dynamic.Origin = &QueryOrigin{Widget: "first", QueryDigest: "query-evidence"}
	otherWidget := clone(dynamic)
	otherWidget.Origin.Widget = "second"
	if groupIdentity(dynamic) == groupIdentity(otherWidget) {
		t.Fatal("distinct dynamic widget provenance was collapsed")
	}
}

func TestReportingCompositionSelectionState(t *testing.T) {
	result := GroupResult{Group: "group-1", Kind: "block", State: "partial", Code: "output_failed",
		Outputs: []RetainedOutput{{ID: "good", State: "succeeded"}, {ID: "bad", State: "failed", Code: "narrative_unavailable"}}}
	result.Digest = GroupResultDigest(result)
	for _, test := range []struct {
		selected []string
		state    string
		code     string
	}{
		{[]string{"good"}, "completed", ""},
		{[]string{"bad"}, "partial", "output_failed"},
		{[]string{"good", "bad"}, "partial", "output_failed"},
	} {
		payload, err := CompositionPayloadFromResult("main", "widget", test.selected, result)
		if err != nil || payload.State != test.state || payload.Code != test.code || len(payload.Outputs) != len(test.selected) {
			t.Fatal("payload and selected-output state disagree", payload, err)
		}
		for i, output := range payload.Outputs {
			if output.ID != test.selected[i] {
				t.Fatal("selected output order changed")
			}
		}
	}
	if _, err := CompositionPayloadFromResult("main", "widget", []string{"missing"}, result); !errors.Is(err, ErrIncomplete) {
		t.Fatal("missing selected output was reported complete", err)
	}
	result.Code = "query_truncated"
	result.Digest = GroupResultDigest(result)
	if state, code := compositionSelectionState(result, []string{"good"}); state != "partial" || code != "query_truncated" {
		t.Fatal("selected output hid a group-wide truncated source result")
	}
	result.Digest = "corrupt"
	if _, err := CompositionPayloadFromResult("main", "widget", []string{"good"}, result); !errors.Is(err, ErrInvalid) {
		t.Fatal("corrupt result digest accepted", err)
	}
}

func TestReportingCompositionMalformedManifests(t *testing.T) {
	for _, raw := range []string{"", "not-json", "null", "{}", `{"version":99}`, strings.Repeat(" ", 16<<20+1)} {
		if _, err := DecodeCompositionManifest([]byte(raw), strings.Repeat("a", 64)); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid persisted manifest was accepted", len(raw), err)
		}
	}
}
