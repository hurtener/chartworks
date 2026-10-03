package reportingapi

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

const AuthoringPath = "/v1/reporting/authoring/v1/"

func authoringEntry[I, O any](suffix, action, effect, summary string, call func(context.Context, identity.Envelope, I) (O, error)) runtimeEndpoint {
	entry := runtimeEntry("POST", AuthoringPath+suffix, action, "reporting_authoring_"+suffix+"_v1", summary, func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in I) (O, error) {
		return call(ctx, e, in)
	})
	entry.definition.Effect = effect
	entry.definition.ResourceLoader = "verified Pengui action and exact report/tenant reach; private read/preview plus every server-derived widget dependency; no client profile or audience authority"
	if effect == "retained_metadata_read" {
		entry.definition.Audit = "read_only_no_domain_audit"
	}
	return entry
}

func authoringEntries(service *reporting.Authoring) []runtimeEndpoint {
	return []runtimeEndpoint{
		authoringEntry("capabilities", "reporting.read", "retained_metadata_read", "Read current capability-derived Builder and Consumer hints for an exact report target", service.Capabilities),
		authoringEntry("drafts", "reporting.write", "retained_metadata_read", "List bounded currently authorized private report draft metadata", service.Drafts),
		authoringEntry("read", "reporting.write", "retained_metadata_read", "Reopen an exact report revision with independent read and preview authority", service.Read),
		authoringEntry("create", "reporting.write", "report_document_draft_cas_commit", "Create a private report draft under tenant and exact report write authority", service.Create),
		authoringEntry("save", "reporting.write", "report_document_draft_cas_commit", "Save an immutable private report amendment under exact version CAS", service.Save),
		authoringEntry("widget", "reporting.write", "report_document_draft_cas_commit", "Patch one stable widget through a server-enforced presentation-only allowlist", service.PatchWidget),
		authoringEntry("preview", "reporting.execute", "private_composition_reservation", "Admit an exact private report preview using the existing composition domain", service.Preview),
		authoringEntry("execute", "reporting.execute", "bounded_frozen_source_read_retained_composition", "Explicitly execute one admitted private report preview with current signed dependency authority", service.Execute),
	}
}

// AuthoringRegistry advertises only the bounded manual slice, with exact native
// action scopes and shared schema/error/audit registration for HTTP and MCP.
func AuthoringRegistry() (*api.Registry, error) { return registryForEntries(authoringEntries(nil)) }

func AuthoringHandler(verifier *auth.Verifier, service *reporting.Authoring, next http.Handler) http.Handler {
	if service == nil {
		return http.NotFoundHandler()
	}
	return serveRuntimeEntries(verifier, authoringEntries(service), next)
}

// AuthoringMCPBindings uses the same typed calls and wire schemas as HTTP. The
// optional immutable application resource changes presentation, never authority.
func AuthoringMCPBindings(service *reporting.Authoring, app ...mcpserver.AppResource) ([]mcpserver.Binding, error) {
	if service == nil || len(app) > 1 {
		return nil, mcpserver.ErrRegistration
	}
	registry, err := AuthoringRegistry()
	if err != nil {
		return nil, err
	}
	mapper := func(err error) mcpserver.Fault { _, code := httpFault(err); return mcpserver.Fault{Code: code} }
	out := []mcpserver.Binding{}
	add := func(b mcpserver.Binding, err error) error {
		if err == nil {
			out = append(out, b)
		}
		return err
	}
	bindApp := func(b mcpserver.Binding, err error) (mcpserver.Binding, error) {
		if err == nil && len(app) == 1 {
			return mcpserver.WithAppResource(b, app[0])
		}
		return b, err
	}
	if err = add(bindApp(mcpserver.Bind(registry, "reporting_authoring_capabilities_v1", "reporting_authoring_capabilities_v1", "reporting", "Read current signed target capability hints and open the optional report application. Hints are not grants; operations recheck all dependencies.", service.Capabilities, mapper))); err != nil {
		return nil, err
	}
	if err = add(mcpserver.Bind(registry, "reporting_authoring_drafts_v1", "reporting_authoring_drafts_v1", "reporting", "List private report draft metadata selected by current signed write reach and stored dependencies before pagination. Never returns widget payloads.", service.Drafts, mapper)); err != nil {
		return nil, err
	}
	if err = add(bindApp(mcpserver.Bind(registry, "reporting_authoring_read_v1", "reporting_authoring_read_v1", "reporting", "Read an exact report revision for manual editing. Draft payloads require separate read and private-preview authority; creator labels grant nothing.", service.Read, mapper))); err != nil {
		return nil, err
	}
	if err = add(mcpserver.Bind(registry, "reporting_authoring_create_v1", "reporting_authoring_create_v1", "reporting", "Create a private report draft using tenant and exact report write authority. Validates every selected widget dependency; never publishes.", service.Create, mapper)); err != nil {
		return nil, err
	}
	if err = add(mcpserver.Bind(registry, "reporting_authoring_save_v1", "reporting_authoring_save_v1", "reporting", "Append a private immutable report amendment under exact expected-version CAS. A conflict requires reopening; never overwrites a publication.", service.Save, mapper)); err != nil {
		return nil, err
	}
	if err = add(mcpserver.Bind(registry, "reporting_authoring_preview_v1", "reporting_authoring_preview_v1", "reporting", "Reserve an exact private report preview under a caller operation key. Requires current write, execute and preview authority; does not execute it.", service.Preview, mapper)); err != nil {
		return nil, err
	}
	if err = add(mcpserver.Bind(registry, "reporting_authoring_execute_v1", "reporting_authoring_execute_v1", "reporting", "Explicitly execute an admitted private report preview. May read sources and persist artifacts; no model generation. Never blindly retry unknown outcomes.", service.Execute, mapper)); err != nil {
		return nil, err
	}
	if err = add(mcpserver.Bind(registry, "reporting_authoring_widget_v1", "reporting_authoring_widget_v1", "reporting", "Change only one selected widget's text or presentation under revision and version CAS. The server preserves every other widget, report field and layout; this is edit intent, not a new ACL.", service.PatchWidget, mapper)); err != nil {
		return nil, err
	}
	return out, nil
}
