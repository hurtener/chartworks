package reporting

import (
	"context"
	"encoding/json"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
)

// DocumentVersion versions the canonical read projection, not historical bytes.
const DocumentVersion = 2

// DocumentMetadata is localized presentation, never identity or access policy.
type DocumentMetadata struct {
	Locale      string `json:"locale"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// GridCell occupies a bounded, nonoverlapping region in a twelve-column grid.
type GridCell struct {
	Column int `json:"column"`
	Row    int `json:"row"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Presentation deliberately has no scripts, URLs, security predicates, column
// selection, chart-type replacement or unit/currency reinterpretation fields.
type Presentation struct {
	Title      string `json:"title,omitempty"`
	Subtitle   string `json:"subtitle,omitempty"`
	Density    string `json:"density,omitempty"`
	ShowLegend *bool  `json:"show_legend,omitempty"`
}

// TextWidget retains text, not executable HTML. Markdown is the inert subset
// described by ValidateDocument; consumers must render it without raw HTML,
// links, images or automatic URL activation.
type TextWidget struct {
	Format string `json:"format" jsonschema:"enum=plain,enum=markdown"`
	Text   string `json:"text"`
}

// BlockWidget may follow publication at admission (revision zero), or pin an
// exact published revision. It cannot select a private block revision implicitly.
type BlockWidget struct {
	Block     string   `json:"block"`
	Revision  int64    `json:"revision"`
	Outputs   []string `json:"outputs"`
	Policy    string   `json:"policy,omitempty" jsonschema:"enum=published,enum=certified_only,enum=explicit_stale"`
	Narrative bool     `json:"narrative"`
}

// QueryWidget is an explicitly dynamic lane, not a block or a certificate.
// Replayable questions have exact semantic pins and no originating session.
// Session-bound references are resolved using the current actor/session only.
type QueryWidget struct {
	Durability string     `json:"durability" jsonschema:"enum=replayable,enum=session_bound"`
	Context    string     `json:"context"`
	Topics     []TopicPin `json:"topics"`
	Question   string     `json:"question,omitempty"`
	Query      string     `json:"query,omitempty"`
}

// FilterBinding names one declared filter and one block parameter.
type FilterBinding struct {
	Filter    string `json:"filter"`
	Parameter string `json:"parameter"`
}

// Widget is a closed tagged union; exactly one payload must match Kind.
type Widget struct {
	ID          string          `json:"id"`
	Kind        string          `json:"kind" jsonschema:"enum=block,enum=query,enum=text"`
	Grid        GridCell        `json:"grid"`
	Presentation Presentation   `json:"presentation"`
	Block       *BlockWidget    `json:"block,omitempty"`
	Query       *QueryWidget    `json:"query,omitempty"`
	Text        *TextWidget     `json:"text,omitempty"`
	Literals    []Argument      `json:"literals,omitempty"`
	Bindings    []FilterBinding `json:"bindings,omitempty"`
	Overrides   []string        `json:"overrides,omitempty"`
	Section     string          `json:"section,omitempty"`
}

// ReportFilter reuses the exact scalar/period type system used by frozen blocks.
type ReportFilter struct {
	Parameter Parameter `json:"parameter"`
	Label     string    `json:"label"`
}

// DocumentPage orders an exact report revision. Page names are held with the
// reference and are projected only after the report's signed reach is checked.
type DocumentPage struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Report   string `json:"report"`
	Revision int64  `json:"revision"`
}

// LegacySection is the supported version-one import form. Projection never
// rewrites the retained revision. Unsupported versions go to private quarantine.
type LegacySection struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Widgets []Widget `json:"widgets"`
}

// DocumentDefinition composes existing execution capabilities. A report has
// widgets; a dashboard has pages. Audience labels remain descriptive only.
type DocumentDefinition struct {
	SchemaVersion  int                `json:"schema_version"`
	Metadata       []DocumentMetadata `json:"metadata"`
	Locale         string             `json:"locale"`
	Timezone       string             `json:"timezone"`
	Audience       []string           `json:"audience,omitempty"`
	Widgets        []Widget           `json:"widgets,omitempty"`
	Filters        []ReportFilter     `json:"filters,omitempty"`
	Defaults       []Argument         `json:"defaults,omitempty"`
	PartialFailure string             `json:"partial_failure,omitempty" jsonschema:"enum=fail_closed,enum=allow_partial"`
	Pages          []DocumentPage     `json:"pages,omitempty"`
	Sections       []LegacySection    `json:"sections,omitempty"`
}

// ExternalReference identifies one immutable external version, not a URL or a
// grant. A reused external coordinate with different content is a conflict.
type ExternalReference struct {
	System  string `json:"system"`
	ID      string `json:"id"`
	Version string `json:"version"`
}

// DocumentReference selects an exact revision or one independent head pointer.
type DocumentReference struct {
	Revision int64  `json:"revision"`
	Stage    string `json:"stage,omitempty" jsonschema:"enum=published,enum=draft,enum=review"`
}

// DocumentState has independent draft, pending-review and published pointers.
// Public projections hide the private pointers, not merely their payloads.
type DocumentState struct {
	Kind              string    `json:"kind"`
	ID                string    `json:"id"`
	Version           int64     `json:"version"`
	LatestRevision    int64     `json:"latest_revision"`
	DraftRevision     int64     `json:"draft_revision"`
	ReviewRevision    int64     `json:"review_revision"`
	PublishedRevision int64     `json:"published_revision"`
	Archived          bool      `json:"archived"`
	Created           time.Time `json:"created_at"`
	Updated           time.Time `json:"updated_at"`
}

// DocumentRevision contains immutable authored bytes and server-derived query
// origins. It is private repository input; normal responses use DocumentView.
type DocumentRevision struct {
	Number  int64               `json:"number"`
	Raw     json.RawMessage     `json:"raw"`
	Digest  string              `json:"digest"`
	Actor   string              `json:"actor"`
	Session string              `json:"session"`
	Created time.Time           `json:"created_at"`
	Origins []QueryOrigin       `json:"origins"`
	External *ExternalReference `json:"external,omitempty"`
}

// QueryOrigin is service-derived evidence; the author cannot submit trust flags.
type QueryOrigin struct {
	Widget         string     `json:"widget"`
	Query          string     `json:"query"`
	Actor          string     `json:"actor"`
	Session        string     `json:"session"`
	Source         string     `json:"source"`
	Context        string     `json:"context"`
	Topics         []TopicPin `json:"topics"`
	SemanticDigest string     `json:"semantic_digest"`
	QueryDigest    string     `json:"query_digest"`
}

// DocumentSnapshot is an internal authorized repository result.
type DocumentSnapshot struct {
	State       DocumentState
	Revision    DocumentRevision
	PublishedAt *time.Time
}

// DocumentView projects legacy content without mutating stored revisions.
type DocumentView struct {
	State      DocumentState      `json:"state"`
	Revision   int64              `json:"revision"`
	Digest     string             `json:"digest"`
	Private    bool               `json:"private"`
	Definition DocumentDefinition `json:"definition"`
}

// DocumentSummary never contains widget payloads, page names or raw results.
type DocumentSummary struct {
	Kind     string             `json:"kind"`
	ID       string             `json:"id"`
	Version  int64              `json:"version"`
	Revision int64              `json:"revision"`
	Metadata []DocumentMetadata `json:"metadata"`
}

// DocumentList is a bounded, permission-filtered metadata page.
type DocumentList struct {
	Items []DocumentSummary `json:"items"`
	Next  string            `json:"next,omitempty"`
}

// DocumentMutation is accepted only through the service-issued prepared proof.
type DocumentMutation struct {
	Kind            string
	ID              string
	Operation       string
	ExpectedVersion int64
	TargetRevision  int64
	Revision        *DocumentRevision
	Note            string
	MaxRevisions    int
	MaxDocuments    int
}

// DocumentRepository owns tenant-composite storage and transactional CAS.
// redactPages is mandatory on dashboard read surfaces and false only for a
// separately authorized authoring/execution caller resolving exact dependencies.
type DocumentRepository interface {
	ReadDocument(context.Context, identity.Envelope, string, string, DocumentReference, Access, bool) (DocumentSnapshot, error)
	CommitDocument(context.Context, identity.Envelope, PreparedDocument) (DocumentState, error)
	ListDocuments(context.Context, identity.Envelope, string, string, int) (DocumentList, error)
	QuarantineDocument(context.Context, identity.Envelope, PreparedQuarantine) (string, error)
}
