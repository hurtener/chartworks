package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// EffectDependencyRequest names one saved block/report revision or retained run.
// It never accepts caller-supplied dependency, context or identity coordinates.
type EffectDependencyRequest struct {
	Operation string `json:"operation"`
	ID        string `json:"id"`
	Revision  int64  `json:"revision"`
}

type EffectDependencyManifest struct {
	Version    string              `json:"version"`
	Operation  string              `json:"operation"`
	Kind       string              `json:"kind"`
	ID         string              `json:"id"`
	Revision   int64               `json:"revision"`
	Digest     string              `json:"digest"`
	Run        string              `json:"run"`
	Private    bool                `json:"private"`
	Actions    []string            `json:"actions"`
	References []ResourceReference `json:"references"`
}

type EffectDependencyRepository interface {
	DiscoverAuthoringEffectDependencies(context.Context, identity.Envelope, EffectDependencyRequest) (EffectDependencyManifest, error)
}

func RequireEffectDependencyDiscovery(e identity.Envelope, in EffectDependencyRequest) error {
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if !identity.Identifier(in.ID) {
		return ErrInvalid
	}
	kind := ""
	switch in.Operation {
	case "block_validate", "block_run":
		kind = "block"
	case "preview", "report_run":
		kind = "report"
	case "execute", "view", "block_view":
		kind = "run"
	default:
		return ErrInvalid
	}
	if kind == "run" && in.Revision != 0 || kind != "run" && (in.Revision < 1 || in.Revision > 256) {
		return ErrInvalid
	}
	// The exact run seed has no reporting.read action. The store returns only
	// metadata after private actor/session custody; Pengui then checks the
	// authoritative parent and full closure before minting any content token.
	if err := access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: kind, Permission: "read", ID: in.ID}); err != nil {
		return err
	}
	if in.Operation == "block_validate" || in.Operation == "preview" {
		return access.Require(e, "reporting.preview", access.Resource{Tenant: e.Tenant(), Kind: kind, Permission: "preview", ID: in.ID})
	}
	return nil
}

func (s *Authoring) EffectDependencies(ctx context.Context, e identity.Envelope, in EffectDependencyRequest) (EffectDependencyManifest, error) {
	if s == nil || ctx == nil {
		return EffectDependencyManifest{}, ErrInvalid
	}
	if err := RequireEffectDependencyDiscovery(e, in); err != nil {
		return EffectDependencyManifest{}, err
	}
	repo, ok := s.documents.repo.(EffectDependencyRepository)
	if !ok {
		return EffectDependencyManifest{}, ErrUnavailable
	}
	return repo.DiscoverAuthoringEffectDependencies(ctx, e, in)
}
