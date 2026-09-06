from pathlib import Path
p=Path('internal/sources/read.go');s=p.read_text();a=s.index('func (s *Service) ControlRead(');b=s.index('// readCompatibility',a)
s=s[:a]+'''func (s *Service) ControlRead(ctx context.Context,e identity.Envelope,control readexec.Control,_ bool) (string,error) {
 id,partition:=control.Coordinates()
 scope,err:=sourceScope(e,"sources.query","query",id);if err!=nil { return "unknown",err }
 state:="unknown"
 err=s.call(ctx,e,true,func(ctx context.Context) error {
  return s.repo.WithSource(ctx,scope,id,func(ctx context.Context,record Record) error {
   if record.Source.ContextID!=partition { return readexec.ErrBinding }
   if _,err:=control.Target(e,record.Binding);err!=nil { return err }
   c,err:=s.connection(e.Tenant(),record.Connection);if err!=nil { return err }
   // Prove actual credentials/catalog context before observing a native identity;
   // a replacement database cannot falsely prove an old backend stopped.
   _,err=s.probe(ctx,c,id,record.Source.Revision,func(ctx context.Context,tx readTransaction,b readexec.Binding) error {
    if readexec.Hash(b)!=readexec.Hash(record.Binding) { return readexec.ErrBinding }
    q,err:=control.Target(e,b);if err!=nil { return err }
    var exists bool
    if err=tx.QueryRow(ctx,`SELECT EXISTS(SELECT 1 FROM pg_catalog.pg_stat_activity WHERE pid=$1 AND backend_start=$2 AND application_name=$3 AND usename=current_user AND datname=current_database())`,q.PID,q.Started,q.Tag).Scan(&exists);err!=nil { return safe(err) }
    state="stopped";if exists { state="running" };return nil
   })
   return err
  })
 })
 return state,err
}

'''+s[b:];p.write_text(s)

# Bound retirement to one old pool per alias and join every reaper on shutdown.
p=Path('internal/sources/sources.go');s=p.read_text()
old='pools     map[string]poolEntry';assert old in s
s=s.replace(old,'pools     map[string]poolEntry\n retiring map[string]bool\n reap sync.WaitGroup',1)
old='pools: map[string]poolEntry{}';assert old in s;s=s.replace(old,'pools: map[string]poolEntry{},retiring:map[string]bool{}',1)
a=s.index('func (s *Service) Close()');b=s.index('func (s *Service) call(',a)
s=s[:a]+'''func (s *Service) Close() {
 s.lifecycle.Lock();defer s.lifecycle.Unlock()
 if s.closed { return };s.closed=true
 s.mu.Lock()
 for key,entry:=range s.pools { entry.pool.Close();delete(s.pools,key) }
 s.mu.Unlock()
 s.reap.Wait()
}
'''+s[b:];p.write_text(s)
p=Path('internal/sources/postgres.go');s=p.read_text()
needle='cfg, err := pgxpool.ParseConfig(dsn)';assert needle in s;s=s.replace(needle,'if s.retiring[key] { s.mu.Unlock();return nil,"",readexec.ErrBinding }\n '+needle,1)
old='''	s.mu.Unlock()
	if old.pool != nil {
		old.pool.Close()
	}'''
new='''if old.pool!=nil {
 s.retiring[key]=true;s.reap.Add(1)
 go func(){
  defer s.reap.Done();old.pool.Close()
  s.mu.Lock();delete(s.retiring,key);s.mu.Unlock()
 }()
}
 s.mu.Unlock()'''
assert old in s;s=s.replace(old,new,1);p.write_text(s)
p=Path('internal/exec/execution.go');s=p.read_text();a=s.index('func (m Manifest) Valid() bool');b=s.index('// Attempt is',a)
s=s[:a]+'''func (m Manifest) Valid() bool {
 r:=m.Receipt
 if !identity.Identifier(m.Operation) || !identity.Identifier(m.Session) || !r.Validated || !identity.Identifier(r.Source) || !identity.Identifier(r.Context) || !identity.Identifier(r.Contract) || len(r.Dependencies)>32 || len(r.Columns)<1 || len(r.Columns)>256 || !m.Limits.Valid() { return false }
 hash,err:=hex.DecodeString(r.Manifest);if err!=nil || len(hash)!=32 || hex.EncodeToString(hash)!=r.Manifest { return false }
 seen:=map[string]bool{}
 for _,id:=range r.Dependencies { if !identity.Identifier(id) || seen[id] { return false };seen[id]=true }
 for _,name:=range r.Columns { if len(name)<1 || len(name)>1024 { return false } }
 return true
}

'''+s[b:];p.write_text(s)
p=Path('internal/store/postgres/errors_test.go');s=p.read_text();assert 'SchemaVersion() != "5"' in s;s=s.replace('SchemaVersion() != "5"','SchemaVersion() != "6"',1);p.write_text(s)
