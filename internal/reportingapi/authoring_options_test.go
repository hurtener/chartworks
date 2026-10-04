package reportingapi

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func optionSchemaRequests() []any {
	dataset := reporting.AuthoringOptionTarget{Dataset: &reporting.AuthoringDatasetOptionTarget{NewBlock: "chart", Topic: reporting.TopicPin{Topic: "topic", Version: "v1", Digest: strings.Repeat("a", 64)}, Dataset: "sales", Dimension: "region"}}
	report := reporting.AuthoringOptionTarget{Report: &reporting.AuthoringReportOptionTarget{Policy: "private_preview", Report: "report", Revision: 2, Digest: strings.Repeat("b", 64), Page: "page", Filter: "region"}}
	const operation = "option:1791093600:0123456789abcdef0123456789abcdef"
	return []any{
		reporting.AuthoringOptionRequest{Target: dataset, Operation: operation, Search: "east", Limit: 20, Locale: "en-US"},
		reporting.AuthoringOptionRequest{Target: report, Operation: operation, Search: "east", Limit: 20, Locale: "en-US"},
		reporting.AuthoringOptionReference{Target: report, Operation: operation},
		reporting.AuthoringOptionControlRequest{Target: report, Operation: operation, Action: "cancel"},
	}
}

func TestAuthoringOptionClosedSchemasAndExactCoordinates(t *testing.T) {
	entries, requests := authoringOptionEntries(nil), optionSchemaRequests()
	if len(entries) != 4 {
		t.Fatal("option inventory drift")
	}
	for i, entry := range entries {
		t.Run(entry.definition.ID, func(t *testing.T) {
			if entry.schemaErr != nil {
				t.Fatal(entry.schemaErr)
			}
			raw, err := json.Marshal(requests[i])
			if err != nil || entry.definition.Request.Validate(raw, MaxBodyBytes) != nil {
				t.Fatal("valid exact option DTO rejected", err)
			}
			for _, field := range []string{"tenant", "actor", "session", "scopes", "source", "context", "sql", "schema", "rows", "binding", "parameters", "authority", "attempt", "retry"} {
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				body[field] = "injected"
				bad, _ := json.Marshal(body)
				if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
					t.Fatal("open request", field)
				}
			}
			for _, field := range []string{"target", "operation"} {
				for _, omit := range []bool{false, true} {
					var body map[string]any
					_ = json.Unmarshal(raw, &body)
					body[field] = nil
					if omit {
						delete(body, field)
					}
					bad, _ := json.Marshal(body)
					if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
						t.Fatal("missing/null required operation coordinate", field, omit)
					}
				}
			}
			kind := "report"
			if i == 0 {
				kind = "dataset"
			}
			for _, field := range []string{"sql", "source", "context", "column", "where", "values", "scopes"} {
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				body["target"].(map[string]any)[kind].(map[string]any)[field] = "injected"
				bad, _ := json.Marshal(body)
				if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
					t.Fatal("open nested target", field)
				}
			}
			for _, private := range []string{`"sql"`, `"search"`, `"cursor"`, `"receipt"`, `"actor"`, `"session"`, `"source_operation"`, `"references"`, `"binding"`} {
				if strings.Contains(string(entry.definition.Response.Document()), private) {
					t.Fatal("private custody material exposed", private)
				}
			}
		})
	}
	for _, i := range []int{0, 1} {
		for _, limit := range []int{0, 1, 199, 200, 201} {
			in := requests[i].(reporting.AuthoringOptionRequest)
			in.Limit = limit
			raw, _ := json.Marshal(in)
			if valid := entries[i].definition.Request.Validate(raw, MaxBodyBytes) == nil; valid != (limit >= 1 && limit <= 199) {
				t.Fatal("option limit widened beyond preview sentinel budget", i, limit, valid)
			}
		}
	}
	for _, policy := range []string{"private_preview", "published", "draft", "any"} {
		in := requests[1].(reporting.AuthoringOptionRequest)
		target := *in.Target.Report
		in.Target.Report = &target
		in.Target.Report.Policy = policy
		raw, _ := json.Marshal(in)
		if valid := entries[1].definition.Request.Validate(raw, MaxBodyBytes) == nil; valid != (policy == "private_preview" || policy == "published") {
			t.Fatal("policy enum drift", policy, valid)
		}
	}
	for _, action := range []string{"cancel", "reconcile", "inspect", "retry", "execute"} {
		in := requests[3].(reporting.AuthoringOptionControlRequest)
		in.Action = action
		raw, _ := json.Marshal(in)
		if valid := entries[3].definition.Request.Validate(raw, MaxBodyBytes) == nil; valid != (action == "cancel" || action == "reconcile") {
			t.Fatal("control enum drift", action, valid)
		}
	}
}

func TestAuthoringOptionHTTPMCPRegistrationParity(t *testing.T) {
	registry, err := AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := authoringOptionMCPBindings(registry, service)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpserver.NewRegistry(bindings)
	if err != nil || len(mcp.Manifest()) != 4 {
		t.Fatal("option binding inventory", err)
	}
	actions := []string{"reporting.validate", "reporting.execute", "reporting.read", "sources.query"}
	effects := []string{"bounded_source_read_option_values", "bounded_source_read_option_values", "retained_metadata_read", "existing_source_attempt_control"}
	for i, entry := range authoringOptionEntries(nil) {
		d := entry.definition
		if d.Method != "POST" || !strings.HasPrefix(d.Path, AuthoringPath) || d.Public || d.Action != actions[i] || d.Effect != effects[i] || d.Audit == "" || d.ResourceLoader == "" || d.Replay != "never" || d.MaxBodyBytes != MaxBodyBytes || !reflect.DeepEqual(d.Errors, runtimeErrors()) {
			t.Fatal("shared native option contract drift", d.ID)
		}
		found := false
		for _, tool := range mcp.Manifest() {
			if tool.Name != d.ID {
				continue
			}
			found = true
			for key, want := range map[string]any{"chartworks/action": d.Action, "chartworks/effect": d.Effect, "chartworks/audit": d.Audit, "chartworks/resourceLoader": d.ResourceLoader, "chartworks/persists": i != 2, "chartworks/maySpend": i < 2} {
				if tool.Meta[key] != want {
					t.Fatal("MCP contract differs", d.ID, key, tool.Meta[key], want)
				}
			}
			if tool.Annotations.ReadOnlyHint != (i == 2) || tool.Annotations.IdempotentHint != (i == 2) || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint != (i != 2) || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Fatal("option cost/control annotations drift", d.ID)
			}
			for key, want := range map[string]json.RawMessage{"chartworks/requestSchema": d.Request.Document(), "chartworks/resultSchema": d.Response.Document()} {
				got, _ := json.Marshal(tool.Meta[key])
				var a, b any
				if json.Unmarshal(got, &a) != nil || json.Unmarshal(want, &b) != nil || !reflect.DeepEqual(a, b) {
					t.Fatal("HTTP/MCP schema mismatch", d.ID, key)
				}
			}
			var faults []struct {
				Status int    `json:"status"`
				Code   string `json:"code"`
			}
			raw, _ := json.Marshal(tool.Meta["chartworks/errorContract"])
			if json.Unmarshal(raw, &faults) != nil || len(faults) != len(d.Errors) {
				t.Fatal("error contract omitted", d.ID)
			}
			for _, want := range d.Errors {
				matched := false
				for _, fault := range faults {
					matched = matched || fault.Status == want.Status && fault.Code == want.Code
				}
				if !matched {
					t.Fatal("structured fault dropped", d.ID, want)
				}
			}
		}
		if !found {
			t.Fatal("option operation missing from MCP", d.ID)
		}
	}
	// Compose with actual public-filter contracts; none is replaced or widened.
	public, err := DocumentsRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Compose(registry, public); err != nil {
		t.Fatal("public option continuity collided", err)
	}
}

func TestAuthoringOptionResponseDistinguishesLostAndEmptyValues(t *testing.T) {
	schema := authoringOptionEntries(nil)[0].definition.Response
	for _, result := range []reporting.AuthoringOptionView{
		{Operation: "option:1791093600:0123456789abcdef0123456789abcdef", InputDigest: strings.Repeat("a", 64), Status: "completed", ValuesAvailable: true, NewOperationAllowed: true, Options: []reporting.FilterOption{}, Complete: true},
		{Operation: "option:1791093600:0123456789abcdef0123456789abcdef", InputDigest: strings.Repeat("a", 64), Status: "completed", Code: "result_not_retained", NewOperationAllowed: true, Options: []reporting.FilterOption{}},
		{Operation: "option:1791093600:0123456789abcdef0123456789abcdef", InputDigest: strings.Repeat("a", 64), Status: "uncertain", Code: "execution_outcome_unknown", RemoteState: "unknown", Options: []reporting.FilterOption{}},
	} {
		raw, err := json.Marshal(result)
		if err != nil || schema.ValidateResponse(raw, MaxBodyBytes) != nil {
			t.Fatal("result state lost at wire boundary", err)
		}
		var decoded reporting.AuthoringOptionView
		if json.Unmarshal(raw, &decoded) != nil || !reflect.DeepEqual(result, decoded) {
			t.Fatal("lost/empty/unknown collapsed")
		}
		for _, required := range []string{`"values_available":`, `"new_operation_allowed":`, `"complete":`} {
			if !strings.Contains(string(raw), required) {
				t.Fatal("false state bit omitted", required)
			}
		}
	}
}
