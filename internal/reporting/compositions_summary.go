package reporting

import "encoding/json"

// SummarizeComposition projects only bounded metadata. Normalized rows, query
// SQL, raw parameter literals and retained narrative text are not included.
func SummarizeComposition(record CompositionRecord) CompositionView {
	m := record.Manifest
	view := CompositionView{ID: m.ID, Kind: m.Kind, Document: m.Document, Revision: m.Revision,
		Manifest: m.ManifestDigest(), State: record.State, Code: record.Code, Private: m.Private,
		Redacted: m.Redacted, Created: m.Created, Expires: m.Expires, Finished: clone(record.Finished),
		Pages: []CompositionPageSummary{}, QueryGroups: len(m.Groups), RetainedBytes: CompositionRetainedBytes(record)}
	results := map[string]GroupResult{}
	for _, result := range record.Results {
		results[result.Group] = result
	}
	groups := map[string]CompositionGroup{}
	for _, group := range m.Groups {
		groups[group.ID] = group
	}
	for _, saved := range m.Pages {
		page := CompositionPageSummary{ID: saved.ID, Report: saved.Report, Revision: saved.Revision, Title: saved.Title, Widgets: []CompositionWidgetSummary{}}
		for _, widget := range saved.Widgets {
			d := widget.Definition
			w := CompositionWidgetSummary{ID: d.ID, Kind: d.Kind, State: "pending", Grid: d.Grid,
				Presentation: clone(d.Presentation), Parameters: clone(widget.Parameters), Outputs: []string{}}
			if d.Block != nil {
				w.Outputs = clone(d.Block.Outputs)
			}
			if d.Query != nil {
				w.Durability = d.Query.Durability
			}
			if d.Kind == "text" {
				w.State = "completed"
			} else if widget.Code != "" {
				w.State, w.Code = "failed", widget.Code
			} else {
				group := groups[widget.Group]
				w.Trust = clone(group.Trust)
				if result, exists := results[widget.Group]; exists {
					w.State, w.Code = compositionSelectionState(result, w.Outputs)
					w.Observed = clone(result.Observed)
					if result.Block != nil {
						w.Trust = clone(&result.Block.Trust)
					}
					if result.Query != nil {
						w.Trust = nil
						w.QueryDigest, w.SemanticDigest = result.Query.QueryDigest, result.Query.SemanticDigest
					}
				}
			}
			page.Widgets = append(page.Widgets, w)
		}
		view.Pages = append(view.Pages, page)
	}
	if state, _, mixed, err := CompositionCompletion(m, record.Results); err == nil {
		view.Complete = record.State == "completed" && state == "completed"
		view.MixedFreshness = mixed
	}
	return view
}

// CompositionResultJSON returns the bounded immutable checkpoint representation.
func CompositionResultJSON(result GroupResult) ([]byte, error) {
	body, err := json.Marshal(result)
	if err != nil || len(body) > 16<<20 {
		return nil, ErrBudget
	}
	return body, nil
}

// DecodeCompositionResult rejects corrupt or substituted group payloads.
func DecodeCompositionResult(body []byte, manifest CompositionManifest, group string) (GroupResult, error) {
	var result GroupResult
	g, ok := compositionGroup(manifest, group)
	if !ok || len(body) == 0 || len(body) > 16<<20 || json.Unmarshal(body, &result) != nil || CheckCompositionResult(manifest, g, result) != nil {
		return GroupResult{}, ErrInvalid
	}
	return result, nil
}

// DecodeCompositionManifest validates content; it does not issue authority.
func DecodeCompositionManifest(body []byte, expected string) (CompositionManifest, error) {
	var m CompositionManifest
	if len(body) == 0 || len(body) > 16<<20 || json.Unmarshal(body, &m) != nil || !validComposition(m) || m.ManifestDigest() != expected {
		return CompositionManifest{}, ErrInvalid
	}
	return m, nil
}
