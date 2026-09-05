package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/jackc/pgx/v5"
)

var _ vindex.Repository = (*DB)(nil)

func vectorCoordinates(s store.Scope, g vindex.Generation) error {
	if !s.Valid() {
		return store.ErrScope
	}
	if !g.Valid() {
		return store.ErrInvalid
	}
	return nil
}
func vectorHeadLock(ctx context.Context, tx pgx.Tx, tenant, topic, partition string) error {
	_, err := tx.Exec(ctx, `INSERT INTO chartworks.vector_heads(tenant_id,topic_id,context_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, tenant, topic, partition)
	if err != nil {
		return err
	}
	var revision int64
	return tx.QueryRow(ctx, `SELECT revision FROM chartworks.vector_heads WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 FOR UPDATE`, tenant, topic, partition).Scan(&revision)
}
func checkedGeneration(ctx context.Context, tx pgx.Tx, tenant string, g vindex.Generation) (string, error) {
	var hash, state string
	err := tx.QueryRow(ctx, `SELECT manifest_hash,state FROM chartworks.vector_generations WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND generation_id=$4 FOR UPDATE`, tenant, g.Topic, g.Context, g.ID).Scan(&hash, &state)
	if err != nil {
		return "", err
	}
	if hash != vindex.Digest(g) {
		return "", store.ErrConflict
	}
	return state, nil
}

// BeginGeneration accepts a bounded, immutable manifest within the supplied tenant.
func (d *DB) BeginGeneration(ctx context.Context, s store.Scope, g vindex.Generation) error {
	if err := vectorCoordinates(s, g); err != nil {
		return err
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := vectorHeadLock(ctx, tx, s.Tenant(), g.Topic, g.Context); err != nil {
			return err
		}
		descriptor, _ := json.Marshal(g.Space)
		manifest, _ := json.Marshal(g.Expected)
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.vector_generations(tenant_id,topic_id,context_id,generation_id,version_id,source_generation,descriptor,space_key,dimensions,manifest,manifest_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`, s.Tenant(), g.Topic, g.Context, g.ID, g.Version, g.SourceGeneration, descriptor, g.Space.Key(), g.Space.Dimensions, manifest, vindex.Digest(g))
		if err != nil {
			return err
		}
		_, err = checkedGeneration(ctx, tx, s.Tenant(), g)
		if err != nil {
			return err
		}
		return auditJob(ctx, tx, s, "facets.generation_staged", g.Topic)
	})
}

// UpsertFacets keeps batches invisible until their complete generation is published.
func (d *DB) UpsertFacets(ctx context.Context, s store.Scope, g vindex.Generation, batch []vindex.Facet) error {
	if err := vectorCoordinates(s, g); err != nil {
		return err
	}
	if err := vindex.CheckBatch(g, batch); err != nil {
		return err
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := vectorHeadLock(ctx, tx, s.Tenant(), g.Topic, g.Context); err != nil {
			return err
		}
		state, err := checkedGeneration(ctx, tx, s.Tenant(), g)
		if err != nil {
			return err
		}
		if state != "staging" {
			return store.ErrConflict
		}
		for _, f := range batch {
			vector, err := vindex.VectorLiteral(f.Vector, g.Space.Dimensions)
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.vector_facets(tenant_id,topic_id,context_id,generation_id,facet_id,kind,source_id,body,space_key,dimensions,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11::public.vector) ON CONFLICT(tenant_id,topic_id,context_id,generation_id,facet_id) DO UPDATE SET embedding=EXCLUDED.embedding`, s.Tenant(), g.Topic, g.Context, g.ID, f.ID, f.Kind, f.SourceID, f.Text, g.Space.Key(), g.Space.Dimensions, vector)
			if err != nil {
				return err
			}
		}
		return nil
	})
}

// PublishGeneration proves complete origin coverage and commits one publication CAS.
func (d *DB) PublishGeneration(ctx context.Context, s store.Scope, g vindex.Generation, expected int64) (out vindex.Publication, err error) {
	if err = vectorCoordinates(s, g); err != nil {
		return out, err
	}
	if expected < 0 || expected >= 1<<62 {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if e := vectorHeadLock(ctx, tx, s.Tenant(), g.Topic, g.Context); e != nil {
			return e
		}
		if _, e := checkedGeneration(ctx, tx, s.Tenant(), g); e != nil {
			return e
		}
		var count, missing int
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.vector_facets WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND generation_id=$4`, s.Tenant(), g.Topic, g.Context, g.ID).Scan(&count); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.vector_generations g CROSS JOIN LATERAL jsonb_to_recordset(g.manifest) AS m(id text,kind text,source_id text,text_hash text) LEFT JOIN chartworks.vector_facets f ON (f.tenant_id,f.topic_id,f.context_id,f.generation_id,f.facet_id)=(g.tenant_id,g.topic_id,g.context_id,g.generation_id,m.id) WHERE g.tenant_id=$1 AND g.topic_id=$2 AND g.context_id=$3 AND g.generation_id=$4 AND (f.facet_id IS NULL OR f.kind<>m.kind OR f.source_id<>m.source_id OR encode(sha256(convert_to(f.body,'UTF8')),'hex')<>m.text_hash)`, s.Tenant(), g.Topic, g.Context, g.ID).Scan(&missing); e != nil {
			return e
		}
		if count != len(g.Expected) || missing != 0 {
			return store.ErrConflict
		}
		if _, e := tx.Exec(ctx, `UPDATE chartworks.vector_generations SET state='ready' WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND generation_id=$4`, s.Tenant(), g.Topic, g.Context, g.ID); e != nil {
			return e
		}
		tag, e := tx.Exec(ctx, `UPDATE chartworks.vector_heads SET revision=revision+1,active_generation=$5,archived=false WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND revision=$4`, s.Tenant(), g.Topic, g.Context, expected, g.ID)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		out = vindex.Publication{Revision: expected + 1, Generation: g.ID, Version: g.Version}
		return auditJob(ctx, tx, s, "facets.generation_published", g.Topic)
	})
	if err != nil {
		return vindex.Publication{}, err
	}
	return out, nil
}

// ArchiveGeneration keeps retained immutable facets but immediately excludes the pointer.
func (d *DB) ArchiveGeneration(ctx context.Context, s store.Scope, topic, partition string, expected int64) (out vindex.Publication, err error) {
	if !s.Valid() {
		return out, store.ErrScope
	}
	if !identity.Identifier(topic) || !identity.Identifier(partition) || expected < 1 || expected >= 1<<62 {
		return out, store.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, e := tx.Exec(ctx, `UPDATE chartworks.vector_heads SET archived=true,revision=revision+1 WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND revision=$4`, s.Tenant(), topic, partition, expected)
		if e != nil {
			return e
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		out = vindex.Publication{Revision: expected + 1, Archived: true}
		return auditJob(ctx, tx, s, "facets.archived", topic)
	})
	if err != nil {
		return vindex.Publication{}, err
	}
	return out, nil
}

const facetSearchSQL = `SELECT facet_id,kind,source_id,body,distance FROM (
 SELECT facet_id,kind,source_id,body,embedding OPERATOR(public.<=>) $5::public.vector AS distance,
 row_number() OVER(PARTITION BY kind ORDER BY embedding OPERATOR(public.<=>) $5::public.vector,facet_id) AS ordinal
 FROM chartworks.vector_facets
 WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND generation_id=$4 AND kind=ANY($6::text[])
) ranked WHERE ordinal<=$7 ORDER BY distance,kind,facet_id`

func vectorPublication(ctx context.Context, tx pgx.Tx, tenant string, q vindex.Query) (p vindex.Publication, source string, err error) {
	var key, state string
	var dimensions int
	err = tx.QueryRow(ctx, `SELECT h.revision,h.archived,COALESCE(h.active_generation,''),COALESCE(g.version_id,''),COALESCE(g.space_key,''),COALESCE(g.dimensions,0),COALESCE(g.source_generation,''),COALESCE(g.state,'') FROM chartworks.vector_heads h LEFT JOIN chartworks.vector_generations g ON (g.tenant_id,g.topic_id,g.context_id,g.generation_id)=(h.tenant_id,h.topic_id,h.context_id,h.active_generation) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND h.context_id=$3`, tenant, q.Topic, q.Context).Scan(&p.Revision, &p.Archived, &p.Generation, &p.Version, &key, &dimensions, &source, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return vindex.Publication{}, "", nil
	}
	if err != nil {
		return p, "", err
	}
	if p.Archived || p.Generation == "" {
		return p, source, nil
	}
	if state != "ready" || key != q.Space.Key() || dimensions != q.Space.Dimensions {
		return vindex.Publication{}, "", store.ErrConflict
	}
	return p, source, nil
}

// SearchFacets applies all tenant/topic/context/generation restrictions inside SQL.
// Repeatable read makes an entire batch observe one immutable publication snapshot.
func (d *DB) SearchFacets(ctx context.Context, s store.Scope, queries []vindex.Query) (out []vindex.Result, err error) {
	if !s.Valid() {
		return nil, store.ErrScope
	}
	if len(queries) < 1 || len(queries) > 8 {
		return nil, store.ErrInvalid
	}
	for _, q := range queries {
		if !q.Valid() {
			return nil, store.ErrInvalid
		}
	}
	out = make([]vindex.Result, 0, len(queries))
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		bytes := 0
		for _, q := range queries {
			p, source, e := vectorPublication(ctx, tx, s.Tenant(), q)
			if e != nil {
				return e
			}
			r := vindex.Result{ID: q.ID, Publication: p, Hits: []vindex.Hit{}}
			if p.Archived || p.Generation == "" {
				out = append(out, r)
				continue
			}
			vector, e := vindex.VectorLiteral(q.Vector, q.Space.Dimensions)
			if e != nil {
				return e
			}
			rows, e := tx.Query(ctx, facetSearchSQL, s.Tenant(), q.Topic, q.Context, p.Generation, vector, q.Kinds, q.LimitPerKind)
			if e != nil {
				return e
			}
			for rows.Next() {
				h := vindex.Hit{Generation: p.Generation, Version: p.Version, SourceGeneration: source}
				if e = rows.Scan(&h.ID, &h.Kind, &h.SourceID, &h.Text, &h.Distance); e != nil {
					rows.Close()
					return e
				}
				bytes += len(h.Text) + len(h.ID) + len(h.SourceID) + len(h.Generation) + len(h.Version) + len(h.SourceGeneration) + 128
				if bytes > 2<<20 {
					rows.Close()
					return store.ErrInvalid
				}
				r.Hits = append(r.Hits, h)
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return e
			}
			out = append(out, r)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteFacets invalidates matching pointers before cascading exact generation cleanup.
func (d *DB) DeleteFacets(ctx context.Context, s store.Scope, r vindex.Removal) error {
	if !s.Valid() {
		return store.ErrScope
	}
	if r.Topic == "" && (r.Context != "" || r.Version != "") || r.Topic != "" && (!identity.Identifier(r.Topic) || !identity.Identifier(r.Context)) || r.Version != "" && !identity.Identifier(r.Version) {
		return store.ErrInvalid
	}
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// Lock all selected heads in canonical order; competing publishers use the same lock order.
		rows, err := tx.Query(ctx, `SELECT topic_id FROM chartworks.vector_heads WHERE tenant_id=$1 AND ($2='' OR topic_id=$2 AND context_id=$3) ORDER BY topic_id,context_id FOR UPDATE`, s.Tenant(), r.Topic, r.Context)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE chartworks.vector_heads h SET active_generation=NULL,archived=true,revision=revision+1 WHERE h.tenant_id=$1 AND ($2='' OR h.topic_id=$2 AND h.context_id=$3) AND ($4='' OR EXISTS(SELECT 1 FROM chartworks.vector_generations g WHERE (g.tenant_id,g.topic_id,g.context_id,g.generation_id)=(h.tenant_id,h.topic_id,h.context_id,h.active_generation) AND g.version_id=$4))`, s.Tenant(), r.Topic, r.Context, r.Version)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM chartworks.vector_generations WHERE tenant_id=$1 AND ($2='' OR topic_id=$2 AND context_id=$3) AND ($4='' OR version_id=$4)`, s.Tenant(), r.Topic, r.Context, r.Version)
		if err != nil {
			return err
		}
		target := r.Topic
		if target == "" {
			target = s.Tenant()
		}
		return auditJob(ctx, tx, s, "facets.erased", target)
	})
}

// ExplainFacets records the real PostgreSQL plan for the fixed bounded retrieval query.
func (d *DB) ExplainFacets(ctx context.Context, s store.Scope, q vindex.Query) (out json.RawMessage, err error) {
	if !s.Valid() {
		return nil, store.ErrScope
	}
	if !q.Valid() {
		return nil, store.ErrInvalid
	}
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		p, _, e := vectorPublication(ctx, tx, s.Tenant(), q)
		if e != nil {
			return e
		}
		if p.Archived || p.Generation == "" {
			return store.ErrNotFound
		}
		vector, e := vindex.VectorLiteral(q.Vector, q.Space.Dimensions)
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, "EXPLAIN (FORMAT JSON) "+facetSearchSQL, s.Tenant(), q.Topic, q.Context, p.Generation, vector, q.Kinds, q.LimitPerKind).Scan(&out)
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
