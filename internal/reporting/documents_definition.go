package reporting

import (
	"bytes"
	"encoding/json"
	"html"
	"io"
	"slices"
	"strings"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
)

func documentKind(kind string) bool { return kind == "report" || kind == "dashboard" }

// DocumentDigest hashes canonical JSON, independent of PostgreSQL JSONB key
// ordering. It includes the authored version, never a newer read projection.
func DocumentDigest(raw json.RawMessage) string {
	value, err := documentJSON(raw)
	if err != nil {
		return ""
	}
	return digest(struct {
		Version string
		Value   any
	}{"reporting-document-v1", value})
}

func decodeDocument(raw json.RawMessage, max int) (DocumentDefinition, error) {
	var d DocumentDefinition
	if len(raw) == 0 || len(raw) > max {
		return d, ErrInvalid
	}
	if _, err := documentJSON(raw); err != nil {
		return d, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil {
		return DocumentDefinition{}, ErrInvalid
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return DocumentDefinition{}, ErrInvalid
	}
	return d, nil
}

// ProjectDocument supports one explicit historical format. Version-one sections
// are ordered stacks; titles become inert heading widgets and children retain
// section membership. The input is never mutated or re-persisted.
func ProjectDocument(raw json.RawMessage, kind string, limits config.ReportingComposition) (DocumentDefinition, error) {
	d, err := decodeDocument(raw, limits.MaxDefinitionBytes)
	if err != nil {
		return DocumentDefinition{}, err
	}
	if d.SchemaVersion == 1 {
		if kind != "report" || len(d.Widgets) != 0 || len(d.Pages) != 0 || len(d.Sections) == 0 || len(d.Sections) > limits.MaxWidgets {
			return DocumentDefinition{}, ErrInvalid
		}
		row := 0
		sections := map[string]bool{}
		for _, section := range d.Sections {
			if !identity.Identifier(section.ID) || sections[section.ID] || !text(section.Title, 256) || len(section.Widgets) == 0 {
				return DocumentDefinition{}, ErrInvalid
			}
			sections[section.ID] = true
			if section.Title != "" {
				d.Widgets = append(d.Widgets, Widget{ID: "section." + section.ID, Kind: "text", Section: section.ID,
					Grid: GridCell{Row: row, Width: 12, Height: 1}, Text: &TextWidget{Format: "plain", Text: section.Title}})
				row++
			}
			for _, original := range section.Widgets {
				if original.Grid != (GridCell{}) || original.Section != "" || len(d.Widgets) >= limits.MaxWidgets {
					return DocumentDefinition{}, ErrInvalid
				}
				w := clone(original)
				w.Grid = GridCell{Row: row, Width: 12, Height: 1}
				w.Section = section.ID
				d.Widgets = append(d.Widgets, w)
				row++
			}
		}
		d.Sections = nil
		d.SchemaVersion = DocumentVersion
	}
	if err := ValidateDocument(kind, d, limits, true); err != nil {
		return DocumentDefinition{}, err
	}
	return d, nil
}

func validDocumentMetadata(d DocumentDefinition) bool {
	if len(d.Metadata) == 0 || len(d.Metadata) > 16 || !locale(d.Locale) || len(d.Audience) > 32 {
		return false
	}
	seen, selected := map[string]bool{}, false
	for _, m := range d.Metadata {
		if !locale(m.Locale) || seen[m.Locale] || strings.TrimSpace(m.Title) == "" || !text(m.Title, 256) || !text(m.Description, 4096) {
			return false
		}
		seen[m.Locale] = true
		selected = selected || m.Locale == d.Locale
	}
	labels := map[string]bool{}
	for _, label := range d.Audience {
		if strings.TrimSpace(label) == "" || !text(label, 128) || labels[label] {
			return false
		}
		labels[label] = true
	}
	_, err := namedZone(d.Timezone)
	return selected && err == nil
}

func safeText(w TextWidget, max int) bool {
	if !slices.Contains([]string{"plain", "markdown"}, w.Format) || !text(w.Text, max) {
		return false
	}
	if w.Format == "plain" {
		return true
	}
	// An inert Markdown subset, not a blacklist of script names. Raw HTML,
	// links, images, entity-obfuscated markup and autolinks are excluded.
	// Emphasis, headings, lists and code remain ordinary retained source text.
	return !strings.ContainsAny(html.UnescapeString(w.Text), "<>[]")
}

func validPresentation(p Presentation) bool {
	return text(p.Title, 256) && text(p.Subtitle, 1024) && slices.Contains([]string{"", "comfortable", "compact"}, p.Density)
}

func validGrid(g GridCell) bool {
	return g.Column >= 0 && g.Column < 12 && g.Row >= 0 && g.Row < 10000 && g.Width >= 1 && g.Width <= 12-g.Column && g.Height >= 1 && g.Height <= 100 && g.Row+g.Height <= 10000
}

func overlapping(a, b GridCell) bool {
	return a.Column < b.Column+b.Width && b.Column < a.Column+a.Width && a.Row < b.Row+b.Height && b.Row < a.Row+a.Height
}

func validWidget(w Widget, limits config.ReportingComposition) bool {
	if !identity.Identifier(w.ID) || !validGrid(w.Grid) || !validPresentation(w.Presentation) || w.Section != "" && !identity.Identifier(w.Section) || len(w.Literals) > 64 || len(w.Bindings) > 64 || len(w.Overrides) > 64 {
		return false
	}
	switch w.Kind {
	case "text":
		return w.Text != nil && w.Block == nil && w.Query == nil && len(w.Literals)+len(w.Bindings)+len(w.Overrides) == 0 && safeText(*w.Text, limits.MaxTextBytes)
	case "query":
		if w.Query == nil || w.Block != nil || w.Text != nil || len(w.Literals)+len(w.Bindings)+len(w.Overrides) != 0 {
			return false
		}
		q := w.Query
		if !identity.Identifier(q.Context) || len(q.Topics) == 0 || len(q.Topics) > 4 {
			return false
		}
		seen := map[string]bool{}
		for _, pin := range q.Topics {
			if !identity.Identifier(pin.Topic) || !identity.Identifier(pin.Version) || !hashValid(pin.Digest) || seen[pin.Topic] {
				return false
			}
			seen[pin.Topic] = true
		}
		return q.Durability == "replayable" && q.Query == "" && strings.TrimSpace(q.Question) != "" && text(q.Question, 4096) ||
			q.Durability == "session_bound" && q.Question == "" && identity.Identifier(q.Query)
	case "block":
		if w.Block == nil || w.Query != nil || w.Text != nil || !identity.Identifier(w.Block.Block) || w.Block.Revision < 0 || w.Block.Revision > 256 || len(w.Block.Outputs) > 64 || !slices.Contains([]string{"", "published", "certified_only", "explicit_stale"}, w.Block.Policy) {
			return false
		}
		seen := map[string]bool{}
		for _, output := range w.Block.Outputs {
			if !identity.Identifier(output) || seen[output] {
				return false
			}
			seen[output] = true
		}
		seen = map[string]bool{}
		for _, a := range w.Literals {
			if !identity.Identifier(a.Name) || seen[a.Name] {
				return false
			}
			seen[a.Name] = true
		}
		seen = map[string]bool{}
		for _, b := range w.Bindings {
			if !identity.Identifier(b.Filter) || !identity.Identifier(b.Parameter) || seen[b.Parameter] {
				return false
			}
			seen[b.Parameter] = true
		}
		seen = map[string]bool{}
		for _, name := range w.Overrides {
			if !identity.Identifier(name) || seen[name] {
				return false
			}
			seen[name] = true
		}
		return true
	default:
		return false
	}
}

// ValidateDocument checks bounded tagged unions, not reference existence or
// source authority. emptyPages is reserved for an authorized redacted read.
func ValidateDocument(kind string, d DocumentDefinition, limits config.ReportingComposition, emptyPages bool) error {
	if limits.Validate() != nil || !documentKind(kind) || d.SchemaVersion != DocumentVersion || len(d.Sections) != 0 || !validDocumentMetadata(d) || !slices.Contains([]string{"", "fail_closed", "allow_partial"}, d.PartialFailure) {
		return ErrInvalid
	}
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > limits.MaxDefinitionBytes {
		return ErrInvalid
	}
	if kind == "dashboard" {
		if len(d.Widgets)+len(d.Filters)+len(d.Defaults) != 0 || len(d.Pages) > limits.MaxPages || !emptyPages && len(d.Pages) == 0 {
			return ErrInvalid
		}
		seen := map[string]bool{}
		for _, page := range d.Pages {
			if !identity.Identifier(page.ID) || seen[page.ID] || !identity.Identifier(page.Report) || page.Revision < 1 || page.Revision > 256 || !text(page.Title, 256) {
				return ErrInvalid
			}
			seen[page.ID] = true
		}
		return nil
	}
	if len(d.Pages) != 0 || len(d.Widgets) == 0 || len(d.Widgets) > limits.MaxWidgets || len(d.Filters) > limits.MaxFilters || len(d.Defaults) > 64 {
		return ErrInvalid
	}
	filters, used := map[string]Parameter{}, map[string]bool{}
	for _, filter := range d.Filters {
		p := filter.Parameter
		if filters[p.Name].Name != "" || validateDeclarations([]Parameter{p}, 1) != nil || !text(filter.Label, 256) {
			return ErrInvalid
		}
		filters[p.Name] = p
	}
	seen := map[string]bool{}
	for _, a := range d.Defaults {
		if !identity.Identifier(a.Name) || seen[a.Name] {
			return ErrInvalid
		}
		seen[a.Name] = true
	}
	seen = map[string]bool{}
	for i, widget := range d.Widgets {
		if !validWidget(widget, limits) || seen[widget.ID] {
			return ErrInvalid
		}
		seen[widget.ID] = true
		for _, previous := range d.Widgets[:i] {
			if overlapping(previous.Grid, widget.Grid) {
				return ErrInvalid
			}
		}
		for _, binding := range widget.Bindings {
			if filters[binding.Filter].Name == "" {
				return ErrInvalid
			}
			used[binding.Filter] = true
		}
	}
	for name := range filters {
		if !used[name] {
			return ErrInvalid
		}
	}
	return nil
}
