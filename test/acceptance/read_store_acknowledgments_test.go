package acceptance

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestReadStoreCloudAcknowledgments(t *testing.T) {
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	raw := support.Raw(t, dsn)
	scope := support.Scope(t, "tenant", "actor")
	ctx := t.Context()
	// Only metadata foreign keys are needed; no warehouse is queried by this
	// store regression and these synthetic receipts are not executable plans.
	sql(t, raw, `INSERT INTO chartworks.source_revisions(tenant_id,source_id,revision,context_id,name,connection_alias,binding,created_by) VALUES('tenant','source',1,'source:v1','Synthetic source','synthetic','{"tenant":"tenant","source":"source","context":"source:v1","dialect":"postgres","revision":1}','actor'); INSERT INTO chartworks.sources VALUES('tenant','source',1)`)
	started := time.Now().UTC().Truncate(time.Microsecond)
	variants := []readexec.RemoteQuery{
		{Driver: "postgres", Postgres: &readexec.PostgresRemoteQuery{PID: 42, Started: started}},
		{Driver: "mysql", MySQL: &readexec.MySQLRemoteQuery{ConnectionID: 42, Account: "reader", Database: "warehouse", ServerUUID: "server-1"}},
		{Driver: "sqlserver", SQLServer: &readexec.SQLServerRemoteQuery{SessionID: 42, RequestID: 1, Started: started, Server: "server-1", Account: "reader", Database: "warehouse"}},
		{Driver: "bigquery", BigQuery: &readexec.BigQueryRemoteQuery{Project: "project-1", Location: "US", JobID: "job-1"}},
		{Driver: "snowflake", Snowflake: &readexec.SnowflakeRemoteQuery{RequestID: "request-1", QueryTag: "cw:attempt-1", Account: "account-1", Database: "warehouse", SessionID: 42}},
		{Driver: "databricks", Databricks: &readexec.DatabricksRemoteQuery{Workspace: "https://workspace.example", Warehouse: "warehouse-1"}},
		{Driver: "snowflake", Snowflake: &readexec.SnowflakeRemoteQuery{RequestID: "request-1", QueryTag: "cw:attempt-1", Account: "account-1", Database: "warehouse", SessionID: 42, QueryID: "query-1"}},
		{Driver: "databricks", Databricks: &readexec.DatabricksRemoteQuery{Workspace: "https://workspace.example", Warehouse: "warehouse-1", StatementID: "statement-1"}},
	}
	for i, dispatched := range variants {
		name := dispatched.Driver
		if i >= 6 {
			name += "_known_id"
		}
		t.Run(name, func(t *testing.T) {
			id := fmt.Sprintf("%032x", i+1)
			dispatched.Tag = "cw-read:" + id
			acknowledged := dispatched
			coordinatePath, identifierPath := []string(nil), []string(nil)
			switch dispatched.Driver {
			case "snowflake":
				native := *dispatched.Snowflake
				native.QueryID = "query-1"
				acknowledged.Snowflake = &native
				coordinatePath, identifierPath = []string{"snowflake", "account"}, []string{"snowflake", "query_id"}
			case "databricks":
				native := *dispatched.Databricks
				native.StatementID = "statement-1"
				acknowledged.Databricks = &native
				coordinatePath, identifierPath = []string{"databricks", "warehouse_id"}, []string{"databricks", "statement_id"}
			}
			now := time.Now().UTC().Truncate(time.Microsecond)
			attempt := readexec.Attempt{
				ID: id, Number: 1, Created: now, Deadline: now.Add(time.Minute),
				Manifest: readexec.Manifest{Operation: "ack-" + name, Session: "session", Receipt: readexec.Receipt{Validated: true, Source: "source", Context: "source:v1", Dialect: dispatched.Driver, Contract: "contract", Columns: []string{"value"}, Manifest: strings.Repeat("a", 64)}, Limits: readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Minute, CancelGrace: time.Second, PlannerCost: 1000}},
			}
			if err := db.BeginRead(ctx, scope, attempt, 3); err != nil {
				t.Fatal("admit read", err)
			}
			if err := db.DispatchRead(ctx, scope, id, dispatched, false); err != nil {
				t.Fatal("record dispatch identity", err)
			}
			encoded, err := json.Marshal(acknowledged)
			if err != nil {
				t.Fatal(err)
			}
			reject := func(statement string, args ...any) {
				t.Helper()
				_, err := raw.Exec(ctx, statement, args...)
				var constraint *pgconn.PgError
				if !errors.As(err, &constraint) || constraint.Code != "23514" {
					t.Fatalf("immutable read rewrite was not rejected by PostgreSQL: %v", err)
				}
			}
			if !dispatched.Controllable() {
				// Exercise the database trigger directly as well as the public
				// store's typed acknowledgment checks.
				reject(`UPDATE chartworks.read_attempts SET remote_query=jsonb_set($2::jsonb,$3,'"different"'),status='running' WHERE attempt_id=$1`, id, encoded, coordinatePath)
				reject(`UPDATE chartworks.read_attempts SET remote_query=$2,status='dispatching' WHERE attempt_id=$1`, id, encoded)
				reject(`UPDATE chartworks.read_attempts SET remote_query=jsonb_set($2::jsonb,$3,'null'),status='running' WHERE attempt_id=$1`, id, encoded, identifierPath)
				reject(`UPDATE chartworks.read_attempts SET remote_query=jsonb_set($2::jsonb,$3,'42'),status='running' WHERE attempt_id=$1`, id, encoded, identifierPath)
				reject(`UPDATE chartworks.read_attempts SET remote_query=jsonb_set($2::jsonb,$3,'""'),status='running' WHERE attempt_id=$1`, id, encoded, identifierPath)
				reject(`UPDATE chartworks.read_attempts SET remote_query=$2,status='running',manifest=jsonb_set(manifest,'{session}','"other"') WHERE attempt_id=$1`, id, encoded)
				conflicting := acknowledged
				conflicting.Tag = "cw-read:" + strings.Repeat("f", 32)
				if err := db.DispatchRead(ctx, scope, id, conflicting, true); !errors.Is(err, store.ErrInvalid) {
					t.Fatal("conflicting acknowledgment accepted", err)
				}
			} else if identifierPath != nil {
				reject(`UPDATE chartworks.read_attempts SET remote_query=jsonb_set(remote_query,$2,'"replacement"'),status='running' WHERE attempt_id=$1`, id, identifierPath)
			}
			if err := db.DispatchRead(ctx, scope, id, acknowledged, true); err != nil {
				t.Fatal("record native acknowledgment", err)
			}
			retained, err := db.GetRead(ctx, scope, id)
			if err != nil || retained.Status != "running" || retained.RemoteState != "running" || retained.Remote == nil || readexec.Hash(*retained.Remote) != readexec.Hash(acknowledged) || readexec.Hash(retained.Manifest) != readexec.Hash(attempt.Manifest) || !retained.Created.Equal(now) || !retained.Deadline.Equal(attempt.Deadline) {
				t.Fatal("acknowledgment changed immutable evidence or lost native identity", err, retained)
			}
			if err := db.DispatchRead(ctx, scope, id, acknowledged, true); !errors.Is(err, readexec.ErrCancelled) {
				t.Fatal("second acknowledgment reopened running read", err)
			}
			if identifierPath != nil {
				reject(`UPDATE chartworks.read_attempts SET remote_query=jsonb_set(remote_query,$2,'"replacement"') WHERE attempt_id=$1`, id, identifierPath)
				reject(`UPDATE chartworks.read_attempts SET remote_query=remote_query #- $2::text[] WHERE attempt_id=$1`, id, identifierPath)
				reject(`UPDATE chartworks.read_attempts SET remote_query=NULL WHERE attempt_id=$1`, id)
			}
		})
	}
}
