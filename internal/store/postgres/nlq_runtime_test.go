package postgres

import (
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

func TestNormalizeTemplateSelectionEvidence(t *testing.T) {
	selection := rulesets.TemplateSelection{ID: "reviewed", Topic: "sales", TopicVersion: "v1", PackDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", RuleVersion: "rules-v1", RuleDigest: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}
	for _, test := range []struct {
		name       string
		record     nlqexec.QueryRecord
		columnJSON string
		routeJSON  string
		want       bool
	}{
		{name: "legacy-omission", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{}}, columnJSON: `[]`, routeJSON: `{}`, want: true},
		{name: "explicit-empty", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{}}}}, columnJSON: `[]`, routeJSON: `{"templates":[],"request":{"templates":[]}}`, want: true},
		{name: "explicit-null", record: nlqexec.QueryRecord{}, columnJSON: `[]`, routeJSON: `{"templates":null,"request":{"templates":null}}`, want: false},
		{name: "partial-empty", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{}}}, columnJSON: `[]`, routeJSON: `{"templates":[],"request":{}}`, want: false},
		{name: "exact-selection", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{selection}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{selection}}}}, columnJSON: mustJSON(t, []rulesets.TemplateSelection{selection}), routeJSON: mustJSON(t, nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{selection}}}), want: true},
		{name: "column-only-selection", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{selection}}, columnJSON: mustJSON(t, []rulesets.TemplateSelection{selection}), routeJSON: `{}`, want: false},
		{name: "malformed-element", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{{}}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{{}}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{{}}}}}, columnJSON: `[null]`, routeJSON: `{"templates":[null],"request":{"templates":[null]}}`, want: false},
		{name: "request-mismatch", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{selection}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{{ID: "other"}}}}}, columnJSON: mustJSON(t, []rulesets.TemplateSelection{selection}), routeJSON: mustJSON(t, nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{{ID: "other"}}}}), want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeTemplateSelectionEvidence(&test.record, []byte(test.columnJSON), []byte(test.routeJSON)); got != test.want {
				t.Fatalf("normalizeTemplateSelectionEvidence() = %v, want %v", got, test.want)
			}
			if test.want && len(test.record.Templates) == 0 && (test.record.Templates == nil || test.record.Route.Templates == nil || test.record.Route.Request.Templates == nil) {
				t.Fatal("empty evidence was not canonicalized")
			}
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
