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
	Key                string     `json:"key"`
	Reference          Reference  `json:"reference"`
	Arguments          []Argument `json:"arguments"`
	Resolution         Resolution `json:"resolution"`
	Outputs            []string   `json:"outputs"`
	Policy             string     `json:"policy" jsonschema:"enum=published,enum=certified_only,enum=explicit_stale,enum=private_preview"`
	Locale             string     `json:"locale"`
	Narrative          bool       `json:"narrative"`
	PartialPolicy      string     `json:"partial_policy" jsonschema:"enum=fail,enum=allow_partial"`
	ReuseMaxAgeSeconds int        `json:"reuse_max_age_seconds"`
}

// RunManifest is private persistence input, never an ordinary API response. The
// exact approved SQL, binds, source partition and output definitions are sealed
// once; a serialized manifest is neither authority nor a validator-issued plan.
type RunManifest struct {
	Version       string                    `json:"version"`
	ID            string                    `json:"id"`
	Tenant        string                    `json:"tenant"`
	Actor         string                    `json:"actor"`
	Session       string                    `json:"session"`
	Block         string                    `json:"block"`
	RequestHash   string                    `json:"request_hash"`
	TaskHash      string                    `json:"task_hash"`
	Revision      Revision                  `json:"revision"`
	Outputs       []Output                  `json:"outputs"`
	Resolved      Resolved                  `json:"resolved"`
	Binding       exec.Binding              `json:"binding"`
	Definitions   []topics.Definition       `json:"definitions"`
	Dependencies  []Dependency              `json:"dependencies"`
	References    []ResourceReference        `json:"references"`
	Trust         Trust                     `json:"trust"`
	Private       bool                      `json:"private"`
	Policy        string                    `json:"policy"`
	PartialPolicy string                    `json:"partial_policy"`
	Locale        string                    `json:"locale"`
	Created       time.Time                 `json:"created_at"`
	Expires       time.Time                 `json:"expires_at"`
	Limits        config.ReportingExecution `json:"limits"`
	ReuseKey      string                    `json:"reuse_key"`
	ReuseMaxAge   int                       `json:"reuse_max_age_seconds"`
	Model         string                    `json:"model"`
}

// Digest binds the admitted execution separately from revision/rendition hashes.
func (m RunManifest) Digest() string { return digest(m) }

// Reach contains only the authority projection needed by a retained-value read.
func (m RunManifest) Reach() access.Artifact {
	privacy, actor := "public", ""
	if m.Private {
		privacy, actor = "private_preview", m.Actor
	}
	return access.Artifact{Tenant: m.Tenant, ID: m.ID, TargetKind: "block", Target: m.Block, Context: m.Binding.Context, Privacy: privacy, PreviewActor: actor}
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
	if err := access.RequireExecution(e, "reporting.execute", "block", m.Block, m.Binding.Context, m.Private); err != nil {
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
	ID             string           `json:"id"`
	Kind           string           `json:"kind"`
	State          string           `json:"state"`
	Code           string           `json:"code,omitempty"`
	Digest         string           `json:"digest"`
	Chart          *charts.Output   `json:"chart,omitempty"`
	Narrative      *NarrativeResult  `json:"narrative,omitempty"`
	ReservedCalls  int              `json:"reserved_calls"`
	ReservedTokens int              `json:"reserved_tokens"`
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
	ID               string          `json:"id"`
	Block            string          `json:"block"`
	Revision         int64           `json:"revision"`
	RevisionDigest   string          `json:"revision_digest"`
	ManifestDigest   string          `json:"manifest_digest"`
	State            string          `json:"state"`
	Code             string          `json:"code,omitempty"`
	Private          bool            `json:"private"`
	Source           string          `json:"source"`
	Context          string          `json:"context"`
	PartitionDigest  string          `json:"partition_digest"`
	Locale           string          `json:"locale"`
	Timezone         string          `json:"timezone"`
	Created          time.Time       `json:"created_at"`
	Expires          time.Time       `json:"expires_at"`
	Observed         *time.Time      `json:"observed_at,omitempty"`
	Finished         *time.Time      `json:"finished_at,omitempty"`
	Attempts         int             `json:"attempts"`
	QueryAttempts    []exec.Attempt  `json:"query_attempts"`
	Parameters       []BoundValue    `json:"parameters"`
	Outputs          []OutputSummary `json:"outputs"`
	Trust            Trust           `json:"trust"`
	ReusedFrom       string          `json:"reused_from,omitempty"`
	RetainedBytes    int64           `json:"retained_bytes"`
	ReservedCalls    int             `json:"reserved_calls"`
	ReservedTokens   int             `json:"reserved_tokens"`
	FrozenVersion    string          `json:"frozen_version"`
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
	Offset    int                `json:"offset"`
	Next      *int               `json:"next,omitempty"`
	TotalRows int                `json:"total_rows"`
	Truncated bool               `json:"truncated"`
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
	ReuseFrozenRun(context.Context, jobs.Invocation, string) (RunRecord, bool, error)
	ListFrozenArtifacts(context.Context, identity.Envelope, string, int) (ArtifactList, error)
	CancelFrozenRun(context.Context, identity.Envelope, string) (RunView, error)
	ExpireFrozenArtifacts(context.Context, identity.Envelope, int) (int64, error)
}
