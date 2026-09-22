// Package migration coordinates neutral, resumable domain migration without
// becoming an alternate owner for source, semantic, reporting, or identity data.
//
//nolint:revive // Public DTO names form the versioned migration wire contract.
package migration

import (
	"errors"
	"time"
)

const ManifestVersion = "chartworks-migration-v1"

var (
	ErrInvalid     = errors.New("migration: invalid manifest")
	ErrLimit       = errors.New("migration: limit exceeded")
	ErrConflict    = errors.New("migration: conflict")
	ErrNotFound    = errors.New("migration: not found")
	ErrUnsupported = errors.New("migration: unsupported record")
	ErrNotReady    = errors.New("migration: cohort is not ready")
)

type Kind string

const (
	KindSource      Kind = "source"
	KindUpload      Kind = "upload"
	KindProfile     Kind = "profile"
	KindTopic       Kind = "topic"
	KindRule        Kind = "rule"
	KindTemplate    Kind = "template"
	KindRuntimePack Kind = "runtime_pack"
	KindEvalSuite   Kind = "evaluation_suite"
	KindBlock       Kind = "block"
	KindReport      Kind = "report"
	KindDashboard   Kind = "dashboard"
	KindFilter      Kind = "filter"
	KindSchedule    Kind = "schedule"
	KindRun         Kind = "run"
	KindArtifact    Kind = "artifact"
	KindRendition   Kind = "rendition"
	KindCertificate Kind = "certificate"
	KindTombstone   Kind = "tombstone"
	KindCalibration Kind = "calibration"
)

type FieldDisposition struct {
	Path   string `json:"path"`
	Status string `json:"status" jsonschema:"enum=retained,enum=transformed,enum=dropped,enum=unsupported"`
	Reason string `json:"reason,omitempty"`
}

type Mapping struct {
	Kind        Kind   `json:"kind"`
	ExternalRef string `json:"external_ref"`
	Destination string `json:"destination"`
	Revision    int64  `json:"revision"`
}

type Retention struct {
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	EraseWith string     `json:"erase_with,omitempty"`
	LegalHold bool       `json:"legal_hold"`
}

type TombstoneTarget struct {
	Kind        Kind   `json:"kind"`
	ExternalRef string `json:"external_ref"`
	Revision    int64  `json:"revision"`
}

type Object struct {
	Kind           Kind             `json:"kind"`
	ExternalRef    string           `json:"external_ref"`
	Parents        []string         `json:"parents" wire:"optional"`
	Revision       int64            `json:"revision"`
	PayloadVersion string           `json:"payload_version"`
	Payload        string           `json:"payload"`
	Lifecycle      string           `json:"lifecycle" jsonschema:"enum=private_draft,enum=historical,enum=deleted"`
	Private        bool             `json:"private"`
	Origin         string           `json:"origin"`
	Retention      Retention        `json:"retention"`
	Deletes        *TombstoneTarget `json:"deletes,omitempty"`
}

type Calibration struct {
	Revision       string `json:"revision"`
	ModelVersion   string `json:"model_version"`
	EmbeddingSpace string `json:"embedding_space"`
	BudgetVersion  string `json:"budget_version"`
	Payload        string `json:"payload"`
	State          string `json:"state" jsonschema:"enum=review_candidate"`
}

type Evidence struct {
	Feature        string `json:"feature"`
	OwnerFeature   string `json:"owner_feature"`
	Disposition    string `json:"disposition" jsonschema:"enum=required,enum=excluded"`
	Outcome        string `json:"outcome" jsonschema:"enum=passed,enum=failed,enum=unsupported"`
	EvidenceType   string `json:"evidence_type" jsonschema:"enum=runtime,enum=live,enum=recorded_fixture,enum=operator"`
	Reference      string `json:"reference"`
	Source         string `json:"source"`
	SourceVersion  string `json:"source_version"`
	EvidenceHash   string `json:"evidence_hash"`
	ComparisonHash string `json:"comparison_hash,omitempty"`
	Engine         string `json:"engine,omitempty"`
	Dialect        string `json:"dialect,omitempty"`
	SourceSnapshot string `json:"source_snapshot,omitempty"`
	SourceRevision int64  `json:"source_revision,omitempty"`
}

type OccurrenceBoundary struct {
	Stream          string    `json:"stream"`
	LastAccepted    string    `json:"last_accepted,omitempty"`
	LastDue         time.Time `json:"last_due_at,omitempty"`
	ResumeAfter     time.Time `json:"resume_after"`
	ScheduleVersion int64     `json:"schedule_version"`
}

type Manifest struct {
	Version        string              `json:"version"`
	Batch          string              `json:"batch"`
	Cohort         string              `json:"cohort"`
	SourceSnapshot string              `json:"source_snapshot"`
	Engine         string              `json:"engine"`
	Dialect        string              `json:"dialect"`
	Mappings       []Mapping           `json:"mappings"`
	Objects        []Object            `json:"objects"`
	Fields         []FieldDisposition  `json:"fields"`
	Evidence       []Evidence          `json:"evidence"`
	Calibration    *Calibration        `json:"calibration,omitempty"`
	Boundary       *OccurrenceBoundary `json:"occurrence_boundary,omitempty"`
}

type DryRunRequest struct {
	Manifest Manifest `json:"manifest"`
}
type ImportRequest struct {
	Manifest Manifest `json:"manifest"`
	Expected int64    `json:"expected_revision"`
}
type ResumeRequest struct {
	Batch    string `json:"batch"`
	Expected int64  `json:"expected_revision"`
}
type ExportRequest struct {
	Batch string `json:"batch"`
	After string `json:"after,omitempty"`
	Limit int    `json:"limit"`
}
type CutoverRequest struct {
	Batch         string `json:"batch"`
	Expected      int64  `json:"expected_generation"`
	Route         string `json:"route"`
	PreviousRoute string `json:"previous_route,omitempty"`
	OperatorRef   string `json:"operator_reference"`
}
type RollbackRequest struct {
	Cohort      string   `json:"cohort"`
	Expected    int64    `json:"expected_generation"`
	OperatorRef string   `json:"operator_reference"`
	Effects     []string `json:"irreversible_effects" wire:"optional"`
}
type EraseRequest struct {
	Batch string `json:"batch"`
	Limit int    `json:"limit"`
}

type ObjectPlan struct {
	ExternalRef string           `json:"external_ref"`
	Kind        Kind             `json:"kind"`
	Action      string           `json:"action" jsonschema:"enum=install_private,enum=historical_quarantine,enum=retention_quarantine,enum=tombstone,enum=unsupported_quarantine"`
	DependsOn   []string         `json:"depends_on" wire:"optional"`
	Destination string           `json:"destination,omitempty"`
	Reason      string           `json:"reason,omitempty"`
	Revision    int64            `json:"source_revision"`
	Digest      string           `json:"object_digest"`
	Deletes     *TombstoneTarget `json:"deletes,omitempty"`
}

type Plan struct {
	Batch       string             `json:"batch"`
	Cohort      string             `json:"cohort"`
	Digest      string             `json:"digest"`
	Ready       bool               `json:"ready"`
	Objects     []ObjectPlan       `json:"objects"`
	Fields      []FieldDisposition `json:"fields"`
	Evidence    []Evidence         `json:"evidence"`
	Limitations []string           `json:"limitations" wire:"optional"`
}

type Batch struct {
	ID          string    `json:"id"`
	Cohort      string    `json:"cohort"`
	Digest      string    `json:"digest"`
	State       string    `json:"state"`
	Revision    int64     `json:"revision"`
	Applied     int       `json:"applied"`
	Quarantined int       `json:"quarantined"`
	Total       int       `json:"total"`
	Next        string    `json:"next,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Export struct {
	Manifest Manifest `json:"manifest"`
	Batch    Batch    `json:"batch"`
}

type Cutover struct {
	Cohort              string             `json:"cohort"`
	Batch               string             `json:"batch"`
	Route               string             `json:"route"`
	PreviousRoute       string             `json:"previous_route,omitempty"`
	State               string             `json:"state"`
	Generation          int64              `json:"generation"`
	Boundary            OccurrenceBoundary `json:"occurrence_boundary"`
	IrreversibleEffects []string           `json:"irreversible_effects" wire:"optional"`
	OperatorReference   string             `json:"operator_reference"`
	UpdatedAt           time.Time          `json:"updated_at"`
}

type EraseResult struct {
	Batch       string `json:"batch"`
	Erased      int64  `json:"erased"`
	Remaining   int64  `json:"remaining"`
	BackupScope string `json:"backup_scope"`
}
