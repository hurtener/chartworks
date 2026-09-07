package sources

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	bruinmysql "github.com/bruin-data/bruin/pkg/mysql"
	"github.com/bruin-data/bruin/pkg/query"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type rawMySQLConfig string

func (c rawMySQLConfig) GetIngestrURI() string     { return string(c) }
func (c rawMySQLConfig) ToDBConnectionURI() string { return string(c) }

type discardMySQLObserver struct{}

func (discardMySQLObserver) OnDispatch(context.Context, bruinmysql.ReadIdentity) error { return nil }
func (discardMySQLObserver) OnAcknowledged(context.Context, bruinmysql.ReadIdentity) error {
	return nil
}

func (s *Service) mysqlClient(ctx context.Context, c config.SourceConnection) (*bruinmysql.Client, string, error) {
	raw, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok || len(raw) > 16384 {
		return nil, "", store.ErrUnavailable
	}
	parsed, err := mysqldriver.ParseDSN(raw)
	if err != nil || parsed.Net != "tcp" || parsed.Addr == "" || parsed.DBName == "" {
		return nil, "", store.ErrInvalid
	}
	if parsed.TLSConfig == "" || parsed.TLSConfig == "false" {
		host, _, splitErr := net.SplitHostPort(parsed.Addr)
		ip := net.ParseIP(host)
		if !c.AllowInsecureLocal || host != "localhost" && (ip == nil || !ip.IsLoopback()) || splitErr != nil {
			return nil, "", readexec.ErrUnsafe
		}
	}
	key := c.Tenant + "/" + c.ID
	material := readexec.Hash([]any{raw, c.Version})
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.mysqlPools[key]; ok && entry.material == material {
		return entry.client, parsed.Net + ":" + parsed.Addr + "/" + parsed.DBName, nil
	}
	client, err := bruinmysql.NewClientWithContext(ctx, rawMySQLConfig(raw))
	if err != nil {
		return nil, "", safe(err)
	}
	old := s.mysqlPools[key]
	s.mysqlPools[key] = mysqlPoolEntry{material: material, client: client}
	if old.client != nil {
		_ = old.client.Close()
	}
	return client, parsed.Net + ":" + parsed.Addr + "/" + parsed.DBName, nil
}

func (s *Service) probe(ctx context.Context, c config.SourceConnection, id string, revision int64, consume func(context.Context, readTransaction, readexec.Binding) error) (readexec.Binding, error) {
	if c.Dialect == "" || c.Dialect == "postgres" {
		return s.probePostgres(ctx, c, id, revision, consume)
	}
	if consume != nil {
		return readexec.Binding{}, readexec.ErrUnsupported
	}
	switch c.Dialect {
	case "bigquery":
		return s.probeBigQuery(ctx, c, id, revision)
	case "snowflake":
		return s.probeSnowflake(ctx, c, id, revision)
	case "databricks":
		return s.probeDatabricks(ctx, c, id, revision)
	case "sqlserver":
		return s.probeSQLServer(ctx, c, id, revision)
	case "mysql":
	default:
		return readexec.Binding{}, readexec.ErrUnsupported
	}
	client, location, err := s.mysqlClient(ctx, c)
	if err != nil {
		return readexec.Binding{}, err
	}
	var binding readexec.Binding
	verify := func(ctx context.Context, session bruinmysql.ReadSession) error {
		binding, err = inspectMySQLContext(ctx, session, c, id, revision, location)
		return err
	}
	stream, _, err := client.OpenReadVerified(ctx, &query.Query{Query: "SELECT 1 AS chartworks_probe"}, "probe-"+readexec.Hash([]any{id, revision})[:24], discardMySQLObserver{}, bruinmysql.ReadOptions{RequireTLS: !c.AllowInsecureLocal}, verify)
	if err != nil {
		return readexec.Binding{}, safe(err)
	}
	defer stream.Close()
	return binding, nil
}

func inspectMySQLContext(ctx context.Context, session bruinmysql.ReadSession, c config.SourceConnection, id string, revision int64, location string) (readexec.Binding, error) {
	evidence := []any{location, c.Version}
	identityRows, err := session.Query(ctx, &query.Query{Query: "SELECT VERSION(), CURRENT_USER(), DATABASE()"})
	if err != nil {
		return readexec.Binding{}, err
	}
	identity, err := oneMySQLRow(identityRows, 3)
	if err != nil || mysqlText(identity[2]) == "" || !strings.HasPrefix(mysqlText(identity[0]), "8.4.") {
		return readexec.Binding{}, readexec.ErrUnsupported
	}
	evidence = append(evidence, mysqlText(identity[0]), mysqlText(identity[1]), mysqlText(identity[2]), "native-read-only-transaction")
	relations := append([]config.SourceRelation(nil), c.Relations...)
	sort.Slice(relations, func(i, j int) bool {
		return relations[i].Schema+"."+relations[i].Name < relations[j].Schema+"."+relations[j].Name
	})
	out := readexec.Binding{Tenant: c.Tenant, Source: id, Context: contextID(id, revision), Revision: revision, Dialect: "mysql"}
	for _, relation := range relations {
		rows, err := session.Query(ctx, &query.Query{Query: "SELECT TABLE_TYPE FROM information_schema.TABLES WHERE TABLE_SCHEMA=? AND TABLE_NAME=?", Args: []any{relation.Schema, relation.Name}})
		if err != nil {
			return readexec.Binding{}, err
		}
		table, err := oneMySQLRow(rows, 1)
		if err != nil || mysqlText(table[0]) != "BASE TABLE" {
			return readexec.Binding{}, readexec.ErrUnsafe
		}
		rows, err = session.Query(ctx, &query.Query{Query: "SELECT COLUMN_NAME,COLUMN_TYPE,DATA_TYPE,IS_NULLABLE,ORDINAL_POSITION FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=? AND TABLE_NAME=? ORDER BY ORDINAL_POSITION", Args: []any{relation.Schema, relation.Name}})
		if err != nil {
			return readexec.Binding{}, err
		}
		columns, nativeEvidence, err := mysqlColumns(rows, relation.Columns)
		if err != nil {
			return readexec.Binding{}, err
		}
		relationID := "ds:" + readexec.Hash([]string{id, relation.Schema, relation.Name})[:32]
		out.Relations = append(out.Relations, readexec.Relation{ID: relationID, Schema: relation.Schema, Name: relation.Name, Columns: columns})
		evidence = append(evidence, []any{relation.Schema, relation.Name, table, nativeEvidence})
	}
	out.Contract = "source-contract:" + readexec.Hash([]any{c.Version, out.Relations})[:32]
	out.Fingerprint = readexec.Hash(evidence)
	if !out.Valid() {
		return readexec.Binding{}, readexec.ErrBinding
	}
	return out, nil
}

func oneMySQLRow(rows query.RowStream, width int) ([]any, error) {
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, readexec.ErrBinding
	}
	values, err := rows.Values()
	if err != nil || len(values) != width || rows.Next() {
		return nil, readexec.ErrBinding
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return values, nil
}

func mysqlColumns(rows query.RowStream, declared []string) ([]readexec.Column, []any, error) {
	defer rows.Close()
	columns := make([]readexec.Column, 0, len(declared))
	evidence := make([]any, 0, len(declared))
	for rows.Next() {
		values, err := rows.Values()
		if err != nil || len(values) != 5 {
			return nil, nil, readexec.ErrBinding
		}
		name, native, base := mysqlText(values[0]), mysqlText(values[1]), mysqlText(values[2])
		if len(columns) >= len(declared) || declared[len(columns)] != name || !mysqlSafeType(base) {
			return nil, nil, readexec.ErrUnsupported
		}
		columns = append(columns, readexec.Column{Name: name, NativeType: native, Category: mysqlCategory(base), Nullable: mysqlText(values[3]) == "YES", Safe: true})
		evidence = append(evidence, []any{name, native, base, mysqlText(values[3]), mysqlText(values[4])})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	if len(columns) != len(declared) {
		return nil, nil, readexec.ErrBinding
	}
	return columns, evidence, nil
}

func mysqlText(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case uint64:
		return strconv.FormatUint(v, 10)
	default:
		return fmt.Sprint(v)
	}
}

func mysqlSafeType(t string) bool { return mysqlCategory(t) != "" }
func mysqlCategory(t string) string {
	switch strings.ToLower(t) {
	case "tinyint", "smallint", "mediumint", "int", "bigint":
		return "integer"
	case "decimal":
		return "decimal"
	case "float", "double":
		return "number"
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set":
		return "text"
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob", "bit":
		return "binary"
	case "date", "time", "datetime", "timestamp", "year":
		return "temporal"
	case "json":
		return "structured"
	}
	return ""
}

func (s *Service) explainMySQL(ctx context.Context, e identity.Envelope, candidate readexec.Candidate, c config.SourceConnection, expected readexec.Binding) error {
	client, location, err := s.mysqlClient(ctx, c)
	if err != nil {
		return err
	}
	statement, parameters, err := candidate.SQL(e, expected)
	if err != nil {
		return err
	}
	args, err := mysqlArguments(parameters)
	if err != nil {
		return err
	}
	verify := func(ctx context.Context, session bruinmysql.ReadSession) error {
		actual, err := inspectMySQLContext(ctx, session, c, expected.Source, expected.Revision, location)
		if err != nil {
			return err
		}
		if readexec.Hash(actual) != readexec.Hash(expected) {
			return readexec.ErrBinding
		}
		return nil
	}
	stream, _, err := client.OpenReadVerified(ctx, &query.Query{Query: "EXPLAIN FORMAT=JSON " + statement, Args: args}, "explain-"+readexec.Hash(statement)[:24], discardMySQLObserver{}, bruinmysql.ReadOptions{RequireTLS: !c.AllowInsecureLocal}, verify)
	if err != nil {
		return safe(err)
	}
	defer stream.Close()
	if !stream.Next() {
		return readexec.ErrUnsafe
	}
	values, err := stream.Values()
	if err != nil || len(values) != 1 || len(mysqlText(values[0])) > 1<<20 {
		return readexec.ErrUnsafe
	}
	return stream.Err()
}

func mysqlArguments(parameters []readexec.Parameter) ([]any, error) {
	out := make([]any, 0, len(parameters))
	for _, p := range parameters {
		switch p.Kind {
		case "null":
			out = append(out, nil)
		case "text":
			out = append(out, p.Value)
		case "boolean":
			out = append(out, p.Value == "true")
		case "integer":
			n, err := strconv.ParseInt(p.Value, 10, 64)
			if err != nil {
				return nil, readexec.ErrBinding
			}
			out = append(out, n)
		case "number":
			// Preserve exact decimal text for MySQL's native conversion rather
			// than routing it through a binary floating-point value.
			out = append(out, p.Value)
		default:
			return nil, readexec.ErrBinding
		}
	}
	return out, nil
}

type mysqlReadObserver struct {
	observer readexec.Observer
	id       string
	issued   bool
}

func (o *mysqlReadObserver) remote(i bruinmysql.ReadIdentity) readexec.RemoteQuery {
	return readexec.RemoteQuery{Driver: "mysql", Tag: "cw-read:" + o.id, MySQL: &readexec.MySQLRemoteQuery{ConnectionID: i.ConnectionID, Account: i.Account, Database: i.Database, ServerUUID: i.ServerUUID}}
}
func (o *mysqlReadObserver) OnDispatch(ctx context.Context, i bruinmysql.ReadIdentity) error {
	o.issued = true
	if o.observer == nil {
		return nil
	}
	return o.observer.Dispatch(ctx, o.remote(i), false)
}
func (o *mysqlReadObserver) OnAcknowledged(ctx context.Context, i bruinmysql.ReadIdentity) error {
	if o.observer == nil {
		return nil
	}
	return o.observer.Dispatch(ctx, o.remote(i), true)
}

func (s *Service) executeMySQL(ctx context.Context, e identity.Envelope, p readexec.Plan, record Record, c config.SourceConnection, limits readexec.Limits, id string, observer readexec.Observer) (out readexec.NativeResult, err error) {
	out.RemoteState = "not_issued"
	client, location, err := s.mysqlClient(ctx, c)
	if err != nil {
		return out, err
	}
	statement, parameters, err := p.SQL(e, record.Binding)
	if err != nil {
		return out, err
	}
	args, err := mysqlArguments(parameters)
	if err != nil {
		return out, err
	}
	verify := func(ctx context.Context, session bruinmysql.ReadSession) error {
		actual, err := inspectMySQLContext(ctx, session, c, record.Source.ID, record.Source.Revision, location)
		if err != nil {
			return err
		}
		if readexec.Hash(actual) != readexec.Hash(record.Binding) {
			return readexec.ErrBinding
		}
		return nil
	}
	journal := &mysqlReadObserver{observer: observer, id: id}
	stream, _, err := client.OpenReadVerified(ctx, &query.Query{Query: statement, Args: args}, "cw-read:"+id, journal, bruinmysql.ReadOptions{RequireTLS: !c.AllowInsecureLocal, MaxRows: limits.Rows + 1}, verify)
	if journal.issued {
		out.RemoteState = "running"
	}
	if err != nil {
		return out, safe(err)
	}
	defer func() {
		if closeErr := stream.Close(); closeErr != nil && err == nil {
			err = safe(closeErr)
		}
		if err == nil {
			out.RemoteState = "stopped"
		}
	}()
	schema := make([]readexec.Field, len(stream.Columns()))
	for i, column := range stream.Columns() {
		schema[i], err = mysqlResultField(column.Name, column.DatabaseType)
		if err != nil {
			return out, err
		}
	}
	collector, err := readexec.NewCollector(schema, limits.Rows, limits.Bytes)
	if err != nil {
		return out, err
	}
	for stream.Next() {
		if observer != nil {
			if err = observer.Check(ctx); err != nil {
				return out, err
			}
		}
		values, valueErr := stream.Values()
		if valueErr != nil || len(values) != len(schema) {
			return out, readexec.ErrType
		}
		raw := make([][]byte, len(values))
		for i := range values {
			raw[i], err = mysqlResultValue(schema[i], values[i])
			if err != nil {
				return out, err
			}
		}
		more, addErr := collector.Add(raw)
		if addErr != nil {
			return out, addErr
		}
		if !more {
			break
		}
	}
	if err = stream.Err(); err != nil {
		return out, safe(err)
	}
	out.Result = collector.Result()
	return out, nil
}

func mysqlResultField(name, native string) (readexec.Field, error) {
	category := mysqlCategory(strings.ToLower(native))
	field := readexec.Field{Name: name, Type: category, Encoding: "string", NativeType: strings.ToLower(native)}
	if category == "number" {
		field.Encoding = "number"
	}
	if category == "" {
		return readexec.Field{}, readexec.ErrType
	}
	return field, nil
}

func mysqlResultValue(field readexec.Field, value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	if field.Type == "binary" {
		bytes, ok := value.([]byte)
		if !ok {
			return nil, readexec.ErrType
		}
		return []byte(`\x` + hex.EncodeToString(bytes)), nil
	}
	switch v := value.(type) {
	case []byte:
		return append([]byte(nil), v...), nil
	case string:
		return []byte(v), nil
	case int64:
		return []byte(strconv.FormatInt(v, 10)), nil
	case uint64:
		if v > 1<<63-1 {
			return nil, readexec.ErrType
		}
		return []byte(strconv.FormatUint(v, 10)), nil
	case float64:
		return []byte(strconv.FormatFloat(v, 'g', -1, 64)), nil
	case bool:
		if v {
			return []byte("t"), nil
		}
		return []byte("f"), nil
	case time.Time:
		return []byte(v.Format("2006-01-02 15:04:05.999999999")), nil
	default:
		return nil, readexec.ErrType
	}
}

func (s *Service) controlMySQL(ctx context.Context, e identity.Envelope, control readexec.Control, record Record, c config.SourceConnection, cancel bool) (string, error) {
	actual, err := s.probe(ctx, c, record.Source.ID, record.Source.Revision, nil)
	if err != nil {
		return "unknown", err
	}
	if readexec.Hash(actual) != readexec.Hash(record.Binding) {
		return "unknown", readexec.ErrBinding
	}
	remote, err := control.Target(e, actual)
	if err != nil {
		return "unknown", err
	}
	client, _, err := s.mysqlClient(ctx, c)
	if err != nil {
		return "unknown", err
	}
	identity := bruinmysql.ReadIdentity{ConnectionID: remote.MySQL.ConnectionID, AttemptTag: remote.Tag, Account: remote.MySQL.Account, Database: remote.MySQL.Database, ServerUUID: remote.MySQL.ServerUUID}
	if cancel {
		if err := client.CancelRead(ctx, identity, bruinmysql.ReadOptions{RequireTLS: !c.AllowInsecureLocal}); err != nil {
			if errors.Is(err, bruinmysql.ErrReadNotActive) {
				return "unknown", readexec.ErrUncertain
			}
			return "unknown", safe(err)
		}
		return "running", nil
	}
	state, err := client.ReadStatus(ctx, identity, bruinmysql.ReadOptions{RequireTLS: !c.AllowInsecureLocal})
	if err != nil {
		return "unknown", safe(err)
	}
	switch state {
	case bruinmysql.ReadStateRunning, bruinmysql.ReadStateCancelRequested:
		return "running", nil
	case bruinmysql.ReadStateStopped:
		return "stopped", nil
	default:
		return "unknown", readexec.ErrUncertain
	}
}
