package config

import (
	"strings"
	"time"
)

// SourceRelation is a technical table/column contract, not a local user grant.
type SourceRelation struct {
	Schema  string   `json:"schema"`
	Name    string   `json:"name"`
	Columns []string `json:"columns"`
}

// SourceConnection is operator-controlled and tenant-bound. Public source creation can
// choose only an alias here, never arbitrary environment variables or network locations.
type SourceConnection struct {
	Dialect       string           `json:"dialect,omitempty"`
	ManagedSchema string           `json:"managed_schema,omitempty"`
	Tenant        string           `json:"tenant"`
	ID            string           `json:"id"`
	Version       string           `json:"version"`
	ReadDSN       string           `json:"read_dsn"`
	WriteDSN      string           `json:"write_dsn,omitempty"`
	Relations     []SourceRelation `json:"relations"`
}

// Sources configures only the implemented PostgreSQL connector and its bounds.
type Sources struct {
	Enabled        bool               `json:"enabled"`
	Connections    []SourceConnection `json:"connections"`
	MaxConns       int32              `json:"max_conns"`
	ConnectTimeout Duration           `json:"connect_timeout"`
	QueryTimeout   Duration           `json:"query_timeout"`
	MaxRows        int                `json:"max_rows"`
	MaxBytes       int                `json:"max_bytes"`
}

// ReadValidation bounds native SQL parsing, independently from connection and execution limits.
type ReadValidation struct {
	RowsDefault          int      `json:"rows_default"`
	RowsCeiling          int      `json:"rows_ceiling"`
	PreviewRows          int      `json:"preview_rows"`
	BytesDefault         int      `json:"bytes_default"`
	BytesCeiling         int      `json:"bytes_ceiling"`
	Timeout              Duration `json:"timeout"`
	CancelGrace          Duration `json:"cancel_grace"`
	PlannerCostCeiling   float64  `json:"planner_cost_ceiling"`
	ExecutionConcurrency int      `json:"execution_concurrency"`
	MaxReadAttempts      int      `json:"max_read_attempts"`
	MaxSQLBytes          int      `json:"max_sql_bytes"`
	MaxParameters        int      `json:"max_parameters"`
	MaxASTDepth          int      `json:"max_ast_depth"`
	MaxASTNodes          int      `json:"max_ast_nodes"`
	Concurrency          int      `json:"concurrency"`
}

// DefaultSources leaves warehouse access opt-in and keeps metadata reads available.
func DefaultSources() Sources {
	return Sources{Connections: []SourceConnection{}, MaxConns: 4, ConnectTimeout: Duration(time.Second), QueryTimeout: Duration(2 * time.Second), MaxRows: 256, MaxBytes: 1 << 20}
}

// DefaultReadValidation supplies explicit conservative parser limits.
func DefaultReadValidation() ReadValidation {
	return ReadValidation{RowsDefault: 10000, RowsCeiling: 100000, PreviewRows: 200, BytesDefault: 4 << 20, BytesCeiling: 16 << 20, Timeout: Duration(time.Minute), CancelGrace: Duration(2 * time.Second), PlannerCostCeiling: 1e7, ExecutionConcurrency: 2, MaxReadAttempts: 3, MaxSQLBytes: 32768, MaxParameters: 64, MaxASTDepth: 64, MaxASTNodes: 8192, Concurrency: 2}
}

// Clone detaches the complete operator connector snapshot.
func (s Sources) Clone() Sources {
	s.Connections = append([]SourceConnection{}, s.Connections...)
	for i := range s.Connections {
		s.Connections[i].Relations = append([]SourceRelation{}, s.Connections[i].Relations...)
		for j := range s.Connections[i].Relations {
			s.Connections[i].Relations[j].Columns = append([]string{}, s.Connections[i].Relations[j].Columns...)
		}
	}
	return s
}
func sourceCoordinate(s string) bool {
	if len(s) < 1 || len(s) > 100 {
		return false
	}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("_.:-", c) {
			continue
		}
		return false
	}
	return true
}
func sourceSQLName(s string) bool {
	if len(s) < 1 || len(s) > 63 {
		return false
	}
	for i, c := range s {
		if c >= 'a' && c <= 'z' || c == '_' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// ValidateSources rejects inline credentials, ambiguous aliases and unbounded settings.
func ValidateSources(s Sources) error {
	if s.MaxConns < 1 || s.MaxConns > 16 || s.ConnectTimeout < Duration(time.Millisecond) || s.ConnectTimeout > Duration(4*time.Second) || s.QueryTimeout < Duration(time.Millisecond) || s.QueryTimeout > Duration(4*time.Second) || s.MaxRows < 1 || s.MaxRows > 1000 || s.MaxBytes < 1024 || s.MaxBytes > 4<<20 || len(s.Connections) > 32 {
		return invalid("sources", "bounds exceeded")
	}
	if s.Enabled && len(s.Connections) == 0 {
		return invalid("sources.connections", "enabled source access requires approved aliases")
	}
	seen := map[string]bool{}
	for _, c := range s.Connections {
		key := c.Tenant + "/" + c.ID
		dialect := c.Dialect
		if dialect == "" {
			dialect = "postgres"
		}
		if !sourceDialect(dialect) || !sourceCoordinate(c.Tenant) || !sourceCoordinate(c.ID) || !sourceCoordinate(c.Version) || seen[key] || c.ManagedSchema == "" && len(c.Relations) < 1 || len(c.Relations) > 32 {
			return invalid("sources.connections", "bounded unique tenant aliases required")
		}
		if c.ManagedSchema != "" && (dialect != "postgres" || !sourceSQLName(c.ManagedSchema) || !strings.HasPrefix(c.ManagedSchema, "cw_") || len(c.ManagedSchema) > 30 || c.WriteDSN == "" || len(c.Relations) != 0) {
			return invalid("sources.connections.managed_schema", "explicit isolated workspace required")
		}
		seen[key] = true
		if _, err := reference(c.ReadDSN); err != nil {
			return invalid("sources.connections.read_dsn", "environment reference required")
		}
		if c.WriteDSN != "" {
			if _, err := reference(c.WriteDSN); err != nil || c.ReadDSN == c.WriteDSN {
				return invalid("sources.connections.write_dsn", "independent environment reference required")
			}
		}
		relations := map[string]bool{}
		for _, r := range c.Relations {
			key := r.Schema + "." + r.Name
			if !sourceSQLNameFor(dialect, r.Schema) || strings.HasPrefix(strings.ToLower(r.Schema), "pg_") || strings.EqualFold(r.Schema, "information_schema") || !sourceSQLNameFor(dialect, r.Name) || relations[key] || len(r.Columns) < 1 || len(r.Columns) > 256 {
				return invalid("sources.connections.relations", "explicit non-system tables and columns required")
			}
			relations[key] = true
			columns := map[string]bool{}
			for _, name := range r.Columns {
				if !sourceSQLNameFor(dialect, name) || columns[name] {
					return invalid("sources.connections.relations.columns", "unique supported column names required")
				}
				columns[name] = true
			}
		}
	}
	return nil
}

func sourceSQLNameFor(dialect, value string) bool {
	if dialect == "postgres" || dialect == "mysql" {
		return sourceSQLName(value)
	}
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for i, c := range value {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func sourceDialect(dialect string) bool {
	switch dialect {
	case "postgres", "mysql", "sqlserver", "bigquery", "snowflake", "databricks":
		return true
	}
	return false
}

// ValidateReadValidation checks parsing admission separately from SQL authorization.
func ValidateReadValidation(v ReadValidation) error {
	if v.RowsDefault < 1 || v.RowsDefault > v.RowsCeiling || v.RowsCeiling > 100000 || v.PreviewRows < 1 || v.PreviewRows > v.RowsDefault || v.BytesDefault < 1024 || v.BytesDefault > v.BytesCeiling || v.BytesCeiling > 16<<20 || v.Timeout < Duration(time.Millisecond) || v.Timeout > Duration(time.Minute) || v.CancelGrace < Duration(time.Millisecond) || v.CancelGrace > Duration(3*time.Second) || !(v.PlannerCostCeiling > 0 && v.PlannerCostCeiling <= 1e12) || v.ExecutionConcurrency < 1 || v.ExecutionConcurrency > 16 || v.MaxReadAttempts < 1 || v.MaxReadAttempts > 3 {
		return invalid("exec", "execution bounds exceeded")
	}
	if v.MaxSQLBytes < 128 || v.MaxSQLBytes > 65536 || v.MaxParameters < 1 || v.MaxParameters > 64 || v.MaxASTDepth < 4 || v.MaxASTDepth > 64 || v.MaxASTNodes < 32 || v.MaxASTNodes > 16384 || v.Concurrency < 1 || v.Concurrency > 8 {
		return invalid("exec", "parser bounds exceeded")
	}
	return nil
}
