// Package maintenance is the first real, in-process consumer of scoped store primitives.
// No HTTP/CLI tenant override exposes it before Pengui verification and enforcement land.
package maintenance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/hurtener/chartworks/internal/store"
)

// Service executes bounded retention work. The caller supplies already-authorized storage scope.
type Service struct {
	store store.Foundation
	batch int
	lease time.Duration
}

// New requires a real repository and bounded settings; there is no memory-only default.
func New(repository store.Foundation, batch int, lease time.Duration) (*Service, error) {
	if repository == nil || batch < 1 || batch > 1000 || lease < time.Second || lease > 5*time.Minute {
		return nil, store.ErrInvalid
	}
	return &Service{store: repository, batch: batch, lease: lease}, nil
}

// Configure is a business-setting change with CAS, not an access-policy decision.
func (s *Service) Configure(ctx context.Context, scope store.Scope, expected int64, p store.Policy) (store.Policy, error) {
	return s.store.SetPolicy(ctx, scope, expected, p)
}

// Sweep reserves a stable operation, replays completed results, and fences its transactional effect.
func (s *Service) Sweep(ctx context.Context, scope store.Scope, key string) (store.Operation, error) {
	h := sha256.Sum256([]byte("retention.sweep/v1/batch=" + strconv.Itoa(s.batch)))
	op, err := s.store.ReserveSweep(ctx, scope, key, hex.EncodeToString(h[:]), s.batch)
	if err != nil {
		return store.Operation{}, err
	}
	if op.Status == "succeeded" {
		return op, nil
	}
	var owner [16]byte
	if _, err = rand.Read(owner[:]); err != nil {
		return store.Operation{}, errors.New("maintenance: randomness unavailable")
	}
	lease, err := s.store.Claim(ctx, scope, op.ID, hex.EncodeToString(owner[:]), s.lease)
	if err != nil {
		return store.Operation{}, err
	}
	return s.store.CommitSweep(ctx, scope, lease)
}
