package acceptance

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/evaluation"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

type exp01Fixture struct {
	base   *phase17Fixture
	query  *nlqexec.Service
	drafts *drafts.Service
	topics *topics.Service
}

func newEXP01Fixture(t *testing.T) *exp01Fixture {
	t.Helper()
	f, draftService, topicService, model, pack := publicationFixture(t)
	amount := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "amount"}
	id := semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "id"}
	pack.Measures = append(pack.Measures, semantics.Measure{ID: "margin", Name: "Margin", Description: "Synthetic average amount", Field: amount, Aggregation: semantics.AggregationAverage, Unit: "currency"})
	pack.Dimensions = []semantics.Dimension{{ID: "region", Name: "Region", Description: "Synthetic grouping", Field: id, Role: semantics.DimensionCategorical}}
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	phase17PublishTopic(t, draftService, topicService, e, pack)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	route, err := nlqroute.New(topicService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(route, topicService, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	return &exp01Fixture{base: &phase17Fixture{f: f, e: e, pack: pack, service: route, model: model, context: pack.Datasets[0].Source.Context}, query: query, drafts: draftService, topics: topicService}
}

func TestEXP01ConversationalContinuity(t *testing.T) {
	fixture := newEXP01Fixture(t)
	ctx := context.Background()
	e := phase18Envelope(t, fixture.base, fixture.base.f.e.User(), "exp01-session", true)
	fixture.base.model.embeddingMode.Store("fixed")
	fixture.base.model.rerankMode.Store("fixed")
	fixture.base.model.mode.Store(phase18RawResponse(t, "SELECT id, amount FROM analytics.sales ORDER BY id"))

	revenue := semantics.Reference{Kind: semantics.KindMeasure, ID: "revenue"}
	region := semantics.Reference{Kind: semantics.KindDimension, ID: "region"}
	question := phase18Question(fixture.base, nlq.LanguageEnglish, fixture.base.pack.Topic)
	question.References = []semantics.Reference{revenue}
	question.MetricIDs = []string{"revenue"}
	initial, err := fixture.query.Plan(ctx, e, nlqexec.PlanRequest{QuestionRequest: question})
	if err != nil {
		t.Fatal(err)
	}
	beforeRejected := fixture.base.model.requests.Load()
	margin := semantics.Reference{Kind: semantics.KindMeasure, ID: "margin"}
	if _, err = fixture.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: initial.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show margin"}, ReferenceEdits: []nlqexec.ReferenceEdit{{Action: "replace", Target: revenue, Replacement: &margin}}}); !errors.Is(err, nlqexec.ErrInvalid) {
		t.Fatalf("unpaired metric/reference edit returned %v", err)
	}
	if after := fixture.base.model.requests.Load(); after != beforeRejected {
		t.Fatalf("incoherent refinement reached model gateway: before=%d after=%d", beforeRejected, after)
	}

	withDimension, err := fixture.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: initial.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Show revenue by region"}, ReferenceEdits: []nlqexec.ReferenceEdit{{Action: "add", Target: region}}})
	if err != nil {
		t.Fatal("add dimension", err)
	}
	changedMetric, err := fixture.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: withDimension.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Ahora muestra el margen por región"}, ReferenceEdits: []nlqexec.ReferenceEdit{{Action: "replace", Target: revenue, Replacement: &margin}}, MetricEdits: []nlqexec.MetricEdit{{Action: "replace", Target: "revenue", Replacement: "margin"}}})
	if err != nil {
		t.Fatal("change metric", err)
	}
	withoutDimension, err := fixture.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: changedMetric.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Remove the region grouping"}, ReferenceEdits: []nlqexec.ReferenceEdit{{Action: "remove", Target: region}}})
	if err != nil {
		t.Fatal("remove dimension", err)
	}

	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		t.Fatal(err)
	}
	record, err := fixture.base.f.db.ReadQuery(ctx, scope, withoutDimension.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	parentRecord, err := fixture.base.f.db.ReadQuery(ctx, scope, changedMetric.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(record.Route.Request.References, []semantics.Reference{margin}) || !reflect.DeepEqual(record.Route.Request.MetricIDs, []string{"margin"}) || record.Parent != changedMetric.QueryID || record.ParentRevision != parentRecord.Revision || record.ParentDigest != nlqexec.QueryLineageDigest(parentRecord) {
		t.Fatalf("stored continuity evidence retained stale selections: %#v", record.Route.Request)
	}
	staleChild := record
	staleChild.ID = "stale-child"
	staleChild.Parent = record.ID
	staleChild.ParentRevision = record.Revision
	staleChild.ParentDigest = nlqexec.QueryLineageDigest(record)
	staleChild.Revision = 1
	mutatedParent := record
	mutatedParent.Status = "executed"
	mutatedParent.Revision++
	if err := fixture.base.f.db.UpdateQuery(ctx, scope, mutatedParent, record.Revision); err != nil {
		t.Fatal(err)
	}
	if err := fixture.base.f.db.CreateQuery(ctx, scope, staleChild); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("stale child lineage committed after concurrent parent update: %v", err)
	}
	spanishSession := phase18Envelope(t, fixture.base, fixture.base.f.e.User(), "exp01-spanish-session", true)
	spanishQuestion := phase18Question(fixture.base, nlq.LanguageSpanish, fixture.base.pack.Topic)
	spanishQuestion.Question = "¿Cuáles son los ingresos?"
	spanishQuestion.References = []semantics.Reference{revenue}
	spanishQuestion.MetricIDs = []string{"revenue"}
	spanishInitial, err := fixture.query.Plan(ctx, spanishSession, nlqexec.PlanRequest{QuestionRequest: spanishQuestion})
	if err != nil {
		t.Fatal("Spanish initial question", err)
	}
	if _, err = fixture.query.Refine(ctx, spanishSession, nlqexec.RefineRequest{QueryID: spanishInitial.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "Ahora por región"}, ReferenceEdits: []nlqexec.ReferenceEdit{{Action: "add", Target: region}}}); err != nil {
		t.Fatal("Spanish continuity", err)
	}

	otherSession := phase18Envelope(t, fixture.base, fixture.base.f.e.User(), "exp01-other-session", true)
	if _, err = fixture.query.Refine(ctx, otherSession, nlqexec.RefineRequest{QueryID: withoutDimension.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "reuse this"}}); !errors.Is(err, nlqexec.ErrForeignSession) {
		t.Fatalf("cross-session continuation returned %v", err)
	}
	if _, err = fixture.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: withoutDimension.QueryID, QuestionRequest: nlqexec.QuestionRequest{Context: "foreign-context", Question: "reuse this"}}); !errors.Is(err, nlqexec.ErrForeignSession) {
		t.Fatalf("cross-context continuation returned %v", err)
	}

	updated := fixture.base.pack
	updated.Version = "v2"
	draft, err := fixture.drafts.Save(ctx, fixture.base.e, drafts.SaveRequest{Expected: 1, Pack: updated, Change: "Synthetic continuity republish"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := fixture.topics.Review(ctx, fixture.base.e, updated.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Synthetic continuity republish"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.topics.Publish(ctx, fixture.base.e, updated.Topic, topics.PublishRequest{Review: review.ID, Expected: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.query.Refine(ctx, e, nlqexec.RefineRequest{QueryID: withoutDimension.QueryID, QuestionRequest: nlqexec.QuestionRequest{Question: "continue after publication"}}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatalf("semantic republish did not require a newly routed question: %v", err)
	}

	journeyDigest := strings.Repeat("e", 64)
	cases := []evaluation.Case{}
	for i, item := range []struct{ id, locale string }{{"add-dimension", "en"}, {"replace-filter", "es"}, {"remove-filter", "en"}, {"change-metric", "es"}, {"correct-clarification", "en"}, {"semantic-republish", "es"}} {
		observation := evaluation.Observation{Decision: "governed_continuity", SemanticDigest: journeyDigest}
		cases = append(cases, evaluation.Case{ID: item.id, Stage: evaluation.StageContext, Category: "EXP-01", Locale: item.locale, HeldOut: i%2 == 0, Input: evaluation.ProtectedRef{Digest: journeyDigest, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: observation.Decision, SemanticDigest: journeyDigest}}, Fixture: &observation})
	}
	suite := evalSuite(evaluation.Fixture, cases)
	suite.ID = "exp01-continuity"
	report, err := evaluation.Evaluate(ctx, "exp01-fixture", suite, nil, evalClock)
	if err != nil || !report.GatePassed || report.QualityPassed != len(cases) {
		t.Fatalf("Phase 24 continuity cases failed: %#v %v", report, err)
	}
}
