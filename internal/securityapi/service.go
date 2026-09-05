// Package securityapi exposes the first verified, authorized operational consumers.
// Reporting and MCP transports remain in their own phases; no placeholder endpoints exist.
package securityapi

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/maintenance"
	"github.com/hurtener/chartworks/internal/store"
)

// Service owns authorization for both HTTP and in-process consumers.
type Service struct {
	repo        store.Foundation
	maintenance *maintenance.Service
}

// New requires the real repository; there is no ambient authority or in-memory default.
func New(repo store.Foundation) (*Service, error) {
	m, err := maintenance.New(repo, 100, 30*time.Second)
	if err != nil {
		return nil, err
	}
	return &Service{repo, m}, nil
}

// Policy reads the signed tenant's operational retention configuration.
func (s *Service) Policy(ctx context.Context, e identity.Envelope) (store.Policy, error) {
	scope, err := access.StoreScope(e, "ops.read", "read")
	if err != nil {
		return store.Policy{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	return s.repo.Policy(ctx, scope)
}

// Configure atomically changes a business retention policy after signed write enforcement.
func (s *Service) Configure(ctx context.Context, e identity.Envelope, expected int64, p store.Policy) (store.Policy, error) {
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return store.Policy{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	return s.maintenance.Configure(ctx, scope, expected, p)
}

// Audits applies tenant scope before any PostgreSQL access.
func (s *Service) Audits(ctx context.Context, e identity.Envelope, limit int) ([]store.Audit, error) {
	scope, err := access.StoreScope(e, "ops.audit", "read")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	return s.repo.Audits(ctx, scope, limit)
}

// Sweep is bounded synchronous maintenance, not an unattended scheduler or token renewal path.
func (s *Service) Sweep(ctx context.Context, e identity.Envelope, key string) (store.Operation, error) {
	scope, err := access.StoreScope(e, "ops.maintain", "erase")
	if err != nil {
		return store.Operation{}, err
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	return s.maintenance.Sweep(ctx, scope, key)
}
