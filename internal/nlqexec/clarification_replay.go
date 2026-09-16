package nlqexec

import (
 "context"
 "encoding/json"
 "sort"

 "github.com/hurtener/chartworks/internal/exec"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/nlq"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
)

func staleClarification(locale nlq.Language) *nlqroute.Clarification {
 message:="The topic, policy or source context changed. Reevaluate the question and confirm the current answers."
 if locale==nlq.LanguageSpanish {message="Cambió el tema, la política o el contexto de origen. Volvé a evaluar la pregunta y confirmá las respuestas actuales."}
 return &nlqroute.Clarification{Reason:"stale_answer",Outcome:semantics.ClarificationInvalid,Errors:[]semantics.ClarificationFieldError{{Field:"version",Code:"stale_answer",Message:message}}}
}

func answerKey(a semantics.ClarificationAnswer) string {return a.Topic+"\x00"+a.Pattern+"\x00"+a.Slot}

// validateAnswerEdits is atomic. Unknown removals, duplicate edits and a changed
// version cannot turn a required answer into an implicit skip.
func validateAnswerEdits(old QueryRecord,delta QuestionRequest) error {
 if len(delta.Answers)>64 {return ErrInvalid}
 if delta.Context!="" && delta.Context!=old.Context {return ErrForeignSession}
 if delta.Topic!="" && delta.Topic!=old.Topic {return ErrForeignSession}
 if len(delta.Topics)>0 && !sameStrings(delta.Topics,old.Topics) {return ErrForeignSession}
 seen:=map[string]bool{}
 previous:=map[string]semantics.ClarificationAnswer{}
 for _,a:=range old.Route.Request.Answers {previous[answerKey(a)]=a}
 for _,a:=range delta.Answers {
  key:=answerKey(a)
  if seen[key] {return ErrInvalid};seen[key]=true
  if !a.Remove {continue}
  p,exists:=previous[key]
  if !exists || a.Value!=nil || p.TopicVersion!=a.TopicVersion || p.RulesetVersion!=a.RulesetVersion || p.PatternVersion!=a.PatternVersion {return staleClarification(old.Locale)}
 }
 return nil
}

func mergeAnswers(base,delta []semantics.ClarificationAnswer) []semantics.ClarificationAnswer {
 answers:=map[string]semantics.ClarificationAnswer{}
 for _,a:=range nlqroute.CloneAnswers(base) {answers[answerKey(a)]=a}
 for _,a:=range nlqroute.CloneAnswers(delta) {
  if a.Remove {delete(answers,answerKey(a))} else {answers[answerKey(a)]=a}
 }
 keys:=make([]string,0,len(answers));for key:=range answers {keys=append(keys,key)};sort.Strings(keys)
 var out []semantics.ClarificationAnswer
 for _,key:=range keys {out=append(out,answers[key])}
 return out
}

// recheckStoredClarifications never uses retained bearer material and never
// calls a model. Current signed topic/source/context reach is checked by the
// ordinary router and source seams before reuse of any business resolution.
func (s *Service) recheckStoredClarifications(ctx context.Context,e identity.Envelope,q QueryRecord) error {
 if q.Route.ClarificationPins==nil && len(q.Route.Resolutions)==0 {return nil}
 pins:=q.Route.ClarificationPins
 if pins==nil || pins.Tenant!=e.Tenant() || pins.Actor!=e.User() || pins.Session!=e.Session() || pins.Context!=q.Context {return ErrForeignSession}
 checker,ok:=s.router.(clarificationRechecker)
 if !ok{return ErrClarificationUnsupported}
 current,err:=checker.Recheck(ctx,e,q.Route.Request)
 if err!=nil{return err}
 if current.Clarification!=nil && (q.Status!="preflight" || current.Clarification.Outcome==semantics.ClarificationInvalid || current.Clarification.Outcome==semantics.ClarificationConflicting) {return current.Clarification}
 if !sameClarificationEvidence(q.Route,current) || !sameStrings(current.RuleVersions,q.RuleVersions) {return staleClarification(q.Locale)}
 return nil
}

func sealClarificationBinding(route *nlqroute.RouteResult,binding exec.Binding,candidate generatedCandidate,proof exec.Plan) error {
 if len(route.Resolutions)==0{return nil}
 receipt:=proof.Receipt()
 if !receipt.Validated {return ErrClarificationBinding}
 ids:=make([]string,0,len(route.Resolutions))
 for _,r:=range route.Resolutions {if r.ID!=semantics.ClarificationResolutionDigest(r){return ErrClarificationBinding};ids=append(ids,r.ID)}
 route.ConstraintBinding=&nlqroute.ConstraintBindingReceipt{SchemaVersion:1,ResolutionIDs:ids,SourceBinding:exec.Hash(binding),Statement:exec.Hash(candidate.SQL),Parameters:exec.Hash(candidate.Parameters),ReadManifest:receipt.Manifest}
 return nil
}

func verifyClarificationBinding(route nlqroute.RouteResult,binding exec.Binding,sql string,parameters []exec.Parameter) error {
 if len(route.Resolutions)==0 {
  if route.ConstraintBinding!=nil{return ErrClarificationBinding}
  return nil
 }
 receipt:=route.ConstraintBinding
 if receipt==nil || receipt.SchemaVersion!=1 || receipt.SourceBinding!=exec.Hash(binding) || receipt.Statement!=exec.Hash(sql) || receipt.Parameters!=exec.Hash(parameters) || receipt.ReadManifest=="" || len(receipt.ResolutionIDs)!=len(route.Resolutions) {return ErrClarificationBinding}
 for i,r:=range route.Resolutions {if r.ID!=semantics.ClarificationResolutionDigest(r) || receipt.ResolutionIDs[i]!=r.ID{return ErrClarificationBinding}}
 return nil
}

// ValidateClarificationRecord is the persistence boundary for versioned
// clarification evidence. Legacy records remain readable; unknown versions and
// forged partition coordinates cannot enter the shared query store.
func ValidateClarificationRecord(q QueryRecord,tenant,actor string) error {
 r:=q.Route
 if len(r.Resolutions)==0 && r.ClarificationPins==nil && r.ConstraintBinding==nil {return nil}
 if r.ClarificationPins==nil || r.ClarificationPins.SchemaVersion!=1 || r.ClarificationPins.Tenant!=tenant || r.ClarificationPins.Actor!=actor || r.ClarificationPins.Session!=q.Session || r.ClarificationPins.Context!=q.Context {return ErrClarificationBinding}
 if len(r.Resolutions)>64 || len(r.Request.Answers)>64 {return ErrInvalid}
 for _,v:=range r.Resolutions {
  if v.SchemaVersion!=1 || v.ID=="" || v.ID!=semantics.ClarificationResolutionDigest(v) || (v.Sensitivity!=semantics.LiteralNonSensitive && v.Sensitivity!=semantics.LiteralSensitive) {return ErrClarificationBinding}
 }
 // Persist only canonical inputs emitted by the resolver, never raw localized
 // answers or caller removal instructions. Defaults are reevaluated on resume.
 canonical:=semantics.CanonicalClarificationAnswers(r.Resolutions)
 a,_:=json.Marshal(canonical);b,_:=json.Marshal(r.Request.Answers)
 if string(a)!=string(b) || len(r.Request.Choices)!=0 {return ErrClarificationBinding}
 if q.SQL!="" && len(r.Resolutions)>0 && (r.ConstraintBinding==nil || r.ConstraintBinding.SchemaVersion!=1 || r.ConstraintBinding.Statement!=exec.Hash(q.SQL) || r.ConstraintBinding.Parameters!=exec.Hash(q.Parameters)) {return ErrClarificationBinding}
 return nil
}
