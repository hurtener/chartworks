package exec

import("slices";"strings")

// ColumnOrigin is validator-owned value provenance, never value authority.
type ColumnOrigin struct{Dataset string `json:"dataset"`;Column string `json:"column"`}
// OutputLineage explicitly distinguishes proved dependencies from unknown ones.
type OutputLineage struct{Output string `json:"output"`;Columns []ColumnOrigin `json:"columns"`;Complete bool `json:"complete"`}
type lineageSource struct{alias string;relation Relation}

// postgresLineage inspects an ALREADY accepted native AST. It neither validates
// nor alters SQL and never expands source reach. Derived/CTE/set-operation or
// unsupported value lineage remains unknown, not guessed from output labels.
func postgresLineage(statement any,binding Binding,outputs []string)[]OutputLineage{
 out:=make([]OutputLineage,len(outputs));for i,name:=range outputs{out[i]=OutputLineage{Output:name,Columns:[]ColumnOrigin{}}}
 m:=object(object(statement)["SelectStmt"])
 if m==nil||m["withClause"]!=nil||m["larg"]!=nil||m["rarg"]!=nil||len(array(m["valuesLists"]))!=0{return out}
 targets:=array(m["targetList"]);if len(targets)!=len(outputs){return out}
 sources:=[]lineageSource{}
 for _,item:=range array(m["fromClause"]){var ok bool;sources,ok=lineageFrom(item,binding,sources);if !ok{return out}}
 if len(sources)==0{return out}
 used:=0
 for i,target:=range targets{
  origins:=map[ColumnOrigin]bool{}
  if !lineageExpression(fieldObject(target,"ResTarget")["val"],sources,origins)||len(origins)==0||used+len(origins)>128{continue}
  used+=len(origins);for origin:=range origins{out[i].Columns=append(out[i].Columns,origin)}
  slices.SortFunc(out[i].Columns,func(a,b ColumnOrigin)int{if n:=strings.Compare(a.Dataset,b.Dataset);n!=0{return n};return strings.Compare(a.Column,b.Column)})
  out[i].Complete=true
 }
 return out
}
func lineageFrom(node any,binding Binding,sources []lineageSource)([]lineageSource,bool){
 root:=object(node);if len(root)!=1{return nil,false}
 if raw,ok:=root["JoinExpr"];ok{join:=object(raw);var valid bool;sources,valid=lineageFrom(join["larg"],binding,sources);if !valid{return nil,false};return lineageFrom(join["rarg"],binding,sources)}
 raw,ok:=root["RangeVar"];if !ok{return nil,false}
 r:=object(raw);schema,name:=text(r["schemaname"]),text(r["relname"]);if schema==""{return nil,false}
 alias:=name
 if a:=fieldObject(r["alias"],"Alias");a!=nil{if len(array(a["colnames"]))>0{return nil,false};alias=text(a["aliasname"])}
 for _,source:=range sources{if source.alias==alias{return nil,false}}
 for _,relation:=range binding.Relations{if relation.Schema==schema&&relation.Name==name{return append(sources,lineageSource{alias:alias,relation:relation}),true}}
 return nil,false
}
func lineageExpression(value any,sources []lineageSource,found map[ColumnOrigin]bool)bool{
 switch v:=value.(type){
 case map[string]any:
  if raw,ok:=v["ColumnRef"];ok{
   parts,valid:=names(object(raw)["fields"]);if !valid||len(parts)<1||len(parts)>2{return false}
   matches:=[]ColumnOrigin{}
   for _,source:=range sources{if len(parts)==2&&source.alias!=parts[0]{continue};for _,column:=range source.relation.Columns{if column.Safe&&column.Name==parts[len(parts)-1]{matches=append(matches,ColumnOrigin{Dataset:source.relation.ID,Column:column.Name})}}}
   if len(matches)!=1{return false};found[matches[0]]=true;return len(found)<=128
  }
  // Literals and parameters have no reviewed value classification. An innocent
  // alias cannot turn them or a scalar subquery into approved evidence.
  for _,key:=range []string{"SubLink","ParamRef","A_Const","SQLValueFunction"}{if _,ok:=v[key];ok{return false}}
  if raw,ok:=v["FuncCall"];ok&&truth(object(raw)["agg_star"]){for _,source:=range sources{for _,column:=range source.relation.Columns{if column.Safe{found[ColumnOrigin{Dataset:source.relation.ID,Column:column.Name}]=true;if len(found)>128{return false}}}}}
  for _,child:=range v{if !lineageExpression(child,sources,found){return false}}
 case []any:
  for _,child:=range v{if !lineageExpression(child,sources,found){return false}}
 }
 return true
}
func cloneLineage(in []OutputLineage)[]OutputLineage{
 if in==nil{return nil};out:=append([]OutputLineage{},in...);for i:=range out{out[i].Columns=append([]ColumnOrigin{},in[i].Columns...)};return out
}
