package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// DatabaseName permits a conservative cross-database workspace check without
// exposing metadata credentials or providing a customer-data write interface.
func (d *DB) DatabaseName() string { return d.pool.Config().ConnConfig.Database }

const uploadColumns = `u.tenant_id,u.actor_id,u.session_id,u.spec,u.spec_hash,u.state,u.created_at,u.expires_at,COALESCE(u.operation_id,''),u.receipt,COALESCE((SELECT s.current_revision FROM chartworks.sources s WHERE s.tenant_id=u.tenant_id AND s.source_id=u.source_id),0)`

func scanUpload(row pgx.Row) (out engineering.UploadRecord, err error) {
	var spec, receipt []byte
	err = row.Scan(&out.Tenant, &out.Actor, &out.Session, &spec, &out.SpecHash, &out.State, &out.Created, &out.Expires, &out.Operation, &receipt, &out.SourceRevision)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(spec, &out.Spec) != nil || !out.Valid() {
		return engineering.UploadRecord{}, store.ErrInvalid
	}
	if receipt != nil {
		out.Receipt = &engineering.WorkspaceReceipt{}
		if json.Unmarshal(receipt, out.Receipt) != nil || out.Receipt.SpecHash != out.SpecHash || out.Receipt.Checksum != out.Spec.SHA256 || out.Receipt.TableOID <= 0 || out.Receipt.Rows < 0 || out.Receipt.Rows > 1000000 || out.Receipt.DecodedBytes < 0 || out.Receipt.DecodedBytes > 256<<20 {
			return engineering.UploadRecord{}, store.ErrInvalid
		}
	}
	return out, nil
}
func uploadTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id, action, permission string, lock bool) (engineering.UploadRecord, error) {
	if err := access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: id}); err != nil {
		return engineering.UploadRecord{}, err
	}
	query := `SELECT ` + uploadColumns + ` FROM chartworks.uploads u WHERE u.tenant_id=$1 AND u.actor_id=$2 AND u.session_id=$3 AND u.source_id=$4`
	if lock {
		query += ` FOR UPDATE OF u`
	}
	out, err := scanUpload(tx.QueryRow(ctx, query, e.Tenant(), e.User(), e.Session(), id))
	if err != nil {
		return engineering.UploadRecord{}, err
	}
	if err = out.Require(e, action, permission); err != nil {
		return engineering.UploadRecord{}, err
	}
	return out, nil
}

// ReserveUpload pins the exact schema/checksum and pessimistic decoded-byte
// budget before accepting content, across all workspaces of the signed tenant.
func (d *DB) ReserveUpload(ctx context.Context, e identity.Envelope, spec engineering.UploadSpec, l config.Uploads) (out engineering.UploadRecord, err error) {
	if !spec.Valid(l) {
		return out, engineering.ErrInvalid
	}
	if err = access.Require(e, "sources.upload", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "write", ID: spec.ID}, access.Tenant(e, "write")); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061110))`, e.Tenant()); err != nil {
			return err
		}
		existing, err := scanUpload(tx.QueryRow(ctx, `SELECT `+uploadColumns+` FROM chartworks.uploads u WHERE u.tenant_id=$1 AND u.source_id=$2`, e.Tenant(), spec.ID))
		if err == nil {
			if err = existing.Require(e, "sources.upload", "write"); err != nil {
				return err
			}
			if existing.SpecHash != readexec.Hash(spec) || existing.State == "erased" {
				return store.ErrConflict
			}
			out = existing
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var taken bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2)`, e.Tenant(), spec.ID).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return store.ErrConflict
		}
		var count int
		var used int64
		if err = tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(accounted_bytes),0)::bigint FROM chartworks.uploads WHERE tenant_id=$1 AND state<>'erased'`, e.Tenant()).Scan(&count, &used); err != nil {
			return err
		}
		// Even uncompressed CSV can grow when values are normalized. Reserve
		// the configured decoded ceiling for every format, then release the
		// unused reservation atomically when actual decoded bytes are known.
		reserve := l.MaxExpandedBytes
		if count >= l.MaxPerTenant || used+reserve > l.MaxTenantBytes {
			return engineering.ErrLimit
		}
		var now time.Time
		if err = tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
			return err
		}
		out = engineering.UploadRecord{Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Spec: spec, SpecHash: readexec.Hash(spec), State: "awaiting_data", Created: now.UTC(), Expires: now.Add(time.Duration(l.StagingTTL)).UTC()}
		body, _ := json.Marshal(spec)
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.uploads(tenant_id,source_id,actor_id,session_id,spec,spec_hash,created_at,expires_at,accounted_bytes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), spec.ID, e.User(), e.Session(), body, out.SpecHash, out.Created, out.Expires, reserve); err != nil {
			return err
		}
		return auditJob(ctx, tx, scope, "upload.reserved", spec.ID)
	})
	if err != nil {
		return engineering.UploadRecord{}, err
	}
	return out, nil
}

// ReadUpload returns retained private metadata without contacting the workspace.
func (d *DB) ReadUpload(ctx context.Context, e identity.Envelope, id, action, permission string) (out engineering.UploadRecord, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e2 error
		out, e2 = uploadTx(ctx, tx, e, id, action, permission, false)
		return e2
	})
	if err != nil {
		return engineering.UploadRecord{}, err
	}
	return out, nil
}

// StageUpload holds the upload-state fence through bounded external staging and
// its metadata update. A concurrent erase cannot silently resurrect staged bytes.
func (d *DB) StageUpload(ctx context.Context, e identity.Envelope, id string, stage func(context.Context, engineering.UploadRecord) error) (out engineering.UploadRecord, err error) {
	if stage == nil {
		return out, engineering.ErrInvalid
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	deadline, ok := ctx.Deadline()
	if !ok {
		return out, engineering.ErrInvalid
	}
	duration := min(time.Until(deadline), 65*time.Second)
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transactionDuration(ctx, pgx.TxOptions{}, duration, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = uploadTx(ctx, tx, e, id, "sources.upload", "write", true)
		if err != nil {
			return err
		}
		if out.State != "awaiting_data" && out.State != "staged" {
			return engineering.ErrState
		}
		if time.Now().After(out.Expires) {
			return store.ErrExpired
		}
		if err = stage(ctx, out); err != nil {
			return err
		}
		if !e.Valid() {
			return access.ErrUnauthenticated
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.uploads SET state='staged' WHERE tenant_id=$1 AND source_id=$2`, e.Tenant(), id); err != nil {
			return err
		}
		out.State = "staged"
		return auditJob(ctx, tx, scope, "upload.staged", id)
	})
	if err != nil {
		return engineering.UploadRecord{}, err
	}
	return out, nil
}

// AttachUpload serializes competing logical operations for one upload. Erasure
// first tombstones the source under its revision lock and cancels an older load;
// actual workspace cleanup then runs as the newly admitted request operation.
func (d *DB) AttachUpload(ctx context.Context, e identity.Envelope, id string, task jobs.RequestTask, erase bool) (out engineering.UploadRecord, err error) {
	action, permission, kind := "sources.upload", "write", "upload.load"
	if erase {
		action, permission, kind = "sources.erase", "erase", "upload.erase"
	}
	if task.Input.Kind != kind || task.Input.Target != id || task.Require(e) != nil {
		return out, jobs.ErrAuthority
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = uploadTx(ctx, tx, e, id, action, permission, true)
		if err != nil {
			return err
		}
		if task.Input.InputHash != out.SpecHash {
			return store.ErrConflict
		}
		if out.SourceRevision > 0 && task.Input.Context != id+":v"+strconv.FormatInt(out.SourceRevision, 10) {
			return readexec.ErrBinding
		}
		actual, err := readRequestTx(ctx, tx, e, task.ID, true)
		if err != nil {
			return err
		}
		if actual.ManifestHash != task.ManifestHash || actual.State != "pending" && actual.State != "retry" && actual.State != "running" {
			return store.ErrConflict
		}
		if out.State == "erased" || !erase && out.State != "staged" {
			return engineering.ErrState
		}
		if out.Operation != "" && out.Operation != task.ID {
			var live bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND status='running' AND lease_until>clock_timestamp())`, e.Tenant(), out.Operation).Scan(&live); err != nil {
				return err
			}
			if live && !erase {
				return store.ErrConflict
			}
			if erase {
				if _, err = tx.Exec(ctx, `UPDATE chartworks.operations SET status='cancelled',error_code='cancelled',finished_at=clock_timestamp(),lease_owner=NULL,lease_until=NULL WHERE tenant_id=$1 AND operation_id=$2 AND status IN('pending','retry','running')`, e.Tenant(), out.Operation); err != nil {
					return err
				}
				if _, err = tx.Exec(ctx, `UPDATE chartworks.operation_attempts SET state='cancelled',error_code='cancelled',finished_at=clock_timestamp() WHERE tenant_id=$1 AND operation_id=$2 AND state='acquiring'`, e.Tenant(), out.Operation); err != nil {
					return err
				}
			}
		}
		if erase {
			if _, err = tx.Exec(ctx, `UPDATE chartworks.sources SET deleted=true WHERE tenant_id=$1 AND source_id=$2`, e.Tenant(), id); err != nil {
				return err
			}
			out.State = "deleting"
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.uploads SET operation_id=$3,state=$4 WHERE tenant_id=$1 AND source_id=$2`, e.Tenant(), id, task.ID, out.State); err != nil {
			return err
		}
		out.Operation = task.ID
		if erase {
			return auditJob(ctx, tx, scope, "upload.erasure_requested", id)
		}
		return nil
	})
	if err != nil {
		return engineering.UploadRecord{}, err
	}
	return out, nil
}

// ActivateUpload makes the upload/source pointer and operation completion atomic.
// A late cancellation, conflicting source reservation or stale task cannot publish.
func (d *DB) ActivateUpload(ctx context.Context, i jobs.Invocation, r engineering.UploadRecord, receipt engineering.WorkspaceReceipt, source sources.Record) error {
	e, err := i.Current("upload.load", r.Spec.ID, r.SpecHash)
	if err != nil {
		return err
	}
	if !source.Valid() || source.Source.ID != r.Spec.ID || source.Source.Name != r.Spec.Name || source.Source.Revision != 1 || source.Connection != r.Spec.Connection || source.Binding.Tenant != e.Tenant() || len(source.Binding.Relations) != 1 || receipt.SpecHash != r.SpecHash || receipt.Checksum != r.Spec.SHA256 || receipt.TableOID <= 0 || receipt.Rows < 0 || receipt.Rows > 1000000 || receipt.DecodedBytes < 0 || receipt.DecodedBytes > 256<<20 {
		return engineering.ErrOwnership
	}
	relation := source.Binding.Relations[0]
	if relation.Schema != receipt.Schema || relation.Name != receipt.Table || len(relation.Columns) != len(r.Spec.Columns) {
		return engineering.ErrOwnership
	}
	for j, col := range relation.Columns {
		if col.Name != r.Spec.Columns[j].Name {
			return engineering.ErrOwnership
		}
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := uploadTx(ctx, tx, e, r.Spec.ID, "sources.upload", "write", true)
		if err != nil {
			return err
		}
		if current.SpecHash != r.SpecHash || current.Operation != i.Lease().Task.ID || current.State != "staged" || time.Now().After(current.Expires) {
			return store.ErrConflict
		}
		if _, err = requestFenceTx(ctx, tx, i); err != nil {
			return err
		}
		if err = putSourceTx(ctx, tx, scope, 0, source, true); err != nil {
			return err
		}
		body, _ := json.Marshal(receipt)
		tag, err := tx.Exec(ctx, `UPDATE chartworks.uploads SET state='active',receipt=$3,accounted_bytes=$4 WHERE tenant_id=$1 AND source_id=$2 AND accounted_bytes>=$4`, e.Tenant(), r.Spec.ID, body, receipt.DecodedBytes)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return engineering.ErrLimit
		}
		if err = auditJob(ctx, tx, scope, "upload.activated", r.Spec.ID); err != nil {
			return err
		}
		return completeRequestTx(ctx, tx, i)
	})
}

// FinishUploadErasure seals only the same live erase operation after external
// ownership-checked removal. Derived values are erased in the same transaction;
// source metadata remains a non-queryable tombstone.
func (d *DB) FinishUploadErasure(ctx context.Context, i jobs.Invocation, r engineering.UploadRecord) error {
	e, err := i.Current("upload.erase", r.Spec.ID, r.SpecHash)
	if err != nil {
		return err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		current, err := uploadTx(ctx, tx, e, r.Spec.ID, "sources.erase", "erase", true)
		if err != nil {
			return err
		}
		if current.Operation != i.Lease().Task.ID || current.SpecHash != r.SpecHash || current.State != "deleting" {
			return store.ErrConflict
		}
		if _, err = requestFenceTx(ctx, tx, i); err != nil {
			return err
		}
		if err = eraseUploadProfiles(ctx, tx, e.Tenant(), r.Spec.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.uploads SET state='erased',receipt=NULL,accounted_bytes=0 WHERE tenant_id=$1 AND source_id=$2`, e.Tenant(), r.Spec.ID); err != nil {
			return err
		}
		if err = auditJob(ctx, tx, scope, "upload.erased", r.Spec.ID); err != nil {
			return err
		}
		return completeRequestTx(ctx, tx, i)
	})
}

// ExpiredUploads applies signed source reach BEFORE LIMIT and excludes active data.
func (d *DB) ExpiredUploads(ctx context.Context, e identity.Envelope, limit int) (out []engineering.UploadRecord, err error) {
	if limit < 1 || limit > 32 {
		return nil, engineering.ErrInvalid
	}
	if err = access.Require(e, "sources.erase", access.Tenant(e, "erase")); err != nil {
		return nil, err
	}
	selection, err := access.Constrain(e, "sources.erase", "source", "erase")
	if err != nil {
		return nil, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer stop()
	out = []engineering.UploadRecord{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+uploadColumns+` FROM chartworks.uploads u WHERE u.tenant_id=$1 AND u.actor_id=$2 AND u.session_id=$3 AND u.state IN('awaiting_data','staged','deleting') AND u.expires_at<=clock_timestamp() AND ($4 OR u.source_id=ANY($5::text[])) ORDER BY u.expires_at,u.source_id LIMIT $6`, e.Tenant(), e.User(), e.Session(), selection.All(), selection.IDs(), limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanUpload(rows)
			if err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
