package chartworks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/gateway"
)

// ErrInvalidCall describes invalid local coordinates/options without echoing input.
var ErrInvalidCall = errors.New("chartworks: invalid operation call")

// ErrUnknownOperation means the current server did not register the operation.
// A planned report, artifact or maintenance endpoint is never synthesized locally.
var ErrUnknownOperation = errors.New("chartworks: operation is not registered")

// ErrUnsafeRetry rejects replay without a registered read or required logical key.
var ErrUnsafeRetry = errors.New("chartworks: operation cannot be retried safely")

// ErrInvalidCatalog rejects ambiguous or unsupported registration metadata.
var ErrInvalidCatalog = errors.New("chartworks: invalid operation catalog")

const operationCatalogLimit = 4 << 20

// OperationParameter is the exact bounded path/query/header contract published
// by the server. It is metadata, never a substitute for signed resource reach.
type OperationParameter struct {
	Name        string          `json:"name"`
	In          string          `json:"in"`
	Required    bool            `json:"required"`
	Description string          `json:"description,omitempty"`
	Schema      json.RawMessage `json:"schema"`
}

// OperationError is one stable owner-registered HTTP failure. Receipt marks
// failures that may carry bounded usage evidence; native diagnostics, prompts,
// SQL and result values are never part of the operation catalog.
type OperationError struct {
	Status  int    `json:"status"`
	Code    string `json:"code"`
	Receipt bool   `json:"receipt,omitempty"`
}

// OperationInfo is one row of the generated HTTP/MCP/SDK/CLI operation matrix.
// The HTTP inventory describes installed routes, not the caller's permissions.
// MCPTool is populated only when a separately authorized MCP catalog is supplied.
// Schemas and rows are detached; changing a returned row cannot change Invoke.
type OperationInfo struct {
	ID                  string               `json:"id"`
	Method              string               `json:"method"`
	Path                string               `json:"path"`
	Summary             string               `json:"summary"`
	Action              string               `json:"action,omitempty"`
	Effect              string               `json:"effect"`
	Audit               string               `json:"audit"`
	ResourceLoader      string               `json:"resource_loader"`
	Public              bool                 `json:"public"`
	Audience            string               `json:"audience"`
	Replay              string               `json:"replay"`
	Interaction         string               `json:"interaction,omitempty"`
	MCPDisposition      string               `json:"mcp_disposition"`
	MaxBodyBytes        int                  `json:"max_body_bytes,omitempty"`
	RequestContentType  string               `json:"request_content_type,omitempty"`
	ResponseContentType string               `json:"response_content_type,omitempty"`
	Parameters          []OperationParameter `json:"parameters,omitempty"`
	Errors              []OperationError     `json:"errors,omitempty"`
	RequestSchema       json.RawMessage      `json:"request_schema,omitempty"`
	ResponseSchema      json.RawMessage      `json:"response_schema,omitempty"`
	SDKMethod           string               `json:"sdk_method"`
	CLICommand          string               `json:"cli_command"`
	MCPTool             string               `json:"mcp_tool,omitempty"`
}

// Operations reads the current server's generated document. There is no saved
// business state, speculative endpoint generator or process-global authority.
func (c *Client) Operations(ctx context.Context) ([]OperationInfo, error) {
	var document json.RawMessage
	if err := c.callLimit(ctx, http.MethodGet, "/openapi.json", "", nil, &document, operationCatalogLimit); err != nil {
		return nil, err
	}
	return ParseOperations(document)
}

// ParseOperations generates the same matrix for an operator-exported document
// or an actual api.Registry.OpenAPI result. Only Chartworks' registered contract
// is accepted; external URLs, references and arbitrary paths are not followed.
func ParseOperations(document []byte) ([]OperationInfo, error) {
	if _, err := gateway.DecodeJSON(document, operationCatalogLimit); err != nil {
		return nil, ErrInvalidCatalog
	}
	var root struct {
		Version string                                `json:"openapi"`
		Paths   map[string]map[string]json.RawMessage `json:"paths"`
	}
	if json.Unmarshal(document, &root) != nil || root.Version != "3.1.1" || len(root.Paths) == 0 || len(root.Paths) > 256 {
		return nil, ErrInvalidCatalog
	}
	rows := make([]OperationInfo, 0, len(root.Paths))
	seen := map[string]bool{}
	for path, methods := range root.Paths {
		if !operationPath(path) || len(methods) == 0 {
			return nil, ErrInvalidCatalog
		}
		for method, raw := range methods {
			row, err := parseOperation(strings.ToUpper(method), path, raw)
			if err != nil || method != strings.ToLower(method) || seen[row.ID] {
				return nil, ErrInvalidCatalog
			}
			seen[row.ID] = true
			rows = append(rows, row)
			if len(rows) > 256 {
				return nil, ErrInvalidCatalog
			}
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

type operationContent map[string]struct {
	Schema json.RawMessage `json:"schema"`
}

type operationDocument struct {
	Replay         string               `json:"x-chartworks-replay"`
	Interaction    string               `json:"x-chartworks-interaction"`
	ID             string               `json:"operationId"`
	Summary        string               `json:"summary"`
	Auth           string               `json:"x-chartworks-auth"`
	Action         string               `json:"x-chartworks-action"`
	Effect         string               `json:"x-chartworks-effect"`
	Audit          string               `json:"x-chartworks-audit"`
	ResourceLoader string               `json:"x-chartworks-resource-loader"`
	Audience       string               `json:"x-chartworks-audience"`
	MaxBodyBytes   int                  `json:"x-chartworks-max-body-bytes"`
	Parameters     []OperationParameter `json:"parameters"`
	RequestBody    *struct {
		Required bool             `json:"required"`
		Content  operationContent `json:"content"`
	} `json:"requestBody"`
	Responses map[string]struct {
		Content operationContent `json:"content"`
	} `json:"responses"`
}

func parseOperation(method, path string, raw json.RawMessage) (OperationInfo, error) {
	var d operationDocument
	if json.Unmarshal(raw, &d) != nil || !wireID(d.ID) || !catalogLine(d.Summary) || !catalogLine(d.Effect) || !catalogLine(d.Audit) || !catalogLine(d.ResourceLoader) {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if d.Auth != "bearer" && d.Auth != "none" || d.Auth == "bearer" && (!catalogLine(d.Action) || (d.Audience != "http" && d.Audience != "mcp")) {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if d.Auth == "bearer" && !strings.HasPrefix(path, "/v1/") && path != "/metrics" || d.Auth == "none" && strings.HasPrefix(path, "/v1/") {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if !interactionRole(d.Interaction) {
		return OperationInfo{}, ErrInvalidCatalog
	}
	row := OperationInfo{ID: d.ID, Method: method, Path: path, Summary: d.Summary, Action: d.Action, Effect: d.Effect, Audit: d.Audit, ResourceLoader: d.ResourceLoader, Public: d.Auth == "none", Audience: d.Audience, Replay: "never", Interaction: d.Interaction, MCPDisposition: "not_queried", MaxBodyBytes: d.MaxBodyBytes, Parameters: d.Parameters, SDKMethod: "Invoke", CLICommand: "client call " + d.ID}
	if row.Audience == "" {
		row.Audience = "http"
	}
	if row.Audience == "mcp" {
		row.SDKMethod, row.CLICommand, row.MCPDisposition = "MCP", "client mcp", "transport"
	}
	switch method {
	case http.MethodGet, http.MethodHead:
		if d.RequestBody != nil || d.MaxBodyBytes != 0 {
			return OperationInfo{}, ErrInvalidCatalog
		}
		row.Replay = "read"
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		if d.RequestBody == nil || !d.RequestBody.Required || d.MaxBodyBytes < 1 || d.MaxBodyBytes > 100<<20 {
			return OperationInfo{}, ErrInvalidCatalog
		}
		media, schema, ok := oneContent(d.RequestBody.Content)
		if !ok || media != "application/json" && media != "application/octet-stream" || media == "application/json" && d.MaxBodyBytes > 10<<20 {
			return OperationInfo{}, ErrInvalidCatalog
		}
		row.RequestContentType, row.RequestSchema = media, schema
	default:
		return OperationInfo{}, ErrInvalidCatalog
	}
	response, exists := d.Responses["200"]
	if !exists {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if method != http.MethodHead {
		media, schema, ok := oneContent(response.Content)
		if !ok {
			return OperationInfo{}, ErrInvalidCatalog
		}
		row.ResponseContentType, row.ResponseSchema = media, schema
	}
	for statusText, response := range d.Responses {
		status, err := strconv.Atoi(statusText)
		if err != nil || status < 200 || status > 599 {
			return OperationInfo{}, ErrInvalidCatalog
		}
		if status < 400 || status == http.StatusMethodNotAllowed {
			continue
		}
		_, schema, ok := oneContent(response.Content)
		if !ok {
			return OperationInfo{}, ErrInvalidCatalog
		}
		var fault struct {
			Properties map[string]struct {
				Enum []string `json:"enum"`
			} `json:"properties"`
		}
		if json.Unmarshal(schema, &fault) != nil || len(fault.Properties["error"].Enum) == 0 || len(fault.Properties["error"].Enum) > 32 {
			return OperationInfo{}, ErrInvalidCatalog
		}
		_, receipt := fault.Properties["receipt"]
		codes := map[string]bool{}
		for _, code := range fault.Properties["error"].Enum {
			if !wireID(code) || codes[code] {
				return OperationInfo{}, ErrInvalidCatalog
			}
			codes[code] = true
			row.Errors = append(row.Errors, OperationError{Status: status, Code: code, Receipt: receipt})
		}
	}
	sort.Slice(row.Errors, func(i, j int) bool {
		if row.Errors[i].Status != row.Errors[j].Status {
			return row.Errors[i].Status < row.Errors[j].Status
		}
		return row.Errors[i].Code < row.Errors[j].Code
	})
	if !row.Public && (!hasOperationError(row.Errors, 401) || !hasOperationError(row.Errors, 403)) {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if len(d.Parameters) > 64 {
		return OperationInfo{}, ErrInvalidCatalog
	}
	parameters := map[string]bool{}
	pathParameter := false
	for _, p := range d.Parameters {
		key := p.In + ":" + strings.ToLower(p.Name)
		if !wireID(p.Name) || len(p.Schema) == 0 || len(p.Schema) > 64<<10 || parameters[key] {
			return OperationInfo{}, ErrInvalidCatalog
		}
		parameters[key] = true
		switch p.In {
		case "path":
			if p.Name != "id" || !p.Required || !strings.Contains(path, "{id}") {
				return OperationInfo{}, ErrInvalidCatalog
			}
			pathParameter = true
		case "query":
		case "header":
			if strings.EqualFold(p.Name, "Idempotency-Key") && p.Required && row.Replay == "never" && row.Audience == "http" && !row.Public {
				if d.Replay == "keyed" {
					row.Replay = "keyed"
				}
			}
		default:
			return OperationInfo{}, ErrInvalidCatalog
		}
	}
	if strings.Contains(path, "{id}") != pathParameter {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if d.Replay != "" && d.Replay != "never" && d.Replay != row.Replay {
		return OperationInfo{}, ErrInvalidCatalog
	}
	if d.Replay == "never" {
		row.Replay = "never"
	}
	return row, nil
}

func interactionRole(value string) bool {
	switch value {
	case "", "query_start_or_clarify", "query_progress_or_clarify", "query_cancel", "query_result", "query_view", "query_feedback", "query_refine_or_clarify":
		return true
	default:
		return false
	}
}

func hasOperationError(errors []OperationError, status int) bool {
	for _, item := range errors {
		if item.Status == status {
			return true
		}
	}
	return false
}

func oneContent(content operationContent) (string, json.RawMessage, bool) {
	if len(content) != 1 {
		return "", nil, false
	}
	for media, value := range content {
		if !catalogLine(media) || len(value.Schema) == 0 {
			return "", nil, false
		}
		// OpenAPI indentation is transport formatting, not schema content. Keep
		// the existing 4 MiB catalog and 64 KiB schema bounds; never increase
		// gateway input limits to admit a pretty-printed reporting definition.
		var compact bytes.Buffer
		if json.Compact(&compact, value.Schema) != nil || compact.Len() > 64<<10 {
			return "", nil, false
		}
		return media, json.RawMessage(compact.Bytes()), true
	}
	return "", nil, false
}

func catalogLine(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n\t")
}

func operationPath(path string) bool {
	if len(path) > 256 || !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") {
		return false
	}
	ids := 0
	for _, segment := range strings.Split(path[1:], "/") {
		if segment == "{id}" {
			ids++
			continue
		}
		if !wireID(segment) || segment == "." || segment == ".." {
			return false
		}
	}
	return ids <= 1
}
