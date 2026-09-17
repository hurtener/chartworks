from pathlib import Path


def edit(path, before, after, count=1):
    p=Path(path); text=p.read_text()
    if text.count(before)!=count: raise SystemExit(f'{path}: changed anchor {text.count(before)}')
    p.write_text(text.replace(before,after))


def create(path,text):
    p=Path(path)
    if p.exists(): raise SystemExit(f'{path}: exists')
    p.write_text(text)

edit('internal/nlqexec/clarification_origin.go','"reflect"','"slices"')
edit('internal/nlqexec/clarification_origin.go','reflect.DeepEqual(', 'slices.Equal(',4)
edit('internal/semantics/clarification_values_test.go','!reflect.DeepEqual(en, es) || en.Time.StartUTC', '!reflect.DeepEqual(en.Time, es.Time) || !reflect.DeepEqual(en.Effect, es.Effect) || en.Locale != "en" || es.Locale != "es" || en.ParserVersion != es.ParserVersion || en.Time.StartUTC')
edit('internal/semantics/clarification_types.go','type CanonicalClarificationTime struct {','type CanonicalClarificationTime struct {\n BoundaryPolicy string `json:"boundary_policy"`')
edit('internal/semantics/clarification_values.go','''	case "unsupported_grain":''',''' case "ambiguous_calendar_boundary":
  message,spanish="The local midnight is missing or ambiguous in the reviewed timezone. Choose an unambiguous boundary; no offset was guessed.","La medianoche local no existe o es ambigua en la zona horaria revisada. Elegí un límite inequívoco; no se supuso ningún desplazamiento horario."
	case "unsupported_grain":''')
edit('internal/semantics/clarification_values.go','''	return CanonicalClarificationTime{StartUTC:''',''' if !uniqueClarificationMidnight(start) || !uniqueClarificationMidnight(end) {
  return CanonicalClarificationTime{},clarificationError(locale,"time.boundary","ambiguous_calendar_boundary")
 }
	return CanonicalClarificationTime{BoundaryPolicy: ClarificationTimeBoundaryPolicy,StartUTC:''')

create('internal/semantics/clarification_calendar.go',r'''package semantics

import "time"

// ClarificationTimeBoundaryPolicy is part of every canonical time resolution.
// Dates mean local calendar midnights. Gaps and folds are rejected rather than
// delegated to time.Date's unspecified choice of an offset during transitions.
const ClarificationTimeBoundaryPolicy = "reject_missing_or_ambiguous_midnight"

func uniqueClarificationMidnight(candidate time.Time) bool {
 if candidate.Hour()!=0 || candidate.Minute()!=0 || candidate.Second()!=0 || candidate.Nanosecond()!=0 {return false}
 zone:=candidate.Location()
 year,month,day:=candidate.Date()
 wall:=time.Date(year,month,day,0,0,0,0,time.UTC)
 // Adjacent zone periods cover every possible civil offset at this boundary.
 // ZoneBounds makes the work bounded by transitions, not by elapsed seconds.
 // An unusually dense/unsupported transition table fails closed.
 cursor,limit:=candidate.Add(-48*time.Hour),candidate.Add(48*time.Hour)
 offsets:=map[int]bool{}
 complete:=false
 for transitions:=0;transitions<16;transitions++ {
  _,offset:=cursor.In(zone).Zone()
  if offset < -24*60*60 || offset > 24*60*60 {return false}
  offsets[offset]=true
  _,end:=cursor.In(zone).ZoneBounds()
  if end.IsZero() || end.After(limit) {complete=true;break}
  if !end.After(cursor) {return false}
  cursor=end
 }
 if !complete{return false}
 matches:=0
 for offset:=range offsets {
  actual:=wall.Add(-time.Duration(offset)*time.Second).In(zone)
  y,m,d:=actual.Date()
  if y==year && m==month && d==day && actual.Hour()==0 && actual.Minute()==0 && actual.Second()==0 && actual.Nanosecond()==0 {matches++}
 }
 return matches==1
}
''')

create('internal/semantics/clarification_calendar_test.go',r'''package semantics

import (
 "testing"
 "time"
)

func TestClarificationCalendarTransitions(t *testing.T) {
 for _,tc:=range []struct{name,zone,start,end string; accepted bool}{
  {"ordinary-day","America/Argentina/Buenos_Aires","2026-01-02","2026-01-03",true},
  {"leap-day","UTC","2024-02-29","2024-03-01",true},
  {"missing-civil-day","Pacific/Apia","2011-12-30","2012-01-01",false},
  {"ambiguous-midnight","America/Havana","2025-11-02","2025-11-03",false},
  {"short-day-with-unambiguous-boundaries","America/New_York","2025-03-09","2025-03-10",true},
 } {t.Run(tc.name,func(t *testing.T){
  slot:=cw01TimeSlot();slot.Effect.TimeZone=tc.zone;slot.Effect.Grains=[]string{"day"}
  resolution,err:=ResolveClarificationValue(slot,ClarificationValue{Time:&ClarificationTimeInput{Start:tc.start,End:tc.end,Grain:"day",Calendar:"gregorian",TimeZone:tc.zone}},"en")
  if (err==nil)!=tc.accepted {t.Fatalf("calendar acceptance: %v",err)}
  if !tc.accepted {if resolution.Time!=nil {t.Fatal("invalid boundary produced partial resolution")};return}
  if resolution.Time.BoundaryPolicy!=ClarificationTimeBoundaryPolicy || resolution.Time.Bounds!="[)" {t.Fatal("boundary policy missing")}
  if tc.name=="short-day-with-unambiguous-boundaries" {
   start,e1:=time.Parse(time.RFC3339,resolution.Time.StartUTC);end,e2:=time.Parse(time.RFC3339,resolution.Time.EndUTC)
   if e1!=nil || e2!=nil || end.Sub(start)!=23*time.Hour {t.Fatal("calendar day coerced to 24 elapsed hours")}
  }
 })}
}

func FuzzClarificationCalendar(f *testing.F) {
 for _,seed:=range []string{"2026-01-01","2024-02-29","2025-11-02","2011-12-30","2026-02-30","","9999-12-31"} {f.Add(seed)}
 f.Fuzz(func(t *testing.T,start string){
  slot:=cw01TimeSlot();slot.Effect.Grains=[]string{"day"};slot.Effect.TimeZone="UTC"
  resolution,err:=ResolveClarificationValue(slot,ClarificationValue{Time:&ClarificationTimeInput{Start:start,End:"2027-01-01",Grain:"day",Calendar:"gregorian",TimeZone:"UTC"}},"en")
  if err!=nil {if resolution.Time!=nil {t.Fatal("failed parse returned a time constraint")};return}
  if resolution.Time==nil || resolution.Time.LocalStart!=start || resolution.Time.BoundaryPolicy!=ClarificationTimeBoundaryPolicy {t.Fatal("calendar parser lost explicit input")}
  roundtrip,problem:=ResolveClarificationValue(slot,ClarificationValue{Time:&ClarificationTimeInput{Start:resolution.Time.LocalStart,End:resolution.Time.LocalEnd,Grain:"day",Calendar:resolution.Time.Calendar,TimeZone:resolution.Time.TimeZone}},"es")
  if problem!=nil || *roundtrip.Time!=*resolution.Time {t.Fatal("canonical time cannot roundtrip independently of locale")}
 })
}
''')

p=Path('test/acceptance/cw01_test.go'); text=p.read_text()
anchor='''	t.Run("AC03", func(t *testing.T) {'''
insert=''' t.Run("BilingualNamedPeriod",func(t *testing.T){
  period:=func(label string)semantics.ClarificationValue{return semantics.ClarificationValue{Time:&semantics.ClarificationTimeInput{Period:label,Calendar:"gregorian",TimeZone:"America/Argentina/Buenos_Aires",Grain:"month"}}}
  en:=f.plan(t,f.question("Show dated sales",nlq.LanguageEnglish),"period",period("January 2026"))
  es:=f.plan(t,f.question("Mostrá ventas fechadas",nlq.LanguageSpanish),"period",period("enero de 2026"))
  if !reflect.DeepEqual(en.Route.Resolutions[0].Time,es.Route.Resolutions[0].Time) {t.Fatal("different language month inputs changed canonical business window")}
  f.run(t,en,2,true);f.run(t,es,2,true)
 })
'''
# Keep the named acceptance corpus exactly AC01..AC10: this belongs under AC02.
boundary='''		f.run(t, child, 1, false)
	})
'''
if text.count(boundary)!=1: raise SystemExit('AC02 boundary changed')
text=text.replace(boundary,'''\t\tf.run(t, child, 1, false)
'''+insert+'''\t})
''')
p.write_text(text)
