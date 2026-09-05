from pathlib import Path
# Keep workflow changes in the explicit connector commit, not a bot-generated code commit.
p=Path('scripts/_gateway_jobs_zfix.py');s=p.read_text();s=s[:s.index("p=Path('.github/workflows/gateway-jobs-work.yml')")];p.write_text(s)
p=Path('internal/foundation/command.go');s=p.read_text();needle='\ts, err := NewServer(cfg, r,';assert needle in s
s=s.replace(needle,'\tworkCtx, stopWork := context.WithCancel(ctx)\n defer stopWork()\n active, err := setupWork(workCtx, v, db, keyProbe, securityapi.Handler(keyProbe, service, r, v.Telemetry.Metrics), os.LookupEnv, log)\n if err != nil {return err}\n defer active.close()\n'+needle,1)
s=s.replace('keyProbe.Check, securityapi.Handler(keyProbe, service, r, v.Telemetry.Metrics))','keyProbe.Check, active.handler)',1)
s=s.replace('\treturn s.Serve(ctx, listener)','\tactive.run(workCtx)\n err = s.Serve(ctx, listener)\n stopWork()\n return err',1);p.write_text(s)
p=Path('internal/foundation/server.go');s=p.read_text().replace('"01-04-authority"','"01-06-gateway-jobs"');s=s.replace('\treturn out\n}', '\tif s.values.Features.Gateway {out=append(out,"remote_bifrost_gateway")}\n if s.values.Jobs.Enabled {out=append(out,"durable_operations","scheduling")}\n\treturn out\n}',1);p.write_text(s)
