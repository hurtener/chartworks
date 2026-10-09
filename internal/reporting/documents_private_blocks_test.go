package reporting

import (
	"errors"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"strings"
	"testing"
	"time"
)

func TestPrivateDocumentBlockReferenceCustody(t *testing.T) {
	e := authoringEnvelope(t, "reporting.read", "reporting.preview", "cw.block.read:private-block", "cw.block.preview:private-block")
	w := BlockWidget{Block: "private-block", Revision: 1, Digest: strings.Repeat("a", 64), Policy: "private_preview", Outputs: []string{"table"}}
	snapshot := Snapshot{State: State{ID: w.Block}, Revision: Revision{Number: 1, Digest: w.Digest, Actor: e.User()}}
	if err := CheckDocumentBlockReference(e, w, snapshot); err != nil {
		t.Fatal("metadata save wrongly requires data validation", err)
	}
	for name, mutate := range map[string]func(*BlockWidget, *Snapshot){
		"digest":          func(w *BlockWidget, s *Snapshot) { w.Digest = strings.Repeat("b", 64) },
		"revision":        func(w *BlockWidget, s *Snapshot) { w.Revision = 2 },
		"floating":        func(w *BlockWidget, s *Snapshot) { w.Revision = 0 },
		"different block": func(w *BlockWidget, s *Snapshot) { w.Block = "other" },
		"actor":           func(w *BlockWidget, s *Snapshot) { s.Revision.Actor = "other" },
		"archive":         func(w *BlockWidget, s *Snapshot) { s.State.Archived = true },
	} {
		t.Run(name, func(t *testing.T) {
			a, b := w, snapshot
			mutate(&a, &b)
			if CheckDocumentBlockReference(e, a, b) == nil {
				t.Fatal("private reference custody bypassed")
			}
		})
	}
	noPreview := authoringEnvelope(t, "reporting.read", "cw.block.read:private-block")
	if err := CheckDocumentBlockReference(noPreview, w, snapshot); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("read became preview authority", err)
	}
	wildcard := authoringEnvelope(t, "reporting.read", "reporting.preview", "cw.block.read:*", "cw.block.preview:private-block")
	if err := CheckDocumentBlockReference(wildcard, w, snapshot); err == nil {
		t.Fatal("wildcard private editing accepted")
	}
	now := time.Now()
	snapshot.PublishedAt = &now
	snapshot.Revision.Actor = "other"
	if err := CheckDocumentBlockReference(e, w, snapshot); err == nil {
		t.Fatal("later publication erased reference actor fence")
	}
}
func TestPrivateDocumentBlockVersionAndLifecycle(t *testing.T) {
	d := pagedDocumentFixture()
	w := Widget{ID: "private-chart", Kind: "block", Grid: GridCell{Width: 12, Height: 1}, Block: &BlockWidget{Block: "private", Revision: 1, Digest: strings.Repeat("a", 64), Policy: "private_preview", Outputs: []string{"table"}}}
	d.ReportPages[0].Widgets = []Widget{w}
	limits := config.DefaultReportingComposition()
	if err := ValidateDocument("report", d, limits, false); err != nil || !HasPrivateBlockReferences(d) {
		t.Fatal(err)
	}
	flat := documentFixture()
	flat.Widgets = []Widget{w}
	if err := ValidateDocument("report", flat, limits, false); err == nil {
		t.Fatal("private block contract admitted under v2")
	}
	d.ReportPages[0].Widgets[0].Block.Policy = "published"
	if err := ValidateDocument("report", d, limits, false); err == nil {
		t.Fatal("private digest silently reinterpreted as publication")
	}
	d.ReportPages[0].Widgets[0].Block.Digest = ""
	if err := ValidateDocument("report", d, limits, false); err != nil || HasPrivateBlockReferences(d) {
		t.Fatal(err)
	}
}
