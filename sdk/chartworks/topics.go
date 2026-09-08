package chartworks

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/semantics"
	"time"
)

// These public authoring DTO aliases preserve the compiler's exact reference
// contract. They carry no verified authority, publication or execution proof.
type TopicPack = semantics.TopicPack
type TopicDataset = semantics.Dataset
type TopicColumn = semantics.Column
type TopicSourceReference = semantics.SourceReference
type TopicReference = semantics.Reference
type TopicMeasure = semantics.Measure
type TopicDimension = semantics.Dimension
type TopicKPI = semantics.KPI
type TopicJoin = semantics.Join
type TopicCanonicalEntity = semantics.CanonicalEntity
type PortableTopicPack = semantics.PortablePack
type TopicExportDatasetSlots = semantics.ExportDatasetSlots
type TopicExportColumnSlot = semantics.ExportColumnSlot
type TopicDraftBindings = semantics.DraftBindings
type TopicImportDatasetBinding = semantics.ImportDatasetBinding
type TopicImportColumnBinding = semantics.ImportColumnBinding
type TopicVersionDiff = semantics.VersionDiff

type TopicDraftRevision struct {
	Topic    string    `json:"topic"`
	Revision int64     `json:"revision"`
	Version  string    `json:"version"`
	Digest   string    `json:"digest"`
	Actor    string    `json:"actor"`
	Session  string    `json:"session"`
	Created  time.Time `json:"created_at"`
	Change   string    `json:"change"`
}
type TopicDraft struct {
	Metadata TopicDraftRevision `json:"metadata"`
	Pack     TopicPack          `json:"pack"`
}
type SaveTopicDraftRequest struct {
	Expected int64     `json:"expected_revision"`
	Pack     TopicPack `json:"pack"`
	Change   string    `json:"change"`
}
type ImportTopicDraftRequest struct {
	Expected int64              `json:"expected_revision"`
	Portable PortableTopicPack  `json:"portable"`
	Bindings TopicDraftBindings `json:"bindings"`
	Change   string             `json:"change"`
}

func (c *Client) SaveTopicDraft(ctx context.Context, in SaveTopicDraftRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-drafts", "", in, &out, 2<<20)
	return
}
func (c *Client) ImportTopicDraft(ctx context.Context, in ImportTopicDraftRequest) (out TopicDraft, err error) {
	err = c.callLimit(ctx, "POST", "/v1/topic-draft-imports", "", in, &out, 2<<20)
	return
}
func (c *Client) TopicDraft(ctx context.Context, id string) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "GET", "/v1/topics/"+id+"/draft", "", nil, &out, 2<<20)
	return
}
func (c *Client) TopicDraftVersion(ctx context.Context, id string, revision int64) (out TopicDraft, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-versions/read", "", struct {
		Revision int64 `json:"revision"`
	}{revision}, &out, 2<<20)
	return
}
func (c *Client) TopicDraftHistory(ctx context.Context, id string, before int64, limit int) (out []TopicDraftRevision, err error) {
	if !wireID(id) {
		return nil, errors.New("chartworks: invalid topic identifier")
	}
	err = c.call(ctx, "POST", "/v1/topics/"+id+"/draft-history", "", struct {
		Before int64 `json:"before"`
		Limit  int   `json:"limit"`
	}{before, limit}, &out)
	return
}
func (c *Client) DiffTopicDraft(ctx context.Context, id string, before, after int64) (out TopicVersionDiff, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-diff", "", struct {
		Before int64 `json:"before"`
		After  int64 `json:"after"`
	}{before, after}, &out, 16<<20)
	return
}
func (c *Client) ExportTopicDraft(ctx context.Context, id string, revision int64, mapping []TopicExportDatasetSlots) (out PortableTopicPack, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid topic identifier")
	}
	err = c.callLimit(ctx, "POST", "/v1/topics/"+id+"/draft-export", "", struct {
		Revision int64                     `json:"revision"`
		Mapping  []TopicExportDatasetSlots `json:"mapping"`
	}{revision, mapping}, &out, 2<<20)
	return
}
