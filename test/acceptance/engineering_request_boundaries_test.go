package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

func TestExpiredRequestResumeCannotResetAttemptBudget(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("exhausted-owner", "csv", raw, columns)
	f.stage(t, spec, raw)
	limits := jobs.Defaults()
	limits.MaxAttempts = 1
	limits.Lease, limits.Heartbeat = time.Second, 100*time.Millisecond
	task, err := f.db.AdmitRequest(ctx, f.e, "exhausted-owner-key", jobs.RequestInput{Kind: "upload.load", Target: spec.ID, InputHash: readexec.Hash(spec)}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ClaimRequest(ctx, f.e, task.ID, "exhausted-owner", limits); err != nil {
		t.Fatal(err)
	}
	awaitExpiredRequestLease(t, f, task.ID)
	if _, err = f.db.ResumeRequest(ctx, f.e, task.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatal("expired-owner recovery reset the accepted attempt ceiling", err)
	}
	current, err := f.db.ReadRequest(ctx, f.e, task.ID)
	if err != nil || current.Attempts != 1 || current.MaxAttempts != 1 || current.ManifestHash != task.ManifestHash {
		t.Fatal("rejected resume changed immutable budget or attempt evidence", err, current)
	}
}

func TestRequestHandlerCannotInventPublicationSuccess(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("unpublished-handler", "csv", raw, columns)
	f.stage(t, spec, raw)
	runner, err := jobs.NewRequestRunner(f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	task, err := runner.Admit(ctx, f.e, "unpublished-handler-key", jobs.RequestInput{Kind: "upload.load", Target: spec.ID, InputHash: readexec.Hash(spec)})
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately faulty internal handler: no source activation or atomic
	// completion. A nil return must not be mistaken for persisted success.
	out, err := runner.Run(ctx, f.e, task, time.Second, func(context.Context, jobs.Invocation) error { return nil })
	if !errors.Is(err, store.ErrConflict) || out.State == "succeeded" {
		t.Fatal("handler return fabricated a successful ledger receipt", err, out)
	}
	upload, err := f.db.ReadUpload(ctx, f.e, spec.ID, "sources.read", "read")
	if err != nil || upload.State != "staged" || upload.SourceRevision != 0 {
		t.Fatal("unpublished handler produced an active dataset", err, upload)
	}
	if _, err = runner.Cancel(ctx, f.e, task.ID); err != nil {
		t.Fatal("cancel unfinished faulty-handler fixture", err)
	}
}
