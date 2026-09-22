package acceptance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/store/postgres"
	"github.com/hurtener/chartworks/test/support"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestPhase34(t *testing.T) {
	t.Run("AC01", phase34DryRunReplay)
	t.Run("AC02", phase34GraphNormalization)
	t.Run("AC03", phase34HistoricalAuthority)
	t.Run("AC04", phase34EvidenceLedger)
	t.Run("AC05", phase34RetentionErasure)
	t.Run("AC06", phase34CutoverRollback)
	t.Run("AC07", phase34FeatureClosure)
	t.Run("AC08", phase34OperationalBoundaries)
}

type phase34Adapter struct {
	mu      sync.Mutex
	applied []migration.Kind
}

type phase34FeedbackFixture struct {
	rows []evaluation.FeedbackEvidence
}

func (f *phase34FeedbackFixture) ReviewedFeedback(_ context.Context, _ identity.Envelope, _ string, _ int) ([]evaluation.FeedbackEvidence, error) {
	return append([]evaluation.FeedbackEvidence(nil), f.rows...), nil
}

func (a *phase34Adapter) Validate(_ context.Context, _ identity.Envelope, _ migration.Object, _ migration.Mapping) error {
	return nil
}
func (a *phase34Adapter) Apply(_ context.Context, _ identity.Envelope, o migration.Object, m migration.Mapping, _ string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applied = append(a.applied, o.Kind)
	if m.Destination != "" {
		if o.Kind == migration.KindSchedule {
			return m.Destination + ":v1", nil
		}
		return m.Destination, nil
	}
	return "dst-" + o.ExternalRef, nil
}

func phase34Actor(t *testing.T, tenant string, scopes ...string) identity.Envelope {
	t.Helper()
	base := []string{"cw.tenant.read:" + tenant, "cw.tenant.write:" + tenant, "cw.tenant.erase:" + tenant, "cw.tenant.export:" + tenant, "ops.read"}
	seen := map[string]bool{}
	all := make([]string, 0, len(base)+len(scopes))
	for _, scope := range append(base, scopes...) {
		if !seen[scope] {
			seen[scope] = true
			all = append(all, scope)
		}
	}
	e, err := identity.FromVerified(tenant, "operator", "session", all, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func phase34EvaluationMaterial() (evaluation.RuntimePackAuthorRequest, evaluation.Suite) {
	config := gateway.RuntimeConfig{Model: "model-v1", Models: []gateway.RuntimeModel{{Role: "routing", Model: "model-v1"}}, SystemInstruction: "synthetic reviewed instruction", AttemptCostUSD: 0.01}
	config.Digest = gateway.ConfigurationDigest(config)
	pack := evaluation.PackRevision{ID: "pack-v1", Revision: 1, Model: config.Model, Models: []evaluation.PackModel{{Role: "routing", Model: "model-v1"}}, ConfigurationDigest: config.Digest}
	pack.Digest = pack.CanonicalDigest()
	quality := 1.0
	evidenceDigest := strings.Repeat("a", 64)
	fixture := evaluation.Observation{Decision: "route", SemanticDigest: evidenceDigest, Usage: evaluation.Usage{ServiceMS: 1}}
	suite := evaluation.Suite{SchemaVersion: evaluation.SchemaVersion, ID: "suite-v1", Revision: 1, Mode: evaluation.Fixture, Seed: 42, Calibration: "reviewed", Threshold: evaluation.Threshold{QualityMin: &quality}, Limits: evaluation.Limits{Cases: 1, Calls: 1, Tokens: 1, DurationMS: 1000}, Provenance: evaluation.Provenance{Implementation: "synthetic-sha", EnvironmentDigest: evidenceDigest, ConfigurationDigest: config.Digest, SemanticVersion: "semantic-v1", RuleVersion: "rules-v1", SourceSnapshot: evidenceDigest, DialectMatrix: []evaluation.DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: evaluation.Fixture, EvidenceDigest: evidenceDigest, Status: "measured"}}}, Packs: []evaluation.PackRevision{pack}, Frontiers: []string{"EVAL-01"}, Cases: []evaluation.Case{{ID: "quality", Stage: evaluation.StageRouting, Locale: "en", HeldOut: true, Input: evaluation.ProtectedRef{Digest: evidenceDigest, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: "route", SemanticDigest: evidenceDigest}}, Fixture: &fixture}}}
	return evaluation.RuntimePackAuthorRequest{Pack: pack, Config: config}, suite
}

func phase34Manifest(suffix string) migration.Manifest {
	now := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	runtime, suite := phase34EvaluationMaterial()
	runtimeRaw, _ := json.Marshal(runtime)
	suiteRaw, _ := json.Marshal(suite)
	suiteDigest, _ := suite.Digest()
	snapshot := strings.Repeat("c", 64)
	kinds := []migration.Kind{migration.KindSource, migration.KindUpload, migration.KindProfile, migration.KindTopic, migration.KindRule, migration.KindTemplate, migration.KindRuntimePack, migration.KindEvalSuite, migration.KindBlock, migration.KindReport, migration.KindDashboard, migration.KindFilter, migration.KindSchedule, migration.KindRun, migration.KindArtifact, migration.KindRendition, migration.KindCertificate, migration.KindTombstone, migration.KindCalibration}
	objects := make([]migration.Object, 0, len(kinds))
	fields := []migration.FieldDisposition{}
	for i, kind := range kinds {
		ref := string(kind) + "-" + suffix
		parents := []string{}
		if i > 0 {
			previous := kinds[i-1] // #nosec G602 -- i is explicitly greater than zero.
			parents = []string{string(previous) + "-" + suffix}
		}
		lifecycle := "private_draft"
		private := true
		if kind == migration.KindRun || kind == migration.KindArtifact || kind == migration.KindRendition || kind == migration.KindCertificate {
			lifecycle = "historical"
		}
		if kind == migration.KindTombstone {
			lifecycle = "deleted"
		}
		payload := `{"name":"synthetic"}`
		if kind == migration.KindSource {
			payload = fmt.Sprintf(`{"engine":"postgres","dialect":"postgres","snapshot":"%s","context":"mapped-source:v1","revision":1}`, snapshot)
		}
		if kind == migration.KindRuntimePack {
			payload = string(runtimeRaw)
		}
		if kind == migration.KindEvalSuite {
			payload = string(suiteRaw)
		}
		if kind == migration.KindCalibration {
			payload = fmt.Sprintf(`{"id":"proposal-%s","suite_id":"%s","suite_revision":%d,"suite_digest":"%s","baseline_run":"baseline-%s","candidate_run":"candidate-%s"}`, suffix, suite.ID, suite.Revision, suiteDigest, suffix, suffix)
		}
		object := migration.Object{Kind: kind, ExternalRef: ref, Parents: parents, Revision: 1, PayloadVersion: "v1", Payload: payload, Lifecycle: lifecycle, Private: private, Origin: "neutral-source", Retention: migration.Retention{ExpiresAt: ptrTime(now.Add(30 * 24 * time.Hour)), EraseWith: "cohort-" + suffix}}
		if kind == migration.KindTombstone {
			object.Deletes = &migration.TombstoneTarget{Kind: migration.KindBlock, ExternalRef: "block-" + suffix, Revision: 1}
		}
		objects = append(objects, object)
		var top map[string]any
		_ = json.Unmarshal([]byte(payload), &top)
		for key := range top {
			fields = append(fields, migration.FieldDisposition{Path: ref + "." + key, Status: "retained"})
		}
	}
	evidence := []migration.Evidence{}
	for _, group := range []struct {
		prefix string
		count  int
	}{{"B", 20}, {"R", 16}, {"Q", 10}, {"N", 16}} {
		for i := 1; i <= group.count; i++ {
			feature := group.prefix + fmt.Sprintf("%02d", i)
			evidence = append(evidence, migration.Evidence{Feature: feature, OwnerFeature: feature, Disposition: "required", Outcome: "passed", EvidenceType: "live", Reference: "evidence-" + strings.ToLower(feature), Source: "evaluation", SourceVersion: suiteDigest, EvidenceHash: strings.Repeat("b", 64), ComparisonHash: strings.Repeat("d", 64), Engine: "postgres", Dialect: "postgres", SourceSnapshot: snapshot, SourceRevision: 1})
		}
	}
	evidence = append(evidence, migration.Evidence{Feature: "Q11", OwnerFeature: "EVAL-01", Disposition: "excluded", Outcome: "unsupported", EvidenceType: "operator", Reference: "discarded-stub", Source: "synthetic", SourceVersion: suiteDigest, EvidenceHash: strings.Repeat("b", 64)})
	return migration.Manifest{Version: migration.ManifestVersion, Batch: "batch-" + suffix, Cohort: "cohort-" + suffix, SourceSnapshot: snapshot, Engine: "postgres", Dialect: "postgres", Mappings: []migration.Mapping{{Kind: migration.KindSource, ExternalRef: "source-" + suffix, Destination: "mapped-source", Revision: 1}}, Objects: objects, Fields: fields, Evidence: evidence, Boundary: &migration.OccurrenceBoundary{Stream: "stream-" + suffix, LastAccepted: "occurrence-prior", LastDue: now.Add(-time.Hour), ResumeAfter: now, ScheduleVersion: 1}}
}
func ptrTime(t time.Time) *time.Time { return &t }

func phase34Service(t *testing.T, suffix string) (*migration.Service, *phase34Adapter, identity.Envelope, string) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	adapter := &phase34Adapter{}
	adapters := map[migration.Kind]migration.Adapter{}
	for _, kind := range []migration.Kind{migration.KindSource, migration.KindUpload, migration.KindProfile, migration.KindTopic, migration.KindRule, migration.KindTemplate, migration.KindRuntimePack, migration.KindEvalSuite, migration.KindBlock, migration.KindReport, migration.KindDashboard, migration.KindFilter, migration.KindSchedule, migration.KindTombstone, migration.KindCalibration} {
		adapters[kind] = adapter
	}
	service, err := migration.New(db, adapters, nil, migration.EvidenceVerifierFunc(func(_ context.Context, _ identity.Envelope, evidence migration.Evidence) error {
		if evidence.Outcome != "passed" {
			return migration.ErrNotReady
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return service, adapter, phase34Actor(t, "tenant-"+suffix, "migration.read", "migration.write", "migration.cutover", "migration.erase", "scheduling.write", "cw.schedule.write:*", "sources.read", "cw.source.read:mapped-source", "cw.execution_context.use:mapped-source:v1"), dsn
}

func phase34DryRunReplay(t *testing.T) {
	s, a, e, _ := phase34Service(t, "ac01")
	m := phase34Manifest("ac01")
	plan, err := s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: m})
	if err != nil || !plan.Ready || len(plan.Fields) != len(m.Fields) {
		t.Fatal("dry-run loss ledger", err, plan)
	}
	first, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m})
	if err != nil || first.State != "complete" {
		t.Fatal(err, first)
	}
	again, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m, Expected: first.Revision})
	if err != nil || again.ID != first.ID || again.Digest != first.Digest || again.Revision != first.Revision || again.State != first.State || again.Applied != first.Applied || again.Quarantined != first.Quarantined {
		t.Fatal("idempotent import", err, again, first)
	}
	if len(a.applied) != 15 {
		t.Fatal("historical rows crossed current adapters or active graph omitted", a.applied)
	}
	changed := m
	changed.Objects = slices.Clone(m.Objects)
	changed.Objects[0].Payload = strings.Replace(changed.Objects[0].Payload, `"context":"mapped-source:v1"`, `"context":"mapped-source:v2"`, 1)
	if _, err = s.Import(t.Context(), e, migration.ImportRequest{Manifest: changed, Expected: first.Revision}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("changed replay accepted", err)
	}
	secret := m
	secret.Batch = "secret-ac01"
	secret.Objects = slices.Clone(m.Objects)
	secret.Objects[0].Payload = `{"token":"forbidden"}`
	secret.Fields = append(secret.Fields, migration.FieldDisposition{Path: "source-ac01.token", Status: "dropped", Reason: "credential"})
	if _, err = s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: secret}); !errors.Is(err, migration.ErrInvalid) {
		t.Fatal("credential entered manifest", err)
	}
}

func phase34GraphNormalization(t *testing.T) {
	s, a, e, _ := phase34Service(t, "ac02")
	m := phase34Manifest("ac02")
	ignoredCalibration := m
	ignoredCalibration.Calibration = &migration.Calibration{Revision: "candidate", State: "review_candidate", Payload: `{}`}
	if _, err := s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: ignoredCalibration}); !errors.Is(err, migration.ErrUnsupported) {
		t.Fatal("non-operative top-level calibration was accepted", err)
	}
	out, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m})
	if err != nil || out.Total != 19 || out.Applied != 15 || out.Quarantined != 4 {
		t.Fatal(err, out)
	}
	a.mu.Lock()
	got := append([]migration.Kind(nil), a.applied...)
	a.mu.Unlock()
	want := []migration.Kind{migration.KindSource, migration.KindUpload, migration.KindProfile, migration.KindTopic, migration.KindRule, migration.KindTemplate, migration.KindRuntimePack, migration.KindEvalSuite, migration.KindBlock, migration.KindReport, migration.KindDashboard, migration.KindFilter, migration.KindSchedule, migration.KindTombstone, migration.KindCalibration}
	if !slices.Equal(got, want) {
		t.Fatal("dependency order changed", got)
	}
	exported, err := s.Export(t.Context(), e, migration.ExportRequest{Batch: m.Batch, Limit: 1000})
	if err != nil || !reflect.DeepEqual(exported.Manifest, m) {
		t.Fatal("normalization lost exact state", err, exported)
	}
	phase34EvaluationDrafts(t)
}

func phase34EvaluationDrafts(t *testing.T) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	eval, err := evaluation.New(db, nil, func() time.Time { return time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	service, err := migration.New(db, migration.EvaluationAdapters(eval), nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant := "tenant-ac02-evaluation"
	actor := phase34Actor(t, tenant, "migration.read", "migration.write", "ops.write")
	runtime, suite := phase34EvaluationMaterial()
	pack, config := runtime.Pack, runtime.Config
	runtimeRaw, _ := json.Marshal(runtime)
	suiteRaw, _ := json.Marshal(suite)
	now := time.Date(2026, 11, 1, 6, 30, 0, 0, time.UTC)
	objects := []migration.Object{
		{Kind: migration.KindRuntimePack, ExternalRef: "runtime-pack-v1", Revision: 1, PayloadVersion: "v1", Payload: string(runtimeRaw), Lifecycle: "private_draft", Private: true, Origin: "synthetic", Retention: migration.Retention{ExpiresAt: ptrTime(now.Add(24 * time.Hour))}},
		{Kind: migration.KindEvalSuite, ExternalRef: "evaluation-suite-v1", Parents: []string{"runtime-pack-v1"}, Revision: 1, PayloadVersion: "v1", Payload: string(suiteRaw), Lifecycle: "private_draft", Private: true, Origin: "synthetic", Retention: migration.Retention{ExpiresAt: ptrTime(now.Add(24 * time.Hour))}},
	}
	fields := []migration.FieldDisposition{}
	for _, object := range objects {
		var top map[string]any
		if err := json.Unmarshal([]byte(object.Payload), &top); err != nil {
			t.Fatal(err)
		}
		for key := range top {
			fields = append(fields, migration.FieldDisposition{Path: object.ExternalRef + "." + key, Status: "retained"})
		}
	}
	// Seed both drafts to prove migration reconciles the public owning service
	// after a crash between domain commit and migration checkpoint.
	if _, err = eval.AuthorRuntimePack(t.Context(), actor, pack, config); err != nil {
		t.Fatal(err)
	}
	if _, err = eval.Author(t.Context(), actor, suite); err != nil {
		t.Fatal(err)
	}
	manifest := migration.Manifest{Version: migration.ManifestVersion, Batch: "batch-ac02-evaluation", Cohort: "cohort-ac02-evaluation", SourceSnapshot: strings.Repeat("c", 64), Engine: "postgres", Dialect: "postgres", Objects: objects, Fields: fields, Evidence: phase34Manifest("ac02-evidence").Evidence}
	batch, err := service.Import(t.Context(), actor, migration.ImportRequest{Manifest: manifest})
	if err != nil || batch.State != "complete" || batch.Applied != 2 || batch.Quarantined != 0 {
		t.Fatal("evaluation drafts were not reconciled", err, batch)
	}
	var runtimeState, suiteState string
	raw := support.Raw(t, dsn)
	if err = raw.QueryRow(t.Context(), `SELECT state FROM chartworks.evaluation_runtime_packs WHERE tenant_id=$1 AND pack_digest=$2`, tenant, pack.Digest).Scan(&runtimeState); err != nil {
		t.Fatal(err)
	}
	if err = raw.QueryRow(t.Context(), `SELECT state FROM chartworks.evaluation_suites WHERE tenant_id=$1 AND suite_id=$2 AND revision=1`, tenant, suite.ID).Scan(&suiteState); err != nil {
		t.Fatal(err)
	}
	if runtimeState != "draft" || suiteState != "draft" {
		t.Fatal("migration promoted review authority", runtimeState, suiteState)
	}
	phase34CalibrationImport(t)
}

func phase34CalibrationImport(t *testing.T) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	feedback := &phase34FeedbackFixture{}
	eval, err := evaluation.New(db, feedback, nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant := "tenant-ac02-calibration"
	author := phase34Actor(t, tenant, "migration.read", "migration.write", "ops.read", "ops.write", "cw.tenant.export:"+tenant)
	reviewer, err := identity.FromVerified(tenant, "reviewer", "calibration-review", []string{"ops.audit", "cw.tenant.certify:" + tenant}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	baseline, _ := evalRuntimePack(t, eval, author, reviewer, "calibration-baseline", true)
	candidate, _ := evalRuntimePack(t, eval, author, reviewer, "calibration-candidate", true)
	makeInput := func(pack evaluation.PackRevision, question string) evaluation.ProtectedRef {
		ref, inputErr := eval.RegisterInput(t.Context(), author, "protected", evaluation.LiveInput{Pack: pack, Route: &nlqroute.RouteRequest{Context: "synthetic-context", Locale: "en", Question: question}})
		if inputErr != nil {
			t.Fatal("register protected optimization input", inputErr)
		}
		return ref
	}
	heldoutOne := makeInput(baseline, "synthetic heldout one")
	heldoutTwo := makeInput(baseline, "synthetic heldout two")
	trainingInput := makeInput(baseline, "synthetic training input")
	feedback.rows = []evaluation.FeedbackEvidence{
		{ID: "calibration-case-one", Locale: "en", InputDigest: heldoutOne.Digest, ExpectedDigest: evalDigest, Decision: "expected", SourceBindingDigest: strings.Repeat("e", 64), CreatedAt: time.Now().UTC()},
		{ID: "calibration-case-two", Locale: "en", InputDigest: heldoutTwo.Digest, ExpectedDigest: evalDigest, Decision: "expected", SourceBindingDigest: strings.Repeat("e", 64), CreatedAt: time.Now().UTC()},
		{ID: "calibration-training-case", Locale: "en", InputDigest: trainingInput.Digest, ExpectedDigest: evalDigest, Decision: "expected", SourceBindingDigest: strings.Repeat("e", 64), CreatedAt: time.Now().UTC()},
	}
	candidateExport, err := eval.ExportFeedback(t.Context(), author, "calibration-feedback", "sales", 10)
	if err != nil {
		t.Fatal("export reviewed feedback for split review", err)
	}
	split, err := eval.ReviewFeedbackSplit(t.Context(), reviewer, candidateExport.ID, candidateExport.EvidenceHash, "calibration-training", "calibration-heldout", []string{"calibration-case-one", "calibration-case-two"})
	if err != nil {
		t.Fatal("independently review heldout calibration evidence", err)
	}
	cases := []evaluation.Case{
		{ID: "heldout-one", Stage: evaluation.StageRouting, Locale: "en", HeldOut: true, Input: heldoutOne, Expected: []evaluation.Expected{{Decision: "expected", SemanticDigest: evalDigest}}},
		{ID: "heldout-two", Stage: evaluation.StageRouting, Locale: "en", HeldOut: true, Input: heldoutTwo, Expected: []evaluation.Expected{{Decision: "expected", SemanticDigest: evalDigest}}},
	}
	suite := evalSuite(evaluation.Live, cases)
	suite.ID = "calibration-suite"
	suite.Seed = 24002
	suite.Packs = []evaluation.PackRevision{baseline, candidate}
	suite.HeldoutLineageDigest = split.Heldout.EvidenceHash
	minimum := 0.5
	suite.Threshold.QualityMin = &minimum
	suite.Limits.Cases, suite.Limits.Calls, suite.Limits.Tokens = 2, 2, 16
	costCap := 1.0
	suite.Limits.CostUSD = &costCap
	draft, err := eval.Author(t.Context(), author, suite)
	if err != nil {
		t.Fatal("author private comparison suite", err)
	}
	accepted, err := eval.Review(t.Context(), reviewer, suite.ID, evaluation.SuiteReviewRequest{Revision: suite.Revision, Digest: draft.Digest, Decision: evaluation.Accepted})
	if err != nil {
		t.Fatal("review comparison suite", err)
	}
	observe := func(_ context.Context, execution evaluation.Execution) (evaluation.Observation, error) {
		semantic := evalDigest
		decision := "expected"
		if execution.Pack.Digest == baseline.Digest && execution.Case.ID == "heldout-two" {
			semantic = strings.Repeat("b", 64)
			decision = "drift"
		}
		return evaluation.Observation{Decision: decision, SemanticDigest: semantic, Usage: evaluation.Usage{ServiceMS: 1, Calls: 1, Tokens: evalPtr(1), CostUSD: evalPtr(0.02)}}, nil
	}
	baselineReport, err := eval.Run(t.Context(), author, evaluation.RunRequest{RunID: "calibration-baseline-run", SuiteID: suite.ID, SuiteRevision: suite.Revision, SuiteDigest: accepted.Digest, PackDigest: baseline.Digest}, evaluation.RunnerFunc(observe))
	if err != nil || !baselineReport.GatePassed || baselineReport.Status != "passed" || baselineReport.QualityPassed != 1 {
		t.Fatal("baseline comparison did not produce owner evidence", err, baselineReport)
	}
	candidateReport, err := eval.Run(t.Context(), author, evaluation.RunRequest{RunID: "calibration-candidate-run", SuiteID: suite.ID, SuiteRevision: suite.Revision, SuiteDigest: accepted.Digest, PackDigest: candidate.Digest}, evaluation.RunnerFunc(observe))
	if err != nil || !candidateReport.GatePassed || candidateReport.Status != "passed" || candidateReport.QualityPassed != 2 {
		t.Fatal("candidate comparison did not produce owner evidence", err, candidateReport)
	}
	storedBaseline, err := eval.Read(t.Context(), author, baselineReport.RunID)
	if err != nil || storedBaseline.EvidenceHash != baselineReport.EvidenceHash || storedBaseline.Status != "passed" {
		t.Fatal("baseline owner evidence was not durably readable", err, storedBaseline)
	}
	storedCandidate, err := eval.Read(t.Context(), author, candidateReport.RunID)
	if err != nil || storedCandidate.EvidenceHash != candidateReport.EvidenceHash || storedCandidate.Status != "passed" {
		t.Fatal("candidate owner evidence was not durably readable", err, storedCandidate)
	}
	request := evaluation.ProposalRequest{ID: "imported-calibration", SuiteID: suite.ID, SuiteRevision: suite.Revision, SuiteDigest: accepted.Digest, BaselineRun: baselineReport.RunID, CandidateRun: candidateReport.RunID}
	if _, err := eval.PreviewOptimization(t.Context(), author, request); err != nil {
		t.Fatal("owner evaluation service could not resolve durable calibration evidence", err)
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err = json.Unmarshal(payload, &top); err != nil {
		t.Fatal(err)
	}
	ref := "calibration-review"
	fields := make([]migration.FieldDisposition, 0, len(top))
	for key := range top {
		fields = append(fields, migration.FieldDisposition{Path: ref + "." + key, Status: "retained"})
	}
	manifest := migration.Manifest{Version: migration.ManifestVersion, Batch: "batch-ac02-calibration", Cohort: "cohort-ac02-calibration", SourceSnapshot: strings.Repeat("c", 64), Engine: "postgres", Dialect: "postgres", Objects: []migration.Object{{Kind: migration.KindCalibration, ExternalRef: ref, Revision: 1, PayloadVersion: "v1", Payload: string(payload), Lifecycle: "private_draft", Private: true, Origin: "synthetic", Retention: migration.Retention{ExpiresAt: ptrTime(time.Now().Add(time.Hour))}}}, Fields: fields, Evidence: phase34Manifest("ac02-calibration-evidence").Evidence}
	service, err := migration.New(db, migration.EvaluationAdapters(eval), nil)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := service.Import(t.Context(), author, migration.ImportRequest{Manifest: manifest})
	if err != nil || batch.State != "complete" || batch.Applied != 1 {
		t.Fatal("calibration did not import through evaluation service", err, batch)
	}
	raw := support.Raw(t, dsn)
	var state, authorID string
	var reviewIsNull bool
	if err = raw.QueryRow(t.Context(), `SELECT state,author_id,review IS NULL FROM chartworks.evaluation_proposals WHERE tenant_id=$1 AND proposal_id=$2`, tenant, request.ID).Scan(&state, &authorID, &reviewIsNull); err != nil || state != "candidate" || authorID != author.User() || !reviewIsNull {
		t.Fatal("import did not persist a private unreviewed evaluation candidate", err, state, authorID, reviewIsNull)
	}
	var selected int
	if err = raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.evaluation_pack_selection WHERE tenant_id=$1`, tenant).Scan(&selected); err != nil || selected != 0 {
		t.Fatal("calibration import changed active pack selection", err, selected)
	}
	if replay, replayErr := service.Import(t.Context(), author, migration.ImportRequest{Manifest: manifest, Expected: batch.Revision}); replayErr != nil || replay.ID != batch.ID || replay.Revision != batch.Revision {
		t.Fatal("calibration import replay changed the candidate", replayErr, replay)
	}
	public := manifest
	public.Batch, public.Cohort = "batch-ac02-calibration-public", "cohort-ac02-calibration-public"
	public.Objects = append([]migration.Object(nil), manifest.Objects...)
	public.Objects[0].Private = false
	if _, err = service.Import(t.Context(), author, migration.ImportRequest{Manifest: public}); !errors.Is(err, migration.ErrInvalid) {
		t.Fatal("public calibration import was persisted", err)
	}
}

func phase34HistoricalAuthority(t *testing.T) {
	s, _, e, _ := phase34Service(t, "ac03")
	m := phase34Manifest("ac03")
	plan, err := s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: m})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range plan.Objects {
		if p.Kind == migration.KindCertificate || p.Kind == migration.KindArtifact || p.Kind == migration.KindRendition || p.Kind == migration.KindRun {
			if p.Action != "historical_quarantine" {
				t.Fatal("history auto-promoted", p)
			}
		}
	}
	foreign := phase34Actor(t, "foreign", "migration.read")
	if _, err = s.DryRun(t.Context(), foreign, migration.DryRunRequest{Manifest: m}); err != nil {
		t.Fatal("dry-run should be tenant-local input", err)
	}
	if _, err = s.Export(t.Context(), foreign, migration.ExportRequest{Batch: m.Batch, Limit: 10}); !errors.Is(err, migration.ErrNotFound) {
		t.Fatal("cross-tenant batch disclosed", err)
	}
	denied := phase34Actor(t, e.Tenant(), "migration.read")
	if _, err = s.Import(t.Context(), denied, migration.ImportRequest{Manifest: m}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("read authority imported", err)
	}
}

func phase34EvidenceLedger(t *testing.T) {
	phase34LiveOwnerEvidence(t)
	s, _, e, _ := phase34Service(t, "ac04")
	m := phase34Manifest("ac04")
	for i := range m.Evidence {
		if m.Evidence[i].Feature == "R01" {
			m.Evidence[i].Outcome = "failed"
			m.Evidence[i].Reference = "semantic-result-diff"
		}
	}
	plan, err := s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: m})
	if err != nil || plan.Ready || len(plan.Limitations) == 0 {
		t.Fatal("failed semantic evidence declared ready", err, plan)
	}
	batch, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m})
	if err != nil || batch.State != "complete" {
		t.Fatal("quarantined import should remain inspectable", err, batch)
	}
	if _, err = s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: "new-route", OperatorRef: "drill", Expected: 0}); !errors.Is(err, migration.ErrNotReady) {
		t.Fatal("incomplete shadow evidence cut over", err)
	}
}

func phase34LiveOwnerEvidence(t *testing.T) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	eval, err := evaluation.New(db, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	tenant := "tenant-ac04-live"
	author := phase34Actor(t, tenant, "ops.write", "migration.read", "migration.write", "migration.cutover", "scheduling.write", "cw.schedule.write:*", "sources.read", "cw.source.read:mapped-source", "cw.execution_context.use:mapped-source:v1")
	reviewer, err := identity.FromVerified(tenant, "reviewer", "review-session", []string{"ops.audit", "cw.tenant.certify:" + tenant}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	pack, _ := evalRuntimePack(t, eval, author, reviewer, "phase34-pack", true)
	manifest := phase34Manifest("ac04-live")
	ref, err := eval.RegisterInput(t.Context(), author, "protected", evaluation.LiveInput{Pack: pack, Route: &nlqroute.RouteRequest{Context: "context", Locale: "en", Question: "synthetic parity comparison"}})
	if err != nil {
		t.Fatal(err)
	}
	cases := make([]evaluation.Case, 0, 62)
	expected := map[string]string{}
	for _, evidence := range manifest.Evidence {
		if evidence.Disposition != "required" {
			continue
		}
		sum := sha256.Sum256([]byte("phase34-synthetic-" + evidence.Feature))
		digest := hex.EncodeToString(sum[:])
		expected[evidence.Feature] = digest
		stage := evaluation.StageRouting
		switch evidence.Feature[0] {
		case 'B':
			stage = evaluation.StageContext
		case 'R':
			stage = evaluation.StageReport
		case 'Q':
			stage = evaluation.StageSQL
		}
		cases = append(cases, evaluation.Case{ID: evidence.Feature, Stage: stage, Locale: "en", HeldOut: true, Input: ref, BindingDigest: manifest.SourceSnapshot, Expected: []evaluation.Expected{{Decision: "expected", SemanticDigest: digest}}})
	}
	suite := evalSuite(evaluation.Live, cases)
	suite.ID = "phase34-suite"
	suite.Packs = []evaluation.PackRevision{pack}
	suite.Provenance.SourceSnapshot = manifest.SourceSnapshot
	suite.Provenance.SourceRevision = 1
	costCap := 2.0
	suite.Limits.CostUSD = &costCap
	draft, err := eval.Author(t.Context(), author, suite)
	if err != nil {
		t.Fatal(err)
	}
	record, err := eval.Review(t.Context(), reviewer, suite.ID, evaluation.SuiteReviewRequest{Revision: suite.Revision, Digest: draft.Digest, Decision: evaluation.Accepted})
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.02
	tokens := 1
	report, err := eval.Run(t.Context(), author, evaluation.RunRequest{RunID: "phase34-evidence", SuiteID: record.Suite.ID, SuiteRevision: record.Suite.Revision, SuiteDigest: record.Digest, PackDigest: pack.Digest}, evaluation.RunnerFunc(func(_ context.Context, execution evaluation.Execution) (evaluation.Observation, error) {
		return evaluation.Observation{Decision: "expected", SemanticDigest: expected[execution.Case.ID], Usage: evaluation.Usage{ServiceMS: 1, Calls: 1, Tokens: &tokens, CostUSD: &cost}}, nil
	}))
	if err != nil || !report.GatePassed || len(report.Cases) != 62 {
		t.Fatal("live owner evidence", err, report)
	}
	adapter := &phase34Adapter{}
	adapters := map[migration.Kind]migration.Adapter{}
	for _, kind := range []migration.Kind{migration.KindSource, migration.KindUpload, migration.KindProfile, migration.KindTopic, migration.KindRule, migration.KindTemplate, migration.KindRuntimePack, migration.KindEvalSuite, migration.KindBlock, migration.KindReport, migration.KindDashboard, migration.KindFilter, migration.KindSchedule, migration.KindTombstone, migration.KindCalibration} {
		adapters[kind] = adapter
	}
	verifier := migration.EvidenceVerifierFunc(func(ctx context.Context, e identity.Envelope, x migration.Evidence) error {
		return eval.VerifyMigrationEvidence(ctx, e, evaluation.MigrationComparison{Feature: x.Feature, RunID: x.Reference, SuiteDigest: x.SourceVersion, EvidenceHash: x.EvidenceHash, ComparisonHash: x.ComparisonHash, Engine: x.Engine, Dialect: x.Dialect, SourceSnapshot: x.SourceSnapshot, SourceRevision: x.SourceRevision})
	})
	service, err := migration.New(db, adapters, nil, verifier)
	if err != nil {
		t.Fatal(err)
	}
	for i := range manifest.Evidence {
		if manifest.Evidence[i].Disposition != "required" {
			continue
		}
		comparison, compareErr := eval.MigrationComparison(t.Context(), author, manifest.Evidence[i].Feature, report.RunID, manifest.Engine, manifest.Dialect, manifest.SourceSnapshot, 1)
		if compareErr != nil {
			t.Fatal("missing owner case comparison", manifest.Evidence[i].Feature, compareErr)
		}
		manifest.Evidence[i].Reference = report.RunID
		manifest.Evidence[i].SourceVersion = report.SuiteDigest
		manifest.Evidence[i].EvidenceHash = report.EvidenceHash
		manifest.Evidence[i].ComparisonHash = comparison.ComparisonHash
	}
	oldRoute, newRoute, accepted, _, _ := phase34ScheduleRoutes(t, dsn, tenant)
	manifest.Boundary.LastAccepted, manifest.Boundary.LastDue = accepted.ID, accepted.DueAt
	manifest.Boundary.ResumeAfter = accepted.DueAt.Add(time.Minute)
	manifest.Mappings = append(manifest.Mappings, migration.Mapping{Kind: migration.KindSchedule, ExternalRef: "schedule-ac04-live", Destination: newRoute, Revision: 1})
	badComparison := manifest
	badComparison.Evidence = slices.Clone(manifest.Evidence)
	badComparison.Evidence[0].ComparisonHash = badComparison.Evidence[1].ComparisonHash
	if badPlan, badErr := service.DryRun(t.Context(), author, migration.DryRunRequest{Manifest: badComparison}); badErr != nil || badPlan.Ready {
		t.Fatal("one case comparison certified another feature", badErr, badPlan)
	}
	plan, err := service.DryRun(t.Context(), author, migration.DryRunRequest{Manifest: manifest})
	if err != nil || !plan.Ready {
		t.Fatal("verified live evidence did not unlock readiness", err, plan.Limitations)
	}
	batch, err := service.Import(t.Context(), author, migration.ImportRequest{Manifest: manifest})
	if err != nil || batch.State != "complete" {
		t.Fatal("owner-evidenced import did not complete", err, batch)
	}
	if _, err = service.Cutover(t.Context(), author, migration.CutoverRequest{Batch: batch.ID, Route: newRoute, PreviousRoute: oldRoute, OperatorRef: "owner-evidence-drill"}); err != nil {
		t.Fatal("current owner report did not authorize evidence-complete cutover", err)
	}
	callerText := phase34Manifest("ac04-caller-text")
	for i := range callerText.Evidence {
		if callerText.Evidence[i].Disposition == "required" {
			callerText.Evidence[i].Reference = "invented-owner-comparison"
			callerText.Evidence[i].SourceVersion = report.SuiteDigest
			callerText.Evidence[i].EvidenceHash = report.EvidenceHash
		}
	}
	callerPlan, err := service.DryRun(t.Context(), author, migration.DryRunRequest{Manifest: callerText})
	if err != nil || callerPlan.Ready {
		t.Fatal("caller-authored comparison reference unlocked readiness", err, callerPlan)
	}
	callerBatch, err := service.Import(t.Context(), author, migration.ImportRequest{Manifest: callerText})
	if err != nil || callerBatch.State != "complete" {
		t.Fatal("failed evidence bundle did not remain inspectable", err, callerBatch)
	}
	if _, err = service.Cutover(t.Context(), author, migration.CutoverRequest{Batch: callerBatch.ID, Route: "unreachable-route", PreviousRoute: "unreachable-prior", OperatorRef: "caller-evidence-drill"}); !errors.Is(err, migration.ErrNotReady) {
		t.Fatal("caller-authored comparison text unlocked cutover", err)
	}
}

func phase34RetentionErasure(t *testing.T) {
	s, _, e, dsn := phase34Service(t, "ac05")
	m := phase34Manifest("ac05")
	batch, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m})
	if err != nil {
		t.Fatal(err)
	}
	limited, err := identity.FromVerified(e.Tenant(), "reader", "session", []string{"migration.read", "ops.read", "cw.tenant.read:" + e.Tenant()}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Export(t.Context(), limited, migration.ExportRequest{Batch: m.Batch, Limit: 1}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("tenant read leaked private manifest payload", err)
	}
	raw := support.Raw(t, dsn)
	var refs int
	if err = raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.migration_external_refs WHERE tenant_id=$1`, e.Tenant()).Scan(&refs); err != nil || refs != batch.Total {
		t.Fatal(err, refs)
	}
	var tombstoned bool
	if err = raw.QueryRow(t.Context(), `SELECT tombstoned FROM chartworks.migration_external_refs WHERE tenant_id=$1 AND kind='block' AND external_ref=$2`, e.Tenant(), "block-ac05").Scan(&tombstoned); err != nil || !tombstoned {
		t.Fatal("tombstone did not fence the deleted external reference", err)
	}
	resurrection := m
	resurrection.Batch = "resurrection-ac05"
	resurrection.Cohort = "resurrection-ac05"
	if _, err = s.Import(t.Context(), e, migration.ImportRequest{Manifest: resurrection}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("tombstoned external reference resurrected", err)
	}
	denied := phase34Actor(t, e.Tenant(), "migration.read")
	if _, err = s.Erase(t.Context(), denied, migration.EraseRequest{Batch: m.Batch, Limit: 100}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("read authority erased", err)
	}
	erased, err := s.Erase(t.Context(), e, migration.EraseRequest{Batch: m.Batch, Limit: 100})
	if err != nil || erased.Remaining != 0 || erased.BackupScope == "" {
		t.Fatal(err, erased)
	}
	if _, err = s.Export(t.Context(), e, migration.ExportRequest{Batch: m.Batch, Limit: 10}); !errors.Is(err, migration.ErrNotFound) {
		t.Fatal("erased payload readable", err)
	}
	hold := phase34Manifest("ac05hold")
	hold.Objects[0].Retention.LegalHold = true
	if _, err = s.Import(t.Context(), e, migration.ImportRequest{Manifest: hold}); err != nil {
		t.Fatal("legal-hold batch import", err)
	}
	if _, err = s.Erase(t.Context(), e, migration.EraseRequest{Batch: hold.Batch, Limit: 100}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("legal hold erased", err)
	}
	phase34ExternalRefAndExpiredExport(t)
}

func phase34ExternalRefAndExpiredExport(t *testing.T) {
	t.Helper()
	s, adapter, actor, _ := phase34Service(t, "ac05mapping")
	m := phase34Manifest("ac05mapping")
	m.Objects = m.Objects[:1]
	m.Fields = slices.DeleteFunc(m.Fields, func(field migration.FieldDisposition) bool {
		return !strings.HasPrefix(field.Path, m.Objects[0].ExternalRef+".")
	})
	m.Boundary = nil
	if _, err := s.Import(t.Context(), actor, migration.ImportRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	before := len(adapter.applied)
	repoint := m
	repoint.Batch, repoint.Cohort = "repoint-ac05", "repoint-ac05"
	repoint.Mappings = slices.Clone(m.Mappings)
	repoint.Mappings[0].Destination = "other-source"
	if _, err := s.Import(t.Context(), actor, migration.ImportRequest{Manifest: repoint}); !errors.Is(err, migration.ErrConflict) || len(adapter.applied) != before {
		t.Fatal("same source revision repointed after adapter effects", err, adapter.applied)
	}
	racingService, racingAdapter, racingActor, _ := phase34Service(t, "ac05race")
	racing := phase34Manifest("ac05race")
	racing.Objects = racing.Objects[:1]
	racing.Fields = slices.DeleteFunc(racing.Fields, func(field migration.FieldDisposition) bool {
		return !strings.HasPrefix(field.Path, racing.Objects[0].ExternalRef+".")
	})
	racing.Boundary = nil
	other := racing
	other.Batch, other.Cohort = "other-ac05race", "other-ac05race"
	other.Mappings = slices.Clone(racing.Mappings)
	other.Mappings[0].Destination = "other-source"
	results := make(chan error, 2)
	for _, contender := range []migration.Manifest{racing, other} {
		go func(contender migration.Manifest) {
			_, importErr := racingService.Import(t.Context(), racingActor, migration.ImportRequest{Manifest: contender})
			results <- importErr
		}(contender)
	}
	successes, conflicts := 0, 0
	for range 2 {
		switch importErr := <-results; {
		case importErr == nil:
			successes++
		case errors.Is(importErr, migration.ErrConflict):
			conflicts++
		default:
			t.Fatal("concurrent mapping result", importErr)
		}
	}
	racingAdapter.mu.Lock()
	applied := len(racingAdapter.applied)
	racingAdapter.mu.Unlock()
	if successes != 1 || conflicts != 1 || applied != 1 {
		t.Fatal("concurrent remap reached an adapter", successes, conflicts, applied)
	}
	expiredService, _, expiredActor, _ := phase34Service(t, "ac05expired")
	expired := phase34Manifest("ac05expired")
	expired.Objects[0].Retention.ExpiresAt = ptrTime(time.Now().Add(-time.Minute))
	if _, err := expiredService.Import(t.Context(), expiredActor, migration.ImportRequest{Manifest: expired}); err != nil {
		t.Fatal(err)
	}
	if _, err := expiredService.Export(t.Context(), expiredActor, migration.ExportRequest{Batch: expired.Batch, Limit: 1}); !errors.Is(err, migration.ErrNotReady) {
		t.Fatal("expired private payload exported", err)
	}
	phase34CrossRevisionReservation(t)
}

func phase34CrossRevisionReservation(t *testing.T) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	actor := phase34Actor(t, "tenant-ac05-revision", "migration.read", "migration.write")
	paused := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	applied := make(chan int64, 2)
	var rev1Validations atomic.Int32
	adapter := migration.AdapterFuncs{
		ValidateFunc: func(ctx context.Context, _ identity.Envelope, object migration.Object, _ migration.Mapping) error {
			if object.Revision == 1 && rev1Validations.Add(1) == 2 {
				close(paused)
				select {
				case <-release:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		},
		ApplyFunc: func(_ context.Context, _ identity.Envelope, object migration.Object, mapping migration.Mapping, _ string) (string, error) {
			applied <- object.Revision
			return mapping.Destination, nil
		},
	}
	service, err := migration.New(db, map[migration.Kind]migration.Adapter{migration.KindSource: adapter}, nil, migration.EvidenceVerifierFunc(func(context.Context, identity.Envelope, migration.Evidence) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	rev1 := phase34Manifest("ac05-revision")
	rev1.Objects = slices.Clone(rev1.Objects[:1])
	rev1.Fields = slices.DeleteFunc(rev1.Fields, func(field migration.FieldDisposition) bool {
		return !strings.HasPrefix(field.Path, rev1.Objects[0].ExternalRef+".")
	})
	rev1.Boundary = nil
	rev2 := rev1
	rev2.Batch, rev2.Cohort = "batch-ac05-revision-2", "cohort-ac05-revision-2"
	rev2.Objects = slices.Clone(rev1.Objects)
	rev2.Objects[0].Revision = 2
	rev2.Objects[0].Payload = strings.ReplaceAll(strings.ReplaceAll(rev2.Objects[0].Payload, `"revision":1`, `"revision":2`), `mapped-source:v1`, `mapped-source:v2`)
	rev2.Mappings = slices.Clone(rev1.Mappings)
	rev2.Mappings[0].Revision = 2
	rev2.Evidence = slices.Clone(rev1.Evidence)
	for i := range rev2.Evidence {
		if rev2.Evidence[i].Disposition == "required" {
			rev2.Evidence[i].SourceRevision = 2
		}
	}
	firstResult := make(chan error, 1)
	go func() {
		_, importErr := service.Import(t.Context(), actor, migration.ImportRequest{Manifest: rev1})
		firstResult <- importErr
	}()
	select {
	case <-paused:
	case <-t.Context().Done():
		t.Fatal("rev1 did not pause before owner apply")
	}
	newer, err := service.Import(t.Context(), actor, migration.ImportRequest{Manifest: rev2})
	if err != nil || newer.State != "complete" || newer.Applied != 1 {
		t.Fatal("rev2 failed to replace the paused reservation", err, newer)
	}
	close(release)
	if err = <-firstResult; !errors.Is(err, migration.ErrConflict) {
		t.Fatal("superseded rev1 reached owner apply", err)
	}
	if len(applied) != 1 || <-applied != 2 {
		t.Fatal("superseded revision produced an owner effect")
	}
	stale, _, stalePlan, err := db.Batch(t.Context(), actor, rev1.Batch)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Checkpoint(t.Context(), actor, rev1.Batch, stale.Revision, stalePlan.Objects[0], "stale:applied"); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("stale checkpoint overwrote the newer owner", err)
	}
	raw := support.Raw(t, dsn)
	var oldCheckpoints, newCheckpoints int
	if err = raw.QueryRow(t.Context(), `SELECT count(*) FILTER (WHERE batch_id=$2),count(*) FILTER (WHERE batch_id=$3) FROM chartworks.migration_checkpoints WHERE tenant_id=$1`, actor.Tenant(), rev1.Batch, rev2.Batch).Scan(&oldCheckpoints, &newCheckpoints); err != nil || oldCheckpoints != 0 || newCheckpoints != 1 {
		t.Fatal("cross-revision checkpoint fence", err, oldCheckpoints, newCheckpoints)
	}
	phase34ReservationHeldDuringApply(t)
}

func phase34ReservationHeldDuringApply(t *testing.T) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	actor := phase34Actor(t, "tenant-ac05-held", "migration.read", "migration.write")
	entered := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	applied := make(chan int64, 2)
	adapter := migration.AdapterFuncs{
		ValidateFunc: func(context.Context, identity.Envelope, migration.Object, migration.Mapping) error { return nil },
		ApplyFunc: func(ctx context.Context, _ identity.Envelope, object migration.Object, mapping migration.Mapping, _ string) (string, error) {
			if object.Revision == 1 {
				close(entered)
				select {
				case <-release:
				case <-ctx.Done():
					return "", ctx.Err()
				}
			}
			applied <- object.Revision
			return mapping.Destination, nil
		},
	}
	service, err := migration.New(db, map[migration.Kind]migration.Adapter{migration.KindSource: adapter}, nil, migration.EvidenceVerifierFunc(func(context.Context, identity.Envelope, migration.Evidence) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	rev1 := phase34Manifest("ac05-held")
	rev1.Objects = slices.Clone(rev1.Objects[:1])
	rev1.Fields = slices.DeleteFunc(rev1.Fields, func(field migration.FieldDisposition) bool {
		return !strings.HasPrefix(field.Path, rev1.Objects[0].ExternalRef+".")
	})
	rev1.Boundary = nil
	rev2 := rev1
	rev2.Batch, rev2.Cohort = "batch-ac05-held-2", "cohort-ac05-held-2"
	rev2.Objects = slices.Clone(rev1.Objects)
	rev2.Objects[0].Revision = 2
	rev2.Objects[0].Payload = strings.ReplaceAll(strings.ReplaceAll(rev2.Objects[0].Payload, `"revision":1`, `"revision":2`), `mapped-source:v1`, `mapped-source:v2`)
	rev2.Mappings = slices.Clone(rev1.Mappings)
	rev2.Mappings[0].Revision = 2
	rev2.Evidence = slices.Clone(rev1.Evidence)
	for i := range rev2.Evidence {
		if rev2.Evidence[i].Disposition == "required" {
			rev2.Evidence[i].SourceRevision = 2
		}
	}
	firstResult := make(chan error, 1)
	go func() {
		_, importErr := service.Import(t.Context(), actor, migration.ImportRequest{Manifest: rev1})
		firstResult <- importErr
	}()
	select {
	case <-entered:
	case <-t.Context().Done():
		t.Fatal("rev1 owner effect did not start")
	}
	raw := support.Raw(t, dsn)
	_, lockErr := raw.Exec(t.Context(), `SELECT external_ref FROM chartworks.migration_ref_reservations WHERE tenant_id=$1 AND kind='source' AND external_ref=$2 FOR UPDATE NOWAIT`, actor.Tenant(), rev1.Objects[0].ExternalRef)
	var pgErr *pgconn.PgError
	if !errors.As(lockErr, &pgErr) || pgErr.Code != "55P03" {
		t.Fatal("owner effect did not hold its source reservation", lockErr)
	}
	secondResult := make(chan error, 1)
	go func() {
		_, importErr := service.Import(t.Context(), actor, migration.ImportRequest{Manifest: rev2})
		secondResult <- importErr
	}()
	close(release)
	if err = <-firstResult; err != nil {
		t.Fatal("rev1 failed while holding its reservation", err)
	}
	if err = <-secondResult; err != nil {
		t.Fatal("rev2 failed after rev1 checkpoint", err)
	}
	if len(applied) != 2 || <-applied != 1 || <-applied != 2 {
		t.Fatal("revision effects escaped reservation order")
	}
}

func phase34CutoverRollback(t *testing.T) {
	s, _, e, dsn := phase34Service(t, "ac06")
	m := phase34Manifest("ac06")
	oldRoute, newRoute, accepted, queueDB, scope := phase34ScheduleRoutes(t, dsn, e.Tenant())
	m.Boundary.ResumeAfter = time.Now().UTC().Add(time.Hour)
	m.Boundary.LastAccepted, m.Boundary.LastDue = accepted.ID, accepted.DueAt
	m.Mappings = append(m.Mappings, migration.Mapping{Kind: migration.KindSchedule, ExternalRef: "schedule-ac06", Destination: newRoute, Revision: 1})
	if _, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	request := migration.CutoverRequest{Batch: m.Batch, Route: newRoute, PreviousRoute: oldRoute, OperatorRef: "cutover-drill", Expected: 0}
	narrow, err := identity.FromVerified(e.Tenant(), "narrow", "session", []string{"migration.cutover", "scheduling.write", "cw.tenant.write:" + e.Tenant(), "cw.schedule.write:" + newRoute, "sources.read", "cw.source.read:mapped-source", "cw.execution_context.use:mapped-source:v1"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Cutover(t.Context(), narrow, request); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("target-only schedule reach toggled prior route", err)
	}
	wrongPrior := request
	wrongPrior.PreviousRoute = newRoute
	if _, err = s.Cutover(t.Context(), e, wrongPrior); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("unproven prior route was accepted", err)
	}
	competingSchedule, err := queueDB.CreateSchedule(t.Context(), scope, "session", "competing-route", jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}, jobs.Defaults())
	if err != nil {
		t.Fatal("create competing migration route", err)
	}
	competing := request
	competing.Route, competing.OperatorRef = competingSchedule.ID, "competing-cutover-drill"
	type cutoverResult struct {
		cut migration.Cutover
		err error
	}
	results := make(chan cutoverResult, 2)
	for _, contender := range []migration.CutoverRequest{request, competing} {
		go func(contender migration.CutoverRequest) {
			cut, err := s.Cutover(t.Context(), e, contender)
			results <- cutoverResult{cut: cut, err: err}
		}(contender)
	}
	successes, conflicts := 0, 0
	winningRoute := ""
	for range 2 {
		result := <-results
		switch {
		case result.err == nil && result.cut.Generation == 1 && result.cut.Boundary.Stream == m.Boundary.Stream && (result.cut.Route == request.Route || result.cut.Route == competing.Route):
			successes++
			winningRoute = result.cut.Route
		case errors.Is(result.err, migration.ErrConflict):
			conflicts++
		default:
			t.Fatal("concurrent cutover escaped CAS", result.err, result.cut)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatal("concurrent cutover did not choose one writer", successes, conflicts)
	}
	winningRequest := request
	if winningRoute == competing.Route {
		winningRequest = competing
	}
	losingRoute := request.Route
	if winningRoute == request.Route {
		losingRoute = competing.Route
	}
	newRoute = winningRoute
	if converged, err := s.Cutover(t.Context(), e, winningRequest); err != nil || converged.Generation != 1 {
		t.Fatal("exact winning cutover did not converge on retry", err, converged)
	}
	replay, err := s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: newRoute, PreviousRoute: oldRoute, OperatorRef: winningRequest.OperatorRef, Expected: 1})
	if err != nil || replay.Generation != 1 {
		t.Fatal("cutover replay duplicated stream", err, replay)
	}
	replay, err = s.Cutover(t.Context(), e, winningRequest)
	if err != nil || replay.Generation != 1 {
		t.Fatal("exact cutover retry duplicated stream", err, replay)
	}
	if _, err = s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: losingRoute, PreviousRoute: oldRoute, OperatorRef: "stale-racer", Expected: 0}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("stale concurrent cutover", err)
	}
	oldSchedule, err := queueDB.ReadSchedule(t.Context(), scope, oldRoute)
	if err != nil {
		t.Fatal("read fenced previous route", err)
	}
	newSchedule, err := queueDB.ReadSchedule(t.Context(), scope, newRoute)
	if err != nil {
		t.Fatal("read active target route", err)
	}
	if _, err = queueDB.FireSchedule(t.Context(), scope, "session", oldRoute, "old-fenced", oldSchedule.Revision, jobs.Defaults()); !errors.Is(err, store.ErrConflict) {
		t.Fatal("old stream admitted after cutover", err)
	}
	if _, err = queueDB.FireSchedule(t.Context(), scope, "session", newRoute, "before-boundary", newSchedule.Revision, jobs.Defaults()); !errors.Is(err, store.ErrConflict) {
		t.Fatal("target occurrence at or before resume boundary was admitted", err)
	}
	raw := support.Raw(t, dsn)
	boundary := time.Now().UTC().Add(-time.Minute)
	if _, err = raw.Exec(t.Context(), `UPDATE chartworks.migration_cutovers SET boundary=jsonb_set(boundary,'{resume_after}',to_jsonb($3::timestamptz)) WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), m.Cohort, boundary); err != nil {
		t.Fatal("prepare post-boundary occurrence fixture", err)
	}
	queued, err := queueDB.FireSchedule(t.Context(), scope, "session", newRoute, "target-live", newSchedule.Revision, jobs.Defaults())
	if err != nil {
		t.Fatal("target stream blocked after cutover", err)
	}
	var admittedStream, admittedSchedule string
	var admittedGeneration int64
	var admittedDue time.Time
	if err = raw.QueryRow(t.Context(), `SELECT stream_id,generation,schedule_id,due_at FROM chartworks.migration_occurrence_admissions WHERE tenant_id=$1 AND stream_id=$2 ORDER BY due_at DESC LIMIT 1`, e.Tenant(), m.Boundary.Stream).Scan(&admittedStream, &admittedGeneration, &admittedSchedule, &admittedDue); err != nil || admittedStream != m.Boundary.Stream || admittedGeneration != 1 || admittedSchedule != newRoute || !admittedDue.After(boundary) {
		t.Fatal("accepted occurrence did not persist its exact cutover generation and boundary", err, admittedStream, admittedGeneration, admittedSchedule, admittedDue)
	}
	if _, err = raw.Exec(t.Context(), `UPDATE chartworks.migration_cutovers SET boundary=jsonb_set(boundary,'{resume_after}',to_jsonb($3::timestamptz)) WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), m.Cohort, time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal("advance dispatch boundary fixture", err)
	}
	if _, err = queueDB.ClaimJob(t.Context(), "phase34-boundary-worker", jobs.Defaults()); !errors.Is(err, jobs.ErrEmpty) {
		t.Fatal("dispatch claimed an occurrence at or before the live resume boundary", err)
	}
	if _, err = raw.Exec(t.Context(), `UPDATE chartworks.migration_cutovers SET boundary=jsonb_set(boundary,'{resume_after}',to_jsonb($3::timestamptz)) WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), m.Cohort, boundary); err != nil {
		t.Fatal("restore cutover boundary", err)
	}
	// Hold the same advisory transaction fence as ClaimJob. Queue rollback first,
	// then a claim, and release it only after both sessions are waiting. The claim
	// must observe the committed rollback generation rather than the old route.
	lockTx, err := raw.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockTx.Rollback(t.Context()) }()
	if _, err = lockTx.Exec(t.Context(), `SELECT pg_advisory_xact_lock(7214060601)`); err != nil {
		t.Fatal(err)
	}
	type rollbackResult struct {
		cut migration.Cutover
		err error
	}
	rollbackDone := make(chan rollbackResult, 1)
	go func() {
		cut, rollbackErr := s.Rollback(t.Context(), e, migration.RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "rollback-drill", Effects: []string{"notification_already_delivered"}})
		rollbackDone <- rollbackResult{cut: cut, err: rollbackErr}
	}()
	waitPhase34AdvisoryWaiters(t, dsn, 1)
	claimDone := make(chan error, 1)
	go func() {
		_, claimErr := queueDB.ClaimJob(t.Context(), "phase34-racing-worker", jobs.Defaults())
		claimDone <- claimErr
	}()
	waitPhase34AdvisoryWaiters(t, dsn, 2)
	if err = lockTx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	rollbackOutcome := <-rollbackDone
	rolled, err := rollbackOutcome.cut, rollbackOutcome.err
	if err != nil || rolled.State != "rolled_back" || len(rolled.IrreversibleEffects) != 1 {
		t.Fatal(err, rolled)
	}
	if claimErr := <-claimDone; !errors.Is(claimErr, jobs.ErrEmpty) {
		t.Fatal("claim leased a stale target occurrence across rollback", claimErr)
	}
	oldSchedule, err = queueDB.ReadSchedule(t.Context(), scope, oldRoute)
	if err != nil {
		t.Fatal("read restored previous route", err)
	}
	newSchedule, err = queueDB.ReadSchedule(t.Context(), scope, newRoute)
	if err != nil {
		t.Fatal("read rolled back target route", err)
	}
	if _, err = queueDB.FireSchedule(t.Context(), scope, "session", newRoute, "target-fenced", newSchedule.Revision, jobs.Defaults()); !errors.Is(err, store.ErrConflict) {
		t.Fatal("target stream admitted after rollback", err)
	}
	if _, err = queueDB.FireSchedule(t.Context(), scope, "session", oldRoute, "old-live", oldSchedule.Revision, jobs.Defaults()); err != nil {
		t.Fatal("old stream blocked after rollback", err)
	}
	replayedRollback, err := s.Rollback(t.Context(), e, migration.RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "rollback-drill", Effects: []string{"notification_already_delivered"}})
	if err != nil || replayedRollback.Generation != rolled.Generation {
		t.Fatal("exact rollback retry changed generation", err, replayedRollback)
	}
	var events int
	if err = raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.migration_cutover_events WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), m.Cohort).Scan(&events); err != nil || events != 2 {
		t.Fatal("retry duplicated cutover occurrence events", err, events)
	}
	lease, err := queueDB.ClaimJob(t.Context(), "phase34-rollback-worker", jobs.Defaults())
	if err != nil || lease.Job.ScheduleID != oldRoute || lease.Job.DueAt.Before(rolled.Boundary.ResumeAfter) {
		t.Fatal("dispatch crossed the rollback generation fence", err, lease.Job.ScheduleID, lease.Job.DueAt)
	}
	if _, err = queueDB.ClaimJob(t.Context(), "phase34-rollback-worker-2", jobs.Defaults()); !errors.Is(err, jobs.ErrEmpty) {
		t.Fatal("stale target occurrence remained dispatchable after rollback", err, queued.ID)
	}
	phase34CurrentSourceCutover(t)
}

func phase34CurrentSourceCutover(t *testing.T) {
	t.Helper()
	dsn := support.Database(t)
	db := support.Open(t, dsn)
	tenant := "tenant-ac06-current-source"
	manifest := phase34Manifest("ac06-current-source")
	oldRoute, newRoute, accepted, queueDB, scope := phase34ScheduleRoutes(t, dsn, tenant)
	manifest.Boundary.LastAccepted, manifest.Boundary.LastDue = accepted.ID, accepted.DueAt
	manifest.Boundary.ResumeAfter = accepted.DueAt.Add(time.Minute)
	manifest.Mappings = append(manifest.Mappings, migration.Mapping{Kind: migration.KindSchedule, ExternalRef: "schedule-ac06-current-source", Destination: newRoute, Revision: 1})
	type sourceState struct {
		Engine, Dialect, Snapshot, Context string
		Revision                           int64
		Available                          bool
	}
	current := sourceState{Engine: manifest.Engine, Dialect: manifest.Dialect, Snapshot: manifest.SourceSnapshot, Context: "mapped-source:v1", Revision: 1, Available: true}
	baseline := current
	sourceAdapter := migration.AdapterFuncs{
		ValidateFunc: func(_ context.Context, e identity.Envelope, object migration.Object, mapping migration.Mapping) error {
			if err := access.Require(e, "sources.read", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: mapping.Destination}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: current.Context}); err != nil {
				return err
			}
			var expected struct {
				Engine, Dialect, Snapshot, Context string
				Revision                           int64
			}
			if json.Unmarshal([]byte(object.Payload), &expected) != nil || !current.Available || current.Engine != expected.Engine || current.Dialect != expected.Dialect || current.Snapshot != expected.Snapshot || current.Context != expected.Context || current.Revision != expected.Revision || mapping.Revision != current.Revision {
				return migration.ErrConflict
			}
			return nil
		},
		ApplyFunc: func(_ context.Context, _ identity.Envelope, _ migration.Object, mapping migration.Mapping, _ string) (string, error) {
			return mapping.Destination, nil
		},
	}
	otherAdapter := &phase34Adapter{}
	adapters := map[migration.Kind]migration.Adapter{migration.KindSource: sourceAdapter}
	for _, kind := range []migration.Kind{migration.KindUpload, migration.KindProfile, migration.KindTopic, migration.KindRule, migration.KindTemplate, migration.KindRuntimePack, migration.KindEvalSuite, migration.KindBlock, migration.KindReport, migration.KindDashboard, migration.KindFilter, migration.KindSchedule, migration.KindTombstone, migration.KindCalibration} {
		adapters[kind] = otherAdapter
	}
	service, err := migration.New(db, adapters, nil, migration.EvidenceVerifierFunc(func(context.Context, identity.Envelope, migration.Evidence) error { return nil }))
	if err != nil {
		t.Fatal(err)
	}
	actor := phase34Actor(t, tenant, "migration.read", "migration.write", "migration.cutover", "scheduling.write", "cw.schedule.write:*", "sources.read", "cw.source.read:mapped-source", "cw.execution_context.use:mapped-source:v1")
	if _, err = service.Import(t.Context(), actor, migration.ImportRequest{Manifest: manifest}); err != nil {
		t.Fatal("import pinned source", err)
	}
	request := migration.CutoverRequest{Batch: manifest.Batch, Route: newRoute, PreviousRoute: oldRoute, OperatorRef: "source-freshness-drill"}
	for _, reach := range []struct {
		name   string
		scopes []string
	}{
		{"source", []string{"cw.execution_context.use:mapped-source:v1"}},
		{"context", []string{"cw.source.read:mapped-source"}},
	} {
		narrow := phase34Actor(t, tenant, append([]string{"migration.cutover", "scheduling.write", "cw.schedule.write:*", "sources.read"}, reach.scopes...)...)
		if _, err = service.Cutover(t.Context(), narrow, request); !errors.Is(err, access.ErrNotFound) {
			t.Fatal("cutover actor without exact source/context reach changed route", reach.name, err)
		}
	}
	mutations := []struct {
		name   string
		mutate func(*sourceState)
	}{
		{"unavailable", func(s *sourceState) { s.Available = false }},
		{"engine", func(s *sourceState) { s.Engine = "other" }},
		{"dialect", func(s *sourceState) { s.Dialect = "other" }},
		{"snapshot", func(s *sourceState) { s.Snapshot = strings.Repeat("f", 64) }},
		{"revision", func(s *sourceState) { s.Revision = 2 }},
	}
	for _, test := range mutations {
		current = baseline
		test.mutate(&current)
		if _, err = service.Cutover(t.Context(), actor, request); !errors.Is(err, migration.ErrNotReady) {
			t.Fatal("source drift changed route", test.name, err)
		}
		prior, readErr := queueDB.ReadSchedule(t.Context(), scope, oldRoute)
		if readErr != nil || !prior.Enabled {
			t.Fatal("source drift disabled the prior schedule", test.name, readErr, prior)
		}
	}
	current = baseline
	if _, err = service.Cutover(t.Context(), actor, request); err != nil {
		t.Fatal("healthy exact source blocked cutover", err)
	}
}

func phase34ScheduleRoutes(t *testing.T, dsn, tenant string) (string, string, jobs.Job, *postgres.DB, store.Scope) {
	t.Helper()
	db := support.Open(t, dsn)
	limits := jobs.Defaults()
	if err := db.ConfigureQueue(t.Context(), limits); err != nil {
		t.Fatal("configure migration route queue", err)
	}
	scope, err := store.NewScope(tenant, "operator")
	if err != nil {
		t.Fatal("migration route scope", err)
	}
	if _, err = db.SetPolicy(t.Context(), scope, 0, store.Policy{AuditDays: 7, OperationHours: 24}); err != nil {
		t.Fatal("configure migration route policy", err)
	}
	request := jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.MaintenanceKind, BindingID: "maintenance"}, Spec: jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}}
	oldSchedule, err := db.CreateSchedule(t.Context(), scope, "session", "old-route", request, limits)
	if err != nil {
		t.Fatal("create previous migration route", err)
	}
	accepted, err := db.FireSchedule(t.Context(), scope, "session", oldSchedule.ID, "last-accepted", oldSchedule.Revision, limits)
	if err != nil {
		t.Fatal("admit prior route boundary", err)
	}
	newSchedule, err := db.CreateImportedSchedule(t.Context(), scope, "session", "new-route", request, limits)
	if err != nil {
		t.Fatal("create target migration route", err)
	}
	if newSchedule.Enabled {
		t.Fatal("imported schedule was enabled at commit")
	}
	if _, err = db.FireSchedule(t.Context(), scope, "session", newSchedule.ID, "premature", newSchedule.Revision, limits); !errors.Is(err, store.ErrConflict) {
		t.Fatal("disabled imported schedule admitted an occurrence", err)
	}
	return oldSchedule.ID, newSchedule.ID, accepted, db, scope
}

func waitPhase34AdvisoryWaiters(t *testing.T, dsn string, want int) {
	t.Helper()
	raw := support.Raw(t, dsn)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiting int
		if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM pg_locks WHERE locktype='advisory' AND NOT granted`).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting >= want {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected %d advisory lock waiters", want)
}

func phase34FeatureClosure(t *testing.T) {
	s, _, e, _ := phase34Service(t, "ac07")
	m := phase34Manifest("ac07")
	m.Evidence[0].Outcome = "unsupported"
	plan, err := s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: m})
	if err != nil || plan.Ready {
		t.Fatal("unsupported required engine declared migrated", err, plan)
	}
	bad := m
	bad.Batch = "bad-ac07"
	bad.Evidence = slices.Clone(m.Evidence)
	bad.Evidence[len(bad.Evidence)-1].Outcome = "passed"
	if _, err = s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: bad}); !errors.Is(err, migration.ErrInvalid) {
		t.Fatal("discarded stub advertised supported", err)
	}
	for _, kind := range []migration.Kind{migration.KindSource, migration.KindSchedule} {
		fixture := phase34Manifest("ac07-" + string(kind))
		for i := range fixture.Objects {
			if fixture.Objects[i].Kind == kind {
				fixture.Objects[i].Retention.ExpiresAt = ptrTime(time.Now().Add(-time.Minute))
			}
		}
		plan, dryErr := s.DryRun(t.Context(), e, migration.DryRunRequest{Manifest: fixture})
		if dryErr != nil || plan.Ready {
			t.Fatal("critical quarantined object declared ready", kind, dryErr, plan)
		}
		batch, importErr := s.Import(t.Context(), e, migration.ImportRequest{Manifest: fixture})
		if importErr != nil || batch.State != "complete" {
			t.Fatal("quarantined batch was not inspectable", kind, importErr, batch)
		}
		if _, cutErr := s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: batch.ID, Route: "target", PreviousRoute: "prior", OperatorRef: "quarantine"}); !errors.Is(cutErr, migration.ErrNotReady) {
			t.Fatal("critical quarantine cut over", kind, cutErr)
		}
	}
}

func phase34OperationalBoundaries(t *testing.T) {
	s, _, e, dsn := phase34Service(t, "ac08")
	m := phase34Manifest("ac08")
	oldRoute, newRoute, accepted, _, _ := phase34ScheduleRoutes(t, dsn, e.Tenant())
	m.Boundary.LastAccepted, m.Boundary.LastDue = accepted.ID, accepted.DueAt
	m.Boundary.ResumeAfter = accepted.DueAt.Add(time.Minute)
	m.Mappings = append(m.Mappings, migration.Mapping{Kind: migration.KindSchedule, ExternalRef: "schedule-ac08", Destination: newRoute, Revision: 1})
	if _, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: newRoute, PreviousRoute: oldRoute, OperatorRef: "observed-drill", Expected: 0}); err != nil {
		t.Fatal(err)
	}
	raw := support.Raw(t, dsn)
	var events int
	if err := raw.QueryRow(t.Context(), `SELECT count(*) FROM chartworks.migration_cutover_events WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), m.Cohort).Scan(&events); err != nil || events != 1 {
		t.Fatal("operator evidence not durable", err, events)
	}
	manifest, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"password", "access_token", "client_secret"} {
		if json.Valid(manifest) && jsonContainsKey(manifest, needle) {
			t.Fatal("operator artifact carried credential field")
		}
	}
}

func jsonContainsKey(raw []byte, key string) bool {
	var v any
	if json.Unmarshal(raw, &v) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(x any) bool {
		switch y := x.(type) {
		case map[string]any:
			for k, v := range y {
				if k == key || walk(v) {
					return true
				}
			}
		case []any:
			for _, v := range y {
				if walk(v) {
					return true
				}
			}
		}
		return false
	}
	return walk(v)
}
