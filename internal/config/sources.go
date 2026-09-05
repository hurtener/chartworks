package config

import (
	"strings"
	"time"
)

// SourceRelation is a technical table/column contract, not a local user grant.
type SourceRelation struct {
	Schema string `json:"schema"`
	Name string `json:"name"`
	Columns []string `json:"columns"`
}

// SourceConnection is operator-controlled and tenant-bound. Public source creation can
// choose only an alias here, never arbitrary environment variables or network locations.
type SourceConnection struct {
	Tenant string `json:"tenant"`
	ID string `json:"id"`
	Version string `json:"version"`
	ReadDSN string `json:"read_dsn"`
	WriteDSN string `json:"write_dsn,omitempty"`
	Relations []SourceRelation `json:"relations"`
}

// Sources configures only the implemented PostgreSQL connector and its bounds.
type Sources struct {
	Enabled bool `json:"enabled"`
	Connections []SourceConnection `json:"connections"`
	MaxConns int32 `json:"max_conns"`
	ConnectTimeout Duration `json:"connect_timeout"`
	QueryTimeout Duration `json:"query_timeout"`
	MaxRows int `json:"max_rows"`
	MaxBytes int `json:"max_bytes"`
}

// ReadValidation bounds native SQL parsing, independently from connection and execution limits.
type ReadValidation struct {
	MaxSQLBytes int `json:"max_sql_bytes"`
	MaxParameters int `json:"max_parameters"`
	MaxASTDepth int `json:"max_ast_depth"`
	MaxASTNodes int `json:"max_ast_nodes"`
	Concurrency int `json:"concurrency"`
}

// DefaultSources leaves warehouse access opt-in and keeps metadata reads available.
func DefaultSources() Sources { return Sources{MaxConns:4,ConnectTimeout:Duration(time.Second),QueryTimeout:Duration(2*time.Second),MaxRows:256,MaxBytes:1<<20} }
// DefaultReadValidation supplies explicit conservative parser limits.
func DefaultReadValidation() ReadValidation { return ReadValidation{MaxSQLBytes:32768,MaxParameters:64,MaxASTDepth:64,MaxASTNodes:8192,Concurrency:2} }

// Clone detaches the complete operator connector snapshot.
func (s Sources) Clone() Sources {
	s.Connections=append([]SourceConnection(nil),s.Connections...)
	for i:=range s.Connections {
		s.Connections[i].Relations=append([]SourceRelation(nil),s.Connections[i].Relations...)
		for j:=range s.Connections[i].Relations { s.Connections[i].Relations[j].Columns=append([]string(nil),s.Connections[i].Relations[j].Columns...) }
	}
	return s
}
func sourceCoordinate(s string) bool {
	if len(s)<1 || len(s)>100 { return false }
	for _,c:=range s { if c>='a'&&c<='z' || c>='A'&&c<='Z' || c>='0'&&c<='9' || strings.ContainsRune("_.:-",c) { continue }; return false }
	return true
}
func sourceSQLName(s string) bool {
	if len(s)<1 || len(s)>63 { return false }
	for i,c:=range s { if c>='a'&&c<='z' || c=='_' || i>0&&c>='0'&&c<='9' { continue }; return false }; return true
}

// ValidateSources rejects inline credentials, ambiguous aliases and unbounded settings.
func ValidateSources(s Sources) error {
	if s.MaxConns<1 || s.MaxConns>16 || s.ConnectTimeout<Duration(time.Millisecond) || s.ConnectTimeout>Duration(4*time.Second) || s.QueryTimeout<Duration(time.Millisecond) || s.QueryTimeout>Duration(4*time.Second) || s.MaxRows<1 || s.MaxRows>1000 || s.MaxBytes<1024 || s.MaxBytes>4<<20 || len(s.Connections)>32 { return invalid("sources","bounds exceeded") }
	if s.Enabled && len(s.Connections)==0 { return invalid("sources.connections","enabled source access requires approved aliases") }
	seen:=map[string]bool{}
	for _,c:=range s.Connections {
		key:=c.Tenant+"/"+c.ID
		if !sourceCoordinate(c.Tenant) || !sourceCoordinate(c.ID) || !sourceCoordinate(c.Version) || seen[key] || len(c.Relations)<1 || len(c.Relations)>32 { return invalid("sources.connections","bounded unique tenant aliases required") }
		seen[key]=true
		if _,err:=reference(c.ReadDSN); err!=nil { return invalid("sources.connections.read_dsn","environment reference required") }
		if c.WriteDSN!="" { if _,err:=reference(c.WriteDSN); err!=nil || c.ReadDSN==c.WriteDSN { return invalid("sources.connections.write_dsn","independent environment reference required") } }
		relations:=map[string]bool{}
		for _,r:=range c.Relations {
			key:=r.Schema+"."+r.Name
			if !sourceSQLName(r.Schema) || strings.HasPrefix(r.Schema,"pg_") || r.Schema=="information_schema" || !sourceSQLName(r.Name) || relations[key] || len(r.Columns)<1 || len(r.Columns)>256 { return invalid("sources.connections.relations","explicit non-system tables and columns required") }
			relations[key]=true
			columns:=map[string]bool{}
			for _,name:=range r.Columns { if !sourceSQLName(name) || columns[name] { return invalid("sources.connections.relations.columns","unique supported column names required") }; columns[name]=true }
		}
	}
	return nil
}

// ValidateReadValidation checks parsing admission separately from SQL authorization.
func ValidateReadValidation(v ReadValidation) error {
	if v.MaxSQLBytes<128 || v.MaxSQLBytes>65536 || v.MaxParameters<1 || v.MaxParameters>64 || v.MaxASTDepth<4 || v.MaxASTDepth>64 || v.MaxASTNodes<32 || v.MaxASTNodes>16384 || v.Concurrency<1 || v.Concurrency>8 { return invalid("exec","parser bounds exceeded") }; return nil
}
