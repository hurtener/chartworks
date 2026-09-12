package acceptance

import "testing"

// TestPhase29 binds each owned acceptance criterion to real PostgreSQL/native
// scenarios. Shared lifecycle/privacy suites intentionally exercise both
// authoring and retained-artifact boundaries rather than testing only DTOs.
func TestPhase29(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		t.Run("http-sdk-lifecycle", TestDocumentHTTPContracts)
		t.Run("cas-and-reference-integrity", TestDocumentStorage)
		t.Run("phase21-inventory-extension", TestDocumentCombinedRegistry)
		t.Run("bounded-resource-filtered-list", TestDocumentListBoundaries)
		t.Run("atomic-lifecycle-audit", TestDocumentAuditRollback)
		t.Run("atomic-import-graph", TestDocumentGraphRollback)
	})
	t.Run("AC02", func(t *testing.T) {
		t.Run("persistent-artifact-privacy", TestReportingCompositionStorage)
		t.Run("private-frozen-child", testPhase29BlockPreview)
	})
	t.Run("AC03", func(t *testing.T) {
		t.Run("sealed-reference-and-output-union", testPhase29SealedFanout)
		t.Run("first-seal-and-page-order", TestReportingCompositionStorage)
		t.Run("atomic-first-seal", TestReportingCompositionAdmissionRollback)
		t.Run("concurrent-block-report-dashboard-publication", TestReportingCompositionConcurrentPublication)
	})
	t.Run("AC04", func(t *testing.T) {
		t.Run("mixed-dynamic-durability", testPhase29DynamicDurability)
		t.Run("saved-query-actual-receipts", TestSavedQuestionReplayable)
		t.Run("routing-clarification-and-replay", TestSavedQuestionClarificationAndSelectionIdentity)
		t.Run("disabled-live-lane", testPhase29PartialPolicies)
		t.Run("independent-query-authority", testPhase29QueryActions)
		t.Run("missing-originating-session", TestSavedQuestionSessionIsolation)
	})
	t.Run("AC05", func(t *testing.T) {
		t.Run("strict-and-partial-omissions", testPhase29PartialPolicies)
		t.Run("query-row-retention-budgets", testPhase29Budgets)
		t.Run("mixed-observation-times", testPhase29SealedFanout)
		t.Run("checkpoint-recovery", TestReportingCompositionRecovery)
		t.Run("checkpoint-transaction-rollback", TestReportingCompositionCheckpointRollback)
		t.Run("cancellation-authority-and-rollback", TestReportingCompositionControlBoundaries)
		t.Run("dynamic-plan-recovery", TestReportingCompositionDynamicRecovery)
		t.Run("output-error-with-retained-table", testPhase29OutputFailure)
		t.Run("per-widget-output-failure", TestReportingCompositionOutputSubsets)
		t.Run("shared-artifact-request-quota", func(t *testing.T) { testPhase29SharedQuota(t, false) })
		t.Run("shared-artifact-byte-quota", func(t *testing.T) { testPhase29SharedQuota(t, true) })
	})
	t.Run("AC06", testPhase29FilterBindings)
	t.Run("AC07", TestDocumentStorage)
	t.Run("AC08", func(t *testing.T) {
		t.Run("document-redaction-and-metadata", TestDocumentStorage)
		t.Run("artifact-redaction-and-metadata", TestReportingCompositionStorage)
		t.Run("data-bearing-metadata-only-read", testPhase29SealedFanout)
		t.Run("tenant-and-context-isolated-retained-reads", testPhase29RetainedBoundaries)
		t.Run("expiry-erasure-and-atomic-audit", TestReportingCompositionRetention)
	})
}
