"""Temporary branch-only editing aid; remove before final review."""
from pathlib import Path
import hashlib
import subprocess
changed=set()
def replace(path,old,new):
 p=Path(path);text=p.read_text()
 if new in text:return
 if text.count(old)!=1:raise RuntimeError(f'Expected one anchor in {path}: {old[:180]!r}')
 p.write_text(text.replace(old,new));changed.add(path)
def write(path,text):
 p=Path(path);p.parent.mkdir(parents=True,exist_ok=True)
 if p.exists():raise RuntimeError(f'Unexpected file: {path}')
 if isinstance(text,bytes):p.write_bytes(text)
 else:p.write_text(text)
 subprocess.run(['git','add','-N','--',path],check=True);changed.add(path)

root=Path(subprocess.check_output(['go','env','GOROOT'],text=True).strip())
assert subprocess.check_output(['go','version'],text=True).split()[2]=='go1.26.4'
data=(root/'lib/time/zoneinfo.zip').read_bytes()
assert 100000<len(data)<1000000
sha=hashlib.sha256(data).hexdigest()
write('internal/calendars/zoneinfo.zip',data)
write('internal/calendars/LICENSE', (root/'LICENSE').read_bytes())
write('internal/calendars/version.go', '// Code generated from the Go 1.26.4 timezone archive; DO NOT EDIT.\npackage calendars\n\n// Version identifies the exact bytes, not the host OS timezone database.\nconst Version="go1.26.4-sha256:'+sha+'"\nconst archiveSHA256="'+sha+'"\n')
write('internal/calendars/README.md', '# Pinned timezone rules\n\n`zoneinfo.zip` is the exact public timezone archive shipped with Go 1.26.4.\nSHA-256: `'+sha+'`. The adjacent Go license is retained. This identifier\ndescribes the actual archive; it is not an invented IANA release label.\n\nJobs and reporting share `Location`; host TZ/ZONEINFO files are not consulted.\nThe archive has no customer data, network loaders or credentials. Updating it\nis a reviewed data/config migration: update the checksum, test DST/leap/first\noccurrences, and record the new version. Already accepted reporting occurrences\nkeep their due instant, half-open window and resolved parameters. Queue replicas\nrefuse a different registered timezone archive.\n')
write('internal/calendars/zones.go', '''// Package calendars supplies one pinned, credential-free timezone database.
package calendars

import (
 "archive/zip"
 "bytes"
 "crypto/sha256"
 _ "embed"
 "encoding/hex"
 "errors"
 "io"
 "strings"
 "sync"
 "time"
)

//go:embed zoneinfo.zip
var archive []byte
var indexOnce sync.Once
var index map[string]*zip.File
var indexErr error
var locations sync.Map

// ErrZone rejects unknown or unsafe names and corrupted archive data.
var ErrZone=errors.New("calendar: timezone unavailable")

func archiveIndex(){
 sum:=sha256.Sum256(archive)
 if hex.EncodeToString(sum[:])!=archiveSHA256{indexErr=ErrZone;return}
 reader,err:=zip.NewReader(bytes.NewReader(archive),int64(len(archive)))
 if err!=nil||len(reader.File)>2048{indexErr=ErrZone;return}
 files:=make(map[string]*zip.File,len(reader.File))
 for _,file:=range reader.File{
  if file.UncompressedSize64==0||file.UncompressedSize64>65536||files[file.Name]!=nil{indexErr=ErrZone;return}
  files[file.Name]=file
 }
 index=files
}

// Location ignores mutable system files and environment-selected databases.
// Successful entries alone are cached, bounding cache size by the archive.
func Location(name string)(*time.Location,error){
 if name=="UTC"{return time.UTC,nil}
 if len(name)==0||len(name)>128||name=="Local"||strings.Contains(name,"..")||strings.HasPrefix(name,"/")||strings.ContainsAny(name,"\\\\\\x00\\r\\n"){return nil,ErrZone}
 indexOnce.Do(archiveIndex)
 if indexErr!=nil{return nil,indexErr}
 if value,ok:=locations.Load(name);ok{return value.(*time.Location),nil}
 file:=index[name]
 if file==nil{return nil,ErrZone}
 reader,err:=file.Open()
 if err!=nil{return nil,ErrZone}
 data,readErr:=io.ReadAll(io.LimitReader(reader,65537))
 closeErr:=reader.Close()
 if readErr!=nil||closeErr!=nil||len(data)>65536{return nil,ErrZone}
 zone,err:=time.LoadLocationFromTZData(name,data)
 if err!=nil{return nil,ErrZone}
 actual,_:=locations.LoadOrStore(name,zone)
 return actual.(*time.Location),nil
}
''')
write('internal/calendars/zones_test.go', '''package calendars

import (
 "crypto/sha256"
 "encoding/hex"
 "sync"
 "testing"
 "time"
)
func TestPinnedTimezoneArchive(t *testing.T){
 sum:=sha256.Sum256(archive)
 if Version!="go1.26.4-sha256:"+hex.EncodeToString(sum[:]){t.Fatal("archive/version mismatch")}
 t.Setenv("ZONEINFO","/does/not/exist")
 zone,err:=Location("America/New_York")
 if err!=nil{t.Fatal(err)}
 before:=time.Date(2024,3,10,6,59,59,0,time.UTC).In(zone)
 after:=before.Add(time.Second)
 if before.Hour()!=1||after.Hour()!=3{t.Fatal("spring transition",before,after)}
 first:=time.Date(2024,11,3,5,30,0,0,time.UTC).In(zone)
 second:=first.Add(time.Hour)
 _,a:=first.Zone();_,b:=second.Zone()
 if first.Hour()!=1||second.Hour()!=1||a==b{t.Fatal("fold transition",first,second)}
 for _,name:=range []string{"","Local","../UTC","/etc/passwd","UTC\\x00","missing","Etc/../UTC","America\\\\New_York"}{
  if _,err:=Location(name);err==nil{t.Fatalf("accepted unsafe/absent zone %q",name)}
 }
 if zone,err:=Location("UTC");err!=nil||zone!=time.UTC{t.Fatal("UTC",err)}
}
func TestTimezoneConcurrentReuse(t *testing.T){
 var wg sync.WaitGroup
 for i:=0;i<100;i++{wg.Add(1);go func(){defer wg.Done();zone,err:=Location("America/Argentina/Buenos_Aires");if err!=nil||zone==nil{t.Error("concurrent location",err)}}()}
 wg.Wait()
}
func FuzzTimezoneNames(f *testing.F){
 for _,name:=range []string{"UTC","Europe/Paris","Australia/Lord_Howe","../UTC",""}{f.Add(name)}
 f.Fuzz(func(t *testing.T,name string){zone,err:=Location(name);if err==nil&&zone==nil{t.Fatal("nil accepted zone")}})
}
''')
write('internal/store/postgres/migrations/032_timezone_database.sql', '''-- Existing accepted due/window instants are untouched. New workers agree on
-- the exact timezone archive before accepting or claiming more occurrences.
ALTER TABLE chartworks.queue_limits ADD COLUMN timezone_database text;
ALTER TABLE chartworks.queue_limits ADD CONSTRAINT queue_timezone_version CHECK
 (timezone_database IS NULL OR timezone_database ~ '^go[0-9]+\\.[0-9]+\\.[0-9]+-sha256:[a-f0-9]{64}$');
''')
p='internal/jobs/schedule.go'
replace(p,'"github.com/robfig/cron/v3"','"github.com/robfig/cron/v3"\n "github.com/hurtener/chartworks/internal/calendars"')
text=Path(p).read_text();assert text.count('time.LoadLocation(')==2
Path(p).write_text(text.replace('time.LoadLocation(', 'calendars.Location('));changed.add(p)
p='internal/reporting/periods.go'
replace(p,'\t_ "time/tzdata" // Named-zone behavior remains available in minimal binaries.', '\t"github.com/hurtener/chartworks/internal/calendars"')
replace(p,'time.LoadLocation(name)', 'calendars.Location(name)')
p='internal/config/jobs.go'
replace(p,'\t"time"', '\t"time"\n "github.com/hurtener/chartworks/internal/calendars"')
replace(p,'type Jobs struct {','type Jobs struct {\n TimezoneDatabaseVersion string `json:"timezone_database_version"`')
replace(p,'return Jobs{Workers:', 'return Jobs{TimezoneDatabaseVersion:calendars.Version, Workers:')
replace(p,'func ValidateJobs(j Jobs, a Auth) error {','func ValidateJobs(j Jobs, a Auth) error {\n if j.TimezoneDatabaseVersion!="" && j.TimezoneDatabaseVersion!=calendars.Version{return invalid("jobs.timezone_database_version","configured timezone archive does not match the bundled version")}')
write('internal/config/jobs_timezone_test.go','''package config
import (
 "testing"
 "github.com/hurtener/chartworks/internal/calendars"
)
func TestJobsTimezoneDatabaseVersion(t *testing.T){
 j:=DefaultJobs()
 if j.TimezoneDatabaseVersion!=calendars.Version||ValidateJobs(j,Auth{})!=nil{t.Fatal("pinned default")}
 j.TimezoneDatabaseVersion="unknown"
 if ValidateJobs(j,Auth{})==nil{t.Fatal("accepted another database")}
 j.TimezoneDatabaseVersion=""
 if ValidateJobs(j,Auth{})!=nil{t.Fatal("omission must use the bundled default")}
}
''')
p='internal/store/postgres/jobs.go'
replace(p,'\t"github.com/hurtener/chartworks/internal/auth"','\t"github.com/hurtener/chartworks/internal/auth"\n "github.com/hurtener/chartworks/internal/calendars"')
replace(p,'\tif fingerprint != l.QueueFingerprint() {',''' if _,err:=tx.Exec(ctx,`UPDATE chartworks.queue_limits SET timezone_database=$1 WHERE singleton AND timezone_database IS NULL`,calendars.Version);err!=nil{return err}
 var timezoneVersion string
 if err:=tx.QueryRow(ctx,`SELECT timezone_database FROM chartworks.queue_limits WHERE singleton`).Scan(&timezoneVersion);err!=nil{return err}
 if timezoneVersion!=calendars.Version{return store.ErrConflict}
\tif fingerprint != l.QueueFingerprint() {''')
subprocess.run(['gofmt','-w',*sorted(p for p in changed if p.endswith('.go'))],check=True)
subprocess.run(['git','diff','--check'],check=True)
