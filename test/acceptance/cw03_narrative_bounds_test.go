package acceptance

import (
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
)

// Each case uses a real retained query, durable reservation/checkpoint, the
// production gateway and an HTTP provider fixture. Provider fixtures establish
// enforcement, not live-provider accuracy, token cost or latency guarantees.
func TestCW03NarrativeHardBounds(t *testing.T) {
	f := newCW03SensitivityFixture(t, "reviewed_and_manual_redaction")
	query, topics := newPhase18Service(t, f)
	blocks, err := reporting.New(f.f.db, topics, f.f.s, f.f.validator, f.f.executor, reporting.CaptureFromQueries(query), config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	author := phase27Actor(t, f, f.f.e.User(), phase27Scopes(f.f.e.Tenant()))
	execute := phase27Actor(t, f, f.f.e.User(), phase28Scopes(f.f.e.Tenant()))
	legacy := phase27Definition(t, f, author, "SELECT id, amount FROM analytics.sales ORDER BY id")
	legacy.Outputs[1].Narrative.SchemaVersion = "grounded-narrative-v1"
	legacy.Outputs[1].Narrative.Fields = []string{"amount"}
	legacy.Outputs[1].Narrative.RedactedFields = []string{"id"}
	legacy.Outputs[1].Narrative.MaxTokens = 8192
	d, err := reporting.MigrateDefinition(legacy)
	if err != nil {
		t.Fatal(err)
	}
	model := newGatewayFixture(t, nil)
	runs := phase28RunService(t, f, blocks, f.f.db, model.engine, config.DefaultReportingExecution())
	cases := []struct {
		name   string
		change func(*reporting.Narrative)
		answer string
		mode   string
		code   string
		calls  int64
	}{
		{name: "claims", change: func(n *reporting.Narrative) { n.MaxClaims = 1 }, answer: `{"claims":[{"kind":"value","evidence":["e1"]},{"kind":"value","evidence":["e2"]}]}`, code: "narrative_failed", calls: 1},
		{name: "type", change: func(n *reporting.Narrative) { n.Type = "summary" }, answer: `{"claims":[{"kind":"difference","evidence":["e1","e2"]}]}`, code: "narrative_failed", calls: 1},
		{name: "characters", change: func(n *reporting.Narrative) { n.MaxCharacters = 40 }, answer: `{"claims":[{"kind":"value","evidence":["e1"]}]}`, code: "narrative_failed", calls: 1},
		{name: "rows", change: func(n *reporting.Narrative) { n.MaxRows = 1; n.Type = "comparison" }, answer: `{"claims":[{"kind":"difference","evidence":["e1","e2"]}]}`, code: "narrative_evidence_unavailable", calls: 0},
		{name: "bytes", change: func(n *reporting.Narrative) { n.MaxBytes = 128; n.Type = "comparison" }, answer: `{"claims":[{"kind":"difference","evidence":["e1","e2"]}]}`, code: "narrative_evidence_unavailable", calls: 0},
		{name: "tokens", change: func(n *reporting.Narrative) { n.MaxTokens = 64 }, answer: `{"claims":[{"kind":"value","evidence":["e1"]}]}`, code: "narrative_failed", calls: 0},
		{name: "calls", change: func(n *reporting.Narrative) { n.MaxCalls = 1 }, mode: "error", code: "narrative_failed", calls: 1},
		{name: "time", change: func(n *reporting.Narrative) { n.TimeoutMillis = 100; n.MaxCalls = 1 }, mode: "delay", code: "narrative_failed", calls: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			definition := phase27Copy(t, d)
			n := definition.Outputs[1].Narrative
			tc.change(n)
			state, err := blocks.Create(t.Context(), author, reporting.CreateRequest{ID: "cw03-bound-" + tc.name, Definition: definition})
			if err != nil {
				t.Fatal(err)
			}
			phase27ValidatePublish(t, blocks, author, state)
			model.mode.Store(phase28Chat(t, model.cfg.Roles["narrative"].Model, tc.answer))
			if tc.mode != "" {
				model.mode.Store(tc.mode)
			}
			accepted, err := runs.Admit(t.Context(), execute, state.State.ID, reporting.RunRequest{Key: "cw03-bound-" + tc.name, Outputs: []string{"narrative-main"}, Narrative: true, PartialPolicy: "allow_partial"})
			if err != nil {
				t.Fatal(err)
			}
			before := model.requests.Load()
			started := time.Now()
			done, err := runs.Run(t.Context(), execute, accepted.ID, false)
			if err != nil || done.State != "partial" || len(done.QueryAttempts) != 1 {
				t.Fatal("typed bounded output failure", done, err)
			}
			output, err := runs.Output(t.Context(), execute, done.ID, "narrative-main")
			if err != nil || output.Code != tc.code || output.State != "failed" || model.requests.Load()-before != tc.calls {
				t.Fatal("hard bound bypassed", output, err, model.requests.Load()-before)
			}
			if output.Narrative != nil && (output.Narrative.Text != "" || len(output.Narrative.Claims) != 0 || len(output.Narrative.Evidence) != 0) {
				t.Fatal("invalid claims/values retained as output", output)
			}
			if strings.HasSuffix(tc.code, "evidence_unavailable") {
				if output.ReservedCalls != 0 || output.ReservedTokens != 0 {
					t.Fatal("excluded evidence reserved model budget", output)
				}
			} else if output.ReservedCalls != n.MaxCalls || output.ReservedTokens != n.MaxTokens {
				t.Fatal("provider uncertainty refunded reservations", output)
			}
			if tc.name == "time" && time.Since(started) > 3*time.Second {
				t.Fatal("100ms provider budget did not bound wall time (including persistence allowance)")
			}
			queries := f.f.lookups.Load()
			model.mode.Store("error")
			again, err := runs.RebuildOutput(t.Context(), execute, done.ID, "narrative-main")
			if err != nil || again.Digest != output.Digest || f.f.lookups.Load() != queries || model.requests.Load()-before != tc.calls {
				t.Fatal("failed retained redraw regenerated work", again, err)
			}
		})
	}
}
