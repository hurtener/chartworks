package reporting

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// WriteDependencyRequest carries native edit intent, never a dependency or grant
// list. Save includes the immutable baseline that the native edit must read.
type WriteDependencyRequest struct {
	Operation       string             `json:"operation" jsonschema:"enum=create,enum=save"`
	ID              string             `json:"id"`
	ExpectedVersion int64              `json:"expected_version"`
	Revision        int64              `json:"revision"`
	Definition      DocumentDefinition `json:"definition"`
}

// WriteDependencyManifest contains only requirements derived by the provider.
// Its digest binds the normalized proposal; it is not a mutation receipt.
type WriteDependencyManifest struct {
	Version          string              `json:"version"`
	Operation        string              `json:"operation"`
	ID               string              `json:"id"`
	ExpectedVersion  int64               `json:"expected_version"`
	BaseRevision     int64               `json:"base_revision"`
	BaseDigest       string              `json:"base_digest"`
	DefinitionDigest string              `json:"definition_digest"`
	MetadataActions  []string            `json:"metadata_actions"`
	References       []ResourceReference `json:"references"`
	Blocks           []DependencyBlock   `json:"blocks"`
}

type WriteDependencyRepository interface {
	DiscoverReportWriteDependencies(context.Context, identity.Envelope, WriteDependencyRequest) (WriteDependencyManifest, error)
}

func RequireWriteDependencyDiscovery(e identity.Envelope, in WriteDependencyRequest) error {
	if !identity.Identifier(in.ID) || !slices.Contains([]string{"create", "save"}, in.Operation) ||
		in.Operation == "create" && (in.Revision != 0 || in.ExpectedVersion != 0) ||
		in.Operation == "save" && (in.Revision < 1 || in.Revision > 256 || in.ExpectedVersion < 1) {
		return ErrInvalid
	}
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if err := access.Require(e, DependencyDiscoveryAction, access.Resource{Tenant: e.Tenant(), Kind: "report", Permission: "write", ID: in.ID}); err != nil {
		return err
	}
	if in.Operation == "create" {
		return access.Require(e, DependencyDiscoveryAction, access.Tenant(e, "write"))
	}
	return nil
}

// WriteDependencies validates the same manual definition format as Create/Save,
// then resolves native metadata. Content, binding validation and effects still
// require the separately authorized operation and its complete dependencies.
func (s *Authoring) WriteDependencies(ctx context.Context, e identity.Envelope, in WriteDependencyRequest) (WriteDependencyManifest, error) {
	if s == nil || ctx == nil {
		return WriteDependencyManifest{}, ErrInvalid
	}
	if err := RequireWriteDependencyDiscovery(e, in); err != nil {
		return WriteDependencyManifest{}, err
	}
	in.Definition = normalizeDocument(in.Definition, s.documents.limits.Composition)
	if manualDocument(in.Definition) != nil || ValidateDocument("report", in.Definition, s.documents.limits.Composition, false) != nil {
		return WriteDependencyManifest{}, ErrInvalid
	}
	raw, err := json.Marshal(in.Definition)
	if err != nil || len(raw) > s.documents.limits.Composition.MaxDefinitionBytes {
		return WriteDependencyManifest{}, ErrInvalid
	}
	repo, ok := s.documents.repo.(WriteDependencyRepository)
	if !ok {
		return WriteDependencyManifest{}, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	out, err := repo.DiscoverReportWriteDependencies(ctx, e, in)
	if err != nil {
		return WriteDependencyManifest{}, err
	}
	if !e.Valid() {
		return WriteDependencyManifest{}, access.ErrUnauthenticated
	}
	out.DefinitionDigest = DocumentDigest(raw)
	out.MetadataActions = []string{}
	for _, canvas := range ReportCanvases(in.Definition) {
		for _, filter := range canvas.Definition.Filters {
			if filter.Options != nil {
				out.MetadataActions = []string{"sources.read", "topics.read"}
			}
		}
	}
	return out, ctx.Err()
}
