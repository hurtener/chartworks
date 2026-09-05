package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ sources.Repository = (*DB)(nil)
const sourceColumns=`r.source_id,r.name,r.revision,r.context_id,r.connection_alias,r.binding`
func scanSource(row pgx.Row) (out sources.Record,err error) {
	var binding []byte
	err=row.Scan(&out.Source.ID,&out.Source.Name,&out.Source.Revision,&out.Source.ContextID,&out.Connection,&binding)
	if err!=nil { return out,err }
	out.Source.Dialect="postgres"
	out.Source.Status="registered"
	if json.Unmarshal(binding,&out.Binding)!=nil || !out.Valid() { return sources.Record{},store.ErrInvalid }
	return out,nil
}

// PutSource persists a revision and audit record in one transaction; rotation is CAS.
func (d *DB) PutSource(ctx context.Context,s store.Scope,expected int64,r sources.Record) error {
	if !s.Valid() || r.Binding.Tenant!=s.Tenant() { return store.ErrScope }
	if !r.Valid() || expected<0 || r.Source.Revision!=expected+1 { return store.ErrInvalid }
	return d.transaction(ctx,func(ctx context.Context,tx pgx.Tx) error {
		if expected>0 {
			tag,err:=tx.Exec(ctx,`UPDATE chartworks.sources SET current_revision=$4 WHERE tenant_id=$1 AND source_id=$2 AND current_revision=$3`,s.Tenant(),r.Source.ID,expected,r.Source.Revision)
			if err!=nil { return err }; if tag.RowsAffected()!=1 { return store.ErrConflict }
		}
		binding,err:=json.Marshal(r.Binding); if err!=nil { return store.ErrInvalid }
		_,err=tx.Exec(ctx,`INSERT INTO chartworks.source_revisions(tenant_id,source_id,revision,context_id,name,connection_alias,binding,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`,s.Tenant(),r.Source.ID,r.Source.Revision,r.Source.ContextID,r.Source.Name,r.Connection,binding,s.Actor())
		if err!=nil { return err }
		if expected==0 { if _,err=tx.Exec(ctx,`INSERT INTO chartworks.sources(tenant_id,source_id,current_revision) VALUES($1,$2,$3)`,s.Tenant(),r.Source.ID,r.Source.Revision); err!=nil { return err } }
		action:="source.created"; if expected>0 { action="source.rotated" }
		return auditJob(ctx,tx,s,action,r.Source.ID)
	})
}

// ReadSource returns internal technical metadata in the exact tenant partition.
func (d *DB) ReadSource(ctx context.Context,s store.Scope,id string) (out sources.Record,err error) {
	if !s.Valid() || !identity.Identifier(id) { return out,store.ErrScope }
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx) error {
		var e error
		out,e=scanSource(tx.QueryRow(ctx,`SELECT `+sourceColumns+` FROM chartworks.sources s JOIN chartworks.source_revisions r ON (r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND s.source_id=$2`,s.Tenant(),id))
		return e
	})
	if err!=nil { return sources.Record{},err }; return out,nil
}

// ListSources selects public columns only and applies current signed reach before LIMIT.
func (d *DB) ListSources(ctx context.Context,s store.Scope,selection access.Selection,limit int) (out []sources.Source,err error) {
	if !s.Valid() || selection.Tenant()!=s.Tenant() || !selection.All() && len(selection.IDs())==0 { return nil,store.ErrScope }
	if limit<1 || limit>100 { return nil,store.ErrInvalid }
	out=[]sources.Source{}
	err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx) error {
		rows,e:=tx.Query(ctx,`SELECT r.source_id,r.name,r.revision,r.context_id FROM chartworks.sources s JOIN chartworks.source_revisions r ON (r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND ($2 OR s.source_id=ANY($3::text[])) ORDER BY s.source_id LIMIT $4`,s.Tenant(),selection.All(),selection.IDs(),limit)
		if e!=nil { return e }; defer rows.Close()
		for rows.Next() { item:=sources.Source{Dialect:"postgres",Status:"registered"}; if e=rows.Scan(&item.ID,&item.Name,&item.Revision,&item.ContextID); e!=nil { return e }; out=append(out,item) }
		return rows.Err()
	})
	if err!=nil { return nil,err }; return out,nil
}

// WithSource holds the current-revision lock through a bounded driver callback.
// Every concurrent replica's CAS rotation must wait until that old-context work ends.
func (d *DB) WithSource(ctx context.Context,s store.Scope,id string,fn func(context.Context,sources.Record) error) error {
	if !s.Valid() || !identity.Identifier(id) || fn==nil { return store.ErrScope }
	return d.transaction(ctx,func(ctx context.Context,tx pgx.Tx) error {
		r,err:=scanSource(tx.QueryRow(ctx,`SELECT `+sourceColumns+` FROM chartworks.sources s JOIN chartworks.source_revisions r ON (r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND s.source_id=$2 FOR SHARE OF s`,s.Tenant(),id))
		if err!=nil { return err }
		return fn(ctx,r)
	})
}
