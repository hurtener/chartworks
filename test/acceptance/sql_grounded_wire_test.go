package acceptance

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

// Inspect decoded provider messages, not model replies accidentally searched in
// the request. Formatting and JSON escaping are not an API conformance rule.
func assertGroundedProviderRequests(t *testing.T, f *gatewayFixture, start int, question string, locale nlq.Language, record nlqexec.QueryRecord) {
	t.Helper()
	f.mu.Lock()
	bodies := append([]string(nil), f.requestBodies[start:]...)
	f.mu.Unlock()
	counts := map[string]int{}
	for _, body := range bodies {
		var wire struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
			ResponseFormat struct {
				Type   string `json:"type"`
				Schema struct {
					Name       string `json:"name"`
					Strict     bool   `json:"strict"`
					Definition struct {
						Type                 string                     `json:"type"`
						AdditionalProperties bool                       `json:"additionalProperties"`
						Required             []string                   `json:"required"`
						Properties           map[string]json.RawMessage `json:"properties"`
					} `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		if err := json.Unmarshal([]byte(body), &wire); err != nil {
			t.Fatal("invalid provider request", err)
		}
		role := ""
		for _, name := range []string{"clarify", "sqlgen", "sqlfix"} {
			if wire.Model == f.cfg.Roles[name].Model {
				role = name
			}
		}
		if role == "" {
			continue
		} // Embedding/reranking remains its existing operation.
		counts[role]++
		if role == "sqlfix" || len(wire.Messages) != 2 || wire.Messages[0].Role != "system" || wire.Messages[0].Content == "" || wire.Messages[1].Role != "user" {
			t.Fatal("unexpected model role or message structure")
		}
		schema := wire.ResponseFormat.Schema
		if wire.ResponseFormat.Type != "json_schema" || !schema.Strict || schema.Definition.Type != "object" || schema.Definition.AdditionalProperties {
			t.Fatal("not the strict provider schema")
		}
		required := append([]string(nil), schema.Definition.Required...)
		sort.Strings(required)
		if role == "clarify" {
			if schema.Name != "nlq_concept_choice" || !reflect.DeepEqual(required, []string{"alternatives", "decision", "selected"}) {
				t.Fatal("wrong concept response contract")
			}
			var input struct {
				Question   string       `json:"question"`
				Locale     nlq.Language `json:"locale"`
				Candidates []struct {
					ID          string `json:"id"`
					Name        string `json:"name"`
					Aggregation string `json:"aggregation"`
				} `json:"candidates"`
			}
			if json.Unmarshal([]byte(wire.Messages[1].Content), &input) != nil || input.Question != question || input.Locale != locale || len(input.Candidates) == 0 {
				t.Fatal("concept request is not the current reviewed input")
			}
			var selected struct {
				Items struct {
					Properties struct {
						ID struct {
							Enum []string `json:"enum"`
						} `json:"id"`
					} `json:"properties"`
				} `json:"items"`
			}
			if json.Unmarshal(schema.Definition.Properties["selected"], &selected) != nil {
				t.Fatal("missing bounded candidate schema")
			}
			if record.Route.Concepts == nil || len(record.Route.Concepts.Choice.Selected) != 1 {
				t.Fatal("missing accepted concept selection")
			}
			expected := record.Route.Concepts.Choice.Selected[0].ID
			ids := make([]string, 0, len(input.Candidates))
			matches := 0
			for _, candidate := range input.Candidates {
				ids = append(ids, candidate.ID)
				if candidate.ID == expected && candidate.Name != "" && candidate.Aggregation == "sum" {
					matches++
				}
			}
			if matches != 1 || !reflect.DeepEqual(ids, selected.Items.Properties.ID.Enum) {
				t.Fatal("wire candidate set does not match its strict schema and accepted metric")
			}
		} else {
			if schema.Name != "nlq_sql_candidate" || !reflect.DeepEqual(required, []string{"ambiguities", "assumptions", "decision", "parameters", "questions", "sql"}) {
				t.Fatal("wrong SQL-generation contract")
			}
			prompt := wire.Messages[1].Content
			if record.Generation.Prompt == "" || !strings.HasPrefix(prompt, record.Generation.Prompt+"\ndialect:") || len(record.Generation.Context.Metrics) != 1 {
				t.Fatal("provider did not receive the final retained generation packet")
			}
			metric := record.Generation.Context.Metrics[0]
			if !strings.Contains(prompt, metric.Text) || len(metric.Dependencies) == 0 {
				t.Fatal("missing selected metric definition")
			}
			for _, dependency := range metric.Dependencies {
				if !strings.Contains(prompt, dependency.Text) {
					t.Fatal("missing mandatory reviewed dependency on real generation wire")
				}
			}
		}
	}
	if counts["clarify"] != 1 || counts["sqlgen"] != 1 || counts["sqlfix"] != 0 {
		t.Fatal("unexpected concept/generation attempt count", counts)
	}
}
