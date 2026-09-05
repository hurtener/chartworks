// Package identity holds immutable, verified Pengui request authority.
// Construction is an internal verifier seam, never a request DTO or local issuer.
package identity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrInvalid is deliberately content-free.
var ErrInvalid = errors.New("identity: invalid verified envelope")

// Reach is one exact signed resource permission. Only the complete ID may be '*'.
type Reach struct{ Kind, Permission, ID string }

// Identifier is the canonical opaque coordinate grammar, including service IDs.
func Identifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || strings.ContainsRune("_.:-", c) {
			continue
		}
		return false
	}
	return true
}

// ParseReach implements cw.<kind>.<permission>:<id>, without URL decoding or globbing.
func ParseReach(s string) (Reach, error) {
	var r Reach
	prefix, id, ok := strings.Cut(s, ":")
	p := strings.Split(prefix, ".")
	if !ok || len(p) != 3 || p[0] != "cw" || (id != "*" && !Identifier(id)) {
		return r, ErrInvalid
	}
	if !member(p[1], "schedule", "source", "dataset", "topic", "block", "report", "dashboard", "run", "execution_context", "execution_binding", "tenant") || !member(p[2], "read", "query", "write", "execute", "preview", "publish", "certify", "export", "use", "erase") {
		return r, ErrInvalid
	}
	return Reach{p[1], p[2], id}, nil
}
func member(s string, values ...string) bool {
	for _, v := range values {
		if s == v {
			return true
		}
	}
	return false
}

// Envelope contains no token bytes and never infers permissions from an actor name.
type Envelope struct {
	tenant, user, session string
	scopes                []string
	reach                 []Reach
	until                 time.Time
	now                   func() time.Time
}

// FromVerified is called only after signature, issuer, audience and time validation.
// In-process callers must obtain envelopes from auth.Verifier, not build identity DTOs.
func FromVerified(tenant, user, session string, scopes []string, until time.Time, now func() time.Time) (Envelope, error) {
	if !Identifier(tenant) || !Identifier(user) || !Identifier(session) || until.IsZero() || len(scopes) > 32 {
		return Envelope{}, ErrInvalid
	}
	if strings.HasPrefix(strings.ToLower(user), "svc:") && (!strings.HasPrefix(user, "svc:") || len(user) == 4) {
		return Envelope{}, ErrInvalid
	}
	if now == nil {
		now = time.Now
	}
	e := Envelope{tenant: tenant, user: user, session: session, until: until, now: now}
	seen := map[string]bool{}
	total := 0
	for _, s := range scopes {
		total += len(s)
		if len(s) == 0 || len(s) > 256 || total > 4096 || seen[s] {
			return Envelope{}, ErrInvalid
		}
		for _, c := range s {
			if c < 33 || c > 126 {
				return Envelope{}, ErrInvalid
			}
		}
		seen[s] = true
		if strings.HasPrefix(s, "cw.") {
			r, err := ParseReach(s)
			if err != nil {
				return Envelope{}, err
			}
			e.reach = append(e.reach, r)
		}
		e.scopes = append(e.scopes, s)
	}
	if !e.Valid() {
		return Envelope{}, ErrInvalid
	}
	return e, nil
}

// Valid also rejects expiry when a previously verified envelope is reused in-process.
func (e Envelope) Valid() bool { return e.now != nil && e.tenant != "" && e.now().Before(e.until) }

// Tenant is the signed storage partition.
func (e Envelope) Tenant() string { return e.tenant }

// User is the signed user or service attribution.
func (e Envelope) User() string { return e.user }

// Session is the signed request-session coordinate.
func (e Envelope) Session() string { return e.session }

// Service reports attribution only; it grants no privilege.
func (e Envelope) Service() bool { return strings.HasPrefix(e.user, "svc:") }

// Deadline bounds use by exp plus the explicitly configured verification skew.
func (e Envelope) Deadline() time.Time { return e.until }

// Scopes returns a detached copy, never shared mutable authority.
func (e Envelope) Scopes() []string { return append([]string(nil), e.scopes...) }

// Reach returns a detached copy of addressed permissions.
func (e Envelope) Reach() []Reach { return append([]Reach(nil), e.reach...) }

// Has is an exact operation-scope comparison, without an admin shortcut.
func (e Envelope) Has(s string) bool {
	if !e.Valid() {
		return false
	}
	for _, v := range e.scopes {
		if v == s {
			return true
		}
	}
	return false
}

// String and GoString deliberately avoid logging identity or scope sets implicitly.
func (e Envelope) String() string { return "verified-authority(redacted)" }

// GoString redacts detailed formatting as well.
func (e Envelope) GoString() string { return e.String() }

// MarshalJSON prevents accidental serialization of authority internals.
func (e Envelope) MarshalJSON() ([]byte, error) { return []byte(`{"authority":"redacted"}`), nil }

type contextKey struct{}

// Context installs only a nonzero current envelope; remote callers cannot set this key.
func (e Envelope) Context(ctx context.Context) (context.Context, error) {
	if !e.Valid() {
		return nil, ErrInvalid
	}
	return context.WithValue(ctx, contextKey{}, e), nil
}

// FromContext returns a current envelope, never a header/body-derived fallback.
func FromContext(ctx context.Context) (Envelope, error) {
	e, ok := ctx.Value(contextKey{}).(Envelope)
	if !ok || !e.Valid() {
		return Envelope{}, ErrInvalid
	}
	return e, nil
}
