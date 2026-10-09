package chartworks

import (
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/reporting"
)

func TestPreparationOperationHelper(t *testing.T) {
	now := time.Unix(1800000000, 0)
	first, err := NewPreparationOperation(now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPreparationOperation(now)
	if err != nil || second == first {
		t.Fatal("nonce reused", err)
	}
	if err := reporting.AuthoringPreparationAdmission(ReportAppPrepareRequest{Operation: first, OperationVersion: PreparationOperationVersion}, now); err != nil {
		t.Fatal(err)
	}
}
