package acceptance

import (
	"context"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
)

func TestProfilePlannerLimitPreservesExecutorFailureCode(t *testing.T) {
	f := newEngineeringFixture(t, func(v *config.Values) {
		v.Profiling.PlannerCostCeiling = 0.001
	}, nil)
	source := f.create(t, "typed-profile-limit")
	spec := f.profileSpec(t, source, "typed-profile-limit-version", []string{"id"}, "")
	ctx := context.Background()
	run, err := f.service.Build(ctx, f.e, spec, "typed-profile-limit-key", false)
	if err != nil || run.Profile.State == "complete" || run.Profile.Profile != nil || run.Code != "limit_exceeded" {
		t.Fatal("a planner refusal was misreported as a generic outage", err, run)
	}
	record, err := f.db.ReadProfile(ctx, f.e, spec.ID, false)
	if err != nil || record.LastReadOperation == "" || record.Result != nil {
		t.Fatal("limit refusal lost read intent or published evidence", err)
	}
	attempt, err := f.executor.ByOperation(ctx, f.e, record.LastReadOperation)
	if err != nil || attempt.Status != "failed" || attempt.Code != run.Code || attempt.Finished == nil || attempt.RemoteState != "stopped" && attempt.RemoteState != "not_issued" {
		t.Fatal("public failure does not match the common executor's receipt", err, attempt)
	}
}
