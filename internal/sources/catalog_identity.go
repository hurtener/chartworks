package sources

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// CatalogIdentity is source-derived continuity evidence, not an authorization
// token or a promise about data values. Unsupported engines explicitly omit
// object identities: callers must require review rather than guess a rename.
type CatalogIdentity struct {
	Version   string             `json:"version"`
	Authority string             `json:"authority"`
	Relations []RelationIdentity `json:"relations"`
}

type RelationIdentity struct {
	Dataset string           `json:"dataset"`
	Object  string           `json:"object"`
	Columns []ColumnIdentity `json:"columns"`
}

type ColumnIdentity struct {
	Name   string `json:"name"`
	Object string `json:"object"`
}

type CatalogObservation struct {
	BindingDigest string          `json:"binding_digest"`
	Identity      CatalogIdentity `json:"identity"`
}

// ObserveCatalog reuses the approved read-only source probe and current metadata
// fence. It inspects native catalog coordinates only, never table rows. It
// rejects a catalog that no longer matches its registered immutable binding.
func (s *Service) ObserveCatalog(ctx context.Context, e identity.Envelope, id, partition string) (out CatalogObservation, err error) {
	if _, err = s.ContextBinding(ctx, e, id, partition); err != nil {
		return out, err
	}
	scope, err := sourceScope(e, "sources.read", "read", id)
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
			if record.Source.ContextID != partition {
				return readexec.ErrBinding
			}
			if err := actualContext(e, "sources.read", record); err != nil {
				return err
			}
			connection, err := s.recordConnection(record)
			if err != nil {
				return err
			}
			if record.Source.Dialect != "postgres" {
				binding, err := s.probe(ctx, connection, id, record.Source.Revision, nil)
				if err != nil {
					return err
				}
				if !observedBindingMatches(record.Binding, binding) {
					return readexec.ErrBinding
				}
				out.BindingDigest = readexec.Hash(record.Binding)
				return nil
			}
			binding, err := s.probePostgres(ctx, connection, id, record.Source.Revision, func(ctx context.Context, tx readTransaction, binding readexec.Binding) error {
				if !observedBindingMatches(record.Binding, binding) {
					return readexec.ErrBinding
				}
				proof, err := postgresCatalogIdentity(ctx, tx, connection, binding)
				if err != nil {
					return err
				}
				out = CatalogObservation{BindingDigest: readexec.Hash(record.Binding), Identity: proof}
				return nil
			})
			if err != nil {
				return err
			}
			if !observedBindingMatches(record.Binding, binding) {
				return readexec.ErrBinding
			}
			return nil
		})
	})
	if err != nil {
		return CatalogObservation{}, err
	}
	if !e.Valid() {
		return CatalogObservation{}, access.ErrUnauthenticated
	}
	return out, nil
}

// Authority includes the actual connected endpoint/database/role and the
// operator's connection generation. Object identity also binds table OID and
// column attnum, so a same-name replacement is not classified as an exact rename.
func postgresCatalogIdentity(ctx context.Context, tx readTransaction, c config.SourceConnection, binding readexec.Binding) (CatalogIdentity, error) {
	var address, database, user string
	var port int
	var databaseOID, roleOID int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(inet_server_addr()::text,''),COALESCE(inet_server_port(),0),current_database(),d.oid::bigint,current_user,r.oid::bigint FROM pg_catalog.pg_database d CROSS JOIN pg_catalog.pg_roles r WHERE d.datname=current_database() AND r.rolname=current_user`).Scan(&address, &port, &database, &databaseOID, &user, &roleOID); err != nil {
		return CatalogIdentity{}, safe(err)
	}
	out := CatalogIdentity{Version: "postgres-catalog-identity-v1", Authority: readexec.Hash([]any{address, port, database, databaseOID, user, roleOID, c.Version}), Relations: []RelationIdentity{}}
	for _, relation := range binding.Relations {
		item := RelationIdentity{Dataset: relation.ID, Columns: []ColumnIdentity{}}
		var oid int64
		if err := tx.QueryRow(ctx, `SELECT c.oid::bigint FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, relation.Schema, relation.Name).Scan(&oid); err != nil {
			return CatalogIdentity{}, safe(err)
		}
		item.Object = readexec.Hash([]any{out.Authority, oid})
		names := make([]string, len(relation.Columns))
		for i, column := range relation.Columns {
			names[i] = column.Name
		}
		rows, err := tx.Query(ctx, `SELECT a.attname,a.attnum::integer FROM pg_catalog.pg_attribute a WHERE a.attrelid=$1 AND a.attnum>0 AND NOT a.attisdropped AND a.attname=ANY($2::text[]) ORDER BY a.attname LIMIT 257`, oid, names)
		if err != nil {
			return CatalogIdentity{}, safe(err)
		}
		for rows.Next() {
			var name string
			var number int
			if err := rows.Scan(&name, &number); err != nil {
				rows.Close()
				return CatalogIdentity{}, safe(err)
			}
			item.Columns = append(item.Columns, ColumnIdentity{Name: name, Object: readexec.Hash([]any{item.Object, number})})
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return CatalogIdentity{}, safe(err)
		}
		if len(item.Columns) != len(relation.Columns) {
			return CatalogIdentity{}, readexec.ErrBinding
		}
		out.Relations = append(out.Relations, item)
	}
	sort.Slice(out.Relations, func(i, j int) bool { return out.Relations[i].Dataset < out.Relations[j].Dataset })
	return out, nil
}
