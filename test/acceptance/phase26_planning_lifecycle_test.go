package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// Planning uses the real source, validator, gateway and PostgreSQL. The runner
// is deliberately non-executable: these tests must never perform managed writes.
// TestPhase26 separately requires the pinned native executable and Linux sandbox.
func newPhase26PlanningFixture(t *testing.T) *phase26Fixture {
	t.Helper()
	provider := newGatewayFixture(t, nil)
	model := &phase26Model{Engine: provider.engine, fixture: provider}
	scratch := t.TempDir()
	runner := filepath.Join(scratch, "must-not-execute")
	artifact := []byte("Planning-only negative execution sentinel.\n")
	if err := os.WriteFile(runner, artifact, 0600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(artifact)
	base := newEngineeringFixture(t, func(v *config.Values) {
		v.Pipelines.Enabled = true
		v.Pipelines.RunnerPath = runner
		v.Pipelines.RunnerSHA256 = hex.EncodeToString(digest[:])
		v.Pipelines.TempDir = scratch
	}, model)
	service, err := engineering.NewPipelineService(base.db, base.s, base.validator, model, base.values, base.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	return phase26FixtureFromPipeline(t, model, &pipelineFixture{engineeringFixture: base, pipelines: service})
}

func TestProposalRejectionAndRevisionFence(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	ctx := context.Background()
	p := f.propose(t)
	review := engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "reject", Reason: "Revise the reviewed projection before execution."}
	rejected, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, review)
	if err != nil || rejected.State != "rejected" {
		t.Fatal("reject exact revision", rejected, err)
	}
	if _, err := f.client.ApplyEngineeringProposal(ctx, p.ID, phase26ApplyRequest(rejected)); err == nil {
		t.Fatal("rejected proposal was executable")
	}
	edit := engineering.AutopilotEditRequest{ExpectedVersion: rejected.Version, Definition: rejected.Material.Pipeline, Reason: "Resubmit the independently reviewed projection."}
	revised, err := f.client.EditEngineeringProposal(ctx, p.ID, edit)
	if err != nil || revised.State != "draft" || revised.Revision != rejected.Revision+1 || revised.Version <= rejected.Version {
		t.Fatal("rejection revision", revised, err)
	}
	if _, err := f.client.EditEngineeringProposal(ctx, p.ID, edit); err == nil {
		t.Fatal("stale edit overwrote current material")
	}
	review.Decision = "approve"
	if _, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, review); err == nil {
		t.Fatal("old review approved new revision")
	}
	approved := f.approve(t, revised)
	if approved.Revision != revised.Revision || approved.Digest != revised.Digest {
		t.Fatal("approval changed material")
	}
}

func TestProposalConcurrentEditsPreserveOneRevision(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	type result struct {
		proposal engineering.AutopilotProposal
		err      error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, reason := range []string{"First independently submitted correction.", "Second independently submitted correction."} {
		go func(reason string) {
			<-start
			proposal, err := f.client.EditEngineeringProposal(context.Background(), p.ID, engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Reason: reason})
			results <- result{proposal, err}
		}(reason)
	}
	close(start)
	var winner engineering.AutopilotProposal
	successes := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			successes++
			winner = result.proposal
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent CAS accepted %d edits", successes)
	}
	current, err := f.auto.Get(context.Background(), f.author, p.ID)
	if err != nil || current.Digest != winner.Digest || current.Version != p.Version+1 || current.Revision != p.Revision+1 || current.State != "draft" {
		t.Fatal("persisted winner is not the sole next revision", current, err)
	}
	f.approve(t, current)
}

func TestProposalAuditFailureRollsBackMaterialAndReview(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	ctx := context.Background()
	raw := support.Raw(t, f.dsn)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_proposal_event() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic audit write failure'; END; $$`)
	reject := func() {
		sql(t, raw, `CREATE TRIGGER reject_proposal_event BEFORE INSERT ON chartworks.engineering_proposal_events FOR EACH ROW EXECUTE FUNCTION chartworks.reject_proposal_event()`)
	}
	allow := func() { sql(t, raw, `DROP TRIGGER reject_proposal_event ON chartworks.engineering_proposal_events`) }
	reject()
	if _, err := f.client.ProposeEngineering(ctx, f.goal); err == nil {
		t.Fatal("proposal survived failed audit")
	}
	var count int
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.engineering_proposal_heads`).Scan(&count); err != nil || count != 0 {
		t.Fatal("partial proposal head", count, err)
	}
	allow()
	p := f.propose(t)
	reject()
	edit := engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Reason: "Synthetic correction requiring an atomic event."}
	if _, err := f.client.EditEngineeringProposal(ctx, p.ID, edit); err == nil {
		t.Fatal("edit survived failed audit")
	}
	current, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.Version != p.Version || current.Digest != p.Digest {
		t.Fatal("failed edit changed material", current, err)
	}
	review := engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Synthetic independent review requiring an atomic event."}
	if _, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, review); err == nil {
		t.Fatal("approval survived failed audit")
	}
	current, err = f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.State != "draft" || current.Review != nil || current.Version != p.Version {
		t.Fatal("failed review changed authority state", current, err)
	}
	if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.engineering_proposal_reviews`).Scan(&count); err != nil || count != 0 {
		t.Fatal("orphan review", count, err)
	}
	allow()
	revised, err := f.client.EditEngineeringProposal(ctx, p.ID, edit)
	if err != nil || revised.Revision != p.Revision+1 {
		t.Fatal("retry after audit recovery", revised, err)
	}
	f.approve(t, revised)
}

func TestProposalCapacityPreservesExistingIdentity(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	ctx := context.Background()
	limits := f.limits
	limits.MaxProposals = 1
	bounded, err := engineering.NewAutopilot(f.db, f.pipelines, limits, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bounded.Close)
	p, err := bounded.Propose(ctx, f.author, f.goal)
	if err != nil {
		t.Fatal(err)
	}
	approved := f.approve(t, p)
	f.model.mu.Lock()
	calls := len(f.model.blindInputs)
	f.model.mu.Unlock()
	retry, err := bounded.Propose(ctx, f.author, f.goal)
	if err != nil || retry.Version != approved.Version || retry.Digest != approved.Digest || retry.State != "approved" {
		t.Fatal("idempotent request lost review", retry, err)
	}
	changed := f.goal
	changed.Goal = "A different intention must not reuse this proposal identity."
	if _, err := bounded.Propose(ctx, f.author, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal("identity collision", err)
	}
	f.model.mu.Lock()
	after := len(f.model.blindInputs)
	f.model.mu.Unlock()
	if after != calls {
		t.Fatal("existing proposal invoked planning again", calls, after)
	}
	changed.ID = "capacity-second"
	changed.Pipeline = "capacity-second-pipeline"
	if _, err := bounded.Propose(ctx, f.author, changed); !errors.Is(err, engineering.ErrLimit) {
		t.Fatal("proposal capacity", err)
	}
	if _, err := bounded.Get(ctx, f.author, changed.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("over-capacity proposal persisted", err)
	}
	retained, err := bounded.Get(ctx, f.author, p.ID)
	if err != nil || retained.Digest != approved.Digest || retained.Version != approved.Version {
		t.Fatal("capacity rejection changed existing proposal", retained, err)
	}
}

func TestProposalEditsCannotRedirectReviewedTarget(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.approve(t, f.propose(t))
	for _, tc := range []struct {
		name   string
		mutate func(*engineering.AutopilotEditRequest)
	}{
		{"pipeline identity", func(r *engineering.AutopilotEditRequest) { r.Definition.ID = "another-pipeline" }},
		{"connection", func(r *engineering.AutopilotEditRequest) { r.Definition.Connection = "another-connection" }},
		{"name", func(r *engineering.AutopilotEditRequest) { r.Definition.Name = "Another purpose" }},
		{"missing stage", func(r *engineering.AutopilotEditRequest) { r.Definition.Steps = nil }},
		{"stage identity", func(r *engineering.AutopilotEditRequest) { r.Definition.Steps[0].ID = "another-output" }},
		{"source", func(r *engineering.AutopilotEditRequest) { r.Definition.Steps[0].Source = "another-source" }},
		{"context", func(r *engineering.AutopilotEditRequest) { r.Definition.Steps[0].Context = "another-context" }},
		{"unreviewed stage dependency", func(r *engineering.AutopilotEditRequest) { r.Definition.Steps[0].FromSteps = []string{"other"} }},
		{"missing rationale", func(r *engineering.AutopilotEditRequest) { r.Reason = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			definition := p.Material.Pipeline
			definition.Steps = append([]engineering.PipelineStep(nil), definition.Steps...)
			request := engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: definition, Reason: "Synthetic redirected edit."}
			tc.mutate(&request)
			if _, err := f.client.EditEngineeringProposal(context.Background(), p.ID, request); err == nil {
				t.Fatal("redirected edit accepted")
			}
			retained, err := f.auto.Get(context.Background(), f.author, p.ID)
			if err != nil || retained.Version != p.Version || retained.Digest != p.Digest || retained.State != "approved" {
				t.Fatal("rejected edit changed reviewed proposal", retained, err)
			}
		})
	}
}

// Both requests observe absence before either plans. Persistence, rather than
// process-local admission order, must resolve the competing proposal identity.
type proposalAdmissionBarrier struct {
	engineering.AutopilotRepository
	arrived atomic.Int32
	release chan struct{}
}

func (r *proposalAdmissionBarrier) ReadAutopilotProposal(ctx context.Context, e identity.Envelope, id, action string) (engineering.AutopilotProposal, error) {
	p, err := r.AutopilotRepository.ReadAutopilotProposal(ctx, e, id, action)
	if errors.Is(err, store.ErrNotFound) && action == "engineering.autopilot.propose" {
		if r.arrived.Add(1) == 2 {
			close(r.release)
		}
		select {
		case <-r.release:
		case <-ctx.Done():
			return p, ctx.Err()
		}
	}
	return p, err
}

func TestProposalConcurrentAdmissionPreservesIdentity(t *testing.T) {
	for _, collision := range []bool{false, true} {
		t.Run(fmt.Sprintf("conflicting-goal-%t", collision), func(t *testing.T) {
			f := newPhase26PlanningFixture(t)
			repository := &proposalAdmissionBarrier{AutopilotRepository: f.db, release: make(chan struct{})}
			service, err := engineering.NewAutopilot(repository, f.pipelines, f.limits, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(service.Close)
			type outcome struct {
				p   engineering.AutopilotProposal
				err error
			}
			results := make(chan outcome, 2)
			for index := range 2 {
				goal := f.goal
				if collision && index == 1 {
					goal.Goal = "Another request must not replace the accepted intention."
				}
				go func() { p, err := service.Propose(context.Background(), f.author, goal); results <- outcome{p, err} }()
			}
			first, second := <-results, <-results
			if !collision {
				if first.err != nil || second.err != nil || first.p.Digest != second.p.Digest || first.p.Version != 1 || second.p.Version != 1 {
					t.Fatal("duplicate admission did not converge", first.err, second.err)
				}
			} else {
				if (first.err == nil) == (second.err == nil) {
					t.Fatal("collision did not accept exactly one intention", first.err, second.err)
				}
				if first.err == nil {
					first, second = second, first
				}
				if !errors.Is(first.err, store.ErrConflict) {
					t.Fatal("collision was not a conflict", first.err)
				}
				current, err := service.Get(context.Background(), f.author, f.goal.ID)
				if err != nil || current.Digest != second.p.Digest || current.Material.Request.Goal != second.p.Material.Request.Goal {
					t.Fatal("collision changed winner", current, err)
				}
			}
		})
	}
}

func TestProposalCreationIsAtomicAcrossPersistenceSteps(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	ctx := context.Background()
	raw := support.Raw(t, f.dsn)
	sql(t, raw, `CREATE FUNCTION chartworks.reject_proposal_write() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF TG_TABLE_NAME='audit_events' THEN IF NEW.action<>'engineering.proposal_created' THEN RETURN NEW; END IF; END IF; RAISE EXCEPTION 'synthetic proposal persistence failure'; END; $$`)
	for _, table := range []string{"engineering_proposal_heads", "engineering_proposal_versions", "engineering_proposal_references", "audit_events"} {
		t.Run(table, func(t *testing.T) {
			// Identifiers are this closed synthetic test inventory, never request data.
			relation := pgx.Identifier{"chartworks", table}.Sanitize()
			sql(t, raw, "CREATE TRIGGER reject_proposal_write BEFORE INSERT ON "+relation+" FOR EACH ROW EXECUTE FUNCTION chartworks.reject_proposal_write()")
			_, err := f.client.ProposeEngineering(ctx, f.goal)
			sql(t, raw, "DROP TRIGGER reject_proposal_write ON "+relation)
			if err == nil {
				t.Fatal("proposal survived failed persistence step")
			}
			var heads, versions, refs, events int
			err = raw.QueryRow(ctx, `SELECT (SELECT count(*) FROM chartworks.engineering_proposal_heads),(SELECT count(*) FROM chartworks.engineering_proposal_versions),(SELECT count(*) FROM chartworks.engineering_proposal_references),(SELECT count(*) FROM chartworks.engineering_proposal_events)`).Scan(&heads, &versions, &refs, &events)
			if err != nil || heads != 0 || versions != 0 || refs != 0 || events != 0 {
				t.Fatal("partial proposal transaction", heads, versions, refs, events, err)
			}
		})
	}
	f.approve(t, f.propose(t))
}

func TestProposalReviewRejectsUnavailableSource(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	ctx := context.Background()
	raw := support.Raw(t, f.dsn)
	// Model a source removal committed after planning and before human approval.
	if _, err := raw.Exec(ctx, `UPDATE chartworks.sources SET deleted=true WHERE tenant_id=$1 AND source_id=$2`, f.author.Tenant(), f.goal.Source); err != nil {
		t.Fatal(err)
	}
	request := engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Review must recheck the source at commit."}
	if _, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, request); err == nil {
		t.Fatal("approval accepted a deleted source")
	}
	current, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.Version != p.Version || current.State != "draft" || current.Review != nil {
		t.Fatal("failed source fence changed proposal", current, err)
	}
	request.Decision = "reject"
	rejected, err := f.reviewer.ReviewEngineeringProposal(ctx, p.ID, request)
	if err != nil || rejected.State != "rejected" {
		t.Fatal("unavailable source prevented explicit rejection", rejected, err)
	}
}

func TestProposalEvidenceBudgetStopsBeforeCatalogModelCall(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	limits := f.limits
	limits.MaxEvidenceBytes = 1024
	service, err := engineering.NewAutopilot(f.db, f.pipelines, limits, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	goal := f.goal
	goal.Goal = strings.Repeat("A bounded synthetic requirement. ", 64)
	before := f.model.fixture.requests.Load()
	if _, err := service.Propose(context.Background(), f.author, goal); !errors.Is(err, engineering.ErrLimit) {
		t.Fatal("oversized catalog evidence was accepted", err)
	}
	if calls := f.model.fixture.requests.Load() - before; calls != 1 {
		t.Fatal("evidence limit allowed a catalog model call", calls)
	}
	if _, err := service.Get(context.Background(), f.author, goal.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("over-budget material persisted", err)
	}
}

func TestDisabledAutopilotPreservesReadOnlyProposalAccess(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	limits := f.limits
	limits.Enabled = false
	service, err := engineering.NewAutopilot(f.db, f.pipelines, limits, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.Close)
	before := f.model.fixture.requests.Load()
	retained, err := service.Get(context.Background(), f.author, p.ID)
	if err != nil || retained.Digest != p.Digest {
		t.Fatal("disabled planner lost retained proposal", retained, err)
	}
	if _, err := service.Propose(context.Background(), f.author, f.goal); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("disabled planner accepted mutation", err)
	}
	if _, err := service.Edit(context.Background(), f.author, p.ID, engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Reason: "Disabled planner edit."}); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("disabled planner accepted edit", err)
	}
	service.Close()
	if _, err := service.Get(context.Background(), f.author, p.ID); !errors.Is(err, store.ErrUnavailable) {
		t.Fatal("closed service admitted a request", err)
	}
	if f.model.fixture.requests.Load() != before {
		t.Fatal("retained access or disabled mutation invoked model")
	}
}

func TestProposalRejectsUnreviewedEffectExpansion(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	ctx := context.Background()
	before := f.model.fixture.requests.Load()
	edit := engineering.AutopilotEditRequest{ExpectedVersion: p.Version, Definition: p.Material.Pipeline, Reason: "Synthetic effect expansion."}
	edit.Schedule = &jobs.Spec{}
	if _, err := f.auto.Edit(ctx, f.author, p.ID, edit); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("edit added an unrequested schedule", err)
	}
	edit.Schedule = nil
	edit.Topic = &semantics.TopicPack{}
	if _, err := f.auto.Edit(ctx, f.author, p.ID, edit); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("edit added an unrequested topic", err)
	}
	edit.Topic = nil
	edit.Definition.Steps = append([]engineering.PipelineStep(nil), edit.Definition.Steps...)
	edit.Definition.Steps[0].SQL = "DELETE FROM analytics.sales"
	if _, err := f.auto.Edit(ctx, f.author, p.ID, edit); err == nil {
		t.Fatal("edit admitted a destructive read plan")
	}
	if _, err := f.auto.Review(ctx, f.author, p.ID, engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "publish", Reason: "Unsupported review effect."}); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("review accepted publication as a decision", err)
	}
	if _, err := f.auto.Amend(ctx, f.author, p.ID, engineering.AutopilotAmendRequest{Drift: "invented"}); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("amendment accepted an invalid evidence identity", err)
	}
	invalid := f.goal
	invalid.Goal = ""
	if _, err := f.auto.Propose(ctx, f.author, invalid); !errors.Is(err, engineering.ErrInvalid) {
		t.Fatal("empty goal admitted", err)
	}
	current, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.Version != p.Version || current.Digest != p.Digest || current.State != "draft" {
		t.Fatal("invalid requests changed proposal", current, err)
	}
	if f.model.fixture.requests.Load() != before {
		t.Fatal("invalid mutation called model")
	}
}

func TestProposalReadRejectsCorruptEffectEvidence(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	ctx := context.Background()
	raw := support.Raw(t, f.dsn)
	for _, body := range []string{`{}`, `{"target":"pipeline","observed_at":42}`} {
		if _, err := raw.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_effects(tenant_id,proposal_id,revision,kind,target_id,evidence,observed_order) VALUES($1,$2,$3,'pipeline_draft','pipeline',$4::jsonb,0)`, f.author.Tenant(), p.ID, p.Revision, body); err != nil {
			t.Fatal(err)
		}
		if _, err := f.auto.Get(ctx, f.author, p.ID); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("corrupt effect evidence exposed", err)
		}
		if _, err := raw.Exec(ctx, `DELETE FROM chartworks.engineering_proposal_effects WHERE tenant_id=$1 AND proposal_id=$2`, f.author.Tenant(), p.ID); err != nil {
			t.Fatal(err)
		}
	}
	current, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || current.Digest != p.Digest || current.Version != p.Version {
		t.Fatal("repaired evidence changed immutable material", current, err)
	}
}

func TestProposalReadBoundsRetainedEffects(t *testing.T) {
	f := newPhase26PlanningFixture(t)
	p := f.propose(t)
	ctx := context.Background()
	raw := support.Raw(t, f.dsn)
	insert := func(index int) {
		t.Helper()
		target := fmt.Sprintf("synthetic-output-%d", index)
		if _, err := raw.Exec(ctx, `INSERT INTO chartworks.engineering_proposal_effects(tenant_id,proposal_id,revision,kind,target_id,evidence,observed_order) VALUES($1,$2,$3,'managed_step',$4,jsonb_build_object('kind','managed_step','target',$4::text,'state','checked','observed_at',clock_timestamp()),3)`, f.author.Tenant(), p.ID, p.Revision, target); err != nil {
			t.Fatal(err)
		}
	}
	for index := range 40 {
		insert(index)
	}
	bounded, err := f.auto.Get(ctx, f.author, p.ID)
	if err != nil || len(bounded.Effects) != 40 {
		t.Fatal("exact receipt limit", len(bounded.Effects), err)
	}
	insert(40)
	if _, err := f.auto.Get(ctx, f.author, p.ID); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("oversized receipt set was exposed", err)
	}
}
