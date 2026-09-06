package sources

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type readTransaction interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type tableEvidence struct {
	OID     int64
	Owner   int64
	ACL     string
	Schema  string
	Name    string
	Columns []columnEvidence
}
type columnEvidence struct {
	Name     string
	OID      int64
	Modifier int32
	Native   string
	Base     string
	Nullable bool
	Safe     bool
}
type contextEvidence struct {
	Location string
	User     string
	RoleOID  int64
	Version  string
	Tables   []tableEvidence
}

func (s *Service) pool(ctx context.Context, c config.SourceConnection) (*pgxpool.Pool, string, error) {
	// Alias resolution happens before lookup. Untrusted source bodies never select
	// arbitrary environment keys, hosts, files or another tenant's connections.
	dsn, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok || len(dsn) > 16384 {
		return nil, "", store.ErrUnavailable
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.User == nil || u.User.Username() == "" || u.Hostname() == "" || u.Path == "" || u.Path == "/" || u.Fragment != "" {
		return nil, "", store.ErrInvalid
	}
	password, has := u.User.Password()
	if !has || password == "" || len(password) > 8192 {
		return nil, "", store.ErrInvalid
	}
	for key, values := range u.Query() {
		if len(values) != 1 {
			return nil, "", store.ErrInvalid
		}
		switch key {
		case "sslmode", "sslrootcert", "sslcert", "sslkey":
		default:
			return nil, "", store.ErrInvalid
		}
	}
	mode := u.Query().Get("sslmode")
	ip := net.ParseIP(u.Hostname())
	loopback := u.Hostname() == "localhost" || ip != nil && ip.IsLoopback()
	if mode != "verify-full" && (!loopback || mode != "disable") {
		return nil, "", store.ErrInvalid
	}
	key := c.Tenant + "/" + c.ID
	material := readexec.Hash([]any{dsn, c.Version}) // ephemeral; never persisted or logged
	s.mu.Lock()
	if entry, ok := s.pools[key]; ok && entry.material == material {
		s.mu.Unlock()
		return entry.pool, u.Host + u.EscapedPath(), nil
	}
	if s.retiring[key] {
		s.mu.Unlock()
		return nil, "", readexec.ErrBinding
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		s.mu.Unlock()
		return nil, "", store.ErrInvalid
	}
	cfg.MaxConns = s.settings.MaxConns
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 30 * time.Second
	cfg.MaxConnLifetime = 15 * time.Minute
	cfg.ConnConfig.ConnectTimeout = time.Duration(s.settings.ConnectTimeout)
	cfg.ConnConfig.Fallbacks = nil
	cfg.ConnConfig.RuntimeParams = map[string]string{"search_path": "pg_catalog", "application_name": "chartworks-source-reader", "default_transaction_read_only": "on", "row_security": "on", "statement_timeout": strconv.FormatInt(time.Duration(s.settings.QueryTimeout).Milliseconds(), 10), "lock_timeout": strconv.FormatInt(time.Duration(s.settings.QueryTimeout).Milliseconds(), 10)}
	cfg.ConnConfig.BuildContextWatcherHandler = func(conn *pgconn.PgConn) ctxwatch.Handler {
		return &pgconn.CancelRequestContextWatcherHandler{Conn: conn, CancelRequestDelay: 0, DeadlineDelay: 2 * time.Second}
	}
	cfg.ConnConfig.OnNotice = nil
	cfg.ConnConfig.OnNotification = nil
	// Bound backend protocol message allocation before pgx receives large row values.
	maximum := (16 << 20) + 32768
	cfg.ConnConfig.BuildFrontend = func(reader io.Reader, writer io.Writer) *pgproto3.Frontend {
		frontend := pgproto3.NewFrontend(reader, writer)
		frontend.SetMaxBodyLen(maximum)
		return frontend
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		s.mu.Unlock()
		return nil, "", safe(err)
	}
	old := s.pools[key]
	s.pools[key] = poolEntry{material: material, pool: pool}
	if old.pool != nil {
		s.retiring[key] = true
		s.reap.Add(1)
		go func() {
			defer s.reap.Done()
			old.pool.Close()
			s.mu.Lock()
			delete(s.retiring, key)
			s.mu.Unlock()
		}()
	}
	s.mu.Unlock()
	return pool, u.Host + u.EscapedPath(), nil
}

// probe locks registered base tables before inspecting their effective role and
// schema. READ COMMITTED sees the post-lock catalog state; ACCESS SHARE prevents
// concurrent DDL from changing those objects through EXPLAIN/read completion.
func (s *Service) probe(ctx context.Context, c config.SourceConnection, id string, revision int64, consume func(context.Context, readTransaction, readexec.Binding) error) (out readexec.Binding, err error) {
	pool, location, err := s.pool(ctx, c)
	if err != nil {
		return out, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(s.settings.QueryTimeout))
	defer cancel()
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted, AccessMode: pgx.ReadOnly})
	if err != nil {
		return out, safe(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	out, err = s.inspectReadContext(ctx, tx, c, id, revision, location)
	if err != nil {
		return readexec.Binding{}, err
	}
	if consume != nil {
		if err = consume(ctx, tx, out.Clone()); err != nil {
			return readexec.Binding{}, safe(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return readexec.Binding{}, safe(err)
	}
	return out, nil
}

func discoverRelation(ctx context.Context, tx readTransaction, relation config.SourceRelation) (out tableEvidence, columns []readexec.Column, err error) {
	out.Schema, out.Name = relation.Schema, relation.Name
	var kind, method, persistence string
	var rls, forceRLS, partition, children, selectable, writable, columnWritable bool
	err = tx.QueryRow(ctx, `SELECT c.oid::bigint,c.relowner::bigint,COALESCE(c.relacl::text,''),c.relkind::text,COALESCE(am.amname,''),c.relpersistence::text,c.relrowsecurity,c.relforcerowsecurity,c.relispartition,c.relhassubclass,has_table_privilege(c.oid,'SELECT'),has_table_privilege(c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER'),has_any_column_privilege(c.oid,'INSERT,UPDATE,REFERENCES') FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace LEFT JOIN pg_catalog.pg_am am ON am.oid=c.relam WHERE n.nspname=$1 AND c.relname=$2`, relation.Schema, relation.Name).Scan(&out.OID, &out.Owner, &out.ACL, &kind, &method, &persistence, &rls, &forceRLS, &partition, &children, &selectable, &writable, &columnWritable)
	if err != nil {
		return out, nil, safe(err)
	}
	// Views, policies, foreign/custom table access methods and inheritance may hide
	// executable dependencies. They are not advertised as proven by a simple catalog lookup.
	if kind != "r" || method != "heap" || persistence == "t" || rls || forceRLS || partition || children {
		return out, nil, readexec.ErrUnsupported
	}
	if !selectable || writable || columnWritable {
		return out, nil, readexec.ErrUnsafe
	}
	rows, err := tx.Query(ctx, `SELECT a.attname,t.typname,n.nspname,CASE WHEN n.nspname='pg_catalog' THEN format_type(a.atttypid,a.atttypmod) ELSE n.nspname||'.'||t.typname END,a.atttypid::bigint,a.atttypmod,NOT a.attnotnull,t.typtype::text,COALESCE(bt.typname,''),COALESCE(bn.nspname,'') FROM pg_catalog.pg_attribute a JOIN pg_catalog.pg_type t ON t.oid=a.atttypid JOIN pg_catalog.pg_namespace n ON n.oid=t.typnamespace LEFT JOIN pg_catalog.pg_type bt ON bt.oid=t.typbasetype LEFT JOIN pg_catalog.pg_namespace bn ON bn.oid=bt.typnamespace WHERE a.attrelid=$1 AND a.attnum>0 AND NOT a.attisdropped AND a.attname=ANY($2::text[]) ORDER BY a.attnum LIMIT 257`, out.OID, relation.Columns)
	if err != nil {
		return out, nil, safe(err)
	}
	defer rows.Close()
	for rows.Next() {
		var proof columnEvidence
		var native, namespace, kind, base, baseNamespace string
		if err = rows.Scan(&proof.Name, &native, &namespace, &proof.Native, &proof.OID, &proof.Modifier, &proof.Nullable, &kind, &base, &baseNamespace); err != nil {
			return out, nil, safe(err)
		}
		proof.Base = baseNamespace + "." + base
		proof.Safe = namespace == "pg_catalog" && kind == "b" && safeOID(proof.OID)
		category := Classify(native)
		if kind == "d" && baseNamespace == "pg_catalog" {
			category = Classify(base)
		}
		columns = append(columns, readexec.Column{Name: proof.Name, NativeType: proof.Native, Category: category, Nullable: proof.Nullable, Safe: proof.Safe})
		out.Columns = append(out.Columns, proof)
	}
	if err = rows.Err(); err != nil {
		return out, nil, safe(err)
	}
	if len(columns) != len(relation.Columns) {
		return out, nil, readexec.ErrBinding
	}
	return out, columns, nil
}
func safeOID(oid int64) bool {
	switch oid {
	case 16, 17, 20, 21, 23, 25, 114, 700, 701, 790, 1042, 1043, 1082, 1083, 1114, 1184, 1186, 1266, 1700, 2950, 3802:
		return true
	}
	return false
}

// Classify is a lossless category label, not authorization to execute a custom type.
func Classify(native string) string {
	switch native {
	case "int2", "int4", "int8", "numeric", "decimal", "float4", "float8", "money", "smallint", "integer", "bigint", "real", "double precision":
		return "numeric"
	case "date", "timestamp", "timestamptz", "time", "timetz", "interval":
		return "temporal"
	case "text", "varchar", "bpchar", "char", "name", "uuid":
		return "text"
	case "bool", "boolean":
		return "boolean"
	case "json", "jsonb", "xml":
		return "structured"
	case "bytea":
		return "binary"
	}
	return "unknown"
}
func arguments(parameters []readexec.Parameter) ([]any, error) {
	out := make([]any, 0, len(parameters))
	for _, p := range parameters {
		if !p.Valid() {
			return nil, readexec.ErrBinding
		}
		switch p.Kind {
		case "null":
			out = append(out, nil)
		case "text":
			out = append(out, p.Value)
		case "boolean":
			out = append(out, p.Value == "true")
		case "integer":
			n, _ := strconv.ParseInt(p.Value, 10, 64)
			out = append(out, n)
		case "number":
			var n pgtype.Numeric
			if err := n.Scan(p.Value); err != nil {
				return nil, readexec.ErrBinding
			}
			out = append(out, n)
		}
	}
	return out, nil
}
func explain(ctx context.Context, tx readTransaction, statement string, parameters []readexec.Parameter) error {
	args, err := arguments(parameters)
	if err != nil {
		return err
	}
	var plan json.RawMessage
	// The candidate has already passed the positive grammar and dependency checks.
	if err = tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+statement, args...).Scan(&plan); err != nil {
		return safe(err)
	}
	if !json.Valid(plan) || len(plan) > 1<<20 {
		return readexec.ErrUnsafe
	}
	return nil
}

// supportedPostgresVersion pins the independently tested source safety contract.
func supportedPostgresVersion(version int) bool { return version >= 170000 && version < 180000 }

// inspectReadContext is shared by native planning and the bounded read cursor.
func (s *Service) inspectReadContext(ctx context.Context, tx readTransaction, c config.SourceConnection, id string, revision int64, location string) (out readexec.Binding, err error) {
	// This catalog/type/exposure proof is qualified against PostgreSQL 17.
	// Reject unknown majors before interpreting catalogs or locking targets.
	var serverVersion int
	if err = tx.QueryRow(ctx, `SELECT current_setting('server_version_num')::integer`).Scan(&serverVersion); err != nil {
		return out, safe(err)
	}
	if !supportedPostgresVersion(serverVersion) {
		return out, readexec.ErrUnsupported
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('search_path','pg_catalog',true),set_config('row_security','on',true),set_config('statement_timeout',$1,true),set_config('lock_timeout',$1,true)`, strconv.FormatInt(time.Duration(s.settings.QueryTimeout).Milliseconds(), 10)); err != nil {
		return out, safe(err)
	}
	relations := append([]config.SourceRelation(nil), c.Relations...)
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].Schema+"."+relations[i].Name < relations[j].Schema+"."+relations[j].Name
	})
	for _, relation := range relations {
		if _, err = tx.Exec(ctx, "LOCK TABLE ONLY "+pgx.Identifier{relation.Schema, relation.Name}.Sanitize()+" IN ACCESS SHARE MODE"); err != nil {
			return out, safe(err)
		}
	}
	evidence := contextEvidence{Location: location, Version: c.Version}
	var session, readonly string
	var super, bypass, createRole, createDB bool
	if err = tx.QueryRow(ctx, `SELECT current_user,session_user,r.oid::bigint,r.rolsuper,r.rolbypassrls,r.rolcreaterole,r.rolcreatedb,current_setting('transaction_read_only') FROM pg_catalog.pg_roles r WHERE r.rolname=current_user`).Scan(&evidence.User, &session, &evidence.RoleOID, &super, &bypass, &createRole, &createDB, &readonly); err != nil {
		return out, safe(err)
	}
	if evidence.User != session || super || bypass || createRole || createDB || readonly != "on" {
		return out, readexec.ErrUnsafe
	}
	out = readexec.Binding{Tenant: c.Tenant, Source: id, Context: contextID(id, revision), Revision: revision, Dialect: "postgres"}
	for _, relation := range relations {
		proof, columns, e := discoverRelation(ctx, tx, relation)
		if e != nil {
			return readexec.Binding{}, e
		}
		evidence.Tables = append(evidence.Tables, proof)
		out.Relations = append(out.Relations, readexec.Relation{ID: "ds:" + readexec.Hash([]string{id, relation.Schema, relation.Name})[:32], Schema: relation.Schema, Name: relation.Name, Columns: columns})
	}
	out.Contract = "source-contract:" + readexec.Hash([]any{c.Version, out.Relations})[:32]
	out.Fingerprint = readexec.Hash(evidence)
	if !out.Valid() {
		return readexec.Binding{}, readexec.ErrBinding
	}

	return out, nil
}
