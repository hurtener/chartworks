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
	reportviewer "github.com/hurtener/chartworks/web/report-viewer"
)

func deliveryEntry[I, O any](path, id, summary string, call func(context.Context, identity.Envelope, I) (O, error)) runtimeEndpoint {
	entry := runtimeEntry("POST", path, "reporting.read", id, summary, func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in I) (O, error) {
		return call(ctx, e, in)
	})
	entry.definition.Effect = "retained_metadata_read"
	entry.definition.Audit = "read_only_no_domain_audit"
	return entry
}

func deliveryEntries(service *reporting.Delivery, execution bool) []runtimeEndpoint {
	entries := []runtimeEndpoint{
		deliveryEntry("/v1/reporting/search", "reportingSearch", "Search authorized published reporting metadata without SQL or values", service.Search),
		deliveryEntry("/v1/reporting/describe", "reportingDescribe", "Describe published outputs and typed business filters without execution", service.Describe),
		deliveryEntry("/v1/reporting/runs", "reportingRuns", "List authorized retained reporting artifact metadata", service.Runs),
		deliveryEntry("/v1/reporting/view", "reportingView", "Read one exact authorized retained output or table page without execution", service.View),
	}
	if execution {
		entry := deliveryEntry("/v1/reporting/run", "reportingRun", "Explicitly run a published reporting revision with fresh signed execution authority", service.Run)
		entry.definition.Action = "reporting.execute"
		entry.definition.Effect = "bounded_source_read_optional_model_retained_artifact"
		entry.definition.Audit = "fenced operation and retained effects; no SQL, rows or credentials"
		entries = append(entries, entry)
	}
	return entries
}

// DeliveryRegistry is also the SDK source. It advertises no run implementation
// when the configured source execution lane is unavailable.
func DeliveryRegistry(execution bool) (*api.Registry, error) {
	return registryForEntries(deliveryEntries(nil, execution))
}

// DeliveryHandler uses the existing strict HTTP decoder, Pengui verifier and
// bounded admission wrapper rather than introducing a second API policy path.
func DeliveryHandler(verifier *auth.Verifier, service *reporting.Delivery, execution bool, next http.Handler) http.Handler {
	if service == nil {
		return http.NotFoundHandler()
	}
	return serveRuntimeEntries(verifier, deliveryEntries(service, execution), next)
}

// DeliveryMCPBindings binds the exact HTTP DTOs to the same domain methods.
// Visibility hints never authorize a call; every call reaches signed checks.
func DeliveryMCPBindings(service *reporting.Delivery, execution bool) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, mcpserver.ErrRegistration
	}
	registry, err := DeliveryRegistry(execution)
	if err != nil {
		return nil, err
	}
	mapper := func(err error) mcpserver.Fault {
		_, code := httpFault(err)
		return mcpserver.Fault{Code: code}
	}
	out := []mcpserver.Binding{}
	appendBinding := func(b mcpserver.Binding, err error) error {
		if err == nil {
			out = append(out, b)
		}
		return err
	}
	if err := appendBinding(mcpserver.Bind(registry, "reportingSearch", "reporting_search", "reporting", "Search currently authorized published blocks, reports or dashboards. Returns metadata only; never SQL or result values.", service.Search, mapper)); err != nil {
		return nil, err
	}
	if err := appendBinding(mcpserver.Bind(registry, "reportingDescribe", "reporting_describe", "reporting", "Describe a published revision, selected output identifiers and typed business filters. Does not run queries or models.", service.Describe, mapper)); err != nil {
		return nil, err
	}
	if err := appendBinding(mcpserver.Bind(registry, "reportingRuns", "reporting_runs", "reporting", "List currently authorized retained reporting artifacts, including private or expired state where authorized. Does not run schedules.", service.Runs, mapper)); err != nil {
		return nil, err
	}
	resource, err := mcpserver.NewAppResource(reportviewer.URI, "Chartworks report viewer", "Bounded read viewer for authorized retained reporting outputs", reportviewer.HTML())
	if err != nil {
		return nil, err
	}
	view, err := mcpserver.Bind(registry, "reportingView", "reporting_view", "reporting", "View one retained block, report or dashboard output. Paging, output navigation and redraw never execute queries or models.", service.View, mapper)
	if err != nil {
		return nil, err
	}
	view, err = mcpserver.WithAppResource(view, resource)
	if err != nil {
		return nil, err
	}
	out = append(out, view)
	if execution {
		run, err := mcpserver.Bind(registry, "reportingRun", "reporting_run", "reporting", "Explicitly execute a published reporting revision. May query data, spend model tokens and persist an artifact. New filters require a new key and current signed authority; do not retry unknown outcomes blindly.", service.Run, mapper)
		if err != nil {
			return nil, err
		}
		run, err = mcpserver.WithAppResource(run, resource)
		if err != nil {
			return nil, err
		}
		out = append(out, run)
	}
	return out, nil
}
