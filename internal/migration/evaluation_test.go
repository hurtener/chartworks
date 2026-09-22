package migration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

type evaluationRepoTest struct {
	evaluation.Repository
	packs  map[string]evaluation.RuntimePackRecord
	suites map[string]evaluation.SuiteRecord
}

func (r *evaluationRepoTest) CreateRuntimePack(_ context.Context, _ store.Scope, record evaluation.RuntimePackRecord) error {
	if _, exists := r.packs[record.Pack.Digest]; exists {
		return store.ErrConflict
	}
	r.packs[record.Pack.Digest] = record
	return nil
}
func (r *evaluationRepoTest) DraftRuntimePack(_ context.Context, _ store.Scope, digest string) (evaluation.RuntimePackRecord, error) {
	record, exists := r.packs[digest]
	if !exists || record.State != evaluation.Draft {
		return record, store.ErrNotFound
	}
	return record, nil
}
func (r *evaluationRepoTest) CreateSuite(_ context.Context, _ store.Scope, record evaluation.SuiteRecord) error {
	key := record.Suite.ID + ":" + record.Digest
	if _, exists := r.suites[key]; exists {
		return store.ErrConflict
	}
	r.suites[key] = record
	return nil
}
func (r *evaluationRepoTest) DraftSuite(_ context.Context, _ store.Scope, id string, revision int64) (evaluation.SuiteRecord, error) {
	for _, record := range r.suites {
		if record.Suite.ID == id && record.Suite.Revision == revision && record.State == evaluation.Draft {
			return record, nil
		}
	}
	return evaluation.SuiteRecord{}, store.ErrNotFound
}
func (*evaluationRepoTest) ValidateHeldoutCases(context.Context, store.Scope, []evaluation.Case) error {
	return nil
}

func TestEvaluationAdaptersDraftAndExactRetry(t *testing.T) {
	repo := &evaluationRepoTest{packs: map[string]evaluation.RuntimePackRecord{}, suites: map[string]evaluation.SuiteRecord{}}
	service, err := evaluation.New(repo, nil, func() time.Time { return time.Unix(10, 0).UTC() })
	if err != nil {
		t.Fatal(err)
	}
	adapters := EvaluationAdapters(service)
	if len(adapters) != 2 || EvaluationAdapters(nil) != nil {
		t.Fatal("evaluation adapter set")
	}
	envelope, err := identity.FromVerified("tenant", "actor", "session", []string{"ops.write", "cw.tenant.write:tenant"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	config := gateway.RuntimeConfig{Model: "model-v1", Models: []gateway.RuntimeModel{{Role: "routing", Model: "model-v1"}}, SystemInstruction: "synthetic instruction", AttemptCostUSD: 0.01}
	config.Digest = gateway.ConfigurationDigest(config)
	pack := evaluation.PackRevision{ID: "pack-v1", Revision: 1, Model: config.Model, Models: []evaluation.PackModel{{Role: "routing", Model: "model-v1"}}, ConfigurationDigest: config.Digest}
	pack.Digest = pack.CanonicalDigest()
	runtimeRaw, _ := json.Marshal(evaluation.RuntimePackAuthorRequest{Pack: pack, Config: config})
	runtimeObject := Object{Kind: KindRuntimePack, ExternalRef: "runtime-pack", Payload: string(runtimeRaw)}
	if err = validateEvaluationObject(runtimeObject); err != nil {
		t.Fatal(err)
	}
	if err = adapters[KindRuntimePack].Validate(t.Context(), envelope, runtimeObject, Mapping{}); err != nil {
		t.Fatal(err)
	}
	first, err := adapters[KindRuntimePack].Apply(t.Context(), envelope, runtimeObject, Mapping{}, strings.Repeat("a", 64))
	if err != nil || first == "" {
		t.Fatal(first, err)
	}
	second, err := adapters[KindRuntimePack].Apply(t.Context(), envelope, runtimeObject, Mapping{}, strings.Repeat("a", 64))
	if err != nil || second != first || repo.packs[pack.Digest].State != evaluation.Draft {
		t.Fatal("runtime retry promoted or changed draft", second, err)
	}
	badRuntime := runtimeObject
	badRuntime.Payload = strings.TrimSuffix(string(runtimeRaw), "}") + `,"unknown":true}`
	if err = adapters[KindRuntimePack].Validate(t.Context(), envelope, badRuntime, Mapping{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unknown runtime field accepted", err)
	}

	quality := 1.0
	digest := strings.Repeat("b", 64)
	observation := evaluation.Observation{Decision: "route", SemanticDigest: digest, Usage: evaluation.Usage{ServiceMS: 1}}
	suite := evaluation.Suite{SchemaVersion: evaluation.SchemaVersion, ID: "suite-v1", Revision: 1, Mode: evaluation.Fixture, Seed: 1, Calibration: "reviewed", Threshold: evaluation.Threshold{QualityMin: &quality}, Limits: evaluation.Limits{Cases: 1, Calls: 1, Tokens: 1, DurationMS: 100}, Provenance: evaluation.Provenance{Implementation: "sha", EnvironmentDigest: digest, ConfigurationDigest: config.Digest, SemanticVersion: "semantic-v1", RuleVersion: "rule-v1", SourceSnapshot: digest, DialectMatrix: []evaluation.DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: evaluation.Fixture, EvidenceDigest: digest, Status: "measured"}}}, Packs: []evaluation.PackRevision{pack}, Frontiers: []string{"EVAL-01"}, Cases: []evaluation.Case{{ID: "case", Stage: evaluation.StageRouting, Locale: "en", HeldOut: true, Input: evaluation.ProtectedRef{Digest: digest, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: "route", SemanticDigest: digest}}, Fixture: &observation}}}
	suiteRaw, _ := json.Marshal(suite)
	suiteObject := Object{Kind: KindEvalSuite, ExternalRef: "suite", Payload: string(suiteRaw)}
	if err = validateEvaluationObject(suiteObject); err != nil {
		t.Fatal(err)
	}
	if err = adapters[KindEvalSuite].Validate(t.Context(), envelope, suiteObject, Mapping{}); err != nil {
		t.Fatal(err)
	}
	first, err = adapters[KindEvalSuite].Apply(t.Context(), envelope, suiteObject, Mapping{}, strings.Repeat("c", 64))
	if err != nil || first == "" {
		t.Fatal(first, err)
	}
	second, err = adapters[KindEvalSuite].Apply(t.Context(), envelope, suiteObject, Mapping{}, strings.Repeat("c", 64))
	if err != nil || second != first {
		t.Fatal("suite retry changed draft", second, err)
	}
	runtimeDigest, _ := evaluation.RuntimePackAuthorRequest{Pack: pack, Config: config}.Digest()
	suiteDigest, _ := suite.Digest()
	packs, suites, err := evaluationObjectDigests([]Object{runtimeObject, suiteObject, {Kind: KindSource, Payload: `{"name":"source"}`}})
	if err != nil || !packs[runtimeDigest] || !suites[suiteDigest] || validateEvaluationObject(Object{Kind: KindSource}) != nil {
		t.Fatal("evaluation digest inventory", err, packs, suites)
	}
	if _, _, err = evaluationObjectDigests([]Object{{Kind: KindRuntimePack, Payload: `{"unknown":true}`}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid evaluation payload inventoried", err)
	}
	if !allowedModelRolePath(KindRuntimePack, []string{"pack", "models"}) || !allowedModelRolePath(KindEvalSuite, []string{"packs", "models"}) || allowedModelRolePath(KindRuntimePack, []string{"identity", "models"}) {
		t.Fatal("model role exception widened")
	}
}
