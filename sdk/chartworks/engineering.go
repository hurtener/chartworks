package chartworks

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"
)

// UploadColumn declares a closed scalar target type; it is not a SQL expression.
type UploadColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Nullable bool   `json:"nullable"`
}

// UploadSpec reserves an immutable upload under a tenant-approved connection
// alias. No filename, path, database address or credential is accepted.
type UploadSpec struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Connection string         `json:"connection"`
	Format     string         `json:"format"`
	Sheet      string         `json:"sheet"`
	Columns    []UploadColumn `json:"columns"`
	Bytes      int64          `json:"bytes"`
	SHA256     string         `json:"sha256"`
}

// UploadStatus contains no storage path, backend identity or credential. Only
// state=active identifies a queryable governed source, not merely staged bytes.
type UploadStatus struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Format    string         `json:"format"`
	State     string         `json:"state"`
	Columns   []UploadColumn `json:"columns"`
	Rows      int            `json:"rows"`
	Created   time.Time      `json:"created_at"`
	Expires   time.Time      `json:"staging_expires_at"`
	Operation string         `json:"operation"`
	Source    *Source        `json:"source"`
}

// EngineeringInput is the accepted content-free operation manifest.
type EngineeringInput struct {
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Context   string `json:"context"`
	InputHash string `json:"input_hash"`
}

// EngineeringOperation is an actor/session-private shared-ledger receipt, not
// authority to execute. Resuming always uses the client's current token provider.
type EngineeringOperation struct {
	ID           string           `json:"id"`
	Tenant       string           `json:"tenant"`
	Actor        string           `json:"actor"`
	Session      string           `json:"session"`
	Input        EngineeringInput `json:"input"`
	State        string           `json:"state"`
	Code         string           `json:"code"`
	Attempts     int              `json:"attempts"`
	MaxAttempts  int              `json:"max_attempts"`
	Created      time.Time        `json:"created_at"`
	Expires      time.Time        `json:"expires_at"`
	ManifestHash string           `json:"manifest_hash"`
}

// UploadRun may describe an interrupted or failed accepted attempt. Check State
// and Code; HTTP success alone never asserts that activation or erasure succeeded.
type UploadRun struct {
	Upload    UploadStatus         `json:"upload"`
	Operation EngineeringOperation `json:"operation"`
	Code      string               `json:"code"`
}

// ReserveUpload binds the checksum, shape and exact byte length before transfer.
func (c *Client) ReserveUpload(ctx context.Context, spec UploadSpec) (out UploadStatus, err error) {
	if !wireID(spec.ID) || !wireID(spec.Connection) || spec.Bytes < 1 || spec.Bytes > 100<<20 || len(spec.Columns) < 1 {
		return out, errors.New("chartworks: invalid upload manifest")
	}
	err = c.call(ctx, "POST", "/v1/uploads", "", spec, &out)
	return out, err
}

// StageUpload streams at most the declared size plus one overflow byte. It does
// not buffer the file, encode it as JSON, reopen it, or retry a failed transfer.
// The caller retains ownership of the reader and any underlying file handle.
func (c *Client) StageUpload(ctx context.Context, id string, content io.Reader, size int64) (out UploadStatus, err error) {
	if !wireID(id) || content == nil || size < 1 || size > 100<<20 {
		return out, errors.New("chartworks: invalid upload stream")
	}
	err = c.callReader(ctx, "PUT", "/v1/uploads/"+id+"/content", "", "application/octet-stream", io.LimitReader(content, size+1), &out, 1<<20)
	return out, err
}

// Upload inspects private retained status without source or model work.
func (c *Client) Upload(ctx context.Context, id string) (out UploadStatus, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid upload identifier")
	}
	err = c.call(ctx, "GET", "/v1/uploads/"+id, "", nil, &out)
	return out, err
}

// LoadUpload explicitly starts or resumes the same immutable accepted load.
func (c *Client) LoadUpload(ctx context.Context, id, key string, resume bool) (out UploadRun, err error) {
	return c.uploadAction(ctx, id, key, resume, "load")
}

// EraseUpload fences new reads and erases only proven owned workspace objects.
func (c *Client) EraseUpload(ctx context.Context, id, key string, resume bool) (out UploadRun, err error) {
	return c.uploadAction(ctx, id, key, resume, "erase")
}
func (c *Client) uploadAction(ctx context.Context, id, key string, resume bool, action string) (out UploadRun, err error) {
	if !wireID(id) || !wireID(key) {
		return out, errors.New("chartworks: invalid upload operation")
	}
	input := struct {
		Key    string `json:"key"`
		Resume bool   `json:"resume"`
	}{key, resume}
	err = c.call(ctx, "POST", "/v1/uploads/"+id+"/"+action, "", input, &out)
	return out, err
}

// SweepUploads explicitly cleans a bounded set of expired private staging records.
// Active datasets are never selected by this retention operation.
func (c *Client) SweepUploads(ctx context.Context, limit int) (out []UploadRun, err error) {
	if limit < 1 || limit > 32 {
		return nil, errors.New("chartworks: invalid sweep limit")
	}
	err = c.call(ctx, "POST", "/v1/upload-sweeps", "", struct {
		Limit int `json:"limit"`
	}{limit}, &out)
	return out, err
}

// ProfileSpec creates a new evidence version, not approved business semantics.
// Previous is the expected active profile; SkipLLM guarantees no summary call.
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

// ColumnProfile reports observed-sample statistics, not extrapolated population totals.
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

// QualityFinding is a deterministic bounded observation with its evidence basis.
type QualityFinding struct {
	Code   string `json:"code"`
	Column string `json:"column"`
	Count  int    `json:"count"`
	Basis  string `json:"basis"`
}

// ProfileFreshness distinguishes observation time from permitted event-time evidence.
type ProfileFreshness struct {
	State       string     `json:"state"`
	Reason      string     `json:"reason"`
	Basis       string     `json:"basis"`
	SampleState string     `json:"sample_state"`
	Latest      *time.Time `json:"latest_permitted_value"`
}

// ProfileSampling records enforced returned-data bounds. ScanBounded=false means
// the adapter does not supply a physical scanned-byte guarantee.
type ProfileSampling struct {
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

// ProfileSummary keeps optional model commentary and its original usage receipt
// separate from deterministic statistics. Unknown usage is not fabricated zero.
type ProfileSummary struct {
	Status  string          `json:"status"`
	Text    string          `json:"text"`
	Receipt json.RawMessage `json:"receipt"`
}

// Profile is immutable versioned evidence, with exact range values as strings.
type Profile struct {
	Version        string           `json:"version"`
	Source         string           `json:"source"`
	Context        string           `json:"context"`
	Dataset        string           `json:"dataset"`
	SourceRevision int64            `json:"source_revision"`
	ObservedAt     time.Time        `json:"observed_at"`
	Schema         []SourceColumn   `json:"schema"`
	Columns        []ColumnProfile  `json:"columns"`
	Findings       []QualityFinding `json:"findings"`
	Sampling       ProfileSampling  `json:"sampling"`
	Freshness      ProfileFreshness `json:"freshness"`
	Cost           ReadCost         `json:"cost"`
	ExecutionNS    int64            `json:"execution_ns"`
	ReadOperation  string           `json:"read_operation"`
	ReadAttempt    string           `json:"read_attempt"`
	PolicyHash     string           `json:"policy_hash"`
	Summary        ProfileSummary   `json:"summary"`
}

// ProfileStatus reveals completed evidence only; checkpoints remain private work.
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

// ProfileRun distinguishes accepted work from complete profile publication.
type ProfileRun struct {
	Profile   ProfileStatus        `json:"profile"`
	Operation EngineeringOperation `json:"operation"`
	Code      string               `json:"code"`
}

// ProfileChange describes structural drift and contains no suggested SQL edit.
type ProfileChange struct {
	Column string `json:"column"`
	Kind   string `json:"kind"`
	Before string `json:"before"`
	After  string `json:"after"`
}

// ProfileDependency refers to one immutable consumer definition and its columns.
type ProfileDependency struct {
	Kind    string   `json:"kind"`
	ID      string   `json:"id"`
	Version string   `json:"definition_version"`
	Source  string   `json:"source"`
	Context string   `json:"context"`
	Dataset string   `json:"dataset"`
	Columns []string `json:"columns"`
}

// ProfileHealthEvent is idempotent for the profile and exact consumer version.
type ProfileHealthEvent struct {
	ID         string            `json:"id"`
	Profile    string            `json:"profile"`
	Dependency ProfileDependency `json:"dependency"`
	State      string            `json:"state"`
	Changes    []ProfileChange   `json:"changes"`
	ObservedAt time.Time         `json:"observed_at"`
}

// ProfileEvidence is the scoped retained input for inspection and semantic
// planning. Reading it never executes SQL or publishes business semantics.
type ProfileEvidence struct {
	Profile             Profile         `json:"profile"`
	Active              bool            `json:"active"`
	AuthorityContext    string          `json:"authority_context"`
	SemanticPublication bool            `json:"semantic_publication"`
	Changes             []ProfileChange `json:"schema_changes"`
}

// BuildProfile starts or explicitly resumes one version and preserves SkipLLM.
func (c *Client) BuildProfile(ctx context.Context, spec ProfileSpec, key string, resume bool) (out ProfileRun, err error) {
	if !wireID(spec.ID) || !wireID(spec.Source) || !wireID(spec.Context) || !wireID(spec.Dataset) || !wireID(key) {
		return out, errors.New("chartworks: invalid profile coordinates")
	}
	if spec.Columns == nil {
		spec.Columns = []string{}
	}
	input := struct {
		Spec   ProfileSpec `json:"spec"`
		Key    string      `json:"key"`
		Resume bool        `json:"resume"`
	}{spec, key, resume}
	err = c.call(ctx, "POST", "/v1/profiles", "", input, &out)
	return out, err
}

// Profile inspects retained version progress without warehouse or model calls.
func (c *Client) Profile(ctx context.Context, id string) (out ProfileStatus, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid profile identifier")
	}
	err = c.call(ctx, "GET", "/v1/profiles/"+id, "", nil, &out)
	return out, err
}

// ProfileEvidence reads scoped immutable evidence through the same service path.
func (c *Client) ProfileEvidence(ctx context.Context, id string) (out ProfileEvidence, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid profile identifier")
	}
	err = c.call(ctx, "GET", "/v1/profiles/"+id+"/evidence", "", nil, &out)
	return out, err
}

// ProfileHistory is bounded and filtered by signed source/context/dataset reach
// before pagination. It cannot use a current context to reveal another partition.
func (c *Client) ProfileHistory(ctx context.Context, source, partition, dataset string, limit int) (out []ProfileStatus, err error) {
	if !wireID(source) || !wireID(partition) || !wireID(dataset) || limit < 1 || limit > 32 {
		return nil, errors.New("chartworks: invalid profile history coordinates")
	}
	input := struct {
		Source  string `json:"source"`
		Context string `json:"context"`
		Dataset string `json:"dataset"`
		Limit   int    `json:"limit"`
	}{source, partition, dataset, limit}
	err = c.callLimit(ctx, "POST", "/v1/profile-history", "", input, &out, 9<<20)
	return out, err
}

// RegisterProfileDependency records a reference; it neither creates nor publishes
// a topic, block, report or dashboard definition.
func (c *Client) RegisterProfileDependency(ctx context.Context, id string, dependency ProfileDependency) error {
	if !wireID(id) {
		return errors.New("chartworks: invalid profile identifier")
	}
	var out struct {
		Registered bool `json:"registered"`
	}
	if err := c.call(ctx, "POST", "/v1/profiles/"+id+"/dependencies", "", dependency, &out); err != nil {
		return err
	}
	if !out.Registered {
		return errors.New("chartworks: invalid dependency receipt")
	}
	return nil
}

// ProfileDependencyHealth reads immutable drift events without refreshing data.
func (c *Client) ProfileDependencyHealth(ctx context.Context, dependency ProfileDependency) (out []ProfileHealthEvent, err error) {
	err = c.callLimit(ctx, "POST", "/v1/profile-dependency-health", "", dependency, &out, 9<<20)
	return out, err
}

// EngineeringOperation inspects the accepted task, requiring current task reach.
func (c *Client) EngineeringOperation(ctx context.Context, id string) (out EngineeringOperation, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid operation identifier")
	}
	err = c.call(ctx, "GET", "/v1/engineering-operations/"+id, "", nil, &out)
	return out, err
}

// CancelEngineeringOperation persists cancellation intent. Observed source
// termination and any prior external commit are reported separately by the core.
func (c *Client) CancelEngineeringOperation(ctx context.Context, id string) (out EngineeringOperation, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid operation identifier")
	}
	err = c.call(ctx, "POST", "/v1/engineering-operations/"+id+"/cancel", "", struct{}{}, &out)
	return out, err
}
