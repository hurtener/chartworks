package acceptance

import "testing"

// TestPhase29 binds each owned acceptance criterion to real PostgreSQL/native
// scenarios. Shared lifecycle/privacy suites intentionally exercise both
// authoring and retained-artifact boundaries rather than testing only DTOs.
func TestPhase29(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		t.Run("http-sdk-lifecycle", TestDocumentHTTPContracts)
		t.Run("cas-and-reference-integrity", TestDocumentStorage)
	})
	t.Run("AC02", func(t *testing.T) {
		t.Run("persistent-artifact-privacy", TestReportingCompositionStorage)
		t.Run("private-frozen-child", testPhase29BlockPreview)
	})
	t.Run("AC03", func(t *testing.T) {
		t.Run("sealed-reference-and-output-union", testPhase29SealedFanout)
		t.Run("first-seal-and-page-order", TestReportingCompositionStorage)
	})
	t.Run("AC04", func(t *testing.T) {
		t.Run("mixed-dynamic-durability", testPhase29DynamicDurability)
		t.Run("saved-query-actual-receipts", TestSavedQuestionReplayable)
		t.Run("routing-clarification-and-replay", TestSavedQuestionClarificationAndSelectionIdentity)
		t.Run("disabled-live-lane", testPhase29PartialPolicies)
	})
	t.Run("AC05", func(t *testing.T) {
		t.Run("strict-and-partial-omissions", testPhase29PartialPolicies)
		t.Run("query-row-retention-budgets", testPhase29Budgets)
		t.Run("mixed-observation-times", testPhase29SealedFanout)
		t.Run("checkpoint-recovery", TestReportingCompositionRecovery)
	})
	t.Run("AC06", testPhase29FilterBindings)
	t.Run("AC07", TestDocumentStorage)
	t.Run("AC08", func(t *testing.T) {
		t.Run("document-redaction-and-metadata", TestDocumentStorage)
		t.Run("artifact-redaction-and-metadata", TestReportingCompositionStorage)
		t.Run("data-bearing-metadata-only-read", testPhase29SealedFanout)
	})
}
