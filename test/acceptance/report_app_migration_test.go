package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// This is an actual populated 086 -> current upgrade, not a current-schema
// fixture with triggers disabled. Synthetic parent metadata comes from the
// native fixture; the old preparation has no fabricated execution settlement.
func TestReportAppPopulatedMigration087088(t *testing.T) {
	f, _, owner, _, prepared, parents, _ := preparationFixtureBase(t, "upgrade-chart")
	attemptsBefore := f.attemptCount(t)
	manifest, err := postgres.Migrations()
	if err != nil || len(manifest) < 88 || manifest[86].Name != "migrations/087_authoring_preparation_retention.sql" || manifest[87].Name != "migrations/088_chart_presentation.sql" {
		t.Fatal("migration sequence unavailable", err)
	}
	for _, invalidPresentation := range []bool{false, true} {
		name := "preserve_and_fence_old_writers"
		if invalidPresentation {
			name = "invalid_presentation_rolls_back_whole_upgrade"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			dsn := support.Database(t)
			raw := upgradeFixture(t, dsn)
			for _, m := range manifest[1:86] {
				sql(t, raw, m.SQL)
				sql(t, raw, `INSERT INTO chartworks.schema_migrations(version,name,checksum) VALUES($1,$2,$3)`, m.Version, m.Name, m.Checksum)
			}
			tx, err := raw.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			// Closed fixture-only table list, in FK order. All parents are from
			// this test's disposable database; no service-private data is read.
			for _, table := range []string{"policy_revisions", "policies", "sources", "source_revisions", "topic_draft_heads", "topic_draft_versions", "topic_reviews", "topic_publication_heads", "topic_published_versions"} {
				qualified := pgx.Identifier{"chartworks", table}.Sanitize()
				var rows []byte
				row := "to_jsonb(p)"
				if table == "topic_publication_heads" {
					// Preserve the historical published version, with the
					// synthetic topic archived before the upgrade. There is
					// no active vector generation in this metadata-only fixture.
					row += " || jsonb_build_object('active_version',NULL,'archived',true)"
				}
				if err = parents.QueryRow(ctx, "SELECT COALESCE(jsonb_agg("+row+"),'[]'::jsonb) FROM "+qualified+" p").Scan(&rows); err != nil {
					t.Fatal("read synthetic parents", table, err)
				}
				if _, err = tx.Exec(ctx, "INSERT INTO "+qualified+" SELECT * FROM jsonb_populate_recordset(NULL::"+qualified+",$1)", rows); err != nil {
					t.Fatal("copy synthetic parents", table, err)
				}
			}
			definition, err := json.Marshal(f.base)
			if err != nil {
				t.Fatal(err)
			}
			definitionSQL := "$3::jsonb"
			if invalidPresentation {
				definitionSQL = `jsonb_set($3::jsonb,'{outputs,0,mapping,presentation}','{"version":99,"columns":[]}'::jsonb)`
			}
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.block_heads(tenant_id,block_id,topic_id,version,draft_revision,draft_state) VALUES($1,'legacy-chart',$2,1,1,'draft')`, owner.Tenant(), prepared.Topics[0].Topic); err != nil {
				t.Fatal(err)
			}
			if _, err = tx.Exec(ctx, `INSERT INTO chartworks.block_revisions(tenant_id,block_id,revision,revision_id,definition,digest,execution_digest,actor_id,session_id,provenance,created_at) VALUES($1,'legacy-chart',1,$2,`+definitionSQL+`,$4,$5,$6,$7,'{}',clock_timestamp())`, owner.Tenant(), strings.Repeat("a", 32), definition, reporting.DefinitionDigest(f.base), reporting.ExecutionDigest(f.base), owner.User(), owner.Session()); err != nil {
				t.Fatal("pre-088 block insert", err)
			}
			sealInput := func(r *reporting.AuthoringPreparationRecord) []byte {
				t.Helper()
				r.InputDigest = readexec.Hash(r.Request)
				r.SourceOperation = "chart-prepare:" + readexec.Hash([]string{r.Binding.Tenant, r.Actor, r.Session, r.Target, r.Operation})
				wire, err := json.Marshal(r)
				if err != nil {
					t.Fatal(err)
				}
				return wire
			}
			legacy := preparationCopy(t, prepared, 8701)
			legacy.Request.OperationVersion = ""
			legacy.Operation, legacy.Request.Operation = "legacy-operation", "legacy-operation"
			legacy.CreatedAt = time.Now().UTC().Add(-48 * time.Hour)
			legacy.Deadline = legacy.CreatedAt.Add(30 * time.Second)
			legacy.ExpiresAt = legacy.CreatedAt.Add(15 * time.Minute)
			wire := sealInput(&legacy)
			const insert = `INSERT INTO chartworks.authoring_preparations(tenant_id,preparation_id,actor_id,session_id,target_id,operation_id,input_digest,source_operation,status,record,created_at,deadline,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`
			args := func(r reporting.AuthoringPreparationRecord, body []byte) []any {
				return []any{r.Binding.Tenant, r.ID, r.Actor, r.Session, r.Target, r.Operation, r.InputDigest, r.SourceOperation, r.Status, body, r.CreatedAt, r.Deadline, r.ExpiresAt}
			}
			if _, err = tx.Exec(ctx, insert, args(legacy, wire)...); err != nil {
				t.Fatal("old writer could not seed original schema", err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			readEvidence := func() [2]string {
				t.Helper()
				var result [2]string
				if err := raw.QueryRow(ctx, `SELECT jsonb_build_array(record,input_digest,source_operation,status,created_at,deadline,expires_at)::text FROM chartworks.authoring_preparations`).Scan(&result[0]); err != nil {
					t.Fatal(err)
				}
				if err := raw.QueryRow(ctx, `SELECT jsonb_build_array(definition,digest,execution_digest,revision_id)::text FROM chartworks.block_revisions`).Scan(&result[1]); err != nil {
					t.Fatal(err)
				}
				return result
			}
			before := readEvidence()
			checkOptions := postgres.Defaults()
			checkOptions.MigrationPolicy = "check"
			unready, checkErr := postgres.Open(ctx, dsn, checkOptions)
			if unready != nil {
				unready.Close()
			}
			if unready != nil || !errors.Is(checkErr, store.ErrMigration) || count(t, raw, `SELECT max(version) FROM chartworks.schema_migrations`) != 86 || before != readEvidence() {
				t.Fatal("check-only startup changed or accepted old schema", checkErr)
			}
			upgraded, err := postgres.Open(ctx, dsn, postgres.Defaults())
			if invalidPresentation {
				if upgraded != nil {
					upgraded.Close()
				}
				if !errors.Is(err, store.ErrMigration) || upgraded != nil {
					t.Fatal("invalid old presentation did not refuse upgrade", err)
				}
				if count(t, raw, `SELECT max(version) FROM chartworks.schema_migrations`) != 86 || count(t, raw, `SELECT count(*) FROM information_schema.columns WHERE table_schema='chartworks' AND table_name='authoring_preparations' AND column_name='admission_guarded'`) != 0 || before != readEvidence() {
					t.Fatal("failed migration left partial schema or changed evidence")
				}
				return
			}
			if err != nil {
				t.Fatal("normal forward upgrade", err)
			}
			defer upgraded.Close()
			if err = upgraded.Check(ctx); err != nil || before != readEvidence() {
				t.Fatal("upgrade rewrote immutable evidence", err)
			}
			stored, err := upgraded.ReadAuthoringPreparation(ctx, owner, legacy.ID)
			if err != nil || !reflect.DeepEqual(stored, legacy) {
				t.Fatal("current binary cannot recover original custody", err)
			}
			if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations p WHERE NOT admission_guarded AND settlement IS NULL AND chartworks.preparation_has_liability(p) AND NOT chartworks.preparation_cleanup_eligible(p)`) != 1 {
				t.Fatal("migration invented settlement or discarded liability")
			}
			assertCode := func(query, code string, args ...any) {
				t.Helper()
				_, err := raw.Exec(ctx, query, args...)
				var pgerr *pgconn.PgError
				if !errors.As(err, &pgerr) || pgerr.Code != code {
					t.Fatalf("expected SQLSTATE %s; got %v", code, err)
				}
			}
			assertCode(`DELETE FROM chartworks.authoring_preparations`, "23514")
			oldWriter := preparationCopy(t, legacy, 8702)
			oldWriter.Request.OperationVersion = ""
			body := sealInput(&oldWriter)
			assertCode(insert, "CW001", args(oldWriter, body)...)
			newWriter := preparationCopy(t, legacy, 8703)
			body = sealInput(&newWriter)
			if _, err = raw.Exec(ctx, insert, args(newWriter, body)...); err != nil {
				t.Fatal("new admission contract rejected", err)
			}
			if count(t, raw, `SELECT count(*) FROM chartworks.authoring_preparations WHERE admission_guarded AND settlement IS NULL`) != 1 {
				t.Fatal("new admission was not guarded")
			}
			stale := preparationCopy(t, legacy, 8704)
			stale.Operation = preparationKey(8704, time.Now().Add(-10*time.Minute))
			stale.Request.Operation = stale.Operation
			body = sealInput(&stale)
			assertCode(insert, "CW002", args(stale, body)...)
			// A second current binary must see the exact schema and apply no
			// further changes. This does not endorse a pre-087 writer rollback.
			second := support.Open(t, dsn)
			if err = second.Check(ctx); err != nil || count(t, raw, `SELECT count(*) FROM chartworks.schema_migrations`) != int64(len(manifest)) {
				t.Fatal("same-version reopen failed", err)
			}
		})
	}
	if f.attemptCount(t) != attemptsBefore {
		t.Fatal("migration replayed source work")
	}
}
