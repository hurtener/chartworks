from pathlib import Path

def change(path,old,new):
 p=Path(path);s=p.read_text()
 if s.count(old)!=1: raise SystemExit('edit did not match once: '+path)
 p.write_text(s.replace(old,new))

change('internal/telemetry/telemetry.go','func New(w io.Writer, format string) (*Reporter, error) {','func New(w io.Writer, format string, enabled bool) (*Reporter, error) {')
change('internal/telemetry/telemetry.go','logger   *slog.Logger','logger   *slog.Logger') if False else None
p=Path('internal/telemetry/telemetry.go');s=p.read_text();s=s.replace('type Reporter struct {','type Reporter struct {\n enabled bool')
s=s.replace('r := &Reporter{','r := &Reporter{enabled: enabled, ')
s=s.replace('r.events.WithLabelValues(string(event), outcome).Inc()','if r.enabled { r.events.WithLabelValues(string(event), outcome).Inc() }')
s=s.replace('r.ready.WithLabelValues(name).Set(value)','if r.enabled { r.ready.WithLabelValues(name).Set(value) }')
s=s.replace('return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})','if !r.enabled { return http.NotFoundHandler() }; return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})')
p.write_text(s)
for p in Path('test').rglob('*.go'):
 s=p.read_text()
 for old,new in [('telemetry.New(io.Discard, "json")','telemetry.New(io.Discard, "json", true)'),('telemetry.New(io.Discard, "text")','telemetry.New(io.Discard, "text", true)'),('telemetry.New(&logs, "json")','telemetry.New(&logs, "json", true)'),('telemetry.New(nil, "json")','telemetry.New(nil, "json", true)'),('telemetry.New(io.Discard, "unknown")','telemetry.New(io.Discard, "unknown", true)')]:s=s.replace(old,new)
 p.write_text(s)
change('internal/foundation/command.go','telemetry.New(log, v.Telemetry.LogFormat)','telemetry.New(log, v.Telemetry.LogFormat, v.Telemetry.Metrics)')
change('internal/foundation/keys.go','"context"','"context"\n "bytes"')
change('internal/foundation/keys.go','c := http.Client{}','c := http.Client{Transport: http.DefaultTransport.(*http.Transport).Clone()}')
change('internal/foundation/command.go','keyProbe := NewKeyProbe(v.Auth, nil)','keyProbe := NewKeyProbe(v.Auth, nil)\n defer keyProbe.Close()')
change('internal/foundation/keys.go','func validKeys(data []byte, allowed []string) bool {','func validKeys(data []byte, allowed []string) bool {\n d:=json.NewDecoder(bytes.NewReader(data));if uniqueValue(d,0)!=nil{return false};if _,e:=d.Token();!errors.Is(e,io.EOF){return false}')
change('internal/foundation/keys.go','for _, k := range doc.Keys {','for _, k := range doc.Keys {\n for _,field:=range []string{"kid","kty","use","alg","n","e","crv","x","y"}{if raw,present:=k[field];present{var v string;if bytes.Equal(bytes.TrimSpace(raw),[]byte("null"))||json.Unmarshal(raw,&v)!=nil{return false}}}')
with Path('internal/foundation/keys.go').open('a') as f:f.write('''
// Close releases idle verification-key connections after the monitor has joined.
func(p *KeyProbe)Close(){p.client.CloseIdleConnections()}
func uniqueValue(d *json.Decoder,depth int)error{
 if depth>16{return errors.New("invalid key document")};v,e:=d.Token();if e!=nil||v==nil{return errors.New("invalid key document")}
 if delimiter,ok:=v.(json.Delim);ok{switch delimiter{case '{':seen:=map[string]bool{};for d.More(){k,err:=d.Token();if err!=nil{return err};key,ok:=k.(string);if !ok||seen[key]{return errors.New("duplicate key field")};seen[key]=true;if e=uniqueValue(d,depth+1);e!=nil{return e}};case '[':for d.More(){if e=uniqueValue(d,depth+1);e!=nil{return e}};default:return errors.New("invalid key document")};_,e=d.Token();return e};return nil
}
''')
# Exact statuses and all metadata changes are staged by the branch normalizer.
r=Path('docs/plans/phase-registry.json');import json
v=json.loads(r.read_text())
for n in ('01','02'): v['phases'][n]['status']='in_progress'
r.write_text(json.dumps(v,indent=2)+'\n')
for n in ('01','02'):
 p=next(Path('docs/plans').glob('phase-'+n+'-*.md'));s=p.read_text().replace('Status: planned.','Status: in_progress.');p.write_text(s)
