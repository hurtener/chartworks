// Package store defines narrow, tenant-scoped metadata persistence contracts.
// A Scope is an isolation coordinate, NOT proof of authentication. Only a later
// verified authority boundary may expose these internal services to remote callers.
package store

import (
	"context"
	"errors"
	"time"
)

var (
	// ErrScope rejects absent or malformed storage isolation coordinates.
	ErrScope = errors.New("store: invalid tenant or actor scope")
	// ErrInvalid rejects a malformed business value without echoing it.
	ErrInvalid = errors.New("store: invalid value or reference")
	// ErrNotFound does not disclose a foreign tenant's record.
	ErrNotFound = errors.New("store: record not found")
	// ErrConflict covers CAS, idempotency, and stale lease conflicts.
	ErrConflict = errors.New("store: conflict")
	// ErrUnavailable is a safe connection/transaction failure.
	ErrUnavailable = errors.New("store: unavailable")
	// ErrExpired retains an operation-key tombstone without re-executing work.
	ErrExpired = errors.New("store: operation expired")
	// ErrMigration indicates missing, unknown, or changed migration history.
	ErrMigration = errors.New("store: migration history invalid")
)

// Scope has no zero-value authority and never derives privileges from its IDs.
type Scope struct{ tenant, actor string }

// NewScope checks the internal storage coordinates. It does not authenticate a user.
func NewScope(tenant, actor string) (Scope, error) {
	if !Identifier(tenant) || !Identifier(actor) {
		return Scope{}, ErrScope
	}
	return Scope{tenant: tenant, actor: actor}, nil
}

// Valid rejects the zero value at every repository entry point.
func (s Scope) Valid() bool { return Identifier(s.tenant) && Identifier(s.actor) }

// Tenant is the exact storage partition, never a request fallback.
func (s Scope) Tenant() string { return s.tenant }

// Actor is attribution and the owner of an idempotency key, not a local role.
func (s Scope) Actor() string { return s.actor }

// Identifier validates bounded opaque metadata identifiers.
func Identifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' || c == '-' || c == ':' || c == '.' {
			continue
		}
		return false
	}
	return true
}

// Policy is operational retention configuration, not identity or access policy.
type Policy struct {
	Revision       int64
	AuditDays      int
	OperationHours int
}

// Validate rejects retention settings that could silently remove all history.
func (p Policy) Validate() error {
	if p.AuditDays < 1 || p.AuditDays > 3650 || p.OperationHours < 1 || p.OperationHours > 8760 {
		return ErrInvalid
	}
	return nil
}

// Policies owns immutable operational-policy revisions and their CAS pointer.
type Policies interface {
	Policy(context.Context, Scope) (Policy, error)
	SetPolicy(context.Context, Scope, int64, Policy) (Policy, error)
}

// Audit contains only bounded identifiers and a closed action vocabulary.
type Audit struct {
	ID, Actor, Action, Resource string
	CreatedAt                   time.Time
}

// Audits provides tenant-scoped, bounded discovery without arbitrary SQL.
type Audits interface {
	Audits(context.Context, Scope, int) ([]Audit, error)
}

// Operation records a retention operation's accepted manifest and durable result.
type Operation struct {
	ID, Status                       string
	PolicyRevision                   int64
	Cutoff                           time.Time
	Limit                            int
	DeletedEvents, DeletedOperations int64
}

// Lease is a fencing token, never identity authority.
type Lease struct {
	OperationID, Owner string
	Fence              int64
}

// Retention is the first real consumer of operation-key and lease primitives.
type Retention interface {
	ReserveSweep(context.Context, Scope, string, string, int) (Operation, error)
	Claim(context.Context, Scope, string, string, time.Duration) (Lease, error)
	Renew(context.Context, Scope, Lease, time.Duration) error
	CommitSweep(context.Context, Scope, Lease) (Operation, error)
}

// Foundation composes only the metadata needed by the first maintenance consumer.
type Foundation interface {
	Policies
	Audits
	Retention
}
