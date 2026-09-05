// Package vindex provides authorized, immutable pgvector generations. It performs no inference.
package vindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"net/url"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Space includes every input and route property that can change vector meaning.
// Endpoint is a non-secret canonical provider endpoint, or the literal "default".
type Space struct {
	Provider      string `json:"provider"`
	Route         string `json:"route"`
	Endpoint      string `json:"endpoint"`
	Model         string `json:"model"`
	Revision      string `json:"revision"`
	Dimensions    int    `json:"dimensions"`
	Preprocessing string `json:"preprocessing"`
	InputType     string `json:"input_type"`
	Normalization string `json:"normalization"`
}

// Valid rejects incomplete identities and dimensions unsupported by pgvector.
func (s Space) Valid() bool {
	for _, v := range []string{s.Provider, s.Route, s.Model, s.Revision, s.Preprocessing, s.InputType, s.Normalization} {
		if len(v) < 1 || len(v) > 256 || strings.TrimSpace(v) != v || strings.ContainsAny(v, "\x00\r\n\t") {
			return false
		}
	}
	u, err := url.Parse(s.Endpoint)
	return s.Dimensions > 0 && s.Dimensions <= 16000 && (s.Endpoint == "default" || err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "")
}

// Key is the full descriptor identity, not merely the model name or dimensions.
func (s Space) Key() string { return Digest(s) }

// Digest fingerprints canonical typed metadata. Callers validate before persistence.
func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// TextHash binds each expected input to its resulting facet.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:])
}

// Origin is an expected facet identity, source association and exact embedding input.
type Origin struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	SourceID string `json:"source_id"`
	TextHash string `json:"text_hash"`
}

// Kind is a closed evidence taxonomy; facets are not permissions.
func Kind(s string) bool {
	switch s {
	case "topic", "entity", "measure", "dimension", "kpi", "relationship", "rule", "example":
		return true
	}
	return false
}

// Generation is an immutable staging manifest. Publishing requires every exact origin.
type Generation struct {
	ID               string   `json:"id"`
	Topic            string   `json:"topic"`
	Version          string   `json:"version"`
	Context          string   `json:"context"`
	SourceGeneration string   `json:"source_generation"`
	Space            Space    `json:"space"`
	Expected         []Origin `json:"expected"`
}

// Valid checks the complete bounded generation manifest.
func (g Generation) Valid() bool {
	for _, s := range []string{g.ID, g.Topic, g.Version, g.Context, g.SourceGeneration} {
		if !identity.Identifier(s) {
			return false
		}
	}
	if !g.Space.Valid() || len(g.Expected) < 1 || len(g.Expected) > 4096 {
		return false
	}
	seen := map[string]bool{}
	for _, o := range g.Expected {
		b, err := hex.DecodeString(o.TextHash)
		if !identity.Identifier(o.ID) || !identity.Identifier(o.SourceID) || !Kind(o.Kind) || err != nil || len(b) != 32 || strings.ToLower(o.TextHash) != o.TextHash || seen[o.ID] {
			return false
		}
		seen[o.ID] = true
	}
	return true
}

// Clone prevents caller-owned slices from becoming a shared generation state.
func (g Generation) Clone() Generation { g.Expected = append([]Origin(nil), g.Expected...); return g }

// Facet carries one vector with its exact source association and protected evidence.
type Facet struct {
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	SourceID string    `json:"source_id"`
	Text     string    `json:"text"`
	Vector   []float32 `json:"vector"`
}

// VectorLiteral validates and serializes pgvector text without SQL interpolation.
func VectorLiteral(v []float32, dimensions int) (string, error) {
	if dimensions < 1 || dimensions > 16000 || len(v) != dimensions {
		return "", store.ErrInvalid
	}
	var b strings.Builder
	b.WriteByte('[')
	nonzero := false
	for i, n := range v {
		if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
			return "", store.ErrInvalid
		}
		nonzero = nonzero || n != 0
		if i != 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(n), 'g', -1, 32))
	}
	b.WriteByte(']')
	if !nonzero {
		return "", store.ErrInvalid
	}
	return b.String(), nil
}

// CheckBatch rejects excess, duplicate, foreign or incorrectly associated embedding outputs.
func CheckBatch(g Generation, facets []Facet) error {
	if !g.Valid() || len(facets) < 1 || len(facets) > 64 {
		return store.ErrInvalid
	}
	origins := make(map[string]Origin, len(g.Expected))
	for _, o := range g.Expected {
		origins[o.ID] = o
	}
	seen := map[string]bool{}
	size := 0
	for _, f := range facets {
		o, ok := origins[f.ID]
		if !ok || seen[f.ID] || f.Kind != o.Kind || f.SourceID != o.SourceID || len(f.Text) < 1 || len(f.Text) > 4096 || TextHash(f.Text) != o.TextHash {
			return store.ErrInvalid
		}
		if _, err := VectorLiteral(f.Vector, g.Space.Dimensions); err != nil {
			return err
		}
		seen[f.ID] = true
		size += len(f.Text) + 4*len(f.Vector)
	}
	if size > 4<<20 {
		return store.ErrInvalid
	}
	return nil
}

// Publication is the version-fenced pointer observed by retrieval consumers.
type Publication struct {
	Revision   int64  `json:"revision"`
	Generation string `json:"generation"`
	Version    string `json:"version"`
	Archived   bool   `json:"archived"`
}

// Query addresses a signed topic/context; all members of a batch share one DB snapshot.
type Query struct {
	ID           string    `json:"id"`
	Topic        string    `json:"topic"`
	Context      string    `json:"context"`
	Space        Space     `json:"space"`
	Vector       []float32 `json:"vector"`
	Kinds        []string  `json:"kinds"`
	LimitPerKind int       `json:"limit_per_kind"`
}

// Valid checks a bounded exact-search request.
func (q Query) Valid() bool {
	if !identity.Identifier(q.ID) || !identity.Identifier(q.Topic) || !identity.Identifier(q.Context) || !q.Space.Valid() || len(q.Kinds) < 1 || len(q.Kinds) > 8 || q.LimitPerKind < 1 || q.LimitPerKind > 10 {
		return false
	}
	seen := map[string]bool{}
	for _, k := range q.Kinds {
		if !Kind(k) || seen[k] {
			return false
		}
		seen[k] = true
	}
	_, err := VectorLiteral(q.Vector, q.Space.Dimensions)
	return err == nil
}

// Hit retains identity and generation provenance, never an anonymous score/vector pair.
type Hit struct {
	ID               string  `json:"id"`
	Kind             string  `json:"kind"`
	SourceID         string  `json:"source_id"`
	Text             string  `json:"text"`
	Generation       string  `json:"generation"`
	Version          string  `json:"version"`
	SourceGeneration string  `json:"source_generation"`
	Distance         float64 `json:"distance"`
}

// Result preserves the original query identity and immutable publication snapshot.
type Result struct {
	ID          string      `json:"id"`
	Publication Publication `json:"publication"`
	Hits        []Hit       `json:"hits"`
}

// Removal selects exact cleanup coordinates. Empty Topic means explicit tenant erasure.
type Removal struct{ Topic, Version, Context string }

// Repository is implemented by the existing PostgreSQL metadata store, not a second backend.
type Repository interface {
	BeginGeneration(context.Context, store.Scope, Generation) error
	UpsertFacets(context.Context, store.Scope, Generation, []Facet) error
	PublishGeneration(context.Context, store.Scope, Generation, int64) (Publication, error)
	ArchiveGeneration(context.Context, store.Scope, string, string, int64) (Publication, error)
	SearchFacets(context.Context, store.Scope, []Query) ([]Result, error)
	DeleteFacets(context.Context, store.Scope, Removal) error
	ExplainFacets(context.Context, store.Scope, Query) (json.RawMessage, error)
}

// Service enforces verified authority before every repository operation. It intentionally
// has no evidence cache: neither identical input text nor a tenant label authorizes reuse.
type Service struct{ repo Repository }

// New binds the pgvector repository without starting workers or loading any learned model.
func New(repo Repository) (*Service, error) {
	if repo == nil || reflect.ValueOf(repo).Kind() == reflect.Ptr && reflect.ValueOf(repo).IsNil() {
		return nil, store.ErrInvalid
	}
	return &Service{repo: repo}, nil
}

func coordinate(e identity.Envelope, action, permission, topic, partition string) (store.Scope, error) {
	if err := access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: permission, ID: topic}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: partition}); err != nil {
		return store.Scope{}, err
	}
	return store.NewScope(e.Tenant(), e.User())
}
func deadline(ctx context.Context, e identity.Envelope) (context.Context, context.CancelFunc) {
	until := time.Now().Add(5 * time.Second)
	if e.Deadline().Before(until) {
		until = e.Deadline()
	}
	return context.WithDeadline(ctx, until)
}

// Begin creates or idempotently replays the same staging manifest.
func (s *Service) Begin(ctx context.Context, e identity.Envelope, g Generation) error {
	scope, err := coordinate(e, "topics.write", "write", g.Topic, g.Context)
	if err != nil {
		return err
	}
	if !g.Valid() {
		return store.ErrInvalid
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	return s.repo.BeginGeneration(ctx, scope, g.Clone())
}

// Upsert accepts only exact expected origins into an unpublished generation.
func (s *Service) Upsert(ctx context.Context, e identity.Envelope, g Generation, batch []Facet) error {
	scope, err := coordinate(e, "topics.write", "write", g.Topic, g.Context)
	if err != nil {
		return err
	}
	if err = CheckBatch(g, batch); err != nil {
		return err
	}
	copyBatch := append([]Facet(nil), batch...)
	for i := range copyBatch {
		copyBatch[i].Vector = append([]float32(nil), batch[i].Vector...)
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	return s.repo.UpsertFacets(ctx, scope, g.Clone(), copyBatch)
}

// Publish atomically seals complete facets and switches the versioned publication pointer.
func (s *Service) Publish(ctx context.Context, e identity.Envelope, g Generation, expected int64) (Publication, error) {
	scope, err := coordinate(e, "topics.publish", "publish", g.Topic, g.Context)
	if err != nil {
		return Publication{}, err
	}
	if !g.Valid() || expected < 0 || expected >= 1<<62 {
		return Publication{}, store.ErrInvalid
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	return s.repo.PublishGeneration(ctx, scope, g.Clone(), expected)
}

// Archive invalidates retrieval through a revision-checked pointer transition.
func (s *Service) Archive(ctx context.Context, e identity.Envelope, topic, partition string, expected int64) (Publication, error) {
	scope, err := coordinate(e, "topics.publish", "publish", topic, partition)
	if err != nil {
		return Publication{}, err
	}
	if expected < 1 || expected >= 1<<62 {
		return Publication{}, store.ErrInvalid
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	return s.repo.ArchiveGeneration(ctx, scope, topic, partition, expected)
}

// Search validates every member before any I/O and never returns a partial denied batch.
func (s *Service) Search(ctx context.Context, e identity.Envelope, queries []Query) ([]Result, error) {
	if len(queries) < 1 || len(queries) > 8 {
		return nil, store.ErrInvalid
	}
	seen := map[string]bool{}
	copyQueries := append([]Query(nil), queries...)
	var scope store.Scope
	for i, q := range queries {
		var err error
		scope, err = coordinate(e, "topics.read", "read", q.Topic, q.Context)
		if err != nil {
			return nil, err
		}
		if !q.Valid() || seen[q.ID] {
			return nil, store.ErrInvalid
		}
		seen[q.ID] = true
		copyQueries[i].Vector = append([]float32(nil), q.Vector...)
		copyQueries[i].Kinds = append([]string(nil), q.Kinds...)
		sort.Strings(copyQueries[i].Kinds)
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	out, err := s.repo.SearchFacets(ctx, scope, copyQueries)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Delete erases only addressed topic/version/context coordinates, or an explicitly authorized tenant.
func (s *Service) Delete(ctx context.Context, e identity.Envelope, r Removal) error {
	var scope store.Scope
	var err error
	if r.Topic == "" {
		if r.Context != "" || r.Version != "" {
			return store.ErrInvalid
		}
		scope, err = access.StoreScope(e, "ops.maintain", "erase")
	} else {
		scope, err = coordinate(e, "topics.write", "erase", r.Topic, r.Context)
		if r.Version != "" && !identity.Identifier(r.Version) {
			return store.ErrInvalid
		}
	}
	if err != nil {
		return err
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	return s.repo.DeleteFacets(ctx, scope, r)
}

// Explain provides an authorized fixed query-plan diagnostic without provider calls.
func (s *Service) Explain(ctx context.Context, e identity.Envelope, q Query) (json.RawMessage, error) {
	scope, err := coordinate(e, "topics.read", "read", q.Topic, q.Context)
	if err != nil {
		return nil, err
	}
	if !q.Valid() {
		return nil, store.ErrInvalid
	}
	ctx, cancel := deadline(ctx, e)
	defer cancel()
	return s.repo.ExplainFacets(ctx, scope, q)
}
