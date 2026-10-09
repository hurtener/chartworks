package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ reporting.DataDependencyRepository = (*DB)(nil)

// Select only dependency coordinates, never definitions, SQL, schema, values or
// execution journal payloads. The ordinary content/effect boundary is unchanged.
func (d *DB) DiscoverAuthoringDataDependencies(ctx context.Context, e identity.Envelope, in reporting.DataDependencyRequest) (out reporting.DataDependencyManifest, err error) {
	if err = reporting.RequireDataDependencyDiscovery(e, in); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	out = reporting.DataDependencyManifest{Version: "report-data-dependencies-v1", SourceDataset: in.SourceDataset, Topic: in.Topic, Dataset: in.Dataset, NewBlock: in.NewBlock, Operation: in.Operation, References: []reporting.ResourceReference{}, QueryReferences: []reporting.ResourceReference{}}
	err = d.transactionOptions(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead}, func(ctx context.Context, tx pgx.Tx) error {
		if in.NewBlock != "" {
			var raw, origin []byte
			var source, partition string
			// Retention moves consumed custody to its existing compact receipt.
			// Both branches bind tenant, actor, native session and exact target
			// before projecting coordinates. Neither branch resurrects an attempt.
			err := tx.QueryRow(ctx, `SELECT preparation_id,operation_id,COALESCE(topic,''),COALESCE(version,''),COALESCE(digest,''),dataset,source,partition,refs,origin FROM (
 SELECT preparation_id,operation_id,record#>>'{topics,0,topic}' topic,record#>>'{topics,0,version}' version,record#>>'{topics,0,digest}' digest,record#>>'{request,intent,dataset}' dataset,record#>>'{binding,source}' source,record#>>'{binding,context}' partition,record->'references' refs,record#>'{request,intent,source_dataset}' origin
 FROM chartworks.authoring_preparations WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND target_id=$4 AND (($5<>'' AND preparation_id=$5) OR ($6<>'' AND operation_id=$6))
 UNION ALL
 SELECT preparation_id,operation_id,record#>>'{topic,topic}',record#>>'{topic,version}',record#>>'{topic,digest}',record->>'dataset',record->>'source',record->>'context',record->'references',record->'source_dataset'
 FROM chartworks.authoring_preparation_consumed WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND target_id=$4 AND (($5<>'' AND preparation_id=$5) OR ($6<>'' AND operation_id=$6))
 ) custody LIMIT 1`, e.Tenant(), e.User(), e.Session(), in.NewBlock, in.Preparation, in.Operation).Scan(&out.Preparation, &out.Operation, &out.Topic.Topic, &out.Topic.Version, &out.Topic.Digest, &out.Dataset, &source, &partition, &raw, &origin)
			if err == nil {
				out.SourceDataset = nil
				if len(origin) > 0 && json.Unmarshal(origin, &out.SourceDataset) != nil {
					return store.ErrInvalid
				}
				if json.Unmarshal(raw, &out.References) != nil || len(out.References) < 1 || len(out.References) > 128 {
					return store.ErrInvalid
				}
				out.QueryReferences = dataQueryReferences(source, partition, out.Dataset)
				return nil
			}
			if !errors.Is(err, pgx.ErrNoRows) || in.Topic.Topic == "" && in.SourceDataset == nil {
				return err
			}
		}
		if pin := in.SourceDataset; pin != nil {
			var revision int64
			var relationJSON []byte
			err := tx.QueryRow(ctx, `SELECT r.revision,relation
 FROM chartworks.sources s JOIN chartworks.source_revisions r
 ON (r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision)
 CROSS JOIN LATERAL jsonb_array_elements(r.binding->'relations') relation
 WHERE s.tenant_id=$1 AND s.source_id=$2 AND NOT s.deleted AND r.context_id=$3
 AND relation->>'id'=$4`, e.Tenant(), pin.Source, pin.Context, pin.Dataset).Scan(&revision, &relationJSON)
			if err != nil {
				return err
			}
			var relation exec.Relation
			if json.Unmarshal(relationJSON, &relation) != nil {
				return store.ErrInvalid
			}
			if revision != pin.SourceRevision || exec.Hash(relation) != pin.SchemaDigest {
				return reporting.ErrStale
			}
			out.References = []reporting.ResourceReference{{Kind: "source", Permission: "read", ID: pin.Source}, {Kind: "dataset", Permission: "query", ID: pin.Dataset}, {Kind: "execution_context", Permission: "use", ID: pin.Context}}
			out.QueryReferences = dataQueryReferences(pin.Source, pin.Context, pin.Dataset)
			return nil
		}
		var archived, active bool
		err := tx.QueryRow(ctx, `SELECT v.version_id,v.digest,h.archived,h.active_version=v.version_id
 FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v USING(tenant_id,topic_id)
 WHERE h.tenant_id=$1 AND h.topic_id=$2 AND v.version_id=CASE WHEN $3='' THEN h.active_version ELSE $3 END`, e.Tenant(), in.Topic.Topic, in.Topic.Version).Scan(&out.Topic.Version, &out.Topic.Digest, &archived, &active)
		if err != nil {
			return err
		}
		if in.Topic.Version == "" && (archived || !active) {
			return store.ErrNotFound
		}
		if archived || !active || in.Topic.Digest != "" && in.Topic.Digest != out.Topic.Digest {
			return reporting.ErrStale
		}
		rows, err := tx.Query(ctx, `SELECT source_id,context_id,dataset_id FROM chartworks.topic_published_dependencies WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3 ORDER BY dataset_id LIMIT 129`, e.Tenant(), in.Topic.Topic, out.Topic.Version)
		if err != nil {
			return err
		}
		defer rows.Close()
		out.References = append(out.References, reporting.ResourceReference{Kind: "topic", Permission: "read", ID: in.Topic.Topic})
		count := 0
		for rows.Next() {
			var source, partition, dataset string
			if err := rows.Scan(&source, &partition, &dataset); err != nil {
				return err
			}
			count++
			out.References = append(out.References, reporting.ResourceReference{Kind: "source", Permission: "read", ID: source}, reporting.ResourceReference{Kind: "dataset", Permission: "query", ID: dataset}, reporting.ResourceReference{Kind: "execution_context", Permission: "use", ID: partition})
			if dataset == in.Dataset {
				out.QueryReferences = dataQueryReferences(source, partition, dataset)
			}
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if count == 0 || count > 128 || in.Dataset != "" && len(out.QueryReferences) == 0 {
			return store.ErrInvalid
		}
		return nil
	})
	if err != nil {
		return reporting.DataDependencyManifest{}, err
	}
	slices.SortFunc(out.References, func(a, b reporting.ResourceReference) int {
		return slices.Compare([]string{a.Kind, a.Permission, a.ID}, []string{b.Kind, b.Permission, b.ID})
	})
	out.References = slices.Compact(out.References)
	if len(out.References) > 128 {
		return reporting.DataDependencyManifest{}, store.ErrInvalid
	}
	if !e.Valid() {
		return reporting.DataDependencyManifest{}, access.ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return reporting.DataDependencyManifest{}, err
	}
	return out, nil
}

func dataQueryReferences(source, partition, dataset string) []reporting.ResourceReference {
	return []reporting.ResourceReference{{Kind: "source", Permission: "query", ID: source}, {Kind: "dataset", Permission: "query", ID: dataset}, {Kind: "execution_context", Permission: "use", ID: partition}}
}
