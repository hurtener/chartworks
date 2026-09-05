package chartworks

import (
	"context"
	"errors"
	"time"
)

// Source is a secret-free current source registration, not a live health assertion.
type Source struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Dialect string `json:"dialect"`
	Revision int64 `json:"revision"`
	ContextID string `json:"context_id"`
	Status string `json:"status"`
}

// SourceRequest selects a tenant-approved operator connection alias, never a DSN or secret key.
type SourceRequest struct {
	ID string `json:"id"`
	Name string `json:"name"`
	Connection string `json:"connection"`
}

// SourceStatus is a receipt from a real bounded warehouse catalog probe.
type SourceStatus struct {
	Available bool `json:"available"`
	Revision int64 `json:"revision"`
	ContextID string `json:"context_id"`
	ObservedAt time.Time `json:"observed_at"`
}

// SourceColumn preserves native type metadata and explicit validation qualification.
type SourceColumn struct {
	Name string `json:"name"`
	NativeType string `json:"native_type"`
	Category string `json:"category"`
	Nullable bool `json:"nullable"`
	Safe bool `json:"safe_for_validation"`
}

// SourceRelation includes the exact dataset scope ID needed for query authorization.
type SourceRelation struct {
	ID string `json:"id"`
	Schema string `json:"schema"`
	Name string `json:"name"`
	Columns []SourceColumn `json:"columns"`
}

// SourceSchema is current, configured discovery; it does not expose source credentials.
type SourceSchema struct {
	SourceID string `json:"source_id"`
	ContextID string `json:"context_id"`
	Revision int64 `json:"revision"`
	Relations []SourceRelation `json:"relations"`
}

// ReadParameter is a bounded typed string representation; NULL is kind=null,value="".
type ReadParameter struct { Kind string `json:"kind"`; Value string `json:"value"` }

// ReadValidationRequest requests proof, not raw execution or an authorization override.
type ReadValidationRequest struct {
	Context string `json:"context"`
	SQL string `json:"sql"`
	Parameters []ReadParameter `json:"parameters"`
}

// ReadValidationReceipt is non-executable metadata. It is not a bearer or serialized Plan.
type ReadValidationReceipt struct {
	Validated bool `json:"validated"`
	Source string `json:"source"`
	Context string `json:"context"`
	Contract string `json:"contract"`
	Dependencies []string `json:"dependencies"`
	Columns []string `json:"columns"`
	Manifest string `json:"manifest"`
}

// CreateSource registers actual context metadata for an approved tenant connection.
func (c *Client) CreateSource(ctx context.Context,input SourceRequest) (out Source,err error) {
	if !wireID(input.ID) || !wireID(input.Connection) { return out,errors.New("chartworks: invalid source identifier") }
	err=c.call(ctx,"POST","/v1/sources","",input,&out); return out,err
}

// Sources lists only server-authorized registrations and never tests warehouse connections.
func (c *Client) Sources(ctx context.Context) (out []Source,err error) { err=c.call(ctx,"GET","/v1/sources","",nil,&out); return out,err }

// Source reads one secret-free retained registration.
func (c *Client) Source(ctx context.Context,id string) (out Source,err error) {
	if !wireID(id) { return out,errors.New("chartworks: invalid source identifier") }
	err=c.call(ctx,"GET","/v1/sources/"+id,"",nil,&out); return out,err
}

// TestSource performs a bounded read-only source health check.
func (c *Client) TestSource(ctx context.Context,id string) (out SourceStatus,err error) {
	if !wireID(id) { return out,errors.New("chartworks: invalid source identifier") }
	err=c.call(ctx,"POST","/v1/sources/"+id+"/test","",struct{}{},&out); return out,err
}

// DiscoverSource returns native categories and the current technical data contract.
func (c *Client) DiscoverSource(ctx context.Context,id string) (out SourceSchema,err error) {
	if !wireID(id) { return out,errors.New("chartworks: invalid source identifier") }
	err=c.call(ctx,"GET","/v1/sources/"+id+"/schema","",nil,&out); return out,err
}

// RotateSource installs a new context only when the expected revision still matches.
func (c *Client) RotateSource(ctx context.Context,id string,expected int64) (out Source,err error) {
	if !wireID(id) || expected<1 { return out,errors.New("chartworks: invalid source revision") }
	err=c.call(ctx,"POST","/v1/sources/"+id+"/rotate","",struct { Expected int64 `json:"expected_revision"` }{expected},&out); return out,err
}

// ValidateRead returns a proof receipt without executing result work or minting authority.
func (c *Client) ValidateRead(ctx context.Context,id string,input ReadValidationRequest) (out ReadValidationReceipt,err error) {
	if !wireID(id) || !wireID(input.Context) { return out,errors.New("chartworks: invalid source context") }
	if input.Parameters==nil { input.Parameters=[]ReadParameter{} }
	err=c.call(ctx,"POST","/v1/sources/"+id+"/validate","",input,&out); return out,err
}
