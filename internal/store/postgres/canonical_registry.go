package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// checkCanonicalMeaningsTx validates exact reuse or one-step advancement. The
// caller decides whether approval is permitted; this function never widens it.
func checkCanonicalMeaningsTx(ctx context.Context, tx pgx.Tx, tenant string, meanings []semantics.CanonicalMeaning, lock bool) (changes bool, err error) {
	if len(meanings) == 0 {
		return false, nil
	}
	if lock {
		if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,721415))`, tenant); err != nil {
			return false, err
		}
	}
	seenIDs := map[string]bool{}
	proposedTerms := map[string]string{}
	allTerms := make([]string, 0, len(meanings))
	for _, meaning := range meanings {
		if !meaning.Valid() || seenIDs[meaning.ID] {
			return false, store.ErrInvalid
		}
		seenIDs[meaning.ID] = true
		for _, term := range meaning.Terms() {
			if owner, ok := proposedTerms[term]; ok && owner != meaning.ID {
				return false, store.ErrConflict
			}
			if _, ok := proposedTerms[term]; !ok {
				allTerms = append(allTerms, term)
			}
			proposedTerms[term] = meaning.ID
		}
	}
	raw, marshalErr := json.Marshal(meanings)
	if marshalErr != nil {
		return false, store.ErrInvalid
	}
	type registryState struct {
		head   int64
		digest string
	}
	states := map[string]registryState{}
	rows, err := tx.Query(ctx, `SELECT p.entity_id,COALESCE(h.current_revision,0),COALESCE(r.digest,'') FROM jsonb_to_recordset($2::jsonb) AS p(entity_id text,revision bigint) LEFT JOIN chartworks.canonical_entity_heads h ON(h.tenant_id=$1 AND h.entity_id=p.entity_id) LEFT JOIN chartworks.canonical_entity_revisions r ON(r.tenant_id=$1 AND r.entity_id=p.entity_id AND r.revision=p.revision)`, tenant, raw)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var id string
		var state registryState
		if err = rows.Scan(&id, &state.head, &state.digest); err != nil {
			rows.Close()
			return false, err
		}
		states[id] = state
	}
	err = rows.Err()
	rows.Close()
	if err != nil || len(states) != len(meanings) {
		if err != nil {
			return false, err
		}
		return false, store.ErrInvalid
	}
	reserved := map[string]string{}
	rows, err = tx.Query(ctx, `SELECT normalized_term,entity_id FROM chartworks.canonical_entity_terms WHERE tenant_id=$1 AND normalized_term=ANY($2::text[])`, tenant, allTerms)
	if err != nil {
		return false, err
	}
	for rows.Next() {
		var term, owner string
		if err = rows.Scan(&term, &owner); err != nil {
			rows.Close()
			return false, err
		}
		reserved[term] = owner
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, meaning := range meanings {
		digest := meaning.Digest()
		terms := meaning.Terms()
		if digest == "" || len(terms) == 0 {
			return false, store.ErrInvalid
		}
		state := states[meaning.ID]
		switch {
		case state.digest != "" && state.digest != digest:
			return false, store.ErrConflict
		case state.digest != "":
			// Exact immutable meaning reuse is valid even after a newer revision.
		case state.head == 0 && meaning.Revision == 1:
			changes = true
		case state.head > 0 && meaning.Revision == state.head+1:
			changes = true
		default:
			return false, store.ErrConflict
		}
		for _, term := range terms {
			if owner, ok := reserved[term]; ok && owner != meaning.ID {
				return false, store.ErrConflict
			}
		}
	}
	return changes, nil
}

func (d *DB) CheckCanonicalMeanings(ctx context.Context, e identity.Envelope, topic string, a drafts.Access, meanings []semantics.CanonicalMeaning) (changes bool, err error) {
	if a != drafts.Write && a != drafts.Publish {
		return false, store.ErrInvalid
	}
	if err = drafts.Require(e, topic, a); err != nil {
		return false, err
	}
	if len(meanings) == 0 {
		return false, nil
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return false, err
	}
	defer cancel()
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		changes, err = checkCanonicalMeaningsTx(ctx, tx, e.Tenant(), meanings, false)
		return err
	})
	return changes, err
}

// approveCanonicalMeaningsTx runs under the publication transaction. It is the
// only path that creates tenant-wide meaning; reviewed topic publication is the
// approval event and topic-local keys are retained only on that publication.
func approveCanonicalMeaningsTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, topic, version, review string, meanings []semantics.CanonicalMeaning) error {
	changes, err := checkCanonicalMeaningsTx(ctx, tx, e.Tenant(), meanings, true)
	if err != nil {
		return err
	}
	if changes {
		if err = access.Require(e, "topics.publish", access.Tenant(e, "write")); err != nil {
			return err
		}
	}
	for _, meaning := range meanings {
		raw, err := json.Marshal(meaning)
		if err != nil {
			return store.ErrInvalid
		}
		command, err := tx.Exec(ctx, `INSERT INTO chartworks.canonical_entity_revisions(tenant_id,entity_id,revision,digest,meaning,approving_topic_id,approving_version_id,review_id,actor_id,session_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT DO NOTHING`, e.Tenant(), meaning.ID, meaning.Revision, meaning.Digest(), raw, topic, version, review, e.User(), e.Session())
		if err != nil {
			return err
		}
		if command.RowsAffected() == 1 {
			if meaning.Revision == 1 {
				_, err = tx.Exec(ctx, `INSERT INTO chartworks.canonical_entity_heads(tenant_id,entity_id,current_revision) VALUES($1,$2,$3)`, e.Tenant(), meaning.ID, meaning.Revision)
			} else {
				command, err = tx.Exec(ctx, `UPDATE chartworks.canonical_entity_heads SET current_revision=$3 WHERE tenant_id=$1 AND entity_id=$2 AND current_revision=($3::bigint-1)`, e.Tenant(), meaning.ID, meaning.Revision)
				if err == nil && command.RowsAffected() != 1 {
					return store.ErrConflict
				}
			}
			if err != nil {
				return err
			}
			for _, term := range meaning.Terms() {
				if _, err = tx.Exec(ctx, `INSERT INTO chartworks.canonical_entity_terms(tenant_id,normalized_term,entity_id,first_revision) VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, e.Tenant(), term, meaning.ID, meaning.Revision); err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_published_canonical_refs(tenant_id,topic_id,version_id,entity_id,revision,digest) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), topic, version, meaning.ID, meaning.Revision, meaning.Digest()); err != nil {
			return err
		}
	}
	return nil
}
