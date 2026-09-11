package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/jobs"
	broker "github.com/hurtener/chartworks/internal/jobs/pengui"
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
}
