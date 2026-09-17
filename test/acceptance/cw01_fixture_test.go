package acceptance

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
)

type cw01Fixture struct {
	*phase17Fixture
	published  topics.Published
	rules      *rulesets.Service
	query      *nlqexec.Service
	definition semantics.RuleSetDefinition
}

func newCW01Fixture(t *testing.T) *cw01Fixture {
	t.Helper()
	f, draftsService, topicService, model, pack := publicationFixture(t)
	ctx := context.Background()
	if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET name=CASE id WHEN 1 THEN 'cw-alpha-731' ELSE 'cw-beta-731' END`); err != nil {
		t.Fatal(err)
	}
	old := pack.Datasets[0]
	profile := f.profile(t, engineering.ProfileSpec{ID: "cw01-typed-profile", Source: old.Source.Source, Context: old.Source.Context, Dataset: old.ID, Columns: []string{"id", "amount", "created_at", "active", "name"}, SkipLLM: true}).Profile.Profile
	dataset := semantics.Dataset{ID: profile.Dataset, Name: "Sales", Source: semantics.SourceReference{Source: old.Source.Source, Context: old.Source.Context, Dataset: profile.Dataset, SourceRevision: profile.SourceRevision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
	for _, column := range profile.Schema {
		switch column.Name {
		case "id", "amount", "created_at", "active", "name":
		default:
			continue
		}
		dataset.Columns = append(dataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
	}
	pack.Datasets[0] = dataset
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	published := phase17PublishTopic(t, draftsService, topicService, e, pack)
	ruleService, err := rulesets.New(f.db, f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	route, err := nlqroute.New(topicService, ruleService, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	pf := &phase17Fixture{f: f, pack: pack, model: model, service: route, context: dataset.Source.Context}
	pf.e = phase18Envelope(t, pf, f.e.User(), "cw01-session", true)
	out := &cw01Fixture{phase17Fixture: pf, published: published, rules: ruleService}
	out.query, _ = newPhase18Service(t, pf)
	out.definition = cw01Definition(published)
	out.publishRules(t, out.definition, 0)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	model.mode.Store(phase18RawResponse(t, "SELECT id, amount FROM analytics.sales ORDER BY id"))
	return out
}

func cw01Definition(p topics.Published) semantics.RuleSetDefinition {
	column := func(id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: p.Definition.Datasets[0].ID, ID: id}
	}
	amount, created, active, name := column("amount"), column("created_at"), column("active"), column("name")
	number := func(required bool) semantics.ClarificationSlot {
		return semantics.ClarificationSlot{ID: "threshold", Prompt: "What minimum amount should count?", PromptES: "¿Qué importe mínimo debe contar?", Required: required, Kind: semantics.SlotNumber, Sensitivity: semantics.LiteralNonSensitive, Effect: &semantics.ClarificationEffect{Kind: "number", Target: amount, Operator: "gte", Nulls: "exclude", Unit: "USD", Precision: 30, Scale: 3}}
	}
	pattern := func(id string, terms []string, slot semantics.ClarificationSlot) semantics.ClarificationPattern {
		return semantics.ClarificationPattern{ID: id, Version: "v1", Targets: []semantics.Reference{slot.Effect.Target}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "cw01-synthetic-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: semantics.ClarificationSchemaVersion, When: semantics.ClarificationWhen{AnyTerms: terms}, Why: "This answer determines which sales are included.", WhySpanish: "Esta respuesta determina qué ventas se incluyen."}, Slots: []semantics.ClarificationSlot{slot}}
	}
	temporal := semantics.ClarificationSlot{ID: "period", Prompt: "Which date window should count?", PromptES: "¿Qué período debe contar?", Required: true, Kind: semantics.SlotDate, Sensitivity: semantics.LiteralNonSensitive, Effect: &semantics.ClarificationEffect{Kind: "time_window", Target: created, Operator: "range", Nulls: "exclude", Bounds: "[)", Calendar: "gregorian", TimeZone: "America/Argentina/Buenos_Aires", TemporalType: "timestamptz", Grains: []string{"day", "month"}}}
	flag := semantics.ClarificationSlot{ID: "active", Prompt: "Include active or inactive sales?", PromptES: "¿Ventas activas o inactivas?", Required: true, Kind: semantics.SlotBoolean, Sensitivity: semantics.LiteralNonSensitive, Effect: &semantics.ClarificationEffect{Kind: "boolean", Target: active, Operator: "eq", Nulls: "exclude"}}
	entity := semantics.ClarificationSlot{ID: "customer", Prompt: "Which reviewed customer?", PromptES: "¿Qué cliente revisado?", Required: true, Kind: semantics.SlotText, Sensitivity: semantics.LiteralSensitive, Effect: &semantics.ClarificationEffect{Kind: "entity", Target: name, Operator: "eq", Nulls: "exclude", MaxLength: 64, Values: []semantics.GovernedClarificationValue{{Canonical: "cw-alpha-731", Label: "First customer", LabelES: "Primer cliente", Aliases: []string{"first", "primero", "alias-secret-731"}}, {Canonical: "cw-beta-731", Label: "Second customer", LabelES: "Segundo cliente", Aliases: []string{"second", "segundo"}}}}}
	patterns := []semantics.ClarificationPattern{
		pattern("amount-required", []string{"large sales", "ventas grandes"}, number(true)),
		pattern("amount-optional", []string{"optional sales"}, number(false)),
		pattern("period", []string{"dated sales", "ventas fechadas"}, temporal),
		pattern("state", []string{"active sales", "ventas activas"}, flag),
		pattern("customer", []string{"named sales", "ventas por nombre"}, entity),
	}
	defaultSlot := number(false)
	defaultSlot.Default = &semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: "10", Unit: "USD"}}
	patterns = append(patterns, pattern("amount-default", []string{"default sales"}, defaultSlot))
	measure := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	patterns = append(patterns, semantics.ClarificationPattern{ID: "metric", Version: "v1", Targets: []semantics.Reference{measure, amount}, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "cw01-synthetic-review"}, Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"choose sales"}}, Why: "Selects the exact reviewed metric."}, Slots: []semantics.ClarificationSlot{{ID: "metric", Prompt: "Which measure?", Required: true, Kind: semantics.SlotChoice, Sensitivity: semantics.LiteralNonSensitive, Choices: []semantics.ClarificationChoice{{ID: "revenue-option", Label: "Revenue", Target: &measure}, {ID: "amount-option", Label: "Raw amount", Target: &amount}}}}})
	return semantics.RuleSetDefinition{SchemaVersion: semantics.SchemaVersion, ID: "cw01-rules", Version: "rules-v1", Topic: p.State.Topic, TopicVersion: p.State.Version, PackDigest: p.Digest, Patterns: patterns}
}

func (f *cw01Fixture) publishRules(t *testing.T, definition semantics.RuleSetDefinition, expected int64) {
	t.Helper()
	ctx := context.Background()
	draft, err := f.rules.Save(ctx, f.e, rulesets.SaveRequest{Expected: expected, Definition: definition, Change: "Reviewed synthetic clarification policy"})
	if err != nil {
		t.Fatal("save policy", err)
	}
	review, err := f.rules.Review(ctx, f.e, definition.Topic, rulesets.ReviewRequest{DraftRevision: draft.Revision, Digest: draft.Digest, Decision: "approve", Note: "Reviewed synthetic clarification effect"})
	if err != nil {
		t.Fatal("review policy", err)
	}
	if _, err = f.rules.Publish(ctx, f.e, definition.Topic, rulesets.PublishRequest{Review: review.ID, Expected: expected}); err != nil {
		t.Fatal("publish policy", err)
	}
}

func (f *cw01Fixture) question(text string, locale nlq.Language) nlqexec.QuestionRequest {
	return nlqexec.QuestionRequest{Topic: f.pack.Topic, Context: f.context, Locale: locale, Question: text, Kinds: []string{"measure"}, LimitPerKind: 1}
}

func (f *cw01Fixture) preflight(t *testing.T, q nlqexec.QuestionRequest) nlqexec.PreflightResult {
	t.Helper()
	out, err := f.query.Preflight(context.Background(), f.e, nlqexec.PreflightRequest{QuestionRequest: q})
	if err != nil || out.QueryID == "" {
		t.Fatalf("preflight: %v", err)
	}
	return out
}

func (f *cw01Fixture) answer(t *testing.T, pattern string, value semantics.ClarificationValue) semantics.ClarificationAnswer {
	t.Helper()
	for _, p := range f.definition.Patterns {
		if p.ID == pattern {
			return semantics.ClarificationAnswer{Topic: f.pack.Topic, TopicVersion: f.published.State.Version, RulesetVersion: f.definition.Version, Pattern: p.ID, PatternVersion: p.Version, Slot: p.Slots[0].ID, Value: &value}
		}
	}
	t.Fatal("fixture policy missing")
	return semantics.ClarificationAnswer{}
}

func (f *cw01Fixture) plan(t *testing.T, q nlqexec.QuestionRequest, pattern string, value semantics.ClarificationValue) nlqexec.PlanResult {
	t.Helper()
	pending := f.preflight(t, q)
	q.AnswerContext = pending.Route.AnswerContext
	q.ClarificationQuery = pending.QueryID
	q.Answers = []semantics.ClarificationAnswer{f.answer(t, pattern, value)}
	out, err := f.query.Plan(context.Background(), f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil || out.QueryID == "" || out.Bindings == nil {
		t.Fatalf("typed plan: %v", err)
	}
	return out
}

func (f *cw01Fixture) run(t *testing.T, plan nlqexec.PlanResult, rows int, large bool) nlqexec.RunResult {
	t.Helper()
	out, err := f.query.Run(context.Background(), f.e, nlqexec.RunRequest{QueryID: plan.QueryID, Operation: plan.QueryID + "-run"})
	if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != rows {
		t.Fatalf("constraint result: expected %d rows: %v", rows, err)
	}
	raw, _ := json.Marshal(out.Execution.Result.Rows)
	contains := false
	for i := 0; i+len("9007199254740993.125") <= len(raw); i++ {
		if string(raw[i:i+len("9007199254740993.125")]) == "9007199254740993.125" {
			contains = true
			break
		}
	}
	if rows > 0 && contains != large {
		t.Fatal("constraint selected the wrong exact source values")
	}
	return out
}

func cw01Number(value string) semantics.ClarificationValue {
	return semantics.ClarificationValue{Number: &semantics.ClarificationNumberInput{Value: value, Unit: "USD"}}
}
func cw01Time(start, end, grain string) semantics.ClarificationValue {
	return semantics.ClarificationValue{Time: &semantics.ClarificationTimeInput{Start: start, End: end, Calendar: "gregorian", TimeZone: "America/Argentina/Buenos_Aires", Grain: grain}}
}
func cw01Bool(value string) semantics.ClarificationValue {
	return semantics.ClarificationValue{Boolean: &value}
}
func cw01Text(value string) semantics.ClarificationValue {
	return semantics.ClarificationValue{Text: &value}
}
