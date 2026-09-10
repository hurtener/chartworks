package engineering

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

func proposalRequestMatches(p AutopilotProposal,r AutopilotApplyRequest)bool{
	return r.Revision==p.Revision&&r.Digest==p.Digest&&r.ExpectedVersion>0
}

func(s *Autopilot) currentProposalBinding(ctx context.Context,e identity.Envelope,m ProposalMaterial)error{
	binding,err:=s.pipelines.source.Binding(ctx,e,m.Binding.Source,m.Binding.Context);if err!=nil{return err}
	if readexec.Hash(binding)!=readexec.Hash(m.Binding){return ErrProposalDrift}
	return nil
}

// Apply commits the exact reviewed pipeline draft/publication as separate local
// effects, records the shared operation ID before native work, then invokes the
// ordinary pipeline runner. Partial effects remain inspectable and resumable.
func(s *Autopilot) Apply(ctx context.Context,e identity.Envelope,id string,r AutopilotApplyRequest)(AutopilotProposal,error){
	ctx,stop,err:=s.begin(ctx,e,true);if err!=nil{return AutopilotProposal{},err};defer stop()
	p,err:=s.repo.ReadAutopilotProposal(ctx,e,id,"engineering.autopilot.apply");if err!=nil{return AutopilotProposal{},err}
	if !proposalRequestMatches(p,r){return AutopilotProposal{},store.ErrConflict}
	if p.State=="compensated"||p.State=="rejected"{return AutopilotProposal{},ErrProposalConflict}
	if p.State=="approved"&&p.Version!=r.ExpectedVersion{return AutopilotProposal{},store.ErrConflict}
	if p.ApplyActor!=""&&(p.ApplyActor!=e.User()||p.ApplySession!=e.Session()){return AutopilotProposal{},access.ErrForbidden}
	if p.State=="applied"{return p,nil}
	if err=s.currentProposalBinding(ctx,e,p.Material);err!=nil{return p,err}
	// Proposal approval grants none of these ordinary operational actions.
	for _,action:=range []string{"engineering.pipeline.write","engineering.pipeline.publish","engineering.pipeline.run"}{
		if err=pipelineAuthority(e,p.Material.Pipeline,action,"write");err!=nil{return p,err}
	}
	if err=s.pipelines.validateInputs(ctx,e,p.Material.Pipeline);err!=nil{return p,err}
	for _,stage:=range []string{"draft","publish"}{
		proof,proofErr:=prepareProposalApply(e,p,"engineering.autopilot.apply","",nil);if proofErr!=nil{return p,proofErr}
		p,err=s.repo.StageAutopilotPipeline(ctx,e,proof,stage);if err!=nil{return p,err}
	}
	key:=readexec.Hash([]any{AutopilotVersion,e.Tenant(),p.ID,p.Revision,p.Digest})
	run,err:=s.pipelines.AdmitRun(ctx,e,p.Material.Pipeline.ID,p.Material.Request.ExpectedPipelineVersion+1,key)
	if err!=nil{return p,err}
	proof,err:=prepareProposalApply(e,p,"engineering.autopilot.apply","",nil);if err!=nil{return p,err}
	p,err=s.repo.RecordAutopilotRun(ctx,e,proof,run);if err!=nil{return p,err}
	if p.State=="applied"{return p,nil}
	run,runErr:=s.pipelines.Run(ctx,e,p.Material.Pipeline.ID,p.Material.Request.ExpectedPipelineVersion+1,key,r.Resume)
	if run.Operation.ID==""{return p,runErr}
	proof,err=prepareProposalApply(e,p,"engineering.autopilot.apply","",nil);if err!=nil{return p,err}
	p,err=s.repo.RecordAutopilotRun(ctx,e,proof,run);if err!=nil{return p,err}
	return p,runErr
}

// Compensate retires only a newly created, still-current, independently owned
// managed generation. The native output locks stay held through the metadata
// decision. Existing or referenced generations are blocked rather than undone.
func(s *Autopilot) Compensate(ctx context.Context,e identity.Envelope,id string,r AutopilotApplyRequest)(AutopilotProposal,error){
	ctx,stop,err:=s.begin(ctx,e,true);if err!=nil{return AutopilotProposal{},err};defer stop()
	p,err:=s.repo.ReadAutopilotProposal(ctx,e,id,"engineering.autopilot.compensate");if err!=nil{return AutopilotProposal{},err}
	if !proposalRequestMatches(p,r){return p,store.ErrConflict}
	if p.State=="compensated"{return p,nil}
	if p.State!="applied"||p.Version!=r.ExpectedVersion||p.Material.Request.ExpectedPipelineVersion!=0||p.Operation==""{return p,ErrCompensationBlocked}
	if p.ApplyActor!=e.User()||p.ApplySession!=e.Session(){return p,access.ErrForbidden}
	if err=pipelineAuthority(e,p.Material.Pipeline,"engineering.pipeline.run","write");err!=nil{return p,err}
	x,err:=s.pipelines.repo.ReadPipelineExecution(ctx,e,p.Operation);if err!=nil{return p,err}
	if x.State!="published"||x.Operation.State!="succeeded"||x.Digest!=readexec.Hash(p.Material.Pipeline){return p,ErrCompensationBlocked}
	steps:=make([]string,0,len(x.Stages))
	for _,stage:=range x.Stages{
		if stage.Previous!=nil||stage.State!="checked"||stage.Stage.Revision!=1||!stage.Stage.Valid(){return p,ErrCompensationBlocked}
		if err=access.Require(e,"engineering.autopilot.compensate",access.Resource{Tenant:e.Tenant(),Kind:"source",Permission:"write",ID:stage.Stage.Source});err!=nil{return p,err}
		steps=append(steps,stage.Stage.Step)
	}
	if len(steps)!=len(p.Material.Pipeline.Steps){return p,ErrCompensationBlocked}
	var out AutopilotProposal
	err=s.pipelines.source.WithPipelineOutputs(ctx,e,p.Operation,steps,func(held context.Context,records []sources.Record)error{
		if len(records)!=len(x.Stages){return ErrOwnership}
		for _,record:=range records{if record.Pipeline==nil||record.Pipeline.Operation!=p.Operation{return ErrOwnership}}
		proof,proofErr:=prepareProposalApply(e,p,"engineering.autopilot.compensate",readexec.Hash(x),nil);if proofErr!=nil{return proofErr}
		var compensateErr error
		out,compensateErr=s.repo.CompensateAutopilot(held,e,proof,x)
		return compensateErr
	})
	if err!=nil{return p,err};return out,nil
}

// DetectDrift creates bounded reviewable evidence, never a rewritten pipeline,
// topic, block or report. Current-source observations are made only on request.
func(s *Autopilot) DetectDrift(ctx context.Context,e identity.Envelope,id string)(AutopilotDrift,error){
	ctx,stop,err:=s.begin(ctx,e,true);if err!=nil{return AutopilotDrift{},err};defer stop()
	p,err:=s.repo.ReadAutopilotProposal(ctx,e,id,"engineering.autopilot.drift");if err!=nil{return AutopilotDrift{},err}
	if p.State!="applied"&&p.State!="applying"{return AutopilotDrift{},ErrState}
	binding,err:=s.pipelines.source.Binding(ctx,e,p.Material.Binding.Source,p.Material.Binding.Context)
	if err!=nil{return AutopilotDrift{},err}
	now:=time.Now().UTC().Truncate(time.Microsecond)
	drift:=AutopilotDrift{Proposal:p.ID,Revision:p.Revision,PriorBindingDigest:readexec.Hash(p.Material.Binding),CurrentBindingDigest:readexec.Hash(binding),Observed:now,State:"proposed",Impacts:[]ProposalImpact{}}
	switch{
	case drift.PriorBindingDigest!=drift.CurrentBindingDigest:drift.Kind="schema_changed"
	case p.Applied!=nil&&now.Sub(*p.Applied)>time.Duration(p.Material.Request.MaxStalenessSeconds)*time.Second:drift.Kind="freshness_expired"
	default:
		if p.Operation==""{return AutopilotDrift{},store.ErrNotFound}
		x,readErr:=s.pipelines.repo.ReadPipelineExecution(ctx,e,p.Operation);if readErr!=nil{return AutopilotDrift{},readErr}
		if x.State!="quality_failed"{return AutopilotDrift{},store.ErrNotFound}
		drift.Kind="quality_failed"
	}
	drift.EvidenceDigest=readexec.Hash([]any{AutopilotVersion,p.ID,p.Revision,p.Digest,drift.Kind,drift.PriorBindingDigest,drift.CurrentBindingDigest,p.Operation,p.Applied})
	proof,err:=prepareProposalApply(e,p,"engineering.autopilot.drift","",&drift);if err!=nil{return AutopilotDrift{},err}
	return s.repo.RecordAutopilotDrift(ctx,e,proof,drift,time.Duration(s.limits.AmendmentDedupInterval))
}
