// Package postgres is the PostgreSQL implementation of the foundation store.
package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Options bounds database work. MigrationPolicy is apply or check; no down migration exists.
type Options struct {
	MaxConns                           int32
	ConnectTimeout, TransactionTimeout time.Duration
	MigrationPolicy                    string
}

// Defaults supplies the standalone driver defaults; config owns operator documentation.
func Defaults() Options {
	return Options{MaxConns: 10, ConnectTimeout: 5 * time.Second, TransactionTimeout: 5 * time.Second, MigrationPolicy: "apply"}
}

// DB owns its pool and exposes only scoped domain methods plus operator lifecycle methods.
type DB struct {
	pool    *pgxpool.Pool
	timeout time.Duration
	closed  atomic.Bool
}

var _ store.Foundation = (*DB)(nil)

// Open validates, connects, and checks/applies the compiled migration history before returning.
func Open(ctx context.Context, dsn string, opts Options) (*DB, error) {
	if opts.MaxConns < 1 || opts.MaxConns > 100 || opts.ConnectTimeout <= 0 || opts.ConnectTimeout > time.Minute || opts.TransactionTimeout <= 0 || opts.TransactionTimeout > time.Minute || (opts.MigrationPolicy != "apply" && opts.MigrationPolicy != "check") {
		return nil, store.ErrInvalid
	}
	if strings.TrimSpace(dsn) == "" || len(dsn) > 16384 || opts.TransactionTimeout < time.Millisecond {
		return nil, store.ErrInvalid
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, store.ErrInvalid
	}
	cfg.MaxConns = opts.MaxConns
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = opts.ConnectTimeout
	// Qualify application relations; user-created public functions cannot shadow helpers.
	cfg.ConnConfig.RuntimeParams["search_path"] = "pg_catalog"
	cfg.ConnConfig.RuntimeParams["application_name"] = "chartworks"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, safe(err)
	}
	db := &DB{pool: pool, timeout: opts.TransactionTimeout}
	connect, cancel := context.WithTimeout(ctx, opts.ConnectTimeout)
	defer cancel()
	if err = pool.Ping(connect); err != nil {
		pool.Close()
		return nil, safe(err)
	}
	if opts.MigrationPolicy == "apply" {
		err = db.migrate(ctx)
	} else {
		err = db.Check(ctx)
	}
	if err != nil {
		pool.Close()
		return nil, err
	}
	return db, nil
}

// Close is safe to call repeatedly. All acquired connections are private and time-bounded.
func (d *DB) Close() {
	if !d.closed.Swap(true) {
		d.pool.Close()
	}
}
func safe(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	for _, e := range []error{readexec.ErrType, readexec.ErrCancelled, readexec.ErrTimeout, readexec.ErrUncertain, readexec.ErrReplay, readexec.ErrUnsafe, readexec.ErrUnsupported, readexec.ErrBinding, readexec.ErrLimit, access.ErrUnauthenticated, access.ErrForbidden, access.ErrNotFound, jobs.ErrInvalid, jobs.ErrBusy, jobs.ErrEmpty, jobs.ErrAuthority, jobs.ErrTransient, store.ErrScope, store.ErrInvalid, store.ErrConflict, store.ErrMigration, store.ErrNotFound, store.ErrUnavailable, store.ErrExpired} {
		if errors.Is(err, e) {
			return e
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505", "40001", "40P01":
			return store.ErrConflict
		case "23503", "23502", "23514", "22003", "55000":
			return store.ErrInvalid
		}
	}
	// Never expose driver text, which may contain connection strings or SQL values.
	return store.ErrUnavailable
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", store.ErrUnavailable
	}
	return hex.EncodeToString(b[:]), nil
}
func (d *DB) transaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return d.transactionOptions(ctx, pgx.TxOptions{}, fn)
}
func (d *DB) transactionOptions(ctx context.Context, options pgx.TxOptions, fn func(context.Context, pgx.Tx) error) error {
	return d.transactionDuration(ctx, options, d.timeout, fn)
}
func (d *DB) transactionDuration(ctx context.Context, options pgx.TxOptions, timeout time.Duration, fn func(context.Context, pgx.Tx) error) error {
	if d.closed.Load() {
		return store.ErrUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := d.pool.BeginTx(ctx, options)
	if err != nil {
		return safe(err)
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer c()
		_ = tx.Rollback(cleanup)
	}()
	// PostgreSQL itself also bounds locks and statements, not merely the client wait.
	if _, err = tx.Exec(ctx, "SELECT set_config('statement_timeout',$1,true),set_config('lock_timeout',$1,true)", strconv.FormatInt(timeout.Milliseconds(), 10)); err != nil {
		return safe(err)
	}
	if err = fn(ctx, tx); err != nil {
		return safe(err)
	}
	return safe(tx.Commit(ctx))
}
func checkScope(s store.Scope) error {
	if !s.Valid() {
		return store.ErrScope
	}
	return nil
}

// Policy reads the current operational revision in the exact tenant partition.
func (d *DB) Policy(ctx context.Context, s store.Scope) (out store.Policy, err error) {
	if err = checkScope(s); err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT r.revision,r.audit_days,r.operation_hours FROM chartworks.policies p JOIN chartworks.policy_revisions r ON (r.tenant_id,r.revision)=(p.tenant_id,p.current_revision) WHERE p.tenant_id=$1`, s.Tenant()).Scan(&out.Revision, &out.AuditDays, &out.OperationHours)
	})
	return out, err
}

// SetPolicy commits an immutable revision, CAS pointer, and audit event atomically.
func (d *DB) SetPolicy(ctx context.Context, s store.Scope, expected int64, p store.Policy) (out store.Policy, err error) {
	if err = checkScope(s); err != nil {
		return out, err
	}
	if p.Validate() != nil || expected < 0 || expected >= 1<<62 {
		return out, store.ErrInvalid
	}
	eventID, e := newID()
	if e != nil {
		return out, e
	}
	out = store.Policy{Revision: expected + 1, AuditDays: p.AuditDays, OperationHours: p.OperationHours}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if expected > 0 {
			tag, e := tx.Exec(ctx, `UPDATE chartworks.policies SET current_revision=$3 WHERE tenant_id=$1 AND current_revision=$2`, s.Tenant(), expected, out.Revision)
			if e != nil {
				return e
			}
			if tag.RowsAffected() != 1 {
				return store.ErrConflict
			}
		}
		if _, e := tx.Exec(ctx, `INSERT INTO chartworks.policy_revisions(tenant_id,revision,audit_days,operation_hours,created_by) VALUES($1,$2,$3,$4,$5)`, s.Tenant(), out.Revision, p.AuditDays, p.OperationHours, s.Actor()); e != nil {
			return e
		}
		if expected == 0 {
			if _, e := tx.Exec(ctx, `INSERT INTO chartworks.policies(tenant_id,current_revision) VALUES($1,$2)`, s.Tenant(), out.Revision); e != nil {
				return e
			}
		}
		_, e := tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'retention_policy.updated','retention')`, s.Tenant(), eventID, s.Actor())
		return e
	})
	if err != nil {
		return store.Policy{}, err
	}
	return out, nil
}

// Audits returns at most limit content-free records and never falls back to an unscoped read.
func (d *DB) Audits(ctx context.Context, s store.Scope, limit int) (out []store.Audit, err error) {
	if err = checkScope(s); err != nil {
		return nil, err
	}
	if limit < 1 || limit > 1000 {
		return nil, store.ErrInvalid
	}
	out = []store.Audit{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, e := tx.Query(ctx, `SELECT event_id,actor_id,action,resource_id,created_at FROM chartworks.audit_events WHERE tenant_id=$1 ORDER BY created_at,event_id LIMIT $2`, s.Tenant(), limit)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			var a store.Audit
			if e = rows.Scan(&a.ID, &a.Actor, &a.Action, &a.Resource, &a.CreatedAt); e != nil {
				return e
			}
			out = append(out, a)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
