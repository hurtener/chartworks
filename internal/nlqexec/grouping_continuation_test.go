package nlqexec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func groupingParent(t *testing.T, a admission, version int) QueryRecord {
	t.Helper()
	c, err := compileAnalyticalVersion(context.Background(), a, version)
	if err != nil {
		t.Fatal(err)
	}
	q := QueryRecord{Question: a.route.Request.Question, Route: a.route, SQL: `SELECT region_native,sum(amount_native) FROM analytics.sales GROUP BY region_native`, AnalyticalVersion: version}
	q.Analytical = &exec.AnalyticalReceipt{Version: c.Version, Scope: exec.AnalyticalGrainScope, Contract: exec.Hash(*c), Query: exec.AnalyticalQueryDigest(q.SQL, nil), Metrics: []string{c.Metrics[0].ID}}
	if c.Grain != nil {
		q.Analytical.Grouping = append([]string(nil), c.Grain.Dimensions...)
	} else {
		q.Analytical.Scope = exec.AnalyticalMetricScope
	}
	return q
}
func TestSQLRecoveryGroupingInheritanceAndReplacement(t *testing.T) {
	a := grainAdmission("Revenue by Region")
	parent := groupingParent(t, a, 4)
	before := exec.Hash(parent)
	for _, tc := range []struct{ question, dimension string }{{"Same results", "region"}, {"Now by Order", "order"}, {"Ahora por orden", "order"}, {"total general", ""}, {"without grouping", ""}} {
		delta := QuestionRequest{Question: tc.question}
		q := refinementQuestion(parent, delta)
		retainCatalogSelection(parent, &q)
		if err := retainGrouping(context.Background(), parent, a, nil, delta, &q); err != nil {
			t.Fatal(tc.question, err)
		}
		if q.Grouping == nil || q.Grouping.Policy != nlqroute.GroupingPolicy {
			t.Fatal("lost exact grouping")
		}
		if tc.dimension == "" {
			if len(q.Grouping.Keys) != 0 {
				t.Fatal("total still grouped")
			}
		} else if len(q.Grouping.Keys) != 1 || q.Grouping.Keys[0].Dimension != tc.dimension {
			t.Fatal("wrong dimension")
		}
		for _, ref := range q.References {
			if ref.Kind == semantics.KindDimension && ref.ID == "region" {
				t.Fatal("promoted old catalog grouping remained a user choice")
			}
		}
		if !reflect.DeepEqual(q.Grouping, q.routeRequest().Grouping) {
			t.Fatal("adapter dropped grouping")
		}
	}
	if exec.Hash(parent) != before {
		t.Fatal("parent mutated")
	}
	unknown := QuestionRequest{Question: "Now by Unknown"}
	q := refinementQuestion(parent, unknown)
	if err := retainGrouping(context.Background(), parent, a, nil, unknown, &q); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
		t.Fatal("unknown new grouping silently inherited old", err)
	}
	// Existing requests without a verified grain remain unmeasured when the new
	// wording names a raw column rather than a reviewed selected dimension.
	// There is no old group to silently inherit; normal route/native validation
	// must still process the whole new request (covered by phase18 AC01/AC06).
	unmeasured := grainAdmission("Revenue")
	unmeasuredParent := groupingParent(t, unmeasured, 4)
	rawColumn := QuestionRequest{Question: "Show revenue by id"}
	unmeasuredQuestion := refinementQuestion(unmeasuredParent, rawColumn)
	if err := retainGrouping(context.Background(), unmeasuredParent, unmeasured, nil, rawColumn, &unmeasuredQuestion); err != nil || unmeasuredQuestion.Grouping != nil {
		t.Fatal("unmeasured legacy grouping acquired a contract or new rejection", err)
	}
	q = refinementQuestion(parent, QuestionRequest{Grouping: &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy}})
	if err := retainGrouping(context.Background(), parent, a, nil, QuestionRequest{Grouping: q.Grouping}, &q); err != nil || len(q.Grouping.Keys) != 0 {
		t.Fatal("typed removal", err)
	}
}

func TestSQLRecoveryGroupingCompileAndRetainedVersionFences(t *testing.T) {
	a := grainAdmission("Revenue by Region")
	a.route.Request.Grouping = &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: "sales_topic", Dimension: "order"}}}
	c, err := compileAnalytical(context.Background(), a)
	if err != nil || c.Grain == nil || !reflect.DeepEqual(c.Grain.Columns, []string{"id_native"}) {
		t.Fatal("typed replacement did not override old words", err)
	}
	for _, v := range []int{1, 2, 3, 4} {
		if _, err := compileAnalyticalVersion(context.Background(), a, v); !errors.Is(err, exec.ErrBinding) {
			t.Fatal("old policy acquired explicit grouping", v, err)
		}
	}
	a.route.Request.Grouping.Keys = nil
	c, err = compileAnalytical(context.Background(), a)
	if err != nil || c.Grain == nil || len(c.Grain.Columns) != 0 || !strings.Contains(analyticalGrainGuidanceOnly(c), "scalar total") {
		t.Fatal("total missing enforced instruction", err)
	}
	a.route.Request.Grouping.Keys = []nlqroute.GroupingKey{{Topic: "other", Dimension: "region"}}
	if _, err = compileAnalytical(context.Background(), a); err == nil {
		t.Fatal("foreign topic accepted")
	}
}

func TestSQLRecoveryGroupingPendingOriginAndSavedState(t *testing.T) {
	e := unitEnvelope(t)
	repo := newUnitRepository()
	group := &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: "topic", Dimension: "region"}}}
	pending := unitQuery(e, "pending-group", "topic", "v1", "context", false)
	pending.SQL = ""
	pending.Status = "preflight"
	pending.Route.AnswerContext = "answer-context"
	pending.Route.Request = nlqroute.RouteRequest{Question: "Revenue", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"}, Grouping: group}
	repo.queries[pending.ID] = pending
	service := &Service{repo: repo}
	input := QuestionRequest{Question: "Revenue", Locale: nlq.LanguageEnglish, Context: "context", Topics: []string{"topic"}, Grouping: nlqroute.CloneGrouping(group), Answers: []semantics.ClarificationAnswer{{Topic: "topic"}}, ClarificationQuery: pending.ID, AnswerContext: "answer-context"}
	if err := service.validateClarificationOrigin(context.Background(), e, input, "query.plan"); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*QuestionRequest){func(q *QuestionRequest) { q.Grouping = nil }, func(q *QuestionRequest) { q.Grouping.Keys = nil }, func(q *QuestionRequest) { q.Grouping.Keys[0].Grain = "month" }} {
		bad := input
		bad.Grouping = nlqroute.CloneGrouping(input.Grouping)
		change(&bad)
		if err := service.validateClarificationOrigin(context.Background(), e, bad, "query.plan"); err == nil {
			t.Fatal("different grouping borrowed preflight")
		}
	}
	saved := SavedQuestion{Durability: "replayable", Context: "context", Question: "Revenue", Topics: []SavedTopic{{Topic: "topic"}}, Selections: &SavedSelections{Grouping: group}}
	q := savedRouting(saved, nlq.LanguageEnglish)
	if !reflect.DeepEqual(q.Grouping, group) {
		t.Fatal("saved state lost")
	}
	q.Grouping.Keys[0].Dimension = "changed"
	if group.Keys[0].Dimension != "region" {
		t.Fatal("saved state aliases caller")
	}
}

func TestSQLRecoveryGroupingCalendarAndPrivateValues(t *testing.T) {
	a := calendarAdmission("Revenue by month of Order date", "timestamptz")
	p := groupingParent(t, a, 4)
	p.Analytical.Scope = exec.AnalyticalCalendarScope
	q := refinementQuestion(p, QuestionRequest{Question: "Same"})
	if err := retainGrouping(context.Background(), p, a, nil, QuestionRequest{Question: "Same"}, &q); err != nil || q.Grouping == nil || q.Grouping.Keys[0].Grain != semantics.GrainMonth {
		t.Fatal("calendar inheritance", err)
	}
	value := "by Order"
	a.route.Resolutions = []semantics.ClarificationResolution{{Topic: "sales_topic", Pattern: "private", Slot: "value", Value: value, Sensitivity: semantics.LiteralSensitive}}
	next := QuestionRequest{Question: "Revenue for by Order", Answers: []semantics.ClarificationAnswer{{Topic: "sales_topic", Pattern: "private", Slot: "value", Value: &semantics.ClarificationValue{Text: &value}}}}
	if g, err := groupingFromQuestion(context.Background(), a, &next, true); err != nil || g != nil {
		t.Fatal("sensitive literal became grouping", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := retainGrouping(ctx, p, a, nil, QuestionRequest{}, &q); !errors.Is(err, context.Canceled) {
		t.Fatal("ignored cancellation", err)
	}
}

func TestSQLRecoveryGroupingSharedFieldKeepsCalendarIdentity(t *testing.T) {
	a := calendarAdmission("Revenue", "timestamptz")
	first := &a.publications[0].Definition.Dimensions[2]
	first.Temporal.Grains = []semantics.TimeGrain{semantics.GrainMonth}
	first.Temporal.Timezone = "UTC"
	second := *first
	second.ID, second.Name = "event_local", "Local date"
	second.Temporal = &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "America/New_York", Grains: []semantics.TimeGrain{semantics.GrainDay}}
	a.publications[0].Definition.Dimensions = append(a.publications[0].Definition.Dimensions, second)
	a.route.Selection.Topics[0].Roots = append(a.route.Selection.Topics[0].Roots, nlqroute.SelectedRoot{Reference: semantics.Reference{Kind: semantics.KindDimension, ID: second.ID}, Reason: "grouping"})
	group := &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: "sales_topic", Dimension: "event", Grain: semantics.GrainMonth}, {Topic: "sales_topic", Dimension: second.ID, Grain: semantics.GrainDay}}}
	a.route.Request.Grouping = group
	analyticalReseal(&a)
	proof, err := compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	got, err := logicalGrouping(a, proof.Grain)
	if err != nil || !reflect.DeepEqual(got, nlqroute.CloneGrouping(group)) {
		t.Fatal("calendar identity crossed between shared-field dimensions", got, err)
	}
	// When both dimensions permit both grains in the same zone, the flat receipt
	// sets alone do not retain the original pairing. The protected request does.
	a.publications[0].Definition.Dimensions[2].Temporal.Grains = []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainDay}
	a.publications[0].Definition.Dimensions[3].Temporal = &semantics.TemporalPolicy{Calendar: "gregorian", Timezone: "UTC", Grains: []semantics.TimeGrain{semantics.GrainMonth, semantics.GrainDay}}
	analyticalReseal(&a)
	proof, err = compileAnalytical(context.Background(), a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = logicalGrouping(a, proof.Grain); !errors.Is(err, exec.ErrAnalyticalUnsupported) {
		t.Fatal("ambiguous flat calendar proof invented dimension/grain pairings", err)
	}
	old := groupingParent(t, a, analyticalRecordVersion)
	old.Analytical.Scope = exec.AnalyticalCalendarScope
	q := refinementQuestion(old, QuestionRequest{})
	if err := retainGrouping(context.Background(), old, a, nil, QuestionRequest{}, &q); err != nil || !reflect.DeepEqual(q.Grouping, nlqroute.CloneGrouping(group)) {
		t.Fatal("verified grouping expanded into an unintended cross-product", q.Grouping, err)
	}
}

func TestSQLRecoveryGroupingPendingReplacementKeepsScalarDataSeparate(t *testing.T) {
	a := grainAdmission("Revenue by Region")
	group := &nlqroute.GroupingSelection{Policy: nlqroute.GroupingPolicy, Keys: []nlqroute.GroupingKey{{Topic: "sales_topic", Dimension: "region"}}}
	a.route.Request.Grouping = group
	old := QueryRecord{Status: "preflight", Question: "Revenue by Region", Route: a.route}
	delta := QuestionRequest{Question: "Revenue by Order"}
	q := refinementQuestion(old, delta)
	if err := retainGrouping(context.Background(), old, a, nil, delta, &q); err != nil || q.Grouping == nil || len(q.Grouping.Keys) != 1 || q.Grouping.Keys[0].Dimension != "order" {
		t.Fatal("pending explicit group overrode the new reviewed question", q.Grouping, err)
	}
	value := "by Order"
	delta = QuestionRequest{Question: "Revenue for customer by Order", Answers: []semantics.ClarificationAnswer{{Topic: "sales_topic", Pattern: "customer", Slot: "name", Value: &semantics.ClarificationValue{Text: &value}}}}
	q = refinementQuestion(old, delta)
	if err := mergeRefinementClarifications(old, delta, &q); err != nil {
		t.Fatal(err)
	}
	before := exec.Hash([]any{old, a.route, q})
	if got, err := groupingFromQuestion(context.Background(), a, &q, true); err != nil || got != nil {
		t.Fatal("unresolved scalar spelling became a grouping", got, err)
	}
	if exec.Hash([]any{old, a.route, q}) != before {
		t.Fatal("temporary redaction mutated request or parent")
	}
	if err := retainGrouping(context.Background(), old, a, nil, delta, &q); err != nil || !reflect.DeepEqual(q.Grouping, group) {
		t.Fatal("scalar text changed inherited pending group", err)
	}
}
