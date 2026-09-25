package nlqexec

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func explanationCandidate(t *testing.T, sql string, assumptions, ambiguities []string, parameters ...exec.Parameter) gateway.Generated {
	t.Helper()
	raw, err := json.Marshal(generatedCandidate{Decision: "ready", Questions: []string{}, SQL: sql, Parameters: append([]exec.Parameter{}, parameters...), Assumptions: append([]string{}, assumptions...), Ambiguities: append([]string{}, ambiguities...)})
	if err != nil {
		t.Fatal(err)
	}
	return gateway.Generated{JSON: raw}
}

func TestSQLRecoveryExplanationsDetachedAndEmpty(t *testing.T) {
	c := generatedCandidate{SQL: "SELECT id FROM analytics.sales", Assumptions: []string{"Only reviewed records count.", "La fecha corresponde al calendario revisado."}, Ambiguities: []string{"No tie ordering was requested."}}
	before := exec.Hash(c)
	q := QueryRecord{Assumptions: []string{"generic route note"}, Ambiguities: []string{"old note"}}
	retainGenerationExplanations(&q, c, nil, nil)
	if !reflect.DeepEqual(q.Assumptions, c.Assumptions) || !reflect.DeepEqual(q.Ambiguities, c.Ambiguities) || exec.Hash(c) != before {
		t.Fatal("accepted notes changed or candidate mutated")
	}
	q.Assumptions[0], q.Ambiguities[0] = "changed", "changed"
	if exec.Hash(c) != before {
		t.Fatal("retained notes alias the candidate")
	}
	retainGenerationExplanations(&q, generatedCandidate{}, nil, nil)
	if len(q.Assumptions)+len(q.Ambiguities) != 0 {
		t.Fatal("empty accepted notes inherited stale/generic statements")
	}
}

func TestSQLRecoveryExplanationsRedactKnownBindingsAndAliases(t *testing.T) {
	rawAnswer := "private\u00a0spelling"
	resolution := semantics.ClarificationResolution{Topic: "topic", Pattern: "customer", Slot: "customer", Sensitivity: semantics.LiteralSensitive, Value: "customer-canonical-981", Effect: &semantics.ClarificationEffect{Values: []semantics.GovernedClarificationValue{{Canonical: "customer-canonical-981", Aliases: []string{"customer-alias-981"}}, {Canonical: "another-customer", Aliases: []string{"unrelated-public-description"}}}}}
	answers := []semantics.ClarificationAnswer{{Topic: "topic", Pattern: "customer", Slot: "customer", Value: &semantics.ClarificationValue{Text: &rawAnswer}}}
	q := QueryRecord{Route: nlqroute.RouteResult{Resolutions: []semantics.ClarificationResolution{resolution}}}
	parent := QueryRecord{Parameters: []exec.Parameter{{Kind: "text", Value: "previous-private-772"}}}
	c := generatedCandidate{SQL: "SELECT id FROM analytics.sales", Parameters: []exec.Parameter{{Kind: "text", Value: "parameter-private-492"}}, Assumptions: []string{"Customer-canonical-981 uses CUSTOMER-ALIAS-981 and private\tspelling."}, Ambiguities: []string{"parameter-private-492, previous-private-772, unrelated-public-description"}}
	before := exec.Hash([]any{q, c, answers, parent})
	retainGenerationExplanations(&q, c, answers, &parent)
	joined := strings.Join(append(append([]string{}, q.Assumptions...), q.Ambiguities...), " ")
	for _, secret := range []string{"canonical-981", "alias-981", "spelling", "private-492", "private-772"} {
		if strings.Contains(strings.ToLower(joined), secret) {
			t.Fatalf("known private spelling retained: %s", secret)
		}
	}
	if !strings.Contains(joined, "[redacted answer]") || !strings.Contains(joined, "unrelated-public-description") {
		t.Fatal("redaction either missing or widened to unrelated vocabulary")
	}
	q.Assumptions, q.Ambiguities = nil, nil
	if exec.Hash([]any{q, c, answers, parent}) != before {
		t.Fatal("redaction mutated inputs, resolutions or parent")
	}
}

func TestSQLRecoveryExplanationsRedactionRemainsBounded(t *testing.T) {
	c := generatedCandidate{SQL: "SELECT id FROM analytics.sales", Parameters: []exec.Parameter{{Kind: "text", Value: "x"}}, Assumptions: []string{strings.Repeat("x", maxGenerationExplanationBytes)}, Ambiguities: []string{"[redacted answer]"}}
	q := QueryRecord{}
	retainGenerationExplanations(&q, c, nil, nil)
	if len(q.Assumptions) != 1 || q.Assumptions[0] != "[explanation withheld: redaction exceeds limit]" || q.Ambiguities[0] != "[redacted answer]" {
		t.Fatal("expanded markers overflowed the contract or damaged an existing marker")
	}
	c.Assumptions = []string{string([]byte{0xff})}
	if validCandidate(c) {
		t.Fatal("invalid UTF-8 accepted into persisted explanatory text")
	}
}

func TestSQLRecoveryExplanationsUseAcceptedValidationCandidate(t *testing.T) {
	e := testEnvelope(t)
	a, generation, call, budget := testGeneration(t, e)
	model := &recoveryCapture{sequenceGateway: sequenceGateway{responses: []gateway.Generated{
		explanationCandidate(t, "SELECT missing FROM analytics.sales", []string{"rejected proposal"}, []string{"rejected ambiguity"}),
		explanationCandidate(t, "SELECT id FROM analytics.sales", []string{"accepted correction"}, []string{"accepted caveat"}),
	}}}
	service := &Service{engine: model, validator: &sequenceValidator{errors: []error{exec.ErrUnsafe, nil}}}
	c, fixes, _, _, err := service.generateAndValidate(context.Background(), e, a, generation, call, budget, "")
	if err != nil || fixes != 1 {
		t.Fatal("validation correction", err)
	}
	q := QueryRecord{}
	retainGenerationExplanations(&q, c, nil, nil)
	if !reflect.DeepEqual(q.Assumptions, []string{"accepted correction"}) || !reflect.DeepEqual(q.Ambiguities, []string{"accepted caveat"}) || len(model.requests) != 2 {
		t.Fatal("rejected notes became the accepted interpretation")
	}
	if strings.Contains(model.requests[1].Prompt, "rejected proposal") || strings.Contains(model.requests[1].Prompt, "rejected ambiguity") {
		t.Fatal("model-authored notes became repair instructions")
	}
}

func TestSQLRecoveryExplanationsExecutionCorrectionBoundary(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(map[bool]string{false: "rejected", true: "accepted"}[accepted], func(t *testing.T) {
			e := unitEnvelope(t)
			a, _, _, _ := testGeneration(t, e)
			q := unitQuery(e, "notes-query", "topic", "v1", "context", false)
			q.Generation.Context = a.assembled
			q.Route.Context = &nlqroute.ContextView{Relations: cloneRelations(a.assembled.Relations)}
			q.Assumptions, q.Ambiguities = []string{"original assumption"}, []string{"original caveat"}
			repo := newUnitRepository()
			repo.queries[q.ID] = q
			sql := "SELECT id  FROM analytics.sales"
			if !accepted {
				sql += " WHERE id=1"
			}
			model := &sequenceGateway{responses: []gateway.Generated{explanationCandidate(t, sql, []string{"corrected assumption"}, []string{"corrected caveat"})}}
			x := &unitExecutor{reports: []exec.ExecutionReport{{Attempt: exec.Attempt{Status: "failed", Code: "query_error"}}, unitResult("succeeded")}, errors: []error{exec.ErrQuery}}
			reader := &unitTopicReader{current: map[string]topics.Contract{"topic": unitContract("topic", "v1", "source", "context", "dataset", true, false)}, retained: map[string]topics.Contract{"topic/v1": unitContract("topic", "v1", "source", "context", "dataset", true, true)}}
			s := &Service{topics: reader, sources: retainedSourceReader{}, validator: &unitValidator{}, executor: x, engine: model, repo: repo}
			r, err := s.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "notes-run"})
			if accepted && err != nil || !accepted && !errors.Is(err, ErrUnsafeCorrection) {
				t.Fatal("correction boundary", err)
			}
			want := "original"
			if accepted {
				want = "corrected"
			}
			if !reflect.DeepEqual(r.Assumptions, []string{want + " assumption"}) || !reflect.DeepEqual(r.Ambiguities, []string{want + " caveat"}) || !reflect.DeepEqual(repo.queries[q.ID].Assumptions, r.Assumptions) {
				t.Fatal("execution correction stored notes from an unaccepted candidate")
			}
			calls := x.calls
			r, _ = s.Run(context.Background(), e, RunRequest{QueryID: q.ID, Operation: "notes-run"})
			if x.calls != calls || model.calls != 1 || !reflect.DeepEqual(r.Assumptions, []string{want + " assumption"}) {
				t.Fatal("terminal replay regenerated notes or changed the accepted description")
			}
		})
	}
}

func TestSQLRecoveryExplanationsReadProjectionAndLegacy(t *testing.T) {
	q := QueryRecord{SQL: "SELECT id FROM analytics.sales", Assumptions: []string{"historical statement"}, Ambiguities: []string{"historical caveat"}}
	s := &Service{}
	out := s.runResult(q, exec.ExecutionReport{}, false)
	if out.SQL != "" || !reflect.DeepEqual(out.Assumptions, q.Assumptions) || !reflect.DeepEqual(out.Ambiguities, q.Ambiguities) {
		t.Fatal("retained notes reinterpreted or SQL projection widened")
	}
	out.Assumptions[0], out.Ambiguities[0] = "mutated", "mutated"
	if q.Assumptions[0] != "historical statement" || q.Ambiguities[0] != "historical caveat" {
		t.Fatal("public note slices alias stored metadata")
	}
}

func TestSQLRecoveryExplanationsConcurrentReuse(t *testing.T) {
	c := generatedCandidate{Assumptions: []string{"Private token-273."}, Parameters: []exec.Parameter{{Kind: "text", Value: "token-273"}}}
	parent := QueryRecord{Assumptions: []string{"parent remains historical"}}
	before := exec.Hash([]any{c, parent})
	results := make(chan bool, 8)
	for i := 0; i < cap(results); i++ {
		go func() {
			q := QueryRecord{}
			retainGenerationExplanations(&q, c, nil, &parent)
			results <- len(q.Assumptions) == 1 && q.Assumptions[0] == "Private [redacted answer]."
		}()
	}
	for i := 0; i < cap(results); i++ {
		if !<-results {
			t.Error("concurrent explanation redaction changed its output")
		}
	}
	if exec.Hash([]any{c, parent}) != before {
		t.Fatal("concurrent callers mutated shared candidate/parent state")
	}
}
