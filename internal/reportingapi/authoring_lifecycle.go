package reportingapi

import (
	"github.com/hurtener/chartworks/internal/api"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/reporting"
)

// Lifecycle adapters share the domain DTOs and native checks. The transition
// entry ceiling permits editor access only; it never substitutes write for the
// native publish action required by publication and rejection.
func authoringLifecycleEntries(s *reporting.Authoring) []runtimeEndpoint {
	out := []runtimeEndpoint{
		authoringEntry("lifecycle", "reporting.read", "retained_metadata_read", "Inspect exact retained report/block lifecycle metadata and every output of each entire block revision", s.InspectLifecycle),
		authoringEntry("block_publish", "reporting.publish", "block_publication_cas_commit", "Explicitly publish one entire validated block revision under current native evidence and CAS checks", s.PublishBlock),
		authoringEntry("rebind_published", "reporting.write", "report_document_draft_cas_commit", "Separately rebind exact selected private widget pins to their already-published revisions under report CAS", s.RebindPublished),
		authoringEntry("report_transition", "reporting.read", "report_document_transition_cas_commit", "Explicitly review, publish or reject one exact report revision through its independent native transition", s.TransitionReport),
	}
	out[0].definition.ResourceLoader = "exact report write or publish and independent read/preview, or exact block read; every current dependency and original private block actor/preview fence; exact revision or independent draft/review pointer; no wildcard reach"
	out[1].definition.ResourceLoader = "exact block read and native publish plus parent/dependency publish reach; original actor/private preview, exact revision/digest/head CAS and current content-bound validation evidence/dependency/health fences; no wildcard reach"
	out[2].definition.ResourceLoader = "exact private report write and independent read/preview; revision/digest/head CAS and selected widget/block pins; current dependencies and original private block actor/preview fence; already-published exact block revisions; no wildcard reach"
	out[3].definition.ResourceLoader = "reporting.read entry; native exact report write for review or reporting.publish and exact report publish for publish/reject; exact revision/head CAS and current dependency/private pin fences; rejection note required; no wildcard reach"
	out[1].definition.Audit = "native block publication transaction and revision/evidence/CAS coordinates; no SQL, rows, credentials or audience grant"
	out[2].definition.Audit = "native report immutable amendment transaction and revision/CAS coordinates; no source/model work or audience grant"
	out[3].definition.Audit = "native independent report review/publication/rejection transaction and revision/CAS coordinates; no source/model work or audience grant"
	return out
}

func authoringLifecycleMCPBindings(r *api.Registry, s *reporting.Authoring) ([]mcpserver.Binding, error) {
	mapper := func(err error) mcpserver.Fault { _, code := httpFault(err); return mcpserver.Fault{Code: code} }
	out := []mcpserver.Binding{}
	add := func(b mcpserver.Binding, err error) error {
		if err == nil {
			out = append(out, b)
		}
		return err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_lifecycle_v1", "reporting_authoring_lifecycle_v1", "reporting", "Inspect exact retained lifecycle metadata, including all outputs of each entire block revision. can_publish is a current hint, never a grant. existing_authorized_readers means eligibility, not an audience count. Recover unknown mutations by inspecting their original exact revision; never automatically repeat them. No source or model work.", s.InspectLifecycle, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_block_publish_v1", "reporting_authoring_block_publish_v1", "reporting", "Obtain explicit user confirmation after disclosing every output and entire_revision publication to existing_authorized_readers. Publish the exact revision/digest/evidence once through native authority and CAS checks. This creates no grant, rebind, report publication, validation, source or model work. Inspect the original exact revision after an unknown result; never automatically retry.", s.PublishBlock, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_rebind_published_v1", "reporting_authoring_rebind_published_v1", "reporting", "Obtain separate user confirmation to change only the selected exact private widget pins to already-published block revisions under report CAS. Preserve outputs, filters/defaults, layout, pages and all other content. Block publication cannot be rolled back by failed rebind. Reopen and inspect on conflict or unknown result; never automatically repeat. Retained previews stay private.", s.RebindPublished, mapper)); err != nil {
		return nil, err
	}
	if err := add(mcpserver.Bind(r, "reporting_authoring_report_transition_v1", "reporting_authoring_report_transition_v1", "reporting", "Keep report review, report publication and rejection explicit and separate. Obtain explicit user confirmation before publication. Closed operation is review, publish or reject; review is not publication, and reject needs a note plus native publish authority. Dispatch requires reporting.read; review requires exact write, while publication and rejection require exact publish without write. Inspect the original exact revision after an unknown result; never automatically repeat or infer rollback. No source/model work or grant.", s.TransitionReport, mapper)); err != nil {
		return nil, err
	}
	return out, nil
}
