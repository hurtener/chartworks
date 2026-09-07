package sources

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"

	bruinbigquery "github.com/bruin-data/bruin/pkg/bigquery"
	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type cloudObserverCapture struct {
	calls    []readexec.RemoteQuery
	accepted []bool
}

func (o *cloudObserverCapture) Dispatch(_ context.Context, q readexec.RemoteQuery, accepted bool) error {
	o.calls = append(o.calls, q)
	o.accepted = append(o.accepted, accepted)
	return nil
}
func (*cloudObserverCapture) Check(context.Context) error { return nil }

func TestBigQueryReadMapping(t *testing.T) {
	params, err := bigQueryArguments([]readexec.Parameter{{Kind: "integer", Value: "42"}, {Kind: "number", Value: "123.450"}, {Kind: "null"}})
	if err != nil || len(params) != 3 {
		t.Fatalf("arguments: %v %#v", err, params)
	}
	decimal := params[1].(bruinbigquery.ReadParameter).Value.(*big.Rat)
	if decimal.RatString() != "2469/20" {
		t.Fatalf("decimal changed: %s", decimal)
	}
	capture := &cloudObserverCapture{}
	observer := bigQueryObserver{observer: capture, tag: "cw-read:0123456789abcdef0123456789abcdef"}
	identity := bruinbigquery.ReadIdentity{ProjectID: "project", Location: "us", JobID: "bruin_read_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}
	if err = observer.OnDispatch(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	if err = observer.OnAcknowledged(t.Context(), identity); err != nil {
		t.Fatal(err)
	}
	if len(capture.calls) != 2 || capture.accepted[0] || !capture.accepted[1] || !capture.calls[1].Valid() || !capture.calls[0].Acknowledges(capture.calls[1]) {
		t.Fatalf("journal mapping: %#v %#v", capture.calls, capture.accepted)
	}
	field, err := cloudField(query.Column{Name: "amount", DatabaseType: "BIGNUMERIC", Precision: 38, Scale: 3, DecimalKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cloudValue(field, query.Column{Scale: 3, DecimalKnown: true}, decimal)
	if err != nil || string(raw) != "123.450" {
		t.Fatalf("precision: %q %v", raw, err)
	}
}

type cloudMemoryRepository struct {
	mu      sync.Mutex
	records map[string]Record
}

func (r *cloudMemoryRepository) PutSource(_ context.Context, scope store.Scope, expected int64, next Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := scope.Tenant() + "/" + next.Source.ID
	current, ok := r.records[key]
	if (!ok && expected != 0) || (ok && current.Source.Revision != expected) {
		return store.ErrConflict
	}
	r.records[key] = next
	return nil
}
func (r *cloudMemoryRepository) ReadSource(_ context.Context, scope store.Scope, id string) (Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[scope.Tenant()+"/"+id]
	if !ok {
		return Record{}, store.ErrNotFound
	}
	return record, nil
}
func (r *cloudMemoryRepository) ListSources(context.Context, store.Scope, access.Selection, int) ([]Source, error) {
	return nil, nil
}
func (r *cloudMemoryRepository) WithSource(ctx context.Context, scope store.Scope, id string, fn func(context.Context, Record) error) error {
	record, err := r.ReadSource(ctx, scope, id)
	if err != nil {
		return err
	}
	return fn(ctx, record)
}

type cloudRows struct {
	columns []query.Column
	rows    [][]any
	index   int
	current []any
	err     error
}

func (r *cloudRows) Columns() []query.Column { return append([]query.Column(nil), r.columns...) }
func (r *cloudRows) Next() bool {
	if r.index >= len(r.rows) {
		return false
	}
	r.current = append([]any(nil), r.rows[r.index]...)
	r.index++
	return true
}
func (r *cloudRows) Values() ([]any, error) {
	if r.current == nil {
		return nil, errors.New("no current row")
	}
	return append([]any(nil), r.current...), nil
}
func (r *cloudRows) Err() error   { return r.err }
func (r *cloudRows) Close() error { return nil }

func cloudEnvelope(t *testing.T, id string) identity.Envelope {
	t.Helper()
	contextID := id + ":v1"
	dataset := "ds:" + readexec.Hash([]string{id, "analytics", "sales"})[:32]
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"sources.write", "sources.read", "sources.rotate", "sources.query", "cw.tenant.write:tenant", "cw.source.read:" + id, "cw.source.write:" + id, "cw.source.query:" + id, "cw.execution_context.use:" + contextID, "cw.dataset.query:" + dataset}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func cloudSettings(dialect, rawName string) config.Sources {
	settings := config.DefaultSources()
	settings.Enabled = true
	settings.Connections = []config.SourceConnection{{
		Dialect: dialect, Tenant: "tenant", ID: "warehouse", Version: "v1", ReadDSN: "env:" + rawName,
		Relations: []config.SourceRelation{{Schema: "analytics", Name: "sales", Columns: []string{"id"}}},
	}}
	return settings
}

type bigQueryFixtureClient struct {
	closed        bool
	project       string
	location      string
	attempts      []string
	userReads     int
	dryRuns       int
	failure       string
	iterationErr  error
	onCatalogRead func()
}

func (c *bigQueryFixtureClient) projectID() string {
	if c.project != "" {
		return c.project
	}
	return "synthetic-project"
}

func (c *bigQueryFixtureClient) queryLocation() string {
	if c.location != "" {
		return c.location
	}
	return "US"
}

func (c *bigQueryFixtureClient) OpenRead(ctx context.Context, q *query.Query, attempt string, o bruinbigquery.ReadObserver, _ bruinbigquery.ReadOptions) (query.RowStream, bruinbigquery.ReadIdentity, error) {
	c.attempts = append(c.attempts, attempt)
	catalogRead := strings.Contains(q.Query, "INFORMATION_SCHEMA")
	if catalogRead && c.onCatalogRead != nil {
		callback := c.onCatalogRead
		c.onCatalogRead = nil
		callback()
	}
	id := bruinbigquery.ReadIdentity{ProjectID: c.projectID(), Location: c.queryLocation(), JobID: "bruin_read_" + readexec.Hash(attempt)}
	if !catalogRead {
		c.userReads++
		if c.failure == "before_dispatch" {
			return nil, id, errors.New("synthetic pre-dispatch failure")
		}
	}
	if err := o.OnDispatch(ctx, id); err != nil {
		return nil, id, err
	}
	if !catalogRead && c.failure == "after_dispatch" {
		return nil, id, errors.New("synthetic post-dispatch failure")
	}
	var rows query.RowStream
	switch {
	case strings.Contains(q.Query, "INFORMATION_SCHEMA.TABLES"):
		rows = &cloudRows{columns: []query.Column{{Name: "table_type", DatabaseType: "STRING"}}, rows: [][]any{{"BASE TABLE"}}}
	case strings.Contains(q.Query, "INFORMATION_SCHEMA.COLUMNS"):
		rows = &cloudRows{columns: []query.Column{{Name: "column_name", DatabaseType: "STRING"}, {Name: "data_type", DatabaseType: "STRING"}, {Name: "is_nullable", DatabaseType: "STRING"}, {Name: "ordinal_position", DatabaseType: "INTEGER"}}, rows: [][]any{{"id", "INT64", "NO", int64(1)}}}
	default:
		rows = &cloudRows{columns: []query.Column{{Name: "id", DatabaseType: "INTEGER"}}, rows: [][]any{{int64(42)}}, err: c.iterationErr}
	}
	if err := o.OnAcknowledged(ctx, id); err != nil {
		return nil, id, err
	}
	return rows, id, nil
}
func (c *bigQueryFixtureClient) DryRunRead(context.Context, *query.Query, int64) (bruinbigquery.ReadDryRun, error) {
	c.dryRuns++
	return bruinbigquery.ReadDryRun{StatementType: "SELECT", TotalBytesProcessed: 512, ReferencedTables: []string{c.projectID() + ".analytics.sales"}, Columns: []query.Column{{Name: "id", DatabaseType: "INTEGER"}}}, nil
}
func (*bigQueryFixtureClient) ReadStatus(context.Context, bruinbigquery.ReadIdentity) (bruinbigquery.ReadState, error) {
	return bruinbigquery.ReadStateStopped, nil
}
func (*bigQueryFixtureClient) CancelRead(context.Context, bruinbigquery.ReadIdentity, bruinbigquery.ReadOptions) (bruinbigquery.ReadState, error) {
	return bruinbigquery.ReadStateStopped, nil
}
func (c *bigQueryFixtureClient) Close() error { c.closed = true; return nil }

func TestBigQuerySourceLifecycleInjected(t *testing.T) {
	raw := `{"ProjectID":"synthetic-project","Location":"US","UseApplicationDefaultCredentials":true}`
	repo := &cloudMemoryRepository{records: map[string]Record{}}
	settings := cloudSettings("bigquery", "BQ_CONFIG")
	service, err := New(repo, settings, func(name string) (string, bool) { return raw, name == "BQ_CONFIG" })
	if err != nil {
		t.Fatal(err)
	}
	client := &bigQueryFixtureClient{}
	service.newBigQuery = func(*bruinbigquery.Config) (bigQueryClient, error) { return client, nil }
	e := cloudEnvelope(t, "source")
	source, err := service.Create(t.Context(), e, CreateRequest{ID: "source", Name: "Source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	if got := repo.records["tenant/source"].Binding.Catalog; got != "synthetic-project" {
		t.Fatalf("project was not bound as catalog: %q", got)
	}
	discovery, err := service.Discover(t.Context(), e, source.ID)
	if err != nil || len(discovery.Relations) != 1 || discovery.Relations[0].Columns[0].Category != "integer" {
		t.Fatalf("discovery: %#v %v", discovery, err)
	}
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: "SELECT id FROM `synthetic-project.analytics.sales`"})
	if err != nil {
		t.Fatal(err)
	}
	capture := &cloudObserverCapture{}
	result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "0123456789abcdef0123456789abcdef", capture)
	if err != nil || result.RemoteState != "stopped" || len(result.Result.Rows) != 1 || string(result.Result.Rows[0][0]) != `"42"` {
		t.Fatalf("execution: %#v %v", result, err)
	}
	rotated, err := service.Rotate(t.Context(), e, source.ID, 1)
	if err != nil || rotated.Revision != 2 {
		t.Fatalf("rotation: %#v %v", rotated, err)
	}
	if _, err = service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "fedcba9876543210fedcba9876543210", capture); !errors.Is(err, readexec.ErrBinding) {
		t.Fatalf("old plan survived rotation: %v", err)
	}
	service.Close()
	if !client.closed {
		t.Fatal("cloud client not closed")
	}
}

func validatedBigQueryPlan(t *testing.T, lookup func(string) (string, bool), factory bigQueryFactory) (*Service, identity.Envelope, readexec.Plan) {
	t.Helper()
	repo := &cloudMemoryRepository{records: map[string]Record{}}
	service, err := New(repo, cloudSettings("bigquery", "BQ_CONFIG"), lookup)
	if err != nil {
		t.Fatal(err)
	}
	service.newBigQuery = factory
	e := cloudEnvelope(t, "source")
	source, err := service.Create(t.Context(), e, CreateRequest{ID: "source", Name: "Source", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	plan, err := validator.Validate(t.Context(), e, readexec.Request{Source: source.ID, Context: source.ContextID, SQL: "SELECT id FROM `synthetic-project.analytics.sales`"})
	if err != nil {
		t.Fatal(err)
	}
	return service, e, plan
}

func TestBigQueryExecutionFailurePreservesRemoteState(t *testing.T) {
	raw := `{"ProjectID":"synthetic-project","Location":"US","UseApplicationDefaultCredentials":true}`
	client := &bigQueryFixtureClient{}
	service, e, plan := validatedBigQueryPlan(t, func(name string) (string, bool) { return raw, name == "BQ_CONFIG" }, func(*bruinbigquery.Config) (bigQueryClient, error) { return client, nil })
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
			attempt := fmt.Sprintf("%032x", i+1)
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

func TestBigQueryExecutionRevalidatesReplacementAndKeepsCapturedClient(t *testing.T) {
	raw := `{"ProjectID":"synthetic-project","Location":"US","UseApplicationDefaultCredentials":true}`
	var clients []*bigQueryFixtureClient
	factory := func(native *bruinbigquery.Config) (bigQueryClient, error) {
		client := &bigQueryFixtureClient{project: native.ProjectID, location: native.Location}
		if native.AccessToken == "rotated" {
			client.onCatalogRead = func() {
				raw = `{"ProjectID":"third-project","Location":"US","UseApplicationDefaultCredentials":true}`
			}
		}
		clients = append(clients, client)
		return client, nil
	}
	service, e, plan := validatedBigQueryPlan(t, func(name string) (string, bool) { return raw, name == "BQ_CONFIG" }, factory)
	t.Cleanup(service.Close)

	raw = `{"ProjectID":"replacement-project","Location":"US","UseApplicationDefaultCredentials":true}`
	validator, err := readexec.NewValidator(service, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = validator.Validate(t.Context(), e, readexec.Request{Source: "source", Context: "source:v1", SQL: "SELECT id FROM `synthetic-project.analytics.sales`"}); !errors.Is(err, readexec.ErrBinding) || len(clients) != 2 || clients[1].dryRuns != 0 {
		t.Fatalf("replacement context reached native planning: err=%v clients=%d dry-runs=%d", err, len(clients), clients[len(clients)-1].dryRuns)
	}
	result, err := service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "11111111111111111111111111111111", &cloudObserverCapture{})
	if !errors.Is(err, readexec.ErrBinding) || result.RemoteState != "not_issued" || len(clients) != 2 || clients[1].userReads != 0 {
		t.Fatalf("replacement context was not denied before dispatch: result=%#v err=%v clients=%d reads=%d", result, err, len(clients), clients[len(clients)-1].userReads)
	}

	raw = `{"ProjectID":"synthetic-project","Location":"US","AccessToken":"rotated","UseApplicationDefaultCredentials":true}`
	result, err = service.ExecuteRead(t.Context(), e, plan, readexec.Limits{Rows: 10, Bytes: 4096, Timeout: time.Second, CancelGrace: time.Second, PlannerCost: 1024}, "22222222222222222222222222222222", &cloudObserverCapture{})
	if err != nil || result.RemoteState != "stopped" || len(clients) != 3 || clients[2].userReads != 1 {
		t.Fatalf("captured client was not retained through dispatch: result=%#v err=%v clients=%d reads=%d", result, err, len(clients), clients[len(clients)-1].userReads)
	}
}

func TestBigQueryProbeUsesFreshAttemptTags(t *testing.T) {
	raw := `{"ProjectID":"synthetic-project","Location":"US","UseApplicationDefaultCredentials":true}`
	client := &bigQueryFixtureClient{}
	service, e, _ := validatedBigQueryPlan(t, func(name string) (string, bool) { return raw, name == "BQ_CONFIG" }, func(*bruinbigquery.Config) (bigQueryClient, error) { return client, nil })
	t.Cleanup(service.Close)
	client.attempts = nil
	if _, err := service.Discover(t.Context(), e, "source"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Discover(t.Context(), e, "source"); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, attempt := range client.attempts {
		if seen[attempt] {
			t.Fatalf("metadata probe reused attempt tag %q", attempt)
		}
		seen[attempt] = true
	}
	if len(seen) != 4 {
		t.Fatalf("recorded %d unique metadata attempts, want 4", len(seen))
	}
}
