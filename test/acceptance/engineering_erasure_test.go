package acceptance

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
)

type interruptedErasurePublication struct {
	*postgres.DB
	interrupt atomic.Bool
}

func (r *interruptedErasurePublication) FinishUploadErasure(ctx context.Context, invocation jobs.Invocation, upload engineering.UploadRecord) error {
	if r.interrupt.Swap(false) {
		return store.ErrUnavailable
	}
	return r.DB.FinishUploadErasure(ctx, invocation, upload)
}

func TestUploadErasureRemovesDerivedProfileValuesAcrossActors(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) {
		v.Profiling.Policies = []config.ProfilePolicy{{ID: "retained-ranges", Tenant: "source-a", Source: "erase-evidence", RangeColumns: []string{"id", "amount", "event_time"}}}
	}, nil)
	ctx := context.Background()
	raw, columns := engineeringCSV()
	victim := f.load(t, engineeringSpec("erase-evidence", "csv", raw, columns), raw)
	survivor := f.load(t, engineeringSpec("retain-evidence", "csv", raw, columns), raw)
	spec := f.profileSpec(t, *victim.Upload.Source, "erase-profile-owner", []string{"id", "amount", "event_time"}, "event_time")
	spec.Policy = "retained-ranges"
	profile := f.profile(t, spec).Profile.Profile
	retainedRange := false
	for _, column := range profile.Columns {
		retainedRange = retainedRange || column.Minimum != nil || column.Maximum != nil
	}
	if !retainedRange {
		t.Fatal("fixture never retained the values whose erasure is under test")
	}
	other := f.actor(t, f.e.Tenant(), "second-profiler")
	otherSpec := spec
	otherSpec.ID = "erase-profile-other"
	otherRun, err := f.service.Build(ctx, other, otherSpec, "other-profile-operation", false)
	if err != nil || otherRun.Profile.State != "complete" || otherRun.Profile.Profile == nil {
		t.Fatal("second actor's real profile did not complete", err, otherRun)
	}
	survivorSpec := f.profileSpec(t, *survivor.Upload.Source, "surviving-profile", []string{"id", "amount"}, "")
	survivorProfile := f.profile(t, survivorSpec).Profile.Profile
	before := survivorProfile.DeterministicHash()
	dependency := engineering.Dependency{Kind: "topic", ID: "profile-evidence-consumer", Version: strings.Repeat("a", 64), Source: spec.Source, Context: spec.Context, Dataset: spec.Dataset, Columns: []string{"amount"}}
	if err = f.service.RegisterDependency(ctx, f.e, spec.ID, dependency); err != nil {
		t.Fatal("real dependency registration", err)
	}
	pending, err := f.db.AdmitRequest(ctx, f.e, "pending-derived-profile", jobs.RequestInput{Kind: "profile.build", Target: spec.Source, Context: spec.Context, InputHash: readexec.Hash("synthetic-pending-profile")}, jobs.Defaults())
	if err != nil || pending.State != "pending" {
		t.Fatal("pending shared-ledger profile fixture", err, pending)
	}
	metadata := support.Raw(t, f.dsn)
	var profiles, dependencies int
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.profile_versions WHERE tenant_id=$1 AND source_id=$2 AND result IS NOT NULL`, f.e.Tenant(), spec.Source).Scan(&profiles); err != nil || profiles != 2 {
		t.Fatal("private evidence fixtures were not persisted", err, profiles)
	}
	if err = metadata.QueryRow(ctx, `SELECT count(*) FROM chartworks.profile_dependencies WHERE tenant_id=$1 AND source_id=$2`, f.e.Tenant(), spec.Source).Scan(&dependencies); err != nil || dependencies != 1 {
		t.Fatal("dependency fixture was not persisted", err, dependencies)
	}
	// Interrupt after workspace removal but before the metadata completion. The
	// source tombstone must deny evidence reads; the receipt must not say erased.
	repo := &interruptedErasurePublication{DB: f.db}
	repo.interrupt.Store(true)
	service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	first, err := service.EraseUpload(ctx, f.e, spec.Source, "erase-derived-evidence", false)
	service.Close()
	if err != nil || first.Upload.State != "deleting" || first.Operation.State != "retry" {
		t.Fatal("interrupted erasure falsely completed", err, first)
	}
	if _, err = f.service.Evidence(ctx, other, otherSpec.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("tombstoned source still exposed another actor's retained values", err)
	}
	resumed, err := f.service.EraseUpload(ctx, f.e, spec.Source, "erase-derived-evidence", true)
	if err != nil || resumed.Upload.State != "erased" || resumed.Operation.ID != first.Operation.ID || resumed.Operation.Attempts != 2 {
		t.Fatal("same-operation erasure did not reconcile", err, resumed)
	}
	var erased, remaining int
	if err = metadata.QueryRow(ctx, `SELECT count(*) FILTER(WHERE state='erased'),count(*) FILTER(WHERE result IS NOT NULL OR deterministic_hash IS NOT NULL OR changes<>'[]'::jsonb OR last_read_operation IS NOT NULL OR last_read_deadline IS NOT NULL) FROM chartworks.profile_versions WHERE tenant_id=$1 AND source_id=$2`, f.e.Tenant(), spec.Source).Scan(&erased, &remaining); err != nil || erased != 2 || remaining != 0 {
		t.Fatal("derived ranges or checkpoint values survived completed erasure", err, erased, remaining)
	}
	for _, query := range []string{
		`SELECT count(*) FROM chartworks.profile_heads WHERE tenant_id=$1 AND source_id=$2`,
		`SELECT count(*) FROM chartworks.profile_dependencies WHERE tenant_id=$1 AND source_id=$2`,
		`SELECT count(*) FROM chartworks.profile_health_events h JOIN chartworks.profile_versions p ON (p.tenant_id,p.profile_id)=(h.tenant_id,h.profile_id) WHERE p.tenant_id=$1 AND p.source_id=$2`,
	} {
		if err = metadata.QueryRow(ctx, query, f.e.Tenant(), spec.Source).Scan(&remaining); err != nil || remaining != 0 {
			t.Fatal("derived profile reference survived erasure", err, remaining)
		}
	}
	pending, err = f.db.ReadRequest(ctx, f.e, pending.ID)
	if err != nil || pending.State != "cancelled" {
		t.Fatal("pending derived work survived erasure", err, pending)
	}
	if _, err = f.service.InspectProfile(ctx, f.e, spec.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("erased profile remained readable", err)
	}
	if _, err = f.service.Build(ctx, other, otherSpec, "late-profile-retry", true); err == nil {
		t.Fatal("profile retry resurrected an erased source")
	}
	evidence, err := f.service.Evidence(ctx, f.e, survivorSpec.ID)
	if err != nil || !evidence.Active || evidence.Profile.DeterministicHash() != before {
		t.Fatal("erasure changed an unrelated source's profile", err)
	}
	f.readUpload(t, *survivor.Upload.Source)
	replay, err := f.service.EraseUpload(ctx, f.e, spec.Source, "erase-derived-evidence", true)
	if err != nil || replay.Upload.State != "erased" || replay.Code != "already_complete" {
		t.Fatal("completed erasure was not idempotent", err, replay)
	}
}
