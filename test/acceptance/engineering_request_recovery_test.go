package acceptance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

// Inject the persisted state left by process loss after a real checkpoint: no
// publication, no failure cleanup and no heartbeat after Run returns. This is a
// failure-injection fixture, not a claim that this test kills an OS process.
type profileOwnerLoss struct{ *postgres.DB }

func (r *profileOwnerLoss) PublishProfile(context.Context, jobs.Invocation, engineering.ProfileRecord, engineering.Profile) error {
	return store.ErrUnavailable
}

func (r *profileOwnerLoss) FailRequest(context.Context, jobs.Invocation, string, bool, time.Duration) error {
	return nil
}

func awaitExpiredRequestLease(t *testing.T, f *engineeringFixture, id string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	raw := support.Raw(t, f.dsn)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		var expired bool
		if err := raw.QueryRow(ctx, `SELECT status='running' AND COALESCE(lease_until<=clock_timestamp(),false) FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2`, f.e.Tenant(), id).Scan(&expired); err != nil {
			t.Fatal("observe original request lease", err)
		}
		if expired {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("request owner did not expire", ctx.Err())
		case <-ticker.C:
		}
	}
}

func TestUploadExplicitResumeReclaimsExpiredOwner(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("expired-load-owner", "csv", raw, columns)
	f.stage(t, spec, raw)
	upload, err := f.db.ReadUpload(ctx, f.e, spec.ID, "sources.upload", "write")
	if err != nil {
		t.Fatal(err)
	}
	limits := jobs.Defaults()
	limits.Lease, limits.Heartbeat = time.Second, 100*time.Millisecond
	task, err := f.db.AdmitRequest(ctx, f.e, "expired-load-key", jobs.RequestInput{Kind: "upload.load", Target: spec.ID, InputHash: upload.SpecHash}, limits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.AttachUpload(ctx, f.e, spec.ID, task, false); err != nil {
		t.Fatal(err)
	}
	lease, err := f.db.ClaimRequest(ctx, f.e, task.ID, "lost-upload-owner", limits)
	if err != nil {
		t.Fatal(err)
	}
	before := f.lookups.Load()
	if _, err = f.db.ResumeRequest(ctx, f.e, task.ID); !errors.Is(err, store.ErrConflict) {
		t.Fatal("explicit resume displaced a live owner", err)
	}
	for _, caller := range []identity.Envelope{{}, f.actor(t, "source-b", f.e.User()), f.actor(t, f.e.Tenant(), "unrelated-profiler")} {
		if _, err = f.db.ResumeRequest(ctx, caller, task.ID); err == nil {
			t.Fatal("request resume accepted foreign authority")
		}
	}
	if f.lookups.Load() != before {
		t.Fatal("resume denial resolved workspace credentials")
	}
	awaitExpiredRequestLease(t, f, task.ID)
	resumable, err := f.db.ResumeRequest(ctx, f.e, task.ID)
	if err != nil || resumable.State != "running" || resumable.Attempts != 1 || resumable.ManifestHash != task.ManifestHash || !resumable.Created.Equal(task.Created) || !resumable.Expires.Equal(task.Expires) {
		t.Fatal("resume did not preserve the crashed owner's manifest and evidence", err, resumable)
	}
	metadata := support.Raw(t, f.dsn)
	var state, owner string
	var fence int64
	if err = metadata.QueryRow(ctx, `SELECT a.state,o.lease_owner,o.fence FROM chartworks.operation_attempts a JOIN chartworks.operations o USING(tenant_id,operation_id) WHERE a.tenant_id=$1 AND a.operation_id=$2 AND a.attempt=1`, f.e.Tenant(), task.ID).Scan(&state, &owner, &fence); err != nil || state != "acquiring" || owner != lease.Owner || fence != lease.Fence {
		t.Fatal("resume falsely declared a native stop or replaced the lease", err, state, owner, fence)
	}
	loaded, err := f.service.LoadUpload(ctx, f.e, spec.ID, "expired-load-key", true)
	if err != nil || loaded.Upload.State != "active" || loaded.Upload.Source == nil || loaded.Operation.ID != task.ID || loaded.Operation.Attempts != 2 || loaded.Code != "" {
		t.Fatal("explicit upload resume did not reclaim the expired owner", err, loaded)
	}
	if err = metadata.QueryRow(ctx, `SELECT a.state,o.fence FROM chartworks.operation_attempts a JOIN chartworks.operations o USING(tenant_id,operation_id) WHERE a.tenant_id=$1 AND a.operation_id=$2 AND a.attempt=1`, f.e.Tenant(), task.ID).Scan(&state, &fence); err != nil || state != "abandoned" || fence != lease.Fence+1 {
		t.Fatal("common claim engine did not replace the original fence exactly once", err, state, fence)
	}
	f.readUpload(t, *loaded.Upload.Source)
}

func TestProfileExplicitResumeReusesExpiredOwnersCheckpoint(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) {
		v.Jobs.Lease, v.Jobs.Heartbeat = config.Duration(time.Second), config.Duration(100*time.Millisecond)
	}, nil)
	ctx := context.Background()
	source := f.create(t, "expired-profile-owner")
	spec := f.profileSpec(t, source, "expired-profile-version", []string{"id", "amount"}, "")
	service, err := engineering.New(&profileOwnerLoss{DB: f.db}, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.Build(ctx, f.e, spec, "expired-profile-key", false)
	service.Close()
	if err != nil || first.Profile.State != "checkpoint" || first.Profile.Profile != nil || first.Operation.State != "running" {
		t.Fatal("failure injection did not preserve an unfinished owned checkpoint", err, first)
	}
	checkpoint, err := f.db.ReadProfile(ctx, f.e, spec.ID, false)
	if err != nil || checkpoint.Result == nil {
		t.Fatal("real deterministic checkpoint was not retained", err)
	}
	hash := checkpoint.Result.DeterministicHash()
	metadata := support.Raw(t, f.dsn)
	reads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	if reads != 1 {
		t.Fatal("fixture did not complete one actual native read", reads)
	}
	awaitExpiredRequestLease(t, f, first.Operation.ID)
	resumed, err := f.service.Build(ctx, f.e, spec, "expired-profile-key", true)
	if err != nil || resumed.Profile.State != "complete" || resumed.Profile.Profile == nil || resumed.Operation.ID != first.Operation.ID || resumed.Operation.Attempts != 2 || resumed.Code != "" {
		t.Fatal("profile resume did not recover its expired owner", err, resumed)
	}
	if resumed.Profile.Profile.DeterministicHash() != hash || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != reads {
		t.Fatal("expired-owner resume repeated source work or changed its checkpoint")
	}
}

// Commit for real, then lose the repository method's reply. A confirmed journal
// receipt may resolve that uncertainty, but handler success alone may not.
type lostEngineeringCommitReply struct{ *postgres.DB }

func (r *lostEngineeringCommitReply) ActivateUpload(ctx context.Context, i jobs.Invocation, upload engineering.UploadRecord, receipt engineering.WorkspaceReceipt, source sources.Record) error {
	if err := r.DB.ActivateUpload(ctx, i, upload, receipt, source); err != nil {
		return err
	}
	return store.ErrUnavailable
}

func (r *lostEngineeringCommitReply) PublishProfile(ctx context.Context, i jobs.Invocation, record engineering.ProfileRecord, profile engineering.Profile) error {
	if err := r.DB.PublishProfile(ctx, i, record, profile); err != nil {
		return err
	}
	return store.ErrUnavailable
}

func (r *lostEngineeringCommitReply) FinishUploadErasure(ctx context.Context, i jobs.Invocation, upload engineering.UploadRecord) error {
	if err := r.DB.FinishUploadErasure(ctx, i, upload); err != nil {
		return err
	}
	return store.ErrUnavailable
}

func TestEngineeringCommitReplyLossUsesConfirmedReceipt(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	service, err := engineering.New(&lostEngineeringCommitReply{DB: f.db}, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("lost-commit-reply", "csv", raw, columns)
	f.stage(t, spec, raw)
	loaded, err := service.LoadUpload(ctx, f.e, spec.ID, "lost-load-reply", false)
	if err != nil || loaded.Upload.State != "active" || loaded.Upload.Source == nil || loaded.Operation.State != "succeeded" || loaded.Code != "" {
		t.Fatal("confirmed upload success was downgraded by a lost reply", err, loaded)
	}
	profileSpec := f.profileSpec(t, *loaded.Upload.Source, "lost-profile-reply", []string{"id", "amount"}, "")
	profile, err := service.Build(ctx, f.e, profileSpec, "lost-profile-reply", false)
	if err != nil || profile.Profile.State != "complete" || profile.Profile.Profile == nil || profile.Operation.State != "succeeded" || profile.Code != "" {
		t.Fatal("confirmed profile success was downgraded by a lost reply", err, profile)
	}
	erased, err := service.EraseUpload(ctx, f.e, spec.ID, "lost-erasure-reply", false)
	if err != nil || erased.Upload.State != "erased" || erased.Operation.State != "succeeded" || erased.Code != "" {
		t.Fatal("confirmed erasure success was downgraded by a lost reply", err, erased)
	}
}
