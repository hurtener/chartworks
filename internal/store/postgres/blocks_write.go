package postgres

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func blockAudit(kind string) string {
	return map[string]string{"create": "created", "capture": "captured", "edit": "edited", "restore": "restored", "rename": "renamed", "parameterize": "parameterized", "validate": "validated", "publish": "published", "certify": "certified", "withdraw": "withdrawn", "reject": "rejected", "archive": "archived", "health": "health_checked"}[kind]
}

func insertBlockRevision(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.Mutation) error {
	r := m.Revision
	if r == nil || len(r.Definition.Topics) == 0 || r.Actor != e.User() || r.Number < 1 || r.Number > int64(m.MaxRevisions) || r.Definition.Topics[0].Topic != m.Topic || reporting.DefinitionDigest(r.Definition) != r.Digest || reporting.ExecutionDigest(r.Definition) != r.ExecutionDigest {
		return store.ErrInvalid
	}
	raw, err := json.Marshal(r.Definition)
	if err != nil {
		return store.ErrInvalid
	}
	provenance, err := json.Marshal(r.Provenance)
	if err != nil {
		return store.ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_revisions(tenant_id,block_id,revision,revision_id,definition,digest,execution_digest,actor_id,session_id,provenance,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, e.Tenant(), m.ID, r.Number, r.ID, raw, r.Digest, r.ExecutionDigest, e.User(), e.Session(), provenance, r.CreatedAt)
	if err != nil {
		return err
	}
	for _, ref := range m.References {
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_revision_references(tenant_id,block_id,revision,kind,permission,resource_id) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), m.ID, r.Number, ref.Kind, ref.Permission, ref.ID)
		if err != nil {
			return err
		}
	}
	// Public topic rows, not a caller manifest, supply the source revision pins.
	sources := map[string]topics.Binding{}
	for _, pin := range r.Definition.Topics {
		var actual string
		var definition []byte
		if err := tx.QueryRow(ctx, `SELECT digest,definition FROM chartworks.topic_published_versions WHERE tenant_id=$1 AND topic_id=$2 AND version_id=$3`, e.Tenant(), pin.Topic, pin.Version).Scan(&actual, &definition); err != nil {
			return err
		}
		if actual != pin.Digest {
			return reporting.ErrStale
		}
		var d topics.Definition
		if json.Unmarshal(definition, &d) != nil {
			return store.ErrInvalid
		}
		for _, dataset := range d.Datasets {
			b := dataset.Source
			if b.Source != r.Definition.Source || b.Context != r.Definition.Context || b.Dataset != dataset.ID {
				return store.ErrInvalid
			}
			if old, ok := sources[b.Source]; ok && (old.Context != b.Context || old.SourceRevision != b.SourceRevision) {
				return store.ErrInvalid
			}
			sources[b.Source] = b
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_topic_pins(tenant_id,block_id,revision,topic_id,version_id,digest) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), m.ID, r.Number, pin.Topic, pin.Version, pin.Digest)
		if err != nil {
			return err
		}
	}
	if len(sources) == 0 {
		return store.ErrInvalid
	}
	for _, b := range sources {
		var contextID string
		if err := tx.QueryRow(ctx, `SELECT context_id FROM chartworks.source_revisions WHERE tenant_id=$1 AND source_id=$2 AND revision=$3`, e.Tenant(), b.Source, b.SourceRevision).Scan(&contextID); err != nil {
			return err
		}
		if contextID != b.Context {
			return store.ErrInvalid
		}
		_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_source_pins(tenant_id,block_id,revision,source_id,source_revision,context_id) VALUES($1,$2,$3,$4,$5,$6)`, e.Tenant(), m.ID, r.Number, b.Source, b.SourceRevision, b.Context)
		if err != nil {
			return err
		}
	}
	return nil
}

// Topic locks precede source locks, matching topic publication's lock order.
// These shared locks last until the block pointer/evidence/audit transaction ends.
func blockCurrentFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.Mutation) error {
	pins := append([]reporting.TopicPin(nil), m.Topics...)
	sort.Slice(pins, func(i, j int) bool { return pins[i].Topic < pins[j].Topic })
	if len(pins) == 0 || len(m.Watch) == 0 {
		return store.ErrInvalid
	}
	for _, pin := range pins {
		var version, digest string
		var archived bool
		if err := tx.QueryRow(ctx, `SELECT COALESCE(h.active_version,''),h.archived,COALESCE(v.digest,'') FROM chartworks.topic_publication_heads h LEFT JOIN chartworks.topic_published_versions v ON(v.tenant_id,v.topic_id,v.version_id)=(h.tenant_id,h.topic_id,h.active_version) WHERE h.tenant_id=$1 AND h.topic_id=$2 FOR SHARE OF h`, e.Tenant(), pin.Topic).Scan(&version, &archived, &digest); err != nil {
			return err
		}
		if archived || version != pin.Version || digest != pin.Digest {
			return reporting.ErrStale
		}
	}
	watched := map[string]reporting.Dependency{}
	for _, dep := range m.Watch {
		if old, ok := watched[dep.Source]; ok && (old.Context != dep.Context || old.SourceRevision != dep.SourceRevision) {
			return store.ErrInvalid
		}
		watched[dep.Source] = dep
	}
	ids := make([]string, 0, len(watched))
	for id := range watched {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		dep := watched[id]
		var revision int64
		var contextID string
		if err := tx.QueryRow(ctx, `SELECT h.current_revision,r.context_id FROM chartworks.sources h JOIN chartworks.source_revisions r ON(r.tenant_id,r.source_id,r.revision)=(h.tenant_id,h.source_id,h.current_revision) WHERE h.tenant_id=$1 AND h.source_id=$2 AND NOT h.deleted FOR SHARE OF h`, e.Tenant(), id).Scan(&revision, &contextID); err != nil {
			return err
		}
		if revision != dep.SourceRevision || contextID != dep.Context {
			return reporting.ErrStale
		}
	}
	return nil
}

func verifyBlockAttempt(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.Mutation, snapshot reporting.Snapshot) error {
	v := m.Validation
	if v == nil {
		return store.ErrInvalid
	}
	a := v.Evidence.Attempt
	if v.Evidence.Revision != snapshot.Revision.Number || v.Evidence.RevisionID != snapshot.Revision.ID || v.Evidence.DefinitionDigest != snapshot.Revision.Digest || v.Evidence.ExecutionDigest != snapshot.Revision.ExecutionDigest || v.Evidence.Actor != e.User() || v.Evidence.DependencyDigest != reporting.DependencyDigest(v.Dependencies, v.Topics) || v.Evidence.SchemaDigest != readexec.Hash(v.Evidence.Schema) || v.Evidence.ValidationManifest != a.Manifest.Receipt.Manifest || a.Manifest.Session != e.Session() || !a.Manifest.Preview || !a.Manifest.Valid() || v.BindingDigest != readexec.Hash(v.Binding) || v.Evidence.ExpiresAt.After(v.Evidence.CreatedAt.Add(7*24*time.Hour)) || !time.Now().Before(v.Evidence.ExpiresAt) {
		return store.ErrInvalid
	}
	manifest, err := json.Marshal(a.Manifest)
	if err != nil {
		return store.ErrInvalid
	}
	var actual string
	err = tx.QueryRow(ctx, `SELECT attempt_id FROM chartworks.read_attempts WHERE tenant_id=$1 AND actor_id=$2 AND attempt_id=$3 AND source_id=$4 AND context_id=$5 AND manifest=$6::jsonb AND status=$7 AND status IN('succeeded','empty','truncated') AND remote_state='stopped' AND finished_at IS NOT NULL AND rows_returned=$8 AND bytes_returned=$9 FOR SHARE`, e.Tenant(), e.User(), a.ID, snapshot.Revision.Definition.Source, snapshot.Revision.Definition.Context, manifest, a.Status, a.Rows, a.Bytes).Scan(&actual)
	return err
}

func blockFresh(ctx context.Context, tx pgx.Tx, e identity.Envelope, m reporting.Mutation, snapshot reporting.Snapshot) (reporting.ValidationRecord, error) {
	var raw []byte
	var v reporting.ValidationRecord
	if err := tx.QueryRow(ctx, `SELECT record FROM chartworks.block_validations WHERE tenant_id=$1 AND block_id=$2 AND revision=$3 AND evidence_id=$4 AND expires_at>clock_timestamp()`, e.Tenant(), m.ID, m.TargetRevision, m.Evidence).Scan(&raw); err != nil {
		return v, err
	}
	if json.Unmarshal(raw, &v) != nil || snapshot.Health.Status != "healthy" || snapshot.Health.DependencyDigest != v.Evidence.DependencyDigest || v.Evidence.DefinitionDigest != snapshot.Revision.Digest || v.Evidence.RevisionID != snapshot.Revision.ID || v.Evidence.ExecutionDigest != snapshot.Revision.ExecutionDigest || v.Evidence.DependencyDigest != reporting.DependencyDigest(m.Watch, m.Topics) || v.Evidence.CanonicalizationVersion != reporting.CanonicalizationVersion {
		return v, reporting.ErrStale
	}
	return v, nil
}

func setBlockHealth(ctx context.Context, tx pgx.Tx, tenant, id string, revision int64, health reporting.Health) error {
	raw, err := json.Marshal(health)
	if err != nil {
		return store.ErrInvalid
	}
	_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_health(tenant_id,block_id,revision,observation) VALUES($1,$2,$3,$4) ON CONFLICT(tenant_id,block_id,revision) DO UPDATE SET observation=EXCLUDED.observation`, tenant, id, revision, raw)
	return err
}

// CommitBlock is the only block write boundary. Prepared is service-issued and
// detached from request aliases. Head, immutable content, evidence, publication,
// attestation and audit transitions are committed atomically under CAS.
func (d *DB) CommitBlock(ctx context.Context, e identity.Envelope, proof reporting.Prepared) (out reporting.State, err error) {
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
		creating := m.Kind == "create" || m.Kind == "capture"
		var snapshot reporting.Snapshot
		if creating {
			if m.ExpectedVersion != 0 || m.Revision == nil || m.Revision.Number != 1 {
				return store.ErrInvalid
			}
			if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "blocks:"+e.Tenant()); err != nil {
				return err
			}
			var count int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.block_heads WHERE tenant_id=$1`, e.Tenant()).Scan(&count); err != nil {
				return err
			}
			if count >= m.MaxBlocks {
				return readexec.ErrLimit
			}
			out, err = scanBlockHead(tx.QueryRow(ctx, `INSERT INTO chartworks.block_heads AS h(tenant_id,block_id,topic_id,version,draft_revision,draft_state) VALUES($1,$2,$3,1,1,'draft') RETURNING `+blockHeadColumns, e.Tenant(), m.ID, m.Topic))
			if err != nil {
				return err
			}
			if err := insertBlockRevision(ctx, tx, e, m); err != nil {
				return err
			}
		} else {
			head, err := scanBlockHead(tx.QueryRow(ctx, `SELECT `+blockHeadColumns+` FROM chartworks.block_heads h WHERE h.tenant_id=$1 AND h.block_id=$2 FOR UPDATE`, e.Tenant(), m.ID))
			if err != nil {
				return err
			}
			if head.Version != m.ExpectedVersion || head.Topic != m.Topic {
				return store.ErrConflict
			}
			snapshot, err = blockTx(ctx, tx, e, m.ID, reporting.Reference{Revision: m.TargetRevision}, m.Access())
			if err != nil {
				return err
			}
			if snapshot.Revision.Digest != m.TargetDigest || snapshot.State.Version != m.ExpectedVersion {
				return store.ErrConflict
			}
			if head.Version >= 4096 && m.Kind != "preview" {
				return readexec.ErrLimit
			}
			if m.CheckCurrent {
				if err := blockCurrentFence(ctx, tx, e, m); err != nil {
					return err
				}
				if !snapshot.Current && m.Revision == nil {
					return reporting.ErrStale
				}
			}
			if m.Kind == "preview" {
				if head.Archived || snapshot.PublishedAt == nil && head.DraftState == "rejected" {
					return store.ErrConflict
				}
				if err := verifyBlockAttempt(ctx, tx, e, m, snapshot); err != nil {
					return err
				}
				out = head
				_, err = proof.Checked(e)
				return err
			}
			out = head
			if m.Revision != nil {
				if m.Revision.Number != head.DraftRevision+1 || m.Revision.Number > int64(m.MaxRevisions) || head.Archived && m.Kind != "restore" {
					return store.ErrConflict
				}
				if m.Kind != "edit" && m.Kind != "restore" && m.Kind != "rename" && m.Kind != "parameterize" {
					return store.ErrInvalid
				}
				if err := insertBlockRevision(ctx, tx, e, m); err != nil {
					return err
				}
				out.DraftRevision = m.Revision.Number
				out.DraftState = "draft"
				if m.Kind == "restore" {
					out.Archived = false
				}
			} else {
				switch m.Kind {
				case "validate":
					if head.Archived || snapshot.PublishedAt == nil && (snapshot.Revision.Number != head.DraftRevision || head.DraftState == "rejected") {
						return store.ErrConflict
					}
					if err := verifyBlockAttempt(ctx, tx, e, m, snapshot); err != nil {
						return err
					}
					var count int
					if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.block_validations WHERE tenant_id=$1 AND block_id=$2 AND revision=$3`, e.Tenant(), m.ID, m.TargetRevision).Scan(&count); err != nil {
						return err
					}
					if count >= 32 {
						return readexec.ErrLimit
					}
					v := m.Validation
					raw, _ := json.Marshal(v)
					_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_validations(tenant_id,block_id,revision,evidence_id,actor_id,attempt_id,record,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, e.Tenant(), m.ID, m.TargetRevision, v.Evidence.ID, e.User(), v.Evidence.Attempt.ID, raw, v.Evidence.CreatedAt, v.Evidence.ExpiresAt)
					if err != nil {
						return err
					}
					if m.TargetRevision == head.DraftRevision && snapshot.PublishedAt == nil {
						out.DraftState = "validated"
					}
					observed := v.Evidence.CreatedAt
					if err := setBlockHealth(ctx, tx, e.Tenant(), m.ID, m.TargetRevision, reporting.Health{Status: "healthy", Reason: "validated_observation", ObservedAt: &observed, DependencyDigest: v.Evidence.DependencyDigest}); err != nil {
						return err
					}
				case "publish":
					if head.Archived || m.TargetRevision != head.DraftRevision || head.DraftState != "validated" || snapshot.PublishedAt != nil {
						return store.ErrConflict
					}
					if _, err := blockFresh(ctx, tx, e, m, snapshot); err != nil {
						return err
					}
					_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_publications(tenant_id,block_id,revision,evidence_id,actor_id) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), m.ID, m.TargetRevision, m.Evidence, e.User())
					if err != nil {
						return err
					}
					out.PublishedRevision = m.TargetRevision
					out.DraftState = "published"
				case "certify":
					if head.Archived || snapshot.PublishedAt == nil || m.Attestation == nil {
						return store.ErrConflict
					}
					v, err := blockFresh(ctx, tx, e, m, snapshot)
					if err != nil {
						return err
					}
					a := m.Attestation
					if a.Revision != m.TargetRevision || a.Evidence != m.Evidence || a.Actor != e.User() || a.DependencyDigest != v.Evidence.DependencyDigest || !a.EvidenceExpiresAt.Equal(v.Evidence.ExpiresAt) {
						return store.ErrInvalid
					}
					var count int
					if err := tx.QueryRow(ctx, `SELECT count(*) FROM chartworks.block_attestations WHERE tenant_id=$1 AND block_id=$2 AND revision=$3`, e.Tenant(), m.ID, m.TargetRevision).Scan(&count); err != nil {
						return err
					}
					if count >= 32 {
						return readexec.ErrLimit
					}
					raw, _ := json.Marshal(a)
					_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_attestations(tenant_id,block_id,revision,attestation_id,evidence_id,attestation,created_at) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), m.ID, m.TargetRevision, a.ID, m.Evidence, raw, a.CreatedAt)
					if err != nil {
						return err
					}
				case "withdraw":
					if m.Withdrawal == nil || snapshot.Attestation == nil || snapshot.Withdrawal != nil || snapshot.Attestation.ID != m.Withdrawal.Attestation || m.Withdrawal.Actor != e.User() {
						return store.ErrConflict
					}
					raw, _ := json.Marshal(m.Withdrawal)
					_, err = tx.Exec(ctx, `INSERT INTO chartworks.block_withdrawals(tenant_id,block_id,revision,attestation_id,withdrawal) VALUES($1,$2,$3,$4,$5)`, e.Tenant(), m.ID, m.TargetRevision, m.Withdrawal.Attestation, raw)
					if err != nil {
						return err
					}
				case "reject":
					if head.Archived || m.TargetRevision != head.DraftRevision || snapshot.PublishedAt != nil || head.DraftState == "rejected" {
						return store.ErrConflict
					}
					out.DraftState = "rejected"
				case "archive":
					if head.Archived {
						return store.ErrConflict
					}
					out.Archived = true
					out.PublishedRevision = 0
				case "health":
					if m.Health == nil {
						return store.ErrInvalid
					}
					if err := setBlockHealth(ctx, tx, e.Tenant(), m.ID, m.TargetRevision, *m.Health); err != nil {
						return err
					}
				default:
					return store.ErrInvalid
				}
			}
			out.Version = head.Version + 1
			var published *int64
			if out.PublishedRevision > 0 {
				published = &out.PublishedRevision
			}
			out, err = scanBlockHead(tx.QueryRow(ctx, `UPDATE chartworks.block_heads h SET version=$3,draft_revision=$4,published_revision=$5,draft_state=$6,archived=$7,updated_at=clock_timestamp() WHERE h.tenant_id=$1 AND h.block_id=$2 RETURNING `+blockHeadColumns, e.Tenant(), m.ID, out.Version, out.DraftRevision, published, out.DraftState, out.Archived))
			if err != nil {
				return err
			}
		}
		revision := m.TargetRevision
		if m.Revision != nil {
			revision = m.Revision.Number
		}
		if _, err := tx.Exec(ctx, `INSERT INTO chartworks.block_events(tenant_id,block_id,version,revision,kind,actor_id,note) VALUES($1,$2,$3,$4,$5,$6,$7)`, e.Tenant(), m.ID, out.Version, revision, m.Kind, e.User(), m.Note); err != nil {
			return err
		}
		suffix := blockAudit(m.Kind)
		if suffix == "" {
			return store.ErrInvalid
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		if err := auditJob(ctx, tx, scope, "block."+suffix, m.ID); err != nil {
			return err
		}
		if _, err := proof.Checked(e); err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		return reporting.State{}, err
	}
	if !e.Valid() {
		return reporting.State{}, access.ErrUnauthenticated
	}
	return out, nil
}
