package reportingapi

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

const DependencyDiscoveryPath = "/v1/reporting/authoring/v1/dependencies"

// Discovery is a BFF control-plane call, deliberately absent from the iframe
// tool inventory and MCP app tools. Both delivery modes share this HTTP seam.
func dependencyEntries(s *reporting.Authoring) []runtimeEndpoint {
	entry := runtimeEntry("POST", DependencyDiscoveryPath, reporting.DependencyDiscoveryAction, "reportAppDependenciesV1", "Discover native dependency identifiers for one exactly authorized target", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in reporting.DependencyRequest) (reporting.DependencyManifest, error) {
		return s.Dependencies(ctx, e, in)
	})
	entry.definition.Effect = "retained_metadata_read"
	entry.definition.MaxBodyBytes = 8 << 10
	entry.definition.Audit = "read_only_no_domain_audit"
	entry.definition.ResourceLoader = "exact signed target read and discovery action; private preview/custody before native dependency index projection; no values or definitions"
	return []runtimeEndpoint{entry}
}

func DependencyRegistry() (*api.Registry, error) { return registryForEntries(dependencyEntries(nil)) }

func DependencyHandler(v *auth.Verifier, s *reporting.Authoring, next http.Handler) http.Handler {
	return serveRuntimeEntries(v, dependencyEntries(s), next)
}
