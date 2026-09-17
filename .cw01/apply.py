from pathlib import Path

def create(path,text):
 p=Path(path)
 if p.exists(): raise SystemExit(f'{path}: exists')
 p.write_text(text)

p=Path('test/acceptance/cw01_test.go')
text=p.read_text(); end=text.rfind('\n}')
if end<0 or '"AC06"' in text: raise SystemExit('acceptance anchor changed')
text=text[:end]+'''
 t.Run("AC06",func(t *testing.T){cw01OrderingAcceptance(t,f)})
 t.Run("AC07",func(t *testing.T){cw01DefaultsAcceptance(t,f)})
 t.Run("AC08",func(t *testing.T){cw01IsolationAcceptance(t)})
 t.Run("AC09",func(t *testing.T){cw01BudgetPrivacyAcceptance(t,f)})
 t.Run("AC10",func(t *testing.T){cw01AuthoringAcceptance(t);cw01ConsumerAcceptance(t)})
'''+text[end:]
p.write_text(text)

create('test/acceptance/cw01_contracts_test.go',r'''package acceptance

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "reflect"
 "strings"
 "testing"

 "github.com/hurtener/chartworks/internal/access"
 "github.com/hurtener/chartworks/internal/auth"
 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqexec"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/semantics/rulesets"
 "github.com/hurtener/chartworks/internal/store"
)

func cw01Pattern(t *testing.T, definition semantics.RuleSetDefinition,id string) semantics.ClarificationPattern {
 t.Helper(); definition=semantics.CloneRuleSetDefinition(definition)
 for _,p:=range definition.Patterns {if p.ID==id{return p}}
 t.Fatal("synthetic pattern missing");return semantics.ClarificationPattern{}
}

func cw01OrderingAcceptance(t *testing.T,f *cw01Fixture){
 t.Helper();ctx:=context.Background()
 question:=f.question("Show large sales and active sales",nlq.LanguageEnglish)
 before:=f.model.requests.Load()
 pending:=f.preflight(t,question)
 if pending.Route.Clarification==nil || len(pending.Route.Clarification.Questions)!=2 || f.model.requests.Load()!=before {t.Fatal("independent blockers not grouped before provider")}
 for _,q:=range pending.Route.Clarification.Questions {if q.Prompt=="" || q.Why=="" {t.Fatal("independent blocker has no explanation")}}
 definition:=semantics.CloneRuleSetDefinition(f.definition)
 cases:=[]semantics.ClarificationInput{{Locale:"en",Question:question.Question}}
 left,err:=f.rules.PreviewClarifications(ctx,f.e,rulesets.ClarificationPreviewRequest{Definition:definition,Cases:cases});if err!=nil{t.Fatal(err)}
 for i,j:=0,len(definition.Patterns)-1;i<j;i,j=i+1,j-1 {definition.Patterns[i],definition.Patterns[j]=definition.Patterns[j],definition.Patterns[i]}
 right,err:=f.rules.PreviewClarifications(ctx,f.e,rulesets.ClarificationPreviewRequest{Definition:definition,Cases:cases})
 if err!=nil || left.RuleDigest!=right.RuleDigest || !reflect.DeepEqual(left.Cases,right.Cases) {t.Fatal("ordering depends on persistence order",err)}
 lower:=cw01Pattern(t,f.definition,"amount-default")
 lower.ID="lower";lower.Policy.When.AnyTerms=[]string{"conflict sales"};lower.Slots[0].Default=&semantics.ClarificationValue{Number:&semantics.ClarificationNumberInput{Value:"10",Unit:"USD"}}
 upper:=cw01Pattern(t,f.definition,"amount-default")
 upper.ID="upper";upper.Policy.When.AnyTerms=[]string{"conflict sales"};upper.Slots[0].Effect.Operator="lt";upper.Slots[0].Default=&semantics.ClarificationValue{Number:&semantics.ClarificationNumberInput{Value:"5",Unit:"USD"}}
 definition.Patterns=[]semantics.ClarificationPattern{upper,lower}
 conflict,err:=f.rules.PreviewClarifications(ctx,f.e,rulesets.ClarificationPreviewRequest{Definition:definition,Cases:[]semantics.ClarificationInput{{Locale:"en",Question:"Show conflict sales"}}})
 if err!=nil || conflict.Cases[0].Outcome!=semantics.ClarificationConflicting || len(conflict.Cases[0].Resolutions)!=0 || f.model.requests.Load()!=before {t.Fatal("incompatible defaults did not fail atomically",err)}
 // Dependent slots are not presented alongside their unanswered prerequisite.
 dependent:=cw01Pattern(t,f.definition,"amount-required")
 dependent.ID="ordered";dependent.Policy.When.AnyTerms=[]string{"dependent sales"}
 dependent.Slots[0].ID="minimum"
 next:=cw01Pattern(t,f.definition,"state").Slots[0]
 next.DependsOn=[]string{"minimum"}
 dependent.Slots=append(dependent.Slots,next)
 dependent.Targets=append(dependent.Targets,next.Effect.Target)
 definition.Patterns=[]semantics.ClarificationPattern{dependent}
 order,err:=f.rules.PreviewClarifications(ctx,f.e,rulesets.ClarificationPreviewRequest{Definition:definition,Cases:[]semantics.ClarificationInput{{Locale:"en",Question:"Show dependent sales"}}})
 if err!=nil{t.Fatal(err)}
 if len(order.Cases[0].Slots)!=2 || order.Cases[0].Slots[0].Slot!="minimum" || order.Cases[0].Slots[1].Reason!="dependency_missing" {t.Fatal("dependent evaluation lost prerequisite ordering")}
}

func cw01DefaultsAcceptance(t *testing.T,f *cw01Fixture){
 t.Helper();ctx:=context.Background()
 defaults,err:=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:f.question("Show default sales",nlq.LanguageEnglish)})
 if err!=nil || len(defaults.Route.Resolutions)!=1 || defaults.Route.Resolutions[0].Provenance!="reviewed_default" || len(defaults.Route.Request.Answers)!=0 {t.Fatal("default is not visible or attributed",err)}
 visible:=false
 for _,g:=range defaults.Route.Clarifications {for _,slot:=range g.Slots {visible=visible || slot.Defaulted && slot.Pattern=="amount-default"}}
 if !visible {t.Fatal("default omitted from user contract")}; f.run(t,defaults,1,true)
 required:=f.plan(t,f.question("Show large sales",nlq.LanguageEnglish),"amount-required",cw01Number("10"))
 remove:=f.answer(t,"amount-required",cw01Number("10"));remove.Value=nil;remove.Remove=true
 before:=f.model.requests.Load()
 out,err:=f.query.Refine(ctx,f.e,nlqexec.RefineRequest{QueryID:required.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{remove}}})
 var failure *nlqroute.Clarification
 if !errors.As(err,&failure) || failure.Outcome!=semantics.ClarificationMissing || out.QueryID!="" || f.model.requests.Load()!=before {t.Fatal("required removal silently skipped or defaulted",err)}
}

func cw01IsolationAcceptance(t *testing.T){
 t.Helper();ctx:=context.Background();f:=newCW01Fixture(t)
 q:=f.question("Show large sales",nlq.LanguageEnglish);pending:=f.preflight(t,q)
 q.AnswerContext=pending.Route.AnswerContext;q.Answers=[]semantics.ClarificationAnswer{f.answer(t,"amount-required",cw01Number("10"))}
 before:=f.model.requests.Load()
 stale:=q;stale.Answers=semantics.CloneClarificationAnswers(q.Answers);stale.Answers[0].PatternVersion="old-version"
 if _,err:=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:stale});err==nil{t.Fatal("stale pattern version accepted")}
 foreign:=phase18Envelope(t,f.phase17Fixture,f.e.User(),"other-session",true)
 if _,err:=f.query.Plan(ctx,foreign,nlqexec.PlanRequest{QuestionRequest:q});err==nil{t.Fatal("same-actor cross-session answers accepted")}
 claims:=f.model.token.claims(f.e.Tenant(),f.e.User(),[]string{"query.plan","query.execute","topics.read","cw.topic.read:"+f.pack.Topic})
 claims["session"]=f.e.Session()
 limited,err:=f.model.token.verifier.Verify(ctx,f.model.token.sign(t,claims,nil),auth.HTTP);if err!=nil{t.Fatal(err)}
 if _,err=f.query.Plan(ctx,limited,nlqexec.PlanRequest{QuestionRequest:q});!errors.Is(err,access.ErrForbidden) && !errors.Is(err,access.ErrNotFound){t.Fatal("current bearer reach was not rechecked",err)}
 wrong:=q;wrong.Context="another-context"
 if _,err=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:wrong});err==nil{t.Fatal("foreign source context accepted")}
 if f.model.requests.Load()!=before {t.Fatal("invalid version/session/context reached provider")}
 planned,err:=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:q});if err!=nil{t.Fatal(err)}
 before=f.model.requests.Load()
 tenantClaims:=f.model.token.claims("other-tenant",f.e.User(),phase18Scopes("other-tenant",true));tenantClaims["session"]=f.e.Session()
 tenant,err:=f.model.token.verifier.Verify(ctx,f.model.token.sign(t,tenantClaims,nil),auth.HTTP);if err!=nil{t.Fatal(err)}
 if _,err=f.query.Refine(ctx,tenant,nlqexec.RefineRequest{QueryID:planned.QueryID});err==nil{t.Fatal("cross-tenant query was replayed")}
 if _,err=f.query.Refine(ctx,foreign,nlqexec.RefineRequest{QueryID:planned.QueryID});!errors.Is(err,nlqexec.ErrForeignSession){t.Fatal("retained query crossed its session",err)}
 newer:=semantics.CloneRuleSetDefinition(f.definition);newer.Version="rules-v2"
 f.publishRules(t,newer,1)
 if _,err=f.query.Refine(ctx,f.e,nlqexec.RefineRequest{QueryID:planned.QueryID});err==nil{t.Fatal("publication change silently reused old answer")}
 if f.model.requests.Load()!=before {t.Fatal("stale or foreign replay reached provider")}
 t.Run("source-revision",func(t *testing.T){
  source:=newCW01Fixture(t)
  plan:=source.plan(t,source.question("Show large sales",nlq.LanguageEnglish),"amount-required",cw01Number("10"))
  binding:=source.pack.Datasets[0].Source
  if _,err:=source.f.s.Rotate(ctx,source.f.e,binding.Source,binding.SourceRevision);err!=nil{t.Fatal("rotate fixture source",err)}
  before:=source.model.requests.Load()
  if _,err:=source.query.Refine(ctx,source.e,nlqexec.RefineRequest{QueryID:plan.QueryID});err==nil{t.Fatal("rotated source accepted stale answers")}
  if source.model.requests.Load()!=before{t.Fatal("source-stale replay reached provider")}
 })
}

func cw01BudgetPrivacyAcceptance(t *testing.T,f *cw01Fixture){
 t.Helper();ctx:=context.Background()
 f.model.mu.Lock();start:=len(f.model.requestBodies);f.model.mu.Unlock()
 sensitive:=f.plan(t,f.question("Show named sales",nlq.LanguageEnglish),"customer",cw01Text("alias-secret-731"))
 f.run(t,sensitive,1,true)
 f.model.mu.Lock();bodies:=strings.Join(append([]string(nil),f.model.requestBodies[start:]...),"\n");f.model.mu.Unlock()
 for _,value:=range []string{"alias-secret-731","cw-alpha-731","cw-beta-731"}{if strings.Contains(bodies,value){t.Fatal("sensitive clarification value reached provider")}}
 scope,err:=store.NewScope(f.e.Tenant(),f.e.User());if err!=nil{t.Fatal(err)}
 record,err:=f.f.db.ReadQuery(ctx,scope,sensitive.QueryID);if err!=nil{t.Fatal(err)}
 encoded,_:=json.Marshal(record.Route.Request)
 if strings.Contains(string(encoded),"alias-secret-731") || len(record.Route.Resolutions)!=1 || record.Route.Resolutions[0].Sensitivity!=semantics.LiteralSensitive || record.Route.Resolutions[0].ParserVersion!="clarification-values-v1" || record.Route.Resolutions[0].Locale!="en" {t.Fatal("raw answer retained or classification/parser provenance lost")}
 diagnostic:=fmt.Sprintf("%v %#v",f.answer(t,"customer",cw01Text("alias-secret-731")),record.Route.Resolutions[0])
 if strings.Contains(diagnostic,"alias-secret-731") || strings.Contains(diagnostic,"cw-alpha-731"){t.Fatal("ordinary diagnostics leak protected values")}
 unresolved:=f.question("Show named sales",nlq.LanguageEnglish);pending:=f.preflight(t,unresolved)
 unresolved.AnswerContext=pending.Route.AnswerContext;unresolved.Answers=[]semantics.ClarificationAnswer{f.answer(t,"customer",cw01Text("unresolved-private-731"))}
 before:=f.model.requests.Load()
 repair,err:=f.query.Preflight(ctx,f.e,nlqexec.PreflightRequest{QuestionRequest:unresolved})
 if err!=nil || repair.Route.Clarification==nil || f.model.requests.Load()!=before {t.Fatal("unresolved sensitive answer executed",err)}
 saved,err:=f.f.db.ReadQuery(ctx,scope,repair.QueryID);if err!=nil{t.Fatal(err)}
 raw,_:=json.Marshal(saved)
 if strings.Contains(string(raw),"unresolved-private-731"){t.Fatal("unresolved raw answer persisted")}
 t.Run("mandatory-group",func(t *testing.T){
  bounded:=newCW01Fixture(t)
  definition:=semantics.CloneRuleSetDefinition(bounded.definition);definition.Version="budget-rules"
  prototype:=cw01Pattern(t,definition,"amount-default");definition.Patterns=nil
  for i:=0;i<64;i++{
   candidate:=semantics.CloneRuleSetDefinition(semantics.RuleSetDefinition{Patterns:[]semantics.ClarificationPattern{prototype}}).Patterns[0]
   candidate.ID=fmt.Sprintf("budget-%02d",i);candidate.Policy.When.AnyTerms=[]string{"budget sales"}
   definition.Patterns=append(definition.Patterns,candidate)
  }
  bounded.publishRules(t,definition,1)
  before:=bounded.model.requests.Load()
  out,err:=bounded.query.Plan(ctx,bounded.e,nlqexec.PlanRequest{QuestionRequest:bounded.question("Show budget sales",nlq.LanguageEnglish)})
  if !errors.Is(err,nlq.ErrInsufficientContext) || out.QueryID!="" || out.Bindings!=nil || bounded.model.requests.Load()!=before {t.Fatalf("mandatory group was partially admitted or reached provider: %v",err)}
 })
}
''')

create('test/acceptance/cw01_consumers_test.go',r'''package acceptance

import (
 "bytes"
 "context"
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "net/http/httptest"
 "strings"
 "testing"

 "github.com/hurtener/chartworks/internal/api"
 "github.com/hurtener/chartworks/internal/config"
 "github.com/hurtener/chartworks/internal/mcpserver"
 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqapi"
 "github.com/hurtener/chartworks/internal/nlqexec"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/semantics/rulesets"
 "github.com/hurtener/chartworks/internal/topicapi"
 sdk "github.com/hurtener/chartworks/sdk/chartworks"
 "github.com/hurtener/chartworks/test/support"
)

func cw01HTTPClient(t *testing.T,f *cw01Fixture)(*sdk.Client,*httptest.Server,string){
 t.Helper()
 _,published:=newPhase18Service(t,f.phase17Fixture)
 handler:=nlqapi.ExecutionHandler(f.model.token.verifier,f.query,topicapi.Handler(f.model.token.verifier,nil,published,f.rules,http.NotFoundHandler()))
 server:=httptest.NewServer(handler);t.Cleanup(server.Close)
 token:=phase18Token(t,f.phase17Fixture,f.e.User(),f.e.Session(),true)
 client,err:=sdk.New(server.URL,server.Client(),func(context.Context)(string,error){return token,nil});if err!=nil{t.Fatal(err)}
 return client,server,token
}

func cw01AuthoringAcceptance(t *testing.T){
 t.Helper();f:=newCW01Fixture(t);ctx:=context.Background()
 client,server,token:=cw01HTTPClient(t,f)
 cases:=[]semantics.ClarificationInput{{Locale:"en",Question:"Show large sales"},{Locale:"es",Question:"Mostrá el total de ventas"},{Locale:"en",Question:"Show turnover review"}}
 before:=f.model.requests.Load()
 preview,err:=client.PreviewClarifications(ctx,f.pack.Topic,sdk.ClarificationPreviewRequest{Definition:f.definition,Cases:cases})
 if err!=nil || len(preview.Cases)!=3 || preview.Cases[0].Outcome!=semantics.ClarificationMissing || preview.Cases[1].Outcome!=semantics.ClarificationSatisfied {t.Fatal("HTTP draft preview did not evaluate matching/nonmatching cases",err)}
 detached,err:=f.rules.Patterns(ctx,f.e,f.pack.Topic,f.definition.Version);if err!=nil{t.Fatal(err)}
 detached[0].Policy.Why="mutated read"
 detached[0].Slots[0].Effect.Unit="mutated-unit"
 again,err:=f.rules.Patterns(ctx,f.e,f.pack.Topic,f.definition.Version)
 if err!=nil || again[0].Policy.Why=="mutated read" || again[0].Slots[0].Effect.Unit=="mutated-unit" {t.Fatal("published policy nested pointers are shared",err)}
 portable,err:=client.ExportClarifications(ctx,f.pack.Topic,sdk.ClarificationExportRequest{Version:f.definition.Version})
 if err!=nil || portable.RuleDigest!=preview.RuleDigest || len(portable.Dispositions)!=len(f.definition.Patterns) {t.Fatal("versioned export lost exact definition",err)}
 imported,err:=client.PreviewClarificationImport(ctx,f.pack.Topic,sdk.ClarificationImportRequest{Pack:portable,Version:"rules-imported",Cases:cases})
 if err!=nil || !imported.ReviewRequired || imported.Definition.Version!="rules-imported" {t.Fatal("portable import preview failed",err)}
 current,err:=f.rules.Read(ctx,f.e,f.pack.Topic,"")
 if err!=nil || current.State.Version!=f.definition.Version {t.Fatal("preview changed active policy",err)}
 for i:=range imported.Definition.Patterns {if imported.Definition.Patterns[i].ID=="amount-required" {imported.Definition.Patterns[i].Policy.When.AnyTerms=append(imported.Definition.Patterns[i].Policy.When.AnyTerms,"turnover review")}}
 f.publishRules(t,imported.Definition,1)
 refs:=[]semantics.Reference{{Kind:semantics.KindDataset,ID:f.pack.Datasets[0].ID}}
 replay,err:=f.rules.Replay(ctx,f.e,f.pack.Topic,rulesets.ReplayRequest{TopicVersion:f.published.State.Version,RuleVersion:f.definition.Version,References:refs,ClarificationCases:cases})
 if err!=nil || len(replay.BaselineClarifications)!=3 {t.Fatal("retained clarification replay failed",err)}
 shadow,err:=f.rules.Shadow(ctx,f.e,f.pack.Topic,rulesets.ShadowRequest{TopicVersion:f.published.State.Version,Baseline:f.definition.Version,Candidate:imported.Definition.Version,References:refs,ClarificationCases:cases})
 if err!=nil || !shadow.Changed || len(shadow.CandidateClarifications)!=3 || shadow.BaselineClarifications[2].Outcome!=semantics.ClarificationSatisfied || shadow.CandidateClarifications[2].Outcome!=semantics.ClarificationMissing {t.Fatal("shadow missed conditional behavior change",err)}
 metadata:=support.Raw(t,f.f.dsn)
 var baseline,candidate []byte
 err=metadata.QueryRow(ctx,`SELECT baseline_clarification_result,candidate_clarification_result FROM chartworks.topic_rule_comparison_evidence WHERE tenant_id=$1 AND comparison_id=$2`,f.e.Tenant(),shadow.ID).Scan(&baseline,&candidate)
 var storedBaseline,storedCandidate []semantics.ClarificationEvaluation
 if err!=nil || json.Unmarshal(baseline,&storedBaseline)!=nil || json.Unmarshal(candidate,&storedCandidate)!=nil || len(storedBaseline)!=3 || len(storedCandidate)!=3 || storedCandidate[2].Outcome!=semantics.ClarificationMissing {t.Fatal("typed shadow evidence not persisted",err)}
 if _,err=metadata.Exec(ctx,`UPDATE chartworks.topic_rule_comparison_evidence SET baseline_clarification_result='[]'::jsonb WHERE tenant_id=$1 AND comparison_id=$2`,f.e.Tenant(),shadow.ID);err==nil{t.Fatal("retained comparison evidence mutated")}
 // Unknown executable matcher fields must fail the actual closed HTTP decoder.
 wire,_:=json.Marshal(sdk.ClarificationPreviewRequest{Definition:imported.Definition,Cases:cases})
 var unknown map[string]any;_ =json.Unmarshal(wire,&unknown)
 definition:=unknown["definition"].(map[string]any)
 policy:=definition["patterns"].([]any)[0].(map[string]any)["policy"].(map[string]any)
 policy["when"].(map[string]any)["regex"]=".*"
 raw,_:=json.Marshal(unknown)
 request,_:=http.NewRequestWithContext(ctx,http.MethodPost,server.URL+"/v1/topics/"+f.pack.Topic+"/clarifications/preview",bytes.NewReader(raw));request.Header.Set("Authorization","Bearer "+token);request.Header.Set("Content-Type","application/json")
 response,err:=server.Client().Do(request);if err!=nil{t.Fatal(err)}
 _,_=io.Copy(io.Discard,response.Body);_=response.Body.Close()
 if response.StatusCode!=400 {t.Fatal("ordinary authoring accepted an arbitrary executable matcher")}
 if f.model.requests.Load()!=before {t.Fatal("authoring/replay/shadow invoked provider")}
 // Legacy imports require a deliberate reference-only disposition and cannot
 // acquire automatic blocking merely because the field was once marked required.
 legacy:=semantics.CloneRuleSetDefinition(imported.Definition);legacy.Version="legacy-rules"
 pattern:=cw01Pattern(t,legacy,"metric");pattern.Policy=nil;legacy.Patterns=[]semantics.ClarificationPattern{pattern}
 f.publishRules(t,legacy,2)
 exported,err:=f.rules.ExportClarifications(ctx,f.e,f.pack.Topic,rulesets.ClarificationExportRequest{Version:legacy.Version});if err!=nil{t.Fatal(err)}
 migration:=rulesets.ClarificationImportRequest{Pack:exported,Version:"legacy-reviewed-copy",Cases:[]semantics.ClarificationInput{{Locale:"en",Question:"Choose sales"}}}
 if _,err=f.rules.PreviewClarificationImport(ctx,f.e,f.pack.Topic,migration);rulesets.IsClarificationImportError(err)!="legacy_disposition_required" {t.Fatal("legacy imported without a safe disposition",err)}
 migration.LegacyDisposition="preserve_reference_only"
 compatible,err:=f.rules.PreviewClarificationImport(ctx,f.e,f.pack.Topic,migration)
 if err!=nil || compatible.Preview.Cases[0].Outcome!=semantics.ClarificationSatisfied {t.Fatal("safe legacy migration introduced blocking",err)}
 if _,err=f.rules.Retire(ctx,f.e,f.pack.Topic,rulesets.RetireRequest{Expected:3,Note:"Retire synthetic clarification policy"});err!=nil{t.Fatal(err)}
 retained,err:=f.rules.Replay(ctx,f.e,f.pack.Topic,rulesets.ReplayRequest{TopicVersion:f.published.State.Version,RuleVersion:legacy.Version,References:refs,ClarificationCases:migration.Cases})
 if err!=nil || retained.BaselineClarifications[0].Outcome!=semantics.ClarificationSatisfied {t.Fatal("retirement destroyed exact retained replay",err)}
}

func cw01ConsumerAcceptance(t *testing.T){
 t.Helper();ctx:=context.Background();f:=newCW01Fixture(t)
 client,_,_:=cw01HTTPClient(t,f)
 question:=f.question("Show optional sales",nlq.LanguageSpanish)
 pending,err:=client.PreflightNLQ(ctx,sdk.NLQPreflightRequest{QuestionRequest:question});if err!=nil{t.Fatal(err)}
 question.AnswerContext=pending.Route.AnswerContext;question.Answers=[]semantics.ClarificationAnswer{f.answer(t,"amount-optional",cw01Number("10"))}
 plan,err:=client.PlanNLQ(ctx,sdk.NLQPlanRequest{QuestionRequest:question})
 if err!=nil || plan.Bindings==nil || plan.Bindings.Validation==nil {t.Fatal("HTTP/SDK typed plan roundtrip failed",err)}
 run,err:=client.RunNLQ(ctx,sdk.NLQRunRequest{QueryID:plan.QueryID,Operation:plan.QueryID+"-http"})
 if err!=nil || run.Execution.Result==nil || len(run.Execution.Result.Rows)!=1 {t.Fatal("HTTP answer did not affect actual rows",err)}
 corrected:=f.answer(t,"amount-optional",cw01Number("1"))
 child,err:=client.RefineNLQ(ctx,sdk.NLQRefineRequest{QueryID:plan.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{corrected}}})
 if err!=nil || len(child.AnswerChanges)!=1 {t.Fatal("HTTP correction lineage missing",err)}
 changed,err:=client.RunNLQ(ctx,sdk.NLQRunRequest{QueryID:child.QueryID,Operation:child.QueryID+"-http"})
 if err!=nil || changed.Execution.Result==nil || len(changed.Execution.Result.Rows)!=2 {t.Fatal("HTTP correction retained stale filter",err)}
 invalid:=f.answer(t,"amount-optional",cw01Number("not-a-number"));before:=f.model.requests.Load()
 _,err=client.RefineNLQ(ctx,sdk.NLQRefineRequest{QueryID:child.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{invalid}}})
 var status *sdk.StatusError
 if !errors.As(err,&status) || status.Clarification==nil || len(status.Clarification.Fields)==0 || !strings.Contains(status.Clarification.Fields[0].Message,"Usá") || f.model.requests.Load()!=before {t.Fatal("HTTP localized repair lost or provider called",err)}
 bindings,err:=nlqapi.ExecutionMCPBindings(f.query);if err!=nil{t.Fatal(err)}
 registry,err:=mcpserver.NewRegistry(bindings);if err!=nil{t.Fatal(err)}
 settings:=config.DefaultMCP()
 server,err:=mcpserver.New(f.model.token.verifier,registry,settings,[]string{"https://console.example"});if err!=nil{t.Fatal(err)}
 transport,err:=mcpserver.HTTPRegistry(settings);if err!=nil{t.Fatal(err)}
 network:=httptest.NewServer(api.Guard(f.model.token.verifier,transport,server.Handler()));defer network.Close()
 claims:=f.model.token.claims(f.e.Tenant(),f.e.User(),append(phase18Scopes(f.e.Tenant(),true),"mcp.use"));claims["session"]=f.e.Session();claims["aud"]=f.model.token.cfg.MCPAudience()
 token:=f.model.token.sign(t,claims,nil)
 mcpClient,err:=sdk.New(network.URL,network.Client(),func(context.Context)(string,error){return token,nil});if err!=nil{t.Fatal(err)}
 // The same HTTP-created retained query is refined by the actual MCP/SDK
 // transport under a newly supplied bearer, not copied to another executor.
 corrected.Value=nil;corrected.Remove=true
 removed:=phase22Call[nlqexec.PlanResult](t,mcpClient,"refine_question",nlqexec.RefineRequest{QueryID:child.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{corrected}}})
 if len(removed.Route.Resolutions)!=0 || len(removed.AnswerChanges)!=1 || removed.AnswerChanges[0].Action!="removed" {t.Fatal("MCP removal contract lost")}
 clean:=phase22Call[nlqexec.RunResult](t,mcpClient,"run_question",nlqexec.RunRequest{QueryID:removed.QueryID,Operation:removed.QueryID+"-mcp"})
 if clean.Execution.Result==nil || len(clean.Execution.Result.Rows)!=2 {t.Fatal("MCP retained stale filter")}
 bad:=phase22RawTool(t,mcpClient,"refine_question",nlqexec.RefineRequest{QueryID:child.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{invalid}}})
 failure:=phase22Fault(t,bad)
 if !bad.IsError || failure.Clarification==nil || failure.Clarification.Outcome!=semantics.ClarificationInvalid {t.Fatal("MCP typed field repair lost")}
}
''')
