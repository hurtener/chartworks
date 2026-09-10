# Replace the malformed recovered regression fixture, retaining real PostgreSQL
# JSONB and complete-partition selection assertions. Never ship this dev script.
from pathlib import Path
p = Path('internal/store/postgres/phase26_28_boundaries_test.go')
assert p.exists() and 'proposalImpactQuery' in p.read_text()
p.write_text(r'''package postgres

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "math"
 "os"
 "reflect"
 "testing"
 "time"

 readexec "github.com/hurtener/chartworks/internal/exec"
 "github.com/hurtener/chartworks/internal/reporting"
 "github.com/hurtener/chartworks/internal/store"
 "github.com/jackc/pgx/v5"
)

// Each test owns a unique disposable database. Selection and JSONB representation
// are exercised by PostgreSQL, not by a mocked or post-filtered repository.
func frozenBoundaryDatabase(t *testing.T) *pgx.Conn {
 t.Helper()
 dsn := os.Getenv("CHARTWORKS_TEST_STORE_URL")
 if dsn == "" { t.Fatal("CHARTWORKS_TEST_STORE_URL is required") }
 ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
 defer cancel()
 admin, err := pgx.Connect(ctx, dsn)
 if err != nil { t.Fatal(err) }
 var random [12]byte
 if _, err = rand.Read(random[:]); err != nil { _ = admin.Close(ctx); t.Fatal(err) }
 name := "cw_boundary_" + hex.EncodeToString(random[:])
 quoted := pgx.Identifier{name}.Sanitize()
 if _, err = admin.Exec(ctx, "CREATE DATABASE " + quoted); err != nil { _ = admin.Close(ctx); t.Fatal(err) }
 var database *pgx.Conn
 t.Cleanup(func() {
  cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
  defer stop()
  if database != nil { _ = database.Close(cleanup) }
  if _, err := admin.Exec(cleanup, "DROP DATABASE " + quoted + " WITH (FORCE)"); err != nil { t.Error("drop owned fixture database", err) }
  _ = admin.Close(cleanup)
 })
 cfg, err := pgx.ParseConfig(dsn)
 if err != nil { t.Fatal(err) }
 cfg.Database = name
 database, err = pgx.ConnectConfig(ctx, cfg)
 if err != nil { t.Fatal(err) }
 return database
}

func TestFrozenCanonicalJSONBRoundTrip(t *testing.T) {
 db := frozenBoundaryDatabase(t)
 ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
 defer cancel()
 before := reporting.RetainedOutput{ID:"narrative-main", Kind:"narrative", State:"indeterminate", Code:"narrative_indeterminate", ReservedCalls:1, ReservedTokens:1024}
 compact, err := json.Marshal(before)
 if err != nil { t.Fatal(err) }
 var stored []byte
 if err = db.QueryRow(ctx, `SELECT $1::jsonb`, compact).Scan(&stored); err != nil { t.Fatal(err) }
 if len(stored) <= len(compact) { t.Fatal("fixture did not expose JSONB representation expansion") }
 var previous reporting.RetainedOutput
 if err = json.Unmarshal(stored, &previous); err != nil { t.Fatal(err) }
 previousSize, err := frozenCanonicalBytes(previous)
 if err != nil || previousSize != len(compact) { t.Fatal("representation-dependent prior charge", previousSize, len(compact), err) }
 after := previous
 after.State, after.Code = "failed", "narrative_failed"
 after.Digest = after.ContentDigest()
 updated, err := json.Marshal(after)
 if err != nil { t.Fatal(err) }
 expectedDelta := len(updated)-len(compact)
 if len(updated)-previousSize != expectedDelta { t.Fatal("output transition refunded JSONB formatting bytes") }

 // Exact decimal cells remain strings. Their value and typed budget must survive
 // a JSONB round trip without floating-point conversion or whitespace charges.
 result := readexec.Result{Outcome:"succeeded"}
 result.Schema = []readexec.Field{{Name:"amount",Type:"decimal"}}
 result.Rows = make([][]json.RawMessage, 1)
 result.Rows[0] = []json.RawMessage{json.RawMessage(`"9007199254740993.125"`)}
 resultBytes, err := json.Marshal(result)
 if err != nil { t.Fatal(err) }
 if err = db.QueryRow(ctx, `SELECT $1::jsonb`, resultBytes).Scan(&stored); err != nil { t.Fatal(err) }
 var read readexec.Result
 if err = json.Unmarshal(stored, &read); err != nil { t.Fatal(err) }
 if len(read.Rows)!=1 || len(read.Rows[0])!=1 || string(read.Rows[0][0])!=`"9007199254740993.125"` { t.Fatal("exact decimal changed") }
 size, err := frozenCanonicalBytes(read)
 if err != nil || size != len(resultBytes) { t.Fatal("retained result budget changed after JSONB storage", size, len(resultBytes), err) }
 if _, err = frozenCanonicalBytes(math.NaN()); !errors.Is(err, store.ErrInvalid) { t.Fatal("unencodable value accepted", err) }
}

func TestProposalImpactSelectionRequiresEveryPartition(t *testing.T) {
 db := frozenBoundaryDatabase(t)
 ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
 defer cancel()
 _, err := db.Exec(ctx, `
 CREATE SCHEMA chartworks;
 CREATE TABLE chartworks.block_heads(tenant_id text,block_id text,published_revision bigint,archived boolean);
 CREATE TABLE chartworks.block_source_pins(tenant_id text,block_id text,revision bigint,source_id text);
 CREATE TABLE chartworks.block_revision_references(tenant_id text,block_id text,revision bigint,kind text,permission text,resource_id text);
 CREATE TABLE chartworks.topic_publication_heads(tenant_id text,topic_id text,active_version text,archived boolean);
 CREATE TABLE chartworks.topic_published_versions(tenant_id text,topic_id text,version_id text,definition jsonb);
 `)
 if err != nil { t.Fatal(err) }
 addBlock := func(tenant,id,contextID,dataset string,withRefs bool) {
  t.Helper()
  if _, err := db.Exec(ctx, `INSERT INTO chartworks.block_heads VALUES($1,$2,1,false)`,tenant,id); err != nil { t.Fatal(err) }
  if _, err := db.Exec(ctx, `INSERT INTO chartworks.block_source_pins VALUES($1,$2,1,'source-a')`,tenant,id); err != nil { t.Fatal(err) }
  if withRefs {
   refs := []struct{kind,permission,id string}{{"source","read","source-a"},{"dataset","query",dataset},{"execution_context","use",contextID}}
   for _, ref := range refs {
    if _, err := db.Exec(ctx, `INSERT INTO chartworks.block_revision_references VALUES($1,$2,1,$3,$4,$5)`,tenant,id,ref.kind,ref.permission,ref.id); err != nil { t.Fatal(err) }
   }
  }
 }
 addTopic := func(tenant,id string,contexts ...string) {
  t.Helper()
  datasets := make([]map[string]any,0,len(contexts))
  for _, contextID := range contexts {
   datasets=append(datasets,map[string]any{"source":map[string]string{"source":"source-a","context":contextID,"dataset":"dataset-a"}})
  }
  definition, err := json.Marshal(map[string]any{"datasets":datasets})
  if err != nil { t.Fatal(err) }
  if _, err = db.Exec(ctx, `INSERT INTO chartworks.topic_publication_heads VALUES($1,$2,'v1',false)`,tenant,id); err != nil { t.Fatal(err) }
  if _, err = db.Exec(ctx, `INSERT INTO chartworks.topic_published_versions VALUES($1,$2,'v1',$3)`,tenant,id,definition); err != nil { t.Fatal(err) }
 }
 addBlock("tenant-a","visible-block","context-a","dataset-a",true)
 addBlock("tenant-a","other-context-block","context-b","dataset-a",true)
 addBlock("tenant-a","other-dataset-block","context-a","dataset-b",true)
 addBlock("tenant-a","missing-references","context-a","dataset-a",false)
 addBlock("foreign-tenant","foreign-block","context-a","dataset-a",true)
 addTopic("tenant-a","visible-topic","context-a")
 addTopic("tenant-a","mixed-topic","context-a","context-b")
 addTopic("foreign-tenant","foreign-topic","context-a")

 type grant struct { Kind string `json:"kind"`; Permission string `json:"permission"`; ID string `json:"id"` }
 base := []grant{{"block","read","*"},{"topic","read","*"},{"source","read","source-a"},{"dataset","query","dataset-a"},{"execution_context","use","context-a"}}
 check := func(tenant string, grants []grant, expected []string) {
  t.Helper()
  encoded, err := json.Marshal(grants)
  if err != nil { t.Fatal(err) }
  rows, err := db.Query(ctx, proposalImpactQuery, tenant, []string{"source-a"}, encoded)
  if err != nil { t.Fatal(err) }
  got := []string{}
  for rows.Next() {
   var kind,id string
   if err = rows.Scan(&kind,&id); err != nil { rows.Close(); t.Fatal(err) }
   got=append(got,kind+":"+id)
  }
  err=rows.Err();rows.Close()
  if err != nil || !reflect.DeepEqual(got,expected) { t.Fatal("impact selection widened or lost current reach",got,expected,err) }
 }
 check("tenant-a",base,[]string{"block:visible-block","topic:visible-topic"})
 check("tenant-a",base[:2],[]string{})
 noDataset:=append(append([]grant{},base[:3]...),base[4])
 check("tenant-a",noDataset,[]string{})
 check("tenant-a",base[:4],[]string{})
 check("unrelated-tenant",base,[]string{})
 wider:=append([]grant{},base...)
 wider[4].ID="*"
 check("tenant-a",wider,[]string{"block:other-context-block","block:visible-block","topic:mixed-topic","topic:visible-topic"})
}
''')
