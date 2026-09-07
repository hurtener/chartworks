package sources

import (
	"context"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/jackc/pgx/v5"
)

// PipelineStage is protected execution evidence for one physical managed table.
// It is never returned by ordinary source list/read operations.
type PipelineStage struct {
	Tenant, Actor, Session string
	Pipeline               string
	Version, Revision      int64
	Operation, Step        string
	Alias, Schema, Table   string
	OID                    int64
	Columns                []string
	Source, Context, State string
	Digest                 string
}

type PipelineLocation struct {
	Pipeline, Operation, Step, Alias, Schema, Table string
	Version                                         int64
	OID, Revision                                   int64
	Source, Context, Digest                         string
}

func pipelineStageDigest(s PipelineStage) string {
	s.Digest = ""
	return readexec.Hash(s)
}
func SealPipelineStage(s PipelineStage) PipelineStage {
	s.Columns = append([]string(nil), s.Columns...)
	s.Digest = pipelineStageDigest(s)
	return s
}
func (s PipelineStage) Valid() bool {
	if !identity.Identifier(s.Tenant) || !identity.Identifier(s.Actor) || !identity.Identifier(s.Session) || !identity.Identifier(s.Pipeline) || s.Version < 1 || s.Revision < 1 || !identity.Identifier(s.Operation) || !readexec.SQLIdentifier(s.Step) || !identity.Identifier(s.Alias) || !readexec.SQLIdentifier(s.Schema) || !readexec.SQLIdentifier(s.Table) || s.OID <= 0 || len(s.Columns) < 1 || len(s.Columns) > 256 || !identity.Identifier(s.Source) || s.Context != contextID(s.Source, s.Revision) || s.State != "checked" {
		return false
	}
	seen := map[string]bool{}
	for _, column := range s.Columns {
		if !readexec.SQLIdentifier(column) || seen[column] {
			return false
		}
		seen[column] = true
	}
	return s.Digest == pipelineStageDigest(s)
}
func (l PipelineLocation) Valid() bool {
	digest, err := hex.DecodeString(l.Digest)
	return err == nil && len(digest) == 32 && l.Digest == pipelineLocationDigest(l) && identity.Identifier(l.Pipeline) && l.Version > 0 && identity.Identifier(l.Operation) && readexec.SQLIdentifier(l.Step) && identity.Identifier(l.Alias) && readexec.SQLIdentifier(l.Schema) && readexec.SQLIdentifier(l.Table) && l.OID > 0 && l.Revision > 0 && identity.Identifier(l.Source) && l.Context == contextID(l.Source, l.Revision)
}
func (s PipelineStage) Location() PipelineLocation {
	l := PipelineLocation{Pipeline: s.Pipeline, Version: s.Version, Operation: s.Operation, Step: s.Step, Alias: s.Alias, Schema: s.Schema, Table: s.Table, OID: s.OID, Revision: s.Revision, Source: s.Source, Context: s.Context}
	l.Digest = pipelineLocationDigest(l)
	return l
}
func pipelineLocationDigest(l PipelineLocation) string { l.Digest = ""; return readexec.Hash(l) }

type pipelineStageRepository interface {
	ReadPipelineStage(context.Context, identity.Envelope, string, string) (PipelineStage, error)
}

type pipelineInput struct {
	service *Service
	stages  []PipelineStage
	binding readexec.Binding
}

type PipelinePlanAdapter interface {
	readexec.ReadAdapter
	WithPlan(context.Context, identity.Envelope, readexec.Plan, func(context.Context, string, []readexec.Parameter, readexec.Binding) error) error
}

var _ PipelinePlanAdapter = (*pipelineInput)(nil)

func (s *Service) pipelineStages(ctx context.Context, e identity.Envelope, operation string, steps []string) ([]PipelineStage, error) {
	repo, ok := s.repo.(pipelineStageRepository)
	if !ok || !identity.Identifier(operation) || len(steps) < 1 || len(steps) > 32 {
		return nil, store.ErrInvalid
	}
	seen := map[string]bool{}
	out := make([]PipelineStage, 0, len(steps))
	for _, step := range steps {
		if !readexec.SQLIdentifier(step) || seen[step] {
			return nil, store.ErrInvalid
		}
		seen[step] = true
		stage, err := repo.ReadPipelineStage(ctx, e, operation, step)
		if err != nil {
			return nil, err
		}
		if !stage.Valid() || stage.Tenant != e.Tenant() || stage.Actor != e.User() || stage.Session != e.Session() || stage.Operation != operation || stage.Step != step {
			return nil, readexec.ErrBinding
		}
		if err = access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: stage.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: stage.Context}); err != nil {
			return nil, err
		}
		out = append(out, stage)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Step < out[j].Step })
	for _, stage := range out[1:] {
		if stage.Pipeline != out[0].Pipeline || stage.Version != out[0].Version || stage.Alias != out[0].Alias {
			return nil, readexec.ErrBinding
		}
	}
	return out, nil
}

// NewPipelineInput creates a request-private validation adapter for checked stages.
func (s *Service) NewPipelineInput(ctx context.Context, e identity.Envelope, operation string, steps []string, sourceID, contextID string) (PipelinePlanAdapter, error) {
	if !identity.Identifier(sourceID) || !identity.Identifier(contextID) {
		return nil, store.ErrInvalid
	}
	if err := access.Require(e, "sources.query", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: sourceID}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: contextID}); err != nil {
		return nil, err
	}
	stages, err := s.pipelineStages(ctx, e, operation, steps)
	if err != nil {
		return nil, err
	}
	adapter := &pipelineInput{service: s, stages: stages}
	var binding readexec.Binding
	err = s.call(ctx, e, true, func(ctx context.Context) error {
		var resolveErr error
		binding, resolveErr = adapter.resolve(ctx, e, sourceID, contextID, nil)
		return resolveErr
	})
	if err != nil {
		return nil, err
	}
	adapter.binding = binding
	return adapter, nil
}

func (p *pipelineInput) WithPlan(ctx context.Context, e identity.Envelope, plan readexec.Plan, run func(context.Context, string, []readexec.Parameter, readexec.Binding) error) error {
	if run == nil {
		return store.ErrInvalid
	}
	id, partition := plan.Coordinates()
	return p.service.managedPipelineCall(ctx, e, func(ctx context.Context) error {
		var callbackErr error
		_, err := p.resolveDuration(ctx, e, id, partition, managedProbeTimeout(ctx), func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
			statement, parameters, err := plan.SQL(e, b)
			if err != nil {
				return err
			}
			callbackErr = run(ctx, statement, parameters, b.Clone())
			return callbackErr
		})
		if callbackErr != nil {
			return callbackErr
		}
		return err
	})
}

func (p *pipelineInput) Binding(_ context.Context, e identity.Envelope, id, context string) (readexec.Binding, error) {
	if !e.Valid() || e.Tenant() != p.stages[0].Tenant || e.User() != p.stages[0].Actor || e.Session() != p.stages[0].Session || id != p.binding.Source || context != p.binding.Context {
		return readexec.Binding{}, readexec.ErrBinding
	}
	refs := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: id}, {Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: context}}
	for _, stage := range p.stages {
		refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: stage.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: stage.Context})
	}
	if err := access.Require(e, "sources.query", refs...); err != nil {
		return readexec.Binding{}, err
	}
	return p.binding.Clone(), nil
}
func (p *pipelineInput) Explain(ctx context.Context, e identity.Envelope, candidate readexec.Candidate) error {
	id, partition := candidate.Coordinates()
	return p.service.call(ctx, e, true, func(ctx context.Context) error {
		_, err := p.resolve(ctx, e, id, partition, func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
			statement, parameters, err := candidate.SQL(e, b)
			if err != nil {
				return err
			}
			return explain(ctx, tx, statement, parameters)
		})
		return err
	})
}
func (p *pipelineInput) resolve(ctx context.Context, e identity.Envelope, id, partition string, consume func(context.Context, readTransaction, readexec.Binding) error) (readexec.Binding, error) {
	return p.resolveDuration(ctx, e, id, partition, time.Duration(p.service.settings.QueryTimeout), consume)
}
func (p *pipelineInput) resolveDuration(ctx context.Context, e identity.Envelope, id, partition string, timeout time.Duration, consume func(context.Context, readTransaction, readexec.Binding) error) (readexec.Binding, error) {
	first := p.stages[0]
	prefix := id + ":v"
	if !strings.HasPrefix(partition, prefix) {
		return readexec.Binding{}, readexec.ErrBinding
	}
	revision, err := strconv.ParseInt(strings.TrimPrefix(partition, prefix), 10, 64)
	if err != nil || revision < 1 {
		return readexec.Binding{}, readexec.ErrBinding
	}
	c, err := p.service.connection(e.Tenant(), first.Alias)
	if err != nil {
		return readexec.Binding{}, err
	}
	if c.ManagedSchema == "" {
		return readexec.Binding{}, readexec.ErrBinding
	}
	c.Relations = make([]config.SourceRelation, 0, len(p.stages))
	for _, stage := range p.stages {
		c.Relations = append(c.Relations, config.SourceRelation{Schema: stage.Schema, Name: stage.Table, Columns: append([]string(nil), stage.Columns...)})
	}
	var result readexec.Binding
	_, err = p.service.probeDuration(ctx, c, id, revision, timeout, func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
		if b.Context != partition {
			return readexec.ErrBinding
		}
		for _, stage := range p.stages {
			var oid int64
			if err := tx.QueryRow(ctx, `SELECT c.oid::bigint FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, stage.Schema, stage.Table).Scan(&oid); err != nil || oid != stage.OID {
				return readexec.ErrBinding
			}
		}
		b.Contract = "pipeline-contract:" + readexec.Hash(p.stages)[:32]
		b.Fingerprint = readexec.Hash([]any{b.Fingerprint, p.stages})
		result = b.Clone()
		if consume != nil {
			return consume(ctx, tx, b)
		}
		return nil
	})
	return result, err
}

// WithPipelineOutputs proves the complete output manifest under one native
// read transaction. Its pool use is constant even when outputs exceed MaxConns.
func (s *Service) WithPipelineOutputs(ctx context.Context, e identity.Envelope, operation string, steps []string, publish func(context.Context, []Record) error) error {
	if publish == nil {
		return store.ErrInvalid
	}
	stages, err := s.pipelineStages(ctx, e, operation, steps)
	if err != nil {
		return err
	}
	first := stages[0]
	c, err := s.connection(e.Tenant(), first.Alias)
	if err != nil {
		return err
	}
	if c.ManagedSchema == "" {
		return readexec.ErrBinding
	}
	c.Relations = make([]config.SourceRelation, 0, len(stages))
	for _, stage := range stages {
		c.Relations = append(c.Relations, config.SourceRelation{Schema: stage.Schema, Name: stage.Table, Columns: append([]string(nil), stage.Columns...)})
	}
	return s.managedPipelineCall(ctx, e, func(ctx context.Context) error {
		var callbackErr error
		_, err := s.probeDuration(ctx, c, first.Source, first.Revision, managedProbeTimeout(ctx), func(ctx context.Context, tx readTransaction, _ readexec.Binding) error {
			records := make([]Record, 0, len(stages))
			location, ok := ctx.Value(pipelineReadLocationKey{}).(pipelineReadLocation)
			if !ok {
				return readexec.ErrBinding
			}
			for _, stage := range stages {
				single := c
				single.Relations = []config.SourceRelation{{Schema: stage.Schema, Name: stage.Table, Columns: append([]string(nil), stage.Columns...)}}
				b, err := s.inspectReadContext(ctx, tx, single, stage.Source, stage.Revision, location.fingerprintLocation)
				if err != nil {
					return err
				}
				var oid int64
				if err := tx.QueryRow(ctx, `SELECT c.oid::bigint FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname=$1 AND c.relname=$2`, stage.Schema, stage.Table).Scan(&oid); err != nil || oid != stage.OID {
					return readexec.ErrBinding
				}
				// Published outputs become ordinary sources. Preserve the native
				// binding that later discovery, validation and reads will reproduce;
				// exact stage lineage remains sealed in the separate location.
				record := Record{Source: Source{ID: stage.Source, Name: stage.Pipeline + " " + stage.Step, Dialect: "postgres", Revision: stage.Revision, ContextID: stage.Context, Status: "registered"}, Connection: stage.Alias, Binding: b, Pipeline: ptrLocation(stage.Location())}
				if !record.Valid() {
					return store.ErrInvalid
				}
				records = append(records, record)
			}
			callbackErr = publish(ctx, records)
			return callbackErr
		})
		if callbackErr != nil {
			return callbackErr
		}
		return err
	})
}

// A location proof is request-private evidence from the actual read pool, not an
// authority grant. Managed writes must use that same physical PostgreSQL endpoint.
type pipelineReadLocationKey struct{}
type pipelineReadLocation struct {
	host, database, fingerprintLocation string
	port                                uint16
}

func withPipelineReadLocation(ctx context.Context, c *pgx.ConnConfig, location string) context.Context {
	return context.WithValue(ctx, pipelineReadLocationKey{}, pipelineReadLocation{host: c.Host, port: c.Port, database: c.Database, fingerprintLocation: location})
}

// RequirePipelineReadLocation binds managed writes to the held read endpoint.
func RequirePipelineReadLocation(ctx context.Context, c *pgx.ConnConfig) error {
	if ctx == nil {
		return readexec.ErrBinding
	}
	location, ok := ctx.Value(pipelineReadLocationKey{}).(pipelineReadLocation)
	if !ok || c == nil || location.host != c.Host || location.port != c.Port || location.database != c.Database {
		return readexec.ErrBinding
	}
	return nil
}

// ValidatePipelineInputLocation rejects unsupported cross-database execution
// before any operation is admitted. Dispatch repeats the proof against the held
// read connection and actual writer configuration.
func (s *Service) ValidatePipelineInputLocation(ctx context.Context, e identity.Envelope, id, partition, destination string) error {
	scope, err := sourceScope(e, "sources.query", "query", id)
	if err != nil {
		return err
	}
	return s.call(ctx, e, true, func(ctx context.Context) error {
		return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
			if record.Source.ContextID != partition {
				return readexec.ErrBinding
			}
			if err := actualContext(e, "sources.query", record); err != nil {
				return err
			}
			input, err := s.recordConnection(record)
			if err != nil {
				return err
			}
			target, err := s.connection(e.Tenant(), destination)
			if err != nil {
				return err
			}
			if input.Dialect != "postgres" || target.Dialect != "postgres" || target.ManagedSchema == "" {
				return readexec.ErrBinding
			}
			inputDSN, ok := s.lookup(strings.TrimPrefix(input.ReadDSN, "env:"))
			if !ok {
				return store.ErrUnavailable
			}
			targetDSN, ok := s.lookup(strings.TrimPrefix(target.ReadDSN, "env:"))
			if !ok {
				return store.ErrUnavailable
			}
			a, _, err := ParseApprovedDSN(inputDSN)
			if err != nil {
				return err
			}
			b, _, err := ParseApprovedDSN(targetDSN)
			if err != nil {
				return err
			}
			if a.Host != b.Host || a.Port != b.Port || a.Database != b.Database {
				return readexec.ErrBinding
			}
			return nil
		})
	})
}
func ptrLocation(l PipelineLocation) *PipelineLocation { return &l }

func managedProbeTimeout(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < time.Minute {
			return remaining
		}
	}
	return time.Minute
}

func (s *Service) managedPipelineCall(ctx context.Context, e identity.Envelope, run func(context.Context) error) error {
	if ctx == nil || !e.Valid() {
		return access.ErrUnauthenticated
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed || !s.settings.Enabled {
		return store.ErrUnavailable
	}
	ctx, expiry := context.WithDeadline(ctx, e.Deadline())
	defer expiry()
	return run(ctx)
}

// WithValidatedPipelineRead holds the ordinary source revision and native table
// locks through one managed runner callback. It does not expose a general SQL API.
func (s *Service) WithValidatedPipelineRead(ctx context.Context, e identity.Envelope, plan readexec.Plan, run func(context.Context, string, []readexec.Parameter, readexec.Binding) error) error {
	if run == nil {
		return store.ErrInvalid
	}
	id, partition := plan.Coordinates()
	scope, err := sourceScope(e, "sources.query", "query", id)
	if err != nil {
		return err
	}
	s.lifecycle.RLock()
	defer s.lifecycle.RUnlock()
	if s.closed || !s.settings.Enabled {
		return store.ErrUnavailable
	}
	ctx, expiry := context.WithDeadline(ctx, e.Deadline())
	defer expiry()
	return s.repo.WithSource(ctx, scope, id, func(ctx context.Context, record Record) error {
		if record.Source.ContextID != partition {
			return readexec.ErrBinding
		}
		connection, err := s.recordConnection(record)
		if err != nil {
			return err
		}
		var callbackErr error
		_, err = s.probeDuration(ctx, connection, id, record.Source.Revision, managedProbeTimeout(ctx), func(ctx context.Context, tx readTransaction, b readexec.Binding) error {
			statement, parameters, err := plan.SQL(e, b)
			if err != nil {
				return err
			}
			callbackErr = run(ctx, statement, parameters, b.Clone())
			return callbackErr
		})
		if callbackErr != nil {
			return callbackErr
		}
		return err
	})
}
