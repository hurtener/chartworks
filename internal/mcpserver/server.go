package mcpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Path is the only MCP mount; it shares the ordinary authenticated server port.
const Path = "/v1/mcp"

// Server holds immutable bindings and bounded admission, never caller tokens or
// analytical sessions. The underlying protocol server is deliberately private.
type Server struct {
	verifier  *auth.Verifier
	registry  *Registry
	settings  config.MCP
	origins   []string
	slots     chan struct{}
	transport http.Handler
}

// New uses the same supported Go SDK as Harbor and the existing Pengui verifier.
// It starts no listener, opens no store, and performs no model/warehouse work.
func New(verifier *auth.Verifier, registry *Registry, settings config.MCP, origins []string) (out *Server, err error) {
	defer func() {
		if recover() != nil {
			out = nil
			err = ErrRegistration
		}
	}()
	if verifier == nil || registry == nil || len(registry.bindings) == 0 || config.ValidateMCP(settings) != nil || len(origins) > 32 {
		return nil, ErrRegistration
	}
	seen := map[string]bool{}
	for _, origin := range origins {
		u, e := url.Parse(origin)
		if e != nil || len(origin) > 256 || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" || seen[origin] {
			return nil, ErrRegistration
		}
		seen[origin] = true
	}
	s := &Server{verifier: verifier, registry: registry, settings: settings.Clone(), origins: append([]string{}, origins...), slots: make(chan struct{}, settings.MaxConcurrent)}
	logger := slog.New(slog.DiscardHandler)
	protocol := mcp.NewServer(&mcp.Implementation{Name: "chartworks", Version: "1"}, &mcp.ServerOptions{Logger: logger, Capabilities: &mcp.ServerCapabilities{}, GetSessionID: func() string { return "" }})
	for _, b := range registry.bindings {
		protocol.AddTool(b.tool(), func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.dispatch(ctx, b.name, req.Params.Arguments), nil
		})
		if b.resource != "" {
			if strings.Contains(b.resource, "{") {
				protocol.AddResourceTemplate(&mcp.ResourceTemplate{Name: b.name, URITemplate: b.resource, MIMEType: "application/json", Description: b.description}, s.readResource)
			} else {
				protocol.AddResource(&mcp.Resource{Name: b.name, URI: b.resource, MIMEType: "application/json", Description: b.description}, s.readResource)
			}
		}
	}
	protocol.AddReceivingMiddleware(s.middleware)
	// Chartworks' mandatory exact host/origin gate also works behind an explicitly
	// configured proxy; it is stricter than the SDK's automatic loopback heuristic
	// and cannot be disabled by MCPGODEBUG compatibility switches.
	s.transport = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return protocol }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, Logger: logger, DisableLocalhostProtection: true})
	return s, nil
}

func (s *Server) admit(ctx context.Context, fn func(context.Context) error) error {
	if ctx == nil {
		return access.ErrUnauthenticated
	}
	e, err := requestAuthority(ctx)
	if err != nil {
		return err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return errBusy
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.settings.Timeout))
	defer cancel()
	ctx, expire := context.WithDeadline(ctx, e.Deadline())
	defer expire()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	ctx = context.WithValue(ctx, admissionKey{}, s)
	return fn(ctx)
}

func (s *Server) middleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(sdkCtx context.Context, method string, req mcp.Request) (out mcp.Result, err error) {
		defer func() {
			if recover() != nil {
				out = nil
				err = protocolError(jsonrpc.CodeInternalError, "unavailable")
			}
		}()
		original, ok := sdkCtx.Value(requestKey{}).(context.Context)
		if !ok || original.Value(admissionKey{}) != s {
			return nil, protocolError(jsonrpc.CodeInvalidRequest, "unauthenticated")
		}
		// The SDK intentionally detaches stream contexts. Reattach each exact request
		// cancellation/deadline, plus SDK cancellation, without sharing caller state.
		ctx, cancel := context.WithCancel(original)
		stop := context.AfterFunc(sdkCtx, cancel)
		defer stop()
		defer cancel()
		e, err := requestAuthority(ctx)
		if err != nil {
			return nil, protocolError(jsonrpc.CodeInvalidRequest, "unauthenticated")
		}
		switch method {
		case "tools/list":
			p, ok := req.GetParams().(*mcp.ListToolsParams)
			if !ok || p != nil && p.Cursor != "" {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "invalid_request")
			}
			return s.listTools(e), nil
		case "resources/list":
			p, ok := req.GetParams().(*mcp.ListResourcesParams)
			if !ok || p != nil && p.Cursor != "" {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "invalid_request")
			}
			return s.listResources(e), nil
		case "resources/templates/list":
			p, ok := req.GetParams().(*mcp.ListResourceTemplatesParams)
			if !ok || p != nil && p.Cursor != "" {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "invalid_request")
			}
			return s.listTemplates(e), nil
		case "tools/call":
			p, ok := req.GetParams().(*mcp.CallToolParamsRaw)
			if !ok || p == nil {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "invalid_request")
			}
			if _, ok = s.registry.find(p.Name); !ok {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "not_found")
			}
		case "resources/read":
			p, ok := req.GetParams().(*mcp.ReadResourceParams)
			if !ok || p == nil {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "invalid_request")
			}
			if _, _, ok = s.registry.resource(p.URI); !ok {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "not_found")
			}
		case "initialize", "notifications/initialized", "ping":
		default:
			return nil, protocolError(jsonrpc.CodeMethodNotFound, "not_found")
		}
		return next(ctx, method, req)
	}
}
func protocolError(code int64, message string) error {
	return &jsonrpc.Error{Code: code, Message: message}
}

func (s *Server) listTools(e identity.Envelope) *mcp.ListToolsResult {
	out := &mcp.ListToolsResult{Tools: []*mcp.Tool{}}
	for _, b := range s.registry.bindings {
		if e.Has(b.definition.Action) {
			out.Tools = append(out.Tools, b.tool())
		}
	}
	return out
}
func (s *Server) listResources(e identity.Envelope) *mcp.ListResourcesResult {
	out := &mcp.ListResourcesResult{Resources: []*mcp.Resource{}}
	for _, b := range s.registry.bindings {
		if b.resource != "" && !strings.Contains(b.resource, "{") && e.Has(b.definition.Action) {
			out.Resources = append(out.Resources, &mcp.Resource{Name: b.name, URI: b.resource, MIMEType: "application/json", Description: b.description})
		}
	}
	return out
}
func (s *Server) listTemplates(e identity.Envelope) *mcp.ListResourceTemplatesResult {
	out := &mcp.ListResourceTemplatesResult{ResourceTemplates: []*mcp.ResourceTemplate{}}
	for _, b := range s.registry.bindings {
		if strings.Contains(b.resource, "{") && e.Has(b.definition.Action) {
			out.ResourceTemplates = append(out.ResourceTemplates, &mcp.ResourceTemplate{Name: b.name, URITemplate: b.resource, MIMEType: "application/json", Description: b.description})
		}
	}
	return out
}
func (s *Server) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	b, args, ok := s.registry.resource(req.Params.URI)
	if !ok {
		return nil, protocolError(jsonrpc.CodeInvalidParams, "not_found")
	}
	result := s.dispatch(ctx, b.name, args)
	if result.IsError {
		return nil, protocolError(-32000, s.faultFor(result).Code)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: req.Params.URI, MIMEType: "application/json", Text: text}}}, nil
}

var errBusy = errors.New("mcp: request capacity exhausted")
