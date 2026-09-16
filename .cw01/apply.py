from pathlib import Path

def create(path, text):
    p=Path(path)
    if p.exists(): raise SystemExit(f'{path}: refusing overwrite')
    p.write_text(text)

def edit(path, before, after, count=1):
    p=Path(path); text=p.read_text()
    if text.count(before)!=count: raise SystemExit(f'{path}: mismatched anchor')
    p.write_text(text.replace(before,after))

create('test/acceptance/cw01_fixture_test.go', r'''package acceptance

import (
 "context"
 "encoding/json"
 "testing"

 "github.com/hurtener/chartworks/internal/engineering"
 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqexec"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/semantics/rulesets"
 "github.com/hurtener/chartworks/internal/semantics/topics"
 "github.com/hurtener/chartworks/internal/vindex"
)

type cw01Fixture struct {
 *phase17Fixture
 published topics.Published
 rules *rulesets.Service
 query *nlqexec.Service
 definition semantics.RuleSetDefinition
}

func newCW01Fixture(t *testing.T) *cw01Fixture {
 t.Helper()
 f, draftsService, topicService, model, pack := publicationFixture(t)
 ctx := context.Background()
 if _, err := f.admin.Exec(ctx, `UPDATE analytics.sales SET name=CASE id WHEN 1 THEN 'cw-alpha-731' ELSE 'cw-beta-731' END`); err != nil { t.Fatal(err) }
 old := pack.Datasets[0]
 profile := f.profile(t, engineering.ProfileSpec{ID:"cw01-typed-profile", Source:old.Source.Source, Context:old.Source.Context, Dataset:old.ID, Columns:[]string{"id","amount","created_at","active","name"}, SkipLLM:true}).Profile.Profile
 dataset := semantics.Dataset{ID:profile.Dataset,Name:"Sales",Source:semantics.SourceReference{Source:old.Source.Source,Context:old.Source.Context,Dataset:profile.Dataset,SourceRevision:profile.SourceRevision,ProfileVersion:profile.Version,ProfileDigest:profile.DeterministicHash()}}
 for _, column := range profile.Schema {
  dataset.Columns=append(dataset.Columns,semantics.Column{ID:column.Name,SourceName:column.Name,Name:column.Name,NativeType:column.NativeType,Category:column.Category,Nullable:column.Nullable})
 }
 pack.Datasets[0]=dataset
 e:=f.token.envelope(t,f.e.Tenant(),f.e.User(),topicScopes(f.e.Tenant())...)
 published:=phase17PublishTopic(t,draftsService,topicService,e,pack)
 ruleService,err:=rulesets.New(f.db,f.db,f.db)
 if err!=nil {t.Fatal(err)}
 index,err:=vindex.New(f.db); if err!=nil {t.Fatal(err)}
 route,err:=nlqroute.New(topicService,ruleService,index,model.engine); if err!=nil {t.Fatal(err)}
 pf:=&phase17Fixture{f:f,pack:pack,model:model,service:route,context:dataset.Source.Context}
 pf.e=phase18Envelope(t,pf,f.e.User(),"cw01-session",true)
 out:=&cw01Fixture{phase17Fixture:pf,published:published,rules:ruleService}
 out.query,_=newPhase18Service(t,pf)
 out.definition=cw01Definition(published)
 out.publishRules(t,out.definition,0)
 model.embeddingMode.Store("fixed")
 model.rerankMode.Store("fixed")
 model.mode.Store(phase18RawResponse(t,"SELECT id, amount FROM analytics.sales ORDER BY id"))
 return out
}

func cw01Definition(p topics.Published) semantics.RuleSetDefinition {
 column:=func(id string)semantics.Reference{return semantics.Reference{Kind:semantics.KindColumn,Dataset:p.Definition.Datasets[0].ID,ID:id}}
 amount,created,active,name:=column("amount"),column("created_at"),column("active"),column("name")
 number:=func(required bool)semantics.ClarificationSlot{return semantics.ClarificationSlot{ID:"threshold",Prompt:"What minimum amount should count?",PromptES:"¿Qué importe mínimo debe contar?",Required:required,Kind:semantics.SlotNumber,Sensitivity:semantics.LiteralNonSensitive,Effect:&semantics.ClarificationEffect{Kind:"number",Target:amount,Operator:"gte",Nulls:"exclude",Unit:"USD",Precision:30,Scale:3}}}
 pattern:=func(id string,terms []string,slot semantics.ClarificationSlot)semantics.ClarificationPattern{return semantics.ClarificationPattern{ID:id,Version:"v1",Targets:[]semantics.Reference{slot.Effect.Target},Provenance:semantics.RuleProvenance{Kind:semantics.ProvenanceHuman,Evidence:"cw01-synthetic-review"},Policy:&semantics.ClarificationPolicy{SchemaVersion:semantics.ClarificationSchemaVersion,When:semantics.ClarificationWhen{AnyTerms:terms},Why:"This answer determines which sales are included.",WhySpanish:"Esta respuesta determina qué ventas se incluyen."},Slots:[]semantics.ClarificationSlot{slot}}}
 temporal:=semantics.ClarificationSlot{ID:"period",Prompt:"Which date window should count?",PromptES:"¿Qué período debe contar?",Required:true,Kind:semantics.SlotDate,Sensitivity:semantics.LiteralNonSensitive,Effect:&semantics.ClarificationEffect{Kind:"time_window",Target:created,Operator:"range",Nulls:"exclude",Bounds:"[)",Calendar:"gregorian",TimeZone:"America/Argentina/Buenos_Aires",TemporalType:"timestamptz",Grains:[]string{"day","month"}}}
 flag:=semantics.ClarificationSlot{ID:"active",Prompt:"Include active or inactive sales?",PromptES:"¿Ventas activas o inactivas?",Required:true,Kind:semantics.SlotBoolean,Sensitivity:semantics.LiteralNonSensitive,Effect:&semantics.ClarificationEffect{Kind:"boolean",Target:active,Operator:"eq",Nulls:"exclude"}}
 entity:=semantics.ClarificationSlot{ID:"customer",Prompt:"Which reviewed customer?",PromptES:"¿Qué cliente revisado?",Required:true,Kind:semantics.SlotText,Sensitivity:semantics.LiteralSensitive,Effect:&semantics.ClarificationEffect{Kind:"entity",Target:name,Operator:"eq",Nulls:"exclude",MaxLength:64,Values:[]semantics.GovernedClarificationValue{{Canonical:"cw-alpha-731",Label:"First customer",LabelES:"Primer cliente",Aliases:[]string{"first","primero","alias-secret-731"}},{Canonical:"cw-beta-731",Label:"Second customer",LabelES:"Segundo cliente",Aliases:[]string{"second","segundo"}}}}}
 patterns:=[]semantics.ClarificationPattern{
  pattern("amount-required",[]string{"large sales","ventas grandes"},number(true)),
  pattern("amount-optional",[]string{"optional sales"},number(false)),
  pattern("period",[]string{"dated sales","ventas fechadas"},temporal),
  pattern("state",[]string{"active sales","ventas activas"},flag),
  pattern("customer",[]string{"named sales","ventas por nombre"},entity),
 }
 defaultSlot:=number(false)
 defaultSlot.Default=&semantics.ClarificationValue{Number:&semantics.ClarificationNumberInput{Value:"10",Unit:"USD"}}
 patterns=append(patterns,pattern("amount-default",[]string{"default sales"},defaultSlot))
 measure:=semantics.Reference{Kind:semantics.KindMeasure,ID:"revenue"}
 patterns=append(patterns,semantics.ClarificationPattern{ID:"metric",Version:"v1",Targets:[]semantics.Reference{measure,amount},Provenance:semantics.RuleProvenance{Kind:semantics.ProvenanceHuman,Evidence:"cw01-synthetic-review"},Policy:&semantics.ClarificationPolicy{SchemaVersion:1,When:semantics.ClarificationWhen{AnyTerms:[]string{"choose sales"}},Why:"Selects the exact reviewed metric."},Slots:[]semantics.ClarificationSlot{{ID:"metric",Prompt:"Which measure?",Required:true,Kind:semantics.SlotChoice,Sensitivity:semantics.LiteralNonSensitive,Choices:[]semantics.ClarificationChoice{{ID:"revenue-option",Label:"Revenue",Target:&measure},{ID:"amount-option",Label:"Raw amount",Target:&amount}}}}})
 return semantics.RuleSetDefinition{SchemaVersion:semantics.SchemaVersion,ID:"cw01-rules",Version:"rules-v1",Topic:p.State.Topic,TopicVersion:p.State.Version,PackDigest:p.Digest,Patterns:patterns}
}

func (f *cw01Fixture) publishRules(t *testing.T, definition semantics.RuleSetDefinition, expected int64) {
 t.Helper(); ctx:=context.Background()
 draft,err:=f.rules.Save(ctx,f.e,rulesets.SaveRequest{Expected:expected,Definition:definition,Change:"Reviewed synthetic clarification policy"}); if err!=nil {t.Fatal("save policy",err)}
 review,err:=f.rules.Review(ctx,f.e,definition.Topic,rulesets.ReviewRequest{DraftRevision:draft.Revision,Digest:draft.Digest,Decision:"approve",Note:"Reviewed synthetic clarification effect"}); if err!=nil {t.Fatal("review policy",err)}
 if _,err=f.rules.Publish(ctx,f.e,definition.Topic,rulesets.PublishRequest{Review:review.ID,Expected:expected}); err!=nil {t.Fatal("publish policy",err)}
}

func (f *cw01Fixture) question(text string, locale nlq.Language) nlqexec.QuestionRequest {
 return nlqexec.QuestionRequest{Topic:f.pack.Topic,Context:f.context,Locale:locale,Question:text,Kinds:[]string{"measure"},LimitPerKind:1}
}

func (f *cw01Fixture) preflight(t *testing.T, q nlqexec.QuestionRequest) nlqexec.PreflightResult {
 t.Helper()
 out,err:=f.query.Preflight(context.Background(),f.e,nlqexec.PreflightRequest{QuestionRequest:q})
 if err!=nil || out.QueryID=="" {t.Fatalf("preflight: %v",err)}
 return out
}

func (f *cw01Fixture) answer(t *testing.T, pattern string, value semantics.ClarificationValue) semantics.ClarificationAnswer {
 t.Helper()
 for _,p:=range f.definition.Patterns {if p.ID==pattern {return semantics.ClarificationAnswer{Topic:f.pack.Topic,TopicVersion:f.published.State.Version,RulesetVersion:f.definition.Version,Pattern:p.ID,PatternVersion:p.Version,Slot:p.Slots[0].ID,Value:&value}}}
 t.Fatal("fixture policy missing"); return semantics.ClarificationAnswer{}
}

func (f *cw01Fixture) plan(t *testing.T,q nlqexec.QuestionRequest,pattern string,value semantics.ClarificationValue) nlqexec.PlanResult {
 t.Helper()
 pending:=f.preflight(t,q)
 q.AnswerContext=pending.Route.AnswerContext
 q.Answers=[]semantics.ClarificationAnswer{f.answer(t,pattern,value)}
 out,err:=f.query.Plan(context.Background(),f.e,nlqexec.PlanRequest{QuestionRequest:q})
 if err!=nil || out.QueryID=="" || out.Bindings==nil {t.Fatalf("typed plan: %v",err)}
 return out
}

func (f *cw01Fixture) run(t *testing.T,plan nlqexec.PlanResult,rows int,large bool) nlqexec.RunResult {
 t.Helper()
 out,err:=f.query.Run(context.Background(),f.e,nlqexec.RunRequest{QueryID:plan.QueryID,Operation:plan.QueryID+"-run"})
 if err!=nil || out.Execution==nil || out.Execution.Result==nil || len(out.Execution.Result.Rows)!=rows {t.Fatalf("constraint result: expected %d rows: %v",rows,err)}
 raw,_:=json.Marshal(out.Execution.Result.Rows)
 contains:=false
 for i:=0;i+len("9007199254740993.125")<=len(raw);i++ {if string(raw[i:i+len("9007199254740993.125")])=="9007199254740993.125" {contains=true;break}}
 if rows>0 && contains!=large {t.Fatal("constraint selected the wrong exact source values")}
 return out
}

func cw01Number(value string) semantics.ClarificationValue {return semantics.ClarificationValue{Number:&semantics.ClarificationNumberInput{Value:value,Unit:"USD"}}}
func cw01Time(start,end,grain string) semantics.ClarificationValue {return semantics.ClarificationValue{Time:&semantics.ClarificationTimeInput{Start:start,End:end,Calendar:"gregorian",TimeZone:"America/Argentina/Buenos_Aires",Grain:grain}}}
func cw01Bool(value string) semantics.ClarificationValue {return semantics.ClarificationValue{Boolean:&value}}
func cw01Text(value string) semantics.ClarificationValue {return semantics.ClarificationValue{Text:&value}}
''')

create('test/acceptance/cw01_test.go', r'''package acceptance

import (
 "context"
 "encoding/json"
 "errors"
 "reflect"
 "strings"
 "testing"

 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqexec"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/store"
)

// This corpus executes Chartworks services with PostgreSQL and recorded model
// responses. It is not a live provider quality or representative-user study.
func TestCW01(t *testing.T) {
 f:=newCW01Fixture(t)
 ctx:=context.Background()
 t.Run("AC01",func(t *testing.T){
  before:=f.model.requests.Load()
  missing:=f.preflight(t,f.question("Show large sales",nlq.LanguageEnglish))
  if missing.Route.Outcome!=nlq.StrategyClarify || missing.Route.Clarification==nil || len(missing.Route.Clarification.Questions)!=1 || f.model.requests.Load()!=before {t.Fatal("matching blocker did not stop before provider")}
  unrelated:=f.preflight(t,f.question("List all sales",nlq.LanguageEnglish))
  if unrelated.Route.Clarification!=nil || unrelated.Route.Context==nil || f.model.requests.Load()<=before {t.Fatal("unrelated complete question interrupted")}
  for _,evaluation:=range unrelated.Route.Clarifications {for _,slot:=range evaluation.Slots {if slot.Outcome!=semantics.ClarificationNotApplicable {t.Fatal("unrelated policy applied")}}}
 })
 t.Run("AC02",func(t *testing.T){
  en:=f.plan(t,f.question("Show dated sales",nlq.LanguageEnglish),"period",cw01Time("2026-01-02","2026-01-03","day"))
  es:=f.plan(t,f.question("Mostrá ventas fechadas",nlq.LanguageSpanish),"period",cw01Time("2026-01-02","2026-01-03","day"))
  a,b:=en.Route.Resolutions[0].Time,es.Route.Resolutions[0].Time
  if a==nil || !reflect.DeepEqual(a,b) || a.StartUTC!="2026-01-02T03:00:00Z" || a.EndUTC!="2026-01-03T03:00:00Z" || a.Bounds!="[)" {t.Fatal("bilingual time bounds lost calendar/zone/boundaries")}
  f.run(t,en,1,true); f.run(t,es,1,true)
  corrected:=f.answer(t,"period",cw01Time("2026-01-03","2026-01-04","day"))
  child,err:=f.query.Refine(ctx,f.e,nlqexec.RefineRequest{QueryID:en.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{corrected}}})
  if err!=nil {t.Fatal("date correction",err)}
  f.run(t,child,1,false)
 })
 t.Run("AC03",func(t *testing.T){
  cases:=[]struct{name,question,pattern string; value semantics.ClarificationValue}{
   {"date","Show dated sales","period",cw01Time("2026-02-30","2026-03-02","day")},
   {"grain","Show dated sales","period",cw01Time("2026-01-02","2026-01-03","hour")},
   {"number","Show large sales","amount-required",cw01Number("NaN")},
   {"precision","Show large sales","amount-required",cw01Number("1.0001")},
   {"boolean","Show active sales","state",cw01Bool("perhaps")},
   {"choice","Choose sales","metric",semantics.ClarificationValue{OptionID:"foreign"}},
   {"nonreference-option","Show large sales","amount-required",semantics.ClarificationValue{OptionID:"10"}},
  }
  for _,tc:=range cases {t.Run(tc.name,func(t *testing.T){
   q:=f.question(tc.question,nlq.LanguageEnglish); p:=f.preflight(t,q)
   q.AnswerContext=p.Route.AnswerContext; q.Answers=[]semantics.ClarificationAnswer{f.answer(t,tc.pattern,tc.value)}
   before:=f.model.requests.Load(); out,err:=f.query.Plan(ctx,f.e,nlqexec.PlanRequest{QuestionRequest:q})
   var failure *nlqroute.Clarification
   if !errors.As(err,&failure) || failure.Outcome!=semantics.ClarificationInvalid || len(failure.Errors)==0 || out.QueryID!="" || out.Bindings!=nil || f.model.requests.Load()!=before {t.Fatalf("invalid accepted or provider called: %v",err)}
  })}
 })
 t.Run("AC04",func(t *testing.T){
  boolean:=f.plan(t,f.question("Show active sales",nlq.LanguageSpanish),"state",cw01Bool("falso"))
  if boolean.Route.Resolutions[0].Value!="false" {t.Fatal("boolean not canonical")}; f.run(t,boolean,1,false)
  entity:=f.plan(t,f.question("Show named sales",nlq.LanguageSpanish),"customer",cw01Text("primero"))
  r:=entity.Route.Resolutions[0]
  if r.Value!="cw-alpha-731" || r.Effect==nil || r.Effect.Target.ID!="name" || r.Effect.Operator!="eq" || r.Effect.Nulls!="exclude" {t.Fatal("governed entity effect lost")}; f.run(t,entity,1,true)
  choice:=f.plan(t,f.question("Choose sales",nlq.LanguageEnglish),"metric",semantics.ClarificationValue{OptionID:"revenue-option"})
  if choice.Route.Resolutions[0].Reference==nil || choice.Route.Resolutions[0].Reference.ID!="revenue" || len(choice.Route.Context.Metrics)!=1 {t.Fatal("reference choice not propagated")}
  q:=f.question("Show named sales",nlq.LanguageEnglish); pending:=f.preflight(t,q); q.AnswerContext=pending.Route.AnswerContext; q.Answers=[]semantics.ClarificationAnswer{f.answer(t,"customer",cw01Text("not-governed"))}
  before:=f.model.requests.Load(); out,err:=f.query.Preflight(ctx,f.e,nlqexec.PreflightRequest{QuestionRequest:q})
  if err!=nil || out.Route.Clarification==nil || f.model.requests.Load()!=before {t.Fatal("unresolved text treated as a filter",err)}
 })
 t.Run("AC05",func(t *testing.T){
  initial:=f.plan(t,f.question("Show optional sales",nlq.LanguageEnglish),"amount-optional",cw01Number("10")); f.run(t,initial,1,true)
  if len(initial.Route.Resolutions)!=1 || initial.Bindings.Validation=="" || initial.Route.Context.Constraints==nil {t.Fatal("resolution did not reach validated plan receipt")}
  scope,err:=store.NewScope(f.e.Tenant(),f.e.User()); if err!=nil {t.Fatal(err)}
  stored,err:=f.f.db.ReadQuery(ctx,scope,initial.QueryID)
  if err!=nil || len(stored.Parameters)!=1 || stored.Clarification==nil || stored.Clarification.BaseSQL==stored.SQL {t.Fatal("canonical binding not persisted",err)}
  changed:=f.answer(t,"amount-optional",cw01Number("1"))
  child,err:=f.query.Refine(ctx,f.e,nlqexec.RefineRequest{QueryID:initial.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{changed}}})
  if err!=nil || len(child.AnswerChanges)!=1 || child.AnswerChanges[0].Action!="superseded" || child.AnswerChanges[0].Previous!=initial.Route.Resolutions[0].ID {t.Fatal("replacement lineage lost",err)}
  f.run(t,child,2,true)
  removed:=changed; removed.Value=nil; removed.Remove=true
  clean,err:=f.query.Refine(ctx,f.e,nlqexec.RefineRequest{QueryID:child.QueryID,QuestionRequest:nlqexec.QuestionRequest{Answers:[]semantics.ClarificationAnswer{removed}}})
  if err!=nil || len(clean.Route.Resolutions)!=0 || len(clean.AnswerChanges)!=1 || clean.AnswerChanges[0].Action!="removed" {t.Fatal("explicit removal lost",err)}
  f.run(t,clean,2,true)
  replay,err:=f.f.db.ReadQuery(ctx,scope,initial.QueryID)
  if err!=nil || !reflect.DeepEqual(stored.Parameters,replay.Parameters) {t.Fatal("replacement mutated retained parent",err)}
  latest,err:=f.f.db.ReadQuery(ctx,scope,clean.QueryID)
  if err!=nil || len(latest.Parameters)!=0 || strings.Contains(latest.SQL,"cw_filter_") {t.Fatal("removed answer retained a stale bound filter",err)}
  raw,_:=json.Marshal(latest.Route.Request)
  if strings.Contains(string(raw),"Bearer") {t.Fatal("authority retained in replay request")}
 })
}
''')
# The existing phase fixture intentionally migrates its reviewed required choice.
# The separate CW-01 corpus covers legacy non-activation rather than restoring
# unsafe automatic blocking for definitions with no applicability policy.
edit('test/acceptance/phase16_test.go',
 'ID: "metric-choice", Version: "v1", Targets:',
 'ID: "metric-choice", Version: "v1", Policy: &semantics.ClarificationPolicy{SchemaVersion: 1, When: semantics.ClarificationWhen{AnyTerms: []string{"metric", "revenue", "ingresos"}}, Why: "Select the reviewed metric used for this question."}, Targets:')
edit('test/acceptance/phase16_test.go', 'out.Clarification.Reason != "required_slot"', 'out.Clarification.Reason != "required_answers"')
edit('test/acceptance/phase16_test.go',
 'out.Context.Constraints == nil || len(out.Context.Constraints.Required) != 1 || out.Context.Constraints.Required[0].ID != "measure:revenue"',
 'out.Context.Constraints == nil || len(out.Context.Constraints.Required) != 2 || out.Context.Constraints.Required[0].ID != "measure:revenue" || out.Context.Constraints.Required[1].Kind != "clarification"')
