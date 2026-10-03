package migration

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// ReleaseSource is a content-free projection of one currently active Phase 34
// cutover. It is never a substitute for the owning source's current health or
// the caller's signed query/context reach.
type ReleaseSource struct {
	Cohort         string
	Batch          string
	ManifestDigest string
	Generation     int64
	SourceID       string
	SourceRevision int64
	ContextID      string
	Dialect        string
	Snapshot       string
}

// CurrentReleaseSource rechecks the accepted cutover, its immutable batch and
// the source adapter against the selected destination on every resolution.
// The adapter's validation probes the actual current source binding and health.
func (s *Service) CurrentReleaseSource(ctx context.Context, e identity.Envelope, cohort, sourceID string) (ReleaseSource, error) {
	if s == nil || ctx == nil || !identity.Identifier(cohort) || !identity.Identifier(sourceID) {
		return ReleaseSource{}, ErrInvalid
	}
	if err := require(e, "migration.read", "read"); err != nil {
		return ReleaseSource{}, err
	}
	if err := access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: sourceID}); err != nil {
		return ReleaseSource{}, err
	}
	cutover, err := s.repo.CurrentCutover(ctx, e, cohort)
	if err != nil {
		return ReleaseSource{}, err
	}
	if cutover.Cohort != cohort || cutover.State != "active" || cutover.Generation < 1 || !identity.Identifier(cutover.Batch) {
		return ReleaseSource{}, ErrNotReady
	}
	batch, manifest, plan, err := s.repo.Batch(ctx, e, cutover.Batch)
	if err != nil {
		return ReleaseSource{}, err
	}
	digest, err := validateManifest(manifest)
	if err != nil || batch.ID != cutover.Batch || batch.Cohort != cohort || manifest.Batch != batch.ID || manifest.Cohort != cohort || batch.State != "complete" || batch.Digest != digest || !plan.Ready || plan.Digest != digest {
		return ReleaseSource{}, ErrNotReady
	}
	if s.evidence == nil {
		return ReleaseSource{}, ErrNotReady
	}
	for _, proof := range manifest.Evidence {
		if proof.Disposition == "required" {
			if err := s.evidence.Verify(ctx, e, proof); err != nil {
				return ReleaseSource{}, ErrNotReady
			}
		}
	}
	var selected *Object
	var mapping Mapping
	matches := 0
	for _, m := range manifest.Mappings {
		if m.Kind == KindSource && m.Destination == sourceID {
			matches++
			if matches != 1 {
				return ReleaseSource{}, ErrNotReady
			}
			mapping = m
			for i := range manifest.Objects {
				if manifest.Objects[i].Kind == KindSource && manifest.Objects[i].ExternalRef == m.ExternalRef {
					selected = &manifest.Objects[i]
					break
				}
			}
		}
	}
	if matches != 1 || selected == nil || selected.Revision != mapping.Revision || selected.Lifecycle != "private_draft" || selected.Retention.ExpiresAt != nil && !s.now().UTC().Before(selected.Retention.ExpiresAt.UTC()) {
		return ReleaseSource{}, ErrNotReady
	}
	var binding sourceBindingPayload
	if json.Unmarshal([]byte(selected.Payload), &binding) != nil || binding.Context == "" || binding.Revision != mapping.Revision || binding.Snapshot != manifest.SourceSnapshot || binding.Dialect != manifest.Dialect || binding.Engine != manifest.Engine {
		return ReleaseSource{}, ErrNotReady
	}
	if err := access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: binding.Context}); err != nil {
		return ReleaseSource{}, err
	}
	installed := false
	for _, item := range plan.Objects {
		if item.Kind == KindSource && item.ExternalRef == selected.ExternalRef && item.Revision == selected.Revision && item.Action == "install_private" && item.Destination == sourceID && item.Digest == objectDigest(*selected) {
			installed = true
			break
		}
	}
	adapter := s.adapters[KindSource]
	if !installed || adapter == nil {
		return ReleaseSource{}, ErrNotReady
	}
	if err := adapter.Validate(ctx, e, *selected, mapping); err != nil {
		if errors.Is(err, access.ErrUnauthenticated) || errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
			return ReleaseSource{}, err
		}
		return ReleaseSource{}, ErrNotReady
	}
	current, err := s.repo.CurrentCutover(ctx, e, cohort)
	if err != nil || current.State != "active" || current.Batch != cutover.Batch || current.Generation != cutover.Generation || current.Route != cutover.Route {
		return ReleaseSource{}, ErrNotReady
	}
	return ReleaseSource{Cohort: cohort, Batch: batch.ID, ManifestDigest: digest, Generation: cutover.Generation, SourceID: sourceID, SourceRevision: binding.Revision, ContextID: binding.Context, Dialect: binding.Dialect, Snapshot: binding.Snapshot}, nil
}
