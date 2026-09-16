package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

func cw03Definition(t *testing.T, legacy reporting.Definition) reporting.Definition {
	t.Helper()
	legacy = phase27Copy(t, legacy)
	legacy.Outputs = legacy.Outputs[:1]
	for _, id := range []string{"second", "disabled", "optional"} {
		o := phase27Copy(t, legacy.Outputs[0])
		o.ID = id
		legacy.Outputs = append(legacy.Outputs, o)
	}
	d, err := reporting.MigrateDefinition(legacy)
	if err != nil {
		t.Fatal(err)
	}
	for i := range d.Outputs {
		d.Outputs[i].Intent.Metadata = []reporting.OutputMetadata{{Locale: "en", DisplayName: "Output " + d.Outputs[i].ID, Description: "Synthetic retained evidence"}, {Locale: "es-AR", DisplayName: "Salida " + d.Outputs[i].ID, Description: "Evidencia sintética retenida"}}
	}
	d.Outputs[0].Intent.DisplayOrder = 10
	d.Outputs[1].Intent.DisplayOrder = 0
	d.Outputs[2].Intent.Enabled = false
	d.Outputs[3].Intent.DefaultSelected = false
	d.QueryLimits = &reporting.QueryLimits{MaxRows: 2, MaxBytes: 65536, TimeoutMillis: 10000, QueryAttempts: 1}
	return d
}

// These tests exercise the real publication, PostgreSQL manifest/checkpoint,
// source execution, retry, retained read and composition boundaries. Fixtures
// classify only synthetic data explicitly; production unknowns remain blocked.
func TestCW03ReportingPublicationAndExecution(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	delivery, err := reporting.NewDelivery(f.blocks, f.runs, f.documents, f.compositions, f.f.f.db, config.DefaultReportingViewer())
	if err != nil {
		t.Fatal(err)
	}
	t.Run("LegacyRevisionAndImmutableMigration", func(t *testing.T) {
		legacy := phase27Copy(t, f.base)
		legacy.Outputs = legacy.Outputs[:1]
		created, err := f.blocks.Create(ctx, f.blockAuthor, reporting.CreateRequest{ID: "cw03-legacy", Definition: legacy})
		if err != nil {
			t.Fatal(err)
		}
		state, _ := phase27ValidatePublish(t, f.blocks, f.blockAuthor, created)
		raw := support.Raw(t, f.f.f.dsn)
		var before, after string
		var version int
		query := `SELECT definition::text,definition_version FROM chartworks.block_revisions WHERE tenant_id=$1 AND block_id=$2 AND revision=1`
		if err = raw.QueryRow(ctx, query, f.blockAuthor.Tenant(), state.ID).Scan(&before, &version); err != nil || version != 1 {
			t.Fatal(version, err)
		}
		accepted, err := f.runs.Admit(ctx, f.execute, state.ID, reporting.RunRequest{Key: "cw03-legacy-accepted", Outputs: []string{}})
		if err != nil {
			t.Fatal(err)
		}
		migrated, err := reporting.MigrateDefinition(legacy)
		if err != nil {
			t.Fatal(err)
		}
		migrated.Outputs[0].Intent.Metadata[0].DisplayName = "New publication label"
		draft, err := f.blocks.Edit(ctx, f.blockAuthor, state.ID, reporting.EditRequest{ExpectedVersion: state.Version, Definition: migrated})
		if err != nil {
			t.Fatal(err)
		}
		phase27ValidatePublish(t, f.blocks, f.blockAuthor, draft)
		if err = raw.QueryRow(ctx, query, f.blockAuthor.Tenant(), state.ID).Scan(&after, &version); err != nil || before != after || version != 1 {
			t.Fatal("historical definition rewritten", err)
		}
		if _, err = raw.Exec(ctx, `UPDATE chartworks.block_revisions SET definition=jsonb_set(definition,'{schema_version}','2') WHERE tenant_id=$1 AND block_id=$2 AND revision=1`, f.blockAuthor.Tenant(), state.ID); err == nil {
			t.Fatal("publication mutated in place")
		}
		done, err := f.runs.Run(ctx, f.execute, accepted.ID, false)
		if err != nil || done.Revision != 1 || done.State != "succeeded" || done.Selection == nil || done.Selection.Mode != "legacy_all" || strings.Contains(done.Selection.Choices[0].Intent.Metadata[0].DisplayName, "New publication") {
			t.Fatal("accepted legacy revision drift", done, err)
		}
	})
	t.Run("SelectionLabelsFanoutAndRevisionDrift", func(t *testing.T) {
		d := cw03Definition(t, f.base)
		f.block(t, "cw03-intent", d)
		described, err := delivery.Describe(ctx, f.execute, reporting.DeliveryDescribeRequest{Target: reporting.DeliveryTarget{Kind: "block", ID: "cw03-intent"}, Locale: "es-AR"})
		if err != nil || len(described.Outputs) != 4 || described.Outputs[0].ID != "second" || described.Outputs[0].Title != "Salida second" || described.Outputs[1].State != "disabled" || described.Outputs[2].State != "omitted" {
			t.Fatal("localized selector", described, err)
		}
		before := f.f.f.lookups.Load()
		models := f.f.model.requests.Load()
		for _, tc := range []struct {
			key  string
			ids  []string
			code string
		}{{"empty", []string{}, "output_selection_empty"}, {"duplicate", []string{"table-main", "table-main"}, "output_duplicate"}, {"unknown", []string{"unknown"}, "output_unknown"}, {"disabled", []string{"disabled"}, "output_disabled"}} {
			if _, err := f.runs.Admit(ctx, f.execute, "cw03-intent", reporting.RunRequest{Key: "cw03-reject-" + tc.key, Outputs: tc.ids}); reporting.SelectionErrorCode(err) != tc.code {
				t.Fatal(tc.key, err)
			}
		}
		if f.f.f.lookups.Load() != before || f.f.model.requests.Load() != models {
			t.Fatal("invalid output selection performed source/model work")
		}
		accepted, err := f.runs.Admit(ctx, f.execute, "cw03-intent", reporting.RunRequest{Key: "cw03-default-accepted"})
		if err != nil {
			t.Fatal(err)
		}
		if accepted.Selection == nil || !slices.Equal(accepted.Selection.Selected, []string{"second", "table-main"}) || accepted.QueryLimits == nil || accepted.QueryLimits.QueryAttempts != 1 {
			t.Fatal(accepted)
		}
		current, err := f.blocks.Read(ctx, f.blockAuthor, "cw03-intent", reporting.Reference{})
		if err != nil {
			t.Fatal(err)
		}
		changed := phase27Copy(t, d)
		changed.Outputs[1].Intent.DefaultSelected = false
		changed.Outputs[0].Intent.Metadata[0].DisplayName = "Later draft label"
		draft, err := f.blocks.Edit(ctx, f.blockAuthor, "cw03-intent", reporting.EditRequest{ExpectedVersion: current.State.Version, Definition: changed})
		if err != nil {
			t.Fatal(err)
		}
		phase27ValidatePublish(t, f.blocks, f.blockAuthor, draft)
		done, err := f.runs.Run(ctx, f.execute, accepted.ID, false)
		if err != nil || done.State != "succeeded" || done.Revision != 1 || len(done.QueryAttempts) != 1 || len(done.Outputs) != 2 || done.Outputs[0].ID != "second" || !reflect.DeepEqual(done.Selection, accepted.Selection) {
			t.Fatal("frozen selection or one-query fanout drift", done, err)
		}
		reader := phase28Reader(t, f.f, "cw03-retained-reader", done.Block, done.Context)
		before, models = f.f.f.lookups.Load(), f.f.model.requests.Load()
		for _, id := range []string{"second", "table-main"} {
			out, err := f.runs.Output(ctx, reader, done.ID, id)
			if err != nil || out.Intent == nil || out.Intent.Metadata[1].Locale != "es-AR" || len(out.ResultPolicy) != len(d.ExpectedSchema) {
				t.Fatal(out, err)
			}
			again, err := f.runs.RebuildOutput(ctx, reader, done.ID, id)
			if err != nil || again.Digest != out.Digest {
				t.Fatal("retained redraw changed", err)
			}
		}
		for _, tc := range []struct{ id, code string }{{"disabled", "output_disabled"}, {"optional", "output_not_selected"}} {
			if _, err := f.runs.Output(ctx, reader, done.ID, tc.id); reporting.SelectionErrorCode(err) != tc.code {
				t.Fatal(tc, err)
			}
		}
		if f.f.f.lookups.Load() != before || f.f.model.requests.Load() != models {
			t.Fatal("retained reads/redraws executed source/model")
		}
		wrong := phase28Reader(t, f.f, "cw03-context-reader", done.Block, "other-context")
		if _, err = f.runs.Output(ctx, wrong, done.ID, "second"); err == nil {
			t.Fatal("same-tenant different-context artifact read")
		}
		if _, err = f.runs.Run(ctx, reader, done.ID, false); err == nil {
			t.Fatal("readable metadata authorized execution")
		}
	})
	t.Run("AcceptedCapsLoweredDeploymentReuseAndRetry", func(t *testing.T) {
		d := cw03Definition(t, f.base)
		f.block(t, "cw03-caps", d)
		saved, err := f.runs.Admit(ctx, f.execute, "cw03-caps", reporting.RunRequest{Key: "cw03-cap-one", Limits: &reporting.QueryLimits{MaxRows: 1}})
		if err != nil {
			t.Fatal(err)
		}
		saved, err = f.runs.Run(ctx, f.execute, saved.ID, false)
		if err != nil || saved.State != "succeeded" {
			t.Fatal(saved, err)
		}
		page, err := f.runs.Rows(ctx, f.execute, saved.ID, 0, 1)
		if err != nil || page.TotalRows != 1 {
			t.Fatal(page, err)
		}
		admitted, err := f.runs.Admit(ctx, f.execute, "cw03-caps", reporting.RunRequest{Key: "cw03-cap-before-deployment"})
		if err != nil {
			t.Fatal(err)
		}
		lower := f.limits.Execution
		lower.MaxRows = 1
		lower.PageRows = 1
		bounded := phase28RunService(t, f.f, f.blocks, f.f.f.db, nil, lower)
		completed, err := bounded.Run(ctx, f.execute, admitted.ID, false)
		if err != nil || completed.QueryLimits.MaxRows != 2 || completed.QueryAttempts[0].Rows != 1 {
			t.Fatal("accepted snapshot mutated or runtime ceiling bypassed", completed, err)
		}
		uncapped, err := f.runs.Admit(ctx, f.execute, "cw03-caps", reporting.RunRequest{Key: "cw03-cap-two", ReuseMaxAgeSeconds: 60})
		if err != nil {
			t.Fatal(err)
		}
		uncapped, err = f.runs.Run(ctx, f.execute, uncapped.ID, false)
		if err != nil || uncapped.ReusedFrom == saved.ID {
			t.Fatal("reuse crossed accepted limits", uncapped, err)
		}
		lost := &phase28LostReply{RunRepository: f.f.f.db, kind: "result"}
		crashing := phase28RunService(t, f.f, f.blocks, lost, nil, f.limits.Execution)
		interrupted, err := crashing.Admit(ctx, f.execute, "cw03-caps", reporting.RunRequest{Key: "cw03-cap-interrupted"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = crashing.Run(ctx, f.execute, interrupted.ID, false); !errors.Is(err, store.ErrUnavailable) || !lost.lost.Load() {
			t.Fatal("result checkpoint loss not exercised", err)
		}
		before := f.f.f.lookups.Load()
		if _, err = bounded.Run(ctx, f.execute, interrupted.ID, true); !errors.Is(err, reporting.ErrBudget) {
			t.Fatal("older evidence bypassed lowered runtime cap", err)
		}
		record, err := f.runs.Get(ctx, f.execute, interrupted.ID)
		if err != nil || len(record.QueryAttempts) != 1 || record.QueryLimits.MaxRows != 2 || f.f.f.lookups.Load() != before {
			t.Fatal("retry regenerated evidence or changed accepted limits", record, err)
		}
	})
	t.Run("CompositionPinsSelectionAndQueryLimits", func(t *testing.T) {
		d := cw03Definition(t, f.base)
		f.block(t, "cw03-composition-block", d)
		report := phase29Text("Scoped synthetic evidence")
		report.Widgets = []reporting.Widget{phase29BlockWidget("first", "cw03-composition-block", 0, "table-main"), phase29BlockWidget("second", "cw03-composition-block", 1, "second")}
		for i := range report.Widgets {
			report.Widgets[i].Block.Limits = &reporting.QueryLimits{MaxRows: 1}
		}
		state := f.report(t, "cw03-composition", report, true)
		accepted, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw03-composed-run"})
		if err != nil {
			t.Fatal(err)
		}
		completed, err := f.compositions.Run(ctx, f.execute, accepted.ID, false)
		if err != nil || !completed.Complete {
			t.Fatal(completed, err)
		}
		raw := support.Raw(t, f.f.f.dsn)
		var body []byte
		if err = raw.QueryRow(ctx, `SELECT manifest FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), accepted.ID).Scan(&body); err != nil {
			t.Fatal(err)
		}
		var manifest reporting.CompositionManifest
		if json.Unmarshal(body, &manifest) != nil {
			t.Fatal("manifest decode")
		}
		if len(manifest.Groups) != 1 || manifest.Groups[0].QueryLimits == nil || manifest.Groups[0].QueryLimits.MaxRows != 1 || len(manifest.Groups[0].Outputs) != 2 {
			t.Fatal("fanout group lost limits/union", manifest.Groups)
		}
		for _, w := range completed.Pages[0].Widgets {
			if w.Selection == nil || w.QueryLimits == nil || w.QueryLimits.MaxRows != 1 || len(w.Selection.Selected) != 1 {
				t.Fatal("widget lost its own selection", w)
			}
		}
		before, models := f.f.f.lookups.Load(), f.f.model.requests.Load()
		for _, w := range []string{"first", "second"} {
			payload, err := f.compositions.Widget(ctx, f.execute, accepted.ID, "main", w)
			if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].Intent == nil {
				t.Fatal(payload, err)
			}
		}
		if f.f.f.lookups.Load() != before || f.f.model.requests.Load() != models {
			t.Fatal("composed retained read executed work")
		}
		// Different authored caps must split groups instead of inheriting the
		// broader widget's child execution or reusing its result implicitly.
		report.Widgets[1].Block.Limits.MaxRows = 2
		state, err = f.documents.Edit(ctx, f.author, "report", state.ID, state.Version, reporting.DocumentReference{}, report)
		if err != nil {
			t.Fatal(err)
		}
		state = phase29Publish(t, f.documents, f.author, state)
		next, err := f.compositions.Admit(ctx, f.execute, "report", state.ID, reporting.CompositionRequest{Key: "cw03-composed-split"})
		if err != nil {
			t.Fatal(err)
		}
		if err = raw.QueryRow(ctx, `SELECT manifest FROM chartworks.composition_run_payloads WHERE tenant_id=$1 AND operation_id=$2`, f.execute.Tenant(), next.ID).Scan(&body); err != nil {
			t.Fatal(err)
		}
		if json.Unmarshal(body, &manifest) != nil || len(manifest.Groups) != 2 {
			t.Fatal("different cap widgets shared an execution", manifest.Groups)
		}
	})
}

func TestCW03NarrativeSensitivityAtProviderBoundary(t *testing.T) {
	for _, reviewed := range []bool{true, false} {
		name := "unknown"
		if reviewed {
			name = "reviewed_and_manual_redaction"
		}
		t.Run(name, func(t *testing.T) {
			f := newPhase18FixtureReviewed(t, reviewed)
			ctx := context.Background()
			query, topics := newPhase18Service(t, f)
			blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
			if err != nil {
				t.Fatal(err)
			}
			author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
			execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
			legacy := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id")
			legacy.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
			legacy.Outputs[1].Narrative.Fields = []string{"amount"}
			legacy.Outputs[1].Narrative.RedactedFields = []string{"id"}
			legacy.Outputs[1].Narrative.MaxTokens = 8192
			d, err := reporting.MigrateDefinition(legacy)
			if err != nil {
				t.Fatal(err)
			}
			d.Outputs[1].Narrative.MaxClaims = 1
			created, err := blocks.Create(ctx, author, reporting.CreateRequest{ID: "cw03-narrative", Definition: d})
			if err != nil {
				t.Fatal(err)
			}
			_, evidence := phase27ValidatePublish(t, blocks, author, created)
			wantPolicy := "unknown"
			if reviewed {
				wantPolicy = "allowed"
			}
			if len(evidence.ResultPolicy) != 2 || evidence.ResultPolicy[1].Status != wantPolicy {
				t.Fatal("reviewed policy not retained with validation", evidence.ResultPolicy)
			}
			model := newGatewayFixture(t, nil)
			model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"value","evidence":["e1"]}]}`))
			runs := phase28RunService(t, f, blocks, f.f.db, model.engine, config.DefaultReportingExecution())
			accepted, err := runs.Admit(ctx, execute, created.State.ID, reporting.RunRequest{Key: "cw03-narrative-run", Narrative: true, PartialPolicy: "allow_partial"})
			if err != nil {
				t.Fatal(err)
			}
			done, err := runs.Run(ctx, execute, accepted.ID, false)
			if err != nil || len(done.QueryAttempts) != 1 {
				t.Fatal(done, err)
			}
			out, err := runs.Output(ctx, execute, done.ID, "narrative-main")
			if err != nil {
				t.Fatal(err)
			}
			if reviewed {
				if out.Narrative == nil || done.State != "succeeded" || model.requests.Load() != 1 || len(out.Narrative.Claims) != 1 {
					t.Fatal(out, done, model.requests.Load())
				}
				model.mu.Lock()
				body := strings.Join(model.requestBodies, "\n")
				model.mu.Unlock()
				if !strings.Contains(body, "9007199254740993.125") || strings.Contains(body, `\"field\":\"id\"`) {
					t.Fatal("provider input redaction mismatch", body)
				}
				for _, item := range out.Narrative.Evidence {
					if item.Field == "id" {
						t.Fatal("manual redaction bypassed", item)
					}
				}
			} else if done.State != "partial" || out.Code != "narrative_evidence_unavailable" || out.Narrative != nil || model.requests.Load() != 0 || done.ReservedCalls != 0 {
				t.Fatal("unknown sensitivity reached provider", done, out, model.requests.Load())
			}
			before := f.f.lookups.Load()
			calls := model.requests.Load()
			model.mode.Store("error")
			again, err := runs.RebuildOutput(ctx, execute, done.ID, "narrative-main")
			if err != nil || again.Digest != out.Digest || model.requests.Load() != calls || f.f.lookups.Load() != before {
				t.Fatal("retained narrative depended on provider availability", again, err)
			}
		})
	}
}
