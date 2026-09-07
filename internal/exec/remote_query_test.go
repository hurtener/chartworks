package exec

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestRemoteQueryClosedVariantsAndLegacyPostgres(t *testing.T) {
	tag := "cw-read:" + strings.Repeat("a", 32)
	started := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	postgres := NewPostgresRemoteQuery(42, started, tag)
	if !postgres.Valid() {
		t.Fatal("postgres variant rejected")
	}
	legacy, _ := json.Marshal(struct {
		PID     uint32    `json:"pid"`
		Started time.Time `json:"backend_started"`
		Tag     string    `json:"tag"`
	}{42, started, tag})
	var decoded RemoteQuery
	if err := json.Unmarshal(legacy, &decoded); err != nil || !decoded.Valid() || decoded.Driver != "postgres" || decoded.Postgres.PID != 42 || !decoded.Postgres.Started.Equal(started) {
		t.Fatal("legacy postgres identity did not migrate", err, decoded)
	}
	if err := json.Unmarshal(append(legacy[:len(legacy)-1], []byte(`,"unknown":1}`)...), &decoded); err == nil {
		t.Fatal("unknown legacy coordinate accepted")
	}
	currentWithUnknown := []byte(`{"driver":"postgres","tag":"` + tag + `","postgres":{"pid":42,"backend_started":"2026-09-07T12:00:00Z"},"unknown":1}`)
	if err := json.Unmarshal(currentWithUnknown, &decoded); err == nil {
		t.Fatal("unknown tagged coordinate accepted")
	}
	encoded, err := json.Marshal(decoded)
	if err != nil || !strings.Contains(string(encoded), `"driver":"postgres"`) || !strings.Contains(string(encoded), `"postgres":{"pid":42`) {
		t.Fatal("legacy identity did not use tagged representation", err, string(encoded))
	}

	valid := []RemoteQuery{
		{Driver: "mysql", Tag: tag, MySQL: &MySQLRemoteQuery{ConnectionID: 7}},
		{Driver: "sqlserver", Tag: tag, SQLServer: &SQLServerRemoteQuery{SessionID: 8, RequestID: 0, Started: started}},
		{Driver: "bigquery", Tag: tag, BigQuery: &BigQueryRemoteQuery{Project: "project-1", Location: "us-central1", JobID: "job_1"}},
		{Driver: "snowflake", Tag: tag, Snowflake: &SnowflakeRemoteQuery{QueryID: "01b2-ABC"}},
		{Driver: "databricks", Tag: tag, Databricks: &DatabricksRemoteQuery{StatementID: "01b2-abc"}},
	}
	for _, q := range valid {
		if !q.Valid() {
			t.Fatal("closed variant rejected", q.Driver)
		}
	}
	invalid := []RemoteQuery{
		{},
		{Driver: "postgres", Tag: tag, Postgres: &PostgresRemoteQuery{PID: 1, Started: started}, MySQL: &MySQLRemoteQuery{ConnectionID: 1}},
		{Driver: "mysql", Tag: tag, Postgres: &PostgresRemoteQuery{PID: 1, Started: started}},
		{Driver: "other", Tag: tag, Snowflake: &SnowflakeRemoteQuery{QueryID: "id"}},
		{Driver: "bigquery", Tag: tag, BigQuery: &BigQueryRemoteQuery{Project: "bad/project", Location: "us", JobID: "job"}},
		{Driver: "databricks", Tag: tag, Databricks: &DatabricksRemoteQuery{StatementID: strings.Repeat("a", 129)}},
	}
	for _, q := range invalid {
		if q.Valid() {
			t.Fatal("malformed or cross-driver identity accepted", q.Driver)
		}
	}
}

func TestReceiptDialectCompatibility(t *testing.T) {
	m := Manifest{Operation: "operation", Session: "session", Receipt: Receipt{Validated: true, Source: "source", Context: "context", Contract: "contract", Dependencies: []string{}, Columns: []string{"value"}, Manifest: strings.Repeat("a", 64)}, Limits: Limits{Rows: 1, Bytes: 128, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1}}
	if !m.Valid() {
		t.Fatal("legacy receipt without dialect rejected")
	}
	for _, dialect := range []string{"postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks"} {
		m.Receipt.Dialect = dialect
		if !m.Valid() {
			t.Fatal("closed dialect rejected", dialect)
		}
	}
	m.Receipt.Dialect = "generic"
	if m.Valid() {
		t.Fatal("open-ended dialect accepted")
	}
}

func TestRemoteQueryAcknowledgementIsMonotonic(t *testing.T) {
	tag := "cw-read:" + strings.Repeat("b", 32)
	pending := RemoteQuery{Driver: "snowflake", Tag: tag, Snowflake: &SnowflakeRemoteQuery{RequestID: "request-1", Account: "account", Database: "analytics"}}
	accepted := pending
	accepted.Snowflake = &SnowflakeRemoteQuery{RequestID: "request-1", Account: "account", Database: "analytics", QueryID: "query-1"}
	if !pending.Valid() || pending.Controllable() || !pending.Acknowledges(accepted) || !accepted.Controllable() {
		t.Fatal("valid pending identity did not admit exact native acknowledgement")
	}
	replaced := accepted
	replaced.Snowflake = &SnowflakeRemoteQuery{RequestID: "request-1", Account: "account", Database: "analytics", QueryID: "query-2"}
	if accepted.Acknowledges(replaced) {
		t.Fatal("accepted native identity was replaceable")
	}
	wrong := accepted
	wrong.Snowflake = &SnowflakeRemoteQuery{RequestID: "request-2", Account: "account", Database: "analytics", QueryID: "query-1"}
	if pending.Acknowledges(wrong) {
		t.Fatal("acknowledgement changed immutable submission coordinates")
	}
	dbPending := RemoteQuery{Driver: "databricks", Tag: tag, Databricks: &DatabricksRemoteQuery{RequestID: "request-1", Workspace: "workspace", Warehouse: "warehouse"}}
	dbAccepted := dbPending
	dbAccepted.Databricks = &DatabricksRemoteQuery{RequestID: "request-1", Workspace: "workspace", Warehouse: "warehouse", StatementID: "statement-1"}
	if !dbPending.Valid() || dbPending.Controllable() || !dbPending.Acknowledges(dbAccepted) {
		t.Fatal("Databricks pending identity did not require its native statement ID")
	}
}
