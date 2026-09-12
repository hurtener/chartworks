package chartworks

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strconv"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
)

// ErrDocumentRequest rejects invalid coordinates before acquiring credentials.
var ErrDocumentRequest = errors.New("chartworks: invalid report or dashboard request")

// DocumentKind selects one of the two versioned composition definition owners.
type DocumentKind string

const (
	// ReportDocument contains block, dynamic query and inert text widgets.
	ReportDocument DocumentKind = "report"
	// DashboardDocument contains ordered exact report revision references.
	DashboardDocument DocumentKind = "dashboard"
)

// DocumentDefinition is the shared closed report/dashboard definition contract.
type DocumentDefinition = reporting.DocumentDefinition

// DocumentMetadata is localized presentation, never a resource grant.
type DocumentMetadata = reporting.DocumentMetadata

// DocumentWidget is the closed block/query/text widget union.
type DocumentWidget = reporting.Widget

// DocumentGrid occupies a bounded, nonoverlapping twelve-column grid region.
type DocumentGrid = reporting.GridCell

// DocumentPresentation contains only bounded inert presentation overrides.
type DocumentPresentation = reporting.Presentation

// DocumentText holds inert plain or restricted Markdown content.
type DocumentText = reporting.TextWidget

// DocumentBlock references selected outputs from one governed block revision.
type DocumentBlock = reporting.BlockWidget

// DocumentQuery declares replayable or originating-session-bound query intent.
type DocumentQuery = reporting.QueryWidget

// DocumentFilter uses the exact governed block parameter type system.
type DocumentFilter = reporting.ReportFilter

// DocumentFilterBinding maps a report filter to a block parameter, not security policy.
type DocumentFilterBinding = reporting.FilterBinding

// DocumentPage is an ordered exact published report reference.
type DocumentPage = reporting.DocumentPage

// DocumentLegacySection describes the supported historical section format.
type DocumentLegacySection = reporting.LegacySection

// DocumentReference selects an exact revision or an independent lifecycle pointer.
type DocumentReference = reporting.DocumentReference

// DocumentState retains independent draft, review and publication pointers.
type DocumentState = reporting.DocumentState

// DocumentView contains only currently authorized definition metadata.
type DocumentView = reporting.DocumentView

// DocumentList is a bounded, permission-filtered published metadata page.
type DocumentList = reporting.DocumentList

// DocumentCreateRequest creates a private draft under explicit write authority.
type DocumentCreateRequest = reportingapi.DocumentCreate

// DocumentEditRequest creates an immutable amendment under CAS.
type DocumentEditRequest = reportingapi.DocumentEdit

// DocumentTransitionRequest identifies an exact lifecycle transition revision.
type DocumentTransitionRequest = reportingapi.DocumentTransition

// DocumentImportRequest carries bounded original JSON and external version identity.
type DocumentImportRequest = reportingapi.DocumentImport

// DocumentImportResult distinguishes accepted drafts from private quarantine.
type DocumentImportResult = reporting.DocumentImportResult

// DocumentExternalReference is a versioned external coordinate, never an access grant.
type DocumentExternalReference = reporting.ExternalReference

// CompositionRequest reserves an operation key and seals exact execution inputs.
type CompositionRequest = reporting.CompositionRequest

// CompositionPageInput supplies business filters and declared widget overrides.
type CompositionPageInput = reporting.PageInput

// CompositionWidgetOverride contains only explicitly permitted parameter values.
type CompositionWidgetOverride = reporting.WidgetOverride

// CompositionView contains retained metadata, not SQL or raw normalized rows.
type CompositionView = reporting.CompositionView

// CompositionPayload contains the addressed visible widget's saved output subset.
type CompositionPayload = reporting.CompositionPayload

func documentPath(kind DocumentKind, id string) (string, error) {
	if kind != ReportDocument && kind != DashboardDocument || id != "" && !identity.Identifier(id) {
		return "", ErrDocumentRequest
	}
	path := "/v1/" + string(kind) + "s"
	if id != "" {
		path += "/" + id
	}
	return path, nil
}

// CreateDocument creates a private draft. Mutations are never automatically retried.
func (c *Client) CreateDocument(ctx context.Context, kind DocumentKind, in DocumentCreateRequest) (out DocumentState, err error) {
	path, err := documentPath(kind, "")
	if err != nil || !identity.Identifier(in.ID) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "POST", path, "", in, &out, 4<<20)
	return
}

// EditDocument appends an immutable amendment without replacing a pending review.
func (c *Client) EditDocument(ctx context.Context, kind DocumentKind, id string, in DocumentEditRequest) (out DocumentState, err error) {
	path, err := documentPath(kind, id)
	if err != nil || id == "" || in.ExpectedVersion < 1 {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "PUT", path, "", in, &out, 4<<20)
	return
}

// TransitionDocument reviews, publishes, rejects or archives an exact revision.
// Creator/audience labels and a prior successful preview never authorize it.
func (c *Client) TransitionDocument(ctx context.Context, kind DocumentKind, id, transition string, in DocumentTransitionRequest) (out DocumentState, err error) {
	path, err := documentPath(kind, id)
	if err != nil || id == "" || in.ExpectedVersion < 1 || in.Revision < 1 || !slices.Contains([]string{"review", "publish", "reject", "archive"}, transition) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "POST", path+"/"+transition, "", in, &out, 4<<20)
	return
}

// ReadDocument reads an authorized definition without executing widgets.
func (c *Client) ReadDocument(ctx context.Context, kind DocumentKind, id string, in DocumentReference) (out DocumentView, err error) {
	path, err := documentPath(kind, id)
	if err != nil || id == "" || in.Revision < 0 || in.Revision > 256 || in.Revision > 0 && in.Stage != "" || !slices.Contains([]string{"", "published", "draft", "review"}, in.Stage) {
		return out, ErrDocumentRequest
	}
	q := url.Values{}
	if in.Revision > 0 {
		q.Set("revision", strconv.FormatInt(in.Revision, 10))
	}
	if in.Stage != "" {
		q.Set("stage", in.Stage)
	}
	if len(q) != 0 {
		path += "?" + q.Encode()
	}
	err = c.callLimit(ctx, "GET", path, "", nil, &out, 4<<20)
	return
}

// ListDocuments reads a bounded published metadata index under current authority.
func (c *Client) ListDocuments(ctx context.Context, kind DocumentKind, after string, limit int) (out DocumentList, err error) {
	path, err := documentPath(kind, "")
	if limit == 0 {
		limit = 20
	}
	if err != nil || after != "" && !identity.Identifier(after) || limit < 1 || limit > 100 {
		return out, ErrDocumentRequest
	}
	q := url.Values{"limit": []string{strconv.Itoa(limit)}}
	if after != "" {
		q.Set("after", after)
	}
	err = c.callLimit(ctx, "GET", path+"?"+q.Encode(), "", nil, &out, 4<<20)
	return
}

// ImportDocument preserves supported original JSON and its exact external version.
func (c *Client) ImportDocument(ctx context.Context, kind DocumentKind, in DocumentImportRequest) (out DocumentImportResult, err error) {
	path, err := documentPath(kind, "")
	if err != nil || !identity.Identifier(in.ID) || len(in.DefinitionJSON) == 0 || len(in.DefinitionJSON) > 2<<20 {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "POST", path+"/import", "", in, &out, 4<<20)
	return
}

// AdmitComposition reserves the body key once; explicit replay retains its first
// seal. The client never silently retries a mutation after a lost response.
func (c *Client) AdmitComposition(ctx context.Context, kind DocumentKind, id string, in CompositionRequest) (out CompositionView, err error) {
	path, err := documentPath(kind, id)
	if err != nil || id == "" || !identity.Identifier(in.Key) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "POST", path+"/runs", "", in, &out, 16<<20)
	return
}

// ExecuteComposition executes or explicitly resumes one signed bounded attempt.
func (c *Client) ExecuteComposition(ctx context.Context, id string, resume bool) (out CompositionView, err error) {
	if !identity.Identifier(id) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/composition-runs/"+id+"/execute", "", reportingapi.RunDispatch{Resume: resume}, &out, 16<<20)
	return
}

// ReadComposition reads metadata only, with current page and actual-context reach.
func (c *Client) ReadComposition(ctx context.Context, id string) (out CompositionView, err error) {
	if !identity.Identifier(id) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "GET", "/v1/composition-runs/"+id, "", nil, &out, 16<<20)
	return
}

// InspectComposition reads an actor/session-private execution receipt.
func (c *Client) InspectComposition(ctx context.Context, id string) (out CompositionView, err error) {
	if !identity.Identifier(id) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "GET", "/v1/composition-runs/"+id+"/receipt", "", nil, &out, 16<<20)
	return
}

// ReadCompositionWidget reads only one visible widget's retained output subset.
func (c *Client) ReadCompositionWidget(ctx context.Context, id, page, widget string) (out CompositionPayload, err error) {
	if !identity.Identifier(id) || !identity.Identifier(page) || !identity.Identifier(widget) {
		return out, ErrDocumentRequest
	}
	q := url.Values{"page": []string{page}, "widget": []string{widget}}
	err = c.callLimit(ctx, "GET", "/v1/composition-runs/"+id+"/widget?"+q.Encode(), "", nil, &out, 32<<20)
	return
}

// CancelComposition records durable cancellation, not proof of native termination.
func (c *Client) CancelComposition(ctx context.Context, id string) (out CompositionView, err error) {
	if !identity.Identifier(id) {
		return out, ErrDocumentRequest
	}
	err = c.callLimit(ctx, "POST", "/v1/composition-runs/"+id+"/cancel", "", struct{}{}, &out, 4<<20)
	return
}

// ExpireCompositions explicitly erases a bounded page of expired retained values.
func (c *Client) ExpireCompositions(ctx context.Context, limit int) (removed int64, err error) {
	if limit < 1 || limit > 100 {
		return 0, ErrDocumentRequest
	}
	var out reportingapi.RetentionResult
	err = c.callLimit(ctx, "POST", "/v1/composition-retention", "", reportingapi.RetentionRequest{Limit: limit}, &out, 4<<20)
	return out.Removed, err
}
