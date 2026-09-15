package acceptance

import (
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
)

// Queue capacity needs a fresh deployment with deliberately small bounds. The
// broad reporting fixture first profiles source data with default bounds, so it
// cannot legitimately be reconfigured afterward. Build a real text-report
// domain without that unrelated profiling operation; never rewrite the shared
// queue fingerprint, accepted operations or admission checks to fit a test.
func newPhase30QueueFixture(t *testing.T, global, tenant int) *phase30Fixture {
	t.Helper()
	limits := jobs.Defaults()
	limits.MaxPending, limits.MaxPendingPerTenant = global, tenant
	base := newEngineeringFixture(t, func(v *config.Values) {
		v.Jobs.MaxPending, v.Jobs.MaxPendingPerTenant = global, tenant
	}, nil)
	if err := base.db.ConfigureQueue(t.Context(), limits); err != nil {
		t.Fatal("configure bounded deployment before its first operation", err)
	}
	model := newGatewayFixture(t, nil)
	f := &phase17Fixture{f: base, model: model}
	index, err := vindex.New(base.db)
	if err != nil {
		t.Fatal(err)
	}
	published, err := topics.New(base.db, base.s, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	reportingLimits := config.DefaultReporting()
	blocks, err := reporting.New(base.db, published, base.s, base.validator, base.executor, nil, reportingLimits)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(base.db, limits)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewRuns(blocks, base.db, runner, nil, "policy-v1", reportingLimits.Execution)
	if err != nil {
		t.Fatal(err)
	}
	documents, err := reporting.NewDocuments(base.db, blocks, nil, reportingLimits)
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(documents, base.db, runs, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	d := &phase29ExecutionFixture{
		f: f, blocks: blocks, runs: runs, documents: documents, compositions: compositions, limits: reportingLimits,
		blockAuthor: phase27Actor(t, f, base.e.User(), phase27Scopes(base.e.Tenant())),
		author:      phase27Actor(t, f, base.e.User(), phase29AuthorScopes(base.e.Tenant())),
		execute:     phase27Actor(t, f, base.e.User(), phase29RuntimeScopes(base.e.Tenant())),
	}
	return newPhase30FixtureWithDomain(t, d, limits)
}
