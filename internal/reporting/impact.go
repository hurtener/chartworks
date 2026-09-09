package reporting

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// CatalogObserver is an optional live catalog capability of the existing source
// service. It provides native object identity without reading result values.
type CatalogObserver interface {
	ObserveCatalog(context.Context,identity.Envelope,string,string)(sources.CatalogObservation,error)
}

// Rename is a source-backed stable semantic column identity, not a caller patch.
type Rename struct {
	Dataset string `json:"dataset"`
	Column string `json:"column"`
	From string `json:"from"`
	To string `json:"to"`
}

type ImpactRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
	Reference Reference `json:"reference"`
}

type Impact struct {
	ID string `json:"id"`
	Revision int64 `json:"revision"`
	Version int64 `json:"version"`
	DefinitionDigest string `json:"definition_digest"`
	Classification string `json:"classification" jsonschema:"enum=unchanged,enum=cosmetic,enum=rename,enum=review_required,enum=unavailable"`
	Reason string `json:"reason"`
	DependencyDigest string `json:"dependency_digest,omitempty"`
	ProposalDigest string `json:"proposal_digest,omitempty"`
	Renames []Rename `json:"renames"`
	ObservedAt time.Time `json:"observed_at"`
}

type ApplyImpactRequest struct {
	ExpectedVersion int64 `json:"expected_version"`
	Revision int64 `json:"revision"`
	ProposalDigest string `json:"proposal_digest"`
	Note string `json:"note"`
}

type impactWork struct {
	impact Impact
	binding exec.Binding
	publications []topics.Published
	pins []TopicPin
	dependencies []Dependency
}

func (s *Service) CanObserve()bool{if s==nil{return false};_,ok:=s.sources.(CatalogObserver);return ok}

func impactAuthorization(err error)bool{
	return errors.Is(err,access.ErrUnauthenticated) || errors.Is(err,access.ErrForbidden) || errors.Is(err,access.ErrNotFound) || errors.Is(err,store.ErrNotFound) || errors.Is(err,context.Canceled) || errors.Is(err,context.DeadlineExceeded)
}

// semanticShape removes explicit presentation names only. Stable IDs, units,
// expressions, joins, canonical definitions and role/type contracts remain.
// Unused datasets are excluded; columns of each used relation are conservatively
// pinned because the common validator proves relation scope, not column lineage.
func semanticShape(d topics.Definition,used map[string]bool)topics.Definition{
	out:=clone(d)
	out.Version="";out.Name="";out.Description=""
	kept:=out.Datasets[:0]
	for _,dataset:=range out.Datasets{
		if !used[dataset.ID]{continue}
		dataset.Name="";dataset.Source.Context="";dataset.Source.SourceRevision=0
		for i:=range dataset.Columns{dataset.Columns[i].Name="";dataset.Columns[i].SourceName=""}
		kept=append(kept,dataset)
	}
	out.Datasets=kept
	for i:=range out.Measures{out.Measures[i].Name="";out.Measures[i].Description=""}
	for i:=range out.Dimensions{out.Dimensions[i].Name="";out.Dimensions[i].Description=""}
	for i:=range out.KPIs{out.KPIs[i].Name="";out.KPIs[i].Description=""}
	return out
}

func nativeColumn(identity sources.CatalogIdentity,dataset,name string)(string,string){
	if identity.Version!="postgres-catalog-identity-v1" || !hashValid(identity.Authority){return "",""}
	for _,relation:=range identity.Relations{
		if relation.Dataset!=dataset{continue}
		for _,column:=range relation.Columns{if column.Name==name{return relation.Object,column.Object}}
	}
	return "",""
}

func compareImpact(old ValidationRecord,binding exec.Binding,observed sources.CatalogObservation,current []topics.Published)(string,string,[]Rename){
	used:=map[string]bool{}
	for _,dep:=range old.Dependencies{used[dep.Dataset]=true}
	if len(used)==0 || len(old.Definitions)!=len(current){return "review_required","missing_dependency_baseline",nil}
	if old.Binding.Source!=binding.Source || old.Binding.Dialect!=binding.Dialect || old.Catalog.Version=="" || old.Catalog.Version!=observed.Identity.Version || old.Catalog.Authority!=observed.Identity.Authority{
		// Without native identity evidence, an identical exact registered binding
		// can still be observed unchanged. It cannot support a rename proposal.
		if old.BindingDigest!=exec.Hash(binding){return "review_required","source_identity_changed_or_unproven",nil}
	}
	renamed:=[]Rename{}
	for i,publication:=range current{
		before,after:=old.Definitions[i],publication.Definition
		if !reflect.DeepEqual(semanticShape(before,used),semanticShape(after,used)){return "review_required","semantic_contract_changed",nil}
		for _,beforeDataset:=range before.Datasets{
			if !used[beforeDataset.ID]{continue}
			for _,afterDataset:=range after.Datasets{
				if beforeDataset.ID!=afterDataset.ID{continue}
				for _,beforeColumn:=range beforeDataset.Columns{
					for _,afterColumn:=range afterDataset.Columns{
						if beforeColumn.ID!=afterColumn.ID{continue}
						oldTable,oldColumn:=nativeColumn(old.Catalog,beforeDataset.ID,beforeColumn.SourceName)
						newTable,newColumn:=nativeColumn(observed.Identity,afterDataset.ID,afterColumn.SourceName)
						if oldTable=="" || oldColumn=="" || oldTable!=newTable || oldColumn!=newColumn{
							if old.BindingDigest!=exec.Hash(binding) || beforeColumn.SourceName!=afterColumn.SourceName{return "review_required","native_object_identity_changed_or_unproven",nil}
						}
						if beforeColumn.SourceName!=afterColumn.SourceName{renamed=append(renamed,Rename{Dataset:beforeDataset.ID,Column:beforeColumn.ID,From:beforeColumn.SourceName,To:afterColumn.SourceName})}
					}
				}
			}
		}
	}
	for _,dep:=range old.Dependencies{
		found:=false
		for _,relation:=range binding.Relations{if relation.ID==dep.Dataset && relation.Schema==dep.Schema && relation.Name==dep.Name{found=true}}
		if !found{return "review_required","relation_identity_changed",nil}
	}
	sort.Slice(renamed,func(i,j int)bool{if renamed[i].Dataset!=renamed[j].Dataset{return renamed[i].Dataset<renamed[j].Dataset};return renamed[i].Column<renamed[j].Column})
	if len(renamed)>0{return "rename","reviewed_columns_renamed_in_place",renamed}
	pins:=[]TopicPin{};for _,publication:=range current{pins=append(pins,TopicPin{Topic:publication.Definition.Topic,Version:publication.Definition.Version,Digest:publication.Digest})}
	if old.BindingDigest==exec.Hash(binding) && digest(pins)==digest(old.Topics){return "unchanged","exact_dependencies_unchanged",renamed}
	return "cosmetic","presentation_or_unreferenced_catalog_changed",renamed
}

func (s *Service) observeImpact(ctx context.Context,e identity.Envelope,id string,snapshot Snapshot)(impactWork,error){
	out:=impactWork{impact:Impact{ID:id,Revision:snapshot.Revision.Number,Version:snapshot.State.Version,DefinitionDigest:snapshot.Revision.Digest,Classification:"review_required",Reason:"missing_validation_baseline",Renames:[]Rename{},ObservedAt:time.Now().UTC()}}
	if snapshot.Validation==nil{return out,nil}
	observer,ok:=s.sources.(CatalogObserver);if !ok{return out,ErrUnavailable}
	ctx,cancel:=context.WithTimeout(ctx,time.Duration(s.limits.ValidationTimeout));defer cancel()
	select{case s.slots<-struct{}{}:defer func(){<-s.slots}();case <-ctx.Done():return out,ctx.Err();default:return out,ErrBusy}
	d:=snapshot.Revision.Definition
	out.publications=[]topics.Published{};out.pins=[]TopicPin{}
	partition:=""
	for _,pin:=range d.Topics{
		publication,err:=s.topics.Read(ctx,e,pin.Topic,"")
		if err!=nil{if impactAuthorization(err){return out,err};out.impact.Classification="unavailable";out.impact.Reason="semantic_catalog_unavailable";return out,nil}
		if publication.State.Archived || !publication.State.Active{out.impact.Reason="topic_not_active";return out,nil}
		for _,dataset:=range publication.Definition.Datasets{
			if dataset.Source.Source!=d.Source{out.impact.Reason="source_changed";return out,nil}
			if partition!="" && partition!=dataset.Source.Context{out.impact.Reason="mixed_execution_contexts";return out,nil};partition=dataset.Source.Context
		}
		out.publications=append(out.publications,publication);out.pins=append(out.pins,TopicPin{Topic:publication.Definition.Topic,Version:publication.Definition.Version,Digest:publication.Digest})
	}
	refs:=definitionReferences(d,out.publications)
	if err:=RequireReferences(e,Read,refs);err!=nil{return out,err}
	binding,err:=s.sources.ContextBinding(ctx,e,d.Source,partition)
	if err!=nil{if impactAuthorization(err){return out,err};out.impact.Classification="review_required";out.impact.Reason="registered_context_changed";return out,nil}
	out.binding=binding
	observed,err:=observer.ObserveCatalog(ctx,e,d.Source,partition)
	if err!=nil{
		if impactAuthorization(err){return out,err}
		out.impact.Classification="unavailable";out.impact.Reason="source_catalog_unavailable"
		if errors.Is(err,exec.ErrBinding){out.impact.Classification="review_required";out.impact.Reason="unregistered_source_drift"}
		return out,nil
	}
	if observed.BindingDigest!=exec.Hash(binding){return out,ErrStale}
	scope,err:=validationScope(binding,out.publications)
	if err!=nil{out.impact.Reason="current_semantics_do_not_match_source";return out,nil}
	ids:=[]string{};for _,dep:=range snapshot.Validation.Dependencies{ids=append(ids,dep.Dataset)}
	out.dependencies,err=deriveDependencies(binding,scope,ids)
	if err!=nil{out.impact.Reason="dependency_missing";return out,nil}
	classification,reason,renames:=compareImpact(*snapshot.Validation,binding,observed,out.publications)
	out.impact.Classification,out.impact.Reason=classification,reason
	if renames!=nil{out.impact.Renames=renames}
	out.impact.DependencyDigest=DependencyDigest(out.dependencies,out.pins)
	if classification=="rename" || classification=="cosmetic"{
		out.impact.ProposalDigest=digest(struct{Version,Revision,Binding string;Pins []TopicPin;Renames []Rename}{CanonicalizationVersion,snapshot.Revision.Digest,observed.BindingDigest,out.pins,out.impact.Renames})
	}
	return out,nil
}

// RecheckImpact explicitly observes source metadata and records health without
// modifying definitions, publication or historical certification evidence.
func (s *Service) RecheckImpact(ctx context.Context,e identity.Envelope,id string,in ImpactRequest)(Impact,error){
	ctx,cancel,err:=s.begin(ctx,e,id,Read);if err!=nil{return Impact{},err};defer cancel()
	snapshot,err:=s.repo.ReadBlock(ctx,e,id,in.Reference,Read);if err!=nil{return Impact{},err}
	if err:=expected(snapshot,in.ExpectedVersion);err!=nil{return Impact{},err}
	work,err:=s.observeImpact(ctx,e,id,snapshot);if err!=nil{return Impact{},err}
	health:=Health{Status:"stale",Reason:work.impact.Reason,ObservedAt:&work.impact.ObservedAt,DependencyDigest:work.impact.DependencyDigest}
	if work.impact.Classification=="unchanged"{health.Status="healthy"}
	if work.impact.Classification=="unavailable"{health.Status="unavailable"}
	state,err:=s.commit(ctx,e,Mutation{ID:id,Topic:snapshot.State.Topic,Kind:"health",ExpectedVersion:in.ExpectedVersion,TargetRevision:snapshot.Revision.Number,TargetDigest:snapshot.Revision.Digest,References:snapshotsReferences(snapshot),Health:&health})
	if err!=nil{return Impact{},err}
	work.impact.Version=state.Version
	return work.impact,nil
}

// ApplyImpact requires the current server-derived proposal and creates a private
// amendment. Native validation and explicit publication are still mandatory.
func (s *Service) ApplyImpact(ctx context.Context,e identity.Envelope,id string,in ApplyImpactRequest)(View,error){
	ctx,cancel,err:=s.begin(ctx,e,id,Write);if err!=nil{return View{},err};defer cancel()
	if in.Revision<1 || !hashValid(in.ProposalDigest) || !note(in.Note){return View{},ErrInvalid}
	snapshot,err:=s.repo.ReadBlock(ctx,e,id,Reference{Revision:in.Revision},Write);if err!=nil{return View{},err}
	if err:=expected(snapshot,in.ExpectedVersion);err!=nil{return View{},err}
	if snapshot.State.Archived{return View{},store.ErrConflict}
	work,err:=s.observeImpact(ctx,e,id,snapshot);if err!=nil{return View{},err}
	if work.impact.ProposalDigest!=in.ProposalDigest || work.impact.Classification!="rename" && work.impact.Classification!="cosmetic"{return View{},ErrStale}
	d:=clone(snapshot.Revision.Definition)
	if work.impact.Classification=="rename"{
		if work.binding.Dialect!="postgres"{return View{},ErrInvalid}
		d.SQL,err=renameSQL(ctx,d.SQL,work.impact.Renames,snapshot.Validation.Dependencies)
		if err!=nil{return View{},err};d.Template=nil
	}
	d.Context=work.binding.Context;d.Topics=clone(work.pins)
	for i:=range d.Parameters{if ref:=d.Parameters[i].Dimension;ref!=nil{for _,pin:=range d.Topics{if pin.Topic==ref.Topic{ref.Version=pin.Version}}}}
	for i:=range d.Outputs{
		if d.Outputs[i].Mapping==nil{continue}
		for j:=range d.Outputs[i].Mapping.Columns{
			provenance:=&d.Outputs[i].Mapping.Columns[j].Provenance
			if provenance.Source==d.Source{provenance.SourceRevision=work.binding.Revision}
			for _,pin:=range d.Topics{if provenance.Topic==pin.Topic{provenance.TopicVersion=pin.Version}}
		}
	}
	if err:=validateDefinition(ctx,d,s.limits,d.Template!=nil);err!=nil{return View{},err}
	_,refs,err:=s.resolveDefinitions(ctx,e,d,true);if err!=nil{return View{},err}
	provenance:=clone(snapshot.Revision.Provenance);provenance.Kind="dependency_amendment";provenance.ParentRevision=in.Revision;provenance.ChangeDigest=in.ProposalDigest
	r,err:=s.newRevision(e,snapshot.State.DraftRevision+1,d,provenance);if err!=nil{return View{},err}
	state,err:=s.commit(ctx,e,Mutation{ID:id,Topic:snapshot.State.Topic,Kind:"rename",ExpectedVersion:in.ExpectedVersion,TargetRevision:in.Revision,TargetDigest:snapshot.Revision.Digest,Note:in.Note,Revision:&r,References:refs,Watch:work.dependencies,Topics:work.pins,CheckCurrent:true})
	if err!=nil{return View{},err}
	return project(Snapshot{State:state,Revision:r},time.Now()),nil
}
