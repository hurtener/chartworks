package nlqroute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
)

type applicabilityTestReader struct {
	rows map[string]ApplicabilityRecord
}

func (r applicabilityTestReader) ReadClarificationApplicability(_ context.Context, _ identity.Envelope, id, _ string) (ApplicabilityRecord, error) {
	row, ok := r.rows[id]
	if !ok {
		return ApplicabilityRecord{}, readexec.ErrBinding
	}
	return row, nil
}
func applicabilityWire(t *testing.T, r RouteResult) RouteResult {
	t.Helper()
	raw, _ := json.Marshal(r)
	var out RouteResult
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("route decode")
	}
	return out
}

func TestProtectedApplicabilityCustodyAndTransitions(t *testing.T) {
	const secret = "fixture-private-trigger-827"
	s, engine, answer := protectedReplayFixture(t, secret, false)
	p := s.topics.(*testTopics).contract.Publication
	patterns := s.rules.(*selectionRules).published.Definition.Patterns
	patterns[0].Policy.When.AnyTerms = []string{secret}
	patterns[0].Slots[0].Effect.Values[0].Aliases = append(patterns[0].Slots[0].Effect.Values[0].Aliases, "another-private-trigger-827")
	s.rules = selectionPolicy(t, p, nil, patterns)
	e := testEnvelope(t, true)
	in := RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue for " + secret + " in March 2025", InterpretationAnchor: "2026-09-22"}
	pending, err := s.Route(t.Context(), e, in)
	if err != nil || pending.Clarification == nil || pending.Applicability == nil {
		t.Fatal("producer", err)
	}
	raw, _ := json.Marshal(pending.Applicability)
	if strings.Contains(string(raw), secret) || strings.Contains(pending.Request.Question, secret) {
		t.Fatal("witness retained private text")
	}
	pending, err = pending.BindApplicabilityQuery(e, "query1")
	if err != nil {
		t.Fatal("bind", err)
	}
	if pending.Applicability.OriginalQuestion != applicabilityOriginal(e, in) {
		t.Fatal("original input was not committed by producer")
	}
	if !pending.ApplicabilityWriteValid(e.Tenant(), e.User(), e.Session(), "query1") {
		t.Fatal("valid producer rejected")
	}
	wire := applicabilityWire(t, pending)
	if wire.ApplicabilityWriteValid(e.Tenant(), e.User(), e.Session(), "query1") {
		t.Fatal("JSON manufactured write custody")
	}
	if _, _, err = s.ReplayClarifications(t.Context(), e, wire); err == nil {
		t.Fatal("JSON manufactured replay custody")
	}
	reader := applicabilityTestReader{rows: map[string]ApplicabilityRecord{"query1": {Query: "query1", Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Context: "ctx", Status: "preflight", Revision: 1, Route: wire}}}
	s = s.WithApplicabilityReader(reader)
	next := cloneRouteRequest(pending.Request)
	next.Answers = []semantics.ClarificationAnswer{answer}
	ctx, err := s.WithQueryApplicability(t.Context(), e, "query1", wire, next, "query.plan", "reply")
	if err != nil {
		t.Fatal("authenticated origin", err)
	}
	changed := cloneRouteRequest(next)
	changed.Question = in.Question
	if _, err = s.WithQueryApplicability(t.Context(), e, "query1", wire, changed, "query.plan", "reply"); err != nil {
		t.Fatal("exact original private question lost its protected origin", err)
	}
	changed.Question = "Revenue for another-private-trigger-827 in March 2025"
	if groundedQuestion(changed, []admittedTopic{{id: "topic", rules: s.rules.(*selectionRules).published}}) != next.Question {
		t.Fatal("same sanitized surface fixture")
	}
	if _, err = s.WithQueryApplicability(t.Context(), e, "query1", wire, changed, "query.plan", "reply"); err == nil {
		t.Fatal("different private question with same sanitized surface borrowed origin")
	}
	changed = cloneRouteRequest(next)
	*changed.Answers[0].Value.Text = "another-private-trigger-827"
	if _, err = s.Route(ctx, e, changed); err == nil {
		t.Fatal("changed answer borrowed a previous transition")
	}
	if _, err = s.WithQueryApplicability(t.Context(), e, "wrong_query", wire, next, "query.plan", "reply"); err == nil {
		t.Fatal("wrong query borrowed origin")
	}
	out, err := s.Route(ctx, e, next)
	if err != nil || out.Clarification != nil || out.Context == nil || out.Applicability == nil {
		t.Fatal("protected reply", err, out.Clarification)
	}
	out, err = out.BindApplicabilityQuery(e, "query2")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := out.ResolvedBusinessConstraints()
	if err != nil || len(expected) != 2 {
		t.Fatal("constraints", err)
	}
	if out.Applicability.OriginalQuestion != pending.Applicability.OriginalQuestion {
		t.Fatal("canonical reply replaced original input commitment")
	}
	stored := applicabilityWire(t, out)
	reader.rows["query2"] = ApplicabilityRecord{Query: "query2", Tenant: e.Tenant(), Actor: e.User(), Session: e.Session(), Context: "ctx", Status: "planned", Revision: 1, Route: stored}
	before := engine.embeds
	replayCtx, err := s.WithQueryApplicability(t.Context(), e, "query2", stored, stored.Request, "query.execute", "replay")
	if err != nil {
		t.Fatal("stored read", err)
	}
	constraints, _, err := s.ReplayClarifications(replayCtx, e, stored)
	if err != nil || readexec.Hash(constraints) != readexec.Hash(expected) || engine.embeds != before {
		t.Fatal("zero-model stored replay", err)
	}
	forged := applicabilityWire(t, stored)
	forged.Applicability.Topics[0].Matches[0].Specificity++
	if _, err = s.WithQueryApplicability(t.Context(), e, "query2", forged, forged.Request, "query.execute", "replay"); err == nil {
		t.Fatal("changed witness borrowed read custody")
	}
	forged = applicabilityWire(t, stored)
	forged.Applicability.OriginalQuestion = applicabilityOriginal(e, changed)
	if _, err = s.WithQueryApplicability(t.Context(), e, "query2", forged, changed, "query.plan", "refine"); err == nil {
		t.Fatal("forged original fingerprint borrowed read custody")
	}
	copy, err := s.ReissueApplicabilityForCopy(t.Context(), e, "query2", stored)
	if err != nil {
		t.Fatal("copy reissue", err)
	}
	copy, err = copy.BindApplicabilityQuery(e, "query3")
	if err != nil || !copy.ApplicabilityWriteValid(e.Tenant(), e.User(), e.Session(), "query3") || copy.ApplicabilityWriteValid(e.Tenant(), e.User(), "foreign-session", "query3") || copy.ApplicabilityWriteValid(e.Tenant(), "foreign-actor", e.Session(), "query3") {
		t.Fatal("saved copy binding", err)
	}
	s.topics.(*testTopics).binding.Revision++
	if _, err = s.WithQueryApplicability(t.Context(), e, "query2", stored, stored.Request, "query.execute", "replay"); err == nil || engine.embeds != before {
		t.Fatal("source rotation accepted or invoked model")
	}
}

func TestProtectedMaskIsDetachedConcurrentAndNonSerializable(t *testing.T) {
	const secret = "private-mask-input-972"
	s, _, _ := protectedReplayFixture(t, secret, false)
	out, err := s.Route(t.Context(), testEnvelope(t, true), RouteRequest{Topic: "topic", Context: "ctx", Locale: nlq.LanguageEnglish, Question: "Revenue for " + secret})
	if err != nil {
		t.Fatal(err)
	}
	encoded, marshalErr := json.Marshal(out)
	without := out
	without.protectedRedact = nil
	baseline, baselineErr := json.Marshal(without)
	if marshalErr != nil || baselineErr != nil || string(encoded) != string(baseline) || readexec.Hash(out) != readexec.Hash(without) || strings.Contains(fmt.Sprintf("%+v", out), secret) || strings.Contains(fmt.Sprintf("%#v", out), secret) {
		t.Fatal("private masking state entered serialization, public hashes or formatting")
	}
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if got := out.RedactProtectedText("Use "+secret, nil); strings.Contains(got, secret) {
				t.Error("admitted private mask lost")
			}
		}()
	}
	wg.Wait()
}
