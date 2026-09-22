package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// The accepted record is created through Phase 24's real review service. The
// selection fixture advances the tenant CAS pointer directly so this test can
// isolate reporting's consumer without fabricating heldout optimization reports.
func reviewedNarrativePack(t *testing.T, f *phase17Fixture, id, role, model string, accepted bool) evaluation.RuntimePackRecord {
	t.Helper()
	tenant := f.f.e.Tenant()
	author := phase27Actor(t, f, "runtime-author", []string{"ops.write", "cw.tenant.write:" + tenant})
	reviewer := phase27Actor(t, f, "runtime-reviewer", []string{"ops.audit", "cw.tenant.certify:" + tenant})
	svc, err := evaluation.New(f.f.db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := gateway.RuntimeConfig{Model: "reviewed-default", Models: []gateway.RuntimeModel{{Role: role, Model: model}},
		SystemInstruction: "Use only the bounded evidence.", AttemptCostUSD: 0.01}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	pack := evaluation.PackRevision{ID: id, Revision: 1, Model: cfg.Model,
		Models: []evaluation.PackModel{{Role: role, Model: model}}, ConfigurationDigest: cfg.Digest}
	pack.Digest = pack.CanonicalDigest()
	draft, err := svc.AuthorRuntimePack(context.Background(), author, pack, cfg)
	if err != nil {
		t.Fatal("author reviewed runtime pack", err)
	}
	decision := evaluation.Accepted
	if !accepted {
		decision = evaluation.Rejected
	}
	out, err := svc.ReviewRuntimePack(context.Background(), reviewer, pack.Digest, evaluation.RuntimePackReviewRequest{
		PackID: pack.ID, PackRevision: pack.Revision, RuntimeDigest: draft.Digest,
		ConfigurationDigest: cfg.Digest, Model: cfg.Model, Models: cfg.Models,
		SystemInstruction: cfg.SystemInstruction, MaxAttemptCostUSD: cfg.AttemptCostUSD, Decision: decision})
	if err != nil {
		t.Fatal("review runtime pack", err)
	}
	return out
}

func selectNarrativePack(t *testing.T, raw *pgx.Conn, tenant string, pack evaluation.RuntimePackRecord, revision int) {
	t.Helper()
	ctx := context.Background()
	proposal := "reviewed-narrative-" + pack.Pack.ID
	_, err := raw.Exec(ctx, `INSERT INTO chartworks.evaluation_proposals(tenant_id,proposal_id,author_id,proposal_digest,proposal,state,review,created_at)
 VALUES($1,$2,'runtime-author',$3,$4::jsonb,'approve','{}'::jsonb,clock_timestamp()) ON CONFLICT DO NOTHING`,
		tenant, proposal, strings.Repeat("a", 64), `{"candidate":{"pack_digest":"`+pack.Pack.Digest+`"}}`)
	if err != nil {
		t.Fatal("selected-pack proposal fixture", err)
	}
	_, err = raw.Exec(ctx, `INSERT INTO chartworks.evaluation_pack_selection(tenant_id,revision,pack_digest,proposal_id,actor_id,selected_at)
 VALUES($1,$2,$3,$4,'runtime-reviewer',clock_timestamp()) ON CONFLICT(tenant_id) DO UPDATE
 SET revision=EXCLUDED.revision,pack_digest=EXCLUDED.pack_digest,proposal_id=EXCLUDED.proposal_id,actor_id=EXCLUDED.actor_id,selected_at=EXCLUDED.selected_at`,
		tenant, revision, pack.Pack.Digest, proposal)
	if err != nil {
		t.Fatal("select reviewed pack fixture", err)
	}
}

func TestReviewedNarrativePackFrozenReuse(t *testing.T) {
	f := newReportingFixture(t)
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor,
		reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	definition := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id")
	definition.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
	definition.Outputs[1].Narrative.MaxTokens = 8192
	definition.Outputs[1].Narrative.Fields = []string{"amount"}
	definition.Outputs[1].Narrative.RedactedFields = []string{"id"}
	created, err := blocks.Create(context.Background(), author, reporting.CreateRequest{ID: "reviewed-narrative-block", Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, blocks, author, created)
	model := newGatewayFixture(t, nil)
	plain := phase28RunService(t, f, blocks, f.f.db, model.engine, config.DefaultReportingExecution())
	runs, err := plain.WithReviewedNarrativePacks(f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, f.f.dsn)
	packA := reviewedNarrativePack(t, f, "pack-a", "narrative", "reviewed-narrative-a", true)
	packB := reviewedNarrativePack(t, f, "pack-b", "narrative", "reviewed-narrative-b", true)
	packRejected := reviewedNarrativePack(t, f, "pack-rejected", "narrative", "reviewed-narrative-rejected", false)
	packUnbound := reviewedNarrativePack(t, f, "pack-unbound", "sqlgen", "reviewed-sqlgen", true)
	request := reporting.RunRequest{Narrative: true, ReuseMaxAgeSeconds: 60}
	admit := func(key string) (reporting.RunView, error) {
		request.Key = key
		return runs.Admit(context.Background(), execute, created.State.ID, request)
	}
	if _, err := admit("before-selection"); err == nil {
		t.Fatal("unselected narrative pack was accepted")
	}
	request.PartialPolicy = "allow_partial"
	partial, err := admit("before-selection-partial")
	if err != nil {
		t.Fatal("optional narrative prevented deterministic admission", err)
	}
	partial, err = runs.Run(context.Background(), execute, partial.ID, false)
	if err != nil || partial.State != "partial" || len(partial.QueryAttempts) != 1 || model.requests.Load() != 0 {
		t.Fatal("unavailable optional narrative discarded deterministic output or called model", err, partial.State)
	}
	partialReader := phase28Reader(t, f, "partial-narrative-reader", created.State.ID, partial.Context)
	table, err := runs.Output(context.Background(), partialReader, partial.ID, "table-main")
	if err != nil || table.State != "succeeded" || table.Chart == nil {
		t.Fatal("deterministic table was lost with unavailable optional narrative", err)
	}
	request.PartialPolicy = "fail"
	selectNarrativePack(t, raw, execute.Tenant(), packA, 1)
	model.mode.Store(phase28Chat(t, "reviewed-narrative-a", `{"claims":[{"kind":"value","evidence":["e1"]}]}`))
	first, err := admit("reviewed-first")
	if err != nil {
		t.Fatal(err)
	}
	first, err = runs.Run(context.Background(), execute, first.ID, false)
	if err != nil || first.State != "succeeded" || len(first.QueryAttempts) != 1 || first.ReusedFrom != "" || model.requests.Load() != 1 {
		t.Fatal("first reviewed narrative did not execute physically", err, first.State, first.ReusedFrom)
	}
	firstStored, err := f.f.db.ReadFrozenRun(context.Background(), execute, first.ID, true)
	if err != nil || firstStored.Manifest == nil || firstStored.Manifest.NarrativePack == nil ||
		firstStored.Manifest.NarrativePack.RuntimeDigest != packA.Digest || firstStored.Manifest.ReuseKey != reporting.ReuseIdentity(*firstStored.Manifest) {
		t.Fatal("reviewed pack was not sealed into the canonical reuse identity", err)
	}
	reader := phase28Reader(t, f, "reviewed-narrative-reader", created.State.ID, first.Context)
	firstOutput, err := runs.Output(context.Background(), reader, first.ID, "narrative-main")
	if err != nil || firstOutput.Narrative == nil || len(firstOutput.Narrative.Receipt.Calls) != 1 ||
		firstOutput.Narrative.Receipt.Calls[0].RequestedModel != "reviewed-narrative-a" ||
		firstOutput.Narrative.Receipt.Calls[0].ConfigurationDigest != packA.Config.Digest {
		t.Fatal("actual narrative gateway receipt did not use the reviewed role/config", err)
	}
	second, err := admit("reviewed-second")
	if err != nil {
		t.Fatal(err)
	}
	second, err = runs.Run(context.Background(), execute, second.ID, false)
	if err != nil || second.State != "succeeded" || second.ReusedFrom != first.ID || len(second.QueryAttempts) != 0 || model.requests.Load() != 1 {
		t.Fatal("same accepted pack did not reuse a distinct frozen run", err, second.ReusedFrom, second.QueryAttempts)
	}
	selectNarrativePack(t, raw, execute.Tenant(), packB, 2)
	model.mode.Store(phase28Chat(t, "reviewed-narrative-b", `{"claims":[{"kind":"value","evidence":["e1"]}]}`))
	third, err := admit("reviewed-third")
	if err != nil {
		t.Fatal(err)
	}
	third, err = runs.Run(context.Background(), execute, third.ID, false)
	if err != nil || third.State != "succeeded" || third.ReusedFrom != "" || len(third.QueryAttempts) != 1 || model.requests.Load() != 2 {
		t.Fatal("changed accepted pack reused a stale narrative result", err, third.ReusedFrom, third.QueryAttempts)
	}
	thirdOutput, err := runs.Output(context.Background(), reader, third.ID, "narrative-main")
	if err != nil || thirdOutput.Narrative == nil || len(thirdOutput.Narrative.Receipt.Calls) != 1 ||
		thirdOutput.Narrative.Receipt.Calls[0].RequestedModel != "reviewed-narrative-b" ||
		thirdOutput.Narrative.Receipt.Calls[0].ConfigurationDigest != packB.Config.Digest {
		t.Fatal("changed pack did not change actual gateway model/config", err)
	}
	model.mu.Lock()
	actualModels := append([]string(nil), model.models...)
	model.mu.Unlock()
	if len(actualModels) != 2 || actualModels[0] != "reviewed-narrative-a" || actualModels[1] != "reviewed-narrative-b" {
		t.Fatal("recorded gateway requests did not use both reviewed role models", actualModels)
	}
	stale, err := admit("reviewed-stale")
	if err != nil {
		t.Fatal(err)
	}
	selectNarrativePack(t, raw, execute.Tenant(), packA, 3)
	before := model.requests.Load()
	_, err = runs.Run(context.Background(), execute, stale.ID, false)
	if !errors.Is(err, reporting.ErrStale) || model.requests.Load() != before {
		t.Fatal("selection changed after seal without a closed failure", err)
	}
	selectNarrativePack(t, raw, execute.Tenant(), packRejected, 4)
	if _, err = admit("rejected-selection"); err == nil || model.requests.Load() != before {
		t.Fatal("rejected selected pack admitted or reached provider", err)
	}
	request.PartialPolicy = "allow_partial"
	if _, err = admit("rejected-selection-partial"); err == nil || model.requests.Load() != before {
		t.Fatal("partial mode hid a rejected selected pack", err)
	}
	request.PartialPolicy = "fail"
	selectNarrativePack(t, raw, execute.Tenant(), packUnbound, 5)
	if _, err = admit("unbound-narrative-role"); err == nil || model.requests.Load() != before {
		t.Fatal("accepted pack without a narrative role reached reporting", err)
	}
	denied := phase27Actor(t, f, execute.User(), []string{"reporting.execute", "cw.block.execute:*", "cw.source.query:*", "cw.dataset.query:*", "cw.execution_context.use:other-context"})
	request.Key = "unauthorized-selection"
	if _, err = runs.Admit(context.Background(), denied, created.State.ID, request); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) {
		t.Fatal("unauthorized context reached pack selection", err)
	}
	if model.requests.Load() != before {
		t.Fatal("negative tests reached the model provider")
	}
}
