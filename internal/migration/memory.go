package migration

import (
	"context"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

type memoryEntry struct {
	manifest Manifest
	plan     Plan
	batch    Batch
	results  map[string]string
}

type MemoryRepository struct {
	mu       sync.Mutex
	batches  map[string]memoryEntry
	cutovers map[string]Cutover
	now      func() time.Time
}

func NewMemoryRepository(now func() time.Time) *MemoryRepository {
	if now == nil {
		now = time.Now
	}
	return &MemoryRepository{batches: map[string]memoryEntry{}, cutovers: map[string]Cutover{}, now: now}
}

func tenantKey(e identity.Envelope, id string) string { return e.Tenant() + "\x00" + id }

func (m *MemoryRepository) Begin(_ context.Context, e identity.Envelope, manifest Manifest, digest string, plan Plan) (Batch, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(e, manifest.Batch)
	if old, ok := m.batches[key]; ok {
		return old.batch, true, nil
	}
	now := m.now().UTC()
	b := Batch{ID: manifest.Batch, Cohort: manifest.Cohort, Digest: digest, State: "importing", Revision: 1, Total: len(plan.Objects), CreatedAt: now, UpdatedAt: now}
	if len(plan.Objects) > 0 {
		b.Next = plan.Objects[0].ExternalRef
	}
	m.batches[key] = memoryEntry{manifest: manifest, plan: plan, batch: b, results: map[string]string{}}
	return b, false, nil
}

func (m *MemoryRepository) Batch(_ context.Context, e identity.Envelope, id string) (Batch, Manifest, Plan, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	x, ok := m.batches[tenantKey(e, id)]
	if !ok {
		return Batch{}, Manifest{}, Plan{}, ErrNotFound
	}
	return x.batch, x.manifest, x.plan, nil
}

func (m *MemoryRepository) Checkpoint(_ context.Context, e identity.Envelope, id string, expected int64, plan ObjectPlan, result string) (Batch, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(e, id)
	x, ok := m.batches[key]
	if !ok {
		return Batch{}, ErrNotFound
	}
	if x.batch.Revision != expected {
		return Batch{}, ErrConflict
	}
	index := x.batch.Applied + x.batch.Quarantined
	if index >= len(x.plan.Objects) || x.plan.Objects[index].ExternalRef != plan.ExternalRef {
		return Batch{}, ErrConflict
	}
	if _, exists := x.results[plan.ExternalRef]; exists {
		return Batch{}, ErrConflict
	}
	x.results[plan.ExternalRef] = result
	if plan.Action == "install_private" || plan.Action == "tombstone" {
		x.batch.Applied++
	} else {
		x.batch.Quarantined++
	}
	x.batch.Revision++
	x.batch.UpdatedAt = m.now().UTC()
	if index+1 == len(x.plan.Objects) {
		x.batch.State, x.batch.Next = "complete", ""
	} else {
		x.batch.Next = x.plan.Objects[index+1].ExternalRef
	}
	m.batches[key] = x
	return x.batch, nil
}

func (m *MemoryRepository) Export(_ context.Context, e identity.Envelope, id, after string, limit int) (Export, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	x, ok := m.batches[tenantKey(e, id)]
	if !ok {
		return Export{}, ErrNotFound
	}
	start := 0
	if after != "" {
		for i, o := range x.manifest.Objects {
			if o.ExternalRef == after {
				start = i + 1
				break
			}
		}
	}
	out := x.manifest
	if start > len(out.Objects) {
		start = len(out.Objects)
	}
	end := start + limit
	if end > len(out.Objects) {
		end = len(out.Objects)
	}
	out.Objects = append([]Object(nil), out.Objects[start:end]...)
	return Export{Manifest: out, Batch: x.batch}, nil
}

func (m *MemoryRepository) Cutover(_ context.Context, e identity.Envelope, b Batch, expected int64, route, operator string, boundary OccurrenceBoundary) (Cutover, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(e, b.Cohort)
	old := m.cutovers[key]
	if old.Generation != expected {
		return Cutover{}, ErrConflict
	}
	if old.Batch == b.ID && old.Route == route && old.State == "active" {
		return old, nil
	}
	next := Cutover{Cohort: b.Cohort, Batch: b.ID, Route: route, PreviousRoute: old.Route, State: "active", Generation: expected + 1, Boundary: boundary, OperatorReference: operator, UpdatedAt: m.now().UTC()}
	m.cutovers[key] = next
	return next, nil
}

func (m *MemoryRepository) CurrentCutover(_ context.Context, e identity.Envelope, cohort string) (Cutover, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	x, ok := m.cutovers[tenantKey(e, cohort)]
	if !ok {
		return Cutover{}, ErrNotFound
	}
	return x, nil
}

func (m *MemoryRepository) Rollback(_ context.Context, e identity.Envelope, cohort string, expected int64, operator string, effects []string) (Cutover, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(e, cohort)
	old, ok := m.cutovers[key]
	if !ok {
		return Cutover{}, ErrNotFound
	}
	if old.Generation != expected {
		return Cutover{}, ErrConflict
	}
	next := old
	next.Route = old.PreviousRoute
	next.PreviousRoute = old.Route
	next.State = "rolled_back"
	next.Generation++
	next.OperatorReference = operator
	next.IrreversibleEffects = append([]string(nil), effects...)
	next.UpdatedAt = m.now().UTC()
	m.cutovers[key] = next
	return next, nil
}

func (m *MemoryRepository) Erase(_ context.Context, e identity.Envelope, id string, limit int) (EraseResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := tenantKey(e, id)
	x, ok := m.batches[key]
	if !ok {
		return EraseResult{}, ErrNotFound
	}
	erased := int64(0)
	for ref := range x.results {
		if erased == int64(limit) {
			break
		}
		delete(x.results, ref)
		erased++
	}
	remaining := int64(len(x.results))
	if remaining == 0 {
		delete(m.batches, key)
	} else {
		m.batches[key] = x
	}
	return EraseResult{Batch: id, Erased: erased, Remaining: remaining, BackupScope: "online records only; immutable backups expire by operator retention"}, nil
}
