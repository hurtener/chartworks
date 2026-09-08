package postgres

import (
	"context"
	"encoding/json"
	"sort"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/jackc/pgx/v5"
)

var _ topics.Repository = (*DB)(nil)

const reviewColumns = `r.review_id,r.topic_id,r.draft_revision,r.digest,r.decision,r.note,r.actor_id,r.created_at`

func scanTopicReview(row pgx.Row) (out topics.Review, err error) {
	var actor string
	err = row.Scan(&out.ID, &out.Topic, &out.DraftRevision, &out.Digest, &out.Decision, &out.Note, &actor, &out.Created)
	return
}
func (d *DB) ReviewTopic(ctx context.Context, e identity.Envelope, id string, in topics.ReviewRequest) (out topics.Review, err error) {
	if in.DraftRevision < 1 || in.DraftRevision > drafts.MaxRevisions || !topics.DigestValid(in.Digest) || !topics.NoteValid(in.Note) || (in.Decision != "approve" && in.Decision != "reject") {
		return out, store.ErrInvalid
	}
	if err = drafts.Require(e, id, drafts.Review); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var current int64
		if err := tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.topic_draft_heads WHERE tenant_id=$1 AND topic_id=$2 AND actor_id=$3 AND session_id=$4 FOR UPDATE`, e.Tenant(), id, e.User(), e.Session()).Scan(&current); err != nil {
			return err
		}
		draft, err := topicTx(ctx, tx, e, id, in.DraftRevision, drafts.Review)
		if err != nil {
			return err
		}
		if draft.Metadata.Digest != in.Digest {
			return store.ErrConflict
		}
		var count int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.topic_reviews WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), id).Scan(&count); err != nil {
			return err
		}
		if count >= 512 {
			return readexec.ErrLimit
		}
		reviewID, err := newID()
		if err != nil {
			return err
		}
		out, err = scanTopicReview(tx.QueryRow(ctx, `INSERT INTO chartworks.topic_reviews AS r(tenant_id,review_id,topic_id,draft_revision,digest,decision,note,actor_id,session_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING `+reviewColumns, e.Tenant(), reviewID, id, in.DraftRevision, in.Digest, in.Decision, in.Note, e.User(), e.Session()))
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_publication_heads(tenant_id,topic_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, e.Tenant(), id); err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		return auditJob(ctx, tx, scope, "topic.reviewed", id)
	})
	return
}
func reviewedTopicTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id, reviewID string) (topics.Review, drafts.Version, error) {
	if err := drafts.Require(e, id, drafts.Publish); err != nil {
		return topics.Review{}, drafts.Version{}, err
	}
	review, err := scanTopicReview(tx.QueryRow(ctx, `SELECT `+reviewColumns+` FROM chartworks.topic_reviews r WHERE r.tenant_id=$1 AND r.topic_id=$2 AND r.review_id=$3 AND r.actor_id=$4 AND r.session_id=$5`, e.Tenant(), id, reviewID, e.User(), e.Session()))
	if err != nil {
		return review, drafts.Version{}, err
	}
	draft, err := topicTx(ctx, tx, e, id, review.DraftRevision, drafts.Publish)
	if err != nil {
		return review, draft, err
	}
	if review.Digest != draft.Metadata.Digest {
		return topics.Review{}, drafts.Version{}, store.ErrConflict
	}
	return review, draft, nil
}
func (d *DB) ReviewedTopic(ctx context.Context, e identity.Envelope, id, reviewID string) (review topics.Review, draft drafts.Version, err error) {
	if !identity.Identifier(reviewID) {
		return review, draft, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return review, draft, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		review, draft, err = reviewedTopicTx(ctx, tx, e, id, reviewID)
		return err
	})
	return
}

const publishedEligibility = `h.tenant_id=$1 AND h.topic_id=$2 AND NOT EXISTS (
 SELECT 1 FROM chartworks.topic_published_dependencies d
 WHERE(d.tenant_id,d.topic_id,d.version_id)=(v.tenant_id,v.topic_id,v.version_id)
 AND(NOT($3 OR d.source_id=ANY($4::text[])) OR NOT($5 OR d.dataset_id=ANY($6::text[])) OR NOT($7 OR d.context_id=ANY($8::text[]))))`

func publishedArgs(e identity.Envelope, id string, a drafts.Access) ([]any, error) {
	args, err := topicArgs(e, id, a)
	if err != nil {
		return nil, err
	}
	return append(args[:2], args[4:]...), nil
}
func publishedGenerations(ctx context.Context, tx pgx.Tx, tenant, id, version string) (out []vindex.Generation, err error) {
	rows, err := tx.Query(ctx, `SELECT manifest FROM chartworks.topic_published_generations WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3 ORDER BY context_id`, tenant, id, version)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		var g vindex.Generation
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if json.Unmarshal(raw, &g) != nil || !g.Valid() || g.Topic != id || g.Version != version {
			return nil, store.ErrInvalid
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func readPublishedTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id, version string, a drafts.Access) (out topics.Published, err error) {
	if version != "" && !identity.Identifier(version) {
		return out, store.ErrInvalid
	}
	args, err := publishedArgs(e, id, a)
	if err != nil {
		return out, err
	}
	args = append(args, version)
	var raw []byte
	var active string
	err = tx.QueryRow(ctx, `SELECT h.revision,h.active_version,h.archived,v.version_id,v.definition,v.digest,v.created_at FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id)=(h.tenant_id,h.topic_id) WHERE `+publishedEligibility+` AND v.version_id=CASE WHEN $9::text='' THEN h.active_version ELSE $9 END`, args...).Scan(&out.State.Revision, &active, &out.State.Archived, &out.State.Version, &raw, &out.Digest, &out.PublishedAt)
	if err != nil {
		return out, err
	}
	if json.Unmarshal(raw, &out.Definition) != nil || out.Definition.Topic != id || out.Definition.Version != out.State.Version {
		return topics.Published{}, store.ErrInvalid
	}
	out.State.Topic = id
	out.State.Active = active == out.State.Version && !out.State.Archived
	generations, err := publishedGenerations(ctx, tx, e.Tenant(), id, out.State.Version)
	if err != nil {
		return topics.Published{}, err
	}
	for _, g := range generations {
		out.State.Generations = append(out.State.Generations, topics.FacetGeneration{Context: g.Context, Generation: g.ID, Space: g.Space.Key(), Facets: len(g.Expected)})
	}
	return out, nil
}
func (d *DB) ReadPublishedTopic(ctx context.Context, e identity.Envelope, id, version string, a drafts.Access) (out topics.Published, err error) {
	if a != drafts.Read && a != drafts.Publish {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		out, err = readPublishedTx(ctx, tx, e, id, version, a)
		return err
	})
	return
}
func topicPublicationLock(ctx context.Context, tx pgx.Tx, tenant, id string, expected int64) (string, error) {
	var revision int64
	var version string
	err := tx.QueryRow(ctx, `SELECT revision,COALESCE(active_version,'') FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, tenant, id).Scan(&revision, &version)
	if err != nil {
		return "", err
	}
	if revision != expected {
		return "", store.ErrConflict
	}
	return version, nil
}
func publishedSourceFence(ctx context.Context, tx pgx.Tx, tenant string, definition topics.Definition) error {
	ordered := append([]topics.Dataset(nil), definition.Datasets...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Source.Source < ordered[j].Source.Source })
	for _, dataset := range ordered {
		b := dataset.Source
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT s.current_revision FROM chartworks.sources s JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(s.tenant_id,s.source_id,s.current_revision) WHERE s.tenant_id=$1 AND s.source_id=$2 AND NOT s.deleted AND r.context_id=$3 FOR SHARE OF s`, tenant, b.Source, b.Context).Scan(&revision); err != nil {
			return err
		}
		if revision != b.SourceRevision {
			return readexec.ErrBinding
		}
	}
	return nil
}

// switchTopicFacets locks the old/new context union in lexical order, seals
// complete target generations and retires removed contexts in this transaction.
func switchTopicFacets(ctx context.Context, tx pgx.Tx, scope store.Scope, id, old string, target []vindex.Generation, archive bool) error {
	previous, err := publishedGenerations(ctx, tx, scope.Tenant(), id, old)
	if err != nil {
		return err
	}
	contexts := map[string]bool{}
	for _, g := range previous {
		contexts[g.Context] = true
	}
	next := map[string]vindex.Generation{}
	for _, g := range target {
		contexts[g.Context] = true
		next[g.Context] = g
	}
	ordered := make([]string, 0, len(contexts))
	for c := range contexts {
		ordered = append(ordered, c)
	}
	sort.Strings(ordered)
	for _, c := range ordered {
		if err = vectorHeadLock(ctx, tx, scope.Tenant(), id, c); err != nil {
			return err
		}
	}
	for _, c := range ordered {
		g, keep := next[c]
		if keep && !archive {
			if err = sealVectorGeneration(ctx, tx, scope, g); err != nil {
				return err
			}
		}
		if keep {
			_, err = tx.Exec(ctx, `UPDATE chartworks.vector_heads SET active_generation=$4,archived=$5,revision=revision+1 WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3`, scope.Tenant(), id, c, g.ID, archive)
		} else {
			_, err = tx.Exec(ctx, `UPDATE chartworks.vector_heads SET archived=true,revision=revision+1 WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3`, scope.Tenant(), id, c)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
func publicationEvent(ctx context.Context, tx pgx.Tx, e identity.Envelope, id, version, kind, note string, expected int64, archive bool) error {
	_, err := tx.Exec(ctx, `UPDATE chartworks.topic_publication_heads SET revision=$3,active_version=$4,archived=$5 WHERE tenant_id=$1 AND topic_id=$2`, e.Tenant(), id, expected+1, version, archive)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_publication_events(tenant_id,topic_id,revision,version_id,archived,kind,actor_id,session_id,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), id, expected+1, version, archive, kind, e.User(), e.Session(), note)
	if err != nil {
		return err
	}
	action := map[string]string{"publish": "topic.published", "rollback": "topic.rolled_back", "archive": "topic.archived"}[kind]
	scope, _ := store.NewScope(e.Tenant(), e.User())
	return auditJob(ctx, tx, scope, action, id)
}
func (d *DB) PublishTopic(ctx context.Context, e identity.Envelope, proof topics.Prepared, generations []vindex.Generation, receipt gateway.Receipt, expected int64) (out topics.Published, err error) {
	if expected < 0 || expected >= 1<<62 {
		return out, store.ErrInvalid
	}
	pack, review, err := proof.Checked(e)
	if err != nil {
		return out, err
	}
	if err = proof.CheckGenerations(e, generations); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	definition := topics.Project(pack)
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		old, err := topicPublicationLock(ctx, tx, e.Tenant(), pack.Topic, expected)
		if err != nil {
			return err
		}
		if old != "" {
			if _, err = readPublishedTx(ctx, tx, e, pack.Topic, old, drafts.Publish); err != nil {
				return err
			}
		}
		actual, draft, err := reviewedTopicTx(ctx, tx, e, pack.Topic, review.ID)
		if err != nil {
			return err
		}
		if actual != review || actual.Decision != "approve" || draft.Metadata.Digest != review.Digest {
			return store.ErrConflict
		}
		raw, _ := json.Marshal(definition)
		usage, _ := json.Marshal(receipt)
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_published_versions(tenant_id,topic_id,version_id,draft_revision,review_id,digest,definition,receipt) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Tenant(), pack.Topic, pack.Version, review.DraftRevision, review.ID, review.Digest, raw, usage)
		if err != nil {
			return err
		}
		for _, dataset := range definition.Datasets {
			b := dataset.Source
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_published_dependencies(tenant_id,topic_id,version_id,dataset_id,source_id,context_id,source_revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), pack.Topic, pack.Version, dataset.ID, b.Source, b.Context, b.SourceRevision)
			if err != nil {
				return err
			}
		}
		for _, g := range generations {
			raw, _ := json.Marshal(g)
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.topic_published_generations(tenant_id,topic_id,version_id,context_id,generation_id,manifest) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), pack.Topic, pack.Version, g.Context, g.ID, raw)
			if err != nil {
				return err
			}
		}
		if err = switchTopicFacets(ctx, tx, scope, pack.Topic, old, generations, false); err != nil {
			return err
		}
		if err = publishedSourceFence(ctx, tx, e.Tenant(), definition); err != nil {
			return err
		}
		if _, _, err = proof.Checked(e); err != nil {
			return err
		}
		if err = publicationEvent(ctx, tx, e, pack.Topic, pack.Version, "publish", "Reviewed publication", expected, false); err != nil {
			return err
		}
		if err = auditJob(ctx, tx, scope, "facets.generation_published", pack.Topic); err != nil {
			return err
		}
		out, err = readPublishedTx(ctx, tx, e, pack.Topic, "", drafts.Publish)
		return err
	})
	return
}
func (d *DB) RollbackTopic(ctx context.Context, e identity.Envelope, id string, in topics.TransitionRequest) (out topics.Published, err error) {
	if in.Expected < 1 || in.Expected >= 1<<62 || !identity.Identifier(in.Version) || !topics.NoteValid(in.Note) {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		old, err := topicPublicationLock(ctx, tx, e.Tenant(), id, in.Expected)
		if err != nil {
			return err
		}
		if _, err = readPublishedTx(ctx, tx, e, id, old, drafts.Publish); err != nil {
			return err
		}
		target, err := readPublishedTx(ctx, tx, e, id, in.Version, drafts.Publish)
		if err != nil {
			return err
		}
		if target.State.Active {
			return store.ErrConflict
		}
		generations, err := publishedGenerations(ctx, tx, e.Tenant(), id, in.Version)
		if err != nil {
			return err
		}
		if err = switchTopicFacets(ctx, tx, scope, id, old, generations, false); err != nil {
			return err
		}
		if err = publishedSourceFence(ctx, tx, e.Tenant(), target.Definition); err != nil {
			return err
		}
		if err = publicationEvent(ctx, tx, e, id, in.Version, "rollback", in.Note, in.Expected, false); err != nil {
			return err
		}
		out, err = readPublishedTx(ctx, tx, e, id, "", drafts.Publish)
		return err
	})
	return
}
func (d *DB) ArchiveTopic(ctx context.Context, e identity.Envelope, id string, expected int64, note string) (out topics.State, err error) {
	if expected < 1 || expected >= 1<<62 || !topics.NoteValid(note) {
		return out, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	scope, _ := store.NewScope(e.Tenant(), e.User())
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		old, err := topicPublicationLock(ctx, tx, e.Tenant(), id, expected)
		if err != nil {
			return err
		}
		current, err := readPublishedTx(ctx, tx, e, id, old, drafts.Publish)
		if err != nil {
			return err
		}
		if current.State.Archived {
			return store.ErrConflict
		}
		generations, err := publishedGenerations(ctx, tx, e.Tenant(), id, old)
		if err != nil {
			return err
		}
		if err = switchTopicFacets(ctx, tx, scope, id, old, generations, true); err != nil {
			return err
		}
		if err = publicationEvent(ctx, tx, e, id, old, "archive", note, expected, true); err != nil {
			return err
		}
		if err = auditJob(ctx, tx, scope, "facets.archived", id); err != nil {
			return err
		}
		out = current.State
		out.Revision = expected + 1
		out.Archived = true
		out.Active = false
		return nil
	})
	return
}
func (d *DB) ConfirmTopicContract(ctx context.Context, e identity.Envelope, id string, expected int64) (out topics.Published, err error) {
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT revision FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR SHARE`, e.Tenant(), id).Scan(&revision); err != nil {
			return err
		}
		if revision != expected {
			return store.ErrConflict
		}
		var err error
		out, err = readPublishedTx(ctx, tx, e, id, "", drafts.Read)
		if err != nil {
			return err
		}
		if out.State.Archived {
			return store.ErrNotFound
		}
		return publishedSourceFence(ctx, tx, e.Tenant(), out.Definition)
	})
	return
}
