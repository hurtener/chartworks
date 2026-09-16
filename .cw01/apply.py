from pathlib import Path

def edit(path,before,after):
 p=Path(path);s=p.read_text()
 if s.count(before)!=1:raise SystemExit(f'{path}: expected one patch anchor, found {s.count(before)}')
 p.write_text(s.replace(before,after,1))

def append(path,text):
 p=Path(path);p.write_text(p.read_text()+text)

edit('internal/nlqroute/service.go','type RouteResult struct {','type RouteResult struct {\n ConstraintBinding *ConstraintBindingReceipt `json:"constraint_binding,omitempty"`')
append('internal/nlqroute/clarification.go','''
// ConstraintBindingReceipt is content-free evidence of a semantically bound
// candidate and its ordinary validator-issued read receipt. It grants no access.
type ConstraintBindingReceipt struct {
 SchemaVersion int `json:"schema_version"`
 Dialect string `json:"dialect"`
 ResolutionIDs []string `json:"resolution_ids"`
 SourceBinding string `json:"source_binding"`
 Statement string `json:"statement"`
 Parameters string `json:"parameters"`
 ReadManifest string `json:"read_manifest"`
}
''')
edit('internal/nlqexec/types.go','type QuestionRequest struct {','type QuestionRequest struct {\n Answers []semantics.ClarificationAnswer `json:"answers,omitempty"`')
edit('internal/nlqexec/service.go','type admission struct {','type admission struct {\n clarificationFields []clarificationField')
edit('internal/nlqexec/service.go','if old.Session != e.Session() || old.Status == "preflight" {','if old.Session != e.Session() || (old.Status == "preflight" && old.Route.Clarification == nil) {')
edit('internal/nlqexec/service.go','\tif old.SQL == "" {','\tif old.SQL == "" && old.Status != "preflight" {')
edit('internal/nlqexec/service.go','\tparent, err := s.admissionForQuery(ctx, e, old)',''' if err := validateAnswerEdits(old,in.QuestionRequest); err!=nil {return PlanResult{},err}
 if err := s.recheckStoredClarifications(ctx,e,old);err!=nil{return PlanResult{},err}
\tparent, err := s.admissionForQuery(ctx, e, old)''')
edit('internal/nlqexec/service.go','\tquestion.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "previous_sql", Text: old.SQL})',''' if old.SQL!="" && len(old.Route.Resolutions)==0 && len(in.Answers)==0 {
  question.EditBase = replaceInstruction(question.EditBase, nlq.Instruction{Key: "previous_sql", Text: old.SQL})
 }
''')
edit('internal/nlqexec/service.go','\trecord.Operation = in.Operation\n\tvar current admission',''' if err:=s.recheckStoredClarifications(ctx,e,record);err!=nil{return RunResult{},err}
\trecord.Operation = in.Operation
\tvar current admission''')
edit('internal/nlqexec/service.go','\tplan, err := s.validator.Validate(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: record.SQL, Parameters: record.Parameters})',''' if err:=verifyClarificationBinding(record.Route,current.binding,record.SQL,record.Parameters);err!=nil{return RunResult{},err}
\tplan, err := s.validator.Validate(ctx, e, exec.Request{Source: current.source, Context: current.context, SQL: record.SQL, Parameters: record.Parameters})''')
edit('internal/nlqexec/service.go','\trecord := queryRecord(e, id, "planned", parent, question, admitted)',''' if err:=sealClarificationBinding(&admitted.route,admitted.binding,candidate,validated);err!=nil{return PlanResult{},err}
\trecord := queryRecord(e, id, "planned", parent, question, admitted)''')
edit('internal/nlqexec/service.go','\troute, err := s.router.Route(ctx, e, in.routeRequest())',''' pre,err:=s.clarificationPreflight(ctx,e,in)
 if err!=nil{return admission{},err}
 if pre!=nil && pre.route.Clarification!=nil{return *pre,nil}
\troute, err := s.router.Route(ctx, e, in.routeRequest())''')
edit('internal/nlqexec/service.go','\troute.Request = in.routeRequest()',''' if route.Request.Question=="" {
  if len(in.Answers)>0{return admission{},ErrClarificationBinding}
  route.Request=in.routeRequest()
 }
 if pre!=nil && !sameClarificationEvidence(pre.route,route){return admission{},staleClarification(in.Locale)}''')
edit('internal/nlqexec/service.go','\tresult := admission{route: route, assembled: assembled}',''' result := admission{route: route, assembled: assembled}
 if pre!=nil {result.clarificationFields=pre.clarificationFields}''')
edit('internal/nlqexec/service.go','\tif withBinding {\n\t\tresult.binding, err = s.sources.Binding(ctx, e, result.source, result.context)',''' if withBinding {
\t\tresult.binding, err = s.sources.Binding(ctx, e, result.source, result.context)''')
# Add the final preflight binding fence only in admit, not retained readers.
p=Path('internal/nlqexec/service.go');s=p.read_text();start=s.index('func (s *Service) admit(');end=s.index('// admissionForQuery',start)
part=s[start:end]
anchor='\treturn result, nil\n}'
assert part.count(anchor)==1
part=part.replace(anchor,''' if withBinding && pre!=nil && len(pre.route.Resolutions)>0 && exec.Hash(pre.binding)!=exec.Hash(result.binding) {return admission{},ErrClarificationBinding}
\treturn result, nil
}''')
s=s[:start]+part+s[end:];p.write_text(s)
edit('internal/nlqexec/service.go','\tbase := QuestionRequest{','\tbase := QuestionRequest{\n Answers: nlqroute.CloneAnswers(request.Answers),')
edit('internal/nlqexec/service.go','\tbase.Choices = mergeChoices(base.Choices, delta.Choices)','\tbase.Choices = mergeChoices(base.Choices, delta.Choices)\n base.Answers = mergeAnswers(base.Answers,delta.Answers)')
edit('internal/nlqexec/service.go','return nlqroute.RouteRequest{Topic: r.Topic,','return nlqroute.RouteRequest{Answers:nlqroute.CloneAnswers(r.Answers),Topic: r.Topic,')
edit('internal/nlqexec/service.go','\tplan, validateErr := s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: candidate.SQL, Parameters: candidate.Parameters})',''' candidate,bindErr:=bindClarificationCandidate(a,candidate)
 var plan exec.Plan
 validateErr:=bindErr
 if bindErr==nil {plan,validateErr=s.validator.Validate(ctx,e,exec.Request{Source:a.source,Context:a.context,SQL:candidate.SQL,Parameters:candidate.Parameters})}''')
edit('internal/nlqexec/service.go','\tplan, validateErr = s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: fixed.SQL, Parameters: fixed.Parameters})',''' fixed,bindErr=bindClarificationCandidate(a,fixed)
 if bindErr!=nil{return generatedCandidate{},1,receipt,exec.Plan{},errors.Join(ErrValidationBudget,bindErr)}
\tplan, validateErr = s.validator.Validate(ctx, e, exec.Request{Source: a.source, Context: a.context, SQL: fixed.SQL, Parameters: fixed.Parameters})''')
# Bindings remain stable across a permitted formatting-only execution repair;
# the ordinary executor separately records the newly issued native read manifest.
edit('internal/nlqexec/clarification_replay.go','SchemaVersion:1,ResolutionIDs:ids,','SchemaVersion:1,Dialect:binding.Dialect,ResolutionIDs:ids,')
p=Path('internal/nlqexec/clarification_replay.go');s=p.read_text();s=s.replace('exec.Hash(candidate.SQL)','clarificationStatementHash(candidate.SQL,binding.Dialect)').replace('exec.Hash(candidate.Parameters)','clarificationParameterHash(candidate.Parameters)').replace('exec.Hash(sql)','clarificationStatementHash(sql,receipt.Dialect)').replace('exec.Hash(parameters)','clarificationParameterHash(parameters)').replace('exec.Hash(q.SQL)','clarificationStatementHash(q.SQL,r.ConstraintBinding.Dialect)').replace('exec.Hash(q.Parameters)','clarificationParameterHash(q.Parameters)');p.write_text(s)
append('internal/nlqexec/clarification_replay.go','''
func clarificationStatementHash(sql,dialect string) string {
 if dialect=="postgres" {tree,ok:=normalizedPostgresTree(sql);if !ok{return ""};return exec.Hash(tree)}
 return exec.Hash(sql)
}
func clarificationParameterHash(parameters []exec.Parameter) string {
 if parameters==nil {parameters=[]exec.Parameter{}}
 return exec.Hash(parameters)
}
''')
# Preserve one user-answer provenance across legacy and canonical roundtrips.
edit('internal/semantics/clarification_evaluate.go','origins[key] = "legacy_reference_choice"','origins[key] = "answer"')
edit('internal/store/postgres/nlq_runtime.go','func insertNLQQuery(ctx context.Context, tx pgx.Tx, scope store.Scope, q nlqexec.QueryRecord) error {','''func insertNLQQuery(ctx context.Context, tx pgx.Tx, scope store.Scope, q nlqexec.QueryRecord) error {
 if err:=nlqexec.ValidateClarificationRecord(q,scope.Tenant(),scope.Actor());err!=nil{return err}''')
edit('internal/store/postgres/nlq_runtime.go','func (d *DB) UpdateQuery(ctx context.Context, scope store.Scope, q nlqexec.QueryRecord, expected int64) error {','''func (d *DB) UpdateQuery(ctx context.Context, scope store.Scope, q nlqexec.QueryRecord, expected int64) error {
 if err:=nlqexec.ValidateClarificationRecord(q,scope.Tenant(),scope.Actor());err!=nil{return err}''')
edit('internal/store/postgres/nlq_runtime.go','\treturn nil\n}\nfunc stringValue(value *string) string {','\treturn nlqexec.ValidateClarificationRecord(*out,tenantValue,actorValue)\n}\nfunc stringValue(value *string) string {')
