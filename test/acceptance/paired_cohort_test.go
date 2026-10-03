package acceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/evaluation"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

const pairedDirectory = "testdata/paired_cohort_v1"

type pairedCase struct {
	adversarialCase
	Locale    string `json:"locale"`
	Contract  string `json:"contract_id"`
	Partition string `json:"partition"`
}
type pairedManifest struct {
	Format       string            `json:"format"`
	Mode         string            `json:"mode"`
	Seed         int64             `json:"seed"`
	SourceCommit string            `json:"source_commit"`
	Sources      map[string]string `json:"source_fixtures_sha256"`
	Files        map[string]string `json:"cohort_files_sha256"`
	Quality      string            `json:"quality_threshold"`
}
type pairedCell struct {
	Arm               string                            `json:"arm"`
	Case              pairedCase                        `json:"case"`
	Actual            string                            `json:"actual"`
	Reason            string                            `json:"reason,omitempty"`
	Matches           bool                              `json:"matches"`
	QueryID           string                            `json:"query_id,omitempty"`
	Operation         string                            `json:"operation,omitempty"`
	PublicationDigest string                            `json:"publication_digest"`
	ResultDigest      string                            `json:"result_digest,omitempty"`
	OracleDigest      string                            `json:"oracle_digest,omitempty"`
	ModelCalls        int64                             `json:"model_calls"`
	ReadAttempts      int64                             `json:"read_attempts"`
	BindingSchema     int                               `json:"binding_schema,omitempty"`
	Learning          string                            `json:"learning"`
	Selection         *nlqexec.ExampleSelectionEvidence `json:"selection,omitempty"`
	ExampleDigests    map[string]string                 `json:"example_digests,omitempty"`
}
type pairedReport struct {
	RepairPolicy                         string                   `json:"repair_policy,omitempty"`
	RepairPassed                         bool                     `json:"repair_passed"`
	BaselinePreserved                    bool                     `json:"baseline_preserved"`
	MonthlyDiagnosticModelCalls          int64                    `json:"post_comparison_monthly_model_calls"`
	MonthlyDiagnosticReadAttempts        int64                    `json:"post_comparison_monthly_read_attempts"`
	MonthlyDiagnosticDiscoveryAccounting string                   `json:"post_comparison_monthly_discovery_accounting,omitempty"`
	GatewayConfigurationDigest           string                   `json:"gateway_configuration_digest"`
	MonthlyDiagnostic                    *nlqexec.PreflightResult `json:"post_comparison_monthly_preflight_diagnostic,omitempty"`
	MonthlyDiagnosticError               string                   `json:"post_comparison_monthly_preflight_error,omitempty"`
	Format                               string                   `json:"format"`
	Mode                                 string                   `json:"mode"`
	ManifestDigest                       string                   `json:"manifest_digest"`
	SourceSnapshot                       string                   `json:"source_snapshot"`
	MeaningDigest                        string                   `json:"meaning_digest"`
	Calibration                          string                   `json:"calibration"`
	GatePassed                           bool                     `json:"gate_passed"`
	Training                             []pairedCell             `json:"training"`
	SharedControls                       []pairedCell             `json:"shared_controls"`
	Cases                                []pairedCell             `json:"cases"`
	Examples                             []nlqexec.ExampleRecord  `json:"examples"`
	PlanReview                           *evaluation.SuiteReview  `json:"synthetic_plan_review,omitempty"`
}

func pairedSHA(raw []byte) string { h := sha256.Sum256(raw); return hex.EncodeToString(h[:]) }
func readPaired(t *testing.T, name string, v any) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(pairedDirectory, name))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, v); e != nil {
		t.Fatal(e)
	}
	return b
}
func pairedSplitValid(training, heldout []pairedCase) bool {
	ids, questions := map[string]bool{}, map[string]bool{}
	for _, set := range [][]pairedCase{training, heldout} {
		for _, c := range set {
			q := pairedSHA([]byte(strings.ToLower(strings.Join(strings.Fields(c.Question), " "))))
			if c.ID == "" || c.Question == "" || ids[c.ID] || questions[q] || c.Locale != "en" && c.Locale != "es" {
				return false
			}
			ids[c.ID] = true
			questions[q] = true
		}
	}
	for _, c := range training {
		if c.Partition != "training" || c.Expected != "answer" || c.Oracle == nil {
			return false
		}
	}
	for _, c := range heldout {
		if c.Partition != "regression_control" && c.Partition != "fresh_paraphrase" {
			return false
		}
	}
	return len(training) == 12 && len(heldout) == 31
}
func pairedFilesValid(m pairedManifest) bool {
	for _, set := range []struct {
		files map[string]string
		names []string
	}{{m.Sources, []string{"warehouse.json", "authoring_inputs.json", "net_business.json", "oracle.json", "held_out.json"}}, {m.Files, []string{"design-source.json", "held_out.json", "training.json"}}} {
		if len(set.files) != len(set.names) {
			return false
		}
		for _, name := range set.names {
			if len(set.files[name]) != 64 {
				return false
			}
		}
	}
	return true
}

func freezePaired(t *testing.T) (pairedManifest, []pairedCase, []pairedCase, map[string]any, string) {
	t.Helper()
	var m pairedManifest
	raw := readPaired(t, "manifest.json", &m)
	if m.Format != "paired-cohort-offline-v1" || m.Mode != "recorded_provider_real_postgresql" || m.Seed != 20261003 || m.Quality != "unknown" {
		t.Fatal("unreviewed cohort identity changed")
	}
	for _, item := range []struct {
		root  string
		files map[string]string
	}{{adversarialCorpusDirectory, m.Sources}, {pairedDirectory, m.Files}} {
		for name, want := range item.files {
			b, e := os.ReadFile(filepath.Join(item.root, name))
			if e != nil || pairedSHA(b) != want {
				t.Fatalf("frozen input %s hash mismatch: %v", name, e)
			}
		}
	}
	if !pairedFilesValid(m) {
		t.Fatal("incomplete frozen lineage")
	}
	var training, heldout []pairedCase
	readPaired(t, "training.json", &training)
	readPaired(t, "held_out.json", &heldout)
	if !pairedSplitValid(training, heldout) {
		t.Fatal("training/held-out partition overlaps or is incomplete")
	}
	var w adversarialWarehouse
	readAdversarialJSON(t, "warehouse.json", &w)
	oracle := adversarialOracle(t, w)
	var expected map[string]any
	readAdversarialJSON(t, "oracle.json", &expected)
	for k, v := range oracle {
		a, _ := json.Marshal(v)
		b, _ := json.Marshal(expected[k])
		if string(a) != string(b) {
			t.Fatal("source-row oracle drift", k)
		}
	}
	var design struct {
		New []map[string]any `json:"new_question_cases"`
	}
	readPaired(t, "design-source.json", &design)
	for _, c := range design.New {
		a, _ := json.Marshal(c["expected"])
		b, _ := json.Marshal(oracle[c["oracle_key"].(string)])
		if string(a) != string(b) {
			t.Fatal("predeclared paraphrase oracle drift", c["id"])
		}
	}
	return m, training, heldout, oracle, pairedSHA(raw)
}
func TestPairedCohortFrozenLineage(t *testing.T) {
	m, training, heldout, oracle, _ := freezePaired(t)
	delete(m.Files, "training.json")
	m.Files["substituted.json"] = strings.Repeat("a", 64)
	if pairedFilesValid(m) {
		t.Fatal("unbound training file accepted via same-size substitution")
	}
	leaked := append([]pairedCase(nil), training...)
	leaked[0].Question = heldout[0].Question
	if pairedSplitValid(leaked, heldout) {
		t.Fatal("leaked held-out question admitted to training")
	}
	leaked = append([]pairedCase(nil), training...)
	leaked[0].ID = heldout[0].ID
	if pairedSplitValid(leaked, heldout) {
		t.Fatal("overlapping case identity admitted")
	}
	if liveNumberEquals(oracle["known_gross_paid_local_2026"].(string), "994.00") {
		t.Fatal("deliberate failing numeric oracle became green")
	}
	if adversarialMonthsMatch([][]json.RawMessage{{json.RawMessage(`"2026-06"`), json.RawMessage(`0`)}}, map[string]any{"2026-06": nil}) {
		t.Fatal("NULL/zero negative oracle became green")
	}
}

func pairedSnapshot(t *testing.T, h *generatedTopicHarness) string {
	t.Helper()
	tables := []string{"adv_customers", "adv_orders", "adv_refunds", "adv_order_lines"}
	parts := []string{}
	for _, table := range tables {
		var raw string
		if err := h.f.admin.QueryRow(t.Context(), "SELECT COALESCE(jsonb_agg(r ORDER BY r::text),'[]'::jsonb)::text FROM (SELECT to_jsonb(t) AS r FROM analytics."+table+" t) s").Scan(&raw); err != nil {
			t.Fatal(err)
		}
		parts = append(parts, raw)
	}
	return readexec.Hash(parts)
}
func pairedMetricsSQL(c pairedCase, ids map[string]string) ([]string, string) {
	gross := semantics.GeneratedEntityID(semantics.EnhancementMeasure, ids["orders"], "misleading_net_total_usd")
	metrics := []string{gross}
	sql := "SELECT SUM(misleading_net_total_usd) AS known_gross,COUNT(order_id)-COUNT(misleading_net_total_usd) AS unknown_amounts FROM analytics.adv_orders WHERE status_code='P'"
	switch c.Contract {
	case "month-en", "month-es":
		sql = "SELECT date_trunc('month',ordered_at,'America/New_York') AS order_month,SUM(misleading_net_total_usd) AS known_gross,COUNT(order_id)-COUNT(misleading_net_total_usd) AS unknown_amounts FROM analytics.adv_orders WHERE status_code='P' GROUP BY date_trunc('month',ordered_at,'America/New_York') ORDER BY order_month"
	case "count-orders":
		metrics = []string{drafts.GeneratedCountMeasureID(ids["orders"], "order_id")}
		sql = "SELECT COUNT(*) AS paid_orders FROM analytics.adv_orders WHERE status_code='P'"
	case "count-known":
		metrics = []string{drafts.GeneratedCountMeasureID(ids["orders"], "misleading_net_total_usd")}
		sql = "SELECT COUNT(misleading_net_total_usd) AS known_paid_amounts FROM analytics.adv_orders WHERE status_code='P'"
	case "cohort-known-net":
		metrics = []string{"known_cohort_net", "unknown_cohort_refund_amounts"}
		sql = adversarialNetSQL(false)
		if c.Partition == "fresh_paraphrase" {
			metrics = []string{"known_cohort_net", "unknown_order_amounts", "unknown_cohort_refund_amounts"}
			sql = adversarialNetSQL(true)
		}
	case "activity-known-net":
		metrics = []string{"known_activity_net", "unknown_order_amounts", "unknown_activity_refund_amounts"}
		sql = adversarialNetSQL(true)
	case "net-ambiguous-en", "net-ambiguous-es", "nullable-refund-net", "dst-fold":
		metrics = nil
	case "unknown-status":
		sql = "SELECT SUM(misleading_net_total_usd) AS gross FROM analytics.adv_orders WHERE status_code='X'"
	case "single-key-join":
		sql = "SELECT SUM(o.misleading_net_total_usd) AS gross FROM analytics.adv_orders o JOIN analytics.adv_customers c ON c.customer_id=o.customer_id WHERE o.status_code='P'"
	case "child-fanout":
		sql = "SELECT SUM(o.misleading_net_total_usd) AS gross FROM analytics.adv_orders o JOIN analytics.adv_order_lines l ON (l.division_id,l.order_id)=(o.division_id,o.order_id) JOIN analytics.adv_refunds r ON (r.division_id,r.order_id)=(o.division_id,o.order_id) WHERE o.status_code='P'"
	}
	return metrics, sql
}
func pairedAnswerMatches(c pairedCase, run nlqexec.RunResult, topic, gross string, oracle map[string]any) bool {
	if run.Execution.Result == nil || c.Oracle == nil {
		return false
	}
	rows := run.Execution.Result.Rows
	if strings.HasPrefix(c.Contract, "count-") {
		return liveSingleNumericEquals(rows, fmt.Sprint(oracle[*c.Oracle]))
	}
	if c.Contract == "cohort-known-net" || c.Contract == "activity-known-net" {
		want := []string{oracle[*c.Oracle].(string)}
		if c.Contract == "activity-known-net" || c.Partition == "fresh_paraphrase" {
			want = append(want, fmt.Sprint(oracle["unknown_amount_paid_orders"]))
		}
		key := "unknown_posted_refund_events_in_2026_paid_cohort"
		if c.Contract == "activity-known-net" {
			key = "unknown_posted_refund_events_on_paid_orders_by_2026_activity"
		}
		want = append(want, fmt.Sprint(oracle[key]))
		if len(rows) != 1 || len(rows[0]) != len(want) {
			return false
		}
		for i, v := range want {
			if !liveSingleNumericEquals([][]json.RawMessage{{rows[0][i]}}, v) {
				return false
			}
		}
		return true
	}
	evidence, ok := adversarialAmountEvidence(run, topic+":measure:"+gross)
	if !ok || evidence.Status != "incomplete" || len(evidence.Rows) != len(rows) {
		return false
	}
	if strings.HasPrefix(c.Contract, "gross") {
		return len(rows) == 1 && evidence.ValueColumn < len(rows[0]) && evidence.Rows[0].UnknownCount == fmt.Sprint(oracle["unknown_amount_paid_orders"]) && liveSingleNumericEquals([][]json.RawMessage{{rows[0][evidence.ValueColumn]}}, oracle[*c.Oracle].(string))
	}
	if strings.HasPrefix(c.Contract, "month") {
		projected := [][]json.RawMessage{}
		for i, row := range rows {
			if len(row) == 0 || evidence.ValueColumn >= len(row) {
				return false
			}
			var month string
			if json.Unmarshal(row[0], &month) != nil || len(month) < 7 {
				return false
			}
			want, exists := oracle["monthly_unknown_paid_amounts_local_2026"].(map[string]int)[month[:7]]
			if !exists || evidence.Rows[i].Row != i || evidence.Rows[i].UnknownCount != fmt.Sprint(want) {
				return false
			}
			status := "complete"
			if want > 0 {
				status = "incomplete"
			}
			if evidence.Rows[i].Status != status {
				return false
			}
			projected = append(projected, []json.RawMessage{row[0], row[evidence.ValueColumn]})
		}
		return adversarialMonthsMatch(projected, oracle[*c.Oracle].(map[string]any))
	}
	return false
}
func runPairedCase(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, current drafts.Version, ids map[string]string, arm string, c pairedCase, oracle map[string]any) (out pairedCell, query nlqexec.QueryRecord) {
	t.Helper()
	out = pairedCell{Arm: arm, Case: c, PublicationDigest: current.Metadata.Digest, Actual: "not_completed", Learning: "no_examples", Operation: "paired-" + strings.ToLower(arm) + "-" + c.ID}
	metadata := support.Raw(t, h.f.dsn)
	defer metadata.Close(t.Context())
	beforeReads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
	beforeCalls := model.requests.Load()
	model.mu.Lock()
	wireStart := len(model.requestBodies)
	model.mu.Unlock()
	defer func() {
		out.ModelCalls = model.requests.Load() - beforeCalls
		// A failed Plan has no final actual-use receipt. Dispatch is observable,
		// but absence of that receipt is not evidence that examples were unused.
		if arm == "C" && out.QueryID == "" {
			out.Learning = "learning_not_reached"
			if out.ModelCalls > 0 {
				out.Learning = "usage_unavailable_after_plan_failure"
			}
		}
		out.ReadAttempts = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) - beforeReads
		if c.Expected != "answer" && out.ReadAttempts != 0 {
			out.Matches = false
			out.Reason += "; refusal executed source query"
		}
	}()
	metrics, sql := pairedMetricsSQL(c, ids)
	model.mode.Store(phase18RawResponse(t, sql))
	policy := ""
	if arm == "D" {
		policy = nlqroute.GroundedCalendarPolicy
	}
	p, err := h.query.Plan(t.Context(), h.queryActor, nlqexec.PlanRequest{Operation: out.Operation, QuestionRequest: nlqexec.QuestionRequest{InterpretationPolicy: policy, Topic: current.Pack.Topic, Topics: []string{current.Pack.Topic}, Context: current.Pack.Datasets[0].Source.Context, Locale: nlq.Language(c.Locale), Question: c.Question, MetricIDs: metrics, Kinds: []string{"measure", "kpi", "dimension"}, LimitPerKind: 5, Rerank: true}})
	if err != nil {
		out.Actual = "reject"
		out.Reason = err.Error()
		var analytical *readexec.AnalyticalError
		out.Matches = c.Expected == "reject" && errors.As(err, &analytical)
		var clarify *nlqroute.Clarification
		if errors.As(err, &clarify) {
			out.Actual = "clarify"
			out.Reason = clarify.Reason
			reasons := map[string]string{"net-ambiguous-en": "ambiguous_metric_meaning", "net-ambiguous-es": "ambiguous_metric_meaning", "nullable-refund-net": "unknown_amount_policy_required", "definitive-gross-unknown": "unknown_amount_policy_required", "dst-fold": "ambiguous_local_time"}
			out.Matches = c.Expected == "clarify" && reasons[c.Contract] != "" && clarify.Reason == reasons[c.Contract]
		}
		return
	}
	out.QueryID = p.QueryID
	scope, _ := store.NewScope(h.queryActor.Tenant(), h.queryActor.User())
	query, err = h.f.db.ReadQuery(t.Context(), scope, p.QueryID)
	if err != nil {
		t.Fatal(err)
	}
	out.Selection = &query.ExampleSelection
	if query.ExampleSelection.Usage != nil {
		model.mu.Lock()
		bodies := append([]string(nil), model.requestBodies[wireStart:]...)
		model.mu.Unlock()
		for _, used := range query.ExampleSelection.Usage.Used {
			text := ""
			for _, instruction := range query.Generation.Selected {
				if instruction.Key == "learned-"+used.ExampleID {
					text = instruction.Text
				}
			}
			if text == "" || !strings.Contains(query.Generation.Prompt, text) || !pairedWireContains(bodies, text) {
				t.Fatal("used receipt did not reach actual gateway prompt")
			}
		}
		for _, omitted := range query.ExampleSelection.Usage.Omitted {
			for _, instruction := range query.Generation.Selected {
				if instruction.Key == "learned-"+omitted.ExampleID {
					t.Fatal("omitted receipt actually rendered")
				}
			}
		}
	}
	if query.Clarification != nil {
		out.BindingSchema = query.Clarification.Binding.SchemaVersion
	}
	if model.requests.Load() == beforeCalls {
		out.Reason = "fresh Plan did not dispatch model; reuse is not a measurement"
		return
	}
	if arm == "C" {
		out.Learning = "no_eligible_examples"
		if len(query.ExampleSelection.Selected) > 0 {
			out.Learning = "eligible_but_not_used"
		}
		if out.BindingSchema == 6 {
			out.Learning = "unsupported_schema6"
		}
		if query.ExampleSelection.Usage != nil && len(query.ExampleSelection.Usage.Used) > 0 {
			out.Learning = "used"
		}
		out.ExampleDigests = map[string]string{}
		for _, selected := range query.ExampleSelection.Selected {
			record, e := h.f.db.ReadExample(t.Context(), scope, selected.ExampleID)
			if e != nil {
				t.Fatal(e)
			}
			out.ExampleDigests[selected.ExampleID] = record.Digest
		}
	}
	if arm != "C" && (len(query.ExampleSelection.Selected) > 0 || query.ExampleSelection.Usage != nil && len(query.ExampleSelection.Usage.Used) > 0) {
		t.Fatal("baseline contaminated with learning", arm)
	}
	run, err := h.query.Run(t.Context(), h.queryActor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: out.Operation + "-run", Rows: 20, Bytes: 65536})
	if err != nil || run.Execution.Result == nil {
		out.Actual = "execution_failed"
		out.Reason = fmt.Sprint(err)
		return
	}
	out.ResultDigest = readexec.Hash([]any{run.Execution.Result.Rows, run.AmountCompleteness})
	if c.Oracle != nil {
		out.OracleDigest = readexec.Hash(oracle[*c.Oracle])
	}
	out.Actual = "answer_mismatch"
	gross := semantics.GeneratedEntityID(semantics.EnhancementMeasure, ids["orders"], "misleading_net_total_usd")
	if c.Expected == "answer" && pairedAnswerMatches(c, run, current.Pack.Topic, gross, oracle) {
		// Deliberate wrong-oracle overlays test the exact scoring path against
		// actual native rows without mutating frozen expected material.
		if c.Contract == "gross-en" || c.Contract == "gross-es" {
			bad := map[string]any{}
			for k, v := range oracle {
				bad[k] = v
			}
			bad[*c.Oracle] = "994.00"
			if pairedAnswerMatches(c, run, current.Pack.Topic, gross, bad) {
				t.Fatal("scoring accepted wrong native-result oracle")
			}
		}
		if c.Contract == "month-en" || c.Contract == "month-es" {
			bad := map[string]any{}
			for k, v := range oracle {
				bad[k] = v
			}
			months := map[string]any{}
			for k, v := range oracle[*c.Oracle].(map[string]any) {
				months[k] = v
			}
			months["2026-06"] = "0.00"
			bad[*c.Oracle] = months
			if pairedAnswerMatches(c, run, current.Pack.Topic, gross, bad) {
				t.Fatal("scoring accepted NULL-to-zero mutation")
			}
		}
		out.Actual = "answer"
		out.Matches = true
	}
	return
}

// This is fixture-only registration of a frozen, externally uncalibrated plan.
// No expected result is derived from an observed response. Not-run observations
// cannot be a passing gate, and the real service refuses unknown calibration.
func registerPairedPlan(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, m pairedManifest, training, heldout []pairedCase, manifestDigest string) *evaluation.SuiteReview {
	t.Helper()
	svc, err := evaluation.New(h.f.db, nil, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	cases := []evaluation.Case{}
	for _, c := range append(append([]pairedCase(nil), training...), heldout...) {
		category := c.Partition
		if c.Expected == "reject" {
			category = "identity_scope"
			if c.Contract == "unknown-status" || c.Contract == "single-key-join" || c.Contract == "child-fanout" {
				category = "injection"
			}
		}
		cases = append(cases, evaluation.Case{ID: c.ID, Stage: evaluation.StageSQL, Category: category, Locale: c.Locale, Critical: c.Expected == "reject", HeldOut: c.Partition == "fresh_paraphrase", Input: evaluation.ProtectedRef{Digest: pairedSHA([]byte(c.Question)), Retention: "frozen_synthetic_cohort"}, Expected: []evaluation.Expected{{Decision: c.Expected, SemanticDigest: readexec.Hash(c)}}, Fixture: &evaluation.Observation{Decision: "not_run", ErrorClass: "not_run"}})
	}
	s := evalSuite(evaluation.Fixture, cases)
	s.ID = "paired-frozen-plan"
	s.Seed = m.Seed
	s.Calibration = "unknown"
	s.Threshold.QualityMin = nil
	s.Provenance.Implementation = m.SourceCommit
	s.Provenance.SourceSnapshot = m.Sources["warehouse.json"]
	s.Provenance.ConfigurationDigest = readexec.Hash(model.cfg)
	s.Provenance.EnvironmentDigest = readexec.Hash([]string{runtime.Version(), runtime.GOOS, runtime.GOARCH, "recorded_provider_real_postgresql", "nativepinnedlibs"})
	pack := evaluation.PackRevision{ID: "paired-recorded-gateway", Revision: 1, Model: model.cfg.Roles["sqlgen"].Model, ConfigurationDigest: s.Provenance.ConfigurationDigest}
	for _, role := range config.RoleNames() {
		if r := model.cfg.Roles[role]; r.Enabled {
			pack.Models = append(pack.Models, evaluation.PackModel{Role: role, Model: r.Model})
		}
	}
	pack.Digest = pack.CanonicalDigest()
	s.Packs = []evaluation.PackRevision{pack}
	s.Provenance.SemanticVersion = "predeclared_business_contract"
	s.Provenance.DialectMatrix = []evaluation.DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: evaluation.Fixture, EvidenceDigest: manifestDigest, Status: "unknown"}}
	author := h.f.token.envelope(t, h.f.e.Tenant(), h.f.e.User(), "ops.write", "ops.read", "ops.audit", "cw.tenant.write:"+h.f.e.Tenant(), "cw.tenant.read:"+h.f.e.Tenant(), "cw.tenant.certify:"+h.f.e.Tenant())
	reviewer := h.f.token.envelope(t, h.f.e.Tenant(), "paired-plan-reviewer", "ops.audit", "cw.tenant.certify:"+h.f.e.Tenant())
	draft, err := svc.Author(t.Context(), author, s)
	if err != nil {
		t.Fatal("frozen plan author", err)
	}
	if _, err = svc.Review(t.Context(), author, s.ID, evaluation.SuiteReviewRequest{Revision: s.Revision, Digest: draft.Digest, Decision: evaluation.Accepted}); err == nil {
		t.Fatal("suite author self-approved")
	}
	accepted, err := svc.Review(t.Context(), reviewer, s.ID, evaluation.SuiteReviewRequest{Revision: s.Revision, Digest: draft.Digest, Decision: evaluation.Accepted})
	if err != nil || accepted.Review == nil || accepted.Review.Reviewer == accepted.Author {
		t.Fatal("distinct fixture plan review", err)
	}
	if _, err = svc.Run(t.Context(), author, evaluation.RunRequest{RunID: "paired-unknown-must-refuse", SuiteID: s.ID, SuiteRevision: 1, SuiteDigest: draft.Digest, PackDigest: s.Packs[0].Digest}, nil); !errors.Is(err, evaluation.ErrReview) {
		t.Fatal("unknown calibration was executable as a reviewed quality gate", err)
	}
	t.Logf("PAIRED_FROZEN_REVIEW suite=%s digest=%s author=%s reviewer=%s calibration=unknown executable_gate=false", s.ID, accepted.Digest, accepted.Author, accepted.Review.Reviewer)
	return accepted.Review
}

func pairedReviewTraining(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, cell *pairedCell, q nlqexec.QueryRecord, trainingIDs map[string]bool) []nlqexec.ExampleRecord {
	t.Helper()
	if !trainingIDs[cell.Case.ID] || cell.Case.Partition != "training" {
		t.Fatal("non-training query reached feedback writer")
	}
	if q.ID != "" && (q.ID != cell.QueryID || q.Question != cell.Case.Question || string(q.Locale) != cell.Case.Locale) {
		t.Fatal("training feedback does not match the frozen query identity")
	}
	if !cell.Matches || q.ID == "" {
		cell.Learning = "training_result_failed"
		return nil
	}
	before, err := h.query.Examples(t.Context(), h.queryActor, q.Topic, nlq.MaxExamples+1)
	if err != nil {
		t.Fatal(err)
	}
	if err = h.query.Feedback(t.Context(), h.queryActor, nlqexec.FeedbackRequest{QueryID: q.ID, Verdict: "positive", Note: "Frozen training-only recorded case matched its independent source-row oracle"}); err != nil {
		t.Fatal("training feedback", err)
	}
	examples, err := h.query.Examples(t.Context(), h.queryActor, q.Topic, nlq.MaxExamples+1)
	if err != nil {
		t.Fatal(err)
	}
	if cell.BindingSchema == 6 {
		if readexec.Hash(examples) != readexec.Hash(before) {
			t.Fatal("schema6 feedback mutated an unsupported learning producer")
		}
		cell.Learning = "unsupported_schema6"
		return nil
	}
	reviewer := h.f.token.envelope(t, h.f.e.Tenant(), "paired-example-reviewer", phase18Scopes(h.f.e.Tenant(), true)...)
	active := []nlqexec.ExampleRecord{}
	for _, example := range examples {
		if example.State != "candidate" {
			continue
		}
		calls := model.requests.Load()
		approved, e := h.query.ExampleState(t.Context(), reviewer, nlqexec.ExampleStateRequest{ExampleID: example.ID, State: "active", ExpectedVersion: example.Version, ReviewNote: "Synthetic reviewer accepts the exact value-free native-validated base from the predeclared training partition"})
		if e != nil {
			t.Fatal("independent example review", e)
		}
		if approved.State != "active" || approved.ReviewedBy != reviewer.User() || approved.ReviewedBy == h.queryActor.User() || model.requests.Load() != calls {
			t.Fatal("example review provenance or zero-model boundary")
		}
		active = append(active, approved)
	}
	cell.Learning = "reviewed_training_example"
	if len(examples) == len(before) {
		cell.Learning = "no_new_training_example"
	}
	return active
}

func pairedAuthorityCells(t *testing.T, h *generatedTopicHarness, model *gatewayFixture, current drafts.Version, ids map[string]string, arm string, heldout []pairedCase, stale bool) []pairedCell {
	t.Helper()
	out := []pairedCell{}
	for _, c := range heldout {
		if c.Contract != "sensitive-note" && c.Contract != "wrong-context" && c.Contract != "stale-profile" {
			continue
		}
		if (c.Contract == "stale-profile") != stale {
			continue
		}
		cell := pairedCell{Arm: arm, Case: c, Actual: "reject", PublicationDigest: current.Metadata.Digest, Learning: "not_applicable"}
		before := model.requests.Load()
		switch c.Contract {
		case "sensitive-note":
			var w adversarialWarehouse
			readAdversarialJSON(t, "warehouse.json", &w)
			model.mu.Lock()
			wire := strings.Join(model.requestBodies, "\n")
			model.mu.Unlock()
			cell.Matches = !strings.Contains(wire, w.PrivateNote)
			cell.Reason = "private raw note absent from every recorded request"
		case "wrong-context":
			scopes := topicScopes(h.f.e.Tenant())
			for i, s := range scopes {
				if strings.HasPrefix(s, "cw.execution_context.use:") {
					scopes[i] = "cw.execution_context.use:unrelated:v1"
				}
			}
			wrong := h.f.token.envelope(t, h.f.e.Tenant(), h.f.e.User(), scopes...)
			_, _, err := h.drafts.PrepareFeedbackVocabulary(t.Context(), wrong, current.Pack.Topic, current.Metadata.Revision, generatedVocabularyFromPack(current.Pack, ids))
			cell.Matches = (errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) || errors.Is(err, store.ErrNotFound)) && model.requests.Load() == before
			cell.Reason = "wrong-context vocabulary rejected before model dispatch"
		case "stale-profile":
			if !stale {
				continue
			}
			_, err := h.topics.Review(t.Context(), h.author, current.Pack.Topic, topics.ReviewRequest{DraftRevision: current.Metadata.Revision, Digest: current.Metadata.Digest, Decision: "approve", Note: "Synthetic stale-profile rejection control"})
			cell.Matches = errors.Is(err, readexec.ErrBinding) && model.requests.Load() == before
			cell.Reason = "shared generated-advisory lifecycle control: changed active profile rejects B approval; manual freshness is not measured"
		}
		cell.ModelCalls = model.requests.Load() - before
		out = append(out, cell)
	}
	return out
}

// TestPairedCohortRecorded asserts the pinned failed quality evaluation; the
// original all-answer red source and report remain immutable at c15e64b.
func TestPairedCohortRecorded(t *testing.T) { runPairedCohortRecorded(t, false) }

// This is a producer-repair regression, not a retroactive A/B/C quality gate.
// Every original expected outcome and actual mismatch remains in the report.
func TestPairedCohortGroundedCalendarV2(t *testing.T) { runPairedCohortRecorded(t, true) }

func runPairedCohortRecorded(t *testing.T, repair bool) {
	m, training, heldout, oracle, digest := freezePaired(t) // Before constructing the gateway.
	report := pairedReport{Format: "paired-cohort-report-v1", Mode: m.Mode, ManifestDigest: digest, Calibration: "unknown", GatePassed: false}
	if repair {
		report.RepairPolicy = nlqroute.GroundedCalendarPolicy
	}
	check := func(cell pairedCell) bool {
		if cell.Arm == "D" || cell.Arm == "shared" {
			return cell.Matches
		}
		return pairedPredecessorPreserved(cell)
	}
	defer func() {
		raw, e := json.Marshal(report)
		if e != nil {
			t.Error(e)
			return
		}
		t.Log("PAIRED_COHORT_REPORT " + string(raw))
	}()
	model := newGatewayFixture(t, func(c *config.Gateway) {
		recordedLiveChatCaps(c)
		r := c.Roles["embedding"]
		r.MaxBatchItems = 128
		r.MaxBatchBytes = 128 << 10
		c.Roles["embedding"] = r
	})
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	h, ids, vocabulary := newAdversarialTopicHarness(t, model.engine, true)
	if model.requests.Load() != 0 {
		t.Fatal("gateway invoked before frozen split review")
	}
	report.PlanReview = registerPairedPlan(t, h, model, m, training, heldout, digest)
	report.GatewayConfigurationDigest = readexec.Hash(model.cfg)
	report.SourceSnapshot = pairedSnapshot(t, h)
	manualPack := pairedManualPack(t, h.scaffold.Pack, ids, vocabulary)
	manual, err := h.client.SaveTopicDraft(t.Context(), sdk.SaveTopicDraftRequest{Pack: manualPack, Change: "Independently authored manual A from the frozen physical and business contract"})
	if err != nil {
		t.Fatal("manual A draft", err)
	}
	review, err := h.client.ReviewTopic(t.Context(), manualPack.Topic, sdk.TopicReviewRequest{DraftRevision: manual.Metadata.Revision, Digest: manual.Metadata.Digest, Decision: "approve", Note: "Synthetic owner independently checks manual meanings against the frozen business contract; no generated advisory or examples"})
	if err != nil {
		t.Fatal("manual A review", err)
	}
	if _, err = h.client.PublishTopic(t.Context(), manualPack.Topic, sdk.PublishTopicRequest{Review: review.ID}); err != nil {
		t.Fatal("manual A publication", err)
	}
	a, err := h.drafts.Read(t.Context(), h.author, manual.Pack.Topic, manual.Metadata.Revision)
	if err != nil {
		t.Fatal(err)
	}
	_, _, b, _ := generateAdversarialPublishedFromHarness(t, h, model, ids, vocabulary, true)
	report.MeaningDigest = pairedMeaningDigest(a.Pack)
	if report.MeaningDigest != pairedMeaningDigest(b.Pack) {
		rawA, _ := json.Marshal(pairedMeaning(a.Pack))
		rawB, _ := json.Marshal(pairedMeaning(b.Pack))
		t.Logf("PAIRED_MEANING_A %s", rawA)
		t.Logf("PAIRED_MEANING_B %s", rawB)
		t.Fatal("manual/generated enforced semantic contracts are not equivalent")
	}
	publications := map[string]drafts.Version{"A": a, "B": b, "C": b, "D": b}
	seenQueries := map[string]bool{}
	runArm := func(arm string) {
		t.Helper()
		for _, c := range heldout {
			if c.Contract == "stale-profile" || c.Contract == "sensitive-note" || c.Contract == "wrong-context" {
				continue
			}
			cell, _ := runPairedCase(t, h, model, publications[arm], ids, arm, c, oracle)
			if cell.QueryID != "" {
				if seenQueries[cell.QueryID] {
					t.Fatal("paired arms reused a retained query")
				}
				seenQueries[cell.QueryID] = true
			}
			report.Cases = append(report.Cases, cell)
			if !check(cell) {
				t.Errorf("arm %s case %s: want %s got %s (%s)", arm, c.ID, c.Expected, cell.Actual, cell.Reason)
			}
		}
	}
	runArm("A")
	runArm("B")
	if repair {
		runArm("D")
	}
	trainingIDs := map[string]bool{}
	for _, c := range training {
		trainingIDs[c.ID] = true
	}
	trainingQueries := []nlqexec.QueryRecord{}
	for _, c := range training {
		cell, q := runPairedCase(t, h, model, b, ids, "TRAIN", c, oracle)
		if q.ID != "" {
			if seenQueries[q.ID] {
				t.Fatal("training reused evaluation query")
			}
			seenQueries[q.ID] = true
		}
		if !check(cell) {
			t.Errorf("training %s: expected %s got %s (%s)", c.ID, c.Expected, cell.Actual, cell.Reason)
		}
		trainingQueries = append(trainingQueries, q)
		report.Training = append(report.Training, cell)
	}
	for i := range report.Training {
		report.Examples = append(report.Examples, pairedReviewTraining(t, h, model, &report.Training[i], trainingQueries[i], trainingIDs)...)
	}
	runArm("C")
	lineage := map[string]string{}
	for _, x := range report.Examples {
		lineage[x.ID] = x.Digest
	}
	consumed := false
	for _, cell := range report.Cases {
		if cell.Arm != "C" || cell.Selection == nil || cell.Selection.Usage == nil {
			continue
		}
		for _, used := range cell.Selection.Usage.Used {
			if lineage[used.ExampleID] == "" || lineage[used.ExampleID] != cell.ExampleDigests[used.ExampleID] {
				t.Fatal("used example is outside reviewed training lineage")
			}
			if cell.BindingSchema == 6 {
				t.Fatal("schema6 consumed unsupported learning")
			}
		}
		if cell.Case.ID == "cohort-known-net" && cell.Matches && len(cell.Selection.Usage.Used) > 0 {
			consumed = true
		}
	}
	if !consumed {
		t.Error("predeclared supported cohort-net control did not actually consume reviewed training learning")
	}
	if pairedSnapshot(t, h) != report.SourceSnapshot {
		t.Fatal("source rows changed between paired arms")
	}
	after, err := h.client.PublishedTopic(t.Context(), b.Pack.Topic)
	if err != nil || after.Digest != b.Metadata.Digest {
		t.Fatal("C changed B semantic publication", err)
	}
	report.SharedControls = append(report.SharedControls, pairedAuthorityCells(t, h, model, b, ids, "shared", heldout, false)...)

	// A separate diagnostic replay, never counted as a paired observation or
	// used as feedback. It inspects the exact unchanged monthly phrase against
	// the still-current B publication before the destructive freshness control.
	for _, c := range heldout {
		if c.ID == "fresh-monthly-known-en" {
			metrics, _ := pairedMetricsSQL(c, ids)
			metadata := support.Raw(t, h.f.dsn)
			beforeReads := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`)
			beforeCalls := model.requests.Load()
			diagnostic, diagnosticErr := h.query.Preflight(t.Context(), h.queryActor, nlqexec.PreflightRequest{QuestionRequest: nlqexec.QuestionRequest{Topic: b.Pack.Topic, Topics: []string{b.Pack.Topic}, Context: b.Pack.Datasets[0].Source.Context, Locale: nlq.Language(c.Locale), Question: c.Question, MetricIDs: metrics, Kinds: []string{"measure", "kpi", "dimension"}, LimitPerKind: 5, Rerank: true}})
			report.MonthlyDiagnosticModelCalls = model.requests.Load() - beforeCalls
			report.MonthlyDiagnosticReadAttempts = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) - beforeReads
			_ = metadata.Close(t.Context())
			report.MonthlyDiagnosticDiscoveryAccounting = "Read-execution attempts measured separately; source metadata/discovery calls are not independently instrumented and remain unknown"
			report.MonthlyDiagnostic = &diagnostic
			if diagnosticErr != nil {
				report.MonthlyDiagnosticError = diagnosticErr.Error()
			}
		}
	}
	// Isolated destructive control runs only after all paired measurements.
	var origin semantics.SourceReference
	for _, d := range b.Pack.Datasets {
		if d.ID == ids["orders"] {
			origin = d.Source
		}
	}
	if _, err = h.f.admin.Exec(t.Context(), `UPDATE analytics.adv_orders SET misleading_net_total_usd=misleading_net_total_usd+1 WHERE division_id='alpha' AND order_id=1`); err != nil {
		t.Fatal(err)
	}
	replacement := h.f.profile(t, engineering.ProfileSpec{ID: "paired-stale-profile", Previous: origin.ProfileVersion, Source: origin.Source, Context: origin.Context, Dataset: origin.Dataset, SkipLLM: true}).Profile.Profile
	if replacement == nil || replacement.DeterministicHash() == origin.ProfileDigest {
		t.Fatal("stale profile control did not change evidence")
	}
	for _, cell := range pairedAuthorityCells(t, h, model, b, ids, "shared", heldout, true) {
		report.SharedControls = append(report.SharedControls, cell)
	}
	expectedCells := 84
	if repair {
		expectedCells = 112
	}
	if len(report.Cases) != expectedCells || len(report.SharedControls) != 3 {
		t.Fatal("paired/shared inventory incomplete", len(report.Cases), len(report.SharedControls))
	}
	for _, cell := range append(append([]pairedCell(nil), report.Cases...), report.SharedControls...) {
		if !check(cell) {
			t.Errorf("arm %s control %s failed: %s", cell.Arm, cell.Case.ID, cell.Reason)
		}
	}
	if len(report.Examples) == 0 {
		t.Error("C has no independently activated training-derived examples; learning qualification unavailable")
	}
	// A green regression explicitly asserts the unchanged FAILED evaluator,
	// matching Phase24's negative-gate pattern rather than skipping its errors.
	baselineErr := pairedBaselineFailureGate(report, training, heldout)
	report.BaselinePreserved = errors.Is(baselineErr, evaluation.ErrGate)
	if !report.BaselinePreserved {
		t.Error("expected exact failed baseline evaluation", baselineErr)
	}
	if len(report.Examples) != 3 {
		t.Error("predecessor reviewed-example inventory changed")
		report.BaselinePreserved = false
	}
	if repair {
		positiveErr := pairedRepairPositiveGate(report, heldout)
		report.RepairPassed = positiveErr == nil
		if positiveErr != nil {
			t.Error("positive D repair gate", positiveErr)
		}
	}
}

func pairedWireContains(bodies []string, needle string) bool {
	var contains func(any) bool
	contains = func(v any) bool {
		switch x := v.(type) {
		case string:
			return strings.Contains(x, needle)
		case []any:
			for _, item := range x {
				if contains(item) {
					return true
				}
			}
		case map[string]any:
			for _, item := range x {
				if contains(item) {
					return true
				}
			}
		}
		return false
	}
	for _, body := range bodies {
		var v any
		if json.Unmarshal([]byte(body), &v) == nil && contains(v) {
			return true
		}
	}
	return false
}

// Pinned observed predecessor outcomes from c15e64b, never expected-answer edits.
func pairedPredecessorPreserved(cell pairedCell) bool {
	failures := map[string]string{
		"fresh-known-gross-es": "invalid_temporal_span", "fresh-paid-count-en": "invalid_temporal_span",
		"fresh-known-count-en": "invalid_temporal_span", "fresh-known-count-es": "invalid_temporal_span",
		"fresh-cohort-net-en": "invalid_temporal_span", "fresh-activity-net-en": "invalid_temporal_span",
		"fresh-activity-net-es": "invalid_temporal_span", "fresh-monthly-known-en": "analytical_group_domain_review_required",
	}
	if cell.Arm == "TRAIN" && cell.Case.ID != "train-3-en" && cell.Case.ID != "train-4-en" && cell.Case.ID != "train-6-en" {
		return cell.Case.Expected == "answer" && !cell.Matches && cell.Actual == "clarify" && cell.Reason == "invalid_temporal_span"
	}
	if reason, failed := failures[cell.Case.ID]; failed {
		outcome := "clarify"
		if reason == "analytical_group_domain_review_required" {
			outcome = "reject"
		}
		return cell.Case.Expected == "answer" && !cell.Matches && cell.Actual == outcome && strings.Contains(cell.Reason, reason)
	}
	return cell.Matches
}
