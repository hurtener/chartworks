from pathlib import Path

def edit(path, before, after):
    p=Path(path)
    s=p.read_text()
    if s.count(before)!=1:
        raise SystemExit(f'{path}: expected one patch anchor, found {s.count(before)}')
    p.write_text(s.replace(before,after,1))

p=Path('internal/nlqroute/service.go')
s=p.read_text()
start=s.index('\tchoiceValues, err := choiceMap(in.Choices)',s.index('func (s *Service) Route('))
end=s.index('\n\tresources := routeResources(e, admitted)',start)
s=s[:start]+'''\tadmitted, result, metrics, err := s.prepareClarifications(ctx, e, in, topicsIDs)
\tif err != nil { return RouteResult{}, err }
\tif result.Clarification != nil { return result, nil }
'''+s[end:]
start=s.index('func (s *Service) resolveRules(')
end=s.index('\nfunc choiceMap(',start)
s=s[:start]+s[end:]
p.write_text(s)
edit('internal/nlqroute/service.go','type RouteRequest struct {','type RouteRequest struct {\n Answers []semantics.ClarificationAnswer `json:"answers,omitempty"`')
edit('internal/nlqroute/service.go','type Clarification struct {','type Clarification struct {\n Outcome semantics.ClarificationOutcome `json:"outcome,omitempty"`\n Questions []semantics.ClarificationSlotOutcome `json:"questions,omitempty"`\n Errors []semantics.ClarificationFieldError `json:"errors,omitempty"`')
edit('internal/nlqroute/service.go','type RouteResult struct {','type RouteResult struct {\n Resolutions []semantics.ClarificationResolution `json:"resolutions,omitempty"`\n ClarificationOutcomes []semantics.ClarificationSlotOutcome `json:"clarification_outcomes,omitempty"`\n ClarificationPins *ClarificationPins `json:"clarification_pins,omitempty"`\n Dispositions []string `json:"clarification_dispositions,omitempty"`')
edit('internal/nlqroute/service.go','\tout := in\n\tout.Topics =','\tout := in\n out.Answers = CloneAnswers(in.Answers)\n\tout.Topics =')
edit('internal/nlqroute/service.go','func normalizeRequest(in RouteRequest) ([]string, error) {','''func normalizeRequest(in RouteRequest) ([]string, error) {
 if len(in.Answers)+len(in.Choices)>64 {return nil,ErrInvalid}
 for _,a:=range in.Answers {
  if !identity.Identifier(a.Topic) || !identity.Identifier(a.TopicVersion) || !identity.Identifier(a.RulesetVersion) || !identity.Identifier(a.Pattern) || !identity.Identifier(a.PatternVersion) || !identity.Identifier(a.Slot) || a.Remove || a.Value==nil {return nil,ErrInvalid}
 }
 rawAnswers,answerErr:=json.Marshal(in.Answers)
 if answerErr!=nil || len(rawAnswers)>65536 {return nil,ErrInvalid}
''')
edit('internal/nlqroute/service.go','\tassembled, err := s.assembler.AssembleForConfidence(ctx, input, result.Confidence)',''' var assembled nlq.AssembledContext
 if len(result.Resolutions)>0 {assembled,err=s.assembler.Assemble(ctx,input,nlq.TierHigh)} else {assembled,err=s.assembler.AssembleForConfidence(ctx,input,result.Confidence)}''')
# The semantic resolver and route use the same reference bound.
edit('internal/semantics/clarification_evaluate.go','len(input.References) > 128','len(input.References) > 256')
