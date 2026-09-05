// Package sources owns the governed source registry and PostgreSQL read adapter.
// Connector secrets remain operator references; Pengui alone supplies identity and reach.
package sources

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Source is the only public registry projection. It has no credential, connection
// string, operator environment reference or physical role field, even when empty.
type Source struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Dialect   string `json:"dialect"`
	Revision  int64  `json:"revision"`
	ContextID string `json:"context_id"`
	Status    string `json:"status"`
}

// CreateRequest may select an operator-approved tenant alias, not arbitrary addresses or secrets.
type CreateRequest struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Connection string `json:"connection"`
}

// Record is private persistence metadata. It never contains resolved secret bytes.
// HTTP/SDK read/list models use Source, not this internal type.
type Record struct {
	Source     Source
	Connection string
	Binding    readexec.Binding
}

// Valid checks coherent immutable source/context coordinates before a store accepts them.
func (r Record) Valid() bool {
	return identity.Identifier(r.Source.ID) && len(r.Source.ID) <= 80 && len(r.Source.Name) > 0 && len(r.Source.Name) <= 128 && !strings.ContainsAny(r.Source.Name, "\x00\r\n\t") && r.Source.Dialect == "postgres" && r.Source.Revision > 0 && r.Source.Revision < 1<<62 && r.Source.ContextID == contextID(r.Source.ID, r.Source.Revision) && r.Source.Status == "registered" && identity.Identifier(r.Connection) && r.Binding.Valid() && r.Binding.Source == r.Source.ID && r.Binding.Context == r.Source.ContextID && r.Binding.Revision == r.Source.Revision && r.Binding.Dialect == r.Source.Dialect
}

// Repository applies tenant selections in SQL and holds a shared current-revision
// fence while executing WithSource. Rotation therefore cannot race an old-context read.
type Repository interface {
	PutSource(context.Context, store.Scope, int64, Record) error
	ReadSource(context.Context, store.Scope, string) (Record, error)
	ListSources(context.Context, store.Scope, access.Selection, int) ([]Source, error)
	WithSource(context.Context, store.Scope, string, func(context.Context, Record) error) error
}

// Status is a live bounded probe receipt, distinct from the registry's registration state.
type Status struct {
	Available  bool      `json:"available"`
	Revision   int64     `json:"revision"`
	ContextID  string    `json:"context_id"`
	ObservedAt time.Time `json:"observed_at"`
}

// Discovery exposes only configured, actually discovered relations and safe type categories.
type Discovery struct {
	SourceID  string              `json:"source_id"`
	ContextID string              `json:"context_id"`
	Revision  int64               `json:"revision"`
	Relations []readexec.Relation `json:"relations"`
}

// Rows is a bounded native text result. Numeric, temporal and binary representations
// remain lossless; nil cells represent database NULL rather than fabricated zero.
type Rows struct {
	Columns []string    `json:"columns"`
	Values  [][]*string `json:"values"`
}

type poolEntry struct {
	material string
	pool     *pgxpool.Pool
}

// Service shares immutable configuration and bounded credential-specific pools.
// Lifecycle locks join all in-flight work before releasing connection pools.
type Service struct {
	repo      Repository
	settings  config.Sources
	lookup    func(string) (string, bool)
	lifecycle sync.RWMutex
	closed    bool
	mu        sync.Mutex
	pools     map[string]poolEntry
}

var _ readexec.ReadAdapter = (*Service)(nil)

// New constructs registry metadata access without connecting to a warehouse.
func New(repo Repository, settings config.Sources, lookup func(string) (string, bool)) (*Service, error) {
	if repo == nil || reflect.ValueOf(repo).Kind() == reflect.Ptr && reflect.ValueOf(repo).IsNil() || lookup == nil || config.ValidateSources(settings) != nil {
		return nil, store.ErrInvalid
	}
	return &Service{repo: repo, settings: settings.Clone(), lookup: lookup, pools: map[string]poolEntry{}}, nil
}

// Enabled reports execution capability, not the availability of retained metadata.
func (s *Service) Enabled() bool { return s.settings.Enabled }

// Close joins bounded operations and idempotently releases every pool.
func (s *Service) Close() {
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, entry := range s.pools {
		entry.pool.Close()
		delete(s.pools, key)
	}
}
func (s *Service) call(ctx context.Context, e identity.Envelope, warehouse bool, fn func(context.Context) error) error {
	if ctx == nil || !e.Valid() {
		return access.ErrUnauthenticated
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed || warehouse && !s.settings.Enabled {
		return store.ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	ctx, bound := context.WithTimeout(ctx, 4*time.Second)
	defer bound()
	return fn(ctx)
}
func contextID(id string, revision int64) string { return id + ":v" + strconv.FormatInt(revision, 10) }
func sourceScope(e identity.Envelope, action, permission, id string) (store.Scope, error) {
	if err := access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: id}); err != nil {
		return store.Scope{}, err
	}
	return store.NewScope(e.Tenant(), e.User())
}
func actualContext(e identity.Envelope, action string, record Record) error {
	return access.Require(e, action, access.Resource{Tenant: record.Binding.Tenant, Kind: "execution_context", Permission: "use", ID: record.Source.ContextID})
}
func (s *Service) connection(tenant, id string) (config.SourceConnection, error) {
	for _, c := range s.settings.Connections {
		if c.Tenant == tenant && c.ID == id {
			return c, nil
		}
	}
	return config.SourceConnection{}, store.ErrNotFound
}

// Create probes only a tenant-approved alias and registers its actual immutable context.
func (s *Service) Create(ctx context.Context, e identity.Envelope, r CreateRequest) (out Source, err error) {
	scope, err := access.StoreScope(e, "sources.write", "write")
	if err != nil {
		return out, err
	}
	if !identity.Identifier(r.ID) || len(r.ID) > 80 || !identity.Identifier(r.Connection) || len(r.Name) < 1 || len(r.Name) > 128 || strings.TrimSpace(r.Name) != r.Name || strings.ContainsAny(r.Name, "\x00\r\n\t") {
		return out, store.ErrInvalid
	}
	connection, err := s.connection(e.Tenant(), r.Connection)
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		binding, e2 := s.probe(ctx, connection, r.ID, 1, nil)
		if e2 != nil {
			return e2
		}
		record := Record{Source: Source{ID: r.ID, Name: r.Name, Dialect: "postgres", Revision: 1, ContextID: contextID(r.ID, 1), Status: "registered"}, Connection: r.Connection, Binding: binding}
		if e2 = s.repo.PutSource(ctx, scope, 0, record); e2 != nil {
			return e2
		}
		out = record.Source
		return nil
	})
	return out, err
}

// Get reads secret-free retained registry metadata without consulting source credentials.
func (s *Service) Get(ctx context.Context, e identity.Envelope, id string) (out Source, err error) {
	scope, err := sourceScope(e, "sources.read", "read", id)
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		record, e2 := s.repo.ReadSource(ctx, scope, id)
		if e2 != nil {
			return e2
		}
		out = record.Source
		return nil
	})
	return out, err
}

// List restricts eligible IDs before the storage limit, never after fetching broad metadata.
func (s *Service) List(ctx context.Context, e identity.Envelope, limit int) (out []Source, err error) {
	selection, err := access.Constrain(e, "sources.read", "source", "read")
	if err != nil {
		return nil, err
	}
	if limit < 1 || limit > 100 {
		return nil, store.ErrInvalid
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return nil, err
	}
	err = s.call(ctx, e, false, func(ctx context.Context) error {
		var e2 error
		out, e2 = s.repo.ListSources(ctx, scope, selection, limit)
		return e2
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Rotate proves current connector state again, refreshes credential-specific pools,
// and atomically installs a new context revision. Previously accepted plans stay old.
func (s *Service) Rotate(ctx context.Context, e identity.Envelope, id string, expected int64) (out Source, err error) {
	scope, err := sourceScope(e, "sources.rotate", "write", id)
	if err != nil {
		return out, err
	}
	if expected < 1 || expected >= 1<<62-1 {
		return out, store.ErrInvalid
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		old, e2 := s.repo.ReadSource(ctx, scope, id)
		if e2 != nil {
			return e2
		}
		if old.Source.Revision != expected {
			return store.ErrConflict
		}
		if e2 = actualContext(e, "sources.rotate", old); e2 != nil {
			return e2
		}
		connection, e2 := s.connection(e.Tenant(), old.Connection)
		if e2 != nil {
			return e2
		}
		binding, e2 := s.probe(ctx, connection, id, expected+1, nil)
		if e2 != nil {
			return e2
		}
		next := old
		next.Source.Revision++
		next.Source.ContextID = contextID(id, next.Source.Revision)
		next.Binding = binding
		if e2 = s.repo.PutSource(ctx, scope, expected, next); e2 != nil {
			return e2
		}
		out = next.Source
		return nil
	})
	return out, err
}

func (s *Service) observed(ctx context.Context, e identity.Envelope, id string, fn func(Record, readexec.Binding) error) error {
	scope, err := sourceScope(e, "sources.read", "read", id)
	if err != nil {
		return err
	}
	return s.call(ctx, e, true, func(ctx context.Context) error {
		return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
			if err := actualContext(e, "sources.read", record); err != nil {
				return err
			}
			connection, err := s.connection(e.Tenant(), record.Connection)
			if err != nil {
				return err
			}
			binding, err := s.probe(ctx, connection, id, record.Source.Revision, nil)
			if err != nil {
				return err
			}
			if readexec.Hash(binding) != readexec.Hash(record.Binding) {
				return readexec.ErrBinding
			}
			return fn(record, binding)
		})
	})
}

// Test returns successful health only after a real read-only context probe.
func (s *Service) Test(ctx context.Context, e identity.Envelope, id string) (out Status, err error) {
	err = s.observed(ctx, e, id, func(record Record, _ readexec.Binding) error {
		out = Status{Available: true, Revision: record.Source.Revision, ContextID: record.Source.ContextID, ObservedAt: time.Now().UTC()}
		return nil
	})
	return out, err
}

// Discover classifies actual catalog columns without reading result rows or credentials.
func (s *Service) Discover(ctx context.Context, e identity.Envelope, id string) (out Discovery, err error) {
	err = s.observed(ctx, e, id, func(record Record, b readexec.Binding) error {
		out = Discovery{SourceID: id, ContextID: record.Source.ContextID, Revision: record.Source.Revision, Relations: b.Clone().Relations}
		return nil
	})
	return out, err
}

// Binding is the pre-parse metadata seam. Missing source/context reach fails before
// any warehouse connection or dependency discovery occurs.
func (s *Service) Binding(ctx context.Context, e identity.Envelope, id, partition string) (out readexec.Binding, err error) {
	scope, err := sourceScope(e, "sources.query", "query", id)
	if err != nil {
		return out, err
	}
	if err = access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: partition}); err != nil {
		return out, err
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		record, e2 := s.repo.ReadSource(ctx, scope, id)
		if e2 != nil {
			return e2
		}
		if record.Source.ContextID != partition {
			return readexec.ErrBinding
		}
		out = record.Binding.Clone()
		return nil
	})
	return out, err
}

// Explain accepts only validator-issued candidates and never uses EXPLAIN ANALYZE.
func (s *Service) Explain(ctx context.Context, e identity.Envelope, candidate readexec.Candidate) error {
	id, partition := candidate.Coordinates()
	scope, err := sourceScope(e, "sources.query", "query", id)
	if err != nil {
		return err
	}
	return s.call(ctx, e, true, func(ctx context.Context) error {
		return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
			if record.Source.ContextID != partition {
				return readexec.ErrBinding
			}
			if _, _, err := candidate.SQL(e, record.Binding); err != nil {
				return err
			}
			connection, err := s.connection(e.Tenant(), record.Connection)
			if err != nil {
				return err
			}
			_, err = s.probe(ctx, connection, id, record.Source.Revision, func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
				statement, parameters, err := candidate.SQL(e, b)
				if err != nil {
					return err
				}
				return explain(ctx, tx, statement, parameters)
			})
			return err
		})
	})
}

// Read is the only query execution entry point. It accepts an opaque Plan, never
// raw SQL, and rechecks current binding under the metadata and warehouse fences.
func (s *Service) Read(ctx context.Context, e identity.Envelope, plan readexec.Plan) (out Rows, err error) {
	id, partition := plan.Coordinates()
	if id == "" || partition == "" {
		return out, readexec.ErrBinding
	}
	scope, err := sourceScope(e, "sources.query", "query", id)
	if err != nil {
		return out, err
	}
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
			if record.Source.ContextID != partition {
				return readexec.ErrBinding
			}
			if _, _, err := plan.SQL(e, record.Binding); err != nil {
				return err
			}
			connection, err := s.connection(e.Tenant(), record.Connection)
			if err != nil {
				return err
			}
			_, err = s.probe(ctx, connection, id, record.Source.Revision, func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
				statement, parameters, err := plan.SQL(e, b)
				if err != nil {
					return err
				}
				out, err = readRows(ctx, tx, statement, parameters, s.settings.MaxRows, s.settings.MaxBytes)
				return err
			})
			return err
		})
	})
	if err != nil {
		return Rows{}, err
	}
	return out, nil
}

func safe(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded, readexec.ErrUnsafe, readexec.ErrUnsupported, readexec.ErrBinding, readexec.ErrLimit, store.ErrInvalid, store.ErrNotFound, store.ErrConflict, store.ErrUnavailable, access.ErrUnauthenticated, access.ErrForbidden, access.ErrNotFound} {
		if errors.Is(err, known) {
			return known
		}
	}
	return store.ErrUnavailable
}
