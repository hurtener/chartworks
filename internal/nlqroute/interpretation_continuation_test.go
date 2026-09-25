package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestSQLRecoveryInterpretationContinuationKeepsExactState(t *testing.T) {
	for _, tc := range []struct {
		locale      nlq.Language
		first, next string
	}{{nlq.LanguageEnglish, "Revenue in north in March 2025", "Show the records"}, {nlq.LanguageSpanish, "Ingresos en norte en marzo de 2025", "Mostrar los registros"}} {
		t.Run(string(tc.locale), func(t *testing.T) {
			svc, engine := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
			e := testEnvelope(t, true)
			original, err := svc.Route(context.Background(), e, RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.first, InterpretationAnchor: "2026-09-22"})
			if err != nil || original.Interpretation == nil {
				t.Fatal("original", err)
			}
			before, _ := json.Marshal(original)
			seed := RetainedInterpretationSelections(original.Interpretation)
			child, err := svc.Route(context.Background(), e, RouteRequest{Topic: "topic", Context: "ctx", Locale: tc.locale, Question: tc.next, InterpretationAnchor: original.Interpretation.Anchor, InterpretationSelections: seed})
			if err != nil || child.Interpretation == nil || len(child.Interpretation.Values) != 1 || len(child.Interpretation.Temporal) != 1 {
				t.Fatal("inherited", err)
			}
			if child.Interpretation.Values[0].CanonicalValue != "NORTH" || child.Interpretation.Temporal[0].Start != "2025-03-01" || child.Interpretation.Temporal[0].End != "2025-04-01" || child.Interpretation.Anchor != "2026-09-22" {
				t.Fatal("retained value/anchor/interval changed")
			}
			c, err := child.ResolvedBusinessConstraints()
			if err != nil || len(c) != 2 {
				t.Fatal("constraints not sealed", err)
			}
			calls := engine.embeds
			if replay, _, err := svc.ReplayClarifications(context.Background(), e, child); err != nil || readexec.Hash(replay) != readexec.Hash(c) || engine.embeds != calls {
				t.Fatal("replay not exact and model-free", err)
			}
			seed[0].Topic = "mutated"
			after, _ := json.Marshal(original)
			if string(before) != string(after) {
				t.Fatal("parent alias mutated")
			}
		})
	}
}
func TestSQLRecoveryInterpretationContinuationReplacesAndRemoves(t *testing.T) {
	p := cw07Publication("topic")
	p.Definition.Dimensions[1].Temporal.Grains = append(p.Definition.Dimensions[1].Temporal.Grains, semantics.GrainQuarter)
	svc, _ := cw07Service(t, p, cw07Binding(1))
	e := testEnvelope(t, true)
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue in north last month", InterpretationAnchor: "2026-09-22"}
	base, err := svc.Route(context.Background(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	in.Question = "Now south last quarter"
	in.InterpretationSelections = RetainedInterpretationSelections(base.Interpretation)
	out, err := svc.Route(context.Background(), e, in)
	if err != nil || out.Interpretation == nil {
		t.Fatal(err)
	}
	if len(out.Interpretation.Values) != 1 || out.Interpretation.Values[0].CanonicalValue != "SOUTH" || len(out.Interpretation.Temporal) != 1 || out.Interpretation.Temporal[0].Start != "2026-04-01" || out.Interpretation.Temporal[0].End != "2026-07-01" {
		t.Fatal("new question failed to replace previous dimension/period")
	}
	in.Question = "Keep the same report"
	in.InterpretationEdits = []InterpretationEdit{{Target: "topic:sales_region:north", Action: "replace", Value: "south"}, {Target: "topic:event_date:time", Action: "replace", Period: &InterpretationPeriod{Start: "2025-01-01", End: "2025-04-01", Grain: "quarter"}}}
	out, err = svc.Route(context.Background(), e, in)
	if err != nil || out.Interpretation == nil {
		t.Fatal(err)
	}
	if out.Interpretation.Values[0].CanonicalValue != "SOUTH" || out.Interpretation.Temporal[0].Start != "2025-01-01" {
		t.Fatal("explicit replacement ignored")
	}
	in.InterpretationEdits = []InterpretationEdit{{Target: "topic:sales_region:north", Action: "remove"}, {Target: "topic:event_date:time", Action: "remove"}}
	out, err = svc.Route(context.Background(), e, in)
	if err != nil || out.Interpretation == nil || len(out.Interpretation.Values)+len(out.Interpretation.Temporal) != 0 {
		t.Fatal("explicit removal ignored", err)
	}
}
func TestSQLRecoveryInterpretationContinuationRejectsForeignState(t *testing.T) {
	for _, change := range []func(*RouteRequest){
		func(in *RouteRequest) { in.InterpretationSelections[0].Topic = "foreign" },
		func(in *RouteRequest) { in.InterpretationSelections[0].Dimension = "foreign" },
		func(in *RouteRequest) { in.InterpretationSelections[0].Value = "unreviewed" },
		func(in *RouteRequest) { in.InterpretationSelections[0].Operator = "or" },
		func(in *RouteRequest) {
			in.InterpretationSelections = append(in.InterpretationSelections, in.InterpretationSelections[0])
		},
		func(in *RouteRequest) { in.InterpretationSelections[1].Period.Start = "2026-09-02" },
		func(in *RouteRequest) { in.InterpretationSelections[1].Period.End = "2026-08-01" },
		func(in *RouteRequest) { in.InterpretationSelections[1].Value = "north" },
	} {
		svc, model := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
		in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Rows", InterpretationSelections: []InterpretationSelection{{Topic: "topic", Dimension: "sales_region", Value: "north", Operator: "eq"}, {Topic: "topic", Dimension: "event_date", Period: &InterpretationPeriod{Start: "2026-09-01", End: "2026-10-01", Grain: "month"}}}}
		change(&in)
		if _, err := svc.Route(context.Background(), testEnvelope(t, true), in); err == nil || model.embeds != 0 {
			t.Fatal("unreviewed/invalid retained coordinate reached model", err)
		}
	}
	p := cw07Publication("topic")
	p.Definition.Dimensions[0].Values[0].Sensitivity = semantics.LiteralSensitive
	svc, model := cw07Service(t, p, cw07Binding(1))
	if _, err := svc.Route(context.Background(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Rows", InterpretationSelections: []InterpretationSelection{{Topic: "topic", Dimension: "sales_region", Value: "north", Operator: "eq"}}}); err == nil || model.embeds != 0 {
		t.Fatal("private value ID bypassed selection review")
	}
}
func TestSQLRecoveryInterpretationContinuationReplayAndSourceFence(t *testing.T) {
	svc, model := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	e := testEnvelope(t, true)
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Rows", InterpretationSelections: []InterpretationSelection{{Topic: "topic", Dimension: "sales_region", Value: "north", Operator: "ne"}}}
	out, err := svc.Route(context.Background(), e, in)
	if err != nil {
		t.Fatal(err)
	}
	out.Request.InterpretationSelections[0].Value = "south"
	if _, _, err := svc.ReplayClarifications(context.Background(), e, out); err == nil {
		t.Fatal("tampered seed replayed original proof")
	}
	oldCalls := model.embeds
	svc.topics.(*testTopics).binding = cw07Binding(2)
	if _, err := svc.Route(context.Background(), e, in); !errors.Is(err, readexec.ErrBinding) || model.embeds != oldCalls {
		t.Fatal("changed source bypassed fence", err)
	}
}
func TestSQLRecoveryInterpretationSelectionCopiesAndBounds(t *testing.T) {
	in := []InterpretationSelection{{Topic: "topic", Dimension: "event_date", Period: &InterpretationPeriod{Start: "2026-01-01", End: "2027-01-01", Grain: "year"}}}
	before, _ := json.Marshal(in)
	out := CloneInterpretationSelections(in)
	out[0].Period.Start = "mutated"
	after, _ := json.Marshal(in)
	if string(before) != string(after) {
		t.Fatal("selection shallow copy")
	}
	edits := []InterpretationEdit{{Target: "topic:event_date:time", Action: "replace", Period: in[0].Period}}
	clone := CloneInterpretationEdits(edits)
	clone[0].Period.Grain = "mutated"
	if edits[0].Period.Grain != "year" {
		t.Fatal("edit shallow copy")
	}
	if validateInterpretationSelections(make([]InterpretationSelection, 65)) {
		t.Fatal("unbounded selections")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc, _ := cw07Service(t, cw07Publication("topic"), cw07Binding(1))
	if _, err := svc.Route(ctx, testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Rows", InterpretationSelections: in}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation", err)
	}
	if !reflect.DeepEqual(CloneInterpretationSelections(nil), []InterpretationSelection(nil)) {
		t.Fatal("legacy nil changed")
	}
}
