package rendering

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
)

// WorkerProtocolVersion pins the closed parent/worker wire contract.
const WorkerProtocolVersion = "chartworks-render-worker-v1"

var (
	// ErrBusy reports bounded worker admission exhaustion.
	ErrBusy = errors.New("rendering: busy")
	// ErrWorker reports a crashed or malformed isolated worker.
	ErrWorker = errors.New("rendering: worker failed")
	// ErrTimeout reports a worker killed at its time bound.
	ErrTimeout = errors.New("rendering: worker timeout")
	// ErrOutputLimit reports output that reached the configured hard cap.
	ErrOutputLimit = errors.New("rendering: output limit")
)

// Options pins worker identity, resource bounds and rendition retention.
type Options struct {
	WorkerVersion, ThemeVersion                  string
	MaxTime                                      time.Duration
	MaxMemoryBytes                               int64
	MaxInputBytes, MaxOutputBytes, MaxConcurrent int
	Retention                                    time.Duration
}

func (o Options) valid() bool {
	return o.WorkerVersion != "" && o.ThemeVersion != "" && o.MaxTime >= 100*time.Millisecond && o.MaxTime <= time.Minute && o.MaxMemoryBytes >= 32<<20 && o.MaxMemoryBytes <= 2<<30 && o.MaxInputBytes >= 1024 && o.MaxInputBytes <= 64<<20 && o.MaxOutputBytes >= 1024 && o.MaxOutputBytes <= 64<<20 && o.MaxConcurrent >= 1 && o.MaxConcurrent <= 16 && o.Retention >= time.Minute && o.Retention <= 90*24*time.Hour
}

// SealedWork is the complete credential-free worker input.
type SealedWork struct {
	Version string                       `json:"version"`
	Request Request                      `json:"request"`
	View    reporting.DeliveryViewResult `json:"view"`
}

// Processor transforms one sealed retained view into static bytes.
type Processor interface {
	Process(context.Context, SealedWork) (Rendition, error)
}

// Record is one tenant-bound immutable rendition and its authority coordinates.
type Record struct {
	Rendition              Rendition `json:"rendition"`
	Tenant, Actor, Session string
	Request                Request `json:"request"`
	Private                bool    `json:"private"`
}

// Repository persists and expires tenant-partitioned renditions.
type Repository interface {
	PutRendition(context.Context, Record) (Record, error)
	ReadRendition(context.Context, string, string) (Record, error)
	ListRenditions(context.Context, string, string, int) ([]Record, error)
	ExpireRenditions(context.Context, string, string, time.Time, int) (int64, error)
}

// NewManaged constructs a durable isolated rendition service.
func NewManaged(viewer Viewer, repo Repository, processor Processor, maxBytes int, options Options) (*Service, error) {
	if viewer == nil || repo == nil || processor == nil || maxBytes < 1024 || maxBytes > 64<<20 || !options.valid() {
		return nil, ErrInvalid
	}
	return &Service{viewer: viewer, repository: repo, processor: processor, maxBytes: maxBytes, options: options}, nil
}

// Durable reports whether immutable rendition persistence is mounted.
func (s *Service) Durable() bool { return s != nil && s.repository != nil && s.processor != nil }

// Generate renders and idempotently persists one authorized retained projection.
func (s *Service) Generate(ctx context.Context, e identity.Envelope, in Request) (Rendition, error) {
	if s == nil || s.repository == nil {
		return Rendition{}, ErrInvalid
	}
	r, err := s.Export(ctx, e, in)
	if err != nil {
		return Rendition{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(s.options.Retention)
	view, err := s.viewer.View(ctx, e, in.View)
	if err != nil {
		return Rendition{}, err
	}
	if !view.Summary.Expires.IsZero() && view.Summary.Expires.Before(expires) {
		expires = view.Summary.Expires
	}
	r.WorkerVersion, r.ThemeVersion, r.CreatedAt, r.ExpiresAt = s.options.WorkerVersion, s.options.ThemeVersion, now, expires
	r.ID = renditionID(e.Tenant(), in, r)
	record, err := s.repository.PutRendition(ctx, Record{Rendition: r, Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Request: in, Private: view.Summary.Private})
	if err != nil {
		return Rendition{}, err
	}
	return record.Rendition, nil
}

func renditionID(tenant string, in Request, r Rendition) string {
	b, _ := json.Marshal(struct {
		Tenant                string
		Request               Request
		Source, Worker, Theme string
	}{tenant, in, r.SourceDigest, r.WorkerVersion, r.ThemeVersion})
	h := sha256.Sum256(b)
	return "rnd-" + hex.EncodeToString(h[:16])
}

// ReadRequest selects one rendition identifier.
type ReadRequest struct {
	ID string `json:"id"`
}

// ListRequest selects one bounded tenant rendition page.
type ListRequest struct {
	After string `json:"after"`
	Limit int    `json:"limit"`
}

// ListResult contains renditions that remain currently authorized.
type ListResult struct {
	Items []Rendition `json:"items"`
}

// ExpireRequest bounds one retention deletion pass.
type ExpireRequest struct {
	Limit int `json:"limit"`
}

// ExpireResult reports deleted rendition records.
type ExpireResult struct {
	Count int64 `json:"count"`
}

// Read rechecks current artifact authority before returning retained bytes.
func (s *Service) Read(ctx context.Context, e identity.Envelope, in ReadRequest) (Rendition, error) {
	if s == nil || s.repository == nil || !e.Valid() {
		return Rendition{}, access.ErrUnauthenticated
	}
	if !identity.Identifier(in.ID) {
		return Rendition{}, ErrInvalid
	}
	if !e.Has("reporting.read") {
		return Rendition{}, access.ErrForbidden
	}
	record, err := s.repository.ReadRendition(ctx, e.Tenant(), in.ID)
	if err != nil {
		return Rendition{}, err
	}
	if time.Now().After(record.Rendition.ExpiresAt) {
		return Rendition{}, reporting.ErrExpired
	}
	// Re-read the artifact through its current exact reach. This performs no execution.
	if _, err = s.viewer.View(ctx, e, record.Request.View); err != nil {
		return Rendition{}, err
	}
	return record.Rendition, nil
}

// List returns only renditions whose underlying artifact is currently readable.
func (s *Service) List(ctx context.Context, e identity.Envelope, in ListRequest) (ListResult, error) {
	if s == nil || s.repository == nil || !e.Valid() {
		return ListResult{}, access.ErrUnauthenticated
	}
	if !e.Has("reporting.read") {
		return ListResult{}, access.ErrForbidden
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 || in.After != "" && !identity.Identifier(in.After) {
		return ListResult{}, ErrInvalid
	}
	records, err := s.repository.ListRenditions(ctx, e.Tenant(), in.After, in.Limit)
	if err != nil {
		return ListResult{}, err
	}
	out := ListResult{Items: []Rendition{}}
	for _, record := range records {
		if _, x := s.viewer.View(ctx, e, record.Request.View); x == nil && time.Now().Before(record.Rendition.ExpiresAt) {
			out.Items = append(out.Items, record.Rendition)
		}
	}
	return out, nil
}

// Expire erases expired rendition bytes under tenant retention authority.
func (s *Service) Expire(ctx context.Context, e identity.Envelope, in ExpireRequest) (ExpireResult, error) {
	if s == nil || s.repository == nil || !e.Valid() {
		return ExpireResult{}, access.ErrUnauthenticated
	}
	if err := access.Require(e, "reporting.retention", access.Resource{Tenant: e.Tenant(), Kind: "tenant", Permission: "erase", ID: e.Tenant()}); err != nil {
		return ExpireResult{}, err
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return ExpireResult{}, ErrInvalid
	}
	n, err := s.repository.ExpireRenditions(ctx, e.Tenant(), e.User(), time.Now(), in.Limit)
	return ExpireResult{Count: n}, err
}

// MemoryRepository is deterministic test/dev persistence; production uses PostgreSQL.
type MemoryRepository struct {
	mu   sync.Mutex
	rows map[string]Record
}

// NewMemoryRepository constructs deterministic test-only rendition storage.
func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{rows: map[string]Record{}} }

// PutRendition idempotently retains one in-memory rendition.
func (m *MemoryRepository) PutRendition(_ context.Context, r Record) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := r.Tenant + "/" + r.Rendition.ID
	if old, ok := m.rows[k]; ok {
		return old, nil
	}
	m.rows[k] = r
	return r, nil
}

// ReadRendition reads one tenant-exact in-memory rendition.
func (m *MemoryRepository) ReadRendition(_ context.Context, t, id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[t+"/"+id]
	if !ok {
		return Record{}, access.ErrNotFound
	}
	return r, nil
}

// ListRenditions returns one ordered tenant-exact page.
func (m *MemoryRepository) ListRenditions(_ context.Context, t, after string, limit int) ([]Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := []string{}
	for k, r := range m.rows {
		if r.Tenant == t && k > t+"/"+after {
			ids = append(ids, k)
		}
	}
	sort.Strings(ids)
	if len(ids) > limit {
		ids = ids[:limit]
	}
	out := []Record{}
	for _, k := range ids {
		out = append(out, m.rows[k])
	}
	return out, nil
}

// ExpireRenditions deletes one bounded tenant-exact expiry page.
func (m *MemoryRepository) ExpireRenditions(_ context.Context, t, _ string, as time.Time, limit int) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for k, r := range m.rows {
		if n < int64(limit) && r.Tenant == t && !r.Rendition.ExpiresAt.After(as) {
			delete(m.rows, k)
			n++
		}
	}
	return n, nil
}
