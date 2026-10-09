package reporting

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// Option targets are exact retained coordinates, never physical source names.
type AuthoringDatasetOptionTarget struct {
	NewBlock      string            `json:"new_block"`
	Topic         TopicPin          `json:"topic,omitempty"`
	Dataset       string            `json:"dataset"`
	Dimension     string            `json:"dimension,omitempty"`
	Column        string            `json:"column,omitempty"`
	SourceDataset *SourceDatasetPin `json:"source_dataset,omitempty"`
}
type AuthoringReportOptionTarget struct {
	Policy   string `json:"policy" jsonschema:"enum=private_preview,enum=published"`
	Report   string `json:"report"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
	Page     string `json:"page"`
	Filter   string `json:"filter"`
}
type AuthoringOptionTarget struct {
	Dataset *AuthoringDatasetOptionTarget `json:"dataset,omitempty"`
	Report  *AuthoringReportOptionTarget  `json:"report,omitempty"`
}
type AuthoringOptionRequest struct {
	Target    AuthoringOptionTarget `json:"target"`
	Operation string                `json:"operation"`
	Search    string                `json:"search"`
	Cursor    string                `json:"cursor"`
	Limit     int                   `json:"limit" jsonschema:"minimum=1,maximum=199"`
	Locale    string                `json:"locale"`
}
type AuthoringOptionReference struct {
	Target    AuthoringOptionTarget `json:"target"`
	Operation string                `json:"operation"`
}
type AuthoringOptionControlRequest struct {
	Target    AuthoringOptionTarget `json:"target"`
	Operation string                `json:"operation"`
	Action    string                `json:"action" jsonschema:"enum=cancel,enum=reconcile"`
}

// ValuesAvailable distinguishes a genuine empty page from lost, unretained
// values. Replaying an operation never repeats its source read.
type AuthoringOptionView struct {
	Operation           string         `json:"operation"`
	InputDigest         string         `json:"input_digest"`
	Status              string         `json:"status"`
	Code                string         `json:"code,omitempty"`
	ExecutionStatus     string         `json:"execution_status,omitempty"`
	RemoteState         string         `json:"remote_state,omitempty"`
	ValuesAvailable     bool           `json:"values_available"`
	NewOperationAllowed bool           `json:"new_operation_allowed"`
	SourceRevision      int64          `json:"source_revision"`
	Options             []FilterOption `json:"options"`
	Next                string         `json:"next,omitempty"`
	Complete            bool           `json:"complete"`
}

// This protected record contains coordinates and digests only. It deliberately
// contains no SQL, search text, cursor payload, option values or bearer token.
type AuthoringOptionRecord struct {
	Tenant           string                 `json:"tenant"`
	Actor            string                 `json:"actor"`
	Session          string                 `json:"session"`
	Operation        string                 `json:"operation"`
	InputDigest      string                 `json:"input_digest"`
	Target           AuthoringOptionTarget  `json:"target"`
	SourceOperation  string                 `json:"source_operation"`
	Source           string                 `json:"source"`
	Context          string                 `json:"context"`
	SourceRevision   int64                  `json:"source_revision"`
	BindingDigest    string                 `json:"binding_digest"`
	Topic            TopicPin               `json:"topic"`
	Dataset          string                 `json:"dataset"`
	Dimension        string                 `json:"dimension,omitempty"`
	Column           *ColumnReference       `json:"column,omitempty"`
	ResolutionDigest string                 `json:"resolution_digest"`
	References       []ResourceReference    `json:"references"`
	Blocks           []AuthoringOptionBlock `json:"blocks"`
	CreatedAt        time.Time              `json:"created_at"`
	Deadline         time.Time              `json:"deadline"`
	Status           string                 `json:"status"`
	Code             string                 `json:"code"`
	ExecutionStatus  string                 `json:"execution_status"`
	RemoteState      string                 `json:"remote_state"`
	CancelRequested  bool                   `json:"cancel_requested"`
	Rows             int                    `json:"rows"`
	Receipt          *exec.Receipt          `json:"receipt,omitempty"`
}
type AuthoringOptionBlock struct {
	Policy   string `json:"policy"`
	Block    string `json:"block"`
	Revision int64  `json:"revision"`
	Digest   string `json:"digest"`
}

func (AuthoringOptionRecord) String() string     { return "authoring-option-operation(redacted)" }
func (r AuthoringOptionRecord) GoString() string { return r.String() }

type AuthoringOptionRepository interface {
	ReserveAuthoringOption(context.Context, identity.Envelope, AuthoringOptionRecord) (AuthoringOptionRecord, bool, error)
	ReadAuthoringOption(context.Context, identity.Envelope, AuthoringOptionReference) (AuthoringOptionRecord, error)
	CheckAuthoringOption(context.Context, identity.Envelope, AuthoringOptionRecord) error
	SealAuthoringOption(context.Context, identity.Envelope, AuthoringOptionRecord, exec.Receipt) error
	FinishAuthoringOption(context.Context, identity.Envelope, AuthoringOptionRecord) error
	StopAuthoringOption(context.Context, identity.Envelope, AuthoringOptionReference, bool) (AuthoringOptionRecord, error)
}

// Timestamped keys make terminal cleanup safe: an evicted operation can never
// pass fresh admission again. The full key is canonical and client-known.
func AuthoringOptionOperationValid(operation string, now time.Time, fresh bool) bool {
	parts := strings.Split(operation, ":")
	if len(parts) != 3 || parts[0] != "option" || len(parts[2]) != 32 {
		return false
	}
	seconds, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || seconds < 1 || strconv.FormatInt(seconds, 10) != parts[1] {
		return false
	}
	raw, err := hex.DecodeString(parts[2])
	if err != nil || hex.EncodeToString(raw) != parts[2] {
		return false
	}
	if !fresh {
		return true
	}
	at := time.Unix(seconds, 0)
	return !at.Before(now.Add(-5*time.Minute)) && !at.After(now.Add(30*time.Second))
}

// Session changes cannot bypass unresolved liability for the same actor/target.
// Revision, search, cursor and dataset dimension changes do not widen this key.
func AuthoringOptionNamespace(target AuthoringOptionTarget) string {
	if d := target.Dataset; d != nil {
		return digest([]string{"dataset", d.NewBlock})
	}
	if r := target.Report; r != nil {
		return digest([]string{"report", r.Report, r.Page, r.Filter})
	}
	return ""
}

func RequireAuthoringOptionTarget(e identity.Envelope, target AuthoringOptionTarget) error {
	if err := requireAuthoringEnvelope(e); err != nil {
		return err
	}
	if (target.Dataset == nil) == (target.Report == nil) {
		return ErrInvalid
	}
	if d := target.Dataset; d != nil {
		if !d.valid() {
			return ErrInvalid
		}
		if err := requireMappingAuthoring(e); err != nil {
			return err
		}
		if err := access.Require(e, Write.Action(), access.Tenant(e, "write")); err != nil {
			return err
		}
		source := ""
		if d.SourceDataset != nil {
			source = d.SourceDataset.Source
		}
		if err := RequireOrigin(e, d.Topic.Topic, source, Write, true); err != nil {
			return err
		}
		for _, a := range []Access{Read, Write, Preview, Validate} {
			if err := Require(e, d.NewBlock, a); err != nil {
				return err
			}
		}
	} else {
		r := target.Report
		if (r.Policy != "private_preview" && r.Policy != "published") || !identity.Identifier(r.Report) || r.Revision < 1 || r.Revision > 256 || !hashValid(r.Digest) || !identity.Identifier(r.Page) || !identity.Identifier(r.Filter) {
			return ErrInvalid
		}
		actions := []Access{Read, Execute}
		if r.Policy == "private_preview" {
			actions = append(actions, Preview)
		}
		for _, a := range actions {
			if err := RequireDocument(e, "report", r.Report, a); err != nil {
				return err
			}
		}
	}
	if !e.Has("sources.query") || target.Dataset != nil && target.Dataset.SourceDataset == nil && !e.Has("topics.read") {
		return access.ErrForbidden
	}
	return nil
}

func RequireAuthoringOption(e identity.Envelope, r AuthoringOptionRecord) error {
	if err := RequireAuthoringOptionTarget(e, r.Target); err != nil {
		return err
	}
	if !r.ValidOrigin() {
		return ErrInvalid
	}
	if r.Topic.Topic != "" && !e.Has("topics.read") {
		return access.ErrForbidden
	}
	if r.Tenant != e.Tenant() || r.Actor != e.User() || r.Session != e.Session() {
		return access.ErrNotFound
	}
	if len(r.References) < 1 || len(r.References) > 4096 {
		return ErrInvalid
	}
	for _, ref := range r.References {
		if err := access.Require(e, Read.Action(), access.Resource{Tenant: e.Tenant(), Kind: ref.Kind, Permission: ref.Permission, ID: ref.ID}); err != nil {
			return err
		}
	}
	for _, b := range r.Blocks {
		actions := []Access{Read, Execute}
		if b.Policy == "private_preview" {
			actions = append(actions, Preview)
		}
		for _, a := range actions {
			if err := Require(e, b.Block, a); err != nil {
				return err
			}
		}
	}
	return access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: r.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: r.Context}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: r.Dataset})
}

func (d AuthoringDatasetOptionTarget) valid() bool {
	if !identity.Identifier(d.NewBlock) || !identity.Identifier(d.Dataset) || (d.Dimension == "") == (d.Column == "") {
		return false
	}
	if d.Dimension != "" && !identity.Identifier(d.Dimension) || d.Column != "" && !identity.Identifier(d.Column) {
		return false
	}
	if d.SourceDataset != nil {
		return d.Column != "" && d.Topic == (TopicPin{}) && d.SourceDataset.valid() && d.Dataset == d.SourceDataset.Dataset
	}
	return identity.Identifier(d.Topic.Topic) && identity.Identifier(d.Topic.Version) && hashValid(d.Topic.Digest)
}

// ValidOrigin checks protected retained coordinates, never grants authority.
func (r AuthoringOptionRecord) ValidOrigin() bool {
	topic := r.Topic != (TopicPin{})
	if topic && (!identity.Identifier(r.Topic.Topic) || !identity.Identifier(r.Topic.Version) || !hashValid(r.Topic.Digest)) {
		return false
	}
	if r.Column == nil {
		return topic && identity.Identifier(r.Dimension)
	}
	c := r.Column
	if r.Dimension != "" || !optionColumnType(c.Type) || !c.valid() || c.SourceDataset.Source != r.Source || c.SourceDataset.Context != r.Context || c.SourceDataset.SourceRevision != r.SourceRevision || c.SourceDataset.Dataset != r.Dataset {
		return false
	}
	if d := r.Target.Dataset; d != nil {
		if d.Column == "" || d.Dimension != "" || d.Topic != r.Topic {
			return false
		}
		if topic {
			return d.SourceDataset == nil
		}
		return d.SourceDataset != nil && *d.SourceDataset == c.SourceDataset
	}
	return r.Target.Report != nil
}
