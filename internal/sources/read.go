package sources

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var _ readexec.ExecutionAdapter = (*Service)(nil)

// ExecuteRead is the sole native result path. Its metadata revision fence lasts
// through source cleanup, independently of ordinary short metadata transactions.
func (s *Service) ExecuteRead(ctx context.Context, e identity.Envelope, p readexec.Plan, l readexec.Limits, id string, observer readexec.Observer) (out readexec.NativeResult, err error) {
	out.RemoteState = "not_issued"
	if ctx == nil || !e.Valid() || !l.Valid() || len(id) != 32 {
		return out, readexec.ErrBinding
	}
	if b, decodeErr := hex.DecodeString(id); decodeErr != nil || len(b) != 16 {
		return out, readexec.ErrBinding
	}
	source, partition := p.Coordinates()
	if source == "" || partition == "" {
		return out, readexec.ErrBinding
	}
	scope, err := sourceScope(e, "sources.query", "query", source)
	if err != nil {
		return out, err
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed || !s.settings.Enabled {
		return out, store.ErrUnavailable
	}
	ctx, stop := context.WithTimeout(ctx, l.Timeout)
	defer stop()
	ctx, expiry := context.WithDeadline(ctx, e.Deadline())
	defer expiry()
	if err = ctx.Err(); err != nil {
		return out, err
	}
	until, _ := ctx.Deadline()
	fence, release := context.WithDeadline(context.WithoutCancel(ctx), until.Add(l.CancelGrace+time.Second))
	defer release()
	err = s.repo.WithSource(fence, scope, source, func(_ context.Context, record Record) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if record.Source.ContextID != partition {
			return readexec.ErrBinding
		}
		if _, _, err := p.SQL(e, record.Binding); err != nil {
			return err
		}
		if observer != nil {
			if err := observer.Check(ctx); err != nil {
				return err
			}
		}
		connection, err := s.recordConnection(record)
		if err != nil {
			return err
		}
		out, err = s.executeNative(ctx, e, p, record, connection, l, id, observer)
		return err
	})
	if err != nil {
		out.Result = readexec.Result{}
	}
	return out, err
}

func (s *Service) executeNative(ctx context.Context, e identity.Envelope, p readexec.Plan, record Record, c config.SourceConnection, l readexec.Limits, id string, observer readexec.Observer) (out readexec.NativeResult, err error) {
	out.RemoteState = "not_issued"
	var remote readexec.RemoteQuery
	pool, location, err := s.pool(ctx, c)
	if err != nil {
		return out, err
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return out, safe(err)
	}
	defer conn.Release()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, safe(err)
	}
	defer func() {
		queryErr := err
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), l.CancelGrace)
		defer stop()
		rollback := tx.Rollback(cleanup)
		if out.RemoteState != "not_issued" {
			if rollback == nil {
				out.RemoteState = "stopped"
			} else {
				out.RemoteState = "unknown"
				out.Result = readexec.Result{}
				err = readexec.ErrUncertain
			}
		}
		if rollback != nil {
			_ = conn.Conn().Close(cleanup)
			conn.Release()
			// Closing a socket alone does not prove remote termination. Independently
			// observe the exact tagged backend identity under the already accepted
			// operation's bounded cleanup allowance; never signal a reusable PID.
			if remote.Valid() && out.RemoteState == "unknown" {
				var active bool
				observeErr := pool.QueryRow(cleanup, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE pid=$1 AND backend_start=$2 AND application_name=$3 AND usename=current_user AND datname=current_database())`, remote.PID, remote.Started, remote.Tag).Scan(&active)
				if observeErr == nil && !active {
					out.RemoteState = "stopped"
					if queryErr != nil {
						err = readFailure(ctx, queryErr)
					}
					if errors.Is(ctx.Err(), context.DeadlineExceeded) {
						err = readexec.ErrTimeout
					} else if errors.Is(ctx.Err(), context.Canceled) {
						err = readexec.ErrCancelled
					}
				}
			}
		}
		if err != nil {
			out.Result = readexec.Result{}
			err = readFailure(ctx, err)
		}
	}()
	binding, err := s.inspectReadContext(ctx, tx, c, record.Source.ID, record.Source.Revision, location)
	if err != nil {
		return out, err
	}
	statement, parameters, err := p.SQL(e, binding)
	if err != nil {
		return out, err
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('TimeZone','UTC',true),set_config('DateStyle','ISO, YMD',true),set_config('IntervalStyle','iso_8601',true),set_config('bytea_output','hex',true),set_config('extra_float_digits','3',true),set_config('lc_monetary','C',true)`); err != nil {
		return out, err
	}
	if err = remainingReadTime(ctx, tx); err != nil {
		return out, err
	}
	cost, err := explainCost(ctx, tx, statement, parameters)
	if err != nil {
		return out, err
	}
	if cost > l.PlannerCost {
		return out, readexec.ErrLimit
	}
	remote = readexec.RemoteQuery{Tag: "cw-read:" + id}
	if err = tx.QueryRow(ctx, `SELECT pg_backend_pid(),backend_start FROM pg_catalog.pg_stat_activity WHERE pid=pg_backend_pid()`).Scan(&remote.PID, &remote.Started); err != nil {
		return out, err
	}
	if !remote.Valid() {
		return out, readexec.ErrBinding
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('application_name',$1,true)`, remote.Tag); err != nil {
		return out, err
	}
	if observer != nil {
		if err = observer.Dispatch(ctx, remote, false); err != nil {
			return out, err
		}
	}
	out.RemoteState = "running" // durable intent exists; a transport failure may be ambiguous
	args, err := arguments(parameters)
	if err != nil {
		return out, err
	}
	// DECLARE preserves the validated query verbatim. FETCH bounds transport without
	// a LIMIT wrapper, reordered SELECT or application-side authorization filtering.
	if _, err = tx.Exec(ctx, "DECLARE chartworks_read NO SCROLL CURSOR WITHOUT HOLD FOR "+statement, args...); err != nil {
		return out, err
	}
	if observer != nil {
		if err = observer.Dispatch(ctx, remote, true); err != nil {
			return out, err
		}
	}
	var collector *readexec.Collector
	for {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if observer != nil {
			if err = observer.Check(ctx); err != nil {
				return out, err
			}
		}
		if err = remainingReadTime(ctx, tx); err != nil {
			return out, err
		}
		count := 128
		if collector != nil {
			left := l.Rows - len(collector.Result().Rows) + 1
			if count > left {
				count = left
			}
		}
		rows, queryErr := tx.Query(ctx, "FETCH FORWARD "+strconv.Itoa(count)+" FROM chartworks_read", pgx.QueryExecModeDescribeExec, pgx.QueryResultFormatsByOID{790: pgx.BinaryFormatCode})
		if queryErr != nil {
			return out, queryErr
		}
		fields, fieldErr := readResultFields(rows)
		if fieldErr != nil {
			return out, fieldErr
		}
		if collector == nil {
			schema := make([]readexec.Field, len(fields))
			for i, f := range fields {
				schema[i], err = resultField(f.Name, f.DataTypeOID)
				if err != nil {
					rows.Close()
					return out, err
				}
			}
			collector, err = readexec.NewCollector(schema, l.Rows, l.Bytes)
			if err != nil {
				rows.Close()
				return out, err
			}
		}
		seen := 0
		more := true
		for rows.Next() {
			seen++
			raw := rows.RawValues()
			for i, f := range fields {
				if f.DataTypeOID == 790 && raw[i] != nil {
					raw[i], err = moneyDecimal(raw[i])
					if err != nil {
						rows.Close()
						return out, err
					}
				}
			}
			more, err = collector.Add(raw)
			if err != nil {
				rows.Close()
				return out, err
			}
			if !more {
				break
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return out, err
		}
		if !more || seen < count {
			break
		}
	}
	if observer != nil {
		if err = observer.Check(ctx); err != nil {
			return out, err
		}
	}
	if !e.Valid() {
		return out, readexec.ErrBinding
	}
	out.Result = collector.Result()
	out.Result.Cost.PlannerUnits = &cost
	return out, nil
}

func remainingReadTime(ctx context.Context, tx readTransaction) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return readexec.ErrBinding
	}
	remaining := time.Until(deadline)
	if remaining < time.Millisecond {
		return readexec.ErrTimeout
	}
	_, err := tx.Exec(ctx, `SELECT set_config('statement_timeout',$1,true),set_config('transaction_timeout',$1,true),set_config('idle_in_transaction_session_timeout',$1,true)`, strconv.FormatInt(remaining.Milliseconds(), 10))
	return err
}

func explainCost(ctx context.Context, tx readTransaction, statement string, parameters []readexec.Parameter) (float64, error) {
	args, err := arguments(parameters)
	if err != nil {
		return 0, err
	}
	var data []byte
	if err = tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+statement, args...).Scan(&data); err != nil {
		return 0, err
	}
	var result []struct {
		Plan struct {
			Total *float64 `json:"Total Cost"`
		} `json:"Plan"`
	}
	if len(data) > 1<<20 || json.Unmarshal(data, &result) != nil || len(result) != 1 || result[0].Plan.Total == nil {
		return 0, readexec.ErrType
	}
	cost := *result[0].Plan.Total
	if cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return 0, readexec.ErrType
	}
	return cost, nil
}

func readFailure(ctx context.Context, err error) error {
	if errors.Is(err, readexec.ErrUncertain) {
		return readexec.ErrUncertain
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return readexec.ErrTimeout
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return readexec.ErrCancelled
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) && (pg.Code == "57014" || pg.Code == "25P03" || pg.Code == "25P04") {
		return readexec.ErrTimeout
	}
	return safe(err)
}

func resultField(name string, oid uint32) (readexec.Field, error) {
	f := readexec.Field{Name: name, Encoding: "string"}
	switch oid {
	case 16:
		f.Type, f.Encoding, f.NativeType = "boolean", "boolean", "bool"
	case 20:
		f.Type, f.NativeType = "integer", "int8"
	case 21:
		f.Type, f.NativeType = "integer", "int2"
	case 23:
		f.Type, f.NativeType = "integer", "int4"
	case 1700:
		f.Type, f.NativeType = "decimal", "numeric"
	case 790:
		f.Type, f.NativeType = "decimal", "money"
	case 700:
		f.Type, f.Encoding, f.NativeType = "number", "number", "float4"
	case 701:
		f.Type, f.Encoding, f.NativeType = "number", "number", "float8"
	case 25:
		f.Type, f.NativeType = "text", "text"
	case 1042:
		f.Type, f.NativeType = "text", "bpchar"
	case 1043:
		f.Type, f.NativeType = "text", "varchar"
	case 2950:
		f.Type, f.NativeType = "text", "uuid"
	case 114:
		f.Type, f.NativeType = "structured", "json"
	case 3802:
		f.Type, f.NativeType = "structured", "jsonb"
	case 17:
		f.Type, f.NativeType = "binary", "bytea"
	case 1082:
		f.Type, f.NativeType = "temporal", "date"
	case 1083:
		f.Type, f.NativeType = "temporal", "time"
	case 1114:
		f.Type, f.NativeType = "temporal", "timestamp"
	case 1184:
		f.Type, f.NativeType = "temporal", "timestamptz"
	case 1186:
		f.Type, f.NativeType = "temporal", "interval"
	case 1266:
		f.Type, f.NativeType = "temporal", "timetz"
	default:
		return readexec.Field{}, readexec.ErrType
	}
	return f, nil
}
func moneyDecimal(raw []byte) ([]byte, error) {
	if len(raw) != 8 {
		return nil, readexec.ErrType
	}
	var n int64
	if err := binary.Read(bytes.NewReader(raw), binary.BigEndian, &n); err != nil {
		return nil, readexec.ErrType
	}
	var magnitude uint64
	sign := ""
	if n < 0 {
		magnitude = uint64(-(n + 1)) + 1
		sign = "-"
	} else {
		magnitude = uint64(n)
	}
	fraction := strconv.FormatUint(magnitude%100, 10)
	if len(fraction) == 1 {
		fraction = "0" + fraction
	}
	return []byte(sign + strconv.FormatUint(magnitude/100, 10) + "." + fraction), nil
}

// ControlRead only controls a verified, journal-backed tagged backend transaction.
// Cancellation signals are sent only by the live owner using its original connection secret.
// ControlRead itself only observes; it cannot accidentally cancel a reused backend PID.
func (s *Service) ControlRead(ctx context.Context, e identity.Envelope, control readexec.Control, _ bool) (string, error) {
	id, partition := control.Coordinates()
	scope, err := sourceScope(e, "sources.query", "query", id)
	if err != nil {
		return "unknown", err
	}
	state := "unknown"
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
			if record.Source.ContextID != partition {
				return readexec.ErrBinding
			}
			if _, err := control.Target(e, record.Binding); err != nil {
				return err
			}
			c, err := s.recordConnection(record)
			if err != nil {
				return err
			}
			// Prove actual credentials/catalog context before observing a native identity;
			// a replacement database cannot falsely prove an old backend stopped.
			_, err = s.probe(ctx, c, id, record.Source.Revision, func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
				if readexec.Hash(b) != readexec.Hash(record.Binding) {
					return readexec.ErrBinding
				}
				q, err := control.Target(e, b)
				if err != nil {
					return err
				}
				var exists bool
				if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE pid=$1 AND backend_start=$2 AND application_name=$3 AND usename=current_user AND datname=current_database())`, q.PID, q.Started, q.Tag).Scan(&exists); err != nil {
					return safe(err)
				}
				state = "stopped"
				if exists {
					state = "running"
				}
				return nil
			})
			return err
		})
	})
	return state, err
}

// readCompatibility preserves the phase-08 in-process shape and strict cap error,
// while sharing the phase-10 cursor, type and execution-authority implementation.
func (s *Service) readCompatibility(ctx context.Context, e identity.Envelope, p readexec.Plan) (Rows, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Rows{}, store.ErrUnavailable
	}
	n, err := s.ExecuteRead(ctx, e, p, readexec.Limits{Rows: s.settings.MaxRows, Bytes: s.settings.MaxBytes, Timeout: time.Duration(s.settings.QueryTimeout), CancelGrace: time.Second, PlannerCost: 1e12}, hex.EncodeToString(id[:]), nil)
	if err != nil {
		return Rows{}, err
	}
	if n.Result.Outcome == "truncated" {
		return Rows{}, readexec.ErrLimit
	}
	out := Rows{Columns: []string{}, Values: [][]*string{}}
	for _, f := range n.Result.Schema {
		out.Columns = append(out.Columns, f.Name)
	}
	for _, row := range n.Result.Rows {
		values := make([]*string, len(row))
		for i, raw := range row {
			if string(raw) == "null" {
				continue
			}
			var value string
			if n.Result.Schema[i].Encoding == "string" {
				if json.Unmarshal(raw, &value) != nil {
					return Rows{}, readexec.ErrType
				}
			} else {
				value = string(raw)
			}
			if n.Result.Schema[i].Type == "boolean" {
				if value == "true" {
					value = "t"
				} else {
					value = "f"
				}
			}
			if n.Result.Schema[i].Type == "binary" {
				value = `\x` + value
			}
			values[i] = &value
		}
		out.Values = append(out.Values, values)
	}
	return out, nil
}

// ReadCapabilities reports the qualified PostgreSQL implementation only.
func (s *Service) ReadCapabilities() readexec.Capabilities {
	return readexec.Capabilities{Cancellation: "owner_cancel_request", Reconciliation: "tagged_backend_observation", ServerDeadline: true, PlannerCost: "optimizer_estimate", ScanByteCeiling: false, ResultRetention: false}
}
