from pathlib import Path
import re

def edit(path,before,after,count=1):
 p=Path(path);text=p.read_text()
 if text.count(before)!=count:raise SystemExit(f'{path}: changed anchor {text.count(before)}')
 p.write_text(text.replace(before,after))

def create(path,text):
 p=Path(path)
 if p.exists():raise SystemExit(f'{path}: exists')
 p.write_text(text)

edit('test/acceptance/cw01_contracts_test.go','nlq.ErrInsufficientContext','nlq.ErrInsufficient')
edit('test/acceptance/cw01_consumers_test.go','Baseline: f.definition.Version, Candidate: imported.Definition.Version','BaselineRuleVersion: f.definition.Version, CandidateRuleVersion: imported.Definition.Version')
edit('test/acceptance/cw01_fixture_test.go','''	for _, column := range profile.Schema {
''','''	for _, column := range profile.Schema {
  switch column.Name {case "id","amount","created_at","active","name": default: continue}
''')

edit('internal/nlqexec/types.go','type QuestionRequest struct {','''type QuestionRequest struct {
 // ClarificationQuery anchors a typed submission to a retained preflight in
 // the current actor/session. The ID grants no authority and is rechecked.
 ClarificationQuery string `json:"clarification_query,omitempty"`''')
create('internal/nlqexec/clarification_origin.go',r'''package nlqexec

import (
 "context"
 "reflect"

 "github.com/hurtener/chartworks/internal/access"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
)

func clarificationOriginError(locale nlq.Language,code string) error {
 message:="Use the query ID and answer context from the same current preflight. To change the question, use session refinement."
 if locale==nlq.LanguageSpanish {message="Usá el identificador y el contexto de la misma consulta previa actual. Para cambiar la pregunta, usá el refinamiento de la sesión."}
 return &nlqroute.Clarification{Reason:code,Outcome:semantics.ClarificationInvalid,Errors:[]semantics.ClarificationFieldError{{Field:"clarification_query",Code:code,Message:message}}}
}

// validateClarificationOrigin verifies provenance before any provider work.
// Caller-supplied query IDs are references, not permission. Stateless routing
// remains a fresh interpretation; execution submissions must address the
// actual retained pending question instead of borrowing another form's pin.
func(s *Service) validateClarificationOrigin(ctx context.Context,e identity.Envelope,question QuestionRequest,action string) error {
 if ctx==nil || !e.Valid(){return access.ErrUnauthenticated}
 if err:=requireQuestionAction(e,action,question);err!=nil{return err}
 if err:=validateQuestion(question);err!=nil{return err}
 if question.ClarificationQuery=="" && len(question.Answers)==0{return nil}
 if !identity.Identifier(question.ClarificationQuery){return clarificationOriginError(question.Locale,"clarification_query_required")}
 old,err:=s.repo.ReadQuery(ctx,mustScope(e),question.ClarificationQuery)
 if err!=nil{return err}
 if old.Session!=e.Session(){return ErrForeignSession}
 if old.Status!="preflight" || old.SQL!="" {return clarificationOriginError(question.Locale,"clarification_query_not_pending")}
 topics:=append([]string(nil),question.Topics...)
 if len(topics)==0 && question.Topic!="" {topics=[]string{question.Topic}}
 request:=old.Route.Request
 if question.Question!=request.Question || question.Locale!=request.Locale || question.Context!=old.Context || !reflect.DeepEqual(topics,old.Topics) || !reflect.DeepEqual(question.References,request.References) || !reflect.DeepEqual(question.MetricIDs,request.MetricIDs) || !reflect.DeepEqual(question.Joins,request.JoinChoices) || question.AnswerContext=="" || question.AnswerContext!=old.Route.AnswerContext {
  return clarificationOriginError(question.Locale,"clarification_question_mismatch")
 }
 return nil
}
''')
edit('internal/nlqexec/service.go','''	if err := validateQuestion(in.QuestionRequest); err != nil {
		return PreflightResult{}, err
	}
''','''	if err := validateQuestion(in.QuestionRequest); err != nil {
		return PreflightResult{}, err
	}
 if err:=s.validateClarificationOrigin(ctx,e,in.QuestionRequest,"query.preflight");err!=nil{return PreflightResult{},err}
''')
edit('internal/nlqexec/service.go','queryRecord(e, id, "preflight", "", in.QuestionRequest, admitted)','queryRecord(e, id, "preflight", in.ClarificationQuery, in.QuestionRequest, admitted)')
edit('internal/nlqexec/service.go','''	return s.plan(ctx, e, in.QuestionRequest, in.Operation, "", "query.plan")''',''' if err:=s.validateClarificationOrigin(ctx,e,in.QuestionRequest,"query.plan");err!=nil{return PlanResult{},err}
	return s.plan(ctx, e, in.QuestionRequest, in.Operation, in.ClarificationQuery, "query.plan")''')
edit('internal/nlqexec/service.go','''	pending := old.Status == "preflight" && old.Route.Clarification != nil
''',''' if in.ClarificationQuery!="" && in.ClarificationQuery!=in.QueryID {return PlanResult{},clarificationOriginError(in.Locale,"clarification_question_mismatch")}
	pending := old.Status == "preflight" && old.Route.Clarification != nil
''')

# Explicitly pair existing typed answer submissions with their actual preflight
# receipts in the acceptance clients. Refine already has its parent QueryID.
for p in Path('test/acceptance').glob('cw01*_test.go'):
 text=p.read_text()
 text,n=re.subn(r'(\b([A-Za-z][A-Za-z0-9_]*)\.AnswerContext = ([A-Za-z][A-Za-z0-9_]*)\.Route\.AnswerContext)',r'\1\n \2.ClarificationQuery = \3.QueryID',text)
 p.write_text(text)

# Add a real same-session cross-question negative rather than relying only on
# tenant/session checks. A token/policy pin from a different question is not a
# pending-form binding for this question.
p=Path('test/acceptance/cw01_contracts_test.go');text=p.read_text()
anchor='''	before := f.model.requests.Load()
	stale := q'''
replacement=''' other:=f.preflight(t,f.question("Show optional sales",nlq.LanguageEnglish))
	before := f.model.requests.Load()
 swapped:=q;swapped.ClarificationQuery=other.QueryID
 if _,err:=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:swapped});err==nil{t.Fatal("answer borrowed a different same-session pending question")}
	stale := q'''
if text.count(anchor)!=1:raise SystemExit('origin negative anchor changed')
p.write_text(text.replace(anchor,replacement))
