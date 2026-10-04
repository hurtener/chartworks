package reporting

import (
	"context"
	"errors"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type AuthoringPrepareRequest struct {
	NewBlock         string                 `json:"new_block"`
	Operation        string                 `json:"operation"`
	OperationVersion string                 `json:"operation_version,omitempty"`
	Intent           AuthoringDatasetIntent `json:"intent"`
	Metadata         []Localized            `json:"metadata"`
}
type AuthoringPreparationRequest struct {
	NewBlock    string `json:"new_block"`
	Preparation string `json:"preparation,omitempty"`
	Operation   string `json:"operation,omitempty"`
}
type AuthoringCreatePreparedRequest struct {
	NewBlock    string `json:"new_block"`
	Preparation string `json:"preparation"`
	Digest      string `json:"digest"`
}
type AuthoringPreparationControlRequest struct {
	NewBlock    string `json:"new_block"`
	Preparation string `json:"preparation,omitempty"`
	Operation   string `json:"operation,omitempty"`
	Action      string `json:"action" jsonschema:"enum=inspect,enum=cancel,enum=reconcile"`
}

// No SQL, native control coordinates or rows are exposed by this view.
type AuthoringPreparationView struct {
	Preparation     string          `json:"preparation"`
	NewBlock        string          `json:"new_block"`
	Operation       string          `json:"operation"`
	Digest          string          `json:"digest,omitempty"`
	Status          string          `json:"status"`
	Code            string          `json:"code,omitempty"`
	Schema          []exec.Field    `json:"schema"`
	Mapping         *charts.Mapping `json:"mapping,omitempty"`
	ExpiresAt       time.Time       `json:"expires_at"`
	Validation      string          `json:"validation"`
	ExecutionStatus string          `json:"execution_status,omitempty"`
	RemoteState     string          `json:"remote_state,omitempty"`
}

// Protected custody only, never transport input/output. Accepted inputs are
// immutable; a terminal outcome can add observed schema but no raw rows.
type AuthoringPreparationRecord struct {
	Consumed        *AuthoringPreparationConsumed `json:"-"`
	Settled         bool                          `json:"-"`
	ID              string                        `json:"id"`
	Actor           string                        `json:"actor"`
	Session         string                        `json:"session"`
	Target          string                        `json:"target"`
	Operation       string                        `json:"operation"`
	InputDigest     string                        `json:"input_digest"`
	Compiler        string                        `json:"compiler"`
	SourceOperation string                        `json:"source_operation"`
	Binding         exec.Binding                  `json:"binding"`
	Topics          []TopicPin                    `json:"topics"`
	References      []ResourceReference           `json:"references"`
	Scope           []exec.RelationScope          `json:"scope"`
	Dependencies    []Dependency                  `json:"dependencies"`
	Statement       string                        `json:"statement"`
	Request         AuthoringPrepareRequest       `json:"request"`
	CreatedAt       time.Time                     `json:"created_at"`
	Deadline        time.Time                     `json:"deadline"`
	ExpiresAt       time.Time                     `json:"expires_at"`
	Status          string                        `json:"status"`
	Code            string                        `json:"code"`
	Digest          string                        `json:"digest"`
	Revision        *Revision                     `json:"revision,omitempty"`
	Attempt         *exec.Attempt                 `json:"attempt,omitempty"`
}

func (AuthoringPreparationRecord) String() string     { return "authoring-preparation(redacted)" }
func (r AuthoringPreparationRecord) GoString() string { return r.String() }

type AuthoringPreparationReference struct {
	ID     string `json:"id"`
	Digest string `json:"digest"`
}
type AuthoringPreparationRepository interface {
	AuthoringRulesActive(context.Context, identity.Envelope, TopicPin) (bool, error)
	ReserveAuthoringPreparation(context.Context, identity.Envelope, AuthoringPreparationRecord) (AuthoringPreparationRecord, bool, error)
	ReadAuthoringPreparation(context.Context, identity.Envelope, string) (AuthoringPreparationRecord, error)
	ReadAuthoringPreparationOperation(context.Context, identity.Envelope, string) (AuthoringPreparationRecord, error)
	FinishAuthoringPreparation(context.Context, identity.Envelope, AuthoringPreparationRecord) error
	SealAuthoringPreparation(context.Context, identity.Envelope, AuthoringPreparationRecord, exec.Receipt) error
	SettleAuthoringPreparation(context.Context, identity.Envelope, string, bool) (AuthoringPreparationRecord, error)
}

// Repeated by storage; source-query reach is required at prepare and consume,
// even though consume does not perform another source read.
func RequireAuthoringPreparation(e identity.Envelope, r AuthoringPreparationRecord, execute bool) error {
	if err := requireMappingAuthoring(e); err != nil {
		return err
	}
	if r.Actor != e.User() || r.Session != e.Session() || r.Binding.Tenant != e.Tenant() {
		return access.ErrNotFound
	}
	if !identity.Identifier(r.Target) || len(r.Topics) != 1 || !identity.Identifier(r.Topics[0].Topic) {
		return ErrInvalid
	}
	for _, a := range []Access{Read, Write, Preview} {
		if err := Require(e, r.Target, a); err != nil {
			return err
		}
	}
	if err := access.Require(e, Write.Action(), access.Tenant(e, "write")); err != nil {
		return err
	}
	if err := RequireParent(e, r.Topics[0].Topic, Write, true); err != nil {
		return err
	}
	if err := RequireReferences(e, Write, r.References); err != nil {
		return err
	}
	if execute {
		if err := Require(e, r.Target, Validate); err != nil {
			return err
		}
		return exec.Require(e, r.Binding, []string{r.Request.Intent.Dataset})
	}
	return nil
}
func (s *Service) preparationRepo() (AuthoringPreparationRepository, error) {
	r, ok := s.repo.(AuthoringPreparationRepository)
	if !ok {
		return nil, ErrUnavailable
	}
	return r, nil
}
func (s *Service) authoringRulesDisposition(ctx context.Context, e identity.Envelope, topic TopicPin) (string, error) {
	r, err := s.preparationRepo()
	if err != nil {
		return "", err
	}
	active, err := r.AuthoringRulesActive(ctx, e, topic)
	if err != nil {
		return "", err
	}
	if active {
		return "reviewed_rules_unsupported", nil
	}
	return "", nil
}
func preparationView(r AuthoringPreparationRecord) AuthoringPreparationView {
	out := AuthoringPreparationView{Preparation: r.ID, NewBlock: r.Target, Operation: r.Operation, Digest: r.Digest, Status: r.Status, Code: r.Code, Schema: []exec.Field{}, ExpiresAt: r.ExpiresAt, Validation: "native_validation_required"}
	if r.Status == "accepted" && !time.Now().Before(r.Deadline) {
		out.Status, out.Code = "uncertain", "execution_outcome_unknown"
	}
	if r.Attempt != nil {
		out.ExecutionStatus = r.Attempt.Status
		out.RemoteState = r.Attempt.RemoteState
	}
	if r.Revision != nil && len(r.Revision.Definition.Outputs) == 1 {
		out.Schema = clone(r.Revision.Definition.ExpectedSchema)
		out.Mapping = clone(r.Revision.Definition.Outputs[0].Mapping)
	}
	return out
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
func (s *Authoring) PrepareDatasetChart(ctx context.Context, e identity.Envelope, in AuthoringPrepareRequest) (AuthoringPreparationView, error) {
	if ctx == nil || !identity.Identifier(in.NewBlock) {
		return AuthoringPreparationView{}, ErrInvalid
	}
	if err := requireMappingAuthoring(e); err != nil {
		return AuthoringPreparationView{}, err
	}
	if !identity.Identifier(in.Operation) {
		return AuthoringPreparationView{}, ErrPreparationContract
	}
	// Retained identity wins before current compiler/publication checks, but only
	// after exact original custody and current signed execution reach are checked.
	blocks, err := s.blockService()
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	repo, err := blocks.preparationRepo()
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if prior, readErr := repo.ReadAuthoringPreparationOperation(ctx, e, in.Operation); readErr == nil {
		if err := RequireAuthoringPreparation(e, prior, true); err != nil {
			return AuthoringPreparationView{}, err
		}
		if prior.Target != in.NewBlock || prior.InputDigest != digest(in) {
			return AuthoringPreparationView{}, store.ErrConflict
		}
		return s.Preparation(ctx, e, AuthoringPreparationRequest{NewBlock: in.NewBlock, Operation: in.Operation})
	} else if !errors.Is(readErr, store.ErrNotFound) && !errors.Is(readErr, access.ErrNotFound) {
		return AuthoringPreparationView{}, readErr
	}
	if err := AuthoringPreparationAdmission(in, time.Now()); err != nil {
		return AuthoringPreparationView{}, err
	}
	blocks, p, dataset, err := s.datasetPublication(ctx, e, AuthoringDatasetRequest{Topic: in.Intent.Topic, Dataset: in.Intent.Dataset})
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if !blocks.CanValidate() {
		return AuthoringPreparationView{}, ErrUnavailable
	}
	if !metadataValid(in.Metadata, blocks.limits) {
		return AuthoringPreparationView{}, ErrInvalid
	}
	for _, m := range in.Metadata {
		if m.Intent != nil {
			return AuthoringPreparationView{}, ErrInvalid
		}
	}
	if reason, err := blocks.authoringRulesDisposition(ctx, e, in.Intent.Topic); err != nil {
		return AuthoringPreparationView{}, err
	} else if reason != "" {
		return AuthoringPreparationView{Status: "unsupported", Code: reason, Schema: []exec.Field{}, Validation: "not_performed"}, nil
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	binding, err := blocks.sources.ContextBinding(ctx, e, dataset.Source.Source, dataset.Source.Context)
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	compiled, err := compileAuthoringDataset(in.Intent, p, dataset, binding)
	if err != nil {
		var unsupported *authoringUnsupported
		if errors.As(err, &unsupported) {
			return AuthoringPreparationView{Status: "unsupported", Code: unsupported.code, Schema: []exec.Field{}, Validation: "not_performed"}, nil
		}
		return AuthoringPreparationView{}, err
	}
	id, err := newID()
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	now := time.Now().UTC()
	timeout := min(time.Duration(blocks.limits.ValidationTimeout), time.Duration(blocks.limits.Execution.Timeout))
	skeleton := Definition{Source: binding.Source, Context: binding.Context, Topics: []TopicPin{in.Intent.Topic}}
	r := AuthoringPreparationRecord{ID: id, Actor: e.User(), Session: e.Session(), Target: in.NewBlock, Operation: in.Operation, InputDigest: digest(in), Compiler: AuthoringCompilerForIntent(in.Intent), SourceOperation: "chart-prepare:" + digest([]string{e.Tenant(), e.User(), e.Session(), in.NewBlock, in.Operation}), Binding: binding.Clone(), Topics: clone(skeleton.Topics), References: definitionReferences(skeleton, []topics.Published{p}), Scope: compiled.Scope, Dependencies: compiled.Dependencies, Statement: compiled.SQL, Request: clone(in), CreatedAt: now, Deadline: minTime(now.Add(timeout), e.Deadline()), ExpiresAt: now.Add(min(time.Duration(blocks.limits.EvidenceTTL), 15*time.Minute)), Status: "accepted"}
	if err := RequireAuthoringPreparation(e, r, true); err != nil {
		return AuthoringPreparationView{}, err
	}
	r, fresh, err := repo.ReserveAuthoringPreparation(ctx, e, r)
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if !fresh {
		return preparationView(r), nil
	}
	// Reservation precedes native planning and dispatch. Missing values never
	// authorize another physical read under the same operation identity.
	work, stop := context.WithDeadline(ctx, r.Deadline)
	defer stop()
	r, err = blocks.runAuthoringPreparation(work, e, r, compiled)
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if err := repo.FinishAuthoringPreparation(work, e, r); err != nil {
		return AuthoringPreparationView{}, err
	}
	return preparationView(r), nil
}
func (s *Service) runAuthoringPreparation(ctx context.Context, e identity.Envelope, r AuthoringPreparationRecord, c authoringCompiled) (AuthoringPreparationRecord, error) {
	reject := func(code string) (AuthoringPreparationRecord, error) {
		r.Status, r.Code = "failed", code
		return r, nil
	}
	if err := RequireAuthoringPreparation(e, r, true); err != nil {
		return r, err
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return r, ctx.Err()
	default:
		return reject("preparation_busy")
	}
	resolved, err := ResolveParameters(c.Parameters, nil, Resolution{At: r.CreatedAt, Timezone: "UTC"})
	if err != nil {
		return reject("filter_values_unsupported")
	}
	if err := validateBoundedFilterSQL(ctx, r.Statement, c.Parameters); err != nil {
		return reject("filter_predicates_unsupported")
	}
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: r.Binding.Source, Context: r.Binding.Context, SQL: r.Statement, Parameters: resolved.Parameters}, r.Scope)
	if err != nil {
		return reject("validation_rejected")
	}
	statement, parameters, err := plan.SQL(e, r.Binding)
	if err != nil || statement != r.Statement || parameterDigest(parameters) != parameterDigest(resolved.Parameters) {
		return reject("source_binding_changed")
	}
	if reason, err := s.authoringRulesDisposition(ctx, e, r.Topics[0]); err != nil {
		return r, err
	} else if reason != "" {
		return reject("reviewed_rules_changed")
	}
	p, err := s.topics.Read(ctx, e, r.Topics[0].Topic, r.Topics[0].Version)
	if err != nil {
		return r, err
	}
	if !p.State.Active || p.State.Archived || p.Digest != r.Topics[0].Digest || p.State.Version != r.Topics[0].Version {
		return reject("topic_changed")
	}
	if err := RequireAuthoringPreparation(e, r, true); err != nil {
		return r, err
	}
	repo, err := s.preparationRepo()
	if err != nil {
		return r, err
	}
	if err := repo.SealAuthoringPreparation(ctx, e, r, plan.Receipt()); err != nil {
		return r, err
	}
	report, runErr := s.executor.Execute(ctx, e, plan, exec.Options{Operation: r.SourceOperation, Number: 1, Preview: true, Rows: min(s.limits.PreviewRows, s.limits.Execution.MaxRows), Bytes: min(s.limits.PreviewBytes, s.limits.Execution.MaxResultBytes)})
	if report.Attempt.ID != "" {
		r.Attempt = clone(&report.Attempt)
	}
	if runErr != nil || report.Result == nil || !successful(report.Attempt.Status) || report.Attempt.Finished == nil || report.Attempt.RemoteState != "stopped" {
		r.Status, r.Code = "failed", "source_read_failed"
		if runErr != nil || r.Attempt == nil || report.Attempt.Status == "uncertain" || report.Attempt.RemoteState == "unknown" {
			r.Status, r.Code = "uncertain", "execution_outcome_unknown"
		}
		return r, nil
	}
	if report.Attempt.Manifest.Operation != r.SourceOperation || report.Attempt.Manifest.Session != e.Session() || !report.Attempt.Manifest.Preview || report.Attempt.Manifest.Receipt.Manifest != plan.Receipt().Manifest {
		return reject("attempt_binding_changed")
	}
	result := *report.Result
	if result.Outcome == "truncated" || result.Truncation != "" || report.Attempt.Status == "truncated" {
		return reject("preparation_truncated")
	}
	if len(result.Schema) != len(c.Columns) {
		return reject("schema_unsupported")
	}
	for i, f := range result.Schema {
		if f.Name != c.Columns[i].Name || chartType(f.Type) == "" {
			return reject("schema_unsupported")
		}
		c.Columns[i].Type = chartType(f.Type)
	}
	data := charts.Data{Version: charts.Version, Columns: c.Columns, Rows: [][]charts.Cell{}, Completeness: charts.Completeness{Status: "complete_result"}}
	input := r.Request.Intent.Mapping
	var mapping charts.Mapping
	if input.KPI != nil || input.Table != nil {
		mapping, err = charts.BindDisplay(ctx, data, input.Kind, input.Bindings, input.Order, input.Options, input.KPI, input.Table, chartLimits(s.limits))
	} else {
		mapping, err = charts.Bind(ctx, data, input.Kind, input.Bindings, input.Order, input.Options, chartLimits(s.limits))
	}
	if err != nil {
		return reject("mapping_unsupported")
	}
	kind := "chart"
	if mapping.Kind == charts.Table {
		kind = "table"
	}
	if mapping.Kind == charts.KPI {
		kind = "kpi"
	}
	d := Definition{SchemaVersion: SchemaVersion, Metadata: clone(r.Request.Metadata), Source: r.Binding.Source, Context: r.Binding.Context, Topics: clone(r.Topics), SQL: r.Statement, Parameters: clone(c.Parameters), ExpectedSchema: clone(result.Schema), Outputs: []Output{{ID: "chart", Kind: kind, Mapping: &mapping}}}
	if len(r.Request.Intent.Filters) > 0 {
		d.SchemaVersion = CurrentSchemaVersion
		intent := OutputIntent{Enabled: true, DefaultSelected: true, DisplayOrder: 0, Metadata: []OutputMetadata{}}
		for _, metadata := range d.Metadata {
			intent.Metadata = append(intent.Metadata, OutputMetadata{Locale: metadata.Locale, DisplayName: metadata.Title, Description: metadata.Description})
		}
		d.Outputs[0].Intent = &intent
	}
	if err := validateDefinition(ctx, d, s.limits, false); err != nil {
		return reject("definition_unsupported")
	}
	if err := checkResult(ctx, d, result, s.limits); err != nil {
		return reject("mapping_unsuitable")
	}
	revision, err := s.newRevision(e, 1, d, Provenance{Kind: "manual", RuleAbsence: clone(r.Topics), CaptureDigest: digest([]any{r.Compiler, r.InputDigest, r.Attempt})})
	if err != nil {
		return r, err
	}
	r.Revision = &revision
	r.Status = "prepared"
	r.Code = ""
	r.Digest = digest([]any{r.Compiler, r.ID, r.Target, r.InputDigest, revision.Digest, r.Attempt})
	return r, nil
}

type authoringPreparationControl interface {
	ByOperation(context.Context, identity.Envelope, string) (exec.Attempt, error)
	Control(context.Context, identity.Envelope, string, bool) (exec.ControlReceipt, error)
}

func (s *Authoring) preparationRecord(ctx context.Context, e identity.Envelope, in AuthoringPreparationRequest) (*Service, AuthoringPreparationRecord, error) {
	if ctx == nil || !identity.Identifier(in.NewBlock) || (in.Preparation == "") == (in.Operation == "") || in.Preparation != "" && !identity.Identifier(in.Preparation) || in.Operation != "" && !identity.Identifier(in.Operation) {
		return nil, AuthoringPreparationRecord{}, ErrInvalid
	}
	blocks, err := s.blockService()
	if err != nil {
		return nil, AuthoringPreparationRecord{}, err
	}
	repo, err := blocks.preparationRepo()
	if err != nil {
		return nil, AuthoringPreparationRecord{}, err
	}
	var r AuthoringPreparationRecord
	if in.Preparation != "" {
		r, err = repo.ReadAuthoringPreparation(ctx, e, in.Preparation)
	} else {
		r, err = repo.ReadAuthoringPreparationOperation(ctx, e, in.Operation)
	}
	if err != nil {
		return nil, r, err
	}
	if r.Target != in.NewBlock {
		return nil, r, access.ErrNotFound
	}
	if err := RequireAuthoringPreparation(e, r, false); err != nil {
		return nil, r, err
	}
	return blocks, r, nil
}
func (s *Authoring) Preparation(ctx context.Context, e identity.Envelope, in AuthoringPreparationRequest) (AuthoringPreparationView, error) {
	blocks, r, err := s.preparationRecord(ctx, e, in)
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	out := preparationView(r)
	if r.Consumed != nil {
		v, err := blocks.Read(ctx, e, r.Target, Reference{Revision: 1})
		if err != nil {
			return AuthoringPreparationView{}, err
		}
		if v.Digest != r.Consumed.RevisionDigest || v.ExecutionDigest != r.Consumed.ExecutionDigest {
			return AuthoringPreparationView{}, ErrStale
		}
		out.Schema = clone(v.ExpectedSchema)
		if len(v.Outputs) == 1 {
			out.Mapping = clone(v.Outputs[0].Mapping)
		}
		out.ExecutionStatus, out.RemoteState = r.Consumed.Settlement.Status, r.Consumed.Settlement.RemoteState
	}
	return out, nil
}

// Explicit control, separate from pure metadata polling; never restarts a query.
func (s *Authoring) PreparationControl(ctx context.Context, e identity.Envelope, in AuthoringPreparationControlRequest) (AuthoringPreparationView, error) {
	if in.Action != "inspect" && in.Action != "cancel" && in.Action != "reconcile" {
		return AuthoringPreparationView{}, ErrInvalid
	}
	blocks, r, err := s.preparationRecord(ctx, e, AuthoringPreparationRequest{NewBlock: in.NewBlock, Preparation: in.Preparation, Operation: in.Operation})
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if err := RequireAuthoringPreparation(e, r, true); err != nil {
		return AuthoringPreparationView{}, err
	}
	if r.Consumed != nil {
		return preparationView(r), nil
	}
	repo, err := blocks.preparationRepo()
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if in.Action != "inspect" {
		r, err = repo.SettleAuthoringPreparation(ctx, e, r.ID, in.Action == "cancel")
		if err != nil {
			return AuthoringPreparationView{}, err
		}
		if r.Settled {
			return preparationView(r), nil
		}
	}
	control, ok := blocks.executor.(authoringPreparationControl)
	if !ok {
		return AuthoringPreparationView{}, ErrUnavailable
	}
	attempt, err := control.ByOperation(ctx, e, r.SourceOperation)
	if err != nil {
		return AuthoringPreparationView{}, err
	}
	if in.Action != "inspect" {
		receipt, err := control.Control(ctx, e, attempt.ID, in.Action == "cancel")
		if err != nil {
			return AuthoringPreparationView{}, err
		}
		attempt = receipt.Attempt
		r, err = repo.SettleAuthoringPreparation(ctx, e, r.ID, in.Action == "cancel")
		if err != nil {
			return AuthoringPreparationView{}, err
		}
	}
	out := preparationView(r)
	out.ExecutionStatus, out.RemoteState = attempt.Status, attempt.RemoteState
	return out, nil
}

// Exact preparation consumption creates an unvalidated private native draft.
func (s *Authoring) CreatePreparedChart(ctx context.Context, e identity.Envelope, in AuthoringCreatePreparedRequest) (AuthoringBlockView, error) {
	if ctx == nil || !identity.Identifier(in.NewBlock) || !identity.Identifier(in.Preparation) || !hashValid(in.Digest) {
		return AuthoringBlockView{}, ErrInvalid
	}
	blocks, err := s.blockService()
	if err != nil {
		return AuthoringBlockView{}, err
	}
	repo, err := blocks.preparationRepo()
	if err != nil {
		return AuthoringBlockView{}, err
	}
	r, err := repo.ReadAuthoringPreparation(ctx, e, in.Preparation)
	if err != nil {
		return AuthoringBlockView{}, err
	}
	if r.Target != in.NewBlock || r.Digest != in.Digest || (r.Revision == nil && r.Consumed == nil) {
		return AuthoringBlockView{}, ErrStale
	}
	if err := RequireAuthoringPreparation(e, r, true); err != nil {
		return AuthoringBlockView{}, err
	}
	if r.Status == "consumed" {
		v, err := blocks.Read(ctx, e, r.Target, Reference{Revision: 1})
		if err == nil && r.Consumed != nil && (v.Revision != 1 || v.Digest != r.Consumed.RevisionDigest || v.ExecutionDigest != r.Consumed.ExecutionDigest) {
			return AuthoringBlockView{}, ErrStale
		}
		return authoringBlockView(v), err
	}
	if r.Status != "prepared" || !time.Now().Before(r.ExpiresAt) {
		return AuthoringBlockView{}, ErrStale
	}
	if _, _, err := blocks.resolveDefinitions(ctx, e, r.Revision.Definition, true); err != nil {
		return AuthoringBlockView{}, err
	}
	if reason, err := blocks.authoringRulesDisposition(ctx, e, r.Topics[0]); err != nil {
		return AuthoringBlockView{}, err
	} else if reason != "" {
		return AuthoringBlockView{}, ErrStale
	}
	state, err := blocks.commit(ctx, e, Mutation{ID: r.Target, Topic: r.Topics[0].Topic, Kind: "create", Revision: r.Revision, References: r.References, Preparation: &AuthoringPreparationReference{ID: r.ID, Digest: r.Digest}})
	if err != nil {
		if errors.Is(err, store.ErrConflict) {
			if latest, readErr := repo.ReadAuthoringPreparation(ctx, e, r.ID); readErr == nil && latest.Status == "consumed" && latest.Digest == r.Digest {
				v, readErr := blocks.Read(ctx, e, r.Target, Reference{Revision: 1})
				return authoringBlockView(v), readErr
			}
		}
		return AuthoringBlockView{}, err
	}
	return authoringBlockView(project(Snapshot{State: state, Revision: *r.Revision}, time.Now())), nil
}
