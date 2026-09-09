package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
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
	MaxBodyBytes        int                  `json:"max_body_bytes,omitempty"`
	RequestContentType  string               `json:"request_content_type,omitempty"`
	ResponseContentType string               `json:"response_content_type,omitempty"`
	Parameters          []OperationParameter `json:"parameters,omitempty"`
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
	row := OperationInfo{ID: d.ID, Method: method, Path: path, Summary: d.Summary, Action: d.Action, Effect: d.Effect, Audit: d.Audit, ResourceLoader: d.ResourceLoader, Public: d.Auth == "none", Audience: d.Audience, Replay: "never", MaxBodyBytes: d.MaxBodyBytes, Parameters: d.Parameters, SDKMethod: "Invoke", CLICommand: "client call " + d.ID}
	if row.Audience == "" {
		row.Audience = "http"
	}
	if row.Audience == "mcp" {
		row.SDKMethod, row.CLICommand = "MCP", "client mcp"
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

func oneContent(content operationContent) (string, json.RawMessage, bool) {
	if len(content) != 1 {
		return "", nil, false
	}
	for media, value := range content {
		if !catalogLine(media) || len(value.Schema) == 0 || len(value.Schema) > 64<<10 {
			return "", nil, false
		}
		return media, value.Schema, true
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
