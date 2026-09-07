package sources

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	bruinmssql "github.com/bruin-data/bruin/pkg/mssql"
	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/microsoft/go-mssqldb/msdsn"
)

type sqlServerPoolEntry struct {
	material string
	client   *bruinmssql.DB
}

func parseSQLServerDSN(raw string) (*bruinmssql.Config, string, error) {
	if len(raw) == 0 || len(raw) > 16384 {
		return nil, "", store.ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "sqlserver" || u.Hostname() == "" || u.User == nil || u.User.Username() == "" || u.Path != "" || u.Fragment != "" {
		return nil, "", store.ErrInvalid
	}
	password, ok := u.User.Password()
	if !ok || password == "" {
		return nil, "", store.ErrInvalid
	}
	seen := map[string]bool{}
	for key, values := range u.Query() {
		key = strings.ToLower(key)
		if len(values) != 1 || seen[key] {
			return nil, "", store.ErrInvalid
		}
		seen[key] = true
		switch key {
		case "database", "encrypt", "trustservercertificate", "certificate", "hostnameincertificate":
		default:
			return nil, "", store.ErrInvalid
		}
	}
	parsed, err := msdsn.Parse(raw)
	if err != nil || parsed.Database == "" || parsed.Instance != "" {
		return nil, "", store.ErrInvalid
	}
	loopback := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		loopback = ip.IsLoopback()
	}
	verified := parsed.TLSConfig != nil && !parsed.TLSConfig.InsecureSkipVerify && (parsed.Encryption == msdsn.EncryptionRequired || parsed.Encryption == msdsn.EncryptionStrict)
	if !verified && (!loopback || parsed.Encryption != msdsn.EncryptionDisabled) {
		return nil, "", store.ErrInvalid
	}
	if parsed.Port == 0 || parsed.Port > 65535 {
		return nil, "", store.ErrInvalid
	}
	return &bruinmssql.Config{Host: parsed.Host, Port: int(parsed.Port), Username: parsed.User, Password: parsed.Password, Database: parsed.Database, Query: u.RawQuery}, net.JoinHostPort(parsed.Host, strconv.FormatUint(parsed.Port, 10)) + "/" + parsed.Database, nil
}
func (s *Service) sqlServerClient(c config.SourceConnection) (*bruinmssql.DB, string, error) {
	raw, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok {
		return nil, "", store.ErrUnavailable
	}
	parsed, location, err := parseSQLServerDSN(raw)
	if err != nil {
		return nil, "", err
	}
	key := c.Tenant + "/" + c.ID
	material := readexec.Hash([]any{raw, c.Version})
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sqlserverPools == nil {
		s.sqlserverPools = map[string]sqlServerPoolEntry{}
	}
	if entry, ok := s.sqlserverPools[key]; ok && entry.material == material {
		return entry.client, location, nil
	}
	client, err := bruinmssql.NewDB(parsed)
	if err != nil {
		return nil, "", safe(err)
	}
	old := s.sqlserverPools[key]
	s.sqlserverPools[key] = sqlServerPoolEntry{material: material, client: client}
	if old.client != nil {
		_ = old.client.Close()
	}
	return client, location, nil
}
func (s *Service) closeSQLServer() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.sqlserverPools {
		_ = entry.client.Close()
		delete(s.sqlserverPools, key)
	}
}
func sqlServerTag(prefix string) (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", store.ErrUnavailable
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

type discardSQLServerObserver struct{}

func (discardSQLServerObserver) OnDispatch(context.Context, bruinmssql.ReadIdentity) error {
	return nil
}
func (discardSQLServerObserver) OnAcknowledged(context.Context, bruinmssql.ReadIdentity) error {
	return nil
}
func (s *Service) probeSQLServer(ctx context.Context, c config.SourceConnection, id string, revision int64) (readexec.Binding, error) {
	client, location, err := s.sqlServerClient(c)
	if err != nil {
		return readexec.Binding{}, err
	}
	tag, err := sqlServerTag("probe-")
	if err != nil {
		return readexec.Binding{}, err
	}
	var binding readexec.Binding
	verifier := func(ctx context.Context, session bruinmssql.ReadSession) error {
		binding, err = inspectSQLServerContext(ctx, session, c, id, revision, location)
		return err
	}
	rows, _, err := client.OpenReadVerified(ctx, &query.Query{Query: "SELECT 1 AS chartworks_probe"}, tag, discardSQLServerObserver{}, bruinmssql.ReadOptions{RequireTLS: !c.AllowInsecureLocal, Timeout: time.Duration(s.settings.QueryTimeout)}, verifier)
	if err != nil {
		return readexec.Binding{}, safe(err)
	}
	if err = rows.Close(); err != nil {
		return readexec.Binding{}, safe(err)
	}
	return binding, nil
}

const sqlServerIdentitySQL = `SELECT CAST(SERVERPROPERTY('ProductVersion') AS nvarchar(128)), ORIGINAL_LOGIN(), USER_NAME(), DB_NAME()`
const sqlServerTableSQL = `SELECT t.object_id, t.modify_date, t.temporal_type, t.is_memory_optimized, t.is_filetable, (SELECT COUNT(*) FROM sys.security_predicates p JOIN sys.security_policies sp ON sp.object_id=p.object_id WHERE p.target_object_id=t.object_id AND sp.is_enabled=1) FROM sys.tables t JOIN sys.schemas s ON s.schema_id=t.schema_id WHERE s.name=@p1 AND t.name=@p2 AND t.is_ms_shipped=0`
const sqlServerColumnsSQL = `SELECT c.name, ty.name, c.max_length, c.precision, c.scale, c.is_nullable, c.column_id, c.is_computed, c.generated_always_type, c.encryption_type, ty.is_user_defined, ty.is_assembly_type FROM sys.columns c JOIN sys.types ty ON ty.user_type_id=c.user_type_id WHERE c.object_id=@p1 ORDER BY c.column_id`

func inspectSQLServerContext(ctx context.Context, session bruinmssql.ReadSession, c config.SourceConnection, id string, revision int64, location string) (readexec.Binding, error) {
	identityRows, err := sqlServerRows(ctx, session, sqlServerIdentitySQL, nil, 1)
	if err != nil || len(identityRows) != 1 || len(identityRows[0]) != 4 {
		return readexec.Binding{}, readexec.ErrBinding
	}
	current := identityRows[0]
	version := sqlServerText(current[0])
	if (!strings.HasPrefix(version, "16.") && !strings.HasPrefix(version, "17.")) || sqlServerText(current[1]) == "" || sqlServerText(current[2]) == "" || sqlServerText(current[3]) == "" {
		return readexec.Binding{}, readexec.ErrUnsupported
	}
	evidence := []any{location, c.Version, current}
	for _, kind := range []string{"SERVER", "DATABASE"} {
		rows, err := sqlServerRows(ctx, session, "SELECT permission_name FROM sys.fn_my_permissions(NULL, @p1) ORDER BY permission_name", []any{kind}, 128)
		if err != nil || !sqlServerPermissions(rows, kind) {
			return readexec.Binding{}, readexec.ErrUnsafe
		}
		evidence = append(evidence, rows)
	}
	relations := append([]config.SourceRelation(nil), c.Relations...)
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].Schema+"."+relations[i].Name < relations[j].Schema+"."+relations[j].Name
	})
	out := readexec.Binding{Tenant: c.Tenant, Source: id, Context: contextID(id, revision), Revision: revision, Dialect: "sqlserver"}
	for _, relation := range relations {
		qualified := sqlServerQuote(relation.Schema) + "." + sqlServerQuote(relation.Name)
		for _, target := range []struct{ name, kind string }{{relation.Schema, "SCHEMA"}, {qualified, "OBJECT"}} {
			permissions, err := sqlServerRows(ctx, session, "SELECT permission_name FROM sys.fn_my_permissions(@p1,@p2) ORDER BY permission_name", []any{target.name, target.kind}, 256)
			if err != nil || !sqlServerPermissions(permissions, target.kind) {
				return readexec.Binding{}, readexec.ErrUnsafe
			}
			evidence = append(evidence, permissions)
		}
		table, err := sqlServerRows(ctx, session, sqlServerTableSQL, []any{relation.Schema, relation.Name}, 1)
		if err != nil || len(table) != 1 || len(table[0]) != 6 {
			return readexec.Binding{}, readexec.ErrBinding
		}
		for _, value := range table[0][2:] {
			if sqlServerText(value) != "0" && sqlServerText(value) != "false" {
				return readexec.Binding{}, readexec.ErrUnsupported
			}
		}
		native, err := sqlServerRows(ctx, session, sqlServerColumnsSQL, []any{table[0][0]}, 256)
		if err != nil {
			return readexec.Binding{}, err
		}
		columns, err := sqlServerColumns(native, relation.Columns)
		if err != nil {
			return readexec.Binding{}, err
		}
		// Hold a table lock through native planning and the user read so the accepted
		// base-table schema cannot change between metadata inspection and execution.
		if _, err = sqlServerRows(ctx, session, "SELECT TOP (1) 1 AS chartworks_lock FROM "+qualified+" WITH (TABLOCK,HOLDLOCK)", nil, 1); err != nil {
			return readexec.Binding{}, err
		}
		// Re-read the same metadata under the retained lock. A concurrent DDL
		// change before lock acquisition invalidates this attempt.
		lockedTable, lockErr := sqlServerRows(ctx, session, sqlServerTableSQL, []any{relation.Schema, relation.Name}, 1)
		if lockErr != nil || readexec.Hash(lockedTable) != readexec.Hash(table) {
			return readexec.Binding{}, readexec.ErrBinding
		}
		lockedColumns, lockErr := sqlServerRows(ctx, session, sqlServerColumnsSQL, []any{table[0][0]}, 256)
		if lockErr != nil || readexec.Hash(lockedColumns) != readexec.Hash(native) {
			return readexec.Binding{}, readexec.ErrBinding
		}
		relationID := "ds:" + readexec.Hash([]string{id, relation.Schema, relation.Name})[:32]
		out.Relations = append(out.Relations, readexec.Relation{ID: relationID, Schema: relation.Schema, Name: relation.Name, Columns: columns})
		evidence = append(evidence, []any{relation.Schema, relation.Name, table, native})
	}
	out.Contract = "source-contract:" + readexec.Hash([]any{c.Version, out.Relations})[:32]
	out.Fingerprint = readexec.Hash(evidence)
	if !out.Valid() {
		return readexec.Binding{}, readexec.ErrBinding
	}
	return out, nil
}
func sqlServerPermissions(rows [][]any, kind string) bool {
	if len(rows) == 0 {
		return false
	}
	selected := false
	for _, row := range rows {
		if len(row) != 1 {
			return false
		}
		permission := sqlServerText(row[0])
		switch permission {
		case "SELECT":
			selected = true
		case "CONNECT", "CONNECT SQL", "VIEW DEFINITION", "VIEW ANY DEFINITION", "VIEW ANY DATABASE", "VIEW DATABASE STATE", "VIEW DATABASE PERFORMANCE STATE", "VIEW SERVER STATE", "VIEW SERVER PERFORMANCE STATE", "SHOWPLAN":
		default:
			return false
		}
	}
	return kind == "SERVER" || kind == "DATABASE" || selected
}
func sqlServerQuote(name string) string { return "[" + strings.ReplaceAll(name, "]", "]]") + "]" }
func sqlServerRows(ctx context.Context, session bruinmssql.ReadSession, statement string, args []any, limit int) ([][]any, error) {
	rows, err := session.Query(ctx, &query.Query{Query: statement, Args: args})
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := [][]any{}
	size := 0
	for rows.Next() {
		if len(result) >= limit {
			return nil, readexec.ErrLimit
		}
		values, err := rows.Values()
		if err != nil {
			return nil, err
		}
		for _, v := range values {
			size += len(sqlServerText(v))
			if size > 1<<20 {
				return nil, readexec.ErrLimit
			}
		}
		result = append(result, values)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}
func sqlServerText(v any) string {
	switch value := v.(type) {
	case nil:
		return ""
	case string:
		return value
	case []byte:
		return string(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case int32:
		return strconv.FormatInt(int64(value), 10)
	case bool:
		return strconv.FormatBool(value)
	case time.Time:
		return value.Format(time.RFC3339Nano)
	}
	return ""
}
func sqlServerColumns(rows [][]any, declared []string) ([]readexec.Column, error) {
	if len(rows) != len(declared) {
		return nil, readexec.ErrBinding
	}
	out := make([]readexec.Column, len(rows))
	for i, row := range rows {
		if len(row) != 12 || sqlServerText(row[0]) != declared[i] {
			return nil, readexec.ErrBinding
		}
		base := sqlServerText(row[1])
		category := sqlServerCategory(base)
		if category == "" {
			return nil, readexec.ErrUnsupported
		}
		for _, j := range []int{7, 8, 10, 11} {
			if sqlServerText(row[j]) != "0" && sqlServerText(row[j]) != "false" {
				return nil, readexec.ErrUnsupported
			}
		}
		if row[9] != nil {
			return nil, readexec.ErrUnsupported
		}
		native := base
		switch base {
		case "decimal", "numeric":
			native += "(" + sqlServerText(row[3]) + "," + sqlServerText(row[4]) + ")"
		case "char", "varchar", "nchar", "nvarchar", "binary", "varbinary":
			native += "(" + sqlServerText(row[2]) + ")"
		case "time", "datetime2", "datetimeoffset":
			native += "(" + sqlServerText(row[4]) + ")"
		}
		nullable := sqlServerText(row[5])
		if nullable != "true" && nullable != "false" && nullable != "0" && nullable != "1" {
			return nil, readexec.ErrBinding
		}
		out[i] = readexec.Column{Name: declared[i], NativeType: native, Category: category, Nullable: nullable == "true" || nullable == "1", Safe: true}
	}
	return out, nil
}
func sqlServerCategory(name string) string {
	switch strings.ToLower(name) {
	case "tinyint", "smallint", "int", "bigint":
		return "integer"
	case "decimal", "numeric", "money", "smallmoney":
		return "decimal"
	case "real", "float":
		return "number"
	case "bit":
		return "boolean"
	case "char", "varchar", "nchar", "nvarchar", "text", "ntext":
		return "text"
	case "binary", "varbinary", "image", "timestamp", "rowversion", "uniqueidentifier":
		return "binary"
	case "date", "time", "datetime", "smalldatetime", "datetime2", "datetimeoffset":
		return "temporal"
	}
	return ""
}

func sqlServerArguments(parameters []readexec.Parameter) ([]any, error) {
	out := make([]any, len(parameters))
	for i, p := range parameters {
		switch p.Kind {
		case "null":
			out[i] = nil
		case "text":
			out[i] = p.Value
		case "integer":
			v, err := strconv.ParseInt(p.Value, 10, 64)
			if err != nil {
				return nil, readexec.ErrBinding
			}
			out[i] = v
		case "boolean":
			if p.Value != "true" && p.Value != "false" {
				return nil, readexec.ErrBinding
			}
			out[i] = p.Value == "true"
		case "number":
			return nil, readexec.ErrUnsupported
		default:
			return nil, readexec.ErrBinding
		}
	}
	return out, nil
}
func sqlServerSet(ctx context.Context, session bruinmssql.ReadSession, statement string) error {
	rows, err := session.Query(ctx, &query.Query{Query: statement})
	if err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}
	return rows.Err()
}
func sqlServerExplain(ctx context.Context, session bruinmssql.ReadSession, statement string, args []any, binding readexec.Binding) (cost float64, err error) {
	databaseRows, databaseErr := sqlServerRows(ctx, session, "SELECT DB_NAME()", nil, 1)
	if databaseErr != nil || len(databaseRows) != 1 || len(databaseRows[0]) != 1 {
		return 0, readexec.ErrBinding
	}
	if err = sqlServerSet(ctx, session, "SET SHOWPLAN_XML ON"); err != nil {
		return 0, err
	}
	defer func() {
		offErr := sqlServerSet(ctx, session, "SET SHOWPLAN_XML OFF")
		if err == nil {
			err = offErr
		}
	}()
	rows, err := sqlServerRows(ctx, session, statement, args, 1)
	if err != nil || len(rows) != 1 || len(rows[0]) != 1 {
		return 0, readexec.ErrUnsafe
	}
	return inspectSQLServerPlan([]byte(sqlServerText(rows[0][0])), binding, sqlServerText(databaseRows[0][0]))
}
func inspectSQLServerPlan(document []byte, binding readexec.Binding, database string) (float64, error) {
	if len(document) == 0 || len(document) > 1<<20 {
		return 0, readexec.ErrLimit
	}
	decoder := xml.NewDecoder(bytes.NewReader(document))
	seenRoot, seenStatement := false, false
	cost := 0.0
	nodes := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, readexec.ErrUnsafe
		}
		nodes++
		if nodes > 32768 {
			return 0, readexec.ErrLimit
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		attrs := map[string]string{}
		for _, attr := range start.Attr {
			attrs[attr.Name.Local] = attr.Value
		}
		switch start.Name.Local {
		case "ShowPlanXML":
			if seenRoot || start.Name.Space != "http://schemas.microsoft.com/sqlserver/2004/07/showplan" {
				return 0, readexec.ErrUnsafe
			}
			seenRoot = true
		case "UserDefinedFunction":
			return 0, readexec.ErrUnsupported
		case "StmtSimple":
			if seenStatement || attrs["StatementType"] != "SELECT" {
				return 0, readexec.ErrUnsafe
			}
			seenStatement = true
			value, err := strconv.ParseFloat(attrs["StatementSubTreeCost"], 64)
			if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return 0, readexec.ErrUnsafe
			}
			cost = value
		case "Object":
			if strings.Trim(attrs["Database"], "[]") != database {
				return 0, readexec.ErrUnsafe
			}
			schema := strings.Trim(attrs["Schema"], "[]")
			table := strings.Trim(attrs["Table"], "[]")
			matched := false
			for _, relation := range binding.Relations {
				if relation.Schema == schema && relation.Name == table {
					matched = true
				}
			}
			if !matched {
				return 0, readexec.ErrUnsafe
			}
		}
	}
	if !seenRoot || !seenStatement {
		return 0, readexec.ErrUnsafe
	}
	return cost, nil
}
func (s *Service) explainSQLServer(ctx context.Context, e identity.Envelope, candidate readexec.Candidate, c config.SourceConnection, expected readexec.Binding) error {
	statement, parameters, err := candidate.SQL(e, expected)
	if err != nil {
		return err
	}
	args, err := sqlServerArguments(parameters)
	if err != nil {
		return err
	}
	client, location, err := s.sqlServerClient(c)
	if err != nil {
		return err
	}
	tag, err := sqlServerTag("explain-")
	if err != nil {
		return err
	}
	verifier := func(ctx context.Context, session bruinmssql.ReadSession) error {
		actual, err := inspectSQLServerContext(ctx, session, c, expected.Source, expected.Revision, location)
		if err != nil {
			return err
		}
		if readexec.Hash(actual) != readexec.Hash(expected) {
			return readexec.ErrBinding
		}
		_, err = sqlServerExplain(ctx, session, statement, args, actual)
		return err
	}
	rows, _, err := client.OpenReadVerified(ctx, &query.Query{Query: "SELECT 1 AS chartworks_explain"}, tag, discardSQLServerObserver{}, bruinmssql.ReadOptions{RequireTLS: !c.AllowInsecureLocal, Timeout: time.Duration(s.settings.QueryTimeout)}, verifier)
	if err != nil {
		return safe(err)
	}
	return safe(rows.Close())
}

type sqlServerObserver struct {
	observer readexec.Observer
	issued   *bool
}

func (o sqlServerObserver) OnDispatch(ctx context.Context, id bruinmssql.ReadIdentity) error {
	remote := sqlServerRemote(id)
	if !remote.Valid() {
		return readexec.ErrBinding
	}
	if o.observer != nil {
		if err := o.observer.Dispatch(ctx, remote, false); err != nil {
			return err
		}
	}
	*o.issued = true
	return nil
}
func (o sqlServerObserver) OnAcknowledged(ctx context.Context, id bruinmssql.ReadIdentity) error {
	if o.observer == nil {
		return nil
	}
	return o.observer.Dispatch(ctx, sqlServerRemote(id), true)
}
func sqlServerRemote(id bruinmssql.ReadIdentity) readexec.RemoteQuery {
	return readexec.RemoteQuery{Driver: "sqlserver", Tag: id.AttemptTag, SQLServer: &readexec.SQLServerRemoteQuery{SessionID: id.SessionID, Started: id.LoginTime, Server: id.Server, Account: id.Account, Database: id.Database}}
}
func (s *Service) executeSQLServer(ctx context.Context, e identity.Envelope, p readexec.Plan, record Record, c config.SourceConnection, l readexec.Limits, id string, observer readexec.Observer) (out readexec.NativeResult, err error) {
	out.RemoteState = "not_issued"
	client, location, err := s.sqlServerClient(c)
	if err != nil {
		return out, err
	}
	statement, parameters, err := p.SQL(e, record.Binding)
	if err != nil {
		return out, err
	}
	args, err := sqlServerArguments(parameters)
	if err != nil {
		return out, err
	}
	cost := 0.0
	verifier := func(ctx context.Context, session bruinmssql.ReadSession) error {
		actual, err := inspectSQLServerContext(ctx, session, c, record.Source.ID, record.Source.Revision, location)
		if err != nil {
			return err
		}
		if _, _, err = p.SQL(e, actual); err != nil {
			return err
		}
		cost, err = sqlServerExplain(ctx, session, statement, args, actual)
		if err != nil {
			return err
		}
		if cost > l.PlannerCost {
			return readexec.ErrLimit
		}
		return nil
	}
	issued := false
	rows, _, err := client.OpenReadVerified(ctx, &query.Query{Query: statement, Args: args}, "cw-read:"+id, sqlServerObserver{observer: observer, issued: &issued}, bruinmssql.ReadOptions{RequireTLS: !c.AllowInsecureLocal, Timeout: l.Timeout, CancelTimeout: l.CancelGrace}, verifier)
	if err != nil {
		if issued {
			out.RemoteState = "unknown"
			var failure *bruinmssql.ReadFailure
			if errors.As(err, &failure) && failure.Stopped {
				out.RemoteState = "stopped"
			}
		}
		return out, readFailure(ctx, err)
	}
	out.RemoteState = "running"
	defer func() {
		closeErr := rows.Close()
		if closeErr == nil {
			out.RemoteState = "stopped"
		} else {
			out.RemoteState = "unknown"
			err = readexec.ErrUncertain
		}
		if err != nil {
			out.Result = readexec.Result{}
			err = readFailure(ctx, err)
		}
	}()
	schema, err := sqlServerResultFields(rows.Columns())
	if err != nil {
		return out, err
	}
	collector, err := readexec.NewCollector(schema, l.Rows, l.Bytes)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		if err = ctx.Err(); err != nil {
			return out, err
		}
		if observer != nil {
			if err = observer.Check(ctx); err != nil {
				return out, err
			}
		}
		values, valueErr := rows.Values()
		if valueErr != nil {
			return out, valueErr
		}
		raw, valueErr := sqlServerResultValues(rows.Columns(), values)
		if valueErr != nil {
			return out, valueErr
		}
		more, valueErr := collector.Add(raw)
		if valueErr != nil {
			return out, valueErr
		}
		if !more {
			break
		}
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	out.Result = collector.Result()
	out.Result.Cost.PlannerUnits = &cost
	return out, nil
}
func sqlServerResultFields(columns []query.Column) ([]readexec.Field, error) {
	fields := make([]readexec.Field, len(columns))
	for i, column := range columns {
		native := strings.ToLower(column.DatabaseType)
		category := sqlServerCategory(native)
		if category == "" {
			return nil, readexec.ErrType
		}
		encoding := "string"
		if category == "number" {
			encoding = "number"
		}
		if category == "boolean" {
			encoding = "boolean"
		}

		fields[i] = readexec.Field{Name: column.Name, Type: category, Encoding: encoding, NativeType: native}
	}
	return fields, nil
}
func sqlServerResultValues(columns []query.Column, values []any) ([][]byte, error) {
	if len(columns) != len(values) {
		return nil, readexec.ErrType
	}
	out := make([][]byte, len(values))
	for i, value := range values {
		if value == nil {
			continue
		}
		native := strings.ToLower(columns[i].DatabaseType)
		switch v := value.(type) {
		case []byte:
			if sqlServerCategory(native) == "binary" {
				out[i] = []byte("\\x" + hex.EncodeToString(v))
			} else {
				out[i] = append([]byte(nil), v...)
			}
		case string:
			out[i] = []byte(v)
		case int64:
			out[i] = []byte(strconv.FormatInt(v, 10))
		case bool:
			if v {
				out[i] = []byte("t")
			} else {
				out[i] = []byte("f")
			}
		case float64:
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return nil, readexec.ErrType
			}
			bits := 64
			if native == "real" {
				bits = 32
			}
			out[i] = []byte(strconv.FormatFloat(v, 'g', -1, bits))
		case time.Time:
			format := "2006-01-02T15:04:05.9999999"
			switch native {
			case "date":
				format = "2006-01-02"
			case "time":
				format = "15:04:05.9999999"
			case "datetimeoffset":
				format = time.RFC3339Nano
			}
			out[i] = []byte(v.Format(format))
		default:
			return nil, readexec.ErrType
		}
	}
	return out, nil
}
func (s *Service) controlSQLServer(ctx context.Context, e identity.Envelope, control readexec.Control, c config.SourceConnection, cancel bool) (string, error) {
	source, partition := control.Coordinates()
	scope, err := sourceScope(e, "sources.query", "query", source)
	if err != nil {
		return "unknown", err
	}
	record, err := s.repo.ReadSource(ctx, scope, source)
	if err != nil {
		return "unknown", err
	}
	if record.Source.ContextID != partition {
		return "unknown", readexec.ErrBinding
	}
	remote, err := control.Target(e, record.Binding)
	if err != nil || remote.SQLServer == nil {
		return "unknown", readexec.ErrBinding
	}
	client, _, err := s.sqlServerClient(c)
	if err != nil {
		return "unknown", err
	}
	native := bruinmssql.ReadIdentity{SessionID: remote.SQLServer.SessionID, LoginTime: remote.SQLServer.Started, Server: remote.SQLServer.Server, Account: remote.SQLServer.Account, Database: remote.SQLServer.Database, AttemptTag: remote.Tag}
	options := bruinmssql.ReadOptions{RequireTLS: !c.AllowInsecureLocal, Timeout: time.Duration(s.settings.QueryTimeout), CancelTimeout: time.Duration(s.settings.QueryTimeout)}
	var state bruinmssql.ReadState
	if cancel {
		state, err = client.CancelRead(ctx, native, options)
	} else {
		state, err = client.ReadStatus(ctx, native, options)
	}
	if err != nil {
		return "unknown", safe(err)
	}
	switch state {
	case bruinmssql.ReadStateStopped:
		return "stopped", nil
	case bruinmssql.ReadStateRunning:
		return "running", nil
	}
	return "unknown", nil
}
