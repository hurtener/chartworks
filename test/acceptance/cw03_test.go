package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

// Classifications are authored before normal topic publication. No test changes
// immutable published topic bytes or grants a label authority over source data.
func newCW03Fixture(t *testing.T, sensitivity, relatedSensitivity map[string]semantics.LiteralSensitivity) *phase17Fixture {
	t.Helper()
	f, draftsService, topicsService, model, pack := publicationFixture(t)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	pack = phase17EnrichPack(t, f, pack)
	for i := range pack.Datasets {
		for j := range pack.Datasets[i].Columns {
			column := &pack.Datasets[i].Columns[j]
			column.Sensitivity = sensitivity[column.SourceName]
		}
	}
	phase17PublishTopic(t, draftsService, topicsService, e, pack)
	related := cloneTopic(t, pack)
	related.Topic, related.Name = "cw03-related", "Related synthetic observations"
	if relatedSensitivity != nil {
		for i := range related.Datasets {
			for j := range related.Datasets[i].Columns {
				column := &related.Datasets[i].Columns[j]
				column.Sensitivity = relatedSensitivity[column.SourceName]
			}
		}
	}
	phase17PublishTopic(t, draftsService, topicsService, e, related)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	route, err := nlqroute.New(topicsService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	return &phase17Fixture{f: f, e: e, pack: pack, related: related, service: route, model: model, context: pack.Datasets[0].Source.Context}
}

func cw03Intent(name string, order int, enabled, selected bool) *reporting.OutputIntent {
	return &reporting.OutputIntent{
		Metadata: []reporting.OutputLocalized{
			{Locale: "en-US", Name: name, Description: "Bounded retained observations"},
			{Locale: "es-AR", Name: "Observaciones " + name, Description: "Observaciones retenidas y acotadas"},
		},
		Enabled: enabled, DefaultSelected: selected, DisplayOrder: order,
	}
}

func cw03Definition(t *testing.T, base reporting.Definition) reporting.Definition {
	t.Helper()
	d := phase27Copy(t, base)
	d.SchemaVersion = reporting.CurrentSchemaVersion
	d.Outputs[0].Intent = cw03Intent("Table", 20, true, true)
	d.Outputs[1].Intent = cw03Intent("Narrative", 30, true, false)
	n := d.Outputs[1].Narrative
	n.Instructions, n.SchemaVersion, n.MaxClaims, n.MaxTokens = "", "grounded-narrative-v1", 1, 8192
	extra := phase27Copy(t, d.Outputs[0])
	extra.ID, extra.Intent = "table-extra", cw03Intent("Earlier table", 5, true, false)
	disabled := phase27Copy(t, d.Outputs[0])
	disabled.ID, disabled.Intent = "table-disabled", cw03Intent("Disabled table", 40, false, true)
	d.Outputs = append(d.Outputs, extra, disabled)
	return d
}

func cw03QueryLimits(rows int) *reporting.QueryLimits {
	return &reporting.QueryLimits{MaxRows: rows, MaxBytes: 65536, TimeoutMillis: 30000, PlannerCost: 100000, MaxAttempts: 1}
}

func cw03IDs(outputs []reporting.OutputSummary) []string {
	ids := make([]string, len(outputs))
	for i, output := range outputs {
		ids[i] = output.ID
	}
	return ids
}

// TestCW03 uses the real metadata database, least-privilege native warehouse
// executor, publication lifecycle, operation ledger, and recorded provider wire.
func TestCW03(t *testing.T) {
	f := newCW03Fixture(t, map[string]semantics.LiteralSensitivity{"id": semantics.LiteralSensitive, "amount": semantics.LiteralNonSensitive}, nil)
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	base := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id /* CW03_SQL_CANARY */")
	limits := config.DefaultReportingExecution()
	runs := phase28RunService(t, f, blocks, f.f.db, nil, limits)
	create := func(t *testing.T, id string, d reporting.Definition, publish bool) reporting.View {
		t.Helper()
		v, createErr := blocks.Create(ctx, author, reporting.CreateRequest{ID: id, Definition: d})
		if createErr != nil {
			t.Fatal("create", createErr)
		}
		if publish {
			phase27ValidatePublish(t, blocks, author, v)
		} else if _, validateErr := blocks.Validate(ctx, author, id, reporting.ValidateRequest{ExpectedVersion: v.State.Version}); validateErr != nil {
			t.Fatal("validate", validateErr)
		}
		return v
	}
	admit := func(t *testing.T, service *reporting.Runs, block, key string, request reporting.RunRequest) reporting.RunView {
		t.Helper()
		request.Key = key
		v, admitErr := service.Admit(ctx, execute, block, request)
		if admitErr != nil {
			t.Fatal("admit", admitErr)
		}
		return v
	}
	run := func(t *testing.T, service *reporting.Runs, id string, resume bool) reporting.RunView {
		t.Helper()
		v, runErr := service.Run(ctx, execute, id, resume)
		if runErr != nil || v.State != "succeeded" {
			t.Fatalf("run state=%s code=%s error=%v", v.State, v.Code, runErr)
		}
		return v
	}

	t.Run("LegacyDefinitionAndExplicitOrder", func(t *testing.T) {
		d := phase27Copy(t, base)
		d.Outputs = d.Outputs[:1]
		second := phase27Copy(t, d.Outputs[0])
		second.ID = "second"
		d.Outputs = append(d.Outputs, second)
		original := reporting.DefinitionDigest(d)
		create(t, "cw03-legacy", d, true)
		stored, readErr := f.f.db.ReadBlock(ctx, author, "cw03-legacy", reporting.Reference{Revision: 1}, reporting.Read)
		if readErr != nil || stored.Revision.Digest != original || !reflect.DeepEqual(stored.Revision.Definition, d) {
			t.Fatal("legacy stored definition was rewritten", readErr)
		}
		for _, selected := range [][]string{nil, {}} {
			key := "cw03-legacy-nil"
			if selected != nil {
				key = "cw03-legacy-empty"
			}
			v := run(t, runs, admit(t, runs, "cw03-legacy", key, reporting.RunRequest{Outputs: selected}).ID, false)
			if !reflect.DeepEqual(cw03IDs(v.Outputs), []string{"table-main", "second"}) || v.SelectionMode != "legacy_all" {
				t.Fatal("legacy all-selection changed", v.Outputs, v.SelectionMode)
			}
		}
		v := run(t, runs, admit(t, runs, "cw03-legacy", "cw03-legacy-explicit", reporting.RunRequest{Outputs: []string{"second", "table-main"}}).ID, false)
		if !reflect.DeepEqual(cw03IDs(v.Outputs), []string{"second", "table-main"}) {
			t.Fatal("legacy explicit frozen order changed", v.Outputs)
		}
	})

	t.Run("LocalizedDefaultsExplicitSelectionAndDisabled", func(t *testing.T) {
		d := cw03Definition(t, base)
		create(t, "cw03-selection", d, true)
		metadata, readErr := blocks.Read(ctx, author, "cw03-selection", reporting.Reference{})
		if readErr != nil || metadata.SchemaVersion != reporting.CurrentSchemaVersion || metadata.Outputs[0].ID != "table-extra" {
			t.Fatal("metadata lost version or authored order", metadata.SchemaVersion, readErr)
		}
		v := run(t, runs, admit(t, runs, "cw03-selection", "cw03-default", reporting.RunRequest{Locale: "es-AR"}).ID, false)
		if !reflect.DeepEqual(cw03IDs(v.Outputs), []string{"table-main"}) || len(v.Selection) != 4 || v.SelectionMode != "default" || v.Selection[0].ID != "table-extra" || v.Selection[0].State != "omitted" || v.Selection[3].State != "disabled" {
			t.Fatal("default/omitted/disabled snapshot lost", v.Selection, v.Outputs)
		}
		if got := reporting.LocalizedOutput(*v.Outputs[0].Intent, "es-MX"); got.Name != "Observaciones Table" || got.Description == "" {
			t.Fatal("localized label/description fallback lost", got)
		}
		explicit := run(t, runs, admit(t, runs, "cw03-selection", "cw03-explicit", reporting.RunRequest{Outputs: []string{"table-main", "table-extra"}}).ID, false)
		if !reflect.DeepEqual(cw03IDs(explicit.Outputs), []string{"table-extra", "table-main"}) || len(explicit.QueryAttempts) != 1 || explicit.ReservedCalls != 0 {
			t.Fatal("v2 display order or one-query fan-out lost", explicit.Outputs, explicit.QueryAttempts)
		}
		for _, tc := range []struct {
			code string
			ids  []string
		}{{"empty", []string{}}, {"unknown", []string{"not-declared"}}, {"duplicate", []string{"table-main", "table-main"}}, {"disabled", []string{"table-disabled"}}} {
			before := f.f.lookups.Load()
			_, selectionErr := runs.Admit(ctx, execute, "cw03-selection", reporting.RunRequest{Key: "cw03-reject-" + tc.code, Outputs: tc.ids})
			var typed *reporting.SelectionError
			if !errors.As(selectionErr, &typed) || typed.Code != tc.code || f.f.lookups.Load() != before {
				t.Fatal("selection did not reject before warehouse work", tc.code, selectionErr)
			}
		}
	})

	t.Run("AcceptedRevisionSelectionLimitsSurviveDriftAndRetry", func(t *testing.T) {
		d := cw03Definition(t, base)
		d.QueryLimits = cw03QueryLimits(1)
		create(t, "cw03-drift", d, true)
		lost := &phase28LostReply{RunRepository: f.f.db}
		crashing := phase28RunService(t, f, blocks, lost, nil, limits)
		request := reporting.RunRequest{Outputs: []string{"table-main", "table-extra"}, QueryLimits: cw03QueryLimits(2)}
		accepted := admit(t, crashing, "cw03-drift", "cw03-drift-key", request)
		if _, runErr := crashing.Run(ctx, execute, accepted.ID, false); !errors.Is(runErr, store.ErrUnavailable) || !lost.lost.Load() {
			t.Fatal("real result checkpoint fault not reached", runErr)
		}
		current, readErr := blocks.Read(ctx, author, "cw03-drift", reporting.Reference{Draft: true})
		if readErr != nil {
			t.Fatal(readErr)
		}
		changed := phase27Copy(t, d)
		changed.Outputs[0].Intent.Enabled = false
		changed.Outputs[2].Intent.DisplayOrder = 50
		changed.QueryLimits = cw03QueryLimits(10)
		draft, editErr := blocks.Edit(ctx, author, "cw03-drift", reporting.EditRequest{ExpectedVersion: current.State.Version, Definition: changed})
		if editErr != nil {
			t.Fatal(editErr)
		}
		phase27ValidatePublish(t, blocks, author, draft)
		before := f.f.lookups.Load()
		recovered := run(t, runs, accepted.ID, true)
		if recovered.ManifestDigest != accepted.ManifestDigest || recovered.Revision != 1 || !reflect.DeepEqual(recovered.Selection, accepted.Selection) || !reflect.DeepEqual(recovered.AcceptedLimits, accepted.AcceptedLimits) || recovered.AcceptedLimits.Query.MaxRows != 1 || len(recovered.QueryAttempts) != 1 || recovered.QueryAttempts[0].Manifest.Limits.Rows != 1 || f.f.lookups.Load() != before {
			t.Fatal("retry drifted from its sealed contract", recovered.State, recovered.Revision, recovered.QueryAttempts)
		}
		request.Key = "cw03-drift-key"
		replayed, replayErr := runs.Admit(ctx, execute, "cw03-drift", request)
		if replayErr != nil || replayed.ID != accepted.ID || replayed.ManifestDigest != accepted.ManifestDigest {
			t.Fatal("admission replay re-resolved publication", replayErr)
		}
		original, readErr := f.f.db.ReadBlock(ctx, author, "cw03-drift", reporting.Reference{Revision: 1}, reporting.Read)
		if readErr != nil || !reflect.DeepEqual(original.Revision.Definition, d) {
			t.Fatal("published definition mutated", readErr)
		}
	})

	t.Run("CurrentCeilingAndReuseSegregation", func(t *testing.T) {
		d := cw03Definition(t, base)
		d.QueryLimits = cw03QueryLimits(2)
		create(t, "cw03-ceilings", d, true)
		first := run(t, runs, admit(t, runs, "cw03-ceilings", "cw03-cap-first", reporting.RunRequest{}).ID, false)
		identical := run(t, runs, admit(t, runs, "cw03-ceilings", "cw03-cap-reuse", reporting.RunRequest{ReuseMaxAgeSeconds: 60}).ID, false)
		if identical.ReusedFrom != first.ID || len(identical.QueryAttempts) != 0 {
			t.Fatal("identical accepted contract was not reused", identical.ReusedFrom)
		}
		different := run(t, runs, admit(t, runs, "cw03-ceilings", "cw03-cap-distinct", reporting.RunRequest{ReuseMaxAgeSeconds: 60, QueryLimits: cw03QueryLimits(1)}).ID, false)
		if different.ReusedFrom != "" || len(different.QueryAttempts) != 1 || different.QueryAttempts[0].Manifest.Limits.Rows != 1 {
			t.Fatal("different accepted cap reused broader evidence", different.ReusedFrom, different.QueryAttempts)
		}
		pending := admit(t, runs, "cw03-ceilings", "cw03-cap-lowered", reporting.RunRequest{ReuseMaxAgeSeconds: 60})
		lowered := limits
		lowered.MaxRows = 1
		bounded := phase28RunService(t, f, blocks, f.f.db, nil, lowered)
		completed := run(t, bounded, pending.ID, false)
		if completed.AcceptedLimits.Query.MaxRows != 2 || completed.ReusedFrom != "" || len(completed.QueryAttempts) != 1 || completed.QueryAttempts[0].Manifest.Limits.Rows != 1 {
			t.Fatal("new deployment ceiling did not narrow execution", completed.AcceptedLimits, completed.QueryAttempts)
		}
	})

	t.Run("ReviewedSensitivityAndManualRedactionBeforeProvider", func(t *testing.T) {
		model := newGatewayFixture(t, nil)
		model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, `{"claims":[{"kind":"value","evidence":["e1"]}]}`))
		service := phase28RunService(t, f, blocks, f.f.db, model.engine, limits)
		d := cw03Definition(t, base)
		n := d.Outputs[1].Narrative
		n.MaxRows, n.MaxBytes, n.MaxCharacters = 1, 512, 500
		create(t, "cw03-sensitive", d, true)
		accepted := admit(t, service, "cw03-sensitive", "cw03-sensitive-key", reporting.RunRequest{Outputs: []string{"table-main", "narrative-main"}, Narrative: true})
		completed := run(t, service, accepted.ID, false)
		if len(completed.QueryAttempts) != 1 || model.requests.Load() != 1 || len(completed.ResultPolicy) != 2 || completed.ResultPolicy[0].Sensitivity != semantics.LiteralSensitive || completed.ResultPolicy[1].Sensitivity != semantics.LiteralNonSensitive {
			t.Fatal("reviewed policy or one-query fan-out missing", completed.ResultPolicy, completed.QueryAttempts)
		}
		reader := phase28Reader(t, f, "cw03-reader", completed.Block, completed.Context)
		output, readErr := service.Output(ctx, reader, completed.ID, "narrative-main")
		if readErr != nil || output.Narrative == nil || len(output.Narrative.Evidence) != 1 || output.Narrative.Evidence[0].Field != "amount" || len(output.Narrative.Claims) != 1 {
			t.Fatal("sensitive values reached retained evidence", readErr)
		}
		model.mu.Lock()
		bodies := append([]string(nil), model.requestBodies...)
		model.mu.Unlock()
		if len(bodies) != 1 || !strings.Contains(bodies[0], `\"field\":\"amount\"`) || strings.Contains(bodies[0], `\"field\":\"id\"`) || strings.Contains(bodies[0], "CW03_SQL_CANARY") {
			t.Fatal("provider input did not use only the permitted retained projection")
		}
		beforeSource, beforeModels := f.f.lookups.Load(), model.requests.Load()
		model.mode.Store("error")
		rebuilt, rebuildErr := service.RebuildOutput(ctx, reader, completed.ID, "narrative-main")
		if rebuildErr != nil || !reflect.DeepEqual(rebuilt, output) || f.f.lookups.Load() != beforeSource || model.requests.Load() != beforeModels {
			t.Fatal("retained narrative regeneration", rebuildErr)
		}
		manual := phase27Copy(t, d)
		manual.Outputs[1].Narrative.RedactedFields = []string{"amount"}
		create(t, "cw03-manual-redaction", manual, true)
		v := admit(t, service, "cw03-manual-redaction", "cw03-manual-key", reporting.RunRequest{Outputs: []string{"table-main", "narrative-main"}, Narrative: true, PartialPolicy: "allow_partial"})
		partial, runErr := service.Run(ctx, execute, v.ID, false)
		if runErr != nil || partial.State != "partial" || model.requests.Load() != beforeModels || partial.ReservedCalls != 0 {
			t.Fatal("manual redaction was applied after provider work", partial.State, runErr)
		}
		failed, readErr := service.Output(ctx, phase28Reader(t, f, "cw03-reader", partial.Block, partial.Context), partial.ID, "narrative-main")
		if readErr != nil || failed.Code != "narrative_evidence_unavailable" || failed.Narrative != nil {
			t.Fatal("redaction exhaustion lacks typed failure", failed.Code, readErr)
		}
	})

	t.Run("PrivateContextAndExpiredArtifactZeroCalls", func(t *testing.T) {
		d := cw03Definition(t, base)
		create(t, "cw03-private", d, false)
		v := run(t, runs, admit(t, runs, "cw03-private", "cw03-private-key", reporting.RunRequest{Policy: "private_preview", Reference: reporting.Reference{Revision: 1}}).ID, false)
		beforeSource, beforeModels := f.f.lookups.Load(), f.model.requests.Load()
		for _, reader := range []identity.Envelope{
			phase28Reader(t, f, execute.User(), v.Block, v.Context),
			phase28Reader(t, f, "cw03-other", v.Block, v.Context),
			phase28Reader(t, f, execute.User(), v.Block, "cw03-other-context"),
			f.f.token.envelope(t, "cw03-foreign-tenant", "cw03-other", phase28Scopes("cw03-foreign-tenant")...),
		} {
			if _, readErr := runs.Get(ctx, reader, v.ID); !errors.Is(readErr, store.ErrNotFound) && !errors.Is(readErr, access.ErrNotFound) {
				t.Fatal("private metadata leaked through labels", readErr)
			}
		}
		if _, readErr := runs.Get(ctx, execute, v.ID); readErr != nil {
			t.Fatal("authorized private read failed", readErr)
		}
		raw := support.Raw(t, f.f.dsn)
		if _, updateErr := raw.Exec(ctx, `UPDATE chartworks.frozen_runs SET payload_expires_at=created_at+interval '1 microsecond' WHERE tenant_id=$1 AND operation_id=$2`, execute.Tenant(), v.ID); updateErr != nil {
			t.Fatal(updateErr)
		}
		if _, readErr := runs.RebuildOutput(ctx, execute, v.ID, "table-main"); !errors.Is(readErr, reporting.ErrExpired) {
			t.Fatal("expired redraw regenerated evidence", readErr)
		}
		if f.f.lookups.Load() != beforeSource || f.model.requests.Load() != beforeModels {
			t.Fatal("private/expired retained path called execution dependencies")
		}
	})

	t.Run("DefinitionRoundTripPreservesAuthoredIntent", func(t *testing.T) {
		d := cw03Definition(t, base)
		d.QueryLimits = cw03QueryLimits(2)
		raw, marshalErr := json.Marshal(d)
		var imported reporting.Definition
		if marshalErr != nil || json.Unmarshal(raw, &imported) != nil || !reflect.DeepEqual(d, imported) {
			t.Fatal("typed authored fields disappeared from wire round trip", marshalErr)
		}
		create(t, "cw03-round-trip", imported, true)
		stored, readErr := f.f.db.ReadBlock(ctx, author, "cw03-round-trip", reporting.Reference{Revision: 1}, reporting.Read)
		if readErr != nil || !reflect.DeepEqual(d, stored.Revision.Definition) || stored.Revision.Digest != reporting.DefinitionDigest(d) {
			t.Fatal("authored fields disappeared at persistence", readErr)
		}
	})
}
