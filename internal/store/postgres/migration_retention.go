package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

type importedParent struct {
	kind, id string
	revision int64
}

func appliedParents(ctx context.Context, tx pgx.Tx, tenant, batch string) ([]importedParent, error) {
	rows, err := tx.Query(ctx, `SELECT kind,destination_result FROM chartworks.migration_checkpoints WHERE tenant_id=$1 AND batch_id=$2 AND action='install_private' AND kind IN('block','report','dashboard') ORDER BY sequence`, tenant, batch)
	if err != nil {
		return nil, err
	}
	parents := []importedParent{}
	for rows.Next() {
		var kind, result string
		if err = rows.Scan(&kind, &result); err != nil {
			rows.Close()
			return nil, err
		}
		if strings.HasPrefix(result, "quarantine:") {
			continue // the owner installed no readable domain revision
		}
		parent, ok := appliedParent(kind, result)
		if !ok {
			rows.Close()
			return nil, store.ErrConflict
		}
		parents = append(parents, parent)
	}
	err = rows.Err()
	rows.Close()
	return parents, err
}

// appliedParent accepts only the destination revision actually committed by an
// owner adapter. A manifest mapping or caller-provided ID is never sufficient.
func appliedParent(kind, result string) (importedParent, bool) {
	if !strings.HasSuffix(result, ":applied") {
		return importedParent{}, false
	}
	body := strings.TrimSuffix(result, ":applied")
	index := strings.LastIndex(body, ":draft:")
	if index < 1 {
		return importedParent{}, false
	}
	id := body[:index]
	revision, err := strconv.ParseInt(body[index+len(":draft:"):], 10, 64)
	if err != nil || revision < 1 || !identity.Identifier(id) {
		return importedParent{}, false
	}
	return importedParent{kind: kind, id: id, revision: revision}, true
}

// DrillRetained previews or erases one bounded page of due outputs and their
// renditions. It holds the immutable migration batch's row lock through the
// owner erasure calls, so migration-record erase/cutover cannot cross the drill.
func (d *DB) DrillRetained(ctx context.Context, e identity.Envelope, in migration.RetentionDrillRequest) (out migration.RetentionDrillResult, err error) {
	out = migration.RetentionDrillResult{Batch: in.Batch, Items: []migration.RetentionDrillItem{}, BackupScope: "online retained values only; backup, replica and WAL expiry needs operator evidence"}
	if !identity.Identifier(in.Batch) || in.Expected < 1 || in.Limit < 1 || in.Limit > 100 {
		return migration.RetentionDrillResult{}, migration.ErrInvalid
	}
	if err := access.Require(e, "migration.erase", access.Tenant(e, "erase")); err != nil {
		return migration.RetentionDrillResult{}, err
	}
	if err := access.Require(e, "reporting.retention", access.Tenant(e, "erase")); err != nil {
		return migration.RetentionDrillResult{}, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return migration.RetentionDrillResult{}, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var raw []byte
		var state string
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT manifest,manifest_digest,state,revision FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, e.Tenant(), in.Batch).Scan(&raw, &out.Digest, &state, &revision); err != nil {
			return err
		}
		if state != "complete" || revision != in.Expected {
			return store.ErrConflict
		}
		var manifest migration.Manifest
		if json.Unmarshal(raw, &manifest) != nil {
			return store.ErrConflict
		}
		for _, object := range manifest.Objects {
			if object.Retention.LegalHold {
				return store.ErrConflict
			}
		}
		var active bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.migration_cutovers WHERE tenant_id=$1 AND batch_id=$2 AND state='active')`, e.Tenant(), in.Batch).Scan(&active); err != nil {
			return err
		}
		if active {
			return store.ErrConflict
		}
		parents, err := appliedParents(ctx, tx, e.Tenant(), in.Batch)
		if err != nil {
			return err
		}
		for _, parent := range parents {
			if err := access.Require(e, "reporting.read", access.Resource{Tenant: e.Tenant(), Kind: parent.kind, Permission: "read", ID: parent.id}); err != nil {
				return err
			}
		}
		items, err := dueImportedRuns(ctx, tx, e.Tenant(), parents, in.Limit)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := requireDrillReach(ctx, tx, e, item); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.render_renditions WHERE tenant_id=$1 AND run_id=$2`, e.Tenant(), item.Run).Scan(&item.Renditions); err != nil {
				return err
			}
			out.Items = append(out.Items, item.RetentionDrillItem)
		}
		if err := tx.QueryRow(ctx, dueImportedCountSQL, append([]any{e.Tenant()}, parentArgs(parents)...)...).Scan(&out.Remaining); err != nil {
			return err
		}
		preview, err := json.Marshal(struct {
			Batch, Digest string
			Items         []migration.RetentionDrillItem
			Remaining     int64
		}{in.Batch, out.Digest, out.Items, out.Remaining})
		if err != nil {
			return store.ErrInvalid
		}
		sum := sha256.Sum256(preview)
		out.PreviewDigest = hex.EncodeToString(sum[:])
		if !in.Apply {
			return nil
		}
		if in.PreviewDigest != out.PreviewDigest {
			return store.ErrConflict
		}
		for _, item := range items {
			if item.Kind == "block" {
				if err := frozenQuotaLock(ctx, tx, e.Tenant()); err != nil {
					return err
				}
				if err := lockDueFrozen(ctx, tx, e.Tenant(), item.Run, item.Parent, item.Revision); err != nil {
					return err
				}
				scope, err := store.NewScope(e.Tenant(), e.User())
				if err != nil {
					return err
				}
				if err := expireFrozenRunTx(ctx, tx, scope, item.Run); err != nil {
					return err
				}
			} else {
				if err := lockDueComposition(ctx, tx, e.Tenant(), item); err != nil {
					return err
				}
				if err := expireCompositionRunTx(ctx, tx, e, item.Run); err != nil {
					return err
				}
			}
		}
		out.Applied = in.Apply
		return tx.QueryRow(ctx, dueImportedCountSQL, append([]any{e.Tenant()}, parentArgs(parents)...)...).Scan(&out.Remaining)
	})
	if err != nil {
		return migration.RetentionDrillResult{}, mapMigration(err)
	}
	return out, nil
}

func liveImportedRuns(ctx context.Context, tx pgx.Tx, tenant string, parents []importedParent) (bool, error) {
	var live bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.frozen_runs f JOIN unnest($2::text[],$3::bigint[]) p(id,revision) ON f.block_id=p.id AND f.revision=p.revision WHERE f.tenant_id=$1 AND f.state<>'expired')
 OR EXISTS(SELECT 1 FROM chartworks.composition_runs c JOIN unnest($4::text[],$5::text[],$6::bigint[]) p(kind,id,revision) ON c.kind=p.kind AND c.document_id=p.id AND c.revision=p.revision WHERE c.tenant_id=$1 AND c.state<>'expired')`, append([]any{tenant}, parentArgs(parents)...)...).Scan(&live)
	return live, err
}

const dueImportedCountSQL = `SELECT
 (SELECT count(*) FROM chartworks.frozen_runs f JOIN unnest($2::text[],$3::bigint[]) p(id,revision) ON f.block_id=p.id AND f.revision=p.revision WHERE f.tenant_id=$1 AND f.state<>'expired' AND f.payload_expires_at<=clock_timestamp())
 +(SELECT count(*) FROM chartworks.composition_runs c JOIN unnest($4::text[],$5::text[],$6::bigint[]) p(kind,id,revision) ON c.kind=p.kind AND c.document_id=p.id AND c.revision=p.revision WHERE c.tenant_id=$1 AND c.state<>'expired' AND c.expires_at<=clock_timestamp())`

type dueRun struct {
	migration.RetentionDrillItem
	context string
	private bool
}

func dueImportedRuns(ctx context.Context, tx pgx.Tx, tenant string, parents []importedParent, limit int) ([]dueRun, error) {
	blockIDs, blockRevs, documentKinds, documentIDs, documentRevs := parentArrays(parents)
	rows, err := tx.Query(ctx, `SELECT 'block',f.operation_id,f.block_id,f.revision,f.context_id,f.private FROM chartworks.frozen_runs f JOIN unnest($2::text[],$3::bigint[]) p(id,revision) ON f.block_id=p.id AND f.revision=p.revision WHERE f.tenant_id=$1 AND f.state<>'expired' AND f.payload_expires_at<=clock_timestamp()
 UNION ALL SELECT c.kind,c.operation_id,c.document_id,c.revision,'',c.private FROM chartworks.composition_runs c JOIN unnest($4::text[],$5::text[],$6::bigint[]) p(kind,id,revision) ON c.kind=p.kind AND c.document_id=p.id AND c.revision=p.revision WHERE c.tenant_id=$1 AND c.state<>'expired' AND c.expires_at<=clock_timestamp()
 ORDER BY 1,2 LIMIT $7`, tenant, blockIDs, blockRevs, documentKinds, documentIDs, documentRevs, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dueRun{}
	for rows.Next() {
		var item dueRun
		if err = rows.Scan(&item.Kind, &item.Run, &item.Parent, &item.Revision, &item.context, &item.private); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func parentArrays(parents []importedParent) (blockIDs []string, blockRevs []int64, documentKinds []string, documentIDs []string, documentRevs []int64) {
	blockIDs, blockRevs, documentKinds, documentIDs, documentRevs = []string{}, []int64{}, []string{}, []string{}, []int64{}
	for _, p := range parents {
		if p.kind == "block" {
			blockIDs, blockRevs = append(blockIDs, p.id), append(blockRevs, p.revision)
		} else {
			documentKinds, documentIDs, documentRevs = append(documentKinds, p.kind), append(documentIDs, p.id), append(documentRevs, p.revision)
		}
	}
	return
}

func parentArgs(parents []importedParent) []any {
	blockIDs, blockRevs, documentKinds, documentIDs, documentRevs := parentArrays(parents)
	return []any{blockIDs, blockRevs, documentKinds, documentIDs, documentRevs}
}

func requireDrillReach(ctx context.Context, tx pgx.Tx, e identity.Envelope, item dueRun) error {
	resources := []access.Resource{{Tenant: e.Tenant(), Kind: item.Kind, Permission: "read", ID: item.Parent}}
	if item.Kind == "block" {
		resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: item.context})
	} else {
		rows, err := tx.Query(ctx, `SELECT kind,permission,resource_id FROM chartworks.composition_run_references WHERE tenant_id=$1 AND operation_id=$2 AND action='reporting.read'`, e.Tenant(), item.Run)
		if err != nil {
			return err
		}
		for rows.Next() {
			var r access.Resource
			if err = rows.Scan(&r.Kind, &r.Permission, &r.ID); err != nil {
				rows.Close()
				return err
			}
			r.Tenant = e.Tenant()
			resources = append(resources, r)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
	}
	if err := access.Require(e, "reporting.read", resources...); err != nil {
		return err
	}
	if item.private {
		return access.Require(e, "reporting.preview", access.Resource{Tenant: e.Tenant(), Kind: item.Kind, Permission: "preview", ID: item.Parent})
	}
	return nil
}

func lockDueFrozen(ctx context.Context, tx pgx.Tx, tenant, run, parent string, revision int64) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT operation_id FROM chartworks.frozen_runs WHERE tenant_id=$1 AND operation_id=$2 AND block_id=$3 AND revision=$4 AND state<>'expired' AND payload_expires_at<=clock_timestamp() FOR UPDATE`, tenant, run, parent, revision).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrConflict
	}
	return err
}

func lockDueComposition(ctx context.Context, tx pgx.Tx, tenant string, item dueRun) error {
	var id string
	err := tx.QueryRow(ctx, `SELECT operation_id FROM chartworks.composition_runs WHERE tenant_id=$1 AND operation_id=$2 AND kind=$3 AND document_id=$4 AND revision=$5 AND state<>'expired' AND expires_at<=clock_timestamp() FOR UPDATE`, tenant, item.Run, item.Kind, item.Parent, item.Revision).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrConflict
	}
	return err
}
