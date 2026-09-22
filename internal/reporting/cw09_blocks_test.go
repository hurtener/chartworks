package reporting

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
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
	d.Metadata[0].Question = "Revenue this month"
	d.Metadata[1].Question = "Ingresos este mes"
	for _, finding := range periodFindings(d) {
		if finding.Code != "unsupported_period_wording" || finding.Observed != "current:month:1" {
			t.Fatalf("current-period wording not explicit: %#v", finding)
		}
	}
	d.Parameters[0].Default = cw09Period("rolling", "month", 1)
	if got := periodFindings(d); len(got) != 2 || got[0].Code != "unsupported_period_wording" || got[1].Code != "unsupported_period_wording" {
		t.Fatalf("current wording against rolling default not explicit: %#v", got)
	}
	d.Metadata = []Localized{{Locale: "es-AR", Title: "Ingresos", Question: "Ingresos del mes pasado"}}
	d.Parameters[0].Default = cw09Period("previous", "month", 1)
	if got := periodFindings(d); len(got) != 0 {
		t.Fatalf("completed Spanish month misclassified: %#v", got)
	}
	d.Parameters[0].Default = cw09Period("rolling", "month", 1)
	if got := periodFindings(d); len(got) != 1 || got[0].Code != "period_wording_mismatch" || got[0].Observed != "previous:month:1" {
		t.Fatalf("Spanish completed month did not contradict rolling default: %#v", got)
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
	caseDuplicate := clone(base)
	caseDuplicate.Filters = append(caseDuplicate.Filters, IntentFilter{Dimension: "Region", Operator: "eq", Value: "North"})
	if validQuestionIntent(&caseDuplicate) {
		t.Fatal("case-normalized duplicate semantic filter accepted")
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
	base := exec.Binding{Tenant: "tenant", Source: "warehouse", Context: "readonly", Revision: 1, Dialect: "postgres", Contract: "v1", Fingerprint: strings.Repeat("b", 64), Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales", Columns: []exec.Column{{Name: "created_at", NativeType: "timestamp"}}}}}
	topics := []TopicPin{{Topic: "sales", Version: "v1", Digest: strings.Repeat("c", 64)}}
	a := proposalDigest(strings.Repeat("a", 64), base, topics, []string{"created_at"}, p, "supported")
	other := clone(base)
	other.Dialect = "mysql"
	b := proposalDigest(strings.Repeat("a", 64), other, topics, []string{"created_at"}, p, "unsupported")
	if !hashValid(a) || a == b {
		t.Fatal("proposal lost dialect binding", a, b)
	}
	other = clone(base)
	other.Revision++
	if a == proposalDigest(strings.Repeat("a", 64), other, topics, []string{"created_at"}, p, "supported") {
		t.Fatal("proposal lost source revision binding")
	}
	if !parameterizationIntentValid(ParameterizeRequest{OriginalQuestion: "Revenue last month", QuestionDisposition: "preserved", TemplateDisposition: "not_applicable", ParaphraseDisposition: "preserved"}) {
		t.Fatal("valid authoring intent rejected")
	}
	if parameterizationIntentValid(ParameterizeRequest{QuestionDisposition: "invented"}) {
		t.Fatal("unbounded disposition accepted")
	}
}

func TestQuestionAssessmentAuthorityAndThresholdAreIdentityEvidence(t *testing.T) {
	now := time.Now
	deadline := now().Add(time.Hour)
	wildcard, err := identity.FromVerified("tenant", "actor", "session", []string{"reporting.read", "cw.block.read:*"}, deadline, now)
	if err != nil {
		t.Fatal(err)
	}
	narrowed, err := identity.FromVerified("tenant", "actor", "session", []string{"reporting.read", "cw.block.read:block"}, deadline, now)
	if err != nil {
		t.Fatal(err)
	}
	wildDigest, narrowDigest := assessmentAuthorityDigest(wildcard), assessmentAuthorityDigest(narrowed)
	if wildDigest == narrowDigest || questionAssessmentID(wildcard, wildDigest, strings.Repeat("d", 64)) == questionAssessmentID(narrowed, narrowDigest, strings.Repeat("d", 64)) {
		t.Fatal("narrowed and wildcard authority shared assessment identity")
	}
	evidence := func(threshold float64) string {
		return digest(struct {
			Version   string
			Threshold float64
		}{"question-assessment-threshold-test", threshold})
	}
	if evidence(0.8) == evidence(0.9) {
		t.Fatal("question threshold absent from evidence identity")
	}
}
