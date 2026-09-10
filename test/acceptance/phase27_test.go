package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/chartdata"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func phase27Scopes(tenant string) []string {
	// Reporting does not submit query feedback. Keep the issuer's 32-scope
	// ceiling intact rather than broadening production authority for the fixture.
	base := slices.DeleteFunc(phase18Scopes(tenant, true), func(scope string) bool { return scope == "feedback.write" })
	return append(base, "reporting.read", "reporting.write", "reporting.validate", "reporting.preview", "reporting.publish", "reporting.certify", "cw.block.read:*", "cw.block.write:*", "cw.block.preview:*", "cw.block.publish:*", "cw.block.certify:*")
}

func phase27Token(t *testing.T, f *phase17Fixture, user, session string, scopes []string) string {
	t.Helper()
	claims := f.f.token.claims(f.f.e.Tenant(), user, scopes)
	claims["session"] = session
	return f.f.token.sign(t, claims, nil)
}

func phase27Actor(t *testing.T, f *phase17Fixture, user string, scopes []string) identity.Envelope {
	t.Helper()
	e, err := f.f.token.verifier.Verify(context.Background(), phase27Token(t, f, user, "phase27-session", scopes), auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func phase27Copy[T any](t *testing.T, value T) T {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func phase27Definition(t *testing.T, f *phase17Fixture, e identity.Envelope, sql string) reporting.Definition {
	t.Helper()
	ctx := context.Background()
	_, topicsService := newPhase18Service(t, f)
	published, err := topicsService.Read(ctx, e, f.pack.Topic, f.pack.Version)
	if err != nil {
		t.Fatal(err)
	}
	binding := published.Definition.Datasets[0].Source
	plan, err := f.f.validator.Validate(ctx, e, readexec.Request{Source: binding.Source, Context: binding.Context, SQL: sql})
	if err != nil {
		t.Fatal("fixture source validation", err)
	}
	report, err := f.f.executor.Execute(ctx, e, plan, readexec.Options{Operation: "block-schema-" + fmt.Sprint(time.Now().UnixNano()), Number: 1, Preview: true, Rows: 10, Bytes: 65536})
	if err != nil || report.Result == nil {
		t.Fatalf("fixture observed schema: %#v %v", report, err)
	}
	data, err := chartdata.FromReadResult(ctx, *report.Result, charts.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	columnIDs := []string{}
	fields := []string{}
	for _, column := range data.Columns {
		columnIDs = append(columnIDs, column.ID)
		fields = append(fields, column.Name)
	}
	mapping := charts.Mapping{Version: charts.Version, Kind: charts.Table, Columns: data.Columns, Bindings: charts.Bindings{Columns: columnIDs}, Options: charts.DefaultOptions()}
	if err := charts.ValidateMapping(ctx, data, mapping, charts.Defaults()); err != nil {
		t.Fatal("fixture output", err)
	}
	return reporting.Definition{SchemaVersion: reporting.SchemaVersion,
		Metadata: []reporting.Localized{{Locale: "en-US", Title: "Revenue evidence", Question: "What is revenue by sale?", Aliases: []string{"Revenue for each transaction"}, Description: "Synthetic governed output"}, {Locale: "es-AR", Title: "Ingresos", Question: "¿Cuál es el ingreso por venta?", Aliases: []string{"Ingresos de cada transacción"}, Description: "Datos sintéticos"}},
		Source:   binding.Source, Context: binding.Context, Topics: []reporting.TopicPin{{Topic: published.Definition.Topic, Version: published.Definition.Version, Digest: published.Digest}}, SQL: sql,
		Parameters: []reporting.Parameter{}, ExpectedSchema: report.Result.Schema,
		Outputs: []reporting.Output{{ID: "table-main", Kind: "table", Mapping: &mapping}, {ID: "narrative-main", Kind: "narrative", Narrative: &reporting.Narrative{Type: "summary", Instructions: "Describe the displayed evidence without inventing values.", Fields: fields, RedactedFields: []string{}, Reduction: "first_rows", MaxRows: 10, MaxBytes: 4096, MaxCharacters: 1000, MaxCalls: 1, MaxTokens: 1024, TimeoutMillis: 10000, PromptVersion: "summary-v1", ModelVersion: "policy-v1", SchemaVersion: "narrative-v1", Locale: "en-US", Tone: "neutral", RequireEvidence: true, RequireCaveats: true}}},
	}
}

func phase27ValidatePublish(t *testing.T, service *reporting.Service, e identity.Envelope, view reporting.View) (reporting.State, reporting.Evidence) {
	t.Helper()
	validation, err := service.Validate(context.Background(), e, view.State.ID, reporting.ValidateRequest{ExpectedVersion: view.State.Version})
	if err != nil {
		t.Fatal("validate exact draft", err)
	}
	state, err := service.Publish(context.Background(), e, view.State.ID, reporting.PublishRequest{ExpectedVersion: validation.State.Version, Evidence: validation.Evidence.ID})
	if err != nil {
		t.Fatal("publish exact evidence", err)
	}
	return state, validation.Evidence
}

// TestPhase27 uses a real PostgreSQL metadata database, least-privilege source
// connection, native validator/executor and cryptographically verified authority.
func TestPhase27(t *testing.T) {
	f := newPhase18Fixture(t)
	queryService, topicService := newPhase18Service(t, f)
	service, err := reporting.New(f.f.db, topicService, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(queryService), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	e := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	ctx := context.Background()
	base := phase27Definition(t, f, e, "SELECT id, amount FROM analytics.sales ORDER BY id /* BLOCK_SQL_CANARY */")
	create := func(t *testing.T, id string) reporting.View {
		t.Helper()
		out, err := service.Create(ctx, e, reporting.CreateRequest{ID: id, Definition: phase27Copy(t, base)})
		if err != nil {
			t.Fatal("create", id, err)
		}
		return out
	}

	t.Run("AC01", func(t *testing.T) {
		created := create(t, "p27-identity")
		if created.State.Version != 1 || created.Revision != 1 || created.RevisionID == "" || !created.Private || len(created.Outputs) != 2 || created.Outputs[0].ID != "table-main" {
			t.Fatal("unstable coordinates", created)
		}
		stored, err := service.Read(ctx, e, created.State.ID, reporting.Reference{Draft: true})
		if err != nil || stored.Digest != created.Digest || !reflect.DeepEqual(stored.Metadata, base.Metadata) {
			t.Fatal(stored, err)
		}
		raw, _ := json.Marshal(stored)
		if bytes.Contains(raw, []byte("BLOCK_SQL_CANARY")) || bytes.Contains(raw, []byte(`"sql"`)) {
			t.Fatal("default reader exposed SQL")
		}
		if _, err := service.Create(ctx, e, reporting.CreateRequest{ID: created.State.ID, Definition: base}); !errors.Is(err, store.ErrConflict) {
			t.Fatal("duplicate identity accepted", err)
		}
		if _, err := service.Read(ctx, e, created.State.ID, reporting.Reference{}); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("unpublished draft became default", err)
		}
		// A valid issuer signature never grants another tenant or execution
		// partition access, even with the same actor and wildcard block reach.
		claims := f.f.token.claims("p27-foreign-tenant", e.User(), phase27Scopes("p27-foreign-tenant"))
		claims["session"] = "phase27-session"
		foreign, err := f.f.token.verifier.Verify(ctx, f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal("verify foreign tenant fixture", err)
		}
		narrowScopes := phase27Scopes(e.Tenant())
		for i, scope := range narrowScopes {
			if strings.HasPrefix(scope, "cw.execution_context.use:") {
				narrowScopes[i] = "cw.execution_context.use:p27-other-context"
			}
		}
		narrow := phase27Actor(t, f, e.User(), narrowScopes)
		before := f.f.lookups.Load()
		for _, denied := range []identity.Envelope{foreign, narrow} {
			if _, err := service.Read(ctx, denied, created.State.ID, reporting.Reference{Draft: true}); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("foreign partition read must conceal existence", err)
			}
			if _, err := service.SQL(ctx, denied, created.State.ID, reporting.Reference{Draft: true}); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("foreign partition SQL must conceal existence", err)
			}
			if _, err := service.History(ctx, denied, created.State.ID); !errors.Is(err, store.ErrNotFound) {
				t.Fatal("foreign partition history must conceal existence", err)
			}
			page, err := service.List(ctx, denied, reporting.ListRequest{Limit: 20, IncludeDrafts: true})
			if err != nil || len(page.Items) != 0 || page.Next != "" {
				t.Fatal("foreign partition list disclosed rows or cursor", page, err)
			}
		}
		if f.f.lookups.Load() != before {
			t.Fatal("foreign partition reads reached source secrets")
		}
		for _, mutate := range []func(*reporting.Definition){func(d *reporting.Definition) { d.Outputs[1].ID = d.Outputs[0].ID }, func(d *reporting.Definition) { d.Metadata[0].Locale = "en-us" }, func(d *reporting.Definition) { d.SchemaVersion = 999 }, func(d *reporting.Definition) { d.SQL = strings.Repeat("x", 65537) }} {
			bad := phase27Copy(t, base)
			mutate(&bad)
			if _, err := service.Create(ctx, e, reporting.CreateRequest{ID: "p27-invalid", Definition: bad}); !errors.Is(err, reporting.ErrInvalid) {
				t.Fatal("invalid definition admitted", err)
			}
		}
	})

	t.Run("AC02", func(t *testing.T) {
		created := create(t, "p27-race")
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				draft := phase27Copy(t, base)
				draft.Metadata[0].Title = fmt.Sprintf("Writer %d", i)
				_, err := service.Edit(ctx, e, created.State.ID, reporting.EditRequest{ExpectedVersion: 1, Definition: draft})
				results <- err
			}(i)
		}
		wg.Wait()
		close(results)
		wins, conflicts := 0, 0
		for err := range results {
			switch {
			case err == nil:
				wins++
			case errors.Is(err, store.ErrConflict):
				conflicts++
			default:
				t.Fatal(err)
			}
		}
		if wins != 1 || conflicts != 1 {
			t.Fatal("CAS did not select one writer", wins, conflicts)
		}
		draft, err := service.Read(ctx, e, created.State.ID, reporting.Reference{Draft: true})
		if err != nil {
			t.Fatal(err)
		}
		state, evidence := phase27ValidatePublish(t, service, e, draft)
		frozen, err := service.SQL(ctx, e, created.State.ID, reporting.Reference{})
		if err != nil {
			t.Fatal(err)
		}
		amend := phase27Copy(t, base)
		amend.SQL = "SELECT id, amount FROM analytics.sales WHERE id = 1 ORDER BY id"
		newDraft, err := service.Edit(ctx, e, created.State.ID, reporting.EditRequest{ExpectedVersion: state.Version, Definition: amend})
		if err != nil {
			t.Fatal(err)
		}
		if newDraft.Evidence != nil || newDraft.Digest == draft.Digest {
			t.Fatal("amendment inherited validation")
		}
		public, err := service.SQL(ctx, e, created.State.ID, reporting.Reference{})
		if err != nil || public.SQL != frozen.SQL || public.Revision != frozen.Revision {
			t.Fatal("published SQL mutated", public, err)
		}
		if _, err := service.Publish(ctx, e, created.State.ID, reporting.PublishRequest{ExpectedVersion: newDraft.State.Version, Evidence: evidence.ID}); err == nil {
			t.Fatal("old evidence published amended SQL")
		}
		rejected, err := service.Reject(ctx, e, created.State.ID, reporting.TransitionRequest{ExpectedVersion: newDraft.State.Version, Note: "Incorrect private amendment"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Validate(ctx, e, created.State.ID, reporting.ValidateRequest{ExpectedVersion: rejected.Version}); err == nil {
			t.Fatal("rejected draft validated without amendment")
		}
		restored, err := service.Restore(ctx, e, created.State.ID, reporting.RestoreRequest{ExpectedVersion: rejected.Version, Revision: frozen.Revision, Note: "Restore reviewed intent for fresh validation"})
		if err != nil {
			t.Fatal(err)
		}
		if restored.Revision != newDraft.Revision+1 || restored.Evidence != nil || !restored.Private {
			t.Fatal("restore transplanted approval", restored)
		}
		history, err := service.History(ctx, e, created.State.ID)
		if err != nil || len(history.Events) < 7 {
			t.Fatal("missing immutable history", history, err)
		}
		admin := support.Raw(t, f.f.dsn)
		if _, err := admin.Exec(ctx, `UPDATE chartworks.block_revisions SET digest=repeat('0',64) WHERE tenant_id=$1 AND block_id=$2`, e.Tenant(), created.State.ID); err == nil {
			t.Fatal("database allowed rewriting immutable revisions")
		}
	})

	t.Run("AC03", func(t *testing.T) {
		created := create(t, "p27-evidence")
		validation, err := service.Validate(ctx, e, created.State.ID, reporting.ValidateRequest{ExpectedVersion: created.State.Version})
		if err != nil {
			t.Fatal(err)
		}
		v := validation.Evidence
		if v.RevisionID != created.RevisionID || v.DefinitionDigest != created.Digest || v.ExecutionDigest != created.ExecutionDigest || v.ValidationManifest == "" || v.Attempt.ID == "" || v.Attempt.Status != "succeeded" || !reflect.DeepEqual(v.Schema, base.ExpectedSchema) || v.ResolvedAt.IsZero() || v.Timezone != "UTC" {
			t.Fatal("incomplete exact evidence", v)
		}
		raw, _ := json.Marshal(v)
		if bytes.Contains(raw, []byte("BLOCK_SQL_CANARY")) || bytes.Contains(raw, []byte("9007199254740993.125")) {
			t.Fatal("evidence retained SQL or rows")
		}
		reader := phase27Scopes(e.Tenant())
		reader = slices.DeleteFunc(reader, func(s string) bool { return s == "cw.dataset.query:*" })
		denied := phase27Actor(t, f, e.User(), reader)
		before := f.f.lookups.Load()
		if _, err := service.Validate(ctx, denied, created.State.ID, reporting.ValidateRequest{ExpectedVersion: validation.State.Version}); err == nil {
			t.Fatal("missing dataset reach validated")
		}
		if f.f.lookups.Load() != before {
			t.Fatal("authorization rejected after secret lookup")
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := service.Validate(cancelled, e, created.State.ID, reporting.ValidateRequest{ExpectedVersion: validation.State.Version}); err == nil {
			t.Fatal("cancelled validation accepted")
		}
		wrong := phase27Copy(t, base)
		wrong.ExpectedSchema[0].NativeType = "bigint"
		draft, err := service.Edit(ctx, e, created.State.ID, reporting.EditRequest{ExpectedVersion: validation.State.Version, Definition: wrong})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := service.Validate(ctx, e, created.State.ID, reporting.ValidateRequest{ExpectedVersion: draft.State.Version}); !errors.Is(err, reporting.ErrStale) {
			t.Fatal("observed schema mismatch accepted", err)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		created := create(t, "p27-trust")
		state, evidence := phase27ValidatePublish(t, service, e, created)
		public, err := service.Read(ctx, e, created.State.ID, reporting.Reference{})
		if err != nil || public.Trust.Certification != "none" || public.Trust.Publication != "published" {
			t.Fatal("publication implicitly certified", public, err)
		}
		authorScopes := slices.DeleteFunc(phase27Scopes(e.Tenant()), func(s string) bool { return s == "reporting.certify" || s == "cw.block.certify:*" })
		author := phase27Actor(t, f, e.User(), authorScopes)
		request := reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: created.Revision, Evidence: evidence.ID, Note: "Exact source evidence reviewed"}
		if _, err := service.Certify(ctx, author, created.State.ID, request); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("author certified without separate authority", err)
		}
		attestation, err := service.Certify(ctx, e, created.State.ID, request)
		if err != nil {
			t.Fatal(err)
		}
		public, err = service.Read(ctx, e, created.State.ID, reporting.Reference{})
		if err != nil || public.Trust.Certification != "valid" || public.Trust.HistoricalAttestation.ID != attestation.ID {
			t.Fatal(public, err)
		}
		withdrawn, err := service.Withdraw(ctx, e, created.State.ID, reporting.WithdrawRequest{ExpectedVersion: public.State.Version, Attestation: attestation.ID, Note: "Reviewer withdrew this attestation"})
		if err != nil {
			t.Fatal(err)
		}
		public, err = service.Read(ctx, e, created.State.ID, reporting.Reference{})
		if err != nil || public.Trust.Certification != "withdrawn" || public.Trust.HistoricalAttestation.ID != withdrawn.Attestation {
			t.Fatal("historical trust erased", public, err)
		}
		before := f.f.lookups.Load()
		if _, err := service.Read(ctx, e, created.State.ID, reporting.Reference{}); err != nil {
			t.Fatal(err)
		}
		if f.f.lookups.Load() != before {
			t.Fatal("ordinary metadata read touched source provider")
		}
	})

	t.Run("AC05", testPhase27Parameters)

	t.Run("AC06", func(t *testing.T) {
		// All four saved output kinds consume one real result, not fabricated rows
		// or an implicit second query/chart selection during preview.
		multi := phase27Definition(t, f, e, "SELECT id, amount FROM analytics.sales WHERE id = 1")
		columns := phase27Copy(t, multi.Outputs[0].Mapping.Columns)
		for i := range columns {
			columns[i].Role = "measure"
		}
		kpi := charts.Mapping{Version: charts.Version, Kind: charts.KPI, Columns: columns[1:], Bindings: charts.Bindings{Value: columns[1].ID}, Options: charts.DefaultOptions()}
		plot := charts.Mapping{Version: charts.Version, Kind: charts.Scatter, Columns: columns, Bindings: charts.Bindings{X: columns[0].ID, Y: columns[1].ID}, Options: charts.DefaultOptions()}
		multi.Outputs = append(multi.Outputs, reporting.Output{ID: "kpi-main", Kind: "kpi", Mapping: &kpi}, reporting.Output{ID: "chart-main", Kind: "chart", Mapping: &plot})
		allKinds, err := service.Create(ctx, e, reporting.CreateRequest{ID: "p27-all-output-kinds", Definition: multi})
		if err != nil {
			t.Fatal("four-kind authoring", err)
		}
		allPreview, err := service.Preview(ctx, e, allKinds.State.ID, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: allKinds.State.Version}})
		if err != nil || len(allPreview.Outputs) != 4 || len(allPreview.Result.Rows) != 1 || !allPreview.Private || allPreview.NarrativesGenerated {
			t.Fatal("four-kind actual preview", allPreview, err)
		}
		created := create(t, "p27-outputs")
		preview, err := service.Preview(ctx, e, created.State.ID, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: 1}, Outputs: []string{"narrative-main", "table-main"}})
		if err != nil || !preview.Private || preview.NarrativesGenerated || preview.Outputs[0].ID != "table-main" || len(preview.Result.Rows) != 2 {
			t.Fatal("saved output subset changed order or claimed generation", preview, err)
		}
		for _, ids := range [][]string{{"missing"}, {"table-main", "table-main"}} {
			if _, err := service.Preview(ctx, e, created.State.ID, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: 1}, Outputs: ids}); !errors.Is(err, reporting.ErrInvalid) {
				t.Fatal("invalid output selection accepted", err)
			}
		}
		readScopes := slices.DeleteFunc(phase27Scopes(e.Tenant()), func(s string) bool { return s == "reporting.sql.read" })
		reader := phase27Actor(t, f, e.User(), readScopes)
		if _, err := service.SQL(ctx, reader, created.State.ID, reporting.Reference{Draft: true}); !errors.Is(err, access.ErrForbidden) {
			t.Fatal("SQL inspection action bypassed", err)
		}
		view, err := service.Read(ctx, reader, created.State.ID, reporting.Reference{Draft: true})
		if err != nil {
			t.Fatal(err)
		}
		if view.State.Version != 1 || view.Evidence != nil {
			t.Fatal("preview silently persisted validation")
		}
		bad := phase27Copy(t, base)
		bad.Outputs[1].Narrative.MaxCalls = 5
		if _, err := service.Edit(ctx, e, created.State.ID, reporting.EditRequest{ExpectedVersion: 1, Definition: bad}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("unbounded saved narrative accepted", err)
		}
	})

	t.Run("AC07", testPhase27DependencyImpact)

	t.Run("AC08", func(t *testing.T) {
		registry, registryErr := reportingapi.Registry(true, true, true)
		if registryErr != nil {
			t.Fatal("reporting registry", registryErr)
		}
		shape, _, _ := registry.Match("POST", "/v1/blocks")
		wire, _ := json.Marshal(sdk.BlockCreateRequest{ID: "p27-sdk", Definition: base})
		if err := shape.Request.Validate(wire, shape.MaxBodyBytes); err != nil {
			t.Fatalf("SDK create wire rejected by registered schema: %v\n%s", err, wire)
		}
		handler := reportingapi.Handler(f.f.token.verifier, service, http.NotFoundHandler())
		server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, handler))
		t.Cleanup(server.Close)
		token := phase27Token(t, f, e.User(), "phase27-session", phase27Scopes(e.Tenant()))
		client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
		if err != nil {
			t.Fatal(err)
		}
		created, err := client.CreateBlock(ctx, sdk.BlockCreateRequest{ID: "p27-sdk", Definition: base})
		if err != nil {
			t.Fatalf("SDK create: %#v", err)
		}
		// Query capture consumes the real durable private query, not client SQL
		// or a fabricated positive validation record.
		f.model.embeddingMode.Store("fixed")
		f.model.rerankMode.Store("fixed")
		f.model.mode.Store(phase18RawResponse(t, base.SQL))
		planned, err := queryService.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: phase18Question(f, nlq.LanguageEnglish, f.pack.Topic), Operation: "p27-capture-query"})
		if err != nil {
			t.Fatal("capture query plan", err)
		}
		capture := sdk.BlockCaptureRequest{ID: "p27-captured", Query: planned.QueryID, Metadata: base.Metadata, Outputs: base.Outputs}
		if _, err := client.CaptureBlock(ctx, capture); err == nil {
			t.Fatal("planned query accepted as executed capture")
		}
		run, err := queryService.Run(ctx, e, nlqexec.RunRequest{QueryID: planned.QueryID, Operation: "p27-capture-query", Rows: 10, Bytes: 65536})
		if err != nil || run.Execution.Result == nil {
			t.Fatal("capture query execution", run, err)
		}
		before := f.f.lookups.Load()
		captured, err := client.CaptureBlock(ctx, capture)
		if err != nil || !captured.Private || captured.Evidence != nil || captured.Trust.Certification != "none" || f.f.lookups.Load() != before {
			t.Fatal("source-backed capture", captured, err)
		}
		capturedSQL, err := client.ReadBlockSQL(ctx, capture.ID, sdk.BlockReference{Draft: true})
		if err != nil || capturedSQL.SQL != base.SQL || capturedSQL.Provenance.Query != planned.QueryID || capturedSQL.Provenance.Kind != "query_capture" {
			t.Fatal("capture provenance", capturedSQL, err)
		}
		wrongSession, err := f.f.token.verifier.Verify(ctx, phase27Token(t, f, e.User(), "other-session", phase27Scopes(e.Tenant())), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		foreignCapture := capture
		foreignCapture.ID = "p27-foreign-capture"
		if _, err := service.CaptureQuery(ctx, wrongSession, foreignCapture); !errors.Is(err, nlqexec.ErrForeignSession) {
			t.Fatal("cross-session capture", err)
		}
		noSQL := phase27Actor(t, f, e.User(), slices.DeleteFunc(phase27Scopes(e.Tenant()), func(scope string) bool { return scope == "reporting.sql.read" }))
		if _, err := service.CaptureQuery(ctx, noSQL, foreignCapture); !errors.Is(err, nlqexec.ErrInspectionRequired) {
			t.Fatal("capture SQL authority", err)
		}
		validation, err := client.ValidateBlock(ctx, created.State.ID, sdk.BlockValidateRequest{ExpectedVersion: 1})
		if err != nil {
			t.Fatal("SDK validate", err)
		}
		state, err := client.PublishBlock(ctx, created.State.ID, sdk.BlockPublishRequest{ExpectedVersion: validation.State.Version, Evidence: validation.Evidence.ID})
		if err != nil {
			t.Fatal("SDK publish", err)
		}
		view, err := client.ReadBlock(ctx, created.State.ID, sdk.BlockReference{})
		if err != nil || view.Private || view.State.DraftRevision != 0 {
			t.Fatal("SDK projection", view, err)
		}
		page, err := client.ListBlocks(ctx, sdk.BlockListRequest{Limit: 100})
		if err != nil || len(page.Items) == 0 {
			t.Fatal("SDK authorized list", page, err)
		}
		assessment, err := client.AssessBlockQuestions(ctx, sdk.BlockQuestionRequest{Locale: "es-AR", Question: "¿CUÁL es el ingreso por venta?"})
		if err != nil || len(assessment.Matches) == 0 || assessment.Matches[0].Kind != "exact" {
			t.Fatal("localized question assessment", assessment, err)
		}
		preview, err := client.PreviewBlock(ctx, created.State.ID, sdk.BlockPreviewRequest{ValidateRequest: sdk.BlockValidateRequest{ExpectedVersion: state.Version}, Outputs: []string{"table-main"}})
		if err != nil || !preview.Private || len(preview.Result.Rows) != 2 {
			t.Fatal("SDK preview", preview, err)
		}
		attestation, err := client.CertifyBlock(ctx, created.State.ID, sdk.BlockCertifyRequest{ExpectedVersion: state.Version, Revision: created.Revision, Evidence: validation.Evidence.ID, Note: "Review exact SDK evidence"})
		if err != nil {
			t.Fatal("SDK certify", err)
		}
		view, err = client.ReadBlock(ctx, created.State.ID, sdk.BlockReference{})
		if err != nil || view.Trust.HistoricalAttestation == nil || view.Trust.HistoricalAttestation.ID != attestation.ID {
			t.Fatal("SDK attestation", view, err)
		}
		_, err = client.WithdrawBlockCertification(ctx, created.State.ID, sdk.BlockWithdrawRequest{ExpectedVersion: view.State.Version, Attestation: attestation.ID, Note: "Withdraw while preserving history"})
		if err != nil {
			t.Fatal("SDK withdraw", err)
		}
		view, err = client.ReadBlock(ctx, created.State.ID, sdk.BlockReference{})
		if err != nil || view.Trust.Certification != "withdrawn" {
			t.Fatal(view, err)
		}
		edited, err := client.EditBlock(ctx, created.State.ID, sdk.BlockEditRequest{ExpectedVersion: view.State.Version, Definition: base})
		if err != nil || edited.Evidence != nil {
			t.Fatal("SDK edit", edited, err)
		}
		rejected, err := client.RejectBlock(ctx, created.State.ID, sdk.BlockTransitionRequest{ExpectedVersion: edited.State.Version, Note: "Reject unvalidated amendment"})
		if err != nil {
			t.Fatal("SDK reject", err)
		}
		restored, err := client.RestoreBlock(ctx, created.State.ID, sdk.BlockRestoreRequest{ExpectedVersion: rejected.Version, Revision: created.Revision, Note: "Restore approved content as private draft"})
		if err != nil || !restored.Private || restored.Evidence != nil {
			t.Fatal("SDK restore", restored, err)
		}
		inspected, err := client.ReadBlockSQL(ctx, created.State.ID, sdk.BlockReference{Draft: true})
		if err != nil || inspected.SQL != base.SQL {
			t.Fatal("SDK SQL inspection", inspected, err)
		}
		history, err := client.BlockHistory(ctx, created.State.ID)
		if err != nil || len(history.Events) < 7 {
			t.Fatal("SDK history", history, err)
		}
		state = restored.State
		archived, err := client.ArchiveBlock(ctx, created.State.ID, sdk.BlockTransitionRequest{ExpectedVersion: state.Version, Note: "Archive fixture publication"})
		if err != nil || !archived.Archived {
			t.Fatal(archived, err)
		}
		if _, err := client.ReadBlock(ctx, created.State.ID, sdk.BlockReference{}); err == nil {
			t.Fatal("archived block still default-readable")
		}
		if _, err := client.ReadBlock(ctx, created.State.ID, sdk.BlockReference{Revision: created.Revision}); err != nil {
			t.Fatal("archival destroyed history", err)
		}
		other := phase27Actor(t, f, "another-actor", phase27Scopes(e.Tenant()))
		private := create(t, "p27-private-leak")
		if _, err := service.Read(ctx, other, private.State.ID, reporting.Reference{Draft: true}); err == nil {
			t.Fatal("another actor read private draft")
		}
		parentless := phase27Actor(t, f, e.User(), slices.DeleteFunc(phase27Scopes(e.Tenant()), func(s string) bool { return s == "cw.topic.read:*" }))
		if _, err := service.Read(ctx, parentless, created.State.ID, reporting.Reference{Revision: created.Revision}); err == nil {
			t.Fatal("block read bypassed parent topic")
		}
		for _, body := range []string{`{"id":"bad","definition":{},"tenant":"injected"}`, `{"expected_version":1,"certificate":true}`, `{"expected_version":1,"definition":null}`} {
			r, err := http.NewRequest("PUT", server.URL+"/v1/blocks/p27-sdk", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(r)
			if err != nil {
				t.Fatal(err)
			}
			raw, readErr := io.ReadAll(response.Body)
			closeErr := response.Body.Close()
			if readErr != nil {
				t.Fatal("read rejection response", readErr)
			}
			if closeErr != nil {
				t.Fatal("close rejection response", closeErr)
			}
			if response.StatusCode != 400 || response.Header.Get("Cache-Control") != "no-store" || bytes.Contains(raw, []byte("BLOCK_SQL_CANARY")) {
				t.Fatalf("closed safe wire rejection: %d %s", response.StatusCode, raw)
			}
		}
	})
}
