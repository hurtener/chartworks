package semantics

import (
	"reflect"
	"strings"
	"testing"
)

func cw01NumberSlot() ClarificationSlot {
	return ClarificationSlot{ID: "amount", Prompt: "What minimum amount?", Required: true, Kind: SlotNumber, Sensitivity: LiteralNonSensitive, Effect: &ClarificationEffect{Kind: "number", Target: Reference{Kind: KindColumn, Dataset: "orders", ID: "amount"}, Operator: "gte", Nulls: "exclude", Unit: "USD", Precision: 38, Scale: 6}}
}

func cw01TimeSlot() ClarificationSlot {
	return ClarificationSlot{ID: "period", Prompt: "Which period?", PromptES: "¿Qué período?", Required: true, Kind: SlotDate, Sensitivity: LiteralNonSensitive, Effect: &ClarificationEffect{Kind: "time_window", Target: Reference{Kind: KindColumn, Dataset: "orders", ID: "created"}, Operator: "range", Nulls: "exclude", Bounds: "[)", Calendar: "gregorian", TimeZone: "America/Argentina/Buenos_Aires", TemporalType: "timestamptz", Grains: []string{"month"}}}
}

func cw01Definition(t *testing.T, slots ...ClarificationSlot) (RuleSubject, RuleSetDefinition) {
	t.Helper()
	pack := TopicPack{SchemaVersion: SchemaVersion, Topic: "orders_topic", Version: "topic_v1", Datasets: []Dataset{{ID: "orders", Name: "Orders", Source: SourceReference{Source: "source1", Context: "context1:v1", Dataset: "public.orders", SourceRevision: 1}, Columns: []Column{{ID: "amount", SourceName: "net_amount", Name: "Amount", NativeType: "numeric", Category: "numeric"}, {ID: "created", SourceName: "created_at", Name: "Created", NativeType: "timestamptz", Category: "temporal"}, {ID: "region", SourceName: "region_code", Name: "Region", NativeType: "text", Category: "text"}, {ID: "active", SourceName: "is_active", Name: "Active", NativeType: "boolean", Category: "boolean"}}}}}
	digest := strings.Repeat("a", 64)
	subject, err := NewRuleSubject(pack, digest)
	if err != nil {
		t.Fatal(err)
	}
	targets := []Reference{}
	for _, slot := range slots {
		if slot.Effect != nil && !hasReference(targets, slot.Effect.Target) {
			targets = append(targets, slot.Effect.Target)
		}
		for _, choice := range slot.Choices {
			if choice.Target != nil && !hasReference(targets, *choice.Target) {
				targets = append(targets, *choice.Target)
			}
		}
	}
	return subject, RuleSetDefinition{SchemaVersion: SchemaVersion, ID: "rules1", Version: "rules_v1", Topic: pack.Topic, TopicVersion: pack.Version, PackDigest: digest, Patterns: []ClarificationPattern{{ID: "order_period", Version: "pattern_v1", Targets: targets, Provenance: RuleProvenance{Kind: ProvenanceHuman, Evidence: "review1"}, Policy: &ClarificationPolicy{SchemaVersion: ClarificationSchemaVersion, When: ClarificationWhen{AnyTerms: []string{"orders", "pedidos"}}, Why: "The selected period changes which orders are counted.", WhySpanish: "El período elegido cambia qué pedidos se cuentan."}, Slots: slots}}}
}

func TestClarificationTypedValues(t *testing.T) {
	t.Run("BilingualMonthBounds", func(t *testing.T) {
		slot := cw01TimeSlot()
		parse := func(period, locale string) ClarificationResolution {
			r, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{Period: period, Calendar: "gregorian", TimeZone: slot.Effect.TimeZone, Grain: "month"}}, locale)
			if err != nil {
				t.Fatal(err)
			}
			return r
		}
		en, es := parse("January 2026", "en"), parse("enero de 2026", "es")
		if !reflect.DeepEqual(en.Time, es.Time) || !reflect.DeepEqual(en.Effect, es.Effect) || en.Locale != "en" || es.Locale != "es" || en.ParserVersion != es.ParserVersion || en.Time.StartUTC != "2026-01-01T03:00:00Z" || en.Time.EndUTC != "2026-02-01T03:00:00Z" || en.Time.Bounds != "[)" {
			t.Fatalf("noncanonical bilingual bounds: %+v %+v", en.Time, es.Time)
		}
	})
	t.Run("ExactDecimal", func(t *testing.T) {
		slot := cw01NumberSlot()
		for _, tc := range []struct{ value, locale, want string }{{"9007199254740993.125000", "en", "9007199254740993.125"}, {"9007199254740993,125", "es", "9007199254740993.125"}, {"-0.000", "en", "0"}, {"00012.3400", "en", "12.34"}} {
			r, err := ResolveClarificationValue(slot, ClarificationValue{Number: &ClarificationNumberInput{Value: tc.value, Unit: "USD"}}, tc.locale)
			if err != nil || r.Value != tc.want {
				t.Fatalf("exact decimal %q: %q %v", tc.value, r.Value, err)
			}
		}
		for _, value := range []string{"NaN", "Infinity", "1e3", "1,234", "1.0000001", "1..2", ".5", "12.", "1/2", "", strings.Repeat("9", 80)} {
			if _, err := ResolveClarificationValue(slot, ClarificationValue{Number: &ClarificationNumberInput{Value: value, Unit: "USD"}}, "en"); err == nil {
				t.Fatalf("accepted invalid number %q", value)
			}
		}
		if _, err := ResolveClarificationValue(slot, ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "EUR"}}, "en"); err == nil || err.Code != "unit_mismatch" {
			t.Fatalf("unit: %v", err)
		}
	})
	t.Run("InvalidDatesAndGrains", func(t *testing.T) {
		slot := cw01TimeSlot()
		for _, tc := range []struct{ start, end, grain string }{{"2026-02-30", "2026-03-01", "month"}, {"2026-02-01", "2026-01-01", "month"}, {"2026-01-01", "2026-02-01", "hour"}, {"2026-01-02", "2026-02-01", "month"}, {"2026-01-01", "2026-01-01", "month"}} {
			_, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{Start: tc.start, End: tc.end, Grain: tc.grain, Calendar: "gregorian", TimeZone: slot.Effect.TimeZone}}, "es")
			if err == nil || err.Message == "" {
				t.Fatalf("accepted invalid dates/grain: %+v", tc)
			}
		}
		_, err := ResolveClarificationValue(slot, ClarificationValue{Time: &ClarificationTimeInput{Start: "2026-01-01", End: "2026-02-01", Grain: "month", Calendar: "gregorian", TimeZone: "UTC"}}, "en")
		if err == nil || err.Code != "calendar_mismatch" {
			t.Fatal("timezone was inferred or ignored")
		}
	})
	t.Run("BooleanAndGovernedText", func(t *testing.T) {
		slot := ClarificationSlot{Kind: SlotBoolean, Sensitivity: LiteralSensitive, Effect: &ClarificationEffect{Kind: "boolean", Operator: "eq", Nulls: "exclude"}}
		for _, tc := range []struct{ value, locale, want string }{{"yes", "en", "true"}, {"sí", "es", "true"}, {"falso", "es", "false"}, {"false", "en", "false"}} {
			r, err := ResolveClarificationValue(slot, ClarificationValue{Boolean: &tc.value}, tc.locale)
			if err != nil || r.Value != tc.want || r.Sensitivity != LiteralSensitive {
				t.Fatalf("boolean: %+v %v", r, err)
			}
		}
		bad := "truthy"
		if _, err := ResolveClarificationValue(slot, ClarificationValue{Boolean: &bad}, "en"); err == nil {
			t.Fatal("accepted invalid boolean")
		}
		text := ClarificationSlot{Kind: SlotText, Sensitivity: LiteralNonSensitive, Effect: &ClarificationEffect{Kind: "entity", Operator: "eq", Nulls: "exclude", MaxLength: 64, Values: []GovernedClarificationValue{{Canonical: "N-01", Label: "North region", Aliases: []string{"north", "norte"}}}}}
		for _, value := range []string{"north", "norte", "N-01"} {
			r, err := ResolveClarificationValue(text, ClarificationValue{Text: &value}, "es")
			if err != nil || r.Value != "N-01" {
				t.Fatalf("governed mapping: %+v %v", r, err)
			}
		}
		for _, value := range []string{"North region", "unknown", "'; DROP TABLE orders;--"} {
			if _, err := ResolveClarificationValue(text, ClarificationValue{Text: &value}, "en"); err == nil || err.Code != "unresolved_value" {
				t.Fatal("inferred a governed value from a label or unreviewed text")
			}
		}
	})
	t.Run("ReferenceChoicesAndUnion", func(t *testing.T) {
		ref := Reference{Kind: KindDataset, ID: "orders"}
		slot := ClarificationSlot{Kind: SlotChoice, Choices: []ClarificationChoice{{ID: "orders_option", Label: "Orders", Target: &ref}}}
		r, err := ResolveClarificationValue(slot, ClarificationValue{OptionID: "orders_option"}, "en")
		if err != nil || r.Reference == nil || *r.Reference != ref {
			t.Fatal("exact reference not preserved")
		}
		for _, value := range []ClarificationValue{{OptionID: "Orders"}, {OptionID: "foreign"}, {OptionID: "orders_option", Null: true}, {}} {
			if _, err := ResolveClarificationValue(slot, value, "en"); err == nil {
				t.Fatal("invalid option/union accepted")
			}
		}
		if _, err := ResolveClarificationValue(cw01NumberSlot(), ClarificationValue{OptionID: "12"}, "en"); err == nil {
			t.Fatal("number accepted in choice channel")
		}
	})
}

func TestClarificationPolicyCompilation(t *testing.T) {
	t.Run("CloneAndDigest", func(t *testing.T) {
		slot := cw01TimeSlot()
		subject, definition := cw01Definition(t, slot)
		model, err := CompilePublishedRules(subject, definition)
		if err != nil {
			t.Fatal(err)
		}
		before := model.Definition()
		definition.Patterns[0].Policy.When.AnyTerms[0] = "changed"
		definition.Patterns[0].Slots[0].Effect.Grains[0] = "changed"
		returned := model.Definition()
		returned.Patterns[0].Policy.When.AnyTerms[0] = "changed_again"
		returned.Patterns[0].Slots[0].Effect.Grains[0] = "changed_again"
		if !reflect.DeepEqual(before, model.Definition()) {
			t.Fatal("compiled policy aliases caller memory")
		}
		again, err := CompilePublishedRules(subject, before)
		if err != nil || again.Digest() != model.Digest() {
			t.Fatal("nondeterministic policy digest")
		}
	})
	t.Run("RejectDefaultsCyclesAndForeignTargets", func(t *testing.T) {
		mutations := []func(*RuleSetDefinition){
			func(d *RuleSetDefinition) { d.Patterns[0].Policy.When = ClarificationWhen{} },
			func(d *RuleSetDefinition) { d.Patterns[0].Policy.SchemaVersion = 999 },
			func(d *RuleSetDefinition) {
				d.Patterns[0].Slots[0].Default = &ClarificationValue{Number: &ClarificationNumberInput{Value: "1", Unit: "USD"}}
			},
			func(d *RuleSetDefinition) { d.Patterns[0].Slots[0].DependsOn = []string{"amount"} },
			func(d *RuleSetDefinition) { d.Patterns[0].Slots[0].Effect.Target.ID = "foreign" },
			func(d *RuleSetDefinition) { d.Patterns[0].Slots[0].Effect.Operator = "execute" },
			func(d *RuleSetDefinition) { d.Patterns[0].Slots[0].Effect.Nulls = "unspecified" },
			func(d *RuleSetDefinition) { d.Patterns[0].Policy = nil },
		}
		for i, mutate := range mutations {
			subject, definition := cw01Definition(t, cw01NumberSlot())
			mutate(&definition)
			if _, err := CompilePublishedRules(subject, definition); err == nil {
				t.Fatalf("accepted invalid policy mutation %d", i)
			}
		}
	})
	t.Run("VisibleOptionalDefault", func(t *testing.T) {
		slot := cw01NumberSlot()
		slot.Required = false
		slot.Default = &ClarificationValue{Number: &ClarificationNumberInput{Value: "10", Unit: "USD"}}
		subject, definition := cw01Definition(t, slot)
		model, err := CompilePublishedRules(subject, definition)
		if err != nil || model.Definition().Patterns[0].Slots[0].Default.Number.Value != "10" {
			t.Fatal("reviewed optional default lost")
		}
	})
}

func FuzzClarificationDecimal(f *testing.F) {
	for _, seed := range []string{"1", "-0", "0.125", "9007199254740993.01", "NaN", "1,25", "1e999", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, value string) {
		canonical, err := canonicalClarificationDecimal(value, "en", 38, 6)
		if err != nil {
			return
		}
		again, err := canonicalClarificationDecimal(canonical, "en", 38, 6)
		if err != nil || again != canonical {
			t.Fatalf("canonical decimal is not idempotent: %q", canonical)
		}
	})
}
