package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func nullableRevision(revision int64) *int64 {
	if revision == 0 {
		return nil
	}
	return &revision
}

func documentAudit(operation string) string {
	return map[string]string{"create": "document.created", "edit": "document.edited", "review": "document.reviewed", "publish": "document.published", "reject": "document.rejected", "archive": "document.archived", "import": "document.imported"}[operation]
}

func insertDocumentReference(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.DocumentMutation, ref reporting.ResourceReference) error {
	if err := access.Require(e, "reporting.write", access.Resource{Tenant: e.Tenant(), Kind: ref.Kind, Permission: ref.Permission, ID: ref.ID}); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO chartworks.document_references(tenant_id,kind,document_id,revision,resource_kind,permission,resource_id)
 VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT DO NOTHING`, e.Tenant(), m.Kind, m.ID, m.Revision.Number, ref.Kind, ref.Permission, ref.ID)
	return err
}

func insertDocumentQuery(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.DocumentMutation, w reporting.Widget, origin reporting.QueryOrigin) error {
	if origin.Widget != w.ID || origin.Context != w.Query.Context || origin.Source == "" || len(origin.Topics) != len(w.Query.Topics) {
		return store.ErrInvalid
	}
	refs := []reporting.ResourceReference{{Kind: "source", Permission: "read", ID: origin.Source}, {Kind: "execution_context", Permission: "use", ID: origin.Context}}
	for i, pin := range origin.Topics {
		if pin != w.Query.Topics[i] {
			return store.ErrInvalid
		}
		if err := access.Require(e, "reporting.write", access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: pin.Topic}); err != nil {
			return err
		}
		var digest string
		if err := tx.QueryRow(ctx, `SELECT digest FROM chartworks.topic_published_versions WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, e.Tenant(), pin.Topic, pin.Version).Scan(&digest); err != nil {
			return err
		}
		if digest != pin.Digest {
			return reporting.ErrStale
		}
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.document_topic_refs(tenant_id,kind,document_id,revision,widget_id,topic_id,version_id,digest) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Tenant(), m.Kind, m.ID, m.Revision.Number, w.ID, pin.Topic, pin.Version, pin.Digest)
		if err != nil {
			return err
		}
		refs = append(refs, reporting.ResourceReference{Kind: "topic", Permission: "read", ID: pin.Topic})
	}
	if w.Query.Durability == "session_bound" {
		if origin.Query != w.Query.Query || origin.Actor != e.User() || origin.Session != e.Session() {
			return access.ErrNotFound
		}
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.document_query_refs(tenant_id,kind,document_id,revision,widget_id,actor_id,session_id,query_id,query_digest)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), m.Kind, m.ID, m.Revision.Number, w.ID, origin.Actor, origin.Session, origin.Query, origin.QueryDigest)
		if err != nil {
			return err
		}
	}
	for _, ref := range refs {
		if err := insertDocumentReference(ctx, tx, e, m, ref); err != nil {
			return err
		}
	}
	return nil
}

func insertDocumentLinks(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.DocumentMutation, definition reporting.DocumentDefinition) error {
	origins := map[string]reporting.QueryOrigin{}
	for _, origin := range m.Revision.Origins {
		if _, ok := origins[origin.Widget]; ok {
			return store.ErrInvalid
		}
		origins[origin.Widget] = origin
	}
	queryCount := 0
	for _, widget := range definition.Widgets {
		switch widget.Kind {
		case "block":
			w := widget.Block
			block, err := blockTx(ctx, tx, e, w.Block, reporting.Reference{Revision: w.Revision}, reporting.Read)
			if err != nil {
				return err
			}
			if block.PublishedAt == nil || block.State.Archived {
				return reporting.ErrStale
			}
			if _, err := reporting.SelectOutputs(block.Revision.Definition.Outputs, w.Outputs); err != nil {
				return err
			}
			if err := reporting.ValidateWidgetBindings(block.Revision.Definition.Parameters, definition, widget); err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO chartworks.document_block_refs(tenant_id,kind,document_id,revision,widget_id,block_id,pinned_revision,observed_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Tenant(), m.Kind, m.ID, m.Revision.Number, widget.ID, w.Block, nullableRevision(w.Revision), block.Revision.Number)
			if err != nil {
				return err
			}
			refs := append([]reporting.ResourceReference{{Kind: "block", Permission: "read", ID: w.Block}}, block.References...)
			for _, ref := range refs {
				if err := insertDocumentReference(ctx, tx, e, m, ref); err != nil {
					return err
				}
			}
		case "query":
			origin, ok := origins[widget.ID]
			if !ok {
				return store.ErrInvalid
			}
			if err := insertDocumentQuery(ctx, tx, e, m, widget, origin); err != nil {
				return err
			}
			queryCount++
		}
	}
	if queryCount != len(origins) {
		return store.ErrInvalid
	}
	for ordinal, page := range definition.Pages {
		child, err := documentTx(ctx, tx, e, "report", page.Report, reporting.DocumentReference{Revision: page.Revision}, reporting.Read, false)
		if err != nil {
			return err
		}
		if child.PublishedAt == nil || child.State.Archived {
			return reporting.ErrStale
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.document_page_refs(tenant_id,kind,document_id,revision,ordinal,page_id,title,report_id,report_revision) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), m.Kind, m.ID, m.Revision.Number, ordinal, page.ID, page.Title, page.Report, page.Revision)
		if err != nil {
			return err
		}
	}
	return nil
}

func insertDocumentRevision(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.DocumentMutation) error {
	r := m.Revision
	if r == nil || r.Actor != e.User() || r.Session != e.Session() || reporting.DocumentDigest(r.Raw) != r.Digest {
		return store.ErrInvalid
	}
	definition, err := reporting.ProjectStoredDocument(r.Raw, m.Kind)
	if err != nil || m.Kind == "dashboard" && len(definition.Pages) == 0 {
		return store.ErrInvalid
	}
	origins, err := json.Marshal(r.Origins)
	if err != nil {
		return store.ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.document_revisions(tenant_id,kind,document_id,revision,definition,digest,actor_id,session_id,origins,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, e.Tenant(), m.Kind, m.ID, r.Number, r.Raw, r.Digest, e.User(), e.Session(), origins, r.Created)
	if err != nil {
		return err
	}
	if err := insertDocumentLinks(ctx, tx, e, m, definition); err != nil {
		return err
	}
	if r.External != nil {
		ref := r.External
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.document_external_refs(tenant_id,kind,external_system,external_id,external_version,document_id,revision) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), m.Kind, ref.System, ref.ID, ref.Version, m.ID, r.Number)
	}
	return err
}

func existingDocumentImport(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.DocumentMutation) (reporting.DocumentState, bool, error) {
	if m.Operation != "import" || m.Revision == nil || m.Revision.External == nil {
		return reporting.DocumentState{}, false, nil
	}
	ref := m.Revision.External
	var digest string
	err := tx.QueryRow(ctx, `SELECT r.digest FROM chartworks.document_external_refs external_ref
 JOIN chartworks.document_revisions r ON(r.tenant_id,r.kind,r.document_id,r.revision)=(external_ref.tenant_id,external_ref.kind,external_ref.document_id,external_ref.revision)
 WHERE external_ref.tenant_id=$1 AND external_ref.kind=$2 AND external_ref.document_id=$3 AND external_ref.external_system=$4 AND external_ref.external_id=$5 AND external_ref.external_version=$6`, e.Tenant(), m.Kind, m.ID, ref.System, ref.ID, ref.Version).Scan(&digest)
	if errors.Is(err, pgx.ErrNoRows) {
		return reporting.DocumentState{}, false, nil
	}
	if err != nil {
		return reporting.DocumentState{}, false, err
	}
	if digest != m.Revision.Digest {
		return reporting.DocumentState{}, false, store.ErrConflict
	}
	state, err := scanDocumentState(tx.QueryRow(ctx, `SELECT `+documentHeadColumns+` FROM chartworks.document_heads h WHERE h.tenant_id=$1 AND h.kind=$2 AND h.document_id=$3`, e.Tenant(), m.Kind, m.ID))
	return state, true, err
}

func documentTransition(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.DocumentMutation, head reporting.DocumentState) (reporting.DocumentState, error) {
	if head.Archived || head.Version != m.ExpectedVersion || head.Version >= 4096 {
		return reporting.DocumentState{}, store.ErrConflict
	}
	var published bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.document_publications p WHERE (p.tenant_id,p.kind,p.document_id,p.revision)=(r.tenant_id,r.kind,r.document_id,r.revision))
 FROM chartworks.document_revisions r WHERE r.tenant_id=$1 AND r.kind=$2 AND r.document_id=$3 AND r.revision=$4`, e.Tenant(), m.Kind, m.ID, m.TargetRevision).Scan(&published); err != nil {
		return reporting.DocumentState{}, err
	}
	switch m.Operation {
	case "edit":
		if m.Revision == nil || m.Revision.Number != head.LatestRevision+1 {
			return reporting.DocumentState{}, store.ErrConflict
		}
		if err := insertDocumentRevision(ctx, tx, e, m); err != nil {
			return reporting.DocumentState{}, err
		}
		head.LatestRevision, head.DraftRevision = m.Revision.Number, m.Revision.Number
	case "review":
		if published || head.ReviewRevision != 0 || head.DraftRevision != m.TargetRevision {
			return reporting.DocumentState{}, store.ErrConflict
		}
		var rejected bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM chartworks.document_events WHERE tenant_id=$1 AND kind=$2 AND document_id=$3 AND revision=$4 AND operation='reject')`, e.Tenant(), m.Kind, m.ID, m.TargetRevision).Scan(&rejected); err != nil {
			return reporting.DocumentState{}, err
		}
		if rejected {
			return reporting.DocumentState{}, store.ErrConflict
		}
		head.ReviewRevision, head.DraftRevision = m.TargetRevision, 0
	case "publish":
		if published || head.ReviewRevision != m.TargetRevision {
			return reporting.DocumentState{}, store.ErrConflict
		}
		if _, err := documentTx(ctx, tx, e, m.Kind, m.ID, reporting.DocumentReference{Revision: m.TargetRevision}, reporting.Publish, false); err != nil {
			return reporting.DocumentState{}, err
		}
		_, err := tx.Exec(ctx, `INSERT INTO chartworks.document_publications(tenant_id,kind,document_id,revision,actor_id) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), m.Kind, m.ID, m.TargetRevision, e.User())
		if err != nil {
			return reporting.DocumentState{}, err
		}
		head.PublishedRevision, head.ReviewRevision = m.TargetRevision, 0
	case "reject":
		if published || head.ReviewRevision != m.TargetRevision || m.Note == "" {
			return reporting.DocumentState{}, store.ErrConflict
		}
		head.ReviewRevision = 0
		if head.DraftRevision == 0 {
			head.DraftRevision = m.TargetRevision
		}
	case "archive":
		if m.Note == "" {
			return reporting.DocumentState{}, store.ErrInvalid
		}
		head.Archived = true
	default:
		return reporting.DocumentState{}, store.ErrInvalid
	}
	head.Version++
	return scanDocumentState(tx.QueryRow(ctx, `UPDATE chartworks.document_heads h SET version=$4,latest_revision=$5,draft_revision=$6,review_revision=$7,published_revision=$8,archived=$9,updated_at=clock_timestamp()
 WHERE h.tenant_id=$1 AND h.kind=$2 AND h.document_id=$3 RETURNING `+documentHeadColumns, e.Tenant(), m.Kind, m.ID, head.Version, head.LatestRevision, nullableRevision(head.DraftRevision), nullableRevision(head.ReviewRevision), nullableRevision(head.PublishedRevision), head.Archived))
}

// CommitDocument atomically commits immutable content, composite references,
// lifecycle pointers and audit under CAS and fresh authority at both boundaries.
func (d *DB) CommitDocument(ctx context.Context, e identity.Envelope, proof reporting.PreparedDocument) (out reporting.DocumentState, err error) {
	m, err := proof.Checked(e)
	if err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := proof.Checked(e); err != nil {
			return err
		}
		if slices.Contains([]string{"create", "import"}, m.Operation) {
			if m.ExpectedVersion != 0 || m.Revision == nil || m.Revision.Number != 1 {
				return store.ErrInvalid
			}
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "documents:"+e.Tenant()); err != nil {
				return err
			}
			if existing, found, err := existingDocumentImport(ctx, tx, e, m); err != nil {
				return err
			} else if found {
				out = existing
				_, err := proof.Checked(e)
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.document_heads WHERE tenant_id=$1 AND kind=$2`, e.Tenant(), m.Kind).Scan(&count); err != nil {
				return err
			}
			if count >= m.MaxDocuments {
				return reporting.ErrBudget
			}
			out, err = scanDocumentState(tx.QueryRow(ctx, `INSERT INTO chartworks.document_heads AS h(tenant_id,kind,document_id,version,latest_revision,draft_revision) VALUES($1,$2,$3,1,1,1) RETURNING `+documentHeadColumns, e.Tenant(), m.Kind, m.ID))
			if err != nil {
				return err
			}
			if err := insertDocumentRevision(ctx, tx, e, m); err != nil {
				return err
			}
		} else {
			head, err := scanDocumentState(tx.QueryRow(ctx, `SELECT `+documentHeadColumns+` FROM chartworks.document_heads h WHERE h.tenant_id=$1 AND h.kind=$2 AND h.document_id=$3 FOR UPDATE`, e.Tenant(), m.Kind, m.ID))
			if err != nil {
				return err
			}
			out, err = documentTransition(ctx, tx, e, m, head)
			if err != nil {
				return err
			}
		}
		revision := m.TargetRevision
		if m.Revision != nil {
			revision = m.Revision.Number
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.document_events(tenant_id,kind,document_id,version,revision,operation,actor_id,note) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, e.Tenant(), m.Kind, m.ID, out.Version, revision, m.Operation, e.User(), m.Note); err != nil {
			return err
		}
		action := documentAudit(m.Operation)
		if action == "" {
			return store.ErrInvalid
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, action, m.ID); err != nil {
			return err
		}
		_, err := proof.Checked(e)
		return err
	})
	if err != nil {
		return reporting.DocumentState{}, err
	}
	if !e.Valid() {
		return reporting.DocumentState{}, access.ErrUnauthenticated
	}
	return out, nil
}

// QuarantineDocument retains only bounded private import failures. It creates no
// executable definition or discoverable public metadata and caps accumulation.
func (d *DB) QuarantineDocument(ctx context.Context, e identity.Envelope, proof reporting.PreparedQuarantine) (string, error) {
	q, err := proof.Checked(e)
	if err != nil {
		return "", err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return "", err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := proof.Checked(e); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "document-quarantine:"+e.Tenant()); err != nil {
			return err
		}
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.document_quarantine WHERE tenant_id=$1`, e.Tenant()).Scan(&count); err != nil {
			return err
		}
		if count >= 1000 {
			return reporting.ErrBudget
		}
		external, err := json.Marshal(q.External)
		if err != nil {
			return store.ErrInvalid
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.document_quarantine(tenant_id,actor_id,quarantine_id,kind,target_id,definition,digest,reason,external_reference) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), e.User(), q.ID, q.Kind, q.Target, q.Raw, q.Digest, q.Reason, external)
		if err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "document.quarantined", q.Target); err != nil {
			return err
		}
		_, err = proof.Checked(e)
		return err
	})
	if err != nil {
		return "", err
	}
	if !e.Valid() {
		return "", access.ErrUnauthenticated
	}
	return q.ID, nil
}
