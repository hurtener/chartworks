package nlqexec

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"testing"
)

func TestLearningNegativeFailureObservation(t *testing.T) {
	for _, failure := range []error{exec.ErrUnsafe, exec.ErrQuery, exec.ErrUnsupported, store.ErrUnavailable} {
		t.Run(failure.Error(), func(t *testing.T) {
			e := unitEnvelope(t)
			repo := newUnitRepository()
			q := unitQuery(e, "failed-query", "topic", "v1", "context", false)
			q.Status = "failed"
			repo.queries[q.ID] = q
			validator := &unitValidator{errors: []error{failure, failure, failure, failure}}
			service := &Service{topics: &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}}, sources: retainedSourceReader{}, validator: validator, repo: repo}
			request := FeedbackRequest{QueryID: q.ID, Verdict: "negative", Note: "The failed result did not answer the question"}
			for i := 0; i < 2; i++ {
				if err := service.Feedback(context.Background(), e, request); err != nil {
					t.Fatal("negative observation lost", err)
				}
			}
			if len(repo.feedback) != 1 || len(repo.examples) != 0 {
				t.Fatal("observation became learning authority", len(repo.feedback), len(repo.examples))
			}
			if err := service.Feedback(context.Background(), e, FeedbackRequest{QueryID: q.ID, Verdict: "positive"}); !errors.Is(err, failure) {
				t.Fatal("unvalidated positive admitted", err)
			}
			if err := service.Feedback(context.Background(), e, FeedbackRequest{QueryID: q.ID, Verdict: "negative", Correction: "SELECT id FROM analytics.sales"}); !errors.Is(err, failure) {
				t.Fatal("invalid correction learned", err)
			}
		})
	}
	for _, failure := range []error{exec.ErrBinding, store.ErrNotFound, access.ErrUnauthenticated, context.Canceled, context.DeadlineExceeded, access.ErrForbidden, access.ErrNotFound} {
		e := unitEnvelope(t)
		repo := newUnitRepository()
		q := unitQuery(e, "denied-query", "topic", "v1", "context", false)
		repo.queries[q.ID] = q
		service := &Service{topics: &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}}, sources: retainedSourceReader{}, validator: &unitValidator{errors: []error{failure}}, repo: repo}
		if err := service.Feedback(context.Background(), e, FeedbackRequest{QueryID: q.ID, Verdict: "negative"}); !errors.Is(err, failure) || len(repo.feedback) != 0 {
			t.Fatal("cancellation/authority failure ignored", err)
		}
	}
}
