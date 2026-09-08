// Package drafts admits private, immutable topic authoring revisions. Drafts are
// not published topics, executable plans, or evidence of current source health.
package drafts

import (
	"context"
	"encoding/json"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// MaxRevisions bounds retained private revisions for one topic.
const MaxRevisions = 128

// Access is closed: callers cannot substitute an unrelated signed action.
type Access uint8

const (
	// Read selects retained private draft access.
	Read Access = iota + 1
	// Write selects private draft mutation access.
	Write
	// Export selects neutral private draft export access.
	Export
	// Review selects publication review access.
	Review
	// Publish selects publication mutation access.
	Publish
)

// Action returns the fixed Pengui action for this access mode.
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

// Permission returns the topic permission for this access mode.
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

// Require enforces one closed draft access mode for a topic.
func Require(e identity.Envelope, topic string, a Access) error {
	if a.Action() == "" {
		return store.ErrInvalid
	}
	return access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: a.Permission(), ID: topic})
}

// RequirePack enforces topic and dependency reach for a complete pack.
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

// Revision is retained private draft metadata.
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

// Version contains one immutable private draft snapshot.
type Version struct {
	Metadata Revision            `json:"metadata"`
	Pack     semantics.TopicPack `json:"pack"`
}

// SaveRequest creates one CAS-fenced private revision.
type SaveRequest struct {
	Expected int64               `json:"expected_revision"`
	Pack     semantics.TopicPack `json:"pack"`
	Change   string              `json:"change"`
}

// ImportRequest binds a neutral portable pack into a private draft.
type ImportRequest struct {
	Expected int64                   `json:"expected_revision"`
	Portable semantics.PortablePack  `json:"portable"`
	Bindings semantics.DraftBindings `json:"bindings"`
	Change   string                  `json:"change"`
}

// ColumnRebinding maps one stable semantic column ID to a discovered source name.
type ColumnRebinding struct {
	Column     string `json:"column"`
	SourceName string `json:"source_name"`
}

// RebindRequest creates a new draft revision against one active target profile.
type RebindRequest struct {
	Expected int64             `json:"expected_revision"`
	Version  string            `json:"version"`
	Dataset  string            `json:"dataset"`
	Profile  string            `json:"profile"`
	Columns  []ColumnRebinding `json:"columns"`
	Change   string            `json:"change"`
}

// EntityMutationRequest applies a bounded atomic entity batch to an exact draft head.
type EntityMutationRequest struct {
	Expected  int64                      `json:"expected_revision"`
	Version   string                     `json:"version"`
	Mutations []semantics.EntityMutation `json:"mutations"`
	Change    string                     `json:"change"`
}

// OnboardRequest creates an unresolved draft scaffold from private profile evidence.
type OnboardRequest struct {
	Topic       string `json:"topic"`
	Version     string `json:"version"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Profile     string `json:"profile"`
	Change      string `json:"change"`
}

// Prepared is an ephemeral admission proof issued only after public source/profile
// checks. The database still fences source revisions and retained evidence at commit.
type Prepared struct {
	model                  semantics.Model
	tenant, actor, session string
	deadline               time.Time
	generation             *GenerationCheckpoint
}

// Pack returns the admitted pack only to its original authority.
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

// GenerationCheckpoint is sealed with a generated draft and committed atomically.
type GenerationCheckpoint struct {
	Cursor   int             `json:"cursor"`
	Complete bool            `json:"complete"`
	Receipt  gateway.Receipt `json:"receipt"`
}

// Generation returns a detached admitted checkpoint for the same authority.
func (p Prepared) Generation(e identity.Envelope) (GenerationCheckpoint, bool, error) {
	if _, err := p.Pack(e); err != nil {
		return GenerationCheckpoint{}, false, err
	}
	if p.generation == nil {
		return GenerationCheckpoint{}, false, nil
	}
	out := *p.generation
	out.Receipt.Calls = append([]gateway.Usage(nil), p.generation.Receipt.Calls...)
	return out, true, nil
}

// Repository owns tenant-composite identity, pre-query scope restrictions, CAS,
// immutable snapshots and an audit event in the same transaction.
type Repository interface {
	CheckCanonicalMeanings(context.Context, identity.Envelope, string, Access, []semantics.CanonicalMeaning) (bool, error)
	SaveTopicDraft(context.Context, identity.Envelope, Prepared, int64, string) (Version, error)
	ReadTopicDraft(context.Context, identity.Envelope, string, int64, Access) (Version, error)
	TopicDraftHistory(context.Context, identity.Envelope, string, int64, int) ([]Revision, error)
	TopicGenerationCheckpoint(context.Context, identity.Envelope, string, int64) (GenerationCheckpoint, bool, error)
}

// Service coordinates private draft admission and persistence.
type Service struct {
	repo     Repository
	sources  *sources.Service
	profiles *engineering.Service
	engine   gateway.Engine
}

// New creates a draft service without optional gateway enhancement.
func New(repo Repository, s *sources.Service, p *engineering.Service) (*Service, error) {
	if repo == nil || s == nil || p == nil {
		return nil, store.ErrInvalid
	}
	return &Service{repo: repo, sources: s, profiles: p}, nil
}

// NewWithEngine adds the sole production Bifrost seam used by bounded draft enhancement.
func NewWithEngine(repo Repository, s *sources.Service, p *engineering.Service, engine gateway.Engine) (*Service, error) {
	service, err := New(repo, s, p)
	if err != nil {
		return nil, err
	}
	service.engine = engine
	return service, nil
}

// Save admits and persists one immutable private draft revision.
func (s *Service) Save(ctx context.Context, e identity.Envelope, in SaveRequest) (Version, error) {
	return s.save(ctx, e, in, nil)
}

func (s *Service) save(ctx context.Context, e identity.Envelope, in SaveRequest, generation *GenerationCheckpoint) (Version, error) {
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
	return s.repo.SaveTopicDraft(ctx, e, Prepared{model: model, tenant: e.Tenant(), actor: e.User(), session: e.Session(), deadline: deadline, generation: generation}, in.Expected, in.Change)
}

// Read returns the current or an exact retained private draft revision.
func (s *Service) Read(ctx context.Context, e identity.Envelope, id string, revision int64) (Version, error) {
	return s.repo.ReadTopicDraft(ctx, e, id, revision, Read)
}

// History returns bounded descending private revision metadata.
func (s *Service) History(ctx context.Context, e identity.Envelope, id string, before int64, limit int) ([]Revision, error) {
	return s.repo.TopicDraftHistory(ctx, e, id, before, limit)
}

// Diff compares two exact retained private revisions.
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

// Export maps an exact private revision into neutral logical slots.
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

// Import admits a mapped portable pack through the normal save path.
func (s *Service) Import(ctx context.Context, e identity.Envelope, in ImportRequest) (Version, error) {
	candidate, err := semantics.ImportDraftCandidate(in.Portable, in.Bindings)
	if err != nil {
		return Version{}, err
	}
	return s.Save(ctx, e, SaveRequest{in.Expected, candidate.Pack(), in.Change})
}

func validEditRequest(ctx context.Context, expected int64, version, change string) bool {
	return ctx != nil && expected > 0 && expected < MaxRevisions && identity.Identifier(version) && len(change) >= 1 && len(change) <= 1024 && utf8.ValidString(change)
}

// MutateEntities creates one new immutable draft revision from a bounded atomic
// entity batch. It never edits the retained prior revision in place.
func (s *Service) MutateEntities(ctx context.Context, e identity.Envelope, topic string, in EntityMutationRequest) (Version, error) {
	if !identity.Identifier(topic) || !validEditRequest(ctx, in.Expected, in.Version, in.Change) {
		return Version{}, store.ErrInvalid
	}
	current, err := s.repo.ReadTopicDraft(ctx, e, topic, in.Expected, Write)
	if err != nil {
		return Version{}, err
	}
	model, err := semantics.Compile(current.Pack)
	if err != nil {
		return Version{}, err
	}
	changed, err := semantics.MutateEntities(model, in.Version, in.Mutations)
	if err != nil {
		return Version{}, err
	}
	return s.Save(ctx, e, SaveRequest{Expected: in.Expected, Pack: changed.Pack(), Change: in.Change})
}

// RebindDataset moves a logical dataset to exact active profile evidence. The
// caller maps stable semantic column IDs to discovered physical names; source,
// context, dataset, revision, type, category, and nullability come from evidence.
func (s *Service) RebindDataset(ctx context.Context, e identity.Envelope, topic string, in RebindRequest) (Version, error) {
	if !identity.Identifier(topic) || !validEditRequest(ctx, in.Expected, in.Version, in.Change) || !identity.Identifier(in.Dataset) || !identity.Identifier(in.Profile) || len(in.Columns) < 1 || len(in.Columns) > 256 {
		return Version{}, store.ErrInvalid
	}
	current, err := s.repo.ReadTopicDraft(ctx, e, topic, in.Expected, Write)
	if err != nil {
		return Version{}, err
	}
	model, err := semantics.Compile(current.Pack)
	if err != nil {
		return Version{}, err
	}
	evidence, err := s.profiles.Evidence(ctx, e, in.Profile)
	if err != nil {
		return Version{}, err
	}
	if !evidence.Active || evidence.Profile.Version != in.Profile {
		return Version{}, readexec.ErrBinding
	}
	oldColumns := map[string]semantics.Column{}
	for _, dataset := range current.Pack.Datasets {
		if dataset.ID == in.Dataset {
			for _, column := range dataset.Columns {
				oldColumns[column.ID] = column
			}
		}
	}
	actual := map[string]readexec.Column{}
	for _, column := range evidence.Profile.Schema {
		actual[column.Name] = column
	}
	columns := make([]semantics.Column, 0, len(in.Columns))
	seenPhysical := map[string]bool{}
	for _, mapping := range in.Columns {
		old, ok := oldColumns[mapping.Column]
		discovered, found := actual[mapping.SourceName]
		if !ok || !found || !discovered.Safe || seenPhysical[mapping.SourceName] {
			return Version{}, readexec.ErrBinding
		}
		seenPhysical[mapping.SourceName] = true
		columns = append(columns, semantics.Column{ID: old.ID, SourceName: discovered.Name, Name: old.Name, NativeType: discovered.NativeType, Category: discovered.Category, Nullable: discovered.Nullable})
	}
	replacement := semantics.DatasetReplacement{Dataset: evidence.Profile.Dataset, Source: semantics.SourceReference{Source: evidence.Profile.Source, Context: evidence.Profile.Context, Dataset: evidence.Profile.Dataset, ProfileVersion: evidence.Profile.Version, ProfileDigest: evidence.Profile.DeterministicHash(), SourceRevision: evidence.Profile.SourceRevision}, Columns: columns}
	changed, err := semantics.ReplaceDataset(model, in.Version, in.Dataset, replacement)
	if err != nil {
		return Version{}, err
	}
	return s.Save(ctx, e, SaveRequest{Expected: in.Expected, Pack: changed.Pack(), Change: in.Change})
}

// OnboardProfile creates a deterministic unresolved topic scaffold from one
// active private profile. It invents no measures, dimensions, joins, or rules.
func (s *Service) OnboardProfile(ctx context.Context, e identity.Envelope, in OnboardRequest) (Version, error) {
	if ctx == nil || !identity.Identifier(in.Topic) || !identity.Identifier(in.Version) || !identity.Identifier(in.Profile) || len(in.Change) < 1 || len(in.Change) > 1024 || !utf8.ValidString(in.Change) {
		return Version{}, store.ErrInvalid
	}
	if err := Require(e, in.Topic, Write); err != nil {
		return Version{}, err
	}
	if err := access.Require(e, "topics.write", access.Tenant(e, "write")); err != nil {
		return Version{}, err
	}
	evidence, err := s.profiles.Evidence(ctx, e, in.Profile)
	if err != nil {
		return Version{}, err
	}
	if !evidence.Active || evidence.Profile.Version != in.Profile {
		return Version{}, readexec.ErrBinding
	}
	columns := make([]semantics.Column, 0, len(evidence.Profile.Schema))
	for _, column := range evidence.Profile.Schema {
		if column.Safe {
			columns = append(columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
		}
	}
	pack := semantics.TopicPack{SchemaVersion: semantics.SchemaVersion, Topic: in.Topic, Version: in.Version, Name: in.Name, Description: in.Description, Datasets: []semantics.Dataset{{ID: evidence.Profile.Dataset, Name: evidence.Profile.Dataset, Source: semantics.SourceReference{Source: evidence.Profile.Source, Context: evidence.Profile.Context, Dataset: evidence.Profile.Dataset, ProfileVersion: evidence.Profile.Version, ProfileDigest: evidence.Profile.DeterministicHash(), SourceRevision: evidence.Profile.SourceRevision}, Columns: columns}}}
	return s.Save(ctx, e, SaveRequest{Pack: pack, Change: in.Change})
}

// EnhanceRequest advances one bounded generation step over stable draft columns.
type EnhanceRequest struct {
	Expected int64  `json:"expected_revision"`
	Version  string `json:"version"`
	Cursor   int    `json:"cursor"`
	Limit    int    `json:"limit"`
	Change   string `json:"change"`
}

// EnhanceResult returns the committed draft checkpoint and its next stable cursor.
type EnhanceResult struct {
	Draft      Version         `json:"draft"`
	NextCursor int             `json:"next_cursor"`
	Complete   bool            `json:"complete"`
	Receipt    gateway.Receipt `json:"receipt"`
}

type enhancementWire struct {
	Results []semantics.Enhancement `json:"results"`
}

var enhancementSchema = []byte(`{"type":"object","additionalProperties":false,"required":["results"],"properties":{"results":{"type":"array","minItems":1,"maxItems":32,"items":{"oneOf":[{"type":"object","additionalProperties":false,"required":["dataset","column","kind","name","aggregation"],"properties":{"dataset":{"type":"string","minLength":1,"maxLength":128},"column":{"type":"string","minLength":1,"maxLength":128},"kind":{"const":"measure"},"name":{"type":"string","minLength":1,"maxLength":256},"aggregation":{"enum":["sum","average","minimum","maximum","count","distinct_count"]}}},{"type":"object","additionalProperties":false,"required":["dataset","column","kind","name","role"],"properties":{"dataset":{"type":"string","minLength":1,"maxLength":128},"column":{"type":"string","minLength":1,"maxLength":128},"kind":{"const":"dimension"},"name":{"type":"string","minLength":1,"maxLength":256},"role":{"enum":["categorical","temporal","numeric","boolean","identifier"]}}},{"type":"object","additionalProperties":false,"required":["dataset","column","kind","reason"],"properties":{"dataset":{"type":"string","minLength":1,"maxLength":128},"column":{"type":"string","minLength":1,"maxLength":128},"kind":{"const":"unresolved"},"reason":{"type":"string","minLength":1,"maxLength":256}}}]}}}}`)

type enhancementColumn struct {
	Dataset  string `json:"dataset"`
	Column   string `json:"column"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Nullable bool   `json:"nullable"`
}

// Enhance performs one resumable Bifrost step and persists the accepted result
// as a normal immutable draft revision. Unresolved results remain in that pack.
func (s *Service) Enhance(ctx context.Context, e identity.Envelope, topic string, in EnhanceRequest) (EnhanceResult, error) {
	if !identity.Identifier(topic) || !validEditRequest(ctx, in.Expected, in.Version, in.Change) || in.Cursor < 0 || in.Limit < 1 || in.Limit > 32 {
		return EnhanceResult{}, store.ErrInvalid
	}
	if s.engine == nil {
		return EnhanceResult{}, gateway.ErrDisabled
	}
	current, err := s.repo.ReadTopicDraft(ctx, e, topic, 0, Write)
	if err != nil {
		return EnhanceResult{}, err
	}
	if current.Metadata.Revision != in.Expected {
		return EnhanceResult{}, store.ErrConflict
	}
	priorCheckpoint, exists, err := s.repo.TopicGenerationCheckpoint(ctx, e, topic, current.Metadata.Revision)
	if err != nil {
		return EnhanceResult{}, err
	}
	if (!exists && in.Cursor != 0) || (exists && (priorCheckpoint.Complete || priorCheckpoint.Cursor != in.Cursor)) {
		return EnhanceResult{}, store.ErrConflict
	}
	model, err := semantics.Compile(current.Pack)
	if err != nil {
		return EnhanceResult{}, err
	}
	columns := semantics.GenerationColumns(model)
	if in.Cursor >= len(columns) {
		return EnhanceResult{}, store.ErrInvalid
	}
	end := min(in.Cursor+in.Limit, len(columns))
	selected := columns[in.Cursor:end]
	pack := model.Pack()
	input := make([]enhancementColumn, 0, len(selected))
	resources := []access.Resource{{Tenant: e.Tenant(), Kind: "topic", Permission: "write", ID: topic}}
	for _, ref := range selected {
		for _, dataset := range pack.Datasets {
			if dataset.ID != ref.Dataset {
				continue
			}
			resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: dataset.Source.Source}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset.ID}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: dataset.Source.Context})
			for _, column := range dataset.Columns {
				if column.ID == ref.ID {
					input = append(input, enhancementColumn{Dataset: dataset.ID, Column: column.ID, Name: column.Name, Category: column.Category, Nullable: column.Nullable})
				}
			}
		}
	}
	if len(input) != len(selected) {
		return EnhanceResult{}, store.ErrInvalid
	}
	call, err := gateway.Authorize(e, "topics.write", model.Digest(), resources...)
	if err != nil {
		return EnhanceResult{}, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 1, Tokens: 64 << 10, Duration: 30 * time.Second})
	if err != nil {
		return EnhanceResult{}, err
	}
	schema, err := gateway.NewSchema("topic_enhancement_step", enhancementSchema)
	if err != nil {
		return EnhanceResult{}, err
	}
	prompt, err := json.Marshal(struct {
		Topic   string              `json:"topic"`
		Version string              `json:"version"`
		Columns []enhancementColumn `json:"columns"`
	}{Topic: topic, Version: in.Version, Columns: input})
	if err != nil {
		return EnhanceResult{}, store.ErrInvalid
	}
	generated, err := s.engine.Generate(ctx, call, budget, "enhance", "Classify every supplied column exactly once as a measure, dimension, or unresolved. Preserve supplied dataset and column IDs. Never invent SQL, joins, KPIs, canonical meaning, source coordinates, or credentials.", string(prompt), schema)
	if err != nil {
		return EnhanceResult{}, err
	}
	var wire enhancementWire
	if json.Unmarshal(generated.JSON, &wire) != nil || len(wire.Results) != len(selected) {
		return EnhanceResult{}, gateway.ErrOutput
	}
	want := map[semantics.Reference]bool{}
	for _, ref := range selected {
		want[ref] = true
	}
	for _, item := range wire.Results {
		ref := semantics.Reference{Kind: semantics.KindColumn, Dataset: item.Dataset, ID: item.Column}
		if !want[ref] {
			return EnhanceResult{}, gateway.ErrOutput
		}
		delete(want, ref)
	}
	if len(want) != 0 {
		return EnhanceResult{}, gateway.ErrOutput
	}
	changed, err := semantics.ApplyEnhancements(model, in.Version, wire.Results)
	if err != nil {
		return EnhanceResult{}, err
	}
	checkpoint := &GenerationCheckpoint{Cursor: end, Complete: end == len(columns), Receipt: generated.Receipt}
	draft, err := s.save(ctx, e, SaveRequest{Expected: in.Expected, Pack: changed.Pack(), Change: in.Change}, checkpoint)
	if err != nil {
		return EnhanceResult{}, err
	}
	return EnhanceResult{Draft: draft, NextCursor: end, Complete: end == len(columns), Receipt: generated.Receipt}, nil
}
