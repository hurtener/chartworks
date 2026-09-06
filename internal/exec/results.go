package exec

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ErrType rejects unqualified result types, malformed values and nonfinite numbers.
var ErrType = errors.New("exec: result type or value unsupported")

// Field describes one ordered output. Exact numbers are JSON strings, not floats.
// NativeType is an adapter-owned type name, never a caller-supplied coercion.
type Field struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Encoding string `json:"encoding"`
	NativeType string `json:"native_type"`
}

// Result contains one ordered, bounded result and its completeness evidence.
// Bytes counts the JSON encoding of Schema and Rows, including delimiters/escapes.
// Receipt metadata is separately bounded; no result values are persisted here.
type Result struct {
	Schema []Field `json:"schema"`
	Rows [][]json.RawMessage `json:"rows"`
	Outcome string `json:"outcome"`
	Truncation string `json:"truncation"`
	Bytes int `json:"bytes"`
	Cost Cost `json:"cost"`
}

// Cost distinguishes an optimizer estimate from actual scanned bytes or billing.
// A nil actual value means unknown, not zero. PostgreSQL cannot enforce scan bytes.
type Cost struct {
	PlannerUnits *float64 `json:"planner_units"`
	ScannedBytes *int64 `json:"scanned_bytes"`
}

// Decimal accepts bounded finite base-ten text without a floating-point conversion.
// Result scale and significant digits are preserved verbatim.
func Decimal(s string) bool {
	if len(s) == 0 || len(s) > 4096 { return false }
	i := 0
	if s[i]=='-' || s[i]=='+' { i++; if i==len(s) { return false } }
	start := i
	for i<len(s) && s[i]>='0' && s[i]<='9' { i++ }
	if i==start { return false }
	if i<len(s) && s[i]=='.' {
		i++; start=i
		for i<len(s) && s[i]>='0' && s[i]<='9' { i++ }
		if i==start { return false }
	}
	if i<len(s) && (s[i]=='e' || s[i]=='E') {
		i++; if i<len(s) && (s[i]=='+' || s[i]=='-') { i++ }; start=i
		for i<len(s) && s[i]>='0' && s[i]<='9' { i++ }
		if i==start || i-start>4 { return false }
	}
	return i==len(s)
}

// Normalize encodes a text-format native value according to a qualified field.
// NULL and booleans retain their JSON types; exact numbers and structured JSON
// retain text so generic JSON consumers cannot round their embedded numbers.
func Normalize(field Field, raw []byte) (json.RawMessage,error) {
	if raw==nil { return json.RawMessage("null"),nil }
	if len(raw)>16<<20 || !utf8.Valid(raw) { return nil,ErrType }
	s:=string(raw)
	switch field.Type {
	case "integer":
		if _,err:=strconv.ParseInt(s,10,64); err!=nil { return nil,ErrType }
	case "decimal":
		if !Decimal(s) { return nil,ErrType }
	case "number":
		bits:=64; if field.NativeType=="float4" { bits=32 }
		n,err:=strconv.ParseFloat(s,bits)
		if err!=nil || math.IsNaN(n) || math.IsInf(n,0) { return nil,ErrType }
		return json.RawMessage(strconv.FormatFloat(n,'g',-1,bits)),nil
	case "boolean":
		if s=="t" { return json.RawMessage("true"),nil }
		if s=="f" { return json.RawMessage("false"),nil }
		return nil,ErrType
	case "binary":
		if !strings.HasPrefix(s,`\x`) { return nil,ErrType }
		if _,err:=hex.DecodeString(s[2:]); err!=nil { return nil,ErrType }
		s=s[2:]
	case "structured":
		if !json.Valid(raw) { return nil,ErrType }
	case "temporal":
		if s=="infinity" || s=="-infinity" || s=="" { return nil,ErrType }
	case "text":
	default:
		return nil,ErrType
	}
	out,err:=json.Marshal(s)
	return out,err
}

// Collector applies serialized schema/row byte limits before a row is retained.
// Reaching a cap is not an error or a claim that a prefix is a full-source total.
type Collector struct { result Result; maxRows,maxBytes int }

// NewCollector validates the ordered schema and accounts for its actual JSON size.
func NewCollector(schema []Field,rows,bytes int) (*Collector,error) {
	if rows<1 || rows>100000 || bytes<128 || bytes>16<<20 || len(schema)<1 || len(schema)>256 { return nil,ErrLimit }
	for _,f:=range schema {
		if len(f.Name)==0 || len(f.Name)>1024 || !utf8.ValidString(f.Name) || len(f.NativeType)==0 || len(f.NativeType)>128 { return nil,ErrType }
		switch f.Type {
		case "integer","decimal","text","binary","temporal","structured":
			if f.Encoding!="string" { return nil,ErrType }
		case "boolean","number":
			if f.Encoding!=f.Type { return nil,ErrType }
		default: return nil,ErrType
		}
	}
	encoded,_:=json.Marshal(schema)
	if len(encoded)+2>bytes { return nil,ErrLimit }
	return &Collector{result:Result{Schema:append([]Field(nil),schema...),Rows:[][]json.RawMessage{},Outcome:"empty",Bytes:len(encoded)+2},maxRows:rows,maxBytes:bytes},nil
}

// Add preserves source ordering. false means this lookahead row proves truncation.
func (c *Collector) Add(raw [][]byte) (bool,error) {
	if len(raw)!=len(c.result.Schema) { return false,ErrType }
	if len(c.result.Rows)==c.maxRows { c.result.Outcome="truncated";c.result.Truncation="rows";return false,nil }
	row:=make([]json.RawMessage,len(raw))
	for i,value:=range raw {
		v,err:=Normalize(c.result.Schema[i],value);if err!=nil { return false,err };row[i]=v
	}
	encoded,_:=json.Marshal(row)
	extra:=len(encoded);if len(c.result.Rows)>0 { extra++ }
	if c.result.Bytes+extra>c.maxBytes { c.result.Outcome="truncated";c.result.Truncation="bytes";return false,nil }
	c.result.Rows=append(c.result.Rows,row);c.result.Bytes+=extra;c.result.Outcome="succeeded"
	return true,nil
}

// Result transfers the request-local collector output; collectors are never shared.
func (c *Collector) Result() Result { return c.result }
