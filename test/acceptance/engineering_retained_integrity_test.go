package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Simulate damaged retained JSON using the test database owner's repair access.
// Ordinary writes cannot change these manifests. Reads must still fail closed
// with zero private output instead of trusting a syntactically valid JSON object.
func TestEngineeringRetainedJSONFailsClosed(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("retained-integrity", "csv", raw, columns)
	loaded := f.load(t, spec, raw)
	profile := f.profileSpec(t, *loaded.Upload.Source, "retained-integrity-profile", []string{"id", "amount"}, "")
	f.profile(t, profile)
	db := support.Raw(t, f.dsn)
	before := count(t, db, `SELECT count(*) FROM chartworks.audit_events`)
	for _, tc := range []struct {
		name, table, trigger, column, value string
		profile                             bool
	}{
		{"profile-malformed-field", "profile_versions", "profile_version_immutable", "manifest", `{"Tenant":42}`, true},
		{"profile-wrong-identity", "profile_versions", "profile_version_immutable", "manifest", `{"Tenant":"another-tenant"}`, true},
		{"profile-invalid-result", "profile_versions", "profile_version_immutable", "result", `{}`, true},
		{"upload-invalid-spec", "uploads", "upload_spec_immutable", "spec", `{"id":42}`, false},
		{"upload-invalid-receipt", "uploads", "upload_spec_immutable", "receipt", `{}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// All SQL identifiers are fixed test literals; values remain parameters.
			var saved []byte
			if err := db.QueryRow(ctx, "SELECT "+tc.column+" FROM chartworks."+tc.table).Scan(&saved); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(ctx, "ALTER TABLE chartworks."+tc.table+" DISABLE TRIGGER "+tc.trigger); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := db.Exec(ctx, "UPDATE chartworks."+tc.table+" SET "+tc.column+"=$1::jsonb", string(saved)); err != nil {
					t.Error(err)
				}
				if _, err := db.Exec(ctx, "ALTER TABLE chartworks."+tc.table+" ENABLE TRIGGER "+tc.trigger); err != nil {
					t.Error(err)
				}
			}()
			if _, err := db.Exec(ctx, "UPDATE chartworks."+tc.table+" SET "+tc.column+"=$1::jsonb", tc.value); err != nil {
				t.Fatal(err)
			}
			if tc.profile {
				out, err := f.db.ReadProfile(ctx, f.e, profile.ID, false)
				if !errors.Is(rejectedEngineeringValue(t, out, err), store.ErrInvalid) {
					t.Fatal("corrupt profile exposed retained data", err)
				}
			} else {
				out, err := f.db.ReadUpload(ctx, f.e, spec.ID, "sources.upload", "write")
				if !errors.Is(rejectedEngineeringValue(t, out, err), store.ErrInvalid) {
					t.Fatal("corrupt upload exposed retained data", err)
				}
			}
		})
	}
	if count(t, db, `SELECT count(*) FROM chartworks.audit_events`) != before {
		t.Fatal("rejected reads changed audit state")
	}
	if _, err := f.db.ReadProfile(ctx, f.e, profile.ID, false); err != nil {
		t.Fatal("profile repair failed", err)
	}
	f.readUpload(t, *loaded.Upload.Source)
}
