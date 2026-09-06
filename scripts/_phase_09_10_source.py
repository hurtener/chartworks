from pathlib import Path
p=Path('internal/sources/postgres.go');s=p.read_text()
if 'func (s *Service) inspectReadContext(' not in s:
    start=s.index('\t// This catalog/type/exposure proof')
    end=s.index('\tif consume != nil',start)
    proof=s[start:end]
    s=s[:start]+'''\tout,err=s.inspectReadContext(ctx,tx,c,id,revision,location)
 if err!=nil { return readexec.Binding{},err }
'''+s[end:]
    s+='''\n// inspectReadContext is shared by native planning and the bounded read cursor.
func (s *Service) inspectReadContext(ctx context.Context,tx readTransaction,c config.SourceConnection,id string,revision int64,location string) (out readexec.Binding,err error) {
'''+proof+'\n return out,nil\n}\n'
    p.write_text(s)
# Keep one cursor/result execution core, with the original phase-08 public wrapper.
p=Path('internal/sources/sources.go');s=p.read_text()
start=s.index('func (s *Service) Read(');end=s.index('\nfunc safe(',start)
s=s[:start]+'''func (s *Service) Read(ctx context.Context,e identity.Envelope,p readexec.Plan) (Rows,error) {
 return s.readCompatibility(ctx,e,p)
}
'''+s[end:];p.write_text(s)
p=Path('internal/sources/postgres.go');s=p.read_text()
if 'func readRows(' in s:
    start=s.index('func readRows(');end=s.index('// supportedPostgresVersion',start);s=s[:start]+s[end:]
if 'pgconn/ctxwatch' not in s:
    s=s.replace('"github.com/jackc/pgx/v5/pgconn"','"github.com/jackc/pgx/v5/pgconn"\n "github.com/jackc/pgx/v5/pgconn/ctxwatch"')
    s=s.replace('cfg.ConnConfig.OnNotice = nil','''cfg.ConnConfig.BuildContextWatcherHandler=func(conn *pgconn.PgConn) ctxwatch.Handler {
 return &pgconn.CancelRequestContextWatcherHandler{Conn:conn,CancelRequestDelay:0,DeadlineDelay:2*time.Second}
 }
 cfg.ConnConfig.OnNotice = nil''')
# Bound each backend message independently from the aggregate normalized response cap.
s=s.replace('maximum := s.settings.MaxBytes + 32768','maximum := (16 << 20) + 32768')
p.write_text(s)
