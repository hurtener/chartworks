package acceptance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

func rejectedEngineeringValue[T any](t *testing.T, value T, err error) error {
	t.Helper()
	if err != nil {
		var zero T
		if !reflect.DeepEqual(value, zero) {
			t.Fatalf("rejected repository operation returned partial private metadata: %T", value)
		}
	}
	return err
}

func TestEngineeringRepositoriesDenyBeforeEffectsAndClearFailedResults(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	uploadSpec := engineeringSpec("repository-boundary", "csv", raw, columns)
	upload := f.load(t, uploadSpec, raw)
	profileSpec := f.profileSpec(t, *upload.Upload.Source, "repository-profile", []string{"id", "amount"}, "")
	profile := f.profile(t, profileSpec)
	record, err := f.db.ReadProfile(ctx, f.e, profileSpec.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	record.Result = nil
	dependency := engineering.Dependency{Kind: "report", ID: "repository-consumer", Version: readexec.Hash("repository-consumer-v1"), Source: profileSpec.Source, Context: profileSpec.Context, Dataset: profileSpec.Dataset, Columns: []string{"id"}}
	if err = f.service.RegisterDependency(ctx, f.e, profileSpec.ID, dependency); err != nil {
		t.Fatal(err)
	}
	limits := jobs.Defaults()
	calls := []struct {
		name string
		run  func(context.Context, *testing.T, identity.Envelope) error
	}{
		{"reserve-upload", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ReserveUpload(ctx, e, uploadSpec, f.values.Uploads)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"read-upload", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ReadUpload(ctx, e, uploadSpec.ID, "sources.read", "read")
			return rejectedEngineeringValue(t, out, err)
		}},
		{"stage-upload", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.StageUpload(ctx, e, uploadSpec.ID, func(context.Context, engineering.UploadRecord) error {
				t.Error("rejected staging reached a warehouse effect")
				return errors.New("unexpected staging callback")
			})
			return rejectedEngineeringValue(t, out, err)
		}},
		{"attach-upload", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.AttachUpload(ctx, e, uploadSpec.ID, upload.Operation, false)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"expired-uploads", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ExpiredUploads(ctx, e, 4)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"reserve-profile", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ReserveProfile(ctx, e, record)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"read-profile", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ReadProfile(ctx, e, profileSpec.ID, false)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"attach-profile", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.AttachProfile(ctx, e, profileSpec.ID, profile.Operation)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"profile-evidence", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ProfileEvidence(ctx, e, profileSpec.ID)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"profile-history", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ProfileHistory(ctx, e, profileSpec.Source, profileSpec.Context, profileSpec.Dataset, 4)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"register-dependency", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			return f.db.RegisterDependency(ctx, e, profileSpec.ID, dependency)
		}},
		{"dependency-health", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.DependencyHealth(ctx, e, dependency)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"admit-request", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.AdmitRequest(ctx, e, "rejected-repository-key", profile.Operation.Input, limits)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"read-request", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ReadRequest(ctx, e, profile.Operation.ID)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"resume-request", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ResumeRequest(ctx, e, profile.Operation.ID, limits)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"cancel-request", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.CancelRequest(ctx, e, profile.Operation.ID)
			return rejectedEngineeringValue(t, out, err)
		}},
		{"claim-request", func(ctx context.Context, t *testing.T, e identity.Envelope) error {
			out, err := f.db.ClaimRequest(ctx, e, profile.Operation.ID, "repository-owner", limits)
			return rejectedEngineeringValue(t, out, err)
		}},
	}
	metadata := support.Raw(t, f.dsn)
	before := count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`)
	lookups := f.lookups.Load()
	restricted := f.token.envelope(t, f.e.Tenant(), f.e.User(), "sources.read", "sources.upload", "sources.erase", "engineering.read", "engineering.profile", "jobs.read", "jobs.cancel")
	if restricted.Session() != f.e.Session() {
		t.Fatal("restricted fixture changed private session instead of resource reach")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	for _, scenario := range []struct {
		name string
		ctx  context.Context
		e    identity.Envelope
	}{
		{"unverified", ctx, identity.Envelope{}},
		{"missing-resource-reach", ctx, restricted},
		{"cancelled-before-start", cancelled, f.e},
	} {
		for _, call := range calls {
			t.Run(scenario.name+"/"+call.name, func(t *testing.T) {
				if err := call.run(scenario.ctx, t, scenario.e); err == nil {
					t.Fatal("rejected repository boundary succeeded")
				}
			})
		}
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`) != before || f.lookups.Load() != lookups {
		t.Fatal("rejected repository calls changed audit state or contacted the warehouse")
	}
	// Close the actual pool, not a fake repository. All public methods must
	// return an error and a zero result rather than leaking retained metadata.
	f.db.Close()
	for _, call := range calls {
		t.Run("unavailable-store/"+call.name, func(t *testing.T) {
			if err := call.run(ctx, t, f.e); err == nil {
				t.Fatal("unavailable repository reported success")
			}
		})
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`) != before {
		t.Fatal("unavailable-store calls changed independent durable state")
	}
}

func TestEngineeringRepositoryClosedInputAndReservationLimits(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("reservation-boundary", "csv", raw, columns)
	loaded := f.load(t, spec, raw)
	profileSpec := f.profileSpec(t, *loaded.Upload.Source, "reservation-profile", []string{"id", "amount"}, "")
	profile := f.profile(t, profileSpec)
	record, err := f.db.ReadProfile(ctx, f.e, profileSpec.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	metadata := support.Raw(t, f.dsn)
	before := count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`)
	if _, err = f.db.ReserveProfile(ctx, f.e, record); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("caller supplied an already computed result at reservation", err)
	}
	record.Result = nil
	if !record.Valid() {
		t.Fatal("retained profile fixture invalid")
	}
	tampered := record
	tampered.Spec.Columns = []string{"amount"}
	if _, err = f.db.ReserveProfile(ctx, f.e, tampered); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("changed profile input bypassed its digest", err)
	}
	tampered.SpecHash = tampered.Digest()
	if _, err = f.db.ReserveProfile(ctx, f.e, tampered); !errors.Is(err, store.ErrConflict) {
		t.Fatal("profile identifier accepted a replacement manifest", err)
	}
	stale := record
	stale.Spec.ID, stale.Settings.MaxVersions, stale.Spec.Previous = "stale-profile-parent", 10, "unknown-previous"
	stale.SpecHash = stale.Digest()
	if _, err = f.db.ReserveProfile(ctx, f.e, stale); !errors.Is(err, store.ErrConflict) {
		t.Fatal("incorrect active parent admitted", err)
	}
	changed := spec
	changed.Name = "Different accepted meaning"
	if _, err = f.db.ReserveUpload(ctx, f.e, changed, f.values.Uploads); !errors.Is(err, store.ErrConflict) {
		t.Fatal("upload identifier accepted a new immutable manifest", err)
	}
	bad := spec
	bad.Format = "executable"
	if _, err = f.db.ReserveUpload(ctx, f.e, bad, f.values.Uploads); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("unsupported format admitted to storage", err)
	}
	if _, err = f.db.StageUpload(ctx, f.e, spec.ID, nil); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("nil staging effect accepted", err)
	}
	if _, err = f.db.StageUpload(ctx, f.e, spec.ID, func(context.Context, engineering.UploadRecord) error {
		t.Error("active data was restaged")
		return nil
	}); !errors.Is(err, engineering.ErrState) {
		t.Fatal("active upload was not immutable to staging", err)
	}
	for _, limit := range []int{0, 33} {
		if _, err = f.db.ExpiredUploads(ctx, f.e, limit); !errors.Is(err, engineering.ErrInvalid) {
			t.Fatal("unbounded upload maintenance selection accepted", limit, err)
		}
		if _, err = f.db.ProfileHistory(ctx, f.e, profileSpec.Source, profileSpec.Context, profileSpec.Dataset, limit); !errors.Is(err, engineering.ErrInvalid) {
			t.Fatal("unbounded profile history accepted", limit, err)
		}
	}
	for _, id := range []string{"", "../outside"} {
		if _, err = f.db.ReadProfile(ctx, f.e, id, false); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal("invalid profile identifier was queried", err)
		}
		if _, err = f.db.ReadRequest(ctx, f.e, id); !errors.Is(err, access.ErrUnauthenticated) {
			t.Fatal("invalid operation identifier was queried", err)
		}
	}
	if err = f.db.RegisterDependency(ctx, f.e, profileSpec.ID, engineering.Dependency{}); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("empty dependency accepted", err)
	}
	if _, err = f.db.DependencyHealth(ctx, f.e, engineering.Dependency{}); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("empty dependency selected health evidence", err)
	}
	if _, err = f.db.AdmitRequest(ctx, f.e, "bad key", profile.Operation.Input, jobs.Defaults()); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("invalid idempotency key accepted", err)
	}
	if _, err = f.db.AdmitRequest(ctx, f.e, "bad-input-key", jobs.RequestInput{}, jobs.Defaults()); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("empty accepted operation manifest", err)
	}
	if _, err = f.db.ClaimRequest(ctx, f.e, profile.Operation.ID, "bad owner", jobs.Defaults()); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("invalid owner claimed the ledger", err)
	}
	if _, err = f.db.AttachProfile(ctx, f.e, profileSpec.ID, loaded.Operation); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("upload authority could attach a profile", err)
	}
	if _, err = f.db.AttachUpload(ctx, f.e, spec.ID, profile.Operation, false); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("profile authority could attach an upload", err)
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`) != before || count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.uploads`) != 1 {
		t.Fatal("rejected manifests changed persisted accepted work")
	}
	// Source collisions are checked against the real source registry, not just
	// the uploads table; ordinary warehouse sources are never adopted as uploads.
	warehouse := f.create(t, "existing-warehouse-source")
	collision := engineeringSpec(warehouse.ID, "csv", raw, columns)
	if _, err = f.db.ReserveUpload(ctx, f.e, collision, f.values.Uploads); !errors.Is(err, store.ErrConflict) {
		t.Fatal("ordinary source was adopted by a managed upload", err)
	}
	fresh := engineeringSpec("over-upload-cap", "csv", raw, columns)
	limits := f.values.Uploads
	limits.MaxPerTenant = 1
	if _, err = f.db.ReserveUpload(ctx, f.e, fresh, limits); !errors.Is(err, engineering.ErrLimit) {
		t.Fatal("tenant upload count cap bypassed", err)
	}
	limits = f.values.Uploads
	limits.MaxTenantBytes = limits.MaxExpandedBytes
	if _, err = f.db.ReserveUpload(ctx, f.e, fresh, limits); !errors.Is(err, engineering.ErrLimit) {
		t.Fatal("decoded reservation exceeded remaining tenant capacity", err)
	}
}

type profileReceiptBoundary struct {
	*postgres.DB
	t *testing.T
}

func (r *profileReceiptBoundary) CheckpointProfile(ctx context.Context, inv jobs.Invocation, record engineering.ProfileRecord, profile engineering.Profile) error {
	r.t.Helper()
	if err := r.DB.CheckpointProfile(ctx, inv, record, engineering.Profile{}); !errors.Is(err, engineering.ErrInvalid) {
		r.t.Fatal("empty profile receipt was accepted", err)
	}
	bad := profile
	bad.ReadOperation = "not-the-accepted-read"
	if err := r.DB.CheckpointProfile(ctx, inv, record, bad); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("checkpoint adopted another operation", err)
	}
	bad = profile
	bad.ReadAttempt = strings.Repeat("f", 64)
	if err := r.DB.CheckpointProfile(ctx, inv, record, bad); !errors.Is(err, store.ErrNotFound) {
		r.t.Fatal("checkpoint manufactured a native receipt", err)
	}
	bad = profile
	bad.Sampling.Bytes++
	if !bad.Valid(record) {
		r.t.Fatal("native-count mismatch fixture failed structural validation")
	}
	if err := r.DB.CheckpointProfile(ctx, inv, record, bad); !errors.Is(err, engineering.ErrState) {
		r.t.Fatal("checkpoint invented returned native byte counts", err)
	}
	if err := r.DB.CheckpointProfile(ctx, inv, record, profile); err != nil {
		return err
	}
	if err := r.DB.CheckpointProfile(ctx, inv, record, profile); err != nil {
		r.t.Fatal("identical checkpoint retry was not idempotent", err)
	}
	bad = profile
	bad.ObservedAt = bad.ObservedAt.Add(time.Microsecond)
	if err := r.DB.CheckpointProfile(ctx, inv, record, bad); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("retained deterministic checkpoint was replaced", err)
	}
	return nil
}

func (r *profileReceiptBoundary) PublishProfile(ctx context.Context, inv jobs.Invocation, record engineering.ProfileRecord, profile engineering.Profile) error {
	r.t.Helper()
	if err := r.DB.PublishProfile(ctx, inv, record, engineering.Profile{}); !errors.Is(err, engineering.ErrInvalid) {
		r.t.Fatal("empty profile became an active publication", err)
	}
	bad := profile
	bad.ObservedAt = bad.ObservedAt.Add(time.Microsecond)
	if err := r.DB.PublishProfile(ctx, inv, record, bad); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("publication changed checkpoint evidence", err)
	}
	bad = profile
	bad.Summary.Status = "available"
	if err := r.DB.PublishProfile(ctx, inv, record, bad); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("unadmitted model summary was published", err)
	}
	if err := r.StartProfileRead(ctx, inv, record, "replace-retained-read", time.Now().Add(time.Second)); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("retained checkpoint was reset to sampling", err)
	}
	for _, until := range []time.Time{{}, time.Now().Add(-time.Second), time.Now().Add(2 * time.Minute)} {
		if err := r.StartProfileRead(ctx, inv, record, "invalid-deadline", until); !errors.Is(err, engineering.ErrInvalid) {
			r.t.Fatal("unbounded or expired native dispatch deadline", err)
		}
	}
	if err := r.StartProfileRead(ctx, inv, record, "bad operation", time.Now().Add(time.Second)); !errors.Is(err, engineering.ErrInvalid) {
		r.t.Fatal("malformed native operation admitted", err)
	}
	if _, err := r.StartProfileSummary(ctx, jobs.Invocation{}, record); !errors.Is(err, jobs.ErrAuthority) {
		r.t.Fatal("forged invocation admitted optional paid work", err)
	}
	wrongActor := record
	wrongActor.Actor = "unrelated-actor"
	if _, err := r.StartProfileSummary(ctx, inv, wrongActor); !errors.Is(err, store.ErrNotFound) {
		r.t.Fatal("caller modified retained profile ownership", err)
	}
	if _, err := r.PulseRequest(ctx, inv, true, 0); !errors.Is(err, jobs.ErrInvalid) {
		r.t.Fatal("unbounded lease renewal accepted", err)
	}
	if _, err := r.PulseRequest(ctx, jobs.Invocation{}, false, time.Second); !errors.Is(err, jobs.ErrAuthority) {
		r.t.Fatal("forged invocation inspected a live lease", err)
	}
	if err := r.FailRequest(ctx, jobs.Invocation{}, "attempt_failed", false, time.Second); !errors.Is(err, jobs.ErrInvalid) {
		r.t.Fatal("forged invocation sealed another owner's attempt", err)
	}
	if err := r.FailRequest(ctx, inv, "PRIVATE_DRIVER_TEXT", false, time.Second); !errors.Is(err, jobs.ErrInvalid) {
		r.t.Fatal("arbitrary error text entered the retained ledger", err)
	}
	if err := r.FailRequest(ctx, inv, "attempt_failed", false, -time.Second); !errors.Is(err, jobs.ErrInvalid) {
		r.t.Fatal("invalid retry delay changed a live operation", err)
	}
	if err := r.DB.PublishProfile(ctx, inv, record, profile); err != nil {
		return err
	}
	if err := r.DB.PublishProfile(ctx, inv, record, profile); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("completed immutable publication was mutated", err)
	}
	state, err := r.PulseRequest(ctx, inv, true, time.Second)
	if err != nil || state != "succeeded" {
		r.t.Fatal("late observer did not preserve confirmed completion", err, state)
	}
	return nil
}

func TestProfileStoreBindsCheckpointsToActualNativeReceipts(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	source := f.create(t, "receipt-boundary-source")
	spec := f.profileSpec(t, source, "receipt-boundary-profile", []string{"id", "amount"}, "")
	repo := &profileReceiptBoundary{DB: f.db, t: t}
	service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	run, err := service.Build(context.Background(), f.e, spec, "receipt-boundary-operation", false)
	if err != nil || run.Profile.State != "complete" || run.Operation.State != "succeeded" {
		t.Fatal("valid native evidence failed after rejected mutations", err, run)
	}
	metadata := support.Raw(t, f.dsn)
	if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE action='profile.checkpoint'`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE action='profile.published'`) != 1 {
		t.Fatal("rejected or idempotent mutations repeated native work or publication")
	}
}

type uploadReceiptBoundary struct {
	*postgres.DB
	t *testing.T
}

func (r *uploadReceiptBoundary) ActivateUpload(ctx context.Context, inv jobs.Invocation, record engineering.UploadRecord, receipt engineering.WorkspaceReceipt, source sources.Record) error {
	r.t.Helper()
	if err := r.DB.ActivateUpload(ctx, jobs.Invocation{}, record, receipt, source); !errors.Is(err, jobs.ErrAuthority) {
		r.t.Fatal("forged activation invocation", err)
	}
	badReceipt := receipt
	badReceipt.TableOID = 0
	if err := r.DB.ActivateUpload(ctx, inv, record, badReceipt, source); !errors.Is(err, engineering.ErrOwnership) {
		r.t.Fatal("receipt without observed object identity accepted", err)
	}
	for _, mutate := range []func(*sources.Record){
		func(s *sources.Record) { s.Binding.Relations[0].Name = "unrelated_table" },
		func(s *sources.Record) { s.Binding.Relations[0].Columns = s.Binding.Relations[0].Columns[:1] },
		func(s *sources.Record) { s.Binding.Relations[0].Columns[0].Name = "unrelated_column" },
	} {
		bad := source
		bad.Binding = source.Binding.Clone()
		mutate(&bad)
		if err := r.DB.ActivateUpload(ctx, inv, record, receipt, bad); !errors.Is(err, engineering.ErrOwnership) {
			r.t.Fatal("activation accepted another table or schema", err)
		}
	}
	if err := r.DB.ActivateUpload(ctx, inv, record, receipt, source); err != nil {
		return err
	}
	if err := r.DB.ActivateUpload(ctx, inv, record, receipt, source); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("completed upload could be reactivated by an old invocation", err)
	}
	return nil
}

func (r *uploadReceiptBoundary) FinishUploadErasure(ctx context.Context, inv jobs.Invocation, record engineering.UploadRecord) error {
	r.t.Helper()
	if err := r.DB.FinishUploadErasure(ctx, jobs.Invocation{}, record); !errors.Is(err, jobs.ErrAuthority) {
		r.t.Fatal("forged erasure invocation", err)
	}
	if err := r.DB.FinishUploadErasure(ctx, inv, record); err != nil {
		return err
	}
	if err := r.DB.FinishUploadErasure(ctx, inv, record); !errors.Is(err, store.ErrConflict) {
		r.t.Fatal("completed erasure was rewritten by an old invocation", err)
	}
	return nil
}

func TestUploadStoreBindsActivationAndErasureToOwnedReceipts(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	spec := engineeringSpec("owned-receipt-boundary", "csv", raw, columns)
	f.stage(t, spec, raw)
	repo := &uploadReceiptBoundary{DB: f.db, t: t}
	service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	loaded, err := service.LoadUpload(ctx, f.e, spec.ID, "owned-receipt-load", false)
	if err != nil || loaded.Upload.State != "active" || loaded.Upload.Source == nil {
		t.Fatal("valid activation failed after ownership negatives", err, loaded)
	}
	f.readUpload(t, *loaded.Upload.Source)
	erased, err := service.EraseUpload(ctx, f.e, spec.ID, "owned-receipt-erase", false)
	if err != nil || erased.Upload.State != "erased" || erased.Operation.State != "succeeded" {
		t.Fatal("valid erasure failed after ownership negatives", err, erased)
	}
	metadata := support.Raw(t, f.dsn)
	if count(t, metadata, `SELECT count(*) FROM chartworks.sources WHERE NOT deleted`) != 0 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE action='upload.activated'`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events WHERE action='upload.erased'`) != 1 {
		t.Fatal("rejected receipts mutated active source or audit evidence")
	}
}

// Exercise the valid minimum against real published versions. An invalid
// configuration must not masquerade as evidence that a storage cap works.
func TestProfileVersionCapCountsRealPublications(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.MaxVersions = 2 }, nil)
	ctx := context.Background()
	source := f.create(t, "version-cap-source")
	first := f.profileSpec(t, source, "version-cap-first", []string{"id", "amount"}, "")
	f.profile(t, first)
	second := first
	second.ID, second.Previous = "version-cap-second", first.ID
	f.profile(t, second)
	metadata := support.Raw(t, f.dsn)
	before := count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`)
	third := second
	third.ID, third.Previous = "version-cap-third", second.ID
	if _, err := f.service.Build(ctx, f.e, third, "version-cap-third-build", false); !errors.Is(err, engineering.ErrLimit) {
		t.Fatal("third version was not rejected at the valid two-version cap", err)
	}
	if count(t, metadata, `SELECT count(*) FROM chartworks.profile_versions`) != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.audit_events`) != before {
		t.Fatal("rejected third version inserted metadata, sampled the source or emitted an audit")
	}
	evidence, err := f.service.Evidence(ctx, f.e, second.ID)
	if err != nil || !evidence.Active {
		t.Fatal("version cap changed the last published head", err)
	}
}
