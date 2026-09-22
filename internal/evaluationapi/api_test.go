package evaluationapi

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/store"
)

const d = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

type apiRepo struct {
	s     evaluation.SuiteRecord
	r     map[string]evaluation.Report
	input bool
	pack  evaluation.RuntimePackRecord
}

func (x *apiRepo) CreateRuntimePack(_ context.Context, _ store.Scope, v evaluation.RuntimePackRecord) error {
	x.pack = v
	return nil
}
func (x *apiRepo) ReviewRuntimePack(_ context.Context, _ store.Scope, v evaluation.RuntimePackReview) (evaluation.RuntimePackRecord, error) {
	if x.pack.State != evaluation.Draft || x.pack.Author == v.Reviewer || x.pack.Pack.Digest != v.PackDigest || x.pack.Digest != v.RuntimeDigest || x.pack.Config.Digest != v.ConfigurationDigest || x.pack.Config.AttemptCostUSD != v.MaxAttemptCostUSD {
		return evaluation.RuntimePackRecord{}, store.ErrConflict
	}
	x.pack.State = v.Decision
	x.pack.Review = &v
	return x.pack, nil
}
func (x *apiRepo) AcceptedRuntimePack(context.Context, store.Scope, string, string) (evaluation.RuntimePackRecord, error) {
	if x.pack.State != evaluation.Accepted {
		return x.pack, store.ErrNotFound
	}
	return x.pack, nil
}

func (x *apiRepo) CreateSuite(_ context.Context, _ store.Scope, v evaluation.SuiteRecord) error {
	x.s = v
	return nil
}

func (x *apiRepo) SaveInput(context.Context, store.Scope, evaluation.ProtectedRef, evaluation.LiveInput) error {
	x.input = true
	return nil
}
func (x *apiRepo) ReviewSuite(_ context.Context, _ store.Scope, v evaluation.SuiteReview) (evaluation.SuiteRecord, error) {
	x.s.State = v.Decision
	x.s.Review = &v
	return x.s, nil
}
func (x *apiRepo) AcceptedSuite(context.Context, store.Scope, string, int64, string) (evaluation.SuiteRecord, error) {
	if x.s.State != evaluation.Accepted {
		return x.s, store.ErrNotFound
	}
	return x.s, nil
}
func (*apiRepo) BeginRun(context.Context, store.Scope, evaluation.RunRequest) error { return nil }
func (x *apiRepo) SaveReport(_ context.Context, _ store.Scope, v evaluation.Report) error {
	x.r[v.RunID] = v
	return nil
}
func (x *apiRepo) ReadReport(_ context.Context, _ store.Scope, id string) (evaluation.Report, error) {
	v, ok := x.r[id]
	if !ok {
		return v, store.ErrNotFound
	}
	return v, nil
}
func (*apiRepo) RequestCancel(context.Context, store.Scope, string) error { return nil }
func (*apiRepo) RecoverRun(context.Context, store.Scope, string, time.Time) (evaluation.Report, error) {
	return evaluation.Report{}, store.ErrNotFound
}
func (*apiRepo) SaveFeedbackExport(context.Context, store.Scope, evaluation.CandidateExport) error {
	return nil
}
func (*apiRepo) ReadFeedbackExport(context.Context, store.Scope, string) (evaluation.CandidateExport, error) {
	return evaluation.CandidateExport{}, store.ErrNotFound
}
func (*apiRepo) ReviewFeedbackSplit(context.Context, store.Scope, string, string, evaluation.FeedbackSplit) error {
	return nil
}
func (*apiRepo) ValidateHeldoutCases(context.Context, store.Scope, []evaluation.Case) error {
	return nil
}
func (*apiRepo) ValidateOptimizationHeldout(context.Context, store.Scope, evaluation.Suite) error {
	return nil
}
func (*apiRepo) SaveProposal(context.Context, store.Scope, evaluation.OptimizationProposal) error {
	return nil
}
func (*apiRepo) ReadProposal(context.Context, store.Scope, string) (evaluation.OptimizationProposal, error) {
	return evaluation.OptimizationProposal{}, store.ErrNotFound
}
func (*apiRepo) ReviewProposal(context.Context, store.Scope, evaluation.ReviewReceipt) error {
	return nil
}
func (*apiRepo) SelectPack(context.Context, store.Scope, evaluation.PackSelection, int64) (evaluation.PackSelection, error) {
	return evaluation.PackSelection{}, store.ErrNotFound
}
func (*apiRepo) SelectedPack(context.Context, store.Scope) (evaluation.PackSelection, error) {
	return evaluation.PackSelection{}, store.ErrNotFound
}

func TestRegistryIncludesReviewedLifecycleAndRunConsumers(t *testing.T) {
	r, err := Registry()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"registerEvaluationInput": false, "authorEvaluationRuntimePack": false, "reviewEvaluationRuntimePack": false, "authorEvaluationSuite": false, "reviewEvaluationSuite": false, "runEvaluation": false, "readEvaluation": false, "cancelEvaluation": false, "recoverEvaluation": false, "exportEvaluationFeedback": false, "reviewEvaluationSplit": false, "proposeEvaluationOptimization": false, "reviewEvaluationOptimization": false, "selectEvaluationPack": false}
	for _, v := range r.Definitions() {
		if _, ok := want[v.ID]; ok {
			want[v.ID] = true
		}
	}
	for id, ok := range want {
		if !ok {
			t.Fatalf("missing %s", id)
		}
	}
}

func TestMCPRuntimePackLifecycleManifestAuthorityAndStrictInput(t *testing.T) {
	verifier, key, issuer, now := apiVerifier(t)
	repo := &apiRepo{r: map[string]evaluation.Report{}}
	svc, err := evaluation.New(repo, nil, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := MCPBindings(svc, nil)
	if err != nil || len(bindings) != 14 {
		t.Fatal(len(bindings), err)
	}
	mcpRegistry, err := mcpserver.NewRegistry(bindings)
	if err != nil {
		t.Fatal(err)
	}
	httpRegistry, _ := Registry()
	definitions := map[string]json.RawMessage{}
	for _, definition := range httpRegistry.Definitions() {
		definitions[definition.ID] = definition.Request.Document()
	}
	want := map[string]string{
		"author_evaluation_runtime_pack": "authorEvaluationRuntimePack",
		"review_evaluation_runtime_pack": "reviewEvaluationRuntimePack",
	}
	wantAction := map[string]string{"authorEvaluationRuntimePack": "ops.write", "reviewEvaluationRuntimePack": "ops.audit"}
	wantEffect := map[string]string{"authorEvaluationRuntimePack": "evaluation_runtime_pack_draft_commit", "reviewEvaluationRuntimePack": "evaluation_runtime_pack_review_commit"}
	for _, tool := range mcpRegistry.Manifest() {
		operation, _ := tool.Meta["chartworks/operation"].(string)
		expectedOperation, ok := want[tool.Name]
		if !ok {
			continue
		}
		if operation != expectedOperation || tool.Meta["chartworks/action"] != wantAction[operation] || tool.Meta["chartworks/effect"] != wantEffect[operation] || tool.Meta["chartworks/group"] != "evaluation" || tool.Meta["chartworks/persists"] != true || tool.Annotations == nil || tool.Annotations.ReadOnlyHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("runtime manifest drift: %#v", tool)
		}
		gotSchema, _ := json.Marshal(tool.InputSchema)
		var got, expected any
		if json.Unmarshal(gotSchema, &got) != nil || json.Unmarshal(definitions[operation], &expected) != nil || !reflect.DeepEqual(got, expected) {
			t.Fatalf("MCP/HTTP DTO drift for %s", operation)
		}
		delete(want, tool.Name)
	}
	if len(want) != 0 {
		t.Fatal("missing runtime pack MCP manifests", want)
	}
	mcpServer, err := mcpserver.New(verifier, mcpRegistry, config.Defaults().MCP, nil)
	if err != nil {
		t.Fatal(err)
	}
	actor := "author"
	bearer := apiTokenAudience(t, key, issuer, now, actor, "chartworks:mcp", []string{"mcp.use", "ops.write", "ops.audit", "cw.tenant.write:tenant", "cw.tenant.certify:tenant"})
	client, err := mcpServer.Client(func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	cfg := gateway.RuntimeConfig{Model: "model-v1", Models: []gateway.RuntimeModel{{Role: "sql_generation", Model: "model-v1"}}, SystemInstruction: "reviewed MCP instruction", AttemptCostUSD: 0.02}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	pack := evaluation.PackRevision{ID: "mcp-runtime", Revision: 1, Model: cfg.Model, Models: []evaluation.PackModel{{Role: "sql_generation", Model: "model-v1"}}, ConfigurationDigest: cfg.Digest}
	pack.Digest = pack.CanonicalDigest()
	raw, _ := json.Marshal(evaluation.RuntimePackAuthorRequest{Pack: pack, Config: cfg})
	result, err := client.CallTool(t.Context(), "author_evaluation_runtime_pack", raw)
	if err != nil || result == nil || result.IsError || repo.pack.Digest == "" || repo.pack.Author != actor {
		t.Fatal("runtime author MCP lifecycle failed", result, err, repo.pack)
	}
	review := RuntimePackReviewInput{PackDigest: pack.Digest, Request: evaluation.RuntimePackReviewRequest{PackID: pack.ID, PackRevision: pack.Revision, RuntimeDigest: repo.pack.Digest, ConfigurationDigest: cfg.Digest, Model: cfg.Model, Models: cfg.Models, SystemInstruction: cfg.SystemInstruction, MaxAttemptCostUSD: cfg.AttemptCostUSD, Decision: evaluation.Accepted}}
	raw, _ = json.Marshal(review)
	result, err = client.CallTool(t.Context(), "review_evaluation_runtime_pack", raw)
	if err != nil || result == nil || !result.IsError || repo.pack.State != evaluation.Draft {
		t.Fatal("same actor review was not denied", result, err, repo.pack)
	}
	bearer = apiTokenAudience(t, key, issuer, now, "reviewer", "chartworks:mcp", []string{"mcp.use", "ops.audit", "cw.tenant.certify:tenant"})
	result, err = client.CallTool(t.Context(), "review_evaluation_runtime_pack", raw)
	if err != nil || result == nil || result.IsError || repo.pack.State != evaluation.Accepted || repo.pack.Review == nil || repo.pack.Review.Reviewer != "reviewer" {
		t.Fatal("distinct MCP runtime review failed", result, err, repo.pack)
	}
	repo.input = false
	legacy, _ := json.Marshal(map[string]any{"pack": pack, "runtime_config": cfg})
	raw, _ = json.Marshal(ProtectedInputRequest{Retention: "protected", Material: string(legacy)})
	bearer = apiTokenAudience(t, key, issuer, now, actor, "chartworks:mcp", []string{"mcp.use", "ops.write", "cw.tenant.write:tenant"})
	result, err = client.CallTool(t.Context(), "register_evaluation_input", raw)
	if err != nil || result == nil || !result.IsError || repo.input {
		t.Fatal("MCP accepted caller runtime_config", result, err)
	}
}
func TestHTTPReviewedSuiteRunAndRead(t *testing.T) {
	verifier, key, issuer, now := apiVerifier(t)
	repo := &apiRepo{r: map[string]evaluation.Report{}}
	svc, _ := evaluation.New(repo, nil, func() time.Time { return now })
	server := httptest.NewServer(Handler(verifier, svc, nil, http.NotFoundHandler()))
	defer server.Close()
	q := 1.0
	obs := evaluation.Observation{Decision: "ok", SemanticDigest: d}
	cfg := gateway.RuntimeConfig{Model: "model-v1", SystemInstruction: "reviewed", AttemptCostUSD: 0.01}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	pack := evaluation.PackRevision{ID: "default", Revision: 1, Model: "model-v1", ConfigurationDigest: cfg.Digest}
	pack.Digest = pack.CanonicalDigest()
	suite := evaluation.Suite{SchemaVersion: 1, ID: "suite", Revision: 1, Mode: evaluation.Fixture, Seed: 1, Calibration: "reviewed", Threshold: evaluation.Threshold{QualityMin: &q}, Limits: evaluation.Limits{Cases: 1, Calls: 1, Tokens: 1, DurationMS: 1000}, Provenance: evaluation.Provenance{Implementation: "head", EnvironmentDigest: d, ConfigurationDigest: d, SemanticVersion: "v1", RuleVersion: "v1", SourceSnapshot: d, DialectMatrix: []evaluation.DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: evaluation.Fixture, EvidenceDigest: d, Status: "measured"}}}, Packs: []evaluation.PackRevision{pack}, Frontiers: []string{"EVAL-01"}, Cases: []evaluation.Case{{ID: "case", Stage: evaluation.StageRouting, Locale: "en", HeldOut: true, Input: evaluation.ProtectedRef{Digest: d, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: "ok", SemanticDigest: d}}, Fixture: &obs}}}
	author := apiToken(t, key, issuer, now, "author", []string{"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant"})
	if _, err := verifier.Verify(context.Background(), author, auth.HTTP); err != nil {
		t.Fatalf("verify: %v", err)
	}
	var inputRef evaluation.ProtectedRef
	material, _ := json.Marshal(evaluation.LiveInput{Pack: pack})
	apiPost(t, server.URL+"/v1/evaluations/inputs", author, ProtectedInputRequest{Retention: "protected", Material: string(material)}, &inputRef)
	if !repo.input || inputRef.Digest == "" {
		t.Fatal("protected input consumer not wired", inputRef)
	}
	legacyMaterial, _ := json.Marshal(map[string]any{"pack": pack, "runtime_config": cfg})
	legacyRequest, _ := json.Marshal(ProtectedInputRequest{Retention: "protected", Material: string(legacyMaterial)})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/evaluations/inputs", bytes.NewReader(legacyRequest))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+author)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatal("caller-supplied runtime cost accepted", resp.StatusCode)
	}
	var draft evaluation.SuiteRecord
	apiPost(t, server.URL+"/v1/evaluations/suites", author, suite, &draft)
	reviewer := apiToken(t, key, issuer, now, "reviewer", []string{"ops.audit", "cw.tenant.certify:tenant"})
	var runtimeDraft evaluation.RuntimePackRecord
	apiPost(t, server.URL+"/v1/evaluations/runtime-packs", author, evaluation.RuntimePackAuthorRequest{Pack: pack, Config: cfg}, &runtimeDraft)
	apiPost(t, server.URL+"/v1/evaluations/runtime-packs/review", reviewer, RuntimePackReviewInput{PackDigest: pack.Digest, Request: evaluation.RuntimePackReviewRequest{PackID: pack.ID, PackRevision: pack.Revision, RuntimeDigest: runtimeDraft.Digest, ConfigurationDigest: cfg.Digest, Model: cfg.Model, SystemInstruction: cfg.SystemInstruction, MaxAttemptCostUSD: cfg.AttemptCostUSD, Decision: evaluation.Accepted}}, &evaluation.RuntimePackRecord{})
	var accepted evaluation.SuiteRecord
	apiPost(t, server.URL+"/v1/evaluations/suites/review", reviewer, SuiteReviewInput{SuiteID: "suite", Request: evaluation.SuiteReviewRequest{Revision: 1, Digest: draft.Digest, Decision: evaluation.Accepted}}, &accepted)
	var report evaluation.Report
	apiPost(t, server.URL+"/v1/evaluations/runs", author, evaluation.RunRequest{RunID: "run", SuiteID: "suite", SuiteRevision: 1, SuiteDigest: draft.Digest, PackDigest: draft.Suite.Packs[0].Digest}, &report)
	if report.Status != "passed" {
		t.Fatal(report)
	}
	var read evaluation.Report
	apiPost(t, server.URL+"/v1/evaluations/runs/read", author, ReadRequest{RunID: "run"}, &read)
	if read.EvidenceHash != report.EvidenceHash {
		t.Fatal("public read drift")
	}
}

func TestHTTPLiveAdversarialReplayShadowRetainsTerminalReport(t *testing.T) {
	verifier, key, issuer, now := apiVerifier(t)
	repo := &apiRepo{r: map[string]evaluation.Report{}}
	svc, _ := evaluation.New(repo, nil, func() time.Time { return now })
	runner := evaluation.RunnerFunc(func(_ context.Context, x evaluation.Execution) (evaluation.Observation, error) {
		if x.Case.Stage == evaluation.StageAdversarial {
			return evaluation.Observation{Decision: "blocked", SemanticDigest: d, ErrorClass: x.Case.Category, Blocked: true}, nil
		}
		return evaluation.Observation{Decision: "retained", SemanticDigest: d}, nil
	})
	server := httptest.NewServer(Handler(verifier, svc, runner, http.NotFoundHandler()))
	defer server.Close()
	q := 1.0
	cases := []evaluation.Case{}
	for _, category := range []string{"identity_scope", "injection", "dialect_escape", "resource_exhaustion", "byo", "frozen_report"} {
		cases = append(cases, evaluation.Case{ID: "case-" + category, Stage: evaluation.StageAdversarial, Category: category, Locale: "en", Critical: true, Input: evaluation.ProtectedRef{Digest: d, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: "blocked", SemanticDigest: d, ErrorClass: category}}})
	}
	for _, stage := range []evaluation.Stage{evaluation.StageReplay, evaluation.StageShadow} {
		cases = append(cases, evaluation.Case{ID: "case-" + string(stage), Stage: stage, Locale: "es", HeldOut: true, Input: evaluation.ProtectedRef{Digest: d, Retention: "protected"}, Expected: []evaluation.Expected{{Decision: "retained", SemanticDigest: d}}})
	}
	cfg := gateway.RuntimeConfig{Model: "model-v1", SystemInstruction: "reviewed", AttemptCostUSD: 0.01}
	cfg.Digest = gateway.ConfigurationDigest(cfg)
	pack := evaluation.PackRevision{ID: "default", Revision: 1, Model: "model-v1", ConfigurationDigest: cfg.Digest}
	pack.Digest = pack.CanonicalDigest()
	suite := evaluation.Suite{SchemaVersion: 1, ID: "live-suite", Revision: 1, Mode: evaluation.Live, Seed: 2, Calibration: "reviewed", Threshold: evaluation.Threshold{QualityMin: &q}, Limits: evaluation.Limits{Cases: 8, Calls: 8, Tokens: 8, Retries: 1, DurationMS: 1000}, Provenance: evaluation.Provenance{Implementation: "head", EnvironmentDigest: d, ConfigurationDigest: d, SemanticVersion: "v1", RuleVersion: "v1", SourceSnapshot: d, DialectMatrix: []evaluation.DialectEvidence{{Engine: "postgres", Dialect: "postgres", Mode: evaluation.Live, EvidenceDigest: d, Status: "measured"}}}, Packs: []evaluation.PackRevision{pack}, Frontiers: []string{"EVAL-01"}, Cases: cases}
	author := apiToken(t, key, issuer, now, "author", []string{"ops.write", "ops.read", "cw.tenant.write:tenant", "cw.tenant.read:tenant"})
	reviewer := apiToken(t, key, issuer, now, "reviewer", []string{"ops.audit", "cw.tenant.certify:tenant"})
	var runtimeDraft evaluation.RuntimePackRecord
	apiPost(t, server.URL+"/v1/evaluations/runtime-packs", author, evaluation.RuntimePackAuthorRequest{Pack: pack, Config: cfg}, &runtimeDraft)
	apiPost(t, server.URL+"/v1/evaluations/runtime-packs/review", reviewer, RuntimePackReviewInput{PackDigest: pack.Digest, Request: evaluation.RuntimePackReviewRequest{PackID: pack.ID, PackRevision: pack.Revision, RuntimeDigest: runtimeDraft.Digest, ConfigurationDigest: cfg.Digest, Model: cfg.Model, SystemInstruction: cfg.SystemInstruction, MaxAttemptCostUSD: cfg.AttemptCostUSD, Decision: evaluation.Accepted}}, &evaluation.RuntimePackRecord{})
	var draft evaluation.SuiteRecord
	apiPost(t, server.URL+"/v1/evaluations/suites", author, suite, &draft)
	apiPost(t, server.URL+"/v1/evaluations/suites/review", reviewer, SuiteReviewInput{SuiteID: suite.ID, Request: evaluation.SuiteReviewRequest{Revision: 1, Digest: draft.Digest, Decision: evaluation.Accepted}}, &evaluation.SuiteRecord{})
	var report evaluation.Report
	apiPost(t, server.URL+"/v1/evaluations/runs", author, evaluation.RunRequest{RunID: "live-terminal", SuiteID: suite.ID, SuiteRevision: 1, SuiteDigest: draft.Digest, PackDigest: pack.Digest}, &report)
	if report.Status != "passed" || !report.GatePassed || report.SecurityFailures != 0 || len(report.Cases) != 8 || repo.r[report.RunID].EvidenceHash != report.EvidenceHash {
		t.Fatal(report)
	}
}
func apiPost(t *testing.T, url, token string, in, out any) {
	t.Helper()
	raw, _ := json.Marshal(in)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if json.NewDecoder(resp.Body).Decode(out) != nil {
		t.Fatal("decode")
	}
}
func apiVerifier(t *testing.T) (*auth.Verifier, *ecdsa.PrivateKey, string, time.Time) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	doc, _ := json.Marshal(map[string]any{"keys": []any{map[string]any{"kty": "EC", "use": "sig", "alg": "ES256", "kid": "key", "crv": "P-256", "x": base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))), "y": base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32)))}}})
	jwks := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	}))
	t.Cleanup(jwks.Close)
	now := time.Now().Truncate(time.Second)
	cfg := config.Defaults().Auth
	cfg.Issuer = jwks.URL + "/issuer"
	cfg.JWKSURL = jwks.URL
	cfg.Audience = ""
	cfg.Audiences = config.Audiences{HTTP: "chartworks:http", MCP: "chartworks:mcp"}
	cfg.RefreshInterval = config.Duration(time.Second)
	cfg.JWKSMaxStale = config.Duration(10 * time.Second)
	v, err := auth.New(cfg, jwks.Client(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(v.Close)
	return v, key, cfg.Issuer, now
}
func apiToken(t *testing.T, key *ecdsa.PrivateKey, issuer string, now time.Time, user string, scopes []string) string {
	return apiTokenAudience(t, key, issuer, now, user, "chartworks:http", scopes)
}

func apiTokenAudience(t *testing.T, key *ecdsa.PrivateKey, issuer string, now time.Time, user, audience string, scopes []string) string {
	t.Helper()
	sort.Strings(scopes)
	claims := jwt.MapClaims{"iss": issuer, "aud": audience, "sub": user, "tenant": "tenant", "user": user, "session": "session-" + user, "iat": now.Unix(), "nbf": now.Add(-time.Minute).Unix(), "exp": now.Add(5 * time.Minute).Unix(), "scopes": scopes}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["kid"] = "key"
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
