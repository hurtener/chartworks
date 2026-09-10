package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/reportingapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func phase28Scopes(tenant string) []string {
	base := slices.DeleteFunc(phase27Scopes(tenant), func(s string) bool {
		return strings.HasPrefix(s, "reporting.") || strings.HasPrefix(s, "cw.block.") || s == "query.preflight" || s == "query.plan" || s == "query.execute"
	})
	return append(base, "reporting.execute", "reporting.read", "reporting.preview", "reporting.retention", "jobs.read", "jobs.cancel", "cw.block.execute:*", "cw.block.read:*", "cw.block.preview:*", "cw.run.read:*", "cw.tenant.erase:"+tenant)
}

func phase28Reader(t *testing.T, f *phase17Fixture, user, block, contextID string) identity.Envelope {
	t.Helper()
	return phase27Actor(t, f, user, []string{"reporting.read", "cw.block.read:" + block, "cw.execution_context.use:" + contextID})
}

func phase28RunService(t *testing.T, f *phase17Fixture, blocks *reporting.Service, repo reporting.RunRepository, model gateway.Engine, limits config.ReportingExecution) *reporting.Runs {
	t.Helper()
	runner, err := jobs.NewRequestRunner(f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	runs, err := reporting.NewRuns(blocks, repo, runner, model, "policy-v1", limits)
	if err != nil {
		t.Fatal(err)
	}
	return runs
}

func phase28Chat(t *testing.T, model, content string) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"id": "recorded-frozen-narrative", "object": "chat.completion", "model": model,
		"choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": content}, "finish_reason": "stop"}},
		"usage":   map[string]any{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
	})
	if err != nil {
		t.Fatal(err)
	}
	return "chat_raw:" + string(raw)
}

// The underlying store commits the real checkpoint before this adapter simulates
// loss of its reply. Recovery must not repeat the already retained source query.
type phase28LostReply struct {
	reporting.RunRepository
	lost atomic.Bool
	kind string
}

func (r *phase28LostReply) CheckpointFrozenRun(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedRunWrite) (reporting.RunRecord, error) {
	w, err := proof.Checked(inv)
	if err != nil {
		return reporting.RunRecord{}, err
	}
	out, err := r.RunRepository.CheckpointFrozenRun(ctx, inv, proof)
	if err == nil && (w.Kind == r.kind || r.kind == "" && w.Kind == "result") && r.lost.CompareAndSwap(false, true) {
		return reporting.RunRecord{}, store.ErrUnavailable
	}
	return out, err
}

// These criteria use actual PostgreSQL metadata, a least-privilege warehouse
// connection, the native SQL validator/executor, and signed Pengui envelopes.
func TestPhase28(t *testing.T) {
	f := newPhase18Fixture(t)
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	base := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id /* FROZEN_SQL_CANARY */")
	limits := config.DefaultReportingExecution()
	runs := phase28RunService(t, f, blocks, f.f.db, nil, limits)
	create := func(t *testing.T, id string, definition reporting.Definition, publish bool) reporting.View {
		t.Helper()
		v, createErr := blocks.Create(ctx, author, reporting.CreateRequest{ID: id, Definition: definition})
		if createErr != nil {
			t.Fatal("create frozen definition", createErr)
		}
		if publish {
			phase27ValidatePublish(t, blocks, author, v)
		} else if _, validateErr := blocks.Validate(ctx, author, id, reporting.ValidateRequest{ExpectedVersion: v.State.Version}); validateErr != nil {
			t.Fatal("validate private frozen definition", validateErr)
		}
		return v
	}
	tableDefinition := func(t *testing.T) reporting.Definition {
		t.Helper()
		d := phase27Copy(t, base)
		d.Outputs = d.Outputs[:1]
		second := phase27Copy(t, d.Outputs[0])
		second.ID = "table-second"
		d.Outputs = append(d.Outputs, second)
		return d
	}
	admit := func(t *testing.T, service *reporting.Runs, id, key string, request reporting.RunRequest) reporting.RunView {
		t.Helper()
		request.Key = key
		v, admitErr := service.Admit(ctx, execute, id, request)
		if admitErr != nil {
			t.Fatal("admit frozen run", admitErr)
		}
		return v
	}
	run := func(t *testing.T, service *reporting.Runs, id string) reporting.RunView {
		t.Helper()
		v, runErr := service.Run(ctx, execute, id, false)
		if runErr != nil || v.State != "succeeded" {
			t.Fatalf("frozen execution: state=%s code=%s error=%v", v.State, v.Code, runErr)
		}
		return v
	}

	t.Run("AC01", func(t *testing.T) {
		create(t, "p28-model-free", tableDefinition(t), true)
		beforeModels := f.model.requests.Load()
		v := admit(t, runs, "p28-model-free", "p28-model-free-key", reporting.RunRequest{})
		v = run(t, runs, v.ID)
		if len(v.QueryAttempts) != 1 || v.QueryAttempts[0].Number != 1 || v.QueryAttempts[0].RemoteState != "stopped" {
			t.Fatal("one confirmed physical query receipt required", v.QueryAttempts)
		}
		if f.model.requests.Load() != beforeModels || v.ReservedCalls != 0 || v.ReservedTokens != 0 {
			t.Fatal("frozen deterministic execution reached a learned model")
		}
		raw, marshalErr := json.Marshal(v)
		if marshalErr != nil || strings.Contains(string(raw), "FROZEN_SQL_CANARY") || strings.Contains(string(raw), "9007199254740993.125") {
			t.Fatal("summary leaked SQL or retained rows", marshalErr)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		create(t, "p28-fanout", tableDefinition(t), true)
		v := admit(t, runs, "p28-fanout", "p28-fanout-key", reporting.RunRequest{Outputs: []string{"table-second", "table-main"}})
		v = run(t, runs, v.ID)
		if len(v.QueryAttempts) != 1 || len(v.Outputs) != 2 || v.Outputs[0].ID != "table-second" || v.Outputs[1].ID != "table-main" {
			t.Fatal("selected output order or single-query fanout lost", v.Outputs, v.QueryAttempts)
		}
		reader := phase28Reader(t, f, "artifact-reader", v.Block, v.Context)
		beforeSource, beforeModels := f.f.lookups.Load(), f.model.requests.Load()
		for _, output := range v.Outputs {
			retained, readErr := runs.Output(ctx, reader, v.ID, output.ID)
			if readErr != nil || retained.Chart == nil || retained.State != "succeeded" || retained.Digest != output.Digest {
				t.Fatal("selected output not retained", readErr)
			}
			rebuilt, rebuildErr := runs.RebuildOutput(ctx, reader, v.ID, output.ID)
			if rebuildErr != nil || rebuilt.Digest != retained.Digest {
				t.Fatal("deterministic rendition changed", rebuildErr)
			}
		}
		if f.f.lookups.Load() != beforeSource || f.model.requests.Load() != beforeModels {
			t.Fatal("artifact output reads reached execution dependencies")
		}
		if _, badErr := runs.Admit(ctx, execute, v.Block, reporting.RunRequest{Key: "p28-unknown-output", Outputs: []string{"missing-output"}}); !errors.Is(badErr, reporting.ErrInvalid) {
			t.Fatal("unknown saved output accepted", badErr)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		d := tableDefinition(t)
		d.SQL = "SELECT id, amount FROM analytics.sales WHERE id >= $1 ORDER BY id"
		d.Parameters = []reporting.Parameter{{Name: "min_id", Type: "integer", Required: true, Default: &reporting.Value{Literal: "1"}, Min: "1", Max: "2"}}
		create(t, "p28-typed", d, true)
		request := reporting.RunRequest{Arguments: []reporting.Argument{{Name: "min_id", Value: reporting.Value{Literal: "1"}}}, Resolution: reporting.Resolution{Timezone: "America/Argentina/Buenos_Aires"}}
		v := admit(t, runs, "p28-typed", "p28-typed-key", request)
		v = run(t, runs, v.ID)
		reader := phase28Reader(t, f, "typed-reader", v.Block, v.Context)
		page, readErr := runs.Rows(ctx, reader, v.ID, 0, 1)
		if readErr != nil || page.Next == nil || *page.Next != 1 || page.TotalRows != 2 || len(page.Rows) != 1 || !reflect.DeepEqual(page.Schema, d.ExpectedSchema) {
			t.Fatal("ordered typed result paging", page, readErr)
		}
		if string(page.Rows[0][1]) != `"9007199254740993.125"` {
			t.Fatal("large decimal lost exact transport", string(page.Rows[0][1]))
		}
		if len(v.Parameters) != 1 || v.Parameters[0].Digest == "" || v.Timezone != request.Resolution.Timezone {
			t.Fatal("parameter provenance was not sealed", v.Parameters)
		}
		bad := request
		bad.Key = "p28-typed-injection"
		bad.Arguments = []reporting.Argument{{Name: "min_id", Value: reporting.Value{Literal: "1 OR TRUE"}}}
		if _, badErr := runs.Admit(ctx, execute, v.Block, bad); !errors.Is(badErr, reporting.ErrInvalid) {
			t.Fatal("parameter literal became SQL", badErr)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		model := newGatewayFixture(t, nil)
		model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"value","evidence":["e1"]}]}`))
		d := phase27Copy(t, base)
		d.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
		d.Outputs[1].Narrative.MaxTokens = 8192
		d.Outputs[1].Narrative.Fields = []string{"amount"}
		d.Outputs[1].Narrative.RedactedFields = []string{"id"}
		create(t, "p28-narrative", d, true)
		withNarrative := phase28RunService(t, f, blocks, f.f.db, model.engine, limits)
		if _, badErr := withNarrative.Admit(ctx, execute, "p28-narrative", reporting.RunRequest{Key: "p28-narrative-disabled"}); !errors.Is(badErr, reporting.ErrUnavailable) || model.requests.Load() != 0 {
			t.Fatal("narrative ran without explicit enablement", badErr)
		}
		v := admit(t, withNarrative, "p28-narrative", "p28-narrative-key", reporting.RunRequest{Narrative: true})
		v = run(t, withNarrative, v.ID)
		reader := phase28Reader(t, f, "narrative-reader", v.Block, v.Context)
		output, readErr := withNarrative.Output(ctx, reader, v.ID, "narrative-main")
		if readErr != nil || output.Narrative == nil || model.requests.Load() != 1 || len(output.Narrative.Receipt.Calls) != 1 || !strings.Contains(output.Narrative.Text, "9007199254740993.125") {
			t.Fatal("grounded narrative and actual provider usage missing", output, readErr)
		}
		for _, evidence := range output.Narrative.Evidence {
			if evidence.Field == "id" {
				t.Fatal("redacted field reached narrative evidence")
			}
		}
		model.mode.Store("error")
		again, rebuildErr := withNarrative.RebuildOutput(ctx, reader, v.ID, output.ID)
		if rebuildErr != nil || !reflect.DeepEqual(again, output) || model.requests.Load() != 1 {
			t.Fatal("retained narrative was regenerated", rebuildErr)
		}
		model.mode.Store("schema")
		failed := admit(t, withNarrative, "p28-narrative", "p28-narrative-partial", reporting.RunRequest{Narrative: true, PartialPolicy: "allow_partial"})
		partial, runErr := withNarrative.Run(ctx, execute, failed.ID, false)
		if runErr != nil || partial.State != "partial" || partial.ReservedCalls != 1 {
			t.Fatal("narrative failure was hidden or budget refunded", partial, runErr)
		}
		failedOutput, outputErr := withNarrative.Output(ctx, reader, partial.ID, "narrative-main")
		if outputErr != nil || failedOutput.State != "failed" || failedOutput.Narrative == nil || len(failedOutput.Narrative.Receipt.Calls) == 0 || failedOutput.Narrative.Text != "" {
			t.Fatal("failed narrative lost paid usage or exposed unchecked prose", failedOutput, outputErr)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		created := create(t, "p28-recovery", tableDefinition(t), true)
		lost := &phase28LostReply{RunRepository: f.f.db}
		crashing := phase28RunService(t, f, blocks, lost, nil, limits)
		v := admit(t, crashing, created.State.ID, "p28-recovery-key", reporting.RunRequest{})
		if _, runErr := crashing.Run(ctx, execute, v.ID, false); !errors.Is(runErr, store.ErrUnavailable) || !lost.lost.Load() {
			t.Fatal("checkpoint fault was not exercised", runErr)
		}
		recovered, runErr := runs.Run(ctx, execute, v.ID, true)
		if runErr != nil || recovered.State != "succeeded" || len(recovered.QueryAttempts) != 1 || recovered.Attempts != 2 {
			t.Fatal("resume repeated or lost normalized query", recovered, runErr)
		}
		before := f.f.lookups.Load()
		replayed := admit(t, runs, created.State.ID, "p28-recovery-key", reporting.RunRequest{})
		if replayed.ID != recovered.ID || replayed.ManifestDigest != recovered.ManifestDigest || f.f.lookups.Load() != before {
			t.Fatal("request replay re-resolved or executed work")
		}
		if _, conflict := runs.Admit(ctx, execute, created.State.ID, reporting.RunRequest{Key: "p28-recovery-key", Locale: "es-AR"}); !errors.Is(conflict, store.ErrConflict) {
			t.Fatal("changed request reused a reserved key", conflict)
		}

		for _, boundary := range []string{"attempt", "output", "complete"} {
			t.Run("lost_reply_"+boundary, func(t *testing.T) {
				lost := &phase28LostReply{RunRepository: f.f.db, kind: boundary}
				crashing := phase28RunService(t, f, blocks, lost, nil, limits)
				admitted := admit(t, crashing, created.State.ID, "p28-lost-"+boundary, reporting.RunRequest{})
				_, firstErr := crashing.Run(ctx, execute, admitted.ID, false)
				if !lost.lost.Load() || boundary != "complete" && !errors.Is(firstErr, store.ErrUnavailable) || boundary == "complete" && firstErr != nil {
					t.Fatal("fault boundary not reached", boundary, firstErr)
				}
				recovered, resumeErr := runs.Run(ctx, execute, admitted.ID, true)
				if boundary == "attempt" {
					if !errors.Is(resumeErr, reporting.ErrIncomplete) || len(recovered.QueryAttempts) != 1 {
						t.Fatal("lost successful source values were silently rerun", recovered, resumeErr)
					}
				} else if resumeErr != nil || recovered.State != "succeeded" || len(recovered.QueryAttempts) != 1 {
					t.Fatal("durable output/commit recovery repeated query", recovered, resumeErr)
				}
			})
		}
		if _, forged := f.f.db.CheckpointFrozenRun(ctx, jobs.Invocation{}, reporting.PreparedRunWrite{}); forged == nil {
			t.Fatal("forged invocation published a checkpoint")
		}
	})

	t.Run("AC06", func(t *testing.T) {
		create(t, "p28-reuse", tableDefinition(t), true)
		first := run(t, runs, admit(t, runs, "p28-reuse", "p28-reuse-first", reporting.RunRequest{}).ID)
		second := admit(t, runs, "p28-reuse", "p28-reuse-second", reporting.RunRequest{ReuseMaxAgeSeconds: 60})
		second = run(t, runs, second.ID)
		if second.ReusedFrom != first.ID || len(second.QueryAttempts) != 0 || second.Observed == nil || first.Observed == nil || !second.Observed.Equal(*first.Observed) || second.Expires.After(first.Expires) {
			t.Fatal("reuse lost partition, observation, expiry or physical attempt truth", first, second)
		}
		reader := phase28Reader(t, f, "restricted-reader", first.Block, first.Context)
		before := f.f.lookups.Load()
		if _, readErr := runs.Get(ctx, reader, first.ID); readErr != nil {
			t.Fatal("retained read unnecessarily required execute authority", readErr)
		}
		otherContext := phase28Reader(t, f, "restricted-reader", first.Block, "different-context")
		if _, readErr := runs.Get(ctx, otherContext, first.ID); !errors.Is(readErr, store.ErrNotFound) && !errors.Is(readErr, access.ErrNotFound) {
			t.Fatal("same tenant foreign partition disclosed", readErr)
		}
		if f.f.lookups.Load() != before {
			t.Fatal("retained read reached source credentials")
		}
		private := create(t, "p28-private", tableDefinition(t), false)
		preview := run(t, runs, admit(t, runs, private.State.ID, "p28-private-key", reporting.RunRequest{Policy: "private_preview", Reference: reporting.Reference{Revision: 1}}).ID)
		if !preview.Private {
			t.Fatal("private execution lost immutable privacy")
		}
		other := phase27Actor(t, f, "other-preview-reader", []string{"reporting.read", "reporting.preview", "cw.run.read:*", "cw.block.read:*", "cw.block.preview:*", "cw.execution_context.use:*"})
		if _, readErr := runs.Get(ctx, other, preview.ID); !errors.Is(readErr, store.ErrNotFound) && !errors.Is(readErr, access.ErrNotFound) {
			t.Fatal("private preview escaped its original actor/session", readErr)
		}
	})

	t.Run("AC07", func(t *testing.T) {
		create(t, "p28-retention", tableDefinition(t), true)
		v := run(t, runs, admit(t, runs, "p28-retention", "p28-retention-key", reporting.RunRequest{}).ID)
		reader := phase28Reader(t, f, "retention-reader", v.Block, v.Context)
		raw := support.Raw(t, f.f.dsn)
		// Simulate an elapsed payload lifetime in this disposable database.
		// The original immutable manifest and its creation/expiry are untouched.
		if _, updateErr := raw.Exec(ctx, `UPDATE chartworks.frozen_runs SET payload_expires_at=created_at+interval '1 microsecond' WHERE tenant_id=$1 AND operation_id=$2`, execute.Tenant(), v.ID); updateErr != nil {
			t.Fatal(updateErr)
		}
		beforeSource, beforeModels := f.f.lookups.Load(), f.model.requests.Load()
		tombstone, readErr := runs.Get(ctx, reader, v.ID)
		if readErr != nil || tombstone.State != "expired" {
			t.Fatal("expired values did not become a tombstone", tombstone, readErr)
		}
		if _, readErr = runs.Rows(ctx, reader, v.ID, 0, 1); !errors.Is(readErr, reporting.ErrExpired) {
			t.Fatal("expired rows were silently reconstructed", readErr)
		}
		if _, readErr = runs.RebuildOutput(ctx, reader, v.ID, "table-main"); !errors.Is(readErr, reporting.ErrExpired) {
			t.Fatal("expired rendition was silently rebuilt", readErr)
		}
		count, expireErr := runs.Expire(ctx, execute, 100)
		if expireErr != nil || count != 1 {
			t.Fatal("bounded retention sweep", count, expireErr)
		}
		var payloads, outputs int
		if queryErr := raw.QueryRow(ctx, `SELECT (SELECT count(*) FROM chartworks.frozen_run_payloads WHERE tenant_id=$1 AND operation_id=$2),(SELECT count(*) FROM chartworks.frozen_run_outputs WHERE tenant_id=$1 AND operation_id=$2)`, execute.Tenant(), v.ID).Scan(&payloads, &outputs); queryErr != nil || payloads != 0 || outputs != 0 {
			t.Fatal("retention left values or derived outputs", payloads, outputs, queryErr)
		}
		if f.f.lookups.Load() != beforeSource || f.model.requests.Load() != beforeModels {
			t.Fatal("expired artifact access triggered source/model work")
		}
	})

	t.Run("AC08", func(t *testing.T) {
		registry, registryErr := reportingapi.RuntimeRegistry(true, false)
		if registryErr != nil {
			t.Fatal(registryErr)
		}
		// Only retained proposal reads are registered; no managed runner is needed by frozen reporting.
		pipelineService, setupErr := engineering.NewPipelineService(f.f.db, f.f.s, f.f.validator, nil, config.Defaults(), func(string) (string, bool) { return "", false })
		if setupErr != nil {
			t.Fatal(setupErr)
		}
		t.Cleanup(pipelineService.Close)
		proposals, setupErr := engineering.NewAutopilot(f.f.db, pipelineService, config.DefaultAutopilot())
		if setupErr != nil {
			t.Fatal(setupErr)
		}
		t.Cleanup(proposals.Close)
		server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, reportingapi.RuntimeHandler(f.f.token.verifier, runs, proposals, true, false, http.NotFoundHandler())))
		t.Cleanup(server.Close)
		bearer := phase27Token(t, f, execute.User(), execute.Session(), phase28Scopes(execute.Tenant()))
		client, setupErr := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
		if setupErr != nil {
			t.Fatal(setupErr)
		}
		create(t, "p28-http", tableDefinition(t), true)
		admitted, apiErr := client.AdmitReportingRun(ctx, "p28-http", sdk.ReportingRunRequest{Key: "p28-http-key"})
		if apiErr != nil {
			t.Fatalf("HTTP admission: %#v", apiErr)
		}
		completed, apiErr := client.ExecuteReportingRun(ctx, admitted.ID, sdk.ReportingRunDispatch{})
		if apiErr != nil || completed.State != "succeeded" {
			t.Fatal("HTTP execution", completed, apiErr)
		}
		page, apiErr := client.ReportingRunRows(ctx, admitted.ID, 0, 1)
		if apiErr != nil || len(page.Rows) != 1 || page.Next == nil {
			t.Fatal("HTTP paging", page, apiErr)
		}
		output, apiErr := client.ReportingRunOutput(ctx, admitted.ID, "table-main")
		if apiErr != nil || output.Chart == nil {
			t.Fatal("HTTP retained output", apiErr)
		}
		listed, apiErr := client.ListReportingRuns(ctx, "", 100)
		if apiErr != nil {
			t.Fatal("HTTP artifact list", listed, apiErr)
		}

		create(t, "p28-cancel-budget", tableDefinition(t), true)
		v, apiErr := client.AdmitReportingRun(ctx, "p28-cancel-budget", sdk.ReportingRunRequest{Key: "p28-cancel-key"})
		if apiErr != nil {
			t.Fatal(apiErr)
		}
		before := f.f.lookups.Load()
		cancelled, cancelErr := client.CancelReportingRun(ctx, v.ID)
		if cancelErr != nil || cancelled.State != "cancelled" {
			t.Fatal("durable cancellation unreachable", cancelled, cancelErr)
		}
		if _, runErr := runs.Run(ctx, execute, v.ID, false); runErr == nil {
			t.Fatal("cancelled run executed without explicit recovery")
		}
		if f.f.lookups.Load() != before {
			t.Fatal("cancelled pending operation reached warehouse")
		}
		bounded := limits
		bounded.MaxTenantBytes = int64(bounded.MaxArtifactBytes)
		limited := phase28RunService(t, f, blocks, f.f.db, nil, bounded)
		if _, budgetErr := limited.Admit(ctx, execute, "p28-cancel-budget", reporting.RunRequest{Key: "p28-budget-refusal"}); !errors.Is(budgetErr, reporting.ErrBudget) {
			t.Fatal("tenant budget was not reserved before dispatch", budgetErr)
		}
		if _, pageErr := runs.Rows(ctx, execute, v.ID, 0, limits.PageRows+1); !errors.Is(pageErr, reporting.ErrInvalid) {
			t.Fatal("unbounded result page accepted", pageErr)
		}
	})
}
