// Package evaluation runs deterministic, authority-bound quality suites.
// Reports contain hashes and bounded measurements, never prompts, SQL, or rows.
package evaluation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/gateway"
)

var (
	// ErrInvalid rejects malformed or unreviewed suite evidence.
	ErrInvalid = errors.New("evaluation: invalid manifest")
	// ErrGate reports a completed suite that did not meet its gate.
	ErrGate = errors.New("evaluation: gate failed")
	// ErrBudget reports bounded evaluation work exhaustion.
	ErrBudget = errors.New("evaluation: budget exhausted")
	// ErrMode reports fixture/live boundary misuse.
	ErrMode = errors.New("evaluation: execution mode unavailable")
	// ErrReview reports attempted promotion without explicit review.
	ErrReview = errors.New("evaluation: explicit review required")
)

// SchemaVersion identifies the manifest and report contract.
const SchemaVersion = 1

// Mode distinguishes fixture evidence from live observations.
type Mode string

// Evaluation execution modes.
const (
	Fixture Mode = "fixture"
	Live    Mode = "live"
)

// Stage identifies one evaluated pipeline or consumer boundary.
type Stage string

// Supported evaluation stages.
const (
	StageRouting     Stage = "routing"
	StageContext     Stage = "context"
	StageSQL         Stage = "sql"
	StageValidation  Stage = "validation"
	StageChart       Stage = "chart"
	StageReport      Stage = "report"
	StageReplay      Stage = "replay"
	StageShadow      Stage = "shadow"
	StageAdversarial Stage = "adversarial"
	StageConsumer    Stage = "consumer"
)

// Limits bounds total suite work.
type Limits struct {
	Cases      int      `json:"cases"`
	Calls      int      `json:"calls"`
	Tokens     int      `json:"tokens"`
	Retries    int      `json:"retries"`
	CostUSD    *float64 `json:"cost_usd,omitempty"`
	DurationMS int64    `json:"duration_ms"`
}

// Threshold contains reviewed gate values.
type Threshold struct {
	QualityMin       *float64 `json:"quality_min,omitempty"`
	SecurityFailures int      `json:"security_failures"`
}

// Lifecycle is durable state derived from an authenticated review receipt.
type Lifecycle string

const (
	// Draft has no review receipt and cannot execute.
	Draft Lifecycle = "draft"
	// Accepted has an exact independent approval receipt.
	Accepted Lifecycle = "accepted"
	// Rejected has an exact independent rejection receipt.
	Rejected Lifecycle = "rejected"
)

// SuiteReview binds a distinct signed actor's decision to one immutable revision.
type SuiteReview struct {
	SuiteID    string    `json:"suite_id"`
	Revision   int64     `json:"revision"`
	Digest     string    `json:"digest"`
	Decision   Lifecycle `json:"decision"`
	Reviewer   string    `json:"reviewer"`
	ReviewedAt time.Time `json:"reviewed_at"`
}

// SuiteRecord is the stored lifecycle projection. Author and reviewer are never caller supplied.
type SuiteRecord struct {
	Suite     Suite        `json:"suite"`
	Digest    string       `json:"digest"`
	State     Lifecycle    `json:"state"`
	Author    string       `json:"author"`
	CreatedAt time.Time    `json:"created_at"`
	Review    *SuiteReview `json:"review,omitempty"`
}

// SuiteReviewRequest pins review to exact immutable material.
type SuiteReviewRequest struct {
	Revision int64     `json:"revision"`
	Digest   string    `json:"digest"`
	Decision Lifecycle `json:"decision"`
}

// RunRequest selects only an accepted stored suite revision.
type RunRequest struct {
	RunID         string `json:"run_id"`
	SuiteID       string `json:"suite_id"`
	SuiteRevision int64  `json:"suite_revision"`
	SuiteDigest   string `json:"suite_digest"`
	PackDigest    string `json:"pack_digest,omitempty"`
}

// PackRevision is one immutable prompt/model/configuration revision admitted by a suite.
type PackRevision struct {
	ID                  string      `json:"id"`
	Revision            int64       `json:"revision"`
	Digest              string      `json:"digest"`
	Model               string      `json:"model"`
	Models              []PackModel `json:"models,omitempty"`
	ConfigurationDigest string      `json:"configuration_digest"`
}

// PackModel binds a gateway role to the exact reviewed model for that role.
type PackModel struct {
	Role  string `json:"role"`
	Model string `json:"model"`
}

// RuntimePackRecord is the server-owned reviewed gateway configuration for one
// exact pack revision. Evaluation inputs never carry this material.
type RuntimePackRecord struct {
	Pack      PackRevision          `json:"pack"`
	Config    gateway.RuntimeConfig `json:"config"`
	Digest    string                `json:"digest"`
	State     Lifecycle             `json:"state"`
	Author    string                `json:"author"`
	CreatedAt time.Time             `json:"created_at"`
	Review    *RuntimePackReview    `json:"review,omitempty"`
}

// RuntimePackAuthorRequest proposes exact runtime material for independent review.
type RuntimePackAuthorRequest struct {
	Pack   PackRevision          `json:"pack"`
	Config gateway.RuntimeConfig `json:"config"`
}

// RuntimePackReview binds a distinct reviewer to the exact effective model,
// role bindings, instruction configuration, and pessimistic per-attempt cost.
type RuntimePackReview struct {
	PackID              string                 `json:"pack_id"`
	PackRevision        int64                  `json:"pack_revision"`
	PackDigest          string                 `json:"pack_digest"`
	RuntimeDigest       string                 `json:"runtime_digest"`
	ConfigurationDigest string                 `json:"configuration_digest"`
	Model               string                 `json:"model"`
	Models              []gateway.RuntimeModel `json:"models,omitempty"`
	SystemInstruction   string                 `json:"system_instruction"`
	MaxAttemptCostUSD   float64                `json:"max_attempt_cost_usd"`
	Decision            Lifecycle              `json:"decision"`
	Reviewer            string                 `json:"reviewer"`
	ReviewedAt          time.Time              `json:"reviewed_at"`
}

// RuntimePackReviewRequest makes cost and routing material explicit at review;
// accepting a digest alone is insufficient.
type RuntimePackReviewRequest struct {
	PackID              string                 `json:"pack_id"`
	PackRevision        int64                  `json:"pack_revision"`
	RuntimeDigest       string                 `json:"runtime_digest"`
	ConfigurationDigest string                 `json:"configuration_digest"`
	Model               string                 `json:"model"`
	Models              []gateway.RuntimeModel `json:"models,omitempty"`
	SystemInstruction   string                 `json:"system_instruction"`
	MaxAttemptCostUSD   float64                `json:"max_attempt_cost_usd"`
	Decision            Lifecycle              `json:"decision"`
}

func runtimePackDigest(pack PackRevision, cfg gateway.RuntimeConfig) string {
	d, _ := digest(struct {
		Pack   PackRevision          `json:"pack"`
		Config gateway.RuntimeConfig `json:"config"`
	}{pack, cfg})
	return d
}

func validRuntimePack(r RuntimePackRecord) bool {
	if !validPack(r.Pack) || gateway.ConfigurationDigest(r.Config) != r.Config.Digest || r.Config.Digest != r.Pack.ConfigurationDigest || !packModelsMatchConfig(r.Pack, r.Config) || r.Config.AttemptCostUSD <= 0 || r.Config.AttemptCostUSD > 1000000 || r.Digest != runtimePackDigest(r.Pack, r.Config) || !identifier(r.Author) || r.CreatedAt.IsZero() {
		return false
	}
	if r.State == Draft {
		return r.Review == nil
	}
	if (r.State != Accepted && r.State != Rejected) || r.Review == nil || r.Review.PackID != r.Pack.ID || r.Review.PackRevision != r.Pack.Revision || r.Review.PackDigest != r.Pack.Digest || r.Review.RuntimeDigest != r.Digest || r.Review.ConfigurationDigest != r.Config.Digest || r.Review.Model != r.Config.Model || r.Review.SystemInstruction != r.Config.SystemInstruction || r.Review.MaxAttemptCostUSD != r.Config.AttemptCostUSD || r.Review.Decision != r.State || !identifier(r.Review.Reviewer) || r.Review.ReviewedAt.IsZero() || len(r.Review.Models) != len(r.Config.Models) {
		return false
	}
	for i := range r.Config.Models {
		if r.Config.Models[i] != r.Review.Models[i] {
			return false
		}
	}
	return true
}

// Validate checks the immutable runtime pack and its lifecycle binding.
func (r RuntimePackRecord) Validate() error {
	if !validRuntimePack(r) {
		return ErrInvalid
	}
	return nil
}

// CanonicalDigest binds a pack identity to its exact prompt/model/configuration revision.
func (p PackRevision) CanonicalDigest() string {
	d, _ := digest(struct {
		ID                  string      `json:"id"`
		Revision            int64       `json:"revision"`
		Model               string      `json:"model"`
		Models              []PackModel `json:"models,omitempty"`
		ConfigurationDigest string      `json:"configuration_digest"`
	}{p.ID, p.Revision, p.Model, p.Models, p.ConfigurationDigest})
	return d
}

// Provenance pins governed suite inputs and environment.
type Provenance struct {
	Implementation      string            `json:"implementation"`
	EnvironmentDigest   string            `json:"environment_digest"`
	ConfigurationDigest string            `json:"configuration_digest"`
	PromptPack          string            `json:"prompt_pack,omitempty"`
	Model               string            `json:"model,omitempty"`
	SemanticVersion     string            `json:"semantic_version"`
	RuleVersion         string            `json:"rule_version"`
	TemplateVersion     string            `json:"template_version,omitempty"`
	SourceSnapshot      string            `json:"source_snapshot"`
	DialectMatrix       []DialectEvidence `json:"dialect_matrix"`
}

// DialectEvidence records independent engine evidence status.
type DialectEvidence struct {
	Engine         string `json:"engine"`
	Dialect        string `json:"dialect"`
	Mode           Mode   `json:"mode"`
	EvidenceDigest string `json:"evidence_digest"`
	Status         string `json:"status"` // measured, unsupported, unknown
}

// ProtectedRef identifies content held by a protected store. It is not content.
type ProtectedRef struct {
	Digest    string `json:"digest"`
	Retention string `json:"retention"`
}

// Expected is one documented semantically equivalent result.
type Expected struct {
	Decision       string `json:"decision"`
	SemanticDigest string `json:"semantic_digest"`
	ErrorClass     string `json:"error_class,omitempty"`
}

// Case describes one protected evaluation input.
type Case struct {
	ID            string       `json:"id"`
	Stage         Stage        `json:"stage"`
	Category      string       `json:"category,omitempty"`
	Locale        string       `json:"locale"`
	Critical      bool         `json:"critical"`
	HeldOut       bool         `json:"held_out"`
	Input         ProtectedRef `json:"input"`
	BindingDigest string       `json:"binding_digest,omitempty"`
	Expected      []Expected   `json:"expected"`
	Fixture       *Observation `json:"fixture,omitempty"`
}

// Suite is an immutable versioned evaluation manifest.
type Suite struct {
	SchemaVersion        int            `json:"schema_version"`
	ID                   string         `json:"id"`
	Revision             int64          `json:"revision"`
	Mode                 Mode           `json:"mode"`
	Seed                 int64          `json:"seed"`
	Calibration          string         `json:"calibration"` // reviewed or unknown
	Threshold            Threshold      `json:"threshold"`
	Limits               Limits         `json:"limits"`
	Provenance           Provenance     `json:"provenance"`
	Packs                []PackRevision `json:"packs"`
	Frontiers            []string       `json:"frontiers"`
	HeldoutLineageDigest string         `json:"heldout_lineage_digest,omitempty"`
	Cases                []Case         `json:"cases"`
}

// Usage separates service, source, and model observations.
type Usage struct {
	ServiceMS int64    `json:"service_ms"`
	SourceMS  *int64   `json:"source_ms,omitempty"`
	ModelMS   *int64   `json:"model_ms,omitempty"`
	Calls     int      `json:"calls"`
	Tokens    *int     `json:"tokens,omitempty"`
	CostUSD   *float64 `json:"cost_usd,omitempty"`
	Retries   int      `json:"retries"`
}

// Observation is a content-free runner result.
type Observation struct {
	Decision       string `json:"decision"`
	SemanticDigest string `json:"semantic_digest"`
	ErrorClass     string `json:"error_class,omitempty"`
	Blocked        bool   `json:"blocked"`
	Usage          Usage  `json:"usage"`
}

// CaseResult records one deterministic comparison.
type CaseResult struct {
	ID          string      `json:"id"`
	Stage       Stage       `json:"stage"`
	Category    string      `json:"category,omitempty"`
	Locale      string      `json:"locale"`
	Critical    bool        `json:"critical"`
	HeldOut     bool        `json:"held_out"`
	Passed      bool        `json:"passed"`
	Reason      string      `json:"reason"`
	Observation Observation `json:"observation"`
}

// Report is reproducible content-free evaluation evidence.
type Report struct {
	SchemaVersion    int          `json:"schema_version"`
	RunID            string       `json:"run_id"`
	SuiteID          string       `json:"suite_id"`
	SuiteRevision    int64        `json:"suite_revision"`
	Mode             Mode         `json:"mode"`
	Seed             int64        `json:"seed"`
	SuiteDigest      string       `json:"suite_digest"`
	Pack             PackRevision `json:"pack"`
	EvidenceHash     string       `json:"evidence_hash"`
	Status           string       `json:"status"`
	FailureClass     string       `json:"failure_class,omitempty"`
	StartedAt        time.Time    `json:"started_at"`
	CompletedAt      time.Time    `json:"completed_at"`
	Cases            []CaseResult `json:"cases"`
	QualityPassed    int          `json:"quality_passed"`
	QualityTotal     int          `json:"quality_total"`
	SecurityFailures int          `json:"security_failures"`
	GatePassed       bool         `json:"gate_passed"`
	Usage            Usage        `json:"usage"`
}

func digest(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", ErrInvalid
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func validDigest(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func identifier(s string) bool {
	if len(s) == 0 || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.' || r == ':' {
			continue
		}
		return false
	}
	return true
}

// Validate checks manifest bounds, provenance, and mode rules.
func (s Suite) Validate() error {
	if s.SchemaVersion != SchemaVersion || !identifier(s.ID) || s.Revision < 1 || (s.Mode != Fixture && s.Mode != Live) || s.Seed == 0 || (s.Calibration != "reviewed" && s.Calibration != "unknown") {
		return ErrInvalid
	}
	if s.HeldoutLineageDigest != "" && !validDigest(s.HeldoutLineageDigest) {
		return ErrInvalid
	}
	if s.Limits.Cases < 1 || s.Limits.Cases > 10000 || s.Limits.Calls < 0 || s.Limits.Calls > 100000 || s.Limits.Tokens < 0 || s.Limits.Tokens > 1<<30 || s.Limits.Retries < 0 || s.Limits.Retries > 8 || s.Limits.CostUSD != nil && (*s.Limits.CostUSD < 0 || *s.Limits.CostUSD > 1000000) || s.Limits.DurationMS < 1 || s.Limits.DurationMS > int64((24*time.Hour)/time.Millisecond) || len(s.Cases) == 0 || len(s.Cases) > s.Limits.Cases {
		return ErrInvalid
	}
	if s.Threshold.SecurityFailures != 0 {
		return ErrInvalid
	}
	if s.Threshold.QualityMin != nil && (*s.Threshold.QualityMin <= 0 || *s.Threshold.QualityMin > 1) {
		return ErrInvalid
	}
	if s.Calibration == "unknown" && s.Threshold.QualityMin != nil {
		return ErrInvalid
	}
	p := s.Provenance
	if !validDigest(p.EnvironmentDigest) || !validDigest(p.ConfigurationDigest) || !validDigest(p.SourceSnapshot) || !identifier(p.Implementation) || !identifier(p.SemanticVersion) || !identifier(p.RuleVersion) || p.PromptPack != "" && !identifier(p.PromptPack) || p.Model != "" && !identifier(p.Model) || p.TemplateVersion != "" && !identifier(p.TemplateVersion) {
		return ErrInvalid
	}
	if len(p.DialectMatrix) == 0 || len(p.DialectMatrix) > 32 {
		return ErrInvalid
	}
	if len(s.Packs) == 0 || len(s.Packs) > 32 {
		return ErrInvalid
	}
	packs := map[string]bool{}
	for _, pack := range s.Packs {
		if !identifier(pack.ID) || pack.Revision < 1 || !validDigest(pack.Digest) || !identifier(pack.Model) || !validDigest(pack.ConfigurationDigest) || packs[pack.Digest] {
			return ErrInvalid
		}
		packs[pack.Digest] = true
	}
	dialects := map[string]bool{}
	for _, d := range p.DialectMatrix {
		key := d.Engine + ":" + d.Dialect
		if dialects[key] || !identifier(d.Engine) || !identifier(d.Dialect) || d.Mode != s.Mode || !validDigest(d.EvidenceDigest) || (d.Status != "measured" && d.Status != "unsupported" && d.Status != "unknown") {
			return ErrInvalid
		}
		dialects[key] = true
	}
	frontiers := map[string]bool{}
	for _, f := range s.Frontiers {
		if frontiers[f] || !knownFrontier(f) {
			return ErrInvalid
		}
		frontiers[f] = true
	}
	seen := map[string]bool{}
	quality := 0
	for _, c := range s.Cases {
		if !identifier(c.ID) || seen[c.ID] || !validStage(c.Stage) || (c.Category != "" && !identifier(c.Category)) || (c.Locale != "en" && c.Locale != "es") || !validDigest(c.Input.Digest) || !identifier(c.Input.Retention) || c.BindingDigest != "" && !validDigest(c.BindingDigest) || len(c.Expected) == 0 || len(c.Expected) > 16 {
			return ErrInvalid
		}
		seen[c.ID] = true
		if !c.Critical {
			quality++
		}
		if c.Critical && !knownAdversarial(c.Category) {
			return ErrInvalid
		}
		for _, e := range c.Expected {
			if !identifier(e.Decision) || !validDigest(e.SemanticDigest) || e.ErrorClass != "" && !identifier(e.ErrorClass) {
				return ErrInvalid
			}
		}
		if s.Mode == Fixture && c.Fixture == nil || s.Mode == Live && c.Fixture != nil {
			return ErrInvalid
		}
		if c.Fixture != nil && validateObservation(*c.Fixture) != nil {
			return ErrInvalid
		}
	}
	if s.Threshold.QualityMin != nil && quality == 0 {
		return ErrInvalid
	}
	return nil
}

func validateObservation(o Observation) error {
	if o.ErrorClass != "" && !identifier(o.ErrorClass) || o.Decision != "" && !identifier(o.Decision) || (!validDigest(o.SemanticDigest) && o.ErrorClass == "") {
		return ErrInvalid
	}
	if !validUsage(o.Usage) {
		return ErrInvalid
	}
	return nil
}

func validUsage(u Usage) bool {
	return u.ServiceMS >= 0 && (u.SourceMS == nil || *u.SourceMS >= 0) && (u.ModelMS == nil || *u.ModelMS >= 0) && u.Calls >= 0 && (u.Tokens == nil || *u.Tokens >= 0) && (u.CostUSD == nil || *u.CostUSD >= 0) && u.Retries >= 0
}

// Digest returns the canonical manifest digest after validation.
func (s Suite) Digest() (string, error) {
	if err := s.Validate(); err != nil {
		return "", err
	}
	return digest(s)
}

// Validate checks a completed report and its reproducible evidence hash.
func (r Report) Validate() error {
	if r.SchemaVersion != SchemaVersion || !identifier(r.RunID) || !identifier(r.SuiteID) || r.SuiteRevision < 1 || (r.Mode != Fixture && r.Mode != Live) || r.Seed == 0 || !validDigest(r.SuiteDigest) || !validDigest(r.EvidenceHash) || !validPack(r.Pack) || r.StartedAt.IsZero() || r.CompletedAt.Before(r.StartedAt) || !validReportStatus(r.Status) {
		return ErrInvalid
	}
	qualityPassed, qualityTotal, securityFailures := 0, 0, 0
	var aggregate Usage
	for i, c := range r.Cases {
		if !identifier(c.ID) || !validStage(c.Stage) || (c.Locale != "en" && c.Locale != "es") || (!validDigest(c.Observation.SemanticDigest) && c.Observation.ErrorClass == "") {
			return ErrInvalid
		}
		if c.Critical {
			if !c.Passed {
				securityFailures++
			}
		} else {
			qualityTotal++
			if c.Passed {
				qualityPassed++
			}
		}
		if validateObservation(c.Observation) != nil {
			return ErrInvalid
		}
		if i == 0 {
			aggregate = c.Observation.Usage
		} else {
			aggregate = addUsage(aggregate, c.Observation.Usage)
		}
	}
	if qualityPassed != r.QualityPassed || qualityTotal != r.QualityTotal || securityFailures != r.SecurityFailures || r.GatePassed && securityFailures != 0 {
		return ErrInvalid
	}
	gotUsage, _ := digest(r.Usage)
	wantUsage, _ := digest(aggregate)
	if gotUsage != wantUsage {
		return ErrInvalid
	}
	evidence := struct {
		Suite   string
		Pack    PackRevision
		Seed    int64
		Cases   []CaseResult
		Mode    Mode
		Status  string
		Failure string
	}{r.SuiteDigest, r.Pack, r.Seed, r.Cases, r.Mode, r.Status, r.FailureClass}
	want, _ := digest(evidence)
	if want != r.EvidenceHash {
		return ErrInvalid
	}
	return nil
}

func validPack(p PackRevision) bool {
	if !identifier(p.ID) || p.Revision < 1 || !validDigest(p.Digest) || !identifier(p.Model) || !validDigest(p.ConfigurationDigest) || len(p.Models) > 16 {
		return false
	}
	seen := map[string]bool{}
	for _, binding := range p.Models {
		if !identifier(binding.Role) || !identifier(binding.Model) || seen[binding.Role] {
			return false
		}
		seen[binding.Role] = true
	}
	return p.Digest == p.CanonicalDigest()
}

func (s Suite) pack(digest string) (PackRevision, bool) {
	for _, p := range s.Packs {
		if p.Digest == digest {
			return p, true
		}
	}
	return PackRevision{}, false
}

// PackByDigest returns an exact pack revision from this immutable suite.
func (s Suite) PackByDigest(digest string) (PackRevision, bool) { return s.pack(digest) }

func validReportStatus(s string) bool {
	switch s {
	case "passed", "failed", "cancelled", "timed_out", "budget_exhausted", "dependency_failed":
		return true
	}
	return false
}

func knownFrontier(s string) bool {
	switch s {
	case "EVAL-01", "EXP-01", "EXP-03", "EXP-05", "EXP-09", "EXP-10", "EXP-11":
		return true
	}
	return false
}

func knownAdversarial(s string) bool {
	switch s {
	case "identity_scope", "injection", "dialect_escape", "resource_exhaustion", "byo", "frozen_report":
		return true
	}
	return false
}

func validStage(s Stage) bool {
	switch s {
	case StageRouting, StageContext, StageSQL, StageValidation, StageChart, StageReport, StageReplay, StageShadow, StageAdversarial, StageConsumer:
		return true
	}
	return false
}

func stableCases(in []Case) []Case {
	out := append([]Case(nil), in...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
