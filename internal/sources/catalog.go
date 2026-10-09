package sources

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// SourceListRequest selects a retained source page, without warehouse work.
type SourceListRequest struct {
	After string `json:"after"`
	Limit int    `json:"limit"`
}

// SourcePage has an explicit cursor so callers need not guess whether more exist.
type SourcePage struct {
	Items []Source `json:"items"`
	Next  string   `json:"next,omitempty"`
}

func (s *Service) ListPage(ctx context.Context, e identity.Envelope, in SourceListRequest) (SourcePage, error) {
	if in.Limit < 1 || in.Limit > 32 || in.After != "" && !identity.Identifier(in.After) {
		return SourcePage{}, store.ErrInvalid
	}
	rows, err := s.listSources(ctx, e, SourceListRequest{After: in.After, Limit: in.Limit + 1})
	if err != nil {
		return SourcePage{}, err
	}
	if rows == nil {
		rows = []Source{}
	}
	out := SourcePage{Items: rows}
	if len(rows) > in.Limit {
		out.Items = rows[:in.Limit]
		out.Next = out.Items[len(out.Items)-1].ID
	}
	return out, nil
}

// DatasetListRequest selects registered metadata in one exact source context.
// This is not a warehouse probe and does not guarantee current source health.
type DatasetListRequest struct {
	Source  string `json:"source"`
	Context string `json:"context"`
	After   string `json:"after"`
	Limit   int    `json:"limit"`
}

// DatasetDescribeRequest addresses one registered dataset without broad discovery.
type DatasetDescribeRequest struct {
	Source  string `json:"source"`
	Context string `json:"context"`
	Dataset string `json:"dataset"`
}

// Dataset is secret-free registered metadata; no rows or live estimates are read.
type Dataset struct {
	SchemaDigest string            `json:"schema_digest"`
	Source       string            `json:"source"`
	Context      string            `json:"context"`
	Revision     int64             `json:"source_revision"`
	Dialect      string            `json:"dialect"`
	Relation     readexec.Relation `json:"relation"`
}

// DatasetQuery is the shared service/store catalog request, not an authority proof.
type DatasetQuery struct {
	DatasetListRequest
	Dataset string
}

// CheckDatasetQuery checks scope and input bounds before any store operation.
func CheckDatasetQuery(e identity.Envelope, in DatasetQuery) error {
	if err := access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: in.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: in.Context}); err != nil {
		return err
	}
	if in.Limit < 1 || in.Limit > 32 || in.After != "" && !identity.Identifier(in.After) || in.Dataset != "" && !identity.Identifier(in.Dataset) {
		return store.ErrInvalid
	}
	if in.Dataset != "" {
		return access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: in.Dataset})
	}
	_, err := access.Constrain(e, "sources.read", "dataset", "query")
	return err
}

// ListDatasets lists only currently authorized registered dataset metadata.
func (s *Service) ListDatasets(ctx context.Context, e identity.Envelope, in DatasetListRequest) ([]Dataset, error) {
	return s.datasetCatalog(ctx, e, DatasetQuery{DatasetListRequest: in})
}

// DescribeDataset loads one authorized registered relation, never a live source.
func (s *Service) DescribeDataset(ctx context.Context, e identity.Envelope, in DatasetDescribeRequest) (Dataset, error) {
	if !identity.Identifier(in.Dataset) {
		return Dataset{}, store.ErrInvalid
	}
	out, err := s.datasetCatalog(ctx, e, DatasetQuery{DatasetListRequest: DatasetListRequest{Source: in.Source, Context: in.Context, Limit: 1}, Dataset: in.Dataset})
	if err != nil {
		return Dataset{}, err
	}
	if len(out) != 1 {
		return Dataset{}, store.ErrNotFound
	}
	return out[0], nil
}
func (s *Service) datasetCatalog(ctx context.Context, e identity.Envelope, in DatasetQuery) (out []Dataset, err error) {
	if ctx == nil {
		return nil, store.ErrInvalid
	}
	if err = CheckDatasetQuery(e, in); err != nil {
		return nil, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		var err error
		out, err = s.repo.ReadDatasetCatalog(ctx, e, in)
		return err
	})
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].SchemaDigest = readexec.Hash(out[i].Relation)
	}
	return out, nil
}
