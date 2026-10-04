// Package docs embeds explicit public contracts from their maintained source.
// It does not expose repository instructions, arbitrary files or private notes.
package docs

import (
	"embed"
	"strconv"

	"github.com/hurtener/chartworks/internal/staticdocs"
)

//go:embed contracts/report-authoring-guide-v1.md contracts/report-app-v1.md contracts/report-pages-v3.md contracts/manual-chart-preparation-v1.md contracts/governed-authoring-options-v1.md contracts/manual-publication-lifecycle-v1.md contracts/pengui-authority.md contracts/report-filter-options-v1.md reporting/contracts.md reporting/delivery.md
var reportAuthoringFiles embed.FS

// ReportAuthoring compiles a fresh immutable allowlisted catalog. Embedding the
// actual source documents prevents a separately maintained resource-text copy.
func ReportAuthoring() (*staticdocs.Catalog, error) {
	entries := []struct {
		name, title, description, file string
		version                        int
	}{
		{"workflows", "Manual report authoring workflows", "Start here: concrete composition, chart, page, filter and publication workflows with supported limits.", "contracts/report-authoring-guide-v1.md", 1},
		{"application", "Report application contract", "Manual report app operations, host boundary, exact edit intent and current supported capabilities.", "contracts/report-app-v1.md", 1},
		{"pages", "Report-owned pages", "Version-three page identity, page-local filters, private block references, CAS and lifecycle constraints.", "contracts/report-pages-v3.md", 3},
		{"preparation", "Reviewed dataset chart preparation", "Finite manual chart compiler, deliberate preparation, private custody, creation and separate validation.", "contracts/manual-chart-preparation-v1.md", 1},
		{"options", "Governed authoring option search", "Explicit Search, typed filter targets, original-attempt recovery and lost-value handling.", "contracts/governed-authoring-options-v1.md", 1},
		{"publication", "Manual publication lifecycle", "Whole-revision disclosure, independently authorized publication, selected rebind and report transitions.", "contracts/manual-publication-lifecycle-v1.md", 1},
		{"authority", "Pengui authority contract", "Native signed action, exact target and dependency enforcement; documentation supplies no authority.", "contracts/pengui-authority.md", 1},
		{"filters", "Report filter option contract", "Saved filter bindings, typed option populations, cursors and deliberate source execution constraints.", "contracts/report-filter-options-v1.md", 1},
		{"reporting", "Governed reporting contracts", "Blocks, reports, immutable revisions, parameters, artifacts and publication contract vocabulary.", "reporting/contracts.md", 1},
		{"delivery", "Reporting delivery contract", "Retained reporting delivery, viewer, explicit execution, private preview and rendering boundaries.", "reporting/delivery.md", 1},
	}
	documents := make([]staticdocs.Document, 0, len(entries))
	for _, entry := range entries {
		text, err := reportAuthoringFiles.ReadFile(entry.file)
		if err != nil {
			return nil, err
		}
		version := strconv.Itoa(entry.version)
		documents = append(documents, staticdocs.Document{Reference: staticdocs.Reference{URI: "chartworks://report_app/docs/" + entry.name + "/v" + version, Name: entry.title, Description: entry.description, MIMEType: staticdocs.MIME, Version: entry.version, DocumentRef: "docs/" + entry.file}, Text: string(text)})
	}
	return staticdocs.New("reporting.read", documents)
}
