package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/test/support"
)

func TestRequestCapacityExcludesTerminalReceiptsAndAllowsErasure(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) { v.Jobs.MaxPending = 2; v.Jobs.MaxPendingPerTenant = 1 }, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("capacity-terminal", "csv", raw, columns)
	loaded := f.load(t, spec, raw)
	profile := f.profileSpec(t, *loaded.Upload.Source, "capacity-terminal-profile", []string{"id", "amount"}, "")
	f.profile(t, profile)
	erased, err := f.service.EraseUpload(ctx, f.e, spec.ID, "capacity-erase", false)
	if err != nil || erased.Operation.State != "succeeded" || erased.Upload.State != "erased" {
		t.Fatal("terminal load/profile receipts blocked erasure", err, erased)
	}
	receipt, err := f.db.ReadRequest(ctx, f.e, loaded.Operation.ID)
	if err != nil || receipt.State != "succeeded" || receipt.ManifestHash != loaded.Operation.ManifestHash {
		t.Fatal("capacity release discarded replay receipt", err)
	}
	limits := jobs.Defaults()
	limits.MaxPending, limits.MaxPendingPerTenant = 2, 1
	replay, err := f.db.AdmitRequest(ctx, f.e, "capacity-erase", erased.Operation.Input, limits)
	if err != nil || replay.ID != erased.Operation.ID {
		t.Fatal("terminal erasure replay changed identity", err)
	}
	db := support.Raw(t, f.dsn)
	if count(t, db, `SELECT count(*) FROM chartworks.operations WHERE dispatch_mode='request' AND status='succeeded'`) != 3 {
		t.Fatal("terminal receipts were deleted to free capacity")
	}
}

func TestRequestCancelledResumeSharesConcurrentAdmissionCapacity(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	limits := jobs.Defaults()
	limits.MaxPending, limits.MaxPendingPerTenant = 2, 1
	input := jobs.RequestInput{Kind: "upload.load", Target: "capacity-pending", InputHash: readexec.Hash("capacity-manifest")}
	first, err := f.db.AdmitRequest(ctx, f.e, "capacity-first", input, limits)
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.db.AdmitRequest(ctx, f.e, "capacity-blocked", input, limits)
	if !errors.Is(rejectedEngineeringValue(t, out, err), jobs.ErrBusy) {
		t.Fatal("pending work did not cap admission", err)
	}
	if _, err = f.db.CancelRequest(ctx, f.e, first.ID); err != nil {
		t.Fatal(err)
	}
	occupied, err := f.db.AdmitRequest(ctx, f.e, "capacity-occupied", input, limits)
	if err != nil {
		t.Fatal(err)
	}
	db := support.Raw(t, f.dsn)
	before := count(t, db, `SELECT count(*) FROM chartworks.audit_events`)
	out, err = f.db.ResumeRequest(ctx, f.e, first.ID, limits)
	if !errors.Is(rejectedEngineeringValue(t, out, err), jobs.ErrBusy) {
		t.Fatal("cancelled resume bypassed full queue", err)
	}
	if count(t, db, `SELECT count(*) FROM chartworks.audit_events`) != before {
		t.Fatal("rejected resume changed audit")
	}
	if _, err = f.db.CancelRequest(ctx, f.e, occupied.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ResumeRequest(ctx, f.e, first.ID, limits); err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.ResumeRequest(ctx, f.e, first.ID, limits); err != nil {
		t.Fatal("pending resume double counted existing slot", err)
	}
	if _, err = f.db.CancelRequest(ctx, f.e, first.ID); err != nil {
		t.Fatal(err)
	}
	// A resume and a new actor admission compete for the same tenant slot.
	other := f.actor(t, f.e.Tenant(), "capacity-other")
	type result struct {
		task jobs.RequestTask
		err  error
	}
	start, results := make(chan struct{}), make(chan result, 2)
	go func() {
		<-start
		task, err := f.db.ResumeRequest(ctx, f.e, first.ID, limits)
		results <- result{task, err}
	}()
	go func() {
		<-start
		task, err := f.db.AdmitRequest(ctx, other, "capacity-concurrent", input, limits)
		results <- result{task, err}
	}()
	close(start)
	winners, denied := 0, 0
	for range 2 {
		r := <-results
		switch {
		case r.err == nil:
			winners++
		case errors.Is(rejectedEngineeringValue(t, r.task, r.err), jobs.ErrBusy):
			denied++
		default:
			t.Fatal(r.err)
		}
	}
	if winners != 1 || denied != 1 {
		t.Fatal("concurrent resume/admission exceeded tenant capacity", winners, denied)
	}
	if count(t, db, `SELECT count(*) FROM chartworks.operations WHERE status IN ('pending','retry','running')`) != 1 {
		t.Fatal("capacity fence did not match durable work")
	}
	retained, err := f.db.ReadRequest(ctx, f.e, first.ID)
	if err != nil || retained.ManifestHash != first.ManifestHash || retained.Attempts != 0 {
		t.Fatal("resume changed accepted manifest or attempt evidence", err)
	}
	if retained.State == "pending" {
		// Already-admitted pending work retains its slot; resume does not double count it.
		if _, err = f.db.ResumeRequest(ctx, f.e, first.ID, limits); err != nil {
			t.Fatal("pending resume counted its existing slot twice", err)
		}
	} else {
		before := count(t, db, `SELECT count(*) FROM chartworks.audit_events`)
		out, err = f.db.ResumeRequest(ctx, f.e, first.ID, limits)
		if !errors.Is(rejectedEngineeringValue(t, out, err), jobs.ErrBusy) {
			t.Fatal("cancelled resume bypassed full queue", err)
		}
		if count(t, db, `SELECT count(*) FROM chartworks.audit_events`) != before {
			t.Fatal("rejected resume changed audit")
		}
	}
}
