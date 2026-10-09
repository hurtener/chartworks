package reportingapi

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func lifecycleSchemaRequests() []any {
	digest := strings.Repeat("a", 64)
	return []any{
		reporting.AuthoringLifecycleRequest{Block: "chart", Revision: 2},
		reporting.AuthoringBlockPublishRequest{Block: "chart", ExpectedVersion: 3, Revision: 2, Digest: digest, Evidence: "evidence"},
		reporting.AuthoringRebindPublishedRequest{Report: "report", ExpectedVersion: 4, Revision: 2, Digest: digest, Widgets: []reporting.AuthoringPublishedWidget{{Widget: "selected", Block: "chart", Revision: 2, Digest: digest}}},
		reporting.AuthoringReportTransitionRequest{Report: "report", ExpectedVersion: 4, Revision: 2, Operation: "review", Note: ""},
	}
}

func TestAuthoringLifecycleClosedSchemasAndExactCoordinates(t *testing.T) {
	entries, requests := authoringLifecycleEntries(nil), lifecycleSchemaRequests()
	if len(entries) != 4 {
		t.Fatal("lifecycle inventory drift")
	}
	for i, entry := range entries {
		t.Run(entry.definition.ID, func(t *testing.T) {
			if entry.schemaErr != nil {
				t.Fatal(entry.schemaErr)
			}
			raw, err := json.Marshal(requests[i])
			if err != nil || entry.definition.Request.Validate(raw, MaxBodyBytes) != nil {
				t.Fatal("valid exact lifecycle DTO rejected", err)
			}
			for _, field := range []string{"tenant", "actor", "session", "scopes", "sql", "rows", "definition", "authority", "audience", "grant", "outputs", "retry", "certify"} {
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				body[field] = "injected"
				bad, _ := json.Marshal(body)
				if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
					t.Fatal("open lifecycle request", field)
				}
			}
			var body map[string]any
			_ = json.Unmarshal(raw, &body)
			for field := range body {
				// The inspection target is an exclusive domain union; each optional
				// selector is intentionally not independently schema-required.
				if i == 0 && field != "revision" {
					continue
				}
				for _, omit := range []bool{false, true} {
					if field == "widgets" && !omit {
						continue // Nullable collections are rejected by native cardinality checks.
					}
					var changed map[string]any
					_ = json.Unmarshal(raw, &changed)
					changed[field] = nil
					if omit {
						delete(changed, field)
					}
					bad, _ := json.Marshal(changed)
					if entry.definition.Request.Validate(bad, MaxBodyBytes) == nil {
						t.Fatal("required coordinate missing/null", field, omit)
					}
				}
			}
			for _, private := range []string{`"sql"`, `"instructions"`, `"prompt_version"`, `"rows"`, `"scopes"`, `"grants"`, `"audience_count"`} {
				if strings.Contains(string(entry.definition.Response.Document()), private) {
					t.Fatal("unsafe disclosure field", private)
				}
			}
		})
	}
	for _, operation := range []string{"review", "publish", "reject", "Publish", "archive", "withdraw", "review-and-publish", ""} {
		in := requests[3].(reporting.AuthoringReportTransitionRequest)
		in.Operation, in.Note = operation, "Explicit note"
		raw, _ := json.Marshal(in)
		want := operation == "review" || operation == "publish" || operation == "reject"
		if valid := entries[3].definition.Request.Validate(raw, MaxBodyBytes) == nil; valid != want {
			t.Fatal("transition enum widened", operation, valid)
		}
	}
	for _, stage := range []string{"draft", "review", "published", "any"} {
		raw, _ := json.Marshal(reporting.AuthoringLifecycleRequest{Report: "report", Stage: stage})
		if valid := entries[0].definition.Request.Validate(raw, MaxBodyBytes) == nil; valid != (stage == "draft" || stage == "review") {
			t.Fatal("inspection stage enum widened", stage, valid)
		}
	}
	for _, count := range []int{0, 1, 100, 101} {
		in := requests[2].(reporting.AuthoringRebindPublishedRequest)
		in.Widgets = make([]reporting.AuthoringPublishedWidget, count)
		for i := range in.Widgets {
			in.Widgets[i] = reporting.AuthoringPublishedWidget{Widget: "selected", Block: "chart", Revision: 2, Digest: strings.Repeat("a", 64)}
		}
		raw, _ := json.Marshal(in)
		if valid := entries[2].definition.Request.Validate(raw, MaxBodyBytes) == nil; valid != (count >= 1 && count <= 100) {
			t.Fatal("rebind bound changed", count, valid)
		}
	}
	raw, _ := json.Marshal(requests[2])
	for _, field := range []string{"policy", "output", "parameters", "filters", "defaults", "layout", "actor", "source", "context"} {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		body["widgets"].([]any)[0].(map[string]any)[field] = "injected"
		bad, _ := json.Marshal(body)
		if entries[2].definition.Request.Validate(bad, MaxBodyBytes) == nil {
			t.Fatal("rebind accepted content replacement", field)
		}
	}
	for _, i := range []int{1, 2, 3} {
		for _, field := range []string{"expected_version", "revision"} {
			for _, value := range []int{0, -1, 1, 256, 257} {
				raw, _ := json.Marshal(requests[i])
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				body[field] = value
				bad, _ := json.Marshal(body)
				want := value >= 1 && (field != "revision" || value <= 256)
				if valid := entries[i].definition.Request.Validate(bad, MaxBodyBytes) == nil; valid != want {
					t.Fatal("mutation coordinate bounds changed", i, field, value, valid)
				}
			}
		}
	}
}

func TestAuthoringLifecycleHTTPMCPRegistrationParity(t *testing.T) {
	registry, err := AuthoringRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := authoringLifecycleMCPBindings(registry, service)
	if err != nil {
		t.Fatal(err)
	}
	mcp, err := mcpserver.NewRegistry(bindings)
	if err != nil || len(mcp.Manifest()) != 4 {
		t.Fatal("lifecycle binding inventory", err)
	}
	actions := []string{"reporting.read", "reporting.publish", "reporting.write", "reporting.read"}
	effects := []string{"retained_metadata_read", "block_publication_cas_commit", "report_document_draft_cas_commit", "report_document_transition_cas_commit"}
	for i, entry := range authoringLifecycleEntries(nil) {
		d := entry.definition
		if d.Method != "POST" || !strings.HasPrefix(d.Path, AuthoringPath) || d.Public || d.Action != actions[i] || d.Effect != effects[i] || d.Audit == "" || d.ResourceLoader == "" || d.Replay != "never" || d.MaxBodyBytes != MaxBodyBytes || !reflect.DeepEqual(d.Errors, runtimeErrors()) {
			t.Fatal("shared native lifecycle contract drift", d.ID)
		}
		found := false
		for _, tool := range mcp.Manifest() {
			if tool.Name != d.ID {
				continue
			}
			found = true
			for key, want := range map[string]any{"chartworks/action": d.Action, "chartworks/effect": d.Effect, "chartworks/audit": d.Audit, "chartworks/resourceLoader": d.ResourceLoader, "chartworks/persists": i != 0, "chartworks/maySpend": false} {
				if tool.Meta[key] != want {
					t.Fatal("MCP contract differs", d.ID, key, tool.Meta[key], want)
				}
			}
			if tool.Annotations.ReadOnlyHint != (i == 0) || tool.Annotations.IdempotentHint != (i == 0) || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint || tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Fatal("metadata-only lifecycle effect/replay drift", d.ID)
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
			t.Fatal("lifecycle operation missing from MCP", d.ID)
		}
	}
	public, err := DocumentsRegistry(true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := api.Compose(registry, public); err != nil {
		t.Fatal("native document transitions collided", err)
	}
}

func TestAuthoringLifecycleAdaptersRetainNativeDenial(t *testing.T) {
	// No repositories are configured: every invalid/unauthorized request must
	// fail at the native method before I/O rather than acquire wrapper authority.
	entries := authoringLifecycleEntries(&reporting.Authoring{})
	reader := bootstrapAuthority(t, "reporting.read", "cw.block.read:chart", "cw.report.read:report")
	writer := bootstrapAuthority(t, "reporting.write", "cw.report.write:report")
	for _, operation := range []string{"publish", "reject"} {
		in := reporting.AuthoringReportTransitionRequest{Report: "report", ExpectedVersion: 1, Revision: 1, Operation: operation, Note: "Explicit note"}
		raw, _ := json.Marshal(in)
		if _, err := entries[3].call(t.Context(), writer, "", nil, raw); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("editor write replaced native publish/reject authority", operation, err)
		}
	}
	for i, in := range lifecycleSchemaRequests() {
		raw, _ := json.Marshal(in)
		if i == 0 {
			raw = []byte(`{"report":"report","revision":1}`)
		}
		if _, err := entries[i].call(t.Context(), reader, "", nil, raw); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("read became mutation/authoring authority", i, err)
		}
	}
	for _, raw := range []string{`{"report":"report","revision":1,"expected_version":1,"operation":"Publish","note":""}`, `{"report":"report","revision":1,"expected_version":1,"operation":"reject","note":""}`} {
		if _, err := entries[3].call(t.Context(), writer, "", nil, []byte(raw)); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("closed enum/rejection note lost", err)
		}
	}
	wildcard := bootstrapAuthority(t, "reporting.read", "cw.block.read:*")
	if _, err := entries[0].call(t.Context(), wildcard, "", nil, []byte(`{"block":"chart","revision":1}`)); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("wildcard lifecycle reach admitted", err)
	}
}

func TestAuthoringLifecycleWholeRevisionWireDisclosure(t *testing.T) {
	entry := authoringLifecycleEntries(nil)[0]
	result := reporting.AuthoringLifecycleView{Version: reporting.AuthoringVersion, Stage: "review", Blocks: []reporting.AuthoringBlockLifecycle{{Block: reporting.AuthoringBlockMetadata{State: reporting.State{ID: "chart", Version: 3, DraftRevision: 2}, Revision: 2, Digest: strings.Repeat("a", 64), Actor: "original-author", Private: true, Outputs: []reporting.AuthoringBlockOutput{{ID: "selected", Kind: "chart", Editable: true}, {ID: "not-selected", Kind: "table", Editable: true}}}, PublicationScope: "entire_revision", AudienceEffect: "existing_authorized_readers"}}}
	raw, err := json.Marshal(result)
	if err != nil || entry.definition.Response.ValidateResponse(raw, MaxBodyBytes) != nil {
		t.Fatal("whole-revision metadata rejected", err)
	}
	var got reporting.AuthoringLifecycleView
	if json.Unmarshal(raw, &got) != nil || !reflect.DeepEqual(result, got) || len(got.Blocks[0].Block.Outputs) != 2 {
		t.Fatal("all-output disclosure changed")
	}
	for _, field := range []string{`"validation_fresh":false`, `"can_publish":false`, `"can_review":false`, `"can_reject":false`, `"publication_scope":"entire_revision"`, `"audience_effect":"existing_authorized_readers"`} {
		if !strings.Contains(string(raw), field) {
			t.Fatal("explicit lifecycle/disclosure bit omitted", field)
		}
	}
	for _, mutation := range []struct{ field, value string }{{"publication_scope", "selected_output"}, {"audience_effect", "everyone"}, {"audience_count", "3"}} {
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		body["blocks"].([]any)[0].(map[string]any)[mutation.field] = mutation.value
		bad, _ := json.Marshal(body)
		if entry.definition.Response.ValidateResponse(bad, MaxBodyBytes) == nil {
			t.Fatal("disclosure falsely narrowed or granted", mutation)
		}
	}
}
