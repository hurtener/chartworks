package jobs

import (
 "encoding/json"
 "strings"
 "time"

 "github.com/robfig/cron/v3"
)

// Spec supports only executable trigger kinds. Definitions are immutable; pause/resume uses CAS.
type Spec struct {
 Type string `json:"type"`
 Cron string `json:"cron,omitempty"`
 Timezone string `json:"timezone"`
 IntervalSeconds int64 `json:"interval_seconds,omitempty"`
 Anchor time.Time `json:"anchor,omitempty"`
 Missed string `json:"missed"`
 MaxCatchUp int `json:"max_catch_up,omitempty"`
 Overlap string `json:"overlap"`
}
func(s Spec)Validate()error{
 if s.Missed!="skip"&&s.Missed!="catch_up"||s.Overlap!="skip"&&s.Overlap!="queue"||s.MaxCatchUp<0||s.MaxCatchUp>32||s.Missed=="catch_up"&&s.MaxCatchUp<1||s.Timezone==""||len(s.Timezone)>128{return ErrInvalid}
 if _,err:=time.LoadLocation(s.Timezone);err!=nil{return ErrInvalid}
 switch s.Type{
 case "manual":if s.Cron!=""||s.IntervalSeconds!=0||!s.Anchor.IsZero(){return ErrInvalid}
 case "interval":if s.Cron!=""||s.IntervalSeconds<60||s.IntervalSeconds>31*86400||s.Anchor.IsZero()||s.Anchor.Year()<2000||s.Anchor.Year()>2100||s.Anchor.Nanosecond()!=0{return ErrInvalid}
 case "cron":if s.IntervalSeconds!=0||!s.Anchor.IsZero()||len(s.Cron)>128||strings.ContainsAny(s.Cron,"@=\r\n") {return ErrInvalid};if _,err:=s.parsed();err!=nil{return err}
 default:return ErrInvalid
 };return nil
}
func(s Spec)parsed()(cron.Schedule,error){
 location,err:=time.LoadLocation(s.Timezone);if err!=nil{return nil,ErrInvalid}
 parsed,err:=cron.NewParser(cron.Minute|cron.Hour|cron.Dom|cron.Month|cron.Dow).Parse(s.Cron);if err!=nil{return nil,ErrInvalid}
 schedule,ok:=parsed.(*cron.SpecSchedule);if !ok{return nil,ErrInvalid};schedule.Location=location;return schedule,nil
}
// Next returns the next real occurrence as UTC; an interval is anchored, never now+interval drift.
func(s Spec)Next(after time.Time)(time.Time,error){
 if s.Validate()!=nil||after.Year()<2000||after.Year()>2100{return time.Time{},ErrInvalid}
 if s.Type=="manual"{return time.Time{},nil}
 if s.Type=="interval"{anchor:=s.Anchor.Unix();n:=(after.Unix()-anchor)/s.IntervalSeconds;if after.Unix()<anchor{return s.Anchor.UTC(),nil};return time.Unix(anchor+(n+1)*s.IntervalSeconds,0).UTC(),nil}
 schedule,err:=s.parsed();if err!=nil{return time.Time{},err};next:=schedule.Next(after);if next.IsZero(){return next,ErrInvalid};return next.UTC(),nil
}
// Previous finds the exact occurrence at or before a time. The monotone Next function is
// searched, rather than approximating the previous cron occurrence by subtracting an interval.
func(s Spec)Previous(at time.Time)(time.Time,error){
 if s.Validate()!=nil||at.Year()<2000||at.Year()>2100{return time.Time{},ErrInvalid}
 if s.Type=="manual"{return at.UTC().Truncate(time.Microsecond),nil}
 if s.Type=="interval"{if at.Before(s.Anchor){return s.Anchor.UTC().Add(-time.Duration(s.IntervalSeconds)*time.Second),nil};return time.Unix(s.Anchor.Unix()+((at.Unix()-s.Anchor.Unix())/s.IntervalSeconds)*s.IntervalSeconds,0).UTC(),nil}
 schedule,err:=s.parsed();if err!=nil{return time.Time{},err};lo,hi:=at.Unix()-366*5*86400,at.Unix()
 first:=schedule.Next(time.Unix(lo,0));if first.IsZero()||first.After(at){return time.Time{},ErrInvalid}
 for lo<hi{mid:=lo+(hi-lo)/2;next:=schedule.Next(time.Unix(mid,0));if next.IsZero()||next.After(at){hi=mid}else{lo=mid+1}}
 previous:=time.Unix(lo,0).UTC();if !schedule.Next(previous.Add(-time.Second)).Equal(previous){return time.Time{},ErrInvalid};return previous,nil
}

// ScheduleRequest admits a fixed implemented target and a closed recurrence definition.
type ScheduleRequest struct {Target Submission `json:"target"`;Spec Spec `json:"spec"`}
func(r ScheduleRequest)Validate()error{if r.Target.Validate()!=nil{return ErrInvalid};return r.Spec.Validate()}
type Schedule struct {
 ID string `json:"id"`;Revision int64 `json:"revision"`;Enabled bool `json:"enabled"`;Tenant string `json:"tenant"`
 Initiator string `json:"initiator"`;InitiatorSession string `json:"initiator_session"`;Request ScheduleRequest `json:"request"`
 NextDue *time.Time `json:"next_due,omitempty"`;PreviousDue *time.Time `json:"previous_due,omitempty"`
}
func(r ScheduleRequest)JSON()[]byte{b,_:=json.Marshal(r);return b}
