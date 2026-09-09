package topics

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ListRequest is a bounded keyset page over authorized active publications.
// After is the last returned topic ID, not a credential or a stable snapshot.
type ListRequest struct {
	After string `json:"after"`
	Limit int    `json:"limit"`
}

// Summary deliberately excludes private drafts, source credentials and result data.
// Publication is retained evidence, not a claim of current warehouse health.
type Summary struct {
	Topic       string    `json:"topic"`
	Version     string    `json:"version"`
	Revision    int64     `json:"revision"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Digest      string    `json:"digest"`
	PublishedAt time.Time `json:"published_at"`
}

// CheckList validates authority and page bounds before any catalog access.
func CheckList(e identity.Envelope, in ListRequest) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if !e.Has("topics.read") {
		return access.ErrForbidden
	}
	if in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) {
		return store.ErrInvalid
	}
	for _, p := range [][2]string{{"topic", "read"}, {"source", "read"}, {"dataset", "query"}, {"execution_context", "use"}} {
		if _, err := access.Constrain(e, "topics.read", p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

// List returns only active publications whose complete dependency manifests are
// authorized. The repository applies every restriction before selection/LIMIT.
func (s *Service) List(ctx context.Context, e identity.Envelope, in ListRequest) ([]Summary, error) {
	if ctx == nil {
		return nil, store.ErrInvalid
	}
	if err := CheckList(e, in); err != nil {
		return nil, err
	}
	return s.repo.ListPublishedTopics(ctx, e, in)
}
