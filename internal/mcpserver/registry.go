// Package mcpserver provides a bounded Pengui-authenticated MCP adapter over
// actual domain registrations. It owns no analytical state or credential issuer.
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ErrRegistration rejects absent, ambiguous or unclassified service bindings.
var ErrRegistration = errors.New("mcp: invalid service registration")

// Fault is the stable content-free tool error contract. Unknown means a started
// operation may have persisted or incurred cost; callers must inspect receipts
// rather than retrying a mutation blindly. Receipt is supplied only by its owner.
type Fault struct {
	Code    string           `json:"code"`
	Outcome string           `json:"outcome"`
	Receipt *gateway.Receipt `json:"receipt,omitempty"`
}

// ErrorMapper must classify errors, never return native diagnostics. Registration
// and the dispatcher restrict its code to the owning HTTP operation's inventory.
type ErrorMapper func(error) Fault

type effects struct{ readOnly, idempotent, openWorld, persists, paid bool }

func effectFor(effect string) (effects, bool) {
	switch effect {
	case "metadata_read", "retained_metadata_read", "byo_context_read":
		return effects{readOnly: true, idempotent: true}, true
	case "caller_data_transform_no_persistence":
		return effects{readOnly: true, idempotent: true}, true
	case "caller_data_selection_optional_gateway_rank":
		return effects{openWorld: true, paid: true}, true
	case "nlq_routing_and_preflight_commit", "nlq_generation_and_plan_commit", "nlq_validated_read_execution", "nlq_refine_generation_and_plan_commit", "nlq_feedback_commit", "byo_context_retrieval_and_commit", "byo_validated_read_and_receipt":
		return effects{openWorld: true, persists: true, paid: true}, true
	}
	return effects{}, false
}

// Binding is opaque: only Bind can connect a registered contract to a typed core.
type Binding struct {
	name, group, description, resource string
	definition                         api.Definition
	input, output                      *gateway.Schema
	invoke                             func(context.Context, identity.Envelope, json.RawMessage) (json.RawMessage, error)
	classify                           ErrorMapper
	effects                            effects
}

// Bind reuses the registered request/response/action/error/effect contract. GET
// adapters supply a typed argument DTO for their path coordinates. Every POST
// DTO must agree with the actual registered request schema.
func Bind[In, Out any](registry *api.Registry, operationID, name, group, description string, call func(context.Context, identity.Envelope, In) (Out, error), mapper ErrorMapper) (Binding, error) {
	if registry == nil || call == nil || mapper == nil || !toolName(name) || !groupName(group) || len(description) < 20 || len(description) > 1024 || strings.ContainsAny(description, "\x00\r\n") {
		return Binding{}, ErrRegistration
	}
	var d api.Definition
	for _, candidate := range registry.Definitions() {
		if candidate.ID == operationID {
			d = candidate
			break
		}
	}
	ef, ok := effectFor(d.Effect)
	if !ok || d.Public || d.Response == nil || d.Action == "" {
		return Binding{}, ErrRegistration
	}
	input, err := api.SchemaFor(name+"Input", reflect.TypeFor[In](), false)
	if d.Request != nil {
		matched := err == nil && sameSchema(input, d.Request)
		for _, option := range []api.SchemaOption{api.OptionalJSONFields, api.NullableCollections} {
			candidate, checkErr := api.SchemaFor(name+"Input", reflect.TypeFor[In](), false, option)
			if checkErr == nil && sameSchema(candidate, d.Request) {
				matched = true
			}
		}
		if !matched {
			return Binding{}, ErrRegistration
		}
		input = d.Request
	} else if err != nil {
		return Binding{}, ErrRegistration
	}
	shape, err := gateway.DecodeJSON(input.Document(), 65536)
	if err != nil || shape.(map[string]any)["type"] != "object" {
		return Binding{}, ErrRegistration
	}
	actualOutput, err := api.SchemaFor(name+"Body", reflect.TypeFor[Out](), true)
	if err != nil || !sameSchema(actualOutput, d.Response) {
		return Binding{}, ErrRegistration
	}
	faultSchema, err := api.SchemaFor(name+"Fault", reflect.TypeFor[Fault](), true)
	if err != nil {
		return Binding{}, ErrRegistration
	}
	wrapper, _ := json.Marshal(map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"result": d.Response.Document(), "error": faultSchema.Document()}, "oneOf": []any{map[string]any{"required": []string{"result"}}, map[string]any{"required": []string{"error"}}}})
	output, err := gateway.NewSchema(name+"Output", wrapper)
	if err != nil {
		return Binding{}, ErrRegistration
	}
	b := Binding{name: name, group: group, description: description, definition: d, input: input, output: output, classify: mapper, effects: ef}
	b.invoke = func(ctx context.Context, e identity.Envelope, raw json.RawMessage) (json.RawMessage, error) {
		var in In
		if json.Unmarshal(raw, &in) != nil {
			return nil, errArguments
		}
		out, err := call(ctx, e, in)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(struct {
			Result Out `json:"result"`
		}{out})
		if err != nil {
			return nil, errResult
		}
		return data, nil
	}
	return b, nil
}

func sameSchema(a, b *gateway.Schema) bool {
	if a == nil || b == nil {
		return false
	}
	var x, y any
	if json.Unmarshal(a.Document(), &x) != nil || json.Unmarshal(b.Document(), &y) != nil {
		return false
	}
	return reflect.DeepEqual(x, y)
}
func toolName(s string) bool {
	if len(s) < 1 || len(s) > 48 {
		return false
	}
	for _, c := range s {
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
func groupName(s string) bool { return s == "discovery" || s == "query" || s == "byo" || s == "charts" }

// WithResource exposes the same pure read through an exact canonical URI or
// template. Resource reads cannot introduce paid work or persistence.
func WithResource(b Binding, uri string) (Binding, error) {
	if b.invoke == nil || !b.effects.readOnly || b.resource != "" || !validResourcePattern(uri, b.input) {
		return Binding{}, ErrRegistration
	}
	b.resource = uri
	return b, nil
}

// Registry is an immutable inventory of concrete service bindings.
type Registry struct{ bindings []Binding }

// NewRegistry fails on omissions/duplicates rather than advertising a stub.
func NewRegistry(bindings []Binding) (*Registry, error) {
	if len(bindings) < 1 || len(bindings) > 64 {
		return nil, ErrRegistration
	}
	names, ids, resources := map[string]bool{}, map[string]bool{}, map[string]bool{}
	out := append([]Binding{}, bindings...)
	for _, b := range out {
		if b.invoke == nil || b.input == nil || b.output == nil || b.classify == nil || !toolName(b.name) || !groupName(b.group) || names[b.name] || ids[b.definition.ID] {
			return nil, ErrRegistration
		}
		names[b.name], ids[b.definition.ID] = true, true
		if b.resource != "" {
			if resources[b.resource] || !b.effects.readOnly || !validResourcePattern(b.resource, b.input) {
				return nil, ErrRegistration
			}
			for previous := range resources {
				if resourcePatternsOverlap(previous, b.resource) {
					return nil, ErrRegistration
				}
			}
			resources[b.resource] = true
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return &Registry{bindings: out}, nil
}

// SelectGroups removes disabled real groups without replacing them with stubs.
func SelectGroups(bindings []Binding, groups []string) ([]Binding, error) {
	enabled := map[string]bool{}
	for _, g := range groups {
		if !groupName(g) || enabled[g] {
			return nil, ErrRegistration
		}
		enabled[g] = true
	}
	var out []Binding
	for _, b := range bindings {
		if enabled[b.group] {
			out = append(out, b)
		}
	}
	return out, nil
}

func (r *Registry) find(name string) (Binding, bool) {
	i := sort.Search(len(r.bindings), func(i int) bool { return r.bindings[i].name >= name })
	if i < len(r.bindings) && r.bindings[i].name == name {
		return r.bindings[i], true
	}
	return Binding{}, false
}
func (b Binding) tool() *mcp.Tool {
	destructive, open := false, b.effects.openWorld
	return &mcp.Tool{Name: b.name, Description: b.description, InputSchema: b.input.Document(), OutputSchema: b.output.Document(), Annotations: &mcp.ToolAnnotations{ReadOnlyHint: b.effects.readOnly, IdempotentHint: b.effects.idempotent, DestructiveHint: &destructive, OpenWorldHint: &open}, Meta: mcp.Meta{"chartworks/operation": b.definition.ID, "chartworks/action": b.definition.Action, "chartworks/effect": b.definition.Effect, "chartworks/audit": b.definition.Audit, "chartworks/group": b.group, "chartworks/persists": b.effects.persists, "chartworks/maySpend": b.effects.paid}}
}

// Manifest returns detached non-secret registration metadata for parity checks.
func (r *Registry) Manifest() []*mcp.Tool {
	out := make([]*mcp.Tool, 0, len(r.bindings))
	for _, b := range r.bindings {
		out = append(out, b.tool())
	}
	return out
}
