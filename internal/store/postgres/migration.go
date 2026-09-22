//nolint:revive // Exported DB methods implement the migration.Repository contract.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func (d *DB) Begin(ctx context.Context, e identity.Envelope, manifest migration.Manifest, digest string, plan migration.Plan) (out migration.Batch, existing bool, err error) {
	mraw, merr := json.Marshal(manifest)
	praw, perr := json.Marshal(plan)
	if merr != nil || perr != nil {
		return out, false, migration.ErrInvalid
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT cohort_id,manifest_digest,state,revision,applied,quarantined,total,next_ref,created_at,updated_at FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, e.Tenant(), manifest.Batch)
		if x := scanBatch(row, manifest.Batch, &out); x == nil {
			existing = true
			return nil
		} else if !errors.Is(x, pgx.ErrNoRows) {
			return x
		}
		keys, refs := make([]string, 0, len(plan.Objects)*2), make([]string, 0, len(plan.Objects)*2)
		for _, item := range plan.Objects {
			keys, refs = append(keys, string(item.Kind)), append(refs, item.ExternalRef)
			if item.Deletes != nil {
				keys, refs = append(keys, string(item.Deletes.Kind)), append(refs, item.Deletes.ExternalRef)
			}
		}
		rows, x := tx.Query(ctx, `SELECT kind,external_ref,source_revision,object_digest,tombstoned FROM chartworks.migration_external_refs WHERE tenant_id=$1 AND (kind,external_ref) IN (SELECT * FROM unnest($2::text[],$3::text[]))`, e.Tenant(), keys, refs)
		if x != nil {
			return x
		}
		type prior struct {
			revision   int64
			digest     string
			tombstoned bool
		}
		priorByRef := map[string]prior{}
		for rows.Next() {
			var kind, ref string
			var old prior
			if x = rows.Scan(&kind, &ref, &old.revision, &old.digest, &old.tombstoned); x != nil {
				rows.Close()
				return x
			}
			priorByRef[kind+"\x00"+ref] = old
		}
		rows.Close()
		if rows.Err() != nil {
			return rows.Err()
		}
		for _, item := range plan.Objects {
			if old, ok := priorByRef[string(item.Kind)+"\x00"+item.ExternalRef]; ok && (old.tombstoned || item.Revision < old.revision || item.Revision == old.revision && item.Digest != old.digest) {
				return store.ErrConflict
			}
			if item.Deletes != nil {
				old, ok := priorByRef[string(item.Deletes.Kind)+"\x00"+item.Deletes.ExternalRef]
				if ok && (item.Deletes.Revision < old.revision || old.tombstoned && (item.Deletes.Revision != old.revision || item.Digest != old.digest)) {
					return store.ErrConflict
				}
			}
		}
		now := time.Now().UTC()
		next := ""
		if len(plan.Objects) > 0 {
			next = plan.Objects[0].ExternalRef
		}
		_, x = tx.Exec(ctx, `INSERT INTO chartworks.migration_batches(tenant_id,batch_id,cohort_id,manifest_digest,manifest,plan,state,revision,total,next_ref,actor_id,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,'importing',1,$7,$8,$9,$10,$10)`, e.Tenant(), manifest.Batch, manifest.Cohort, digest, mraw, praw, len(plan.Objects), next, e.User(), now)
		if x != nil {
			return x
		}
		if x = migrationAudit(ctx, tx, e, "migration.started", manifest.Batch); x != nil {
			return x
		}
		out = migration.Batch{ID: manifest.Batch, Cohort: manifest.Cohort, Digest: digest, State: "importing", Revision: 1, Total: len(plan.Objects), Next: next, CreatedAt: now, UpdatedAt: now}
		return nil
	})
	if err != nil {
		return migration.Batch{}, false, mapMigration(err)
	}
	return out, existing, nil
}

type rowScanner interface{ Scan(...any) error }

func migrationAudit(ctx context.Context, tx pgx.Tx, e identity.Envelope, action, resource string) error {
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return err
	}
	return auditJob(ctx, tx, scope, action, resource)
}

func scanBatch(row rowScanner, id string, out *migration.Batch) error {
	out.ID = id
	return row.Scan(&out.Cohort, &out.Digest, &out.State, &out.Revision, &out.Applied, &out.Quarantined, &out.Total, &out.Next, &out.CreatedAt, &out.UpdatedAt)
}

func (d *DB) Batch(ctx context.Context, e identity.Envelope, id string) (out migration.Batch, manifest migration.Manifest, plan migration.Plan, err error) {
	var mraw, praw []byte
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT cohort_id,manifest_digest,state,revision,applied,quarantined,total,next_ref,created_at,updated_at,manifest,plan FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2`, e.Tenant(), id)
		out.ID = id
		return row.Scan(&out.Cohort, &out.Digest, &out.State, &out.Revision, &out.Applied, &out.Quarantined, &out.Total, &out.Next, &out.CreatedAt, &out.UpdatedAt, &mraw, &praw)
	})
	if err != nil {
		return out, manifest, plan, mapMigration(err)
	}
	if json.Unmarshal(mraw, &manifest) != nil || json.Unmarshal(praw, &plan) != nil {
		return out, manifest, plan, migration.ErrConflict
	}
	return
}

func (d *DB) Checkpoint(ctx context.Context, e identity.Envelope, id string, expected int64, item migration.ObjectPlan, result string) (out migration.Batch, err error) {
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		row := tx.QueryRow(ctx, `SELECT cohort_id,manifest_digest,state,revision,applied,quarantined,total,next_ref,created_at,updated_at FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, e.Tenant(), id)
		if x := scanBatch(row, id, &out); x != nil {
			return x
		}
		if out.Revision != expected || out.State != "importing" || out.Next != item.ExternalRef {
			return store.ErrConflict
		}
		seq := out.Applied + out.Quarantined
		if _, x := tx.Exec(ctx, `INSERT INTO chartworks.migration_checkpoints(tenant_id,batch_id,sequence,external_ref,kind,action,destination_result) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), id, seq, item.ExternalRef, string(item.Kind), item.Action, result); x != nil {
			return x
		}
		tombstoned := item.Action == "tombstone"
		command, x := tx.Exec(ctx, `INSERT INTO chartworks.migration_external_refs(tenant_id,kind,external_ref,manifest_digest,source_revision,object_digest,destination_result,tombstoned) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant_id,kind,external_ref) DO UPDATE SET manifest_digest=excluded.manifest_digest,source_revision=excluded.source_revision,object_digest=excluded.object_digest,destination_result=excluded.destination_result,tombstoned=excluded.tombstoned,updated_at=clock_timestamp() WHERE NOT chartworks.migration_external_refs.tombstoned AND (excluded.source_revision>chartworks.migration_external_refs.source_revision OR excluded.source_revision=chartworks.migration_external_refs.source_revision AND excluded.object_digest=chartworks.migration_external_refs.object_digest)`, e.Tenant(), string(item.Kind), item.ExternalRef, out.Digest, item.Revision, item.Digest, result, tombstoned)
		if x != nil {
			return x
		}
		if command.RowsAffected() != 1 {
			return store.ErrConflict
		}
		if item.Deletes != nil {
			command, x = tx.Exec(ctx, `INSERT INTO chartworks.migration_external_refs(tenant_id,kind,external_ref,manifest_digest,source_revision,object_digest,destination_result,tombstoned) VALUES($1,$2,$3,$4,$5,$6,$7,true) ON CONFLICT(tenant_id,kind,external_ref) DO UPDATE SET manifest_digest=excluded.manifest_digest,source_revision=excluded.source_revision,object_digest=excluded.object_digest,destination_result=excluded.destination_result,tombstoned=true,updated_at=clock_timestamp() WHERE (NOT chartworks.migration_external_refs.tombstoned AND excluded.source_revision>=chartworks.migration_external_refs.source_revision) OR (chartworks.migration_external_refs.tombstoned AND excluded.source_revision=chartworks.migration_external_refs.source_revision AND excluded.object_digest=chartworks.migration_external_refs.object_digest)`, e.Tenant(), string(item.Deletes.Kind), item.Deletes.ExternalRef, out.Digest, item.Deletes.Revision, item.Digest, result)
			if x != nil {
				return x
			}
			if command.RowsAffected() != 1 {
				return store.ErrConflict
			}
		}
		if item.Action == "install_private" || item.Action == "tombstone" {
			out.Applied++
		} else {
			out.Quarantined++
		}
		out.Revision++
		out.UpdatedAt = time.Now().UTC()
		out.Next = ""
		var praw []byte
		if x := tx.QueryRow(ctx, `SELECT plan FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2`, e.Tenant(), id).Scan(&praw); x != nil {
			return x
		}
		var plan migration.Plan
		if json.Unmarshal(praw, &plan) != nil {
			return store.ErrConflict
		}
		if seq+1 >= len(plan.Objects) {
			out.State = "complete"
		} else {
			out.Next = plan.Objects[seq+1].ExternalRef
		}
		if _, x = tx.Exec(ctx, `UPDATE chartworks.migration_batches SET state=$3,revision=$4,applied=$5,quarantined=$6,next_ref=$7,updated_at=$8,actor_id=$9 WHERE tenant_id=$1 AND batch_id=$2`, e.Tenant(), id, out.State, out.Revision, out.Applied, out.Quarantined, out.Next, out.UpdatedAt, e.User()); x != nil {
			return x
		}
		return migrationAudit(ctx, tx, e, "migration.checkpointed", item.ExternalRef)
	})
	if err != nil {
		return migration.Batch{}, mapMigration(err)
	}
	return out, nil
}

func (d *DB) Export(ctx context.Context, e identity.Envelope, id, after string, limit int) (migration.Export, error) {
	b, m, p, err := d.Batch(ctx, e, id)
	_ = p
	if err != nil {
		return migration.Export{}, err
	}
	start := 0
	if after != "" {
		found := false
		for i, o := range m.Objects {
			if o.ExternalRef == after {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return migration.Export{}, migration.ErrInvalid
		}
	}
	end := start + limit
	if end > len(m.Objects) {
		end = len(m.Objects)
	}
	m.Objects = append([]migration.Object(nil), m.Objects[start:end]...)
	return migration.Export{Manifest: m, Batch: b}, nil
}

func (d *DB) Cutover(ctx context.Context, e identity.Envelope, b migration.Batch, expected int64, route, previousRoute, operator string, boundary migration.OccurrenceBoundary) (out migration.Cutover, err error) {
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var old migration.Cutover
		var boundaryRaw, effectsRaw []byte
		x := tx.QueryRow(ctx, `SELECT batch_id,route,previous_route,state,generation,boundary,irreversible_effects,operator_reference,updated_at FROM chartworks.migration_cutovers WHERE tenant_id=$1 AND cohort_id=$2 FOR UPDATE`, e.Tenant(), b.Cohort).Scan(&old.Batch, &old.Route, &old.PreviousRoute, &old.State, &old.Generation, &boundaryRaw, &effectsRaw, &old.OperatorReference, &old.UpdatedAt)
		if x != nil && !errors.Is(x, pgx.ErrNoRows) {
			return x
		}
		if old.Batch == b.ID && old.Route == route && old.State == "active" {
			if (old.Generation != expected && old.Generation != expected+1) || old.OperatorReference != operator {
				return store.ErrConflict
			}
			out = old
			if json.Unmarshal(boundaryRaw, &out.Boundary) != nil || json.Unmarshal(effectsRaw, &out.IrreversibleEffects) != nil {
				return store.ErrConflict
			}
			if out.Boundary != boundary {
				return store.ErrConflict
			}
			out.Cohort = b.Cohort
			return nil
		}
		if old.Generation != expected {
			return store.ErrConflict
		}
		previous := old.Route
		if expected == 0 {
			previous = previousRoute
		}
		var targetRevision, previousRevision int64
		if x = tx.QueryRow(ctx, `SELECT revision FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR SHARE`, e.Tenant(), route).Scan(&targetRevision); x != nil {
			return store.ErrConflict
		}
		if x = tx.QueryRow(ctx, `SELECT revision FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2 FOR SHARE`, e.Tenant(), previous).Scan(&previousRevision); x != nil {
			return store.ErrConflict
		}
		out = migration.Cutover{Cohort: b.Cohort, Batch: b.ID, Route: route, PreviousRoute: previous, State: "active", Generation: expected + 1, Boundary: boundary, OperatorReference: operator, UpdatedAt: time.Now().UTC()}
		br, _ := json.Marshal(boundary)
		er := []byte(`[]`)
		record, _ := json.Marshal(out)
		_, x = tx.Exec(ctx, `INSERT INTO chartworks.migration_cutovers(tenant_id,cohort_id,batch_id,route,previous_route,state,generation,boundary,irreversible_effects,operator_reference,actor_id,updated_at) VALUES($1,$2,$3,$4,$5,'active',$6,$7,$8,$9,$10,$11) ON CONFLICT(tenant_id,cohort_id) DO UPDATE SET batch_id=excluded.batch_id,route=excluded.route,previous_route=excluded.previous_route,state=excluded.state,generation=excluded.generation,boundary=excluded.boundary,irreversible_effects=excluded.irreversible_effects,operator_reference=excluded.operator_reference,actor_id=excluded.actor_id,updated_at=excluded.updated_at`, e.Tenant(), b.Cohort, b.ID, route, previous, out.Generation, br, er, operator, e.User(), out.UpdatedAt)
		if x != nil {
			return x
		}
		if _, x = tx.Exec(ctx, `UPDATE chartworks.job_schedules SET revision=revision+1,enabled=(schedule_id=$3) WHERE tenant_id=$1 AND schedule_id=ANY($2::text[]) AND enabled IS DISTINCT FROM (schedule_id=$3)`, e.Tenant(), []string{route, previous}, route); x != nil {
			return x
		}
		if x = tx.QueryRow(ctx, `SELECT revision FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`, e.Tenant(), route).Scan(&targetRevision); x != nil {
			return x
		}
		if x = tx.QueryRow(ctx, `SELECT revision FROM chartworks.job_schedules WHERE tenant_id=$1 AND schedule_id=$2`, e.Tenant(), previous).Scan(&previousRevision); x != nil {
			return x
		}
		for _, binding := range []struct {
			route    string
			revision int64
		}{{route, targetRevision}, {previous, previousRevision}} {
			if _, x = tx.Exec(ctx, `INSERT INTO chartworks.migration_schedule_routes(tenant_id,cohort_id,stream_id,route,schedule_id,schedule_revision) VALUES($1,$2,$3,$4,$4,$5) ON CONFLICT(tenant_id,cohort_id,route) DO UPDATE SET stream_id=excluded.stream_id,schedule_id=excluded.schedule_id,schedule_revision=excluded.schedule_revision`, e.Tenant(), b.Cohort, boundary.Stream, binding.route, binding.revision); x != nil {
				return x
			}
		}
		if _, x = tx.Exec(ctx, `INSERT INTO chartworks.migration_cutover_events(tenant_id,cohort_id,generation,event,record) VALUES($1,$2,$3,'cutover',$4)`, e.Tenant(), b.Cohort, out.Generation, record); x != nil {
			return x
		}
		return migrationAudit(ctx, tx, e, "migration.cutover", b.Cohort)
	})
	if err != nil {
		return migration.Cutover{}, mapMigration(err)
	}
	return out, nil
}

func (d *DB) CurrentCutover(ctx context.Context, e identity.Envelope, cohort string) (out migration.Cutover, err error) {
	var braw, eraw []byte
	out.Cohort = cohort
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT batch_id,route,previous_route,state,generation,boundary,irreversible_effects,operator_reference,updated_at FROM chartworks.migration_cutovers WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), cohort).Scan(&out.Batch, &out.Route, &out.PreviousRoute, &out.State, &out.Generation, &braw, &eraw, &out.OperatorReference, &out.UpdatedAt)
	})
	if err != nil {
		return migration.Cutover{}, mapMigration(err)
	}
	if json.Unmarshal(braw, &out.Boundary) != nil || json.Unmarshal(eraw, &out.IrreversibleEffects) != nil {
		return migration.Cutover{}, migration.ErrConflict
	}
	return out, nil
}

func (d *DB) Rollback(ctx context.Context, e identity.Envelope, cohort string, expected int64, operator string, effects []string) (out migration.Cutover, err error) {
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var braw, oldEffects []byte
		out.Cohort = cohort
		if x := tx.QueryRow(ctx, `SELECT batch_id,route,previous_route,state,generation,boundary,irreversible_effects,operator_reference,updated_at FROM chartworks.migration_cutovers WHERE tenant_id=$1 AND cohort_id=$2 FOR UPDATE`, e.Tenant(), cohort).Scan(&out.Batch, &out.Route, &out.PreviousRoute, &out.State, &out.Generation, &braw, &oldEffects, &out.OperatorReference, &out.UpdatedAt); x != nil {
			return x
		}
		if json.Unmarshal(braw, &out.Boundary) != nil || json.Unmarshal(oldEffects, &out.IrreversibleEffects) != nil {
			return store.ErrConflict
		}
		if out.State == "rolled_back" {
			if (out.Generation != expected && out.Generation != expected+1) || out.OperatorReference != operator || !slices.Equal(out.IrreversibleEffects, effects) {
				return store.ErrConflict
			}
			return nil
		}
		if out.Generation != expected || out.State != "active" {
			return store.ErrConflict
		}
		out.Route, out.PreviousRoute = out.PreviousRoute, out.Route
		out.State = "rolled_back"
		out.Generation++
		out.OperatorReference = operator
		out.IrreversibleEffects = append([]string(nil), effects...)
		out.UpdatedAt = time.Now().UTC()
		if _, x := tx.Exec(ctx, `UPDATE chartworks.job_schedules SET revision=revision+1,enabled=(schedule_id=$3) WHERE tenant_id=$1 AND schedule_id=ANY($2::text[]) AND enabled IS DISTINCT FROM (schedule_id=$3)`, e.Tenant(), []string{out.Route, out.PreviousRoute}, out.Route); x != nil {
			return x
		}
		if _, x := tx.Exec(ctx, `UPDATE chartworks.migration_schedule_routes r SET schedule_revision=s.revision FROM chartworks.job_schedules s WHERE r.tenant_id=$1 AND r.cohort_id=$2 AND s.tenant_id=r.tenant_id AND s.schedule_id=r.schedule_id`, e.Tenant(), cohort); x != nil {
			return x
		}
		eraw, _ := json.Marshal(effects)
		record, _ := json.Marshal(out)
		if _, x := tx.Exec(ctx, `UPDATE chartworks.migration_cutovers SET route=$3,previous_route=$4,state='rolled_back',generation=$5,irreversible_effects=$6,operator_reference=$7,actor_id=$8,updated_at=$9 WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), cohort, out.Route, out.PreviousRoute, out.Generation, eraw, operator, e.User(), out.UpdatedAt); x != nil {
			return x
		}
		if _, x := tx.Exec(ctx, `INSERT INTO chartworks.migration_cutover_events(tenant_id,cohort_id,generation,event,record) VALUES($1,$2,$3,'rollback',$4)`, e.Tenant(), cohort, out.Generation, record); x != nil {
			return x
		}
		return migrationAudit(ctx, tx, e, "migration.rolled_back", cohort)
	})
	if err != nil {
		return migration.Cutover{}, mapMigration(err)
	}
	return out, nil
}

func (d *DB) Erase(ctx context.Context, e identity.Envelope, id string, limit int) (out migration.EraseResult, err error) {
	out = migration.EraseResult{Batch: id, BackupScope: "online records only; immutable backups expire by operator retention"}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var manifestRaw []byte
		if x := tx.QueryRow(ctx, `SELECT manifest FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2 FOR UPDATE`, e.Tenant(), id).Scan(&manifestRaw); x != nil {
			return x
		}
		var manifest migration.Manifest
		if json.Unmarshal(manifestRaw, &manifest) != nil {
			return store.ErrConflict
		}
		for _, object := range manifest.Objects {
			if object.Retention.LegalHold {
				return store.ErrConflict
			}
		}
		var active int
		if x := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.migration_cutovers WHERE tenant_id=$1 AND batch_id=$2 AND state='active'`, e.Tenant(), id).Scan(&active); x != nil {
			return x
		}
		if active > 0 {
			return store.ErrConflict
		}
		command, x := tx.Exec(ctx, `DELETE FROM chartworks.migration_checkpoints WHERE (tenant_id,batch_id,sequence) IN (SELECT tenant_id,batch_id,sequence FROM chartworks.migration_checkpoints WHERE tenant_id=$1 AND batch_id=$2 ORDER BY sequence LIMIT $3)`, e.Tenant(), id, limit)
		if x != nil {
			return x
		}
		out.Erased = command.RowsAffected()
		if x = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.migration_checkpoints WHERE tenant_id=$1 AND batch_id=$2`, e.Tenant(), id).Scan(&out.Remaining); x != nil {
			return x
		}
		if out.Remaining == 0 {
			if _, x = tx.Exec(ctx, `DELETE FROM chartworks.migration_cutovers WHERE tenant_id=$1 AND batch_id=$2 AND state='rolled_back'`, e.Tenant(), id); x != nil {
				return x
			}
			command, x = tx.Exec(ctx, `DELETE FROM chartworks.migration_batches WHERE tenant_id=$1 AND batch_id=$2`, e.Tenant(), id)
			if x != nil {
				return x
			}
			if command.RowsAffected() == 0 {
				return store.ErrNotFound
			}
		}
		return migrationAudit(ctx, tx, e, "migration.erased", id)
	})
	if err != nil {
		return migration.EraseResult{}, mapMigration(err)
	}
	return out, nil
}

func mapMigration(err error) error {
	if err == nil {
		return nil
	}
	for _, x := range []error{migration.ErrInvalid, migration.ErrLimit, migration.ErrConflict, migration.ErrNotFound, migration.ErrUnsupported, migration.ErrNotReady, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, x) {
			return x
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return migration.ErrNotFound
	}
	if errors.Is(err, store.ErrNotFound) {
		return migration.ErrNotFound
	}
	if errors.Is(err, store.ErrConflict) {
		return migration.ErrConflict
	}
	return safe(err)
}
