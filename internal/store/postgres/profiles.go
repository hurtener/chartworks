package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ engineering.ProfileRepository = (*DB)(nil)

const profileColumns = `p.tenant_id,p.actor_id,p.session_id,p.profile_id,p.source_id,p.context_id,p.dataset_id,p.manifest,p.manifest_hash,p.state,COALESCE(p.operation_id,''),p.created_at,p.result,p.summary_started,COALESCE(p.last_read_operation,''),p.last_read_deadline`

func scanProfile(row pgx.Row) (r engineering.ProfileRecord, err error) {
	var tenant, actor, session, id, source, partition, dataset, hash, state, operation, lastRead string
	var manifest, result []byte
	var created time.Time
	var lastDeadline *time.Time
	var summary bool
	err = row.Scan(&tenant, &actor, &session, &id, &source, &partition, &dataset, &manifest, &hash, &state, &operation, &created, &result, &summary, &lastRead, &lastDeadline)
	if err != nil {
		return r, err
	}
	if json.Unmarshal(manifest, &r) != nil {
		return r, store.ErrInvalid
	}
	if r.Tenant != tenant || r.Actor != actor || r.Session != session || r.Spec.ID != id || r.Spec.Source != source || r.Spec.Context != partition || r.Spec.Dataset != dataset || r.SpecHash != hash || !r.Valid() {
		return engineering.ProfileRecord{}, store.ErrInvalid
	}
	r.State = state
	r.Operation = operation
	r.Created = created
	r.SummaryStarted = summary
	r.LastReadOperation = lastRead
	r.LastReadDeadline = time.Time{}
	if lastDeadline != nil {
		r.LastReadDeadline = *lastDeadline
	}
	r.Result = nil
	if result != nil {
		r.Result = &engineering.Profile{}
		if json.Unmarshal(result, r.Result) != nil || !r.Result.Valid(r) {
			return engineering.ProfileRecord{}, store.ErrInvalid
		}
	}
	return r, nil
}
func profileTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, write, lock bool) (engineering.ProfileRecord, error) {
	if !e.Valid() || !identity.Identifier(id) {
		return engineering.ProfileRecord{}, access.ErrUnauthenticated
	}
	query := `SELECT ` + profileColumns + ` FROM chartworks.profile_versions p JOIN chartworks.sources s ON(s.tenant_id,s.source_id)=(p.tenant_id,p.source_id) WHERE p.tenant_id=$1 AND p.actor_id=$2 AND p.session_id=$3 AND p.profile_id=$4 AND p.state<>'erased' AND NOT s.deleted`
	if lock {
		query += ` FOR UPDATE OF p`
	}
	r, err := scanProfile(tx.QueryRow(ctx, query, e.Tenant(), e.User(), e.Session(), id))
	if err != nil {
		return r, err
	}
	if err = r.Require(e, write); err != nil {
		return engineering.ProfileRecord{}, err
	}
	return r, nil
}
func profileSourceTx(ctx context.Context, tx pgx.Tx, r engineering.ProfileRecord) error {
	current, err := scanSource(tx.QueryRow(ctx, `SELECT `+sourceColumns+` FROM chartworks.sources s JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND s.source_id=$2 AND NOT s.deleted FOR SHARE OF s`, r.Tenant, r.Spec.Source))
	if err != nil {
		return err
	}
	if readexec.Hash(current.Binding) != readexec.Hash(r.Binding) {
		return readexec.ErrBinding
	}
	return nil
}
func profileHeadLock(ctx context.Context, tx pgx.Tx, r engineering.ProfileRecord) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,7214061210))`, readexec.Hash([]string{r.Tenant, r.Actor, r.Session, r.Spec.Source, r.Spec.Dataset}))
	return err
}
func profileHead(ctx context.Context, tx pgx.Tx, r engineering.ProfileRecord) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `SELECT profile_id FROM chartworks.profile_heads WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND source_id=$4 AND dataset_id=$5`, r.Tenant, r.Actor, r.Session, r.Spec.Source, r.Spec.Dataset).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// ReserveProfile stores only resolved bounded metadata. It does not create an
// active pointer or claim sampling/model work has already happened.
func (d *DB) ReserveProfile(ctx context.Context, e identity.Envelope, r engineering.ProfileRecord) (out engineering.ProfileRecord, err error) {
	if !r.Valid() || r.Result != nil {
		return out, engineering.ErrInvalid
	}
	if err = r.Require(e, true); err != nil {
		return out, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := profileHeadLock(ctx, tx, r); err != nil {
			return err
		}
		previous, err := profileTx(ctx, tx, e, r.Spec.ID, true, false)
		if err == nil {
			if previous.SpecHash != r.SpecHash {
				return store.ErrConflict
			}
			out = previous
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err = profileSourceTx(ctx, tx, r); err != nil {
			return err
		}
		head, err := profileHead(ctx, tx, r)
		if err != nil {
			return err
		}
		if head != r.Spec.Previous {
			return store.ErrConflict
		}
		if head != "" {
			if _, err = profileTx(ctx, tx, e, head, false, false); err != nil {
				return err
			}
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.profile_versions WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND source_id=$4 AND dataset_id=$5 AND state<>'erased'`, r.Tenant, r.Actor, r.Session, r.Spec.Source, r.Spec.Dataset).Scan(&count); err != nil {
			return err
		}
		if count >= r.Settings.MaxVersions {
			return engineering.ErrLimit
		}
		r.State = "reserved"
		r.Operation = ""
		r.Result = nil
		r.SummaryStarted = false
		r.LastReadOperation = ""
		r.LastReadDeadline = time.Time{}
		raw, _ := json.Marshal(r)
		if len(raw) > 128<<10 {
			return engineering.ErrLimit
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.profile_versions(tenant_id,profile_id,source_id,context_id,dataset_id,actor_id,session_id,manifest,manifest_hash,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, r.Tenant, r.Spec.ID, r.Spec.Source, r.Spec.Context, r.Spec.Dataset, r.Actor, r.Session, raw, r.SpecHash, r.Created)
		if err != nil {
			return err
		}
		out = r
		return auditJob(ctx, tx, scope, "profile.reserved", r.Spec.ID)
	})
	if err != nil {
		return engineering.ProfileRecord{}, err
	}
	return out, nil
}

// ReadProfile is metadata-only and always enforces persisted private provenance.
func (d *DB) ReadProfile(ctx context.Context, e identity.Envelope, id string, write bool) (out engineering.ProfileRecord, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var e2 error
		out, e2 = profileTx(ctx, tx, e, id, write, false)
		return e2
	})
	if err != nil {
		return engineering.ProfileRecord{}, err
	}
	return out, nil
}

// AttachProfile preserves checkpoints but prevents concurrent logical jobs from
// owning the same version. A cancelled attempt can be explicitly resumed.
func (d *DB) AttachProfile(ctx context.Context, e identity.Envelope, id string, task jobs.RequestTask) (out engineering.ProfileRecord, err error) {
	if task.Require(e) != nil || task.Input.Kind != "profile.build" {
		return out, jobs.ErrAuthority
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = profileTx(ctx, tx, e, id, true, true)
		if err != nil {
			return err
		}
		if out.SpecHash != task.Input.InputHash || out.Spec.Source != task.Input.Target || out.Spec.Context != task.Input.Context || out.State == "complete" {
			return store.ErrConflict
		}
		actual, err := readRequestTx(ctx, tx, e, task.ID, true)
		if err != nil {
			return err
		}
		if actual.ManifestHash != task.ManifestHash {
			return store.ErrConflict
		}
		if out.Operation != "" && out.Operation != task.ID {
			var live bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.operations WHERE tenant_id=$1 AND operation_id=$2 AND status='running' AND lease_until>clock_timestamp())`, out.Tenant, out.Operation).Scan(&live); err != nil {
				return err
			}
			if live {
				return store.ErrConflict
			}
		}
		if _, err = tx.Exec(ctx, `UPDATE chartworks.profile_versions SET operation_id=$3 WHERE tenant_id=$1 AND profile_id=$2`, out.Tenant, id, task.ID); err != nil {
			return err
		}
		out.Operation = task.ID
		return nil
	})
	if err != nil {
		return engineering.ProfileRecord{}, err
	}
	return out, nil
}

func (d *DB) profileMutation(ctx context.Context, i jobs.Invocation, r engineering.ProfileRecord, fn func(context.Context, pgx.Tx, engineering.ProfileRecord) error) error {
	e, err := i.Current("profile.build", r.Spec.Source, r.SpecHash)
	if err != nil {
		return err
	}
	if err = r.Require(e, true); err != nil {
		return err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := profileSourceTx(ctx, tx, r); err != nil {
			return err
		}
		current, err := profileTx(ctx, tx, e, r.Spec.ID, true, true)
		if err != nil {
			return err
		}
		if current.SpecHash != r.SpecHash || current.Operation != i.Lease().Task.ID || current.State == "complete" {
			return store.ErrConflict
		}
		if _, err = requestFenceTx(ctx, tx, i); err != nil {
			return err
		}
		return fn(ctx, tx, current)
	})
}

// StartProfileRead records intent before the common executor can admit a native
// query. An absent receipt only proves safety after its earlier dispatch deadline.
func (d *DB) StartProfileRead(ctx context.Context, i jobs.Invocation, r engineering.ProfileRecord, operation string, until time.Time) error {
	if !identity.Identifier(operation) || until.IsZero() || !time.Now().Before(until) || time.Until(until) > time.Minute {
		return engineering.ErrInvalid
	}
	return d.profileMutation(ctx, i, r, func(ctx context.Context, tx pgx.Tx, current engineering.ProfileRecord) error {
		if current.Result != nil {
			return store.ErrConflict
		}
		if current.LastReadOperation != "" && current.LastReadOperation != operation {
			old, err := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND operation_id=$3 ORDER BY attempt_number DESC LIMIT 1`, r.Tenant, r.Actor, current.LastReadOperation))
			if errors.Is(err, pgx.ErrNoRows) {
				if time.Now().Before(current.LastReadDeadline) {
					return engineering.ErrState
				}
			} else if err != nil {
				return err
			} else if old.Finished == nil || old.RemoteState != "stopped" && old.RemoteState != "not_issued" {
				return engineering.ErrState
			}
		}
		_, err := tx.Exec(ctx, `UPDATE chartworks.profile_versions SET state='sampling',last_read_operation=$3,last_read_deadline=$4 WHERE tenant_id=$1 AND profile_id=$2`, r.Tenant, r.Spec.ID, operation, until)
		return err
	})
}

// CheckpointProfile retains deterministic aggregates only after an actual stopped
// successful read receipt, without marking the profile ready or moving its head.
func (d *DB) CheckpointProfile(ctx context.Context, i jobs.Invocation, r engineering.ProfileRecord, p engineering.Profile) error {
	if !p.Valid(r) {
		return engineering.ErrInvalid
	}
	return d.profileMutation(ctx, i, r, func(ctx context.Context, tx pgx.Tx, current engineering.ProfileRecord) error {
		if current.Result != nil {
			if current.Result.DeterministicHash() != p.DeterministicHash() {
				return store.ErrConflict
			}
			return nil
		}
		if current.LastReadOperation != p.ReadOperation || current.State != "sampling" {
			return store.ErrConflict
		}
		attempt, err := scanRead(tx.QueryRow(ctx, `SELECT `+readAttemptColumns+` FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3`, r.Tenant, r.Actor, p.ReadAttempt))
		if err != nil {
			return err
		}
		if attempt.Manifest.Operation != p.ReadOperation || attempt.Manifest.Session != r.Session || attempt.Manifest.Receipt.Context != r.Spec.Context || attempt.Manifest.Receipt.Source != r.Spec.Source || attempt.Finished == nil || attempt.RemoteState != "stopped" || attempt.Rows != p.Sampling.Rows || attempt.Bytes != p.Sampling.Bytes || attempt.Status != "succeeded" && attempt.Status != "empty" && attempt.Status != "truncated" {
			return engineering.ErrState
		}
		raw, _ := json.Marshal(p)
		if _, err = tx.Exec(ctx, `UPDATE chartworks.profile_versions SET state='checkpoint',result=$3,deterministic_hash=$4 WHERE tenant_id=$1 AND profile_id=$2`, r.Tenant, r.Spec.ID, raw, p.DeterministicHash()); err != nil {
			return err
		}
		scope, _ := store.NewScope(r.Tenant, r.Actor)
		return auditJob(ctx, tx, scope, "profile.checkpoint", r.Spec.ID)
	})
}

// StartProfileSummary is a one-way durable admission. A resumed lost summary is
// reported as unknown, not silently invoked again with a fresh model budget.
func (d *DB) StartProfileSummary(ctx context.Context, i jobs.Invocation, r engineering.ProfileRecord) (admitted bool, err error) {
	err = d.profileMutation(ctx, i, r, func(ctx context.Context, tx pgx.Tx, current engineering.ProfileRecord) error {
		if current.Result == nil || current.Spec.SkipLLM || !current.Settings.Summaries {
			return store.ErrConflict
		}
		if current.SummaryStarted {
			return nil
		}
		if _, err := tx.Exec(ctx, `UPDATE chartworks.profile_versions SET summary_started=true WHERE tenant_id=$1 AND profile_id=$2`, r.Tenant, r.Spec.ID); err != nil {
			return err
		}
		admitted = true
		scope, _ := store.NewScope(r.Tenant, r.Actor)
		return auditJob(ctx, tx, scope, "profile.summary_started", r.Spec.ID)
	})
	return admitted, err
}
