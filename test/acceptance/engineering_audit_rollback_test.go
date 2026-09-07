package acceptance

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Inject a real PostgreSQL audit failure, not a successful fake transaction.
// Its parameter is data in a disposable fixture table, never interpolated SQL.
func rejectEngineeringAudit(t *testing.T, f *engineeringFixture, action string) func() {
	t.Helper()
	metadata := support.Raw(t, f.dsn)
	ctx := context.Background()
	if _, err := metadata.Exec(ctx, `
CREATE TABLE chartworks.test_rejected_audit(action text PRIMARY KEY);
CREATE FUNCTION chartworks.test_reject_audit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS(SELECT 1 FROM chartworks.test_rejected_audit r WHERE r.action=NEW.action) THEN
  RAISE EXCEPTION 'synthetic engineering audit failure';
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER test_reject_audit BEFORE INSERT ON chartworks.audit_events
FOR EACH ROW EXECUTE FUNCTION chartworks.test_reject_audit();`); err != nil {
		t.Fatal("audit failure fixture", err)
	}
	if _, err := metadata.Exec(ctx, `INSERT INTO chartworks.test_rejected_audit(action) VALUES($1)`, action); err != nil {
		t.Fatal(err)
	}
	return func() {
		t.Helper()
		if _, err := metadata.Exec(ctx, `DELETE FROM chartworks.test_rejected_audit WHERE action=$1`, action); err != nil {
			t.Fatal("restore audit availability", err)
		}
	}
}

func TestUploadAuditRollbackAndExternalReconciliation(t *testing.T) {
	t.Run("reservation", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		ctx := context.Background()
		raw, columns := engineeringCSV()
		spec := engineeringSpec("audit-reserve", "csv", raw, columns)
		restore := rejectEngineeringAudit(t, f, "upload.reserved")
		before := f.lookups.Load()
		if _, err := f.service.ReserveUpload(ctx, f.e, spec); err == nil {
			t.Fatal("reservation succeeded without its audit")
		}
		metadata := support.Raw(t, f.dsn)
		if count(t, metadata, `SELECT count(*) FROM chartworks.uploads`) != 0 || f.lookups.Load() != before {
			t.Fatal("failed reserve retained metadata or contacted the warehouse")
		}
		restore()
		f.stage(t, spec, raw)
	})
	t.Run("staging", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		ctx := context.Background()
		raw, columns := engineeringCSV()
		spec := engineeringSpec("audit-stage", "csv", raw, columns)
		if _, err := f.service.ReserveUpload(ctx, f.e, spec); err != nil {
			t.Fatal(err)
		}
		restore := rejectEngineeringAudit(t, f, "upload.staged")
		if _, err := f.service.StageUpload(ctx, f.e, spec.ID, bytes.NewReader(raw)); err == nil {
			t.Fatal("staging succeeded without its audit")
		}
		status, err := f.service.InspectUpload(ctx, f.e, spec.ID)
		if err != nil || status.State != "awaiting_data" || status.Source != nil {
			t.Fatal("failed stage exposed a partially committed source", err, status)
		}
		restore()
		status, err = f.service.StageUpload(ctx, f.e, spec.ID, bytes.NewReader(raw))
		if err != nil || status.State != "staged" {
			t.Fatal("exact external staging effect was not recoverable", err, status)
		}
		run, err := f.service.LoadUpload(ctx, f.e, spec.ID, "audit-stage-load", false)
		if err != nil || run.Upload.Source == nil || run.Upload.State != "active" {
			t.Fatal("reconciled staging did not activate", err, run)
		}
		f.readUpload(t, *run.Upload.Source)
	})
	for _, action := range []string{"upload.activated", "request.succeeded"} {
		t.Run("activation-"+action, func(t *testing.T) {
			f := newEngineeringFixture(t, nil, nil)
			ctx := context.Background()
			raw, columns := engineeringCSV()
			spec := engineeringSpec("audit-activate", "csv", raw, columns)
			f.stage(t, spec, raw)
			restore := rejectEngineeringAudit(t, f, action)
			first, err := f.service.LoadUpload(ctx, f.e, spec.ID, "audit-load", false)
			if err != nil || first.Operation.State != "retry" || first.Upload.State != "staged" || first.Upload.Source != nil {
				t.Fatal("failed publication did not roll back domain and operation together", err, first)
			}
			metadata := support.Raw(t, f.dsn)
			if count(t, metadata, `SELECT count(*) FROM chartworks.sources`) != 0 {
				t.Fatal("unaudited source pointer survived rollback")
			}
			restore()
			run, err := f.service.LoadUpload(ctx, f.e, spec.ID, "audit-load", true)
			if err != nil || run.Upload.Source == nil || run.Operation.ID != first.Operation.ID || run.Operation.Attempts != 2 || run.Operation.State != "succeeded" {
				t.Fatal("load retry did not reconcile the original table", err, run)
			}
			if count(t, metadata, `SELECT count(*) FROM chartworks.sources`) != 1 {
				t.Fatal("recovery produced duplicate source pointers")
			}
			f.readUpload(t, *run.Upload.Source)
		})
	}
	for _, action := range []string{"upload.erased", "request.succeeded"} {
		t.Run("erasure-"+action, func(t *testing.T) {
			f := newEngineeringFixture(t, nil, nil)
			ctx := context.Background()
			raw, columns := engineeringCSV()
			loaded := f.load(t, engineeringSpec("audit-erase", "csv", raw, columns), raw)
			spec := f.profileSpec(t, *loaded.Upload.Source, "audit-erase-profile", []string{"id", "amount"}, "")
			f.profile(t, spec)
			restore := rejectEngineeringAudit(t, f, action)
			first, err := f.service.EraseUpload(ctx, f.e, loaded.Upload.ID, "audit-erase-key", false)
			if err != nil || first.Operation.State != "retry" || first.Upload.State != "deleting" {
				t.Fatal("unaudited erasure falsely completed", err, first)
			}
			metadata := support.Raw(t, f.dsn)
			if count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions WHERE result IS NOT NULL`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_heads`) != 1 {
				t.Fatal("derived-value cleanup escaped its failed completion transaction")
			}
			if _, err = f.service.Evidence(ctx, f.e, spec.ID); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("tombstoned source still exposed evidence during recovery", err)
			}
			restore()
			run, err := f.service.EraseUpload(ctx, f.e, loaded.Upload.ID, "audit-erase-key", true)
			if err != nil || run.Upload.State != "erased" || run.Operation.State != "succeeded" || run.Operation.ID != first.Operation.ID || run.Operation.Attempts != 2 {
				t.Fatal("erasure did not reconcile its observed external deletion", err, run)
			}
			if count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions WHERE result IS NOT NULL`) != 0 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_heads`) != 0 {
				t.Fatal("derived evidence survived audited erasure")
			}
		})
	}
}

func TestProfileAuditRollbackPreservesStageEvidence(t *testing.T) {
	for _, action := range []string{"profile.reserved", "profile.checkpoint", "profile.published", "request.succeeded"} {
		t.Run(action, func(t *testing.T) {
			f := newEngineeringFixture(t, nil, nil)
			ctx := context.Background()
			source := f.create(t, "profile-audit-source")
			spec := f.profileSpec(t, source, "profile-audit-version", []string{"id", "amount"}, "")
			restore := rejectEngineeringAudit(t, f, action)
			first, err := f.service.Build(ctx, f.e, spec, "profile-audit-key", false)
			metadata := support.Raw(t, f.dsn)
			wantReads := int64(1)
			resume := true
			if action == "profile.reserved" {
				if err == nil || count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions`) != 0 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 0 {
					t.Fatal("failed profile reservation retained state or ran the source", err)
				}
				resume = false
			} else {
				state := "checkpoint"
				if action == "profile.checkpoint" {
					state = "sampling"
					wantReads = 2
				}
				if err != nil || first.Operation.State != "retry" || first.Profile.State != state || first.Profile.Profile != nil || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 1 {
					t.Fatal("failed audit lost the exact completed read or stage", err, first)
				}
			}
			if count(t, metadata, `SELECT count(*) FROM chartworks.profile_heads`) != 0 {
				t.Fatal("failed audit published an active profile")
			}
			restore()
			run, err := f.service.Build(ctx, f.e, spec, "profile-audit-key", resume)
			if err != nil || run.Profile.State != "complete" || run.Profile.Profile == nil || run.Operation.State != "succeeded" {
				t.Fatal("profile recovery failed", err, run)
			}
			if resume && (run.Operation.ID != first.Operation.ID || run.Operation.Attempts != 2) {
				t.Fatal("profile recovery replaced its logical operation")
			}
			if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != wantReads || count(t, metadata, `SELECT count(*) FROM chartworks.profile_heads`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions`) != 1 {
				t.Fatal("recovery repeated a retained read or duplicated immutable evidence")
			}
		})
	}
}

func TestDependencyAuditFailureCannotRetainUnauditedReference(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	source := f.create(t, "dependency-audit-source")
	spec := f.profileSpec(t, source, "dependency-audit-profile", []string{"id", "amount"}, "")
	f.profile(t, spec)
	dep := engineering.Dependency{Kind: "report", ID: "dependency-audit-report", Version: readexec.Hash("fixed-consumer-definition"), Source: spec.Source, Context: spec.Context, Dataset: spec.Dataset, Columns: []string{"id"}}
	restore := rejectEngineeringAudit(t, f, "profile.dependency_registered")
	if err := f.service.RegisterDependency(ctx, f.e, spec.ID, dep); err == nil {
		t.Fatal("unaudited dependency registration succeeded")
	}
	metadata := support.Raw(t, f.dsn)
	if count(t, metadata, `SELECT count(*) FROM chartworks.profile_dependencies`) != 0 {
		t.Fatal("unaudited dependency survived rollback")
	}
	restore()
	for range 2 {
		if err := f.service.RegisterDependency(ctx, f.e, spec.ID, dep); err != nil {
			t.Fatal("idempotent audited dependency registration", err)
		}
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.profile_dependencies`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE action='profile.dependency_registered'`) != 1 {
		t.Fatal("idempotent recovery duplicated a dependency or its audit")
	}
	changed := dep
	changed.Columns = []string{"amount"}
	if err := f.service.RegisterDependency(ctx, f.e, spec.ID, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("accepted consumer definition changed under its immutable version", err)
	}
	changed.ID, changed.Columns = "unknown-column-consumer", []string{"missing"}
	if err := f.service.RegisterDependency(ctx, f.e, spec.ID, changed); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("dependency referenced a column without profile evidence", err)
	}
}

func TestSchemaHealthAuditRollbackKeepsPreviousPublication(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	source := f.create(t, "health-audit-source")
	firstSpec := f.profileSpec(t, source, "health-audit-before", []string{"id", "amount"}, "")
	before := f.profile(t, firstSpec)
	dep := engineering.Dependency{Kind: "report", ID: "health-audit-report", Version: readexec.Hash("approved-health-consumer"), Source: firstSpec.Source, Context: firstSpec.Context, Dataset: firstSpec.Dataset, Columns: []string{"amount"}}
	if err := f.service.RegisterDependency(ctx, f.e, firstSpec.ID, dep); err != nil {
		t.Fatal(err)
	}
	if _, err := f.admin.Exec(ctx, `ALTER TABLE analytics.sales ALTER COLUMN amount TYPE text USING amount::text`); err != nil {
		t.Fatal(err)
	}
	rotated, err := f.s.Rotate(ctx, f.e, source.ID, source.Revision)
	if err != nil {
		t.Fatal(err)
	}
	afterSpec := f.profileSpec(t, rotated, "health-audit-after", []string{"id", "amount"}, "")
	afterSpec.Previous = firstSpec.ID
	restore := rejectEngineeringAudit(t, f, "profile.health_changed")
	interrupted, err := f.service.Build(ctx, f.e, afterSpec, "health-audit-key", false)
	if err != nil || interrupted.Profile.State != "checkpoint" || interrupted.Operation.State != "retry" {
		t.Fatal("unaudited health publication completed", err, interrupted)
	}
	metadata := support.Raw(t, f.dsn)
	if count(t, metadata, `SELECT count(*) FROM chartworks.profile_health_events`) != 0 {
		t.Fatal("unaudited health event survived rollback")
	}
	old, err := f.service.Evidence(ctx, f.e, firstSpec.ID)
	if err != nil || !old.Active || old.Profile.DeterministicHash() != before.Profile.Profile.DeterministicHash() {
		t.Fatal("health failure changed previous immutable publication", err)
	}
	restore()
	completed, err := f.service.Build(ctx, f.e, afterSpec, "health-audit-key", true)
	if err != nil || completed.Profile.State != "complete" || completed.Operation.ID != interrupted.Operation.ID {
		t.Fatal("health publication did not resume", err, completed)
	}
	events, err := f.service.Health(ctx, f.e, dep)
	if err != nil || len(events) != 1 || events[0].State != "needs_review" || events[0].Profile != afterSpec.ID {
		t.Fatal("drift was lost or duplicated on recovery", err, events)
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_heads`) != 1 {
		t.Fatal("health recovery repeated native sampling or forked the active head")
	}
}
