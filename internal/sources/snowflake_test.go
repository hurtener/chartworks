package sources

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/bruin-data/bruin/pkg/query"
	bruinsnowflake "github.com/bruin-data/bruin/pkg/snowflake"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestSnowflakeReadMapping(t *testing.T) {
	if _, err := snowflakeArguments([]readexec.Parameter{{Kind: "number", Value: "1.25"}}); err != readexec.ErrUnsupported {
		t.Fatalf("inexact decimal accepted: %v", err)
	}
	capture := &cloudObserverCapture{}
	observer := snowflakeObserver{observer: capture, tag: "cw-read:0123456789abcdef0123456789abcdef"}
	pending := bruinsnowflake.ReadIdentity{RequestID: "01234567-89ab-5def-8123-456789abcdef", QueryTag: "cw:attempt", Account: "account", Database: "database", SessionID: 7}
	ack := pending
	ack.QueryID = "01b-query"
	if err := observer.OnDispatch(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	if err := observer.OnAcknowledged(t.Context(), ack); err != nil {
		t.Fatal(err)
	}
	if len(capture.calls) != 2 || capture.calls[0].Controllable() || !capture.calls[1].Controllable() || !capture.calls[0].Acknowledges(capture.calls[1]) {
		t.Fatalf("snowflake identity transition: %#v", capture.calls)
	}
	for native, want := range map[string]string{"NUMBER(38,3)": "decimal", "BINARY": "binary", "TIMESTAMP_TZ": "temporal", "VARIANT": "structured"} {
		if got := snowflakeCategory(native); got != want {
			t.Fatalf("%s: %s", native, got)
		}
	}
}

type snowflakeFixtureClient struct{ closed bool }

func (c *snowflakeFixtureClient) OpenRead(ctx context.Context, q *query.Query, attempt string, o bruinsnowflake.ReadObserver) (query.RowStream, bruinsnowflake.ReadIdentity, error) {
	pending := bruinsnowflake.ReadIdentity{RequestID: "01234567-89ab-5def-8123-456789abcdef", QueryTag: "cw:" + attempt, Account: "account", Database: "database", SessionID: 7}
	if err := o.OnDispatch(ctx, pending); err != nil {
		return nil, pending, err
	}
	ack := pending
	ack.QueryID = "01b-query"
	var rows query.RowStream
	switch {
	case strings.Contains(strings.ToLower(q.Query), "information_schema.tables"):
		rows = &cloudRows{columns: []query.Column{{Name: "table_type", DatabaseType: "TEXT"}}, rows: [][]any{{"BASE TABLE"}}}
	case strings.Contains(strings.ToLower(q.Query), "information_schema.columns"):
		rows = &cloudRows{columns: []query.Column{{Name: "column_name", DatabaseType: "TEXT"}, {Name: "data_type", DatabaseType: "TEXT"}, {Name: "is_nullable", DatabaseType: "TEXT"}, {Name: "ordinal_position", DatabaseType: "NUMBER(38,0)"}}, rows: [][]any{{"id", "NUMBER(38,0)", "NO", big.NewInt(1)}}}
	case strings.HasPrefix(q.Query, "EXPLAIN"):
		rows = &cloudRows{columns: []query.Column{{Name: "plan", DatabaseType: "TEXT"}}, rows: [][]any{{`{"plan":"scan"}`}}}
	default:
		rows = &cloudRows{columns: []query.Column{{Name: "id", DatabaseType: "NUMBER(38,0)", Precision: 38, Scale: 0, DecimalKnown: true}}, rows: [][]any{{big.NewInt(42)}}}
	}
	if err := o.OnAcknowledged(ctx, ack); err != nil {
		return nil, ack, err
	}
	return rows, ack, nil
}
func (*snowflakeFixtureClient) ReadStatus(context.Context, bruinsnowflake.ReadIdentity) (bruinsnowflake.ReadState, error) {
	return bruinsnowflake.ReadStateStopped, nil
}
func (*snowflakeFixtureClient) CancelRead(context.Context, bruinsnowflake.ReadIdentity) error {
	return nil
}
func (c *snowflakeFixtureClient) Close() error { c.closed = true; return nil }

func TestSnowflakeSourceLifecycleInjected(t *testing.T) {
	raw, _ := json.Marshal(bruinsnowflake.Config{Account: "account", Database: "database", Schema: "analytics", Token: "token"})
	repo := &cloudMemoryRepository{records: map[string]Record{}}
	service, err := New(repo, cloudSettings("snowflake", "SF_CONFIG"), func(name string) (string, bool) { return string(raw), name == "SF_CONFIG" })
	if err != nil {
		t.Fatal(err)
	}
	client := &snowflakeFixtureClient{}
	service.newSnowflake = func(*bruinsnowflake.Config) (snowflakeClient, error) { return client, nil }
	e := cloudEnvelope(t, "source")
	source, err := service.Create(t.Context(), e, CreateRequest{ID: "source", Name: "Source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: "SELECT id FROM analytics.sales"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "0123456789abcdef0123456789abcdef", &cloudObserverCapture{})
	if err != nil || result.RemoteState != "stopped" || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"42"` {
		t.Fatalf("snowflake execution: %#v %v", result, err)
	}
	service.Close()
	if !client.closed {
		t.Fatal("snowflake client not closed")
	}
}
