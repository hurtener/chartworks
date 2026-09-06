package engineering

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/store"
)

// ProfileSpec identifies an immutable evidence version, not approved semantics.
// Previous is the expected active version; empty means no prior active profile.
type ProfileSpec struct {
	ID         string   `json:"id"`
	Source     string   `json:"source"`
	Context    string   `json:"context"`
	Dataset    string   `json:"dataset"`
	Columns    []string `json:"columns"`
	Policy     string   `json:"policy"`
	TimeColumn string   `json:"time_column"`
	SkipLLM    bool     `json:"skip_llm"`
	Previous   string   `json:"previous"`
}

func (s ProfileSpec) Valid() bool {
	if !identity.Identifier(s.ID) || !identity.Identifier(s.Source) || !identity.Identifier(s.Context) || !identity.Identifier(s.Dataset) || (s.Policy != "" && !identity.Identifier(s.Policy)) || (s.Previous != "" && !identity.Identifier(s.Previous)) || len(s.Columns) > 256 || s.TimeColumn != "" && !readexec.SQLIdentifier(s.TimeColumn) {
		return false
	}
	seen := map[string]bool{}
	for _, c := range s.Columns {
		if !readexec.SQLIdentifier(c) || seen[c] {
			return false
		}
		seen[c] = true
	}
	return true
}
func (s ProfileSpec) Require(e identity.Envelope, write bool) error {
	action, permission := "engineering.read", "read"
	if write {
		action, permission = "engineering.profile", "write"
	}
	return access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: s.Source}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: s.Dataset}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: s.Context})
}

// ProfileRecord is a private, resumable version. A checkpoint is not an active
// semantic artifact; only the atomically published pointer is consumable.
type ProfileRecord struct {
	Tenant, Actor, Session string
	Spec                   ProfileSpec
	SpecHash               string
	Binding                readexec.Binding
	Policy                 config.ProfilePolicy
	Settings               config.Profiling
	State                  string
	Operation              string
	Created                time.Time
	Result                 *Profile
	SummaryStarted         bool
}

func (r ProfileRecord) Digest() string {
	return readexec.Hash([]any{"profile-v1", r.Tenant, r.Actor, r.Session, r.Spec, r.Binding, r.Policy, r.Settings})
}
func (r ProfileRecord) Valid() bool {
	return identity.Identifier(r.Tenant) && identity.Identifier(r.Actor) && identity.Identifier(r.Session) && r.Spec.Valid() && r.Binding.Valid() && r.Binding.Tenant == r.Tenant && r.Binding.Source == r.Spec.Source && r.Binding.Context == r.Spec.Context && config.ValidateProfiling(r.Settings) == nil && r.SpecHash == r.Digest() && !r.Created.IsZero()
}
func (r ProfileRecord) Require(e identity.Envelope, write bool) error {
	if !r.Valid() || r.Tenant != e.Tenant() || r.Actor != e.User() || r.Session != e.Session() {
		return store.ErrNotFound
	}
	return r.Spec.Require(e, write)
}

// ColumnProfile contains bounded aggregate evidence. Distinct is exact only
// when DistinctExact is true, and always applies to the observed sample.
type ColumnProfile struct {
	Name          string         `json:"name"`
	NativeType    string         `json:"native_type"`
	Category      string         `json:"category"`
	Nullable      bool           `json:"nullable"`
	Observed      int            `json:"observed"`
	Nulls         int            `json:"nulls"`
	Distinct      int            `json:"distinct"`
	DistinctExact bool           `json:"distinct_exact"`
	Families      map[string]int `json:"families"`
	Minimum       *string        `json:"minimum"`
	Maximum       *string        `json:"maximum"`
	RangeStatus   string         `json:"range_status"`
}

// QualityFinding is a fixed-rule observation, never an instruction to repair SQL.
type QualityFinding struct {
	Code   string `json:"code"`
	Column string `json:"column"`
	Count  int    `json:"count"`
	Basis  string `json:"basis"`
}

// Freshness distinguishes observation time from a dataset event-time assertion.
type Freshness struct {
	State       string     `json:"state"`
	Reason      string     `json:"reason"`
	Basis       string     `json:"basis"`
	SampleState string     `json:"sample_state"`
	Latest      *time.Time `json:"latest_permitted_value"`
}

// Sampling states both returned-data bounds and the absence of a physical scan cap.
type Sampling struct {
	Strategy       string  `json:"strategy"`
	Rows           int     `json:"rows"`
	Bytes          int     `json:"bytes"`
	Complete       bool    `json:"complete"`
	Truncation     string  `json:"truncation"`
	ScanBounded    bool    `json:"scan_bounded"`
	RowCeiling     int     `json:"row_ceiling"`
	ByteCeiling    int     `json:"byte_ceiling"`
	PlannerCeiling float64 `json:"planner_ceiling"`
	TimeoutNS      int64   `json:"timeout_ns"`
}

// ProfileSummary is optional model commentary and never changes deterministic data.
type ProfileSummary struct {
	Status  string          `json:"status"`
	Text    string          `json:"text"`
	Receipt gateway.Receipt `json:"receipt"`
}

// Profile is immutable evidence with explicit type, sampling, privacy and cost basis.
type Profile struct {
	Version        string            `json:"version"`
	Source         string            `json:"source"`
	Context        string            `json:"context"`
	Dataset        string            `json:"dataset"`
	SourceRevision int64             `json:"source_revision"`
	ObservedAt     time.Time         `json:"observed_at"`
	Schema         []readexec.Column `json:"schema"`
	Columns        []ColumnProfile   `json:"columns"`
	Findings       []QualityFinding  `json:"findings"`
	Sampling       Sampling          `json:"sampling"`
	Freshness      Freshness         `json:"freshness"`
	Cost           readexec.Cost     `json:"cost"`
	ExecutionNS    int64             `json:"execution_ns"`
	ReadOperation  string            `json:"read_operation"`
	ReadAttempt    string            `json:"read_attempt"`
	PolicyHash     string            `json:"policy_hash"`
	Summary        ProfileSummary    `json:"summary"`
}

func (p Profile) DeterministicHash() string { p.Summary = ProfileSummary{}; return readexec.Hash(p) }
func (p Profile) Valid(r ProfileRecord) bool {
	if p.Version != r.Spec.ID || p.Source != r.Spec.Source || p.Context != r.Spec.Context || p.Dataset != r.Spec.Dataset || p.SourceRevision != r.Binding.Revision || p.ObservedAt.IsZero() || p.ExecutionNS < 0 || p.ReadAttempt == "" || p.ReadOperation == "" || len(p.Columns) < 1 || len(p.Columns) > 256 || len(p.Schema) < 1 || len(p.Schema) > 256 || len(p.Findings) > 1024 || len(p.Summary.Text) > 2048 || p.PolicyHash != readexec.Hash(r.Policy) || p.Sampling.Rows < 0 || p.Sampling.Rows > r.Settings.SampleRows || p.Sampling.Bytes < 0 || p.Sampling.Bytes > r.Settings.SampleBytes || p.Sampling.ScanBounded || p.Cost.ScannedBytes != nil {
		return false
	}
	for _, c := range p.Columns {
		if c.Observed != p.Sampling.Rows || c.Nulls < 0 || c.Nulls > c.Observed || c.Distinct < 0 || c.Distinct > 1025 || len(c.Families) > 16 || c.Minimum != nil && len(*c.Minimum) > 128 || c.Maximum != nil && len(*c.Maximum) > 128 {
			return false
		}
	}
	data, err := json.Marshal(p)
	return err == nil && len(data) <= 256<<10
}

// ProfileStatus exposes checkpoint progress without exposing unpublished findings.
type ProfileStatus struct {
	Version        string    `json:"version"`
	Source         string    `json:"source"`
	Context        string    `json:"context"`
	Dataset        string    `json:"dataset"`
	State          string    `json:"state"`
	Operation      string    `json:"operation"`
	SummaryStarted bool      `json:"summary_started"`
	Created        time.Time `json:"created_at"`
	Profile        *Profile  `json:"profile"`
}

func (r ProfileRecord) Public() ProfileStatus {
	p := ProfileStatus{Version: r.Spec.ID, Source: r.Spec.Source, Context: r.Spec.Context, Dataset: r.Spec.Dataset, State: r.State, Operation: r.Operation, SummaryStarted: r.SummaryStarted, Created: r.Created}
	if r.State == "complete" {
		p.Profile = r.Result
	}
	return p
}

// ProfileRun describes the actual request attempt, including retry/cancellation.
type ProfileRun struct {
	Profile   ProfileStatus    `json:"profile"`
	Operation jobs.RequestTask `json:"operation"`
	Code      string           `json:"code"`
}

// SchemaChange records structural drift only, never a proposed SQL edit.
type SchemaChange struct {
	Column string `json:"column"`
	Kind   string `json:"kind"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func SchemaDiff(before, after []readexec.Column) []SchemaChange {
	old, next := map[string]readexec.Column{}, map[string]readexec.Column{}
	for _, c := range before {
		old[c.Name] = c
	}
	for _, c := range after {
		next[c.Name] = c
	}
	out := []SchemaChange{}
	for name, c := range old {
		n, ok := next[name]
		if !ok {
			out = append(out, SchemaChange{Column: name, Kind: "removed", Before: c.NativeType})
			continue
		}
		if c.NativeType != n.NativeType {
			out = append(out, SchemaChange{Column: name, Kind: "type_changed", Before: c.NativeType, After: n.NativeType})
		}
		if c.Nullable != n.Nullable {
			out = append(out, SchemaChange{Column: name, Kind: "nullability_changed"})
		}
		if c.Safe != n.Safe {
			out = append(out, SchemaChange{Column: name, Kind: "safety_changed"})
		}
	}
	for name, c := range next {
		if _, ok := old[name]; !ok {
			out = append(out, SchemaChange{Column: name, Kind: "added", After: c.NativeType})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Column == out[j].Column {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Column < out[j].Column
	})
	return out
}

// Dependency is a signed, versioned reference to a consumer definition. It stores
// no SQL and cannot rewrite that definition when a profile changes.
type Dependency struct {
	Kind    string   `json:"kind"`
	ID      string   `json:"id"`
	Version string   `json:"definition_version"`
	Source  string   `json:"source"`
	Context string   `json:"context"`
	Dataset string   `json:"dataset"`
	Columns []string `json:"columns"`
}

func (d Dependency) Valid() bool {
	if !identity.Identifier(d.ID) || !validHash(d.Version) || !identity.Identifier(d.Source) || !identity.Identifier(d.Context) || !identity.Identifier(d.Dataset) || len(d.Columns) < 1 || len(d.Columns) > 256 {
		return false
	}
	switch d.Kind {
	case "topic", "block", "report", "dashboard":
	default:
		return false
	}
	seen := map[string]bool{}
	for _, c := range d.Columns {
		if !readexec.SQLIdentifier(c) || seen[c] {
			return false
		}
		seen[c] = true
	}
	return true
}
func (d Dependency) Require(e identity.Envelope, write bool) error {
	permission, action := "read", "engineering.read"
	if write {
		permission, action = "write", "engineering.profile"
	}
	return access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: d.Kind, Permission: permission, ID: d.ID}, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: d.Source}, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: d.Dataset}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: d.Context})
}

// HealthEvent is idempotent per profile and immutable consumer version.
type HealthEvent struct {
	ID         string         `json:"id"`
	Profile    string         `json:"profile"`
	Dependency Dependency     `json:"dependency"`
	State      string         `json:"state"`
	Changes    []SchemaChange `json:"changes"`
	ObservedAt time.Time      `json:"observed_at"`
}

// ProfileEvidence is the real inspection/semantic-planning consumer. It is not
// a semantic publication or a promise that approved definitions remain healthy.
type ProfileEvidence struct {
	Profile             Profile        `json:"profile"`
	Active              bool           `json:"active"`
	AuthorityContext    string         `json:"authority_context"`
	SemanticPublication bool           `json:"semantic_publication"`
	Changes             []SchemaChange `json:"schema_changes"`
}
