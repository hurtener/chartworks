package nlqexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/store"
	pgquery "github.com/wasilibs/go-pgquery"
)

type admission struct {
	route     nlqroute.RouteResult
	assembled nlq.AssembledContext
	binding   exec.Binding
	source    string
	context   string
	resources []access.Resource
}

type generatedCandidate struct {
	SQL         string           `json:"sql"`
	Parameters  []exec.Parameter `json:"parameters"`
	Assumptions []string         `json:"assumptions"`
	Ambiguities []string         `json:"ambiguities"`
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
	if err := requireQuestionAction(e, "query.preflight", in.QuestionRequest); err != nil {
		return PreflightResult{}, err
	}
	if err := validateQuestion(in.QuestionRequest); err != nil {
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
	record := queryRecord(e, id, "preflight", "", in.QuestionRequest, admitted)
	if err = s.repo.CreateQuery(ctx, mustScope(e), record); err != nil {
		return PreflightResult{}, err
	}
	return PreflightResult{QueryID: id, SessionID: e.Session(), Route: admitted.route, Confidence: admitted.route.Confidence, Assumptions: assumptions(admitted.route), Ambiguities: ambiguities(admitted.route)}, nil
}

// Plan generates one bounded candidate and validates it through the existing read core.
func (s *Service) Plan(ctx context.Context, e identity.Envelope, in PlanRequest) (PlanResult, error) {
	return s.plan(ctx, e, in.QuestionRequest, in.Operation, "", "query.plan")
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
	if old.Session != e.Session() || old.Status == "preflight" {
		return PlanResult{}, ErrForeignSession
	}
	if old.SQL == "" {
		return PlanResult{}, ErrNoPlan
	}
	parent, err := s.admissionForQuery(ctx, e, old)
	if err != nil {
		return PlanResult{}, err
	}
	if err := gatewayRequirement(e, "query.execute", parent.resources); err != nil {
		return PlanResult{}, err
	}
	question := refinementQuestion(old, in.QuestionRequest)
	if question.Question == "" {
		return PlanResult{}, ErrInvalid
	}
	// The parent SQL is a protected generation base. It is injected only into
	// the in-process edit lane; the request/response types never carry it.
	question.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "previous_sql", Text: old.SQL})
	return s.plan(ctx, e, question, "", in.QueryID, "query.execute")
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
	plan, err := s.validator.Validate(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: record.SQL, Parameters: record.Parameters})
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
	if runErr != nil || report.Result == nil || report.Attempt.Status == "failed" {
		if !executionRepairable(report, runErr) {
			return s.finishRun(ctx, e, record, report, 0, runErr)
		}
		current.assembled, err = s.resealQueryContext(ctx, record)
		if err != nil {
			return RunResult{}, err
		}
		candidate, gen, correctionReceipt, genErr := s.fixCandidate(ctx, e, current, call, budget, record.SQL, executionErrorCode(report, runErr))
		record.Receipt = appendReceipts(record.Receipt, correctionReceipt)
		if genErr != nil {
			return s.finishRun(ctx, e, record, report, 1, errors.Join(ErrExecutionBudget, genErr))
		}
		candidatePlan, validateErr := s.validator.Validate(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: candidate.SQL, Parameters: candidate.Parameters})
		if validateErr != nil {
			return s.finishRun(ctx, e, record, report, 1, errors.Join(ErrExecutionBudget, validateErr))
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
	admitted, admissionErr := s.currentAdmission(ctx, e, q)
	if admissionErr != nil {
		return admissionErr
	}
	if err = gatewayRequirement(e, "feedback.write", admitted.resources); err != nil {
		return err
	}
	correction := strings.TrimSpace(in.Correction)
	if correction != "" {
		if _, err = s.validator.Validate(ctx, e, exec.Request{Source: admitted.source, Context: admitted.context, SQL: correction, Parameters: q.Parameters}); err != nil {
			return err
		}
	}
	feedbackID, err := newID()
	if err != nil {
		return err
	}
	if err = s.repo.RecordFeedback(ctx, sc, FeedbackRecord{ID: feedbackID, QueryID: q.ID, Session: e.Session(), Verdict: in.Verdict, Correction: correction, Note: in.Note, Provenance: "phase18.feedback", Created: time.Now().UTC()}); err != nil {
		return err
	}
	if in.Verdict == "positive" || correction != "" {
		id, idErr := newID()
		if idErr != nil {
			return idErr
		}
		sqlText := q.SQL
		if correction != "" {
			sqlText = correction
		}
		_, err = s.repo.UpsertExample(ctx, sc, ExampleRecord{ID: id, Topic: q.Topic, Question: q.Question, SQL: sqlText, Digest: exampleDigest(q.Topic, q.Question, sqlText), State: "candidate", Weight: 0.5, EvidenceCount: 1, Provenance: "feedback:" + feedbackID, Created: time.Now().UTC(), Updated: time.Now().UTC()})
	}
	return err
}

// ExampleState advances one retained learning example through its explicit review state.
func (s *Service) ExampleState(ctx context.Context, e identity.Envelope, in ExampleStateRequest) (ExampleRecord, error) {
	if ctx == nil || !e.Valid() || !identity.Identifier(in.ExampleID) || (in.State != "candidate" && in.State != "active" && in.State != "retired") {
		return ExampleRecord{}, ErrInvalid
	}
	if !e.Has("feedback.write") {
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
	q, err := s.learningQuery(ctx, e, example.Topic)
	if err != nil {
		return ExampleRecord{}, err
	}
	admitted, err := s.currentAdmission(ctx, e, q)
	if err != nil {
		return ExampleRecord{}, err
	}
	if err = gatewayRequirement(e, "feedback.write", admitted.resources); err != nil {
		return ExampleRecord{}, err
	}
	updated, err := s.repo.SetExampleState(ctx, sc, in.ExampleID, in.State)
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

func (s *Service) plan(ctx context.Context, e identity.Envelope, question QuestionRequest, operation, parent, action string) (PlanResult, error) {
	if ctx == nil || !e.Valid() {
		return PlanResult{}, access.ErrUnauthenticated
	}
	if err := requireQuestionAction(e, action, question); err != nil {
		return PlanResult{}, err
	}
	if err := validateQuestion(question); err != nil {
		return PlanResult{}, err
	}
	admitted, err := s.admit(ctx, e, question, true)
	if err != nil {
		return PlanResult{}, err
	}
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
	learned := s.learnedInstructions(ctx, e, admitted.route.Topic)
	generation, err := assembler.ResolvePrecedence(ctx, nlq.GenerationInput{Context: admitted.assembled, EditBase: question.EditBase, Hints: question.Hints, Examples: append(learned, question.ExampleInput...), Default: question.Default})
	if err != nil {
		return PlanResult{}, err
	}
	candidate, fixes, receipt, validated, err := s.generateAndValidate(ctx, e, admitted, generation, call, budget, "")
	if err != nil {
		return PlanResult{}, err
	}
	id, err := newID()
	if err != nil {
		return PlanResult{}, err
	}
	record := queryRecord(e, id, "planned", parent, question, admitted)
	record.Operation, record.SQL, record.Parameters, record.Generation, record.Receipt, record.ValidationFixes = operation, candidate.SQL, candidate.Parameters, generation, receipt, fixes
	if err = s.repo.CreateQuery(ctx, mustScope(e), record); err != nil {
		return PlanResult{}, err
	}
	out := PlanResult{QueryID: id, SessionID: e.Session(), Status: "planned", Route: admitted.route, Confidence: admitted.route.Confidence, Generation: string(generation.Strategy), ValidationFixes: fixes, Assumptions: candidate.Assumptions, Ambiguities: candidate.Ambiguities, Receipt: receipt, validated: validated}
	if canInspect(e) {
		out.SQL = candidate.SQL
	}
	return out, nil
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
	route.Request = in.routeRequest()
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
		Question: request.Question, Kinds: append([]string(nil), request.Kinds...), LimitPerKind: request.LimitPerKind,
		References: append([]semantics.Reference(nil), request.References...), Choices: append([]nlqroute.ChoiceSelection(nil), request.Choices...),
		Joins: append([]nlqroute.JoinChoice(nil), request.JoinChoices...), MetricIDs: append([]string(nil), request.MetricIDs...),
		Examples: append([]nlq.OptionalItem(nil), request.Examples...), Rerank: request.Rerank,
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
	base.Choices = mergeChoices(base.Choices, delta.Choices)
	base.Joins = mergeJoins(base.Joins, delta.Joins)
	base.MetricIDs = mergeStrings(base.MetricIDs, delta.MetricIDs)
	base.Examples = append(base.Examples, delta.Examples...)
	base.Rerank = base.Rerank || delta.Rerank
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

func (s *Service) resealQueryContext(ctx context.Context, q QueryRecord) (nlq.AssembledContext, error) {
	if len(q.Topics) == 0 || len(q.Topics) != len(q.TopicVersions) {
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
			Question: view.Question, Evidence: append([]nlq.Evidence(nil), view.Evidence...), Constraints: view.Constraints,
			Metrics: append([]nlq.PinnedMetric(nil), view.Metrics...), Advisory: append([]nlq.OptionalItem(nil), view.Advisory...), Examples: append([]nlq.OptionalItem(nil), view.Examples...),
		}
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
		Topics: append([]nlq.TopicRevision(nil), persisted.Topics...), Question: persisted.Question,
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

func (s *Service) currentAdmission(ctx context.Context, e identity.Envelope, q QueryRecord) (admission, error) {
	result := admission{route: q.Route}
	for i, topicID := range q.Topics {
		contract, err := s.topics.Contract(ctx, e, topicID)
		if err != nil {
			return admission{}, err
		}
		publication := contract.Publication
		if publication.State.Topic != topicID || publication.State.Archived || !publication.State.Active || i >= len(q.TopicVersions) || publication.State.Version != q.TopicVersions[i] {
			return admission{}, exec.ErrBinding
		}
		for _, dataset := range publication.Definition.Datasets {
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
	result.binding, err = s.sources.Binding(ctx, e, result.source, result.context)
	if err != nil {
		return admission{}, err
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
	plan, validateErr := s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: candidate.SQL, Parameters: candidate.Parameters})
	if validateErr == nil {
		return candidate, 0, receipt, plan, nil
	}
	if errors.Is(validateErr, access.ErrNotFound) || errors.Is(validateErr, access.ErrForbidden) || errors.Is(validateErr, access.ErrUnauthenticated) || errors.Is(validateErr, exec.ErrBinding) {
		return generatedCandidate{}, 0, receipt, exec.Plan{}, validateErr
	}
	fixed, fixedReceipt, fixErr := s.generate(ctx, e, a, generation, call, budget, "sqlfix", validationCode(validateErr, candidate.SQL))
	receipt = appendReceipts(receipt, fixedReceipt)
	if fixErr != nil {
		return generatedCandidate{}, 1, receipt, exec.Plan{}, errors.Join(ErrValidationBudget, fixErr)
	}
	plan, validateErr = s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: fixed.SQL, Parameters: fixed.Parameters})
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
	out := RunResult{QueryID: q.ID, SessionID: q.Session, Status: q.Status, EvidenceStale: q.EvidenceStale, Route: q.Route, Confidence: q.Route.Confidence, Assumptions: append([]string(nil), q.Assumptions...), Ambiguities: append([]string(nil), q.Ambiguities...), ValidationFixes: q.ValidationFixes, ExecutionFixes: q.ExecutionFixes, Execution: report}
	if q.Result != nil && out.Execution.Result == nil {
		out.Execution.Result = q.Result
	}
	if inspect {
		out.SQL = q.SQL
	}
	return out
}

func (s *Service) learnedInstructions(ctx context.Context, e identity.Envelope, topic string) []nlq.Instruction {
	sc, err := scope(e)
	if err != nil {
		return nil
	}
	examples, err := s.repo.ListExamples(ctx, sc, topic, nlq.MaxExamples)
	if err != nil {
		return nil
	}
	result := make([]nlq.Instruction, 0, len(examples))
	for _, example := range examples {
		if example.State != "active" || example.SQL == "" || example.Question == "" {
			continue
		}
		result = append(result, nlq.Instruction{Key: "learned-" + example.ID, Text: "question:" + example.Question + " sql:" + example.SQL})
	}
	return result
}

func queryRecord(e identity.Envelope, id, status, parent string, in QuestionRequest, a admission) QueryRecord {
	return QueryRecord{ID: id, Session: e.Session(), Parent: parent, Topic: a.route.Topic, Topics: append([]string(nil), a.route.Topics...), TopicVersions: append([]string(nil), a.route.TopicVersions...), RuleVersions: append([]string(nil), a.route.RuleVersions...), Context: in.Context, Locale: in.Locale, Question: in.Question, Route: a.route, Status: status, Assumptions: assumptions(a.route), Ambiguities: ambiguities(a.route), Created: time.Now().UTC(), Updated: time.Now().UTC(), Revision: 1}
}

func (r QuestionRequest) routeRequest() nlqroute.RouteRequest {
	return nlqroute.RouteRequest{Topic: r.Topic, Topics: append([]string(nil), r.Topics...), Context: r.Context, Locale: r.Locale, Question: r.Question, Kinds: append([]string(nil), r.Kinds...), LimitPerKind: r.LimitPerKind, References: append([]semantics.Reference(nil), r.References...), Choices: append([]nlqroute.ChoiceSelection(nil), r.Choices...), JoinChoices: append([]nlqroute.JoinChoice(nil), r.Joins...), MetricIDs: append([]string(nil), r.MetricIDs...), Examples: cloneRouteExamples(r.Examples), Rerank: r.Rerank}
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
		return ErrInvalid
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
