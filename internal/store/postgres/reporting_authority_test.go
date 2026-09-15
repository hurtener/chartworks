package postgres

import (
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

// A repository with no connection makes any attempted database use a hard
// failure. These entry guards must reject forged/empty ownership before I/O.
func TestReportingEntryGuardsRejectUnownedWork(t *testing.T) {
	db := &DB{}
	ctx := t.Context()
	inv := jobs.Invocation{}
	limits := jobs.Defaults()
	if out, err := db.SealScheduledFrozenRun(ctx, inv, reporting.PreparedRun{}); !errors.Is(err, jobs.ErrAuthority) || out.Manifest != nil {
		t.Fatal("unowned frozen seal", out, err)
	}
	if out, err := db.SealScheduledComposition(ctx, inv, reporting.PreparedComposition{}); !errors.Is(err, jobs.ErrAuthority) || out.Manifest.ID != "" {
		t.Fatal("unowned composition seal", out, err)
	}
	if err := db.ReserveReportingUsage(ctx, inv, jobs.ReportingCharge{}); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("empty charge", err)
	}
	if err := db.ReserveReportingUsage(ctx, inv, jobs.ReportingCharge{Queries: 1}); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("unowned charge", err)
	}
	if _, err := db.AdmitNestedRequest(ctx, inv, "child", jobs.RequestInput{Kind: "reporting.run"}, limits); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("unowned child", err)
	}
	if _, err := db.ClaimNestedRequest(ctx, inv, "child", "owner", limits); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("unowned claim", err)
	}
	if _, err := db.ClaimNestedRequest(ctx, inv, "../child", "owner", limits); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("malformed child", err)
	}
	if _, err := db.RetireSchedule(ctx, store.Scope{}, "schedule", 1); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("unscoped retirement", err)
	}
	if _, err := db.ScheduleHistory(ctx, store.Scope{}, "schedule", jobs.ScheduleHistoryRequest{Kind: "revisions", Limit: 1}); !errors.Is(err, jobs.ErrInvalid) {
		t.Fatal("unscoped history", err)
	}
	if a, b, err := reportingDispatchJSON(jobs.Job{}); !errors.Is(err, jobs.ErrInvalid) || a != nil || b != nil {
		t.Fatal("unowned dispatch serialized", err)
	}
	if err := completeReportingRequestTx(ctx, nil, inv); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("unowned publication", err)
	}
	if err := prepareNestedAdmissionTx(ctx, nil, &inv, jobs.RequestInput{Kind: "reporting.run"}); !errors.Is(err, jobs.ErrAuthority) {
		t.Fatal("unowned nested admission", err)
	}
}
