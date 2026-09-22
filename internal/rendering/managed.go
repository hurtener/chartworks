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

const WorkerProtocolVersion = "chartworks-render-worker-v1"

var (
	ErrBusy        = errors.New("rendering: busy")
	ErrWorker      = errors.New("rendering: worker failed")
	ErrTimeout     = errors.New("rendering: worker timeout")
	ErrOutputLimit = errors.New("rendering: output limit")
)

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

type SealedWork struct {
	Version string                       `json:"version"`
	Request Request                      `json:"request"`
	View    reporting.DeliveryViewResult `json:"view"`
}

type Processor interface {
	Process(context.Context, SealedWork) (Rendition, error)
}

type Record struct {
	Rendition              Rendition `json:"rendition"`
	Tenant, Actor, Session string
	Request                Request `json:"request"`
	Private                bool    `json:"private"`
}

type Repository interface {
	PutRendition(context.Context, Record) (Record, error)
	ReadRendition(context.Context, string, string) (Record, error)
	ListRenditions(context.Context, string, string, int) ([]Record, error)
	ExpireRenditions(context.Context, string, time.Time, int) (int64, error)
}

func NewManaged(viewer Viewer, repo Repository, processor Processor, maxBytes int, options Options) (*Service, error) {
	if viewer == nil || repo == nil || processor == nil || maxBytes < 1024 || maxBytes > 64<<20 || !options.valid() {
		return nil, ErrInvalid
	}
	return &Service{viewer: viewer, repository: repo, processor: processor, maxBytes: maxBytes, options: options}, nil
}

func (s *Service) Durable() bool { return s != nil && s.repository != nil && s.processor != nil }

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

type ReadRequest struct {
	ID string `json:"id"`
}
type ListRequest struct {
	After string `json:"after"`
	Limit int    `json:"limit"`
}
type ListResult struct {
	Items []Rendition `json:"items"`
}
type ExpireRequest struct {
	Limit int `json:"limit"`
}
type ExpireResult struct {
	Count int64 `json:"count"`
}

func (s *Service) Read(ctx context.Context, e identity.Envelope, in ReadRequest) (Rendition, error) {
	if s == nil || s.repository == nil || !e.Valid() {
		return Rendition{}, access.ErrUnauthenticated
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

func (s *Service) List(ctx context.Context, e identity.Envelope, in ListRequest) (ListResult, error) {
	if !e.Valid() {
		return ListResult{}, access.ErrUnauthenticated
	}
	if !e.Has("reporting.read") {
		return ListResult{}, access.ErrForbidden
	}
	if in.Limit == 0 {
		in.Limit = 20
	}
	if in.Limit < 1 || in.Limit > 100 {
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

func (s *Service) Expire(ctx context.Context, e identity.Envelope, in ExpireRequest) (ExpireResult, error) {
	if !e.Valid() {
		return ExpireResult{}, access.ErrUnauthenticated
	}
	if err := access.Require(e, "reporting.retention", access.Resource{Tenant: e.Tenant(), Kind: "tenant", Permission: "erase", ID: e.Tenant()}); err != nil {
		return ExpireResult{}, err
	}
	if in.Limit < 1 || in.Limit > 1000 {
		return ExpireResult{}, ErrInvalid
	}
	n, err := s.repository.ExpireRenditions(ctx, e.Tenant(), time.Now(), in.Limit)
	return ExpireResult{Count: n}, err
}

// MemoryRepository is deterministic test/dev persistence; production uses PostgreSQL.
type MemoryRepository struct {
	mu   sync.Mutex
	rows map[string]Record
}

func NewMemoryRepository() *MemoryRepository { return &MemoryRepository{rows: map[string]Record{}} }
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
func (m *MemoryRepository) ReadRendition(_ context.Context, t, id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[t+"/"+id]
	if !ok {
		return Record{}, access.ErrNotFound
	}
	return r, nil
}
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
func (m *MemoryRepository) ExpireRenditions(_ context.Context, t string, as time.Time, limit int) (int64, error) {
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
