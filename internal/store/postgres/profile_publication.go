package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// PublishProfile resolves source revision, previous active version and late
// cancellation in one metadata commit. It never modifies approved SQL/definitions.
func (d *DB) PublishProfile(ctx context.Context, i jobs.Invocation, r engineering.ProfileRecord, p engineering.Profile) error {
	if !p.Valid(r) {
		return engineering.ErrInvalid
	}
	return d.profileMutation(ctx, i, r, func(ctx context.Context, tx pgx.Tx, current engineering.ProfileRecord) error {
		if current.Result == nil || current.State != "checkpoint" || current.Result.DeterministicHash() != p.DeterministicHash() {
			return store.ErrConflict
		}
		if p.Summary.Status == "available" && !current.SummaryStarted {
			return store.ErrConflict
		}
		if err := profileHeadLock(ctx, tx, r); err != nil {
			return err
		}
		head, err := profileHead(ctx, tx, r)
		if err != nil {
			return err
		}
		if head != r.Spec.Previous {
			return store.ErrConflict
		}
		e, err := i.Current("profile.build", r.Spec.Source, r.SpecHash)
		if err != nil {
			return err
		}
		changes := []engineering.SchemaChange{}
		if head != "" {
			previous, err := profileTx(ctx, tx, e, head, false, false)
			if err != nil {
				return err
			}
			if previous.Result == nil || previous.State != "complete" {
				return store.ErrConflict
			}
			changes = engineering.SchemaDiff(previous.Result.Schema, p.Schema)
		}
		if err = profileHealthTx(ctx, tx, r, p, changes); err != nil {
			return err
		}
		raw, _ := json.Marshal(p)
		diff, _ := json.Marshal(changes)
		if _, err = tx.Exec(ctx, `UPDATE chartworks.profile_versions SET state='complete',result=$3,changes=$4 WHERE tenant_id=$1 AND profile_id=$2`, r.Tenant, r.Spec.ID, raw, diff); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.profile_heads(tenant_id,actor_id,session_id,source_id,dataset_id,profile_id) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(tenant_id,actor_id,session_id,source_id,dataset_id) DO UPDATE SET profile_id=EXCLUDED.profile_id`, r.Tenant, r.Actor, r.Session, r.Spec.Source, r.Spec.Dataset, r.Spec.ID); err != nil {
			return err
		}
		scope, _ := store.NewScope(r.Tenant, r.Actor)
		if err = auditJob(ctx, tx, scope, "profile.published", r.Spec.ID); err != nil {
			return err
		}
		return completeRequestTx(ctx, tx, i)
	})
}

func profileHealthTx(ctx context.Context, tx pgx.Tx, r engineering.ProfileRecord, p engineering.Profile, changes []engineering.SchemaChange) error {
	rows, err := tx.Query(ctx, `SELECT manifest FROM chartworks.profile_dependencies WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND source_id=$4 AND dataset_id=$5 ORDER BY kind,resource_id,definition_version LIMIT 257`, r.Tenant, r.Actor, r.Session, r.Spec.Source, r.Spec.Dataset)
	if err != nil {
		return err
	}
	dependencies := []engineering.Dependency{}
	for rows.Next() {
		var raw []byte
		if err = rows.Scan(&raw); err != nil {
			rows.Close()
			return err
		}
		var dep engineering.Dependency
		if json.Unmarshal(raw, &dep) != nil || !dep.Valid() {
			rows.Close()
			return store.ErrInvalid
		}
		dependencies = append(dependencies, dep)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(dependencies) > 256 {
		return engineering.ErrLimit
	}
	for _, dep := range dependencies {
		affected := []engineering.SchemaChange{}
		for _, change := range changes {
			for _, column := range dep.Columns {
				if column == change.Column {
					affected = append(affected, change)
					break
				}
			}
		}
		state := "unchanged"
		if len(affected) > 0 || dep.Context != r.Spec.Context {
			state = "needs_review"
		} else if p.Freshness.State == "stale" {
			state = "stale"
		}
		if state == "unchanged" {
			continue
		}
		event := engineering.HealthEvent{Profile: r.Spec.ID, Dependency: dep, State: state, Changes: affected, ObservedAt: p.ObservedAt}
		event.ID = readexec.Hash([]any{r.Tenant, r.Actor, r.Session, r.Spec.ID, dep.Kind, dep.ID, dep.Version})
		raw, _ := json.Marshal(event)
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.profile_health_events(tenant_id,event_id,actor_id,session_id,profile_id,kind,resource_id,definition_version,event) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) ON CONFLICT DO NOTHING`, r.Tenant, event.ID, r.Actor, r.Session, r.Spec.ID, dep.Kind, dep.ID, dep.Version, raw)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			scope, _ := store.NewScope(r.Tenant, r.Actor)
			if err = auditJob(ctx, tx, scope, "profile.health_changed", event.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// ProfileEvidence is an authenticated metadata consumer, not a warehouse query.
func (d *DB) ProfileEvidence(ctx context.Context, e identity.Envelope, id string) (out engineering.ProfileEvidence, err error) {
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer stop()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		r, err := profileTx(ctx, tx, e, id, false, false)
		if err != nil {
			return err
		}
		if r.State != "complete" || r.Result == nil {
			return store.ErrConflict
		}
		head, err := profileHead(ctx, tx, r)
		if err != nil {
			return err
		}
		var changes []byte
		if err = tx.QueryRow(ctx, `SELECT changes FROM chartworks.profile_versions WHERE tenant_id=$1 AND profile_id=$2`, r.Tenant, id).Scan(&changes); err != nil {
			return err
		}
		out = engineering.ProfileEvidence{Profile: *r.Result, Active: head == id, AuthorityContext: r.Spec.Context, SemanticPublication: false, Changes: []engineering.SchemaChange{}}
		if json.Unmarshal(changes, &out.Changes) != nil {
			return store.ErrInvalid
		}
		return nil
	})
	if err != nil {
		return engineering.ProfileEvidence{}, err
	}
	return out, nil
}

// ProfileHistory applies actor/session/source/dataset/context reach before LIMIT.
func (d *DB) ProfileHistory(ctx context.Context, e identity.Envelope, source, partition, dataset string, limit int) (out []engineering.ProfileStatus, err error) {
	spec := engineering.ProfileSpec{ID: "history", Source: source, Context: partition, Dataset: dataset}
	if !spec.Valid() || limit < 1 || limit > 32 {
		return nil, engineering.ErrInvalid
	}
	if err = spec.Require(e, false); err != nil {
		return nil, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer stop()
	out = []engineering.ProfileStatus{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+profileColumns+` FROM chartworks.profile_versions p JOIN chartworks.sources s ON(s.tenant_id,s.source_id)=(p.tenant_id,p.source_id) WHERE p.tenant_id=$1 AND p.actor_id=$2 AND p.session_id=$3 AND p.source_id=$4 AND p.context_id=$5 AND p.dataset_id=$6 AND p.state<>'erased' AND NOT s.deleted ORDER BY p.created_at DESC,p.profile_id DESC LIMIT $7`, e.Tenant(), e.User(), e.Session(), source, partition, dataset, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			r, err := scanProfile(rows)
			if err != nil {
				return err
			}
			if err = r.Require(e, false); err != nil {
				return err
			}
			out = append(out, r.Public())
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RegisterDependency stores only an immutable consumer definition reference. It
// neither publishes that consumer nor supplies authority to its future executions.
func (d *DB) RegisterDependency(ctx context.Context, e identity.Envelope, id string, dep engineering.Dependency) error {
	if !dep.Valid() {
		return engineering.ErrInvalid
	}
	if err := dep.Require(e, true); err != nil {
		return err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return err
	}
	defer stop()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	return d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		r, err := profileTx(ctx, tx, e, id, false, false)
		if err != nil {
			return err
		}
		if r.State != "complete" || r.Result == nil || dep.Source != r.Spec.Source || dep.Context != r.Spec.Context || dep.Dataset != r.Spec.Dataset {
			return store.ErrConflict
		}
		// The head advisory lock excludes publication, not source deletion.
		// Lock the live source first, as profile publication does, so erasure
		// cannot finish between this proof and insertion of a new reference.
		var liveSource string
		if err = tx.QueryRow(ctx, `SELECT source_id FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 AND NOT deleted FOR SHARE`, r.Tenant, r.Spec.Source).Scan(&liveSource); err != nil {
			return err
		}
		if err = profileHeadLock(ctx, tx, r); err != nil {
			return err
		}
		// Serialize with profile publication. Registering a new consumer against an
		// obsolete head could otherwise silently miss an already-published drift.
		head, err := profileHead(ctx, tx, r)
		if err != nil {
			return err
		}
		if head != id {
			return store.ErrConflict
		}
		known := map[string]bool{}
		for _, c := range r.Result.Schema {
			known[c.Name] = true
		}
		for _, c := range dep.Columns {
			if !known[c] {
				return engineering.ErrInvalid
			}
		}
		var previous []byte
		err = tx.QueryRow(ctx, `SELECT manifest FROM chartworks.profile_dependencies WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND kind=$4 AND resource_id=$5 AND definition_version=$6`, r.Tenant, r.Actor, r.Session, dep.Kind, dep.ID, dep.Version).Scan(&previous)
		raw, _ := json.Marshal(dep)
		if err == nil {
			var old engineering.Dependency
			if json.Unmarshal(previous, &old) != nil || readexec.Hash(old) != readexec.Hash(dep) {
				return store.ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.profile_dependencies WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND source_id=$4 AND dataset_id=$5`, r.Tenant, r.Actor, r.Session, r.Spec.Source, r.Spec.Dataset).Scan(&count); err != nil {
			return err
		}
		if count >= 256 {
			return engineering.ErrLimit
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.profile_dependencies(tenant_id,actor_id,session_id,kind,resource_id,definition_version,source_id,dataset_id,profile_id,manifest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, r.Tenant, r.Actor, r.Session, dep.Kind, dep.ID, dep.Version, dep.Source, dep.Dataset, id, raw); err != nil {
			return err
		}
		return auditJob(ctx, tx, scope, "profile.dependency_registered", dep.ID)
	})
}

// DependencyHealth requires both the referenced definition and each event's
// actual profile context. Unauthorized contexts are excluded before the limit.
func (d *DB) DependencyHealth(ctx context.Context, e identity.Envelope, dep engineering.Dependency) (out []engineering.HealthEvent, err error) {
	if !dep.Valid() {
		return nil, engineering.ErrInvalid
	}
	if err = dep.Require(e, false); err != nil {
		return nil, err
	}
	contexts, err := access.Constrain(e, "engineering.read", "execution_context", "use")
	if err != nil {
		return nil, err
	}
	ctx, stop, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer stop()
	out = []engineering.HealthEvent{}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var manifest []byte
		if err := tx.QueryRow(ctx, `SELECT d.manifest FROM chartworks.profile_dependencies d JOIN chartworks.sources s ON(s.tenant_id,s.source_id)=(d.tenant_id,d.source_id) WHERE d.tenant_id=$1 AND d.actor_id=$2 AND d.session_id=$3 AND d.kind=$4 AND d.resource_id=$5 AND d.definition_version=$6 AND NOT s.deleted`, e.Tenant(), e.User(), e.Session(), dep.Kind, dep.ID, dep.Version).Scan(&manifest); err != nil {
			return err
		}
		var actual engineering.Dependency
		if json.Unmarshal(manifest, &actual) != nil || readexec.Hash(actual) != readexec.Hash(dep) {
			return store.ErrConflict
		}
		rows, err := tx.Query(ctx, `SELECT h.event FROM chartworks.profile_health_events h JOIN chartworks.profile_versions p ON(p.tenant_id,p.profile_id)=(h.tenant_id,h.profile_id) WHERE h.tenant_id=$1 AND h.actor_id=$2 AND h.session_id=$3 AND h.kind=$4 AND h.resource_id=$5 AND h.definition_version=$6 AND p.state='complete' AND ($7 OR p.context_id=ANY($8::text[])) ORDER BY p.created_at DESC,h.event_id LIMIT 32`, e.Tenant(), e.User(), e.Session(), dep.Kind, dep.ID, dep.Version, contexts.All(), contexts.IDs())
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if err = rows.Scan(&raw); err != nil {
				return err
			}
			var event engineering.HealthEvent
			if json.Unmarshal(raw, &event) != nil || event.ID == "" || readexec.Hash(event.Dependency) != readexec.Hash(dep) {
				return store.ErrInvalid
			}
			out = append(out, event)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
