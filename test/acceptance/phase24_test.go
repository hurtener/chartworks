package acceptance

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

const evalDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func evalPtr[T any](v T) *T { return &v }
func evalSuite(mode evaluation.Mode, cases []evaluation.Case) evaluation.Suite {
	q := 1.0
	return evaluation.Suite{SchemaVersion: 1, ID: "phase24", Revision: 1, Mode: mode, Seed: 24001, Calibration: "reviewed", Threshold: evaluation.Threshold{QualityMin: &q, Reviewed: true}, Limits: evaluation.Limits{Cases: 100, Calls: 100, Tokens: 10000, Retries: 4, DurationMS: 60000}, Provenance: evaluation.Provenance{Implementation: "acceptance-head", EnvironmentDigest: evalDigest, ConfigurationDigest: evalDigest, SemanticVersion: "semantic-v1", RuleVersion: "rule-v1", TemplateVersion: "template-v1", SourceSnapshot: evalDigest, DialectMatrix: []evaluation.DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: mode, EvidenceDigest: evalDigest, Status: "measured"}, {Engine: "mysql", Dialect: "mysql", Mode: mode, EvidenceDigest: evalDigest, Status: "unknown"}, {Engine: "sqlserver", Dialect: "sqlserver", Mode: mode, EvidenceDigest: evalDigest, Status: "unknown"}, {Engine: "bigquery", Dialect: "bigquery", Mode: mode, EvidenceDigest: evalDigest, Status: "unknown"}, {Engine: "snowflake", Dialect: "snowflake", Mode: mode, EvidenceDigest: evalDigest, Status: "unknown"}, {Engine: "databricks", Dialect: "databricks", Mode: mode, EvidenceDigest: evalDigest, Status: "unknown"}}}, Frontiers: []string{"EVAL-01", "EXP-01", "EXP-03", "EXP-05", "EXP-09", "EXP-10", "EXP-11"}, Cases: cases}
}
func evalCase(id string, stage evaluation.Stage, locale string, critical bool, category string) evaluation.Case {
	o := evaluation.Observation{Decision: "expected", SemanticDigest: evalDigest, Blocked: critical, Usage: evaluation.Usage{ServiceMS: 1, SourceMS: evalPtr(int64(0)), ModelMS: evalPtr(int64(0)), Tokens: evalPtr(0), CostUSD: nil}}
	if critical {
		o.ErrorClass = "blocked"
	}
	return evaluation.Case{ID: id, Stage: stage, Category: category, Locale: locale, Critical: critical, HeldOut: !critical, Input: evaluation.ProtectedRef{Digest: evalDigest, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: o.Decision, SemanticDigest: o.SemanticDigest, ErrorClass: o.ErrorClass}}, Fixture: &o}
}
func evalClock() time.Time { return time.Unix(2400, 0) }

func TestPhase24(t *testing.T) {
	t.Run("AC01", func(t *testing.T) {
		c := evalCase("seeded", evaluation.StageSQL, "en", false, "")
		s := evalSuite(evaluation.Fixture, []evaluation.Case{c})
		r, err := evaluation.Evaluate(context.Background(), "good", s, nil, evalClock)
		if err != nil || !r.GatePassed {
			t.Fatal(r, err)
		}
		s.Cases[0].Fixture.SemanticDigest = strings.Repeat("b", 64)
		r, err = evaluation.Evaluate(context.Background(), "bad", s, nil, evalClock)
		if !errors.Is(err, evaluation.ErrGate) || r.GatePassed {
			t.Fatal("seeded regression did not fail", r, err)
		}
		s = evalSuite(evaluation.Fixture, []evaluation.Case{evalCase("critical", evaluation.StageAdversarial, "en", true, "injection")})
		s.Cases[0].Fixture.Blocked = false
		r, err = evaluation.Evaluate(context.Background(), "critical", s, nil, evalClock)
		if !errors.Is(err, evaluation.ErrGate) || r.SecurityFailures != 1 {
			t.Fatal("security tolerance widened", r, err)
		}
	})

	t.Run("AC02", func(t *testing.T) {
		stages := []evaluation.Stage{evaluation.StageRouting, evaluation.StageContext, evaluation.StageSQL, evaluation.StageValidation, evaluation.StageChart, evaluation.StageReport}
		cases := []evaluation.Case{}
		for i, s := range stages {
			locale := "en"
			if i%2 == 1 {
				locale = "es"
			}
			c := evalCase("golden-"+string(rune('a'+i)), s, locale, false, "")
			c.Expected = append(c.Expected, evaluation.Expected{Decision: "equivalent", SemanticDigest: strings.Repeat("b", 64)})
			if i == 2 {
				c.Fixture.Decision = "equivalent"
				c.Fixture.SemanticDigest = strings.Repeat("b", 64)
			}
			cases = append(cases, c)
		}
		suite := evalSuite(evaluation.Fixture, cases)
		r, err := evaluation.Evaluate(context.Background(), "goldens", suite, nil, evalClock)
		if err != nil || r.QualityPassed != len(stages) || r.SuiteDigest == "" || r.EvidenceHash == "" {
			t.Fatal(r, err)
		}
		if suite.Provenance.SemanticVersion == "" || suite.Provenance.RuleVersion == "" || suite.Seed == 0 {
			t.Fatal("versions not pinned")
		}
	})

	t.Run("AC03", func(t *testing.T) {
		categories := []string{"identity_scope", "injection", "dialect_escape", "resource_exhaustion", "byo", "frozen_report"}
		cases := []evaluation.Case{}
		for i, c := range categories {
			cases = append(cases, evalCase("adversarial-"+c, evaluation.StageAdversarial, []string{"en", "es"}[i%2], true, c))
		}
		r, err := evaluation.Evaluate(context.Background(), "adversarial", evalSuite(evaluation.Fixture, cases), nil, evalClock)
		if err != nil || r.SecurityFailures != 0 || len(r.Cases) != 6 {
			t.Fatal(r, err)
		}
		cases[4].Fixture.Blocked = false
		r, err = evaluation.Evaluate(context.Background(), "adversarial-fail", evalSuite(evaluation.Fixture, cases), nil, evalClock)
		if !errors.Is(err, evaluation.ErrGate) || r.SecurityFailures != 1 {
			t.Fatal("BYO escape was not critical", r, err)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		cases := []evaluation.Case{evalCase("replay", evaluation.StageReplay, "en", false, ""), evalCase("shadow", evaluation.StageShadow, "es", false, "")}
		s := evalSuite(evaluation.Fixture, cases)
		candidate, _ := evaluation.Evaluate(context.Background(), "candidate", s, nil, evalClock)
		baseline := candidate
		baseline.Cases = append([]evaluation.CaseResult(nil), candidate.Cases...)
		baseline.Cases[0].Passed = false
		baseline.QualityPassed--
		baseline.EvidenceHash = strings.Repeat("b", 64)
		p, err := evaluation.ProposeOptimization(s, baseline, candidate, evalDigest, strings.Repeat("c", 64), evalClock())
		if err != nil || p.State != "candidate" {
			t.Fatal(p, err)
		}
		if _, err = evaluation.ReviewPromotion(p, "reviewer", "approve", evalClock()); err != nil {
			t.Fatal(err)
		}
		if _, err = evaluation.ReviewPromotion(p, "optimizer", "publish", evalClock()); !errors.Is(err, evaluation.ErrReview) {
			t.Fatal("optimizer could publish")
		}
	})

	t.Run("AC05", func(t *testing.T) {
		c := evalCase("live", evaluation.StageRouting, "es", false, "")
		c.Fixture = nil
		s := evalSuite(evaluation.Live, []evaluation.Case{c})
		if _, err := evaluation.Evaluate(context.Background(), "live", s, nil, evalClock); !errors.Is(err, evaluation.ErrMode) {
			t.Fatal("live mislabeled as fixture", err)
		}
		runner := evaluation.RunnerFunc(func(context.Context, evaluation.Suite, evaluation.Case) (evaluation.Observation, error) {
			return evaluation.Observation{Decision: "expected", SemanticDigest: evalDigest, Usage: evaluation.Usage{ServiceMS: 7, SourceMS: evalPtr(int64(2)), ModelMS: evalPtr(int64(4)), Calls: 1, Tokens: evalPtr(12)}}, nil
		})
		r, err := evaluation.Evaluate(context.Background(), "live", s, runner, evalClock)
		if err != nil || r.Mode != evaluation.Live || r.Usage.CostUSD != nil || r.Usage.SourceMS == nil || r.Usage.ModelMS == nil {
			t.Fatal("fixture/live or unknown cost conflated", r, err)
		}
		unknown := s
		unknown.Calibration = "unknown"
		unknown.Threshold.QualityMin = nil
		unknown.Threshold.Reviewed = false
		if unknown.Validate() != nil {
			t.Fatal("explicit unknown calibration rejected")
		}
		if r, err = evaluation.Evaluate(context.Background(), "unknown", unknown, runner, evalClock); !errors.Is(err, evaluation.ErrGate) || r.GatePassed {
			t.Fatal("unknown calibration fabricated a pass")
		}
	})

	t.Run("AC06", func(t *testing.T) {
		db := support.Open(t, support.Database(t))
		feedback := evalFeedback{{ID: "feedback-1", Locale: "es", InputDigest: evalDigest, ExpectedDigest: evalDigest, Decision: "expected", SourceBindingDigest: evalDigest}}
		svc, err := evaluation.New(db, feedback, evalClock)
		if err != nil {
			t.Fatal(err)
		}
		e := evalEnvelope(t, true)
		c := evalCase("durable", evaluation.StageConsumer, "en", false, "")
		r, err := svc.Run(context.Background(), e, "durable-run", evalSuite(evaluation.Fixture, []evaluation.Case{c}), nil)
		if err != nil {
			t.Fatal("run not durable", r, err)
		}
		got, err := svc.Read(context.Background(), e, "durable-run")
		if err != nil || got.EvidenceHash != r.EvidenceHash {
			t.Fatal(got, err)
		}
		export, err := svc.ExportFeedback(context.Background(), e, 10)
		if err != nil || export.Status != "candidate" || len(export.Cases) != 1 || export.Cases[0].Fixture != nil || export.EvidenceHash == "" {
			t.Fatal(export, err)
		}
		if _, err = svc.ExportFeedback(context.Background(), evalEnvelope(t, false), 10); err == nil {
			t.Fatal("feedback exported without signed reach")
		}
		if _, err = svc.Read(context.Background(), evalEnvelopeActor(t, "other", true), "durable-run"); !errors.Is(err, store.ErrNotFound) {
			t.Fatal("actor boundary widened", err)
		}
	})
}

type evalFeedback []evaluation.FeedbackEvidence

func (f evalFeedback) ReviewedFeedback(context.Context, store.Scope, int) ([]evaluation.FeedbackEvidence, error) {
	return append([]evaluation.FeedbackEvidence(nil), f...), nil
}
func evalEnvelope(t *testing.T, allowed bool) identity.Envelope {
	return evalEnvelopeActor(t, "actor", allowed)
}
func evalEnvelopeActor(t *testing.T, actor string, allowed bool) identity.Envelope {
	t.Helper()
	scopes := []string{"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant"}
	if allowed {
		scopes = append(scopes, "cw.tenant.export:tenant")
	}
	e, err := identity.FromVerified("tenant", actor, "session", scopes, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}
