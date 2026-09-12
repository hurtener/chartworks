package mcpserver

import (
	"context"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// AppMIME is the established MCP Apps HTML profile, not a host-specific format.
const AppMIME = "text/html;profile=mcp-app"

// MaxAppBytes bounds a bundled static resource independently of tenant payloads.
const MaxAppBytes = 256 << 10

// AppResource is immutable bundled presentation code. It contains neither an
// executable callback nor a tenant-data loader. NewAppResource is its only
// constructor; request-specific values must travel through authorized tools.
type AppResource struct {
	uri, name, description, html string
}

// NewAppResource admits a bounded versioned ui:// resource without external CSP
// origins or requested device permissions. The caller supplies compiled assets,
// not HTML assembled from records, tokens, labels or tool arguments.
func NewAppResource(uri, name, description, html string) (AppResource, error) {
	out := AppResource{uri: uri, name: name, description: description, html: html}
	if !out.valid() {
		return AppResource{}, ErrRegistration
	}
	return out, nil
}

func (a AppResource) valid() bool {
	u, err := url.Parse(a.uri)
	if err != nil || u.Scheme != "ui" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.RawPath != "" || u.Port() != "" || len(a.uri) > 256 || !strings.HasPrefix(u.Path, "/") || strings.HasSuffix(u.Path, "/") {
		return false
	}
	for _, c := range u.Host + u.Path {
		if c != '/' && c != '-' && c != '.' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return false
		}
	}
	if strings.Contains(u.Path, "..") || len(a.name) < 1 || len(a.name) > 80 || len(a.description) < 20 || len(a.description) > 512 || strings.ContainsAny(a.name+a.description, "\x00\r\n") || !utf8.ValidString(a.name+a.description+a.html) || len(a.html) < 128 || len(a.html) > MaxAppBytes || strings.ContainsRune(a.html, 0) || !strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.html)), "<!doctype html>") {
		return false
	}
	return true
}

func (a AppResource) meta() mcp.Meta {
	return mcp.Meta{"ui": map[string]any{
		"csp": map[string]any{"connectDomains": []string{}, "resourceDomains": []string{}, "frameDomains": []string{}, "baseUriDomains": []string{}},
		"permissions": map[string]any{}, "prefersBorder": true,
	}}
}

func (a AppResource) resource() *mcp.Resource {
	return &mcp.Resource{Name: a.name, Description: a.description, URI: a.uri, MIMEType: AppMIME, Meta: a.meta()}
}

// WithAppResource associates an actual tool with immutable presentation code.
// The metadata does not change its signed action, schema, effect or error checks.
func WithAppResource(b Binding, a AppResource) (Binding, error) {
	if b.invoke == nil || b.app != nil || !a.valid() {
		return Binding{}, ErrRegistration
	}
	copy := a
	b.app = &copy
	return b, nil
}

func validateApps(bindings []Binding) error {
	seen := map[string]AppResource{}
	for _, b := range bindings {
		if b.app == nil {
			continue
		}
		if !b.app.valid() {
			return ErrRegistration
		}
		if previous, exists := seen[b.app.uri]; exists && previous != *b.app {
			return ErrRegistration
		}
		seen[b.app.uri] = *b.app
	}
	return nil
}

func (r *Registry) app(uri string) (AppResource, bool) {
	for _, b := range r.bindings {
		if b.app != nil && b.app.uri == uri {
			return *b.app, true
		}
	}
	return AppResource{}, false
}

func (r *Registry) apps() []AppResource {
	out := []AppResource{}
	seen := map[string]bool{}
	for _, b := range r.bindings {
		if b.app != nil && !seen[b.app.uri] {
			seen[b.app.uri] = true
			out = append(out, *b.app)
		}
	}
	return out
}

func (r *Registry) canReadApp(e identity.Envelope, uri string) bool {
	if !e.Valid() || !e.Has("mcp.use") {
		return false
	}
	for _, b := range r.bindings {
		if b.app != nil && b.app.uri == uri && e.Has(b.definition.Action) {
			return true
		}
	}
	return false
}

func (s *Server) readAppResource(ctx context.Context, uri string) (*mcp.ReadResourceResult, error) {
	e, err := requestAuthority(ctx)
	if err != nil || ctx.Value(admissionKey{}) != s {
		return nil, protocolError(jsonrpc.CodeInvalidRequest, "unauthenticated")
	}
	a, exists := s.registry.app(uri)
	if !exists || !s.registry.canReadApp(e, uri) {
		return nil, protocolError(jsonrpc.CodeInvalidParams, "not_found")
	}
	if ctx.Err() != nil {
		return nil, protocolError(-32000, "cancelled_or_timed_out")
	}
	if len(a.html)+4096 > s.settings.MaxResponseBytes {
		return nil, protocolError(-32000, "limit_exceeded")
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: AppMIME, Text: a.html, Meta: a.meta()}}}, nil
}
