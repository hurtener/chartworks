package acceptance
import("net/http";"net/http/httptest")
func httpRequestWithDuplicateHeader()*http.Request{r:=httptest.NewRequest("GET","/",nil);r.Header.Add("Authorization","Bearer one");r.Header.Add("Authorization","Bearer two");return r}
func newRecorder()*httptest.ResponseRecorder{return httptest.NewRecorder()}

func FuzzAuthorityJSON(f *testing.F){
 for _,s:=range []string{`{}`,`{"a":1,"a":2}`,`{"x":null}`,`{"a":[1]}`} {f.Add([]byte(s))}
 f.Fuzz(func(t *testing.T,b []byte){_,_=auth.Object(b,8192)})
}
