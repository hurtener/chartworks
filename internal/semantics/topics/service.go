package topics

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

type Service struct {
	repo   Repository
	source *sources.Service
	index  *vindex.Service
	engine gateway.Engine
}

func New(repo Repository, source *sources.Service, index *vindex.Service, engine gateway.Engine) (*Service, error) {
	if repo == nil || source == nil || index == nil {
		return nil, store.ErrInvalid
	}
	return &Service{repo, source, index, engine}, nil
}
func NoteValid(note string) bool { return len(note) > 0 && len(note) <= 1024 && utf8.ValidString(note) }
func (s *Service) Review(ctx context.Context, e identity.Envelope, topic string, in ReviewRequest) (Review, error) {
	if ctx == nil || in.DraftRevision < 1 || in.DraftRevision > drafts.MaxRevisions || !DigestValid(in.Digest) || !NoteValid(in.Note) || (in.Decision != "approve" && in.Decision != "reject") {
		return Review{}, store.ErrInvalid
	}
	return s.repo.ReviewTopic(ctx, e, topic, in)
}

// currentSources performs actual discovery, independently of private profile
// authorization. Retained published definition reads do not invoke this method.
func (s *Service) currentSources(ctx context.Context, e identity.Envelope, d Definition) error {
	catalogs := map[string]sources.Discovery{}
	for _, dataset := range d.Datasets {
		binding := dataset.Source
		catalog, ok := catalogs[binding.Source]
		if !ok {
			var err error
			catalog, err = s.source.Discover(ctx, e, binding.Source)
			if err != nil {
				return err
			}
			catalogs[binding.Source] = catalog
		}
		if catalog.ContextID != binding.Context || catalog.Revision != binding.SourceRevision {
			return readexec.ErrBinding
		}
		var columns []readexec.Column
		for _, relation := range catalog.Relations {
			if relation.ID == dataset.ID {
				columns = relation.Columns
				break
			}
		}
		if len(columns) == 0 {
			return readexec.ErrBinding
		}
		for _, column := range dataset.Columns {
			found := false
			for _, actual := range columns {
				if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
					found = true
					break
				}
			}
			if !found {
				return readexec.ErrBinding
			}
		}
	}
	return nil
}
func (s *Service) Publish(ctx context.Context, e identity.Envelope, topic string, in PublishRequest) (Published, error) {
	if ctx == nil || !identity.Identifier(in.Review) || in.Expected < 0 || in.Expected >= 1<<62 {
		return Published{}, store.ErrInvalid
	}
	if s.engine == nil {
		return Published{}, gateway.ErrDisabled
	}
	review, draft, err := s.repo.ReviewedTopic(ctx, e, topic, in.Review)
	if err != nil {
		return Published{}, err
	}
	if review.Decision != "approve" || review.Digest != draft.Metadata.Digest {
		return Published{}, store.ErrConflict
	}
	model, err := semantics.Compile(draft.Pack)
	if err != nil {
		return Published{}, err
	}
	if err = drafts.RequirePack(e, model.Pack(), drafts.Publish); err != nil {
		return Published{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err = s.currentSources(ctx, e, Project(model.Pack())); err != nil {
		return Published{}, err
	}
	descriptor := s.engine.EmbeddingSpace()
	if descriptor.Key() != s.engine.Space() {
		return Published{}, gateway.ErrSpace
	}
	groups, err := facetPlan(model, descriptor)
	if err != nil {
		return Published{}, err
	}
	var resources []access.Resource
	resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "publish", ID: topic})
	for _, d := range draft.Pack.Datasets {
		resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: d.Source.Source}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: d.ID}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: d.Source.Context})
	}
	call, err := gateway.Authorize(e, "topics.publish", model.Digest(), resources...)
	if err != nil {
		return Published{}, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 64, Tokens: 1 << 20, Duration: 30 * time.Second})
	if err != nil {
		return Published{}, err
	}
	texts := []string{}
	for _, group := range groups {
		for _, facet := range group.facets {
			texts = append(texts, facet.Text)
		}
	}
	embedded, err := s.engine.Embed(ctx, call, budget, descriptor.Key(), texts)
	if err != nil {
		return Published{}, err
	}
	if embedded.Space != descriptor.Key() || embedded.Descriptor != descriptor || len(embedded.Vectors) != len(texts) {
		return Published{}, gateway.ErrSpace
	}
	// Validate every returned vector and every batch before writing even invisible
	// staging data. A malformed later batch never leaves a partly accepted manifest.
	offset := 0
	for i := range groups {
		for j := range groups[i].facets {
			groups[i].facets[j].Vector = embedded.Vectors[offset]
			offset++
		}
		for start := 0; start < len(groups[i].facets); start += 32 {
			end := min(start+32, len(groups[i].facets))
			if err = vindex.CheckBatch(groups[i].generation, groups[i].facets[start:end]); err != nil {
				return Published{}, err
			}
		}
	}
	generations := make([]vindex.Generation, 0, len(groups))
	for _, group := range groups {
		if err = s.index.BeginPublication(ctx, e, group.generation); err != nil {
			return Published{}, err
		}
		for start := 0; start < len(group.facets); start += 32 {
			end := min(start+32, len(group.facets))
			if err = s.index.UpsertPublication(ctx, e, group.generation, group.facets[start:end]); err != nil {
				return Published{}, err
			}
		}
		generations = append(generations, group.generation)
	}
	deadline, _ := ctx.Deadline()
	if e.Deadline().Before(deadline) {
		deadline = e.Deadline()
	}
	return s.repo.PublishTopic(ctx, e, Prepared{model: model, review: review, tenant: e.Tenant(), actor: e.User(), session: e.Session(), deadline: deadline, descriptor: descriptor}, generations, embedded.Receipt, in.Expected)
}
func (s *Service) Read(ctx context.Context, e identity.Envelope, topic, version string) (Published, error) {
	return s.repo.ReadPublishedTopic(ctx, e, topic, version, drafts.Read)
}
func (s *Service) Contract(ctx context.Context, e identity.Envelope, topic string) (Contract, error) {
	if ctx == nil {
		return Contract{}, store.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	current, err := s.Read(ctx, e, topic, "")
	if err != nil {
		return Contract{}, err
	}
	if current.State.Archived {
		return Contract{}, store.ErrNotFound
	}
	if err = s.currentSources(ctx, e, current.Definition); err != nil {
		return Contract{}, err
	}
	after, err := s.repo.ConfirmTopicContract(ctx, e, topic, current.State.Revision)
	if err != nil {
		return Contract{}, err
	}
	return Contract{after, time.Now().UTC()}, nil
}
func (s *Service) Rollback(ctx context.Context, e identity.Envelope, topic string, in TransitionRequest) (Published, error) {
	if ctx == nil || !identity.Identifier(in.Version) || in.Expected < 1 || in.Expected >= 1<<62 || !NoteValid(in.Note) {
		return Published{}, store.ErrInvalid
	}
	target, err := s.repo.ReadPublishedTopic(ctx, e, topic, in.Version, drafts.Publish)
	if err != nil {
		return Published{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err = s.currentSources(ctx, e, target.Definition); err != nil {
		return Published{}, err
	}
	return s.repo.RollbackTopic(ctx, e, topic, in)
}
func (s *Service) Archive(ctx context.Context, e identity.Envelope, topic string, expected int64, note string) (State, error) {
	if ctx == nil || expected < 1 || expected >= 1<<62 || !NoteValid(note) {
		return State{}, store.ErrInvalid
	}
	return s.repo.ArchiveTopic(ctx, e, topic, expected, note)
}
