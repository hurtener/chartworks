package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/bruin-data/bruin/pkg/query"
	bruinsnowflake "github.com/bruin-data/bruin/pkg/snowflake"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type snowflakePoolEntry struct {
	material string
	client   *bruinsnowflake.DB
}
type snowflakeObserver struct {
	observer readexec.Observer
	tag      string
}

func (o snowflakeObserver) OnDispatch(ctx context.Context, id bruinsnowflake.ReadIdentity) error {
	return o.dispatch(ctx, id, false)
}
func (o snowflakeObserver) OnAcknowledged(ctx context.Context, id bruinsnowflake.ReadIdentity) error {
	return o.dispatch(ctx, id, true)
}
func (o snowflakeObserver) dispatch(ctx context.Context, id bruinsnowflake.ReadIdentity, accepted bool) error {
	if o.observer == nil {
		return nil
	}
	return o.observer.Dispatch(ctx, readexec.RemoteQuery{Driver: "snowflake", Tag: o.tag, Snowflake: &readexec.SnowflakeRemoteQuery{RequestID: id.RequestID, QueryID: id.QueryID, QueryTag: id.QueryTag, Account: id.Account, Database: id.Database, SessionID: id.SessionID}}, accepted)
}

func (s *Service) snowflakeClient(c config.SourceConnection) (*bruinsnowflake.DB, *bruinsnowflake.Config, error) {
	raw, ok := s.lookup(strings.TrimPrefix(c.ReadDSN, "env:"))
	if !ok || len(raw) == 0 || len(raw) > 1<<20 {
		return nil, nil, store.ErrUnavailable
	}
	var native bruinsnowflake.Config
	if json.Unmarshal([]byte(raw), &native) != nil || !native.IsValid() || native.Database == "" || native.Schema == "" {
		return nil, nil, store.ErrInvalid
	}
	key := c.Tenant + "/" + c.ID
	material := readexec.Hash([]any{raw, c.Version})
	s.mu.Lock()
	defer s.mu.Unlock()
	if entry, ok := s.snowflakePools[key]; ok && entry.material == material {
		return entry.client, &native, nil
	}
	client, err := bruinsnowflake.NewDB(&native)
	if err != nil {
		return nil, nil, safe(err)
	}
	old := s.snowflakePools[key]
	s.snowflakePools[key] = snowflakePoolEntry{material: material, client: client}
	if old.client != nil {
		_ = old.client.Close()
	}
	return client, &native, nil
}
func (s *Service) closeSnowflake() {
	for key, entry := range s.snowflakePools {
		_ = entry.client.Close()
		delete(s.snowflakePools, key)
	}
}

func (s *Service) probeSnowflake(ctx context.Context, c config.SourceConnection, id string, revision int64) (readexec.Binding, error) {
	client, native, err := s.snowflakeClient(c)
	if err != nil {
		return readexec.Binding{}, err
	}
	out := readexec.Binding{Tenant: c.Tenant, Source: id, Context: contextID(id, revision), Revision: revision, Dialect: "snowflake"}
	evidence := []any{native.Account, native.Database, native.Schema, native.Role, native.Warehouse, c.Version}
	for _, relation := range c.Relations {
		tableSQL := "SELECT table_type FROM " + snowflakeName(native.Database) + ".information_schema.tables WHERE table_schema=? AND table_name=?"
		tableStream, _, tableErr := client.OpenRead(ctx, &query.Query{Query: tableSQL, Args: []any{relation.Schema, relation.Name}}, "probe-table:"+readexec.Hash([]any{id, revision, relation.Schema, relation.Name})[:24], snowflakeObserver{})
		if tableErr != nil {
			return readexec.Binding{}, safe(tableErr)
		}
		tableType, tableErr := catalogTableType(tableStream, map[string]bool{"BASE TABLE": true})
		if tableErr != nil {
			return readexec.Binding{}, tableErr
		}
		statement := "SELECT column_name,data_type,is_nullable,ordinal_position FROM " + snowflakeName(native.Database) + ".information_schema.columns WHERE table_schema=? AND table_name=? ORDER BY ordinal_position"
		stream, _, openErr := client.OpenRead(ctx, &query.Query{Query: statement, Args: []any{relation.Schema, relation.Name}}, "probe:"+readexec.Hash([]any{id, revision, relation.Schema, relation.Name})[:24], snowflakeObserver{})
		if openErr != nil {
			return readexec.Binding{}, safe(openErr)
		}
		columns, nativeEvidence, rowsErr := catalogColumns(stream, relation.Columns, snowflakeCategory)
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
func snowflakeName(value string) string { return `"` + strings.ReplaceAll(value, `"`, `""`) + `"` }

func (s *Service) explainSnowflake(ctx context.Context, e identity.Envelope, candidate readexec.Candidate, c config.SourceConnection, expected readexec.Binding) error {
	client, _, err := s.snowflakeClient(c)
	if err != nil {
		return err
	}
	statement, parameters, err := candidate.SQL(e, expected)
	if err != nil {
		return err
	}
	args, err := snowflakeArguments(parameters)
	if err != nil {
		return err
	}
	stream, _, err := client.OpenRead(ctx, &query.Query{Query: "EXPLAIN USING JSON " + statement, Args: args}, "explain:"+readexec.Hash(statement)[:24], snowflakeObserver{})
	if err != nil {
		return safe(err)
	}
	defer stream.Close()
	if !stream.Next() {
		return readexec.ErrUnsafe
	}
	values, err := stream.Values()
	if err != nil || len(values) != 1 || len(fmt.Sprint(values[0])) > 1<<20 {
		return readexec.ErrUnsafe
	}
	return stream.Err()
}
func (s *Service) executeSnowflake(ctx context.Context, e identity.Envelope, p readexec.Plan, record Record, c config.SourceConnection, l readexec.Limits, id string, observer readexec.Observer) (readexec.NativeResult, error) {
	out := readexec.NativeResult{RemoteState: "not_issued"}
	client, _, err := s.snowflakeClient(c)
	if err != nil {
		return out, err
	}
	statement, parameters, err := p.SQL(e, record.Binding)
	if err != nil {
		return out, err
	}
	args, err := snowflakeArguments(parameters)
	if err != nil {
		return out, err
	}
	stream, _, err := client.OpenRead(ctx, &query.Query{Query: statement, Args: args}, id, snowflakeObserver{observer: observer, tag: "cw-read:" + id})
	if err != nil {
		return out, cloudReadFailure(ctx, err)
	}
	defer stream.Close()
	out.RemoteState = "running"
	out.Result, err = collectCloudRows(ctx, stream, l, observer)
	if err != nil {
		return out, cloudReadFailure(ctx, err)
	}
	out.RemoteState = "stopped"
	return out, nil
}
func (s *Service) controlSnowflake(ctx context.Context, e identity.Envelope, control readexec.Control, record Record, c config.SourceConnection, cancel bool) (string, error) {
	actual, err := s.probeSnowflake(ctx, c, record.Source.ID, record.Source.Revision)
	if err != nil {
		return "unknown", err
	}
	if readexec.Hash(actual) != readexec.Hash(record.Binding) {
		return "unknown", readexec.ErrBinding
	}
	target, err := control.Target(e, actual)
	if err != nil || target.Snowflake == nil {
		return "unknown", readexec.ErrBinding
	}
	client, _, err := s.snowflakeClient(c)
	if err != nil {
		return "unknown", err
	}
	id := bruinsnowflake.ReadIdentity{RequestID: target.Snowflake.RequestID, QueryID: target.Snowflake.QueryID, QueryTag: target.Snowflake.QueryTag, Account: target.Snowflake.Account, Database: target.Snowflake.Database, SessionID: target.Snowflake.SessionID}
	if cancel {
		err = client.CancelRead(ctx, id)
		if err != nil {
			return "unknown", safe(err)
		}
	}
	state, err := client.ReadStatus(ctx, id)
	if err != nil {
		return "unknown", safe(err)
	}
	return cloudState(string(state)), nil
}
func snowflakeArguments(parameters []readexec.Parameter) ([]any, error) {
	out := make([]any, len(parameters))
	for i, p := range parameters {
		switch p.Kind {
		case "null":
			out[i] = nil
		case "text":
			out[i] = p.Value
		case "boolean":
			out[i] = p.Value == "true"
		case "integer":
			v, err := strconv.ParseInt(p.Value, 10, 64)
			if err != nil {
				return nil, readexec.ErrBinding
			}
			out[i] = v
		case "number":
			v, ok := new(big.Int).SetString(p.Value, 10)
			if !ok {
				return nil, readexec.ErrUnsupported
			}
			out[i] = v
		default:
			return nil, readexec.ErrBinding
		}
	}
	return out, nil
}
func snowflakeCategory(t string) string {
	u := strings.ToUpper(t)
	switch {
	case strings.HasPrefix(u, "NUMBER"), strings.HasPrefix(u, "DECIMAL"), strings.HasPrefix(u, "NUMERIC"):
		return "decimal"
	case u == "FLOAT" || u == "DOUBLE" || u == "REAL":
		return "number"
	case u == "BOOLEAN":
		return "boolean"
	case u == "BINARY" || strings.HasPrefix(u, "BINARY("):
		return "binary"
	case u == "DATE" || strings.HasPrefix(u, "TIME") || strings.HasPrefix(u, "TIMESTAMP"):
		return "temporal"
	case u == "VARIANT":
		return "structured"
	case u == "TEXT" || strings.HasPrefix(u, "VARCHAR") || strings.HasPrefix(u, "CHAR"):
		return "text"
	}
	return ""
}
