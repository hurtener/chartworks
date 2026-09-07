package engineering

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

// ErrPipelineQuality reports a blocking quality check failure before activation.
var ErrPipelineQuality = errors.New("pipeline quality check failed")

// ErrPipelineUncertain requires observation before retrying an external effect.
var ErrPipelineUncertain = errors.New("pipeline external effect requires reconciliation")

// PipelineColumn is a declared output contract, never an arbitrary SQL fragment.
type PipelineColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primary_key"`
}

// PipelineCheck declares a bounded blocking quality assertion for one output.
type PipelineCheck struct {
	Kind    string `json:"kind"`
	Column  string `json:"column,omitempty"`
	Minimum int64  `json:"minimum,omitempty"`
	Maximum int64  `json:"maximum,omitempty"`
}

// PipelineStep references one governed source or private preceding outputs. SQL
// has no caller-selected executable asset type or arbitrary template context.
type PipelineStep struct {
	ID         string           `json:"id"`
	Source     string           `json:"source,omitempty"`
	Context    string           `json:"context,omitempty"`
	SQL        string           `json:"sql"`
	Inputs     []string         `json:"inputs"`
	DependsOn  []string         `json:"depends_on"`
	FromSteps  []string         `json:"from_steps,omitempty"`
	Strategy   string           `json:"strategy"`
	Columns    []PipelineColumn `json:"columns"`
	Key        string           `json:"key,omitempty"`
	TimeColumn string           `json:"time_column,omitempty"`
	Start      string           `json:"start,omitempty"`
	End        string           `json:"end,omitempty"`
	Checks     []PipelineCheck  `json:"checks"`
}

// PipelineDefinition declares the SQL graph and its managed destination.
type PipelineDefinition struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Connection string         `json:"connection"`
	Steps      []PipelineStep `json:"steps"`
}

// PipelineVersion is an immutable definition with its lifecycle evidence.
type PipelineVersion struct {
	Definition PipelineDefinition `json:"definition"`
	Version    int64              `json:"version"`
	State      string             `json:"state"`
	Digest     string             `json:"digest"`
	Created    time.Time          `json:"created_at"`
	Published  *time.Time         `json:"published_at,omitempty"`
}

// PipelineEffect exposes a stage outcome without private execution metadata.
type PipelineEffect struct {
	Step    string `json:"step"`
	State   string `json:"state"`
	Rows    int64  `json:"rows"`
	Source  string `json:"source,omitempty"`
	Context string `json:"context,omitempty"`
	Digest  string `json:"digest"`
	Code    string `json:"code,omitempty"`
}

// PipelineRun is the public receipt of one accepted pipeline operation.
type PipelineRun struct {
	Pipeline  string           `json:"pipeline"`
	Version   int64            `json:"version"`
	State     string           `json:"state"`
	Effects   []PipelineEffect `json:"effects"`
	Operation jobs.RequestTask `json:"operation"`
}

// PipelineRecord binds an immutable version to its tenant and audit attribution.
type PipelineRecord struct {
	Tenant, Actor, Session string
	PipelineVersion
}

// Require enforces signed reach to the pipeline within its recorded tenant.
func (p PipelineRecord) Require(e identity.Envelope, action, permission string) error {
	if p.Tenant != e.Tenant() {
		return store.ErrNotFound
	}
	return access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: p.Definition.ID})
}

// ValidatePipelineDefinition checks the closed graph before source/runner work.
// Native SQL validation remains mandatory against real bindings at execution.
func ValidatePipelineDefinition(d PipelineDefinition, l config.Pipelines) error {
	if !identity.Identifier(d.ID) || len(d.ID) > 48 || len(d.Name) < 1 || len(d.Name) > 128 || strings.ContainsAny(d.Name, "\x00\r\n") || !identity.Identifier(d.Connection) || len(d.Steps) < 1 || len(d.Steps) > l.MaxSteps {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, s := range d.Steps {
		if !readexec.SQLIdentifier(s.ID) || len(s.ID) > 24 || seen[s.ID] || len(s.SQL) < 1 || len(s.SQL) > l.MaxSQLBytes || strings.ContainsAny(s.SQL, "\x00") || strings.Contains(s.SQL, "{%") || strings.Contains(strings.ToLower(s.SQL), "@bruin") || len(s.Columns) < 1 || len(s.Columns) > 128 || len(s.Checks) > 32 {
			return ErrInvalid
		}
		seen[s.ID] = true
		if len(s.FromSteps) == 0 {
			if !identity.Identifier(s.Source) || !identity.Identifier(s.Context) || len(s.Inputs) < 1 {
				return ErrInvalid
			}
		} else if s.Source != "" || s.Context != "" {
			return ErrInvalid
		} else if err := validatePipelineDerivedSQL(s); err != nil {
			return err
		}
		template := s.SQL
		for _, input := range s.FromSteps {
			template = strings.ReplaceAll(template, "{{step."+input+"}}", "input")
		}
		if strings.Contains(template, "{{") || strings.Contains(template, "}}") {
			return ErrInvalid
		}
		columns := map[string]bool{}
		keys := 0
		for _, c := range s.Columns {
			if !readexec.SQLIdentifier(c.Name) || columns[c.Name] || s.Strategy == "scd2" && (c.Name == "_valid_from" || c.Name == "_valid_until" || c.Name == "_is_current") {
				return ErrInvalid
			}
			columns[c.Name] = true
			if c.PrimaryKey {
				keys++
			}
			switch c.Type {
			case "bigint", "numeric", "text", "boolean", "date", "timestamp", "timestamptz", "bytea":
			default:
				return ErrInvalid
			}
		}
		switch s.Strategy {
		case "replace", "append":
		case "merge", "scd2":
			if keys == 0 {
				return ErrInvalid
			}
		case "incremental":
			if !columns[s.Key] {
				return ErrInvalid
			}
		case "interval":
			if !columns[s.TimeColumn] {
				return ErrInvalid
			}
			a, e := time.Parse(time.RFC3339Nano, s.Start)
			b, e2 := time.Parse(time.RFC3339Nano, s.End)
			if e != nil || e2 != nil || !a.Before(b) {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
		for _, c := range s.Checks {
			switch c.Kind {
			case "row_count":
				if c.Minimum < 0 || c.Maximum < 0 || c.Maximum != 0 && c.Maximum < c.Minimum {
					return ErrInvalid
				}
			case "not_null", "unique":
				if !columns[c.Column] {
					return ErrInvalid
				}
			default:
				return ErrInvalid
			}
		}
	}
	edges := map[string][]string{}
	for _, s := range d.Steps {
		deps := map[string]bool{}
		for _, id := range s.DependsOn {
			if !seen[id] || id == s.ID || deps[id] {
				return ErrInvalid
			}
			deps[id] = true
		}
		for _, id := range s.FromSteps {
			if !deps[id] {
				return ErrInvalid
			}
		}
		edges[s.ID] = s.DependsOn
	}
	state := map[string]int{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		state[id] = 1
		for _, dep := range edges[id] {
			if !visit(dep) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for id := range seen {
		if !visit(id) {
			return ErrInvalid
		}
	}
	return nil
}

// PipelineRepository holds definitions and staged effect evidence with ordinary
// signed provenance. The request ledger remains the execution/fencing authority.
type PipelineRepository interface {
	ReservePipelineExecution(context.Context, identity.Envelope, PipelineRecord, jobs.RequestTask, string) (PipelineExecution, error)
	ReadPipelineExecution(context.Context, identity.Envelope, string) (PipelineExecution, error)
	MutatePipelineStage(context.Context, jobs.Invocation, PipelineRecord, PipelineStageState) (PipelineExecution, error)
	CompletePipelineExecution(context.Context, jobs.Invocation, PipelineRecord, []sources.Record) error
	ReadPipelineStage(context.Context, identity.Envelope, string, string) (sources.PipelineStage, error)
	jobs.RequestRepository
	DatabaseName() string
	SavePipeline(context.Context, identity.Envelope, PipelineDefinition, int64) (PipelineRecord, error)
	ReadPipeline(context.Context, identity.Envelope, string, int64, string, string) (PipelineRecord, error)
	PublishPipeline(context.Context, identity.Envelope, string, int64) (PipelineRecord, error)
}

// PipelineStageState records accepted and observed effects separately. Unknown
// physical outcomes are never represented by a successful publication pointer.
type PipelineStageState struct {
	Stage                                     sources.PipelineStage
	State                                     string
	Previous                                  *sources.PipelineStage
	Application                               string
	PriorApplications                         []string
	Fence                                     int64
	Rows                                      int64
	RenderedHash, ValidationHash, LineageHash string
	Input                                     readexec.Binding
	Dependencies                              []string
	Compensated                               bool
	Code                                      string
}

// PipelineExecution retains the accepted operation and private stage evidence.
type PipelineExecution struct {
	Pipeline  string
	Version   int64
	Digest    string
	Operation jobs.RequestTask
	State     string
	Stages    []PipelineStageState
}

// Public returns a detached receipt and exposes source coordinates only after publication.
func (x PipelineExecution) Public() PipelineRun {
	out := PipelineRun{Pipeline: x.Pipeline, Version: x.Version, State: x.State, Operation: x.Operation, Effects: make([]PipelineEffect, len(x.Stages))}
	for n, s := range x.Stages {
		out.Effects[n] = PipelineEffect{Step: s.Stage.Step, State: s.State, Rows: s.Rows, Digest: s.Stage.Digest, Code: s.Code}
		if x.State == "published" {
			out.Effects[n].Source = s.Stage.Source
			out.Effects[n].Context = s.Stage.Context
		}
	}
	return out
}

// Valid checks coherent phase-specific stage evidence and accepted dependencies.
func (s PipelineStageState) Valid() bool {
	stage := s.Stage
	stage.State = "checked"
	if stage.OID == 0 {
		stage.OID = 1
	}
	stage = sources.SealPipelineStage(stage)
	if !stage.Valid() || s.Rows < 0 || len(s.PriorApplications) > 32 {
		return false
	}
	if s.Previous != nil && !s.Previous.Valid() {
		return false
	}
	if s.State == "prepared" {
		return s.Fence == 0 && s.Stage.OID == 0 && s.Stage.State == "prepared" && s.Application == ""
	}
	if !s.Input.Valid() || s.Input.Tenant != s.Stage.Tenant || s.Fence < 1 || !strings.HasPrefix(s.Application, "cw-pipeline-") || len(s.Dependencies) < 1 {
		return false
	}
	allowed := map[string]bool{}
	for _, r := range s.Input.Relations {
		allowed[r.ID] = true
	}
	for _, id := range s.Dependencies {
		if !allowed[id] {
			return false
		}
		delete(allowed, id)
	}
	if s.State == "dispatching" {
		return s.Stage.State == "dispatching"
	}
	if s.State != "checked" && s.State != "quality_failed" && s.State != "applied" {
		return false
	}
	for _, hash := range []string{s.RenderedHash, s.ValidationHash, s.LineageHash} {
		raw, err := hex.DecodeString(hash)
		if err != nil || len(raw) != 32 || hex.EncodeToString(raw) != hash {
			return false
		}
	}
	return s.Stage.OID > 0 && (s.State != "checked" || s.Stage.Valid())
}
