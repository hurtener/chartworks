package nlqexec

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "sort"
 "strings"

 "github.com/hurtener/chartworks/internal/exec"
 "github.com/hurtener/chartworks/internal/identity"
 "github.com/hurtener/chartworks/internal/nlqroute"
 "github.com/hurtener/chartworks/internal/semantics"
 pgtypes "github.com/pganalyze/pg_query_go/v6"
 pgquery "github.com/wasilibs/go-pgquery"
 "google.golang.org/protobuf/proto"
 "google.golang.org/protobuf/reflect/protoreflect"
)

// ErrClarificationBinding reports a semantic constraint that could not be
// applied. It is deliberately separate from the validator's authority/read proof.
var ErrClarificationBinding = errors.New("nlqexec: clarification constraint binding required")

// ErrClarificationUnsupported is a fail-closed semantic capability boundary.
var ErrClarificationUnsupported = errors.New("nlqexec: clarification binding unsupported for this source or query shape")

type clarificationField struct {
 resolution semantics.ClarificationResolution
 relation exec.Relation
 column exec.Column
}

type clarificationRechecker interface {
 Recheck(context.Context,identity.Envelope,nlqroute.RouteRequest)(nlqroute.RouteResult,error)
}

// clarificationPreflight performs all parsing, publication and target-type
// checks before the first embedding or generation call. It does not plan SQL.
func (s *Service) clarificationPreflight(ctx context.Context,e identity.Envelope,in QuestionRequest) (*admission,error) {
 checker,ok:=s.router.(clarificationRechecker)
 if !ok {
  if len(in.Answers)>0 {return nil,ErrClarificationUnsupported}
  return nil,nil
 }
 route,err:=checker.Recheck(ctx,e,in.routeRequest())
 if err!=nil{return nil,err}
 a:=&admission{route:route}
 if route.Clarification!=nil || len(route.Resolutions)==0 {return a,nil}
 if route.ClarificationPins==nil || route.ClarificationPins.Tenant!=e.Tenant() || route.ClarificationPins.Actor!=e.User() || route.ClarificationPins.Session!=e.Session() || route.ClarificationPins.Context!=in.Context {return nil,ErrClarificationBinding}
 for _,pin:=range route.ClarificationPins.Sources {
  if a.source=="" {a.source,a.context=pin.Source,pin.Context}
  if pin.Source!=a.source || pin.Context!=a.context {return nil,ErrClarificationBinding}
 }
 if a.source=="" {return nil,ErrClarificationBinding}
 a.binding,err=s.sources.Binding(ctx,e,a.source,a.context)
 if err!=nil{return nil,err}
 a.clarificationFields,err=s.clarificationFields(ctx,e,route,a.binding)
 if err!=nil{return nil,err}
 return a,nil
}

func (s *Service) clarificationFields(ctx context.Context,e identity.Envelope,route nlqroute.RouteResult,binding exec.Binding) ([]clarificationField,error) {
 if len(route.Resolutions)==0 {return nil,nil}
 if !binding.Valid() || binding.Tenant!=e.Tenant() || binding.Context!=route.Request.Context {return nil,ErrClarificationBinding}
 byTopic:=map[string][]semantics.ClarificationResolution{}
 for _,r:=range route.Resolutions {
  if r.ID=="" || r.ID!=semantics.ClarificationResolutionDigest(r) {return nil,ErrClarificationBinding}
  byTopic[r.Topic]=append(byTopic[r.Topic],r)
 }
 var fields []clarificationField
 parameterCount:=0
 for _,topic:=range route.Topics {
  resolutions:=byTopic[topic]
  if len(resolutions)==0 {continue}
  contract,err:=s.topics.Contract(ctx,e,topic)
  if err!=nil{return nil,err}
  p:=contract.Publication
  for _,r:=range resolutions {
   if p.State.Version!=r.TopicVersion || p.Digest!=r.PackDigest || !p.State.Active || p.State.Archived {return nil,staleClarification(route.Request.Locale)}
   if r.Reference!=nil {continue}
   if r.Effect==nil || binding.Dialect!="postgres" {return nil,ErrClarificationUnsupported}
   ref:=r.Effect.Target
   if ref.Kind==semantics.KindDimension {
    found:=false
    for _,d:=range p.Definition.Dimensions {if d.ID==ref.ID {ref=d.Field;found=true;break}}
    if !found {return nil,ErrClarificationBinding}
   }
   // A measure denotes an aggregate, not a row field. Do not silently turn a
   // threshold on an aggregate into a pre-aggregation row predicate.
   if ref.Kind!=semantics.KindColumn {return nil,ErrClarificationUnsupported}
   var relationID,columnName string
   for _,d:=range p.Definition.Datasets {
    if d.ID!=ref.Dataset {continue}
    if d.Source.Source!=binding.Source || d.Source.Context!=binding.Context || d.Source.SourceRevision!=binding.Revision {return nil,ErrClarificationBinding}
    relationID=d.Source.Dataset
    for _,c:=range d.Columns {if c.ID==ref.ID {columnName=c.SourceName;break}}
   }
   field:=clarificationField{resolution:r}
   found:=false
   for _,relation:=range binding.Relations {
    if relation.ID!=relationID {continue}
    field.relation=relation
    for _,column:=range relation.Columns {if column.Name==columnName && column.Safe {field.column=column;found=true;break}}
   }
   if !found || !clarificationColumnType(field) {return nil,ErrClarificationUnsupported}
   if !r.Null {parameterCount++;if r.Time!=nil || r.Effect.Operator=="range" {parameterCount++}}
   if parameterCount>64 {return nil,exec.ErrLimit}
   fields=append(fields,field)
  }
  delete(byTopic,topic)
 }
 if len(byTopic)!=0 {return nil,ErrClarificationBinding}
 sort.Slice(fields,func(i,j int)bool{return fields[i].resolution.ID<fields[j].resolution.ID})
 return fields,nil
}

func clarificationColumnType(f clarificationField) bool {
 n:=strings.ToLower(strings.TrimSpace(f.column.NativeType))
 e:=f.resolution.Effect
 if e==nil{return false}
 switch e.Kind {
 case "number":
  return n=="smallint" || n=="integer" || n=="bigint" || n=="int2" || n=="int4" || n=="int8" || n=="numeric" || strings.HasPrefix(n,"numeric(") || n=="decimal" || strings.HasPrefix(n,"decimal(")
 case "boolean": return n=="boolean" || n=="bool"
 case "entity","text": return n=="text" || n=="varchar" || strings.HasPrefix(n,"varchar(") || n=="character varying" || strings.HasPrefix(n,"character varying(")
 case "time_window":
  switch e.TemporalType {
  case "date":return n=="date"
  case "timestamp":return n=="timestamp" || n=="timestamp without time zone"
  case "timestamptz":return n=="timestamptz" || n=="timestamp with time zone"
  }
 }
 return false
}

// bindClarificationCandidate builds server-owned row predicates into every
// matching source scan. Only declared parameter values and registry identifiers
// are used. The resulting statement still goes through the ordinary validator.
// The initial proof supports one SELECT with physical FROM/join inputs. CTEs,
// subqueries and set operations are explicit unsupported outcomes, not a text
// replacement that might constrain an unused or unrelated query branch.
func bindClarificationCandidate(a admission,candidate generatedCandidate) (generatedCandidate,error) {
 if len(a.clarificationFields)==0 {return candidate,nil}
 if a.binding.Dialect!="postgres" || len(candidate.SQL)>maxSQLBytes {return generatedCandidate{},ErrClarificationUnsupported}
 parsed,err:=pgquery.Parse(candidate.SQL)
 if err!=nil || len(parsed.Stmts)!=1 {return generatedCandidate{},ErrClarificationBinding}
 root:=parsed.Stmts[0].Stmt.GetSelectStmt()
 if root==nil || root.WithClause!=nil || root.Larg!=nil || root.Rarg!=nil || len(root.FromClause)==0 {return generatedCandidate{},ErrClarificationUnsupported}
 unsupported:=false
 visitClarificationMessages(parsed,func(m proto.Message) bool {
  switch m.(type) {
  case *pgtypes.RangeSubselect,*pgtypes.SubLink,*pgtypes.CommonTableExpr:unsupported=true;return false
  }
  return true
 })
 if unsupported {return generatedCandidate{},ErrClarificationUnsupported}
 parameters:=append([]exec.Parameter(nil),candidate.Parameters...)
 used:=map[string]bool{}
 aliases:=map[string]string{}
 var bindErr error
 visitClarificationMessages(parsed,func(m proto.Message) bool {
  if bindErr!=nil{return false}
  node,ok:=m.(*pgtypes.Node)
  if !ok || node.GetRangeVar()==nil{return true}
  rv:=node.GetRangeVar()
  var matching []clarificationField
  for _,f:=range a.clarificationFields {
   if rv.Relname==f.relation.Name && (rv.Schemaname==f.relation.Schema || rv.Schemaname=="") {
    if rv.Schemaname=="" {count:=0;for _,rel:=range a.binding.Relations {if rel.Name==rv.Relname {count++}};if count!=1 {bindErr=ErrClarificationUnsupported;return false}}
    matching=append(matching,f)
   }
  }
  if len(matching)==0{return true}
  alias:=rv.Relname
  if rv.Alias!=nil {alias=rv.Alias.Aliasname}
  if !exec.SQLIdentifier(alias) {bindErr=ErrClarificationUnsupported;return false}
  relation:=matching[0].relation
  var predicates []string
  for _,f:=range matching {
   predicate,err:=clarificationPredicate(f,&parameters)
   if err!=nil{bindErr=err;return false}
   predicates=append(predicates,predicate)
   used[f.resolution.ID]=true
  }
  columns:=make([]string,0,len(relation.Columns))
  for _,c:=range relation.Columns {if c.Safe {columns=append(columns,quoteClarificationIdentifier(c.Name))}}
  if len(columns)==0 || len(parameters)>64 {bindErr=exec.ErrLimit;return false}
  sql:="SELECT 1 FROM (SELECT "+strings.Join(columns,",")+" FROM "+quoteClarificationIdentifier(relation.Schema)+"."+quoteClarificationIdentifier(relation.Name)+" WHERE "+strings.Join(predicates," AND ")+") AS "+quoteClarificationIdentifier(alias)
  filter,err:=pgquery.Parse(sql)
  if err!=nil || len(filter.Stmts)!=1 {bindErr=ErrClarificationBinding;return false}
  replacement:=filter.Stmts[0].Stmt.GetSelectStmt().FromClause[0]
  if rv.Alias!=nil {replacement.GetRangeSubselect().Alias=proto.Clone(rv.Alias).(*pgtypes.Alias)}
  aliases[relation.Schema+"."+relation.Name]=alias
  node.Node=replacement.Node
  return false // Do not recursively bind the newly built physical scan again.
 })
 if bindErr!=nil{return generatedCandidate{},bindErr}
 for _,f:=range a.clarificationFields {if !used[f.resolution.ID] {return generatedCandidate{},ErrClarificationBinding}}
 // Fully qualified original column references must address the preserved alias
 // after replacing a physical range with a filtered range. Registry coordinates,
 // never display labels, determine this exact rewrite.
 visitClarificationMessages(parsed,func(m proto.Message)bool{
  node,ok:=m.(*pgtypes.Node);if !ok{return true}
  cr:=node.GetColumnRef();if cr==nil || len(cr.Fields)!=3{return true}
  schema,table:=cr.Fields[0].GetString_(),cr.Fields[1].GetString_()
  if schema==nil || table==nil{return true}
  if alias,ok:=aliases[schema.Sval+"."+table.Sval];ok {
   cr.Fields=cr.Fields[1:]
   cr.Fields[0].GetString_().Sval=alias
  }
  return true
 })
 statement,err:=pgquery.Deparse(parsed)
 if err!=nil || len(statement)>maxSQLBytes {return generatedCandidate{},ErrClarificationBinding}
 candidate.SQL,candidate.Parameters=statement,parameters
 return candidate,nil
}

func quoteClarificationIdentifier(value string) string {return `"`+strings.ReplaceAll(value,`"`,`""`)+`"`}

func clarificationPredicate(f clarificationField,parameters *[]exec.Parameter) (string,error) {
 r,e:=f.resolution,f.resolution.Effect
 if e==nil || r.ID!=semantics.ClarificationResolutionDigest(r) {return "",ErrClarificationBinding}
 column:=quoteClarificationIdentifier(f.column.Name)
 if r.Null {if e.Nulls!="only" {return "",ErrClarificationBinding};return "("+column+" IS NULL)",nil}
 bind:=func(value,kind string) string {
  // Text transport plus an explicit closed native cast preserves exact decimal
  // digits; no driver is asked to round a business threshold through float64.
  *parameters=append(*parameters,exec.Parameter{Kind:"text",Value:value})
  return fmt.Sprintf("CAST($%d AS %s)",len(*parameters),kind)
 }
 kind:="text"
 switch e.Kind {case "number":kind="numeric";case "boolean":kind="boolean";case "time_window":kind=e.TemporalType;case "entity","text":default:return "",ErrClarificationBinding}
 lower,upper:=r.Value,r.Upper
 bounds:=e.Bounds
 if r.Time!=nil {
  lower,upper=r.Time.LocalStart,r.Time.LocalEnd
  if e.TemporalType=="timestamptz" {lower,upper=r.Time.StartUTC,r.Time.EndUTC}
  bounds=r.Time.Bounds
 }
 var predicate string
 if e.Operator=="range" {
  if len(bounds)!=2{return "",ErrClarificationBinding}
  lo,hi:=">","<"
  if bounds[0]=='[' {lo=">="};if bounds[1]==']'{hi="<="}
  predicate=column+" "+lo+" "+bind(lower,kind)+" AND "+column+" "+hi+" "+bind(upper,kind)
 } else {
  op:=map[string]string{"eq":"=","ne":"<>","gt":">","gte":">=","lt":"<","lte":"<="}[e.Operator]
  if op==""{return "",ErrClarificationBinding}
  predicate=column+" "+op+" "+bind(lower,kind)
 }
 if e.Nulls=="include" {predicate="("+predicate+") OR "+column+" IS NULL"} else if e.Nulls!="exclude" {return "",ErrClarificationBinding}
 return "("+predicate+")",nil
}

func visitClarificationMessages(message proto.Message,visit func(proto.Message)bool) {
 if message==nil || !visit(message){return}
 m:=message.ProtoReflect()
 m.Range(func(field protoreflect.FieldDescriptor,value protoreflect.Value)bool{
  if field.IsList() && field.Kind()==protoreflect.MessageKind {
   list:=value.List();for i:=0;i<list.Len();i++{visitClarificationMessages(list.Get(i).Message().Interface(),visit)}
  } else if field.Kind()==protoreflect.MessageKind && !field.IsMap() {visitClarificationMessages(value.Message().Interface(),visit)}
  return true
 })
}

func sameClarificationEvidence(a,b nlqroute.RouteResult) bool {
 // Provenance is retained evidence; canonical replay cannot silently substitute
 // a default or reinterpret a value under another publication/source partition.
 left,_:=json.Marshal(struct{Pins *nlqroute.ClarificationPins;Resolutions []semantics.ClarificationResolution}{a.ClarificationPins,a.Resolutions})
 right,_:=json.Marshal(struct{Pins *nlqroute.ClarificationPins;Resolutions []semantics.ClarificationResolution}{b.ClarificationPins,b.Resolutions})
 return string(left)==string(right)
}
