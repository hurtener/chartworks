package reportingapi

import (
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func authoringPrepareEntries(s *reporting.Authoring) []runtimeEndpoint {
	out := []runtimeEndpoint{
		authoringEntry("dataset", "sources.read", "retained_metadata_read", "Read typed dataset columns and reviewed fields with explicit compiler capabilities", s.Dataset),
		authoringEntry("prepare_chart", "reporting.validate", "bounded_source_read_private_preparation", "Prepare a deterministic PostgreSQL chart using actual schema and immutable operation custody", s.PrepareDatasetChart),
		authoringEntry("preparation", "reporting.write", "retained_metadata_read", "Inspect exact private preparation metadata without source or model work", s.Preparation),
		authoringEntry("create_prepared", "reporting.write", "private_block_preparation_consume", "Consume private preparation into an unvalidated native block draft", s.CreatePreparedChart),
		authoringEntry("preparation_control", "reporting.validate", "existing_source_attempt_control", "Explicitly inspect cancel or reconcile the existing preparation source attempt", s.PreparationControl),
	}
	out[1].definition.Errors = append(out[1].definition.Errors, api.ErrorResponse{Status: 409, Code: "preparation_contract_required"}, api.ErrorResponse{Status: 410, Code: "preparation_operation_expired"})
	out[0].definition.ResourceLoader = "verified exact topic/source/dataset/context read reach; retained metadata only; unsupported policies explicit"
	out[1].definition.ResourceLoader = "charts.bind; exact tenant read/write; new block read/write/preview/validate; real topic write or source read and every dependency; sources.query source/dataset/context; actor/session/operation custody before native planning/read"
	out[2].definition.ResourceLoader = "original actor/session/tenant preparation and exact target/dependency reach; retained metadata only"
	out[3].definition.ResourceLoader = "original actor/session/tenant/target preparation digest; current source query and target/dependency authority; atomic source/topic/rule-absence fences; no validation/publication"
	out[4].definition.ResourceLoader = "original actor/session/tenant preparation; target validation and source query reach; existing attempt inspection/control only; no query restart"
	return out
}
func authoringPrepareMCPBindings(r *api.Registry, s *reporting.Authoring) ([]mcpserver.Binding, error) {
	mapper := func(err error) mcpserver.Fault { _, code := httpFault(err); return mcpserver.Fault{Code: code} }
	out := []mcpserver.Binding{}
	add := func(b mcpserver.Binding, err error) error {
		if err == nil {
			out = append(out, b)
		}
		return err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_dataset_v1", "reporting_authoring_dataset_v1", "reporting", "Read retained typed columns, reviewed fields and unsupported compiler dispositions. No source or model calls.", s.Dataset, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_prepare_chart_v1", "reporting_authoring_prepare_chart_v1", "reporting", "Explicit bounded validated PostgreSQL read from advertised typed fields or legacy reviewed selections. Actual observed schema only. Operation replay never queries again. Preparation is private and is not block validation. Fresh requests require operation_version prepare-v1 and timestamped prepare keys; retain the exact key and body for recovery.", s.PrepareDatasetChart, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_preparation_v1", "reporting_authoring_preparation_v1", "reporting", "Read private preparation status by ID or operation under original actor/session/target authority. Never restart uncertain work.", s.Preparation, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_create_prepared_v1", "reporting_authoring_create_prepared_v1", "reporting", "Consume exact private preparation into an unvalidated native block. Recheck current source/topic/rule pins and authority atomically. Native validation is a separate deliberate read; no publication or certification.", s.CreatePreparedChart, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_preparation_control_v1", "reporting_authoring_preparation_control_v1", "reporting", "Explicitly inspect cancel or reconcile the original source attempt through existing native controls. Current actor/session/source authority required. Never restart data queries or reconstruct lost values.", s.PreparationControl, mapper)); err != nil {
		return nil, err
	}
	return out, nil
}
