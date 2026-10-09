package reportingapi

import (
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

func authoringBlockEntries(service *reporting.Authoring) []runtimeEndpoint {
	out := []runtimeEndpoint{
		authoringEntry("block_read", "reporting.read", "retained_metadata_read", "Read an exact SQL-free block revision and server-owned chart binding candidates", service.ReadBlock),
		authoringEntry("block_mapping", "reporting.write", "block_draft_cas_commit", "Amend one exact private output with exactly one mapping or bounded presentation patch", amendAuthoringBlock(service)),
		authoringEntry("block_copy", "reporting.write", "private_block_copy_commit", "Copy an exact source to an authorized private block with exactly one mapping or presentation patch", copyAuthoringBlock(service)),
		authoringEntry("block_validate", "reporting.validate", "explicit_bounded_source_read_private_evidence", "Explicitly validate one exact private block draft through the native bounded source read", service.ValidateBlock),
	}
	out[0].definition.ResourceLoader = "verified Pengui exact block read plus parent/dependency reach and private actor/preview eligibility; SQL-free native projection"
	out[1].definition.ResourceLoader = "verified Pengui charts.bind and exact tenant read; exact private block read/write/preview, parent/dependencies; immutable revision/digest and head CAS"
	out[2].definition.ResourceLoader = "verified Pengui charts.bind and exact tenant read/write; source block read/preview and private eligibility; exact distinct new block and parent/dependency write; no copied validation or publication"
	out[3].definition.ResourceLoader = "verified Pengui exact private block read/preview and validate/write; original actor, exact revision/digest/head CAS and every native topic/dependency/source query/context permission; explicit physical source work"
	return out
}

func authoringBlockMCPBindings(registry *api.Registry, service *reporting.Authoring) ([]mcpserver.Binding, error) {
	mapper := func(err error) mcpserver.Fault { _, code := httpFault(err); return mcpserver.Fault{Code: code} }
	out := []mcpserver.Binding{}
	add := func(binding mcpserver.Binding, err error) error {
		if err == nil {
			out = append(out, binding)
		}
		return err
	}
	if err := add(mcpserver.Bind(registry, "reporting_authoring_block_read_v1", "reporting_authoring_block_read_v1", "reporting", "Read one exact SQL-free block revision with current CAS/digest/schema/mappings and server-owned binding candidates. Private revisions retain native read/preview and actor eligibility. No source or model calls.", service.ReadBlock, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(registry, "reporting_authoring_block_mapping_v1", "reporting_authoring_block_mapping_v1", "reporting", "Change one exact private output under version/revision/digest CAS. Supply exactly one non-null mapping or versioned presentation patch; presentation changes only supported field labels or decimal display digits. Requires charts.bind, tenant read and native block/dependency write/read authority. Schema binding is not data validation; publication still requires fresh native validation.", amendAuthoringBlock(service), mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(registry, "reporting_authoring_block_copy_v1", "reporting_authoring_block_copy_v1", "reporting", "Create a distinct explicitly authorized private block copy with exactly one non-null mapping or versioned presentation patch for one output. Requires exact source read and preview plus target/tenant/topic/dependency write and charts.bind. Keeps server-owned SQL and other outputs unchanged; never copies approval, executes or publishes.", copyAuthoringBlock(service), mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(registry, "reporting_authoring_block_validate_v1", "reporting_authoring_block_validate_v1", "reporting", "Explicitly validate the exact current private block draft using the native bounded source executor and fresh exact authority. Returns only content-free evidence/schema coordinates and CAS state. Never publishes or generates SQL; an unknown outcome must not be blindly retried.", service.ValidateBlock, mapper)); err != nil {
		return nil, err
	}
	return out, nil
}
