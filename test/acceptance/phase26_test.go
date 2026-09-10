package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// This adapter selects recorded provider responses, then invokes the REAL shared
// Bifrost gateway, including its authority, budget, schema and usage accounting.
// No production planner, validator, pipeline runner or store is replaced.
type phase26Model struct {
	gateway.Engine
	fixture     *gatewayFixture
	mu          sync.Mutex
	foreign     atomic.Bool
	blindInputs []string
}

func (m *phase26Model) Generate(ctx context.Context, call gateway.Call, budget *gateway.Budget, role, instructions, input string, schema *gateway.Schema) (gateway.Generated, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if role != "pipeline_draft" {
		return gateway.Generated{}, fmt.Errorf("unexpected learned stage: %s", role)
	}
	var supplied struct {
		Relations []readexec.Relation `json:"relations"`
	}
	if err := json.Unmarshal([]byte(input), &supplied); err != nil {
		return gateway.Generated{}, err
	}
	var answer any
	if len(supplied.Relations) == 0 {
		m.blindInputs = append(m.blindInputs, input)
		answer = map[string]any{"requirements": []string{"A stable sale identifier for the bounded requested dataset"}, "rationale": "Identify the needed key before considering source alternatives."}
	} else {
		var relation readexec.Relation
		for _, candidate := range supplied.Relations {
			if candidate.Schema == "analytics" && candidate.Name == "sales" {
				relation = candidate
			}
		}
		if relation.ID == "" {
			return gateway.Generated{}, errors.New("synthetic authorized sales relation absent")
		}
		evidence, alternative := "relation:"+relation.ID, relation.ID
		if m.foreign.Load() {
			evidence, alternative = "relation:foreign-dataset", "foreign-dataset"
		}
		answer = map[string]any{
			"sql":          "SELECT id::bigint AS id FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize(),
			"columns":      []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}},
			"rationale":    "A reviewed managed identifier projection avoids exposing unrelated source columns.",
			"evidence":     []string{"binding", evidence},
			"alternatives": []engineering.ProposalAlternative{{Dataset: alternative, Rationale: "Direct reuse exposes a wider source schema than this managed projection."}},
		}
	}
	content, err := json.Marshal(answer)
	if err != nil {
		return gateway.Generated{}, err
	}
	raw, err := json.Marshal(map[string]any{
		"id": "recorded-reviewed-engineering", "object": "chat.completion", "model": m.fixture.cfg.Roles[role].Model,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": string(content)}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	})
	if err != nil {
		return gateway.Generated{}, err
	}
	m.fixture.mode.Store("chat_raw:" + string(raw))
	return m.Engine.Generate(ctx, call, budget, role, instructions, input, schema)
}

func phase26Scopes() []string {
	return []string{
		"sources.read", "sources.query", "engineering.read", "topics.read", "topics.write", "cw.tenant.write:source-a", "cw.topic.write:*", "engineering.autopilot.read", "engineering.autopilot.propose", "engineering.autopilot.review", "engineering.autopilot.apply", "engineering.autopilot.compensate", "engineering.autopilot.drift",
		"engineering.pipeline.read", "engineering.pipeline.write", "engineering.pipeline.publish", "engineering.pipeline.run", "jobs.read", "jobs.cancel",
		"cw.source.read:*", "cw.source.write:*", "cw.source.query:*", "cw.execution_context.use:*", "cw.dataset.query:*", "cw.topic.read:*", "cw.block.read:*",
	}
}

type phase26Fixture struct {
	*pipelineFixture
	model    *phase26Model
	auto     *engineering.Autopilot
	author   identity.Envelope
	limits   config.Autopilot
	goal     engineering.AutopilotGoal
	client   *sdk.Client
	reviewer *sdk.Client
}

func newPhase26Fixture(t *testing.T) *phase26Fixture {
	t.Helper()
	provider := newGatewayFixture(t, nil)
	model := &phase26Model{Engine: provider.engine, fixture: provider}
	f := newPipelineFixture(t, model, nil)
	source := f.create(t, "p26-authorized-source")
	limits := config.DefaultAutopilot()
	limits.Enabled, limits.ModelVersion = true, "reviewed-model-v1"
	topicDrafts, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	auto, err := engineering.NewAutopilot(f.db, f.pipelines, limits, topicDrafts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(auto.Close)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase26Scopes()...)

	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	published, err := topics.New(f.db, f.s, index, model)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := reporting.New(f.db, published, f.s, f.validator, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	runner, err := jobs.NewRequestRunner(f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewRuns(blocks, f.db, runner, nil, "", config.DefaultReportingExecution())
	if err != nil {
		t.Fatal(err)
	}
	registry, err := reportingapi.RuntimeRegistry(false, true)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, reportingapi.RuntimeHandler(f.token.verifier, runs, auto, false, true, http.NotFoundHandler())))
	t.Cleanup(server.Close)
	clientFor := func(user string) *sdk.Client {
		bearer := f.token.sign(t, f.token.claims(e.Tenant(), user, phase26Scopes()), nil)
		client, clientErr := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
		if clientErr != nil {
			t.Fatal(clientErr)
		}
		return client
	}
	return &phase26Fixture{client: clientFor(e.User()), reviewer: clientFor("independent-reviewer"), pipelineFixture: f, model: model, auto: auto, author: e, limits: limits,
		goal: engineering.AutopilotGoal{ID: "p26-goal", Pipeline: "p26-managed", Name: "Reviewed identifier projection", Connection: "workspace", Source: source.ID, Context: source.ContextID, Goal: "Prepare a managed sale identifier dataset for review; do not publish business meaning.", MaxStalenessSeconds: 60}}
}

func (f *phase26Fixture) propose(t *testing.T) engineering.AutopilotProposal {
	t.Helper()
	p, err := f.client.ProposeEngineering(context.Background(), f.goal)
	if err != nil {
		t.Fatal("bounded L2 proposal", err)
	}
	return p
}

func (f *phase26Fixture) approve(t *testing.T, p engineering.AutopilotProposal) engineering.AutopilotProposal {
	t.Helper()
	approved, err := f.reviewer.ReviewEngineeringProposal(context.Background(), p.ID, engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Reviewed exact SQL, ownership and declared evidence."})
	if err != nil {
		t.Fatal("independent approval", err)
	}
	return approved
}

func phase26ApplyRequest(p engineering.AutopilotProposal) engineering.AutopilotApplyRequest {
	return engineering.AutopilotApplyRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest}
}

func (f *phase26Fixture) apply(t *testing.T, p engineering.AutopilotProposal) engineering.AutopilotProposal {
	t.Helper()
	out, err := f.client.ApplyEngineeringProposal(context.Background(), p.ID, phase26ApplyRequest(p))
	if err != nil || out.State != "applied" || out.Operation == "" {
		t.Fatal("reviewed native managed application", out.State, err)
	}
	return out
}

// A committed local effect survives a lost client reply. The proposal's effect
// ledger must recover it rather than creating a second pipeline version.
type phase26LostEffect struct {
	engineering.AutopilotRepository
	lost atomic.Bool
}

func (r *phase26LostEffect) StageAutopilotPipeline(ctx context.Context, e identity.Envelope, p engineering.PreparedProposalApply, stage string) (engineering.AutopilotProposal, error) {
	out, err := r.AutopilotRepository.StageAutopilotPipeline(ctx, e, p, stage)
	if err == nil && stage == "draft" && r.lost.CompareAndSwap(false, true) {
		return engineering.AutopilotProposal{}, store.ErrUnavailable
	}
	return out, err
}

func TestPhase26(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		f := newPhase26Fixture(t)
		p := f.propose(t)
		if p.State != "draft" || p.Review != nil || p.Revision != 1 || len(p.Material.Usage.Calls) != 2 || f.model.fixture.requests.Load() != 2 || len(p.Material.Objects) != 2 {
			t.Fatal("proposal lacks bounded real planning evidence", p.State, p.Material.Usage)
		}
		for _, object := range p.Material.Objects {
			if len(object.Evidence) == 0 || len(object.Alternatives) == 0 || object.Rationale == "" || object.Author != f.author.User() || object.ModelVersion != f.limits.ModelVersion || object.Decision != "build_managed" {
				t.Fatal("object lacks a reviewable match/build decision", object)
			}
		}
		f.model.mu.Lock()
		blind := append([]string(nil), f.model.blindInputs...)
		f.model.mu.Unlock()
		if len(blind) != 1 || strings.Contains(blind[0], "analytics.sales") || strings.Contains(blind[0], `"relations"`) || strings.Contains(blind[0], f.goal.Context) {
			t.Fatal("blind planning received the catalog", blind)
		}
		replay := f.propose(t)
		if replay.Digest != p.Digest || replay.Version != p.Version || f.model.fixture.requests.Load() != 2 {
			t.Fatal("proposal idempotency repeated inference")
		}
		if _, err := f.pipelines.Get(context.Background(), f.author, f.goal.Pipeline, 1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("proposal creation performed pipeline effects", err)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		f := newPhase26Fixture(t)
		p := f.propose(t)
		before := f.model.fixture.requests.Load()
		foreign := f.token.envelope(t, "other-tenant", f.author.User(), phase26Scopes()...)
		if _, err := f.auto.Get(context.Background(), foreign, p.ID); !errors.Is(err, store.ErrNotFound) && !errors.Is(err, access.ErrNotFound) {
			t.Fatal("cross-tenant proposal disclosed", err)
		}
		narrowScopes := phase26Scopes()
		for i, s := range narrowScopes {
			if s == "cw.execution_context.use:*" {
				narrowScopes[i] = "cw.execution_context.use:other-context"
			}
		}
		narrow := f.token.envelope(t, f.author.Tenant(), f.author.User(), narrowScopes...)
		if _, err := f.auto.Propose(context.Background(), narrow, f.goal); err == nil || f.model.fixture.requests.Load() != before {
			t.Fatal("foreign execution partition reached inference", err)
		}
		f.model.foreign.Store(true)
		bad := f.goal
		bad.ID, bad.Pipeline = "p26-foreign-evidence", "p26-foreign-evidence-pipeline"
		if _, err := f.auto.Propose(context.Background(), f.author, bad); err == nil {
			t.Fatal("model-supplied foreign evidence became authorized material")
		}
		if _, err := f.auto.Get(context.Background(), f.author, bad.ID); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("rejected model evidence left a proposal", err)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		f := newPhase26Fixture(t)
		p := f.propose(t)
		request := engineering.AutopilotReviewRequest{ExpectedVersion: p.Version, Revision: p.Revision, Digest: p.Digest, Decision: "approve", Reason: "Synthetic review"}
		if _, err := f.auto.Review(context.Background(), f.author, p.ID, request); !errors.Is(err, engineering.ErrProposalReview) {
			t.Fatal("submitter self-approved", err)
		}
		if _, err := f.client.ApplyEngineeringProposal(context.Background(), p.ID, phase26ApplyRequest(p)); err == nil {
			t.Fatal("unreviewed proposal applied")
		} else {
			var status *sdk.StatusError
			if !errors.As(err, &status) || status.Status != 409 {
				t.Fatal("unreviewed proposal failure lost its wire contract", err)
			}
		}
		reviewer := f.token.envelope(t, f.author.Tenant(), "independent-reviewer", phase26Scopes()...)
		results := make(chan error, 2)
		var group sync.WaitGroup
		for range 2 {
			group.Add(1)
			go func() {
				defer group.Done()
				_, err := f.auto.Review(context.Background(), reviewer, p.ID, request)
				results <- err
			}()
		}
		group.Wait()
		close(results)
		wins, conflicts := 0, 0
		for err := range results {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, store.ErrConflict):
				conflicts++
			default:
				t.Fatal("unexpected competing review result", err)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatal("review CAS admitted competing decisions", wins, conflicts)
		}
		approved, err := f.auto.Get(context.Background(), f.author, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		edited, err := f.auto.Edit(context.Background(), f.author, p.ID, engineering.AutopilotEditRequest{ExpectedVersion: approved.Version, Definition: approved.Material.Pipeline, Reason: "A separately reviewed amendment is required."})
		if err != nil || edited.State != "draft" || edited.Review != nil || edited.Revision != p.Revision+1 || edited.Digest == p.Digest {
			t.Fatal("edit retained stale approval", edited, err)
		}
		if _, err = f.auto.Apply(context.Background(), f.author, p.ID, phase26ApplyRequest(approved)); !errors.Is(err, store.ErrConflict) {
			t.Fatal("old approved digest applied after edit", err)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		f := newPhase26Fixture(t)
		p := f.approve(t, f.propose(t))
		lost := &phase26LostEffect{AutopilotRepository: f.db}
		crashing, err := engineering.NewAutopilot(lost, f.pipelines, f.limits)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(crashing.Close)
		if _, err = crashing.Apply(context.Background(), f.author, p.ID, phase26ApplyRequest(p)); !errors.Is(err, store.ErrUnavailable) || !lost.lost.Load() {
			t.Fatal("committed draft fault not exercised", err)
		}
		partial, err := f.auto.Get(context.Background(), f.author, p.ID)
		if err != nil || partial.State != "applying" || len(partial.Effects) != 1 || partial.Effects[0].Kind != "pipeline_draft" {
			t.Fatal("partial application evidence was lost", partial, err)
		}
		applied := f.apply(t, p)
		execution, err := f.db.ReadPipelineExecution(context.Background(), f.author, applied.Operation)
		if err != nil || execution.Version != 1 || len(execution.Stages) != 1 || execution.Stages[0].Stage.Revision != 1 {
			t.Fatal("effect replay created another generation", execution, err)
		}
		stage := execution.Stages[0].Stage
		binding, err := f.s.Binding(context.Background(), f.author, stage.Source, stage.Context)
		if err != nil || len(binding.Relations) != 1 {
			t.Fatal("actual managed generation missing", binding, err)
		}
		var rows int
		table := pgx.Identifier{binding.Relations[0].Schema, binding.Relations[0].Name}.Sanitize()
		if err = f.admin.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&rows); err != nil || rows != 2 {
			t.Fatal("native runner did not materialize the reviewed query", rows, err)
		}
		compensated, err := f.auto.Compensate(context.Background(), f.author, applied.ID, phase26ApplyRequest(applied))
		if err != nil || compensated.State != "compensated" {
			t.Fatal("owned generation quarantine failed", compensated.State, err)
		}
		if err = f.admin.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&rows); err != nil || rows != 2 {
			t.Fatal("quarantine incorrectly claimed or performed global warehouse rollback", rows, err)
		}
		if _, err = f.s.Binding(context.Background(), f.author, stage.Source, stage.Context); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("compensated generation remained publicly addressable", err)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		f := newPhase26Fixture(t)
		p := f.apply(t, f.approve(t, f.propose(t)))
		raw := support.Raw(t, f.dsn)
		// Advance only the synthetic observation's age, not any immutable
		// proposal/pipeline content or its accepted review.
		if _, err := raw.Exec(context.Background(), `UPDATE chartworks.engineering_proposal_heads SET applied_at=clock_timestamp()-interval '2 minutes' WHERE tenant_id=$1 AND proposal_id=$2`, f.author.Tenant(), p.ID); err != nil {
			t.Fatal(err)
		}
		impactTopic := phase26ImpactTopic(t, f, p)
		phase26CheckImpactReach(t, f, p, impactTopic)
		before := f.model.fixture.requests.Load()
		first, err := f.auto.DetectDrift(context.Background(), f.author, p.ID)
		if err != nil || first.Kind != "freshness_expired" || first.State != "proposed" || first.ID == "" || len(first.Impacts) == 0 {
			t.Fatal("reviewable drift evidence missing", first, err)
		}
		second, err := f.auto.DetectDrift(context.Background(), f.author, p.ID)
		if err != nil || second.ID != first.ID || second.EvidenceDigest != first.EvidenceDigest || f.model.fixture.requests.Load() != before {
			t.Fatal("unchanged drift spammed amendments or called a model", second, err)
		}
		amendment, err := f.client.AmendEngineeringProposal(context.Background(), p.ID, sdk.EngineeringProposalAmend{Drift: first.ID})
		if err != nil || amendment.State != "draft" || amendment.Review != nil || amendment.AmendmentOf != p.ID || amendment.Material.Request.ExpectedPipelineVersion != 1 || amendment.Material.Origin == nil || amendment.Material.Origin.EvidenceDigest != first.EvidenceDigest {
			t.Fatal("drift did not become independent review material", amendment, err)
		}
		afterAmend := f.model.fixture.requests.Load()
		replay, err := f.client.AmendEngineeringProposal(context.Background(), p.ID, sdk.EngineeringProposalAmend{Drift: first.ID})
		if err != nil || replay.ID != amendment.ID || f.model.fixture.requests.Load() != afterAmend {
			t.Fatal("amendment replay repeated planning", replay, err)
		}
		if _, err = f.auto.Apply(context.Background(), f.author, amendment.ID, phase26ApplyRequest(amendment)); !errors.Is(err, engineering.ErrProposalReview) {
			t.Fatal("amendment inherited approval", err)
		}
		unchanged, err := f.auto.Get(context.Background(), f.author, p.ID)
		if err != nil || unchanged.Digest != p.Digest || unchanged.Revision != p.Revision || unchanged.Version != p.Version {
			t.Fatal("drift detection mutated reviewed meaning", unchanged, err)
		}
	})

	t.Run("AC06", func(t *testing.T) {
		t.Run("reviewed_topic", testPhase26TopicApply)
		f := newPhase26Fixture(t)
		approved := f.approve(t, f.propose(t))
		applyOnly := slices.DeleteFunc(phase26Scopes(), func(scope string) bool { return scope == "engineering.pipeline.run" })
		limited := f.token.envelope(t, f.author.Tenant(), f.author.User(), applyOnly...)
		if _, err := f.auto.Apply(context.Background(), limited, approved.ID, phase26ApplyRequest(approved)); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("proposal approval substituted for pipeline authority", err)
		}
		p := f.apply(t, approved)
		execution, err := f.db.ReadPipelineExecution(context.Background(), f.author, p.Operation)
		if err != nil || execution.State != "published" || execution.Operation.State != "succeeded" {
			t.Fatal("goal-to-managed-data path did not complete", execution, err)
		}
		stage := execution.Stages[0].Stage
		binding, err := f.s.Binding(context.Background(), f.author, stage.Source, stage.Context)
		if err != nil {
			t.Fatal(err)
		}
		relation := binding.Relations[0]
		dependent := engineering.PipelineDefinition{ID: "p26-dependent", Name: "Independent published consumer", Connection: "workspace", Steps: []engineering.PipelineStep{{ID: "output", Source: stage.Source, Context: stage.Context, SQL: "SELECT id FROM " + pgx.Identifier{relation.Schema, relation.Name}.Sanitize(), Inputs: []string{relation.ID}, DependsOn: []string{}, Strategy: "replace", Columns: []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []engineering.PipelineCheck{}}}}
		version, err := f.pipelines.Draft(context.Background(), f.author, dependent, 0)
		if err != nil {
			t.Fatal("independent consumer draft", err)
		}
		if _, err = f.pipelines.Publish(context.Background(), f.author, dependent.ID, version.Version); err != nil {
			t.Fatal("independent consumer publication", err)
		}
		if _, err = f.auto.Compensate(context.Background(), f.author, p.ID, phase26ApplyRequest(p)); !errors.Is(err, engineering.ErrCompensationBlocked) {
			t.Fatal("compensation retired an independently referenced generation", err)
		}
		if _, err = f.db.SaveAutopilotProposal(context.Background(), f.author, engineering.PreparedProposal{}, 0, 100); err == nil {
			t.Fatal("caller manufactured reviewed proposal material")
		}
	})
}
