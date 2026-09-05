// Package access enforces signed Pengui authority; it never resolves local roles or grants.
package access

import (
	"errors"
	"sort"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

var (
	// ErrUnauthenticated rejects missing or expired verified context.
	ErrUnauthenticated = errors.New("access: verified authority required")
	// ErrForbidden denies an operation without disclosing resource existence.
	ErrForbidden = errors.New("access: operation forbidden")
	// ErrNotFound is nondisclosing for foreign or inaccessible coordinates.
	ErrNotFound = errors.New("access: resource not found")
)

// Resource must be resolved by the domain from actual metadata, never a supplied audience label.
type Resource struct{ Tenant, Kind, Permission, ID string }

func valid(r Resource) bool {
	if !identity.Identifier(r.Tenant) || !identity.Identifier(r.ID) {
		return false
	}
	_, err := identity.ParseReach("cw." + r.Kind + "." + r.Permission + ":" + r.ID)
	return err == nil && (r.Kind != "tenant" || r.ID == r.Tenant)
}
func reaches(e identity.Envelope, r Resource) bool {
	if !valid(r) || r.Tenant != e.Tenant() {
		return false
	}
	for _, s := range e.Reach() {
		if s.Kind == r.Kind && s.Permission == r.Permission && (s.ID == r.ID || s.ID == "*") {
			return true
		}
	}
	return false
}

// Require needs both an exact action and every target/dependency permission.
func Require(e identity.Envelope, action string, resources ...Resource) error {
	if !e.Valid() {
		return ErrUnauthenticated
	}
	if !e.Has(action) {
		return ErrForbidden
	}
	if len(resources) == 0 {
		return ErrNotFound
	}
	for _, r := range resources {
		if !reaches(e, r) {
			return ErrNotFound
		}
	}
	return nil
}

// Tenant binds an operational requirement to the signed tenant, not a caller tenant.
func Tenant(e identity.Envelope, permission string) Resource {
	return Resource{e.Tenant(), "tenant", permission, e.Tenant()}
}

// StoreScope enforces authority before producing the lower-level storage coordinates.
func StoreScope(e identity.Envelope, action, permission string) (store.Scope, error) {
	if err := Require(e, action, Tenant(e, permission)); err != nil {
		return store.Scope{}, err
	}
	return store.NewScope(e.Tenant(), e.User())
}

// Selection is immutable pre-query restriction data. The zero value selects nothing.
// Database adapters MUST apply Tenant and IDs/All in SQL, never fetch-wide then filter.
type Selection struct {
	authority identity.Envelope
	tenant    string
	ids       []string
	all       bool
}

// Tenant is mandatory even when All is true.
func (s Selection) Tenant() string {
	if !s.authority.Valid() {
		return ""
	}
	return s.tenant
}

// IDs is a detached, canonical list of allowed identifiers.
func (s Selection) IDs() []string {
	if !s.authority.Valid() {
		return nil
	}
	return append([]string(nil), s.ids...)
}

// All means all eligible IDs within this tenant, never all tenants.
func (s Selection) All() bool { return s.authority.Valid() && s.all && s.tenant != "" }

// Contains is useful for pre-I/O reference checks, not post-fetch security filtering.
func (s Selection) Contains(tenant, id string) bool {
	if !s.authority.Valid() || s.tenant == "" || tenant != s.tenant || !identity.Identifier(id) {
		return false
	}
	if s.all {
		return true
	}
	i := sort.SearchStrings(s.ids, id)
	return i < len(s.ids) && s.ids[i] == id
}

// Constrain derives the allowed query set before a store/source is called.
func Constrain(e identity.Envelope, action, kind, permission string) (Selection, error) {
	if !e.Valid() {
		return Selection{}, ErrUnauthenticated
	}
	if !e.Has(action) {
		return Selection{}, ErrForbidden
	}
	if _, err := identity.ParseReach("cw." + kind + "." + permission + ":probe"); err != nil {
		return Selection{}, ErrNotFound
	}
	s := Selection{tenant: e.Tenant(), authority: e}
	for _, r := range e.Reach() {
		if r.Kind != kind || r.Permission != permission {
			continue
		}
		if r.ID == "*" {
			s.all = true
		} else if kind != "tenant" || r.ID == e.Tenant() {
			s.ids = append(s.ids, r.ID)
		}
	}
	sort.Strings(s.ids)
	if !s.all && len(s.ids) == 0 {
		return Selection{}, ErrNotFound
	}
	return s, nil
}

// Execution describes actual resolved domain references, not user-supplied metadata.
type Execution struct {
	Target                 Resource
	Dependencies, Contexts []Resource
}

// RequireExecution checks every executable dependency and every actual context version.
// The later executor supplies a complete validated manifest; this function cannot infer omissions.
func RequireExecution(e identity.Envelope, x Execution) error {
	if x.Target.Permission != "execute" || len(x.Dependencies) == 0 || len(x.Contexts) == 0 {
		return ErrNotFound
	}
	refs := []Resource{x.Target}
	for _, r := range x.Dependencies {
		if r.Permission != "query" && r.Permission != "execute" {
			return ErrNotFound
		}
		refs = append(refs, r)
	}
	for _, r := range x.Contexts {
		if r.Kind != "execution_context" || r.Permission != "use" {
			return ErrNotFound
		}
		refs = append(refs, r)
	}
	return Require(e, "reporting.execute", refs...)
}

// Artifact describes retained metadata without loading rows. Context IDs are version-addressed.
type Artifact struct {
	Tenant, RunID, ParentKind, ParentID string
	Published, Private                  bool
	Contexts                            []Resource
}

// RequireArtifact allows exact-run read or eligible-publication parent read, plus actual partitions.
// Private artifacts always require preview scope and target preview reach, even with exact run read.
func RequireArtifact(e identity.Envelope, a Artifact) error {
	if !e.Valid() {
		return ErrUnauthenticated
	}
	if !e.Has("reporting.read") {
		return ErrForbidden
	}
	if a.Tenant != e.Tenant() || !identity.Identifier(a.RunID) || !identity.Identifier(a.ParentID) || (a.ParentKind != "report" && a.ParentKind != "block") || len(a.Contexts) == 0 {
		return ErrNotFound
	}
	run := Resource{a.Tenant, "run", "read", a.RunID}
	parent := Resource{a.Tenant, a.ParentKind, "read", a.ParentID}
	if !reaches(e, run) && (!a.Published || a.Private || !reaches(e, parent)) {
		return ErrNotFound
	}
	for _, r := range a.Contexts {
		if r.Kind != "execution_context" || r.Permission != "use" || !reaches(e, r) {
			return ErrNotFound
		}
	}
	if a.Private {
		return Require(e, "reporting.preview", Resource{a.Tenant, a.ParentKind, "preview", a.ParentID})
	}
	return nil
}
