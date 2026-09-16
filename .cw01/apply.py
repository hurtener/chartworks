from pathlib import Path
import re


def edit(path, before, after, count=1):
    p = Path(path)
    text = p.read_text()
    actual = text.count(before)
    if actual != count:
        raise SystemExit(f'{path}: expected {count} matches, found {actual}: {before[:140]!r}')
    p.write_text(text.replace(before, after))


def function(path, name, text):
    p = Path(path)
    source = p.read_text()
    pattern = r'func ' + re.escape(name) + r'\([^\n]*\n[\s\S]*?\n}\n'
    source, count = re.subn(pattern, lambda _: text.rstrip() + '\n', source, count=1)
    if count != 1:
        raise SystemExit(f'{path}: function not found: {name}')
    p.write_text(source)


def write(path, content):
    p = Path(path)
    if p.exists():
        raise SystemExit(f'{path}: refusing to overwrite an uninspected file')
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content)


types = 'internal/nlqexec/types.go'
service = 'internal/nlqexec/service.go'
route = 'internal/nlqroute/clarification.go'
store = 'internal/store/postgres/nlq_runtime.go'
helper = 'internal/nlqexec/clarification.go'

edit(types, 'type QueryRecord struct {', 'type QueryRecord struct {\n\tClarification *ClarificationEvidence `json:"-"`')
edit(types, 'type QuestionRequest struct {', '''type QuestionRequest struct {
    Answers []semantics.ClarificationAnswer `json:"answers,omitempty"`
    AnswerContext string `json:"answer_context,omitempty"`
    previousResolutions []semantics.ClarificationResolution''')
for name in ('PlanResult', 'RunResult'):
    edit(types, f'type {name} struct {{', f'''type {name} struct {{
    Bindings *exec.BusinessBindingReceipt `json:"bindings,omitempty"`
    AnswerChanges []ClarificationChange `json:"answer_changes,omitempty"`''')
edit(service, 'type generatedCandidate struct {', 'type generatedCandidate struct {\n\tclarification *ClarificationEvidence')
edit(service, '\troute.Request = in.routeRequest()\n', '''    // A current router has already classified and canonicalized its request.
    // Never replace that protected representation with raw answer strings.
    if route.Request.Question == "" && len(route.Resolutions) == 0 {
        route.Request = in.routeRequest()
    }
''')
edit(service, 'Question: in.Question, Route: a.route,', 'Question: a.route.Request.Question, Route: a.route,')
edit(service, 'return nlqroute.RouteRequest{Topic: r.Topic,', 'return nlqroute.RouteRequest{Answers: semantics.CloneClarificationAnswers(r.Answers), AnswerContext: r.AnswerContext, Topic: r.Topic,')
edit(service, '''	if withBinding {
		result.binding, err = s.sources.Binding(ctx, e, result.source, result.context)
		if err != nil {
			return admission{}, err
		}
	}
''', '''    if withBinding {
        result.binding, err = s.sources.Binding(ctx, e, result.source, result.context)
        if err != nil { return admission{}, err }
        if route.SourceBindingDigest != "" && route.SourceBindingDigest != exec.Hash(result.binding) {
            return admission{}, exec.ErrBinding
        }
    }
''')

function(service, '(s *Service) Refine', '''func (s *Service) Refine(ctx context.Context, e identity.Envelope, in RefineRequest) (PlanResult, error) {
    if ctx == nil || !e.Valid() || !identity.Identifier(in.QueryID) { return PlanResult{}, ErrInvalid }
    if !e.Has("query.execute") { return PlanResult{}, access.ErrForbidden }
    old, err := s.repo.ReadQuery(ctx, mustScope(e), in.QueryID)
    if err != nil { return PlanResult{}, err }
    if old.Session != e.Session() { return PlanResult{}, ErrForeignSession }
    pending := old.Status == "preflight" && old.Route.Clarification != nil
    if old.SQL == "" && !pending { return PlanResult{}, ErrNoPlan }
    if in.Context != "" && in.Context != old.Context { return PlanResult{}, ErrForeignSession }
    parent, err := s.admissionForQuery(ctx, e, old)
    if err != nil { return PlanResult{}, err }
    if err := gatewayRequirement(e, "query.execute", parent.resources); err != nil { return PlanResult{}, err }
    if _, err := s.replayQueryClarifications(ctx, e, old); err != nil { return PlanResult{}, err }
    question := refinementQuestion(old, in.QuestionRequest)
    if err := mergeRefinementClarifications(old, in.QuestionRequest, &question); err != nil { return PlanResult{}, err }
    if question.Question == "" { return PlanResult{}, ErrInvalid }
    // Answer edits cannot inherit old filters from protected SQL edit context.
    if len(in.Answers) == 0 && len(in.Choices) == 0 && old.SQL != "" {
        base := old.SQL
        if old.Clarification != nil { base = old.Clarification.BaseSQL }
        if base != "" { question.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "previous_sql", Text: base}) }
    }
    return s.plan(ctx, e, question, "", in.QueryID, "query.execute")
}''')
edit(service, '\tif admitted.route.Context == nil {', '''    question = redactClarificationInstructions(question, admitted.route)
    if admitted.route.Context == nil {''')
edit(service, '\trecord := queryRecord(e, id, "planned", parent, question, admitted)\n', '''    if err := sealClarificationCandidate(&candidate, validated, question.previousResolutions, admitted.route.Resolutions); err != nil {
        return PlanResult{}, err
    }
    record := queryRecord(e, id, "planned", parent, question, admitted)
    record.Clarification = candidate.clarification
''')
edit(service, 'out := PlanResult{QueryID: id,', 'out := PlanResult{Bindings: publicClarificationBinding(record.Clarification), AnswerChanges: publicClarificationChanges(record.Clarification), QueryID: id,')
edit(service, 'out := RunResult{QueryID: q.ID,', 'out := RunResult{Bindings: publicClarificationBinding(q.Clarification), AnswerChanges: publicClarificationChanges(q.Clarification), QueryID: q.ID,')
function(service, '(s *Service) generateAndValidate', '''func (s *Service) generateAndValidate(ctx context.Context, e identity.Envelope, a admission, generation nlq.GenerationContext, call gateway.Call, budget *gateway.Budget, correction string) (generatedCandidate, int, gateway.Receipt, exec.Plan, error) {
    candidate, receipt, err := s.generate(ctx, e, a, generation, call, budget, "sqlgen", correction)
    if err != nil { return generatedCandidate{}, 0, receipt, exec.Plan{}, err }
    candidate, err = bindClarificationCandidate(ctx, a, candidate)
    if err != nil { return generatedCandidate{}, 0, receipt, exec.Plan{}, err }
    plan, validateErr := s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: candidate.SQL, Parameters: candidate.Parameters})
    if validateErr == nil { return candidate, 0, receipt, plan, nil }
    if errors.Is(validateErr, access.ErrNotFound) || errors.Is(validateErr, access.ErrForbidden) || errors.Is(validateErr, access.ErrUnauthenticated) || errors.Is(validateErr, exec.ErrBinding) {
        return generatedCandidate{}, 0, receipt, exec.Plan{}, validateErr
    }
    fixed, fixedReceipt, fixErr := s.generate(ctx, e, a, generation, call, budget, "sqlfix", validationCode(validateErr, candidate.SQL))
    receipt = appendReceipts(receipt, fixedReceipt)
    if fixErr != nil { return generatedCandidate{}, 1, receipt, exec.Plan{}, errors.Join(ErrValidationBudget, fixErr) }
    fixed, fixErr = bindClarificationCandidate(ctx, a, fixed)
    if fixErr != nil { return generatedCandidate{}, 1, receipt, exec.Plan{}, fixErr }
    plan, validateErr = s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: fixed.SQL, Parameters: fixed.Parameters})
    if validateErr != nil { return generatedCandidate{}, 1, receipt, exec.Plan{}, errors.Join(ErrValidationBudget, validateErr) }
    return fixed, 1, receipt, plan, nil
}''')
edit(service, '\tprompt := generation.Prompt + "\\ndialect:" + dialect + "\\nsource_context:" + a.context', '''    if len(a.route.Resolutions) != 0 {
        system += " Reviewed clarification constraints are bound by the service after generation. Select their exact governed base relations; do not invent, repeat, or infer their scalar values or add predicates for those owned targets."
    }
    prompt := generation.Prompt + "\\ndialect:" + dialect + "\\nsource_context:" + a.context''')
edit(service, '\tplan, err := s.validator.Validate(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: record.SQL, Parameters: record.Parameters})', '''    if err := s.verifyQueryClarificationBinding(ctx, e, record, current); err != nil { return RunResult{}, err }
    plan, err := s.validator.Validate(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: record.SQL, Parameters: record.Parameters})''')
edit(service, '\t\tif !correctionEquivalent(', '''        if record.Clarification != nil && (candidate.SQL != record.SQL || !parametersEqual(candidate.Parameters, record.Parameters)) {
            return s.finishRun(ctx, e, record, report, 1, ErrUnsafeCorrection)
        }
        if !correctionEquivalent(''')
edit(service, '\tif in.Verdict == "positive" || correction != "" {', '\tif len(q.Route.Resolutions) == 0 && (in.Verdict == "positive" || correction != "") {')
edit(route, '\tif current.Clarification != nil {', '\tif current.Clarification != nil && previous.Clarification == nil {')
edit(route, '\tresult.Request.Question = safeQuestion', '''    result.Request.Question = safeQuestion
    for i := range result.Request.Examples {
        result.Request.Examples[i].Text = semantics.RedactClarificationText(result.Request.Examples[i].Text, in.Answers, redactions)
    }''')
edit(helper, '''	if candidate.clarification == nil {
		return nil
	}
	receipt := plan.Receipt()''', '''    if candidate.clarification == nil {
        if len(previous) != 0 { candidate.clarification = &ClarificationEvidence{SchemaVersion: 1, Changes: clarificationChanges(previous, current)} }
        return nil
    }
    receipt := plan.Receipt()''')
edit(helper, '''		if record.Clarification != nil {
			return exec.ErrBinding
		}
		return nil''', '''        if record.Clarification != nil {
            evidence := record.Clarification
            if evidence.SchemaVersion != 1 || evidence.BaseSQL != "" || len(evidence.BaseParameters) != 0 || evidence.Binding.SchemaVersion != 0 { return exec.ErrBinding }
            for _, change := range evidence.Changes { if change.Action != "removed" || change.Current != "" { return exec.ErrBinding } }
        }
        return nil''')
edit(helper, '\tif evidence == nil { return nil }\n\traw, err := json.Marshal(evidence.Binding)', '\tif evidence == nil || evidence.Binding.SchemaVersion == 0 { return nil }\n\traw, err := json.Marshal(evidence.Binding)')
with Path(helper).open('a') as f:
    f.write('''
func publicClarificationChanges(evidence *ClarificationEvidence) []ClarificationChange {
    if evidence == nil { return nil }
    return append([]ClarificationChange(nil), evidence.Changes...)
}
''')
write('internal/store/postgres/migrations/035_nlq_clarification.sql', '''ALTER TABLE chartworks.nlq_queries ADD COLUMN clarification jsonb;
ALTER TABLE chartworks.nlq_queries ADD CONSTRAINT nlq_clarification_shape CHECK (
    clarification IS NULL OR (
        jsonb_typeof(clarification) = 'object'
        AND clarification->>'schema_version' = '1'
        AND octet_length(clarification::text) <= 262144
    )
);
CREATE FUNCTION chartworks.keep_nlq_clarification_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.clarification IS DISTINCT FROM OLD.clarification THEN
        RAISE EXCEPTION 'immutable query clarification evidence';
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER nlq_clarification_immutable BEFORE UPDATE ON chartworks.nlq_queries
    FOR EACH ROW EXECUTE FUNCTION chartworks.keep_nlq_clarification_immutable();
''')
edit(store, '\t_, e = tx.Exec(ctx, `INSERT INTO chartworks.nlq_queries', '''    var clarification any
    if q.Clarification != nil {
        if q.Clarification.SchemaVersion != 1 { return store.ErrInvalid }
        clarification, e = marshalNLQ(q.Clarification)
        if e != nil { return e }
    }
    _, e = tx.Exec(ctx, `INSERT INTO chartworks.nlq_queries''')
edit(store, 'execution_fixes,revision,created_at,updated_at) VALUES(', 'execution_fixes,revision,created_at,updated_at,clarification) VALUES(')
edit(store, '$24,$25,$26,$27,$28)`, scope.Tenant(),', '$24,$25,$26,$27,$28,$29::jsonb)`, scope.Tenant(),')
edit(store, 'q.ExecutionFixes, q.Revision, q.Created, q.Updated)', 'q.ExecutionFixes, q.Revision, q.Created, q.Updated, clarification)')
edit(store, 'execution_fixes,revision,created_at,updated_at`', 'execution_fixes,revision,created_at,updated_at,clarification`')
edit(store, 'var topics, versions, rules, route, generation, params, receipt, result, assumptions, ambiguities, queryErrors []byte', 'var topics, versions, rules, route, generation, params, receipt, result, assumptions, ambiguities, queryErrors, clarification []byte')
edit(store, '&out.ExecutionFixes, &out.Revision, &out.Created, &out.Updated);', '&out.ExecutionFixes, &out.Revision, &out.Created, &out.Updated, &clarification);')
edit(store, '\tif len(result) > 0 {\n\t\tvar parsed exec.Result', '''    if len(clarification) > 0 && string(clarification) != "null" {
        var evidence nlqexec.ClarificationEvidence
        if json.Unmarshal(clarification, &evidence) != nil || evidence.SchemaVersion != 1 { return store.ErrMigration }
        out.Clarification = &evidence
    }
    if len(result) > 0 {
        var parsed exec.Result''')
print('Applied CW-01 consumer, persistence and refinement changes.')
