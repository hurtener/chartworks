package reportingapi

import (
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

// Option transport shares the domain's exact target, operation and result DTOs.
// Search is deliberate source work; status and control never reconstruct values.
func authoringOptionEntries(s *reporting.Authoring) []runtimeEndpoint {
	out := []runtimeEndpoint{
		authoringEntry("dataset_options", "reporting.validate", "bounded_source_read_option_values", "Explicitly search governed reviewed dataset options under an allocated new-block target", s.DatasetOptions),
		authoringEntry("report_options", "reporting.execute", "bounded_source_read_option_values", "Explicitly search exact saved-page filter options under private-preview or published policy", s.ReportOptions),
		authoringEntry("option_status", "reporting.read", "retained_metadata_read", "Inspect original option operation metadata without source work or retained values", s.OptionStatus),
		authoringEntry("option_control", "sources.query", "existing_source_attempt_control", "Explicitly cancel or reconcile the original option source attempt without restarting it", s.OptionControl),
	}
	out[0].definition.ResourceLoader = "charts.bind; exact tenant read/write, topic write and new block read/write/preview/validate; reviewed topic and every source/dataset/context dependency; original actor/session/operation custody before native source work"
	out[1].definition.ResourceLoader = "exact report revision/digest/page/filter and read/execute; private report actor/preview eligibility; each block uses its own published read/execute or private actor/read/preview/execute policy; all topic/source/dataset/context dependencies"
	out[2].definition.ResourceLoader = "original actor/session/tenant/operation and exact current target/dependency reach; retained metadata only; no option values or source work"
	out[3].definition.ResourceLoader = "original actor/session/tenant/operation and exact current target/dependency/source query reach; original source attempt only; unresolved same-actor target liability cannot be bypassed by another session"
	for _, entry := range []*runtimeEndpoint{&out[0], &out[1]} {
		entry.definition.Audit = "authoring.option_reserved and authoring.option_finished plus native read events; coordinates/digests only; no search, cursor, SQL, values or credentials"
	}
	out[3].definition.Audit = "authoring.option_control plus native attempt control events; coordinates/outcome only; no search, cursor, SQL, values or credentials"
	return out
}

func authoringOptionMCPBindings(r *api.Registry, s *reporting.Authoring) ([]mcpserver.Binding, error) {
	mapper := func(err error) mcpserver.Fault { _, code := httpFault(err); return mcpserver.Fault{Code: code} }
	out := []mcpserver.Binding{}
	add := func(b mcpserver.Binding, err error) error {
		if err == nil {
			out = append(out, b)
		}
		return err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_dataset_options_v1", "reporting_authoring_dataset_options_v1", "reporting", "Explicit Search only: bounded governed option read for an allocated new-block target and exact reviewed topic/dataset/dimension. Use option:<UnixSeconds>:<32 lowercase hex>. Never search on typing, selection or replay. Lost values require status and a new explicit search only when new_operation_allowed is true.", s.DatasetOptions, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_report_options_v1", "reporting_authoring_report_options_v1", "reporting", "Explicit Search only: derive option source from an exact saved report revision/digest/page/filter. Policy is private_preview or published; current native target/dependency/source authority is required. Use option:<UnixSeconds>:<32 lowercase hex>. Never automatically repeat lost or uncertain reads.", s.ReportOptions, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_option_status_v1", "reporting_authoring_option_status_v1", "reporting", "Read original option-operation metadata under current original actor/session/target authority. No source query, planning, control or retained choices. values_available=false is not an empty result. Only new_operation_allowed=true permits a separately explicit new search; unresolved liability survives session changes.", s.OptionStatus, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_option_control_v1", "reporting_authoring_option_control_v1", "reporting", "Explicitly cancel or reconcile the original option source attempt. This mutates custody and may contact source control; it never restarts a query or reconstructs values. Preserve operation/target and inspect status after unknown outcomes. A new session cannot bypass same-actor unresolved target liability.", s.OptionControl, mapper)); err != nil {
		return nil, err
	}
	return out, nil
}
