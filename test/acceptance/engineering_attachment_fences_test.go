package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// A structurally valid task is retained data, not permission to replace the
// actual stored manifest or a live owner's attachment.
func TestProfileAttachmentChecksStoredManifestAndLiveOwner(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	source := f.create(t, "attachment-source")
	spec := f.profileSpec(t, source, "attachment-first", []string{"id", "amount"}, "")
	f.profile(t, spec)
	record, err := f.db.ReadProfile(ctx, f.e, spec.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	record.Spec.ID, record.Spec.Previous, record.Result = "attachment-next", spec.ID, nil
	record.SpecHash = record.Digest()
	record, err = f.db.ReserveProfile(ctx, f.e, record)
	if err != nil {
		t.Fatal(err)
	}
	input := jobs.RequestInput{Kind: "profile.build", Target: spec.Source, Context: spec.Context, InputHash: record.SpecHash}
	task, err := f.db.AdmitRequest(ctx, f.e, "profile-attachment-one", input, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	metadata := support.Raw(t, f.dsn)
	before := count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`)
	for _, tc := range []struct {
		name string
		edit func(*jobs.RequestTask)
		want error
	}{
		{"different-input", func(v *jobs.RequestTask) { v.Input.InputHash = readexec.Hash("other-input") }, store.ErrConflict},
		{"missing-operation", func(v *jobs.RequestTask) { v.ID = strings.Repeat("f", 32) }, store.ErrNotFound},
		{"changed-budget", func(v *jobs.RequestTask) { v.MaxAttempts = 8 }, store.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := task
			tc.edit(&changed)
			changed.ManifestHash = changed.Digest()
			if !changed.Valid() {
				t.Fatal("fixture must reach stored-manifest validation")
			}
			out, err := f.db.AttachProfile(ctx, f.e, record.Spec.ID, changed)
			if !errors.Is(rejectedEngineeringValue(t, out, err), tc.want) {
				t.Fatal("invalid attachment was not rejected", err)
			}
		})
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`) != before {
		t.Fatal("rejected attachment changed durable audit state")
	}
	if _, err = f.db.AttachProfile(ctx, f.e, record.Spec.ID, task); err != nil {
		t.Fatal(err)
	}
	// This claims only a ledger owner; it does not dispatch native work.
	if _, err = f.db.ClaimRequest(ctx, f.e, task.ID, "profile-live-owner", jobs.Defaults()); err != nil {
		t.Fatal(err)
	}
	other, err := f.db.AdmitRequest(ctx, f.e, "profile-attachment-two", input, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.db.AttachProfile(ctx, f.e, record.Spec.ID, other)
	if !errors.Is(rejectedEngineeringValue(t, out, err), store.ErrConflict) {
		t.Fatal("a second logical operation displaced the live owner", err)
	}
	retained, err := f.db.ReadProfile(ctx, f.e, record.Spec.ID, true)
	if err != nil || retained.Operation != task.ID || retained.State != "reserved" || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 1 {
		t.Fatal("attachment conflict dispatched or changed accepted work", err)
	}
}

func TestUploadAttachmentChecksStoredManifestAndAuditedErasureFence(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("attachment-upload", "csv", raw, columns)
	f.stage(t, spec, raw)
	record, err := f.db.ReadUpload(ctx, f.e, spec.ID, "sources.upload", "write")
	if err != nil {
		t.Fatal(err)
	}
	input := jobs.RequestInput{Kind: "upload.load", Target: spec.ID, InputHash: record.SpecHash}
	task, err := f.db.AdmitRequest(ctx, f.e, "upload-attachment-one", input, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(*jobs.RequestTask)
		want error
	}{
		{"different-input", func(v *jobs.RequestTask) { v.Input.InputHash = readexec.Hash("other-input") }, store.ErrConflict},
		{"missing-operation", func(v *jobs.RequestTask) { v.ID = strings.Repeat("f", 32) }, store.ErrNotFound},
		{"changed-budget", func(v *jobs.RequestTask) { v.MaxAttempts = 8 }, store.ErrConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := task
			tc.edit(&changed)
			changed.ManifestHash = changed.Digest()
			if !changed.Valid() {
				t.Fatal("fixture must reach stored-manifest validation")
			}
			out, err := f.db.AttachUpload(ctx, f.e, spec.ID, changed, false)
			if !errors.Is(rejectedEngineeringValue(t, out, err), tc.want) {
				t.Fatal("invalid upload attachment was not rejected", err)
			}
		})
	}
	other, err := f.db.AdmitRequest(ctx, f.e, "upload-attachment-two", input, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.AttachUpload(ctx, f.e, spec.ID, task, false); err != nil {
		t.Fatal(err)
	}
	// Retain an acquiring attempt but deliberately do not dispatch a warehouse
	// transaction: this test covers the metadata erasure fence, not remote stop.
	if _, err = f.db.ClaimRequest(ctx, f.e, task.ID, "upload-live-owner", jobs.Defaults()); err != nil {
		t.Fatal(err)
	}
	out, err := f.db.AttachUpload(ctx, f.e, spec.ID, other, false)
	if !errors.Is(rejectedEngineeringValue(t, out, err), store.ErrConflict) {
		t.Fatal("second load displaced the live owner", err)
	}
	if _, err = f.db.CancelRequest(ctx, f.e, other.ID); err != nil {
		t.Fatal(err)
	}
	out, err = f.db.AttachUpload(ctx, f.e, spec.ID, other, false)
	if !errors.Is(rejectedEngineeringValue(t, out, err), store.ErrConflict) {
		t.Fatal("cancelled operation was attached using stale pending metadata", err)
	}
	eraseInput := input
	eraseInput.Kind = "upload.erase"
	eraseTask, err := f.db.AdmitRequest(ctx, f.e, "upload-attachment-erase", eraseInput, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	restore := rejectEngineeringAudit(t, f, "upload.erasure_requested")
	out, err = f.db.AttachUpload(ctx, f.e, spec.ID, eraseTask, true)
	if rejectedEngineeringValue(t, out, err) == nil {
		t.Fatal("erasure intent succeeded without its audit")
	}
	retained, err := f.db.ReadUpload(ctx, f.e, spec.ID, "sources.upload", "write")
	if err != nil || retained.State != "staged" || retained.Operation != task.ID {
		t.Fatal("failed erasure audit partially changed the upload", err)
	}
	old, err := f.db.ReadRequest(ctx, f.e, task.ID)
	if err != nil || old.State != "running" {
		t.Fatal("failed erasure audit partially cancelled the old owner", err)
	}
	restore()
	erased, err := f.service.EraseUpload(ctx, f.e, spec.ID, "upload-attachment-erase", false)
	if err != nil || erased.Upload.State != "erased" || erased.Operation.State != "succeeded" {
		t.Fatal("audited erasure did not finish", err, erased)
	}
	old, err = f.db.ReadRequest(ctx, f.e, task.ID)
	metadata := support.Raw(t, f.dsn)
	if err != nil || old.State != "cancelled" || count(t, metadata, `SELECT count(*) FROM chartworks.operation_attempts WHERE state='cancelled'`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.sources WHERE NOT deleted`) != 0 {
		t.Fatal("erasure did not fence the acquiring owner atomically", err, old)
	}
}

func TestUploadErasureAttachmentRejectsWrongSourceRevision(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("revision-attachment", "csv", raw, columns)
	loaded := f.load(t, spec, raw)
	record, err := f.db.ReadUpload(ctx, f.e, spec.ID, "sources.erase", "erase")
	if err != nil {
		t.Fatal(err)
	}
	input := jobs.RequestInput{Kind: "upload.erase", Target: spec.ID, Context: "wrong:v1", InputHash: record.SpecHash}
	task, err := f.db.AdmitRequest(ctx, f.e, "wrong-revision-erasure", input, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.db.AttachUpload(ctx, f.e, spec.ID, task, true)
	if !errors.Is(rejectedEngineeringValue(t, out, err), readexec.ErrBinding) {
		t.Fatal("unrelated execution context could tombstone the source", err)
	}
	f.readUpload(t, *loaded.Upload.Source)
}

type summaryAuditBoundary struct {
	*postgres.DB
	t       *testing.T
	restore func()
}

func (r *summaryAuditBoundary) CheckpointProfile(ctx context.Context, inv jobs.Invocation, record engineering.ProfileRecord, profile engineering.Profile) error {
	r.t.Helper()
	if admitted, err := r.StartProfileSummary(ctx, inv, record); admitted || !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("summary was admitted before deterministic evidence", admitted, err)
	}
	if err := r.DB.CheckpointProfile(ctx, inv, record, profile); err != nil {
		return err
	}
	admitted, err := r.StartProfileSummary(ctx, inv, record)
	if err == nil || admitted {
		r.t.Fatal("failed audit returned a successful model-admission receipt", admitted, err)
	}
	e, authorityErr := inv.Current("profile.build", record.Spec.Source, record.SpecHash)
	if authorityErr != nil {
		r.t.Fatal(authorityErr)
	}
	retained, readErr := r.ReadProfile(ctx, e, record.Spec.ID, true)
	if readErr != nil || retained.SummaryStarted || retained.State != "checkpoint" {
		r.t.Fatal("failed summary admission escaped rollback", readErr)
	}
	r.restore()
	return err
}

func TestProfileSummaryAdmissionReceiptRequiresCommittedAudit(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.Summaries = true }, nil)
	ctx := context.Background()
	source := f.create(t, "summary-audit-source")
	spec := f.profileSpec(t, source, "summary-audit-profile", []string{"id", "amount"}, "")
	spec.SkipLLM = false
	repo := &summaryAuditBoundary{DB: f.db, t: t, restore: rejectEngineeringAudit(t, f, "profile.summary_started")}
	service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	first, err := service.Build(ctx, f.e, spec, "summary-audit-operation", false)
	if err != nil || first.Operation.State != "retry" || first.Profile.State != "checkpoint" {
		t.Fatal("failed summary audit lost deterministic evidence", err, first)
	}
	// With no model configured, recovery truthfully publishes deterministic
	// evidence with a disabled summary; it must not repeat the retained read.
	completed, err := f.service.Build(ctx, f.e, spec, "summary-audit-operation", true)
	if err != nil || completed.Operation.ID != first.Operation.ID || completed.Operation.State != "succeeded" || completed.Profile.Profile == nil || completed.Profile.Profile.Summary.Status != "disabled" {
		t.Fatal("deterministic recovery after summary audit failure", err, completed)
	}
	metadata := support.Raw(t, f.dsn)
	if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE action='profile.summary_started'`) != 0 {
		t.Fatal("failed summary admission repeated sampling or survived rollback")
	}
}
