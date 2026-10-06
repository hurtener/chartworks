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
const WriteDependencyDiscoveryPath = "/v1/reporting/authoring/v1/write-dependencies"
const EffectDependencyDiscoveryPath = "/v1/reporting/authoring/v1/effect-dependencies"
const DataDependencyDiscoveryPath = "/v1/reporting/authoring/v1/data-dependencies"

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
	write := runtimeEntry("POST", WriteDependencyDiscoveryPath, reporting.DependencyDiscoveryAction, "reportAppWriteDependenciesV1", "Discover native requirements for a proposed manual report write", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in reporting.WriteDependencyRequest) (reporting.WriteDependencyManifest, error) {
		return s.WriteDependencies(ctx, e, in)
	})
	write.definition.Effect = "retained_metadata_read"
	write.definition.MaxBodyBytes = 1 << 20
	write.definition.Audit = "read_only_no_domain_audit"
	write.definition.ResourceLoader = "exact signed report write and discovery action; create requires tenant write; native proposal pins and save baseline requirements; private block custody; no values or definitions returned"
	data := runtimeEntry("POST", DataDependencyDiscoveryPath, reporting.DependencyDiscoveryAction, "reportAppDataDependenciesV1", "Discover native publication or original preparation dependency coordinates", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in reporting.DataDependencyRequest) (reporting.DataDependencyManifest, error) {
		return s.DataDependencies(ctx, e, in)
	})
	data.definition.Effect = "retained_metadata_read"
	data.definition.MaxBodyBytes = 8 << 10
	data.definition.Audit = "read_only_no_domain_audit"
	data.definition.ResourceLoader = "exact topic read or block read/write/preview; original preparation tenant/actor/session/target custody; no definitions, values or source work"
	effect := runtimeEntry("POST", EffectDependencyDiscoveryPath, reporting.DependencyDiscoveryAction, "reportAppEffectDependenciesV1", "Discover saved validation, preview and retained-run requirements", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in reporting.EffectDependencyRequest) (reporting.EffectDependencyManifest, error) {
		return s.EffectDependencies(ctx, e, in)
	})
	effect.definition.Effect = "retained_metadata_read"
	effect.definition.MaxBodyBytes = 8 << 10
	effect.definition.Audit = "read_only_no_domain_audit"
	effect.definition.ResourceLoader = "exact block/report read and preview or exact run discovery; private run actor/session custody before native metadata; no definitions, SQL, results or source work"
	return []runtimeEndpoint{entry, write, data, effect}
}

func DependencyRegistry() (*api.Registry, error) { return registryForEntries(dependencyEntries(nil)) }

func DependencyHandler(v *auth.Verifier, s *reporting.Authoring, next http.Handler) http.Handler {
	return serveRuntimeEntries(v, dependencyEntries(s), next)
}
