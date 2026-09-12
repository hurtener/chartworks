package reporting

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/chartdata"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ViewerTable is an exact window of a retained table. Totals retain their declared
// whole-result scope; they are never recalculated over just the visible page.
type ViewerTable struct {
	Columns      []charts.Column     `json:"columns"`
	Rows         [][]charts.Cell     `json:"rows"`
	Totals       []charts.Total      `json:"totals"`
	Completeness charts.Completeness `json:"completeness"`
	Warnings     []string            `json:"warnings"`
}

// ViewerOutput distinguishes a bounded projection from the original artifact.
// RetainedDigest identifies the full retained output, NOT this paged response.
// Non-table charts stay complete: the viewer must not silently sample points.
type ViewerOutput struct {
	ID             string           `json:"id"`
	Kind           string           `json:"kind"`
	State          string           `json:"state"`
	Code           string           `json:"code"`
	RetainedDigest string           `json:"retained_digest"`
	Chart          *charts.Output   `json:"chart,omitempty"`
	Table          *ViewerTable     `json:"table,omitempty"`
	Narrative      *NarrativeResult `json:"narrative,omitempty"`
}

func (s *Delivery) viewRequest(in ReportingViewRequest) (ReportingViewRequest, error) {
	if !deliveryKind(in.Kind) || !identity.Identifier(in.Run) || in.Page != "" && !identity.Identifier(in.Page) || in.Widget != "" && !identity.Identifier(in.Widget) || in.Output != "" && !identity.Identifier(in.Output) || in.Offset < 0 || in.Offset > 100000 || in.Limit < 0 || in.Limit > s.limits.MaxRows {
		return in, ErrInvalid
	}
	if in.Limit == 0 {
		in.Limit = s.limits.PageRows
	}
	if in.Kind == "block" && (in.Page != "" || in.Widget != "") {
		return in, ErrInvalid
	}
	return in, nil
}

func tableBounds(total, offset, limit int) (ViewerPage, int, error) {
	if total < 0 || offset < 0 || offset > total || limit < 1 {
		return ViewerPage{}, 0, ErrInvalid
	}
	end := offset + min(limit, total-offset)
	out := ViewerPage{Offset: offset, Limit: limit, Total: total}
	if end < total {
		out.Next = &end
	}
	return out, end, nil
}

func (s *Delivery) projectOutput(v RetainedOutput, in ReportingViewRequest) (*ViewerOutput, ViewerPage, error) {
	out := &ViewerOutput{ID: v.ID, Kind: v.Kind, State: v.State, Code: v.Code, RetainedDigest: v.Digest}
	bounds := ViewerPage{Offset: 0, Limit: in.Limit, Total: 0}
	if v.State != "succeeded" {
		if in.Offset != 0 {
			return nil, bounds, ErrInvalid
		}
		return out, bounds, nil
	}
	if v.Chart != nil {
		c := v.Chart
		if c.Kind == charts.Kind("table") {
			b, end, err := tableBounds(len(c.Rows), in.Offset, in.Limit)
			if err != nil {
				return nil, bounds, err
			}
			out.Table = &ViewerTable{Columns: clone(c.Columns), Rows: clone(c.Rows[in.Offset:end]), Totals: clone(c.Totals), Completeness: c.Completeness, Warnings: clone(c.Warnings)}
			return out, b, nil
		}
		if in.Offset != 0 {
			return nil, bounds, ErrInvalid
		}
		if len(c.Points) > s.limits.MaxPoints || len(c.Rows) > s.limits.MaxPoints {
			return nil, bounds, ErrBudget
		}
		out.Chart = clone(c)
		bounds.Total = len(c.Points)
		return out, bounds, nil
	}
	if in.Offset != 0 || v.Narrative == nil {
		return nil, bounds, ErrInvalid
	}
	out.Narrative = clone(v.Narrative)
	return out, bounds, nil
}

// View selects only previously retained values. All branches below are metadata
// or artifact reads; none can call Run, a source, the gateway or a schedule.
func (s *Delivery) View(ctx context.Context, e identity.Envelope, input ReportingViewRequest) (ReportingViewResult, error) {
	out := ReportingViewResult{Version: DeliveryVersion, Outputs: []ViewerOutputChoice{}, Pages: []CompositionPageSummary{}, Filters: []ViewerFilter{}}
	ctx, cancel, err := s.begin(ctx, e, "reporting.read")
	if err != nil {
		return out, err
	}
	defer cancel()
	in, err := s.viewRequest(input)
	if err != nil {
		return out, err
	}
	out.Selection = in
	out.PageBounds.Limit = in.Limit
	if in.Kind == "block" {
		err = s.viewBlock(ctx, e, &out)
	} else {
		err = s.viewComposition(ctx, e, &out)
	}
	if err != nil {
		// No partial payload accompanies denial, expiration races or corruption.
		return ReportingViewResult{}, err
	}
	if out.Summary.State != "expired" && !out.Summary.Private {
		// Artifact-only readers need not also possess definition-read authority.
		// Optional business-filter descriptions never expand artifact permission.
		description, descErr := s.Describe(ctx, e, ReportingDescribeRequest{Target: out.Summary.Target, Locale: out.Locale})
		if descErr == nil {
			out.Filters = description.Filters
		} else if !errors.Is(descErr, access.ErrForbidden) && !errors.Is(descErr, access.ErrNotFound) && !errors.Is(descErr, store.ErrNotFound) && !errors.Is(descErr, ErrStale) {
			return ReportingViewResult{}, descErr
		}
	}
	if len(out.Outputs) > s.limits.MaxOutputs {
		return ReportingViewResult{}, ErrBudget
	}
	if err := s.bound(out); err != nil {
		return ReportingViewResult{}, err
	}
	if !e.Valid() {
		return ReportingViewResult{}, access.ErrUnauthenticated
	}
	if err := ctx.Err(); err != nil {
		return ReportingViewResult{}, err
	}
	return out, nil
}

func (s *Delivery) viewBlock(ctx context.Context, e identity.Envelope, out *ReportingViewResult) error {
	v, err := s.runs.Get(ctx, e, out.Selection.Run)
	if err != nil {
		return err
	}
	out.Summary = blockRunSummary(v)
	out.Locale, out.Timezone, out.Trust, out.Observed = v.Locale, v.Timezone, clone(&v.Trust), clone(v.Observed)
	if v.State == "expired" {
		return nil
	}
	for _, item := range v.Outputs {
		out.Outputs = append(out.Outputs, ViewerOutputChoice{ID: item.ID, Kind: item.Kind, Title: item.ID})
	}
	if out.Selection.Output == "" && len(out.Outputs) != 0 {
		out.Selection.Output = out.Outputs[0].ID
	}
	if out.Selection.Output == "" {
		return nil
	}
	found := false
	for _, item := range out.Outputs {
		found = found || item.ID == out.Selection.Output
	}
	if !found {
		return access.ErrNotFound
	}
	if v.State != "succeeded" && v.State != "partial" {
		return nil
	}
	value, err := s.runs.Output(ctx, e, v.ID, out.Selection.Output)
	if err != nil {
		return err
	}
	out.Output, out.PageBounds, err = s.projectOutput(value, out.Selection)
	return err
}

func (s *Delivery) viewComposition(ctx context.Context, e identity.Envelope, out *ReportingViewResult) error {
	v, err := s.compositions.Get(ctx, e, out.Selection.Run)
	if err != nil {
		return err
	}
	if v.Kind != out.Selection.Kind {
		return access.ErrNotFound
	}
	out.Summary = CompositionRunSummary(v)
	out.MixedFreshness, out.Redacted = v.MixedFreshness, v.Redacted
	if v.State == "expired" {
		return nil
	}
	out.Pages = clone(v.Pages)
	if out.Selection.Page == "" && len(v.Pages) != 0 {
		out.Selection.Page = v.Pages[0].ID
	}
	var selected *CompositionWidgetSummary
	pageFound := false
	for _, page := range v.Pages {
		if page.ID != out.Selection.Page {
			continue
		}
		pageFound = true
		if out.Selection.Widget == "" && len(page.Widgets) != 0 {
			out.Selection.Widget = page.Widgets[0].ID
		}
		for _, widget := range page.Widgets {
			if widget.ID == out.Selection.Widget {
				copy := clone(widget)
				selected = &copy
				break
			}
		}
	}
	if !pageFound && out.Selection.Page != "" || selected == nil && out.Selection.Widget != "" {
		return access.ErrNotFound
	}
	if selected == nil {
		return nil
	}
	out.Trust, out.Observed = clone(selected.Trust), clone(selected.Observed)
	if v.State != "succeeded" && v.State != "partial" {
		return nil
	}
	payload, err := s.compositions.Widget(ctx, e, v.ID, out.Selection.Page, out.Selection.Widget)
	if err != nil {
		return err
	}
	if payload.State != "succeeded" {
		out.Output = &ViewerOutput{ID: selected.ID, Kind: selected.Kind, State: payload.State, Code: payload.Code}
		return nil
	}
	if payload.Text != nil {
		if out.Selection.Output != "" || out.Selection.Offset != 0 {
			return ErrInvalid
		}
		out.Text = clone(payload.Text)
		return nil
	}
	if payload.Query != nil {
		if out.Selection.Output != "" && out.Selection.Output != "result" {
			return access.ErrNotFound
		}
		out.Selection.Output = "result"
		out.Outputs = []ViewerOutputChoice{{ID: "result", Kind: "table", Title: selected.ID}}
		result := payload.Query.Execution.Result
		if result == nil {
			return ErrIncomplete
		}
		return s.queryTable(ctx, out, *result)
	}
	for _, output := range payload.Outputs {
		out.Outputs = append(out.Outputs, ViewerOutputChoice{ID: output.ID, Kind: output.Kind, Title: output.ID})
	}
	if out.Selection.Output == "" && len(payload.Outputs) != 0 {
		out.Selection.Output = payload.Outputs[0].ID
	}
	for _, output := range payload.Outputs {
		if output.ID == out.Selection.Output {
			out.Output, out.PageBounds, err = s.projectOutput(output, out.Selection)
			return err
		}
	}
	return access.ErrNotFound
}

func (s *Delivery) queryTable(ctx context.Context, out *ReportingViewResult, retained exec.Result) error {
	bounds, end, err := tableBounds(len(retained.Rows), out.Selection.Offset, out.Selection.Limit)
	if err != nil {
		return err
	}
	// Convert only this exact page. The original retained completeness and total
	// remain explicit; an empty last page is not evidence of an empty query.
	page := retained
	page.Rows = retained.Rows[out.Selection.Offset:end]
	if len(page.Rows) == 0 && page.Outcome == "succeeded" {
		page.Outcome = "empty"
	}
	limits := charts.DefaultLimits()
	limits.MaxRows = s.limits.MaxRows
	limits.MaxBytes = s.limits.MaxMessageBytes
	data, err := chartdata.FromReadResult(ctx, page, limits)
	if err != nil {
		return err
	}
	out.Output = &ViewerOutput{ID: "result", Kind: "table", State: "succeeded", Table: &ViewerTable{
		Columns: data.Columns, Rows: data.Rows, Totals: []charts.Total{}, Completeness: data.Completeness, Warnings: []string{},
	}}
	out.PageBounds = bounds
	return nil
}
