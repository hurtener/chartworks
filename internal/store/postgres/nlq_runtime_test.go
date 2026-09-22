package postgres

import (
	"testing"

	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
)

func TestNormalizeTemplateSelectionEvidence(t *testing.T) {
	selection := rulesets.TemplateSelection{ID: "reviewed", Topic: "sales", TopicVersion: "v1", PackDigest: "pack", RuleVersion: "rules-v1", RuleDigest: "rules"}
	for _, test := range []struct {
		name   string
		record nlqexec.QueryRecord
		want   bool
	}{
		{name: "legacy-empty", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{}}, want: true},
		{name: "exact-selection", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{selection}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{selection}}}}, want: true},
		{name: "column-only-selection", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{selection}}, want: false},
		{name: "route-only-selection", record: nlqexec.QueryRecord{Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{selection}}}}, want: false},
		{name: "request-mismatch", record: nlqexec.QueryRecord{Templates: []rulesets.TemplateSelection{selection}, Route: nlqroute.RouteResult{Templates: []rulesets.TemplateSelection{selection}, Request: nlqroute.RouteRequest{Templates: []rulesets.TemplateSelection{{ID: "other"}}}}}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeTemplateSelectionEvidence(&test.record); got != test.want {
				t.Fatalf("normalizeTemplateSelectionEvidence() = %v, want %v", got, test.want)
			}
			if test.want && len(test.record.Templates) == 0 && (test.record.Templates == nil || test.record.Route.Templates == nil || test.record.Route.Request.Templates == nil) {
				t.Fatal("empty evidence was not canonicalized")
			}
		})
	}
}
