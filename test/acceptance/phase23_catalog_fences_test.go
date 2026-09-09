package acceptance

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	cw "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

// The public clients do not make the repository's own checks optional. These
// regressions reach the real PostgreSQL catalog owner directly, then exercise
// the same failures through both client transports. Metadata outages must not
// become a successful empty page, a partial page, or cached caller authority.
func TestPhase23CatalogRepositoryFences(t *testing.T) {
	f := newPhase23Fixture(t)
	e, err := f.authority.verifier.Verify(t.Context(), f.httpToken, auth.HTTP)
	if err != nil {
		t.Fatal(err)
	}
	ref := f.domain.pack.Datasets[0].Source
	query := sources.DatasetQuery{DatasetListRequest: sources.DatasetListRequest{Source: ref.Source, Context: ref.Context, Limit: 32}}
	page := topics.ListRequest{Limit: 100}
	db := f.domain.f.db
	beforeModel, beforeSource := f.domain.model.requests.Load(), f.domain.f.lookups.Load()

	t.Run("repository_rechecks_authority_and_bounds", func(t *testing.T) {
		bare := f.token(t, e.Tenant(), e.User(), "phase23-session", nil, false)
		denied, err := f.authority.verifier.Verify(t.Context(), bare, auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		for _, actor := range []identity.Envelope{{}, denied} {
			rows, err := db.ReadDatasetCatalog(t.Context(), actor, query)
			if rows != nil || (!errors.Is(err, access.ErrUnauthenticated) && !errors.Is(err, access.ErrForbidden)) {
				t.Fatal("repository accepted unverified or unscoped dataset authority", err)
			}
			listed, err := db.ListPublishedTopics(t.Context(), actor, page)
			if listed != nil || (!errors.Is(err, access.ErrUnauthenticated) && !errors.Is(err, access.ErrForbidden)) {
				t.Fatal("repository accepted unverified or unscoped topic authority", err)
			}
		}
		bad := query
		bad.Limit = 33
		if rows, err := db.ReadDatasetCatalog(t.Context(), e, bad); rows != nil || !errors.Is(err, store.ErrInvalid) {
			t.Fatal("repository accepted an unbounded dataset page", err)
		}
		if rows, err := db.ListPublishedTopics(t.Context(), e, topics.ListRequest{Limit: 101}); rows != nil || !errors.Is(err, store.ErrInvalid) {
			t.Fatal("repository accepted an unbounded topic page", err)
		}
	})

	t.Run("cancelled_catalog_is_not_an_empty_success", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if rows, err := db.ReadDatasetCatalog(ctx, e, query); rows != nil || !errors.Is(err, context.Canceled) {
			t.Fatal("dataset repository lost cancellation", err)
		}
		if rows, err := db.ListPublishedTopics(ctx, e, page); rows != nil || !errors.Is(err, context.Canceled) {
			t.Fatal("topic repository lost cancellation", err)
		}
	})

	t.Run("metadata_outage_is_typed_and_recovers", func(t *testing.T) {
		raw := support.Raw(t, f.domain.f.dsn)
		for _, unavailable := range []struct {
			name, remove, restore string
		}{
			{"datasets", "ALTER TABLE chartworks.source_revisions RENAME TO phase23_unavailable_source_revisions", "ALTER TABLE chartworks.phase23_unavailable_source_revisions RENAME TO source_revisions"},
			{"topics", "ALTER TABLE chartworks.topic_publication_heads RENAME TO phase23_unavailable_topic_heads", "ALTER TABLE chartworks.phase23_unavailable_topic_heads RENAME TO topic_publication_heads"},
		} {
			t.Run(unavailable.name, func(t *testing.T) {
				// Fixture-only failure injection into an isolated metadata database;
				// no applied migration, published payload, or authority is edited.
				sql(t, raw, unavailable.remove)
				defer sql(t, raw, unavailable.restore)
				if unavailable.name == "datasets" {
					if rows, err := db.ReadDatasetCatalog(t.Context(), e, query); rows != nil || !errors.Is(err, store.ErrUnavailable) {
						t.Fatal("dataset outage returned data or an unsafe error", err)
					}
				} else if rows, err := db.ListPublishedTopics(t.Context(), e, page); rows != nil || !errors.Is(err, store.ErrUnavailable) {
					t.Fatal("topic outage returned data or an unsafe error", err)
				}
				for _, local := range []bool{false, true} {
					client := f.sdk(t, f.httpToken, local)
					var err error
					if unavailable.name == "datasets" {
						var rows []cw.Dataset
						rows, err = client.ListDatasets(t.Context(), query.DatasetListRequest)
						if rows != nil {
							t.Fatal("client returned a dataset page during metadata failure")
						}
					} else {
						var rows []cw.TopicSummary
						rows, err = client.ListTopics(t.Context(), page)
						if rows != nil {
							t.Fatal("client returned a topic page during metadata failure")
						}
					}
					var status *cw.StatusError
					if !errors.As(err, &status) || status.Status != 503 {
						t.Fatal("catalog outage lost its typed unavailable status", local, err)
					}
				}
			})
		}
		for _, local := range []bool{false, true} {
			client := f.sdk(t, f.httpToken, local)
			if rows, err := client.ListDatasets(t.Context(), query.DatasetListRequest); err != nil || len(rows) == 0 {
				t.Fatal("dataset outage poisoned subsequent client reads", err)
			}
			if rows, err := client.ListTopics(t.Context(), page); err != nil || len(rows) < 2 {
				t.Fatal("topic outage poisoned subsequent client reads", err)
			}
		}
	})
	if f.domain.model.requests.Load() != beforeModel || f.domain.f.lookups.Load() != beforeSource {
		t.Fatal("catalog failure or recovery opened a warehouse or model dependency")
	}
}
