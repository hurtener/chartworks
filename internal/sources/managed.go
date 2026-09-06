package sources

import (
	"context"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ManagedLocation deterministically addresses one owned tenant namespace and one
// immutable upload table. No caller-supplied SQL path or filename is accepted.
func ManagedLocation(c config.SourceConnection, id string) (string, string, error) {
	if c.ManagedSchema == "" || len(c.ManagedSchema) > 30 || !strings.HasPrefix(c.ManagedSchema, "cw_") || !readexec.SQLIdentifier(c.ManagedSchema) || !identity.Identifier(c.Tenant) || !identity.Identifier(c.ID) || !identity.Identifier(id) || len(id) > 80 {
		return "", "", store.ErrInvalid
	}
	return c.ManagedSchema + "_" + readexec.Hash([]string{c.Tenant, c.ID})[:16], "cw_u_" + readexec.Hash([]string{c.Tenant, id})[:32], nil
}

func (s *Service) recordConnection(record Record) (config.SourceConnection, error) {
	c, err := s.connection(record.Binding.Tenant, record.Connection)
	if err != nil {
		return c, err
	}
	if c.ManagedSchema == "" {
		return c, nil
	}
	schema, table, err := ManagedLocation(c, record.Source.ID)
	if err != nil {
		return c, err
	}
	if len(record.Binding.Relations) != 1 {
		return c, readexec.ErrBinding
	}
	r := record.Binding.Relations[0]
	if r.Schema != schema || r.Name != table || len(r.Columns) < 1 || len(r.Columns) > 256 {
		return c, readexec.ErrBinding
	}
	names := make([]string, len(r.Columns))
	for i, column := range r.Columns {
		if !readexec.SQLIdentifier(column.Name) {
			return c, readexec.ErrBinding
		}
		names[i] = column.Name
	}
	c.Relations = []config.SourceRelation{{Schema: schema, Name: table, Columns: names}}
	return c, nil
}

// DescribeManaged proves an already loaded managed table using exactly the same
// PostgreSQL read-only catalog/dependency proof as a warehouse source. It returns
// no authority and publishes nothing. The engineering transaction activates the
// source pointer only with its live operation fence and workspace receipt.
func (s *Service) DescribeManaged(ctx context.Context, e identity.Envelope, id, name, alias string, columns []string) (out Record, err error) {
	if err = access.Require(e, "sources.upload", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: id}, access.Tenant(e, "write")); err != nil {
		return out, err
	}
	c, err := s.connection(e.Tenant(), alias)
	if err != nil {
		return out, err
	}
	schema, table, err := ManagedLocation(c, id)
	if err != nil {
		return out, err
	}
	if len(columns) < 1 || len(columns) > 256 || len(name) < 1 || len(name) > 128 {
		return out, store.ErrInvalid
	}
	seen := map[string]bool{}
	for _, column := range columns {
		if !readexec.SQLIdentifier(column) || seen[column] {
			return out, store.ErrInvalid
		}
		seen[column] = true
	}
	c.Relations = []config.SourceRelation{{Schema: schema, Name: table, Columns: append([]string(nil), columns...)}}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		binding, err := s.probe(ctx, c, id, 1, nil)
		if err != nil {
			return err
		}
		out = Record{Source: Source{ID: id, Name: name, Dialect: "postgres", Revision: 1, ContextID: contextID(id, 1), Status: "registered"}, Connection: alias, Binding: binding}
		if !out.Valid() {
			return store.ErrInvalid
		}
		return nil
	})
	if err != nil {
		return Record{}, err
	}
	return out, nil
}
