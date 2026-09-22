package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
)

func cw09Relative(mode, unit string, count int) *reporting.Value {
	return &reporting.Value{Period: &reporting.Period{Mode: mode, Unit: unit, Count: count, DSTPolicy: "reject", MonthPolicy: "clamp"}}
}

func TestCW09GovernedBlockDepth(t *testing.T) {
	f := newPhase27CatalogFixture(t)
	ctx, e := context.Background(), f.actor

	t.Run("period certification and fixed list binds", func(t *testing.T) {
		d := phase27Definition(t, f.phase17Fixture, e, "SELECT id, amount FROM analytics.sales WHERE name IN ($1,$2) AND created_at >= $3 AND created_at < $4 ORDER BY id")
		var err error
		d, err = reporting.MigrateDefinition(d)
		if err != nil {
			t.Fatal(err)
		}
		d.Metadata[0].Question = "Revenue for the last 3 months"
		d.Metadata[1].Question = "Ingresos de los últimos 3 meses"
		d.Metadata[0].Intent = &reporting.QuestionIntent{Metrics: []string{"revenue"}, Grain: "month", Population: "sales", Period: &reporting.IntentPeriod{Mode: "previous", Unit: "month", Count: 1}}
		d.Metadata[1].Intent = phase27Copy(t, d.Metadata[0].Intent)
		d.Parameters = []reporting.Parameter{
			{Name: "categories", Type: "dimension_list", Required: true, ListLength: 2, Enum: []string{"one", "two", "x' OR true --"}, Dimension: &reporting.DimensionReference{Topic: f.pack.Topic, Version: f.pack.Version, Dimension: "category"}, Default: &reporting.Value{Items: []string{"one", "two"}}},
			{Name: "window", Type: "relative_period", Required: true, Default: cw09Relative("previous", "month", 1)},
		}
		created, err := f.blocks.Create(ctx, e, reporting.CreateRequest{ID: "cw09-depth", Definition: d})
		if err != nil {
			t.Fatal("create", err)
		}
		validation, err := f.blocks.Validate(ctx, e, created.State.ID, reporting.ValidateRequest{ExpectedVersion: created.State.Version})
		if err != nil {
			t.Fatal("validate fixed binds", err)
		}
		state, err := f.blocks.Publish(ctx, e, created.State.ID, reporting.PublishRequest{ExpectedVersion: validation.State.Version, Evidence: validation.Evidence.ID})
		if err != nil {
			t.Fatal("publish", err)
		}
		review, err := f.blocks.ReviewPeriodLanguage(ctx, e, created.State.ID, reporting.PeriodReviewRequest{Revision: created.Revision})
		if err != nil || len(review.Findings) != 2 {
			t.Fatal("visible bilingual findings", review, err)
		}
		if _, err = f.blocks.Certify(ctx, e, created.State.ID, reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: created.Revision, Evidence: validation.Evidence.ID, Note: "review without acknowledgement"}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("contradiction certified without review", err)
		}
		reviews := make([]reporting.PeriodReview, len(review.Findings))
		for i, finding := range review.Findings {
			reviews[i] = reporting.PeriodReview{Finding: finding.Digest, Disposition: "accepted_exception"}
		}
		type certificationResult struct {
			attestation reporting.Attestation
			err         error
		}
		results := make(chan certificationResult, 2)
		for range 2 {
			go func() {
				attestation, certifyErr := f.blocks.Certify(ctx, e, created.State.ID, reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: created.Revision, Evidence: validation.Evidence.ID, Note: "reviewed localized discrepancy", PeriodReviews: reviews})
				results <- certificationResult{attestation: attestation, err: certifyErr}
			}()
		}
		var attestation reporting.Attestation
		successes, conflicts := 0, 0
		for range 2 {
			result := <-results
			switch {
			case result.err == nil:
				successes++
				attestation = result.attestation
			case errors.Is(result.err, store.ErrConflict):
				conflicts++
			default:
				t.Fatal("unexpected concurrent certification result", result.err)
			}
		}
		if successes != 1 || conflicts != 1 || len(attestation.PeriodFindings) != 2 || len(attestation.PeriodReviews) != 2 {
			t.Fatal("concurrent reviewed certification", successes, conflicts, attestation)
		}

		injected, err := f.blocks.Resolve(ctx, e, created.State.ID, reporting.ResolveRequest{Arguments: []reporting.Argument{{Name: "categories", Value: reporting.Value{Items: []string{"one", "x' OR true --"}}}}})
		if err != nil || len(injected.Resolved.Parameters) != 4 || injected.Resolved.Parameters[1].Value != "x' OR true --" {
			t.Fatal("list did not remain bound", injected, err)
		}
	})

	t.Run("reviewed overlap and durable authorized scope", func(t *testing.T) {
		intent := &reporting.QuestionIntent{Metrics: []string{"revenue"}, Grain: "month", Population: "sales", Filters: []reporting.IntentFilter{{Dimension: "category", Operator: "eq", Value: "one"}}, Period: &reporting.IntentPeriod{Mode: "previous", Unit: "month", Count: 1}}
		assessment, err := f.blocks.AssessQuestions(ctx, e, reporting.QuestionRequest{Locale: "en-US", Question: "Monthly proceeds for active sales", Intent: intent, IncludeDrafts: true})
		if err != nil || !hash64(assessment.CandidateScopeDigest) || !hash64(assessment.EvidenceDigest) || assessment.ID == "" {
			t.Fatal("durable assessment", assessment, err)
		}
		found := false
		for _, match := range assessment.Matches {
			if match.ID == "cw09-depth" && match.Kind == "duplicate" && match.Method == "reviewed_intent" {
				found = true
			}
		}
		if !found {
			t.Fatal("paraphrase not recognized from reviewed intent", assessment)
		}
	})
}

func hash64(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
