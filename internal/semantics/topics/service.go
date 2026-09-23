package topics

import (
	"context"
	"errors"
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

// Service coordinates reviewed topic publication and current health.
type Service struct {
	repo   Repository
	source *sources.Service
	index  *vindex.Service
	engine gateway.Engine
}

type reviewContractConfirmer interface {
	ConfirmReviewTopicContract(context.Context, identity.Envelope, string, int64) (Published, error)
}

// New constructs the reviewed publication service.
func New(repo Repository, source *sources.Service, index *vindex.Service, engine gateway.Engine) (*Service, error) {
	if repo == nil || source == nil || index == nil {
		return nil, store.ErrInvalid
	}
	return &Service{repo, source, index, engine}, nil
}

// NoteValid reports whether lifecycle evidence is bounded valid UTF-8.
func NoteValid(note string) bool { return len(note) > 0 && len(note) <= 1024 && utf8.ValidString(note) }

// Review records a decision for one exact private draft revision.
func (s *Service) Review(ctx context.Context, e identity.Envelope, topic string, in ReviewRequest) (Review, error) {
	if ctx == nil || in.DraftRevision < 1 || in.DraftRevision > drafts.MaxRevisions || !DigestValid(in.Digest) || !NoteValid(in.Note) || (in.Decision != "approve" && in.Decision != "reject") {
		return Review{}, store.ErrInvalid
	}
	return s.repo.ReviewTopic(ctx, e, topic, in)
}

// currentSources performs actual discovery, independently of private profile
// authorization. Retained published definition reads do not invoke this method.
func (s *Service) currentSources(ctx context.Context, e identity.Envelope, d Definition) error {
	return s.currentSourcesWith(ctx, e, d, s.source.Discover)
}

func (s *Service) currentSourcesWith(ctx context.Context, e identity.Envelope, d Definition, discover func(context.Context, identity.Envelope, string) (sources.Discovery, error)) error {
	_, err := currentRelations(ctx, e, d, discover)
	return err
}

func currentRelations(ctx context.Context, e identity.Envelope, d Definition, discover func(context.Context, identity.Envelope, string) (sources.Discovery, error)) ([]readexec.Relation, error) {
	catalogs := map[string]sources.Discovery{}
	relations := make([]readexec.Relation, 0, len(d.Datasets))
	for _, dataset := range d.Datasets {
		binding := dataset.Source
		catalog, ok := catalogs[binding.Source]
		if !ok {
			var err error
			catalog, err = discover(ctx, e, binding.Source)
			if err != nil {
				return nil, err
			}
			catalogs[binding.Source] = catalog
		}
		if catalog.ContextID != binding.Context || catalog.Revision != binding.SourceRevision {
			return nil, readexec.ErrBinding
		}
		var relation readexec.Relation
		for _, candidate := range catalog.Relations {
			if candidate.ID == dataset.ID {
				relation = candidate
				break
			}
		}
		if len(relation.Columns) == 0 {
			return nil, readexec.ErrBinding
		}
		reviewed := readexec.Relation{ID: relation.ID, Schema: relation.Schema, Name: relation.Name}
		for _, column := range dataset.Columns {
			found := false
			for _, actual := range relation.Columns {
				if actual.Name == column.SourceName && actual.NativeType == column.NativeType && actual.Category == column.Category && actual.Nullable == column.Nullable && actual.Safe {
					found = true
					reviewed.Columns = append(reviewed.Columns, actual)
					break
				}
			}
			if !found {
				return nil, readexec.ErrBinding
			}
		}
		relations = append(relations, reviewed)
	}
	return relations, nil
}

// Publish stages facets and atomically activates one reviewed definition.
func (s *Service) Publish(ctx context.Context, e identity.Envelope, topic string, in PublishRequest) (Published, error) {
	return s.PublishBounded(ctx, e, topic, in, gateway.Limits{Calls: 64, Tokens: 1 << 20, Duration: 30 * time.Second})
}

// PublishBounded performs publication with a caller-owned pessimistic gateway
// allowance. It is used by durable orchestrators that must reserve inference
// before the external effect and later reconcile the retained receipt.
func (s *Service) PublishBounded(ctx context.Context, e identity.Envelope, topic string, in PublishRequest, limits gateway.Limits) (Published, error) {
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
	changesRegistry, err := s.repo.CheckCanonicalMeanings(ctx, e, topic, drafts.Publish, model.CanonicalMeanings())
	if err != nil {
		return Published{}, err
	}
	if changesRegistry {
		if err = access.Require(e, "topics.publish", access.Tenant(e, "write")); err != nil {
			return Published{}, err
		}
	}
	if limits.Calls < 1 || limits.Tokens < 1 || limits.Duration <= 0 || limits.Duration > 30*time.Second {
		return Published{}, gateway.ErrBudget
	}
	ctx, cancel := context.WithTimeout(ctx, limits.Duration)
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
	budget, err := gateway.NewBudget(call, limits)
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
	out, err := s.repo.PublishTopic(ctx, e, Prepared{model: model, review: review, tenant: e.Tenant(), actor: e.User(), session: e.Session(), deadline: deadline, descriptor: descriptor}, generations, embedded.Receipt, in.Expected)
	if err == nil {
		out.Receipt = embedded.Receipt
	}
	return out, err
}

// Read returns the active or an exact retained published version.
func (s *Service) Read(ctx context.Context, e identity.Envelope, topic, version string) (Published, error) {
	return s.repo.ReadPublishedTopic(ctx, e, topic, version, drafts.Read)
}

// Contract verifies live source continuity before returning a definition.
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
	relations, err := currentRelations(ctx, e, current.Definition, s.source.Discover)
	if err != nil {
		return Contract{}, err
	}
	after, err := s.repo.ConfirmTopicContract(ctx, e, topic, current.State.Revision)
	if err != nil {
		return Contract{}, err
	}
	return Contract{Publication: after, ObservedAt: time.Now().UTC(), Relations: relations}, nil
}

// ReviewContract verifies the same current publication and live source
// continuity under feedback.write plus exact dependency reach. It exists for
// learned-example review and does not grant general topic reads.
func (s *Service) ReviewContract(ctx context.Context, e identity.Envelope, topic string) (Contract, error) {
	if ctx == nil {
		return Contract{}, store.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	current, err := s.repo.ReadPublishedTopic(ctx, e, topic, "", drafts.FeedbackRead)
	if err != nil {
		return Contract{}, err
	}
	if current.State.Archived {
		return Contract{}, store.ErrNotFound
	}
	if err = s.currentSourcesWith(ctx, e, current.Definition, s.source.ReviewDiscover); err != nil {
		return Contract{}, err
	}
	confirmer, ok := s.repo.(reviewContractConfirmer)
	if !ok {
		return Contract{}, store.ErrInvalid
	}
	after, err := confirmer.ConfirmReviewTopicContract(ctx, e, topic, current.State.Revision)
	if err != nil {
		return Contract{}, err
	}
	return Contract{Publication: after, ObservedAt: time.Now().UTC()}, nil
}

// RetainedContract returns one exact immutable publication without consulting
// the current publication pointer or current source catalog. Callers that
// execute against the retained definition must still resolve the live source
// binding before touching the warehouse. This is the historical-read seam for
// governed query replay after a topic transition or archive.
func (s *Service) RetainedContract(ctx context.Context, e identity.Envelope, topic, version string) (Contract, error) {
	if ctx == nil || !identity.Identifier(topic) || !identity.Identifier(version) {
		return Contract{}, store.ErrInvalid
	}
	published, err := s.Read(ctx, e, topic, version)
	if err != nil {
		return Contract{}, err
	}
	if published.State.Topic != topic || published.State.Version != version || published.Definition.Topic != topic || published.Definition.Version != version {
		return Contract{}, store.ErrConflict
	}
	return Contract{Publication: published, ObservedAt: time.Now().UTC()}, nil
}

// Health reads the latest retained observation without consulting private profiles
// or making a source/model request.
func (s *Service) Health(ctx context.Context, e identity.Envelope, topic string) (Health, error) {
	if ctx == nil || !identity.Identifier(topic) {
		return Health{}, store.ErrInvalid
	}
	current, err := s.Read(ctx, e, topic, "")
	if err != nil {
		return Health{}, err
	}
	return s.repo.ReadTopicHealth(ctx, e, current)
}

func healthAuthorization(err error) bool {
	return errors.Is(err, access.ErrUnauthenticated) || errors.Is(err, access.ErrForbidden)
}

// Recheck observes every current public source binding and atomically replaces the
// retained health snapshot only after the complete bounded observation finishes.
func (s *Service) Recheck(ctx context.Context, e identity.Envelope, topic string) (Health, error) {
	if ctx == nil || !identity.Identifier(topic) {
		return Health{}, store.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	current, err := s.Read(ctx, e, topic, "")
	if err != nil {
		return Health{}, err
	}
	issues := make([]HealthIssue, 0)
	catalogs := map[string]sources.Discovery{}
	failed := map[string]bool{}
	for _, dataset := range current.Definition.Datasets {
		binding := dataset.Source
		catalog, ok := catalogs[binding.Source]
		if !ok && !failed[binding.Source] {
			catalog, err = s.source.Discover(ctx, e, binding.Source)
			if err != nil {
				if healthAuthorization(err) {
					return Health{}, err
				}
				failed[binding.Source] = true
			}
			catalogs[binding.Source] = catalog
		}
		code := ""
		switch {
		case failed[binding.Source]:
			code = "source_unavailable"
		case catalog.ContextID != binding.Context || catalog.Revision != binding.SourceRevision:
			code = "source_revision_changed"
		default:
			var columns []readexec.Column
			for _, relation := range catalog.Relations {
				if relation.ID == dataset.ID {
					columns = relation.Columns
					break
				}
			}
			if len(columns) == 0 {
				code = "dataset_missing"
			} else {
				for _, expected := range dataset.Columns {
					matched := false
					for _, actual := range columns {
						if actual.Name == expected.SourceName && actual.NativeType == expected.NativeType && actual.Category == expected.Category && actual.Nullable == expected.Nullable && actual.Safe {
							matched = true
							break
						}
					}
					if !matched {
						code = "schema_changed"
						break
					}
				}
			}
		}
		if code != "" {
			issues = append(issues, HealthIssue{Source: binding.Source, Context: binding.Context, Dataset: dataset.ID, Code: code})
		}
	}
	return s.repo.SaveTopicHealth(ctx, e, current, issues)
}

// Rollback reactivates one exact retained version without gateway work.
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

// Archive makes the current topic and matching facet heads unavailable.
func (s *Service) Archive(ctx context.Context, e identity.Envelope, topic string, expected int64, note string) (State, error) {
	if ctx == nil || expected < 1 || expected >= 1<<62 || !NoteValid(note) {
		return State{}, store.ErrInvalid
	}
	return s.repo.ArchiveTopic(ctx, e, topic, expected, note)
}
