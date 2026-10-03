package reportingapi

import (
	"context"
	"net/http"
	"net/url"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

// ReportAppBootstrapRequest selects interaction intent, never identity or grants.
// Targets are exact report locators; each is independently checked server-side.
type ReportAppBootstrapRequest struct {
	Version int      `json:"version" jsonschema:"enum=1"`
	Mode    string   `json:"mode" jsonschema:"enum=chat,enum=plan,enum=apply"`
	Targets []string `json:"targets"`
}

type ReportAppTarget struct {
	Report       string                          `json:"report"`
	Capabilities reporting.AuthoringCapabilities `json:"capabilities"`
}

type ReportAppGuide struct {
	Version                int      `json:"version"`
	DocumentSchemaVersion  int      `json:"document_schema_version"`
	DocumentSchemaVersions []int    `json:"document_schema_versions"`
	Modes                  []string `json:"modes"`
	Steps                  []string `json:"steps"`
	Constraints            []string `json:"constraints"`
	Documentation          []string `json:"documentation"`
}

type ReportAppBootstrap struct {
	Version  int               `json:"version"`
	Mode     string            `json:"mode"`
	Targets  []ReportAppTarget `json:"targets"`
	Guidance ReportAppGuide    `json:"guidance"`
}

// reportAppGuide is compiled capability guidance, not hidden agent configuration.
// Its content contains no identities, data, credentials or authority snapshots.
func reportAppGuide(ctx context.Context, e identity.Envelope, _ struct{}) (ReportAppGuide, error) {
	if err := ctx.Err(); err != nil {
		return ReportAppGuide{}, err
	}
	if !e.Valid() {
		return ReportAppGuide{}, access.ErrUnauthenticated
	}
	if !e.Has("reporting.read") {
		return ReportAppGuide{}, access.ErrForbidden
	}
	return ReportAppGuide{
		Version: 1, DocumentSchemaVersion: reporting.DocumentVersion, DocumentSchemaVersions: []int{reporting.DocumentVersion, reporting.PagedDocumentVersion},
		Modes: []string{"chat", "plan", "apply"},
		Steps: []string{
			"Read current target capabilities and exact document revision before proposing changes.",
			"Discover authorized published blocks and describe their stable output identifiers and typed parameters.",
			"Compose bounded text and approved-output widgets using the canonical document schema. Preserve existing schema and page identity unless explicitly upgrading a revision.",
			"Read exact SQL-free block metadata before offering real chart types or fields. Never invent bindings, units, aggregations or unsupported chart kinds.",
			"For report-local changes create an explicitly authorized private block copy. Amend only its selected output under version, revision and digest CAS.",
			"In chat mode explain options; in plan mode return a proposed change without mutation; in apply mode use an explicitly authorized target and expected-version save.",
			"After a successful save, reopen the exact retained revision. On conflict reload and reconcile; never overwrite blindly.",
			"Preview is explicit private execution. Read and redraw retained outputs without rerunning queries.",
		},
		Constraints: []string{
			"Guidance, interaction mode, profile, audience, creator and target locators never grant authority.",
			"Pengui owns identity, Teams, resource grants, entitlement and credential issuance. Every operation rechecks current exact action/resource/context reach.",
			"Selected-widget changes are server-enforced. Do not modify another widget, report metadata or dependencies outside the admitted edit scope.",
			"Published definitions are immutable; amendments create private revisions under CAS. Private previews remain private after publication.",
			"Do not invoke natural-language query generation, narratives, models or arbitrary code as part of manual composition.",
			"Copying requires independent source read/preview, new-target tenant/block/topic/dependency authority and charts.bind. Report-write never substitutes for them.",
			"Schema binding is not data validation. Never execute or publish automatically. Publication can expose an existing central audience and is not private approval.",
			"Page IDs are stable, widget IDs report-global, and filters/defaults page-local. Never silently drop unsupported pages.",
			"Credentials and host-only nonces stay in the trusted host and never enter iframe messages, URLs or persisted browser state.",
		},
		Documentation: []string{"docs/contracts/report-app-v1.md", "docs/contracts/pengui-authority.md", "docs/reporting/contracts.md", "docs/reporting/delivery.md"},
	}, nil
}

func bootstrapReportApp(service *reporting.Authoring) func(context.Context, identity.Envelope, ReportAppBootstrapRequest) (ReportAppBootstrap, error) {
	return func(ctx context.Context, e identity.Envelope, in ReportAppBootstrapRequest) (ReportAppBootstrap, error) {
		if in.Version != 1 || (in.Mode != "chat" && in.Mode != "plan" && in.Mode != "apply") || len(in.Targets) > 16 {
			return ReportAppBootstrap{}, reporting.ErrInvalid
		}
		guide, err := reportAppGuide(ctx, e, struct{}{})
		if err != nil {
			return ReportAppBootstrap{}, err
		}
		out := ReportAppBootstrap{Version: 1, Mode: in.Mode, Targets: []ReportAppTarget{}, Guidance: guide}
		seen := map[string]bool{}
		for _, id := range in.Targets {
			if !identity.Identifier(id) || seen[id] {
				return ReportAppBootstrap{}, reporting.ErrInvalid
			}
			seen[id] = true
			// Exact reach is required here even if another older service accepts an
			// explicitly signed tenant-wide selection. App intent cannot expand it.
			exact := false
			for _, reach := range e.Reach() {
				if reach.Kind == "report" && reach.ID == id && (reach.Permission == "read" || reach.Permission == "write") {
					exact = true
				}
			}
			if !exact {
				return ReportAppBootstrap{}, access.ErrNotFound
			}
			caps, err := service.Capabilities(ctx, e, reporting.AuthoringCapabilitiesRequest{Report: id})
			if err != nil {
				return ReportAppBootstrap{}, err
			}
			out.Targets = append(out.Targets, ReportAppTarget{Report: id, Capabilities: caps})
		}
		return out, nil
	}
}

func reportAppBootstrapEntries(service *reporting.Authoring) []runtimeEndpoint {
	bootstrap := runtimeEntry("POST", "/v1/reporting/authoring/v1/bootstrap", "reporting.read", "reportAppBootstrapV1", "Read bounded target capabilities and versioned report application guidance", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in ReportAppBootstrapRequest) (ReportAppBootstrap, error) {
		return bootstrapReportApp(service)(ctx, e, in)
	})
	guide := runtimeEntry("POST", "/v1/reporting/authoring/v1/guide", "reporting.read", "reportAppGuideV1", "Read static versioned report application guidance without document data", func(ctx context.Context, e identity.Envelope, _ string, _ url.Values, in struct{}) (ReportAppGuide, error) {
		return reportAppGuide(ctx, e, in)
	})
	for _, entry := range []*runtimeEndpoint{&bootstrap, &guide} {
		entry.definition.Effect = "retained_metadata_read"
		entry.definition.Audit = "read_only_no_domain_audit"
		entry.definition.Replay = "never"
	}
	return []runtimeEndpoint{bootstrap, guide}
}

func ReportAppBootstrapRegistry() (*api.Registry, error) {
	return registryForEntries(reportAppBootstrapEntries(nil))
}
func ReportAppBootstrapHandler(v *auth.Verifier, s *reporting.Authoring, next http.Handler) http.Handler {
	return serveRuntimeEntries(v, reportAppBootstrapEntries(s), next)
}
func ReportAppBootstrapMCPBindings(s *reporting.Authoring, app mcpserver.AppResource) ([]mcpserver.Binding, error) {
	if s == nil {
		return nil, mcpserver.ErrRegistration
	}
	registry, err := ReportAppBootstrapRegistry()
	if err != nil {
		return nil, err
	}
	mapper := func(err error) mcpserver.Fault { _, code := httpFault(err); return mcpserver.Fault{Code: code} }
	boot, err := mcpserver.Bind(registry, "reportAppBootstrapV1", "report_app_bootstrap_v1", "reporting", "Open the shared report app with typed chat, plan or apply intent and exact current target capabilities. This read operation never mutates or grants authority.", bootstrapReportApp(s), mapper)
	if err != nil {
		return nil, err
	}
	boot, err = mcpserver.WithAppResource(boot, app)
	if err != nil {
		return nil, err
	}
	guide, err := mcpserver.Bind(registry, "reportAppGuideV1", "report_app_guide_v1", "reporting", "Read versioned report composition guidance and contract references. No hidden agent, model invocation, private document or credential is involved.", reportAppGuide, mapper)
	if err != nil {
		return nil, err
	}
	guide, err = mcpserver.WithResource(guide, "chartworks://report_app/guide/v1")
	if err != nil {
		return nil, err
	}
	return []mcpserver.Binding{boot, guide}, nil
}
