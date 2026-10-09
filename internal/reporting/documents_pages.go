package reporting

// ReportCanvas is a detached effective single-page projection. It is never a
// separately stored report and grants no authority. Historical definitions keep
// their original bytes, schema version and digest.
type ReportCanvas struct {
	ID         string
	Title      string
	Definition DocumentDefinition
}

// ReportCanvases projects validated reports. Version two retains exactly main;
// version three uses its authored stable page coordinates and page-local state.
func ReportCanvases(d DocumentDefinition) []ReportCanvas {
	if d.SchemaVersion == DocumentVersion {
		return []ReportCanvas{{ID: "main", Title: documentTitle(d), Definition: clone(d)}}
	}
	if d.SchemaVersion != PagedDocumentVersion {
		return nil
	}
	out := make([]ReportCanvas, 0, len(d.ReportPages))
	for _, p := range d.ReportPages {
		locale, zone := p.Locale, p.Timezone
		if locale == "" {
			locale = d.Locale
		}
		if zone == "" {
			zone = d.Timezone
		}
		leaf := DocumentDefinition{SchemaVersion: DocumentVersion, Metadata: []DocumentMetadata{{Locale: locale, Title: p.Title}}, Locale: locale, Timezone: zone, PartialFailure: d.PartialFailure, Widgets: clone(p.Widgets), Filters: clone(p.Filters), Defaults: clone(p.Defaults)}
		out = append(out, ReportCanvas{ID: p.ID, Title: p.Title, Definition: leaf})
	}
	return out
}

// SelectReportCanvas does not infer a page for a paged report. A missing page is
// a backwards-compatible main selector only for the historical flat format.
func SelectReportCanvas(d DocumentDefinition, page string) (ReportCanvas, error) {
	if page == "" && d.SchemaVersion == DocumentVersion {
		page = "main"
	}
	for _, canvas := range ReportCanvases(d) {
		if canvas.ID == page {
			return canvas, nil
		}
	}
	return ReportCanvas{}, ErrInvalid
}

// DashboardCanvasID preserves historical dashboard coordinates and gives every
// nested inline canvas a collision-resistant, bounded projection identity.
func DashboardCanvasID(container string, d DocumentDefinition, page string) string {
	if d.SchemaVersion == DocumentVersion && page == "main" {
		return container
	}
	return "page-" + digest([]string{"dashboard-canvas-v1", container, page})
}
