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

// WithManagedRecord proves the loaded table through the ordinary read-only
// source adapter and invokes publication while its ACCESS SHARE lock is held.
// expectedOID comes from the committed workspace receipt, never a public request.
// A same-name replacement cannot become the uploaded dataset between loading,
// discovery and metadata activation. The callback still owns its signed scope,
// cancellation and operation fence; the table lock is not execution authority.
// This is not a distributed transaction: the source transaction is read-only and
// a lost metadata response must still be resolved through the operation ledger.
func (s *Service) WithManagedRecord(ctx context.Context, e identity.Envelope, id, name, alias string, columns []string, expectedOID int64, publish func(context.Context, Record) error) error {
	if err := access.Require(e, "sources.upload", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: id}, access.Tenant(e, "write")); err != nil {
		return err
	}
	if expectedOID <= 0 || publish == nil {
		return store.ErrInvalid
	}
	c, err := s.connection(e.Tenant(), alias)
	if err != nil {
		return err
	}
	schema, table, err := ManagedLocation(c, id)
	if err != nil {
		return err
	}
	if len(columns) < 1 || len(columns) > 256 || len(name) < 1 || len(name) > 128 {
		return store.ErrInvalid
	}
	seen := map[string]bool{}
	for _, column := range columns {
		if !readexec.SQLIdentifier(column) || seen[column] {
			return store.ErrInvalid
		}
		seen[column] = true
	}
	c.Relations = []config.SourceRelation{{Schema: schema, Name: table, Columns: append([]string(nil), columns...)}}
	return s.call(ctx, e, true, func(ctx context.Context) error {
		var publishErr error
		_, err := s.probe(ctx, c, id, 1, func(ctx context.Context, tx readTransaction, binding readexec.Binding) error {
			var actualOID int64
			if err := tx.QueryRow(ctx, `SELECT c.oid::bigint FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, schema, table).Scan(&actualOID); err != nil {
				return err
			}
			if actualOID != expectedOID {
				return readexec.ErrBinding
			}
			record := Record{Source: Source{ID: id, Name: name, Dialect: "postgres", Revision: 1, ContextID: contextID(id, 1), Status: "registered"}, Connection: alias, Binding: binding}
			if !record.Valid() {
				return store.ErrInvalid
			}
			publishErr = publish(ctx, record)
			return publishErr
		})
		if publishErr != nil {
			return publishErr
		}
		return err
	})
}
