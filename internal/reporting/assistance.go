package reporting

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// ParameterizeRequest selects a parsed, half-open date range and its new typed
// period declaration. The digest/CAS prevent applying a stale suggested edit.
// Column contains exact SQL identifier components, not a SQL fragment.
type ParameterizeRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
	DefinitionDigest string `json:"definition_digest"`
	Column []string `json:"column"`
	Parameter Parameter `json:"parameter"`
	Note string `json:"note"`
}

// Parameterize appends a private draft and nothing more. It never silently
// modifies published SQL or fabricates validation for a suggested period edit.
func (s *Service) Parameterize(ctx context.Context,e identity.Envelope,id string,in ParameterizeRequest)(View,error){
	ctx,cancel,err:=s.begin(ctx,e,id,Write);if err!=nil{return View{},err};defer cancel()
	if !note(in.Note) || !hashValid(in.DefinitionDigest) || in.Parameter.Type!="relative_period" || in.Parameter.Default==nil || in.Parameter.Default.Period==nil{return View{},ErrInvalid}
	snapshot,err:=s.repo.ReadBlock(ctx,e,id,Reference{Draft:true},Write);if err!=nil{return View{},err}
	if err:=expected(snapshot,in.ExpectedVersion);err!=nil{return View{},err}
	if snapshot.State.Archived || snapshot.Revision.Digest!=in.DefinitionDigest{return View{},store.ErrConflict}
	if s.sources==nil{return View{},ErrUnavailable}
	d:=clone(snapshot.Revision.Definition)
	binding,err:=s.sources.ContextBinding(ctx,e,d.Source,d.Context);if err!=nil{return View{},err}
	if binding.Dialect!="postgres"{return View{},ErrInvalid}
	select{case s.slots<-struct{}{}:defer func(){<-s.slots}();case <-ctx.Done():return View{},ctx.Err();default:return View{},ErrBusy}
	d.Parameters=append(d.Parameters,clone(in.Parameter))
	if validateDeclarations(d.Parameters,s.limits.MaxParameters)!=nil || scalarSlots(d.Parameters)>64{return View{},ErrInvalid}
	d.SQL,err=parameterizePeriod(ctx,d.SQL,in.Column,scalarSlots(snapshot.Revision.Definition.Parameters)+1)
	if err!=nil{return View{},err}
	d.Template=nil
	if err:=validateDefinition(ctx,d,s.limits,false);err!=nil{return View{},err}
	_,refs,err:=s.resolveDefinitions(ctx,e,d,false);if err!=nil{return View{},err}
	provenance:=clone(snapshot.Revision.Provenance)
	provenance.Kind="parameterize";provenance.ParentRevision=snapshot.Revision.Number
	provenance.ChangeDigest=digest(struct{Before,After string;Column []string;Parameter Parameter}{snapshot.Revision.Digest,DefinitionDigest(d),in.Column,in.Parameter})
	r,err:=s.newRevision(e,snapshot.State.DraftRevision+1,d,provenance);if err!=nil{return View{},err}
	state,err:=s.commit(ctx,e,Mutation{ID:id,Topic:snapshot.State.Topic,Kind:"parameterize",ExpectedVersion:in.ExpectedVersion,TargetRevision:snapshot.Revision.Number,TargetDigest:snapshot.Revision.Digest,Note:in.Note,Revision:&r,References:refs})
	if err!=nil{return View{},err}
	return project(Snapshot{State:state,Revision:r},time.Now()),nil
}
