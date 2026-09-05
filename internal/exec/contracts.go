// Package exec owns validation and the opaque read contract. Concrete source adapters
// depend on this package; it never imports a concrete source driver.
package exec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

var (
	// ErrUnsafe means the complete SQL safety contract could not be proven.
	ErrUnsafe = errors.New("exec: SQL safety proof insufficient")
	// ErrUnsupported is truthful about an unqualified dialect or adapter feature.
	ErrUnsupported = errors.New("exec: capability unsupported")
	// ErrBinding rejects a zero plan or changed source, context, authority or parameters.
	ErrBinding = errors.New("exec: validated binding required")
	// ErrLimit rejects resource bounds before returning partial evidence.
	ErrLimit = errors.New("exec: bounded work limit exceeded")
)

// Column is a discovered, versioned database column, not a caller assertion.
type Column struct {
	Name       string `json:"name"`
	NativeType string `json:"native_type"`
	Category   string `json:"category"`
	Nullable   bool   `json:"nullable"`
	Safe       bool   `json:"safe_for_validation"`
}

// Relation is a registered technical data contract. Pengui still supplies query reach.
type Relation struct {
	ID      string   `json:"id"`
	Schema  string   `json:"schema"`
	Name    string   `json:"name"`
	Columns []Column `json:"columns"`
}

// Binding is resolved from registered connector configuration and actual database
// role/schema state. A client context label cannot create one at a public endpoint.
type Binding struct {
	Tenant      string     `json:"tenant"`
	Source      string     `json:"source"`
	Context     string     `json:"context"`
	Revision    int64      `json:"revision"`
	Dialect     string     `json:"dialect"`
	Contract    string     `json:"contract"`
	Fingerprint string     `json:"fingerprint"`
	Relations   []Relation `json:"relations"`
}

// Valid checks bounded resolved coordinates. Database evidence is supplied by the adapter.
func (b Binding) Valid() bool {
	if !identity.Identifier(b.Tenant) || !identity.Identifier(b.Source) || !identity.Identifier(b.Context) || !identity.Identifier(b.Contract) || b.Revision<1 || len(b.Fingerprint)!=64 || len(b.Relations)<1 || len(b.Relations)>32 { return false }
	if _,err:=hex.DecodeString(b.Fingerprint); err!=nil { return false }
	seen:=map[string]bool{}
	for _,r:=range b.Relations {
		if !identity.Identifier(r.ID) || !SQLIdentifier(r.Schema) || !SQLIdentifier(r.Name) || seen[r.Schema+"."+r.Name] || len(r.Columns)<1 || len(r.Columns)>256 { return false }
		seen[r.Schema+"."+r.Name]=true
		columns:=map[string]bool{}
		for _,c:=range r.Columns { if !SQLIdentifier(c.Name) || columns[c.Name] || len(c.NativeType)<1 || len(c.NativeType)>128 { return false }; columns[c.Name]=true }
	}
	return true
}

// SQLIdentifier is the declared initial connector-name support, not an SQL safety parser.
func SQLIdentifier(s string) bool {
	if len(s)<1 || len(s)>63 { return false }
	for i,c:=range s { if c>='a' && c<='z' || c=='_' || i>0 && c>='0' && c<='9' { continue }; return false }
	return true
}

// Clone detaches discovered relation and column state for each request.
func (b Binding) Clone() Binding {
	b.Relations=append([]Relation(nil),b.Relations...)
	for i:=range b.Relations { b.Relations[i].Columns=append([]Column(nil),b.Relations[i].Columns...) }
	return b
}

// Hash is a canonical structural identity used only after validation.
func Hash(v any) string { b,_:=json.Marshal(v); h:=sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func authority(e identity.Envelope) string {
	scopes:=e.Scopes(); sort.Strings(scopes)
	return Hash([]any{e.Tenant(),e.User(),e.Session(),scopes})
}

// Parameter is a bounded typed value. SQL and parameters are bound together forever.
type Parameter struct { Kind string `json:"kind"`; Value string `json:"value"` }

// Valid rejects nonfinite, ambiguous and excessively large parameter values.
func (p Parameter) Valid() bool {
	if len(p.Value)>4096 || strings.ContainsRune(p.Value,0) { return false }
	switch p.Kind {
	case "null": return p.Value==""
	case "text": return true
	case "boolean": return p.Value=="true" || p.Value=="false"
	case "integer": _,err:=strconv.ParseInt(p.Value,10,64); return err==nil && len(p.Value)<=20
	case "number":
		f,err:=strconv.ParseFloat(p.Value,64)
		return len(p.Value)<=128 && json.Valid([]byte(p.Value)) && err==nil && !math.IsNaN(f) && !math.IsInf(f,0)
	}
	return false
}

// ReadAdapter supplies actual binding and safe native planning; it exposes no raw query method.
type ReadAdapter interface {
	Binding(context.Context,identity.Envelope,string,string) (Binding,error)
	Explain(context.Context,identity.Envelope,Candidate) error
}

// Candidate can only be constructed by the validator after whole-tree safety checks.
// It is not executable: only native dry planning may consume this type.
type Candidate struct {
	binding Binding
	owner identity.Envelope
	authority string
	statement string
	parameters []Parameter
	dependencies []string
	columns []string
	checked bool
}

// Coordinates reveals only addressed metadata for an adapter's current-state lookup.
func (c Candidate) Coordinates() (string,string) { return c.binding.Source,c.binding.Context }

// SQL permits native EXPLAIN only after current binding and signed reach are rechecked.
func (c Candidate) SQL(e identity.Envelope,current Binding) (string,[]Parameter,error) {
	if !c.checked || !c.owner.Valid() || !current.Valid() || Hash(c.binding)!=Hash(current) || c.authority!=authority(e) { return "",nil,ErrBinding }
	if err:=Require(e,current,c.dependencies); err!=nil { return "",nil,err }
	return c.statement,append([]Parameter(nil),c.parameters...),nil
}

// Plan is intentionally opaque and has no public constructor or deserializer.
type Plan struct { candidate Candidate; nativeChecked bool }

// Coordinates permits lookup without exposing raw SQL or authority internals.
func (p Plan) Coordinates() (string,string) { return p.candidate.Coordinates() }

// SQL is the final adapter gate. A new token cannot widen or substitute the accepted plan.
func (p Plan) SQL(e identity.Envelope,current Binding) (string,[]Parameter,error) {
	if !p.nativeChecked { return "",nil,ErrBinding }
	return p.candidate.SQL(e,current)
}

// Receipt contains only non-secret validation evidence, never an executable token.
type Receipt struct {
	Validated bool `json:"validated"`
	Source string `json:"source"`
	Context string `json:"context"`
	Contract string `json:"contract"`
	Dependencies []string `json:"dependencies"`
	Columns []string `json:"columns"`
	Manifest string `json:"manifest"`
}

// Receipt returns a detached result without disclosing SQL or parameter values.
func (p Plan) Receipt() Receipt {
	if !p.nativeChecked || !p.candidate.owner.Valid() { return Receipt{} }
	c:=p.candidate
	return Receipt{Validated:true,Source:c.binding.Source,Context:c.binding.Context,Contract:c.binding.Contract,Dependencies:append([]string(nil),c.dependencies...),Columns:append([]string(nil),c.columns...),Manifest:Hash([]any{c.binding,c.statement,c.parameters,c.dependencies,c.authority})}
}

// String prevents accidental SQL disclosure through ordinary logging.
func (p Plan) String() string { return "validated-read-plan(redacted)" }
// GoString prevents detailed logging from disclosing the plan.
func (p Plan) GoString() string { return p.String() }
// MarshalJSON emits metadata only and cannot be used to reconstruct a plan.
func (p Plan) MarshalJSON() ([]byte,error) { return json.Marshal(p.Receipt()) }
// String prevents accidental candidate SQL disclosure.
func (c Candidate) String() string { return "read-candidate(redacted)" }
// GoString redacts detailed candidate formatting.
func (c Candidate) GoString() string { return c.String() }
// MarshalJSON does not serialize the candidate or its owner.
func (c Candidate) MarshalJSON() ([]byte,error) { return []byte(`{"candidate":"redacted"}`),nil }

// Require applies every resolved source, context and dataset dependency before I/O.
func Require(e identity.Envelope,b Binding,dependencies []string) error {
	refs:=[]access.Resource{{Tenant:b.Tenant,Kind:"source",Permission:"query",ID:b.Source},{Tenant:b.Tenant,Kind:"execution_context",Permission:"use",ID:b.Context}}
	for _,id:=range dependencies { refs=append(refs,access.Resource{Tenant:b.Tenant,Kind:"dataset",Permission:"query",ID:id}) }
	return access.Require(e,"sources.query",refs...)
}
