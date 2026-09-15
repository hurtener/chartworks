package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
)

var _ nlqexec.SavedAttemptReader = (*DB)(nil)

// ReadSavedAttempt reuses the common content-free read journal. Only current
// signed actor/session and actual resource reach may receive the stored receipt.
func (d *DB) ReadSavedAttempt(ctx context.Context, e identity.Envelope, operation string) (exec.Attempt, error) {
	if !identity.Identifier(operation) {
		return exec.Attempt{}, store.ErrInvalid
	}
	ctx, cancel, err := requestContext(ctx, e)
	if err != nil {
		return exec.Attempt{}, err
	}
	defer cancel()
	if !e.Has("query.execute") {
		return exec.Attempt{}, access.ErrForbidden
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return exec.Attempt{}, err
	}
	attempt, err := d.GetReadOperation(ctx, scope, operation)
	if err != nil {
		return exec.Attempt{}, err
	}
	if attempt.Manifest.Operation != operation || attempt.Manifest.Session != e.Session() {
		return exec.Attempt{}, access.ErrNotFound
	}
	r := attempt.Manifest.Receipt
	refs := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: r.Source},
		{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: r.Context}}
	for _, dataset := range r.Dependencies {
		refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset})
	}
	if err := access.Require(e, "query.execute", refs...); err != nil {
		return exec.Attempt{}, err
	}
	return attempt, ctx.Err()
}
