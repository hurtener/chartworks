package reporting

import (
	"context"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// RunCandidates is BFF-only identity metadata. A candidate is not permission to
// expose a run: its original dependencies must be independently authorized.
type RunCandidateRequest struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
	After    string `json:"after"`
	Limit    int    `json:"limit"`
}
type RunCandidatePage struct {
	Version  string   `json:"version"`
	Kind     string   `json:"kind"`
	Resource string   `json:"resource"`
	Runs     []string `json:"runs"`
	Next     string   `json:"next"`
}
type RunCandidateRepository interface {
	DiscoverRunCandidates(context.Context, identity.Envelope, RunCandidateRequest) (RunCandidatePage, error)
}

func RequireRunCandidateDiscovery(e identity.Envelope, in RunCandidateRequest) error {
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if !slices.Contains([]string{"report", "block"}, in.Kind) || !identity.Identifier(in.Resource) || in.After != "" && !identity.Identifier(in.After) || in.Limit < 1 || in.Limit > 32 {
		return ErrInvalid
	}
	return access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: in.Kind, ID: in.Resource, Permission: "read"})
}
func (s *Authoring) RunCandidates(ctx context.Context, e identity.Envelope, in RunCandidateRequest) (RunCandidatePage, error) {
	if s == nil || ctx == nil {
		return RunCandidatePage{}, ErrInvalid
	}
	if err := RequireRunCandidateDiscovery(e, in); err != nil {
		return RunCandidatePage{}, err
	}
	repo, ok := s.documents.repo.(RunCandidateRepository)
	if !ok {
		return RunCandidatePage{}, ErrUnavailable
	}
	return repo.DiscoverRunCandidates(ctx, e, in)
}
