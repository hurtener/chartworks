package acceptance

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// Inject interruption after durable intent but before native dispatch. This is
// not an OS-kill fixture and does not fabricate a native execution receipt.
type missingNativeReceipt struct {
	*postgres.DB
	interrupt atomic.Bool
}

func (r *missingNativeReceipt) StartProfileRead(ctx context.Context, invocation jobs.Invocation, profile engineering.ProfileRecord, operation string, deadline time.Time) error {
	if err := r.DB.StartProfileRead(ctx, invocation, profile, operation, deadline); err != nil {
		return err
	}
	if r.interrupt.Swap(false) {
		return store.ErrUnavailable
	}
	return nil
}

func waitProfileDeadline(t *testing.T, ctx context.Context, deadline time.Time) {
	t.Helper()
	delay := time.Until(deadline) + 25*time.Millisecond
	if delay < 0 || delay > 8*time.Second {
		t.Fatal("invalid bounded reconciliation fixture deadline", delay)
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
		t.Fatal("reconciliation fixture exceeded its deadline", ctx.Err())
	}
}

func TestMissingProfileReadReceiptWaitsForOriginalDeadline(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.Timeout = config.Duration(2 * time.Second) }, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	source := f.create(t, "missing-native-receipt")
	spec := f.profileSpec(t, source, "missing-native-profile", []string{"id", "amount"}, "")
	repo := &missingNativeReceipt{DB: f.db}
	repo.interrupt.Store(true)
	service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Build(ctx, f.e, spec, "missing-native-operation", false)
	service.Close()
	if err != nil || first.Profile.State != "sampling" || first.Operation.State != "retry" || first.Profile.Profile != nil {
		t.Fatal("interrupted read intent was published or lost", err, first)
	}
	record, err := f.db.ReadProfile(ctx, f.e, spec.ID, true)
	if err != nil || record.LastReadOperation == "" || !time.Now().Before(record.LastReadDeadline) {
		t.Fatal("native intent deadline was not durably retained", err)
	}
	metadata := support.Raw(t, f.dsn)
	blocked, err := f.service.Build(ctx, f.e, spec, "missing-native-operation", true)
	if err != nil || blocked.Operation.State != "retry" || blocked.Code != "reconciliation_required" || blocked.Profile.Profile != nil || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 0 {
		t.Fatal("absent receipt was treated as proof that dispatch never happened", err, blocked)
	}
	waitProfileDeadline(t, ctx, record.LastReadDeadline)
	completed, err := f.service.Build(ctx, f.e, spec, "missing-native-operation", true)
	if err != nil || completed.Operation.ID != first.Operation.ID || completed.Operation.Attempts != 3 || completed.Operation.State != "succeeded" || completed.Profile.State != "complete" {
		t.Fatal("expired intent did not resume the same accepted profile", err, completed)
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions`) != 1 {
		t.Fatal("read-intent recovery duplicated native work or profile identity")
	}
}

func TestProfileReconcilesUnknownReadBeforeAnotherDispatch(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.Timeout = config.Duration(2 * time.Second) }, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	source := f.create(t, "uncertain-native-source")
	spec := f.profileSpec(t, source, "uncertain-native-profile", []string{"id", "amount"}, "")
	restore := rejectEngineeringAudit(t, f, "read.succeeded")
	first, err := f.service.Build(ctx, f.e, spec, "uncertain-native-operation", false)
	if err != nil || first.Profile.State != "sampling" || first.Operation.State != "retry" || first.Code != "reconciliation_required" || first.Profile.Profile != nil {
		t.Fatal("lost native completion receipt falsely became a profile", err, first)
	}
	record, err := f.db.ReadProfile(ctx, f.e, spec.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := f.executor.ByOperation(ctx, f.e, record.LastReadOperation)
	if err != nil || attempt.Remote == nil || attempt.Finished != nil {
		t.Fatal("failure injection did not retain an unresolved native attempt", err, attempt)
	}
	restore()
	metadata := support.Raw(t, f.dsn)
	blocked, err := f.service.Build(ctx, f.e, spec, "uncertain-native-operation", true)
	if err != nil || blocked.Code != "reconciliation_required" || blocked.Profile.Profile != nil || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 1 {
		t.Fatal("retry dispatched before the original attempt was reconciled", err, blocked)
	}
	waitProfileDeadline(t, ctx, attempt.Deadline.Add(attempt.Manifest.Limits.CancelGrace))
	completed, err := f.service.Build(ctx, f.e, spec, "uncertain-native-operation", true)
	if err != nil || completed.Operation.ID != first.Operation.ID || completed.Operation.Attempts != 3 || completed.Operation.State != "succeeded" || completed.Profile.State != "complete" || completed.Profile.Profile == nil {
		t.Fatal("native stop reconciliation did not permit an explicit retry", err, completed)
	}
	old, err := f.executor.Inspect(ctx, f.e, attempt.ID)
	if err != nil || old.Status != "interrupted" || old.RemoteState != "stopped" || old.Rows != 0 || old.Bytes != 0 || old.Finished == nil {
		t.Fatal("reconciliation invented lost values or did not prove remote stop", err, old)
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 2 || completed.Profile.Profile.ReadAttempt == attempt.ID {
		t.Fatal("recovery reused lost result values or issued duplicate retries")
	}
}
