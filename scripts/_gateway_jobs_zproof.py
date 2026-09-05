from pathlib import Path
p=Path('internal/auth/execution.go');s=p.read_text().replace('(identity.Envelope, error)', '(Execution, error)').replace('return identity.Envelope{},','return Execution{},').replace('return e, nil','return Execution{envelope:e,binding:binding,job:job,manifest:digest}, nil')
s+='\n// Execution is an opaque proof produced only by the dedicated, manifest-bound verifier.\n// A regular verified HTTP/MCP envelope cannot be passed to a durable effect by accident.\ntype Execution struct {envelope identity.Envelope;binding,job,manifest string}\nfunc (e Execution) Envelope() identity.Envelope {return e.envelope}\nfunc (e Execution) Matches(binding,job,manifest string) bool {return e.envelope.Valid() && e.binding==binding && e.job==job && e.manifest==manifest && manifest!=""}\n'
p.write_text(s)
p=Path('internal/jobs/jobs.go');s=p.read_text().replace('"github.com/hurtener/chartworks/internal/access"','"github.com/hurtener/chartworks/internal/access"\n "github.com/hurtener/chartworks/internal/auth"')
s=s.replace('Acquire(context.Context, Job) (identity.Envelope, error)','Acquire(context.Context, Job) (auth.Execution, error)')
s=s.replace('func AssertExecution(e identity.Envelope, j Job) error {','func AssertExecution(proof auth.Execution, j Job) error {\n e:=proof.Envelope()\n if !proof.Matches(j.BindingID,j.ID,j.ManifestHash){return ErrAuthority}')
s=s.replace('CompleteJob(context.Context, Lease, identity.Envelope)', 'CompleteJob(context.Context, Lease, auth.Execution)');p.write_text(s)
p=Path('internal/jobs/pengui/authority.go');s=p.read_text().replace('(identity.Envelope, error)', '(auth.Execution, error)').replace('return identity.Envelope{},','return auth.Execution{},');p.write_text(s)
p=Path('internal/jobs/service.go');s=p.read_text().replace('envelope.Deadline()', 'envelope.Envelope().Deadline()');p.write_text(s)
p=Path('internal/store/postgres/jobs.go');s=p.read_text().replace('"github.com/hurtener/chartworks/internal/access"','"github.com/hurtener/chartworks/internal/access"\n "github.com/hurtener/chartworks/internal/auth"')
s=s.replace('lease jobs.Lease, e identity.Envelope)', 'lease jobs.Lease, proof auth.Execution)')
needle='ctx, cancel := context.WithDeadline(ctx, e.Deadline())';assert needle in s
s=s.replace(needle,'e := proof.Envelope()\n '+needle,1).replace('jobs.AssertExecution(e, lease.Job)','jobs.AssertExecution(proof, lease.Job)').replace('jobs.AssertExecution(e, out)','jobs.AssertExecution(proof, out)');p.write_text(s)
p=Path('internal/foundation/work.go');s=p.read_text().replace('type work struct {','type work struct {ctx context.Context;').replace('&work{cancel:cancel,','&work{ctx:ctx,cancel:cancel,').replace('func(w *work)run(ctx context.Context){','func(w *work)run(){\n ctx:=w.ctx');p.write_text(s)
p=Path('internal/foundation/command.go');s=p.read_text().replace('active.run(workCtx)','active.run()');p.write_text(s)
