package acceptance

import (
 "bytes"
 "encoding/json"
 "os"
 "strings"
 "testing"
 "time"

 "github.com/hurtener/chartworks/internal/config"
)

func TestReadExecutionReferenceExcerpt(t *testing.T){
 excerpt,err:=os.ReadFile("../../examples/chartworks.execution.json");if err!=nil{t.Fatal(err)}
 defaults,_:=json.Marshal(config.Defaults());var base,patch map[string]any
 if json.Unmarshal(defaults,&base)!=nil || json.Unmarshal(excerpt,&patch)!=nil{t.Fatal("reference JSON")}
 for key,value:=range patch{dst,ok:=base[key].(map[string]any);if !ok{t.Fatal("reference unknown object")};for field,v:=range value.(map[string]any){dst[field]=v}}
 document,_:=json.Marshal(base)
 lookup:=func(key string)(string,bool){return "postgres://synthetic:synthetic@localhost:5434/reference?sslmode=disable",key=="CHARTWORKS_STORE_URL"}
 loaded,err:=config.Load(bytes.NewReader(document),lookup,config.Overrides{})
 if err!=nil{t.Fatal("actual configuration decoder rejected execution excerpt",err)}
 v:=loaded.Values();if v.Exec.RowsCeiling!=100000 || v.Exec.Timeout!=config.Duration(time.Minute) || v.Server.WriteTimeout!=config.Duration(75*time.Second){t.Fatal("reference did not match default contract")}
 for _,bad:=range []string{strings.Replace(string(document),`"rows_ceiling":100000`,`"rows_ceiling":100001`,1),strings.Replace(string(document),`"timeout":"60s"`,`"skip_validation":true,"timeout":"60s"`,1)}{
  if bad==string(document){t.Fatal("negative configuration fixture was not mutated")}
  if _,err=config.Load(strings.NewReader(bad),lookup,config.Overrides{});err==nil{t.Fatal("invalid execution configuration")}
 }
}
