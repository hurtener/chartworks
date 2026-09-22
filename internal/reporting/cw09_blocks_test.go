package reporting

import (
	"errors"
	"strings"
	"testing"
)

func cw09Period(mode, unit string, count int) *Value {
	return &Value{Period: &Period{Mode: mode, Unit: unit, Count: count, DSTPolicy: "reject", MonthPolicy: "clamp"}}
}

func TestCertificationPeriodLanguageRequiresExactReviewedDisposition(t *testing.T) {
	d := contractDefinition()
	d.Metadata = []Localized{
		{Locale: "en-US", Title: "Revenue", Question: "Revenue for the last 3 months"},
		{Locale: "es-AR", Title: "Ingresos", Question: "Ingresos de los últimos 3 meses"},
	}
	d.Parameters = []Parameter{{Name: "window", Type: "relative_period", Required: true, Default: cw09Period("previous", "month", 1)}}
	findings := periodFindings(d)
	if len(findings) != 2 || findings[0].Code != "period_wording_mismatch" || findings[1].Code != "period_wording_mismatch" {
		t.Fatalf("bilingual mismatches not exposed: %#v", findings)
	}
	if err := validatePeriodReviews(findings, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("unreviewed contradiction certified", err)
	}
	reviews := []PeriodReview{{Finding: findings[0].Digest, Disposition: "accepted_exception"}, {Finding: findings[1].Digest, Disposition: "accepted_exception"}}
	if err := validatePeriodReviews(findings, reviews); err != nil {
		t.Fatal(err)
	}
	reviews[0].Finding = strings.Repeat("a", 64)
	if err := validatePeriodReviews(findings, reviews); !errors.Is(err, ErrInvalid) {
		t.Fatal("stale finding acknowledgement accepted")
	}

	d.Parameters[0].Default = cw09Period("previous", "month", 3)
	if got := periodFindings(d); len(got) != 0 {
		t.Fatalf("matching wording rejected: %#v", got)
	}
	d.Metadata[0].Question = "Revenue for the last month and last year"
	if got := periodFindings(d); len(got) == 0 || got[0].Code != "ambiguous_period_wording" {
		t.Fatalf("ambiguous wording accepted: %#v", got)
	}
}

func TestReviewedQuestionIntentDistinguishesParaphraseOverlapAndUnique(t *testing.T) {
	base := QuestionIntent{Metrics: []string{"net_revenue"}, Grain: "month", Population: "active_accounts", Filters: []IntentFilter{{Dimension: "region", Operator: "eq", Value: "north"}}, Period: &IntentPeriod{Mode: "previous", Unit: "month", Count: 1}}
	paraphrase := clone(base)
	kind, score, evidence := semanticQuestionMatch(base, paraphrase)
	if kind != "duplicate" || score != 1 || !hashValid(evidence) {
		t.Fatal(kind, score, evidence)
	}
	overlap := clone(base)
	overlap.Period.Count = 3
	kind, score, _ = semanticQuestionMatch(base, overlap)
	if kind != "overlap" || score >= 1 {
		t.Fatal(kind, score)
	}
	unique := clone(base)
	unique.Metrics = []string{"customer_churn"}
	kind, _, _ = semanticQuestionMatch(base, unique)
	if kind != "unique" {
		t.Fatal(kind)
	}
	if !validQuestionIntent(&base) {
		t.Fatal("valid reviewed intent rejected")
	}
	bad := clone(base)
	bad.Filters[0].Operator = "sql"
	if validQuestionIntent(&bad) {
		t.Fatal("executable filter intent accepted")
	}
	duplicateFilter := clone(base)
	duplicateFilter.Filters = append(duplicateFilter.Filters, duplicateFilter.Filters[0])
	if validQuestionIntent(&duplicateFilter) {
		t.Fatal("duplicate semantic filter accepted")
	}
	explicit := clone(base)
	explicit.Period = &IntentPeriod{Mode: "explicit", Start: "2026-01-01", End: "2026-02-01"}
	if !validQuestionIntent(&explicit) {
		t.Fatal("valid explicit period rejected")
	}
	explicit.Period.End = "2025-12-31"
	if validQuestionIntent(&explicit) {
		t.Fatal("reversed explicit period accepted")
	}
}

func TestParameterizationProposalIsDialectBoundAndTamperEvident(t *testing.T) {
	p := Parameter{Name: "window", Type: "relative_period", Required: true, Default: cw09Period("previous", "month", 1)}
	if disposition, reason := parameterizationDisposition("mysql"); disposition != "unsupported" || reason == "" {
		t.Fatal(disposition, reason)
	}
	if disposition, reason := parameterizationDisposition("postgres"); disposition != "supported" || reason != "" {
		t.Fatal(disposition, reason)
	}
	a := proposalDigest(strings.Repeat("a", 64), "postgres", []string{"created_at"}, p, "supported")
	b := proposalDigest(strings.Repeat("a", 64), "mysql", []string{"created_at"}, p, "unsupported")
	if !hashValid(a) || a == b {
		t.Fatal("proposal lost dialect binding", a, b)
	}
	if !parameterizationIntentValid(ParameterizeRequest{OriginalQuestion: "Revenue last month", QuestionDisposition: "preserved", TemplateDisposition: "not_applicable", ParaphraseDisposition: "preserved"}, true) {
		t.Fatal("valid authoring intent rejected")
	}
	if parameterizationIntentValid(ParameterizeRequest{QuestionDisposition: "invented"}, false) {
		t.Fatal("unbounded disposition accepted")
	}
}
