package sources

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/civil"
	bruinbigquery "github.com/bruin-data/bruin/pkg/bigquery"
	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type bigQueryPoolEntry struct {
	material string
	client   *bruinbigquery.Client
}

type bigQueryObserver struct {
	observer readexec.Observer
	tag      string
}

func (o bigQueryObserver) OnDispatch(ctx context.Context, id bruinbigquery.ReadIdentity) error {
	return o.dispatch(ctx, id, false)
}
func (o bigQueryObserver) OnAcknowledged(ctx context.Context, id bruinbigquery.ReadIdentity) error {
	return o.dispatch(ctx, id, true)
}
func (o bigQueryObserver) dispatch(ctx context.Context, id bruinbigquery.ReadIdentity, accepted bool) error {
	if o.observer == nil {
		return nil
	}
	return o.observer.Dispatch(ctx, readexec.RemoteQuery{Driver: "bigquery", Tag: o.tag, BigQuery: &readexec.BigQueryRemoteQuery{Project: id.ProjectID, Location: id.Location, JobID: id.JobID}}, accepted)
}

func (s *Service) bigQueryClient(c config.SourceConnection) (*bruinbigquery.Client, *bruinbigquery.Config, error) {
	raw, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok || len(raw) == 0 || len(raw) > 1<<20 {
		return nil, nil, store.ErrUnavailable
	}
	var native bruinbigquery.Config
	if json.Unmarshal([]byte(raw), &native) != nil || !native.IsValid() || native.Location == "" {
		return nil, nil, store.ErrInvalid
	}
	key := c.Tenant + "/" + c.ID
	material := readexec.Hash([]any{raw, c.Version})
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.bigQueryPools[key]; ok && entry.material == material {
		return entry.client, &native, nil
	}
	client, err := bruinbigquery.NewDB(&native)
	if err != nil {
		return nil, nil, safe(err)
	}
	old := s.bigQueryPools[key]
	s.bigQueryPools[key] = bigQueryPoolEntry{material: material, client: client}
	if old.client != nil {
		_ = old.client.Close()
	}
	return client, &native, nil
}

func (s *Service) closeBigQuery() {
	for key, entry := range s.bigQueryPools {
		_ = entry.client.Close()
		delete(s.bigQueryPools, key)
	}
}

func (s *Service) probeBigQuery(ctx context.Context, c config.SourceConnection, id string, revision int64) (readexec.Binding, error) {
	client, native, err := s.bigQueryClient(c)
	if err != nil {
		return readexec.Binding{}, err
	}
	out := readexec.Binding{Tenant: c.Tenant, Source: id, Context: contextID(id, revision), Revision: revision, Dialect: "bigquery"}
	evidence := []any{native.ProjectID, native.Location, c.Version}
	for _, relation := range c.Relations {
		tableSQL := fmt.Sprintf("SELECT table_type FROM `%s.%s.INFORMATION_SCHEMA.TABLES` WHERE table_name=@table", native.ProjectID, relation.Schema)
		tableArgs := []any{bruinbigquery.ReadParameter{Name: "table", Type: "STRING", Value: relation.Name}}
		tableStream, _, tableErr := client.OpenRead(ctx, &query.Query{Query: tableSQL, Args: tableArgs}, "probe-table:"+readexec.Hash([]any{id, revision, relation.Schema, relation.Name})[:24], bigQueryObserver{}, bruinbigquery.ReadOptions{MaxBytesBilled: 1 << 30, JobTimeout: time.Minute, CancelTimeout: time.Second})
		if tableErr != nil {
			return readexec.Binding{}, safe(tableErr)
		}
		tableType, tableErr := catalogTableType(tableStream, map[string]bool{"BASE TABLE": true})
		_ = tableStream.Close()
		if tableErr != nil {
			return readexec.Binding{}, tableErr
		}
		statement := fmt.Sprintf("SELECT column_name,data_type,is_nullable,ordinal_position FROM `%s.%s.INFORMATION_SCHEMA.COLUMNS` WHERE table_name=@table ORDER BY ordinal_position", native.ProjectID, relation.Schema)
		args := []any{bruinbigquery.ReadParameter{Name: "table", Type: "STRING", Value: relation.Name}}
		stream, _, openErr := client.OpenRead(ctx, &query.Query{Query: statement, Args: args}, "probe:"+readexec.Hash([]any{id, revision, relation.Schema, relation.Name})[:24], bigQueryObserver{}, bruinbigquery.ReadOptions{MaxBytesBilled: 1 << 30, JobTimeout: time.Minute, CancelTimeout: time.Second})
		if openErr != nil {
			return readexec.Binding{}, safe(openErr)
		}
		columns, nativeEvidence, rowsErr := catalogColumns(stream, relation.Columns, bigQueryCategory)
		_ = stream.Close()
		if rowsErr != nil {
			return readexec.Binding{}, rowsErr
		}
		relationID := "ds:" + readexec.Hash([]string{id, relation.Schema, relation.Name})[:32]
		out.Relations = append(out.Relations, readexec.Relation{ID: relationID, Schema: relation.Schema, Name: relation.Name, Columns: columns})
		evidence = append(evidence, []any{relation.Schema, relation.Name, tableType, nativeEvidence})
	}
	out.Contract = "source-contract:" + readexec.Hash([]any{c.Version, out.Relations})[:32]
	out.Fingerprint = readexec.Hash(evidence)
	if !out.Valid() {
		return readexec.Binding{}, readexec.ErrBinding
	}
	return out, nil
}

func (s *Service) explainBigQuery(ctx context.Context, e identity.Envelope, candidate readexec.Candidate, c config.SourceConnection, expected readexec.Binding) error {
	client, native, err := s.bigQueryClient(c)
	if err != nil {
		return err
	}
	statement, parameters, err := candidate.SQL(e, expected)
	if err != nil {
		return err
	}
	// Bruin's current dry-run path does not bind Query.Args. Deny parameterized
	// plans here rather than executing user SQL as a validation side effect.
	if len(parameters) != 0 {
		return readexec.ErrUnsupported
	}
	result, err := client.DryRunQuery(ctx, &query.Query{Query: statement})
	if err != nil || result == nil || !result.Valid || result.StatementType != "SELECT" {
		return readexec.ErrUnsafe
	}
	allowed := map[string]bool{}
	for _, relation := range expected.Relations {
		allowed[relation.Schema+"."+relation.Name] = true
		allowed[native.ProjectID+"."+relation.Schema+"."+relation.Name] = true
	}
	if len(result.ReferencedTables) == 0 {
		return readexec.ErrUnsafe
	}
	for _, relation := range result.ReferencedTables {
		if !allowed[relation] {
			return readexec.ErrUnsafe
		}
	}
	if len(result.Schema) == 0 {
		return readexec.ErrUnsafe
	}
	for _, column := range result.Schema {
		if column.Name == "" || bigQueryCategory(column.Type) == "" {
			return readexec.ErrUnsupported
		}
	}
	return nil
}

func (s *Service) executeBigQuery(ctx context.Context, e identity.Envelope, p readexec.Plan, record Record, c config.SourceConnection, l readexec.Limits, id string, observer readexec.Observer) (readexec.NativeResult, error) {
	out := readexec.NativeResult{RemoteState: "not_issued"}
	client, _, err := s.bigQueryClient(c)
	if err != nil {
		return out, err
	}
	statement, parameters, err := p.SQL(e, record.Binding)
	if err != nil {
		return out, err
	}
	args, err := bigQueryArguments(parameters)
	if err != nil {
		return out, err
	}
	tag := "cw-read:" + id
	stream, _, err := client.OpenRead(ctx, &query.Query{Query: statement, Args: args}, id, bigQueryObserver{observer: observer, tag: tag}, bruinbigquery.ReadOptions{MaxBytesBilled: int64(l.PlannerCost), JobTimeout: l.Timeout, CancelTimeout: l.CancelGrace})
	if err != nil {
		return out, cloudReadFailure(ctx, err)
	}
	defer stream.Close()
	out.RemoteState = "running"
	result, err := collectCloudRows(ctx, stream, l, observer)
	if err != nil {
		return out, cloudReadFailure(ctx, err)
	}
	out.Result = result
	out.RemoteState = "stopped"
	return out, nil
}

func (s *Service) controlBigQuery(ctx context.Context, e identity.Envelope, control readexec.Control, record Record, c config.SourceConnection, cancel bool) (string, error) {
	actual, err := s.probeBigQuery(ctx, c, record.Source.ID, record.Source.Revision)
	if err != nil {
		return "unknown", err
	}
	if readexec.Hash(actual) != readexec.Hash(record.Binding) {
		return "unknown", readexec.ErrBinding
	}
	target, err := control.Target(e, actual)
	if err != nil {
		return "unknown", err
	}
	if target.BigQuery == nil {
		return "unknown", readexec.ErrBinding
	}
	client, _, err := s.bigQueryClient(c)
	if err != nil {
		return "unknown", err
	}
	id := bruinbigquery.ReadIdentity{ProjectID: target.BigQuery.Project, Location: target.BigQuery.Location, JobID: target.BigQuery.JobID}
	var state bruinbigquery.ReadState
	if cancel {
		state, err = client.CancelRead(ctx, id, bruinbigquery.ReadOptions{CancelTimeout: time.Second})
	} else {
		state, err = client.ReadStatus(ctx, id)
	}
	return cloudState(string(state)), safe(err)
}

func bigQueryArguments(parameters []readexec.Parameter) ([]any, error) {
	out := make([]any, len(parameters))
	for i, p := range parameters {
		name := "p" + strconv.Itoa(i+1)
		switch p.Kind {
		case "null":
			out[i] = bruinbigquery.ReadParameter{Name: name, Type: "STRING", Value: nil}
		case "text":
			out[i] = bruinbigquery.ReadParameter{Name: name, Type: "STRING", Value: p.Value}
		case "boolean":
			out[i] = bruinbigquery.ReadParameter{Name: name, Type: "BOOL", Value: p.Value == "true"}
		case "integer":
			value, err := strconv.ParseInt(p.Value, 10, 64)
			if err != nil {
				return nil, readexec.ErrBinding
			}
			out[i] = bruinbigquery.ReadParameter{Name: name, Type: "INT64", Value: value}
		case "number":
			value, ok := new(big.Rat).SetString(p.Value)
			if !ok {
				return nil, readexec.ErrBinding
			}
			out[i] = bruinbigquery.ReadParameter{Name: name, Type: "BIGNUMERIC", Value: value}
		default:
			return nil, readexec.ErrBinding
		}
	}
	return out, nil
}

func bigQueryCategory(t string) string {
	switch strings.ToUpper(t) {
	case "INT64", "INTEGER":
		return "integer"
	case "NUMERIC", "BIGNUMERIC":
		return "decimal"
	case "FLOAT64", "FLOAT":
		return "number"
	case "BOOL", "BOOLEAN":
		return "boolean"
	case "BYTES":
		return "binary"
	case "DATE", "TIME", "DATETIME", "TIMESTAMP":
		return "temporal"
	case "JSON":
		return "structured"
	case "STRING", "GEOGRAPHY":
		return "text"
	}
	return ""
}

func catalogColumns(stream query.RowStream, declared []string, category func(string) string) ([]readexec.Column, []any, error) {
	columns := make([]readexec.Column, 0, len(declared))
	evidence := make([]any, 0, len(declared))
	for stream.Next() {
		values, err := stream.Values()
		if err != nil || len(values) < 3 {
			return nil, nil, readexec.ErrBinding
		}
		name, native, nullable := cloudText(values[0]), cloudText(values[1]), strings.EqualFold(cloudText(values[2]), "YES")
		kind := category(native)
		if len(columns) >= len(declared) || declared[len(columns)] != name || kind == "" {
			return nil, nil, readexec.ErrUnsupported
		}
		columns = append(columns, readexec.Column{Name: name, NativeType: native, Category: kind, Nullable: nullable, Safe: true})
		evidence = append(evidence, values)
	}
	if err := stream.Err(); err != nil {
		return nil, nil, safe(err)
	}
	if len(columns) != len(declared) {
		return nil, nil, readexec.ErrBinding
	}
	return columns, evidence, nil
}

func catalogTableType(stream query.RowStream, allowed map[string]bool) (string, error) {
	defer stream.Close()
	if !stream.Next() {
		if err := stream.Err(); err != nil {
			return "", safe(err)
		}
		return "", readexec.ErrBinding
	}
	values, err := stream.Values()
	if err != nil || len(values) != 1 || !allowed[strings.ToUpper(cloudText(values[0]))] || stream.Next() {
		return "", readexec.ErrUnsupported
	}
	if err := stream.Err(); err != nil {
		return "", safe(err)
	}
	return strings.ToUpper(cloudText(values[0])), nil
}

func cloudText(value any) string {
	if raw, ok := value.([]byte); ok {
		return string(raw)
	}
	return fmt.Sprint(value)
}

func collectCloudRows(ctx context.Context, stream query.RowStream, l readexec.Limits, observer readexec.Observer) (readexec.Result, error) {
	columns := stream.Columns()
	schema := make([]readexec.Field, len(columns))
	for i, column := range columns {
		field, err := cloudField(column)
		if err != nil {
			return readexec.Result{}, err
		}
		schema[i] = field
	}
	collector, err := readexec.NewCollector(schema, l.Rows, l.Bytes)
	if err != nil {
		return readexec.Result{}, err
	}
	for stream.Next() {
		if err := ctx.Err(); err != nil {
			return readexec.Result{}, err
		}
		if observer != nil {
			if err := observer.Check(ctx); err != nil {
				return readexec.Result{}, err
			}
		}
		values, err := stream.Values()
		if err != nil {
			return readexec.Result{}, err
		}
		raw := make([][]byte, len(values))
		for i, value := range values {
			raw[i], err = cloudValue(schema[i], columns[i], value)
			if err != nil {
				return readexec.Result{}, err
			}
		}
		more, err := collector.Add(raw)
		if err != nil {
			return readexec.Result{}, err
		}
		if !more {
			break
		}
	}
	if err := stream.Err(); err != nil {
		return readexec.Result{}, err
	}
	return collector.Result(), nil
}

func cloudField(c query.Column) (readexec.Field, error) {
	category := bigQueryCategory(c.DatabaseType)
	if category == "" {
		category = snowflakeCategory(c.DatabaseType)
	}
	if category == "" {
		category = databricksCategory(c.DatabaseType)
	}
	if category == "" {
		return readexec.Field{}, readexec.ErrUnsupported
	}
	encoding := "string"
	if category == "boolean" || category == "number" {
		encoding = category
	}
	return readexec.Field{Name: c.Name, Type: category, Encoding: encoding, NativeType: c.DatabaseType}, nil
}

func cloudValue(field readexec.Field, column query.Column, value any) ([]byte, error) {
	if value == nil {
		return nil, nil
	}
	switch v := value.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return []byte(`\x` + hex.EncodeToString(v)), nil
	case bool:
		if v {
			return []byte("t"), nil
		}
		return []byte("f"), nil
	case int64:
		return []byte(strconv.FormatInt(v, 10)), nil
	case int32:
		return []byte(strconv.FormatInt(int64(v), 10)), nil
	case float64:
		return []byte(strconv.FormatFloat(v, 'g', -1, 64)), nil
	case float32:
		return []byte(strconv.FormatFloat(float64(v), 'g', -1, 32)), nil
	case time.Time:
		return []byte(v.UTC().Format(time.RFC3339Nano)), nil
	case civil.Date:
		return []byte(v.String()), nil
	case civil.Time:
		return []byte(v.String()), nil
	case civil.DateTime:
		return []byte(v.String()), nil
	case *big.Rat:
		if v == nil {
			return nil, nil
		}
		if column.DecimalKnown {
			return []byte(v.FloatString(int(column.Scale))), nil
		}
		return []byte(v.RatString()), nil
	case *big.Int:
		if v != nil {
			return []byte(v.String()), nil
		}
	case big.Int:
		return []byte(v.String()), nil
	case *big.Float:
		if v != nil && column.DecimalKnown {
			return []byte(v.Text('f', int(column.Scale))), nil
		}
	case big.Float:
		if column.DecimalKnown {
			return []byte(v.Text('f', int(column.Scale))), nil
		}
	}
	return nil, readexec.ErrUnsupported
}

func cloudReadFailure(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return readexec.ErrTimeout
		}
		if ctx.Err() == context.Canceled {
			return readexec.ErrCancelled
		}
	}
	return safe(err)
}
func cloudState(state string) string {
	switch state {
	case "running":
		return "running"
	case "stopped":
		return "stopped"
	default:
		return "unknown"
	}
}
