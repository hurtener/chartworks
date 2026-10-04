package foundation

import (
	"reflect"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/chartapi"
	"github.com/hurtener/chartworks/internal/chartservice"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/evaluationapi"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/migrationapi"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqbyo"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/onboardingapi"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sourceapi"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/internal/topicapi"
	"github.com/hurtener/chartworks/internal/topicfeedback"
	"github.com/hurtener/chartworks/internal/topicfeedbackapi"
	reportapp "github.com/hurtener/chartworks/web/report-app"
)

// TestReportAppFullFactoryInventory registers actual factory contracts, including
// every optional rendition tool. Zero-valued services are never invoked: this
// is metadata/schema composition evidence, not source, model or renderer work.
// TestWorkAssemblyLifecycle separately covers real production assembly.
func TestReportAppFullFactoryInventory(t *testing.T) {
	defaults := config.Defaults()
	// A nonnil actual router enables the third BYO factory binding. New performs
	// dependency validation only; none of these registration-only services run.
	byo, err := nlqbyo.New(&nlqroute.Service{}, &topics.Service{}, &rulesets.Service{}, &sources.Service{}, &readexec.Validator{}, &readexec.Executor{}, &postgres.DB{}, defaults.QueryBundles, defaults.Exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	authoring, err := reporting.NewAuthoring(&reporting.Documents{}, &reporting.Compositions{})
	if err != nil {
		t.Fatal(err)
	}
	renderer, err := rendering.NewManaged(foundationRenditionViewer{}, rendering.NewMemoryRepository(), rendering.LocalProcessor{MaxBytes: 1 << 20}, 1<<20,
		rendering.Options{WorkerVersion: "synthetic-worker", ThemeVersion: "synthetic-theme", MaxTime: time.Second, MaxMemoryBytes: 64 << 20, MaxInputBytes: 1 << 20, MaxOutputBytes: 1 << 20, MaxConcurrent: 1, MaxWidgets: 10, Retention: time.Hour, Isolation: "development"})
	if err != nil {
		t.Fatal(err)
	}
	app, err := mcpserver.NewAppResource(reportapp.URI, "Chartworks report app", "Manual report composition and authorized retained consumption", reportapp.HTML())
	if err != nil {
		t.Fatal(err)
	}
	factories := []struct {
		name  string
		count int
		build func() ([]mcpserver.Binding, error)
	}{
		{"source", 3, func() ([]mcpserver.Binding, error) { return sourceapi.MCPBindings(&sources.Service{}) }},
		{"topic", 2, func() ([]mcpserver.Binding, error) { return topicapi.MCPBindings(&topics.Service{}) }},
		{"query", 10, func() ([]mcpserver.Binding, error) { return nlqapi.ExecutionMCPBindings(&nlqexec.Service{}) }},
		{"byo", 3, func() ([]mcpserver.Binding, error) { return nlqapi.BYOMCPBindings(byo) }},
		{"charts", 5, func() ([]mcpserver.Binding, error) { return chartapi.MCPBindings(&chartservice.Service{}) }},
		{"feedback", 4, func() ([]mcpserver.Binding, error) { return topicfeedbackapi.MCPBindings(&topicfeedback.Service{}) }},
		{"onboarding", 8, func() ([]mcpserver.Binding, error) { return onboardingapi.MCPBindings(&onboarding.Service{}) }},
		{"evaluation", 14, func() ([]mcpserver.Binding, error) { return evaluationapi.MCPBindings(&evaluation.Service{}, nil) }},
		{"migration", 8, func() ([]mcpserver.Binding, error) { return migrationapi.MCPBindings(&migration.Service{}) }},
		{"delivery-and-renditions", 12, func() ([]mcpserver.Binding, error) {
			return reportingapi.DeliveryMCPBindings(&reporting.Delivery{}, true, renderer)
		}},
		{"authoring", 25, func() ([]mcpserver.Binding, error) { return reportingapi.AuthoringMCPBindings(authoring, app) }},
		{"bootstrap", 2, func() ([]mcpserver.Binding, error) { return reportingapi.ReportAppBootstrapMCPBindings(authoring, app) }},
	}
	bindings := []mcpserver.Binding{}
	defaultBindings := []mcpserver.Binding{}
	for _, factory := range factories {
		group, err := factory.build()
		if err != nil || len(group) != factory.count {
			t.Fatalf("actual %s factory: count=%d want=%d error=%v", factory.name, len(group), factory.count, err)
		}
		bindings = append(bindings, group...)
		if factory.name != "delivery-and-renditions" {
			defaultBindings = append(defaultBindings, group...)
		}
	}
	defaultDelivery, err := reportingapi.DeliveryMCPBindings(&reporting.Delivery{}, true)
	if err != nil || len(defaultDelivery) != 7 {
		t.Fatal("default delivery inventory drift", len(defaultDelivery), err)
	}
	defaultBindings = append(defaultBindings, defaultDelivery...)
	defaultSelected, err := mcpserver.SelectGroups(defaultBindings, defaults.MCP.Groups)
	if err != nil || len(defaultBindings) != 91 || len(defaultSelected) != 91 {
		t.Fatal("default inventory drift", len(defaultBindings), len(defaultSelected), err)
	}
	defaultRegistry, err := mcpserver.NewRegistry(defaultSelected)
	if err != nil || len(defaultRegistry.Manifest()) != 91 || mcpserver.MaxRegisteredTools != 96 {
		t.Fatal("default actual registry rejected or ceiling changed", err)
	}
	selected, err := mcpserver.SelectGroups(bindings, defaults.MCP.Groups)
	if err != nil || len(bindings) != 96 || len(selected) != 96 {
		t.Fatal("complete configured inventory drift", len(bindings), len(selected), err)
	}
	registry, err := mcpserver.NewRegistry(selected)
	if err != nil || len(registry.Manifest()) != 96 {
		t.Fatal("complete actual registry rejected", err)
	}
	names := map[string]bool{}
	for _, tool := range registry.Manifest() {
		names[tool.Name] = true
	}
	for _, name := range []string{"get_query_context", "reporting_search", "reporting_view", "reporting_rendition_create", "reporting_rendition_read", "reporting_authoring_widget_v1", "reporting_authoring_block_read_v1", "reporting_authoring_block_mapping_v1", "reporting_authoring_block_copy_v1", "reporting_authoring_block_validate_v1", "reporting_authoring_dataset_v1", "reporting_authoring_prepare_chart_v1", "reporting_authoring_preparation_v1", "reporting_authoring_create_prepared_v1", "reporting_authoring_preparation_control_v1", "reporting_authoring_dataset_options_v1", "reporting_authoring_report_options_v1", "reporting_authoring_option_status_v1", "reporting_authoring_option_control_v1", "report_app_bootstrap_v1", "reporting_authoring_lifecycle_v1", "reporting_authoring_block_publish_v1", "reporting_authoring_rebind_published_v1", "reporting_authoring_report_transition_v1"} {
		if !names[name] {
			t.Fatal("missing actual optional contract", name)
		}
	}
	settings := defaults.MCP
	if settings.MaxRequestBytes != 10<<20 || settings.MaxResponseBytes != 16<<20 || settings.MaxConcurrent != 16 || time.Duration(settings.Timeout) != 65*time.Second {
		t.Fatal("metadata inventory changed request/execution budgets", settings)
	}
	cfg := defaults.Auth
	cfg.Issuer, cfg.JWKSURL = "https://issuer.example.test", "https://issuer.example.test/jwks"
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp", Jobs: "chartworks:execution"}
	verifier, err := auth.New(cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer verifier.Close()
	if _, err := mcpserver.New(verifier, registry, settings, nil); err != nil {
		t.Fatal("complete actual inventory cannot start with existing MCP limits", err)
	}
	if !reflect.DeepEqual(settings, defaults.MCP) {
		t.Fatal("MCP server mutated caller limits")
	}
}
