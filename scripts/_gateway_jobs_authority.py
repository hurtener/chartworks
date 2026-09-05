from pathlib import Path
p=Path('internal/config/auth.go');s=p.read_text().replace('type Audiences struct {','type Audiences struct {\n Jobs string `json:"jobs,omitempty"`');needle='\tif len(a.Algorithms) == 0'
s=s.replace(needle,'\tif a.Audiences.Jobs != "" && (len(a.Audiences.Jobs)>512 || !strings.HasSuffix(a.Audiences.Jobs, ":execution") || strings.ContainsAny(a.Audiences.Jobs, " \\t\\r\\n") || a.Audiences.Jobs==a.HTTPAudience() || a.Audiences.Jobs==a.MCPAudience()) {return invalid("auth.audiences.jobs", "distinct execution audience required")}\n'+needle,1);p.write_text(s)
p=Path('internal/auth/verifier.go');s=p.read_text().replace('\tMCP\n)','\tMCP\n // Execution is accepted only by the durable worker, never the API/MCP router.\n Execution\n)',1)
s=s.replace('\texpected := ""','\tskew := time.Duration(v.cfg.ClockSkew)\n\texpected := ""',1)
s=s.replace('\tcase MCP:\n\t\texpected = v.cfg.MCPAudience()','\tcase MCP:\n\t\texpected = v.cfg.MCPAudience()\n case Execution:\n  expected=v.cfg.Audiences.Jobs\n  skew=0\n  if expected=="" || len(audiences)!=1 || expires-issued>60 {return fail()}')
s=s.replace('jwt.WithLeeway(time.Duration(v.cfg.ClockSkew))','jwt.WithLeeway(skew)').replace('time.Unix(expires, 0).Add(time.Duration(v.cfg.ClockSkew))','time.Unix(expires, 0).Add(skew)');p.write_text(s)
p=Path('internal/config/config.go');s=p.read_text().replace('type Values struct {','type Values struct {\n Jobs Jobs `json:"jobs"`',1).replace('\t\tTelemetry: Telemetry{','\t\tJobs: DefaultJobs(),\n\t\tTelemetry: Telemetry{',1)
s=s.replace('\tv.Auth.Algorithms = append','\tv.Jobs.Credentials = append([]BrokerCredential(nil),v.Jobs.Credentials...)\n\tv.Auth.Algorithms = append',1)
s=s.replace('{"auth.audiences.mcp", &v.Auth.Audiences.MCP}', '{"auth.audiences.mcp", &v.Auth.Audiences.MCP}, {"auth.audiences.jobs", &v.Auth.Audiences.Jobs}, {"jobs.broker_url", &v.Jobs.BrokerURL}')
s=s.replace('\treturn ValidateGateway(v.Gateway, v.Features.Gateway)','\tif err:=ValidateJobs(v.Jobs,v.Auth);err!=nil{return err}\n\treturn ValidateGateway(v.Gateway, v.Features.Gateway)');p.write_text(s)
# A missing job is an empty successful poll, not a rollback of bounded expiry cleanup.
p=Path('internal/store/postgres/jobs.go');s=p.read_text();a=s.index('func(d *DB)ClaimJob');b=s.index('func(d *DB)HeartbeatJob',a)
x=s[a:b].replace('err=d.transaction','empty:=false\n err=d.transaction',1).replace('return jobs.ErrEmpty','empty=true;return nil').replace('});return out,err','});if err==nil&&empty{return out,jobs.ErrEmpty};return out,err',1);s=s[:a]+x+s[b:];p.write_text(s)
# The environment has no networked Go toolchain; this temporary branch workflow materializes edits.
p=Path('.github/workflows/gateway-jobs-work.yml');s=p.read_text().replace('for f in scripts/_gateway_jobs_edits.py scripts/_gateway_jobs_followup.py; do','for f in scripts/_gateway_jobs*.py; do');p.write_text(s)
