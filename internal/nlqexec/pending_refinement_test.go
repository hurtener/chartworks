package nlqexec

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func pendingRefinementFixture(t *testing.T) (*Service, identity.Envelope, QueryRecord) {
	t.Helper()
	e := unitEnvelope(t)
	q := unitQuery(e, "pending-refine", "topic", "v1", "context", false)
	q.Status, q.SQL, q.RelationScope = "preflight", "", nil
	q.Route.Outcome, q.Route.Context = nlq.StrategyClarify, nil
	q.Route.Clarification = &nlqroute.Clarification{Reason: "required_answers"}
	q.Route.Request = nlqroute.RouteRequest{Topic: q.Topic, Topics: q.Topics, Context: q.Context, Question: q.Question, Locale: q.Locale}
	q.Route.AnswerContext = "pending-form"
	binding, _ := (retainedSourceReader{}).Binding(context.Background(), e, "source", "context")
	q.Route.SourceBindingDigest = exec.Hash(binding)
	reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}}
	return &Service{topics: reader, sources: retainedSourceReader{}}, e, q
}

func TestSQLRecoveryPendingRefinementReadsCurrentScopeWithoutIssuingPlan(t *testing.T) {
	s, e, q := pendingRefinementFixture(t)
	before := QueryLineageDigest(q)
	a, err := s.admissionForQuery(context.Background(), e, q)
	if err != nil || len(a.relationScope) != 1 || a.relationScope[0].Dataset != "dataset" || !reflect.DeepEqual(a.relationScope[0].Columns, []string{"id"}) || !a.binding.Valid() {
		t.Fatal("pending form did not resolve current reviewed scope", err)
	}
	if before != QueryLineageDigest(q) || q.RelationScope != nil || q.SQL != "" || q.Generation.Prompt != "" || q.Analytical != nil {
		t.Fatal("pending parent gained executable evidence or was mutated")
	}
	// Run and learning review retain their old durable-scope fence. Only the
	// pending refinement path can resolve an unplanned form's current scope.
	if _, err := s.currentAdmission(context.Background(), e, q); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("executable-query admission accepted absent scope", err)
	}
	q.SQL, q.Status = "SELECT id FROM analytics.sales", "planned"
	if _, err := s.admissionForQuery(context.Background(), e, q); !errors.Is(err, exec.ErrBinding) {
		t.Fatal("planned query was downgraded to pending scope reconstruction", err)
	}
}

func TestSQLRecoveryPendingRefinementCannotCarryExecutableEvidence(t *testing.T) {
	for name, change := range map[string]func(*QueryRecord){
		"not_pending":        func(q *QueryRecord) { q.Status = "planned" },
		"has_parameters":     func(q *QueryRecord) { q.Parameters = []exec.Parameter{{Kind: "text", Value: "private"}} },
		"has_operation":      func(q *QueryRecord) { q.Operation = "old-run" },
		"has_result":         func(q *QueryRecord) { q.Result = &exec.Result{} },
		"stale":              func(q *QueryRecord) { q.EvidenceStale = true },
		"analytical_version": func(q *QueryRecord) { q.AnalyticalVersion = 5 },
		"analytical_receipt": func(q *QueryRecord) { q.Analytical = &exec.AnalyticalReceipt{} },
		"owned_sql_receipt":  func(q *QueryRecord) { q.Clarification = &ClarificationEvidence{} },
		"generated_prompt":   func(q *QueryRecord) { q.Generation.Prompt = "old prompt" },
		"correction_count":   func(q *QueryRecord) { q.ExecutionFixes = 1 },
		"not_clarification":  func(q *QueryRecord) { q.Route.Outcome = nlq.StrategySingleTopic },
		"missing_form":       func(q *QueryRecord) { q.Route.Clarification = nil },
		"missing_version":    func(q *QueryRecord) { q.TopicVersions = nil },
		"missing_id":         func(q *QueryRecord) { q.ID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			_, e, q := pendingRefinementFixture(t)
			change(&q)
			// Nil readers deliberately catch any I/O before shape checks.
			if _, err := (&Service{}).pendingRefinementAdmission(context.Background(), e, q); !errors.Is(err, exec.ErrBinding) {
				t.Fatal("invalid pending evidence reached resource reads", err)
			}
		})
	}
}

func TestSQLRecoveryPendingRefinementPreservesSourceAndPolicyPins(t *testing.T) {
	for name, change := range map[string]func(*Service, *QueryRecord){
		"scope_substitution": func(_ *Service, q *QueryRecord) {
			q.RelationScope = []exec.RelationScope{{Dataset: "dataset", Columns: []string{"other"}}}
		},
		"binding_substitution": func(_ *Service, q *QueryRecord) { q.Route.SourceBindingDigest = strings.Repeat("f", 64) },
		"topic_version": func(s *Service, _ *QueryRecord) {
			s.topics.(*unitTopicReader).current["topic"] = unitContract("topic", "v2", "source", "context", "dataset", true, false)
		},
		"archived": func(s *Service, _ *QueryRecord) {
			s.topics.(*unitTopicReader).current["topic"] = unitContract("topic", "v1", "source", "context", "dataset", false, true)
		},
		"source_revision": func(s *Service, _ *QueryRecord) {
			b, _ := (retainedSourceReader{}).Binding(context.Background(), identity.Envelope{}, "source", "context")
			b.Revision++
			s.sources = fixedSourceReader{binding: b}
		},
		"context_partition": func(_ *Service, q *QueryRecord) { q.Context = "another-context" },
	} {
		t.Run(name, func(t *testing.T) {
			s, e, q := pendingRefinementFixture(t)
			change(s, &q)
			if _, err := s.admissionForQuery(context.Background(), e, q); err == nil {
				t.Fatal("pending form widened stale or foreign scope")
			}
		})
	}
}

func TestSQLRecoveryPendingRefinementStillRequiresActorActionAndCancellation(t *testing.T) {
	s, e, q := pendingRefinementFixture(t)
	q.Session = "other-session"
	if _, err := s.pendingRefinementAdmission(context.Background(), e, q); !errors.Is(err, ErrForeignSession) {
		t.Fatal(err)
	}
	q.Session = e.Session()
	withoutAction, err := identity.FromVerified(e.Tenant(), e.User(), e.Session(), nil, time.Now().Add(time.Hour), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (&Service{}).pendingRefinementAdmission(context.Background(), withoutAction, q); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("missing action reached I/O", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = (&Service{}).pendingRefinementAdmission(ctx, e, q); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
