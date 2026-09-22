package foundation

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/migration"
)

func TestMigrationPayloadDecoderRejectsUnknownFields(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
	}
	var out payload
	if err := decodeMigrationPayload(`{"name":"known"}`, &out); err != nil || out.Name != "known" {
		t.Fatal("closed payload rejected", err, out)
	}
	for _, raw := range []string{`{"name":"known","unknown":true}`, `{"name":"known"} {}`, ``} {
		if err := decodeMigrationPayload(raw, &out); !errors.Is(err, migration.ErrInvalid) {
			t.Fatal("payload decoder accepted unknown or trailing data", raw, err)
		}
	}
}
