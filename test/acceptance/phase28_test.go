package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

func phase28Scopes(tenant string)[]string{
	out:=[]string{}
	remove:=map[string]bool{"query.preflight":true,"query.plan":true,"query.execute":true,"reporting.sql.read":true,"reporting.certify":true,"cw.block.certify:*":true}
	for _,scope:=range phase27Scopes(tenant){if !remove[scope]{out=append(out,scope)}}
	return append(out,"reporting.execute","cw.block.execute:*","jobs.cancel","reporting.retention","cw.tenant.erase:"+tenant)
}

func phase28Envelope(t *testing.T,f *phase17Fixture,tenant,user,session string,scopes []string)identity.Envelope{
	t.Helper()
	claims:=f.model.token.claims(tenant,user,scopes);claims["session"]=session
	e,err:=f.model.token.verifier.Verify(context.Background(),f.model.token.sign(t,claims,nil),auth.HTTP)
	if err!=nil{t.Fatal(err)};return e
}

func phase28Runner(t *testing.T,f *phase17Fixture)*jobs.RequestRunner{
	t.Helper();limits:=jobs.Defaults();limits.Backoff=10*time.Millisecond
	runner,err:=jobs.NewRequestRunner(f.f.db,limits);if err!=nil{t.Fatal(err)};return runner
}

func phase28Chat(t *testing.T,content any)string{
	t.Helper();message,err:=json.Marshal(content);if err!=nil{t.Fatal(err)}
	raw,err:=json.Marshal(map[string]any{"id":"frozen-narrative-fixture","object":"chat.completion","model":"model-narrative","choices":[]any{map[string]any{"index":0,"message":map[string]any{"role":"assistant","content":string(message)},"finish_reason":"stop"}},"usage":map[string]int{"prompt_tokens":20,"completion_tokens":20,"total_tokens":40}})
	if err!=nil{t.Fatal(err)};return "chat_raw:"+string(raw)
}

type interruptedFrozenCheckpoint struct{
	reporting.RunRepository
	once atomic.Bool
}

func(r *interruptedFrozenCheckpoint) CheckpointFrozenRun(ctx context.Context,inv jobs.Invocation,proof reporting.PreparedRunWrite)(reporting.RunRecord,error){
	write,err:=proof.Checked(inv);if err!=nil{return reporting.RunRecord{},err}
	out,err:=r.RunRepository.CheckpointFrozenRun(ctx,inv,proof)
	if err==nil&&write.Kind=="result"&&r.once.CompareAndSwap(true,false){return reporting.RunRecord{},errors.New("synthetic interruption after durable normalized result")}
	return out,err
}

func TestFrozenRunsRealStore(t *testing.T){
	f:=newPhase18Fixture(t)
	query,topics:=newPhase18Service(t,f)
	blocks,err:=reporting.New(f.f.db,topics,f.f.s,f.f.validator,f.f.executor,reporting.CaptureFromQueries(query),config.DefaultReporting())
	if err!=nil{t.Fatal(err)};t.Cleanup(blocks.Close)
	ctx:=context.Background();e:=phase28Envelope(t,f,f.f.e.Tenant(),f.f.e.User(),"phase28-session",phase28Scopes(f.f.e.Tenant()))
	runner:=phase28Runner(t,f)
	runs,err:=reporting.NewRuns(blocks,f.f.db,runner,f.model.engine,"policy-v1",config.DefaultReportingExecution());if err!=nil{t.Fatal(err)}
	def:=phase27Definition(t,f,e,"SELECT id, amount FROM analytics.sales ORDER BY id")
	second:=phase27Copy(t,def.Outputs[0]);second.ID="table-secondary"
	def.Outputs=append(def.Outputs,second)
	def.Outputs[1].Narrative.SchemaVersion="grounded-narrative-v1"
	def.Outputs[1].Narrative.MaxTokens=8192
	def.Outputs[1].Narrative.Fields=[]string{"id","amount"}
	def.Outputs[1].Narrative.RedactedFields=[]string{"id"}
	created,err:=blocks.Create(ctx,e,reporting.CreateRequest{ID:"phase28-block",Definition:def});if err!=nil{t.Fatal(err)}
	published:=phase27ValidatePublish(t,blocks,e,created)
	var first reporting.RunView
	t.Run("frozen_outputs_and_read_only_views",func(t *testing.T){
		before:=f.model.requests.Load();f.model.mode.Store("error")
		admitted,admitErr:=runs.Admit(ctx,e,published.ID,reporting.RunRequest{Key:"phase28-multiple",Outputs:[]string{"table-main","table-secondary"}})
		if admitErr!=nil||admitted.ID==""{t.Fatalf("admit: %v %#v",admitErr,admitted)}
		first,err=runs.Run(ctx,e,admitted.ID,false)
		if err!=nil||first.State!="succeeded"||len(first.QueryAttempts)!=1||len(first.Outputs)!=2||first.QueryAttempts[0].Number!=1{t.Fatalf("frozen fanout: %v %#v",err,first)}
		if f.model.requests.Load()!=before{t.Fatal("frozen deterministic execution invoked inference")}
		reader:=phase28Envelope(t,f,e.Tenant(),"artifact-reader","reader-session",[]string{"reporting.read","cw.block.read:"+published.ID,"cw.execution_context.use:"+first.Context})
		lookups:=f.f.lookups.Load()
		page,readErr:=runs.Rows(ctx,reader,first.ID,0,1)
		if readErr!=nil||len(page.Rows)!=1||page.TotalRows!=2||page.Next==nil||len(page.Schema)!=2{t.Fatalf("retained paging: %v %#v",readErr,page)}
		for _,id:=range []string{"table-main","table-secondary"}{
			output,outputErr:=runs.Output(ctx,reader,first.ID,id);if outputErr!=nil||output.State!="succeeded"||output.Chart==nil{t.Fatal("retained output",outputErr,output)}
			rebuilt,buildErr:=runs.RebuildOutput(ctx,reader,first.ID,id);if buildErr!=nil||rebuilt.Digest!=output.Digest{t.Fatal("deterministic retained rebuild",buildErr)}
		}
		if f.f.lookups.Load()!=lookups||f.model.requests.Load()!=before{t.Fatal("retained read/rebuild called a source or model")}
		for _,bad:=range []identity.Envelope{
			phase28Envelope(t,f,"other-tenant",e.User(),e.Session(),[]string{"reporting.read","cw.block.read:*","cw.execution_context.use:*"}),
			phase28Envelope(t,f,e.Tenant(),e.User(),e.Session(),[]string{"reporting.read","cw.block.read:*","cw.execution_context.use:wrong-context"}),
			phase28Envelope(t,f,e.Tenant(),e.User(),e.Session(),[]string{"reporting.read","cw.execution_context.use:*"}),
		}{if _,denied:=runs.Rows(ctx,bad,first.ID,0,1);denied==nil{t.Fatal("retained values leaked across reach/context")}}
		if f.f.lookups.Load()!=lookups{t.Fatal("denied artifact access touched source credentials")}
	})
	t.Run("idempotency_and_checkpoint_recovery",func(t *testing.T){
		request:=reporting.RunRequest{Key:"phase28-recovery",Outputs:[]string{"table-main"}}
		ids:=make(chan string,4);failures:=make(chan error,4);var wait sync.WaitGroup
		for range 4{wait.Add(1);go func(){defer wait.Done();v,admitErr:=runs.Admit(ctx,e,published.ID,request);ids<-v.ID;failures<-admitErr}()}
		wait.Wait();close(ids);close(failures)
		var id string;for current:=range ids{if id!=""&&current!=id{t.Fatal("concurrent admission accepted multiple manifests")};id=current}
		for admitErr:=range failures{if admitErr!=nil{t.Fatal("concurrent admission",admitErr)}}
		request.Outputs=[]string{"table-secondary"};if _,conflict:=runs.Admit(ctx,e,published.ID,request);conflict==nil{t.Fatal("changed request reused an existing key")}
		interrupted:=&interruptedFrozenCheckpoint{RunRepository:f.f.db};interrupted.once.Store(true)
		crashing,constructorErr:=reporting.NewRuns(blocks,interrupted,runner,nil,"",config.DefaultReportingExecution());if constructorErr!=nil{t.Fatal(constructorErr)}
		failed,failure:=crashing.Run(ctx,e,id,false)
		if failure==nil||failed.State=="succeeded"||len(failed.QueryAttempts)!=1{t.Fatalf("checkpoint interruption hidden: %v %#v",failure,failed)}
		lookups:=f.f.lookups.Load();calls:=f.model.requests.Load();time.Sleep(20*time.Millisecond)
		recovered,recoverErr:=runs.Run(ctx,e,id,true)
		if recoverErr!=nil||recovered.State!="succeeded"||recovered.Attempts!=2||len(recovered.QueryAttempts)!=1{t.Fatalf("normalized recovery: %v %#v",recoverErr,recovered)}
		if f.f.lookups.Load()!=lookups||f.model.requests.Load()!=calls{t.Fatal("normalized checkpoint recovery repeated query/inference")}
		again,replayErr:=runs.Run(ctx,e,id,true);if replayErr!=nil||again.ManifestDigest!=recovered.ManifestDigest||again.Attempts!=2{t.Fatal("completed replay changed manifest/attempts",replayErr)}
	})
	t.Run("grounded_redacted_narrative",func(t *testing.T){
		f.model.mode.Store(phase28Chat(t,map[string]any{"claims":[]any{map[string]any{"kind":"difference","evidence":[]string{"e1","e2"}}}}))
		before:=f.model.requests.Load()
		admitted,admitErr:=runs.Admit(ctx,e,published.ID,reporting.RunRequest{Key:"phase28-narrative",Outputs:[]string{"narrative-main"},Narrative:true})
		if admitErr!=nil{t.Fatal(admitErr)}
		completed,runErr:=runs.Run(ctx,e,admitted.ID,false);if runErr!=nil||completed.State!="succeeded"{t.Fatalf("narrative: %v %#v",runErr,completed)}
		output,outputErr:=runs.Output(ctx,e,completed.ID,"narrative-main");if outputErr!=nil||output.Narrative==nil{t.Fatal(outputErr)}
		n:=output.Narrative
		if len(n.Evidence)!=2||n.Evidence[0].Field!="amount"||n.Evidence[1].Field!="amount"||len(n.Claims)!=1||!strings.Contains(n.Text,"Difference")||len(n.Receipt.Calls)!=1||f.model.requests.Load()-before!=1{t.Fatalf("narrative evidence/provenance: %#v",n)}
		if n.SchemaVersion!="grounded-narrative-v1"||n.ModelVersion!="policy-v1"||n.PromptVersion!="summary-v1"||n.OutputHash==""||n.EvidenceHash==""{t.Fatal("narrative provenance omitted")}
	})
	t.Run("private_preview_survives_later_publication",func(t *testing.T){
		private,createErr:=blocks.Create(ctx,e,reporting.CreateRequest{ID:"phase28-private",Definition:def});if createErr!=nil{t.Fatal(createErr)}
		validated,validationErr:=blocks.Validate(ctx,e,private.ID,reporting.ValidationRequest{ExpectedVersion:private.Version});if validationErr!=nil{t.Fatal(validationErr)}
		admitted,admitErr:=runs.Admit(ctx,e,private.ID,reporting.RunRequest{Key:"phase28-preview",Reference:reporting.Reference{Revision:validated.View.Revision},Policy:"private_preview",Outputs:[]string{"table-main"}})
		if admitErr!=nil{t.Fatal(admitErr)}
		completed,runErr:=runs.Run(ctx,e,admitted.ID,false);if runErr!=nil||!completed.Private{t.Fatalf("preview run: %v %#v",runErr,completed)}
		if _,publishErr:=blocks.Publish(ctx,e,private.ID,reporting.EvidenceRequest{ExpectedVersion:validated.View.Version,Evidence:validated.Evidence.ID});publishErr!=nil{t.Fatal(publishErr)}
		outsider:=phase28Envelope(t,f,e.Tenant(),"other-reader",e.Session(),[]string{"reporting.read","reporting.preview","cw.run.read:*","cw.block.read:*","cw.block.preview:*","cw.execution_context.use:*"})
		if _,denied:=runs.Rows(ctx,outsider,completed.ID,0,1);denied==nil{t.Fatal("publication exposed original private preview")}
	})
	t.Run("explicit_cancel_and_disabled_output",func(t *testing.T){
		before:=f.f.lookups.Load();models:=f.model.requests.Load()
		for index,request:=range []reporting.RunRequest{{Key:"phase28-unknown",Outputs:[]string{"unknown"}},{Key:"phase28-disabled-narrative",Outputs:[]string{"narrative-main"}}}{
			if _,bad:=runs.Admit(ctx,e,published.ID,request);bad==nil{t.Fatalf("invalid selection %d accepted",index)}
		}
		if f.f.lookups.Load()!=before||f.model.requests.Load()!=models{t.Fatal("invalid output selection performed execution")}
		admitted,admitErr:=runs.Admit(ctx,e,published.ID,reporting.RunRequest{Key:"phase28-cancel",Outputs:[]string{"table-main"}});if admitErr!=nil{t.Fatal(admitErr)}
		cancelled,cancelErr:=runs.Cancel(ctx,e,admitted.ID);if cancelErr!=nil||cancelled.State!="cancelled"{t.Fatal("durable cancellation",cancelErr,cancelled)}
		if _,runErr:=runs.Run(ctx,e,admitted.ID,false);runErr==nil{t.Fatal("cancelled operation ran without explicit resume")}
		if f.f.lookups.Load()!=before{t.Fatal("pre-dispatch cancellation performed a query")}
	})
}
