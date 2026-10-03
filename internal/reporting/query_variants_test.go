package reporting

import (
	"context"
	"encoding/json"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"strings"
	"testing"
	"time"
)

func TestCapturedQueryVariantPinsAndLowering(t *testing.T) {
	now := time.Now()
	d := contractDefinition()
	p := &QueryVariantReference{Block: "captured", Revision: 2, Digest: strings.Repeat("a", 64), CaptureDigest: strings.Repeat("b", 64)}
	snapshot := Snapshot{State: State{ID: p.Block}, Revision: Revision{Number: 2, Digest: p.Digest, Definition: d, Provenance: Provenance{Kind: "parameterize", Query: "private-query", CaptureDigest: p.CaptureDigest}}, PublishedAt: &now, Current: true}
	if err := CheckCapturedQueryVariant(p, snapshot); err != nil {
		t.Fatal(err)
	}
	w := Widget{ID: "view", Kind: "query", Grid: GridCell{Width: 12, Height: 1}, Query: &QueryWidget{Durability: "captured_variant", Variant: p}, Bindings: []FilterBinding{{Filter: "minimum", Parameter: "minimum"}}}
	if !validWidget(w, config.DefaultReportingComposition()) {
		t.Fatal("valid variant rejected")
	}
	lowered, err := CapturedVariantBlock(w)
	if err != nil || lowered.Kind != "block" || lowered.Query != nil || lowered.Block.Revision != 2 || w.Query == nil {
		t.Fatal("unsafe lowering", err)
	}
	lowered.Bindings[0].Parameter = "other"
	if w.Bindings[0].Parameter != "minimum" {
		t.Fatal("input was aliased")
	}
	empty := clone(p)
	empty.Outputs = []string{}
	if CheckCapturedQueryVariant(empty, snapshot) == nil {
		t.Fatal("empty selection became defaults")
	}
	for _, mutate := range []func(*Snapshot){
		func(s *Snapshot) { s.PublishedAt = nil }, func(s *Snapshot) { s.State.Archived = true }, func(s *Snapshot) { s.Revision.Digest = strings.Repeat("c", 64) }, func(s *Snapshot) { s.Revision.Number = 3 }, func(s *Snapshot) { s.Current = false }, func(s *Snapshot) { s.Revision.Provenance.CaptureDigest = "" },
	} {
		s := clone(snapshot)
		mutate(&s)
		if CheckCapturedQueryVariant(p, s) == nil {
			t.Fatal("invalid provenance accepted")
		}
	}
	for _, mutate := range []func(*Widget){
		func(w *Widget) { w.Query.Question = "invent another filter" }, func(w *Widget) { w.Query.Context = "another" }, func(w *Widget) { w.Query.Variant.Revision = 0 }, func(w *Widget) { w.Query.Durability = "replayable" }, func(w *Widget) { w.Block = &BlockWidget{Block: "other"} },
	} {
		bad := clone(w)
		mutate(&bad)
		if validWidget(bad, config.DefaultReportingComposition()) {
			t.Fatal("open variant contract")
		}
	}
	raw, _ := json.Marshal(QueryVariantDescriptor{Query: *w.Query})
	if strings.Contains(string(raw), "private-query") {
		t.Fatal("private lineage disclosed")
	}
	base := CompositionGroup{Kind: "block", Block: p.Block, Revision: p.Revision}
	plain := groupIdentity(base)
	base.Variant = p
	if groupIdentity(base) == plain {
		t.Fatal("variant and ordinary execution identity collapsed")
	}
	var delivery *Delivery
	if _, err = delivery.PrepareQueryVariant(context.Background(), identity.Envelope{}, QueryVariantRequest{}); err == nil {
		t.Fatal("unmounted service accepted")
	}
}

func TestCapturedQueryVariantSchedulePins(t *testing.T) {
	d := documentFixture()
	pin := &QueryVariantReference{Block: "captured", Revision: 2, Digest: strings.Repeat("a", 64), CaptureDigest: strings.Repeat("b", 64)}
	d.Widgets = append(d.Widgets, Widget{ID: "view", Kind: "query", Grid: GridCell{Row: 1, Width: 12, Height: 1}, Query: &QueryWidget{Durability: "captured_variant", Variant: pin}})
	dispatch := &jobs.ReportingDispatch{Target: jobs.ReportingTarget{Type: "report", ID: "report", Locale: d.Locale, Timezone: d.Timezone}, Pins: []jobs.ReportingPin{{ID: "view", Block: pin.Block, Revision: 2, Digest: pin.Digest}}}
	out, err := scheduledDefinition(d, dispatch)
	if err != nil || len(out.Widgets) != 2 || !IsCapturedQueryVariant(out.Widgets[1]) {
		t.Fatal("frozen variant acquired dynamic schedule requirement", err)
	}
	for _, mutate := range []func(*jobs.ReportingDispatch){
		func(j *jobs.ReportingDispatch) { j.Pins = nil }, func(j *jobs.ReportingDispatch) { j.Pins[0].Digest = strings.Repeat("c", 64) }, func(j *jobs.ReportingDispatch) { j.Pins[0].Revision = 3 }, func(j *jobs.ReportingDispatch) { j.Target.Type = "saved_question"; j.Target.Widget = "view" },
	} {
		bad := clone(dispatch)
		mutate(bad)
		if _, err := scheduledDefinition(d, bad); err == nil {
			t.Fatal("stale or wrong schedule target admitted")
		}
	}
}
