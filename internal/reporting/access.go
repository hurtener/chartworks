package reporting

import (
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

// Access names domain actions, not local roles or an alternative grant store.
type Access string

const (
	Read     Access = "read"
	Write    Access = "write"
	Validate Access = "validate"
	Preview  Access = "preview"
	Publish  Access = "publish"
	Certify  Access = "certify"
	SQLRead  Access = "sql.read"
)

func (a Access) Action() string {
	switch a {
	case Read, Write, Validate, Preview, Publish, Certify, SQLRead:
		return "reporting." + string(a)
	default:
		return ""
	}
}

func (a Access) Permission() string {
	switch a {
	case Read, SQLRead:
		return "read"
	case Write, Validate:
		return "write"
	case Preview:
		return "preview"
	case Publish:
		return "publish"
	case Certify:
		return "certify"
	default:
		return ""
	}
}

// Require is shared by service and store. No author name or prior approval
// expands the current supplied Pengui authority.
func Require(e identity.Envelope, id string, a Access) error {
	if !identity.Identifier(id) || a.Action() == "" {
		return ErrInvalid
	}
	return access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: "block", Permission: a.Permission(), ID: id})
}

func RequireParent(e identity.Envelope, topic string, a Access, creating bool) error {
	permission := "read"
	if creating || a == Write {
		permission = "write"
	}
	return access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: permission, ID: topic})
}

// RequirePrivate preserves private draft ownership across later publication of
// other revisions. Writers may change their own drafts; viewing private values
// and SQL additionally requires explicit preview reach.
func RequirePrivate(e identity.Envelope, id, actor string, a Access) error {
	if e.User() != actor {
		return access.ErrNotFound
	}
	if a == Write || a == Validate || a == Publish {
		return nil
	}
	return Require(e, id, Preview)
}

// ResourceReference is a server-derived, tenant-composite eligibility row.
type ResourceReference struct {
	Kind       string `json:"kind"`
	Permission string `json:"permission"`
	ID         string `json:"id"`
}

func RequireReferences(e identity.Envelope, a Access, refs []ResourceReference) error {
	if len(refs) == 0 || len(refs) > 4096 {
		return ErrInvalid
	}
	for _, r := range refs {
		valid := r.Kind == "topic" && r.Permission == "read" || r.Kind == "source" && r.Permission == "read" || r.Kind == "dataset" && r.Permission == "query" || r.Kind == "execution_context" && r.Permission == "use"
		if !valid || !identity.Identifier(r.ID) {
			return ErrInvalid
		}
		if err := access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: r.Kind, Permission: r.Permission, ID: r.ID}); err != nil {
			return err
		}
	}
	return nil
}

// Mutation is exposed only through Prepared.Checked. Repositories recheck head,
// revision, evidence and dependency fences in the same transaction as the write.
type Mutation struct {
	ID              string              `json:"id"`
	Topic           string              `json:"topic"`
	Kind            string              `json:"kind"`
	ExpectedVersion int64               `json:"expected_version"`
	TargetRevision  int64               `json:"target_revision"`
	TargetDigest    string              `json:"target_digest"`
	Note            string              `json:"note"`
	Revision        *Revision           `json:"revision,omitempty"`
	References      []ResourceReference `json:"references"`
	Validation      *ValidationRecord   `json:"validation,omitempty"`
	Evidence        string              `json:"evidence,omitempty"`
	Attestation     *Attestation        `json:"attestation,omitempty"`
	Withdrawal      *Withdrawal         `json:"withdrawal,omitempty"`
	Health          *Health             `json:"health,omitempty"`
	Watch           []Dependency        `json:"watch"`
	Topics          []TopicPin          `json:"topics"`
	CheckCurrent    bool                `json:"check_current"`
	MaxRevisions    int                 `json:"max_revisions"`
	MaxBlocks       int                 `json:"max_blocks"`
}

func (m Mutation) Access() Access {
	switch m.Kind {
	case "create", "edit", "capture", "restore", "rename", "parameterize", "reject", "archive":
		return Write
	case "validate":
		return Validate
	case "publish":
		return Publish
	case "certify", "withdraw":
		return Certify
	case "health":
		return Read
	case "preview":
		return Preview
	default:
		return ""
	}
}

// Prepared cannot be constructed by transport input or a store caller. The
// encoded snapshot prevents mutations through slice/map aliases after admission.
type Prepared struct {
	encoded   []byte
	authority string
	deadline  time.Time
}

func authority(e identity.Envelope) string {
	return digest(struct {
		Tenant  string
		User    string
		Session string
		Scopes  []string
	}{e.Tenant(), e.User(), e.Session(), e.Scopes()})
}

func prepare(e identity.Envelope, m Mutation, limits config.Reporting) (Prepared, error) {
	m.MaxRevisions, m.MaxBlocks = limits.MaxRevisions, limits.MaxBlocks
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > 4<<20 {
		return Prepared{}, ErrInvalid
	}
	p := Prepared{encoded: raw, authority: authority(e), deadline: e.Deadline()}
	if _, err := p.Checked(e); err != nil {
		return Prepared{}, err
	}
	return p, nil
}

// Checked returns detached admitted state, rechecking the current envelope.
func (p Prepared) Checked(e identity.Envelope) (Mutation, error) {
	var m Mutation
	if len(p.encoded) == 0 || len(p.encoded) > 4<<20 || !e.Valid() || authority(e) != p.authority || !time.Now().Before(p.deadline) {
		return m, access.ErrUnauthenticated
	}
	if json.Unmarshal(p.encoded, &m) != nil || !identity.Identifier(m.Topic) || m.ExpectedVersion < 0 || m.MaxRevisions < 2 || m.MaxRevisions > 256 || m.MaxBlocks < 1 || m.MaxBlocks > 100000 {
		return Mutation{}, ErrInvalid
	}
	if err := Require(e, m.ID, m.Access()); err != nil {
		return Mutation{}, err
	}
	if err := RequireParent(e, m.Topic, m.Access(), m.ExpectedVersion == 0); err != nil {
		return Mutation{}, err
	}
	if err := RequireReferences(e, m.Access(), m.References); err != nil {
		return Mutation{}, err
	}
	return m, nil
}
