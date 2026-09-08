package postgres

import (
	"context"
	"encoding/json"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

func validHealthIssues(issues []topics.HealthIssue) bool {
	if len(issues) > 1024 {
		return false
	}
	seen := map[string]bool{}
	for _, issue := range issues {
		if !identity.Identifier(issue.Source) || !identity.Identifier(issue.Context) || !identity.Identifier(issue.Dataset) || seen[issue.Dataset] {
			return false
		}
		switch issue.Code {
		case "source_unavailable", "source_revision_changed", "dataset_missing", "schema_changed":
		default:
			return false
		}
		seen[issue.Dataset] = true
	}
	return true
}

func saveTopicHealthTx(ctx context.Context, tx pgx.Tx, e identity.Envelope, published topics.Published, issues []topics.HealthIssue) (topics.Health, error) {
	if !validHealthIssues(issues) {
		return topics.Health{}, store.ErrInvalid
	}
	if issues == nil {
		issues = []topics.HealthIssue{}
	}
	raw, err := json.Marshal(issues)
	if err != nil {
		return topics.Health{}, store.ErrInvalid
	}
	out := topics.Health{Topic: published.State.Topic, Version: published.State.Version, Revision: published.State.Revision, Healthy: len(issues) == 0, Issues: append([]topics.HealthIssue(nil), issues...)}
	err = tx.QueryRow(ctx, `INSERT INTO chartworks.topic_health(tenant_id,topic_id,publication_revision,version_id,healthy,issues,actor_id,session_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(tenant_id,topic_id) DO UPDATE SET publication_revision=excluded.publication_revision,version_id=excluded.version_id,healthy=excluded.healthy,issues=excluded.issues,actor_id=excluded.actor_id,session_id=excluded.session_id,observed_at=clock_timestamp() RETURNING observed_at`, e.Tenant(), out.Topic, out.Revision, out.Version, out.Healthy, raw, e.User(), e.Session()).Scan(&out.ObservedAt)
	return out, err
}

// SaveTopicHealth commits a complete current-source observation behind the exact
// publication and source revision locks. A concurrent transition or rotation wins.
func (d *DB) SaveTopicHealth(ctx context.Context, e identity.Envelope, published topics.Published, issues []topics.HealthIssue) (out topics.Health, err error) {
	if err = topics.Require(e, published.Definition, drafts.Read); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	err = d.transaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var revision int64
		var version string
		var archived bool
		if err := tx.QueryRow(ctx, `SELECT revision,active_version,archived FROM chartworks.topic_publication_heads WHERE tenant_id=$1 AND topic_id=$2 FOR UPDATE`, e.Tenant(), published.State.Topic).Scan(&revision, &version, &archived); err != nil {
			return err
		}
		if archived || revision != published.State.Revision || version != published.State.Version {
			return store.ErrConflict
		}
		// Clearing issues requires the exact published source revision to remain
		// current at commit. Recording a conservative issue stays valid if a source
		// changes again; a later successful recheck may clear it.
		if len(issues) == 0 {
			// A healthy result may clear old issues only while every persisted source
			// revision remains current under lock. Unhealthy evidence deliberately records
			// the observed mismatch instead of requiring the stale fence to succeed.
			if err := publishedSourceFence(ctx, tx, e.Tenant(), published.Definition); err != nil {
				return err
			}
		}
		var err error
		out, err = saveTopicHealthTx(ctx, tx, e, published, issues)
		if err != nil {
			return err
		}
		scope, _ := store.NewScope(e.Tenant(), e.User())
		return auditJob(ctx, tx, scope, "topic.health_rechecked", published.State.Topic)
	})
	return out, err
}

// ReadTopicHealth returns the retained observation only for the exact current publication.
func (d *DB) ReadTopicHealth(ctx context.Context, e identity.Envelope, published topics.Published) (out topics.Health, err error) {
	if err = topics.Require(e, published.Definition, drafts.Read); err != nil {
		return out, err
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return out, err
	}
	defer cancel()
	var raw []byte
	err = d.pool.QueryRow(ctx, `SELECT publication_revision,version_id,healthy,issues,observed_at FROM chartworks.topic_health WHERE tenant_id=$1 AND topic_id=$2 AND publication_revision=$3 AND version_id=$4`, e.Tenant(), published.State.Topic, published.State.Revision, published.State.Version).Scan(&out.Revision, &out.Version, &out.Healthy, &raw, &out.ObservedAt)
	if err != nil {
		return out, safe(err)
	}
	out.Topic = published.State.Topic
	if json.Unmarshal(raw, &out.Issues) != nil || !validHealthIssues(out.Issues) || out.Healthy != (len(out.Issues) == 0) {
		return topics.Health{}, store.ErrInvalid
	}
	return out, nil
}
