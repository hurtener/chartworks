package nlqexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	pgquery "github.com/wasilibs/go-pgquery"
)

type admission struct {
	route         nlqroute.RouteResult
	assembled     nlq.AssembledContext
	binding       exec.Binding
	source        string
	context       string
	resources     []access.Resource
	relationScope []exec.RelationScope
	relations     []nlq.SourceRelation
	reviewed      []reviewedDataset
}

type reviewedDataset struct {
	topic   string
	dataset topics.Dataset
}

func cloneRelationScope(in []exec.RelationScope) []exec.RelationScope {
	out := append([]exec.RelationScope(nil), in...)
	for i := range out {
		out[i].Columns = append([]string(nil), in[i].Columns...)
	}
	return out
}

// reviewedProjection is derived from immutable reviewed topic columns and the
// current source binding. No caller-supplied dataset reach can enlarge it.
func reviewedProjection(datasets []reviewedDataset, binding exec.Binding) ([]exec.RelationScope, []nlq.SourceRelation, error) {
	if len(datasets) == 0 || len(datasets) > 128 || !binding.Valid() {
		return nil, nil, exec.ErrBinding
	}
	allowed := map[string]map[string]bool{}
	relations := make([]nlq.SourceRelation, 0, len(datasets))
	for _, item := range datasets {
		dataset := item.dataset
		if dataset.Source.Source != binding.Source || dataset.Source.Context != binding.Context || dataset.Source.SourceRevision != binding.Revision || dataset.Source.Dataset != dataset.ID || len(dataset.Columns) == 0 || len(dataset.Columns) > 256 {
			return nil, nil, exec.ErrBinding
		}
		var physical *exec.Relation
		for i := range binding.Relations {
			if binding.Relations[i].ID == dataset.ID {
				if physical != nil {
					return nil, nil, exec.ErrBinding
				}
				physical = &binding.Relations[i]
			}
		}
		if physical == nil {
			return nil, nil, exec.ErrBinding
		}
		relation := nlq.SourceRelation{Topic: item.topic, Dataset: dataset.ID, Name: physical.Schema + "." + physical.Name}
		if allowed[dataset.ID] == nil {
			allowed[dataset.ID] = map[string]bool{}
		}
		seen := map[string]bool{}
		for _, reviewed := range dataset.Columns {
			if seen[reviewed.SourceName] {
				return nil, nil, exec.ErrBinding
			}
			seen[reviewed.SourceName] = true
			found := false
			for _, actual := range physical.Columns {
				if actual.Name == reviewed.SourceName && actual.NativeType == reviewed.NativeType && actual.Category == reviewed.Category && actual.Nullable == reviewed.Nullable && actual.Safe {
					found = true
					break
				}
			}
			if !found {
				return nil, nil, exec.ErrBinding
			}
			relation.Columns = append(relation.Columns, reviewed.SourceName)
			allowed[dataset.ID][reviewed.SourceName] = true
		}
		relations = append(relations, relation)
	}
	ids := make([]string, 0, len(allowed))
	for id := range allowed {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	scope := make([]exec.RelationScope, 0, len(ids))
	for _, id := range ids {
		columns := make([]string, 0, len(allowed[id]))
		for column := range allowed[id] {
			columns = append(columns, column)
		}
		sort.Strings(columns)
		scope = append(scope, exec.RelationScope{Dataset: id, Columns: columns})
	}
	return scope, relations, nil
}

type generatedCandidate struct {
	clarification *ClarificationEvidence
	SQL           string           `json:"sql"`
	Parameters    []exec.Parameter `json:"parameters"`
	Assumptions   []string         `json:"assumptions"`
	Ambiguities   []string         `json:"ambiguities"`
}

var generationSchema, generationSchemaErr = gateway.NewSchema("nlq_sql_candidate", []byte(`{
  "type":"object","additionalProperties":false,
  "required":["sql","parameters","assumptions","ambiguities"],
  "properties":{
    "sql":{"type":"string","minLength":1,"maxLength":32768},
    "parameters":{"type":"array","maxItems":64,"items":{"type":"object","additionalProperties":false,"required":["kind","value"],"properties":{"kind":{"type":"string","enum":["null","text","boolean","integer","number"]},"value":{"type":"string","maxLength":4096}}}},
    "assumptions":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":1024}},
    "ambiguities":{"type":"array","maxItems":32,"items":{"type":"string","maxLength":1024}}
  }
}`))

// Preflight admits a question and persists routing evidence without generation or execution.
func (s *Service) Preflight(ctx context.Context, e identity.Envelope, in PreflightRequest) (PreflightResult, error) {
	if ctx == nil || !e.Valid() {
		return PreflightResult{}, access.ErrUnauthenticated
	}
	canonicalizeQuestion(&in.QuestionRequest)
	if err := requireQuestionAction(e, "query.preflight", in.QuestionRequest); err != nil {
		return PreflightResult{}, err
	}
	if err := validateQuestion(in.QuestionRequest); err != nil {
		return PreflightResult{}, err
	}
	observedParent, err := s.observeParent(ctx, e, in.ClarificationQuery, in.Context)
	if err != nil {
		return PreflightResult{}, err
	}
	if err := s.validateClarificationOrigin(ctx, e, in.QuestionRequest, "query.preflight"); err != nil {
		return PreflightResult{}, err
	}
	admitted, err := s.admit(ctx, e, in.QuestionRequest, false)
	if err != nil {
		return PreflightResult{}, err
	}
	if err := s.ensureSession(ctx, e, in.QuestionRequest, admitted.route.Topics); err != nil {
		return PreflightResult{}, err
	}
	id, err := newID()
	if err != nil {
		return PreflightResult{}, err
	}
	record := queryRecord(e, id, "preflight", in.ClarificationQuery, in.QuestionRequest, admitted)
	bindParentLineage(&record, observedParent)
	if err = s.repo.CreateQuery(ctx, mustScope(e), record); err != nil {
		return PreflightResult{}, err
	}
	return PreflightResult{QueryID: id, SessionID: e.Session(), Route: admitted.route, Confidence: admitted.route.Confidence, Assumptions: assumptions(admitted.route), Ambiguities: ambiguities(admitted.route)}, nil
}

// Plan generates one bounded candidate and validates it through the existing read core.
func (s *Service) Plan(ctx context.Context, e identity.Envelope, in PlanRequest) (PlanResult, error) {
	canonicalizeQuestion(&in.QuestionRequest)
	if ctx == nil || !e.Valid() {
		return PlanResult{}, access.ErrUnauthenticated
	}
	if err := requireQuestionAction(e, "query.plan", in.QuestionRequest); err != nil {
		return PlanResult{}, err
	}
	if err := validateQuestion(in.QuestionRequest); err != nil {
		return PlanResult{}, err
	}
	observedParent, err := s.observeParent(ctx, e, in.ClarificationQuery, in.Context)
	if err != nil {
		return PlanResult{}, err
	}
	if err := s.validateClarificationOrigin(ctx, e, in.QuestionRequest, "query.plan"); err != nil {
		return PlanResult{}, err
	}
	return s.plan(ctx, e, in.QuestionRequest, in.Operation, in.ClarificationQuery, observedParent, "query.plan")
}

// PlanAndRun performs the smallest governed Plan→Run composition under one
// durable operation lock. This keeps the model receipt and the matching
// physical-source receipt in one request under concurrent cold access. The
// operation ledger remains authoritative for replay; this method adds no
// process-local result cache.
func (s *Service) PlanAndRun(ctx context.Context, e identity.Envelope, plan PlanRequest, run RunRequest) (PlanResult, RunResult, error) {
	canonicalizeQuestion(&plan.QuestionRequest)
	if ctx == nil || !e.Valid() || !identity.Identifier(plan.Operation) || run.Operation != plan.Operation {
		return PlanResult{}, RunResult{}, ErrInvalid
	}
	if err := requireQuestionAction(e, "query.plan", plan.QuestionRequest); err != nil {
		return PlanResult{}, RunResult{}, err
	}
	if !e.Has("query.execute") {
		return PlanResult{}, RunResult{}, access.ErrForbidden
	}
	if err := validateQuestion(plan.QuestionRequest); err != nil {
		return PlanResult{}, RunResult{}, err
	}
	observedParent, err := s.observeParent(ctx, e, plan.ClarificationQuery, plan.Context)
	if err != nil {
		return PlanResult{}, RunResult{}, err
	}
	if err := s.validateClarificationOrigin(ctx, e, plan.QuestionRequest, "query.plan"); err != nil {
		return PlanResult{}, RunResult{}, err
	}
	scopeValue, err := scope(e)
	if err != nil {
		return PlanResult{}, RunResult{}, err
	}
	var planned PlanResult
	var result RunResult
	compose := func() error {
		record, readErr := s.repo.ReadOperation(ctx, scopeValue, plan.Operation)
		if readErr == nil {
			planned, readErr = reusablePlanResult(record, e, plan.QuestionRequest, plan.Operation, plan.ClarificationQuery)
			if readErr != nil {
				return readErr
			}
		} else if errors.Is(readErr, store.ErrNotFound) {
			planned, readErr = s.planFreshWithActions(ctx, e, plan.QuestionRequest, plan.Operation, plan.ClarificationQuery, observedParent, "query.plan", nil, []string{"query.execute"})
			if readErr != nil {
				return readErr
			}
		} else {
			return readErr
		}
		run.QueryID = planned.QueryID
		result, readErr = s.Run(ctx, e, run)
		return readErr
	}
	if locker, ok := s.repo.(PlanOperationLocker); ok {
		err = locker.WithPlanOperationLock(ctx, scopeValue, plan.Operation, compose)
	} else {
		// Non-PostgreSQL repositories are useful for unit-level compositions;
		// production release composition uses the scoped PostgreSQL lock.
		err = compose()
	}
	if err != nil {
		return PlanResult{}, result, err
	}
	return planned, result, nil
}

// PerformanceModelMode reports the evidence mode of the configured gateway.
// The release adapter deliberately requires an engine that attests this
// boundary; an arbitrary queryRuntime cannot label its measurements recorded
// or live by assertion alone.
func (s *Service) PerformanceModelMode() string {
	if s == nil || s.engine == nil {
		return ""
	}
	attestor, ok := s.engine.(interface{ PerformanceModelMode() string })
	if !ok {
		return ""
	}
	switch mode := attestor.PerformanceModelMode(); mode {
	case "recorded", "live":
		return mode
	default:
		return ""
	}
}

// PerformancePlanRunIsolated reports whether the query repository provides the
// cross-process operation lock required to keep concurrent cold Plan→Run
// evidence attached to one durable operation.
func (s *Service) PerformancePlanRunIsolated() bool {
	if s == nil || s.repo == nil {
		return false
	}
	_, ok := s.repo.(PlanOperationLocker)
	return ok
}

// Refine creates a child plan anchored to the original query's signed session and topics.
func (s *Service) Refine(ctx context.Context, e identity.Envelope, in RefineRequest) (PlanResult, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(in.QueryID) {
		return PlanResult{}, ErrInvalid
	}
	if !e.Has("query.execute") {
		return PlanResult{}, access.ErrForbidden
	}
	old, err := s.repo.ReadQuery(ctx, mustScope(e), in.QueryID)
	if err != nil {
		return PlanResult{}, err
	}
	if old.Session != e.Session() {
		return PlanResult{}, ErrForeignSession
	}
	if err := s.checkRefinementDepth(ctx, e, old); err != nil {
		return PlanResult{}, err
	}
	if in.ClarificationQuery != "" && in.ClarificationQuery != in.QueryID {
		return PlanResult{}, clarificationOriginError(in.Locale, "clarification_question_mismatch")
	}
	pending := old.Status == "preflight" && old.Route.Clarification != nil
	if old.SQL == "" && !pending {
		return PlanResult{}, ErrNoPlan
	}
	if in.Context != "" && in.Context != old.Context {
		return PlanResult{}, ErrForeignSession
	}
	parent, err := s.admissionForQuery(ctx, e, old)
	if err != nil {
		return PlanResult{}, err
	}
	if err := gatewayRequirement(e, "query.execute", parent.resources); err != nil {
		return PlanResult{}, err
	}
	if _, err := s.replayQueryClarifications(ctx, e, old); err != nil {
		return PlanResult{}, err
	}
	if len(in.Templates) > 0 && exec.Hash(in.Templates) != exec.Hash(old.Templates) {
		return PlanResult{}, ErrInvalid
	}
	question := refinementQuestion(old, in.QuestionRequest)
	retainCatalogSelection(old, &question)
	if err := applyReferenceEdits(&question, in.ReferenceEdits); err != nil {
		return PlanResult{}, err
	}
	if err := applyMetricEdits(&question, in.MetricEdits); err != nil {
		return PlanResult{}, err
	}
	retainSelectionOmissions(&question, in.ReferenceEdits)
	canonicalizeQuestion(&question)
	if err := validateMetricReferenceCoherence(question, in.ReferenceEdits, in.MetricEdits); err != nil {
		return PlanResult{}, err
	}
	if err := mergeRefinementClarifications(old, in.QuestionRequest, &question); err != nil {
		return PlanResult{}, err
	}
	if question.Question == "" {
		return PlanResult{}, ErrInvalid
	}
	// Answer edits cannot inherit old filters from protected SQL edit context.
	if len(in.Answers) == 0 && len(in.Choices) == 0 && old.SQL != "" {
		base := old.SQL
		if old.Clarification != nil {
			base = old.Clarification.BaseSQL
		}
		if base != "" {
			question.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "previous_sql", Text: base})
		}
	}
	return s.plan(ctx, e, question, "", in.QueryID, &old, "query.execute", old.Route.Resolutions...)
}

func (s *Service) checkRefinementDepth(ctx context.Context, e identity.Envelope, q QueryRecord) error {
	seen := map[string]bool{}
	for depth := 0; q.Parent != ""; depth++ {
		if depth >= MaxRefinementDepth-1 || seen[q.ID] {
			return ErrRefinementLimit
		}
		seen[q.ID] = true
		parent, err := s.repo.ReadQuery(ctx, mustScope(e), q.Parent)
		if err != nil {
			return err
		}
		if parent.Session != e.Session() || parent.Context != q.Context {
			return ErrForeignSession
		}
		q = parent
	}
	return nil
}

// Run revalidates and executes one previously planned query with idempotent operation handling.
func (s *Service) Run(ctx context.Context, e identity.Envelope, in RunRequest) (RunResult, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(in.QueryID) || !identity.Identifier(in.Operation) {
		return RunResult{}, ErrInvalid
	}
	if err := e.Has("query.execute"); !err {
		return RunResult{}, access.ErrForbidden
	}
	scopeValue, err := scope(e)
	if err != nil {
		return RunResult{}, err
	}
	var record QueryRecord
	if replay, replayErr := s.repo.ReadOperation(ctx, scopeValue, in.Operation); replayErr == nil {
		if replay.ID != in.QueryID {
			return RunResult{}, store.ErrConflict
		}
		if replay.Session != e.Session() {
			return RunResult{}, ErrForeignSession
		}
		// ReadOperation is only an idempotency index. Re-read the query under
		// the actor/session scope before deciding whether the terminal receipt
		// is replayable, so a stale index cannot bypass current metadata.
		record, err = s.repo.ReadQuery(ctx, scopeValue, replay.ID)
		if err != nil {
			return RunResult{}, err
		}
		if record.ID != in.QueryID || record.Session != e.Session() {
			return RunResult{}, ErrForeignSession
		}
		if record.Operation != in.Operation {
			return RunResult{}, store.ErrConflict
		}
		if terminalQueryStatus(record.Status) {
			// Terminal rows and SQL are retained evidence. Reauthorize every
			// exact topic/version, source, dataset and context dependency before
			// exposing the stored result, even when current publication state has
			// moved on.
			admitted, admissionErr := s.retainedAdmission(ctx, e, record)
			if admissionErr != nil {
				return RunResult{}, admissionErr
			}
			if admissionErr = gatewayRequirement(e, "query.execute", admitted.resources); admissionErr != nil {
				return RunResult{}, admissionErr
			}
			if admissionErr = s.verifyQueryClarificationBinding(ctx, e, record, admitted); admissionErr != nil {
				return RunResult{}, admissionErr
			}
			return s.runResult(record, exec.ExecutionReport{}, canInspect(e)), replayError(record.Status)
		}
	} else if !errors.Is(replayErr, store.ErrNotFound) {
		return RunResult{}, replayErr
	}
	if record.ID == "" {
		record, err = s.repo.ReadQuery(ctx, scopeValue, in.QueryID)
		if err != nil {
			return RunResult{}, err
		}
	}
	if record.Session != e.Session() || record.Status == "preflight" || record.SQL == "" {
		return RunResult{}, ErrNoPlan
	}
	record.Operation = in.Operation
	var current admission
	if record.EvidenceStale {
		// A rules publication/retirement invalidation makes the retained query
		// evidence stale, but it does not erase the immutable query pins. Replay
		// through the exact retained topic revisions and the same validator/
		// executor seams; never silently reinterpret it against the new current
		// publication.
		current, err = s.retainedAdmission(ctx, e, record)
	} else {
		current, err = s.currentAdmission(ctx, e, record)
	}
	if err != nil {
		return RunResult{}, err
	}
	if err := gatewayRequirement(e, "query.execute", current.resources); err != nil {
		return RunResult{}, err
	}
	if err := s.verifyQueryClarificationBinding(ctx, e, record, current); err != nil {
		return RunResult{}, err
	}
	plan, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: record.SQL, Parameters: record.Parameters}, current.relationScope)
	if err != nil {
		return RunResult{}, err
	}
	originalPlan := plan
	call, err := gateway.Authorize(e, "query.execute", executionPartition(record), current.resources...)
	if err != nil {
		return RunResult{}, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 2, Tokens: 1 << 20, Duration: 30 * time.Second})
	if err != nil {
		return RunResult{}, err
	}
	report, runErr := s.executor.Execute(ctx, e, plan, exec.Options{Operation: in.Operation, Number: 1, Preview: in.Preview, Rows: in.Rows, Bytes: in.Bytes})
	if errors.Is(runErr, exec.ErrReplay) {
		return s.waitForRun(ctx, e, record.ID, in.Operation)
	}
	if runErr != nil || report.Result == nil || report.Attempt.Status == "failed" {
		if !executionRepairable(report, runErr) {
			return s.finishRun(ctx, e, record, report, 0, runErr)
		}
		current.assembled, err = s.resealQueryContext(ctx, record, current)
		if err != nil {
			return RunResult{}, err
		}
		candidate, gen, correctionReceipt, genErr := s.fixCandidate(ctx, e, current, call, budget, record.SQL, executionErrorCode(report, runErr))
		record.Receipt = appendReceipts(record.Receipt, correctionReceipt)
		if genErr != nil {
			return s.finishRun(ctx, e, record, report, 1, errors.Join(ErrExecutionBudget, genErr))
		}
		candidatePlan, validateErr := s.validator.ValidateWithin(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: candidate.SQL, Parameters: candidate.Parameters}, current.relationScope)
		if validateErr != nil {
			return s.finishRun(ctx, e, record, report, 1, errors.Join(ErrExecutionBudget, validateErr))
		}
		if record.Clarification != nil && (candidate.SQL != record.SQL || !parametersEqual(candidate.Parameters, record.Parameters)) {
			return s.finishRun(ctx, e, record, report, 1, ErrUnsafeCorrection)
		}
		if !correctionEquivalent(record.SQL, record.Parameters, candidate, current.binding, originalPlan, candidatePlan) {
			return s.finishRun(ctx, e, record, report, 1, errors.Join(ErrExecutionBudget, ErrUnsafeCorrection))
		}
		plan = candidatePlan
		record.SQL, record.Parameters, record.Generation = candidate.SQL, candidate.Parameters, gen
		report, runErr = s.executor.Execute(ctx, e, plan, exec.Options{Operation: in.Operation, Number: 2, Preview: in.Preview, Rows: in.Rows, Bytes: in.Bytes})
		if runErr != nil || report.Result == nil || report.Attempt.Status == "failed" {
			return s.finishRun(ctx, e, record, report, 1, errors.Join(ErrExecutionBudget, runErr))
		}
		record.ExecutionFixes = 1
	}
	return s.finishRun(ctx, e, record, report, record.ExecutionFixes, runErr)
}

// waitForRun joins the winner of a concurrent first execution through the
// durable query operation record. It never reads another attempt's rows until
// the terminal result has been committed and reauthorized under current reach.
func (s *Service) waitForRun(ctx context.Context, e identity.Envelope, queryID, operation string) (RunResult, error) {
	delay := 5 * time.Millisecond
	for {
		if err := ctx.Err(); err != nil {
			return RunResult{}, err
		}
		record, err := s.repo.ReadOperation(ctx, mustScope(e), operation)
		if err != nil {
			return RunResult{}, err
		}
		if record.ID != queryID || record.Session != e.Session() || record.Operation != operation {
			return RunResult{}, ErrForeignSession
		}
		if terminalQueryStatus(record.Status) {
			admitted, admissionErr := s.retainedAdmission(ctx, e, record)
			if admissionErr != nil {
				return RunResult{}, admissionErr
			}
			if admissionErr = gatewayRequirement(e, "query.execute", admitted.resources); admissionErr != nil {
				return RunResult{}, admissionErr
			}
			if admissionErr = s.verifyQueryClarificationBinding(ctx, e, record, admitted); admissionErr != nil {
				return RunResult{}, admissionErr
			}
			return s.runResult(record, exec.ExecutionReport{}, canInspect(e)), replayError(record.Status)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return RunResult{}, ctx.Err()
		case <-timer.C:
		}
		if delay < 50*time.Millisecond {
			delay *= 2
			if delay > 50*time.Millisecond {
				delay = 50 * time.Millisecond
			}
		}
	}
}

func terminalQueryStatus(status string) bool {
	switch status {
	case "succeeded", "empty", "truncated", "failed", "uncertain", "cancelled", "timed_out", "interrupted":
		return true
	default:
		return false
	}
}

func replayError(status string) error {
	switch status {
	case "failed":
		return ErrExecutionFailed
	case "uncertain", "interrupted":
		return exec.ErrUncertain
	case "cancelled":
		return exec.ErrCancelled
	case "timed_out":
		return exec.ErrTimeout
	default:
		return nil
	}
}

// Feedback records a governed review and optionally validates a corrected query candidate.
func (s *Service) Feedback(ctx context.Context, e identity.Envelope, in FeedbackRequest) error {
	if ctx == nil || !e.Valid() || !identity.Identifier(in.QueryID) || (in.Verdict != "positive" && in.Verdict != "negative") || len(in.Note) > maxFeedbackNote || strings.ContainsRune(in.Note, 0) {
		return ErrInvalid
	}
	if !e.Has("feedback.write") {
		return access.ErrForbidden
	}
	sc, err := scope(e)
	if err != nil {
		return err
	}
	q, err := s.repo.ReadQuery(ctx, sc, in.QueryID)
	if err != nil {
		return err
	}
	if q.Session != e.Session() {
		return ErrForeignSession
	}
	if q.Status == "preflight" || q.SQL == "" {
		return ErrNoPlan
	}
	if len(q.TopicVersions) == 0 {
		return store.ErrMigration
	}
	admitted, admissionErr := s.currentAdmission(ctx, e, q)
	if admissionErr != nil {
		return admissionErr
	}
	if err = gatewayRequirement(e, "feedback.write", admitted.resources); err != nil {
		return err
	}
	currentBindingDigest := exec.Hash(admitted.binding)
	if q.Route.SourceBindingDigest != "" && q.Route.SourceBindingDigest != currentBindingDigest {
		return exec.ErrBinding
	}
	correction := strings.TrimSpace(in.Correction)
	sqlText := q.SQL
	if correction != "" {
		sqlText = correction
	}
	// Feedback may outlive the source pool that produced the plan. Validate the
	// exact stored or corrected SQL against the current binding before attaching
	// current-origin evidence; a retained binding digest, when present, must also
	// match exactly.
	if _, err = s.validator.ValidateWithin(ctx, e, exec.Request{Source: admitted.source, Context: admitted.context, SQL: sqlText, Parameters: q.Parameters}, admitted.relationScope); err != nil {
		return err
	}
	feedbackID := deterministicFeedbackID(e, q, in)
	feedback := FeedbackRecord{ID: feedbackID, QueryID: q.ID, Session: e.Session(), Verdict: in.Verdict, Correction: correction, Note: in.Note, Provenance: "phase18.feedback", Created: time.Now().UTC()}
	var example ExampleRecord
	if len(q.Route.Resolutions) == 0 {
		id, idErr := newID()
		if idErr != nil {
			return idErr
		}
		positive, negative := 0, 0
		if in.Verdict == "positive" || correction != "" {
			positive = 1
		} else {
			negative = 1
		}
		now := time.Now().UTC()
		example = ExampleRecord{
			ID: id, Topic: q.Topic, Question: q.Question, SQL: sqlText,
			Digest: exampleDigest(q.Topic, q.Question, sqlText), State: "candidate",
			Weight: evidenceScore(positive, negative), Uncertainty: evidenceUncertainty(positive, negative),
			EvidenceCount: 1, PositiveEvidence: positive, NegativeEvidence: negative, EvidenceOutcome: in.Verdict,
			Origin:  ExampleOrigin{SchemaVersion: 1, Locale: q.Locale, TopicVersion: q.TopicVersions[0], Context: q.Context, SourceBindingDigest: currentBindingDigest, RuleVersions: append([]string(nil), q.RuleVersions...), Templates: append([]rulesets.TemplateSelection(nil), q.Templates...)},
			Version: 1, Provenance: "feedback:" + feedbackID, Created: now, Updated: now,
		}
	}
	_, _, err = s.repo.ApplyFeedback(ctx, sc, feedback, example)
	return err
}

// ExampleState advances one retained learning example through its explicit review state.
func (s *Service) ExampleState(ctx context.Context, e identity.Envelope, in ExampleStateRequest) (ExampleRecord, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(in.ExampleID) || (in.State != "candidate" && in.State != "active" && in.State != "retired") || len(in.ReviewNote) > maxFeedbackNote || strings.ContainsRune(in.ReviewNote, 0) {
		return ExampleRecord{}, ErrInvalid
	}
	if !e.Has("feedback.write") {
		return ExampleRecord{}, access.ErrForbidden
	}
	// Require every action used by the selected review path before looking up an
	// example ID. Otherwise a caller lacking a later action could distinguish
	// existing IDs (forbidden) from missing IDs (not found).
	if !e.Has("sources.query") || in.State != "active" && (!e.Has("topics.read") || !e.Has("sources.read")) {
		return ExampleRecord{}, access.ErrForbidden
	}
	sc, err := scope(e)
	if err != nil {
		return ExampleRecord{}, err
	}
	example, err := s.repo.ReadExample(ctx, sc, in.ExampleID)
	if err != nil {
		return ExampleRecord{}, err
	}
	var q QueryRecord
	if in.State == "active" {
		verifier, ok := s.router.(originVerifier)
		if !ok {
			return ExampleRecord{}, store.ErrInvalid
		}
		route, routeErr := verifier.VerifyOrigin(ctx, e, nlqroute.RouteRequest{Topic: example.Topic, Context: example.Origin.Context, Locale: example.Origin.Locale, Question: example.Question, Templates: append([]rulesets.TemplateSelection(nil), example.Origin.Templates...)})
		if routeErr != nil {
			return ExampleRecord{}, routeErr
		}
		if route.Topic != example.Topic || len(route.Topics) == 0 || route.Context == nil || (route.Outcome != nlq.StrategySingleTopic && route.Outcome != nlq.StrategyMultiTopic) {
			return ExampleRecord{}, exec.ErrBinding
		}
		q = QueryRecord{Topic: route.Topic, Topics: append([]string(nil), route.Topics...), TopicVersions: append([]string(nil), route.TopicVersions...), Context: example.Origin.Context, Route: route}
	} else {
		q, err = s.learningQuery(ctx, e, example.Topic)
		if err != nil {
			return ExampleRecord{}, err
		}
	}
	var admitted admission
	if in.State == "active" {
		admitted, err = s.reviewAdmission(ctx, e, q)
	} else {
		admitted, err = s.currentAdmission(ctx, e, q)
	}
	if err != nil {
		return ExampleRecord{}, err
	}
	if err = gatewayRequirement(e, "feedback.write", admitted.resources); err != nil {
		return ExampleRecord{}, err
	}
	if in.State == "active" {
		candidate := example
		candidate.State = "active"
		if reason := exampleApplicabilityReason(candidate, admitted, exec.Hash(admitted.binding)); reason != "" {
			return ExampleRecord{}, exec.ErrBinding
		}
	}
	if in.ExpectedVersion != 0 && in.ExpectedVersion != example.Version {
		return ExampleRecord{}, store.ErrConflict
	}
	if in.State == "active" && (strings.TrimSpace(in.ReviewNote) == "" || example.PositiveEvidence < 1 || example.Weight < 0.60 || example.NegativeEvidence >= example.PositiveEvidence) {
		return ExampleRecord{}, store.ErrConflict
	}
	if in.State == "active" {
		if _, err := s.validator.ValidateWithin(ctx, e, exec.Request{Source: admitted.source, Context: admitted.context, SQL: example.SQL}, admitted.relationScope); err != nil {
			return ExampleRecord{}, err
		}
	}
	// Always CAS the row read above. The store repeats activation eligibility in
	// the same UPDATE so concurrent negative feedback either wins first and
	// blocks activation, or observes the completed activation afterwards.
	in.ExpectedVersion = example.Version
	updated, err := s.repo.SetExampleState(ctx, sc, in, e.User())
	if err != nil {
		return ExampleRecord{}, err
	}
	return redactExample(updated, canInspect(e)), nil
}

// Examples reads bounded DB-first learning examples for an authorized topic.
func (s *Service) Examples(ctx context.Context, e identity.Envelope, topic string, limit int) ([]ExampleRecord, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(topic) || limit < 1 || limit > maxExampleResults {
		return nil, ErrInvalid
	}
	if !e.Has("query.plan") && !e.Has("feedback.write") {
		return nil, access.ErrForbidden
	}
	action := "query.plan"
	if !e.Has(action) {
		action = "feedback.write"
	}
	if err := access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: topic}); err != nil {
		return nil, err
	}
	sc, err := scope(e)
	if err != nil {
		return nil, err
	}
	examples, err := s.repo.ListExamples(ctx, sc, topic, limit)
	if err != nil {
		return nil, err
	}
	if !canInspect(e) {
		for i := range examples {
			examples[i].SQL = ""
		}
	}
	return examples, nil
}

// ExportExamples returns a bounded protected migration bundle. Ordinary list
// operations continue to redact SQL without the separate inspection scope.
func (s *Service) ExportExamples(ctx context.Context, e identity.Envelope, in ExampleExportRequest) (ExampleBundle, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(in.Topic) || in.Limit < 1 || in.Limit > maxExampleResults {
		return ExampleBundle{}, ErrInvalid
	}
	if !e.Has("feedback.write") || !canInspect(e) {
		return ExampleBundle{}, access.ErrForbidden
	}
	anchor, err := s.learningQuery(ctx, e, in.Topic)
	if err != nil {
		return ExampleBundle{}, err
	}
	admitted, err := s.currentAdmission(ctx, e, anchor)
	if err != nil {
		return ExampleBundle{}, err
	}
	if err = gatewayRequirement(e, "feedback.write", admitted.resources); err != nil {
		return ExampleBundle{}, err
	}
	examples, err := s.repo.ListExamples(ctx, mustScope(e), in.Topic, in.Limit)
	if err != nil {
		return ExampleBundle{}, err
	}
	bundle := ExampleBundle{SchemaVersion: 1, Topic: in.Topic, Examples: make([]PortableExample, 0, len(examples))}
	for _, example := range examples {
		bundle.Examples = append(bundle.Examples, PortableExample{SchemaVersion: 1, Question: example.Question, SQL: example.SQL, Digest: example.Digest, Origin: example.Origin, PositiveEvidence: example.PositiveEvidence, NegativeEvidence: example.NegativeEvidence})
	}
	return bundle, nil
}

// ImportExample revalidates one neutral row against a freshly routed semantic
// environment and the native SQL validator. Imported evidence is always a
// candidate and requires a separate explicit review before activation.
func (s *Service) ImportExample(ctx context.Context, e identity.Envelope, in ExampleImportRequest) (ExampleRecord, error) {
	if ctx == nil || !e.Valid() || in.Example.SchemaVersion != 1 || !topics.DigestValid(in.Example.Digest) || in.Example.PositiveEvidence < 0 || in.Example.NegativeEvidence < 0 || in.Example.PositiveEvidence+in.Example.NegativeEvidence < 1 || in.Example.PositiveEvidence+in.Example.NegativeEvidence > 1000 {
		return ExampleRecord{}, ErrInvalid
	}
	if !e.Has("feedback.write") || !canInspect(e) {
		return ExampleRecord{}, access.ErrForbidden
	}
	if err := requireQuestionAction(e, "query.plan", in.Anchor); err != nil {
		return ExampleRecord{}, err
	}
	if err := validateQuestion(in.Anchor); err != nil {
		return ExampleRecord{}, err
	}
	admitted, err := s.admit(ctx, e, in.Anchor, true)
	if err != nil {
		return ExampleRecord{}, err
	}
	if admitted.route.Context == nil || admitted.route.Topic == "" {
		return ExampleRecord{}, nlqroute.ErrNoRoute
	}
	if reason := exampleApplicabilityReason(ExampleRecord{State: "active", Topic: admitted.route.Topic, Question: in.Example.Question, SQL: in.Example.SQL, Weight: 1, PositiveEvidence: 1, Origin: in.Example.Origin}, admitted, exec.Hash(admitted.binding)); reason != "" {
		return ExampleRecord{}, exec.ErrBinding
	}
	if in.Example.Digest != exampleDigest(admitted.route.Topic, in.Example.Question, in.Example.SQL) {
		return ExampleRecord{}, ErrInvalid
	}
	if _, err = s.validator.ValidateWithin(ctx, e, exec.Request{Source: admitted.source, Context: admitted.context, SQL: in.Example.SQL}, admitted.relationScope); err != nil {
		return ExampleRecord{}, err
	}
	id, err := newID()
	if err != nil {
		return ExampleRecord{}, err
	}
	positive, negative := in.Example.PositiveEvidence, in.Example.NegativeEvidence
	outcome := "negative"
	if positive > 0 {
		outcome = "positive"
	}
	now := time.Now().UTC()
	stored, _, err := s.repo.ImportExample(ctx, mustScope(e), ExampleRecord{ID: id, Topic: admitted.route.Topic, Question: in.Example.Question, SQL: in.Example.SQL, Digest: in.Example.Digest, State: "candidate", Weight: evidenceScore(positive, negative), Uncertainty: evidenceUncertainty(positive, negative), EvidenceCount: positive + negative, PositiveEvidence: positive, NegativeEvidence: negative, EvidenceOutcome: outcome, Origin: in.Example.Origin, Version: 1, Provenance: "neutral-import-v1", Created: now, Updated: now})
	if err != nil {
		return ExampleRecord{}, err
	}
	return redactExample(stored, true), nil
}

func (s *Service) plan(ctx context.Context, e identity.Envelope, question QuestionRequest, operation, parent string, observedParent *QueryRecord, action string, previous ...semantics.ClarificationResolution) (PlanResult, error) {
	if operation == "" || action != "query.plan" || ctx == nil || !e.Valid() || !identity.Identifier(operation) {
		return s.planFresh(ctx, e, question, operation, parent, observedParent, action, previous...)
	}
	var result PlanResult
	compose := func() error {
		record, err := s.repo.ReadOperation(ctx, mustScope(e), operation)
		if err == nil {
			result, err = reusablePlanResult(record, e, question, operation, parent)
			return err
		}
		if !errors.Is(err, store.ErrNotFound) {
			return err
		}
		result, err = s.planFresh(ctx, e, question, operation, parent, observedParent, action, previous...)
		return err
	}
	if locker, ok := s.repo.(PlanOperationLocker); ok {
		scope, err := scope(e)
		if err != nil {
			return PlanResult{}, err
		}
		if err := locker.WithPlanOperationLock(ctx, scope, operation, compose); err != nil {
			return PlanResult{}, err
		}
		return result, nil
	}
	if err := compose(); err != nil {
		return PlanResult{}, err
	}
	return result, nil
}

func reusablePlanResult(record QueryRecord, e identity.Envelope, question QuestionRequest, operation, parent string) (PlanResult, error) {
	if record.ID == "" || record.Operation != operation || record.Session != e.Session() || record.Parent != parent || record.Context != question.Context || record.Locale != question.Locale || record.Question != question.Question || len(question.EditBase)+len(question.Hints)+len(question.ExampleInput)+len(question.Default) != 0 {
		return PlanResult{}, store.ErrConflict
	}
	want, wantErr := json.Marshal(question.routeRequest())
	got, gotErr := json.Marshal(record.Route.Request)
	if wantErr != nil || gotErr != nil || string(want) != string(got) {
		return PlanResult{}, store.ErrConflict
	}
	// Idempotent lookup exposes only the opaque plan ID. Run performs the full
	// current-reach/source fence before executing or returning retained output.
	return PlanResult{QueryID: record.ID, SessionID: record.Session, Status: "planned"}, nil
}

func (s *Service) planFresh(ctx context.Context, e identity.Envelope, question QuestionRequest, operation, parent string, observedParent *QueryRecord, action string, previous ...semantics.ClarificationResolution) (PlanResult, error) {
	return s.planFreshWithActions(ctx, e, question, operation, parent, observedParent, action, previous, nil)
}

func (s *Service) planFreshWithActions(ctx context.Context, e identity.Envelope, question QuestionRequest, operation, parent string, observedParent *QueryRecord, action string, previous []semantics.ClarificationResolution, additionalActions []string) (PlanResult, error) {
	if ctx == nil || !e.Valid() {
		return PlanResult{}, access.ErrUnauthenticated
	}
	canonicalizeQuestion(&question)
	if err := requireQuestionAction(e, action, question); err != nil {
		return PlanResult{}, err
	}
	if err := validateQuestion(question); err != nil {
		return PlanResult{}, err
	}
	if parent != "" && observedParent == nil {
		value, readErr := s.repo.ReadQuery(ctx, mustScope(e), parent)
		if readErr != nil {
			return PlanResult{}, readErr
		}
		observedParent = &value
	}
	if observedParent != nil && (observedParent.ID != parent || observedParent.Session != e.Session() || observedParent.Context != question.Context) {
		return PlanResult{}, ErrForeignSession
	}
	admitted, err := s.admit(ctx, e, question, true)
	if err != nil {
		return PlanResult{}, err
	}
	for _, required := range additionalActions {
		if err := gatewayRequirement(e, required, admitted.resources); err != nil {
			return PlanResult{}, err
		}
	}
	question = redactClarificationInstructions(question, admitted.route)
	if admitted.route.Context == nil {
		if admitted.route.Clarification != nil {
			return PlanResult{}, admitted.route.Clarification
		}
		return PlanResult{}, nlqroute.ErrNoRoute
	}
	if err = s.ensureSession(ctx, e, question, admitted.route.Topics); err != nil {
		return PlanResult{}, err
	}
	if operation != "" && !identity.Identifier(operation) {
		return PlanResult{}, ErrInvalid
	}
	call, err := gateway.Authorize(e, action, generationPartition(admitted), admitted.resources...)
	if err != nil {
		return PlanResult{}, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 3, Tokens: 1 << 20, Duration: 30 * time.Second})
	if err != nil {
		return PlanResult{}, err
	}
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		return PlanResult{}, err
	}
	learned, selection, selectionReceipt, err := s.selectGenerationExamples(ctx, e, admitted, question, call, budget)
	if err != nil {
		return PlanResult{}, err
	}
	generation, err := assembler.ResolvePrecedence(ctx, nlq.GenerationInput{Context: admitted.assembled, EditBase: question.EditBase, Hints: question.Hints, Examples: append(learned, question.ExampleInput...), Default: question.Default})
	if err != nil {
		return PlanResult{}, err
	}
	selection.Usage = actualExampleUsage(selection, generation)
	candidate, fixes, receipt, validated, err := s.generateAndValidate(ctx, e, admitted, generation, call, budget, "")
	if err != nil {
		return PlanResult{}, err
	}
	id, err := newID()
	if err != nil {
		return PlanResult{}, err
	}
	if err := sealClarificationCandidate(&candidate, validated, previous, admitted.route.Resolutions); err != nil {
		return PlanResult{}, err
	}
	record := queryRecord(e, id, "planned", parent, question, admitted)
	bindParentLineage(&record, observedParent)
	record.Clarification = candidate.clarification
	receipt = appendReceipts(selectionReceipt, receipt)
	record.Operation, record.SQL, record.Parameters, record.Generation, record.Receipt, record.ValidationFixes, record.ExampleSelection = operation, candidate.SQL, candidate.Parameters, generation, receipt, fixes, selection
	if err = s.repo.CreateQuery(ctx, mustScope(e), record); err != nil {
		return PlanResult{}, err
	}
	out := PlanResult{Bindings: publicClarificationBinding(record.Clarification), AnswerChanges: publicClarificationChanges(record.Clarification), QueryID: id, SessionID: e.Session(), Status: "planned", Route: admitted.route, Confidence: admitted.route.Confidence, Generation: string(generation.Strategy), ValidationFixes: fixes, Assumptions: candidate.Assumptions, Ambiguities: candidate.Ambiguities, Receipt: receipt, validated: validated}
	if canInspect(e) {
		out.SQL = candidate.SQL
	}
	return out, nil
}

// ResolveOperationQueryID reads only a session-bound query ID for a durable
// plan operation. Run rechecks current signed reach and all source/topic pins
// before exposing any retained result or executing the source.
func (s *Service) ResolveOperationQueryID(ctx context.Context, e identity.Envelope, operation string) (string, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(operation) {
		return "", ErrInvalid
	}
	if err := e.Has("query.execute"); !err {
		return "", access.ErrForbidden
	}
	record, err := s.repo.ReadOperation(ctx, mustScope(e), operation)
	if err != nil {
		return "", err
	}
	if record.ID == "" || record.Operation != operation || record.Session != e.Session() {
		return "", ErrForeignSession
	}
	return record.ID, nil
}

func (s *Service) admit(ctx context.Context, e identity.Envelope, in QuestionRequest, withBinding bool) (admission, error) {
	route, err := s.router.Route(ctx, e, in.routeRequest())
	if err != nil {
		// Routing evaluates the request against the current publication. Its
		// only binding failure is a caller-supplied context that does not
		// match that publication, so expose the NLQ request error at this
		// boundary rather than leaking the execution seam's classification.
		if errors.Is(err, exec.ErrBinding) {
			return admission{}, ErrInvalid
		}
		return admission{}, err
	}
	// RouteResult is persisted as the semantic base for later refinement. The
	// request contains only bounded caller selections; authority and SQL remain
	// resolved from the current/retained domain seams below.
	// A current router has already classified and canonicalized its request.
	// Never replace that protected representation with raw answer strings.
	if route.Request.Question == "" && len(route.Resolutions) == 0 {
		route.Request = in.routeRequest()
	}
	assembled, err := route.GenerationContext()
	if err != nil {
		if route.Outcome == nlq.StrategyClarify || route.Outcome == nlq.StrategyNoRoute {
			return admission{route: route}, nil
		}
		return admission{route: route}, err
	}
	result := admission{route: route, assembled: assembled}
	for _, topicID := range route.Topics {
		contract, contractErr := s.topics.Contract(ctx, e, topicID)
		if contractErr != nil {
			return admission{}, contractErr
		}
		publication := contract.Publication
		if publication.State.Topic != topicID || publication.State.Archived || !publication.State.Active || publication.State.Version != routeVersion(route, topicID) {
			return admission{}, exec.ErrBinding
		}
		for _, dataset := range publication.Definition.Datasets {
			result.reviewed = append(result.reviewed, reviewedDataset{topic: topicID, dataset: dataset})
			if in.Context != dataset.Source.Context {
				// The publication/version was just confirmed against the route, so
				// a context that does not match it is a malformed caller request.
				// Retained-query checks below still use ErrBinding for source
				// rotation or other current-state drift.
				return admission{}, ErrInvalid
			}
			if result.source == "" {
				result.source, result.context = dataset.Source.Source, dataset.Source.Context
			} else if result.source != dataset.Source.Source || result.context != dataset.Source.Context {
				return admission{}, store.ErrConflict
			}
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: topicID})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: dataset.Source.Source})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset.ID})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: dataset.Source.Context})
		}
	}
	if result.source == "" || result.context == "" {
		return admission{}, store.ErrInvalid
	}
	if withBinding {
		result.binding, err = s.sources.Binding(ctx, e, result.source, result.context)
		if err != nil {
			return admission{}, err
		}
		if route.SourceBindingDigest != "" && route.SourceBindingDigest != exec.Hash(result.binding) {
			return admission{}, exec.ErrBinding
		}
		result.relationScope, result.relations, err = reviewedProjection(result.reviewed, result.binding)
		if err != nil {
			return admission{}, err
		}
		if !reflect.DeepEqual(assembled.Relations, result.relations) {
			return admission{}, exec.ErrBinding
		}
	}
	return result, nil
}

// admissionForQuery selects the same exact/current publication boundary used
// by execution. It is shared by refinement and terminal replay so neither can
// use the repository projection as a substitute for addressed-resource checks.
func (s *Service) admissionForQuery(ctx context.Context, e identity.Envelope, q QueryRecord) (admission, error) {
	if q.EvidenceStale {
		return s.retainedAdmission(ctx, e, q)
	}
	return s.currentAdmission(ctx, e, q)
}

func refinementQuestion(old QueryRecord, delta QuestionRequest) QuestionRequest {
	request := old.Route.Request
	base := QuestionRequest{
		Topic: request.Topic, Topics: append([]string(nil), request.Topics...), Context: request.Context, Locale: request.Locale,
		Question: request.Question, Templates: append([]rulesets.TemplateSelection(nil), request.Templates...), Kinds: append([]string(nil), request.Kinds...), LimitPerKind: request.LimitPerKind,
		References: append([]semantics.Reference(nil), request.References...), OmittedRoots: append([]semantics.Reference(nil), request.OmittedRoots...), Choices: append([]nlqroute.ChoiceSelection(nil), request.Choices...),
		Joins: append([]nlqroute.JoinChoice(nil), request.JoinChoices...), MetricIDs: append([]string(nil), request.MetricIDs...),
		Examples: append([]nlq.OptionalItem(nil), request.Examples...), Rerank: request.Rerank,
		InterpretationAnchor: request.InterpretationAnchor, InterpretationEdits: append([]nlqroute.InterpretationEdit(nil), request.InterpretationEdits...),
	}
	base.Topic = old.Topic
	base.Topics = append([]string(nil), old.Topics...)
	base.Context = old.Context
	base.Locale = old.Locale
	base.Question = old.Question
	base.Kinds = append([]string(nil), base.Kinds...)
	base.References = append([]semantics.Reference(nil), base.References...)
	base.Choices = append([]nlqroute.ChoiceSelection(nil), base.Choices...)
	base.MetricIDs = append([]string(nil), base.MetricIDs...)
	base.Examples = append([]nlq.OptionalItem(nil), base.Examples...)
	if delta.Question != "" {
		base.Question = delta.Question
	}
	base.Kinds = mergeStrings(base.Kinds, delta.Kinds)
	if delta.LimitPerKind != 0 {
		base.LimitPerKind = delta.LimitPerKind
	}
	base.References = mergeReferences(base.References, delta.References)
	base.OmittedRoots = mergeReferences(base.OmittedRoots, delta.OmittedRoots)
	base.Choices = mergeChoices(base.Choices, delta.Choices)
	base.Joins = mergeJoins(base.Joins, delta.Joins)
	base.MetricIDs = mergeStrings(base.MetricIDs, delta.MetricIDs)
	base.Examples = append(base.Examples, delta.Examples...)
	base.Rerank = base.Rerank || delta.Rerank
	if delta.InterpretationAnchor != "" {
		base.InterpretationAnchor = delta.InterpretationAnchor
	}
	base.InterpretationEdits = mergeInterpretationEdits(base.InterpretationEdits, delta.InterpretationEdits)
	base.EditBase = append(base.EditBase, delta.EditBase...)
	base.Hints = append(base.Hints, delta.Hints...)
	base.ExampleInput = append(base.ExampleInput, delta.ExampleInput...)
	base.Default = append(base.Default, delta.Default...)
	return base
}

func mergeStrings(base, delta []string) []string {
	out := append([]string(nil), base...)
	seen := make(map[string]bool, len(out))
	for _, value := range out {
		seen[value] = true
	}
	for _, value := range delta {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

func applyReferenceEdits(question *QuestionRequest, edits []ReferenceEdit) error {
	if question == nil || len(edits) > 64 {
		return ErrInvalid
	}
	values := append([]semantics.Reference(nil), question.References...)
	seen := map[semantics.Reference]bool{}
	for _, edit := range edits {
		if !edit.Target.Valid() || seen[edit.Target] {
			return ErrInvalid
		}
		seen[edit.Target] = true
		index := -1
		for i := range values {
			if values[i] == edit.Target {
				index = i
				break
			}
		}
		switch edit.Action {
		case "add":
			if edit.Replacement != nil || index >= 0 {
				return ErrInvalid
			}
			values = append(values, edit.Target)
		case "remove":
			if edit.Replacement != nil || index < 0 {
				return ErrInvalid
			}
			values = append(values[:index], values[index+1:]...)
		case "replace":
			if edit.Replacement == nil || !edit.Replacement.Valid() || index < 0 || *edit.Replacement == edit.Target {
				return ErrInvalid
			}
			for i := range values {
				if values[i] == *edit.Replacement {
					return ErrInvalid
				}
			}
			values[index] = *edit.Replacement
		default:
			return ErrInvalid
		}
	}
	question.References = values
	return nil
}

func applyMetricEdits(question *QuestionRequest, edits []MetricEdit) error {
	if question == nil || len(edits) > 32 {
		return ErrInvalid
	}
	values := append([]string(nil), question.MetricIDs...)
	seen := map[string]bool{}
	for _, edit := range edits {
		if !identity.Identifier(edit.Target) || seen[edit.Target] {
			return ErrInvalid
		}
		seen[edit.Target] = true
		index := -1
		for i := range values {
			if values[i] == edit.Target {
				index = i
				break
			}
		}
		switch edit.Action {
		case "add":
			if edit.Replacement != "" || index >= 0 {
				return ErrInvalid
			}
			values = append(values, edit.Target)
		case "remove":
			if edit.Replacement != "" || index < 0 {
				return ErrInvalid
			}
			values = append(values[:index], values[index+1:]...)
		case "replace":
			if !identity.Identifier(edit.Replacement) || edit.Replacement == edit.Target || index < 0 {
				return ErrInvalid
			}
			for i := range values {
				if values[i] == edit.Replacement {
					return ErrInvalid
				}
			}
			values[index] = edit.Replacement
		default:
			return ErrInvalid
		}
	}
	question.MetricIDs = values
	return nil
}

func canonicalizeQuestion(question *QuestionRequest) {
	if question == nil {
		return
	}
	question.References = append([]semantics.Reference(nil), question.References...)
	question.MetricIDs = append([]string(nil), question.MetricIDs...)
	question.OmittedRoots = append([]semantics.Reference(nil), question.OmittedRoots...)
	sort.Slice(question.OmittedRoots, func(i, j int) bool {
		return referenceKey(question.OmittedRoots[i]) < referenceKey(question.OmittedRoots[j])
	})
	sort.Slice(question.References, func(i, j int) bool {
		a, b := question.References[i], question.References[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.Dataset != b.Dataset {
			return a.Dataset < b.Dataset
		}
		if a.ID != b.ID {
			return a.ID < b.ID
		}
		return a.Revision < b.Revision
	})
	sort.Strings(question.MetricIDs)
}

func validateMetricReferenceCoherence(question QuestionRequest, referenceEdits []ReferenceEdit, metricEdits []MetricEdit) error {
	touched := len(metricEdits) > 0
	for _, edit := range referenceEdits {
		if edit.Target.Kind == semantics.KindMeasure || edit.Target.Kind == semantics.KindKPI || (edit.Replacement != nil && (edit.Replacement.Kind == semantics.KindMeasure || edit.Replacement.Kind == semantics.KindKPI)) {
			touched = true
		}
	}
	if !touched {
		return nil
	}
	refs := make([]string, 0, len(question.References))
	seen := map[string]bool{}
	for _, ref := range question.References {
		if ref.Kind != semantics.KindMeasure && ref.Kind != semantics.KindKPI {
			continue
		}
		if seen[ref.ID] {
			return ErrInvalid
		}
		seen[ref.ID] = true
		refs = append(refs, ref.ID)
	}
	sort.Strings(refs)
	if len(refs) != len(question.MetricIDs) {
		return ErrInvalid
	}
	for i := range refs {
		if refs[i] != question.MetricIDs[i] {
			return ErrInvalid
		}
	}
	return nil
}

// QueryLineageDigest binds a child to the exact protected parent state observed
// before any gateway work. Read-derived freshness markers are deliberately
// excluded because they are not persisted query mutations.
func QueryLineageDigest(q QueryRecord) string {
	// Saved-query reads intentionally redact result rows. Lineage binds only
	// protected metadata, so the digest must remain stable across that projection.
	q.Result = nil
	if q.EvidenceStale {
		errors := make([]string, 0, len(q.Errors))
		for _, code := range q.Errors {
			if code != "rule_evidence_stale" {
				errors = append(errors, code)
			}
		}
		q.Errors = errors
	}
	q.EvidenceStale = false
	return exec.Hash(struct {
		Record        QueryRecord            `json:"record"`
		Clarification *ClarificationEvidence `json:"clarification,omitempty"`
		SQL           string                 `json:"sql"`
		Parameters    []exec.Parameter       `json:"parameters"`
	}{Record: q, Clarification: q.Clarification, SQL: q.SQL, Parameters: q.Parameters})
}

func bindParentLineage(child *QueryRecord, parent *QueryRecord) {
	if child == nil || parent == nil {
		return
	}
	child.ParentRevision = parent.Revision
	child.ParentDigest = QueryLineageDigest(*parent)
}

func (s *Service) observeParent(ctx context.Context, e identity.Envelope, id, contextID string) (*QueryRecord, error) {
	if id == "" {
		return nil, nil
	}
	parent, err := s.repo.ReadQuery(ctx, mustScope(e), id)
	if err != nil {
		return nil, err
	}
	if parent.ID != id || parent.Session != e.Session() || parent.Context != contextID {
		return nil, ErrForeignSession
	}
	return &parent, nil
}

func mergeInterpretationEdits(base, delta []nlqroute.InterpretationEdit) []nlqroute.InterpretationEdit {
	out := append([]nlqroute.InterpretationEdit(nil), base...)
	for _, edit := range delta {
		replaced := false
		for i := range out {
			if out[i].Target == edit.Target {
				out[i], replaced = edit, true
				break
			}
		}
		if !replaced {
			out = append(out, edit)
		}
	}
	return out
}

func mergeReferences(base, delta []semantics.Reference) []semantics.Reference {
	out := append([]semantics.Reference(nil), base...)
	for _, value := range delta {
		found := false
		for _, existing := range out {
			if existing == value {
				found = true
				break
			}
		}
		if !found {
			out = append(out, value)
		}
	}
	return out
}

func mergeChoices(base, delta []nlqroute.ChoiceSelection) []nlqroute.ChoiceSelection {
	out := append([]nlqroute.ChoiceSelection(nil), base...)
	for _, value := range delta {
		key := value.Pattern + "\x00" + value.Slot
		for i := range out {
			if out[i].Pattern+"\x00"+out[i].Slot == key {
				out[i] = value
				key = ""
				break
			}
		}
		if key != "" {
			out = append(out, value)
		}
	}
	return out
}

func mergeJoins(base, delta []nlqroute.JoinChoice) []nlqroute.JoinChoice {
	out := append([]nlqroute.JoinChoice(nil), base...)
	for _, value := range delta {
		for i := range out {
			if out[i].Topic == value.Topic {
				out[i] = value
				value.Topic = ""
				break
			}
		}
		if value.Topic != "" {
			out = append(out, value)
		}
	}
	return out
}

func replaceInstruction(items []nlq.Instruction, replacement nlq.Instruction) []nlq.Instruction {
	out := make([]nlq.Instruction, 0, len(items)+1)
	for _, item := range items {
		if item.Key != replacement.Key {
			out = append(out, item)
		}
	}
	return append(out, replacement)
}

func (s *Service) learningQuery(ctx context.Context, e identity.Envelope, topic string) (QueryRecord, error) {
	if !identity.Identifier(topic) {
		return QueryRecord{}, store.ErrNotFound
	}
	contract, err := s.topics.Contract(ctx, e, topic)
	if err != nil {
		return QueryRecord{}, err
	}
	publication := contract.Publication
	if publication.State.Topic != topic || publication.State.Archived || !publication.State.Active || publication.State.Version == "" {
		return QueryRecord{}, exec.ErrBinding
	}
	if len(publication.Definition.Datasets) == 0 {
		return QueryRecord{}, exec.ErrBinding
	}
	contextID := publication.Definition.Datasets[0].Source.Context
	return QueryRecord{Topic: topic, Topics: []string{topic}, TopicVersions: []string{publication.State.Version}, Context: contextID}, nil
}

func redactExample(value ExampleRecord, inspect bool) ExampleRecord {
	if !inspect {
		value.SQL = ""
	}
	return value
}

func (s *Service) resealQueryContext(ctx context.Context, q QueryRecord, a admission) (nlq.AssembledContext, error) {
	if len(q.Topics) == 0 || len(q.Topics) != len(q.TopicVersions) {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	if q.Route.Context == nil || len(a.relations) == 0 || !reflect.DeepEqual(q.Route.Context.Relations, a.relations) || !reflect.DeepEqual(q.RelationScope, a.relationScope) {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	persisted := q.Generation.Context
	if persisted.Tier == "" || persisted.Question == "" {
		if q.Route.Context == nil {
			return nlq.AssembledContext{}, exec.ErrBinding
		}
		view := q.Route.Context
		persisted = nlq.AssembledContext{
			Tier: view.Tier, Budget: view.Budget, Tokens: view.Tokens, Locale: view.Locale, Strategy: view.Strategy,
			Topic: view.Topic, TopicVersion: view.TopicVersion, Topics: append([]nlq.TopicRevision(nil), view.Topics...),
			Question: view.Question, Relations: cloneRelations(view.Relations), Evidence: append([]nlq.Evidence(nil), view.Evidence...), Constraints: view.Constraints,
			Metrics: append([]nlq.PinnedMetric(nil), view.Metrics...), Advisory: append([]nlq.OptionalItem(nil), view.Advisory...), Examples: append([]nlq.OptionalItem(nil), view.Examples...),
		}
	}
	if !reflect.DeepEqual(persisted.Relations, a.relations) {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	if err := validateResealedRelations(persisted.Relations, q.Topics, a.binding); err != nil {
		return nlq.AssembledContext{}, err
	}
	if persisted.Tier == "" {
		var err error
		persisted.Tier, err = nlq.TierForConfidence(q.Route.Confidence)
		if err != nil {
			return nlq.AssembledContext{}, exec.ErrBinding
		}
	}
	if len(persisted.Topics) == 0 {
		if len(q.Topics) != 1 || persisted.Topic != q.Topics[0] || persisted.TopicVersion != q.TopicVersions[0] {
			return nlq.AssembledContext{}, exec.ErrBinding
		}
		persisted.Topics = []nlq.TopicRevision{{Topic: q.Topics[0], Version: q.TopicVersions[0]}}
	}
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		return nlq.AssembledContext{}, err
	}
	assembled, err := assembler.Assemble(ctx, nlq.ContextInput{
		Locale: persisted.Locale, Strategy: persisted.Strategy, Topic: persisted.Topic, TopicVersion: persisted.TopicVersion,
		Topics: append([]nlq.TopicRevision(nil), persisted.Topics...), Question: persisted.Question, Relations: cloneRelations(persisted.Relations),
		Evidence: append([]nlq.Evidence(nil), persisted.Evidence...), Constraints: persisted.Constraints,
		Metrics: append([]nlq.PinnedMetric(nil), persisted.Metrics...), Advisory: append([]nlq.OptionalItem(nil), persisted.Advisory...), Examples: append([]nlq.OptionalItem(nil), persisted.Examples...),
	}, persisted.Tier)
	if err != nil || assembled.Question != q.Question || assembled.Strategy != q.Route.Outcome {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	if len(q.Topics) != len(q.TopicVersions) {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	if len(q.Topics) > 0 && len(assembled.Topics) != len(q.Topics) {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	if len(q.Topics) == 1 && (assembled.Topic != q.Topics[0] || assembled.TopicVersion != q.TopicVersions[0]) {
		return nlq.AssembledContext{}, exec.ErrBinding
	}
	for i, topic := range q.Topics {
		if i >= len(assembled.Topics) || assembled.Topics[i].Topic != topic || assembled.Topics[i].Version != q.TopicVersions[i] {
			return nlq.AssembledContext{}, exec.ErrBinding
		}
	}
	return assembled, nil
}

func cloneRelations(in []nlq.SourceRelation) []nlq.SourceRelation {
	out := append([]nlq.SourceRelation(nil), in...)
	for i := range out {
		out[i].Columns = append([]string(nil), in[i].Columns...)
	}
	return out
}

// Repair may start from a durable, seal-free query record. Rebind physical
// names against the current source before passing them back to the model.
func validateResealedRelations(relations []nlq.SourceRelation, topics []string, binding exec.Binding) error {
	if len(relations) == 0 {
		return exec.ErrBinding
	}
	for _, relation := range relations {
		foundTopic := false
		for _, topic := range topics {
			foundTopic = foundTopic || relation.Topic == topic
		}
		if !foundTopic {
			return exec.ErrBinding
		}
		var physical *exec.Relation
		for i := range binding.Relations {
			if binding.Relations[i].ID == relation.Dataset {
				if physical != nil {
					return exec.ErrBinding
				}
				physical = &binding.Relations[i]
			}
		}
		if physical == nil || relation.Name != physical.Schema+"."+physical.Name {
			return exec.ErrBinding
		}
		for _, column := range relation.Columns {
			found := false
			for _, actual := range physical.Columns {
				if actual.Name == column && actual.Safe {
					found = true
					break
				}
			}
			if !found {
				return exec.ErrBinding
			}
		}
	}
	return nil
}

func (s *Service) currentAdmission(ctx context.Context, e identity.Envelope, q QueryRecord) (admission, error) {
	return s.admissionWith(ctx, e, q, s.topics.Contract, s.sources.Binding)
}

func (s *Service) reviewAdmission(ctx context.Context, e identity.Envelope, q QueryRecord) (admission, error) {
	topicsReader, ok := s.topics.(reviewTopicReader)
	if !ok {
		return admission{}, store.ErrInvalid
	}
	sourcesReader, ok := s.sources.(reviewSourceReader)
	if !ok {
		return admission{}, store.ErrInvalid
	}
	return s.admissionWith(ctx, e, q, topicsReader.ReviewContract, sourcesReader.ReviewBinding)
}

func (s *Service) admissionWith(ctx context.Context, e identity.Envelope, q QueryRecord, contract func(context.Context, identity.Envelope, string) (topics.Contract, error), binding func(context.Context, identity.Envelope, string, string) (exec.Binding, error)) (admission, error) {
	result := admission{route: q.Route}
	for i, topicID := range q.Topics {
		contractValue, err := contract(ctx, e, topicID)
		if err != nil {
			return admission{}, err
		}
		publication := contractValue.Publication
		if publication.State.Topic != topicID || publication.State.Archived || !publication.State.Active || i >= len(q.TopicVersions) || publication.State.Version != q.TopicVersions[i] {
			return admission{}, exec.ErrBinding
		}
		for _, dataset := range publication.Definition.Datasets {
			result.reviewed = append(result.reviewed, reviewedDataset{topic: topicID, dataset: dataset})
			if dataset.Source.Context != q.Context {
				return admission{}, exec.ErrBinding
			}
			if result.source == "" {
				result.source, result.context = dataset.Source.Source, dataset.Source.Context
			} else if result.source != dataset.Source.Source || result.context != dataset.Source.Context {
				return admission{}, store.ErrConflict
			}
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: topicID})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: dataset.Source.Source})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset.ID})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: dataset.Source.Context})
		}
	}
	if result.source == "" || result.context == "" {
		return admission{}, store.ErrInvalid
	}
	var err error
	result.binding, err = binding(ctx, e, result.source, result.context)
	if err != nil {
		return admission{}, err
	}
	result.relationScope, result.relations, err = reviewedProjection(result.reviewed, result.binding)
	if err != nil || q.ID != "" && (len(q.RelationScope) == 0 || !reflect.DeepEqual(q.RelationScope, result.relationScope)) {
		return admission{}, exec.ErrBinding
	}
	return result, nil
}

// retainedAdmission resolves the exact topic versions captured with a query.
// It is intentionally separate from currentAdmission: current reads require
// active pointers, while a query whose rule evidence was invalidated must
// replay only against the immutable retained definitions it originally used.
func (s *Service) retainedAdmission(ctx context.Context, e identity.Envelope, q QueryRecord) (admission, error) {
	if len(q.Topics) == 0 || len(q.Topics) != len(q.TopicVersions) {
		return admission{}, exec.ErrBinding
	}
	result := admission{route: q.Route}
	for i, topicID := range q.Topics {
		if !identity.Identifier(topicID) || !identity.Identifier(q.TopicVersions[i]) {
			return admission{}, exec.ErrBinding
		}
		contract, err := s.topics.RetainedContract(ctx, e, topicID, q.TopicVersions[i])
		if err != nil {
			return admission{}, err
		}
		publication := contract.Publication
		if publication.State.Topic != topicID || publication.State.Version != q.TopicVersions[i] || publication.Definition.Topic != topicID || publication.Definition.Version != q.TopicVersions[i] {
			return admission{}, exec.ErrBinding
		}
		for _, dataset := range publication.Definition.Datasets {
			result.reviewed = append(result.reviewed, reviewedDataset{topic: topicID, dataset: dataset})
			if dataset.Source.Context != q.Context {
				return admission{}, exec.ErrBinding
			}
			if result.source == "" {
				result.source, result.context = dataset.Source.Source, dataset.Source.Context
			} else if result.source != dataset.Source.Source || result.context != dataset.Source.Context {
				return admission{}, store.ErrConflict
			}
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: topicID})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: dataset.Source.Source})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset.ID})
			result.resources = appendResource(result.resources, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: dataset.Source.Context})
		}
	}
	if result.source == "" || result.context == "" {
		return admission{}, exec.ErrBinding
	}
	var err error
	result.binding, err = s.sources.Binding(ctx, e, result.source, result.context)
	if err != nil {
		return admission{}, err
	}
	result.relationScope, result.relations, err = reviewedProjection(result.reviewed, result.binding)
	if err != nil || len(q.RelationScope) == 0 || !reflect.DeepEqual(q.RelationScope, result.relationScope) {
		return admission{}, exec.ErrBinding
	}
	return result, nil
}

func (s *Service) ensureSession(ctx context.Context, e identity.Envelope, in QuestionRequest, topics []string) error {
	now := time.Now().UTC()
	record := SessionRecord{ID: e.Session(), Tenant: e.Tenant(), Actor: e.User(), Context: in.Context, Topics: append([]string(nil), topics...), Locale: in.Locale, Created: now, Updated: now}
	sc, err := scope(e)
	if err != nil {
		return err
	}
	if err = s.repo.CreateSession(ctx, sc, record); err == nil {
		return nil
	} else if !errors.Is(err, store.ErrConflict) {
		return err
	}
	existing, readErr := s.repo.ReadSession(ctx, sc, e.Session())
	if readErr != nil {
		return readErr
	}
	// The session anchors tenant/actor/context scope. Locale and topic sets are
	// per-question choices; every request still rechecks signed topic reach and
	// current publication admission before generation or execution.
	if existing.Context != in.Context || existing.Actor != e.User() || existing.Tenant != e.Tenant() {
		return store.ErrConflict
	}
	return nil
}

func (s *Service) generateAndValidate(ctx context.Context, e identity.Envelope, a admission, generation nlq.GenerationContext, call gateway.Call, budget *gateway.Budget, correction string) (generatedCandidate, int, gateway.Receipt, exec.Plan, error) {
	candidate, receipt, err := s.generate(ctx, e, a, generation, call, budget, "sqlgen", correction)
	if err != nil {
		return generatedCandidate{}, 0, receipt, exec.Plan{}, err
	}
	// Keep the model-authored statement separate from service-owned predicates.
	// Bound SQL and scalar answers must never become validation-repair input.
	unbound := candidate
	unbound.Parameters = append([]exec.Parameter(nil), candidate.Parameters...)
	candidate, err = bindClarificationCandidate(ctx, a, candidate)
	if err != nil {
		return generatedCandidate{}, 0, receipt, exec.Plan{}, err
	}
	plan, validateErr := s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: candidate.SQL, Parameters: candidate.Parameters}, a.relationScope)
	if validateErr == nil {
		return candidate, 0, receipt, plan, nil
	}
	if !validationRepairable(validateErr) {
		return generatedCandidate{}, 0, receipt, exec.Plan{}, validateErr
	}
	repairContext, repairErr := validationRepairContext(ctx, generation, unbound, validationCode(validateErr, unbound.SQL))
	if repairErr != nil {
		return generatedCandidate{}, 0, receipt, exec.Plan{}, errors.Join(ErrValidationBudget, repairErr)
	}
	fixed, fixedReceipt, fixErr := s.generate(ctx, e, a, repairContext, call, budget, "sqlfix", "")
	receipt = appendReceipts(receipt, fixedReceipt)
	if fixErr != nil {
		return generatedCandidate{}, 1, receipt, exec.Plan{}, errors.Join(ErrValidationBudget, fixErr)
	}
	fixed, fixErr = restoreValidationRepairParameters(fixed, unbound.Parameters)
	if fixErr != nil {
		return generatedCandidate{}, 1, receipt, exec.Plan{}, fixErr
	}
	fixed, fixErr = bindClarificationCandidate(ctx, a, fixed)
	if fixErr != nil {
		return generatedCandidate{}, 1, receipt, exec.Plan{}, fixErr
	}
	plan, validateErr = s.validator.ValidateWithin(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: fixed.SQL, Parameters: fixed.Parameters}, a.relationScope)
	if validateErr != nil {
		return generatedCandidate{}, 1, receipt, exec.Plan{}, errors.Join(ErrValidationBudget, validateErr)
	}
	return fixed, 1, receipt, plan, nil
}

func (s *Service) generate(ctx context.Context, e identity.Envelope, a admission, generation nlq.GenerationContext, call gateway.Call, budget *gateway.Budget, role, correction string) (generatedCandidate, gateway.Receipt, error) {
	if generationSchemaErr != nil {
		return generatedCandidate{}, gateway.Receipt{}, generationSchemaErr
	}
	dialect := a.binding.Dialect
	system := "Return one safe, read-only SQL statement for the native " + dialect + " dialect. Never change the topic, source, execution context, required filters, pinned metrics, or permissions. Return only the requested JSON object."
	if hasActiveBusinessEvidence(a.route) {
		system += " Reviewed clarification constraints are bound by the service after generation. Select their exact governed base relations; do not invent, repeat, or infer their scalar values or add predicates for those owned targets."
	}
	prompt := generation.Prompt + "\ndialect:" + dialect + "\nsource_context:" + a.context
	if correction != "" {
		prompt += "\ncorrection_reason:" + correction
	}
	generated, err := s.engine.Generate(ctx, call, budget, role, system, prompt, generationSchema)
	if err != nil {
		return generatedCandidate{}, generated.Receipt, err
	}
	var candidate generatedCandidate
	if json.Unmarshal(generated.JSON, &candidate) != nil || !validCandidate(candidate) {
		return generatedCandidate{}, generated.Receipt, ErrGeneration
	}
	return candidate, generated.Receipt, nil
}

func (s *Service) fixCandidate(ctx context.Context, e identity.Envelope, a admission, call gateway.Call, budget *gateway.Budget, oldSQL, reason string) (generatedCandidate, nlq.GenerationContext, gateway.Receipt, error) {
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		return generatedCandidate{}, nlq.GenerationContext{}, gateway.Receipt{}, err
	}
	generation, err := assembler.ResolvePrecedence(ctx, nlq.GenerationInput{Context: a.assembled, EditBase: []nlq.Instruction{{Key: "previous_sql", Text: oldSQL}}, Default: []nlq.Instruction{{Key: "repair", Text: "Preserve all required filters, metric pins, source, context, and permissions while correcting the statement."}}})
	if err != nil {
		return generatedCandidate{}, nlq.GenerationContext{}, gateway.Receipt{}, err
	}
	candidate, receipt, err := s.generate(ctx, e, a, generation, call, budget, "sqlfix", reason)
	if err != nil {
		return generatedCandidate{}, generation, receipt, err
	}
	return candidate, generation, receipt, nil
}

func (s *Service) finishRun(ctx context.Context, e identity.Envelope, q QueryRecord, report exec.ExecutionReport, fixes int, runErr error) (RunResult, error) {
	if report.Result != nil {
		q.Result = report.Result
	}
	if report.Attempt.Status != "" {
		q.Status = report.Attempt.Status
	}
	if runErr != nil || q.Status == "failed" || q.Status == "uncertain" || q.Status == "cancelled" || q.Status == "timed_out" {
		q.Errors = []string{executionErrorCode(report, runErr)}
		if runErr == nil {
			switch q.Status {
			case "uncertain":
				runErr = exec.ErrUncertain
			case "cancelled":
				runErr = exec.ErrCancelled
			case "timed_out":
				runErr = exec.ErrTimeout
			default:
				runErr = ErrExecutionFailed
			}
		}
	}
	q.ExecutionFixes = fixes
	if report.Attempt.Manifest.Operation != "" {
		q.Operation = report.Attempt.Manifest.Operation
	}
	if q.Operation == "" {
		q.Operation = q.ID + ":run"
	}
	q.Revision++
	if err := s.repo.UpdateQuery(ctx, mustScope(e), q, q.Revision-1); err != nil {
		return RunResult{}, err
	}
	out := s.runResult(q, report, canInspect(e))
	if runErr != nil {
		return out, runErr
	}
	return out, nil
}

func (s *Service) runResult(q QueryRecord, report exec.ExecutionReport, inspect bool) RunResult {
	out := RunResult{Bindings: publicClarificationBinding(q.Clarification), AnswerChanges: publicClarificationChanges(q.Clarification), QueryID: q.ID, SessionID: q.Session, Status: q.Status, EvidenceStale: q.EvidenceStale, Route: q.Route, Confidence: q.Route.Confidence, Assumptions: append([]string(nil), q.Assumptions...), Ambiguities: append([]string(nil), q.Ambiguities...), ValidationFixes: q.ValidationFixes, ExecutionFixes: q.ExecutionFixes, Execution: report, Receipt: q.Receipt}
	if q.Result != nil && out.Execution.Result == nil {
		out.Execution.Result = q.Result
	}
	if inspect {
		out.SQL = q.SQL
	}
	return out
}

func (s *Service) selectLearnedInstructions(ctx context.Context, e identity.Envelope, admitted admission, question string, rerank bool, call gateway.Call, budget *gateway.Budget) ([]nlq.Instruction, ExampleSelectionEvidence, gateway.Receipt, error) {
	evidence := ExampleSelectionEvidence{SchemaVersion: 1, PolicyVersion: "bayes-jaccard-v1"}
	sc, err := scope(e)
	if err != nil {
		return nil, evidence, gateway.Receipt{}, err
	}
	examples, err := s.repo.ListExamples(ctx, sc, admitted.route.Topic, maxLearningCandidates)
	if err != nil {
		return nil, evidence, gateway.Receipt{}, err
	}
	type applicable struct {
		record     ExampleRecord
		similarity float64
		rankScore  *float64
	}
	candidates := make([]applicable, 0, len(examples))
	bindingDigest := exec.Hash(admitted.binding)
	for _, example := range examples {
		selection := ExampleSelection{ExampleID: example.ID, Version: example.Version, Lane: "examples", Score: example.Weight, Uncertainty: example.Uncertainty}
		if reason := exampleApplicabilityReason(example, admitted, bindingDigest); reason != "" {
			selection.Decision, selection.Reason = "excluded", reason
			evidence.Excluded = append(evidence.Excluded, selection)
			continue
		}
		candidates = append(candidates, applicable{record: example, similarity: lexicalSimilarity(question, example.Question)})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i].similarity*0.7 + candidates[i].record.Weight*0.3
		right := candidates[j].similarity*0.7 + candidates[j].record.Weight*0.3
		if left != right {
			return left > right
		}
		return candidates[i].record.ID < candidates[j].record.ID
	})
	for i, candidate := range candidates {
		evidence.ShadowBaseline = append(evidence.ShadowBaseline, ExampleSelection{ExampleID: candidate.record.ID, Version: candidate.record.Version, Lane: "examples", Position: i + 1, Decision: "baseline", Score: candidate.record.Weight, Uncertainty: candidate.record.Uncertainty})
	}
	var receipt gateway.Receipt
	if rerank && len(candidates) > 1 {
		items := make([]gateway.Candidate, len(candidates))
		for i := range candidates {
			items[i] = gateway.Candidate{ID: candidates[i].record.ID, Text: candidates[i].record.Question, Resource: access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: admitted.route.Topic}}
		}
		sealed, sealErr := gateway.AdmitCandidates(call, "query.plan", items)
		if sealErr != nil {
			return nil, evidence, receipt, sealErr
		}
		ranked, rankErr := s.engine.Rerank(ctx, call, budget, question, sealed)
		if rankErr != nil {
			return nil, evidence, ranked.Receipt, rankErr
		}
		receipt = ranked.Receipt
		byID := make(map[string]applicable, len(candidates))
		for _, candidate := range candidates {
			byID[candidate.record.ID] = candidate
		}
		seen := make(map[string]bool, len(candidates))
		reordered := make([]applicable, 0, len(candidates))
		for _, item := range ranked.Items {
			if item.Score != nil && (math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0)) {
				return nil, evidence, receipt, gateway.ErrOutput
			}
			candidate, ok := byID[item.ID]
			if !ok || seen[item.ID] {
				return nil, evidence, receipt, gateway.ErrOutput
			}
			seen[item.ID] = true
			candidate.rankScore = item.Score
			reordered = append(reordered, candidate)
		}
		if len(reordered) != len(candidates) {
			return nil, evidence, receipt, gateway.ErrOutput
		}
		candidates = reordered
	}
	if len(candidates) > nlq.MaxExamples {
		for _, candidate := range candidates[nlq.MaxExamples:] {
			evidence.Excluded = append(evidence.Excluded, ExampleSelection{ExampleID: candidate.record.ID, Version: candidate.record.Version, Lane: "examples", Decision: "excluded", Reason: "candidate_limit", Score: candidate.record.Weight, Uncertainty: candidate.record.Uncertainty, RankScore: candidate.rankScore})
		}
		candidates = candidates[:nlq.MaxExamples]
	}
	result := make([]nlq.Instruction, 0, len(candidates))
	for i, candidate := range candidates {
		record := candidate.record
		evidence.Selected = append(evidence.Selected, ExampleSelection{ExampleID: record.ID, Version: record.Version, Lane: "examples", Position: i + 1, Decision: "selected", Score: record.Weight, Uncertainty: record.Uncertainty, RankScore: candidate.rankScore})
		result = append(result, nlq.Instruction{Key: "learned-" + record.ID, Text: "question:" + record.Question + " sql:" + record.SQL})
	}
	evidence.Receipt = receipt
	return result, evidence, receipt, nil
}

func deterministicFeedbackID(e identity.Envelope, q QueryRecord, in FeedbackRequest) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{e.Tenant(), e.User(), e.Session(), q.ID, in.Verdict, strings.TrimSpace(in.Correction), in.Note}, "\x00")))
	return hex.EncodeToString(sum[:16])
}

func evidenceScore(positive, negative int) float64 {
	return float64(positive+1) / float64(positive+negative+2)
}

func evidenceUncertainty(positive, negative int) float64 {
	return 1 / math.Sqrt(float64(positive+negative+2))
}

func exampleApplicabilityReason(example ExampleRecord, admitted admission, bindingDigest string) string {
	if example.State != "active" {
		return "not_active"
	}
	if example.SQL == "" || example.Question == "" {
		return "invalid_content"
	}
	if example.Origin.SchemaVersion != 1 {
		return "unsupported_origin"
	}
	if example.Origin.Context != admitted.context {
		return "context_changed"
	}
	if admitted.route.Context != nil && example.Origin.Locale != admitted.route.Context.Locale {
		return "locale_changed"
	}
	if example.Origin.SourceBindingDigest != bindingDigest {
		return "source_changed"
	}
	if example.Origin.TopicVersion != routeVersion(admitted.route, example.Topic) {
		return "topic_changed"
	}
	if exec.Hash(example.Origin.RuleVersions) != exec.Hash(admitted.route.RuleVersions) {
		return "rules_changed"
	}
	// An absent selection has the same meaning whether JSON decoded it as nil
	// or the router returned an allocated empty slice.
	if len(example.Origin.Templates)+len(admitted.route.Templates) > 0 && exec.Hash(example.Origin.Templates) != exec.Hash(admitted.route.Templates) {
		return "templates_changed"
	}
	if example.PositiveEvidence < 1 || example.Weight < 0.60 || example.NegativeEvidence >= example.PositiveEvidence {
		return "insufficient_evidence"
	}
	return ""
}

func lexicalSimilarity(left, right string) float64 {
	a, b := tokenSet(left), tokenSet(right)
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	intersection := 0
	for token := range a {
		if b[token] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(a)+len(b)-intersection)
}

func tokenSet(value string) map[string]bool {
	result := map[string]bool{}
	for _, token := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r < 0x80
	}) {
		if token != "" {
			result[token] = true
		}
	}
	return result
}

func queryRecord(e identity.Envelope, id, status, parent string, in QuestionRequest, a admission) QueryRecord {
	return QueryRecord{ID: id, Session: e.Session(), Parent: parent, Topic: a.route.Topic, Topics: append([]string(nil), a.route.Topics...), TopicVersions: append([]string(nil), a.route.TopicVersions...), RuleVersions: append([]string(nil), a.route.RuleVersions...), Templates: append([]rulesets.TemplateSelection(nil), a.route.Templates...), Context: in.Context, Locale: in.Locale, Question: a.route.Request.Question, Route: a.route, RelationScope: cloneRelationScope(a.relationScope), Status: status, Assumptions: assumptions(a.route), Ambiguities: ambiguities(a.route), Created: time.Now().UTC(), Updated: time.Now().UTC(), Revision: 1}
}

func (r QuestionRequest) routeRequest() nlqroute.RouteRequest {
	return nlqroute.RouteRequest{Answers: semantics.CloneClarificationAnswers(r.Answers), AnswerContext: r.AnswerContext, Topic: r.Topic, Topics: append([]string(nil), r.Topics...), Context: r.Context, Locale: r.Locale, Question: r.Question, Templates: append([]rulesets.TemplateSelection(nil), r.Templates...), Kinds: append([]string(nil), r.Kinds...), LimitPerKind: r.LimitPerKind, References: append([]semantics.Reference(nil), r.References...), OmittedRoots: append([]semantics.Reference(nil), r.OmittedRoots...), Choices: append([]nlqroute.ChoiceSelection(nil), r.Choices...), JoinChoices: append([]nlqroute.JoinChoice(nil), r.Joins...), MetricIDs: append([]string(nil), r.MetricIDs...), Examples: cloneRouteExamples(r.Examples), Rerank: r.Rerank, InterpretationAnchor: r.InterpretationAnchor, InterpretationEdits: append([]nlqroute.InterpretationEdit(nil), r.InterpretationEdits...)}
}

func cloneRouteExamples(items []nlq.OptionalItem) []nlq.OptionalItem {
	out := append([]nlq.OptionalItem(nil), items...)
	for i := range out {
		if out[i].Confidence == nil {
			continue
		}
		confidence := *out[i].Confidence
		out[i].Confidence = &confidence
	}
	return out
}

func validateQuestion(in QuestionRequest) error {
	if !identity.Identifier(in.Context) || (in.Locale != nlq.LanguageEnglish && in.Locale != nlq.LanguageSpanish) || len(in.Question) == 0 || len(in.Question) > 16<<10 || strings.TrimSpace(in.Question) != in.Question || strings.ContainsAny(in.Question, "\x00\r\n") {
		return ErrInvalid
	}
	if !instructionValid(in.EditBase) || !instructionValid(in.Hints) || !instructionValid(in.ExampleInput) || !instructionValid(in.Default) {
		return ErrInvalid
	}
	return nil
}

func requireQuestionAction(e identity.Envelope, action string, in QuestionRequest) error {
	if !e.Has(action) {
		return access.ErrForbidden
	}
	ids := append([]string(nil), in.Topics...)
	if in.Topic != "" {
		ids = append(ids, in.Topic)
	}
	if len(ids) == 0 {
		return nil
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if !identity.Identifier(id) || seen[id] {
			continue
		}
		seen[id] = true
		if err := access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: id}); err != nil {
			return err
		}
	}
	return nil
}

func gatewayRequirement(e identity.Envelope, action string, resources []access.Resource) error {
	return access.Require(e, action, resources...)
}

func appendResource(out []access.Resource, r access.Resource) []access.Resource {
	for _, existing := range out {
		if existing == r {
			return out
		}
	}
	return append(out, r)
}

func routeVersion(route nlqroute.RouteResult, topic string) string {
	for i, id := range route.Topics {
		if id == topic && i < len(route.TopicVersions) {
			return route.TopicVersions[i]
		}
	}
	return ""
}

func generationPartition(a admission) string {
	return "nlq:plan:" + exec.Hash([]string{a.source, a.context, a.route.Topic, routeDigest(a.route)})
}
func executionPartition(q QueryRecord) string {
	return "nlq:run:" + exec.Hash([]string{q.ID, q.Context, q.Topic})
}

func validCandidate(c generatedCandidate) bool {
	if len(c.SQL) == 0 || len(c.SQL) > maxSQLBytes || strings.ContainsRune(c.SQL, 0) || strings.TrimSpace(c.SQL) != c.SQL || len(c.Parameters) > 64 || len(c.Assumptions) > 32 || len(c.Ambiguities) > 32 {
		return false
	}
	for _, p := range c.Parameters {
		if !p.Valid() {
			return false
		}
	}
	for _, text := range append(append([]string{}, c.Assumptions...), c.Ambiguities...) {
		if len(text) > 1024 || strings.ContainsRune(text, 0) {
			return false
		}
	}
	return true
}

func validationCode(err error, sql string) string {
	if err == nil {
		return ""
	}
	_ = sql
	switch {
	case errors.Is(err, exec.ErrUnsafe):
		return "validation_unsafe"
	case errors.Is(err, exec.ErrLimit):
		return "validation_limit"
	case errors.Is(err, exec.ErrUnsupported):
		return "validation_unsupported"
	default:
		return "validation_failed"
	}
}

func executionRepairable(report exec.ExecutionReport, err error) bool {
	if report.Attempt.Status == "uncertain" || report.Attempt.Status == "cancelled" || report.Attempt.Status == "timed_out" || errors.Is(err, exec.ErrUncertain) || errors.Is(err, exec.ErrCancelled) || errors.Is(err, exec.ErrTimeout) || errors.Is(err, exec.ErrBinding) || errors.Is(err, exec.ErrLimit) || errors.Is(err, exec.ErrType) || errors.Is(err, exec.ErrUnsupported) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, store.ErrUnavailable) || errors.Is(err, store.ErrConflict) {
		return false
	}
	// The source boundary may explicitly classify a durable, retryable query
	// rejection without exposing provider text. Unknown transport or journal
	// failures stay terminal; only this registered code can spend the one
	// execution-correction budget.
	return report.Attempt.Status == "failed" && report.Attempt.Code == "query_error" && errors.Is(err, exec.ErrQuery)
}

func executionErrorCode(report exec.ExecutionReport, err error) string {
	if report.Attempt.Code != "" {
		return report.Attempt.Code
	}
	if errors.Is(err, exec.ErrBinding) {
		return "context_changed"
	}
	if errors.Is(err, exec.ErrLimit) {
		return "limit_exceeded"
	}
	return "execution_failed"
}

func appendReceipts(a, b gateway.Receipt) gateway.Receipt {
	a.Calls = append(a.Calls, b.Calls...)
	if b.Warning != "" {
		a.Warning = b.Warning
	}
	return a
}

// correctionEquivalent applies the narrow proof available to execution
// repair. Native validation proves that a candidate is safe to read, but it
// does not prove that the candidate still answers the governed question. The
// PostgreSQL parse tree therefore has to retain every projection, filter,
// relation, join, grouping, limit and expression, while parameter bindings
// and validated plan coordinates must remain identical. Parse locations are
// formatting metadata and are removed before comparing the trees.
func correctionEquivalent(oldSQL string, oldParameters []exec.Parameter, candidate generatedCandidate, binding exec.Binding, oldPlan, candidatePlan exec.Plan) bool {
	if !sameParameters(oldParameters, candidate.Parameters) {
		return false
	}
	oldReceipt, candidateReceipt := oldPlan.Receipt(), candidatePlan.Receipt()
	if oldReceipt.Validated != candidateReceipt.Validated {
		return false
	}
	if oldReceipt.Validated && (oldReceipt.Source != candidateReceipt.Source || oldReceipt.Context != candidateReceipt.Context || oldReceipt.Dialect != candidateReceipt.Dialect || oldReceipt.Contract != candidateReceipt.Contract || !sameStrings(oldReceipt.Dependencies, candidateReceipt.Dependencies) || !sameStrings(oldReceipt.Columns, candidateReceipt.Columns)) {
		return false
	}
	if oldSQL == candidate.SQL {
		return true
	}
	if binding.Dialect != "postgres" {
		return false
	}
	oldTree, ok := normalizedPostgresTree(oldSQL)
	if !ok {
		return false
	}
	candidateTree, ok := normalizedPostgresTree(candidate.SQL)
	return ok && oldTree == candidateTree
}

func sameParameters(a, b []exec.Parameter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizedPostgresTree(sql string) (string, bool) {
	raw, err := pgquery.ParseToJSON(sql)
	if err != nil {
		return "", false
	}
	var tree any
	if err := json.Unmarshal([]byte(raw), &tree); err != nil {
		return "", false
	}
	normalized, err := json.Marshal(normalizePostgresTree(tree))
	if err != nil {
		return "", false
	}
	return string(normalized), true
}

func normalizePostgresTree(value any) any {
	switch value := value.(type) {
	case []any:
		out := make([]any, len(value))
		for i, item := range value {
			out[i] = normalizePostgresTree(item)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(value))
		for key, item := range value {
			switch key {
			case "location", "stmt_location", "stmt_len":
				continue
			}
			out[key] = normalizePostgresTree(item)
		}
		return out
	default:
		return value
	}
}

func assumptions(route nlqroute.RouteResult) []string {
	if route.Context == nil {
		return nil
	}
	return []string{"current topic publication and source context admitted before generation"}
}
func ambiguities(route nlqroute.RouteResult) []string {
	if route.Clarification == nil {
		return nil
	}
	return []string{route.Clarification.Reason}
}

func exampleDigest(topic, question, sql string) string {
	h := sha256.Sum256([]byte(topic + "\x00" + question + "\x00" + sql))
	return hex.EncodeToString(h[:])
}

func mustScope(e identity.Envelope) store.Scope {
	s, _ := scope(e)
	return s
}

// routeDigest creates a stable routing identity without exposing prompt or
// evidence text in an operation partition.
func routeDigest(r nlqroute.RouteResult) string {
	return r.Topic + ":" + strings.Join(r.Topics, ",") + ":" + strings.Join(r.TopicVersions, ",")
}
