package mcpserver

import (
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
)

// Protocol envelopes are bounded and closed. Each tools/call argument object is
// additionally validated against its real binding, not this transport envelope.
var requestSchema, requestSchemaError = gateway.NewSchema("mcpRequest", []byte(`{
 "type":"object","additionalProperties":false,"required":["jsonrpc","method"],
 "properties":{
  "jsonrpc":{"const":"2.0"},
  "id":{"oneOf":[{"type":"string","minLength":1,"maxLength":128,"pattern":"^[A-Za-z0-9_.:-]+$"},{"type":"integer","minimum":0,"maximum":9007199254740991}]},
  "method":{"enum":["initialize","notifications/initialized","ping","tools/list","tools/call","resources/list","resources/templates/list","resources/read"]},
  "params":{"type":"object"}
 },
 "allOf":[
  {"if":{"properties":{"method":{"const":"notifications/initialized"}}},"then":{"not":{"required":["id"]}},"else":{"required":["id"]}},
  {"if":{"properties":{"method":{"enum":["initialize","tools/call","resources/read"]}}},"then":{"required":["params"]}}
 ]
}`))
var responseSchema, responseSchemaError = gateway.NewSchema("mcpResponse", []byte(`{
 "type":"object","additionalProperties":false,"required":["jsonrpc","id"],
 "properties":{
  "jsonrpc":{"const":"2.0"},"id":{"type":["string","integer","null"]},"result":{"type":"object"},
  "error":{"type":"object","additionalProperties":false,"required":["code","message"],"properties":{"code":{"type":"integer"},"message":{"type":"string"}}}
 },"oneOf":[{"required":["result"]},{"required":["error"]}]
}`))

// HTTPRegistry extends phase 21 with the actual MCP mount and MCP audience.
// Domain definitions remain the source of tool schemas, actions and effects.
func HTTPRegistry(settings config.MCP) (*api.Registry, error) {
	if config.ValidateMCP(settings) != nil || requestSchemaError != nil || responseSchemaError != nil {
		return nil, ErrRegistration
	}
	return api.New([]api.Definition{{Operation: api.Operation{Method: "POST", Path: Path, Action: "mcp.use", Effect: "delegated_registered_tool_or_resource"}, Surface: auth.MCP, ID: "mcpTransport", Summary: "Call the bounded stateless Pengui-authenticated MCP transport", ResourceLoader: "mcpserver dispatcher followed by registered domain service", Audit: "registered domain audit; protocol metadata reads have no mutation audit", MaxBodyBytes: settings.MaxRequestBytes, Request: requestSchema, Response: responseSchema, EmptySuccess: []int{202}, Headers: []api.Parameter{
		{Name: "Accept", In: "header", Required: true, Type: "string", Min: 1, Max: maxAcceptBytes, Description: "Must accept application/json and text/event-stream; responses use JSON"},
		{Name: "Mcp-Protocol-Version", In: "header", Type: "string", Min: 10, Max: 10, Description: "Negotiated SDK protocol version; omitted versions use 2025-03-26"},
	}, Errors: []api.ErrorResponse{{Status: 400, Code: "invalid_request"}, {Status: 401, Code: "unauthenticated"}, {Status: 401, Code: "unauthorized"}, {Status: 403, Code: "forbidden"}, {Status: 403, Code: "forbidden_origin"}, {Status: 404, Code: "not_found"}, {Status: 413, Code: "limit_exceeded"}, {Status: 415, Code: "unsupported_media_type"}, {Status: 429, Code: "busy"}, {Status: 503, Code: "unavailable"}, {Status: 504, Code: "cancelled_or_timed_out"}}}})
}
