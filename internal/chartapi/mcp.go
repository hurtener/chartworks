package chartapi

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
)

// MCPBindings binds only the installed service, never an unavailable placeholder.
func MCPBindings(service *chartservice.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := Registry()
	if err != nil {
		return nil, err
	}
	var bindings []mcpserver.Binding
	mapper := func(err error) mcpserver.Fault {
		_, code := classify(err)
		fault := mcpserver.Fault{Code: code}
		var interrupted *chartservice.Failure
		if errors.As(err, &interrupted) {
			fault.Receipt = &interrupted.Receipt
		}
		return fault
	}
	b0, err := mcpserver.Bind(registry, "chartCatalog", "chart_catalog", "charts", "Read the fourteen-kind portable output catalog. This returns specification metadata, not rendered charts; no warehouse or model work is performed.", func(ctx context.Context, e identity.Envelope, _ struct{}) (chartservice.CatalogResult, error) {
		return service.Catalog(ctx, e)
	}, mapper)
	if err != nil {
		return nil, err
	}
	b0, err = mcpserver.WithResource(b0, "chartworks://charts/catalog")
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b0)
	b1, err := mcpserver.Bind(registry, "selectChart", "select_chart", "charts", "Select a suitable output specification for caller-supplied data. Deterministic by default; explicit rank assistance may incur bounded model cost. Never queries a source or certifies caller data.", service.Select, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b1)
	b2, err := mcpserver.Bind(registry, "specifyChart", "specify_chart", "charts", "Build one explicitly chosen portable output kind for caller-supplied data, without fallback, source access, persistence or inference. This is a specification, not an image.", service.Specify, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b2)
	b3, err := mcpserver.Bind(registry, "buildChart", "build_chart", "charts", "Apply an exact saved output mapping to caller-supplied data without selecting another chart, querying a source or invoking a model. Schema drift fails instead of silently rebinding.", service.Build, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b3)
	b4, err := mcpserver.Bind(registry, "rebindChart", "rebind_chart", "charts", "Propose an unambiguous semantic mapping change for explicit review. Does not modify an approved definition, persist changes, query a warehouse or invoke a model.", service.Rebind, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b4)
	return bindings, nil
}
