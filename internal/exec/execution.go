package exec

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

var (
	// ErrCancelled is a deliberate cancellation, not a valid empty result.
	ErrCancelled = errors.New("exec: cancelled")
	// ErrTimeout is a client/server deadline, never an invitation to broaden SQL.
	ErrTimeout = errors.New("exec: timed out")
	// ErrUncertain means query termination or durable outcome could not be proved.
	ErrUncertain = errors.New("exec: outcome uncertain; inspect or reconcile the attempt")
	// ErrReplay prevents a repeated physical attempt from executing twice.
	ErrReplay = errors.New("exec: attempt already admitted; read its receipt")
)

// Limits is the effective bounded execution contract, shared by interactive and job consumers.
type Limits struct {
	Rows        int           `json:"rows"`
	Bytes       int           `json:"bytes"`
	Timeout     time.Duration `json:"timeout_ns"`
	CancelGrace time.Duration `json:"cancel_grace_ns"`
	PlannerCost float64       `json:"planner_cost_ceiling"`
}

// Valid checks every execution ceiling independently of caller authority.
func (l Limits) Valid() bool {
	return l.Rows > 0 && l.Rows <= 100000 && l.Bytes >= 128 && l.Bytes <= 16<<20 && l.Timeout >= time.Millisecond && l.Timeout <= time.Minute && l.CancelGrace >= time.Millisecond && l.CancelGrace <= 3*time.Second && l.PlannerCost > 0 && l.PlannerCost <= 1e12 && !math.IsNaN(l.PlannerCost)
}

// Options explicitly identifies one logical operation and one physical attempt.
// Reusing the same attempt never reruns a query. Retried failures require Number+1;
// uncertain attempts require reconciliation first. Result values are not cached.
type Options struct {
	Operation string `json:"operation"`
	Number    int    `json:"attempt"`
	Preview   bool   `json:"preview"`
	Rows      int    `json:"rows"`
	Bytes     int    `json:"bytes"`
}

// RemoteQuery is a closed, engine-tagged native control identity. It contains no
// credential and cannot be constructed from request input. Only PostgreSQL is
// currently executable; the remaining variants reserve exact native coordinates
// without introducing a generic, forgeable handle.
type RemoteQuery struct {
	Driver     string                 `json:"driver"`
	Tag        string                 `json:"tag"`
	Postgres   *PostgresRemoteQuery   `json:"postgres,omitempty"`
	MySQL      *MySQLRemoteQuery      `json:"mysql,omitempty"`
	SQLServer  *SQLServerRemoteQuery  `json:"sqlserver,omitempty"`
	BigQuery   *BigQueryRemoteQuery   `json:"bigquery,omitempty"`
	Snowflake  *SnowflakeRemoteQuery  `json:"snowflake,omitempty"`
	Databricks *DatabricksRemoteQuery `json:"databricks,omitempty"`
}

// PostgresRemoteQuery identifies an owned backend by PID and start time.
type PostgresRemoteQuery struct {
	PID     uint32    `json:"pid"`
	Started time.Time `json:"backend_started"`
}

// MySQLRemoteQuery identifies a connection in its server and account context.
type MySQLRemoteQuery struct {
	ConnectionID uint64 `json:"connection_id"`
	Account      string `json:"account"`
	Database     string `json:"database"`
	ServerUUID   string `json:"server_uuid"`
}

// SQLServerRemoteQuery identifies an owned session and request with its start time.
type SQLServerRemoteQuery struct {
	SessionID int32     `json:"session_id"`
	RequestID int32     `json:"request_id"`
	Started   time.Time `json:"started_at"`
	Server    string    `json:"server"`
	Account   string    `json:"account"`
	Database  string    `json:"database"`
}

// BigQueryRemoteQuery identifies a job in its project and location.
type BigQueryRemoteQuery struct {
	Project  string `json:"project"`
	Location string `json:"location"`
	JobID    string `json:"job_id"`
}

// SnowflakeRemoteQuery retains observed request, session and query coordinates.
type SnowflakeRemoteQuery struct {
	RequestID string `json:"request_id,omitempty"`
	QueryTag  string `json:"query_tag,omitempty"`
	Account   string `json:"account,omitempty"`
	Database  string `json:"database,omitempty"`
	SessionID int64  `json:"session_id,omitempty"`
	QueryID   string `json:"query_id,omitempty"`
}

// DatabricksRemoteQuery identifies a statement within its workspace and warehouse.
type DatabricksRemoteQuery struct {
	Workspace   string `json:"workspace"`
	Warehouse   string `json:"warehouse_id"`
	StatementID string `json:"statement_id,omitempty"`
}

// NewPostgresRemoteQuery constructs the only currently executable variant.
func NewPostgresRemoteQuery(pid uint32, started time.Time, tag string) RemoteQuery {
	return RemoteQuery{Driver: "postgres", Tag: tag, Postgres: &PostgresRemoteQuery{PID: pid, Started: started}}
}

// Valid checks a canonical native backend identity without exposing its secret.
func (q RemoteQuery) Valid() bool {
	if len(q.Tag) != 40 || q.Tag[:8] != "cw-read:" || !hashID(q.Tag[8:]) {
		return false
	}
	variants := 0
	for _, present := range []bool{q.Postgres != nil, q.MySQL != nil, q.SQLServer != nil, q.BigQuery != nil, q.Snowflake != nil, q.Databricks != nil} {
		if present {
			variants++
		}
	}
	if variants != 1 {
		return false
	}
	switch q.Driver {
	case "postgres":
		return q.Postgres != nil && q.Postgres.PID > 0 && !q.Postgres.Started.IsZero()
	case "mysql":
		return q.MySQL != nil && q.MySQL.ConnectionID > 0 && remoteText(q.MySQL.Account, 256) && remoteText(q.MySQL.Database, 128) && remoteCoordinate(q.MySQL.ServerUUID, 128)
	case "sqlserver":
		return q.SQLServer != nil && q.SQLServer.SessionID > 0 && q.SQLServer.RequestID >= 0 && !q.SQLServer.Started.IsZero() && remoteText(q.SQLServer.Server, 256) && remoteText(q.SQLServer.Account, 256) && remoteText(q.SQLServer.Database, 128)
	case "bigquery":
		return q.BigQuery != nil && remoteCoordinate(q.BigQuery.Project, 128) && remoteCoordinate(q.BigQuery.Location, 64) && remoteCoordinate(q.BigQuery.JobID, 256)
	case "snowflake":
		return q.Snowflake != nil && remoteCoordinate(q.Snowflake.RequestID, 128) && remoteCoordinate(q.Snowflake.QueryTag, 128) && remoteText(q.Snowflake.Account, 128) && remoteText(q.Snowflake.Database, 128) && q.Snowflake.SessionID > 0 && (q.Snowflake.QueryID == "" || remoteCoordinate(q.Snowflake.QueryID, 128))
	case "databricks":
		return q.Databricks != nil && remoteHTTPSOrigin(q.Databricks.Workspace) && remoteCoordinate(q.Databricks.Warehouse, 128) && (q.Databricks.StatementID == "" || remoteCoordinate(q.Databricks.StatementID, 128))
	}
	return false
}

func remoteText(s string, maximum int) bool {
	return len(s) > 0 && len(s) <= maximum && !strings.ContainsAny(s, "\x00\r\n\t")
}

func remoteHTTPSOrigin(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && u.User == nil && u.Path == "" && u.RawQuery == "" && u.Fragment == ""
}

// Controllable reports whether this identity already contains the native
// coordinate required to observe or interrupt the submitted operation. Some
// APIs disclose that coordinate only in the server acknowledgement.
func (q RemoteQuery) Controllable() bool {
	if !q.Valid() {
		return false
	}
	switch q.Driver {
	case "snowflake":
		return q.Snowflake.QueryID != ""
	case "databricks":
		return q.Databricks.StatementID != ""
	default:
		return true
	}
}

// Acknowledges permits only a monotonic enrichment of one pre-dispatch
// identity. Native identifiers cannot be replaced after they are known.
func (q RemoteQuery) Acknowledges(next RemoteQuery) bool {
	if !q.Valid() || !next.Valid() || q.Driver != next.Driver || q.Tag != next.Tag || !next.Controllable() {
		return false
	}
	if q.Controllable() {
		switch q.Driver {
		case "postgres":
			return q.Postgres.PID == next.Postgres.PID && q.Postgres.Started.Equal(next.Postgres.Started)
		case "mysql":
			return *q.MySQL == *next.MySQL
		case "sqlserver":
			return q.SQLServer.SessionID == next.SQLServer.SessionID && q.SQLServer.RequestID == next.SQLServer.RequestID && q.SQLServer.Started.Equal(next.SQLServer.Started) && q.SQLServer.Server == next.SQLServer.Server && q.SQLServer.Account == next.SQLServer.Account && q.SQLServer.Database == next.SQLServer.Database
		case "bigquery":
			return *q.BigQuery == *next.BigQuery
		case "snowflake":
			return *q.Snowflake == *next.Snowflake
		case "databricks":
			return *q.Databricks == *next.Databricks
		}
		return false
	}
	switch q.Driver {
	case "snowflake":
		return q.Snowflake.RequestID == next.Snowflake.RequestID && q.Snowflake.QueryTag == next.Snowflake.QueryTag && q.Snowflake.Account == next.Snowflake.Account && q.Snowflake.Database == next.Snowflake.Database && q.Snowflake.SessionID == next.Snowflake.SessionID
	case "databricks":
		return q.Databricks.Workspace == next.Databricks.Workspace && q.Databricks.Warehouse == next.Databricks.Warehouse
	}
	return false
}

func remoteCoordinate(s string, maximum int) bool {
	if len(s) < 1 || len(s) > maximum {
		return false
	}
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

// UnmarshalJSON accepts the phase-10 PostgreSQL shape so retained attempts remain
// inspectable and reconcilable after the tagged-union migration.
func (q *RemoteQuery) UnmarshalJSON(data []byte) error {
	type wire RemoteQuery
	var current wire
	if err := decodeRemote(data, &current); err == nil && current.Driver != "" {
		*q = RemoteQuery(current)
		if !q.Valid() {
			return ErrBinding
		}
		return nil
	}
	var legacy struct {
		PID     uint32    `json:"pid"`
		Started time.Time `json:"backend_started"`
		Tag     string    `json:"tag"`
	}
	if err := decodeRemote(data, &legacy); err != nil {
		return err
	}
	*q = NewPostgresRemoteQuery(legacy.PID, legacy.Started, legacy.Tag)
	if !q.Valid() {
		return ErrBinding
	}
	return nil
}
func decodeRemote(data []byte, out any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err != nil {
			return err
		}
		return ErrBinding
	}
	return nil
}
func hashID(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 16 && hex.EncodeToString(b) == s
}

// Manifest retains content-free operation identity and exact dependency reach.
// It stores neither SQL/parameters, result values nor JWTs.
type Manifest struct {
	Operation string  `json:"operation"`
	Session   string  `json:"session"`
	Receipt   Receipt `json:"validation"`
	Limits    Limits  `json:"limits"`
	Preview   bool    `json:"preview"`
}

// Valid checks bounded retained coordinates and exact validation metadata.
func (m Manifest) Valid() bool {
	r := m.Receipt
	if !identity.Identifier(m.Operation) || !identity.Identifier(m.Session) || !r.Validated || !identity.Identifier(r.Source) || !identity.Identifier(r.Context) || r.Dialect != "" && !remoteDialect(r.Dialect) || !identity.Identifier(r.Contract) || len(r.Dependencies) > 32 || len(r.Columns) < 1 || len(r.Columns) > 256 || !m.Limits.Valid() {
		return false
	}
	hash, err := hex.DecodeString(r.Manifest)
	if err != nil || len(hash) != 32 || hex.EncodeToString(hash) != r.Manifest {
		return false
	}
	seen := map[string]bool{}
	for _, id := range r.Dependencies {
		if !identity.Identifier(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	for _, name := range r.Columns {
		if len(name) < 1 || len(name) > 1024 {
			return false
		}
	}
	return true
}
func remoteDialect(s string) bool {
	switch s {
	case "postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks":
		return true
	}
	return false
}

// Attempt is protected, content-free execution evidence; active receipts expire
// into uncertainty rather than pretending a process crash completed its query.
type Attempt struct {
	ID              string       `json:"id"`
	Number          int          `json:"number"`
	Manifest        Manifest     `json:"manifest"`
	Status          string       `json:"status"`
	Remote          *RemoteQuery `json:"remote"`
	RemoteState     string       `json:"remote_state"`
	CancelRequested bool         `json:"cancel_requested"`
	Created         time.Time    `json:"created_at"`
	Deadline        time.Time    `json:"deadline"`
	Finished        *time.Time   `json:"finished_at"`
	Rows            int          `json:"rows_returned"`
	Bytes           int          `json:"bytes_returned"`
	Code            string       `json:"code"`
}

// AttemptStore journals physical attempts independently from the source-revision
// transaction. Capacity reserves a connection for journal/cancel traffic.
type AttemptStore interface {
	ReadCapacity() int
	BeginRead(context.Context, store.Scope, Attempt, int) error
	DispatchRead(context.Context, store.Scope, string, RemoteQuery, bool) error
	GetRead(context.Context, store.Scope, string) (Attempt, error)
	GetReadOperation(context.Context, store.Scope, string) (Attempt, error)
	CancelRead(context.Context, store.Scope, string) error
	FinishRead(context.Context, store.Scope, Attempt, bool) error
}

// Observer journals intent BEFORE issuing SQL and acknowledgment AFTER DECLARE.
// Check is also called between cursor fetches, so cancellation is not a transient signal.
type Observer interface {
	Dispatch(context.Context, RemoteQuery, bool) error
	Check(context.Context) error
}

// NativeResult reports termination separately from the logical result.
type NativeResult struct {
	Result      Result
	RemoteState string
}

// ExecutionAdapter is the plan-only concrete source boundary. It contains no raw SQL API.
type ExecutionAdapter interface {
	ReadCapabilities() Capabilities
	ReadAdapter
	ExecuteRead(context.Context, identity.Envelope, Plan, Limits, string, Observer) (NativeResult, error)
	ControlRead(context.Context, identity.Envelope, Control, bool) (string, error)
}

// Control can only be created from an authorized retained attempt by Executor.
// An opaque query ID, PID or serialized receipt alone cannot cancel a query.
type Control struct {
	attempt Attempt
	actor   string
	tenant  string
}

// Target checks the current signed reach and exact source context before control I/O.
func (c Control) Target(e identity.Envelope, b Binding) (RemoteQuery, error) {
	a := c.attempt
	dialect := a.Manifest.Receipt.Dialect
	if dialect == "" {
		dialect = "postgres"
	}
	if c.actor != e.User() || c.tenant != e.Tenant() || a.Manifest.Session != e.Session() || a.Remote == nil || !a.Remote.Valid() || a.Remote.Driver != dialect || b.Dialect != dialect || a.Manifest.Receipt.Source != b.Source || a.Manifest.Receipt.Context != b.Context {
		return RemoteQuery{}, ErrBinding
	}
	if !a.Remote.Controllable() {
		return RemoteQuery{}, ErrUncertain
	}
	if err := Require(e, b, a.Manifest.Receipt.Dependencies); err != nil {
		return RemoteQuery{}, err
	}
	return *a.Remote, nil
}

// Coordinates permits a control adapter to load its current protected source binding.
func (c Control) Coordinates() (string, string) {
	return c.attempt.Manifest.Receipt.Source, c.attempt.Manifest.Receipt.Context
}

// ExecutionReport distinguishes a terminal query failure from transport/admission failure.
// HTTP 200 means an accepted attempt has a durable receipt; inspect Attempt.Status.
type ExecutionReport struct {
	Capabilities Capabilities `json:"capabilities"`
	Attempt      Attempt      `json:"attempt"`
	Result       *Result      `json:"result"`
}

// Executor is one model-free read core; it never rewrites SQL or retries automatically.
type Executor struct {
	adapter  ExecutionAdapter
	repo     AttemptStore
	settings config.ReadValidation
	slots    chan struct{}
}

func nilDependency(v any) bool {
	return v == nil || reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil()
}

// NewExecutor reserves bounded concurrent reads and leaves journal/control capacity.
func NewExecutor(adapter ExecutionAdapter, repo AttemptStore, settings config.ReadValidation) (*Executor, error) {
	if nilDependency(adapter) || nilDependency(repo) || config.ValidateReadValidation(settings) != nil || repo.ReadCapacity() < settings.ExecutionConcurrency+2 {
		return nil, ErrBinding
	}
	return &Executor{adapter: adapter, repo: repo, settings: settings, slots: make(chan struct{}, settings.ExecutionConcurrency)}, nil
}
func (x *Executor) limits(o Options) (Limits, error) {
	r := o.Rows
	if r == 0 {
		r = x.settings.RowsDefault
	}
	b := o.Bytes
	if b == 0 {
		b = x.settings.BytesDefault
	}
	if !identity.Identifier(o.Operation) || o.Number < 1 || o.Number > x.settings.MaxReadAttempts || r < 1 || r > x.settings.RowsCeiling || b < 128 || b > x.settings.BytesCeiling {
		return Limits{}, ErrLimit
	}
	if o.Preview && r > x.settings.PreviewRows {
		r = x.settings.PreviewRows
	}
	return Limits{Rows: r, Bytes: b, Timeout: time.Duration(x.settings.Timeout), CancelGrace: time.Duration(x.settings.CancelGrace), PlannerCost: x.settings.PlannerCostCeiling}, nil
}

// Execute requires a currently authorized nonzero plan, then admits exactly one
// explicit attempt. A failed durable finalization discards rows and leaves recovery evidence.
func (x *Executor) execute(ctx context.Context, e identity.Envelope, p Plan, o Options, caps *Caps) (ExecutionReport, error) {
	if ctx == nil || !e.Valid() {
		return ExecutionReport{}, ErrBinding
	}
	if _, _, err := p.SQL(e, p.candidate.binding); err != nil {
		return ExecutionReport{}, err
	}
	limits, err := x.limits(o)
	if err != nil {
		return ExecutionReport{}, err
	}
	if caps != nil {
		limits.Rows = min(limits.Rows, caps.Rows)
		limits.Bytes = min(limits.Bytes, caps.Bytes)
		limits.Timeout = min(limits.Timeout, caps.Timeout)
		limits.PlannerCost = min(limits.PlannerCost, caps.PlannerCost)
	}
	ctx, cancel := context.WithTimeout(ctx, limits.Timeout)
	defer cancel()
	ctx, expiry := context.WithDeadline(ctx, e.Deadline())
	defer expiry()
	select {
	case x.slots <- struct{}{}:
		defer func() { <-x.slots }()
	case <-ctx.Done():
		return ExecutionReport{}, ctx.Err()
	}
	if err = ctx.Err(); err != nil {
		return ExecutionReport{}, err
	}
	var random [16]byte
	if _, err = rand.Read(random[:]); err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	deadline, _ := ctx.Deadline()
	a := Attempt{ID: hex.EncodeToString(random[:]), Number: o.Number, Manifest: Manifest{Operation: o.Operation, Session: e.Session(), Receipt: p.Receipt(), Limits: limits, Preview: o.Preview}, Status: "accepted", RemoteState: "not_issued", Created: time.Now().UTC(), Deadline: deadline.UTC()}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return ExecutionReport{}, err
	}
	if err = reserveAttempt(ctx, e, o); err != nil {
		return ExecutionReport{}, err
	}
	if err = x.repo.BeginRead(ctx, scope, a, x.settings.MaxReadAttempts); err != nil {
		return ExecutionReport{}, err
	}
	observer := &readObserver{repo: x.repo, scope: scope, id: a.ID}
	nativeContext, joinWatcher := watchRead(ctx, observer)
	native, runErr := x.adapter.ExecuteRead(nativeContext, e, p, limits, a.ID, observer)
	if watchErr := joinWatcher(); watchErr != nil {
		runErr = watchErr
		native.Result = Result{}
	}
	status, code := "failed", "source_unavailable"
	if runErr == nil {
		status = native.Result.Outcome
		code = ""
		if status != "succeeded" && status != "empty" && status != "truncated" {
			runErr = ErrType
			status = "failed"
			code = "invalid_result"
		}
	} else {
		switch {
		case errors.Is(runErr, ErrCancelled), errors.Is(runErr, context.Canceled):
			status, code = "cancelled", "cancelled"
		case errors.Is(runErr, ErrTimeout), errors.Is(runErr, context.DeadlineExceeded):
			status, code = "timed_out", "timed_out"
		case errors.Is(runErr, ErrUncertain):
			status, code = "uncertain", "remote_outcome_unknown"
		case errors.Is(runErr, ErrType):
			code = "result_type_unsupported"
		case errors.Is(runErr, ErrLimit):
			code = "limit_exceeded"
		case errors.Is(runErr, ErrQuery):
			code = "query_error"
		case errors.Is(runErr, ErrBinding):
			code = "context_changed"
		case errors.Is(runErr, ErrUnsupported):
			code = "unsupported"
		}
	}
	if native.RemoteState == "unknown" {
		status, code = "uncertain", "remote_outcome_unknown"
	}
	cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), limits.CancelGrace)
	defer stop()
	current, err := x.repo.GetRead(cleanup, scope, a.ID)
	if err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	if current.CancelRequested && status != "uncertain" {
		status, code = "cancelled", "cancelled"
		runErr = ErrCancelled
	}
	current.Status = status
	current.Code = code
	current.RemoteState = native.RemoteState
	if current.RemoteState == "" {
		current.RemoteState = "not_issued"
	}
	if runErr == nil && status != "uncertain" {
		current.Rows = len(native.Result.Rows)
		current.Bytes = native.Result.Bytes
	}
	now := time.Now().UTC()
	current.Finished = &now
	if err = x.repo.FinishRead(cleanup, scope, current, false); err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	current, err = x.repo.GetRead(cleanup, scope, a.ID)
	if err != nil {
		return ExecutionReport{}, ErrUncertain
	}
	status = current.Status
	report := ExecutionReport{Attempt: current, Capabilities: x.adapter.ReadCapabilities()}
	if runErr == nil && (status == "succeeded" || status == "empty" || status == "truncated") {
		report.Result = &native.Result
	}
	return report, nil
}

type readObserver struct {
	repo  AttemptStore
	scope store.Scope
	id    string
}

func (o *readObserver) Dispatch(ctx context.Context, q RemoteQuery, accepted bool) error {
	return o.repo.DispatchRead(ctx, o.scope, o.id, q, accepted)
}
func (o *readObserver) Check(ctx context.Context) error {
	a, err := o.repo.GetRead(ctx, o.scope, o.id)
	if err != nil {
		return err
	}
	if a.CancelRequested {
		return ErrCancelled
	}
	if a.Status == "uncertain" || a.Finished != nil {
		return ErrUncertain
	}
	return nil
}

func (x *Executor) authorized(ctx context.Context, e identity.Envelope, id string) (Attempt, store.Scope, error) {
	if ctx == nil || !e.Valid() || !hashID(id) {
		return Attempt{}, store.Scope{}, ErrBinding
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Attempt{}, scope, err
	}
	a, err := x.repo.GetRead(ctx, scope, id)
	if err != nil {
		return Attempt{}, scope, err
	}
	if a.Manifest.Session != e.Session() {
		return Attempt{}, scope, store.ErrNotFound
	}
	b, err := x.adapter.Binding(ctx, e, a.Manifest.Receipt.Source, a.Manifest.Receipt.Context)
	if err != nil {
		return Attempt{}, scope, err
	}
	if err = Require(e, b, a.Manifest.Receipt.Dependencies); err != nil {
		return Attempt{}, scope, err
	}
	return a, scope, nil
}

// Inspect reads content-free evidence with fresh actor/session/source/context reach.
func (x *Executor) Inspect(ctx context.Context, e identity.Envelope, id string) (Attempt, error) {
	a, _, err := x.authorized(ctx, e, id)
	return a, err
}

// ControlReceipt never equates delivery of a cancel request with remote termination.
type ControlReceipt struct {
	Attempt     Attempt `json:"attempt"`
	RemoteState string  `json:"remote_state"`
}

// Control requests cancellation or reconciles a stopped uncertain attempt. It does
// not reconstruct rows, rerun queries or infer successful execution after a crash.
func (x *Executor) Control(ctx context.Context, e identity.Envelope, id string, cancel bool) (ControlReceipt, error) {
	a, scope, err := x.authorized(ctx, e, id)
	if err != nil {
		return ControlReceipt{}, err
	}
	if a.Finished != nil && a.Status != "uncertain" {
		return ControlReceipt{Attempt: a, RemoteState: a.RemoteState}, nil
	}
	if cancel {
		if err = x.repo.CancelRead(ctx, scope, id); err != nil {
			return ControlReceipt{}, err
		}
		a.CancelRequested = true
	}
	state := "not_issued"
	if a.Remote != nil {
		state, err = x.adapter.ControlRead(ctx, e, Control{attempt: a, tenant: e.Tenant(), actor: e.User()}, cancel)
		if err != nil {
			state = "unknown"
			if errors.Is(err, ErrUnsupported) {
				state = "unsupported"
			}
		}
	}
	// Only a crashed/expired attempt is reconciled here. A live owner remains
	// responsible for its result and observes the durable cancel flag at fetch boundaries.
	if a.Status == "uncertain" && (state == "stopped" || state == "not_issued") {
		a.Status = "interrupted"
		a.Code = "result_not_retained"
		a.RemoteState = state
		now := time.Now().UTC()
		a.Finished = &now
		if err = x.repo.FinishRead(ctx, scope, a, true); err != nil {
			return ControlReceipt{}, err
		}
	}
	return ControlReceipt{Attempt: a, RemoteState: state}, nil
}

// watchRead owns one bounded, joined cancellation observer per active read.
// Metadata failure cancels work instead of silently losing the cancellation fence.
func watchRead(ctx context.Context, observer Observer) (context.Context, func() error) {
	child, cancel := context.WithCancel(ctx)
	stop := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				done <- nil
				return
			case <-ctx.Done():
				done <- nil
				return
			case <-ticker.C:
				probe, end := context.WithTimeout(child, time.Second)
				err := observer.Check(probe)
				end()
				if err != nil {
					cancel()
					done <- err
					return
				}
			}
		}
	}()
	return child, func() error { close(stop); err := <-done; cancel(); return err }
}

// ByOperation recovers a content-free attempt after a lost response. It never reruns values.
func (x *Executor) ByOperation(ctx context.Context, e identity.Envelope, operation string) (Attempt, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(operation) {
		return Attempt{}, ErrBinding
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return Attempt{}, err
	}
	a, err := x.repo.GetReadOperation(ctx, scope, operation)
	if err != nil {
		return Attempt{}, err
	}
	return x.Inspect(ctx, e, a.ID)
}

// ValidateOptions rejects unsupported invocation bounds before native validation I/O.
// It is admission only and never authorizes or constructs an executable plan.
func (x *Executor) ValidateOptions(o Options) error {
	_, err := x.limits(o)
	return err
}
