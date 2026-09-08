package nlqbyo

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// Service contains immutable dependencies/configuration, never request state.
type Service struct {
	router    Router
	topics    TopicReader
	rules     RuleReader
	sources   SourceReader
	validator Validator
	executor  Executor
	repo      Repository
	limits    config.QueryBundles
	read      config.ReadValidation
	now       func() time.Time
}

// New constructs the BYO consumer. A nil router disables only new contexts;
// previously issued references remain independently useful without a model.
func New(router Router, topicReader TopicReader, rules RuleReader, sources SourceReader, validator Validator, executor Executor, repo Repository, limits config.QueryBundles, read config.ReadValidation, now func() time.Time) (*Service, error) {
	for _, dep := range []any{topicReader, rules, sources, validator, executor, repo} {
		if nilValue(dep) {
			return nil, ErrInvalid
		}
	}
	if config.ValidateQueryBundles(limits) != nil || config.ValidateReadValidation(read) != nil {
		return nil, ErrInvalid
	}
	if nilValue(router) {
		router = nil
	}
	if now == nil {
		now = time.Now
	}
	return &Service{router: router, topics: topicReader, rules: rules, sources: sources, validator: validator, executor: executor, repo: repo, limits: limits, read: read, now: now}, nil
}

func nilValue(v any) bool {
	return v == nil || reflect.ValueOf(v).Kind() == reflect.Pointer && reflect.ValueOf(v).IsNil()
}

// CanCreate reports actual routing availability for operation registration.
func (s *Service) CanCreate() bool { return s != nil && s.router != nil }

// Create constructs and persists one compact exact snapshot. No SQL is generated
// or executed, and caller examples can never masquerade as reviewed evidence.
func (s *Service) Create(ctx context.Context, e identity.Envelope, in CreateRequest) (CreateResult, error) {
	out := CreateResult{SchemaVersion: Version}
	if err := admit(ctx, e, "query.context"); err != nil {
		return out, err
	}
	if in.SchemaVersion != Version || !identity.Identifier(in.Route.Context) {
		return out, ErrInvalid
	}
	if !s.CanCreate() {
		return out, ErrUnavailable
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	// Detach before adding trustworthy provenance; no mutation of caller slices.
	in.Route.Examples = append([]nlq.OptionalItem(nil), in.Route.Examples...)
	for i := range in.Route.Examples {
		in.Route.Examples[i].Source = "external_input_unreviewed"
		in.Route.Examples[i].Confidence = nil
	}
	routed, err := s.router.Route(ctx, e, in.Route)
	if err != nil {
		return out, err
	}
	out.Outcome, out.Clarification = routed.Outcome, routed.Clarification
	if routed.Context == nil || routed.Outcome == nlq.StrategyClarify || routed.Outcome == nlq.StrategyNoRoute {
		return out, nil
	}
	// A JSON context or arbitrary submitter cannot reconstruct this seal.
	if _, err = routed.GenerationContext(); err != nil {
		return out, err
	}
	pins, definitions, err := s.current(ctx, e, routed.Topics)
	if err != nil {
		return out, err
	}
	if len(pins) == 0 || len(pins) != len(routed.TopicVersions) || len(pins) != len(routed.RuleVersions) {
		return out, ErrReplan
	}
	for i, pin := range pins {
		if pin.Version != routed.TopicVersions[i] || pin.RuleVersion != routed.RuleVersions[i] {
			return out, ErrReplan
		}
	}
	var source string
	for _, d := range definitions {
		for _, dataset := range d.Datasets {
			if dataset.Source.Context != in.Route.Context || source != "" && source != dataset.Source.Source {
				return out, ErrReplan
			}
			source = dataset.Source.Source
		}
	}
	binding, err := s.sources.Binding(ctx, e, source, in.Route.Context)
	if err != nil {
		return out, err
	}
	relations, err := semanticRelations(binding, definitions)
	if err != nil {
		return out, err
	}
	id, err := opaqueID()
	if err != nil {
		return out, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	bundle := Bundle{
		Reference: Reference{Version, id, in.Route.Context}, CreatedAt: now, ExpiresAt: now.Add(time.Duration(s.limits.TTL)), Source: source,
		Semantics: pins, Context: *routed.Context, MaxSteps: s.limits.MaxSteps,
		ContextUsage: routed.RemoteCalls, Warnings: append([]string{}, routed.Warnings...), Provenance: "reviewed_topics_and_rules; external_examples_unreviewed; external_sql_not_certified",
		Requirements: SQLRequirements{Dialect: binding.Dialect, Catalog: binding.Catalog, ParameterStyle: parameterStyle(binding.Dialect), ParameterKinds: []string{"null", "text", "boolean", "integer", "number"}, MaxSQLBytes: s.read.MaxSQLBytes, MaxParameters: s.read.MaxParameters, Relations: relations, ReadOnly: true, Rows: s.read.RowsDefault, Bytes: s.read.BytesDefault, TimeoutMillis: min(time.Duration(s.limits.StepTimeout), time.Duration(s.read.Timeout)).Milliseconds()},
	}
	// Canonical JSON round-trip detaches all nested context/receipt structures.
	raw, err := json.Marshal(bundle)
	if err != nil || len(raw) > s.limits.MaxBytes {
		return out, ErrBudget
	}
	if json.Unmarshal(raw, &bundle) != nil {
		return out, ErrInvalid
	}
	record := Record{Bundle: bundle, Session: e.Session(), Binding: binding.Clone(), DataReach: dataReach(e), Digest: exec.Hash(bundle), RetainUntil: now.Add(time.Duration(s.limits.Retention))}
	storedBytes, err := json.Marshal(record)
	if err != nil || len(storedBytes) > s.limits.MaxBytes {
		return out, ErrBudget
	}
	if !e.Valid() {
		return out, access.ErrUnauthenticated
	}
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return out, err
	}
	if err = s.repo.CreateBYOBundle(ctx, scope, record, s.limits); err != nil {
		return out, err
	}
	out.Bundle = &bundle
	return out, nil
}

// Lookup reauthorizes the caller, partition and exact current revisions before
// returning either bundle contents or per-step evidence.
func (s *Service) Lookup(ctx context.Context, e identity.Envelope, in Reference) (View, error) {
	record, scope, err := s.load(ctx, e, in, "query.context")
	if err != nil {
		return View{}, err
	}
	steps, err := s.repo.ReadBYOSteps(ctx, scope, in, e.Session())
	if err != nil {
		return View{}, err
	}
	if !e.Valid() {
		return View{}, access.ErrUnauthenticated
	}
	if !s.now().Before(record.Bundle.ExpiresAt) {
		return View{}, ErrReplan
	}
	return View{record.Bundle, steps}, nil
}

// Submit performs exactly one externally requested validation/read. Reusing an
// operation returns its receipt, never another warehouse call or fabricated values.
func (s *Service) Submit(ctx context.Context, e identity.Envelope, in SubmitRequest) (SubmitResult, error) {
	out := SubmitResult{SchemaVersion: Version}
	// Action and basic shape are checked before any reference/source access.
	if err := admit(ctx, e, "query.submit"); err != nil {
		return out, err
	}
	if !identity.Identifier(in.Operation) || !utf8.ValidString(in.SQL) || len(in.SQL) < 1 || len(in.SQL) > s.read.MaxSQLBytes || len(in.Parameters) > s.read.MaxParameters {
		return out, ErrInvalid
	}
	in.Parameters = append([]exec.Parameter{}, in.Parameters...)
	for _, p := range in.Parameters {
		if !p.Valid() {
			return out, exec.ErrBinding
		}
	}
	record, scope, err := s.load(ctx, e, in.Reference, "query.submit")
	if err != nil {
		return out, err
	}
	b := record.Bundle
	if len(in.SQL) > b.Requirements.MaxSQLBytes || len(in.Parameters) > b.Requirements.MaxParameters {
		return out, ErrInvalid
	}
	if err = exec.Require(e, record.Binding, datasetIDs(b.Requirements.Relations)); err != nil {
		return out, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	deadline := now.Add(time.Duration(b.Requirements.TimeoutMillis) * time.Millisecond)
	if b.ExpiresAt.Before(deadline) {
		deadline = b.ExpiresAt
	}
	if e.Deadline().Before(deadline) {
		deadline = e.Deadline()
	}
	step := Step{Operation: in.Operation, InputDigest: exec.Hash([]any{in.SQL, in.Parameters}), BundleDigest: record.Digest, Semantics: b.Semantics, Status: "accepted", Code: "pending", CreatedAt: now, Deadline: deadline}
	step, fresh, err := s.repo.ReserveBYOStep(ctx, scope, in.Reference, e.Session(), step, now)
	if err != nil {
		return out, err
	}
	out.Step = step
	if !fresh {
		out.Replayed = true
		if step.Status == "accepted" && !s.now().Before(step.Deadline) {
			// A process may have died before or after a physical dispatch. Never
			// reinterpret an accepted ledger row as permission for another try.
			out.Step.Status, out.Step.Code = "uncertain", "execution_outcome_unknown"
		}
		return out, nil
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	plan, runErr := s.validator.ValidateWithin(ctx, e, exec.Request{Source: b.Source, Context: in.Context, SQL: in.SQL, Parameters: append([]exec.Parameter(nil), in.Parameters...)}, relationScope(b.Requirements.Relations))
	if runErr == nil {
		// Reject rotation during admission instead of accepting the validator's
		// newer binding and silently replacing the captured data partition.
		_, _, runErr = plan.SQL(e, record.Binding)
	}
	if runErr != nil {
		step.Status, step.Code = "rejected", errorCode(runErr)
	} else {
		operation := "byo:" + exec.Hash([]string{e.Tenant(), e.User(), e.Session(), in.ID, in.Operation})
		report, executeErr := s.executor.Execute(ctx, e, plan, exec.Options{Operation: operation, Number: 1, Rows: b.Requirements.Rows, Bytes: b.Requirements.Bytes})
		if report.Attempt.ID != "" {
			step.Execution = &report.Attempt
			step.Status, step.Code = report.Attempt.Status, report.Attempt.Code
			if step.Code == "" {
				step.Code = "ok"
			}
			out.Result, out.ValuesAvailable = report.Result, report.Result != nil
		} else {
			step.Status, step.Code = "uncertain", errorCode(executeErr)
		}
		if executeErr != nil {
			out.Result, out.ValuesAvailable = nil, false
			step.Code = errorCode(executeErr)
		}
	}
	finished := s.now().UTC().Truncate(time.Microsecond)
	step.FinishedAt = &finished
	// Finishing content-free evidence is cleanup of an accepted step, not new
	// authority or a detached query. Disconnect/expiry must not lose its receipt.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
	err = s.repo.FinishBYOStep(finishCtx, scope, in.Reference, e.Session(), step)
	finishCancel()
	if err != nil {
		return SubmitResult{}, err
	}
	out.Step = step
	if !e.Valid() {
		return SubmitResult{}, access.ErrUnauthenticated
	}
	if !s.now().Before(b.ExpiresAt) {
		return SubmitResult{}, ErrReplan
	}
	if err = ctx.Err(); err != nil {
		return SubmitResult{}, err
	}
	return out, nil
}

func admit(ctx context.Context, e identity.Envelope, action string) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if !e.Has(action) {
		return access.ErrForbidden
	}
	return nil
}

func (s *Service) load(ctx context.Context, e identity.Envelope, in Reference, action string) (Record, store.Scope, error) {
	var zero Record
	var scope store.Scope
	if err := admit(ctx, e, action); err != nil {
		return zero, scope, err
	}
	if in.SchemaVersion != Version {
		return zero, scope, ErrInvalid
	}
	if !ReferenceValid(in) {
		return zero, scope, ErrReplan
	}
	ctx, cancel := context.WithDeadline(ctx, e.Deadline())
	defer cancel()
	scope, err := store.NewScope(e.Tenant(), e.User())
	if err != nil {
		return zero, scope, err
	}
	record, err := s.repo.ReadBYOBundle(ctx, scope, in, e.Session(), s.now())
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			err = ErrReplan
		}
		return zero, scope, err
	}
	if record.DataReach != dataReach(e) || !RecordValid(record, scope, s.limits) || !s.now().Before(record.Bundle.ExpiresAt) || record.Session != e.Session() || record.Bundle.Reference != in {
		return zero, scope, ErrReplan
	}
	// Complete dependency reach is rechecked before any current-source lookup.
	resources := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: record.Bundle.Source}, {Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: in.Context}}
	ids := make([]string, len(record.Bundle.Semantics))
	for i, pin := range record.Bundle.Semantics {
		ids[i] = pin.Topic
		resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: pin.Topic})
	}
	for _, r := range record.Bundle.Requirements.Relations {
		resources = append(resources, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: r.ID})
	}
	if err = access.Require(e, action, resources...); err != nil {
		return zero, scope, ErrReplan
	}
	if action == "query.submit" {
		// Context-only authority must not even reach native discovery/planning.
		if err = exec.Require(e, record.Binding, datasetIDs(record.Bundle.Requirements.Relations)); err != nil {
			return zero, scope, err
		}
	}
	pins, _, err := s.current(ctx, e, ids)
	if err != nil {
		return zero, scope, replanError(err)
	}
	if exec.Hash(pins) != exec.Hash(record.Bundle.Semantics) {
		return zero, scope, ErrReplan
	}
	binding, err := s.sources.Binding(ctx, e, record.Bundle.Source, in.Context)
	if err != nil {
		return zero, scope, replanError(err)
	}
	if exec.Hash(binding) != exec.Hash(record.Binding) {
		return zero, scope, ErrReplan
	}
	r := record.Bundle.Requirements
	if r.MaxSQLBytes > s.read.MaxSQLBytes || r.MaxParameters > s.read.MaxParameters || r.Rows > s.read.RowsCeiling || r.Bytes > s.read.BytesCeiling || r.TimeoutMillis > min(time.Duration(s.limits.StepTimeout), time.Duration(s.read.Timeout)).Milliseconds() {
		return zero, scope, ErrReplan
	}
	return record, scope, nil
}

func (s *Service) current(ctx context.Context, e identity.Envelope, ids []string) ([]SemanticPin, []topics.Definition, error) {
	if len(ids) == 0 || len(ids) > 4 {
		return nil, nil, ErrReplan
	}
	pins := make([]SemanticPin, 0, len(ids))
	definitions := make([]topics.Definition, 0, len(ids))
	for _, id := range ids {
		contract, err := s.topics.Contract(ctx, e, id)
		if err != nil {
			return nil, nil, err
		}
		p := contract.Publication
		pin := SemanticPin{Topic: id, Version: p.State.Version, Digest: p.Digest}
		if !p.State.Active || p.State.Archived || p.Definition.Topic != id || p.Definition.Version != pin.Version || !topics.DigestValid(pin.Digest) {
			return nil, nil, ErrReplan
		}
		rules, err := s.rules.Read(ctx, e, id, "")
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, nil, err
		}
		if err == nil {
			if !rules.State.Active || rules.Definition.Topic != id || rules.Definition.TopicVersion != pin.Version || rules.Definition.PackDigest != pin.Digest {
				return nil, nil, ErrReplan
			}
			pin.RuleVersion, pin.RuleDigest = rules.State.Version, rules.Digest
		}
		pins = append(pins, pin)
		definitions = append(definitions, p.Definition)
	}
	return pins, definitions, nil
}

func semanticRelations(binding exec.Binding, definitions []topics.Definition) ([]exec.Relation, error) {
	if !binding.Valid() {
		return nil, ErrReplan
	}
	allowed := map[string]map[string]bool{}
	for _, definition := range definitions {
		for _, dataset := range definition.Datasets {
			if dataset.Source.Source != binding.Source || dataset.Source.Context != binding.Context || dataset.Source.SourceRevision != binding.Revision {
				return nil, ErrReplan
			}
			if allowed[dataset.ID] == nil {
				allowed[dataset.ID] = map[string]bool{}
			}
			for _, column := range dataset.Columns {
				allowed[dataset.ID][column.SourceName] = true
			}
		}
	}
	out := make([]exec.Relation, 0, len(allowed))
	for _, relation := range binding.Relations {
		columns, ok := allowed[relation.ID]
		if !ok {
			continue
		}
		r := relation
		r.Columns = nil
		for _, c := range relation.Columns {
			if columns[c.Name] && c.Safe {
				r.Columns = append(r.Columns, c)
			}
		}
		if len(r.Columns) != len(columns) || len(r.Columns) == 0 {
			return nil, ErrReplan
		}
		sort.Slice(r.Columns, func(i, j int) bool { return r.Columns[i].Name < r.Columns[j].Name })
		out = append(out, r)
	}
	if len(out) == 0 || len(out) != len(allowed) {
		return nil, ErrReplan
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func relationScope(relations []exec.Relation) []exec.RelationScope {
	out := make([]exec.RelationScope, len(relations))
	for i, r := range relations {
		out[i].Dataset = r.ID
		for _, c := range r.Columns {
			out[i].Columns = append(out[i].Columns, c.Name)
		}
	}
	return out
}

func datasetIDs(relations []exec.Relation) []string {
	out := make([]string, len(relations))
	for i, r := range relations {
		out[i] = r.ID
	}
	return out
}

func opaqueID() (string, error) {
	var value [32]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", ErrUnavailable
	}
	return hex.EncodeToString(value[:]), nil
}

// ReferenceValid checks only syntax; no caller may use it as authorization.
func ReferenceValid(in Reference) bool {
	b, err := hex.DecodeString(in.ID)
	return in.SchemaVersion == Version && identity.Identifier(in.Context) && err == nil && len(b) == 32 && hex.EncodeToString(b) == in.ID
}

// RecordValid defends the storage seam without manufacturing caller authority.
func RecordValid(r Record, scope store.Scope, limits config.QueryBundles) bool {
	b := r.Bundle
	if !scope.Valid() || !ReferenceValid(b.Reference) || !identity.Identifier(r.Session) || !r.Binding.Valid() || r.Binding.Tenant != scope.Tenant() || r.Binding.Source != b.Source || r.Binding.Context != b.Reference.Context || r.Digest != exec.Hash(b) || !topics.DigestValid(r.DataReach) || b.CreatedAt.IsZero() || !b.ExpiresAt.After(b.CreatedAt) || b.ExpiresAt.Sub(b.CreatedAt) > time.Hour || r.RetainUntil.Before(b.ExpiresAt) || r.RetainUntil.Sub(b.CreatedAt) > 7*24*time.Hour || b.MaxSteps < 1 || b.MaxSteps > limits.MaxSteps || len(b.Semantics) < 1 || len(b.Semantics) > 4 {
		return false
	}
	seen := map[string]bool{}
	for _, pin := range b.Semantics {
		if seen[pin.Topic] || !identity.Identifier(pin.Topic) || !identity.Identifier(pin.Version) || !topics.DigestValid(pin.Digest) || (pin.RuleVersion == "") != (pin.RuleDigest == "") || pin.RuleVersion != "" && (!identity.Identifier(pin.RuleVersion) || !topics.DigestValid(pin.RuleDigest)) {
			return false
		}
		seen[pin.Topic] = true
	}
	raw, err := json.Marshal(r)
	return err == nil && len(raw) <= limits.MaxBytes
}

// dataReach pins data-reading reach, not operation permissions. A separately
// granted query.submit/sources.query action and cw.source.query reach may enable
// submission, but changing topic/dataset/partition reach requires new context.
func dataReach(e identity.Envelope) string {
	var values []string
	for _, r := range e.Reach() {
		if r.Kind == "topic" && r.Permission == "read" || r.Kind == "dataset" && r.Permission == "query" || r.Kind == "source" && r.Permission == "read" || r.Kind == "execution_context" && r.Permission == "use" {
			values = append(values, r.Kind+"."+r.Permission+":"+r.ID)
		}
	}
	sort.Strings(values)
	return exec.Hash(values)
}

func replanError(err error) error {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrConflict) || errors.Is(err, exec.ErrBinding) || errors.Is(err, access.ErrForbidden) || errors.Is(err, access.ErrNotFound) {
		return ErrReplan
	}
	return err
}

func parameterStyle(dialect string) string {
	switch dialect {
	case "postgres":
		return "$1, $2, ..."
	case "sqlserver":
		return "@p1, @p2, ..."
	default:
		return "? (positional)"
	}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, exec.ErrUnsafe):
		return "sql_unsafe"
	case errors.Is(err, exec.ErrUnsupported), errors.Is(err, exec.ErrType):
		return "unsupported"
	case errors.Is(err, exec.ErrLimit):
		return "limit_exceeded"
	case errors.Is(err, exec.ErrBinding), errors.Is(err, ErrReplan):
		return "replan_required"
	case errors.Is(err, access.ErrForbidden), errors.Is(err, access.ErrUnauthenticated):
		return "forbidden"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded), errors.Is(err, exec.ErrCancelled), errors.Is(err, exec.ErrTimeout):
		return "cancelled_or_timed_out"
	default:
		return "execution_outcome_unknown"
	}
}

// StepValid restricts receipt payloads to the fixed content-free durable contract.
func StepValid(s Step) bool {
	if !identity.Identifier(s.Operation) || s.Number < 0 || s.Number > 32 || !topics.DigestValid(s.InputDigest) || !topics.DigestValid(s.BundleDigest) || len(s.Semantics) < 1 || len(s.Semantics) > 4 || s.ModelCalls != 0 || s.CreatedAt.IsZero() || !s.Deadline.After(s.CreatedAt) || s.Deadline.Sub(s.CreatedAt) > time.Minute {
		return false
	}
	if !strings.Contains("|accepted|rejected|succeeded|empty|truncated|failed|uncertain|cancelled|timed_out|interrupted|", "|"+s.Status+"|") || len(s.Code) == 0 || len(s.Code) > 64 {
		return false
	}
	raw, err := json.Marshal(s)
	return err == nil && len(raw) <= 32<<10
}
