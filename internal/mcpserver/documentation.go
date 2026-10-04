package mcpserver

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/staticdocs"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// WithDocumentation associates a bounded immutable catalog with an existing
// native read operation. It registers resources, not additional tools. The
// catalog action must match the registered HTTP/tool action exactly.
func WithDocumentation(b Binding, catalog *staticdocs.Catalog) (Binding, error) {
	if b.invoke == nil || !b.effects.readOnly || b.documentation != nil || catalog == nil || catalog.Action() != b.definition.Action || len(catalog.References()) == 0 {
		return Binding{}, ErrRegistration
	}
	b.documentation = catalog
	return b, nil
}

func validateDocumentation(bindings []Binding) error {
	catalogs := 0
	seen, count, total := map[string]bool{}, 0, 0
	for _, b := range bindings {
		if b.documentation == nil {
			continue
		}
		catalogs++
		if catalogs > 1 {
			return ErrRegistration
		}
		if !b.effects.readOnly || b.documentation.Action() != b.definition.Action {
			return ErrRegistration
		}
		for _, ref := range b.documentation.References() {
			if seen[ref.URI] {
				return ErrRegistration
			}
			seen[ref.URI] = true
			count++
			total += ref.Bytes
			if count > staticdocs.MaxDocuments || total > staticdocs.MaxCatalogBytes {
				return ErrRegistration
			}
			for _, other := range bindings {
				parts, valid := resourceParts(other.resource)
				if valid && len(parts) > 2 && parts[0] == "report_app" && (parts[1] == "docs" || strings.HasPrefix(parts[1], "{")) {
					return ErrRegistration
				}
			}
		}
	}
	return nil
}

func (r *Registry) documentation(uri string) (*staticdocs.Catalog, bool) {
	// Dispatch the reserved namespace before looking up exact content so even an
	// unknown document requires the catalog's native action first.
	if !strings.HasPrefix(uri, staticdocs.Namespace) {
		return nil, false
	}
	for _, b := range r.bindings {
		if b.documentation != nil {
			return b.documentation, true
		}
	}
	return nil, false
}

func documentationMeta(ref staticdocs.Reference, action string) mcp.Meta {
	return mcp.Meta{"chartworks/document": ref, "chartworks/action": action, "chartworks/effect": "retained_metadata_read"}
}
func documentationResource(ref staticdocs.Reference, action string) *mcp.Resource {
	return &mcp.Resource{Name: ref.Name, URI: ref.URI, MIMEType: ref.MIMEType, Description: ref.Description, Meta: documentationMeta(ref, action)}
}

func (s *Server) readDocumentation(ctx context.Context, catalog *staticdocs.Catalog, uri string) (*mcp.ReadResourceResult, error) {
	e, err := requestAuthority(ctx)
	if err != nil || ctx.Value(admissionKey{}) != s {
		return nil, protocolError(jsonrpc.CodeInvalidRequest, "unauthenticated")
	}
	document, err := catalog.Read(ctx, e, uri)
	if err != nil {
		code := "unavailable"
		switch err {
		case access.ErrNotFound:
			code = "not_found"
		case access.ErrUnauthenticated:
			code = "unauthenticated"
		case access.ErrForbidden:
			code = "forbidden"
		case context.Canceled, context.DeadlineExceeded:
			code = "cancelled_or_timed_out"
		}
		return nil, protocolError(-32000, code)
	}
	out := &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: document.Reference.MIMEType, Text: document.Text, Meta: documentationMeta(document.Reference, catalog.Action())}}}
	wire, err := json.Marshal(out)
	if err != nil || len(wire)+512 > s.settings.MaxResponseBytes {
		return nil, protocolError(-32000, "limit_exceeded")
	}
	return out, nil
}

func (s *Server) documentationResources(e identity.Envelope) []*mcp.Resource {
	out := []*mcp.Resource{}
	if !e.Valid() || !e.Has("mcp.use") {
		return out
	}
	for _, b := range s.registry.bindings {
		if b.documentation != nil && e.Has(b.definition.Action) {
			for _, ref := range b.documentation.References() {
				out = append(out, documentationResource(ref, b.definition.Action))
			}
		}
	}
	return out
}
