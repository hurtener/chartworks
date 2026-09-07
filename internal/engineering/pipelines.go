package engineering

import (
	"context"
	"reflect"
	"sort"
	"sync"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
)

// PipelineService owns only managed SQL pipelines; ordinary query adapters have
// no reference to its writer/runner. Construction does not resolve credentials.
type PipelineService struct {
	repo      PipelineRepository
	source    *sources.Service
	validator *readexec.Validator
	values    config.Values
	lookup    func(string) (string, bool)
	runner    *jobs.RequestRunner
	model     gateway.Engine
	mu        sync.RWMutex
	closed    bool
	life      context.Context
	cancel    context.CancelFunc
	slots     chan struct{}
}

func NewPipelineService(repo PipelineRepository, source *sources.Service, validator *readexec.Validator, model gateway.Engine, values config.Values, lookup func(string) (string, bool)) (*PipelineService, error) {
	if repo == nil || source == nil || lookup == nil || config.ValidatePipelines(values.Pipelines) != nil || (values.Pipelines.Enabled && (validator == nil || !values.Sources.Enabled)) {
		return nil, ErrInvalid
	}
	limits := jobs.Defaults()
	limits.GlobalConcurrency = values.Jobs.GlobalConcurrency
	limits.TenantConcurrency = values.Jobs.TenantConcurrency
	limits.MaxPending = values.Jobs.MaxPending
	limits.MaxPendingPerTenant = values.Jobs.MaxPendingPerTenant
	limits.MaxAttempts = values.Jobs.MaxAttempts
	runner, err := jobs.NewRequestRunner(repo, limits)
	if err != nil {
		return nil, err
	}
	values.Sources = values.Sources.Clone()
	life, cancel := context.WithCancel(context.Background())
	return &PipelineService{life: life, cancel: cancel, slots: make(chan struct{}, values.Pipelines.Concurrency), repo: repo, source: source, validator: validator, values: values, lookup: lookup, runner: runner, model: model}, nil
}
func (s *PipelineService) Enabled() bool { return s.values.Pipelines.Enabled }
func (s *PipelineService) Close()        { s.cancel(); s.mu.Lock(); defer s.mu.Unlock(); s.closed = true }
func (s *PipelineService) begin(ctx context.Context, mutate bool) (func(), error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, ErrUnavailable
	}
	if mutate && !s.Enabled() {
		s.mu.RUnlock()
		return nil, ErrUnavailable
	}
	return s.mu.RUnlock, nil
}
func pipelineAuthority(e identity.Envelope, d PipelineDefinition, action, permission string) error {
	refs := []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: permission, ID: d.ID}}
	for _, step := range d.Steps {
		if step.Source != "" {
			refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: step.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: step.Context})
		}
	}
	return access.Require(e, action, refs...)
}
func (s *PipelineService) validateInputLocations(ctx context.Context, e identity.Envelope, d PipelineDefinition) error {
	c, err := s.connection(e, d.Connection)
	if err != nil {
		return err
	}
	// Admission must also reject a writer reference that has moved independently
	// from the destination reader. Dispatch repeats this against the held pool.
	if _, _, err = managedWriterConfig(s.values, s.lookup, s.repo.DatabaseName(), c); err != nil {
		return err
	}
	for _, step := range d.Steps {
		if len(step.FromSteps) == 0 {
			if err := s.source.ValidatePipelineInputLocation(ctx, e, step.Source, step.Context, d.Connection); err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *PipelineService) validateInputs(ctx context.Context, e identity.Envelope, d PipelineDefinition) error {
	if err := s.validatePipelineDestination(ctx, e, d.Connection); err != nil {
		return err
	}
	if err := ValidatePipelineDefinition(d, s.values.Pipelines); err != nil {
		return err
	}
	if err := s.validateInputLocations(ctx, e, d); err != nil {
		return err
	}
	for _, step := range d.Steps {
		if len(step.FromSteps) > 0 {
			continue
		} // Real private stage bindings are required before this step executes.
		plan, err := s.validator.Validate(ctx, e, readexec.Request{Source: step.Source, Context: step.Context, SQL: step.SQL})
		if err != nil {
			return err
		}
		actual := plan.Receipt().Dependencies
		expected := append([]string(nil), step.Inputs...)
		sort.Strings(expected)
		sort.Strings(actual)
		if !reflect.DeepEqual(actual, expected) {
			return readexec.ErrBinding
		}
	}
	return nil
}
func (s *PipelineService) Draft(ctx context.Context, e identity.Envelope, d PipelineDefinition, expected int64) (PipelineVersion, error) {
	stop, err := s.begin(ctx, true)
	if err != nil {
		return PipelineVersion{}, err
	}
	defer stop()
	if err = pipelineAuthority(e, d, "engineering.pipeline.write", "write"); err != nil {
		return PipelineVersion{}, err
	}
	if err = s.validateInputs(ctx, e, d); err != nil {
		return PipelineVersion{}, err
	}
	record, err := s.repo.SavePipeline(ctx, e, d, expected)
	if err != nil {
		return PipelineVersion{}, err
	}
	return record.PipelineVersion, nil
}
func (s *PipelineService) Publish(ctx context.Context, e identity.Envelope, id string, version int64) (PipelineVersion, error) {
	stop, err := s.begin(ctx, true)
	if err != nil {
		return PipelineVersion{}, err
	}
	defer stop()
	record, err := s.repo.ReadPipeline(ctx, e, id, version, "engineering.pipeline.publish", "write")
	if err != nil {
		return PipelineVersion{}, err
	}
	if err = pipelineAuthority(e, record.Definition, "engineering.pipeline.publish", "write"); err != nil {
		return PipelineVersion{}, err
	}
	if err = s.validateInputs(ctx, e, record.Definition); err != nil {
		return PipelineVersion{}, err
	}
	record, err = s.repo.PublishPipeline(ctx, e, id, version)
	if err != nil {
		return PipelineVersion{}, err
	}
	return record.PipelineVersion, nil
}
func (s *PipelineService) Get(ctx context.Context, e identity.Envelope, id string, version int64) (PipelineVersion, error) {
	stop, err := s.begin(ctx, false)
	if err != nil {
		return PipelineVersion{}, err
	}
	defer stop()
	record, err := s.repo.ReadPipeline(ctx, e, id, version, "engineering.pipeline.read", "read")
	if err != nil {
		return PipelineVersion{}, err
	}
	return record.PipelineVersion, nil
}
