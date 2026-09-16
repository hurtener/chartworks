package reporting

import("slices";"time";"github.com/hurtener/chartworks/internal/exec")

// CheckRunPolicy revalidates the sealed contract under the store's existing
// transaction/fence, with no source, model or identity service calls.
func CheckRunPolicy(m RunManifest)error{
 if m.Version==LegacyFrozenVersion{return nil}
 if m.Version!=FrozenVersion||m.AcceptedLimits==nil||!m.AcceptedLimits.valid()||!validResultPolicy(m){return ErrInvalid}
 q:=m.AcceptedLimits.Query
 if clampQuery(q,deploymentQuery(m.Limits))!=q||m.Revision.Definition.QueryLimits!=nil&&clampQuery(q,*m.Revision.Definition.QueryLimits)!=q{return ErrBudget}
 d:=m.Revision.Definition;all:=map[string]Output{};for _,o:=range effectiveOutputs(d){all[o.ID]=o}
 ids:=[]string{}
 for _,o:=range m.Outputs{if o.Intent==nil||!o.Intent.Enabled||digest(all[o.ID])!=digest(o)||slices.Contains(ids,o.ID){return ErrInvalid};ids=append(ids,o.ID)}
 if len(ids)==0||digest(selectionSnapshot(d,m.Outputs))!=digest(m.Selection){return ErrInvalid}
 switch m.SelectionMode{
 case "default":
  if d.SchemaVersion!=CurrentSchemaVersion{return ErrInvalid};selected,err:=SelectOutputs(d.Outputs,nil);if err!=nil||digest(attachOutputIntent(d,selected))!=digest(m.Outputs){return ErrInvalid}
 case "legacy_all":
  if d.SchemaVersion!=SchemaVersion||digest(effectiveOutputs(d))!=digest(m.Outputs){return ErrInvalid}
 case "explicit":
  selected,err:=SelectOutputs(d.Outputs,ids);if err!=nil{return err};if d.SchemaVersion==CurrentSchemaVersion&&digest(attachOutputIntent(d,selected))!=digest(m.Outputs){return ErrInvalid}
 default:return ErrInvalid
 }
 return nil
}
func queryAttemptWithin(m RunManifest,attempt exec.Attempt)bool{
 if m.AcceptedLimits==nil{return m.Version!=FrozenVersion};q:=m.AcceptedLimits.Query;l:=attempt.Manifest.Limits
 return q.Valid()&&attempt.Number>=1&&attempt.Number<=q.MaxAttempts&&l.Rows<=q.MaxRows&&l.Bytes<=q.MaxBytes&&l.Timeout<=time.Duration(q.TimeoutMillis)*time.Millisecond&&l.PlannerCost<=q.PlannerCost
}

// ProjectRunMetadata excludes SQL, raw values, source lineage and topic payloads.
func ProjectRunMetadata(view *RunView,m RunManifest){
 if view==nil{return};view.AcceptedLimits=clone(m.AcceptedLimits);view.Selection=clone(m.Selection);view.SelectionMode=m.SelectionMode;view.ResultPolicy=clone(m.ResultPolicy)
 if m.Version==LegacyFrozenVersion&&len(view.Selection)==0{view.Selection=selectionSnapshot(m.Revision.Definition,m.Outputs);view.SelectionMode="legacy_all"}
 view.ExpectedSchema=annotatedSchema(m.Revision.Definition.ExpectedSchema,view.ResultPolicy)
}
