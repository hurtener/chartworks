package reporting

import (
	"context"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// DependencyDiscoveryAction is a metadata-only BFF permission. It never
// authorizes a definition, result, source read, mutation, or token issuance.
const DependencyDiscoveryAction = "reporting.discover"

// DependencyRequest addresses one native revision. It contains no proposed
// dependency list, tenant, actor, scopes, or provider URL.
type DependencyRequest struct {
	Kind     string `json:"kind" jsonschema:"enum=report,enum=block"`
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Stage    string `json:"stage,omitempty" jsonschema:"enum=published,enum=draft,enum=review,enum=editing"`
}

// DependencyBlock records the actual revision selected by a stored widget pin.
// A latest-publication widget resolves the current native publication pointer.
type DependencyBlock struct {
	Source   string `json:"source,omitempty"`
	ID       string `json:"id"`
	Topic    string `json:"topic"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
	Private  bool   `json:"private"`
}

// DependencyManifest is eligibility metadata, not a grant or safety proof.
// Definitions, names, SQL, schemas, values and credentials are never projected.
type DependencyManifest struct {
	Version    string              `json:"version"`
	Kind       string              `json:"kind"`
	ID         string              `json:"id"`
	Revision   int64               `json:"revision"`
	Digest     string              `json:"digest"`
	Private    bool                `json:"private"`
	References []ResourceReference `json:"references"`
	Blocks     []DependencyBlock   `json:"blocks"`
}

// DependencyRepository applies tenant, exact target and private-custody checks
// before projecting the existing native dependency indexes. Ordinary reads
// retain their full checks; discovery must not call them with forged reach.
type DependencyRepository interface {
	DiscoverReportDependencies(context.Context, identity.Envelope, DependencyRequest) (DependencyManifest, error)
}

func RequireDependencyDiscovery(e identity.Envelope, in DependencyRequest) error {
	if !slices.Contains([]string{"report", "block"}, in.Kind) || !identity.Identifier(in.ID) || in.Revision < 0 || in.Revision > 256 || !slices.Contains([]string{"", "published", "draft", "review", "editing"}, in.Stage) || in.Revision > 0 && in.Stage != "" || in.Kind == "block" && (in.Stage == "review" || in.Stage == "editing") {
		return ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	return access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: in.Kind, Permission: "read", ID: in.ID})
}

func (s *Authoring) Dependencies(ctx context.Context, e identity.Envelope, in DependencyRequest) (DependencyManifest, error) {
	if s == nil || ctx == nil {
		return DependencyManifest{}, ErrInvalid
	}
	if err := RequireDependencyDiscovery(e, in); err != nil {
		return DependencyManifest{}, err
	}
	repo, ok := s.documents.repo.(DependencyRepository)
	if !ok {
		return DependencyManifest{}, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	out, err := repo.DiscoverReportDependencies(ctx, e, in)
	if err != nil {
		return DependencyManifest{}, err
	}
	if !e.Valid() {
		return DependencyManifest{}, access.ErrUnauthenticated
	}
	return out, ctx.Err()
}
