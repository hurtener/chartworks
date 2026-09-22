package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

// These cases exercise the real PostgreSQL decoder separately from the ordinary
// service's authoring checks. Faults are inserted as NEW fixture-only revisions:
// no immutable revision, publication, trigger or production gate is changed.
func testCW02StoredChartIntegrity(t *testing.T, f *phase31Fixture, client *cw.Client, definition reporting.Definition) {
	t.Helper()
	domain := f.domain
	e := domain.blockAuthor
	for _, tc := range []struct {
		name          string
		path          []string
		value         string
		executionHash bool
		decodeFailure bool
		listFailure   bool
	}{
		{name: "unchanged-rich-roundtrip"},
		{name: "malformed-repeated-slot", path: []string{"outputs", "0", "mapping", "bindings", "values"}, value: `"not-an-ordered-array"`, decodeFailure: true},
		{name: "reordered-measures-with-old-digest", path: []string{"outputs", "0", "mapping", "bindings", "values"}, value: `["quantity","revenue"]`},
		{name: "changed-source-pin-with-old-digest", path: []string{"outputs", "0", "mapping", "columns", "0", "provenance", "source_revision"}, value: `999`},
		{name: "changed-execution-digest", executionHash: true},
		{name: "malformed-retained-labels", path: []string{"metadata"}, value: `{"unexpected":"object"}`, decodeFailure: true, listFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			id := "cw02-integrity-" + tc.name
			created, err := client.CreateBlock(ctx, cw.BlockCreateRequest{ID: id, Definition: definition})
			if err != nil {
				t.Fatal("save ordinary reviewed rich definition", err)
			}
			before, err := client.ReadBlockSQL(ctx, id, cw.BlockReference{Draft: true})
			if err != nil {
				t.Fatal("read original persisted definition", err)
			}
			raw := support.Raw(t, domain.f.f.dsn)
			tx, err := raw.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = tx.Rollback(context.Background()) }()
			// Read the actual stored normalization, not a reconstructed input.
			var body []byte
			if err := tx.QueryRow(ctx, `SELECT definition FROM chartworks.block_revisions WHERE tenant_id=$1 AND block_id=$2 AND revision=1`, e.Tenant(), id).Scan(&body); err != nil {
				t.Fatal(err)
			}
			if tc.path != nil {
				var changed []byte
				if err := tx.QueryRow(ctx, `SELECT jsonb_set($1::jsonb,$2::text[],$3::jsonb,false)`, body, tc.path, tc.value).Scan(&changed); err != nil {
					t.Fatal(err)
				}
				// Ensure each injected path exists and really changes persisted data.
				var oldObject, newObject any
				if json.Unmarshal(body, &oldObject) != nil || json.Unmarshal(changed, &newObject) != nil || reflect.DeepEqual(oldObject, newObject) {
					t.Fatal("fault did not alter the declared stored field")
				}
				body = changed
			}
			if _, err := tx.Exec(ctx, `INSERT INTO chartworks.block_revisions
 (tenant_id,block_id,revision,revision_id,definition,digest,execution_digest,actor_id,session_id,provenance,created_at)
 SELECT tenant_id,block_id,2,repeat('b',32),$3::jsonb,digest,
 CASE WHEN $4 THEN repeat('f',64) ELSE execution_digest END,actor_id,session_id,provenance,created_at
 FROM chartworks.block_revisions WHERE tenant_id=$1 AND block_id=$2 AND revision=1`, e.Tenant(), id, body, tc.executionHash); err != nil {
				t.Fatal("insert disposable revision fault", err)
			}
			for _, query := range []string{
				`INSERT INTO chartworks.block_revision_references (tenant_id,block_id,revision,kind,permission,resource_id) SELECT tenant_id,block_id,2,kind,permission,resource_id FROM chartworks.block_revision_references WHERE tenant_id=$1 AND block_id=$2 AND revision=1`,
				`INSERT INTO chartworks.block_topic_pins (tenant_id,block_id,revision,topic_id,version_id,digest) SELECT tenant_id,block_id,2,topic_id,version_id,digest FROM chartworks.block_topic_pins WHERE tenant_id=$1 AND block_id=$2 AND revision=1`,
				`INSERT INTO chartworks.block_source_pins (tenant_id,block_id,revision,source_id,source_revision,context_id) SELECT tenant_id,block_id,2,source_id,source_revision,context_id FROM chartworks.block_source_pins WHERE tenant_id=$1 AND block_id=$2 AND revision=1`,
				`UPDATE chartworks.block_heads SET draft_revision=2,version=version+1 WHERE tenant_id=$1 AND block_id=$2`,
			} {
				if _, err := tx.Exec(ctx, query, e.Tenant(), id); err != nil {
					t.Fatal("prepare independent fixture revision", err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			beforeSource, beforeModels := domain.f.f.lookups.Load(), domain.f.model.requests.Load()
			ref := reporting.Reference{Draft: true}
			snapshot, err := domain.f.f.db.ReadBlock(ctx, e, id, ref, reporting.SQLRead)
			if tc.path == nil && !tc.executionHash {
				if err != nil || snapshot.Revision.Number != 2 || !reflect.DeepEqual(snapshot.Revision.Definition.Outputs, definition.Outputs) {
					t.Fatal("unmodified rich mappings must remain readable", err)
				}
				read, err := client.ReadBlock(ctx, id, cw.BlockReference{Draft: true})
				if err != nil || !reflect.DeepEqual(read.Outputs, definition.Outputs) {
					t.Fatal("real client lost ordered binding identities", err)
				}
			} else {
				if !errors.Is(err, store.ErrInvalid) || !reflect.DeepEqual(snapshot, reporting.Snapshot{}) {
					t.Fatalf("invalid stored mapping returned data: %+v %v", snapshot, err)
				}
				_, err = client.ReadBlockSQL(ctx, id, cw.BlockReference{Draft: true})
				chartStatus(t, err, http.StatusBadRequest)
				if tc.decodeFailure {
					snapshot, err = domain.f.f.db.ReadBlock(ctx, e, id, ref, reporting.Read)
					if !errors.Is(err, store.ErrInvalid) || !reflect.DeepEqual(snapshot, reporting.Snapshot{}) {
						t.Fatal("metadata projection exposed malformed rich data", err)
					}
					_, err = client.ReadBlock(ctx, id, cw.BlockReference{Draft: true})
					chartStatus(t, err, http.StatusBadRequest)
				}
			}
			if tc.listFailure {
				page, err := domain.f.f.db.ListBlocks(ctx, e, reporting.ListRequest{Limit: 100, IncludeDrafts: true})
				if !errors.Is(err, store.ErrInvalid) || !reflect.DeepEqual(page, reporting.Page{}) {
					t.Fatal("malformed stored labels leaked a partial catalog", page, err)
				}
			}
			// The original immutable revision and its exact mapping are untouched.
			after, err := client.ReadBlockSQL(ctx, id, cw.BlockReference{Revision: 1})
			if err != nil || !reflect.DeepEqual(after, before) || created.Revision != 1 {
				t.Fatal("fault changed the original immutable definition", err)
			}
			if domain.f.f.lookups.Load() != beforeSource || domain.f.model.requests.Load() != beforeModels {
				t.Fatal("stored corruption handling performed source or model work")
			}
		})
	}
}
