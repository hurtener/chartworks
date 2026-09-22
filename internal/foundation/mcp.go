package foundation

import (
	"net/http"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/evaluationapi"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/topicapi"
)

// mountMCP composes real services before the common HTTP registry guard. It does
// not create an additional server, issuer, query engine or analytical session.
type deliveryServices struct {
	delivery         *reporting.Delivery
	renderer         *rendering.Service
	evaluation       *evaluation.Service
	evaluationRunner evaluation.Runner
	onboarding       *onboarding.Service
}

func mountMCP(v config.Values, verifier *auth.Verifier, source *sources.Service, published *topics.Service, query *nlqexec.Service, byo *nlqbyo.Service, charts *chartservice.Service, registry *api.Registry, next http.Handler, services ...deliveryServices) (*api.Registry, http.Handler, error) {
	if !v.Features.MCP {
		return registry, next, nil
	}
	var bindings []mcpserver.Binding
	for _, build := range []func() ([]mcpserver.Binding, error){
		func() ([]mcpserver.Binding, error) { return sourceapi.MCPBindings(source) },
		func() ([]mcpserver.Binding, error) { return topicapi.MCPBindings(published) },
		func() ([]mcpserver.Binding, error) { return nlqapi.ExecutionMCPBindings(query) },
		func() ([]mcpserver.Binding, error) { return nlqapi.BYOMCPBindings(byo) },
		func() ([]mcpserver.Binding, error) { return chartapi.MCPBindings(charts) },
		func() ([]mcpserver.Binding, error) {
			if len(services) == 1 {
				return onboardingapi.MCPBindings(services[0].onboarding)
			}
			return nil, nil
		},
	} {
		group, err := build()
		if err != nil {
			return nil, nil, err
		}
		bindings = append(bindings, group...)
	}
	var delivery *reporting.Delivery
	var renderer *rendering.Service
	if len(services) > 1 {
		return nil, nil, mcpserver.ErrRegistration
	}
	if len(services) == 1 {
		delivery, renderer = services[0].delivery, services[0].renderer
		group, bindErr := evaluationapi.MCPBindings(services[0].evaluation, services[0].evaluationRunner)
		if bindErr != nil {
			return nil, nil, bindErr
		}
		bindings = append(bindings, group...)
	}
	if delivery != nil {
		group, err := reportingapi.DeliveryMCPBindings(delivery, delivery.CanExecute(), renderer)
		if err != nil {
			return nil, nil, err
		}
		bindings = append(bindings, group...)
	}
	selected, err := mcpserver.SelectGroups(bindings, v.MCP.Groups)
	if err != nil {
		return nil, nil, err
	}
	tools, err := mcpserver.NewRegistry(selected)
	if err != nil {
		return nil, nil, err
	}
	server, err := mcpserver.New(verifier, tools, v.MCP, v.Server.CORSAllowlist)
	if err != nil {
		return nil, nil, err
	}
	transport, err := mcpserver.HTTPRegistry(v.MCP)
	if err != nil {
		return nil, nil, err
	}
	combined, err := api.Compose(registry, transport)
	if err != nil {
		return nil, nil, err
	}
	handler := server.Handler()
	return combined, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == mcpserver.Path {
			handler.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}), nil
}
