package postgres

import (
	"context"
	"encoding/json"
	"sort"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

var _ drafts.Repository = (*DB)(nil)

// All dependency and private-owner restrictions are applied by SQL before a
// manifest is selected. Retained reads do not require a live warehouse, but
// erased profiles/deleted sources cease to make private authoring data readable.
const topicEligibility = `h.tenant_id=$1 AND h.topic_id=$2 AND h.actor_id=$3 AND h.session_id=$4 AND NOT EXISTS (
 SELECT 1 FROM chartworks.topic_draft_dependencies d
 JOIN chartworks.sources s ON(s.tenant_id,s.source_id)=(d.tenant_id,d.source_id)
 JOIN chartworks.profile_versions p ON(p.tenant_id,p.profile_id)=(d.tenant_id,d.profile_id)
 WHERE (d.tenant_id,d.topic_id,d.revision)=(v.tenant_id,v.topic_id,v.revision)
 AND (NOT ($5 OR d.source_id=ANY($6::text[])) OR NOT ($7 OR d.dataset_id=ANY($8::text[]))
 OR NOT ($9 OR d.context_id=ANY($10::text[])) OR s.deleted OR p.state<>'complete'
 OR p.actor_id<>$3 OR p.session_id<>$4))`
const topicFrom = ` FROM chartworks.topic_draft_heads h JOIN chartworks.topic_draft_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) WHERE `
const topicMetadata = `v.topic_id,v.revision,v.version_id,v.digest,h.actor_id,h.session_id,v.created_at,v.change_note`

func topicArgs(e identity.Envelope, id string, a drafts.Access) ([]any, error) {
	if !identity.Identifier(id) {
		return nil, store.ErrInvalid
	}
	if err := drafts.Require(e, id, a); err != nil {
		return nil, err
	}
	s, err := access.Constrain(e, a.Action(), "source", "read")
	if err != nil {
		return nil, err
	}
	d, err := access.Constrain(e, a.Action(), "dataset", "query")
	if err != nil {
		return nil, err
	}
	c, err := access.Constrain(e, a.Action(), "execution_context", "use")
	if err != nil {
		return nil, err
	}
	return []any{e.Tenant(), id, e.User(), e.Session(), s.All(), s.IDs(), d.All(), d.IDs(), c.All(), c.IDs()}, nil
}
func scanTopic(row pgx.Row) (out drafts.Version, err error) {
	var body []byte
	m := &out.Metadata
	err = row.Scan(&m.Topic, &m.Revision, &m.Version, &m.Digest, &m.Actor, &m.Session, &m.Created, &m.Change, &body)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(body, &out.Pack) != nil {
		return drafts.Version{}, store.ErrInvalid
	}
	model, err := semantics.Compile(out.Pack)
	if err != nil || model.Digest() != m.Digest || out.Pack.Topic != m.Topic || out.Pack.Version != m.Version {
		return drafts.Version{}, store.ErrInvalid
	}
	out.Pack = model.Pack()
	return out, nil
}
func topicTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string, revision int64, a drafts.Access) (drafts.Version, error) {
	args, err := topicArgs(e, id, a)
	if err != nil {
		return drafts.Version{}, err
	}
	args = append(args, revision)
	return scanTopic(tx.QueryRow(ctx, `SELECT `+topicMetadata+`,v.manifest`+topicFrom+topicEligibility+` AND v.revision=CASE WHEN $11::bigint=0 THEN h.current_revision ELSE $11 END`, args...))
}
func (d *DB) ReadTopicDraft(ctx context.Context, e identity.Envelope, id string, revision int64, a drafts.Access) (out drafts.Version, err error) {
	if revision < 0 || revision > drafts.MaxRevisions {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = topicTx(ctx, tx, e, id, revision, a)
		return err
	})
	return out, err
}
func (d *DB) TopicDraftHistory(ctx context.Context, e identity.Envelope, id string, before int64, limit int) (out []drafts.Revision, err error) {
	if before < 0 || before > drafts.MaxRevisions+1 || limit < 1 || limit > 32 {
		return nil, store.ErrInvalid
	}
	args, err := topicArgs(e, id, drafts.Read)
	if err != nil {
		return nil, err
	}
	args = append(args, before, limit)
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT `+topicMetadata+topicFrom+topicEligibility+` AND ($11::bigint=0 OR v.revision<$11) ORDER BY v.revision DESC LIMIT $12`, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = []drafts.Revision{}
		for rows.Next() {
			var r drafts.Revision
			if err = rows.Scan(&r.Topic, &r.Revision, &r.Version, &r.Digest, &r.Actor, &r.Session, &r.Created, &r.Change); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return out, err
}
func (d *DB) SaveTopicDraft(ctx context.Context, e identity.Envelope, prepared drafts.Prepared, expected int64, change string) (out drafts.Version, err error) {
	if expected < 0 || expected >= drafts.MaxRevisions || len(change) < 1 || len(change) > 1024 || !utf8.ValidString(change) {
		return out, store.ErrInvalid
	}
	pack, err := prepared.Pack(e)
	if err != nil {
		return out, err
	}
	if expected == 0 {
		if err = access.Require(e, "topics.write", access.Tenant(e, "write")); err != nil {
			return out, err
		}
	}
	model, err := semantics.Compile(pack)
	if err != nil {
		return out, store.ErrInvalid
	}
	body, err := json.Marshal(pack)
	if err != nil {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// One tenant lock also makes the fixed registration cap race-safe. Draft
		// editing is metadata-only; warehouse I/O has already finished above.
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,721415))`, e.Tenant()); err != nil {
			return err
		}
		var current int64
		var actor, session string
		err := tx.QueryRow(ctx, `SELECT current_revision,actor_id,session_id FROM chartworks.topic_draft_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, e.Tenant(), pack.Topic).Scan(&current, &actor, &session)
		if err != nil && err != pgx.ErrNoRows {
			return err
		}
		if err == nil && (actor != e.User() || session != e.Session()) {
			return access.ErrNotFound
		}
		if current != expected {
			return store.ErrConflict
		}
		if current > 0 {
			if _, err = topicTx(ctx, tx, e, pack.Topic, current, drafts.Write); err != nil {
				return err
			}
		} else {
			var count int
			if err = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.topic_draft_heads WHERE tenant_id=$1`, e.Tenant()).Scan(&count); err != nil {
				return err
			}
			if count >= 256 {
				return readexec.ErrLimit
			}
		}
		// Stable order is shared with source rotation/erasure fences; every pinned
		// profile must still be complete, private and the active admission profile.
		datasets := append([]semantics.Dataset(nil), pack.Datasets...)
		sort.Slice(datasets, func(i, j int) bool {
			if datasets[i].Source.Source != datasets[j].Source.Source {
				return datasets[i].Source.Source < datasets[j].Source.Source
			}
			return datasets[i].ID < datasets[j].ID
		})
		for _, dataset := range datasets {
			r := dataset.Source
			var revision int64
			if err = tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 AND NOT deleted FOR SHARE`, e.Tenant(), r.Source).Scan(&revision); err != nil {
				return err
			}
			if revision != r.SourceRevision {
				return readexec.ErrBinding
			}
			var digest string
			err = tx.QueryRow(ctx, `SELECT p.deterministic_hash FROM chartworks.profile_versions p JOIN chartworks.profile_heads h ON(h.tenant_id,h.actor_id,h.session_id,h.source_id,h.dataset_id,h.profile_id)=(p.tenant_id,p.actor_id,p.session_id,p.source_id,p.dataset_id,p.profile_id) WHERE p.tenant_id=$1 AND p.profile_id=$2 AND p.source_id=$3 AND p.context_id=$4 AND p.dataset_id=$5 AND p.actor_id=$6 AND p.session_id=$7 AND p.state='complete' FOR SHARE OF p,h`, e.Tenant(), r.ProfileVersion, r.Source, r.Context, r.Dataset, e.User(), e.Session()).Scan(&digest)
			if err != nil {
				return err
			}
			if digest != r.ProfileDigest {
				return readexec.ErrBinding
			}
		}
		if _, err = prepared.Pack(e); err != nil {
			return err
		}
		if current == 0 {
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_draft_heads(tenant_id,topic_id,actor_id,session_id,current_revision) VALUES($1,$2,$3,$4,1)`, e.Tenant(), pack.Topic, e.User(), e.Session())
		} else {
			_, err = tx.Exec(ctx, `UPDATE chartworks.topic_draft_heads SET current_revision=$3 WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), pack.Topic, expected+1)
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_draft_versions(tenant_id,topic_id,revision,version_id,manifest,digest,change_note) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), pack.Topic, expected+1, pack.Version, body, model.Digest(), change)
		if err != nil {
			return err
		}
		for _, dataset := range pack.Datasets {
			r := dataset.Source
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_draft_dependencies(tenant_id,topic_id,revision,dataset_id,source_id,context_id,source_revision,profile_id,profile_digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), pack.Topic, expected+1, dataset.ID, r.Source, r.Context, r.SourceRevision, r.ProfileVersion, r.ProfileDigest)
			if err != nil {
				return err
			}
		}
		scope, err := store.NewScope(e.Tenant(), e.User())
		if err != nil {
			return err
		}
		if err = auditJob(ctx, tx, scope, "topic.drafted", pack.Topic); err != nil {
			return err
		}
		out, err = topicTx(ctx, tx, e, pack.Topic, expected+1, drafts.Write)
		return err
	})
	return out, err
}
