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
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
)

func TestPhase13(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		limits := config.DefaultPipelines()
		base := engineering.PipelineDefinition{ID: "graph", Name: "Graph", Connection: "workspace", Steps: []engineering.PipelineStep{{ID: "one", Source: "source", Context: "context", SQL: "SELECT id FROM analytics.sales", Inputs: []string{"analytics.sales"}, DependsOn: []string{}, Strategy: "replace", Columns: []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []engineering.PipelineCheck{}}}}
		if err := engineering.ValidatePipelineDefinition(base, limits); err != nil {
			t.Fatal("valid bounded pipeline rejected", err)
		}
		for _, strategy := range []string{"replace", "append", "incremental", "merge", "interval", "scd2"} {
			d := base
			d.Steps = append([]engineering.PipelineStep(nil), base.Steps...)
			d.Steps[0].Strategy = strategy
			d.Steps[0].Key = "id"
			d.Steps[0].TimeColumn, d.Steps[0].Start, d.Steps[0].End = "event_time", "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
			d.Steps[0].Columns = append(d.Steps[0].Columns, engineering.PipelineColumn{Name: "event_time", Type: "timestamptz"})
			if err := engineering.ValidatePipelineDefinition(d, limits); err != nil {
				t.Fatal("declared strategy contract rejected", strategy, err)
			}
		}
		cyclic := base
		cyclic.Steps = []engineering.PipelineStep{
			{ID: "one", SQL: "SELECT id FROM {{step.two}}", DependsOn: []string{"two"}, FromSteps: []string{"two"}, Strategy: "replace", Columns: base.Steps[0].Columns, Checks: []engineering.PipelineCheck{}},
			{ID: "two", SQL: "SELECT id FROM {{step.one}}", DependsOn: []string{"one"}, FromSteps: []string{"one"}, Strategy: "replace", Columns: base.Steps[0].Columns, Checks: []engineering.PipelineCheck{}},
		}
		if !errors.Is(engineering.ValidatePipelineDefinition(cyclic, limits), engineering.ErrInvalid) {
			t.Fatal("cyclic graph accepted")
		}
	})

	t.Run("AC02", func(t *testing.T) {
		model := newGatewayFixture(t, nil)
		f := newPipelineFixture(t, model.engine, func(v *config.Values) {
			v.Features.Gateway = true
			v.Gateway = model.cfg
		})
		source := f.create(t, "proposal-source")
		nlq := f.token.envelope(t, f.e.Tenant(), "reader", "query.execute", "cw.source.query:"+source.ID, "cw.execution_context.use:"+source.ContextID)
		request := engineering.PipelineProposalRequest{ID: "nlq-proposal", Name: "NLQ proposal", Connection: "workspace", Source: source.ID, Context: source.ContextID, Instruction: "Select the synthetic identifier only"}
		if _, err := f.pipelines.Propose(context.Background(), nlq, request); err == nil || model.requests.Load() != 0 {
			t.Fatal("NLQ authority reached the managed-write model consumer", err)
		}
		request.ID, request.Name = "model-proposal", "Model proposal"
		version, err := f.pipelines.Propose(context.Background(), f.e, request)
		if err != nil || version.State != "draft" || version.Version != 1 || version.Published != nil || model.requests.Load() != 1 {
			t.Fatal("model proposal did not create one unapproved draft", err, version)
		}
		model.mu.Lock()
		payload := strings.Join(model.requestBodies, "\n")
		model.mu.Unlock()
		for _, forbidden := range []string{"SYNTHETIC_WRITER_PASSWORD", "PRIMARY_KEY", "PRIVATE_PERSON_CANARY"} {
			if strings.Contains(payload, forbidden) {
				t.Fatal("model draft payload included secret or source value", forbidden)
			}
		}
		model.mode.Store("schema")
		if _, err = f.pipelines.Propose(context.Background(), f.e, engineering.PipelineProposalRequest{ID: "bad-proposal", Name: "Bad", Connection: "workspace", Source: source.ID, Context: source.ContextID, Instruction: "Return an invalid shape"}); !errors.Is(err, gateway.ErrOutput) {
			t.Fatal("malformed model proposal accepted", err)
		}
		if _, err = f.pipelines.Get(context.Background(), f.e, "bad-proposal", 1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("rejected model output persisted a draft", err)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		f := newPipelineFixture(t, nil, nil)
		source := f.create(t, "authority-source")
		definition := f.definition(t, source, "shared-pipeline")
		draft, err := f.pipelines.Draft(context.Background(), f.e, definition, 0)
		if err != nil || draft.State != "draft" {
			t.Fatal("authorized draft", err, draft)
		}
		published, err := f.pipelines.Publish(context.Background(), f.e, definition.ID, draft.Version)
		if err != nil || published.State != "published" || published.Published == nil {
			t.Fatal("authorized publication", err, published)
		}
		reviewer := f.actor(t, f.e.Tenant(), "reviewer")
		read, err := f.pipelines.Get(context.Background(), reviewer, definition.ID, draft.Version)
		if err != nil || read.Digest != published.Digest {
			t.Fatal("same-tenant signed reviewer could not read immutable definition", err, read)
		}
		foreign := f.actor(t, "foreign-tenant", "reviewer")
		if _, err = f.pipelines.Get(context.Background(), foreign, definition.ID, draft.Version); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("cross-tenant definition read did not fail closed", err)
		}
		noContext := f.token.envelope(t, f.e.Tenant(), "limited", "engineering.pipeline.write", "cw.source.write:limited-pipeline", "cw.source.query:"+source.ID)
		limited := definition
		limited.ID = "limited-pipeline"
		if _, err = f.pipelines.Draft(context.Background(), noContext, limited, 0); err == nil {
			t.Fatal("missing signed execution-context reach reached draft validation")
		}
		unowned := definition
		unowned.ID, unowned.Connection = "unowned-pipeline", "unregistered"
		if _, err = f.pipelines.Draft(context.Background(), f.e, unowned, 0); !errors.Is(err, engineering.ErrOwnership) {
			t.Fatal("unregistered managed connection persisted a draft", err)
		}
		if _, err = f.pipelines.Get(context.Background(), f.e, unowned.ID, 1); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("rejected destination left a definition", err)
		}
		f.mu.Lock()
		writerDSN := f.sourceFixture.values["CHARTWORKS_SOURCE_WRITE"]
		f.mu.Unlock()
		writer, err := pgx.Connect(context.Background(), writerDSN)
		if err != nil {
			t.Fatal("managed writer fixture", err)
		}
		defer writer.Close(context.Background())
		if _, err = writer.Exec(context.Background(), "UPDATE analytics.sales SET id=id"); err == nil {
			t.Fatal("managed writer credential modified a baseline object")
		}
	})

	t.Run("AC04", func(t *testing.T) {
		f := newPipelineFixture(t, nil, nil)
		source := f.create(t, "quality-source")
		definition := f.definition(t, source, "quality-pipeline")
		draft, err := f.pipelines.Draft(context.Background(), f.e, definition, 0)
		if err != nil {
			t.Fatal("quality pipeline draft", err)
		}
		if _, err = f.pipelines.Publish(context.Background(), f.e, definition.ID, draft.Version); err != nil {
			t.Fatal("quality pipeline publish", err)
		}
		active, err := f.pipelines.Run(context.Background(), f.e, definition.ID, draft.Version, "quality-baseline", false)
		if err != nil || active.State != "published" || len(active.Effects) != 1 || active.Effects[0].Source == "" || active.Effects[0].Digest == "" {
			t.Fatal("quality baseline activation", err, active)
		}
		oldBinding, err := f.s.Binding(context.Background(), f.e, active.Effects[0].Source, active.Effects[0].Context)
		if err != nil || len(oldBinding.Relations) != 1 {
			t.Fatal("active baseline binding", err, oldBinding)
		}
		metadata := support.Raw(t, f.dsn)
		var activeOperation string
		if err = metadata.QueryRow(context.Background(), `SELECT operation_id FROM chartworks.pipeline_outputs WHERE tenant_id=$1 AND pipeline_id=$2 AND step_id='output'`, f.e.Tenant(), definition.ID).Scan(&activeOperation); err != nil || activeOperation != active.Operation.ID {
			t.Fatal("baseline activation pointer", err, activeOperation)
		}
		definition.Steps = append(definition.Steps, engineering.PipelineStep{ID: "late", SQL: "SELECT id::bigint AS id FROM {{step.output}}", DependsOn: []string{"output"}, FromSteps: []string{"output"}, Inputs: []string{}, Strategy: "replace", Columns: []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}}, Checks: []engineering.PipelineCheck{{Kind: "row_count", Minimum: 3}}})
		next, err := f.pipelines.Draft(context.Background(), f.e, definition, draft.Version)
		if err != nil {
			t.Fatal("late quality draft", err)
		}
		if _, err = f.pipelines.Publish(context.Background(), f.e, definition.ID, next.Version); err != nil {
			t.Fatal("late quality publication", err)
		}
		run, err := f.pipelines.Run(context.Background(), f.e, definition.ID, next.Version, "quality-failure", false)
		effects := map[string]engineering.PipelineEffect{}
		for _, effect := range run.Effects {
			effects[effect.Step] = effect
		}
		if !errors.Is(err, engineering.ErrPipelineQuality) || run.State != "quality_failed" || len(effects) != 2 || effects["output"].State != "checked" || effects["output"].Digest == "" || effects["late"].State != "quality_failed" || effects["late"].Digest != "" {
			t.Fatal("failed check silently activated or lost staged evidence", err, run)
		}
		for _, effect := range run.Effects {
			if effect.Source != "" || effect.Context != "" {
				t.Fatal("partial staged output projected as active", effect)
			}
		}
		retained, err := f.pipelines.InspectRun(context.Background(), f.e, run.Operation.ID)
		retainedEffects := map[string]engineering.PipelineEffect{}
		for _, effect := range retained.Effects {
			retainedEffects[effect.Step] = effect
		}
		if err != nil || retained.State != "quality_failed" || len(retainedEffects) != 2 || retainedEffects["output"].State != "checked" || retainedEffects["late"].State != "quality_failed" {
			t.Fatal("quality failure receipt was not retained privately", err, retained)
		}
		stillActive, err := f.s.Binding(context.Background(), f.e, active.Effects[0].Source, active.Effects[0].Context)
		if err != nil || len(stillActive.Relations) != 1 || stillActive.Relations[0].ID != oldBinding.Relations[0].ID {
			t.Fatal("late failed version displaced the prior active generation", err, stillActive)
		}
		if err = metadata.QueryRow(context.Background(), `SELECT operation_id FROM chartworks.pipeline_outputs WHERE tenant_id=$1 AND pipeline_id=$2 AND step_id='output'`, f.e.Tenant(), definition.ID).Scan(&activeOperation); err != nil || activeOperation != active.Operation.ID {
			t.Fatal("late failed version changed active pointer", err, activeOperation)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		tempDir := os.Getenv("CHARTWORKS_TEST_PIPELINE_TMPDIR")
		if !filepath.IsAbs(tempDir) {
			t.Fatal("CHARTWORKS_TEST_PIPELINE_TMPDIR must name the executable private tmpfs")
		}
		caseDir, err := os.MkdirTemp(tempDir, "cw-runner-negative-")
		if err != nil {
			t.Fatal(err)
		}
		defer os.RemoveAll(caseDir)
		mode, capture, runner := filepath.Join(caseDir, "mode"), filepath.Join(caseDir, "capture"), filepath.Join(caseDir, "runner")
		quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
		script := "#!/bin/sh\n" +
			"printf 'HOME=%s\\nTMPDIR=%s\\nTELEMETRY_OPTOUT=%s\\n' \"$HOME\" \"$TMPDIR\" \"$TELEMETRY_OPTOUT\" >>" + quote(capture) + "\n" +
			"printf 'ARG=<%s>\\n' \"$@\" >>" + quote(capture) + "\n" +
			"case \"$(cat " + quote(mode) + ")\" in\n" +
			"critical) printf '%s\\n' '[{\"pipeline\":\"synthetic\",\"issues\":{\"critical\":[{\"message\":\"rejected\"}]}}]'; exit 0;;\n" +
			"timeout) sleep 5; exit 0;;\n" +
			"crash) exit 17;;\n" +
			"esac\n"
		if err = os.WriteFile(runner, []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(script))
		f := newPipelineFixture(t, nil, func(v *config.Values) {
			v.Pipelines.RunnerPath = runner
			v.Pipelines.RunnerSHA256 = hex.EncodeToString(sum[:])
			v.Pipelines.Timeout = config.Duration(time.Second)
		})
		source := f.create(t, "runner-negative-source")
		published := func(id string) int64 {
			draft, err := f.pipelines.Draft(context.Background(), f.e, f.definition(t, source, id), 0)
			if err != nil {
				t.Fatal("negative runner draft", id, err)
			}
			if _, err = f.pipelines.Publish(context.Background(), f.e, id, draft.Version); err != nil {
				t.Fatal("negative runner publication", id, err)
			}
			return draft.Version
		}
		cancelVersion := published("cancel-before-dispatch")
		admitted, err := f.pipelines.AdmitRun(context.Background(), f.e, "cancel-before-dispatch", cancelVersion, "cancel-before-dispatch-key")
		if err != nil || admitted.Operation.ID == "" {
			t.Fatal("admit-only operation", err, admitted)
		}
		if _, err = f.pipelines.Cancel(context.Background(), f.e, admitted.Operation.ID); err != nil {
			t.Fatal("cancel admitted operation", err)
		}
		if _, err = f.pipelines.Run(context.Background(), f.e, "cancel-before-dispatch", cancelVersion, "cancel-before-dispatch-key", false); err == nil {
			t.Fatal("cancelled operation dispatched")
		}
		if raw, readErr := os.ReadFile(capture); readErr == nil && len(raw) != 0 {
			t.Fatal("cancel-before-dispatch invoked runner", string(raw))
		}
		for _, tc := range []struct {
			mode, id string
			want     error
		}{{"critical", "critical-output", engineering.ErrInvalid}, {"timeout", "runner-timeout", context.DeadlineExceeded}, {"crash", "runner-crash", engineering.ErrUnavailable}} {
			if err = os.WriteFile(mode, []byte(tc.mode), 0600); err != nil {
				t.Fatal(err)
			}
			version := published(tc.id)
			run, runErr := f.pipelines.Run(context.Background(), f.e, tc.id, version, tc.id+"-key", false)
			if !errors.Is(runErr, tc.want) || run.State == "published" || len(run.Effects) != 1 || run.Effects[0].Source != "" {
				t.Fatal("invalid runner outcome activated an output", tc.mode, runErr, run)
			}
		}
		raw, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal("runner invocation capture", err)
		}
		text := string(raw)
		if strings.Contains(text, "SYNTHETIC_WRITER_PASSWORD") || !strings.Contains(text, "TELEMETRY_OPTOUT=true") || !strings.Contains(text, "TMPDIR="+tempDir) || !strings.Contains(text, "HOME="+tempDir) {
			t.Fatal("runner invocation escaped private custody or exposed secret", text)
		}
	})

	t.Run("AC06", func(t *testing.T) {
		f := newPipelineFixture(t, nil, nil)
		source := f.create(t, "strategy-source")
		expectedSecond := map[string]int64{"replace": 2, "append": 4, "incremental": 3, "merge": 3, "interval": 2, "scd2": 4}
		for strategy, expectedRows := range expectedSecond {
			strategy, expectedRows := strategy, expectedRows
			t.Run(strategy, func(t *testing.T) {
				if _, err := f.admin.Exec(context.Background(), `TRUNCATE analytics.sales; INSERT INTO analytics.sales(id,amount,created_at) VALUES(1,NULL,'2026-01-02T03:04:05Z'),(2,5.5,'2026-01-03T03:04:05Z')`); err != nil {
					t.Fatal("reset strategy input", err)
				}
				definition := f.definition(t, source, "strategy-"+strategy)
				step := &definition.Steps[0]
				step.SQL = "SELECT id::bigint AS id, amount, created_at FROM analytics.sales"
				step.Strategy = strategy
				step.Columns = []engineering.PipelineColumn{{Name: "id", Type: "bigint", PrimaryKey: true}, {Name: "amount", Type: "numeric"}, {Name: "created_at", Type: "timestamptz"}}
				switch strategy {
				case "incremental":
					step.Key = "id"
				case "interval":
					step.TimeColumn, step.Start, step.End = "created_at", "2020-01-01T00:00:00Z", "2030-01-01T00:00:00Z"
				}
				draft, err := f.pipelines.Draft(context.Background(), f.e, definition, 0)
				if err != nil {
					t.Fatal("strategy draft", err)
				}
				if _, err = f.pipelines.Publish(context.Background(), f.e, definition.ID, draft.Version); err != nil {
					t.Fatal("strategy publication", err)
				}
				first, err := f.pipelines.Run(context.Background(), f.e, definition.ID, draft.Version, strategy+"-first", false)
				if err != nil || first.State != "published" || len(first.Effects) != 1 || first.Effects[0].Rows != 2 || first.Effects[0].Source == "" || first.Effects[0].Context == "" || first.Effects[0].Digest == "" {
					t.Fatal("initial strategy semantics", err, first)
				}
				if _, err = f.admin.Exec(context.Background(), `UPDATE analytics.sales SET amount=42 WHERE id=1; DELETE FROM analytics.sales WHERE id=2; INSERT INTO analytics.sales(id,amount,created_at) VALUES(3,7,'2026-01-04T03:04:05Z')`); err != nil {
					t.Fatal("change strategy input", err)
				}
				second, err := f.pipelines.Run(context.Background(), f.e, definition.ID, draft.Version, strategy+"-second", false)
				if err != nil || second.State != "published" || len(second.Effects) != 1 || second.Effects[0].Rows != expectedRows || second.Effects[0].Source != first.Effects[0].Source || second.Effects[0].Context == first.Effects[0].Context || second.Effects[0].Digest == "" {
					t.Fatal("second-run strategy semantics", err, second)
				}
				replayed, err := f.pipelines.Run(context.Background(), f.e, definition.ID, draft.Version, strategy+"-second", false)
				if err != nil || replayed.Operation.ID != second.Operation.ID || replayed.Effects[0].Digest != second.Effects[0].Digest {
					t.Fatal("accepted operation key replayed physical work", err, replayed)
				}
				binding, err := f.s.Binding(context.Background(), f.e, second.Effects[0].Source, second.Effects[0].Context)
				if err != nil || len(binding.Relations) != 1 {
					t.Fatal("published output binding", err, binding)
				}
				table := pgx.Identifier{binding.Relations[0].Schema, binding.Relations[0].Name}.Sanitize()
				var exact bool
				switch strategy {
				case "replace", "interval":
					err = f.admin.QueryRow(context.Background(), `SELECT count(*)=2 AND count(*) FILTER (WHERE id=1 AND amount=42)=1 AND count(*) FILTER (WHERE id=3 AND amount=7)=1 AND count(*) FILTER (WHERE id=2)=0 FROM `+table).Scan(&exact)
				case "append":
					err = f.admin.QueryRow(context.Background(), `SELECT count(*)=4 AND count(*) FILTER (WHERE id=1 AND amount IS NULL)=1 AND count(*) FILTER (WHERE id=1 AND amount=42)=1 AND count(*) FILTER (WHERE id=2 AND amount=5.5)=1 AND count(*) FILTER (WHERE id=3 AND amount=7)=1 FROM `+table).Scan(&exact)
				case "incremental", "merge":
					err = f.admin.QueryRow(context.Background(), `SELECT count(*)=3 AND count(*) FILTER (WHERE id=1 AND amount=42)=1 AND count(*) FILTER (WHERE id=2 AND amount=5.5)=1 AND count(*) FILTER (WHERE id=3 AND amount=7)=1 FROM `+table).Scan(&exact)
				case "scd2":
					err = f.admin.QueryRow(context.Background(), `SELECT count(*)=4 AND count(*) FILTER (WHERE id=1 AND amount IS NULL AND NOT _is_current AND _valid_until IS NOT NULL)=1 AND count(*) FILTER (WHERE id=1 AND amount=42 AND _is_current)=1 AND count(*) FILTER (WHERE id=2 AND amount=5.5 AND NOT _is_current AND _valid_until IS NOT NULL)=1 AND count(*) FILTER (WHERE id=3 AND amount=7 AND _is_current)=1 AND (SELECT _valid_until FROM `+table+` WHERE id=1 AND amount IS NULL)=(SELECT _valid_from FROM `+table+` WHERE id=1 AND amount=42) AND (SELECT _valid_until FROM `+table+` WHERE id=2)=(SELECT _valid_from FROM `+table+` WHERE id=1 AND amount=42) FROM `+table).Scan(&exact)
				}
				if err != nil || !exact {
					if strategy == "scd2" {
						rows, queryErr := f.admin.Query(context.Background(), `SELECT id,coalesce(amount::text,'NULL'),_valid_from::text,coalesce(_valid_until::text,'NULL'),_is_current FROM `+table+` ORDER BY id,_valid_from`)
						actual := []string{}
						if queryErr == nil {
							for rows.Next() {
								var id int64
								var amount, from, until string
								var current bool
								if scanErr := rows.Scan(&id, &amount, &from, &until, &current); scanErr != nil {
									queryErr = scanErr
									break
								}
								actual = append(actual, fmt.Sprintf("%d|%s|%s|%s|%t", id, amount, from, until, current))
							}
							rows.Close()
						}
						t.Fatalf("strategy lost NULL-safe history semantics: predicate=%t query=%v rows=%v", exact, queryErr, actual)
					}
					t.Fatal("strategy lost update/insert/delete or history semantics", err, strategy)
				}
			})
		}
	})
}
