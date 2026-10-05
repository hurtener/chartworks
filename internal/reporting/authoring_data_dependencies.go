package reporting

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// DataDependencyRequest selects a retained publication or original preparation.
// A prepare retry supplies its original operation and reviewed topic selection;
// native custody, when present, takes precedence over that proposed selection.
type DataDependencyRequest struct {
	Topic       TopicPin `json:"topic"`
	Dataset     string   `json:"dataset"`
	NewBlock    string   `json:"new_block"`
	Preparation string   `json:"preparation"`
	Operation   string   `json:"operation"`
}

// DataDependencyManifest contains identifiers only. QueryReferences identify the
// selected dataset's execution partition; References includes every dependency
// of the whole publication, including datasets not selected for this chart.
type DataDependencyManifest struct {
	Version         string              `json:"version"`
	Topic           TopicPin            `json:"topic"`
	Dataset         string              `json:"dataset"`
	NewBlock        string              `json:"new_block"`
	Preparation     string              `json:"preparation"`
	Operation       string              `json:"operation"`
	References      []ResourceReference `json:"references"`
	QueryReferences []ResourceReference `json:"query_references"`
}

type DataDependencyRepository interface {
	DiscoverAuthoringDataDependencies(context.Context, identity.Envelope, DataDependencyRequest) (DataDependencyManifest, error)
}

func RequireDataDependencyDiscovery(e identity.Envelope, in DataDependencyRequest) error {
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if in.Topic.Topic != "" {
		if !identity.Identifier(in.Topic.Topic) || (in.Topic.Version == "") != (in.Topic.Digest == "") || in.Topic.Version != "" && (!identity.Identifier(in.Topic.Version) || !hashValid(in.Topic.Digest)) || in.Dataset != "" && !identity.Identifier(in.Dataset) {
			return ErrInvalid
		}
	} else if in.Topic.Version != "" || in.Topic.Digest != "" || in.Dataset != "" {
		return ErrInvalid
	}
	if in.NewBlock == "" {
		if in.Topic.Topic == "" || in.Preparation != "" || in.Operation != "" {
			return ErrInvalid
		}
		return access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: in.Topic.Topic})
	}
	if !identity.Identifier(in.NewBlock) || (in.Preparation == "") == (in.Operation == "") || in.Preparation != "" && !identity.Identifier(in.Preparation) || in.Operation != "" && !identity.Identifier(in.Operation) || in.Topic.Topic != "" && (in.Preparation != "" || in.Topic.Version == "" || in.Dataset == "") {
		return ErrInvalid
	}
	for _, permission := range []string{"read", "write"} {
		if err := access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: "block", Permission: permission, ID: in.NewBlock}); err != nil {
			return err
		}
	}
	if err := access.Require(e, "reporting.preview", access.Resource{Tenant: e.Tenant(), Kind: "block", Permission: "preview", ID: in.NewBlock}); err != nil {
		return err
	}
	if in.Topic.Topic != "" {
		return access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: in.Topic.Topic})
	}
	return nil
}

func (s *Authoring) DataDependencies(ctx context.Context, e identity.Envelope, in DataDependencyRequest) (DataDependencyManifest, error) {
	if s == nil || ctx == nil {
		return DataDependencyManifest{}, ErrInvalid
	}
	if err := RequireDataDependencyDiscovery(e, in); err != nil {
		return DataDependencyManifest{}, err
	}
	repo, ok := s.documents.repo.(DataDependencyRepository)
	if !ok {
		return DataDependencyManifest{}, ErrUnavailable
	}
	return repo.DiscoverAuthoringDataDependencies(ctx, e, in)
}
