package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// FrozenVersion independently versions execution, not authored block content.
const FrozenVersion = "frozen-block-run-v1"

var (
	// ErrExpired never authorizes reconstructing a missing retained result.
	ErrExpired = errors.New("reporting: retained values expired")
	// ErrIncomplete identifies a run without a complete eligible retained result.
	ErrIncomplete = errors.New("reporting: result is not complete")
	// ErrBudget identifies a pre-call or retained-byte budget refusal.
	ErrBudget = errors.New("reporting: execution or retention budget exhausted")
)

// RunRequest contains only caller-controlled intent. Replaying this exact
// request does not resolve a newer revision or replace its accepted logical time.
type RunRequest struct {
	Limits             *QueryLimits `json:"limits,omitempty"`
	Key                string       `json:"key"`
	Reference          Reference    `json:"reference"`
	Arguments          []Argument   `json:"arguments"`
	Resolution         Resolution   `json:"resolution"`
	Outputs            []string     `json:"outputs" wire:"optional"`
	Policy             string       `json:"policy,omitempty" jsonschema:"enum=published,enum=certified_only,enum=explicit_stale,enum=private_preview"`
	Locale             string       `json:"locale"`
	Narrative          bool         `json:"narrative"`
	PartialPolicy      string       `json:"partial_policy,omitempty" jsonschema:"enum=fail,enum=allow_partial"`
	ReuseMaxAgeSeconds int          `json:"reuse_max_age_seconds"`
}

// RunManifest is private persistence input, never an ordinary API response. The
// exact approved SQL, binds, source partition and output definitions are sealed
// once; a serialized manifest is neither authority nor a validator-issued plan.
type RunManifest struct {
	Selection                *OutputSelection          `json:"selection,omitempty"`
	QueryLimits              *QueryLimits              `json:"query_limits,omitempty"`
	ResultPolicy             []EffectiveFieldPolicy    `json:"result_policy,omitempty"`
	Version                  string                    `json:"version"`
	ID                       string                    `json:"id"`
	Tenant                   string                    `json:"tenant"`
	Actor                    string                    `json:"actor"`
	Session                  string                    `json:"session"`
	Block                    string                    `json:"block"`
	RequestHash              string                    `json:"request_hash"`
	TaskHash                 string                    `json:"task_hash"`
	Revision                 Revision                  `json:"revision"`
	Outputs                  []Output                  `json:"outputs"`
	Resolved                 Resolved                  `json:"resolved"`
	Binding                  exec.Binding              `json:"binding"`
	Definitions              []topics.Definition       `json:"definitions"`
	Rules                    []RulePin                 `json:"rules,omitempty"`
	Dependencies             []Dependency              `json:"dependencies"`
	References               []ResourceReference       `json:"references"`
	Trust                    Trust                     `json:"trust"`
	Private                  bool                      `json:"private"`
	Policy                   string                    `json:"policy"`
	PartialPolicy            string                    `json:"partial_policy"`
	Locale                   string                    `json:"locale"`
	Created                  time.Time                 `json:"created_at"`
	Expires                  time.Time                 `json:"expires_at"`
	Limits                   config.ReportingExecution `json:"limits"`
	ReuseKey                 string                    `json:"reuse_key"`
	ReuseMaxAge              int                       `json:"reuse_max_age_seconds"`
	Model                    string                    `json:"model"`
	NarrativePack            *NarrativePackPin         `json:"narrative_pack,omitempty"`
	NarrativePackUnavailable bool                      `json:"narrative_pack_unavailable,omitempty"`
}

// NarrativePackPin is selected from an accepted server-owned runtime pack.
// It is execution provenance, never a caller supplied run parameter.
type NarrativePackPin struct {
	PackDigest          string `json:"pack_digest"`
	RuntimeDigest       string `json:"runtime_digest"`
	ConfigurationDigest string `json:"configuration_digest"`
	Model               string `json:"model"`
}

func (p NarrativePackPin) Valid() bool {
	return hashValid(p.PackDigest) && hashValid(p.RuntimeDigest) && hashValid(p.ConfigurationDigest) &&
		p.Model != "" && len(p.Model) <= 256
}

// NarrativeRuntime is an exact reviewed selection plus its protected gateway
// configuration. Only the pin is persisted in the frozen manifest.
type NarrativeRuntime struct {
	Pin    NarrativePackPin
	Config gateway.RuntimeConfig
}

func (r NarrativeRuntime) Valid() bool {
	if !r.Pin.Valid() || r.Config.Digest != r.Pin.ConfigurationDigest ||
		gateway.ConfigurationDigest(r.Config) != r.Config.Digest {
		return false
	}
	for _, binding := range r.Config.Models {
		if binding.Role == "narrative" {
			return binding.Model == r.Pin.Model
		}
	}
	return false
}

// Digest binds the admitted execution separately from revision/rendition hashes.
func (m RunManifest) Digest() string { return digest(m) }

// ReuseIdentity binds the complete resolved inputs of a frozen run, excluding
// only per-operation identity, timestamps and retention. Resolved values carry
// any time window that can affect the query; the operation's creation time does
// not prevent reuse when the bound values are otherwise identical. It is recomputed at
// the persistence boundary; a stored reuse_key is never itself proof that two
// manifests describe the same result or the same required resource reach.
func ReuseIdentity(m RunManifest) string {
	privacyActor := ""
	if m.Private {
		privacyActor = m.Actor
	}
	parts := []any{"frozen-result-reuse-v2", FrozenVersion, charts.BuildVersion,
		m.Tenant, m.Block, m.Revision.Digest, m.Definitions, m.Rules,
		m.Dependencies, m.References, m.Outputs, m.Resolved.Values, m.Resolved.Parameters,
		m.Resolved.Timezone, m.Locale, exec.Hash(m.Binding), m.Private,
		privacyActor, m.Policy, m.PartialPolicy, m.Trust, m.Model,
		"reporting-output-policy-v2", m.Selection, m.QueryLimits,
		m.ResultPolicy, m.Limits}
	if m.NarrativePack != nil {
		parts = append(parts, "reviewed-narrative-pack-v1", m.NarrativePack)
	} else if m.NarrativePackUnavailable {
		parts = append(parts, "reviewed-narrative-pack-unavailable-v1")
	}
	return digest(parts)
}

// Reach contains only the authority projection needed by a retained-value read.
func (m RunManifest) Reach() access.Artifact {
	return access.Artifact{Tenant: m.Tenant, RunID: m.ID, ParentKind: "block", ParentID: m.Block,
		Published: !m.Private, Private: m.Private,
		Contexts: []access.Resource{{Tenant: m.Tenant, Kind: "execution_context", Permission: "use", ID: m.Binding.Context}}}
}

// RequireRunManifest applies present authority to the original actor/session and
// exact execution reach. Artifact reads deliberately use RequireArtifact instead.
func RequireRunManifest(e identity.Envelope, m RunManifest) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if m.Version != FrozenVersion || m.Tenant != e.Tenant() || m.Actor != e.User() || m.Session != e.Session() {
		return access.ErrNotFound
	}
	dependencies := []access.Resource{{Tenant: m.Tenant, Kind: "source", Permission: "query", ID: m.Binding.Source}}
	for _, ref := range m.References {
		if ref.Kind == "dataset" && ref.Permission == "query" {
			dependencies = append(dependencies, access.Resource{Tenant: m.Tenant, Kind: ref.Kind, Permission: ref.Permission, ID: ref.ID})
		}
	}
	if m.Private {
		if err := Require(e, m.Block, Preview); err != nil {
			return err
		}
	}
	if err := access.RequireExecution(e, access.Execution{
		Target:       access.Resource{Tenant: m.Tenant, Kind: "block", Permission: "execute", ID: m.Block},
		Dependencies: dependencies,
		Contexts:     []access.Resource{{Tenant: m.Tenant, Kind: "execution_context", Permission: "use", ID: m.Binding.Context}},
	}); err != nil {
		return err
	}
	return RequireReferences(e, Execute, m.References)
}

// NarrativeClaim is a model-selected, mechanically checked statement. The model
// cannot supply arbitrary factual prose, SQL, tools or a caller-selected URL.
type NarrativeClaim struct {
	Kind     string   `json:"kind" jsonschema:"enum=value,enum=difference"`
	Evidence []string `json:"evidence"`
}

// NarrativeAnswer is the closed gateway output before exact-value rendering.
type NarrativeAnswer struct {
	Claims []NarrativeClaim `json:"claims"`
}

// NarrativeEvidence is the bounded, already-redacted evidence actually supplied.
type NarrativeEvidence struct {
	ID    string `json:"id"`
	Field string `json:"field"`
	Type  string `json:"type"`
	Value string `json:"value"`
	Row   int    `json:"row"`
}

// NarrativeResult retains exact text and actual provider/model usage. Rendering
// never invokes the model again or substitutes a newer prompt/model version.
type NarrativeResult struct {
	PolicyVersion string              `json:"policy_version,omitempty"`
	Text          string              `json:"text"`
	Claims        []NarrativeClaim    `json:"claims"`
	Evidence      []NarrativeEvidence `json:"evidence"`
	Caveats       []string            `json:"caveats"`
	EvidenceHash  string              `json:"evidence_hash"`
	OutputHash    string              `json:"output_hash"`
	PromptVersion string              `json:"prompt_version"`
	ModelVersion  string              `json:"model_version"`
	SchemaVersion string              `json:"schema_version"`
	Locale        string              `json:"locale"`
	Tone          string              `json:"tone"`
	Receipt       gateway.Receipt     `json:"receipt"`
}

// RetainedOutput is one independently checkpointed fan-out of the same logical
// normalized result. A failed narrative is explicit, not an empty successful one.
type RetainedOutput struct {
	ResultPolicy   []EffectiveFieldPolicy `json:"result_policy,omitempty"`
	Intent         *OutputIntent          `json:"intent,omitempty"`
	EvidencePolicy []EffectiveFieldPolicy `json:"evidence_policy,omitempty"`
	ID             string                 `json:"id"`
	Kind           string                 `json:"kind"`
	State          string                 `json:"state"`
	Code           string                 `json:"code,omitempty"`
	Digest         string                 `json:"digest"`
	Chart          *charts.Output         `json:"chart,omitempty"`
	Narrative      *NarrativeResult       `json:"narrative,omitempty"`
	ReservedCalls  int                    `json:"reserved_calls"`
	ReservedTokens int                    `json:"reserved_tokens"`
}

// OutputSummary excludes values and narrative text from list/summary responses.
type OutputSummary struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Code   string `json:"code,omitempty"`
	Digest string `json:"digest"`
}

// RunView has no approved SQL, raw binds, result rows or bearer credentials.
type RunView struct {
	Selection       *OutputSelection       `json:"selection,omitempty"`
	QueryLimits     *QueryLimits           `json:"query_limits,omitempty"`
	ResultPolicy    []EffectiveFieldPolicy `json:"result_policy,omitempty"`
	Policy          string                 `json:"policy,omitempty"`
	Scheduled       *ScheduledProvenance   `json:"scheduled,omitempty"`
	ID              string                 `json:"id"`
	Block           string                 `json:"block"`
	Revision        int64                  `json:"revision"`
	RevisionDigest  string                 `json:"revision_digest"`
	ManifestDigest  string                 `json:"manifest_digest"`
	State           string                 `json:"state"`
	Code            string                 `json:"code,omitempty"`
	Private         bool                   `json:"private"`
	Source          string                 `json:"source"`
	Context         string                 `json:"context"`
	PartitionDigest string                 `json:"partition_digest"`
	Locale          string                 `json:"locale"`
	Timezone        string                 `json:"timezone"`
	Created         time.Time              `json:"created_at"`
	Expires         time.Time              `json:"expires_at"`
	Observed        *time.Time             `json:"observed_at,omitempty"`
	Finished        *time.Time             `json:"finished_at,omitempty"`
	Attempts        int                    `json:"attempts"`
	QueryAttempts   []exec.Attempt         `json:"query_attempts"`
	Parameters      []BoundValue           `json:"parameters"`
	Outputs         []OutputSummary        `json:"outputs"`
	Trust           Trust                  `json:"trust"`
	ReusedFrom      string                 `json:"reused_from,omitempty"`
	RetainedBytes   int64                  `json:"retained_bytes"`
	ReservedCalls   int                    `json:"reserved_calls"`
	ReservedTokens  int                    `json:"reserved_tokens"`
	FrozenVersion   string                 `json:"frozen_version"`
}

// RunRecord is an internal repository result; its manifest/data are protected
// separately from the transport projection. Expired records contain no payload.
type RunRecord struct {
	View     RunView
	Reach    access.Artifact
	Manifest *RunManifest
	Result   *exec.Result
	Outputs  []RetainedOutput
}

// ResultPage keeps column order and exact JSON cells, never floating-point casts.
type ResultPage struct {
	ID        string              `json:"id"`
	Schema    []exec.Field        `json:"schema"`
	Rows      [][]json.RawMessage `json:"rows"`
	Offset    int                 `json:"offset"`
	Next      *int                `json:"next,omitempty"`
	TotalRows int                 `json:"total_rows"`
	Truncated bool                `json:"truncated"`
	Observed  *time.Time          `json:"observed_at,omitempty"`
}

// ArtifactList is a bounded permission-filtered result, not a tenant-wide cache.
type ArtifactList struct {
	Items []RunView `json:"items"`
	Next  string    `json:"next,omitempty"`
}

// RunRepository extends the common operation ledger, not another work queue.
// PostgreSQL combines checkpoint/publication with the live shared attempt fence.
type RunRepository interface {
	ReadFrozenRun(context.Context, identity.Envelope, string, bool) (RunRecord, error)
	SealFrozenRun(context.Context, identity.Envelope, jobs.RequestTask, PreparedRun) (RunRecord, error)
	CheckpointFrozenRun(context.Context, jobs.Invocation, PreparedRunWrite) (RunRecord, error)
	ReuseFrozenRun(context.Context, jobs.Invocation, string, config.ReportingExecution) (RunRecord, bool, error)
	ListFrozenArtifacts(context.Context, identity.Envelope, string, int) (ArtifactList, error)
	CancelFrozenRun(context.Context, identity.Envelope, string) (RunView, error)
	ExpireFrozenArtifacts(context.Context, identity.Envelope, int) (int64, error)
}
