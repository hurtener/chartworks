package nlqbyo

import (
	"context"
	"encoding/json"
	"reflect"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
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
	if err = ctx.Err(); err != nil {
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
	if runErr == nil {
		// Native planning can outlive an intervening publication or retirement.
		// Recheck exact pins before dispatch; never replace them with current ones.
		_, _, runErr = s.load(ctx, e, in.Reference, "query.submit")
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
