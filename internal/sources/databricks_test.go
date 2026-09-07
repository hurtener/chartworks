package sources

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	bruindatabricks "github.com/bruin-data/bruin/pkg/databricks"
	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
)

func TestDatabricksReadMapping(t *testing.T) {
	params, err := databricksArguments([]readexec.Parameter{{Kind: "number", Value: "123.450"}, {Kind: "boolean", Value: "true"}})
	if err != nil {
		t.Fatal(err)
	}
	if params[0].(bruindatabricks.ReadParameter).Value.(*big.Rat).RatString() != "2469/20" {
		t.Fatal("decimal changed")
	}
	capture := &cloudObserverCapture{}
	observer := databricksObserver{observer: capture, tag: "cw-read:0123456789abcdef0123456789abcdef"}
	pending := bruindatabricks.ReadIdentity{Workspace: "https://workspace.example", WarehouseID: "warehouse", AttemptTag: "attempt"}
	ack := pending
	ack.StatementID = "01234567-89ab-cdef-8123-456789abcdef"
	if err = observer.OnDispatch(t.Context(), pending); err != nil {
		t.Fatal(err)
	}
	if err = observer.OnAcknowledged(t.Context(), ack); err != nil {
		t.Fatal(err)
	}
	if len(capture.calls) != 2 || capture.calls[0].Controllable() || !capture.calls[1].Controllable() || !capture.calls[0].Acknowledges(capture.calls[1]) {
		t.Fatalf("databricks identity transition: %#v", capture.calls)
	}
	field, err := cloudField(query.Column{Name: "amount", DatabaseType: "DECIMAL(38,3)", Precision: 38, Scale: 3, DecimalKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cloudValue(field, query.Column{Scale: 3, DecimalKnown: true}, params[0].(bruindatabricks.ReadParameter).Value)
	if err != nil || string(raw) != "123.450" {
		t.Fatalf("precision: %q %v", raw, err)
	}
}

type databricksFixtureClient struct{ closed bool }

func (c *databricksFixtureClient) OpenRead(ctx context.Context, q *query.Query, attempt string, o bruindatabricks.ReadObserver, _ bruindatabricks.ReadOptions) (query.RowStream, bruindatabricks.ReadIdentity, error) {
	pending := bruindatabricks.ReadIdentity{Workspace: "https://workspace.example", WarehouseID: "warehouse", AttemptTag: attempt}
	if err := o.OnDispatch(ctx, pending); err != nil {
		return nil, pending, err
	}
	ack := pending
	ack.StatementID = "01234567-89ab-cdef-8123-456789abcdef"
	var rows query.RowStream
	switch {
	case strings.Contains(strings.ToLower(q.Query), "information_schema.tables"):
		rows = &cloudRows{columns: []query.Column{{Name: "table_type", DatabaseType: "STRING"}}, rows: [][]any{{"MANAGED"}}}
	case strings.Contains(strings.ToLower(q.Query), "information_schema.columns"):
		rows = &cloudRows{columns: []query.Column{{Name: "column_name", DatabaseType: "STRING"}, {Name: "full_data_type", DatabaseType: "STRING"}, {Name: "is_nullable", DatabaseType: "STRING"}, {Name: "ordinal_position", DatabaseType: "INT"}}, rows: [][]any{{"id", "BIGINT", "NO", int64(1)}}}
	case strings.HasPrefix(q.Query, "EXPLAIN"):
		rows = &cloudRows{columns: []query.Column{{Name: "plan", DatabaseType: "STRING"}}, rows: [][]any{{"scan"}}}
	default:
		rows = &cloudRows{columns: []query.Column{{Name: "id", DatabaseType: "BIGINT"}}, rows: [][]any{{int64(42)}}}
	}
	if err := o.OnAcknowledged(ctx, ack); err != nil {
		return nil, ack, err
	}
	return rows, ack, nil
}
func (*databricksFixtureClient) ReadStatus(context.Context, bruindatabricks.ReadIdentity) (bruindatabricks.ReadState, error) {
	return bruindatabricks.ReadStateStopped, nil
}
func (*databricksFixtureClient) CancelRead(context.Context, bruindatabricks.ReadIdentity, bruindatabricks.ReadOptions) (bruindatabricks.ReadState, error) {
	return bruindatabricks.ReadStateStopped, nil
}
func (c *databricksFixtureClient) Close() error { c.closed = true; return nil }
func TestDatabricksSourceLifecycleRecorded(t *testing.T) {
	raw, _ := json.Marshal(bruindatabricks.Config{Host: "workspace.example", Port: 443, Path: "/sql/1.0/warehouses/warehouse", Catalog: "catalog", Schema: "analytics", Token: "token"})
	repo := &cloudMemoryRepository{records: map[string]Record{}}
	service, err := New(repo, cloudSettings("databricks", "DBX_CONFIG"), func(name string) (string, bool) { return string(raw), name == "DBX_CONFIG" })
	if err != nil {
		t.Fatal(err)
	}
	client := &databricksFixtureClient{}
	service.newDatabricks = func(*bruindatabricks.Config) (databricksClient, error) { return client, nil }
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
		t.Fatalf("databricks execution: %#v %v", result, err)
	}
	service.Close()
	if !client.closed {
		t.Fatal("databricks client not closed")
	}
}
