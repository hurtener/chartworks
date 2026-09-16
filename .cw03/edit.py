from pathlib import Path
import re

def edit(path, old, new, count=1):
    p=Path(path);s=p.read_text()
    if old not in s:
        if new in s:return
        raise RuntimeError(f'missing exact edit in {path}: {old[:90]!r}')
    if count and s.count(old)!=count:raise RuntimeError(f'ambiguous edit {path}: {old[:70]!r}')
    p.write_text(s.replace(old,new))

def field(path, kind, declaration):
    p=Path(path);s=p.read_text();pattern=rf'(type {kind} struct \{{\n)(.*?)(\n\}})'
    m=re.search(pattern,s,re.S)
    if not m:raise RuntimeError(f'missing type {kind} in {path}')
    if declaration.split()[0] in [line.strip().split()[0] for line in m[2].splitlines() if line.strip()]:return
    s=s[:m.start()]+m[1]+m[2]+'\n\t'+declaration+m[3]+s[m.end():];p.write_text(s)

def append(path, text):
    p=Path(path);s=p.read_text()
    if text.strip() not in s:p.write_text(s+'\n'+text)

append('internal/reporting/output_intent.go','const CurrentSchemaVersion = 2\n')
append('internal/reporting/run_policy.go','const LegacyFrozenVersion = "frozen-block-run-v1"\n')
field('internal/semantics/model.go','Column','Sensitivity LiteralSensitivity `json:"sensitivity,omitempty" jsonschema:"enum=non_sensitive,enum=sensitive"`')
field('internal/semantics/portable.go','PortableColumn','Sensitivity LiteralSensitivity `json:"sensitivity,omitempty" jsonschema:"enum=non_sensitive,enum=sensitive"`')
edit('internal/semantics/portable.go','PortableColumn{Slot: ref.ID, Name: column.Name, Category: column.Category, Nullable: column.Nullable}', 'PortableColumn{Slot: ref.ID, Name: column.Name, Category: column.Category, Nullable: column.Nullable, Sensitivity: column.Sensitivity}')
# Carry classification into the ordinary, untrusted imported draft; approval is unchanged.
p=Path('internal/semantics/portable.go');s=p.read_text()
needle='bound.Columns = append(bound.Columns, Column{'
start=s.index(needle);end=s.index('})',start)
if 'Sensitivity:' not in s[start:end]:s=s[:end]+', Sensitivity: column.Sensitivity'+s[end:]
p.write_text(s)
field('internal/exec/results.go','Field','Sensitivity semantics.LiteralSensitivity `json:"sensitivity,omitempty" jsonschema:"enum=non_sensitive,enum=sensitive"`')
edit('internal/exec/results.go','import (','import (\n "github.com/hurtener/chartworks/internal/semantics"')
field('internal/exec/contracts.go','Candidate','lineage []OutputLineage')
field('internal/exec/contracts.go','Receipt','Lineage []OutputLineage `json:"lineage,omitempty"`')
edit('internal/exec/contracts.go','\treturn Receipt{Validated: true, Source: c.binding.Source,','\tif len(c.lineage)>0 { manifest=append(manifest,c.lineage) }\n\treturn Receipt{Lineage: cloneLineage(c.lineage), Validated: true, Source: c.binding.Source,')
edit('internal/exec/validator.go','columns: append([]string(nil), columns...), checked: true}', 'columns: append([]string(nil), columns...), lineage: postgresLineage(statement, scoped, columns), checked: true}')
append('internal/exec/caps.go','''// ConsumerCaps exposes ceilings, not execution authority. ExecuteCapped checks again.
func (x *Executor) ConsumerCaps() (Caps,int) {
 if x==nil{return Caps{},0}
 return Caps{Rows:x.settings.RowsCeiling,Bytes:x.settings.BytesCeiling,Timeout:time.Duration(x.settings.Timeout),PlannerCost:x.settings.PlannerCostCeiling},x.settings.MaxReadAttempts
}
''')
field('internal/reporting/model.go','Narrative','MaxClaims int `json:"max_claims,omitempty"`')
field('internal/reporting/model.go','Output','Intent *OutputIntent `json:"intent,omitempty"`')
field('internal/reporting/model.go','Definition','QueryLimits *QueryLimits `json:"query_limits,omitempty"`')
field('internal/reporting/model.go','View','SchemaVersion int `json:"schema_version"`')
field('internal/reporting/model.go','View','QueryLimits *QueryLimits `json:"query_limits,omitempty"`')
field('internal/reporting/model.go','View','ResultPolicy []FieldPolicy `json:"result_policy,omitempty"`')
field('internal/reporting/model.go','Evidence','ResultPolicy []FieldPolicy `json:"result_policy,omitempty"`')
field('internal/reporting/model.go','Evidence','OutputIntent []OutputDescriptor `json:"output_intent,omitempty"`')
edit('internal/reporting/model.go','jsonschema:"enum=1"','jsonschema:"enum=1,enum=2"',count=0)
edit('internal/reporting/definition.go','}{CanonicalizationVersion, d})','}{DefinitionCanonicalVersion(d), d})')
edit('internal/reporting/definition.go','func ExecutionDigest(d Definition) string {','func ExecutionDigest(d Definition) string {\n if d.SchemaVersion == CurrentSchemaVersion {return digest([]any{DefinitionCanonicalVersion(d),d.Source,d.Context,d.Topics,d.Template,d.SQL,d.Parameters,d.ExpectedSchema,d.QueryLimits})}')
edit('internal/reporting/definition.go','d.SchemaVersion != SchemaVersion ||','(d.SchemaVersion != SchemaVersion && d.SchemaVersion != CurrentSchemaVersion) ||')
edit('internal/reporting/definition.go','\tencoded, err := json.Marshal(d)','\tif err:=validateOutputPolicies(d,limits);err!=nil{return err}\n\tencoded, err := json.Marshal(d)')
edit('internal/reporting/definition.go','strings.TrimSpace(n.Instructions) == "" ||','(strings.TrimSpace(n.Instructions) == "" && n.MaxClaims == 0) || n.MaxClaims < 0 || n.MaxClaims > 32 ||')
edit('internal/reporting/definition.go','\tseen := map[string]bool{}\n\tfor _, names := range [][]string{n.Fields, n.RedactedFields} {','\tfor _, names := range [][]string{n.Fields, n.RedactedFields} {\n\t\tseen := map[string]bool{}')
p=Path('internal/reporting/definition.go');s=p.read_text();start=s.index('// SelectOutputs returns');end=s.index('// checkResult validates',start)
s=s[:start]+'''// SelectOutputs applies the versioned selection contract before execution.
func SelectOutputs(all []Output, selected []string)([]Output,error){return selectOutputIntent(all,selected)}

'''+s[end:];p.write_text(s)
edit('internal/reporting/definition.go','digest(result.Schema) != digest(d.ExpectedSchema)','digest(physicalSchema(result.Schema)) != digest(physicalSchema(d.ExpectedSchema))')
# Frozen manifest version is independent of the stable admission hash domain.
edit('internal/reporting/runs_model.go','const FrozenVersion = "frozen-block-run-v1"','const FrozenVersion = "frozen-block-run-v2"')
edit('internal/reporting/runs_model.go','m.Version != FrozenVersion','(m.Version != FrozenVersion && m.Version != LegacyFrozenVersion)',count=0)
field('internal/reporting/runs_model.go','RunRequest','QueryLimits *QueryLimits `json:"query_limits,omitempty"`')
for declaration in ['AcceptedLimits *AcceptedLimits `json:"accepted_limits,omitempty"`','Selection []OutputSelection `json:"selection,omitempty"`','SelectionMode string `json:"selection_mode,omitempty" jsonschema:"enum=default,enum=explicit,enum=legacy_all"`','ResultPolicy []FieldPolicy `json:"result_policy,omitempty"`','ResultLineage []exec.OutputLineage `json:"result_lineage,omitempty"`']:
 field('internal/reporting/runs_model.go','RunManifest',declaration)
for declaration in ['AcceptedLimits *AcceptedLimits `json:"accepted_limits,omitempty"`','Selection []OutputSelection `json:"selection,omitempty"`','SelectionMode string `json:"selection_mode,omitempty" jsonschema:"enum=default,enum=explicit,enum=legacy_all"`','ExpectedSchema []exec.Field `json:"expected_schema,omitempty"`','ResultPolicy []FieldPolicy `json:"result_policy,omitempty"`']:
 field('internal/reporting/runs_model.go','RunView',declaration)
for kind in ['RetainedOutput','OutputSummary']:field('internal/reporting/runs_model.go',kind,'Intent *OutputIntent `json:"intent,omitempty"`')
# Selection, exact accepted policy and version-compatible operation replay.
edit('internal/reporting/runs_service.go','if !identity.Identifier(id) || seen[id] {\n\t\t\treturn RunRequest{}, ErrInvalid\n\t\t}','if !identity.Identifier(id) { return RunRequest{}, ErrInvalid }\n if seen[id] {return RunRequest{},selectionError("duplicate")}')
edit('internal/reporting/runs_service.go','\treturn in, nil\n}','\tif in.QueryLimits!=nil&&!in.QueryLimits.Valid(){return RunRequest{},ErrInvalid}\n\treturn in, nil\n}')
edit('internal/reporting/runs_service.go','if len(in.Outputs) > 0 {','if len(in.Outputs) > 0 && d.SchemaVersion == SchemaVersion {')
edit('internal/reporting/runs_service.go','\treturn selected, nil\n}','\treturn attachOutputIntent(d,selected), nil\n}')
edit('internal/reporting/runs_service.go','digest([]any{FrozenVersion, id, in})','digest([]any{LegacyFrozenVersion, id, in})')
edit('internal/reporting/runs_service.go','\tif in.Resolution.At.IsZero() {','\taccepted, err:=s.acceptLimits(d,in.QueryLimits,selected)\n if err!=nil{return RunView{},err}\n\tif in.Resolution.At.IsZero() {')
edit('internal/reporting/runs_service.go','\tprivacyActor := ""',''' m.AcceptedLimits=&accepted
 m.Selection=selectionSnapshot(d,selected)
 m.SelectionMode=selectionMode(d,in.Outputs)
 m.ResultLineage=clone(snapshot.Validation.Evidence.Attempt.Manifest.Receipt.Lineage)
 m.ResultPolicy=inheritedPolicy(d,m.Definitions,m.ResultLineage)
 if accepted.NarrativeCalls==0 {m.Model=""}
 privacyActor := ""''')
edit('internal/reporting/runs_service.go','m.Policy, m.Trust, m.Model, m.Limits.MaxRows, m.Limits.MaxResultBytes})','m.Policy, m.Trust, m.Model, m.AcceptedLimits,m.Selection,m.SelectionMode,m.ResultPolicy,m.Limits})')
# The native executor continues to own slots, durable receipts and cancellation.
edit('internal/reporting/runs_execution.go','\tplan, err := s.pinnedPlan(ctx, e, m)','\tq:=s.currentQuery(m)\n if number>q.MaxAttempts{return RunRecord{},ErrBudget}\n\tplan, err := s.pinnedPlan(ctx, e, m)')
edit('internal/reporting/runs_execution.go','report, executeErr := s.blocks.executor.Execute(ctx, e, plan, exec.Options{Operation: m.ID, Number: number,\n\t\tPreview: m.Private, Rows: min(m.Limits.MaxRows, s.limits.MaxRows), Bytes: min(m.Limits.MaxResultBytes, s.limits.MaxResultBytes)})','report, executeErr := executeWithCaps(ctx,e,s.blocks.executor,plan,exec.Options{Operation:m.ID,Number:number,Preview:m.Private,Rows:q.MaxRows,Bytes:q.MaxBytes},q,m.Version==FrozenVersion)')
edit('internal/reporting/runs_execution.go','out := RetainedOutput{ID: saved.ID, Kind: saved.Kind, State: "succeeded"}','out := RetainedOutput{ID:saved.ID,Kind:saved.Kind,Intent:clone(saved.Intent),State:"succeeded"}\n if saved.Intent!=nil&&!saved.Intent.Enabled{return out,selectionError("disabled")}')
edit('internal/reporting/runs_execution.go','\t\t} else {\n\t\t\t// Persist reservations','''  } else if s.narrativeBudget(m,*n)!=nil {
   out.State,out.Code="failed","narrative_budget_exceeded"
  } else if _,_,err:=narrativeEvidence(narrativeResult(m,result),*n);err!=nil {
   // Filter before reservation and provider input; no implicit evidence refresh.
   out.State,out.Code="failed","narrative_evidence_unavailable"
  } else {
   // Persist reservations''')
edit('internal/reporting/runs_execution.go','start := RetainedOutput{ID: saved.ID, Kind: "narrative", State:','start := RetainedOutput{ID: saved.ID, Kind: "narrative", Intent:clone(saved.Intent), State:')
edit('internal/reporting/runs_execution.go','current.Result == nil && m.ReuseMaxAge > 0 && len(current.View.QueryAttempts) == 0','current.Result == nil && m.ReuseMaxAge > 0 && len(current.View.QueryAttempts) == 0 && s.currentQuery(m)==acceptedQuery(m)')
edit('internal/reporting/runs_execution.go','timeout := min(time.Duration(r.Manifest.Limits.Timeout), time.Duration(s.limits.Timeout))','timeout := time.Duration(s.currentQuery(*r.Manifest).TimeoutMillis)*time.Millisecond')
# Preview/validation are bounded by the authored cap too.
edit('internal/reporting/validation.go','report, err := s.executor.Execute(ctx, e, plan, exec.Options{Operation: operation, Number: 1, Preview: true, Rows: s.limits.PreviewRows, Bytes: s.limits.PreviewBytes})','''q:=QueryLimits{MaxRows:s.limits.PreviewRows,MaxBytes:s.limits.PreviewBytes,TimeoutMillis:int(time.Duration(s.limits.ValidationTimeout)/time.Millisecond),PlannerCost:1e12,MaxAttempts:1}
 if native,ok:=executorQuery(s.executor);ok{q=clampQuery(q,native)}
 if d.QueryLimits!=nil{q=clampQuery(q,*d.QueryLimits)}
 report,err:=executeWithCaps(ctx,e,s.executor,plan,exec.Options{Operation:operation,Number:1,Preview:true,Rows:q.MaxRows,Bytes:q.MaxBytes},q,d.QueryLimits!=nil)''')
edit('internal/reporting/validation.go','CanonicalizationVersion: CanonicalizationVersion,','CanonicalizationVersion: DefinitionCanonicalVersion(d),')
edit('internal/reporting/validation.go','\treturn record, clone(*report.Result), resolved, refs, nil',''' record.Evidence.ResultPolicy=inheritedPolicy(d,record.Definitions,receipt.Lineage)
 record.Evidence.Schema=annotatedSchema(report.Result.Schema,record.Evidence.ResultPolicy)
 record.Evidence.SchemaDigest=digest(record.Evidence.Schema)
 record.Evidence.OutputIntent=outputDescriptors(d)
 return record, clone(*report.Result), resolved, refs, nil''')
edit('internal/reporting/validation.go','Outputs: selected, Result: result,','Outputs: attachOutputIntent(snapshot.Revision.Definition,selected), Result: result,')
edit('internal/reporting/validation.go','v.Evidence.CanonicalizationVersion != CanonicalizationVersion','v.Evidence.CanonicalizationVersion != DefinitionCanonicalVersion(r.Definition)')
# Provider sees bounded retained evidence only; unknown classification is denied.
edit('internal/reporting/runs_narrative.go','"github.com/hurtener/chartworks/internal/identity"','"github.com/hurtener/chartworks/internal/identity"\n "github.com/hurtener/chartworks/internal/semantics"')
edit('internal/reporting/runs_narrative.go','\titems := []NarrativeEvidence{}','\tfor _,field:=range result.Schema{if field.Sensitivity!=semantics.LiteralNonSensitive{delete(allowed,field.Name)}}\n\titems := []NarrativeEvidence{}')
edit('internal/reporting/runs_narrative.go','len(answer.Claims) > 32','len(answer.Claims) > narrativeClaims(n)')
edit('internal/reporting/runs_narrative.go','if !strings.HasPrefix(n.Locale, "en") && !strings.HasPrefix(n.Locale, "es") {','if !supportedNarrativeLocale(n.Locale) {')
edit('internal/reporting/runs_narrative.go','evidence, caveats, err := narrativeEvidence(result, n)','if err:=s.narrativeBudget(m,n);err!=nil{return NarrativeResult{},err}\n evidence,caveats,err:=narrativeEvidence(narrativeResult(m,result),n)')
edit('internal/reporting/runs_narrative.go','\tbudget, err := gateway.NewBudget(call,','\tif m.AcceptedLimits!=nil{duration=min(duration,time.Duration(m.AcceptedLimits.NarrativeTimeoutMillis)*time.Millisecond)}\n\tbudget, err := gateway.NewBudget(call,')
edit('internal/reporting/runs_narrative.go','[]byte(narrativeSchema)','[]byte(strings.Replace(narrativeSchema,`"maxItems":32`,fmt.Sprintf(`"maxItems":%d`,narrativeClaims(n)),1))')
# Revalidate policy at real frozen persistence boundaries.
edit('internal/reporting/runs_guards.go','\treturn runEligibility(e, snapshot, m.Policy, now)','\tif err:=CheckRunPolicy(m);err!=nil{return err}\n\treturn runEligibility(e, snapshot, m.Policy, now)')
edit('internal/reporting/runs_guards.go','\twant := make([]string, 0, len(m.Dependencies))','\tif !queryAttemptWithin(m,a){return ErrBudget}\n\twant := make([]string, 0, len(m.Dependencies))')
edit('internal/reporting/runs_guards.go','digest(result.Schema) != digest(m.Revision.Definition.ExpectedSchema)','digest(physicalSchema(result.Schema)) != digest(physicalSchema(m.Revision.Definition.ExpectedSchema))')
edit('internal/reporting/runs_guards.go','if saved == nil || saved.Kind != o.Kind {','if saved == nil || saved.Kind != o.Kind || digest(saved.Intent)!=digest(o.Intent) || saved.Intent!=nil&&!saved.Intent.Enabled {')
edit('internal/reporting/runs_guards.go','"narrative_indeterminate", "output_failed"','"narrative_indeterminate", "narrative_budget_exceeded", "narrative_evidence_unavailable", "output_failed"')
edit('internal/reporting/runs_guards.go','\ttext, err := groundedText(','\tif m.Version==FrozenVersion&&!validNarrativeEvidence(m,*spec,n.Evidence){return ErrInvalid}\n encoded,err:=json.Marshal(n.Evidence)\n if err!=nil||len(encoded)>spec.MaxBytes||len(n.Evidence)>256{return ErrBudget}\n\ttext, err := groundedText(')
edit('internal/reporting/runs_proof.go','\treturn m, nil\n}','\tif err:=CheckRunPolicy(m);err!=nil{return RunManifest{},err}\n\treturn m, nil\n}')
edit('internal/reporting/runs_proof.go','\treturn w, nil\n}','\tif w.Attempt!=nil&&!queryAttemptWithin(w.Manifest,*w.Attempt){return RunWrite{},ErrBudget}\n\treturn w, nil\n}')
edit('internal/store/postgres/frozen_runs_read.go','\tout.Manifest = &m','\tif reporting.CheckRunPolicy(m)!=nil{return store.ErrInvalid}\n\tout.Manifest = &m\n reporting.ProjectRunMetadata(&out.View,m)')
edit('internal/store/postgres/frozen_runs_read.go','reporting.OutputSummary{ID: output.ID, Kind: output.Kind, State:','reporting.OutputSummary{ID: output.ID, Kind: output.Kind, Intent:output.Intent, State:')
edit('internal/store/postgres/frozen_runs_read.go','\tif values {\n\t\tif err = frozenValuesTx','\tif !values{if err=frozenIntentTx(ctx,tx,e.Tenant(),id,&out.View);err!=nil{return reporting.RunRecord{},err}}\n\tif values {\n\t\tif err = frozenValuesTx')
print('Applied reporting intent, caps, sensitivity and frozen-boundary edits')
