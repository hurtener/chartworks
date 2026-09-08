// Package topics publishes reviewed semantic definitions with matching facet
// generations. Retained definitions and current source health are separate reads.
package topics

import (
	"context"
	"encoding/hex"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

// Binding structurally excludes the original private profile ID and digest.
type Binding struct {
	Source         string `json:"source"`
	Context        string `json:"context"`
	Dataset        string `json:"dataset"`
	SourceRevision int64  `json:"source_revision"`
}
type Dataset struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Source  Binding            `json:"source"`
	Columns []semantics.Column `json:"columns"`
}
type Definition struct {
	SchemaVersion     int                         `json:"schema_version"`
	Topic             string                      `json:"topic"`
	Version           string                      `json:"version"`
	Name              string                      `json:"name"`
	Description       string                      `json:"description"`
	Datasets          []Dataset                   `json:"datasets"`
	Measures          []semantics.Measure         `json:"measures"`
	Dimensions        []semantics.Dimension       `json:"dimensions"`
	KPIs              []semantics.KPI             `json:"kpis"`
	Joins             []semantics.Join            `json:"joins"`
	CanonicalEntities []semantics.CanonicalEntity `json:"canonical_entities"`
}

func Project(p semantics.TopicPack) Definition {
	out := Definition{SchemaVersion: p.SchemaVersion, Topic: p.Topic, Version: p.Version, Name: p.Name, Description: p.Description, Measures: p.Measures, Dimensions: p.Dimensions, KPIs: p.KPIs, Joins: p.Joins, CanonicalEntities: p.CanonicalEntities}
	for _, d := range p.Datasets {
		out.Datasets = append(out.Datasets, Dataset{d.ID, d.Name, Binding{d.Source.Source, d.Source.Context, d.ID, d.Source.SourceRevision}, d.Columns})
	}
	return out
}
func Require(e identity.Envelope, d Definition, a drafts.Access) error {
	if err := drafts.Require(e, d.Topic, a); err != nil {
		return err
	}
	for _, d := range d.Datasets {
		for _, r := range []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: d.Source.Source}, {Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: d.ID}, {Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: d.Source.Context}} {
			if err := access.Require(e, a.Action(), r); err != nil {
				return err
			}
		}
	}
	return nil
}

type ReviewRequest struct {
	DraftRevision int64  `json:"draft_revision"`
	Digest        string `json:"digest"`
	Decision      string `json:"decision"`
	Note          string `json:"note"`
}
type Review struct {
	ID            string    `json:"id"`
	Topic         string    `json:"topic"`
	DraftRevision int64     `json:"draft_revision"`
	Digest        string    `json:"digest"`
	Decision      string    `json:"decision"`
	Note          string    `json:"note"`
	Created       time.Time `json:"created_at"`
}

func DigestValid(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

type PublishRequest struct {
	Review   string `json:"review"`
	Expected int64  `json:"expected_revision"`
}
type TransitionRequest struct {
	Version  string `json:"version"`
	Expected int64  `json:"expected_revision"`
	Note     string `json:"note"`
}
type FacetGeneration struct {
	Context    string `json:"context"`
	Generation string `json:"generation"`
	Space      string `json:"space"`
	Facets     int    `json:"facets"`
}
type State struct {
	Topic       string            `json:"topic"`
	Revision    int64             `json:"revision"`
	Version     string            `json:"version"`
	Archived    bool              `json:"archived"`
	Active      bool              `json:"active"`
	Generations []FacetGeneration `json:"generations"`
}
type Published struct {
	State       State      `json:"state"`
	Definition  Definition `json:"definition"`
	Digest      string     `json:"digest"`
	PublishedAt time.Time  `json:"published_at"`
}
type Contract struct {
	Publication Published `json:"publication"`
	ObservedAt  time.Time `json:"observed_at"`
}

// Prepared seals the reviewed model after live source checks. Only this service
// creates it; SQL rechecks current authority, review and source revisions at commit.
type Prepared struct {
	model                  semantics.Model
	review                 Review
	tenant, actor, session string
	deadline               time.Time
	descriptor             gateway.EmbeddingSpace
}

func (p Prepared) Checked(e identity.Envelope) (semantics.TopicPack, Review, error) {
	if p.model.Digest() == "" || !e.Valid() || e.Tenant() != p.tenant || e.User() != p.actor || e.Session() != p.session || !time.Now().Before(p.deadline) {
		return semantics.TopicPack{}, Review{}, access.ErrUnauthenticated
	}
	pack := p.model.Pack()
	if err := drafts.RequirePack(e, pack, drafts.Publish); err != nil {
		return semantics.TopicPack{}, Review{}, err
	}
	if p.review.Decision != "approve" || p.review.Digest != p.model.Digest() || p.review.Topic != pack.Topic {
		return semantics.TopicPack{}, Review{}, store.ErrInvalid
	}
	return pack, p.review, nil
}

func (p Prepared) CanonicalMeanings(e identity.Envelope) ([]semantics.CanonicalMeaning, error) {
	if _, _, err := p.Checked(e); err != nil {
		return nil, err
	}
	return p.model.CanonicalMeanings(), nil
}

type Repository interface {
	CheckCanonicalMeanings(context.Context, identity.Envelope, string, drafts.Access, []semantics.CanonicalMeaning) (bool, error)
	ReviewTopic(context.Context, identity.Envelope, string, ReviewRequest) (Review, error)
	ReviewedTopic(context.Context, identity.Envelope, string, string) (Review, drafts.Version, error)
	PublishTopic(context.Context, identity.Envelope, Prepared, []vindex.Generation, gateway.Receipt, int64) (Published, error)
	ConfirmTopicContract(context.Context, identity.Envelope, string, int64) (Published, error)
	ReadPublishedTopic(context.Context, identity.Envelope, string, string, drafts.Access) (Published, error)
	RollbackTopic(context.Context, identity.Envelope, string, TransitionRequest) (Published, error)
	ArchiveTopic(context.Context, identity.Envelope, string, int64, string) (State, error)
}

// CheckGenerations compares supplied staging manifests to the exact gateway
// descriptor and semantic input graph sealed by admission.
func (p Prepared) CheckGenerations(e identity.Envelope, gs []vindex.Generation) error {
	if _, _, err := p.Checked(e); err != nil {
		return err
	}
	planned, err := facetPlan(p.model, p.descriptor)
	if err != nil {
		return err
	}
	if len(gs) != len(planned) {
		return store.ErrInvalid
	}
	for i, g := range gs {
		if vindex.Digest(g) != vindex.Digest(planned[i].generation) {
			return store.ErrInvalid
		}
	}
	return nil
}
