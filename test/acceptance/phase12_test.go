package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func TestPhase12(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		model := newGatewayFixture(t, nil)
		f := newEngineeringFixture(t, func(v *config.Values) {
			v.Features.Gateway = true
			v.Gateway = model.cfg
			v.Profiling.Summaries = true
			v.Profiling.Policies = []config.ProfilePolicy{{ID: "permitted-ranges", Tenant: "source-a", Source: "sensitive", RangeColumns: []string{"amount", "created_at"}}}
		}, model.engine)
		ctx := context.Background()
		if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET name=CASE WHEN id=1 THEN 'PRIVATE_PERSON_CANARY@example.test' ELSE '' END`); err != nil {
			t.Fatal(err)
		}
		source := f.create(t, "sensitive")
		spec := f.profileSpec(t, source, "golden-profile", []string{"id", "amount", "active", "name", "payload", "created_at"}, "created_at")
		spec.Policy, spec.SkipLLM = "permitted-ranges", false
		p := f.profile(t, spec).Profile.Profile
		if len(p.Schema) != 10 || len(p.Columns) != 6 || p.Sampling.Rows != 2 || !p.Sampling.Complete || p.Sampling.ScanBounded || p.Freshness.State != "stale" || p.Freshness.Latest == nil {
			t.Fatal("golden structural/sample/freshness evidence diverged", p)
		}
		want := map[string]struct {
			native, category string
			nulls, distinct int
		}{
			"id": {"int4", "numeric", 0, 2},
			"amount": {"numeric", "numeric", 0, 2},
			"active": {"bool", "boolean", 0, 2},
			"name": {"text", "text", 0, 2},
			"payload": {"bytea", "binary", 1, 1},
			"created_at": {"timestamptz", "temporal", 0, 2},
		}
		for _, c := range p.Columns {
			expected, ok := want[c.Name]
			if !ok || c.NativeType != expected.native || c.Category != expected.category || c.Nulls != expected.nulls || c.Distinct != expected.distinct || c.Observed != 2 || !c.DistinctExact {
				t.Fatal("golden column evidence diverged", c)
			}
			switch c.Name {
			case "amount":
				if c.Minimum == nil || c.Maximum == nil || *c.Minimum != "5.500" || *c.Maximum != "9007199254740993.125" {
					t.Fatal("permitted exact decimal range changed", c)
				}
			case "created_at":
				if c.Minimum == nil || c.Maximum == nil {
					t.Fatal("permitted temporal range absent", c)
				}
			case "name":
				if c.Minimum != nil || c.Maximum != nil || c.Families["email_like"] != 1 || c.Families["empty"] != 1 {
					t.Fatal("sensitive values retained or family classification changed", c)
				}
			default:
				if c.Minimum != nil || c.Maximum != nil {
					t.Fatal("range escaped the explicit policy", c.Name)
				}
			}
		}
		findings := map[string]int{}
		for _, finding := range p.Findings {
			if finding.Basis != "sample" {
				t.Fatal("quality observation claimed population certainty", finding)
			}
			findings[finding.Code+":"+finding.Column] = finding.Count
		}
		if findings["contains_nulls:payload"] != 1 || findings["empty_text:name"] != 1 || findings["constant_observed_value:payload"] != 1 {
			t.Fatal("fixed quality rules changed", findings)
		}
		if p.Summary.Status != "available" || p.Summary.Text != "fixture result" || model.requests.Load() != 1 || len(p.Summary.Receipt.Calls) != 1 {
			t.Fatal("optional summary did not use the existing gateway", p.Summary)
		}
		usage := p.Summary.Receipt.Calls[0]
		if usage.InputTokens == nil || *usage.InputTokens != 10 || usage.OutputTokens == nil || *usage.OutputTokens != 5 || usage.CostUSD != nil || usage.DurationMS < 0 {
			t.Fatal("model usage was fabricated or discarded", usage)
		}
		model.mu.Lock()
		payloads := append([]string(nil), model.requestBodies...)
		model.mu.Unlock()
		if len(payloads) != 1 {
			t.Fatal("missing actual provider-input capture")
		}
		for _, forbidden := range []string{"PRIVATE_PERSON_CANARY", "PRIVATE_COLUMN_CANARY", "9007199254740993", "2026-01-02", "created_at", "analytics", "sensitive", "source-a", "operator", "test-session"} {
			if strings.Contains(payloads[0], forbidden) {
				t.Fatal("private values or identity reached the provider", forbidden)
			}
		}
		wire, err := json.Marshal(p)
		if err != nil || strings.Contains(string(wire), "PRIVATE_PERSON_CANARY") || strings.Contains(string(wire), "PRIVATE_COLUMN_CANARY") {
			t.Fatal("raw samples retained in evidence", err)
		}
	})
	t.Run("AC02", func(t *testing.T) {
		f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.SampleRows = 1 }, nil)
		source := f.create(t, "bounded-profile-source")
		spec := f.profileSpec(t, source, "one-row", []string{"id", "created_at"}, "created_at")
		p := f.profile(t, spec).Profile.Profile
		if p.Sampling.Rows != 1 || p.Sampling.Complete || p.Sampling.Truncation == "" || p.Sampling.RowCeiling != 1 || p.Sampling.Bytes > p.Sampling.ByteCeiling || p.Sampling.ScanBounded || p.Cost.ScannedBytes != nil || p.Freshness.State != "unknown" || p.Freshness.Reason != "partial_sample" || p.ExecutionNS <= 0 || p.ReadAttempt == "" {
			t.Fatal("bounded prefix misrepresented as complete or scan-bounded", p)
		}
		t.Run("byte-bound", func(t *testing.T) {
			f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.SampleBytes = 1024 }, nil)
			if _, err := f.admin.Exec(context.Background(), `UPDATE analytics.sales SET name=repeat('x',900)`); err != nil {
				t.Fatal(err)
			}
			source := f.create(t, "byte-bound")
			p := f.profile(t, f.profileSpec(t, source, "byte-profile", []string{"id", "name"}, "")).Profile.Profile
			if p.Sampling.Complete || p.Sampling.Bytes > 1024 || p.Sampling.ByteCeiling != 1024 || p.Sampling.Rows >= 2 {
				t.Fatal("profile exceeded its independent result-byte bound", p.Sampling)
			}
		})
		t.Run("planner-ceiling", func(t *testing.T) {
			f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.PlannerCostCeiling = 0.001 }, nil)
			source := f.create(t, "cost-bound")
			spec := f.profileSpec(t, source, "cost-profile", []string{"id"}, "")
			run, err := f.service.Build(context.Background(), f.e, spec, "cost-key", false)
			if err != nil || run.Profile.State == "complete" || run.Profile.Profile != nil || run.Code == "" {
				t.Fatal("planner ceiling did not refuse physical execution", err, run)
			}
			metadata := support.Raw(t, f.dsn)
			if count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts WHERE remote_state NOT IN ('not_issued','stopped')`) != 0 {
				t.Fatal("cost refusal left unaccounted remote work")
			}
		})
		t.Run("deadline", func(t *testing.T) {
			f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.Timeout = config.Duration(100 * time.Millisecond) }, nil)
			source := f.create(t, "time-bound")
			spec := f.profileSpec(t, source, "time-profile", []string{"id"}, "")
			lock := support.Raw(t, f.warehouse)
			tx, err := lock.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			if _, err = tx.Exec(context.Background(), `LOCK TABLE analytics.sales IN ACCESS EXCLUSIVE MODE`); err != nil {
				t.Fatal(err)
			}
			start := time.Now()
			run, err := f.service.Build(context.Background(), f.e, spec, "deadline-key", false)
			if time.Since(start) > 3*time.Second || run.Profile.State == "complete" || run.Profile.Profile != nil || err == nil && run.Code == "" {
				t.Fatal("profile deadline did not fence blocked source work", err, run)
			}
		})
	})
	t.Run("AC03", func(t *testing.T) {
		now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
		settings := config.DefaultProfiling()
		for _, tc := range []struct {
			name string
			latest *time.Time
			complete bool
			state, reason string
		}{
			{"no-event", nil, true, "unknown", "no_event_time"},
			{"fresh", profileTime(now.Add(-time.Hour)), true, "fresh", "configured_thresholds"},
			{"aging", profileTime(now.Add(-48*time.Hour)), true, "aging", "configured_thresholds"},
			{"stale", profileTime(now.Add(-8*24*time.Hour)), true, "stale", "configured_thresholds"},
			{"future", profileTime(now.Add(time.Hour)), true, "unknown", "future_event_time"},
			{"partial", profileTime(now.Add(-time.Hour)), false, "unknown", "partial_sample"},
		} {
			got := engineering.FreshnessAt(now, tc.latest, tc.complete, false, settings)
			if got.State != tc.state || got.Reason != tc.reason || got.Latest != nil {
				t.Fatal("freshness rule or value minimization changed", tc.name, got)
			}
		}
		f := newEngineeringFixture(t, nil, nil)
		raw, columns := engineeringCSV()
		loaded := f.load(t, engineeringSpec("timezone-aware", "csv", raw, columns), raw)
		p := f.profile(t, f.profileSpec(t, *loaded.Upload.Source, "aware-profile", []string{"id", "event_time"}, "event_time")).Profile.Profile
		if p.Freshness.State != "stale" || p.Freshness.Latest != nil || p.Freshness.Basis != "complete_result_event_max" {
			t.Fatal("offset-aware source freshness lost provenance", p.Freshness)
		}
		// A wall-clock timestamp with no source timezone cannot be interpreted
		// as an instant merely because the server uses UTC.
		if _, err := f.admin.Exec(context.Background(), `ALTER TABLE analytics.sales ALTER COLUMN created_at TYPE timestamp USING created_at AT TIME ZONE 'America/Argentina/Buenos_Aires'`); err != nil {
			t.Fatal(err)
		}
		source := f.create(t, "timezone-unproven")
		unknown := f.profile(t, f.profileSpec(t, source, "naive-profile", []string{"id", "created_at"}, "created_at")).Profile.Profile
		if unknown.Freshness.State != "unknown" || unknown.Freshness.Reason != "timezone_unproven" || unknown.Freshness.Latest != nil {
			t.Fatal("timezone-free values were silently treated as UTC", unknown.Freshness)
		}
	})
	t.Run("AC04", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		ctx := context.Background()
		source := f.create(t, "drifting")
		first := f.profile(t, f.profileSpec(t, source, "before-drift", []string{"id", "amount"}, ""))
		dependency := engineering.Dependency{Kind: "report", ID: "report-reference", Version: readexec.Hash("approved-definition-v1"), Source: source.ID, Context: source.ContextID, Dataset: first.Profile.Dataset, Columns: []string{"id", "amount"}}
		if err := f.service.RegisterDependency(ctx, f.e, first.Profile.Version, dependency); err != nil {
			t.Fatal(err)
		}
		if _, err := f.admin.Exec(ctx, `ALTER TABLE analytics.sales ALTER COLUMN amount TYPE text USING amount::text`); err != nil {
			t.Fatal(err)
		}
		rotated, err := f.s.Rotate(ctx, f.e, source.ID, source.Revision)
		if err != nil {
			t.Fatal(err)
		}
		spec := f.profileSpec(t, rotated, "after-drift", []string{"id", "amount"}, "")
		spec.Previous = first.Profile.Version
		second := f.profile(t, spec)
		evidence, err := f.service.Evidence(ctx, f.e, second.Profile.Version)
		if err != nil || !evidence.Active || evidence.SemanticPublication || len(evidence.Changes) != 1 || evidence.Changes[0].Column != "amount" || evidence.Changes[0].Kind != "type_changed" {
			t.Fatal("structural drift not published atomically", err, evidence)
		}
		events, err := f.service.Health(ctx, f.e, dependency)
		if err != nil || len(events) != 1 || events[0].State != "needs_review" || events[0].Profile != second.Profile.Version || !reflect.DeepEqual(events[0].Dependency, dependency) {
			t.Fatal("consumer version was rewritten or drift not signalled", err, events)
		}
		for range 2 {
			if _, err = f.service.Build(ctx, f.e, spec, "after-drift-build", true); err != nil {
				t.Fatal(err)
			}
		}
		again, err := f.service.Health(ctx, f.e, dependency)
		if err != nil || !reflect.DeepEqual(events, again) {
			t.Fatal("profile replay duplicated invalidations", err)
		}
		late := dependency
		late.ID = "late-reference"
		if err = f.service.RegisterDependency(ctx, f.e, first.Profile.Version, late); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale registration silently missed an already-published drift", err)
		}
		old, err := f.service.Evidence(ctx, f.e, first.Profile.Version)
		if err != nil || old.Active || old.Profile.DeterministicHash() != first.Profile.Profile.DeterministicHash() {
			t.Fatal("old immutable evidence changed", err)
		}
		metadata := support.Raw(t, f.dsn)
		if count(t, metadata, `SELECT count(*) FROM chartworks.profile_health`) != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.profile_dependencies`) != 1 {
			t.Fatal("health or dependency publication was not idempotent")
		}
	})
	t.Run("AC05", func(t *testing.T) {
		t.Run("checkpoint-and-model-receipt", func(t *testing.T) {
			model := newGatewayFixture(t, nil)
			f := newEngineeringFixture(t, func(v *config.Values) { v.Profiling.Summaries = true }, model.engine)
			source := f.create(t, "resumable")
			spec := f.profileSpec(t, source, "resumable-profile", []string{"id", "name"}, "")
			spec.SkipLLM = false
			repo := &interruptedProfilePublication{DB: f.db, before: func(context.Context, jobs.Invocation) error { return store.ErrUnavailable }}
			repo.interrupt.Store(true)
			service, err := engineering.New(repo, f.s, f.validator, f.executor, model.engine, f.values, f.lookup)
			if err != nil {
				t.Fatal(err)
			}
			first, err := service.Build(context.Background(), f.e, spec, "stable-profile-key", false)
			service.Close()
			if err != nil || first.Profile.State == "complete" || first.Profile.Profile != nil || first.Operation.State != "retry" || model.requests.Load() != 1 {
				t.Fatal("interrupted checkpoint incorrectly published", err, first)
			}
			metadata := support.Raw(t, f.dsn)
			reads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			resumed, err := f.service.Build(context.Background(), f.e, spec, "stable-profile-key", true)
			if err != nil || resumed.Profile.State != "complete" || resumed.Profile.Profile == nil || resumed.Profile.Profile.Summary.Status != "unknown_previous_attempt" || resumed.Operation.ID != first.Operation.ID || resumed.Operation.Attempts != 2 {
				t.Fatal("checkpoint did not resume with truthful model uncertainty", err, resumed)
			}
			if reads != 1 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != reads || model.requests.Load() != 1 {
				t.Fatal("resume invisibly repeated source or paid model work")
			}
		})
		t.Run("late-cancellation", func(t *testing.T) {
			f := newEngineeringFixture(t, nil, nil)
			source := f.create(t, "cancel-profile-source")
			spec := f.profileSpec(t, source, "cancel-profile", []string{"id"}, "")
			repo := &interruptedProfilePublication{DB: f.db, before: func(_ context.Context, inv jobs.Invocation) error {
				_, err := f.service.CancelOperation(context.Background(), f.e, inv.Lease().Task.ID)
				return err
			}}
			repo.interrupt.Store(true)
			service, err := engineering.New(repo, f.s, f.validator, f.executor, nil, f.values, f.lookup)
			if err != nil {
				t.Fatal(err)
			}
			first, err := service.Build(context.Background(), f.e, spec, "cancel-profile-key", false)
			service.Close()
			if err != nil || first.Profile.State == "complete" || first.Profile.Profile != nil || first.Operation.State != "cancelled" {
				t.Fatal("late cancellation published a profile", err, first)
			}
			metadata := support.Raw(t, f.dsn)
			reads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			resumed, err := f.service.Build(context.Background(), f.e, spec, "cancel-profile-key", true)
			if err != nil || resumed.Profile.State != "complete" || resumed.Operation.Attempts != 2 || count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) != reads {
				t.Fatal("explicit cancellation recovery lost or re-executed its checkpoint", err, resumed)
			}
		})
	})
	t.Run("AC06", func(t *testing.T) {
		f := newEngineeringFixture(t, nil, nil)
		ctx := context.Background()
		source := f.create(t, "inspection-source")
		spec := f.profileSpec(t, source, "inspection-version", []string{"id", "amount", "name"}, "")
		wire, err := json.Marshal(spec)
		if err != nil {
			t.Fatal(err)
		}
		var input sdk.ProfileSpec
		if err = json.Unmarshal(wire, &input); err != nil {
			t.Fatal(err)
		}
		client := f.client(t)
		run, err := client.BuildProfile(ctx, input, "inspection-build", false)
		if err != nil || run.Profile.Profile == nil || run.Profile.Profile.Summary.Status != "skipped" || run.Profile.Profile.ExecutionNS <= 0 || run.Profile.Profile.ReadAttempt == "" || run.Profile.Profile.Cost.ScannedBytes != nil {
			t.Fatal("real SDK inspection consumer did not retain measured execution evidence", err, run)
		}
		f.setReadDSN("REMOVED_SOURCE_CREDENTIAL")
		before := f.lookups.Load()
		status, err := client.Profile(ctx, input.ID)
		if err != nil || status.Profile == nil || status.Profile.Version != input.ID {
			t.Fatal("retained progress required warehouse access", err)
		}
		evidence, err := client.ProfileEvidence(ctx, input.ID)
		if err != nil || !evidence.Active || evidence.SemanticPublication || evidence.AuthorityContext != source.ContextID || evidence.Profile.Version != input.ID {
			t.Fatal("retained evidence lost its authority partition", err, evidence)
		}
		history, err := client.ProfileHistory(ctx, input.Source, input.Context, input.Dataset, 10)
		if err != nil || len(history) != 1 || history[0].Profile == nil || f.lookups.Load() != before {
			t.Fatal("history re-executed source or lost its completed artifact", err)
		}
		operation, err := client.EngineeringOperation(ctx, run.Operation.ID)
		if err != nil || operation.ID != run.Operation.ID || operation.Input.Kind != "profile.build" {
			t.Fatal("shared operation inspection missing", err, operation)
		}
		for _, e := range []identity.Envelope{identity.Envelope{}, f.actor(t, "source-b", "operator"), f.actor(t, "source-a", "different-user"), f.token.envelope(t, "source-a", "operator", "engineering.read", "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:unrelated")} {
			if _, err = f.service.Evidence(ctx, e, input.ID); err == nil {
				t.Fatal("inspection crossed signed/private reach")
			}
			if _, err = f.service.History(ctx, e, input.Source, input.Context, input.Dataset, 1); err == nil {
				t.Fatal("history crossed signed/private reach")
			}
		}
		if f.lookups.Load() != before {
			t.Fatal("denied retained inspection consulted source credentials")
		}
	})
}

func profileTime(t time.Time) *time.Time { return &t }

// The interruption is injected at the publication boundary only. Source reads,
// checkpoints, gateway calls, leases and both database commits are real.
type interruptedProfilePublication struct {
	*postgres.DB
	interrupt atomic.Bool
	before func(context.Context, jobs.Invocation) error
}

func (r *interruptedProfilePublication) PublishProfile(ctx context.Context, inv jobs.Invocation, record engineering.ProfileRecord, profile engineering.Profile) error {
	if r.interrupt.Swap(false) {
		if err := r.before(ctx, inv); err != nil {
			return err
		}
	}
	return r.DB.PublishProfile(ctx, inv, record, profile)
}
