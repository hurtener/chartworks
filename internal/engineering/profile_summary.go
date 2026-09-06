package engineering

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

func emptyModelReceipt()gateway.Receipt{return gateway.Receipt{Calls:[]gateway.Usage{}}}
func profileSummarySchema()(*gateway.Schema,error){return gateway.NewSchema("profile_summary",[]byte(`{"type":"object","additionalProperties":false,"required":["summary"],"properties":{"summary":{"type":"string","minLength":1,"maxLength":1024}}}`))}

// SummaryContext structurally excludes SQL, table/column names, raw values,
// min/max values, caller identity, context identifiers and provider credentials.
// The model sees only ordinal column labels, type families and aggregate counts.
func SummaryContext(p Profile)(string,error){
	type column struct{ID string `json:"id"`;Category string `json:"category"`;Observed int `json:"observed"`;Nulls int `json:"nulls"`;Distinct int `json:"sample_distinct"`;Exact bool `json:"distinct_exact"`;Families map[string]int `json:"families"`}
	out:=struct{Columns []column `json:"columns"`;Complete bool `json:"complete_sample"`;Freshness string `json:"freshness"`;ScanBounded bool `json:"physical_scan_bounded"`}{Columns:make([]column,len(p.Columns)),Complete:p.Sampling.Complete,Freshness:p.Freshness.State,ScanBounded:false}
	for i,c:=range p.Columns{
		category:="other";switch c.Category{case "integer","decimal","number","boolean","text","temporal","binary","json","structured","numeric","string","date","timestamp":category=c.Category}
		families:=map[string]int{}
		for family,count:=range c.Families{switch family{case "null","integer","decimal","number","boolean","binary","temporal","json","text","empty_text","whitespace_text","email_like","uuid_like","date_like":families[family]=count;default:return "",ErrInvalid}}
		out.Columns[i]=column{ID:fmt.Sprintf("column_%03d",i+1),Category:category,Observed:c.Observed,Nulls:c.Nulls,Distinct:c.Distinct,Exact:c.DistinctExact,Families:families}
	}
	raw,err:=json.Marshal(out);if err!=nil||len(raw)>64<<10{return "",ErrLimit};return string(raw),nil
}
func(s *Service)summarize(ctx context.Context,e identity.Envelope,r ProfileRecord,p Profile)ProfileSummary{
	out:=ProfileSummary{Status:"unavailable",Receipt:emptyModelReceipt()}
	if r.Spec.SkipLLM{out.Status="skipped";return out}
	if !r.Settings.Summaries||s.gateway==nil{out.Status="disabled";return out}
	input,err:=SummaryContext(p);if err!=nil{out.Status="input_rejected";return out}
	call,err:=gateway.Authorize(e,"engineering.profile",r.SpecHash,access.Resource{Tenant:e.Tenant(),Kind:"source",Permission:"write",ID:r.Spec.Source},access.Resource{Tenant:e.Tenant(),Kind:"dataset",Permission:"query",ID:r.Spec.Dataset},access.Resource{Tenant:e.Tenant(),Kind:"execution_context",Permission:"use",ID:r.Spec.Context})
	if err!=nil{out.Status="authority_blocked";return out}
	// One durable summary admission per version plus one nonrefundable gateway
	// reservation: a timeout/lost response cannot become an invisible model retry.
	budget,err:=gateway.NewBudget(call,gateway.Limits{Calls:1,Tokens:128<<10,Duration:time.Duration(r.Settings.Timeout)});if err!=nil{return out}
	generated,err:=s.gateway.Generate(ctx,call,budget,"profile_summary","Summarize only the supplied aggregate profile evidence. Column labels are anonymous ordinals. Distinguish sample counts from population estimates. Physical scan cost is unknown. Do not infer business meaning, sensitive values, identities, or SQL changes. Return only the requested JSON object.",input,s.summarySchema)
	out.Receipt=generated.Receipt
	if err!=nil{return out}
	if s.summarySchema.Validate(generated.JSON,8192)!=nil{out.Status="invalid_output";return out}
	var value struct{Summary string `json:"summary"`};if json.Unmarshal(generated.JSON,&value)!=nil||len(value.Summary)>2048{out.Status="invalid_output";return out}
	out.Status="available";out.Text=value.Summary;return out
}
