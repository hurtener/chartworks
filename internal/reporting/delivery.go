package reporting

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

// Delivery is the shared HTTP/MCP/SDK catalog and selected-output facade. Read
// methods call only metadata and retained-result services. Run alone requests
// new warehouse/model work; the facade stores no duplicate analytical state.
type Delivery struct {
	blocks       *Service
	runs         *Runs
	documents    *Documents
	compositions *Compositions
	catalog      CompositionCatalog
	limits       config.ReportingViewer
}

// NewDelivery performs no metadata, source, model or network I/O.
func NewDelivery(blocks *Service, runs *Runs, documents *Documents, compositions *Compositions, catalog CompositionCatalog, limits config.ReportingViewer) (*Delivery, error) {
	if blocks == nil || runs == nil || documents == nil || compositions == nil || nilValue(catalog) || limits.Validate() != nil {
		return nil, ErrInvalid
	}
	return &Delivery{blocks: blocks, runs: runs, documents: documents, compositions: compositions, catalog: catalog, limits: limits}, nil
}

// CanExecute reports actual configured admission support, not user authority.
func (s *Delivery) CanExecute() bool { return s != nil && s.blocks.CanValidate() }

func deliveryKind(kind string) bool { return kind == "block" || documentKind(kind) }
func validDeliveryTarget(t DeliveryTarget) bool {
	return deliveryKind(t.Kind) && identity.Identifier(t.ID) && t.Revision >= 0
}
func (s *Delivery) begin(ctx context.Context, e identity.Envelope, action string) (context.Context, context.CancelFunc, error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if !e.Valid() {
		return nil, nil, access.ErrUnauthenticated
	}
	if !e.Has(action) {
		return nil, nil, access.ErrForbidden
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	if err := ctx.Err(); err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, cancel, nil
}

func deliveryText(s string, maxBytes int) bool {
	return len(s) <= maxBytes && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}

func localizedResource(kind, id string, revision int64, metadata []DocumentMetadata, wanted string) DeliveryResource {
	out := DeliveryResource{Target: DeliveryTarget{kind, id, revision}, Title: id, Locale: "en"}
	for i, m := range metadata {
		if i == 0 || m.Locale == wanted {
			out.Title, out.Description, out.Locale = m.Title, m.Description, m.Locale
		}
		if m.Locale == wanted {
			break
		}
	}
	return out
}
func blockResource(id string, revision int64, metadata []Localized, wanted string) DeliveryResource {
	m := make([]DocumentMetadata, len(metadata))
	for i, v := range metadata {
		m[i] = DocumentMetadata{Locale: v.Locale, Title: v.Title, Description: v.Description}
	}
	return localizedResource("block", id, revision, m, wanted)
}
func matchesResource(r DeliveryResource, query string) bool {
	return query == "" || strings.Contains(strings.ToLower(r.Title+" "+r.Description+" "+r.Target.ID), strings.ToLower(query))
}

// Search returns a continuation even when all authorized rows in a bounded page
// fail the text filter. It never scans an unbounded tenant index.
func (s *Delivery) Search(ctx context.Context, e identity.Envelope, in DeliverySearchRequest) (DeliverySearchResult, error) {
	out := DeliverySearchResult{Version: DeliveryVersion, Items: []DeliveryResource{}}
	ctx, cancel, err := s.begin(ctx, e, "reporting.read")
	if err != nil {
		return out, err
	}
	defer cancel()
	if !deliveryKind(in.Kind) || !deliveryText(in.Query, 256) || in.Locale != "" && !locale(in.Locale) || in.After != "" && !identity.Identifier(in.After) || in.Limit < 1 || in.Limit > 100 {
		return out, ErrInvalid
	}
	if in.Kind == "block" {
		page, err := s.blocks.List(ctx, e, ListRequest{After: in.After, Limit: in.Limit})
		if err != nil {
			return out, err
		}
		out.Next = page.Next
		for _, v := range page.Items {
			if v.Private || v.State.Archived || v.Revision == 0 {
				continue
			}
			r := blockResource(v.State.ID, v.Revision, v.Metadata, in.Locale)
			if matchesResource(r, in.Query) {
				out.Items = append(out.Items, r)
			}
		}
	} else {
		page, err := s.documents.List(ctx, e, in.Kind, in.After, in.Limit)
		if err != nil {
			return out, err
		}
		out.Next = page.Next
		for _, v := range page.Items {
			r := localizedResource(in.Kind, v.ID, v.Revision, v.Metadata, in.Locale)
			if matchesResource(r, in.Query) {
				out.Items = append(out.Items, r)
			}
		}
	}
	if err := s.bound(out); err != nil {
		return DeliverySearchResult{}, err
	}
	return out, ctx.Err()
}

func viewerChoices(selection *OutputSelection, locale string) []ViewerOutputChoice {
	out := []ViewerOutputChoice{}
	if selection == nil {
		return out
	}
	for _, choice := range selection.Choices {
		label := LocalizedOutput(choice.Intent, locale)
		out = append(out, ViewerOutputChoice{Metadata: clone(choice.Intent.Metadata), ID: choice.ID, Kind: choice.Kind, Title: label.DisplayName, Description: label.Description,
			Locale: label.Locale, DisplayOrder: choice.Intent.DisplayOrder, Enabled: choice.Intent.Enabled,
			DefaultSelected: choice.Intent.DefaultSelected, Selected: choice.Selected, State: choice.State, Code: choice.Code})
	}
	return out
}

func legacyViewerChoices(outputs []RetainedOutput, locale string) []ViewerOutputChoice {
	out := make([]ViewerOutputChoice, 0, len(outputs))
	for index, output := range outputs {
		choice := ViewerOutputChoice{ID: output.ID, Kind: output.Kind, Title: output.ID, Locale: locale, DisplayOrder: index, Enabled: true, DefaultSelected: true, Selected: true, State: "selected"}
		if output.Intent != nil {
			label := LocalizedOutput(*output.Intent, locale)
			choice.Title, choice.Description, choice.Locale = label.DisplayName, label.Description, label.Locale
			choice.Metadata = clone(output.Intent.Metadata)
			choice.DisplayOrder, choice.Enabled, choice.DefaultSelected = output.Intent.DisplayOrder, output.Intent.Enabled, output.Intent.DefaultSelected
		}
		out = append(out, choice)
	}
	slices.SortStableFunc(out, func(a, b ViewerOutputChoice) int { return a.DisplayOrder - b.DisplayOrder })
	return out
}

func chooseRetainedOutput(choices []ViewerOutputChoice, requested string) (string, error) {
	if requested == "" {
		for _, choice := range choices {
			if choice.Enabled && choice.Selected {
				return choice.ID, nil
			}
		}
		return "", nil
	}
	for _, choice := range choices {
		if choice.ID != requested {
			continue
		}
		if !choice.Enabled {
			return "", selectionError("output_disabled")
		}
		if !choice.Selected {
			return "", selectionError("output_not_selected")
		}
		return requested, nil
	}
	return "", access.ErrNotFound
}

func describeReportPage(id, report string, revision int64, d DocumentDefinition) CompositionPageSummary {
	page := CompositionPageSummary{ID: id, Report: report, Revision: revision, Title: report, Locale: d.Locale, Timezone: d.Timezone, Widgets: []CompositionWidgetSummary{}}
	if len(d.Metadata) != 0 {
		page.Title = d.Metadata[0].Title
	}
	for _, w := range d.Widgets {
		v := CompositionWidgetSummary{ID: w.ID, Kind: w.Kind, State: "not_run", Grid: w.Grid, Presentation: w.Presentation, Outputs: []string{}, Parameters: []BoundValue{}}
		if w.Block != nil {
			v.Outputs = clone(w.Block.Outputs)
		}
		if w.Query != nil {
			v.Durability = w.Query.Durability
		}
		page.Widgets = append(page.Widgets, v)
	}
	return page
}

func (s *Delivery) appendReportDescription(ctx context.Context, e identity.Envelope, out *DeliveryDescription, page string, view DocumentView, wanted string) error {
	d := view.Definition
	described := describeReportPage(page, view.State.ID, view.Revision, d)
	resource := localizedResource("report", view.State.ID, view.Revision, d.Metadata, wanted)
	described.Title, described.Locale = resource.Title, resource.Locale
	if err := s.describeBlockSelectors(ctx, e, &described, d); err != nil {
		return err
	}
	out.Pages = append(out.Pages, described)
	for _, f := range d.Filters {
		out.Filters = append(out.Filters, ViewerFilter{Page: page, Label: f.Label, Parameter: f.Parameter})
	}
	for _, w := range d.Widgets {
		if w.Kind == "query" {
			out.Dynamic = true
		}
	}
	return nil
}

// Describe never returns saved SQL, query text, raw narrative instructions or a
// private definition. Dashboard pages remain independently permission-filtered.
func (s *Delivery) Describe(ctx context.Context, e identity.Envelope, in DeliveryDescribeRequest) (DeliveryDescription, error) {
	out := DeliveryDescription{Version: DeliveryVersion, Outputs: []ViewerOutputChoice{}, Filters: []ViewerFilter{}, Pages: []CompositionPageSummary{}}
	ctx, cancel, err := s.begin(ctx, e, "reporting.read")
	if err != nil {
		return out, err
	}
	defer cancel()
	if !validDeliveryTarget(in.Target) || in.Locale != "" && !locale(in.Locale) || len(in.Outputs) > s.limits.MaxOutputs {
		return out, ErrInvalid
	}
	t := in.Target
	if t.Kind == "block" {
		v, err := s.blocks.Read(ctx, e, t.ID, Reference{Revision: t.Revision})
		if err != nil {
			return out, err
		}
		if v.Private || v.State.Archived {
			return out, access.ErrNotFound
		}
		_, selection, err := ResolveOutputSelection(Definition{SchemaVersion: v.SchemaVersion, Metadata: v.Metadata, Outputs: v.Outputs}, in.Outputs)
		if err != nil && (in.Outputs != nil || SelectionErrorCode(err) != "output_selection_empty") {
			return out, err
		}
		out.Selection, out.SelectionCode = &selection, SelectionErrorCode(err)
		out.Resource = blockResource(t.ID, v.Revision, v.Metadata, in.Locale)
		out.Outputs, out.Trust, out.Timezone = viewerChoices(&selection, out.Resource.Locale), clone(&v.Trust), "UTC"
		out.QueryLimits, out.ResultPolicy = clone(v.QueryLimits), clone(v.ResultPolicy)
		for _, p := range v.Parameters {
			out.Filters = append(out.Filters, ViewerFilter{Page: "main", Label: p.Name, Parameter: p})
		}
	} else {
		if len(in.Outputs) != 0 {
			return out, ErrInvalid
		}
		v, err := s.documents.Read(ctx, e, t.Kind, t.ID, DocumentReference{Revision: t.Revision})
		if err != nil {
			return out, err
		}
		if v.Private || v.State.Archived {
			return out, access.ErrNotFound
		}
		out.Resource = localizedResource(t.Kind, t.ID, v.Revision, v.Definition.Metadata, in.Locale)
		out.Timezone = v.Definition.Timezone
		if t.Kind == "report" {
			if err := s.appendReportDescription(ctx, e, &out, "main", v, in.Locale); err != nil {
				return DeliveryDescription{}, err
			}
		} else {
			for _, p := range v.Definition.Pages {
				child, err := s.documents.Read(ctx, e, "report", p.Report, DocumentReference{Revision: p.Revision})
				if err != nil {
					return DeliveryDescription{}, err
				}
				if child.Private || child.State.Archived {
					continue
				}
				if err := s.appendReportDescription(ctx, e, &out, p.ID, child, in.Locale); err != nil {
					return DeliveryDescription{}, err
				}
			}
		}
	}
	if err := s.bound(out); err != nil {
		return DeliveryDescription{}, err
	}
	return out, ctx.Err()
}

// Run requires the exact published revision returned by Describe. This makes
// dynamic/narrative consent stable across concurrent publication. The existing
// admission still pins dependencies and checks immutable target eligibility.
func (s *Delivery) Run(ctx context.Context, e identity.Envelope, in DeliveryRunRequest) (DeliveryRunResult, error) {
	out := DeliveryRunResult{Version: DeliveryVersion, Kind: in.Target.Kind, Target: in.Target}
	ctx, cancel, err := s.begin(ctx, e, "reporting.execute")
	if err != nil {
		return out, err
	}
	defer cancel()
	if !validDeliveryTarget(in.Target) || in.Target.Revision < 1 || !identity.Identifier(in.Key) || in.Locale != "" && !locale(in.Locale) || len(in.Arguments) > 64 || len(in.Pages) > 32 || len(in.Outputs) > s.limits.MaxOutputs || in.Policy == "private_preview" {
		return out, ErrInvalid
	}
	t := in.Target
	resolution := Resolution{Timezone: in.Timezone}
	if t.Kind == "block" {
		if len(in.Pages) != 0 || in.Dynamic {
			return out, ErrInvalid
		}
		r, err := s.runs.Admit(ctx, e, t.ID, RunRequest{Key: in.Key, Reference: Reference{Revision: t.Revision}, Arguments: in.Arguments, Resolution: resolution, Outputs: in.Outputs, Limits: clone(in.Limits), Policy: in.Policy, Locale: in.Locale, Narrative: in.Narrative, PartialPolicy: in.PartialFailure})
		if err != nil {
			return out, err
		}
		r, err = s.runs.Run(ctx, e, r.ID, false)
		out.Run, out.State, out.Code, out.Target.Revision = r.ID, r.State, r.Code, r.Revision
		return out, err
	}
	if in.Limits != nil || len(in.Arguments) != 0 || len(in.Outputs) != 0 || in.Policy != "" {
		return out, ErrInvalid
	}
	if err := s.requireRunOptIns(ctx, e, t, in.Dynamic, in.Narrative); err != nil {
		return out, err
	}
	r, err := s.compositions.Admit(ctx, e, t.Kind, t.ID, CompositionRequest{Key: in.Key, Reference: DocumentReference{Revision: t.Revision}, Resolution: resolution, Pages: in.Pages, PartialFailure: in.PartialFailure})
	if err != nil {
		return out, err
	}
	r, err = s.compositions.Run(ctx, e, r.ID, false)
	out.Run, out.State, out.Code, out.Target.Revision = r.ID, r.State, r.Code, r.Revision
	return out, err
}

func (s *Delivery) requireRunOptIns(ctx context.Context, e identity.Envelope, t DeliveryTarget, dynamic, narrative bool) error {
	v, err := s.documents.repo.ReadDocument(ctx, e, t.Kind, t.ID, DocumentReference{Revision: t.Revision}, Execute, false)
	if err != nil {
		return err
	}
	if v.PublishedAt == nil || v.State.Archived {
		return ErrStale
	}
	d, err := ProjectStoredDocument(v.Revision.Raw, t.Kind)
	if err != nil {
		return err
	}
	for _, w := range d.Widgets {
		if w.Kind == "query" && !dynamic || w.Block != nil && w.Block.Narrative && !narrative {
			return ErrInvalid
		}
	}
	for _, p := range d.Pages {
		if err := s.requireRunOptIns(ctx, e, DeliveryTarget{Kind: "report", ID: p.Report, Revision: p.Revision}, dynamic, narrative); err != nil {
			return err
		}
	}
	return nil
}

// Runs returns catalog metadata only, including expired tombstones. Recipients
// or schedule provenance never substitute for current signed artifact reach.
func (s *Delivery) Runs(ctx context.Context, e identity.Envelope, in DeliveryRunsRequest) (DeliveryRunsResult, error) {
	out := DeliveryRunsResult{Version: DeliveryVersion, Items: []DeliveryRunSummary{}}
	ctx, cancel, err := s.begin(ctx, e, "reporting.read")
	if err != nil {
		return out, err
	}
	defer cancel()
	if !deliveryKind(in.Kind) || in.Resource != "" && !identity.Identifier(in.Resource) || in.After != "" && !identity.Identifier(in.After) || in.Limit < 1 || in.Limit > 100 {
		return out, ErrInvalid
	}
	if in.Kind != "block" {
		result, err := s.catalog.ListCompositionArtifacts(ctx, e, in.Kind, in.Resource, in.After, in.Limit)
		if err != nil {
			return DeliveryRunsResult{}, err
		}
		if err := s.bound(result); err != nil {
			return DeliveryRunsResult{}, err
		}
		return result, ctx.Err()
	}
	page, err := s.runs.List(ctx, e, in.After, in.Limit)
	if err != nil {
		return out, err
	}
	out.Next = page.Next
	for _, v := range page.Items {
		if in.Resource == "" || v.Block == in.Resource {
			out.Items = append(out.Items, blockRunSummary(v))
		}
	}
	if err := s.bound(out); err != nil {
		return DeliveryRunsResult{}, err
	}
	return out, ctx.Err()
}

func blockRunSummary(v RunView) DeliveryRunSummary {
	return DeliveryRunSummary{Kind: "block", Run: v.ID, Target: DeliveryTarget{"block", v.Block, v.Revision}, State: v.State, Code: v.Code, Private: v.Private, Created: v.Created, Expires: v.Expires, Scheduled: clone(v.Scheduled)}
}

// CompositionRunSummary contains no invisible page names, rows or aggregate
// counts. The store supplies only an already authorized composition head.
func CompositionRunSummary(v CompositionView) DeliveryRunSummary {
	return DeliveryRunSummary{Kind: v.Kind, Run: v.ID, Target: DeliveryTarget{v.Kind, v.Document, v.Revision}, State: v.State, Code: v.Code, Private: v.Private, Created: v.Created, Expires: v.Expires, Scheduled: clone(v.Scheduled)}
}

func (s *Delivery) bound(value any) error {
	b, err := json.Marshal(value)
	if err != nil || len(b) > s.limits.MaxMessageBytes {
		return ErrBudget
	}
	return nil
}
