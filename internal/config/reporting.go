package config

import (
	"math"
	"time"
)

// Reporting bounds definition authoring and explicitly requested validation.
// These settings never grant identity, source or publication authority.
type Reporting struct {
	Viewer             ReportingViewer      `json:"viewer"`
	Execution          ReportingExecution   `json:"execution"`
	Composition        ReportingComposition `json:"composition"`
	MaxSQLBytes        int                  `json:"max_sql_bytes"`
	MaxDefinitionBytes int                  `json:"max_definition_bytes"`
	MaxSchemaColumns   int                  `json:"max_schema_columns"`
	MaxOutputs         int                  `json:"max_outputs"`
	MaxParameters      int                  `json:"max_parameters"`
	MaxLocales         int                  `json:"max_locales"`
	MaxAliases         int                  `json:"max_aliases"`
	MaxRevisions       int                  `json:"max_revisions"`
	MaxBlocks          int                  `json:"max_blocks"`
	MaxConcurrent      int                  `json:"max_concurrent"`
	PreviewRows        int                  `json:"preview_rows"`
	PreviewBytes       int                  `json:"preview_bytes"`
	ValidationTimeout  Duration             `json:"validation_timeout"`
	EvidenceTTL        Duration             `json:"evidence_ttl"`
	QuestionThreshold  float64              `json:"question_threshold"`
}

// DefaultReporting requires neither a model provider nor a live warehouse.
func DefaultReporting() Reporting {
	return Reporting{Viewer: DefaultReportingViewer(), Execution: DefaultReportingExecution(), Composition: DefaultReportingComposition(), MaxSQLBytes: 64 << 10, MaxDefinitionBytes: 512 << 10, MaxSchemaColumns: 128, MaxOutputs: 32, MaxParameters: 32, MaxLocales: 16, MaxAliases: 32, MaxRevisions: 128, MaxBlocks: 10000, MaxConcurrent: 4, PreviewRows: 1000, PreviewBytes: 1 << 20, ValidationTimeout: Duration(30 * time.Second), EvidenceTTL: Duration(24 * time.Hour), QuestionThreshold: 0.8}
}

// Validate is also used by the domain constructor, so in-process callers cannot
// circumvent the bounds enforced by configuration decoding.
func (c Reporting) Validate() error {
	if err := c.Viewer.Validate(); err != nil {
		return err
	}
	if err := c.Execution.Validate(); err != nil {
		return err
	}
	if err := c.Composition.Validate(); err != nil {
		return err
	}
	if c.MaxSQLBytes < 128 || c.MaxSQLBytes > 64<<10 || c.MaxDefinitionBytes < c.MaxSQLBytes || c.MaxDefinitionBytes > 1<<20 || c.MaxSchemaColumns < 1 || c.MaxSchemaColumns > 256 || c.MaxOutputs < 1 || c.MaxOutputs > 64 || c.MaxParameters < 1 || c.MaxParameters > 64 || c.MaxLocales < 1 || c.MaxLocales > 32 || c.MaxAliases < 1 || c.MaxAliases > 64 || c.MaxRevisions < 2 || c.MaxRevisions > 256 || c.MaxBlocks < 1 || c.MaxBlocks > 100000 || c.MaxConcurrent < 1 || c.MaxConcurrent > 32 || c.PreviewRows < 1 || c.PreviewRows > 10000 || c.PreviewBytes < 1024 || c.PreviewBytes > 4<<20 || time.Duration(c.ValidationTimeout) < time.Second || time.Duration(c.ValidationTimeout) > time.Minute || time.Duration(c.EvidenceTTL) < time.Minute || time.Duration(c.EvidenceTTL) > 7*24*time.Hour || math.IsNaN(c.QuestionThreshold) || math.IsInf(c.QuestionThreshold, 0) || c.QuestionThreshold < 0.5 || c.QuestionThreshold > 1 {
		return invalid("reporting", "bounded authoring, validation and evidence limits required")
	}
	return nil
}

func (c Reporting) validate() error { return c.Validate() }
