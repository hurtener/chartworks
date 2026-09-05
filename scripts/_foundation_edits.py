# Temporary, exact-match development edits. The branch bootstrap removes this file
# after applying them; it is not part of the delivered application/toolchain.
from pathlib import Path

def replace(path, old, new):
    p=Path(path); s=p.read_text()
    if s.count(old)!=1:
        raise SystemExit('expected exact source edit not found: '+path)
    p.write_text(s.replace(old,new))

p='internal/store/postgres/postgres.go'
replace(p,'"sync/atomic"','"sync/atomic"\n "strings"\n "strconv"')
replace(p,'cfg,err:=pgxpool.ParseConfig(dsn);','if strings.TrimSpace(dsn)==""||len(dsn)>16384||opts.TransactionTimeout<time.Millisecond{return nil,store.ErrInvalid}\n cfg,err:=pgxpool.ParseConfig(dsn);')
replace(p,'store.ErrNotFound,store.ErrUnavailable','store.ErrNotFound,store.ErrUnavailable,store.ErrExpired')
replace(p,'func id() string {var b [16]byte;if _,err:=rand.Read(b[:]);err!=nil{panic("cryptographic randomness unavailable")};return hex.EncodeToString(b[:])}','func newID()(string,error){var b [16]byte;if _,err:=rand.Read(b[:]);err!=nil{return "",store.ErrUnavailable};return hex.EncodeToString(b[:]),nil}')
replace(p,'d.timeout.String());err!=nil','strconv.FormatInt(d.timeout.Milliseconds(),10));err!=nil')
replace(p,'out=store.Policy{Revision:expected+1','eventID,e:=newID();if e!=nil{return out,e}\n out=store.Policy{Revision:expected+1')
replace(p,'s.Tenant(),id(),s.Actor());return e','s.Tenant(),eventID,s.Actor());return e')
p='internal/store/store.go'
replace(p,'// ErrMigration indicates','// ErrExpired retains an operation-key tombstone without re-executing work.\n ErrExpired = errors.New("store: operation expired")\n // ErrMigration indicates')
p='internal/store/postgres/migrations/002_maintenance_operations.sql'
replace(p,"status IN ('pending','running','succeeded')","status IN ('pending','running','succeeded','expired')")
replace(p,"(status = 'succeeded') = (finished_at IS NOT NULL)","(status IN ('succeeded','expired')) = (finished_at IS NOT NULL)")
p='internal/store/postgres/operations.go'
replace(p,'err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error {\n\t\t_,e:=tx.Exec','operationID,e:=newID();if e!=nil{return out,e}\n err=d.transaction(ctx,func(ctx context.Context,tx pgx.Tx)error {\n\t\t_,e:=tx.Exec')
replace(p,'s.Tenant(),id(),s.Actor(),key,hash,limit)','s.Tenant(),operationID,s.Actor(),key,hash,limit)')
replace(p,'var storedHash string','var storedHash string;var expired bool')
replace(p,'`,request_hash FROM chartworks.operations','`,request_hash,(expires_at<=clock_timestamp()) FROM chartworks.operations')
replace(p,'&out.DeletedOperations,&storedHash);e!=nil{return e};if storedHash!=hash{return store.ErrConflict};return nil','&out.DeletedOperations,&storedHash,&expired);e!=nil{return e};if storedHash!=hash||out.Limit!=limit{return store.ErrConflict};if expired||out.Status=="expired"{return store.ErrExpired};return nil')
replace(p,"AND status<>'succeeded' AND expires_at", "AND status IN ('pending','running') AND expires_at")
replace(p,'DELETE FROM chartworks.operations WHERE (tenant_id,operation_id) IN','UPDATE chartworks.operations SET status=\'expired\',deleted_events=0,deleted_operations=0 WHERE (tenant_id,operation_id) IN')
replace(p,'_,e=tx.Exec(ctx,`INSERT INTO chartworks.audit_events','eventID,e:=newID();if e!=nil{return e}\n _,e=tx.Exec(ctx,`INSERT INTO chartworks.audit_events')
replace(p,'s.Tenant(),id(),s.Actor(),out.ID)','s.Tenant(),eventID,s.Actor(),out.ID)')
p='internal/foundation/server.go'
replace(p,'if !ok{state="starting"}else if v.Ready {state="ready";if !v.ValidUntil.IsZero()&&!now.Before(v.ValidUntil){state="stale"}}','if !ok{state="starting"}else if !v.ValidUntil.IsZero()&&!now.Before(v.ValidUntil){state="stale"}else if v.Ready {state="ready"}')
p='internal/foundation/command.go'
replace(p,'"os"','"os"\n "time"')
replace(p,'timeDuration(v.Store.ConnectTimeout)','time.Duration(v.Store.ConnectTimeout)')
replace(p,'timeDuration(v.Store.TransactionTimeout)','time.Duration(v.Store.TransactionTimeout)')
Path('internal/foundation/duration.go').unlink()
p='internal/config/config.go'
replace(p,'"fmt"','"reflect"\n "strconv"')
replace(p,'v := Defaults()','if err=checkShape(b,reflect.TypeOf(Values{}),"document");err!=nil{return Config{},err}\n v := Defaults()')
replace(p,'if e!=nil || p=="" || h==""','port,portErr:=strconv.Atoi(p)\n if e!=nil || portErr!=nil || port<0 || port>65535 || p=="" || h==""')
replace(p,'return fmt.Errorf("write configuration defaults: %w",err)','return invalid("defaults","output failed")')
with Path(p).open('a') as f:f.write('''
// checkShape reports a known containing field rather than echoing an unknown input key.
func checkShape(data []byte,typ reflect.Type,path string)error{
 switch typ.Kind(){
 case reflect.Struct:
  var object map[string]json.RawMessage;if json.Unmarshal(data,&object)!=nil{return nil}
  known:=map[string]reflect.Type{};for i:=0;i<typ.NumField();i++{field:=typ.Field(i);known[strings.Split(field.Tag.Get("json"),",")[0]]=field.Type}
  for key,value:=range object{child,ok:=known[key];if !ok{return invalid(path,"unknown or retired field")};if e:=checkShape(value,child,path+"."+key);e!=nil{return e}}
 case reflect.Map:
  var object map[string]json.RawMessage;if json.Unmarshal(data,&object)!=nil{return nil};for _,value:=range object{if e:=checkShape(value,typ.Elem(),path+".entry");e!=nil{return e}}
 case reflect.Slice:
  var values []json.RawMessage;if json.Unmarshal(data,&values)!=nil{return nil};for _,value:=range values{if e:=checkShape(value,typ.Elem(),path+".item");e!=nil{return e}}
 };return nil
}
''')
