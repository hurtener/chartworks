package foundation

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/reporting"
)

func TestBlockImportRejectsCallerOriginProvenance(t *testing.T) {
	// Native migration imports CreateRequest, never an exported SQLView or
	// Revision. Unknown origin metadata is rejected rather than stripped.
	for _, payload := range []string{
		`{"id":"copy","definition":{},"provenance":{"rule_absence":[]}}`,
		`{"id":"copy","definition":{"rule_absence":[]}}`,
	} {
		var request reporting.CreateRequest
		if err := decodeMigrationPayload(payload, &request); !errors.Is(err, migration.ErrInvalid) {
			t.Fatal("import silently discarded origin policy", err)
		}
	}
}
