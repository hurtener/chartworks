from pathlib import Path


def edit(path, before, after, count=1):
    p = Path(path)
    text = p.read_text()
    if text.count(before) != count:
        raise SystemExit(f'{path}: expected {count} exact anchors, got {text.count(before)}')
    p.write_text(text.replace(before, after))


def create(path, text):
    p = Path(path)
    if p.exists():
        raise SystemExit(f'{path}: refusing overwrite')
    p.write_text(text)

# Repair the actual typed receipt assertions, not their behavioral expectations.
edit('test/acceptance/cw01_test.go', 'initial.Bindings.Validation == ""', 'initial.Bindings.Validation == nil')
edit('test/acceptance/cw01_fixture_test.go', 'out.Execution == nil || ', '')

# Detach every nested reviewed policy/effect on read, not only old choice targets.
create('internal/semantics/clarification_authoring.go', '''package semantics

// CloneRuleSetDefinition detaches the entire versioned definition, including
// conditions, effect dictionaries, defaults and dependent question ordering.
// It is a copy operation, not compilation, approval or authority.
func CloneRuleSetDefinition(definition RuleSetDefinition) RuleSetDefinition {
 return cloneRules(definition)
}
''')
p = Path('internal/semantics/rulesets/service.go')
text = p.read_text()
start = text.index('\tout := append([]semantics.ClarificationPattern(nil), published.Definition.Patterns...)', text.index('func (s *Service) Patterns('))
end = text.index('\n\treturn out, nil', start)
text = text[:start] + '\tout := semantics.CloneRuleSetDefinition(published.Definition).Patterns' + text[end:]
p.write_text(text)

# A publishable conditional group must be answerable within the shared wire cap.
edit('internal/semantics/rules_compile.go', '''	if len(p.Rules) > 256 || len(p.Patterns) > 128 {
		return invalid(CodeLimit, "ruleset")
	}
''', '''	if len(p.Rules) > 256 || len(p.Patterns) > 128 {
		return invalid(CodeLimit, "ruleset")
	}
 conditionalSlots := 0
 for _, pattern := range p.Patterns {
  if pattern.Policy != nil && !pattern.Policy.Disabled { conditionalSlots += len(pattern.Slots) }
 }
 if conditionalSlots > 64 { return invalid(CodeLimit, "patterns.slots") }
''')
# Row dimensions and aggregate measures are different semantic constraints.
edit('internal/semantics/clarification_conflicts.go', 'if target.Kind == KindColumn {', 'if target.Kind == KindColumn || target.Kind == KindMeasure {')
# Do not present dependent fields before their prerequisites are answerable.
edit('internal/nlqroute/clarification.go', 'if question.Outcome != semantics.ClarificationMissing {', 'if question.Outcome != semantics.ClarificationMissing || question.Reason == "dependency_missing" {')

create('internal/semantics/rulesets/clarification_authoring.go', '''package rulesets

import (
 "context"
 "encoding/json"
 "errors"
 "reflect"
 "sort"

 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/semantics/drafts"
 "github.com/hurtener/chartworks/internal/store"
)

// ClarificationDisposition explains how a retained pattern is interpreted.
// Imported dispositions are checked against the definition, never trusted.
type ClarificationDisposition struct {
 Pattern string `json:"pattern"`
 Version string `json:"version"`
 Status string `json:"status"`
 Explanation string `json:"explanation"`
}

// PortableClarifications carries exact reviewed references, not SQL mappings or
// authority. Importing is preview-only until the ordinary save/review/publish
// lifecycle explicitly approves a new version.
type PortableClarifications struct {
 SchemaVersion int `json:"schema_version"`
 Definition semantics.RuleSetDefinition `json:"definition"`
 RuleDigest string `json:"rule_digest"`
 Dispositions []ClarificationDisposition `json:"dispositions"`
}

type ClarificationExportRequest struct { Version string `json:"version"` }

// ClarificationImportRequest intentionally supports an exact semantic-topic
// roundtrip. A foreign topic/digest requires explicit semantic remapping and a
// new reviewed authoring draft; display labels never provide an implicit map.
type ClarificationImportRequest struct {
 Pack PortableClarifications `json:"pack"`
 Version string `json:"version"`
 LegacyDisposition string `json:"legacy_disposition,omitempty"`
 Cases []semantics.ClarificationInput `json:"cases"`
}

type ClarificationImportPreview struct {
 Definition semantics.RuleSetDefinition `json:"definition"`
 Preview ClarificationPreview `json:"preview"`
 Dispositions []ClarificationDisposition `json:"dispositions"`
 ReviewRequired bool `json:"review_required"`
}

// ClarificationImportError is content-free but carries an actionable mapping or
// legacy-disposition reason. It never echoes an imported definition or answer.
type ClarificationImportError struct { Code string `json:"code"` }
func (e *ClarificationImportError) Error() string { return "rulesets: clarification import requires review" }
func (e *ClarificationImportError) Unwrap() error { return store.ErrConflict }

func clarificationDispositions(definition semantics.RuleSetDefinition) []ClarificationDisposition {
 out := make([]ClarificationDisposition, 0, len(definition.Patterns))
 for _, pattern := range definition.Patterns {
  item := ClarificationDisposition{Pattern:pattern.ID,Version:pattern.Version,Status:semantics.ClarificationMigrationDisposition(pattern)}
  switch item.Status {
  case "conditional_v1": item.Explanation = "Reviewed conditions determine applicability; accepted answers have typed effects."
  case "reviewed_disabled": item.Explanation = "Explicitly disabled by the reviewed policy; no automatic blocking or default."
  default: item.Explanation = "Legacy reference choices remain explicit selections only; conditional activation and non-reference effects require a reviewed rewrite."
  }
  out = append(out,item)
 }
 sort.Slice(out,func(i,j int)bool{return out[i].Pattern < out[j].Pattern})
 return out
}

// ExportClarifications checks export reach on the exact retained topic before
// loading the rule payload. It performs no provider or warehouse query.
func (s *Service) ExportClarifications(ctx context.Context, e identity.Envelope, topic string, in ClarificationExportRequest) (PortableClarifications,error) {
 if ctx==nil || !identity.Identifier(topic) || !identity.Identifier(in.Version) { return PortableClarifications{},store.ErrInvalid }
 pin,err:=s.repo.RuleVersionPin(ctx,e,topic,in.Version,drafts.Read)
 if err!=nil{return PortableClarifications{},err}
 publication,err:=s.topics.ReadPublishedTopic(ctx,e,topic,pin.TopicVersion,drafts.Export)
 if err!=nil{return PortableClarifications{},err}
 published,err:=s.repo.ReadPublishedRules(ctx,e,topic,pin.RuleVersion,drafts.Read,false)
 if err!=nil{return PortableClarifications{},err}
 subject,err:=publishedSubject(publication)
 if err!=nil{return PortableClarifications{},err}
 model,err:=semantics.CompilePublishedRules(subject,published.Definition)
 if err!=nil{return PortableClarifications{},err}
 if publication.Digest!=pin.PackDigest || model.Digest()!=published.Digest {return PortableClarifications{},store.ErrConflict}
 definition:=model.Definition()
 return PortableClarifications{SchemaVersion:1,Definition:definition,RuleDigest:model.Digest(),Dispositions:clarificationDispositions(definition)},nil
}

// PreviewClarificationImport validates the complete pack and explicitly refuses
// automatic legacy activation. The result is a proposal, not a saved or approved
// rule version. The existing rule-draft endpoint supplies the actual save.
func (s *Service) PreviewClarificationImport(ctx context.Context,e identity.Envelope,topic string,in ClarificationImportRequest)(ClarificationImportPreview,error){
 if ctx==nil || in.Pack.SchemaVersion!=1 || !identity.Identifier(topic) || !identity.Identifier(in.Version) || in.Version==in.Pack.Definition.Version || len(in.Cases)<1 || len(in.Cases)>16 {return ClarificationImportPreview{},store.ErrInvalid}
 definition:=semantics.CloneRuleSetDefinition(in.Pack.Definition)
 if definition.Topic!=topic {return ClarificationImportPreview{},&ClarificationImportError{Code:"semantic_mapping_required"}}
 publication,err:=s.topics.ReadPublishedTopic(ctx,e,topic,definition.TopicVersion,drafts.Write)
 if err!=nil{return ClarificationImportPreview{},err}
 if !publication.State.Active || publication.Digest!=definition.PackDigest {return ClarificationImportPreview{},&ClarificationImportError{Code:"semantic_mapping_required"}}
 subject,err:=publishedSubject(publication)
 if err!=nil{return ClarificationImportPreview{},err}
 model,err:=semantics.CompilePublishedRules(subject,definition)
 if err!=nil{return ClarificationImportPreview{},err}
 dispositions:=clarificationDispositions(model.Definition())
 if model.Digest()!=in.Pack.RuleDigest || !reflect.DeepEqual(dispositions,in.Pack.Dispositions) {return ClarificationImportPreview{},&ClarificationImportError{Code:"portable_definition_changed"}}
 if in.LegacyDisposition!="" && in.LegacyDisposition!="preserve_reference_only" {return ClarificationImportPreview{},store.ErrInvalid}
 for _,pattern:=range definition.Patterns {
  if pattern.Policy==nil && in.LegacyDisposition!="preserve_reference_only" {return ClarificationImportPreview{},&ClarificationImportError{Code:"legacy_disposition_required"}}
 }
 definition.Version=in.Version
 for i:=range definition.Patterns {definition.Patterns[i].Provenance=semantics.RuleProvenance{Kind:semantics.ProvenanceImport,Evidence:"portable-"+in.Pack.RuleDigest}}
 preview,err:=s.PreviewClarifications(ctx,e,ClarificationPreviewRequest{Definition:definition,Cases:in.Cases})
 if err!=nil{return ClarificationImportPreview{},err}
 return ClarificationImportPreview{Definition:definition,Preview:preview,Dispositions:clarificationDispositions(definition),ReviewRequired:true},nil
}

func evaluateClarificationCases(ctx context.Context,model semantics.RuleModel,cases []semantics.ClarificationInput)([]semantics.ClarificationEvaluation,error){
 if len(cases)>16 {return nil,store.ErrInvalid}
 if len(cases)==0{return nil,nil}
 out:=make([]semantics.ClarificationEvaluation,0,len(cases))
 for _,input:=range cases {
  if err:=ctx.Err();err!=nil{return nil,err}
  // Raw case questions and submitted spellings are not stored in Comparison.
  // Only reviewed sensitivity labels, canonical resolutions and outcome data
  // leave this pure evaluator for protected evidence persistence.
  out=append(out,semantics.ResolveClarifications(model,input))
 }
 raw,err:=json.Marshal(out)
 if err!=nil || len(raw)>1<<20{return nil,store.ErrInvalid}
 return out,nil
}

func sameClarificationCases(a,b []semantics.ClarificationEvaluation) bool {
 normalize:=func(cases []semantics.ClarificationEvaluation) []any {
  out:=make([]any,0,len(cases))
  for _,evaluation:=range cases {
   slots:=make([]any,0,len(evaluation.Slots))
   for _,slot:=range evaluation.Slots {slots=append(slots,[]any{slot.Pattern,slot.Slot,slot.Outcome,slot.Reason,slot.Defaulted,slot.Prompt,slot.Why,slot.Choices,slot.Effect})}
   resolutions:=make([]any,0,len(evaluation.Resolutions))
   for _,r:=range evaluation.Resolutions {resolutions=append(resolutions,[]any{r.Pattern,r.Slot,r.Provenance,r.Reference,r.Effect,r.Time,r.Value,r.Upper,r.Null})}
   codes:=make([]any,0,len(evaluation.Errors))
   for _,field:=range evaluation.Errors {codes=append(codes,[]any{field.Pattern,field.Slot,field.Field,field.Code})}
   out=append(out,[]any{evaluation.Outcome,slots,resolutions,codes})
  }
  return out
 }
 left,le:=json.Marshal(normalize(a));right,re:=json.Marshal(normalize(b))
 return le==nil && re==nil && string(left)==string(right)
}

// IsClarificationImportError exposes a stable repair code without importing
// arbitrary payload text into a transport error.
func IsClarificationImportError(err error) string {
 var failure *ClarificationImportError
 if errors.As(err,&failure){return failure.Code}
 return ""
}
''')

edit('internal/semantics/rulesets/model.go', 'type ReplayRequest struct {', 'type ReplayRequest struct {\n ClarificationCases []semantics.ClarificationInput `json:"clarification_cases,omitempty"`')
edit('internal/semantics/rulesets/model.go', 'type ShadowRequest struct {', 'type ShadowRequest struct {\n ClarificationCases []semantics.ClarificationInput `json:"clarification_cases,omitempty"`')
edit('internal/semantics/rulesets/model.go', 'type Comparison struct {', 'type Comparison struct {\n BaselineClarifications []semantics.ClarificationEvaluation `json:"baseline_clarifications,omitempty"`\n CandidateClarifications []semantics.ClarificationEvaluation `json:"candidate_clarifications,omitempty"`')
edit('internal/semantics/rulesets/service.go', '''	base.EvaluatedAt = time.Now().UTC()
	id, err := newComparisonID()''', '''	base.EvaluatedAt = time.Now().UTC()
 clarificationCases, err := evaluateClarificationCases(ctx, model, in.ClarificationCases)
 if err != nil { return Comparison{}, err }
	id, err := newComparisonID()''')
edit('internal/semantics/rulesets/service.go', 'Comparison{ID: id, Mode: comparisonReplay,', 'Comparison{BaselineClarifications: clarificationCases, ID: id, Mode: comparisonReplay,')
edit('internal/semantics/rulesets/service.go', '''	if baseline.PackDigest != candidate.PackDigest || baseline.TopicVersion != candidate.TopicVersion {
		return Comparison{}, store.ErrConflict
	}
	id, err := newComparisonID()''', '''	if baseline.PackDigest != candidate.PackDigest || baseline.TopicVersion != candidate.TopicVersion {
		return Comparison{}, store.ErrConflict
	}
 baselineCases, err := evaluateClarificationCases(ctx, baselineModel, in.ClarificationCases)
 if err != nil { return Comparison{}, err }
 candidateCases, err := evaluateClarificationCases(ctx, candidateModel, in.ClarificationCases)
 if err != nil { return Comparison{}, err }
	id, err := newComparisonID()''')
edit('internal/semantics/rulesets/service.go', 'Comparison{ID: id, Mode: comparisonShadow,', 'Comparison{BaselineClarifications: baselineCases, CandidateClarifications: candidateCases, ID: id, Mode: comparisonShadow,')
edit('internal/semantics/rulesets/service.go', 'Changed: !sameConstraintEvaluation(baseline.Result, candidate.Result)', 'Changed: !sameConstraintEvaluation(baseline.Result, candidate.Result) || !sameClarificationCases(baselineCases, candidateCases)')

create('internal/store/postgres/migrations/036_clarification_comparison.sql', '''-- Retained clarification replay/shadow evidence shares the existing immutable,
-- tenant-composite comparison identity and exact topic/rule foreign keys.
ALTER TABLE chartworks.topic_rule_comparison_evidence
 ADD COLUMN baseline_clarification_result jsonb,
 ADD COLUMN candidate_clarification_result jsonb;
ALTER TABLE chartworks.topic_rule_comparison_evidence
 ADD CONSTRAINT baseline_clarification_bounded CHECK (
  baseline_clarification_result IS NULL OR baseline_clarification_result = 'null'::jsonb OR
  (jsonb_typeof(baseline_clarification_result) = 'array' AND
   jsonb_array_length(baseline_clarification_result) <= 16 AND
   octet_length(baseline_clarification_result::text) <= 1048576)),
 ADD CONSTRAINT candidate_clarification_bounded CHECK (
  candidate_clarification_result IS NULL OR candidate_clarification_result = 'null'::jsonb OR
  (jsonb_typeof(candidate_clarification_result) = 'array' AND
   jsonb_array_length(candidate_clarification_result) <= 16 AND
   octet_length(candidate_clarification_result::text) <= 1048576));
''')
create('internal/store/postgres/clarification_comparison.go', '''package postgres

import (
 "encoding/json"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/semantics/rulesets"
 "github.com/hurtener/chartworks/internal/store"
)

func clarificationComparisonJSON(cases []semantics.ClarificationEvaluation,pin rulesets.Evaluation)([]byte,error){
 if len(cases)>16{return nil,store.ErrInvalid}
 for _,evaluation:=range cases {
  if evaluation.SchemaVersion!=semantics.ClarificationSchemaVersion{return nil,store.ErrInvalid}
  for _,r:=range evaluation.Resolutions {
   if r.Topic!=pin.Topic || r.TopicVersion!=pin.TopicVersion || r.RulesetVersion!=pin.RuleVersion || r.RulesetDigest!=pin.RuleDigest || r.PackDigest!=pin.PackDigest || (r.Sensitivity!=semantics.LiteralSensitive && r.Sensitivity!=semantics.LiteralNonSensitive){return nil,store.ErrInvalid}
  }
 }
 raw,err:=json.Marshal(cases)
 if err!=nil || len(raw)>1<<20{return nil,store.ErrInvalid}
 return raw,nil
}
''')
edit('internal/store/postgres/rulesets.go', '''	created := comparison.CreatedAt
''', ''' baselineClarifications, err := clarificationComparisonJSON(comparison.BaselineClarifications, comparison.Baseline)
 if err != nil { return rulesets.Comparison{}, err }
 var candidateClarifications []byte
 if comparison.Candidate != nil {
  candidateClarifications, err = clarificationComparisonJSON(comparison.CandidateClarifications, *comparison.Candidate)
  if err != nil { return rulesets.Comparison{}, err }
 } else if len(comparison.CandidateClarifications) != 0 { return rulesets.Comparison{}, store.ErrInvalid }
	created := comparison.CreatedAt
''')
edit('internal/store/postgres/rulesets.go', 'candidate_result,changed,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11::jsonb,$12,$13::jsonb,$14,$15)', 'candidate_result,changed,created_at,baseline_clarification_result,candidate_clarification_result) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10,$11::jsonb,$12,$13::jsonb,$14,$15,$16::jsonb,$17::jsonb)')
edit('internal/store/postgres/rulesets.go', 'candidateResult, comparison.Changed, created)', 'candidateResult, comparison.Changed, created, baselineClarifications, candidateClarifications)')

# Register every new operation with the existing action, resource, schema,
# effect and audit registry instead of adding a separate authoring application.
edit('internal/topicapi/http.go', '''		{"POST", "/v1/topics/{id}/rule-patterns/read",''', '''		{"POST", "/v1/topics/{id}/clarifications/preview", "previewClarifications", "topics.write", "deterministic_draft_preview", "rulesets.Service.PreviewClarifications", "read_only_no_domain_audit", reflect.TypeFor[rulesets.ClarificationPreviewRequest](), reflect.TypeFor[rulesets.ClarificationPreview]()},
  {"POST", "/v1/topics/{id}/clarifications/export", "exportClarifications", "topics.export", "retained_metadata_export", "rulesets.Service.ExportClarifications", "read_only_no_domain_audit", reflect.TypeFor[rulesets.ClarificationExportRequest](), reflect.TypeFor[rulesets.PortableClarifications]()},
  {"POST", "/v1/topics/{id}/clarifications/import-preview", "previewClarificationImport", "topics.write", "deterministic_import_preview", "rulesets.Service.PreviewClarificationImport", "read_only_no_domain_audit", reflect.TypeFor[rulesets.ClarificationImportRequest](), reflect.TypeFor[rulesets.ClarificationImportPreview]()},
		{"POST", "/v1/topics/{id}/rule-patterns/read",''')
edit('internal/topicapi/http.go', '''				"getPublishedRulePatterns":''', '''    "previewClarifications": "Preview conditional questions and typed effects without publication",
    "exportClarifications": "Export exact reviewed rules with explicit clarification migration dispositions",
    "previewClarificationImport": "Validate an exact-topic portable ruleset before a new reviewed draft",
				"getPublishedRulePatterns":''')
edit('internal/topicapi/http.go', 'case "saveRuleDraft", "reviewRules",', 'case "previewClarifications", "exportClarifications", "previewClarificationImport", "saveRuleDraft", "reviewRules",')
edit('internal/topicapi/http.go', '''		case "getPublishedRulePatterns":
''', '''  case "previewClarifications":
   var in rulesets.ClarificationPreviewRequest
   if err = decode(&in); err == nil {
    if in.Definition.Topic != id { err = store.ErrInvalid } else { out, err = rules.PreviewClarifications(r.Context(), e, in) }
   }
  case "exportClarifications":
   var in rulesets.ClarificationExportRequest
   if err = decode(&in); err == nil { out, err = rules.ExportClarifications(r.Context(), e, id, in) }
  case "previewClarificationImport":
   var in rulesets.ClarificationImportRequest
   if err = decode(&in); err == nil { out, err = rules.PreviewClarificationImport(r.Context(), e, id, in) }
		case "getPublishedRulePatterns":
''')
edit('internal/topicapi/http.go', '''	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{code})''', '''	_ = json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
   Reason string `json:"reason,omitempty"`
	}{code, rulesets.IsClarificationImportError(err)})''')

create('sdk/chartworks/clarification_authoring.go', '''package chartworks

import (
 "context"
 "errors"
 "github.com/hurtener/chartworks/internal/semantics"
 "github.com/hurtener/chartworks/internal/semantics/rulesets"
)

type ClarificationInput = semantics.ClarificationInput
type ClarificationPreviewRequest = rulesets.ClarificationPreviewRequest
type ClarificationPreview = rulesets.ClarificationPreview
type ClarificationExportRequest = rulesets.ClarificationExportRequest
type PortableClarifications = rulesets.PortableClarifications
type ClarificationImportRequest = rulesets.ClarificationImportRequest
type ClarificationImportPreview = rulesets.ClarificationImportPreview

// PreviewClarifications evaluates bounded synthetic cases through the same
// reviewed compiler as routing. It cannot activate a draft or issue a plan.
func(c *Client) PreviewClarifications(ctx context.Context,topic string,in ClarificationPreviewRequest)(out ClarificationPreview,err error){
 if !wireID(topic){return out,errors.New("chartworks: invalid topic")}
 err=c.callLimit(ctx,"POST","/v1/topics/"+topic+"/clarifications/preview","",in,&out,2<<20)
 return
}

func(c *Client) ExportClarifications(ctx context.Context,topic string,in ClarificationExportRequest)(out PortableClarifications,err error){
 if !wireID(topic){return out,errors.New("chartworks: invalid topic")}
 err=c.callLimit(ctx,"POST","/v1/topics/"+topic+"/clarifications/export","",in,&out,2<<20)
 return
}

func(c *Client) PreviewClarificationImport(ctx context.Context,topic string,in ClarificationImportRequest)(out ClarificationImportPreview,err error){
 if !wireID(topic){return out,errors.New("chartworks: invalid topic")}
 err=c.callLimit(ctx,"POST","/v1/topics/"+topic+"/clarifications/import-preview","",in,&out,2<<20)
 return
}
''')
