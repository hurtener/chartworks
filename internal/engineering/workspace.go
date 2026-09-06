package engineering

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgconn/ctxwatch"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Service) writerConfig(c config.SourceConnection) (*pgx.ConnConfig, *pgx.ConnConfig, error) {
	if !strings.HasPrefix(c.ReadDSN, "env:") || !strings.HasPrefix(c.WriteDSN, "env:") {
		return nil, nil, ErrOwnership
	}
	read, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok {
		return nil, nil, ErrUnavailable
	}
	write, ok := s.lookup(strings.TrimPrefix(c.WriteDSN, "env:"))
	if !ok {
		return nil, nil, ErrUnavailable
	}
	r, _, err := sources.ParseApprovedDSN(read)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	w, _, err := sources.ParseApprovedDSN(write)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	// Deliberately conservative: even different hosts cannot reuse the metadata
	// database name. Customer work must be explicitly deployed separately.
	if r.Host != w.Host || r.Port != w.Port || r.Database != w.Database || r.User == w.User || w.Database == s.repo.DatabaseName() {
		return nil, nil, ErrOwnership
	}
	w.ConnectTimeout = time.Duration(s.values.Sources.ConnectTimeout)
	w.Fallbacks = nil
	w.BuildContextWatcherHandler = func(conn *pgconn.PgConn) ctxwatch.Handler {
		return &pgconn.CancelRequestContextWatcherHandler{Conn: conn, CancelRequestDelay: 0, DeadlineDelay: 2 * time.Second}
	}
	w.BuildFrontend = func(reader io.Reader, writer io.Writer) *pgproto3.Frontend {
		frontend := pgproto3.NewFrontend(reader, writer)
		frontend.SetMaxBodyLen(int(s.values.Uploads.MaxBytes) + (1 << 20))
		return frontend
	}
	w.OnNotice = nil
	w.OnNotification = nil
	w.RuntimeParams["search_path"] = "pg_catalog"
	w.RuntimeParams["application_name"] = "chartworks-managed-writer"
	return w, r, nil
}

func (s *Service) workspaceTx(ctx context.Context, e identity.Envelope, c config.SourceConnection, r UploadRecord, fn func(context.Context, pgx.Tx, string, string, string) error) (err error) {
	if !e.Valid() || !r.Valid() || r.Tenant != e.Tenant() || r.Actor != e.User() || r.Session != e.Session() {
		return ErrOwnership
	}
	schema, table, err := sources.ManagedLocation(c, r.Spec.ID)
	if err != nil {
		return ErrOwnership
	}
	w, read, err := s.writerConfig(c)
	if err != nil {
		return err
	}
	conn, err := pgx.ConnectConfig(ctx, w)
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		defer stop()
		_ = conn.Close(cleanup)
	}()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadWrite})
	if err != nil {
		return ErrUnavailable
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	deadline, ok := ctx.Deadline()
	if !ok {
		return ErrInvalid
	}
	remaining := time.Until(deadline)
	if remaining < time.Millisecond {
		return context.DeadlineExceeded
	}
	if _, err = tx.Exec(ctx, `SELECT set_config('statement_timeout',$1,true),set_config('transaction_timeout',$1,true),set_config('idle_in_transaction_session_timeout',$1,true),set_config('lock_timeout',$1,true)`, strconv.FormatInt(remaining.Milliseconds(), 10)); err != nil {
		return ErrUnavailable
	}
	var database, user, session, readonly string
	var roleOID int64
	var version int
	var super, bypass, createRole, createDB bool
	if err = tx.QueryRow(ctx, `SELECT current_database(),current_user,session_user,r.oid::bigint,r.rolsuper,r.rolbypassrls,r.rolcreaterole,r.rolcreatedb,current_setting('transaction_read_only'),current_setting('server_version_num')::integer FROM pg_catalog.pg_roles r WHERE r.rolname=current_user`).Scan(&database, &user, &session, &roleOID, &super, &bypass, &createRole, &createDB, &readonly, &version); err != nil {
		return ErrUnavailable
	}
	if database != w.Database || database == s.repo.DatabaseName() || user != session || user != w.User || super || bypass || createRole || createDB || readonly != "off" || version < 170000 || version >= 180000 {
		return ErrOwnership
	}
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061111))`, c.Tenant+":"+c.ID); err != nil {
		return workspaceFailure(ctx, err)
	}
	if err = ensureWorkspace(ctx, tx, c, schema, roleOID, user, read.User); err != nil {
		return workspaceFailure(ctx, err)
	}
	if err = fn(ctx, tx, schema, table, read.User); err != nil {
		return workspaceFailure(ctx, err)
	}
	if !e.Valid() {
		return ErrOwnership
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	// A lost Commit reply is uncertain. The next explicit attempt observes the
	// same registry transaction under the same advisory lock before any mutation.
	if err = tx.Commit(ctx); err != nil {
		return ErrState
	}
	return nil
}

func ensureWorkspace(ctx context.Context, tx pgx.Tx, c config.SourceConnection, schema string, roleOID int64, writer, reader string) error {
	var oid, owner int64
	err := tx.QueryRow(ctx, `SELECT oid::bigint,nspowner::bigint FROM pg_catalog.pg_namespace WHERE nspname=$1`, schema).Scan(&oid, &owner)
	name := pgx.Identifier{schema}.Sanitize()
	marker := pgx.Identifier{schema, "_chartworks_workspace"}.Sanitize()
	registry := pgx.Identifier{schema, "_uploads"}.Sanitize()
	if errors.Is(err, pgx.ErrNoRows) {
		if _, err = tx.Exec(ctx, "CREATE SCHEMA "+name+" AUTHORIZATION CURRENT_USER"); err != nil {
			return err
		}
		if err = tx.QueryRow(ctx, `SELECT oid::bigint,nspowner::bigint FROM pg_catalog.pg_namespace WHERE nspname=$1`, schema).Scan(&oid, &owner); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "REVOKE ALL ON SCHEMA "+name+" FROM PUBLIC"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "CREATE TABLE "+marker+" (singleton boolean PRIMARY KEY CHECK(singleton),tenant_id text NOT NULL,connection_id text NOT NULL,schema_oid bigint NOT NULL,writer_role text NOT NULL,reader_role text NOT NULL)"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "CREATE TABLE "+registry+" (upload_id text PRIMARY KEY,spec_hash text NOT NULL,checksum text NOT NULL,table_name text NOT NULL,table_oid bigint,state text NOT NULL CHECK(state IN('staged','loaded','erased')),raw bytea,rows_loaded bigint NOT NULL DEFAULT 0,decoded_bytes bigint NOT NULL DEFAULT 0,created_at timestamptz NOT NULL DEFAULT clock_timestamp(),CHECK((state='staged')=(raw IS NOT NULL)),CHECK(state<>'loaded' OR table_oid IS NOT NULL))"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO "+marker+" VALUES(true,$1,$2,$3,$4,$5)", c.Tenant, c.ID, oid, writer, reader); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "REVOKE ALL ON TABLE "+marker+","+registry+" FROM PUBLIC"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "GRANT USAGE ON SCHEMA "+name+" TO "+pgx.Identifier{reader}.Sanitize()); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if owner != roleOID {
		return ErrOwnership
	}
	for _, table := range []string{"_chartworks_workspace", "_uploads"} {
		if _, err = ownedTable(ctx, tx, schema, table, 0); err != nil {
			return ErrOwnership
		}
	}
	var tenant, alias, w, r string
	var recordedOID int64
	if err = tx.QueryRow(ctx, "SELECT tenant_id,connection_id,schema_oid,writer_role,reader_role FROM "+marker+" WHERE singleton").Scan(&tenant, &alias, &recordedOID, &w, &r); err != nil {
		return ErrOwnership
	}
	if tenant != c.Tenant || alias != c.ID || recordedOID != oid || w != writer || r != reader {
		return ErrOwnership
	}
	return nil
}

// ownedTable proves the exact object both before and after acquiring its DDL
// lock. The workspace advisory lock alone cannot fence non-Chartworks DDL.
// Hold the table lock through the caller's mutation and transaction completion.
func ownedTable(ctx context.Context, tx pgx.Tx, schema, table string, expected int64) (int64, error) {
	var oid int64
	for pass := 0; pass < 2; pass++ {
		var kind string
		var owned, unsafe bool
		err := tx.QueryRow(ctx, `SELECT c.oid::bigint,c.relkind::text,c.relowner=(SELECT oid FROM pg_catalog.pg_roles WHERE rolname=current_user),c.relrowsecurity OR c.relforcerowsecurity OR c.relispartition OR EXISTS(SELECT 1 FROM pg_catalog.pg_inherits WHERE inhrelid=c.oid OR inhparent=c.oid) OR EXISTS(SELECT 1 FROM pg_catalog.pg_trigger WHERE tgrelid=c.oid AND NOT tgisinternal) OR EXISTS(SELECT 1 FROM pg_catalog.pg_rewrite WHERE ev_class=c.oid) FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, schema, table).Scan(&oid, &kind, &owned, &unsafe)
		if err != nil {
			return 0, err
		}
		if kind != "r" || !owned || unsafe || expected != 0 && expected != oid {
			return 0, ErrOwnership
		}
		if pass == 0 {
			expected = oid
			if _, err = tx.Exec(ctx, "LOCK TABLE "+pgx.Identifier{schema, table}.Sanitize()+" IN ACCESS EXCLUSIVE MODE"); err != nil {
				return 0, err
			}
		}
	}
	return oid, nil
}

type workspaceEntry struct {
	spec, checksum, table, state string
	oid                          *int64
	rows, bytes                  int64
}

func workspaceRecord(ctx context.Context, tx pgx.Tx, schema, id string) (out workspaceEntry, err error) {
	err = tx.QueryRow(ctx, "SELECT spec_hash,checksum,table_name,state,table_oid,rows_loaded,decoded_bytes FROM "+pgx.Identifier{schema, "_uploads"}.Sanitize()+" WHERE upload_id=$1 FOR UPDATE", id).Scan(&out.spec, &out.checksum, &out.table, &out.state, &out.oid, &out.rows, &out.bytes)
	return out, err
}
func matchWorkspace(entry workspaceEntry, r UploadRecord, table string) error {
	if entry.spec != r.SpecHash || entry.checksum != r.Spec.SHA256 || entry.table != table {
		return ErrOwnership
	}
	return nil
}
func (s *Service) stageWorkspace(ctx context.Context, e identity.Envelope, c config.SourceConnection, r UploadRecord, raw []byte) error {
	if err := r.Require(e, "sources.upload", "write"); err != nil {
		return err
	}
	return s.workspaceTx(ctx, e, c, r, func(ctx context.Context, tx pgx.Tx, schema, table, _ string) error {
		entry, err := workspaceRecord(ctx, tx, schema, r.Spec.ID)
		if err == nil {
			if err = matchWorkspace(entry, r, table); err != nil {
				return err
			}
			if entry.state == "erased" {
				return ErrState
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		registry := pgx.Identifier{schema, "_uploads"}.Sanitize()
		var count int
		var used int64
		if err = tx.QueryRow(ctx, "SELECT count(*),COALESCE(sum(COALESCE(octet_length(raw),0)+decoded_bytes),0)::bigint FROM "+registry+" WHERE state<>'erased'").Scan(&count, &used); err != nil {
			return err
		}
		if count >= s.values.Uploads.MaxPerTenant || used+int64(len(raw)) > s.values.Uploads.MaxTenantBytes {
			return ErrLimit
		}
		_, err = tx.Exec(ctx, "INSERT INTO "+registry+"(upload_id,spec_hash,checksum,table_name,state,raw) VALUES($1,$2,$3,$4,'staged',$5)", r.Spec.ID, r.SpecHash, r.Spec.SHA256, table, raw)
		return err
	})
}
func (s *Service) loadWorkspace(ctx context.Context, e identity.Envelope, c config.SourceConnection, r UploadRecord) (out WorkspaceReceipt, err error) {
	if err = r.Require(e, "sources.upload", "write"); err != nil {
		return out, err
	}
	err = s.workspaceTx(ctx, e, c, r, func(ctx context.Context, tx pgx.Tx, schema, table, reader string) error {
		entry, err := workspaceRecord(ctx, tx, schema, r.Spec.ID)
		if err != nil {
			return err
		}
		if err = matchWorkspace(entry, r, table); err != nil {
			return err
		}
		if entry.state == "erased" {
			return ErrState
		}
		if entry.state == "loaded" {
			if entry.oid == nil {
				return ErrOwnership
			}
			if _, err = ownedTable(ctx, tx, schema, table, *entry.oid); err != nil {
				return err
			}
			if entry.rows < 0 || entry.rows > int64(s.values.Uploads.MaxRows) || entry.bytes < 0 || entry.bytes > s.values.Uploads.MaxExpandedBytes {
				return ErrOwnership
			}
			out = WorkspaceReceipt{SpecHash: r.SpecHash, Checksum: r.Spec.SHA256, Schema: schema, Table: table, TableOID: *entry.oid, Rows: int(entry.rows), DecodedBytes: entry.bytes}
			return nil
		}
		if entry.state != "staged" {
			return ErrState
		}
		// Existing arbitrary same-name objects are not our partial staging table.
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON c.relnamespace=n.oid WHERE n.nspname=$1 AND c.relname=$2)`, schema, table).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return ErrOwnership
		}
		registry := pgx.Identifier{schema, "_uploads"}.Sanitize()
		var raw []byte
		if err = tx.QueryRow(ctx, "SELECT raw FROM "+registry+" WHERE upload_id=$1 AND octet_length(raw)=$2", r.Spec.ID, r.Spec.Bytes).Scan(&raw); err != nil {
			return ErrChecksum
		}
		columns := make([]string, len(r.Spec.Columns))
		definitions := make([]string, len(columns))
		for i, col := range r.Spec.Columns {
			columns[i] = col.Name
			definitions[i] = pgx.Identifier{col.Name}.Sanitize() + " " + sqlType(col.Type)
			if !col.Nullable {
				definitions[i] += " NOT NULL"
			}
		}
		qualified := pgx.Identifier{schema, table}.Sanitize()
		if _, err = tx.Exec(ctx, "CREATE TABLE "+qualified+" ("+strings.Join(definitions, ",")+")"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "REVOKE ALL ON TABLE "+qualified+" FROM PUBLIC"); err != nil {
			return err
		}
		receipt, err := s.copyUpload(ctx, tx, raw, r.Spec, pgx.Identifier{schema, table}, columns)
		if err != nil {
			return err
		}
		var used int64
		if err = tx.QueryRow(ctx, "SELECT COALESCE(sum(COALESCE(octet_length(raw),0)+decoded_bytes),0)::bigint FROM "+registry+" WHERE state<>'erased' AND upload_id<>$1", r.Spec.ID).Scan(&used); err != nil {
			return err
		}
		if used+receipt.DecodedBytes > s.values.Uploads.MaxTenantBytes {
			return ErrLimit
		}
		oid, err := ownedTable(ctx, tx, schema, table, 0)
		if err != nil {
			return err
		}
		// The ordinary source adapter requires table-level SELECT and rejects all
		// write privileges. Grant it only on this newly created declared dataset;
		// do not weaken source qualification or grant workspace-wide access.
		if _, err = tx.Exec(ctx, "GRANT SELECT ON "+qualified+" TO "+pgx.Identifier{reader}.Sanitize()); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE "+registry+" SET state='loaded',raw=NULL,table_oid=$2,rows_loaded=$3,decoded_bytes=$4 WHERE upload_id=$1", r.Spec.ID, oid, receipt.Rows, receipt.DecodedBytes); err != nil {
			return err
		}
		out = WorkspaceReceipt{SpecHash: r.SpecHash, Checksum: r.Spec.SHA256, Schema: schema, Table: table, TableOID: oid, Rows: receipt.Rows, DecodedBytes: receipt.DecodedBytes}
		return nil
	})
	if err != nil {
		return WorkspaceReceipt{}, err
	}
	return out, nil
}
func (s *Service) copyUpload(ctx context.Context, tx pgx.Tx, raw []byte, spec UploadSpec, target pgx.Identifier, columns []string) (ParseReceipt, error) {
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	type parsed struct {
		receipt ParseReceipt
		err     error
	}
	rows := make(chan []Cell, 1)
	done := make(chan parsed, 1)
	go func() {
		receipt, err := Parse(child, raw, spec, s.values.Uploads, func(row []Cell) error {
			select {
			case rows <- row:
				return nil
			case <-child.Done():
				return child.Err()
			}
		})
		close(rows)
		done <- parsed{receipt, err}
	}()
	var parseErr error
	copied, copyErr := tx.CopyFrom(child, target, columns, pgx.CopyFromFunc(func() ([]any, error) {
		select {
		case <-child.Done():
			return nil, child.Err()
		case row, ok := <-rows:
			if !ok {
				return nil, nil
			}
			values := make([]any, len(row))
			for i, cell := range row {
				v, err := copyCell(spec.Columns[i], cell)
				if err != nil {
					parseErr = err
					return nil, err
				}
				values[i] = v
			}
			return values, nil
		}
	}))
	cancel()
	result := <-done
	if result.err != nil {
		return ParseReceipt{}, result.err
	}
	if parseErr != nil {
		return ParseReceipt{}, parseErr
	}
	if copyErr != nil {
		return ParseReceipt{}, copyErr
	}
	if copied != int64(result.receipt.Rows) {
		return ParseReceipt{}, ErrState
	}
	return result.receipt, nil
}
func copyCell(column UploadColumn, cell Cell) (any, error) {
	if cell.Null {
		return nil, nil
	}
	switch column.Type {
	case "integer":
		return strconv.ParseInt(cell.Text, 10, 64)
	case "decimal":
		var n pgtype.Numeric
		if err := n.Scan(cell.Text); err != nil {
			return nil, ErrFormat
		}
		return n, nil
	case "number":
		return strconv.ParseFloat(cell.Text, 64)
	case "boolean":
		return cell.Text == "true", nil
	case "date":
		return time.Parse("2006-01-02", cell.Text)
	case "timestamp":
		return time.Parse(time.RFC3339Nano, cell.Text)
	case "binary":
		return hex.DecodeString(cell.Text)
	case "json":
		return json.RawMessage(cell.Text), nil
	case "text":
		return cell.Text, nil
	}
	return nil, ErrFormat
}
func (s *Service) eraseWorkspace(ctx context.Context, e identity.Envelope, c config.SourceConnection, r UploadRecord) error {
	if err := r.Require(e, "sources.erase", "erase"); err != nil {
		return err
	}
	return s.workspaceTx(ctx, e, c, r, func(ctx context.Context, tx pgx.Tx, schema, table, _ string) error {
		entry, err := workspaceRecord(ctx, tx, schema, r.Spec.ID)
		registry := pgx.Identifier{schema, "_uploads"}.Sanitize()
		if errors.Is(err, pgx.ErrNoRows) {
			// A tombstone also fences a delayed Stage request from recreating bytes.
			_, err = tx.Exec(ctx, "INSERT INTO "+registry+"(upload_id,spec_hash,checksum,table_name,state) VALUES($1,$2,$3,$4,'erased')", r.Spec.ID, r.SpecHash, r.Spec.SHA256, table)
			return err
		}
		if err != nil {
			return err
		}
		if err = matchWorkspace(entry, r, table); err != nil {
			return err
		}
		if entry.state == "erased" {
			return nil
		}
		if entry.state == "loaded" {
			if entry.oid == nil {
				return ErrOwnership
			}
			if _, err = ownedTable(ctx, tx, schema, table, *entry.oid); err != nil {
				return ErrOwnership
			}
			// Never CASCADE into unrelated baseline objects or silently adopt OIDs.
			if _, err = tx.Exec(ctx, "DROP TABLE "+pgx.Identifier{schema, table}.Sanitize()); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, "UPDATE "+registry+" SET state='erased',raw=NULL,table_oid=NULL,rows_loaded=0,decoded_bytes=0 WHERE upload_id=$1", r.Spec.ID)
		return err
	})
}
func workspaceFailure(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, allowed := range []error{ErrFormat, ErrChecksum, ErrLimit, ErrOwnership, ErrState, ErrInvalid, ErrUnavailable, store.ErrNotFound} {
		if errors.Is(err, allowed) {
			return allowed
		}
	}
	return ErrUnavailable
}
