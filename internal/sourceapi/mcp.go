package sourceapi

import (
	"context"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/sources"
)

// MCPBindings binds only the installed service, never an unavailable placeholder.
func MCPBindings(service *sources.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := SourceRegistry(service.Enabled(), false)
	if err != nil {
		return nil, err
	}
	var bindings []mcpserver.Binding
	mapper := func(err error) mcpserver.Fault { _, code := classify(err); return mcpserver.Fault{Code: code} }
	b0, err := mcpserver.Bind(registry, "listSources", "list_sources", "discovery", "List retained source registrations permitted by current signed reach. No warehouse connection or inference is performed.", func(ctx context.Context, e identity.Envelope, _ struct{}) ([]sources.Source, error) {
		return service.List(ctx, e, 100)
	}, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b0)
	b1, err := mcpserver.Bind(registry, "listDatasets", "list_datasets", "discovery", "List a bounded page of authorized retained dataset metadata in one exact source execution context. Use source and context IDs, an empty after for the first page, and a limit from 1 to 32. No warehouse call.", service.ListDatasets, mapper)
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b1)
	b2, err := mcpserver.Bind(registry, "describeDataset", "describe_dataset", "discovery", "Describe the registered columns of one authorized dataset in its exact source context, without querying the warehouse. This is retained metadata, not a current source health check.", service.DescribeDataset, mapper)
	if err != nil {
		return nil, err
	}
	b2, err = mcpserver.WithResource(b2, "chartworks://datasets/{source}/{context}/{dataset}")
	if err != nil {
		return nil, err
	}
	bindings = append(bindings, b2)
	return bindings, nil
}
