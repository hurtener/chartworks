package postgres

import (
	"context"
	"errors"
	"sort"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/jackc/pgx/v5"
)

// A retained generated advisory describes exact profile evidence, not just the
// source schema. A newer profile invalidates it even without source rotation.
// Head share locks fence concurrent replacement through review/publication commit.
// Manual/legacy drafts have no generated advisory and keep their explicit policy.
func generatedTopicQualityFence(ctx context.Context, tx pgx.Tx, e identity.Envelope, draft drafts.Version) error {
	if draft.Quality == nil {
		return nil
	}
	datasets := append([]semantics.Dataset(nil), draft.Pack.Datasets...)
	sort.Slice(datasets, func(i, j int) bool {
		if datasets[i].Source.Source != datasets[j].Source.Source {
			return datasets[i].Source.Source < datasets[j].Source.Source
		}
		return datasets[i].ID < datasets[j].ID
	})
	for _, dataset := range datasets {
		r := dataset.Source
		var revision int64
		err := tx.QueryRow(ctx, `SELECT current_revision FROM chartworks.sources WHERE tenant_id=$1 AND source_id=$2 AND NOT deleted FOR SHARE`, e.Tenant(), r.Source).Scan(&revision)
		if errors.Is(err, pgx.ErrNoRows) {
			return readexec.ErrBinding
		}
		if err != nil {
			return err
		}
		if revision != r.SourceRevision {
			return readexec.ErrBinding
		}
		var digest string
		err = tx.QueryRow(ctx, `SELECT p.deterministic_hash FROM chartworks.profile_versions p JOIN chartworks.profile_heads h ON(h.tenant_id,h.actor_id,h.session_id,h.source_id,h.dataset_id,h.profile_id)=(p.tenant_id,p.actor_id,p.session_id,p.source_id,p.dataset_id,p.profile_id) WHERE p.tenant_id=$1 AND p.profile_id=$2 AND p.source_id=$3 AND p.context_id=$4 AND p.dataset_id=$5 AND p.actor_id=$6 AND p.session_id=$7 AND p.state='complete' FOR SHARE OF p,h`, e.Tenant(), r.ProfileVersion, r.Source, r.Context, r.Dataset, e.User(), e.Session()).Scan(&digest)
		if errors.Is(err, pgx.ErrNoRows) {
			return readexec.ErrBinding
		}
		if err != nil {
			return err
		}
		if digest != r.ProfileDigest {
			return readexec.ErrBinding
		}
	}
	return nil
}
