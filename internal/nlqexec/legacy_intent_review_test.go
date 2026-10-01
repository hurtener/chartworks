package nlqexec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
)

func TestSQLRecoveryLegacyIntentReviewEvidenceShape(t *testing.T) {
	q := QueryRecord{ID: "child", Parent: "legacy", AnalyticalVersion: 7, IntentReview: &IntentReviewEvidence{Legacy: IntentReviewOrigin{QueryID: "legacy", Revision: 1, Digest: exec.Hash("legacy")}, Preflight: IntentReviewOrigin{QueryID: "preflight", Revision: 1, Digest: exec.Hash("preflight")}, AnswerDigest: exec.Hash("answer")}}
	if !IntentReviewValid(q) || !IntentReviewValid(QueryRecord{}) {
		t.Fatal("valid review shape")
	}
	for _, mutate := range []func(*QueryRecord){
		func(q *QueryRecord) { q.Parent = "" }, func(q *QueryRecord) { q.IntentReview.Preflight.QueryID = q.ID }, func(q *QueryRecord) { q.IntentReview.Preflight = q.IntentReview.Legacy }, func(q *QueryRecord) { q.IntentReview.Legacy.Revision = 0 }, func(q *QueryRecord) { q.IntentReview.AnswerDigest = "bad" },
	} {
		copy := q
		proof := *q.IntentReview
		copy.IntentReview = &proof
		mutate(&copy)
		if IntentReviewValid(copy) {
			t.Fatal("invalid review accepted")
		}
	}
	before := QueryLineageDigest(q)
	q.IntentReview.AnswerDigest = exec.Hash("changed")
	if before == QueryLineageDigest(q) {
		t.Fatal("review omitted from lineage")
	}
}

func TestSQLRecoveryLegacyOrdinaryDomainReviewClassification(t *testing.T) {
	current := exec.AnalyticalContract{Version: exec.AnalyticalGroupedProgramsVersion, Grain: &exec.AnalyticalGrain{Columns: []string{"region"}}}
	domainMismatch := &exec.AnalyticalError{Code: "analytical_population_mismatch"}
	for version := 1; version <= 6; version++ {
		if !legacyCurrentIntentReviewNeeded(version, &current, fmt.Errorf("current proof: %w", domainMismatch)) {
			t.Fatal("legacy grouped predicate cannot request complete review", version)
		}
	}
	for _, version := range []int{0, 7, 8} {
		if legacyCurrentIntentReviewNeeded(version, &current, domainMismatch) {
			t.Fatal("retained-v7 or current policy acquired legacy review", version)
		}
	}
	for _, mutate := range []func(*exec.AnalyticalContract){
		func(c *exec.AnalyticalContract) { c.Version = exec.AnalyticalGroupedPopulationsVersion },
		func(c *exec.AnalyticalContract) { c.Grain = nil },
		func(c *exec.AnalyticalContract) { c.Grain = &exec.AnalyticalGrain{} },
		func(c *exec.AnalyticalContract) { c.Populations = []string{"one", "two"} },
		func(c *exec.AnalyticalContract) { c.GroupedPopulations = &exec.AnalyticalGroupedPopulations{} },
	} {
		copy := current
		mutate(&copy)
		if legacyCurrentIntentReviewNeeded(6, &copy, domainMismatch) {
			t.Fatal("non-ordinary grouping mismatch was reclassified")
		}
	}
	for _, err := range []error{nil, exec.ErrBinding, exec.ErrAnalyticalMismatch, errors.New("analytical_population_mismatch"), &exec.AnalyticalError{Code: "analytical_metric_mismatch"}, &exec.AnalyticalError{Code: exec.AnalyticalGroupDomainReviewCode, Unsupported: true}} {
		if legacyCurrentIntentReviewNeeded(6, &current, err) {
			t.Fatal("unrelated or dedicated owner-review error reclassified", err)
		}
	}
	for _, code := range []string{"analytical_query_population_mismatch", "analytical_limit_mismatch", "analytical_order_mismatch"} {
		if !legacyCurrentIntentReviewNeeded(6, &current, &exec.AnalyticalError{Code: code}) {
			t.Fatal("existing complete-intent review lost", code)
		}
	}
}
func TestSQLRecoveryLegacyIntentReviewPrivateDetachedState(t *testing.T) {
	in := LegacyIntentReview{QueryID: "private-query", AnswerContext: "private-context"}
	if strings.Contains(fmt.Sprintf("%v %#v", in, in), "private-") || strings.Contains(in.LogValue().String(), "private-") {
		t.Fatal("private review logging")
	}
	proof := &IntentReviewEvidence{AnswerDigest: exec.Hash("answer")}
	ctx := context.WithValue(context.Background(), intentReviewKey{}, proof)
	var record QueryRecord
	bindIntentReview(ctx, &record)
	proof.AnswerDigest = "mutated"
	if record.IntentReview == nil || record.IntentReview.AnswerDigest == "mutated" {
		t.Fatal("mutable evidence escaped request")
	}
}

func TestSQLRecoveryLegacyGroupedReviewGuidance(t *testing.T) {
	for _, locale := range []nlq.Language{nlq.LanguageEnglish, nlq.LanguageSpanish} {
		p := GenerationProblem(groupedDomainReviewGuidance(QueryRecord{Locale: locale}))
		if p == nil || !strings.Contains(p.Questions[0], "intent_review") {
			t.Fatal("missing guidance")
		}
		if locale == nlq.LanguageSpanish && !strings.HasPrefix(p.Questions[0], "Revisa") {
			t.Fatal("missing Spanish review")
		}
	}
	if isGroupedDomainReview(exec.ErrBinding) || !isGroupedDomainReview(&exec.AnalyticalError{Code: exec.AnalyticalGroupDomainReviewCode, Unsupported: true}) {
		t.Fatal("broad group review gate")
	}
}
