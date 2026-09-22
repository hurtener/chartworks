package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/test/support"
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

func (a *phase34Adapter) Validate(_ context.Context, _ identity.Envelope, _ migration.Object, _ migration.Mapping) error {
	return nil
}
func (a *phase34Adapter) Apply(_ context.Context, _ identity.Envelope, o migration.Object, m migration.Mapping, _ string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applied = append(a.applied, o.Kind)
	if m.Destination != "" {
		return m.Destination, nil
	}
	return "dst-" + o.ExternalRef, nil
}

func phase34Actor(t *testing.T, tenant string, scopes ...string) identity.Envelope {
	t.Helper()
	base := []string{"cw.tenant.read:" + tenant, "cw.tenant.write:" + tenant, "cw.tenant.erase:" + tenant}
	e, err := identity.FromVerified(tenant, "operator", "session", append(base, scopes...), time.Now().Add(time.Hour), time.Now)
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
	runtimeDigest, _ := runtime.Digest()
	suiteDigest, _ := suite.Digest()
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
		if kind == migration.KindRuntimePack {
			payload = string(runtimeRaw)
		}
		if kind == migration.KindEvalSuite {
			payload = string(suiteRaw)
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
			evidence = append(evidence, migration.Evidence{Feature: feature, Disposition: "required", Outcome: "passed", EvidenceType: "runtime", Reference: "evidence-" + strings.ToLower(feature), Source: "synthetic", SourceVersion: "v1"})
		}
	}
	evidence = append(evidence, migration.Evidence{Feature: "Q11", Disposition: "excluded", Outcome: "unsupported", EvidenceType: "operator", Reference: "discarded-stub", Source: "synthetic", SourceVersion: "v1"})
	calibration := fmt.Sprintf(`{"prompt_pack":"pack-one","optimization_revision":"opt-one","locale":"en-US","temperature":0.2,"max_output_tokens":2048,"example_policy_revision":"examples-one","template_thresholds":[{"template":"sales","threshold":0.72}],"evaluation_suite_digest":"%s","evaluation_run_digest":"%s","runtime_pack_digest":"%s"}`, suiteDigest, strings.Repeat("b", 64), runtimeDigest)
	return migration.Manifest{Version: migration.ManifestVersion, Batch: "batch-" + suffix, Cohort: "cohort-" + suffix, SourceSnapshot: "snapshot-" + suffix, Engine: "postgres", Dialect: "postgres", Mappings: []migration.Mapping{{Kind: migration.KindSource, ExternalRef: "source-" + suffix, Destination: "mapped-source", Revision: 1}}, Objects: objects, Fields: fields, Evidence: evidence, Calibration: &migration.Calibration{Revision: "cal-one", ModelVersion: "model-one", EmbeddingSpace: "space-one", BudgetVersion: "budget-one", Payload: calibration, State: "review_candidate"}, Boundary: &migration.OccurrenceBoundary{Stream: "stream-" + suffix, LastAccepted: "occurrence-prior", LastDue: now.Add(-time.Hour), ResumeAfter: now, ScheduleVersion: 1}}
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
	service, err := migration.New(db, adapters, nil)
	if err != nil {
		t.Fatal(err)
	}
	return service, adapter, phase34Actor(t, "tenant-"+suffix, "migration.read", "migration.write", "migration.cutover", "migration.erase"), dsn
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
	changed.Objects[0].Payload = `{"name":"changed"}`
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
	manifest := migration.Manifest{Version: migration.ManifestVersion, Batch: "batch-ac02-evaluation", Cohort: "cohort-ac02-evaluation", SourceSnapshot: "snapshot-ac02-evaluation", Engine: "postgres", Dialect: "postgres", Objects: objects, Fields: fields, Evidence: phase34Manifest("ac02-evidence").Evidence}
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

func phase34RetentionErasure(t *testing.T) {
	s, _, e, dsn := phase34Service(t, "ac05")
	m := phase34Manifest("ac05")
	batch, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m})
	if err != nil {
		t.Fatal(err)
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
}

func phase34CutoverRollback(t *testing.T) {
	s, _, e, dsn := phase34Service(t, "ac06")
	m := phase34Manifest("ac06")
	if _, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	cut, err := s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: "route-new", OperatorRef: "cutover-drill", Expected: 0})
	if err != nil || cut.Generation != 1 || cut.Boundary.Stream != m.Boundary.Stream {
		t.Fatal(err, cut)
	}
	replay, err := s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: "route-new", OperatorRef: "cutover-drill", Expected: 1})
	if err != nil || replay.Generation != 1 {
		t.Fatal("cutover replay duplicated stream", err, replay)
	}
	replay, err = s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: "route-new", OperatorRef: "cutover-drill", Expected: 0})
	if err != nil || replay.Generation != 1 {
		t.Fatal("exact cutover retry duplicated stream", err, replay)
	}
	if _, err = s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: "route-other", OperatorRef: "racer", Expected: 0}); !errors.Is(err, migration.ErrConflict) {
		t.Fatal("stale concurrent cutover", err)
	}
	rolled, err := s.Rollback(t.Context(), e, migration.RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "rollback-drill", Effects: []string{"notification_already_delivered"}})
	if err != nil || rolled.State != "rolled_back" || len(rolled.IrreversibleEffects) != 1 {
		t.Fatal(err, rolled)
	}
	replayedRollback, err := s.Rollback(t.Context(), e, migration.RollbackRequest{Cohort: m.Cohort, Expected: 1, OperatorRef: "rollback-drill", Effects: []string{"notification_already_delivered"}})
	if err != nil || replayedRollback.Generation != rolled.Generation {
		t.Fatal("exact rollback retry changed generation", err, replayedRollback)
	}
	var events int
	if err = support.Raw(t, dsn).QueryRow(t.Context(), `SELECT count(*) FROM chartworks.migration_cutover_events WHERE tenant_id=$1 AND cohort_id=$2`, e.Tenant(), m.Cohort).Scan(&events); err != nil || events != 2 {
		t.Fatal("retry duplicated cutover occurrence events", err, events)
	}
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
}

func phase34OperationalBoundaries(t *testing.T) {
	s, _, e, dsn := phase34Service(t, "ac08")
	m := phase34Manifest("ac08")
	if _, err := s.Import(t.Context(), e, migration.ImportRequest{Manifest: m}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cutover(t.Context(), e, migration.CutoverRequest{Batch: m.Batch, Route: "route-runbook", OperatorRef: "observed-drill", Expected: 0}); err != nil {
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
