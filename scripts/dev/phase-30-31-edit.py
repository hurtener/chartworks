"""Temporary branch-only editing aid; remove before submitting the final PR."""
from pathlib import Path
import subprocess


def replace(path, old, new):
    file = Path(path)
    text = file.read_text()
    if new in text:
        return
    if text.count(old) != 1:
        raise RuntimeError(f"Expected exactly one edit anchor in {path}: {old[:80]!r}")
    file.write_text(text.replace(old, new, 1))


replace('internal/reporting/delivery_view.go', 'charts.DefaultLimits()', 'charts.Defaults()')
replace('internal/reporting/delivery_view.go', 'out.Filters = description.Filters', 'out.Filters = description.Filters\n\t\t\tif out.Locale == "" { out.Locale = description.Resource.Locale }\n\t\t\tif out.Timezone == "" { out.Timezone = description.Timezone }')
replace('internal/reporting/delivery.go', 'func deliveryKind(kind string) bool', '// CanExecute reports actual configured admission support, not user authority.\nfunc (s *Delivery) CanExecute() bool { return s != nil && s.blocks.CanValidate() }\n\nfunc deliveryKind(kind string) bool')
replace('web/report-viewer/styles.css', '.line{fill:none;', '.chart .line{fill:none;')

replace('internal/reportingapi/runtime.go', '''func RuntimeRegistry(execution, planning bool) (*api.Registry, error) {
	entries := runtimeEntries(nil, nil, execution, planning)
''', '''func RuntimeRegistry(execution, planning bool) (*api.Registry, error) {
	return registryForEntries(runtimeEntries(nil, nil, execution, planning))
}

func registryForEntries(entries []runtimeEndpoint) (*api.Registry, error) {
''')
replace('internal/reportingapi/runtime.go', '''	registry, err := RuntimeRegistry(execution, planning)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	entries := runtimeEntries(runs, proposals, execution, planning)
	calls := map[string]runtimeEndpoint{}''', '''	return serveRuntimeEntries(verifier, runtimeEntries(runs, proposals, execution, planning), next)
}

func serveRuntimeEntries(verifier *auth.Verifier, entries []runtimeEndpoint, next http.Handler) http.Handler {
	if verifier == nil || next == nil { return http.NotFoundHandler() }
	registry, err := registryForEntries(entries)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { failure(w, err) })
	}
	calls := map[string]runtimeEndpoint{}''')
replace('internal/reportingapi/runtime.go', '''		encoded, err := json.Marshal(out)''', '''		if !e.Valid() { failure(w, access.ErrUnauthenticated); return }
		encoded, err := json.Marshal(out)''')
replace('internal/reportingapi/http.go', 'func failure(w http.ResponseWriter, err error) {\n\tstatus, code :=', 'func httpFault(err error) (int, string) {\n\tstatus, code :=')
replace('internal/reportingapi/http.go', '''	headers(w)
	w.WriteHeader(status)
''', '''	return status, code
}

func failure(w http.ResponseWriter, err error) {
	status, code := httpFault(err)
	headers(w)
	w.WriteHeader(status)
''')

replace('internal/mcpserver/registry.go', 'case "nlq_routing_and_preflight_commit",', 'case "bounded_source_read_optional_model_retained_artifact", "nlq_routing_and_preflight_commit",')
replace('internal/mcpserver/registry.go', 'type Binding struct {\n', 'type Binding struct {\n\tapp *AppResource\n')
replace('internal/mcpserver/registry.go', 's == "byo" || s == "charts"', 's == "byo" || s == "charts" || s == "reporting"')
replace('internal/mcpserver/registry.go', '\tsort.Slice(out, func(i, j int) bool', '\tif err := validateApps(out); err != nil { return nil, err }\n\tsort.Slice(out, func(i, j int) bool')
replace('internal/mcpserver/registry.go', '\treturn &mcp.Tool{Name: b.name,', '\tout := &mcp.Tool{Name: b.name,')
replace('internal/mcpserver/registry.go', '''"chartworks/maySpend": b.effects.paid}}
}''', '''"chartworks/maySpend": b.effects.paid}}
	if b.app != nil { out.Meta["ui"] = map[string]any{"resourceUri": b.app.uri, "visibility": []string{"model", "app"}} }
	return out
}''')
replace('internal/mcpserver/server.go', '\tprotocol.AddReceivingMiddleware(s.middleware)', '\tfor _, app := range registry.apps() { protocol.AddResource(app.resource(), s.readResource) }\n\tprotocol.AddReceivingMiddleware(s.middleware)')
replace('internal/mcpserver/server.go', '''			if _, _, ok = s.registry.resource(p.URI); !ok {
				return nil, protocolError(jsonrpc.CodeInvalidParams, "not_found")
			}''', '''			if _, _, ok = s.registry.resource(p.URI); !ok {
				if _, exists := s.registry.app(p.URI); !exists { return nil, protocolError(jsonrpc.CodeInvalidParams, "not_found") }
			}''')
replace('internal/mcpserver/server.go', '''	return out
}
func (s *Server) listTemplates''', '''	for _, app := range s.registry.apps() {
		if s.registry.canReadApp(e, app.uri) { out.Resources = append(out.Resources, app.resource()) }
	}
	return out
}
func (s *Server) listTemplates''')
replace('internal/mcpserver/server.go', '''func (s *Server) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
''', '''func (s *Server) readResource(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if _, exists := s.registry.app(req.Params.URI); exists { return s.readAppResource(ctx, req.Params.URI) }
''')
replace('internal/mcpserver/apps.go', '"context"\n', '"context"\n\t"encoding/json"\n')
replace('internal/mcpserver/apps.go', '''	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: AppMIME, Text: a.html, Meta: a.meta()}}}, nil''', '''	out := &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: AppMIME, Text: a.html, Meta: a.meta()}}}
	wire, encodeErr := json.Marshal(out)
	if encodeErr != nil || len(wire)+512 > s.settings.MaxResponseBytes { return nil, protocolError(-32000, "limit_exceeded") }
	return out, nil''')
replace('internal/config/mcp.go', 'Groups: []string{"discovery", "query", "byo", "charts"}', 'Groups: []string{"discovery", "query", "byo", "charts", "reporting"}')
replace('internal/config/mcp.go', 'len(m.Groups) > 4', 'len(m.Groups) > 5')
replace('internal/config/mcp.go', 'g != "byo" && g != "charts")', 'g != "byo" && g != "charts" && g != "reporting")')

path = Path('internal/foundation/documents.go')
source = path.read_text()
if '*reporting.Delivery' not in source:
    source = source.replace('(*api.Registry, http.Handler, error)', '(*api.Registry, *reporting.Delivery, http.Handler, error)')
    source = source.replace('return nil, nil, err', 'return nil, nil, nil, err')
    source = source.replace('return registry, reportingapi.DocumentsHandler(verifier, documents, compositions, next), nil', '''delivery, err := reporting.NewDelivery(blocks, runs, documents, compositions, db, limits.Viewer)
	if err != nil { return nil, nil, nil, err }
	deliveryRegistry, err := reportingapi.DeliveryRegistry(delivery.CanExecute())
	if err != nil { return nil, nil, nil, err }
	registry, err = api.Compose(registry, deliveryRegistry)
	if err != nil { return nil, nil, nil, err }
	handler := reportingapi.DocumentsHandler(verifier, documents, compositions, next)
	return registry, delivery, reportingapi.DeliveryHandler(verifier, delivery, delivery.CanExecute(), handler), nil''')
    path.write_text(source)
replace('internal/foundation/work.go', 'documentRegistry, handler, err := mountDocuments', 'documentRegistry, delivery, handler, err := mountDocuments')
replace('internal/foundation/work.go', 'byo, chartService, w.registry, w.handler)', 'byo, chartService, w.registry, w.handler, delivery)')
replace('internal/foundation/mcp.go', '"github.com/hurtener/chartworks/internal/nlqexec"', '"github.com/hurtener/chartworks/internal/nlqexec"\n\t"github.com/hurtener/chartworks/internal/reporting"\n\t"github.com/hurtener/chartworks/internal/reportingapi"')
replace('internal/foundation/mcp.go', 'registry *api.Registry, next http.Handler) (*api.Registry, http.Handler, error)', 'registry *api.Registry, next http.Handler, delivery ...*reporting.Delivery) (*api.Registry, http.Handler, error)')
replace('internal/foundation/mcp.go', '\tselected, err := mcpserver.SelectGroups', '''	if len(delivery) > 1 { return nil, nil, mcpserver.ErrRegistration }
	if len(delivery) == 1 && delivery[0] != nil {
		group, err := reportingapi.DeliveryMCPBindings(delivery[0], delivery[0].CanExecute())
		if err != nil { return nil, nil, err }
		bindings = append(bindings, group...)
	}
	selected, err := mcpserver.SelectGroups''')

files = subprocess.check_output(['git','ls-files','*.go'],text=True).splitlines()
subprocess.run(['gofmt','-w',*files],check=True)
subprocess.run(['node','--check','web/report-viewer/app.js'],check=True)
