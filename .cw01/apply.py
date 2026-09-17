from pathlib import Path


def edit(path,before,after,count=1):
 p=Path(path); text=p.read_text()
 if text.count(before)!=count: raise SystemExit(f'{path}: changed anchor {text.count(before)}')
 p.write_text(text.replace(before,after))

def create(path,text):
 p=Path(path)
 if p.exists(): raise SystemExit(f'{path}: exists')
 p.write_text(text)

edit('internal/semantics/clarification_types.go','type ClarificationResolution struct {','type ClarificationResolution struct {\n ParserVersion string `json:"parser_version"`\n Locale string `json:"locale"`')
edit('internal/semantics/clarification_values.go','out := ClarificationResolution{SchemaVersion: ClarificationSchemaVersion, Sensitivity: slot.Sensitivity}', 'out := ClarificationResolution{SchemaVersion: ClarificationSchemaVersion, ParserVersion: "clarification-values-v1", Locale: locale, Sensitivity: slot.Sensitivity}')
edit('internal/semantics/clarification_types.go','type ClarificationSlotOutcome struct {','type ClarificationSlotOutcome struct {\n Specificity int `json:"specificity"`\n Priority int `json:"priority"`\n Order int `json:"order"`')
edit('internal/semantics/clarification_evaluate.go','for _, slot := range slots {','for slotIndex, slot := range slots {')
edit('internal/semantics/clarification_evaluate.go','state := clarificationSlotView(definition, pattern, slot, input.Locale)','state := clarificationSlotView(definition, pattern, slot, input.Locale)\n state.Specificity, state.Order = ranked.specificity, slotIndex\n if pattern.Policy != nil {state.Priority=pattern.Policy.Priority}')

create('internal/semantics/clarification_group.go', '''package semantics

// ClarificationEffectsConflict compares reviewed constraints known by the
// caller to address the same physical field and aggregation level. It does not
// infer mappings or grant authority. Unknown or oversized input fails closed.
func ClarificationEffectsConflict(effects []ClarificationEffect) bool {
 if len(effects)>64{return true}
 for i,a:=range effects {
  if a.Kind=="" || a.Operator=="" || a.Nulls=="" {return true}
  for _,b:=range effects[i+1:] {
   if clarificationScalarKind(a.Kind)!=clarificationScalarKind(b.Kind) || a.Unit!=b.Unit || a.Calendar!=b.Calendar || a.TimeZone!=b.TimeZone || a.TemporalType!=b.TemporalType || a.Nulls=="only" && b.Nulls=="exclude" || b.Nulls=="only" && a.Nulls=="exclude" {return true}
  }
 }
 return false
}

// ClarificationResolutionGroupConflict applies exact interval/null semantics
// after physical field equivalence has been established by the binding owner.
func ClarificationResolutionGroupConflict(group []ClarificationResolution) bool {
 effects:=make([]ClarificationEffect,0,len(group))
 for _,r:=range group {if r.Effect==nil {return true}; effects=append(effects,*r.Effect)}
 return ClarificationEffectsConflict(effects) || clarificationValueGroupConflict(group)
}
''')
create('internal/nlqroute/clarification_groups.go', '''package nlqroute

import (
 "sort"
 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/semantics"
)

type clarificationPhysicalField struct { Dataset,Column,Aggregation string }
type clarificationPhysicalGroup struct {
 Effects []semantics.ClarificationEffect
 Questions []semantics.ClarificationSlotOutcome
 Resolutions []semantics.ClarificationResolution
}

// Cross-topic policies are composed by exact live physical mapping, never by
// display label or independently budgeted topic groups. Aggregate and row-level
// predicates stay distinct. Conflicts precede provider work and plan issuance.
func crossTopicClarificationConflict(admitted []admittedTopic,locale nlq.Language) error {
 groups:=map[clarificationPhysicalField]*clarificationPhysicalGroup{}
 for _,item:=range admitted {
  if item.clarifications==nil {continue}
  for _,slot:=range item.clarifications.Slots {
   if slot.Effect==nil || slot.Outcome==semantics.ClarificationNotApplicable || !slot.Required && !slot.Defaulted && slot.Outcome!=semantics.ClarificationSatisfied {continue}
   var resolved *semantics.ClarificationResolution
   for i:=range item.clarifications.Resolutions {r:=&item.clarifications.Resolutions[i]; if r.Pattern==slot.Pattern && r.Slot==slot.Slot {resolved=r;break}}
   fragment:=semantics.ClarificationResolution{Effect:slot.Effect}
   if resolved!=nil {fragment=*resolved}
   mapped,err:=businessConstraint(item,fragment)
   if err!=nil{return err}
   key:=clarificationPhysicalField{mapped.Dataset,mapped.Column,mapped.Aggregation}
   group:=groups[key]
   if group==nil {group=&clarificationPhysicalGroup{};groups[key]=group}
   group.Effects=append(group.Effects,*slot.Effect)
   group.Questions=append(group.Questions,slot)
   if resolved!=nil {group.Resolutions=append(group.Resolutions,*resolved)}
  }
 }
 failure:=&Clarification{Reason:"conflicting_policies",Outcome:semantics.ClarificationConflicting}
 for _,group:=range groups {
  if !semantics.ClarificationEffectsConflict(group.Effects) && !semantics.ClarificationResolutionGroupConflict(group.Resolutions){continue}
  for _,question:=range group.Questions {
   question.Outcome,question.Reason=semantics.ClarificationConflicting,"conflicting_policies"
   failure.Questions=append(failure.Questions,question)
  }
 }
 if len(failure.Questions)==0{return nil}
 sortClarificationQuestions(failure.Questions)
 for _,q:=range failure.Questions {
  message:="Reviewed policies impose incompatible constraints on the same field. Resolve the policy conflict before continuing."
  if locale==nlq.LanguageSpanish {message="Las políticas revisadas imponen restricciones incompatibles sobre el mismo campo. Resolvé el conflicto antes de continuar."}
  failure.Errors=append(failure.Errors,semantics.ClarificationFieldError{Topic:q.Topic,Pattern:q.Pattern,Slot:q.Slot,Field:"policy",Code:"conflicting_policies",Message:message})
 }
 return failure
}

func sortClarificationQuestions(questions []semantics.ClarificationSlotOutcome) {
 sort.SliceStable(questions,func(i,j int)bool{
  a,b:=questions[i],questions[j]
  if a.Required!=b.Required{return a.Required}
  if a.Specificity!=b.Specificity{return a.Specificity>b.Specificity}
  if a.Priority!=b.Priority{return a.Priority>b.Priority}
  if a.Topic!=b.Topic{return a.Topic<b.Topic}
  if a.Pattern!=b.Pattern{return a.Pattern<b.Pattern}
  if a.Order!=b.Order{return a.Order<b.Order}
  return a.Slot<b.Slot
 })
}
''')
edit('internal/nlqroute/clarification.go','''	if missing {
		clarification := &Clarification''',''' if err := crossTopicClarificationConflict(admitted,in.Locale); err != nil {return err}
	if missing {
		clarification := &Clarification''')
edit('internal/nlqroute/clarification.go','''		result.Clarification, result.Outcome = clarification, nlq.StrategyClarify
''','''  sortClarificationQuestions(clarification.Questions)
  if len(clarification.Questions)>0 {
   first:=clarification.Questions[0]
   clarification.Pattern,clarification.Slot,clarification.Prompt=first.Pattern,first.Slot,first.Prompt
   clarification.Choices=nil
   for _,choice:=range first.Choices {clarification.Choices=append(clarification.Choices,ClarificationChoice{ID:choice.ID,Label:choice.Label})}
  }
		result.Clarification, result.Outcome = clarification, nlq.StrategyClarify
''')

create('internal/semantics/clarification_group_test.go', '''package semantics

import "testing"

func TestPhysicalClarificationConjunctions(t *testing.T){
 number:=func(op,value,upper,nulls string)ClarificationResolution{return ClarificationResolution{Value:value,Upper:upper,Effect:&ClarificationEffect{Kind:"number",Operator:op,Unit:"USD",Nulls:nulls,Bounds:"[)"}}}
 for _,tc:=range []struct{name string;group []ClarificationResolution; conflict bool}{
  {"contradictory-cross-topic-bounds",[]ClarificationResolution{number("gte","10","","exclude"),number("lt","5","","exclude")},true},
  {"compatible-intersection",[]ClarificationResolution{number("gte","10","","exclude"),number("lt","20","","exclude")},false},
  {"explicit-null-intersection",[]ClarificationResolution{number("gte","10","","include"),number("lt","5","","include")},false},
  {"exclusive-adjacent-ranges",[]ClarificationResolution{number("range","0","10","exclude"),number("range","10","20","exclude")},true},
  {"unreviewed-fragment",[]ClarificationResolution{{Value:"10"}},true},
 }{t.Run(tc.name,func(t *testing.T){if ClarificationResolutionGroupConflict(tc.group)!=tc.conflict{t.Fatal("incorrect exact conjunction")}})}
 a,b:=number("gte","10","","exclude"),number("gte","10","","exclude"); b.Effect.Unit="kg"
 if !ClarificationResolutionGroupConflict([]ClarificationResolution{a,b}){t.Fatal("incompatible units composed")}
}
''')
