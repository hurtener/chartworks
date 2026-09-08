// Package drafts admits private, immutable topic authoring revisions. Drafts are
// not published topics, executable plans, or evidence of current source health.
package drafts

import (
	"context"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

const MaxRevisions = 128

// Access is closed: callers cannot substitute an unrelated signed action.
type Access uint8

const (
	Read Access = iota + 1
	Write
	Export
	Review
	Publish
)

func (a Access) Action() string {
	switch a {
	case Read:
		return "topics.read"
	case Write:
		return "topics.write"
	case Export:
		return "topics.export"
	case Review:
		return "topics.review"
	case Publish:
		return "topics.publish"
	}
	return ""
}
func (a Access) Permission() string {
	switch a {
	case Read:
		return "read"
	case Write:
		return "write"
	case Export:
		return "export"
	case Review, Publish:
		return "publish"
	}
	return ""
}
func Require(e identity.Envelope, topic string, a Access) error {
	if a.Action() == "" {
		return store.ErrInvalid
	}
	return access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: a.Permission(), ID: topic})
}
func RequirePack(e identity.Envelope, p semantics.TopicPack, a Access) error {
	if err := Require(e, p.Topic, a); err != nil {
		return err
	}
	for _, d := range p.Datasets {
		for _, r := range []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: d.Source.Source}, {Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: d.ID}, {Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: d.Source.Context}} {
			if err := access.Require(e, a.Action(), r); err != nil {
				return err
			}
		}
	}
	return nil
}

type Revision struct {
	Topic    string    `json:"topic"`
	Revision int64     `json:"revision"`
	Version  string    `json:"version"`
	Digest   string    `json:"digest"`
	Actor    string    `json:"actor"`
	Session  string    `json:"session"`
	Created  time.Time `json:"created_at"`
	Change   string    `json:"change"`
}
type Version struct {
	Metadata Revision            `json:"metadata"`
	Pack     semantics.TopicPack `json:"pack"`
}
type SaveRequest struct {
	Expected int64               `json:"expected_revision"`
	Pack     semantics.TopicPack `json:"pack"`
	Change   string              `json:"change"`
}
type ImportRequest struct {
	Expected int64                   `json:"expected_revision"`
	Portable semantics.PortablePack  `json:"portable"`
	Bindings semantics.DraftBindings `json:"bindings"`
	Change   string                  `json:"change"`
}

// Prepared is an ephemeral admission proof issued only after public source/profile
// checks. The database still fences source revisions and retained evidence at commit.
type Prepared struct {
	model                  semantics.Model
	tenant, actor, session string
	deadline               time.Time
}

func (p Prepared) Pack(e identity.Envelope) (semantics.TopicPack, error) {
	if p.model.Digest() == "" || !e.Valid() || p.tenant != e.Tenant() || p.actor != e.User() || p.session != e.Session() || !time.Now().Before(p.deadline) {
		return semantics.TopicPack{}, access.ErrUnauthenticated
	}
	out := p.model.Pack()
	if err := RequirePack(e, out, Write); err != nil {
		return semantics.TopicPack{}, err
	}
	return out, nil
}

// Repository owns tenant-composite identity, pre-query scope restrictions, CAS,
// immutable snapshots and an audit event in the same transaction.
type Repository interface {
	CheckCanonicalMeanings(context.Context, identity.Envelope, string, Access, []semantics.CanonicalMeaning) (bool, error)
	SaveTopicDraft(context.Context, identity.Envelope, Prepared, int64, string) (Version, error)
	ReadTopicDraft(context.Context, identity.Envelope, string, int64, Access) (Version, error)
	TopicDraftHistory(context.Context, identity.Envelope, string, int64, int) ([]Revision, error)
}
type Service struct {
	repo     Repository
	sources  *sources.Service
	profiles *engineering.Service
}

func New(repo Repository, s *sources.Service, p *engineering.Service) (*Service, error) {
	if repo == nil || s == nil || p == nil {
		return nil, store.ErrInvalid
	}
	return &Service{repo, s, p}, nil
}
func (s *Service) Save(ctx context.Context, e identity.Envelope, in SaveRequest) (Version, error) {
	if ctx == nil || in.Expected < 0 || in.Expected >= MaxRevisions || len(in.Change) < 1 || len(in.Change) > 1024 || !utf8.ValidString(in.Change) {
		return Version{}, store.ErrInvalid
	}
	model, err := semantics.Compile(in.Pack)
	if err != nil {
		return Version{}, err
	}
	p := model.Pack()
	if err = RequirePack(e, p, Write); err != nil {
		return Version{}, err
	}
	if in.Expected == 0 {
		if err = access.Require(e, "topics.write", access.Tenant(e, "write")); err != nil {
			return Version{}, err
		}
	} else {
		if _, err = s.repo.ReadTopicDraft(ctx, e, p.Topic, in.Expected, Write); err != nil {
			return Version{}, err
		}
	}
	// Drafts carry proposals only. This preflight rejects impossible revision
	// sequences and already-reserved terms without approving new meaning.
	if _, err = s.repo.CheckCanonicalMeanings(ctx, e, p.Topic, Write, model.CanonicalMeanings()); err != nil {
		return Version{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	catalogs := map[string]sources.Discovery{}
	for _, d := range p.Datasets {
		evidence, err := s.profiles.Evidence(ctx, e, d.Source.ProfileVersion)
		if err != nil {
			return Version{}, err
		}
		profile := evidence.Profile
		if !evidence.Active || profile.Version != d.Source.ProfileVersion || profile.Source != d.Source.Source || profile.Context != d.Source.Context || profile.Dataset != d.ID || profile.SourceRevision != d.Source.SourceRevision || profile.DeterministicHash() != d.Source.ProfileDigest {
			return Version{}, readexec.ErrBinding
		}
		catalog, ok := catalogs[d.Source.Source]
		if !ok {
			catalog, err = s.sources.Discover(ctx, e, d.Source.Source)
			if err != nil {
				return Version{}, err
			}
			catalogs[d.Source.Source] = catalog
		}
		if catalog.ContextID != d.Source.Context || catalog.Revision != d.Source.SourceRevision {
			return Version{}, readexec.ErrBinding
		}
		var schema []readexec.Column
		for _, r := range catalog.Relations {
			if r.ID == d.ID {
				schema = r.Columns
				break
			}
		}
		if len(schema) == 0 || !reflect.DeepEqual(schema, profile.Schema) {
			return Version{}, readexec.ErrBinding
		}
		for _, column := range d.Columns {
			found := false
			for _, actual := range schema {
				if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
					found = true
					break
				}
			}
			if !found {
				return Version{}, readexec.ErrBinding
			}
		}
	}
	deadline, _ := ctx.Deadline()
	if e.Deadline().Before(deadline) {
		deadline = e.Deadline()
	}
	return s.repo.SaveTopicDraft(ctx, e, Prepared{model, e.Tenant(), e.User(), e.Session(), deadline}, in.Expected, in.Change)
}
func (s *Service) Read(ctx context.Context, e identity.Envelope, id string, revision int64) (Version, error) {
	return s.repo.ReadTopicDraft(ctx, e, id, revision, Read)
}
func (s *Service) History(ctx context.Context, e identity.Envelope, id string, before int64, limit int) ([]Revision, error) {
	return s.repo.TopicDraftHistory(ctx, e, id, before, limit)
}
func (s *Service) Diff(ctx context.Context, e identity.Envelope, id string, before, after int64) (semantics.VersionDiff, error) {
	if before < 1 || after < 1 {
		return semantics.VersionDiff{}, store.ErrInvalid
	}
	a, err := s.Read(ctx, e, id, before)
	if err != nil {
		return semantics.VersionDiff{}, err
	}
	b, err := s.Read(ctx, e, id, after)
	if err != nil {
		return semantics.VersionDiff{}, err
	}
	am, err := semantics.Compile(a.Pack)
	if err != nil {
		return semantics.VersionDiff{}, err
	}
	bm, err := semantics.Compile(b.Pack)
	if err != nil {
		return semantics.VersionDiff{}, err
	}
	return semantics.DiffModels(am, bm)
}
func (s *Service) Export(ctx context.Context, e identity.Envelope, id string, revision int64, mapping []semantics.ExportDatasetSlots) (semantics.PortablePack, error) {
	if revision < 1 {
		return semantics.PortablePack{}, store.ErrInvalid
	}
	v, err := s.repo.ReadTopicDraft(ctx, e, id, revision, Export)
	if err != nil {
		return semantics.PortablePack{}, err
	}
	model, err := semantics.Compile(v.Pack)
	if err != nil {
		return semantics.PortablePack{}, err
	}
	return semantics.ExportPortable(model, mapping)
}
func (s *Service) Import(ctx context.Context, e identity.Envelope, in ImportRequest) (Version, error) {
	candidate, err := semantics.ImportDraftCandidate(in.Portable, in.Bindings)
	if err != nil {
		return Version{}, err
	}
	return s.Save(ctx, e, SaveRequest{in.Expected, candidate.Pack(), in.Change})
}
