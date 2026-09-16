#!/usr/bin/env python3
"""One-shot, fingerprint-guarded CW-01 integration; removed by its preparation job."""
from pathlib import Path
import hashlib

root = Path(__file__).resolve().parents[2]
def read(path, expected=None):
    data = (root/path).read_bytes()
    if expected:
        actual = hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest()
        if actual != expected:
            raise SystemExit(f'Concurrent or unexpected source change: {path} ({actual})')
    return data.decode()
def replace(text, old, new, count=1):
    if text.count(old) != count:
        raise SystemExit(f'Expected {count} exact anchors, found {text.count(old)}: {old[:100]!r}')
    return text.replace(old, new)
def save(path, text):
    (root/path).write_text(text)

path='internal/nlqroute/service.go'
s=read(path,'41927b6f177e01914409b189fa8e076c3f37d96a')
s=replace(s,'type RouteRequest struct {','type RouteRequest struct {\n\tAnswers []semantics.ClarificationAnswer `json:"answers,omitempty"`\n\tAnswerContext string `json:"answer_context,omitempty"`')
s=replace(s,'type Clarification struct {','type Clarification struct {\n\tOutcome semantics.ClarificationOutcome `json:"outcome,omitempty"`\n\tWhy string `json:"why,omitempty"`\n\tQuestions []semantics.ClarificationSlotOutcome `json:"questions,omitempty"`\n\tErrors []semantics.ClarificationFieldError `json:"errors,omitempty"`')
s=replace(s,'type RouteResult struct {','type RouteResult struct {\n\tAnswerContext string `json:"answer_context,omitempty"`\n\tSourceBindingDigest string `json:"source_binding_digest,omitempty"`\n\tClarifications []semantics.ClarificationEvaluation `json:"clarifications,omitempty"`\n\tResolutions []semantics.ClarificationResolution `json:"resolutions,omitempty"`\n\tbusiness []readexec.BusinessConstraint\n\tresolutionSeal string')
s=replace(s,'type admittedTopic struct {','type admittedTopic struct {\n\tclarifications *semantics.ClarificationEvaluation\n\tbinding *readexec.Binding')
start=s.index('\t\titem.constraints, item.advisory, err = s.resolveRules(')
end=s.index('\t\tadmitted = append(admitted, item)', start)
s=s[:start]+s[end:]
anchor='\tmetrics, err := resolveMetrics(admitted, in.MetricIDs)'
replacement='''	if !contextMatches(admitted, in.Context) { return RouteResult{}, readexec.ErrBinding }
	if err := s.prepareClarifications(ctx, e, in, admitted, &result); err != nil { return RouteResult{}, err }
	if result.Clarification != nil { return result, nil }
	in = cloneRouteRequest(result.Request)
	for i := range admitted {
		item := &admitted[i]
		item.constraints, item.advisory, err = s.resolveRules(ctx, e, in, choiceValues, *item)
		if err != nil {
			var clarification *Clarification
			if errors.As(err, &clarification) { result.Clarification, result.Outcome = clarification, nlq.StrategyClarify; return result, nil }
			return RouteResult{}, err
		}
	}
	metrics, err := resolveMetrics(admitted, selectedClarificationMetrics(in.MetricIDs, result.Resolutions))'''
s=replace(s,anchor,replacement)
s=replace(s,'\tresources := routeResources(e, admitted)','\tminimumTier, err := s.preflightClarificationBudget(ctx, in, admitted, result, metrics)\n\tif err != nil { return RouteResult{}, err }\n\tresources := routeResources(e, admitted)')
s=replace(s,'\tassembled, err := s.assembler.AssembleForConfidence(ctx, input, result.Confidence)','\ttier, err := nlq.TierForConfidence(result.Confidence)\n\tif err != nil { return RouteResult{}, err }\n\tif minimumTier.Budget() > tier.Budget() { tier = minimumTier }\n\tassembled, err := s.assembler.Assemble(ctx, input, tier)')
s=replace(s,'\treturn result, nil\n}\n\nfunc admissionVector','\tresult.resolutionSeal = result.resolutionProof()\n\treturn result, nil\n}\n\nfunc admissionVector')
s=replace(s,'len(in.Choices) > 64 {','len(in.Choices) > 64 || len(in.Answers) > 64 || in.AnswerContext != "" && !topics.DigestValid(in.AnswerContext) {')
s=replace(s,'\tout.Choices = append([]ChoiceSelection(nil), in.Choices...)','\tout.Choices = append([]ChoiceSelection(nil), in.Choices...)\n\tout.Answers = semantics.CloneClarificationAnswers(in.Answers)')
start=s.index('func (s *Service) resolveRules(')
end=s.index('\nfunc choiceMap(',start)
s=s[:start]+'''func (s *Service) resolveRules(ctx context.Context, e identity.Envelope, in RouteRequest, _ map[string]string, item admittedTopic) (*nlq.ConstraintState, []nlq.OptionalItem, error) {
	if !item.hasRules {
		if len(in.References) > 0 { return nil, nil, ErrInvalid }
		return nil, nil, nil
	}
	refs := append([]semantics.Reference(nil), in.References...)
	if item.clarifications != nil { refs = append([]semantics.Reference(nil), item.clarifications.References...) }
	if len(refs) == 0 {
		if len(item.publication.Definition.Datasets) == 0 { return nil, nil, store.ErrConflict }
		ids := make([]string, 0, len(item.publication.Definition.Datasets))
		for _, dataset := range item.publication.Definition.Datasets { ids = append(ids, dataset.ID) }
		sort.Strings(ids)
		refs = append(refs, semantics.Reference{Kind: semantics.KindDataset, ID: ids[0]})
	}
	evaluation, err := s.rules.Evaluate(ctx, e, item.id, rulesets.EvaluateRequest{References: refs})
	if err != nil { return nil, nil, err }
	if evaluation.TopicVersion != item.publication.State.Version || evaluation.PackDigest != item.publication.Digest || evaluation.RuleVersion != item.rules.State.Version || evaluation.RuleDigest != item.rules.Digest { return nil, nil, store.ErrConflict }
	state := &nlq.ConstraintState{Allowed: evaluation.Result.Allowed}
	for _, ref := range evaluation.Result.Required { state.Required = append(state.Required, nlq.MandatoryConstraint{ID: referenceID(ref), Kind: "required", Text: referenceText(ref)}) }
	for _, ref := range evaluation.Result.Excluded { state.Excluded = append(state.Excluded, nlq.MandatoryConstraint{ID: referenceID(ref), Kind: "excluded", Text: referenceText(ref)}) }
	if !state.Allowed { return nil, nil, &Clarification{Reason: "mandatory_constraint", Outcome: semantics.ClarificationMissing, Prompt: "Choose a metric or dimension that satisfies the published rules."} }
	if item.clarifications != nil {
		state, err = appendResolvedConstraints(state, item.clarifications.Resolutions, false)
		if err != nil { return nil, nil, err }
	}
	var advisory []nlq.OptionalItem
	for _, rule := range item.rules.Definition.Rules {
		if rule.Class != semantics.RuleAdvisoryContext || rule.Guidance == nil || rule.Guidance.Sensitivity != semantics.LiteralNonSensitive { continue }
		advisory = append(advisory, nlq.OptionalItem{ID: rule.ID, Text: rule.Guidance.Text, Priority: rule.Priority, Source: "rules"})
	}
	sort.Slice(advisory, func(i, j int) bool { return advisory[i].ID < advisory[j].ID })
	return state, advisory, nil
}
''' +s[end:]
save(path,s)

path='internal/semantics/clarification_compile.go'
s=read(path,'3af31599a78d184f40b84395b8865a00a3f6871c')
s=replace(s,'func orderedClarificationSlots(slots []ClarificationSlot) ([]ClarificationSlot, error) {','func orderedClarificationSlots(slots []ClarificationSlot) ([]ClarificationSlot, error) {\n\tslots = clarificationPresentationOrder(slots)')
save(path,s)

path='internal/exec/business_model.go'
s=read(path)
s=replace(s,'\tKind string `json:"kind"`','\tKind string `json:"kind"`\n\tAggregation string `json:"aggregation,omitempty"`')
s=replace(s,'\tcase "number":\n\t\treturn category ==','\tcase "number":\n\t\tif c.Aggregation == "count" || c.Aggregation == "distinct_count" { return true }\n\t\treturn category ==')
save(path,s)

path='internal/nlqroute/clarification.go'
s=read(path)
s=replace(s,'kept := constraints.Required[:0]','kept := make([]nlq.MandatoryConstraint, 0, len(constraints.Required))')
save(path,s)
print('Applied exact CW-01 integration edits; no tests have been claimed by this script.')
