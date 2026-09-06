from pathlib import Path

def replace(path,old,new):
 p=Path(path);s=p.read_text()
 if new in s:return
 assert old in s,(path,old[:100])
 p.write_text(s.replace(old,new,1))

replace('sdk/chartworks/client.go','func (c *Client) call(ctx context.Context, method, path, key string, body, out any) error {','''func (c *Client) call(ctx context.Context, method, path, key string, body, out any) error {
 return c.callLimit(ctx,method,path,key,body,out,1<<20)
}
func (c *Client) callLimit(ctx context.Context, method,path,key string,body,out any,limit int64) error {''')
p=Path('sdk/chartworks/client.go');s=p.read_text();a=s.index('func (c *Client) callLimit(');b=s.index('// RetentionPolicy',a);part=s[a:b].replace('(1<<20)+1','limit+1').replace('len(data) <= 1<<20','int64(len(data)) <= limit').replace('len(data) > 1<<20','int64(len(data)) > limit');s=s[:a]+part+s[b:];s=s.replace('Timeout: 30 * time.Second','Timeout: 75 * time.Second');p.write_text(s)
replace('internal/config/config.go','WriteTimeout: Duration(30 * time.Second)','WriteTimeout: Duration(75 * time.Second)')
replace('internal/foundation/work.go','var validator *readexec.Validator','var executor *readexec.Executor\n var validator *readexec.Validator')
replace('internal/foundation/work.go','w.handler = sourceapi.Handler(','''if validator!=nil {
 executor,err=readexec.NewExecutor(w.sourceService,db,v.Exec)
 if err!=nil { w.close();return nil,err }
 }
 w.handler = sourceapi.Handler(''')
replace('internal/foundation/work.go','return w, nil','w.handler=sourceapi.ExecutionHandler(verifier,validator,executor,w.handler)\n return w, nil')
replace('internal/sourceapi/http.go','case errors.Is(err, store.ErrConflict):','case errors.Is(err, readexec.ErrReplay), errors.Is(err, store.ErrConflict):')
replace('internal/sourceapi/http.go','case errors.Is(err, readexec.ErrUnsupported):','case errors.Is(err, readexec.ErrType), errors.Is(err, readexec.ErrUnsupported):')
# Binding is a metadata-only read; disabled warehouse access still fails in Explain/Execute.
p=Path('internal/sources/sources.go');s=p.read_text();a=s.index('func (s *Service) Binding(');b=s.index('// Explain',a);s=s[:a]+s[a:b].replace('s.call(ctx, e, true,','s.call(ctx, e, false,')+s[b:];p.write_text(s)
# FETCH is cursor-dependent: a connection's old prepared FETCH description must not be reused.
replace('internal/sources/read.go','" FROM chartworks_read", pgx.QueryResultFormatsByOID','" FROM chartworks_read", pgx.QueryExecModeDescribeExec, pgx.QueryResultFormatsByOID')
