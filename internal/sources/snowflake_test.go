package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/bruin-data/bruin/pkg/query"
	bruinsnowflake "github.com/bruin-data/bruin/pkg/snowflake"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
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

type snowflakeFixtureClient struct {
	closed        bool
	database      string
	userReads     int
	failure       string
	iterationErr  error
	onCatalogRead func()
}

func (c *snowflakeFixtureClient) databaseName() string {
	if c.database != "" {
		return c.database
	}
	return "database"
}

func (c *snowflakeFixtureClient) OpenRead(ctx context.Context, q *query.Query, attempt string, o bruinsnowflake.ReadObserver) (query.RowStream, bruinsnowflake.ReadIdentity, error) {
	lower := strings.ToLower(q.Query)
	catalogRead := strings.Contains(lower, "information_schema")
	userRead := !catalogRead && !strings.HasPrefix(q.Query, "EXPLAIN")
	if catalogRead && c.onCatalogRead != nil {
		callback := c.onCatalogRead
		c.onCatalogRead = nil
		callback()
	}
	pending := bruinsnowflake.ReadIdentity{RequestID: "01234567-89ab-5def-8123-456789abcdef", QueryTag: "cw:" + attempt, Account: "account", Database: c.databaseName(), SessionID: 7}
	if userRead {
		c.userReads++
		if c.failure == "before_dispatch" {
			return nil, pending, errors.New("synthetic pre-dispatch failure")
		}
	}
	if err := o.OnDispatch(ctx, pending); err != nil {
		return nil, pending, err
	}
	if userRead && c.failure == "after_dispatch" {
		return nil, pending, errors.New("synthetic post-dispatch failure")
	}
	ack := pending
	ack.QueryID = "01b-query"
	var rows query.RowStream
	switch {
	case strings.Contains(lower, "information_schema.tables"):
		rows = &cloudRows{columns: []query.Column{{Name: "table_type", DatabaseType: "TEXT"}}, rows: [][]any{{"BASE TABLE"}}}
	case strings.Contains(lower, "information_schema.columns"):
		rows = &cloudRows{columns: []query.Column{{Name: "column_name", DatabaseType: "TEXT"}, {Name: "data_type", DatabaseType: "TEXT"}, {Name: "is_nullable", DatabaseType: "TEXT"}, {Name: "ordinal_position", DatabaseType: "NUMBER(38,0)"}}, rows: [][]any{{"id", "NUMBER(38,0)", "NO", big.NewInt(1)}}}
	case strings.HasPrefix(q.Query, "EXPLAIN"):
		rows = &cloudRows{columns: []query.Column{{Name: "plan", DatabaseType: "TEXT"}}, rows: [][]any{{`{"plan":"scan"}`}}}
	default:
		rows = &cloudRows{columns: []query.Column{{Name: "id", DatabaseType: "NUMBER(38,0)", Precision: 38, Scale: 0, DecimalKnown: true}}, rows: [][]any{{big.NewInt(42)}}, err: c.iterationErr}
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
	// #nosec G117 -- serializer input uses an inert synthetic token, not a live credential.
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
	if got := repo.records["tenant/source"].Binding.Catalog; got != "database" {
		t.Fatalf("database was not bound as catalog: %q", got)
	}
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: "SELECT id FROM database.analytics.sales"})
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

func validatedSnowflakePlan(t *testing.T, lookup func(string) (string, bool), factory snowflakeFactory) (*Service, identity.Envelope, readexec.Plan) {
	t.Helper()
	repo := &cloudMemoryRepository{records: map[string]Record{}}
	service, err := New(repo, cloudSettings("snowflake", "SF_CONFIG"), lookup)
	if err != nil {
		t.Fatal(err)
	}
	service.newSnowflake = factory
	e := cloudEnvelope(t, "source")
	source, err := service.Create(t.Context(), e, CreateRequest{ID: "source", Name: "Source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: "SELECT id FROM database.analytics.sales"})
	if err != nil {
		t.Fatal(err)
	}
	return service, e, plan
}

func TestSnowflakeExecutionFailurePreservesRemoteState(t *testing.T) {
	// #nosec G117 -- serializer input is deliberately synthetic configuration for a local injected fixture.
	raw, _ := json.Marshal(bruinsnowflake.Config{Account: "account", Database: "database", Schema: "analytics", Token: "token"})
	client := &snowflakeFixtureClient{}
	service, e, plan := validatedSnowflakePlan(t, func(name string) (string, bool) { return string(raw), name == "SF_CONFIG" }, func(*bruinsnowflake.Config) (snowflakeClient, error) { return client, nil })
	t.Cleanup(service.Close)

	tests := []struct {
		name         string
		failure      string
		iterationErr error
		wantState    string
		wantErr      bool
		wantDispatch int
	}{
		{name: "pre-dispatch failure", failure: "before_dispatch", wantState: "not_issued", wantErr: true},
		{name: "post-dispatch failure", failure: "after_dispatch", wantState: "unknown", wantErr: true, wantDispatch: 1},
		{name: "iteration failure", iterationErr: errors.New("synthetic iteration failure"), wantState: "unknown", wantErr: true, wantDispatch: 2},
		{name: "completed", wantState: "stopped", wantDispatch: 2},
	}
	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client.failure = test.failure
			client.iterationErr = test.iterationErr
			capture := &cloudObserverCapture{}
			attempt := fmt.Sprintf("%032x", i+11)
			result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, attempt, capture)
			if (err != nil) != test.wantErr || result.RemoteState != test.wantState {
				t.Fatalf("state=%q err=%v, want state=%q error=%v", result.RemoteState, err, test.wantState, test.wantErr)
			}
			if len(capture.calls) != test.wantDispatch {
				t.Fatalf("recorded %d dispatch transitions, want %d", len(capture.calls), test.wantDispatch)
			}
		})
	}
}

func TestSnowflakeExecutionRequiresRotationAfterCredentialReplacement(t *testing.T) {
	marshal := func(c bruinsnowflake.Config) string {
		// #nosec G117 -- serializer input is deliberately synthetic configuration for a local injected fixture.
		raw, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	raw := marshal(bruinsnowflake.Config{Account: "account", Username: "reader", Password: "first", Region: "region", Database: "database", Schema: "analytics"})
	var clients []*snowflakeFixtureClient
	factory := func(native *bruinsnowflake.Config) (snowflakeClient, error) {
		client := &snowflakeFixtureClient{database: native.Database}
		clients = append(clients, client)
		return client, nil
	}
	service, e, plan := validatedSnowflakePlan(t, func(name string) (string, bool) { return raw, name == "SF_CONFIG" }, factory)
	t.Cleanup(service.Close)
	clients[0].onCatalogRead = func() {
		raw = marshal(bruinsnowflake.Config{Account: "third_account", Username: "third", Password: "third", Region: "region", Database: "third_database", Schema: "analytics"})
	}
	result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "32323232323232323232323232323232", &cloudObserverCapture{})
	if err != nil || result.RemoteState != "stopped" || len(clients) != 1 || clients[0].userReads != 1 {
		t.Fatalf("captured client was replaced between probe and dispatch: result=%#v err=%v clients=%d", result, err, len(clients))
	}

	raw = marshal(bruinsnowflake.Config{Account: "account", Username: "reader", Password: "first", Region: "region", Database: "replacement_database", Schema: "analytics"})
	result, err = service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "33333333333333333333333333333333", &cloudObserverCapture{})
	if !errors.Is(err, readexec.ErrBinding) || result.RemoteState != "not_issued" || len(clients) != 2 || clients[1].userReads != 0 {
		t.Fatalf("replacement context was not denied before dispatch: result=%#v err=%v clients=%d reads=%d", result, err, len(clients), clients[len(clients)-1].userReads)
	}

	raw = marshal(bruinsnowflake.Config{Account: "account", Username: "replacement_reader", Password: "second", Region: "region", Database: "database", Schema: "analytics"})
	result, err = service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "44444444444444444444444444444444", &cloudObserverCapture{})
	if !errors.Is(err, readexec.ErrBinding) || result.RemoteState != "not_issued" || len(clients) != 3 || clients[2].userReads != 0 {
		t.Fatalf("credential replacement executed before rotation: result=%#v err=%v clients=%d reads=%d", result, err, len(clients), clients[len(clients)-1].userReads)
	}
}
