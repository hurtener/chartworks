from pathlib import Path

p=Path('internal/nlqroute/service_test.go')
text=p.read_text()
start=text.index('func TestRouteClarifiesRequiredRuleSlotBeforeGateway(')
end=text.index('func TestConfirmJoinsRequiresPublishedSameSourceOneToOne(',start)
text=text[:start]+r'''func compiledRouteChoiceRules(t *testing.T, conditional bool) rulesets.Published {
 t.Helper()
 target := semantics.Reference{Kind:semantics.KindDataset,ID:"dataset"}
 definition := semantics.RuleSetDefinition{
  SchemaVersion:1, ID:"rules",Version:"rules-v1",Topic:"topic",TopicVersion:"v1",PackDigest:testPublication().Digest,
  Patterns:[]semantics.ClarificationPattern{{ID:"metric-choice",Version:"pattern-v1",Targets:[]semantics.Reference{target},Provenance:semantics.RuleProvenance{Kind:semantics.ProvenanceHuman,Evidence:"synthetic-reviewed-choice"},Slots:[]semantics.ClarificationSlot{{ID:"metric",Prompt:"Choose a reviewed dataset",Required:true,Kind:semantics.SlotChoice,Sensitivity:semantics.LiteralNonSensitive,Choices:[]semantics.ClarificationChoice{{ID:"revenue",Label:"Revenue",Target:&target},{ID:"orders",Label:"Orders"}}}}}},
 }
 if conditional {
  definition.Patterns[0].Policy=&semantics.ClarificationPolicy{SchemaVersion:1,When:semantics.ClarificationWhen{AnyTerms:[]string{"revenue","ingresos"}},Why:"Selects the reviewed dataset for this question."}
  definition.Patterns[0].Slots[0].Choices[1].Target=&target
 }
 subject,err:=semantics.NewRuleSubject(semantics.TopicPack{SchemaVersion:1,Topic:"topic",Version:"v1",Datasets:[]semantics.Dataset{{ID:"dataset"}}},testPublication().Digest)
 if err!=nil {t.Fatal(err)}
 model,err:=semantics.CompilePublishedRules(subject,definition)
 if err!=nil {t.Fatal(err)}
 return rulesets.Published{State:rulesets.State{Topic:"topic",Version:"rules-v1",Revision:1,Active:true},Digest:model.Digest(),Definition:model.Definition()}
}

func TestRouteClarifiesRequiredRuleSlotBeforeGateway(t *testing.T) {
 published:=compiledRouteChoiceRules(t,true)
 service,engine,_:=newTestService(t,testRules{published:published})
 out,err:=service.Route(context.Background(),testEnvelope(t,true),RouteRequest{Topic:"topic",Context:"ctx",Locale:nlq.LanguageSpanish,Question:"¿Qué ingresos?"})
 if err!=nil || out.Outcome!=nlq.StrategyClarify || out.Clarification==nil || out.Clarification.Reason!="required_answers" || engine.embeds!=0 {t.Fatalf("reviewed required choice did not block: %v",err)}
 if len(out.Clarification.Questions)!=1 || out.Clarification.Questions[0].Why=="" {t.Fatal("reviewed explanation missing")}
 out,err=service.Route(context.Background(),testEnvelope(t,true),RouteRequest{Topic:"topic",Context:"ctx",Locale:nlq.LanguageEnglish,Question:"List the complete dataset"})
 if err!=nil || out.Context==nil || out.Clarification!=nil || engine.embeds!=1 {t.Fatal("unrelated question was interrupted",err)}
}

func TestRouteEvaluatesChoiceTargetsAndRejectsUnknownChoices(t *testing.T) {
 t.Run("exact reviewed target reaches sealed context",func(t *testing.T){
  service,engine,_:=newTestService(t,testRules{published:compiledRouteChoiceRules(t,true)})
  out,err:=service.Route(context.Background(),testEnvelope(t,true),RouteRequest{Topic:"topic",Context:"ctx",Locale:nlq.LanguageEnglish,Question:"What is revenue?",Choices:[]ChoiceSelection{{Pattern:"metric-choice",Slot:"metric",Value:"revenue"}}})
  if err!=nil || out.Context==nil || engine.embeds!=1 || len(out.Resolutions)!=1 || out.Resolutions[0].Reference==nil || out.Resolutions[0].Reference.ID!="dataset" {t.Fatal("exact choice lost its reviewed effect",err)}
 })
 t.Run("legacy effectless choice cannot silently default",func(t *testing.T){
  service,engine,_:=newTestService(t,testRules{published:compiledRouteChoiceRules(t,false)})
  out,err:=service.Route(context.Background(),testEnvelope(t,true),RouteRequest{Topic:"topic",Context:"ctx",Locale:nlq.LanguageEnglish,Question:"What is revenue?",Choices:[]ChoiceSelection{{Pattern:"metric-choice",Slot:"metric",Value:"orders"}}})
  if err==nil && (out.Clarification==nil || out.Context!=nil) {t.Fatal("effectless legacy choice was accepted")}
  if engine.embeds!=0 {t.Fatal("effectless choice reached provider")}
 })
 t.Run("unknown choice fails atomically with detached repair",func(t *testing.T){
  published:=compiledRouteChoiceRules(t,true)
  service,engine,_:=newTestService(t,testRules{published:published})
  _,err:=service.Route(context.Background(),testEnvelope(t,true),RouteRequest{Topic:"topic",Context:"ctx",Locale:nlq.LanguageEnglish,Question:"What is revenue?",Choices:[]ChoiceSelection{{Pattern:"metric-choice",Slot:"metric",Value:"missing"}}})
  var failure *Clarification
  if !errors.As(err,&failure) || failure.Outcome!=semantics.ClarificationInvalid || len(failure.Errors)==0 || engine.embeds!=0 {t.Fatal("foreign choice was not rejected before provider",err)}
  if len(failure.Questions)>0 && len(failure.Questions[0].Choices)>0 {
   failure.Questions[0].Choices[0].Label="mutated"
   if published.Definition.Patterns[0].Slots[0].Choices[0].Label=="mutated" {t.Fatal("repair aliases publication")}
  }
 })
 t.Run("legacy patterns do not introduce new blockers",func(t *testing.T){
  service,engine,_:=newTestService(t,testRules{published:compiledRouteChoiceRules(t,false)})
  out,err:=service.Route(context.Background(),testEnvelope(t,true),RouteRequest{Topic:"topic",Context:"ctx",Locale:nlq.LanguageEnglish,Question:"What is revenue?"})
  if err!=nil || out.Context==nil || out.Clarification!=nil || engine.embeds!=1 {t.Fatal("legacy pattern accidentally became required",err)}
 })
}

'''+text[end:]
p.write_text(text)

p=Path('internal/nlqroute/service.go')
text=p.read_text()
before='''	if !contextMatches(admitted, in.Context) {
		return RouteResult{}, readexec.ErrBinding
	}
	if err := s.prepareClarifications'''
after='''	if len(admitted) > 1 {
  if incompatible := confirmJoins(admitted, in.JoinChoices); incompatible != nil && incompatible.Reason == "unconfirmed_source" {
   result.Outcome, result.Clarification = nlq.StrategyClarify, incompatible
   return result, nil
  }
 }
	if !contextMatches(admitted, in.Context) {
		return RouteResult{}, readexec.ErrBinding
	}
	if err := s.prepareClarifications'''
if text.count(before)!=1: raise SystemExit('route admission anchor changed')
p.write_text(text.replace(before,after))
