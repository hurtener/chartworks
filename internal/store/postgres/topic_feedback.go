package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/topicfeedback"
	"github.com/jackc/pgx/v5"
)

// ReadSemanticFeedback reads protected content only for the exact owning actor
// and session. The NLQ service reauthorizes current source/topic reach next.
func (d *DB) ReadSemanticFeedback(ctx context.Context, e identity.Envelope, id string) (out nlqexec.FeedbackRecord, err error) {
	if !e.Has("feedback.write") || !e.Has("topics.write") {
		return out, access.ErrForbidden
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT feedback_id,query_id,session_id,verdict,COALESCE(correction_sql,''),COALESCE(note,''),provenance,created_at FROM chartworks.nlq_feedback WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND feedback_id=$4`, e.Tenant(), e.User(), e.Session(), id).Scan(&out.ID, &out.QueryID, &out.Session, &out.Verdict, &out.Correction, &out.Note, &out.Provenance, &out.Created)
	})
	return
}
func scanTopicFeedback(row pgx.Row) (out topicfeedback.Record, err error) {
	var p, c []byte
	var applied *int64
	var erased bool
	if err = row.Scan(&p, &c, &applied, &erased); err != nil {
		return
	}
	if json.Unmarshal(p, &out.Proposal) != nil || json.Unmarshal(c, &out.Candidate) != nil {
		return out, store.ErrInvalid
	}
	model, e := semantics.Compile(out.Candidate)
	if e != nil || model.Digest() != out.Proposal.CandidateDigest || out.Candidate.Topic != out.Proposal.Topic {
		return out, store.ErrInvalid
	}
	out.Proposal.OriginErased = erased
	if erased {
		out.Proposal.State = "origin_erased"
	}
	if applied != nil {
		out.Proposal.State = "applied"
		out.Proposal.AppliedRevision = *applied
	}
	return
}
func topicFeedbackTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, id string) (topicfeedback.Record, error) {
	if !e.Has("feedback.write") {
		return topicfeedback.Record{}, access.ErrForbidden
	}
	t, err := access.Constrain(e, "topics.write", "topic", "write")
	if err != nil {
		return topicfeedback.Record{}, err
	}
	// Do not select protected proposal/candidate bytes before topic and complete
	// draft-dependency reach have passed the ordinary private draft read seam.
	var topic string
	var revision int64
	err = tx.QueryRow(ctx, `SELECT topic_id,draft_revision FROM chartworks.topic_feedback_proposals WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND proposal_id=$4 AND ($5 OR topic_id=ANY($6::text[]))`, e.Tenant(), e.User(), e.Session(), id, t.All(), t.IDs()).Scan(&topic, &revision)
	if err != nil {
		return topicfeedback.Record{}, err
	}
	if _, err = topicTx(ctx, tx, e, topic, revision, drafts.Write); err != nil {
		return topicfeedback.Record{}, err
	}
	return scanTopicFeedback(tx.QueryRow(ctx, `SELECT proposal,candidate,applied_revision,EXISTS(SELECT 1 FROM chartworks.topic_feedback_origin_tombstones t WHERE (t.tenant_id,t.proposal_id)=(p.tenant_id,p.proposal_id)) FROM chartworks.topic_feedback_proposals p WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND proposal_id=$4`, e.Tenant(), e.User(), e.Session(), id))
}
func (d *DB) ReadTopicFeedback(ctx context.Context, e identity.Envelope, id string) (out topicfeedback.Record, err error) {
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error { out, err = topicFeedbackTx(ctx, tx, e, id); return err })
	return
}

// topicFeedbackOriginFence protects current publication and exact immutable
// feedback/query revision. Source/profile fences run in normal draft admission.
func topicFeedbackOriginFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, p topicfeedback.Proposal, wantQuery int64) (int64, error) {
	var digest string
	err := tx.QueryRow(ctx, `SELECT v.digest FROM chartworks.topic_publication_heads h JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version) WHERE h.tenant_id=$1 AND h.topic_id=$2 AND h.active_version=$3 AND NOT h.archived FOR SHARE OF h`, e.Tenant(), p.Topic, p.Origin.TopicVersion).Scan(&digest)
	if err != nil {
		return 0, err
	}
	if digest != p.Origin.PublicationDigest {
		return 0, readexec.ErrBinding
	}
	var revision int64
	err = tx.QueryRow(ctx, `SELECT q.revision FROM chartworks.nlq_feedback f JOIN chartworks.nlq_queries q ON(q.tenant_id,q.actor_id,q.session_id,q.query_id)=(f.tenant_id,f.actor_id,f.session_id,f.query_id) WHERE f.tenant_id=$1 AND f.actor_id=$2 AND f.session_id=$3 AND f.feedback_id=$4 AND q.query_id=$5 FOR SHARE OF q`, e.Tenant(), e.User(), e.Session(), p.Origin.FeedbackID, p.Origin.QueryID).Scan(&revision)
	if err != nil {
		return 0, err
	}
	if wantQuery < 1 || revision != wantQuery {
		return 0, readexec.ErrBinding
	}
	return revision, nil
}
func (d *DB) PutTopicFeedback(ctx context.Context, e identity.Envelope, r topicfeedback.Record) (out topicfeedback.Record, err error) {
	if err = drafts.RequirePack(e, r.Candidate, drafts.Write); err != nil {
		return out, err
	}
	if !e.Has("feedback.write") {
		return out, access.ErrForbidden
	}
	p := r.Proposal
	if !identity.Identifier(p.ID) || p.State != "proposed" || p.AppliedRevision != 0 || p.OriginErased {
		return out, store.ErrInvalid
	}
	model, err := semantics.Compile(r.Candidate)
	if err != nil || model.Digest() != p.CandidateDigest {
		return out, store.ErrInvalid
	}
	body, err := json.Marshal(p)
	if err != nil {
		return out, err
	}
	candidate, err := json.Marshal(r.Candidate)
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,721415))`, e.Tenant()); err != nil {
			return err
		}
		var current int64
		if err := tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.topic_draft_heads WHERE tenant_id=$1 AND topic_id=$2 AND actor_id=$3 AND session_id=$4 FOR SHARE`, e.Tenant(), p.Topic, e.User(), e.Session()).Scan(&current); err != nil {
			return err
		}
		if current != p.DraftRevision {
			return store.ErrConflict
		}
		draft, err := topicTx(ctx, tx, e, p.Topic, current, drafts.Write)
		if err != nil {
			return err
		}
		if draft.Metadata.Digest != p.DraftDigest {
			return store.ErrConflict
		}
		if err = topicFeedbackProfileFence(ctx, tx, e, draft.Pack); err != nil {
			return err
		}
		if err = topicfeedback.ValidateRecord(draft.Pack, r); err != nil {
			return err
		}
		revision, err := topicFeedbackOriginFence(ctx, tx, e, p, p.Origin.QueryRevision)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO chartworks.topic_feedback_proposals(tenant_id,proposal_id,actor_id,session_id,topic_id,feedback_id,draft_revision,query_revision,request_digest,proposal_digest,candidate_digest,proposal,candidate) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT DO NOTHING`, e.Tenant(), p.ID, e.User(), e.Session(), p.Topic, p.Origin.FeedbackID, p.DraftRevision, revision, p.RequestDigest, p.Digest, p.CandidateDigest, body, candidate)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			sc, scopeErr := store.NewScope(e.Tenant(), e.User())
			if scopeErr != nil {
				return scopeErr
			}
			if err = auditJob(ctx, tx, sc, "topic.feedback_proposed", p.ID); err != nil {
				return err
			}
		}
		out, err = topicFeedbackTx(ctx, tx, e, p.ID)
		if err != nil {
			return err
		}
		if out.Proposal.RequestDigest != p.RequestDigest {
			return store.ErrConflict
		}
		return nil
	})
	return
}
func fenceTopicFeedbackApplication(ctx context.Context, tx pgx.Tx, e identity.Envelope, a drafts.FeedbackApplication, topic, digest string, expected int64) error {
	r, err := topicFeedbackTx(ctx, tx, e, a.ID)
	if err != nil {
		return err
	}
	p := r.Proposal
	if p.Topic != topic || p.Digest != a.Digest || p.CandidateDigest != digest || p.DraftRevision != expected || p.State != "proposed" {
		return store.ErrConflict
	}
	var queryRevision int64
	if err = tx.QueryRow(ctx, `SELECT query_revision FROM chartworks.topic_feedback_proposals WHERE tenant_id=$1 AND proposal_id=$2 FOR UPDATE`, e.Tenant(), a.ID).Scan(&queryRevision); err != nil {
		return err
	}
	_, err = topicFeedbackOriginFence(ctx, tx, e, p, queryRevision)
	return err
}
func markTopicFeedbackApplication(ctx context.Context, tx pgx.Tx, e identity.Envelope, a drafts.FeedbackApplication, revision int64) error {
	tag, err := tx.Exec(ctx, `UPDATE chartworks.topic_feedback_proposals SET applied_revision=$5 WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND proposal_id=$4 AND applied_revision IS NULL`, e.Tenant(), e.User(), e.Session(), a.ID, revision)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return store.ErrConflict
	}
	return nil
}

func (d *DB) ListSemanticFeedback(ctx context.Context, e identity.Envelope, query string) (out []nlqexec.SemanticFeedbackReference, err error) {
	if !e.Has("feedback.write") || !e.Has("topics.write") {
		return nil, access.ErrForbidden
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return nil, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT feedback_id,verdict,correction_sql IS NOT NULL,created_at FROM chartworks.nlq_feedback WHERE tenant_id=$1 AND actor_id=$2 AND session_id=$3 AND query_id=$4 AND (verdict='negative' OR correction_sql IS NOT NULL) ORDER BY created_at,feedback_id LIMIT 64`, e.Tenant(), e.User(), e.Session(), query)
		if err != nil {
			return err
		}
		defer rows.Close()
		out = []nlqexec.SemanticFeedbackReference{}
		for rows.Next() {
			var r nlqexec.SemanticFeedbackReference
			if err = rows.Scan(&r.ID, &r.Verdict, &r.Corrected, &r.Created); err != nil {
				return err
			}
			out = append(out, r)
		}
		return rows.Err()
	})
	return
}

// Proposal creation fences current metadata origins as well as its later apply.
// Sampling statistics do not become uniqueness or execution proof.
func topicFeedbackProfileFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, p semantics.TopicPack) error {
	for _, d := range p.Datasets {
		var revision int64
		if err := tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 AND NOT deleted FOR SHARE`, e.Tenant(), d.Source.Source).Scan(&revision); err != nil {
			return err
		}
		if revision != d.Source.SourceRevision {
			return readexec.ErrBinding
		}
		var digest string
		err := tx.QueryRow(ctx, `SELECT v.deterministic_hash FROM chartworks.profile_versions v JOIN chartworks.profile_heads h ON(h.tenant_id,h.actor_id,h.session_id,h.source_id,h.dataset_id,h.profile_id)=(v.tenant_id,v.actor_id,v.session_id,v.source_id,v.dataset_id,v.profile_id) WHERE v.tenant_id=$1 AND v.actor_id=$2 AND v.session_id=$3 AND v.profile_id=$4 AND v.source_id=$5 AND v.context_id=$6 AND v.dataset_id=$7 AND v.state='complete' FOR SHARE OF v,h`, e.Tenant(), e.User(), e.Session(), d.Source.ProfileVersion, d.Source.Source, d.Source.Context, d.ID).Scan(&digest)
		if err != nil {
			return err
		}
		if digest != d.Source.ProfileDigest {
			return readexec.ErrBinding
		}
	}
	return nil
}

// tombstoneDocumentFeedbackOrigins runs under the same tenant fence as proposal
// creation/application. Only identity/provenance markers survive owned erasure.
func tombstoneDocumentFeedbackOrigins(ctx context.Context, tx pgx.Tx, tenant string, roots []string) error {
	_, err := tx.Exec(ctx, `INSERT INTO chartworks.topic_feedback_origin_tombstones(tenant_id,proposal_id,reason)
 SELECT p.tenant_id,p.proposal_id,'document_deleted' FROM chartworks.topic_feedback_proposals p
 JOIN chartworks.nlq_feedback f ON(f.tenant_id,f.feedback_id)=(p.tenant_id,p.feedback_id)
 JOIN chartworks.nlq_queries q ON(q.tenant_id,q.actor_id,q.session_id,q.query_id)=(f.tenant_id,f.actor_id,f.session_id,f.query_id)
 WHERE q.tenant_id=$1 AND EXISTS(SELECT 1 FROM unnest($2::text[]) root WHERE q.operation LIKE 'composition:'||root||':%')
 ON CONFLICT DO NOTHING`, tenant, roots)
	return err
}
