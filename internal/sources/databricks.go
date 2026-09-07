package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	bruindatabricks "github.com/bruin-data/bruin/pkg/databricks"
	"github.com/bruin-data/bruin/pkg/query"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type databricksPoolEntry struct {
	material string
	client   databricksClient
}
type databricksClient interface {
	OpenRead(context.Context, *query.Query, string, bruindatabricks.ReadObserver, bruindatabricks.ReadOptions) (query.RowStream, bruindatabricks.ReadIdentity, error)
	ReadStatus(context.Context, bruindatabricks.ReadIdentity) (bruindatabricks.ReadState, error)
	CancelRead(context.Context, bruindatabricks.ReadIdentity, bruindatabricks.ReadOptions) (bruindatabricks.ReadState, error)
	Close() error
}
type databricksLeaf struct{ *bruindatabricks.DB }

func (c databricksLeaf) OpenRead(ctx context.Context, q *query.Query, attempt string, o bruindatabricks.ReadObserver, options bruindatabricks.ReadOptions) (query.RowStream, bruindatabricks.ReadIdentity, error) {
	return c.DB.OpenRead(ctx, q, attempt, o, options)
}

type databricksFactory func(*bruindatabricks.Config) (databricksClient, error)
type databricksObserver struct {
	observer readexec.Observer
	tag      string
}

func (o databricksObserver) OnDispatch(ctx context.Context, id bruindatabricks.ReadIdentity) error {
	return o.dispatch(ctx, id, false)
}
func (o databricksObserver) OnAcknowledged(ctx context.Context, id bruindatabricks.ReadIdentity) error {
	return o.dispatch(ctx, id, true)
}
func (o databricksObserver) dispatch(ctx context.Context, id bruindatabricks.ReadIdentity, accepted bool) error {
	if o.observer == nil {
		return nil
	}
	return o.observer.Dispatch(ctx, readexec.RemoteQuery{Driver: "databricks", Tag: o.tag, Databricks: &readexec.DatabricksRemoteQuery{Workspace: id.Workspace, Warehouse: id.WarehouseID, StatementID: id.StatementID}}, accepted)
}

func (s *Service) databricksClient(c config.SourceConnection) (databricksClient, *bruindatabricks.Config, error) {
	raw, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok || len(raw) == 0 || len(raw) > 1<<20 {
		return nil, nil, store.ErrUnavailable
	}
	var native bruindatabricks.Config
	if json.Unmarshal([]byte(raw), &native) != nil || native.Host == "" || native.Path == "" || (!native.UseOAuthM2M() && native.Token == "") {
		return nil, nil, store.ErrInvalid
	}
	key := c.Tenant + "/" + c.ID
	material := readexec.Hash([]any{raw, c.Version})
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.databricksPools[key]; ok && entry.material == material {
		return entry.client, &native, nil
	}
	create := s.newDatabricks
	if create == nil {
		create = func(c *bruindatabricks.Config) (databricksClient, error) {
			client, err := bruindatabricks.NewDB(c)
			return databricksLeaf{client}, err
		}
	}
	client, err := create(&native)
	if err != nil {
		return nil, nil, safe(err)
	}
	old := s.databricksPools[key]
	s.databricksPools[key] = databricksPoolEntry{material: material, client: client}
	if old.client != nil {
		_ = old.client.Close()
	}
	return client, &native, nil
}
func (s *Service) closeDatabricks() {
	for key, entry := range s.databricksPools {
		_ = entry.client.Close()
		delete(s.databricksPools, key)
	}
}
func databricksOptions(l readexec.Limits) bruindatabricks.ReadOptions {
	return bruindatabricks.ReadOptions{MaxRows: int64(l.Rows + 1), MaxBytes: int64(l.Bytes), MaxResponseBytes: int64(l.Bytes) + (1 << 20), PollInterval: 25 * time.Millisecond, CancelTimeout: l.CancelGrace, Timeout: l.Timeout}
}

func (s *Service) probeDatabricks(ctx context.Context, c config.SourceConnection, id string, revision int64) (readexec.Binding, error) {
	client, native, err := s.databricksClient(c)
	if err != nil {
		return readexec.Binding{}, err
	}
	out := readexec.Binding{Tenant: c.Tenant, Source: id, Context: contextID(id, revision), Revision: revision, Dialect: "databricks"}
	evidence := []any{native.Host, native.Path, native.Catalog, native.Schema, c.Version}
	options := bruindatabricks.ReadOptions{MaxRows: 257, MaxBytes: 1 << 20, MaxResponseBytes: 2 << 20, PollInterval: 25 * time.Millisecond, CancelTimeout: time.Second, Timeout: time.Minute}
	for _, relation := range c.Relations {
		tableSQL := "SELECT table_type FROM information_schema.tables WHERE table_schema=:schema AND table_name=:table"
		tableArgs := []any{bruindatabricks.ReadParameter{Name: "schema", Type: "STRING", Value: relation.Schema}, bruindatabricks.ReadParameter{Name: "table", Type: "STRING", Value: relation.Name}}
		tableStream, _, tableErr := client.OpenRead(ctx, &query.Query{Query: tableSQL, Args: tableArgs}, "probe-table:"+readexec.Hash([]any{id, revision, relation.Schema, relation.Name})[:24], databricksObserver{}, options)
		if tableErr != nil {
			return readexec.Binding{}, safe(tableErr)
		}
		tableType, tableErr := catalogTableType(tableStream, map[string]bool{"MANAGED": true, "EXTERNAL": true})
		if tableErr != nil {
			return readexec.Binding{}, tableErr
		}
		statement := "SELECT column_name,full_data_type,is_nullable,ordinal_position FROM information_schema.columns WHERE table_schema=:schema AND table_name=:table ORDER BY ordinal_position"
		args := []any{bruindatabricks.ReadParameter{Name: "schema", Type: "STRING", Value: relation.Schema}, bruindatabricks.ReadParameter{Name: "table", Type: "STRING", Value: relation.Name}}
		stream, _, openErr := client.OpenRead(ctx, &query.Query{Query: statement, Args: args}, "probe:"+readexec.Hash([]any{id, revision, relation.Schema, relation.Name})[:24], databricksObserver{}, options)
		if openErr != nil {
			return readexec.Binding{}, safe(openErr)
		}
		columns, nativeEvidence, rowsErr := catalogColumns(stream, relation.Columns, databricksCategory)
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
func (s *Service) explainDatabricks(ctx context.Context, e identity.Envelope, candidate readexec.Candidate, c config.SourceConnection, expected readexec.Binding) error {
	client, _, err := s.databricksClient(c)
	if err != nil {
		return err
	}
	statement, parameters, err := candidate.SQL(e, expected)
	if err != nil {
		return err
	}
	args, err := databricksArguments(parameters)
	if err != nil {
		return err
	}
	options := bruindatabricks.ReadOptions{MaxRows: 2, MaxBytes: 1 << 20, MaxResponseBytes: 2 << 20, PollInterval: 25 * time.Millisecond, CancelTimeout: time.Second, Timeout: time.Minute}
	stream, _, err := client.OpenRead(ctx, &query.Query{Query: "EXPLAIN FORMATTED " + statement, Args: args}, "explain:"+readexec.Hash(statement)[:24], databricksObserver{}, options)
	if err != nil {
		return safe(err)
	}
	defer func() { _ = stream.Close() }()
	if !stream.Next() {
		return readexec.ErrUnsafe
	}
	values, err := stream.Values()
	if err != nil || len(values) < 1 || len(fmt.Sprint(values[0])) > 1<<20 {
		return readexec.ErrUnsafe
	}
	return stream.Err()
}
func (s *Service) executeDatabricks(ctx context.Context, e identity.Envelope, p readexec.Plan, record Record, c config.SourceConnection, l readexec.Limits, id string, observer readexec.Observer) (readexec.NativeResult, error) {
	out := readexec.NativeResult{RemoteState: "not_issued"}
	client, _, err := s.databricksClient(c)
	if err != nil {
		return out, err
	}
	statement, parameters, err := p.SQL(e, record.Binding)
	if err != nil {
		return out, err
	}
	args, err := databricksArguments(parameters)
	if err != nil {
		return out, err
	}
	stream, _, err := client.OpenRead(ctx, &query.Query{Query: statement, Args: args}, id, databricksObserver{observer: observer, tag: "cw-read:" + id}, databricksOptions(l))
	if err != nil {
		return out, cloudReadFailure(ctx, err)
	}
	defer func() { _ = stream.Close() }()
	out.RemoteState = "running"
	out.Result, err = collectCloudRows(ctx, stream, l, observer)
	if err != nil {
		return out, cloudReadFailure(ctx, err)
	}
	out.RemoteState = "stopped"
	return out, nil
}
func (s *Service) controlDatabricks(ctx context.Context, e identity.Envelope, control readexec.Control, record Record, c config.SourceConnection, cancel bool) (string, error) {
	actual, err := s.probeDatabricks(ctx, c, record.Source.ID, record.Source.Revision)
	if err != nil {
		return "unknown", err
	}
	if readexec.Hash(actual) != readexec.Hash(record.Binding) {
		return "unknown", readexec.ErrBinding
	}
	target, err := control.Target(e, actual)
	if err != nil || target.Databricks == nil {
		return "unknown", readexec.ErrBinding
	}
	client, _, err := s.databricksClient(c)
	if err != nil {
		return "unknown", err
	}
	id := bruindatabricks.ReadIdentity{Workspace: target.Databricks.Workspace, WarehouseID: target.Databricks.Warehouse, AttemptTag: target.Tag, StatementID: target.Databricks.StatementID}
	options := bruindatabricks.ReadOptions{CancelTimeout: time.Second}
	var state bruindatabricks.ReadState
	if cancel {
		state, err = client.CancelRead(ctx, id, options)
	} else {
		state, err = client.ReadStatus(ctx, id)
	}
	if err != nil {
		return "unknown", safe(err)
	}
	return cloudState(string(state)), nil
}
func databricksArguments(parameters []readexec.Parameter) ([]any, error) {
	out := make([]any, len(parameters))
	for i, p := range parameters {
		name := "p" + strconv.Itoa(i+1)
		switch p.Kind {
		case "null":
			out[i] = bruindatabricks.ReadParameter{Name: name, Type: "STRING", Value: nil}
		case "text":
			out[i] = bruindatabricks.ReadParameter{Name: name, Type: "STRING", Value: p.Value}
		case "boolean":
			out[i] = bruindatabricks.ReadParameter{Name: name, Type: "BOOLEAN", Value: p.Value == "true"}
		case "integer":
			v, err := strconv.ParseInt(p.Value, 10, 64)
			if err != nil {
				return nil, readexec.ErrBinding
			}
			out[i] = bruindatabricks.ReadParameter{Name: name, Type: "BIGINT", Value: v}
		case "number":
			v, ok := new(big.Rat).SetString(p.Value)
			if !ok {
				return nil, readexec.ErrBinding
			}
			out[i] = bruindatabricks.ReadParameter{Name: name, Type: "DECIMAL(38,18)", Value: v}
		default:
			return nil, readexec.ErrBinding
		}
	}
	return out, nil
}
func databricksCategory(t string) string {
	u := strings.ToUpper(t)
	switch {
	case u == "TINYINT" || u == "SMALLINT" || u == "INT" || u == "BIGINT":
		return "integer"
	case strings.HasPrefix(u, "DECIMAL("):
		return "decimal"
	case u == "FLOAT" || u == "DOUBLE":
		return "number"
	case u == "BOOLEAN":
		return "boolean"
	case u == "DATE" || u == "TIMESTAMP" || u == "TIMESTAMP_NTZ":
		return "temporal"
	case u == "STRING":
		return "text"
	}
	return ""
}
