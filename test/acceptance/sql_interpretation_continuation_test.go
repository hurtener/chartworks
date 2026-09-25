package acceptance

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	"github.com/hurtener/chartworks/test/support"
)

func interpretationContinuationFixture(t *testing.T) (*phase17Fixture, *nlqexec.Service) {
	t.Helper()
	ctx := context.Background()
	f, drafts, topics, model, pack := publicationFixture(t)
	if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET name=CASE id WHEN 1 THEN 'NORTH' ELSE 'SOUTH' END,created_at='2025-03-10T12:00:00Z';
 INSERT INTO analytics.sales(id,amount,name,created_at) VALUES (3,30,'NORTH','2025-04-01T00:00:00Z'),(4,40,'NORTH','2026-04-10T12:00:00Z'),(5,50,'SOUTH','2026-04-20T12:00:00Z'),(6,60,'NORTH','2025-03-31T23:59:59Z'),(7,70,'NORTH',NULL)`); err != nil {
		t.Fatal(err)
	}
	for i := range pack.Datasets[0].Columns {
		pack.Datasets[0].Columns[i].Sensitivity = semantics.LiteralNonSensitive
	}
	id := pack.Datasets[0].ID
	pack.Dimensions = []semantics.Dimension{
		{ID: "region", Name: "Region", Aliases: []string{"región"}, Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: id, ID: "name"}, Role: semantics.DimensionCategorical, Values: []semantics.GovernedValue{{ID: "north", Value: "NORTH", Aliases: []string{"north", "norte"}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "synthetic-north", Policy: "review-v1"}}, {ID: "south", Value: "SOUTH", Aliases: []string{"south", "sur"}, Sensitivity: semantics.LiteralNonSensitive, Provenance: semantics.ValueProvenance{Kind: "review", Evidence: "synthetic-south", Policy: "review-v1"}}}},
		{ID: "event_date", Name: "Event date", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: id, ID: "created_at"}, Role: semantics.DimensionTemporal, Temporal: &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainQuarter, semantics.GrainYear}}},
	}
	pub := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	phase17PublishTopic(t, drafts, topics, pub, pack)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topics, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &phase17Fixture{f: f, pack: pack, model: model, service: router, context: pack.Datasets[0].Source.Context}
	fixture.e = phase18Envelope(t, fixture, f.e.User(), "interpretation-session", true)
	model.mode.Store(phase18RawResponse(t, `SELECT id,sum(amount) AS amount FROM analytics.sales GROUP BY id ORDER BY id`))
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	query, _ := newPhase18Service(t, fixture)
	return fixture, query
}

func TestSQLRecoveryInterpretationContinuationLifecycleAcceptance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		t.Run(string(locale), func(t *testing.T) {
			f, query := interpretationContinuationFixture(t)
			ctx := context.Background()
			first, next, south, quarter := "Revenue in north in March 2025", "Show the same records", "Now south", "Now last quarter"
			if locale == nlq.LanguageSpanish {
				first, next, south, quarter = "Revenue en norte en marzo de 2025", "Mostrar los mismos registros", "Ahora sur", "Ahora el trimestre pasado"
			}
			parent, err := query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: f.pack.Topic, Context: f.context, Locale: locale, Question: first, InterpretationAnchor: "2026-09-22", Kinds: []string{"measure"}, LimitPerKind: 1}})
			if err != nil || parent.QueryID == "" || parent.Bindings == nil {
				t.Fatal("initial inferred plan", err)
			}
			run := func(p nlqexec.PlanResult, ids ...string) nlqexec.RunResult {
				t.Helper()
				out, err := query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
				if err != nil {
					t.Fatal(err)
				}
				requireParameterIDs(t, out, ids...)
				return out
			}
			run(parent, "1", "6")
			scope, _ := store.NewScope(f.e.Tenant(), f.e.User())
			original, err := f.f.db.ReadQuery(ctx, scope, parent.QueryID)
			if err != nil {
				t.Fatal(err)
			}
			originalDigest := nlqexec.QueryLineageDigest(original)
			query, _ = newPhase18Service(t, f)
			child, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: parent.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: next}})
			if err != nil || child.QueryID == parent.QueryID || child.Bindings == nil {
				t.Fatal("abbreviated refinement", err)
			}
			run(child, "1", "6")
			retained, err := f.f.db.ReadQuery(ctx, scope, child.QueryID)
			if err != nil || retained.Parent != parent.QueryID || retained.ParentDigest != originalDigest || retained.Route.Interpretation.Anchor != "2026-09-22" || len(retained.Route.Request.InterpretationSelections) != 2 {
				t.Fatal("durable inferred state/lineage", err)
			}
			if period := retained.Route.Interpretation.Temporal[0]; period.Start != "2025-03-01T00:00:00Z" || period.End != "2025-04-01T00:00:00Z" {
				t.Fatal("exact interval shifted")
			}
			changed, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: south}})
			if err != nil {
				t.Fatal(err)
			}
			run(changed, "2")
			last, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: changed.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: quarter}})
			if err != nil {
				t.Fatal(err)
			}
			out := run(last, "5")
			if p := last.Route.Interpretation.Temporal[0]; p.LocalStart != "2026-04-01" || p.LocalEnd != "2026-07-01" {
				t.Fatal("relative follow-up lost retained anchor")
			}
			metadata := support.Raw(t, f.f.dsn)
			calls := f.model.requests.Load()
			attempts := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			query, _ = newPhase18Service(t, f)
			replay := run(last, "5")
			if calls != f.model.requests.Load() || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || readexec.Hash(replay.Analytical) != readexec.Hash(out.Analytical) {
				t.Fatal("replay performed model/source work or changed proof")
			}
			saved, err := f.f.db.ReadSavedQuery(ctx, f.e, last.QueryID, false)
			if err != nil || saved.Result != nil || !reflect.DeepEqual(saved.Route.Interpretation, last.Route.Interpretation) {
				t.Fatal("saved state or result privacy", err)
			}
			current, err := f.f.db.ReadQuery(ctx, scope, parent.QueryID)
			if err != nil || nlqexec.QueryLineageDigest(current) != originalDigest {
				t.Fatal("parent modified", err)
			}
		})
	}
}

func TestSQLRecoveryInterpretationContinuationEditsAcceptance(t *testing.T) {
	f, query := interpretationContinuationFixture(t)
	ctx := context.Background()
	q := nlqexec.QuestionRequest{Topic: f.pack.Topic, Context: f.context, Locale: nlq.LanguageEnglish, Question: "Revenue north in March 2025", InterpretationAnchor: "2026-09-22", Kinds: []string{"measure"}, LimitPerKind: 1}
	p, err := query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	topic := f.pack.Topic
	child, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: p.QueryID, QuestionRequest: nlqexec.QuestionRequest{InterpretationEdits: []nlqroute.InterpretationEdit{{Target: topic + ":event_date:time", Action: "replace", Period: &nlqroute.InterpretationPeriod{Start: "2026-04-01", End: "2026-07-01", Grain: "quarter"}}}}})
	if err != nil {
		t.Fatal("typed date replacement", err)
	}
	out, err := query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "4")
	removed, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: child.QueryID, QuestionRequest: nlqexec.QuestionRequest{InterpretationEdits: []nlqroute.InterpretationEdit{{Target: topic + ":event_date:time", Action: "remove"}, {Target: topic + ":region:north", Action: "remove"}}}})
	if err != nil {
		t.Fatal("typed date/value removal", err)
	}
	out, err = query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: removed.QueryID, Operation: removed.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "1", "2", "3", "4", "5", "6", "7")
	if len(removed.Route.Interpretation.Values)+len(removed.Route.Interpretation.Temporal) != 0 {
		t.Fatal("removed state remained active")
	}
	// A changed prompt can explicitly select a previously removed value again.
	again, err := query.Refine(ctx, f.e, nlqexec.RefineRequest{QueryID: removed.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Now south"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err = query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: again.QueryID, Operation: again.QueryID + "-run"})
	if err != nil {
		t.Fatal(err)
	}
	requireParameterIDs(t, out, "2", "5")
	// Parent state cannot be borrowed from another actor/session.
	other := phase18Envelope(t, f, f.e.User(), "different-session", true)
	calls := f.model.requests.Load()
	if _, err := query.Refine(ctx, other, nlqexec.RefineRequest{QueryID: p.QueryID}); err == nil || calls != f.model.requests.Load() {
		t.Fatal("foreign-session state reached provider")
	}
	// The checked parameter list remains private, not embedded in selection JSON.
	sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
	stored, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(stored.Route.Request.InterpretationSelections)
	if err != nil || len(raw) == 0 || len(stored.Parameters) != 3 {
		t.Fatal("durable typed selections/bindings", err)
	}
}
