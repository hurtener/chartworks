package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

// The accepted record is created through Phase 24's real review service.
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

func selectNarrativePack(t *testing.T, f *phase17Fixture, raw *pgx.Conn, pack evaluation.RuntimePackRecord, revision int64) {
	t.Helper()
	ctx := context.Background()
	tenant := f.f.e.Tenant()
	proposal := "reviewed-narrative-" + pack.Pack.ID
	author := phase27Actor(t, f, "proposal-author", []string{"ops.write", "cw.tenant.write:" + tenant})
	reviewer := phase27Actor(t, f, "proposal-reviewer", []string{"ops.audit", "cw.tenant.certify:" + tenant})
	selector := phase27Actor(t, f, "proposal-selector", []string{"ops.write", "cw.tenant.write:" + tenant})
	scope, err := store.NewScope(tenant, author.User())
	if err != nil {
		t.Fatal(err)
	}
	digestA, digestB, evidence, lineage := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64), strings.Repeat("d", 64)
	p := evaluation.OptimizationProposal{SchemaVersion: evaluation.SchemaVersion, ID: proposal, SuiteID: "narrative-suite", SuiteRevision: 1,
		SuiteDigest: digestA, Mode: evaluation.Live, Seed: 1, State: "candidate", CreatedAt: time.Now().UTC(),
		Baseline:  evaluation.CandidateScore{ID: "baseline-" + pack.Pack.ID, PackDigest: digestB, Passed: 0, Total: 1, EvidenceHash: evidence, HeldoutLineageDigest: lineage},
		Candidate: evaluation.CandidateScore{ID: "candidate-" + pack.Pack.ID, PackDigest: pack.Pack.Digest, Passed: 1, Total: 1, EvidenceHash: evidence, HeldoutLineageDigest: lineage}}
	svc, err := evaluation.New(f.f.db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.f.db.SaveProposal(ctx, scope, p); err == nil {
		var proposalDigest string
		if err = raw.QueryRow(ctx, `SELECT proposal_digest FROM chartworks.evaluation_proposals WHERE tenant_id=$1 AND proposal_id=$2`, tenant, proposal).Scan(&proposalDigest); err != nil {
			t.Fatal("read stored proposal digest", err)
		}
		if _, err = svc.ReviewOptimization(ctx, reviewer, proposal, evaluation.ProposalReviewRequest{Digest: proposalDigest, Decision: "approve"}); err != nil {
			t.Fatal("review optimization proposal", err)
		}
	} else if errors.Is(err, store.ErrConflict) {
		previous, readErr := f.f.db.ReadProposal(ctx, scope, proposal)
		if readErr != nil || previous.Candidate.PackDigest != pack.Pack.Digest {
			t.Fatal("existing approved proposal did not match the selected pack", readErr)
		}
	} else {
		t.Fatal("save structured proposal fixture", err)
	}
	selection, err := svc.SelectPack(ctx, selector, proposal, revision-1)
	if err != nil {
		t.Fatal("select approved pack", err)
	}
	if selection.Revision != revision || selection.PackDigest != pack.Pack.Digest {
		t.Fatal("selected pack did not advance the expected revision", selection)
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
	selectNarrativePack(t, f, raw, packA, 1)
	legacyPartialRequest := reporting.RunRequest{Key: "legacy-sealed-partial", Narrative: true, PartialPolicy: "allow_partial"}
	legacyPartial, err := plain.Admit(context.Background(), execute, created.State.ID, legacyPartialRequest)
	if err != nil {
		t.Fatal("seal pre-policy narrative", err)
	}
	legacyPartial, err = runs.Run(context.Background(), execute, legacyPartial.ID, false)
	if err != nil || legacyPartial.State != "partial" || len(legacyPartial.QueryAttempts) != 1 || model.requests.Load() != 0 {
		t.Fatal("legacy sealed narrative reached model under reviewed-pack policy", err, legacyPartial.State)
	}
	legacyTable, err := runs.Output(context.Background(), partialReader, legacyPartial.ID, "table-main")
	if err != nil || legacyTable.State != "succeeded" || legacyTable.Chart == nil {
		t.Fatal("legacy deterministic artifact was not retained", err)
	}
	legacyNarrative, err := runs.Output(context.Background(), partialReader, legacyPartial.ID, "narrative-main")
	if err != nil || legacyNarrative.State != "failed" || legacyNarrative.Code != "narrative_unavailable" {
		t.Fatal("legacy narrative did not record a model-free unavailable result", err, legacyNarrative.Code)
	}
	legacyFail, err := plain.Admit(context.Background(), execute, created.State.ID, reporting.RunRequest{Key: "legacy-sealed-fail", Narrative: true, PartialPolicy: "fail"})
	if err != nil {
		t.Fatal("seal fail-policy legacy narrative", err)
	}
	legacyFail, err = runs.Run(context.Background(), execute, legacyFail.ID, false)
	if !errors.Is(err, reporting.ErrIncomplete) || legacyFail.State != "failed" || model.requests.Load() != 0 {
		t.Fatal("fail-policy legacy narrative reached model", err, legacyFail.State)
	}
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
	selectNarrativePack(t, f, raw, packB, 2)
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
	selectNarrativePack(t, f, raw, packA, 3)
	before := model.requests.Load()
	_, err = runs.Run(context.Background(), execute, stale.ID, false)
	if !errors.Is(err, reporting.ErrStale) || model.requests.Load() != before {
		t.Fatal("selection changed after seal without a closed failure", err)
	}
	selectNarrativePack(t, f, raw, packRejected, 4)
	if _, err = admit("rejected-selection"); err == nil || model.requests.Load() != before {
		t.Fatal("rejected selected pack admitted or reached provider", err)
	}
	request.PartialPolicy = "allow_partial"
	if _, err = admit("rejected-selection-partial"); err == nil || model.requests.Load() != before {
		t.Fatal("partial mode hid a rejected selected pack", err)
	}
	request.PartialPolicy = "fail"
	selectNarrativePack(t, f, raw, packUnbound, 5)
	if _, err = admit("unbound-narrative-role"); err == nil || model.requests.Load() != before {
		t.Fatal("accepted pack without a narrative role reached reporting", err)
	}
	selectNarrativePack(t, f, raw, packA, 6)
	if _, err = raw.Exec(context.Background(), `UPDATE chartworks.evaluation_proposals SET review='{}'::jsonb WHERE tenant_id=$1 AND proposal_id=$2`,
		execute.Tenant(), "reviewed-narrative-"+packA.Pack.ID); err != nil {
		t.Fatal("tamper review receipt fixture", err)
	}
	if _, err = admit("tampered-proposal-review"); err == nil || model.requests.Load() != before {
		t.Fatal("selected pack without an exact approved reviewer receipt was admitted", err)
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
