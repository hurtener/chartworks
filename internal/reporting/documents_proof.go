package reporting

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

// RequireDocument applies signed report/dashboard scope before metadata I/O.
// Creator, reviewer, audience and import-origin labels are never consulted.
func RequireDocument(e identity.Envelope, kind, id string, a Access) error {
	if !documentKind(kind) || !identity.Identifier(id) || !slices.Contains([]Access{Read, Write, Preview, Publish, Execute}, a) {
		return ErrInvalid
	}
	return access.Require(e, a.Action(), access.Resource{Tenant: e.Tenant(), Kind: kind, Permission: a.Permission(), ID: id})
}

func documentMutationAccess(operation string) Access {
	switch operation {
	case "create", "edit", "import", "review", "archive":
		return Write
	case "publish", "reject":
		return Publish
	default:
		return ""
	}
}

// PreparedDocument cannot be reconstructed from a caller-supplied JSON object.
// It binds detached validated content, configured bounds and exact authority.
type PreparedDocument struct {
	encoded   []byte
	authority string
	deadline  time.Time
}

func prepareDocument(e identity.Envelope, m DocumentMutation, limits config.Reporting) (PreparedDocument, error) {
	if limits.Validate() != nil {
		return PreparedDocument{}, ErrInvalid
	}
	m.MaxRevisions, m.MaxDocuments = limits.MaxRevisions, limits.MaxBlocks
	if m.Revision != nil {
		d, err := ProjectDocument(m.Revision.Raw, m.Kind, limits.Composition)
		if err != nil || ValidateDocument(m.Kind, d, limits.Composition, false) != nil {
			return PreparedDocument{}, ErrInvalid
		}
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > 4<<20 {
		return PreparedDocument{}, ErrInvalid
	}
	p := PreparedDocument{encoded: raw, authority: authority(e), deadline: e.Deadline()}
	if _, err := p.Checked(e); err != nil {
		return PreparedDocument{}, err
	}
	return p, nil
}

// Checked is repeated by the repository inside the commit transaction. It is
// not a serializer for identity or a locally issued execution credential.
func (p PreparedDocument) Checked(e identity.Envelope) (DocumentMutation, error) {
	var m DocumentMutation
	if !e.Valid() || len(p.encoded) == 0 || len(p.encoded) > 4<<20 || p.authority != authority(e) || !time.Now().Before(p.deadline) {
		return m, access.ErrUnauthenticated
	}
	if json.Unmarshal(p.encoded, &m) != nil || m.ExpectedVersion < 0 || m.TargetRevision < 0 || m.MaxRevisions < 2 || m.MaxRevisions > 256 || m.MaxDocuments < 1 || m.MaxDocuments > 100000 {
		return DocumentMutation{}, ErrInvalid
	}
	if err := RequireDocument(e, m.Kind, m.ID, documentMutationAccess(m.Operation)); err != nil {
		return DocumentMutation{}, err
	}
	if m.Revision != nil {
		r := m.Revision
		if r.Number < 1 || r.Number > int64(m.MaxRevisions) || r.Actor != e.User() || r.Session != e.Session() || r.Created.IsZero() || r.Digest == "" || DocumentDigest(r.Raw) != r.Digest || len(r.Origins) > 100 || !validExternal(r.External) {
			return DocumentMutation{}, ErrInvalid
		}
	}
	return m, nil
}

func validExternal(ref *ExternalReference) bool {
	return ref == nil || identity.Identifier(ref.System) && identity.Identifier(ref.ID) && identity.Identifier(ref.Version)
}

// QuarantineRecord retains unsupported bounded input privately. It never
// becomes a document revision, publication or executable report by default.
type QuarantineRecord struct {
	ID       string
	Kind     string
	Target   string
	Raw      json.RawMessage
	Digest   string
	Reason   string
	External *ExternalReference
}

// PreparedQuarantine limits the separate, private import-failure write seam.
type PreparedQuarantine struct {
	encoded   []byte
	authority string
	deadline  time.Time
}

func prepareQuarantine(e identity.Envelope, q QuarantineRecord, limits config.ReportingComposition) (PreparedQuarantine, error) {
	if limits.Validate() != nil || len(q.Raw) == 0 || len(q.Raw) > limits.MaxDefinitionBytes {
		return PreparedQuarantine{}, ErrInvalid
	}
	raw, err := json.Marshal(q)
	if err != nil {
		return PreparedQuarantine{}, ErrInvalid
	}
	p := PreparedQuarantine{encoded: raw, authority: authority(e), deadline: e.Deadline()}
	_, err = p.Checked(e)
	return p, err
}

// Checked binds quarantine writes to current signed target-write reach.
func (p PreparedQuarantine) Checked(e identity.Envelope) (QuarantineRecord, error) {
	var q QuarantineRecord
	if !e.Valid() || p.authority != authority(e) || !time.Now().Before(p.deadline) {
		return q, access.ErrUnauthenticated
	}
	if len(p.encoded) == 0 || len(p.encoded) > 4<<20 || json.Unmarshal(p.encoded, &q) != nil || !identity.Identifier(q.ID) || !json.Valid(q.Raw) || q.Digest != DocumentDigest(q.Raw) || !validExternal(q.External) || q.Reason != "unsupported_definition" {
		return QuarantineRecord{}, ErrInvalid
	}
	if err := RequireDocument(e, q.Kind, q.Target, Write); err != nil {
		return QuarantineRecord{}, err
	}
	return q, nil
}
