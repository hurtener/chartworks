package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/test/support"
)

func phase34ImportedRetainedDrill(t *testing.T) {
	f := newReportingFixture(t)
	queries, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(queries), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	tenant := f.f.e.Tenant()
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(tenant))
	executor := phase27Actor(t, f, f.f.e.User(), phase28Scopes(tenant))
	definition := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id")
	definition.Outputs = definition.Outputs[:1]
	importID, unrelatedID := "exp08-imported-block", "exp08-unrelated-block"
	manifest := phase34Manifest("exp08-retained")
	manifest.Objects = manifest.Objects[:9] // source through block; no fake historical domain values
	manifest.Boundary = nil
	request := reporting.CreateRequest{ID: importID, Definition: definition}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Objects[8].Payload = string(raw)
	manifest.Fields = slices.DeleteFunc(manifest.Fields, func(field migration.FieldDisposition) bool {
		return strings.HasPrefix(field.Path, manifest.Objects[8].ExternalRef+".") || !slices.ContainsFunc(manifest.Objects, func(o migration.Object) bool { return strings.HasPrefix(field.Path, o.ExternalRef+".") })
	})
	manifest.Fields = append(manifest.Fields, migration.FieldDisposition{Path: manifest.Objects[8].ExternalRef + ".id", Status: "retained"}, migration.FieldDisposition{Path: manifest.Objects[8].ExternalRef + ".definition", Status: "retained"})
	importActor := phase27Actor(t, f, f.f.e.User(), []string{"migration.write", "cw.tenant.write:" + tenant, "reporting.write", "cw.block.write:*", "cw.topic.write:*", "topics.read", "cw.topic.read:*", "sources.read", "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"})
	generic := migration.AdapterFuncs{ValidateFunc: func(context.Context, identity.Envelope, migration.Object, migration.Mapping) error { return nil }, ApplyFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
		return o.ExternalRef, nil
	}}
	adapters := map[migration.Kind]migration.Adapter{}
	for _, o := range manifest.Objects {
		adapters[o.Kind] = generic
	}
	adapters[migration.KindBlock] = migration.AdapterFuncs{ValidateFunc: func(_ context.Context, _ identity.Envelope, o migration.Object, _ migration.Mapping) error {
		var in reporting.CreateRequest
		return json.Unmarshal([]byte(o.Payload), &in)
	}, ApplyFunc: func(ctx context.Context, e identity.Envelope, o migration.Object, _ migration.Mapping, _ string) (string, error) {
		var in reporting.CreateRequest
		if err := json.Unmarshal([]byte(o.Payload), &in); err != nil {
			return "", err
		}
		view, err := blocks.Create(ctx, e, in)
		if err != nil {
			return "", err
		}
		return view.State.ID + ":draft:1", nil
	}}
	service, err := migration.New(f.f.db, adapters, nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := service.Import(t.Context(), importActor, migration.ImportRequest{Manifest: manifest})
	if err != nil || batch.State != "complete" {
		t.Fatal("synthetic owner import", err, batch)
	}
	imported, err := blocks.Read(t.Context(), author, importID, reporting.Reference{Draft: true})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, blocks, author, imported)
	unrelated, err := blocks.Create(t.Context(), author, reporting.CreateRequest{ID: unrelatedID, Definition: definition})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, blocks, author, unrelated)
	runs := phase28RunService(t, f, blocks, f.f.db, nil, config.DefaultReportingExecution())
	makeRun := func(block, key string) string {
		t.Helper()
		view, err := runs.Admit(t.Context(), executor, block, reporting.RunRequest{Key: key})
		if err != nil {
			t.Fatal(err)
		}
		view, err = runs.Run(t.Context(), executor, view.ID, false)
		if err != nil || view.State != "succeeded" {
			t.Fatal("retained output", err, view.State, view.Code)
		}
		return view.ID
	}
	importedRun := makeRun(importID, "exp08-imported-run")
	unrelatedRun := makeRun(unrelatedID, "exp08-unrelated-run")
	putRendition := func(run, id string) {
		t.Helper()
		now := time.Now().UTC()
		_, err := f.f.db.PutRendition(t.Context(), rendering.Record{Tenant: tenant, Actor: executor.User(), Session: executor.Session(), Request: rendering.Request{View: reporting.DeliveryViewRequest{Run: run}}, Rendition: rendering.Rendition{ID: id, Version: rendering.Version, Format: "html", WorkerVersion: "synthetic-worker", ThemeVersion: "synthetic-theme", SourceDigest: strings.Repeat("a", 64), Digest: strings.Repeat("b", 64), CreatedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour)}})
		if err != nil {
			t.Fatal(err)
		}
	}
	importedRendition := "rnd-" + strings.Repeat("a", 32)
	unrelatedRendition := "rnd-" + strings.Repeat("b", 32)
	putRendition(importedRun, importedRendition)
	putRendition(unrelatedRun, unrelatedRendition)
	driller := phase27Actor(t, f, f.f.e.User(), []string{"migration.erase", "reporting.retention", "reporting.read", "reporting.preview", "cw.tenant.erase:" + tenant, "cw.block.read:*", "cw.block.preview:*", "cw.execution_context.use:*"})
	requestDrill := migration.RetentionDrillRequest{Batch: batch.ID, Expected: batch.Revision, Limit: 10}
	preExpiry, err := service.RetentionDrill(t.Context(), driller, requestDrill)
	if err != nil || len(preExpiry.Items) != 0 {
		t.Fatal("retention was shortened before due time", err, preExpiry)
	}
	requestDrill.Apply, requestDrill.PreviewDigest = true, preExpiry.PreviewDigest
	if _, err := service.RetentionDrill(t.Context(), driller, requestDrill); err != nil {
		t.Fatal("empty preview could not be applied", err)
	}
	requestDrill.Apply, requestDrill.PreviewDigest = false, ""
	rawDB := support.Raw(t, f.f.dsn)
	for _, run := range []string{importedRun, unrelatedRun} {
		if _, err := rawDB.Exec(t.Context(), `UPDATE chartworks.frozen_runs SET payload_expires_at=created_at+interval '1 microsecond' WHERE tenant_id=$1 AND operation_id=$2`, tenant, run); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := service.RetentionDrill(t.Context(), driller, migration.RetentionDrillRequest{Batch: batch.ID, Expected: batch.Revision + 1, Limit: 10}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("stale batch revision selected outputs", err)
	}
	foreign := phase34Actor(t, "foreign-exp08", "migration.erase", "reporting.retention", "reporting.read", "reporting.preview", "cw.block.read:*", "cw.block.preview:*", "cw.execution_context.use:*")
	if _, err := service.RetentionDrill(t.Context(), foreign, requestDrill); !errors.Is(err, migration.ErrNotFound) {
		t.Fatal("cross-tenant drill selected outputs", err)
	}
	preview, err := service.RetentionDrill(t.Context(), driller, requestDrill)
	if err != nil || preview.Applied || len(preview.Items) != 1 || preview.Items[0].Run != importedRun || preview.Items[0].Renditions != 1 {
		t.Fatal("exact preview", err, preview)
	}
	if _, err := f.f.db.ReadRendition(t.Context(), tenant, importedRendition); err != nil {
		t.Fatal("preview erased rendition", err)
	}
	if _, err := service.RetentionDrill(t.Context(), driller, migration.RetentionDrillRequest{Batch: batch.ID, Expected: batch.Revision, Limit: 10, Apply: true, PreviewDigest: preExpiry.PreviewDigest}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("changed candidate set accepted stale preview", err)
	}
	if _, err := service.Erase(t.Context(), driller, migration.EraseRequest{Batch: batch.ID, Limit: 100}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("migration provenance erased ahead of retained descendants", err)
	}
	denied := phase27Actor(t, f, f.f.e.User(), []string{"migration.erase", "reporting.retention", "reporting.read", "cw.tenant.erase:" + tenant, "cw.block.read:*"})
	if _, err := service.RetentionDrill(t.Context(), denied, requestDrill); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("missing context reach revealed retained descendant", err)
	}
	requestDrill.Apply, requestDrill.PreviewDigest = true, preview.PreviewDigest
	applied, err := service.RetentionDrill(t.Context(), driller, requestDrill)
	if err != nil || !applied.Applied || len(applied.Items) != 1 || applied.Remaining != 0 {
		t.Fatal("bounded owner erasure", err, applied)
	}
	if _, err := f.f.db.ReadRendition(t.Context(), tenant, importedRendition); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("imported rendition remains", err)
	}
	if _, err := f.f.db.ReadRendition(t.Context(), tenant, unrelatedRendition); err != nil {
		t.Fatal("unrelated rendition erased", err)
	}
	var importedState, unrelatedState string
	if err := rawDB.QueryRow(t.Context(), `SELECT state FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2`, tenant, importedRun).Scan(&importedState); err != nil {
		t.Fatal(err)
	}
	if err := rawDB.QueryRow(t.Context(), `SELECT state FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2`, tenant, unrelatedRun).Scan(&unrelatedState); err != nil {
		t.Fatal(err)
	}
	if importedState != "expired" || unrelatedState != "succeeded" {
		t.Fatal("owner retention touched unrelated run", importedState, unrelatedState)
	}
	if _, err := service.RetentionDrill(t.Context(), driller, requestDrill); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("stale preview replayed", err)
	}
	requestDrill.Apply, requestDrill.PreviewDigest = false, ""
	replayed, err := service.RetentionDrill(t.Context(), driller, requestDrill)
	if err != nil || len(replayed.Items) != 0 || replayed.Remaining != 0 {
		t.Fatal("owner erasure repeat preview", err, replayed)
	}
	requestDrill.Apply, requestDrill.PreviewDigest = true, replayed.PreviewDigest
	if _, err := service.RetentionDrill(t.Context(), driller, requestDrill); err != nil {
		t.Fatal("owner erasure empty apply", err)
	}
	if _, err := service.Erase(t.Context(), driller, migration.EraseRequest{Batch: batch.ID, Limit: 100}); err != nil {
		t.Fatal("migration erasure after owner cleanup", err)
	}
}
