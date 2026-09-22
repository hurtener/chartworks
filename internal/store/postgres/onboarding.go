package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ onboarding.Repository = (*DB)(nil)

func onboardingPayload(r onboarding.Run) ([]byte, string, error) {
	b, e := json.Marshal(r)
	if e != nil || len(b) > 1<<20 {
		return nil, "", store.ErrInvalid
	}
	s := sha256.Sum256(b)
	return b, hex.EncodeToString(s[:]), nil
}
func scanOnboarding(row pgx.Row) (onboarding.Run, error) {
	var r onboarding.Run
	var body []byte
	var digest, stage, state, source, executionContext string
	var version int64
	if err := row.Scan(&body, &digest, &version, &stage, &state, &source, &executionContext); err != nil {
		return r, err
	}
	if json.Unmarshal(body, &r) != nil {
		return onboarding.Run{}, store.ErrInvalid
	}
	_, canonical, err := onboardingPayload(r)
	if err != nil || canonical != digest {
		return onboarding.Run{}, store.ErrInvalid
	}
	if r.Version != version || string(r.Stage) != stage || string(r.Status) != state || r.Input.Source != source || r.Input.Context != executionContext {
		return onboarding.Run{}, store.ErrInvalid
	}
	return r, nil
}

func (d *DB) CreateOnboarding(ctx context.Context, e identity.Envelope, r onboarding.Run, requestDigest string) (out onboarding.Run, err error) {
	body, digest, err := onboardingPayload(r)
	if err != nil || len(requestDigest) != 64 {
		return out, store.ErrInvalid
	}
	eventSum := sha256.Sum256([]byte("onboarding.created\x00" + e.Tenant() + "\x00" + r.ID))
	event := hex.EncodeToString(eventSum[:16])
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, x := tx.Exec(ctx, `INSERT INTO chartworks.onboarding_runs(tenant_id,onboarding_id,operation_key,request_digest,actor_id,session_id,version,stage,state,source_id,context_id,payload,payload_digest,created_at,updated_at,deadline) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) ON CONFLICT DO NOTHING`, e.Tenant(), r.ID, r.Key, requestDigest, e.User(), e.Session(), r.Version, r.Stage, r.Status, r.Input.Source, r.Input.Context, body, digest, r.CreatedAt, r.UpdatedAt, r.Deadline)
		if x != nil {
			return x
		}
		existing, x := scanOnboarding(tx.QueryRow(ctx, `SELECT payload,payload_digest,version,stage,state,source_id,context_id FROM chartworks.onboarding_runs WHERE tenant_id=$1 AND onboarding_id=$2 AND actor_id=$3 AND session_id=$4 AND operation_key=$5 AND request_digest=$6`, e.Tenant(), r.ID, e.User(), e.Session(), r.Key, requestDigest))
		if errors.Is(x, pgx.ErrNoRows) && tag.RowsAffected() == 0 {
			return store.ErrConflict
		}
		if x != nil {
			return x
		}
		out = existing
		if out.Version == 1 && out.UpdatedAt.Equal(out.CreatedAt) {
			_, x = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,'onboarding.created',$4) ON CONFLICT DO NOTHING`, e.Tenant(), event, e.User(), r.ID)
		}
		return x
	})
	return out, err
}

func (d *DB) ReadOnboarding(ctx context.Context, e identity.Envelope, id string) (out onboarding.Run, err error) {
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var x error
		out, x = scanOnboarding(tx.QueryRow(ctx, `SELECT payload,payload_digest,version,stage,state,source_id,context_id FROM chartworks.onboarding_runs WHERE tenant_id=$1 AND onboarding_id=$2 AND actor_id=$3 AND session_id=$4`, e.Tenant(), id, e.User(), e.Session()))
		return x
	})
	return out, err
}

func (d *DB) SaveOnboarding(ctx context.Context, e identity.Envelope, r onboarding.Run, expected int64) (out onboarding.Run, err error) {
	if expected < 1 || r.Version != expected {
		return out, store.ErrInvalid
	}
	r.Version = expected + 1
	body, digest, err := onboardingPayload(r)
	if err != nil {
		return out, err
	}
	event, err := newID()
	if err != nil {
		return out, err
	}
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		tag, x := tx.Exec(ctx, `UPDATE chartworks.onboarding_runs SET version=$5,stage=$6,state=$7,payload=$8,payload_digest=$9,updated_at=$10 WHERE tenant_id=$1 AND onboarding_id=$2 AND actor_id=$3 AND session_id=$4 AND version=$11`, e.Tenant(), r.ID, e.User(), e.Session(), r.Version, r.Stage, r.Status, body, digest, r.UpdatedAt, expected)
		if x != nil {
			return x
		}
		if tag.RowsAffected() != 1 {
			return store.ErrConflict
		}
		action := "onboarding.progressed"
		if r.Status == onboarding.StatusCancelled {
			action = "onboarding.cancelled"
		}
		_, x = tx.Exec(ctx, `INSERT INTO chartworks.audit_events(tenant_id,event_id,actor_id,action,resource_id) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), event, e.User(), action, r.ID)
		out = r
		return x
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = store.ErrNotFound
	}
	return out, err
}
