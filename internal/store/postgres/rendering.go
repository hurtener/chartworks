package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/rendering"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func (d *DB) PutRendition(ctx context.Context, r rendering.Record) (out rendering.Record, err error) {
	request, e := json.Marshal(r.Request)
	if e != nil {
		return out, store.ErrInvalid
	}
	body, e := json.Marshal(r.Rendition)
	if e != nil {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `INSERT INTO chartworks.render_renditions(tenant_id,rendition_id,run_id,output_id,actor_id,session_id,private,source_digest,renderer_version,theme_version,format,request,rendition,content_digest,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT DO NOTHING`, r.Tenant, r.Rendition.ID, r.Request.View.Run, r.Request.View.Output, r.Actor, r.Session, r.Private, r.Rendition.SourceDigest, r.Rendition.WorkerVersion, r.Rendition.ThemeVersion, r.Rendition.Format, request, body, r.Rendition.Digest, r.Rendition.CreatedAt, r.Rendition.ExpiresAt)
		if e != nil {
			return e
		}
		if tag.RowsAffected() == 0 {
			var b, req []byte
			e = tx.QueryRow(ctx, `SELECT rendition,request,actor_id,session_id,private FROM chartworks.render_renditions WHERE tenant_id=$1 AND rendition_id=$2`, r.Tenant, r.Rendition.ID).Scan(&b, &req, &out.Actor, &out.Session, &out.Private)
			if e != nil {
				return e
			}
			out.Tenant = r.Tenant
			if json.Unmarshal(b, &out.Rendition) != nil || json.Unmarshal(req, &out.Request) != nil {
				return store.ErrInvalid
			}
			return nil
		}
		out = r
		return nil
	})
	return out, safe(err)
}

func (d *DB) ReadRendition(ctx context.Context, tenant, id string) (out rendering.Record, err error) {
	var body, request []byte
	err = d.pool.QueryRow(ctx, `SELECT rendition,request,actor_id,session_id,private FROM chartworks.render_renditions WHERE tenant_id=$1 AND rendition_id=$2`, tenant, id).Scan(&body, &request, &out.Actor, &out.Session, &out.Private)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, access.ErrNotFound
	}
	if err != nil {
		return out, safe(err)
	}
	out.Tenant = tenant
	if json.Unmarshal(body, &out.Rendition) != nil || json.Unmarshal(request, &out.Request) != nil {
		return rendering.Record{}, store.ErrInvalid
	}
	return out, nil
}

func (d *DB) ListRenditions(ctx context.Context, tenant, after string, limit int) ([]rendering.Record, error) {
	rows, err := d.pool.Query(ctx, `SELECT rendition_id FROM chartworks.render_renditions WHERE tenant_id=$1 AND rendition_id>$2 ORDER BY rendition_id LIMIT $3`, tenant, after, limit)
	if err != nil {
		return nil, safe(err)
	}
	defer rows.Close()
	out := []rendering.Record{}
	for rows.Next() {
		var id string
		if rows.Scan(&id) != nil {
			return nil, store.ErrInvalid
		}
		r, e := d.ReadRendition(ctx, tenant, id)
		if e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, safe(rows.Err())
}

func (d *DB) ExpireRenditions(ctx context.Context, tenant string, as time.Time, limit int) (count int64, err error) {
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `DELETE FROM chartworks.render_renditions WHERE (tenant_id,rendition_id) IN (SELECT tenant_id,rendition_id FROM chartworks.render_renditions WHERE tenant_id=$1 AND expires_at<=$2 ORDER BY expires_at,rendition_id LIMIT $3 FOR UPDATE SKIP LOCKED)`, tenant, as, limit)
		if e == nil {
			count = tag.RowsAffected()
		}
		return e
	})
	return count, safe(err)
}

var _ rendering.Repository = (*DB)(nil)
