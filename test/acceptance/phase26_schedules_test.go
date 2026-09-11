package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
	"github.com/hurtener/chartworks/test/support"
)

func testPhase26ScheduledPipeline(t *testing.T) {
	f := newPhase26Fixture(t)
	applied := f.apply(t, f.approve(t, f.propose(t)))
	record, err := f.db.ReadPipeline(context.Background(), f.author, f.goal.Pipeline, 1, "engineering.pipeline.run", "write")
	if err != nil {
		t.Fatal(err)
	}
	target := jobs.PipelineTarget{ID: record.Definition.ID, Version: record.Version, Digest: record.Digest}
	cfg := f.token.cfg
	cfg.Audiences.Jobs = "chartworks:execution"
	verifier, err := auth.New(cfg, f.token.server.Client(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Close)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, secret, ok := r.BasicAuth()
		if !ok || id != "broker" || secret != "SYNTHETIC_BROKER_SECRET" {
			w.WriteHeader(401)
			return
		}
		var input struct {
			Version  int    `json:"version"`
			Binding  string `json:"binding_id"`
			Job      string `json:"job_id"`
			Manifest string `json:"manifest_hash"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&input) != nil || input.Version != 1 || input.Binding != "pipeline" {
			w.WriteHeader(403)
			return
		}
		scopes := []string{"engineering.pipeline.run", "sources.query", "sources.read", "cw.source.write:" + target.ID, "cw.execution_binding.use:pipeline", "cw.run.execute:" + input.Job}
		for _, step := range record.Definition.Steps {
			scopes = append(scopes, "cw.source.read:"+step.Source, "cw.source.query:"+step.Source, "cw.execution_context.use:"+step.Context)
			for _, dataset := range step.Inputs {
				scopes = append(scopes, "cw.dataset.query:"+dataset)
			}
		}
		claims := f.token.claims(f.author.Tenant(), jobs.Executor("pipeline"), scopes)
		now := time.Now().Unix()
		claims["iat"], claims["exp"], claims["aud"], claims["session"] = now, now+30, "chartworks:execution", input.Job
		claims["execution_version"], claims["execution_binding"], claims["execution_binding_revision"], claims["execution_manifest"] = 1, "pipeline", 1, input.Manifest
		token := f.token.sign(t, claims, nil)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"version": 1, "access_token": token, "token_type": "Bearer", "expires_in": 30, "binding_id": "pipeline", "binding_revision": 1})
	}))
	t.Cleanup(server.Close)
	provider, err := broker.New(server.URL+"/exchange/execution-authority", map[string]broker.Credential{f.author.Tenant(): {ClientID: "broker", Secret: "SYNTHETIC_BROKER_SECRET"}}, verifier, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provider.Close)
	queue, err := jobs.NewWithPipeline(f.db, provider, jobs.Defaults(), f.pipelines)
	if err != nil {
		t.Fatal(err)
	}
	scopes := append(phase26Scopes(), "scheduling.write", "scheduling.execute", "scheduling.read", "cw.execution_binding.use:pipeline", "cw.schedule.execute:*", "cw.run.read:*")
	author := f.token.envelope(t, f.author.Tenant(), f.author.User(), scopes...)
	schedule, err := queue.CreateSchedule(context.Background(), author, "pipeline-schedule", jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.PipelineKind, BindingID: "pipeline", Pipeline: &target}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}})
	if err != nil {
		t.Fatal("schedule admission", err)
	}
	job, err := queue.Fire(context.Background(), author, schedule.ID, "first-occurrence")
	if err != nil {
		t.Fatal("schedule fire", err)
	}
	if err = queue.RunOnce(context.Background()); err != nil {
		t.Fatal("native scheduled pipeline", err)
	}
	completed, err := queue.Get(context.Background(), author, job.ID)
	if err != nil || completed.State != "succeeded" || completed.Attempts != 1 {
		t.Fatal("occurrence completion", completed.State, err)
	}
	replay, err := queue.Fire(context.Background(), author, schedule.ID, "first-occurrence")
	if err != nil || replay.ID != job.ID || replay.Attempts != 1 || replay.ManifestHash != job.ManifestHash {
		t.Fatal("occurrence replay", replay, err)
	}
	proof, err := provider.Acquire(context.Background(), completed)
	if err != nil {
		t.Fatal(err)
	}
	execution, err := f.db.ReadPipelineExecution(context.Background(), proof.Envelope(), job.ID)
	if err != nil || execution.State != "published" || execution.Operation.ID != job.ID || execution.Operation.ID == applied.Operation {
		t.Fatal("scheduled operation did not own pipeline effects", err)
	}
	testPhase26ReviewedSchedule(t, f, queue)
}

func testPhase26ReviewedSchedule(t *testing.T, f *phase26Fixture, queue *jobs.Service) {
	ctx := context.Background()
	auto, err := engineering.NewAutopilotWithSchedules(f.db, f.pipelines, f.limits, queue)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auto.Close)
	// This proposal has no topic effect. Keep its bearer within the consumed
	// 32-scope bound instead of requesting unrelated topic/compensation actions.
	scopes := slices.DeleteFunc(phase26Scopes(), func(scope string) bool {
		return slices.Contains([]string{"topics.read", "topics.write", "cw.topic.read:*", "cw.topic.write:*", "engineering.autopilot.compensate"}, scope)
	})
	scopes = append(scopes, "scheduling.write", "scheduling.read", "scheduling.execute", "cw.execution_binding.use:pipeline", "cw.schedule.write:*", "cw.schedule.read:*", "cw.schedule.execute:*", "cw.run.read:*")
	author := f.token.envelope(t, f.author.Tenant(), f.author.User(), scopes...)
	reviewer := f.token.envelope(t, f.author.Tenant(), "independent-reviewer", scopes...)
	goal := f.goal
	goal.ID, goal.ExpectedPipelineVersion = "reviewed-schedule-goal", 1
	goal.Schedule = &engineering.AutopilotScheduleGoal{BindingID: "pipeline", Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}
	raw := support.Raw(t, f.dsn)
	count := func() int {
		t.Helper()
		var n int
		if err := raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.job_schedules`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	before := count()
	proposal, err := auto.Propose(ctx, author, goal)
	if err != nil {
		t.Fatal("schedule proposal", err)
	}
	spec := jobs.Spec{Type: "interval", IntervalSeconds: 60, Anchor: time.Now().UTC().Truncate(time.Second), Timezone: "UTC", Missed: "skip", Overlap: "queue"}
	proposal, err = auto.Edit(ctx, author, proposal.ID, engineering.AutopilotEditRequest{ExpectedVersion: proposal.Version, Definition: proposal.Material.Pipeline, Schedule: &spec, Reason: "Review the exact interval before enabling refresh."})
	if err != nil {
		t.Fatal("schedule edit", err)
	}
	if count() != before {
		t.Fatal("planning created an active schedule")
	}
	approve := func(p engineering.AutopilotProposal) engineering.AutopilotProposal {
		t.Helper()
		out, err := auto.Review(ctx, reviewer, p.ID, engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Reviewed the exact pipeline and recurrence."})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	proposal = approve(proposal)
	limited := f.token.envelope(t, author.Tenant(), author.User(), slices.DeleteFunc(append([]string(nil), scopes...), func(s string) bool { return s == "scheduling.write" })...)
	if _, err = auto.Apply(ctx, limited, proposal.ID, phase26ApplyRequest(proposal)); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("review bypassed schedule authority", err)
	}
	applied, err := auto.Apply(ctx, author, proposal.ID, phase26ApplyRequest(proposal))
	if err != nil || applied.State != "applied" {
		t.Fatal("reviewed schedule apply", applied.State, err)
	}
	scheduleID := ""
	for _, effect := range applied.Effects {
		if effect.Kind == "schedule" && effect.State == "committed" {
			scheduleID = effect.Target
		}
	}
	schedule, err := queue.GetSchedule(ctx, author, scheduleID)
	if err != nil || schedule.Request.Target.Pipeline == nil || schedule.Request.Target.Pipeline.Version != 2 || schedule.Request.Spec.Type != "interval" || count() != before+1 {
		t.Fatal("durable reviewed schedule missing", schedule, err)
	}
	accepted, err := queue.Fire(ctx, author, scheduleID, "accepted-reviewed-occurrence")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = raw.Exec(ctx, `UPDATE chartworks.engineering_proposal_heads SET applied_at=clock_timestamp()-interval '2 minutes' WHERE tenant_id=$1 AND proposal_id=$2`, author.Tenant(), applied.ID); err != nil {
		t.Fatal(err)
	}
	drift, err := auto.DetectDrift(ctx, author, applied.ID)
	if err != nil {
		t.Fatal(err)
	}
	amendment, err := auto.Amend(ctx, author, applied.ID, engineering.AutopilotAmendRequest{Drift: drift.ID})
	if err != nil || amendment.Material.Request.Schedule.ID != scheduleID || amendment.Material.Request.Schedule.ExpectedRevision != 1 {
		t.Fatal("schedule amendment did not address existing revision", err)
	}
	amendment = approve(amendment)
	amended, err := auto.Apply(ctx, author, amendment.ID, phase26ApplyRequest(amendment))
	if err != nil || amended.State != "applied" {
		t.Fatal("schedule amendment apply", err)
	}
	current, err := queue.GetSchedule(ctx, author, scheduleID)
	if err != nil || current.Revision != 2 || current.Request.Target.Pipeline.Version != 3 || count() != before+1 {
		t.Fatal("amendment duplicated or failed to replace schedule", current, err)
	}
	retained, err := queue.Get(ctx, author, accepted.ID)
	if err != nil || retained.ManifestHash != accepted.ManifestHash || retained.Pipeline.Version != 2 || retained.ScheduleRevision != 1 {
		t.Fatal("amendment rewrote accepted work", err)
	}
}
