package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// PerformanceProfileKind separates a bounded development measurement from the
// final release stress profile. Production commands only inspect these profiles.
type PerformanceProfileKind string

const (
	// PerformanceSmoke identifies the bounded development profile.
	PerformanceSmoke PerformanceProfileKind = "smoke"
	// PerformanceFinalStress identifies the Phase 25 release profile.
	PerformanceFinalStress PerformanceProfileKind = "final_stress"
)

// PerformanceEvidenceMode prevents synthetic, recorded, and live evidence from
// being compared as if they measured the same boundary.
type PerformanceEvidenceMode string

const (
	// PerformanceSynthetic records harness-only evidence without source or model work.
	PerformanceSynthetic PerformanceEvidenceMode = "synthetic"
	// PerformanceIntegration records real-source and recorded-model evidence.
	PerformanceIntegration PerformanceEvidenceMode = "integration"
	// PerformanceLive records real-source and live-model evidence.
	PerformanceLive PerformanceEvidenceMode = "live"
)

// PerformanceBinding is the required revision/authority identity in the
// profile. Digests are content-free references to reviewed material; the
// product reuse key must be observed independently by a concrete adapter.
type PerformanceBinding struct {
	TenantDigest      string `json:"tenant_digest"`
	ContextDigest     string `json:"context_digest"`
	ActionsDigest     string `json:"actions_digest"`
	SourceRevision    string `json:"source_revision"`
	RuleRevision      string `json:"rule_revision"`
	TopicRevision     string `json:"topic_revision"`
	RuntimePackDigest string `json:"runtime_pack_digest"`
}

func (b PerformanceBinding) digest() string {
	raw, _ := json.Marshal(b)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// PerformanceEnvironment records the machine and adapter boundaries used for
// raw values. SourceMode is synthetic or real_postgres. ModelMode is none,
// recorded, or live.
type PerformanceEnvironment struct {
	RunnerLabel             string `json:"runner_label"`
	OS                      string `json:"os"`
	Architecture            string `json:"architecture"`
	CPUs                    int    `json:"cpus"`
	GoVersion               string `json:"go_version"`
	DatasetDigest           string `json:"dataset_digest"`
	DatasetRows             int64  `json:"dataset_rows"`
	SourceMode              string `json:"source_mode"`
	ModelMode               string `json:"model_mode"`
	EvaluationSuiteID       string `json:"evaluation_suite_id"`
	EvaluationSuiteRevision int64  `json:"evaluation_suite_revision"`
	EvaluationSuite         string `json:"evaluation_suite_digest"`
	EvaluationReportID      string `json:"evaluation_report_id"`
	EvaluationReport        string `json:"evaluation_report_hash"`
	WorkloadCaseID          string `json:"workload_case_id"`
}

// PerformanceAuthorityFixture is a content-free fixture representing one
// already verified authority envelope and the resolved resources presented to
// the real access enforcer. Allowed remains an expectation; it never controls
// the authorization decision.
type PerformanceAuthorityFixture struct {
	Tenant       string   `json:"tenant"`
	User         string   `json:"user"`
	Session      string   `json:"session"`
	Scopes       []string `json:"scopes"`
	TargetTenant string   `json:"target_tenant"`
	ReportID     string   `json:"report_id"`
	SourceID     string   `json:"source_id"`
	ContextID    string   `json:"context_id"`
}

// PerformanceStep is one ordered cold/warm/reuse/invalidation experiment.
// ExpectedExecutions counts physical source/model pipeline executions; blocked
// authority negatives must expect zero.
type PerformanceStep struct {
	ID                 string                       `json:"id"`
	Kind               string                       `json:"kind"`
	Binding            PerformanceBinding           `json:"binding"`
	EvidenceCaseID     string                       `json:"evidence_case_id,omitempty"`
	EvidenceReportID   string                       `json:"evidence_report_id,omitempty"`
	EvidenceReport     string                       `json:"evidence_report_hash,omitempty"`
	Allowed            bool                         `json:"allowed"`
	ResetBefore        bool                         `json:"reset_before"`
	Iterations         int                          `json:"iterations"`
	Concurrency        int                          `json:"concurrency"`
	Workload           string                       `json:"workload"`
	ExpectedDigest     string                       `json:"expected_digest"`
	ExpectedExecutions int                          `json:"expected_executions"`
	ExpectedBlocks     int                          `json:"expected_blocks"`
	DeniedAction       string                       `json:"denied_action,omitempty"`
	AuthorityOverride  *PerformanceAuthorityFixture `json:"authority_override,omitempty"`
}

// PerformanceManifest is immutable input for a bounded measurement.
type PerformanceManifest struct {
	SchemaVersion int                         `json:"schema_version"`
	ID            string                      `json:"id"`
	Kind          PerformanceProfileKind      `json:"kind"`
	EvidenceMode  PerformanceEvidenceMode     `json:"evidence_mode"`
	Environment   PerformanceEnvironment      `json:"environment"`
	Authority     PerformanceAuthorityFixture `json:"authority"`
	Steps         []PerformanceStep           `json:"steps"`
	MaxDurationMS int64                       `json:"max_duration_ms"`
}

var requiredPerformanceKinds = []string{
	"cold", "warm", "repeat", "concurrent", "source_changed", "rule_changed",
	"context_changed", "topic_changed", "runtime_pack_changed", "tenant_negative", "context_negative", "actions_negative",
}

// Validate enforces bounded, authority-aware and attribution-complete input.
func (m PerformanceManifest) Validate() error {
	if m.SchemaVersion != 1 || !identifier(m.ID) || (m.Kind != PerformanceSmoke && m.Kind != PerformanceFinalStress) || (m.EvidenceMode != PerformanceSynthetic && m.EvidenceMode != PerformanceIntegration && m.EvidenceMode != PerformanceLive) || m.MaxDurationMS < 1 || m.MaxDurationMS > int64((24*time.Hour)/time.Millisecond) {
		return ErrInvalid
	}
	e := m.Environment
	if !identifier(e.RunnerLabel) || e.OS == "" || e.Architecture == "" || e.CPUs < 1 || e.GoVersion == "" || !validDigest(e.DatasetDigest) || e.DatasetRows < 1 || !identifier(e.EvaluationSuiteID) || e.EvaluationSuiteRevision < 1 || !validDigest(e.EvaluationSuite) || !identifier(e.EvaluationReportID) || !validDigest(e.EvaluationReport) || !identifier(e.WorkloadCaseID) {
		return ErrInvalid
	}
	if e.SourceMode != "synthetic" && e.SourceMode != "real_postgres" || e.ModelMode != "none" && e.ModelMode != "recorded" && e.ModelMode != "live" {
		return ErrInvalid
	}
	switch m.EvidenceMode {
	case PerformanceSynthetic:
		if e.SourceMode != "synthetic" || e.ModelMode != "none" || m.Kind != PerformanceSmoke {
			return ErrInvalid
		}
	case PerformanceIntegration:
		if e.SourceMode != "real_postgres" || e.ModelMode != "recorded" || m.Kind != PerformanceFinalStress {
			return ErrInvalid
		}
	case PerformanceLive:
		if e.SourceMode != "real_postgres" || e.ModelMode != "live" || m.Kind != PerformanceFinalStress {
			return ErrInvalid
		}
	}
	if !validPerformanceAuthorityFixture(m.Authority) || len(m.Steps) < len(requiredPerformanceKinds) || len(m.Steps) > 64 {
		return ErrInvalid
	}
	limitIterations, limitConcurrency := 32, 32
	if m.Kind == PerformanceFinalStress {
		limitIterations, limitConcurrency = 100000, 256
	}
	kinds := map[string]PerformanceStep{}
	ids := map[string]bool{}
	for _, s := range m.Steps {
		if !identifier(s.ID) || ids[s.ID] || !validDigest(s.ExpectedDigest) || s.Workload == "" || len(s.Workload) > 4096 || s.Iterations < 1 || s.Iterations > limitIterations || s.Concurrency < 1 || s.Concurrency > limitConcurrency || s.Concurrency > s.Iterations || s.ExpectedExecutions < 0 || s.ExpectedExecutions > s.Iterations || s.ExpectedBlocks < 0 || s.ExpectedBlocks > s.Iterations {
			return ErrInvalid
		}
		if s.EvidenceCaseID != "" && !identifier(s.EvidenceCaseID) || (s.EvidenceReportID == "") != (s.EvidenceReport == "") || s.EvidenceReportID != "" && (!identifier(s.EvidenceReportID) || !validDigest(s.EvidenceReport)) {
			return ErrInvalid
		}
		if (s.EvidenceReportID != "" || s.EvidenceReport != "") && s.EvidenceCaseID == "" {
			return ErrInvalid
		}
		if s.DeniedAction != "" && (s.Kind != "actions_negative" || !identifier(s.DeniedAction)) || s.Kind == "actions_negative" && !identifier(s.DeniedAction) {
			return ErrInvalid
		}
		if !validPerformanceBinding(s.Binding) || s.Allowed && s.ExpectedBlocks != 0 || !s.Allowed && (s.ExpectedBlocks != s.Iterations || s.ExpectedExecutions != 0) {
			return ErrInvalid
		}
		if s.AuthorityOverride != nil && !validPerformanceAuthorityFixture(*s.AuthorityOverride) {
			return ErrInvalid
		}
		if s.Allowed && s.AuthorityOverride != nil {
			return ErrInvalid
		}
		ids[s.ID] = true
		if _, exists := kinds[s.Kind]; exists {
			return ErrInvalid
		}
		kinds[s.Kind] = s
	}
	for _, kind := range requiredPerformanceKinds {
		if _, ok := kinds[kind]; !ok {
			return ErrInvalid
		}
	}
	base := kinds["cold"].Binding
	for kind, field := range map[string]string{"source_changed": "source", "rule_changed": "rule", "context_changed": "context", "topic_changed": "topic", "runtime_pack_changed": "runtime", "tenant_negative": "tenant", "context_negative": "context", "actions_negative": "actions"} {
		if m.Kind == PerformanceFinalStress && strings.HasSuffix(kind, "_changed") && kind != "runtime_pack_changed" {
			if !finalPerformanceDependencyClosure(base, kinds[kind].Binding, kind) {
				return ErrInvalid
			}
			continue
		}
		if !onlyPerformanceBindingFieldChanged(base, kinds[kind].Binding, field) {
			return ErrInvalid
		}
	}
	for _, kind := range []string{"warm", "repeat", "concurrent"} {
		if kinds[kind].Binding != base {
			return ErrInvalid
		}
	}
	positive := []string{"cold", "warm", "repeat", "concurrent", "source_changed", "rule_changed", "context_changed", "topic_changed", "runtime_pack_changed"}
	for _, kind := range positive {
		step := kinds[kind]
		wantExecutions := 0
		if kind == "cold" || kind == "concurrent" || strings.HasSuffix(kind, "_changed") {
			wantExecutions = 1
		}
		if !step.Allowed || step.ExpectedExecutions != wantExecutions || step.ExpectedBlocks != 0 || step.ResetBefore != (kind == "cold" || kind == "concurrent") {
			return ErrInvalid
		}
		if m.Kind == PerformanceFinalStress {
			if strings.HasSuffix(kind, "_changed") && step.EvidenceCaseID == "" {
				return ErrInvalid
			}
			if strings.HasSuffix(kind, "_changed") && step.Workload != step.EvidenceCaseID {
				return ErrInvalid
			}
			if strings.HasSuffix(kind, "_changed") && step.EvidenceReportID == "" {
				return ErrInvalid
			}
			if (kind == "cold" || kind == "warm" || kind == "repeat" || kind == "concurrent") && (step.EvidenceCaseID != "" || step.EvidenceReportID != "") {
				return ErrInvalid
			}
			if (kind == "cold" || kind == "warm" || kind == "repeat" || kind == "concurrent") && step.Workload != e.WorkloadCaseID {
				return ErrInvalid
			}
		}
	}
	if kinds["concurrent"].Concurrency < 2 || kinds["tenant_negative"].Allowed || kinds["context_negative"].Allowed || kinds["actions_negative"].Allowed {
		return ErrInvalid
	}
	for _, kind := range []string{"tenant_negative", "context_negative", "actions_negative"} {
		if kinds[kind].AuthorityOverride == nil {
			return ErrInvalid
		}
	}
	tenantNegative := *kinds["tenant_negative"].AuthorityOverride
	contextNegative := *kinds["context_negative"].AuthorityOverride
	actionsNegative := *kinds["actions_negative"].AuthorityOverride
	if tenantNegative.TargetTenant == m.Authority.TargetTenant || tenantNegative.ReportID != m.Authority.ReportID || tenantNegative.SourceID != m.Authority.SourceID || tenantNegative.ContextID != m.Authority.ContextID || tenantNegative.Tenant != m.Authority.Tenant || tenantNegative.User != m.Authority.User || tenantNegative.Session != m.Authority.Session || !sameStrings(tenantNegative.Scopes, m.Authority.Scopes) {
		return ErrInvalid
	}
	if contextNegative.TargetTenant != m.Authority.TargetTenant || contextNegative.ReportID != m.Authority.ReportID || contextNegative.SourceID != m.Authority.SourceID || contextNegative.ContextID == m.Authority.ContextID || contextNegative.Tenant != m.Authority.Tenant || contextNegative.User != m.Authority.User || contextNegative.Session != m.Authority.Session || !sameStrings(contextNegative.Scopes, m.Authority.Scopes) {
		return ErrInvalid
	}
	if actionsNegative.TargetTenant != m.Authority.TargetTenant || actionsNegative.ReportID != m.Authority.ReportID || actionsNegative.SourceID != m.Authority.SourceID || actionsNegative.ContextID != m.Authority.ContextID || actionsNegative.Tenant != m.Authority.Tenant || actionsNegative.User != m.Authority.User || actionsNegative.Session != m.Authority.Session || sameStrings(actionsNegative.Scopes, m.Authority.Scopes) {
		return ErrInvalid
	}
	deniedAction := kinds["actions_negative"].DeniedAction
	if m.Kind == PerformanceFinalStress {
		if deniedAction != "query.plan" && deniedAction != "query.execute" && deniedAction != "reporting.execute" {
			return ErrInvalid
		}
		if !onlyScopeRemoved(m.Authority.Scopes, actionsNegative.Scopes, deniedAction) {
			return ErrInvalid
		}
	} else {
		for _, scope := range m.Authority.Scopes {
			if scope == deniedAction {
				return ErrInvalid
			}
		}
	}
	if m.Kind == PerformanceFinalStress && (m.MaxDurationMS != int64(time.Hour/time.Millisecond) || !validFinalPerformanceShape(kinds)) {
		return ErrInvalid
	}
	return nil
}

func onlyScopeRemoved(base, altered []string, removed string) bool {
	if len(base) != len(altered)+1 {
		return false
	}
	want := make([]string, 0, len(altered))
	found := false
	for _, scope := range base {
		if scope == removed {
			found = true
			continue
		}
		want = append(want, scope)
	}
	return found && sameStrings(want, altered)
}

func validPerformanceAuthorityFixture(a PerformanceAuthorityFixture) bool {
	if !identifier(a.Tenant) || !identifier(a.User) || !identifier(a.Session) || !identifier(a.TargetTenant) || !identifier(a.ReportID) || !identifier(a.SourceID) || !identifier(a.ContextID) || len(a.Scopes) == 0 || len(a.Scopes) > 32 {
		return false
	}
	seen := map[string]bool{}
	for _, scope := range a.Scopes {
		if scope == "" || len(scope) > 256 || seen[scope] {
			return false
		}
		seen[scope] = true
	}
	return true
}

func (m PerformanceManifest) effectiveStep(step PerformanceStep) PerformanceStep {
	if step.AuthorityOverride == nil {
		authority := m.Authority
		step.AuthorityOverride = &authority
	}
	return step
}

func validFinalPerformanceShape(steps map[string]PerformanceStep) bool {
	want := map[string][2]int{
		"cold": {20, 1}, "warm": {1000, 16}, "repeat": {10000, 64}, "concurrent": {2000, 128},
		"source_changed": {500, 32}, "rule_changed": {500, 32}, "context_changed": {500, 32}, "topic_changed": {500, 32}, "runtime_pack_changed": {500, 32},
		"tenant_negative": {1000, 64}, "context_negative": {1000, 64}, "actions_negative": {1000, 64},
	}
	for kind, dimensions := range want {
		step := steps[kind]
		if step.Iterations != dimensions[0] || step.Concurrency != dimensions[1] {
			return false
		}
	}
	return true
}

func validPerformanceBinding(b PerformanceBinding) bool {
	return validDigest(b.TenantDigest) && validDigest(b.ContextDigest) && validDigest(b.ActionsDigest) && validDigest(b.SourceRevision) && validDigest(b.RuleRevision) && validDigest(b.TopicRevision) && validDigest(b.RuntimePackDigest)
}

func onlyPerformanceBindingFieldChanged(a, b PerformanceBinding, field string) bool {
	changed := 0
	check := func(name, x, y string) {
		if x != y {
			changed++
			if name != field {
				changed = 99
			}
		}
	}
	check("tenant", a.TenantDigest, b.TenantDigest)
	check("context", a.ContextDigest, b.ContextDigest)
	check("actions", a.ActionsDigest, b.ActionsDigest)
	check("source", a.SourceRevision, b.SourceRevision)
	check("rule", a.RuleRevision, b.RuleRevision)
	check("topic", a.TopicRevision, b.TopicRevision)
	check("runtime", a.RuntimePackDigest, b.RuntimePackDigest)
	return changed == 1
}

// Current source/context publications are pinned into topics, and current
// rules pin the reviewed topic version/digest. A final profile must declare
// the primary axis and include exactly its owner dependency closure. Values
// are compared with freshly resolved owner evidence before any measurement.
func finalPerformanceDependencyClosure(base, changed PerformanceBinding, kind string) bool {
	if changed.TenantDigest != base.TenantDigest || changed.ActionsDigest != base.ActionsDigest || changed.RuntimePackDigest != base.RuntimePackDigest {
		return false
	}
	source := changed.SourceRevision != base.SourceRevision
	rule := changed.RuleRevision != base.RuleRevision
	context := changed.ContextDigest != base.ContextDigest
	topic := changed.TopicRevision != base.TopicRevision
	switch kind {
	case "source_changed":
		return source && rule && topic && !context
	case "rule_changed":
		return rule && !source && !context && !topic
	case "context_changed":
		return context && source && topic && rule
	case "topic_changed":
		return topic && rule && !source && !context
	default:
		return false
	}
}

// PerformanceUsage preserves unknown model/token/cost observations as nil.
type PerformanceUsage struct {
	ServiceNS   int64    `json:"service_ns"`
	SourceNS    *int64   `json:"source_ns,omitempty"`
	ModelNS     *int64   `json:"model_ns,omitempty"`
	SourceCalls int      `json:"source_calls"`
	ModelCalls  int      `json:"model_calls"`
	Retries     int      `json:"retries"`
	Tokens      *int     `json:"tokens,omitempty"`
	CostUSD     *float64 `json:"cost_usd,omitempty"`
}

// PerformanceReceipt is independently collected by a concrete adapter around
// its source and gateway seams. The harness derives executed versus reused from
// this receipt; an adapter cannot declare its own outcome.
type PerformanceReceipt struct {
	Usage      PerformanceUsage `json:"usage"`
	RunID      string           `json:"run_id,omitempty"`
	ReuseKey   string           `json:"reuse_key,omitempty"`
	ReusedFrom string           `json:"reused_from,omitempty"`
}

// PerformanceAdapterResult is returned by a concrete synthetic/real adapter.
type PerformanceAdapterResult struct {
	SemanticDigest string             `json:"semantic_digest"`
	BindingDigest  string             `json:"binding_digest"`
	Denied         bool               `json:"denied"`
	Receipt        PerformanceReceipt `json:"receipt"`
}

// PerformanceObservation contains the harness-derived terminal outcome.
type PerformanceObservation struct {
	SemanticDigest string           `json:"semantic_digest"`
	BindingDigest  string           `json:"binding_digest"`
	Blocked        bool             `json:"blocked"`
	Executed       bool             `json:"executed"`
	Reused         bool             `json:"reused"`
	Usage          PerformanceUsage `json:"usage"`
}

// PerformanceRunner is implemented by synthetic smoke, real PostgreSQL/source,
// and recorded/live model adapters. Check must not populate a measured cache.
type PerformanceRunner interface {
	Check(context.Context, PerformanceStep) (PerformanceAdapterResult, error)
	Reset(context.Context) error
	Run(context.Context, PerformanceStep, int) (PerformanceAdapterResult, error)
}

// A concrete adapter may need to make an ordered, authorized revision
// selection before each correctness probe and measured step. The harness calls
// this serially, outside the timed worker pool.
type performanceStepPreparer interface {
	PreparePerformanceStep(context.Context, PerformanceStep) error
}

// PerformanceSample is one raw measured request.
type PerformanceSample struct {
	StepID         string           `json:"step_id"`
	Iteration      int              `json:"iteration"`
	WallNS         int64            `json:"wall_ns"`
	SemanticDigest string           `json:"semantic_digest"`
	BindingDigest  string           `json:"binding_digest"`
	Blocked        bool             `json:"blocked"`
	Executed       bool             `json:"executed"`
	Reused         bool             `json:"reused"`
	Usage          PerformanceUsage `json:"usage"`
	RunID          string           `json:"run_id,omitempty"`
	ReuseKey       string           `json:"reuse_key,omitempty"`
	ReusedFrom     string           `json:"reused_from,omitempty"`
}

// PerformanceSummary derives aggregates from retained raw samples for one step.
type PerformanceSummary struct {
	StepID       string `json:"step_id"`
	Samples      int    `json:"samples"`
	Executions   int    `json:"executions"`
	Blocks       int    `json:"blocks"`
	MinWallNS    int64  `json:"min_wall_ns"`
	MedianWallNS int64  `json:"median_wall_ns"`
	P95WallNS    int64  `json:"p95_wall_ns"`
	MaxWallNS    int64  `json:"max_wall_ns"`
}

// PerformanceReport is content-free raw release evidence.
type PerformanceReport struct {
	SchemaVersion     int                     `json:"schema_version"`
	ManifestID        string                  `json:"manifest_id"`
	ManifestDigest    string                  `json:"manifest_digest"`
	CurrentBinding    PerformanceBinding      `json:"current_binding"`
	Kind              PerformanceProfileKind  `json:"kind"`
	EvidenceMode      PerformanceEvidenceMode `json:"evidence_mode"`
	Environment       PerformanceEnvironment  `json:"environment"`
	StartedAt         time.Time               `json:"started_at"`
	CompletedAt       time.Time               `json:"completed_at"`
	CorrectnessPassed bool                    `json:"correctness_passed"`
	Samples           []PerformanceSample     `json:"samples"`
	Summaries         []PerformanceSummary    `json:"summaries"`
	EvidenceHash      string                  `json:"evidence_hash"`
}

// Validate verifies a stored report against its immutable manifest, including
// raw observations, derived summaries, authority bindings, and evidence hash.
func (r PerformanceReport) Validate(manifest PerformanceManifest) error {
	if manifest.Validate() != nil || r.SchemaVersion != 1 || r.ManifestID != manifest.ID || r.CurrentBinding != coldPerformanceBinding(manifest) || r.Kind != manifest.Kind || r.EvidenceMode != manifest.EvidenceMode || r.Environment != manifest.Environment || !validDigest(r.ManifestDigest) || !validDigest(r.EvidenceHash) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || !r.CorrectnessPassed {
		return ErrInvalid
	}
	raw, _ := json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	if r.ManifestDigest != hex.EncodeToString(sum[:]) || len(r.Summaries) != len(manifest.Steps) {
		return ErrInvalid
	}
	byStep := make(map[string][]PerformanceSample, len(manifest.Steps))
	steps := make(map[string]PerformanceStep, len(manifest.Steps))
	for _, step := range manifest.Steps {
		steps[step.ID] = step
	}
	for _, sample := range r.Samples {
		step, ok := steps[sample.StepID]
		o := PerformanceObservation{SemanticDigest: sample.SemanticDigest, BindingDigest: sample.BindingDigest, Blocked: sample.Blocked, Executed: sample.Executed, Reused: sample.Reused, Usage: sample.Usage}
		if !ok || sample.Iteration < 0 || sample.WallNS < 0 || validatePerformanceObservation(manifest.Environment, manifest.effectiveStep(step), o, true) != nil || validatePerformanceLineage(manifest.Environment, o, sample.RunID, sample.ReuseKey, sample.ReusedFrom) != nil {
			return ErrInvalid
		}
		byStep[sample.StepID] = append(byStep[sample.StepID], sample)
	}
	for i, step := range manifest.Steps {
		samples := byStep[step.ID]
		if len(samples) != step.Iterations {
			return ErrInvalid
		}
		seen := make(map[int]bool, len(samples))
		for _, sample := range samples {
			if sample.Iteration >= step.Iterations || seen[sample.Iteration] {
				return ErrInvalid
			}
			seen[sample.Iteration] = true
		}
		summary := summarizePerformance(step.ID, samples)
		if summary != r.Summaries[i] || summary.Executions != step.ExpectedExecutions || summary.Blocks != step.ExpectedBlocks {
			return ErrInvalid
		}
	}
	sealed := sealPerformanceReport(r)
	if sealed.EvidenceHash != r.EvidenceHash {
		return ErrInvalid
	}
	return nil
}

// MeasurePerformance runs every correctness gate before collecting any timing.
func MeasurePerformance(ctx context.Context, manifest PerformanceManifest, runner PerformanceRunner, clock Clock) (PerformanceReport, error) {
	if ctx == nil || runner == nil || manifest.Validate() != nil {
		return PerformanceReport{}, ErrInvalid
	}
	if clock == nil {
		clock = time.Now
	}
	raw, _ := json.Marshal(manifest)
	sum := sha256.Sum256(raw)
	report := PerformanceReport{SchemaVersion: 1, ManifestID: manifest.ID, ManifestDigest: hex.EncodeToString(sum[:]), CurrentBinding: coldPerformanceBinding(manifest), Kind: manifest.Kind, EvidenceMode: manifest.EvidenceMode, Environment: manifest.Environment, StartedAt: clock().UTC()}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(manifest.MaxDurationMS)*time.Millisecond)
	defer cancel()
	for _, step := range manifest.Steps {
		step = manifest.effectiveStep(step)
		if preparer, ok := runner.(performanceStepPreparer); ok {
			if err := preparer.PreparePerformanceStep(runCtx, step); err != nil {
				report.CompletedAt = clock().UTC()
				return sealPerformanceReport(report), err
			}
		}
		result, err := runner.Check(runCtx, step)
		o, outcomeErr := derivePerformanceObservation(manifest.Environment, step, result, false)
		if err != nil || outcomeErr != nil || validatePerformanceObservation(manifest.Environment, step, o, false) != nil {
			report.CompletedAt = clock().UTC()
			if runCtx.Err() != nil {
				return sealPerformanceReport(report), runCtx.Err()
			}
			return sealPerformanceReport(report), ErrGate
		}
	}
	report.CorrectnessPassed = true
	for _, step := range manifest.Steps {
		step = manifest.effectiveStep(step)
		if preparer, ok := runner.(performanceStepPreparer); ok {
			if err := preparer.PreparePerformanceStep(runCtx, step); err != nil {
				report.CompletedAt = clock().UTC()
				return sealPerformanceReport(report), err
			}
		}
		if step.ResetBefore {
			if err := runner.Reset(runCtx); err != nil {
				report.CompletedAt = clock().UTC()
				return sealPerformanceReport(report), err
			}
		}
		samples, err := measurePerformanceStep(runCtx, manifest.Environment, step, runner)
		if err != nil {
			report.CompletedAt = clock().UTC()
			return sealPerformanceReport(report), err
		}
		report.Samples = append(report.Samples, samples...)
		summary := summarizePerformance(step.ID, samples)
		if summary.Executions != step.ExpectedExecutions || summary.Blocks != step.ExpectedBlocks {
			report.Summaries = append(report.Summaries, summary)
			report.CompletedAt = clock().UTC()
			return sealPerformanceReport(report), ErrGate
		}
		report.Summaries = append(report.Summaries, summary)
	}
	report.CompletedAt = clock().UTC()
	return sealPerformanceReport(report), nil
}

func coldPerformanceBinding(manifest PerformanceManifest) PerformanceBinding {
	for _, step := range manifest.Steps {
		if step.Kind == "cold" {
			return step.Binding
		}
	}
	return PerformanceBinding{}
}

func measurePerformanceStep(ctx context.Context, environment PerformanceEnvironment, step PerformanceStep, runner PerformanceRunner) ([]PerformanceSample, error) {
	results := make(chan PerformanceSample, step.Iterations)
	errs := make(chan error, step.Iterations)
	jobs := make(chan int, step.Iterations)
	for i := 0; i < step.Iterations; i++ {
		jobs <- i
	}
	close(jobs)
	var wg sync.WaitGroup
	for worker := 0; worker < step.Concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case iteration, ok := <-jobs:
					if !ok {
						return
					}
					began := time.Now()
					result, err := runner.Run(ctx, step, iteration)
					wall := time.Since(began).Nanoseconds()
					if err != nil {
						errs <- err
						return
					}
					o, err := derivePerformanceObservation(environment, step, result, true)
					if err != nil || validatePerformanceObservation(environment, step, o, true) != nil {
						errs <- ErrInvalid
						return
					}
					results <- PerformanceSample{StepID: step.ID, Iteration: iteration, WallNS: wall, SemanticDigest: o.SemanticDigest, BindingDigest: o.BindingDigest, Blocked: o.Blocked, Executed: o.Executed, Reused: o.Reused, Usage: o.Usage, RunID: result.Receipt.RunID, ReuseKey: result.Receipt.ReuseKey, ReusedFrom: result.Receipt.ReusedFrom}
				}
			}
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}
	out := make([]PerformanceSample, 0, step.Iterations)
	for sample := range results {
		out = append(out, sample)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Iteration < out[j].Iteration })
	if len(out) != step.Iterations {
		return nil, ErrInvalid
	}
	return out, nil
}

func derivePerformanceObservation(environment PerformanceEnvironment, step PerformanceStep, result PerformanceAdapterResult, measured bool) (PerformanceObservation, error) {
	u := result.Receipt.Usage
	physical := u.SourceCalls > 0 || u.ModelCalls > 0
	o := PerformanceObservation{SemanticDigest: result.SemanticDigest, BindingDigest: result.BindingDigest, Blocked: result.Denied, Usage: u}
	if measured && !result.Denied {
		o.Executed = physical
		o.Reused = !physical
	}
	if result.Denied && physical {
		return PerformanceObservation{}, ErrInvalid
	}
	if environment.ModelMode == "none" && (u.ModelCalls != 0 || u.ModelNS != nil || u.Tokens != nil || u.CostUSD != nil) {
		return PerformanceObservation{}, ErrInvalid
	}
	if err := validatePerformanceLineage(environment, o, result.Receipt.RunID, result.Receipt.ReuseKey, result.Receipt.ReusedFrom); err != nil {
		return PerformanceObservation{}, err
	}
	return o, nil
}

func validatePerformanceLineage(environment PerformanceEnvironment, o PerformanceObservation, runID, reuseKey, reusedFrom string) error {
	if runID == "" && reuseKey == "" && reusedFrom == "" {
		return nil // synthetic and ledger-only adapters have no product frozen-run lineage.
	}
	if !identifier(runID) || !validDigest(reuseKey) || reusedFrom != "" && (!identifier(reusedFrom) || reusedFrom == runID) || o.Blocked || o.Executed && reusedFrom != "" || o.Reused && reusedFrom == "" {
		return ErrInvalid
	}
	if environment.SourceMode != "real_postgres" || environment.ModelMode != "recorded" && environment.ModelMode != "live" {
		return ErrInvalid
	}
	return nil
}

func validatePerformanceObservation(environment PerformanceEnvironment, step PerformanceStep, o PerformanceObservation, measured bool) error {
	if o.SemanticDigest != step.ExpectedDigest || o.BindingDigest != step.Binding.digest() || o.Blocked != !step.Allowed || o.Executed && o.Reused || o.Usage.ServiceNS < 0 || o.Usage.SourceNS != nil && *o.Usage.SourceNS < 0 || o.Usage.ModelNS != nil && *o.Usage.ModelNS < 0 || o.Usage.SourceCalls < 0 || o.Usage.ModelCalls < 0 || o.Usage.Retries < 0 || o.Usage.Tokens != nil && *o.Usage.Tokens < 0 || o.Usage.CostUSD != nil && *o.Usage.CostUSD < 0 {
		return ErrInvalid
	}
	if measured && !o.Blocked && o.Executed == o.Reused {
		return ErrInvalid
	}
	if o.Blocked && (o.Executed || o.Reused || o.Usage.SourceCalls != 0 || o.Usage.ModelCalls != 0 || o.Usage.SourceNS != nil && *o.Usage.SourceNS != 0 || o.Usage.ModelNS != nil && *o.Usage.ModelNS != 0 || o.Usage.Retries != 0 || o.Usage.Tokens != nil && *o.Usage.Tokens != 0 || o.Usage.CostUSD != nil && *o.Usage.CostUSD != 0) {
		return ErrInvalid
	}
	if o.Reused && (o.Usage.SourceCalls != 0 || o.Usage.ModelCalls != 0 || o.Usage.SourceNS != nil && *o.Usage.SourceNS != 0 || o.Usage.ModelNS != nil && *o.Usage.ModelNS != 0 || o.Usage.Retries != 0 || o.Usage.Tokens != nil && *o.Usage.Tokens != 0 || o.Usage.CostUSD != nil && *o.Usage.CostUSD != 0) {
		return ErrInvalid
	}
	needsPhysicalReceipt := !measured || o.Executed
	if !o.Blocked && needsPhysicalReceipt && environment.SourceMode == "real_postgres" && (o.Usage.SourceNS == nil || o.Usage.SourceCalls < 1) {
		return ErrInvalid
	}
	if !o.Blocked && needsPhysicalReceipt && (environment.ModelMode == "recorded" || environment.ModelMode == "live") && (o.Usage.ModelNS == nil || o.Usage.ModelCalls < 1) {
		return ErrInvalid
	}
	return nil
}

func summarizePerformance(id string, samples []PerformanceSample) PerformanceSummary {
	walls := make([]int64, len(samples))
	s := PerformanceSummary{StepID: id, Samples: len(samples)}
	for i, sample := range samples {
		walls[i] = sample.WallNS
		if sample.Executed {
			s.Executions++
		}
		if sample.Blocked {
			s.Blocks++
		}
	}
	sort.Slice(walls, func(i, j int) bool { return walls[i] < walls[j] })
	if len(walls) > 0 {
		s.MinWallNS, s.MaxWallNS = walls[0], walls[len(walls)-1]
		s.MedianWallNS = walls[(len(walls)-1)/2]
		// Nearest-rank percentile: ceil(0.95*n)-1 in zero-based indexing.
		s.P95WallNS = walls[((95*len(walls)+99)/100)-1]
	}
	return s
}

func sealPerformanceReport(r PerformanceReport) PerformanceReport {
	r.EvidenceHash = ""
	raw, _ := json.Marshal(r)
	sum := sha256.Sum256(raw)
	r.EvidenceHash = hex.EncodeToString(sum[:])
	return r
}

// RuntimePerformanceEnvironment supplies explicit current machine metadata for
// newly authored manifests; dataset and evaluation hashes remain operator input.
func RuntimePerformanceEnvironment(label string) PerformanceEnvironment {
	return PerformanceEnvironment{RunnerLabel: label, OS: runtime.GOOS, Architecture: runtime.GOARCH, CPUs: runtime.NumCPU(), GoVersion: runtime.Version()}
}
