package engineering

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
)

func (s *PipelineService) authorizePipelineControl(ctx context.Context, e identity.Envelope, run PipelineExecution, action string) error {
	record, err := s.repo.ReadPipeline(ctx, e, run.Pipeline, run.Version, action, "write")
	if err != nil {
		return err
	}
	if record.Digest != run.Digest {
		return store.ErrInvalid
	}
	return pipelineAuthority(e, record.Definition, action, "write")
}

// InspectRun returns the authorized public receipt of a durable pipeline operation.
func (s *PipelineService) InspectRun(ctx context.Context, e identity.Envelope, id string) (PipelineRun, error) {
	stop, err := s.begin(ctx, false)
	if err != nil {
		return PipelineRun{}, err
	}
	defer stop()
	if !e.Valid() {
		return PipelineRun{}, access.ErrUnauthenticated
	}
	if !e.Has("jobs.read") {
		return PipelineRun{}, access.ErrForbidden
	}
	run, err := s.repo.ReadPipelineExecutionControl(ctx, e, id, "jobs.read")
	if err != nil {
		return PipelineRun{}, err
	}
	if err = s.authorizePipelineControl(ctx, e, run, "jobs.read"); err != nil {
		return PipelineRun{}, err
	}
	return run.Public(), nil
}

// Cancel records cancellation intent for an authorized pipeline operation.
func (s *PipelineService) Cancel(ctx context.Context, e identity.Envelope, id string) (jobs.RequestTask, error) {
	stop, err := s.begin(ctx, false)
	if err != nil {
		return jobs.RequestTask{}, err
	}
	defer stop()
	if !e.Valid() {
		return jobs.RequestTask{}, access.ErrUnauthenticated
	}
	if !e.Has("jobs.cancel") {
		return jobs.RequestTask{}, access.ErrForbidden
	}
	run, err := s.repo.ReadPipelineExecutionControl(ctx, e, id, "jobs.cancel")
	if err != nil {
		return jobs.RequestTask{}, err
	}
	if err = s.authorizePipelineControl(ctx, e, run, "jobs.cancel"); err != nil {
		return jobs.RequestTask{}, err
	}
	return s.repo.CancelPipelineExecution(ctx, e, id)
}

// Run admits or resumes an exact published version and executes its bounded stages.
func (s *PipelineService) Run(ctx context.Context, e identity.Envelope, id string, version int64, key string, resume bool) (PipelineRun, error) {
	stop, err := s.begin(ctx, true)
	if err != nil {
		return PipelineRun{}, err
	}
	defer stop()
	workContext, cancel := context.WithCancel(ctx)
	defer cancel()
	joined := context.AfterFunc(s.life, cancel)
	defer joined()
	ctx = workContext
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return PipelineRun{}, ctx.Err()
	}
	record, err := s.repo.ReadPipeline(ctx, e, id, version, "engineering.pipeline.run", "write")
	if err != nil {
		return PipelineRun{}, err
	}
	if record.State != "published" {
		return PipelineRun{}, ErrState
	}
	if err = pipelineAuthority(e, record.Definition, "engineering.pipeline.run", "write"); err != nil {
		return PipelineRun{}, err
	}
	c, err := s.connection(e, record.Definition.Connection)
	if err != nil {
		return PipelineRun{}, err
	}
	if err = s.validateInputLocations(ctx, e, record.Definition); err != nil {
		return PipelineRun{}, err
	}
	schema, _, err := sources.ManagedLocation(c, id)
	if err != nil {
		return PipelineRun{}, ErrOwnership
	}
	task, err := s.runner.Admit(ctx, e, key, jobs.RequestInput{Kind: "pipeline.run", Target: id, InputHash: record.Digest})
	if err != nil {
		return PipelineRun{}, err
	}
	execution, err := s.repo.ReservePipelineExecution(ctx, e, record, task, schema)
	if err != nil {
		return PipelineRun{}, err
	}
	if execution.State == "published" {
		return execution.Public(), nil
	}
	if resume {
		task, err = s.runner.Resume(ctx, e, task.ID)
		if err != nil {
			return PipelineRun{}, err
		}
	}
	_, runErr := s.runner.Run(ctx, e, task, time.Duration(s.values.Pipelines.Timeout), func(work context.Context, inv jobs.Invocation) error {
		done := map[string]bool{}
		for len(done) < len(record.Definition.Steps) {
			progressed := false
			for _, step := range record.Definition.Steps {
				if done[step.ID] {
					continue
				}
				ready := true
				for _, dep := range step.DependsOn {
					if !done[dep] {
						ready = false
					}
				}
				if !ready {
					continue
				}
				current, err := s.repo.ReadPipelineExecution(work, e, task.ID)
				if err != nil {
					return err
				}
				var state PipelineStageState
				for _, stage := range current.Stages {
					if stage.Stage.Step == step.ID {
						state = stage
					}
				}
				if state.Stage.Step == "" {
					return ErrState
				}
				if state.State != "checked" {
					statement := step.SQL
					validator := s.validator
					withPlan := s.source.WithValidatedPipelineRead
					sourceID, contextID := step.Source, step.Context
					if len(step.FromSteps) > 0 {
						sourceID, contextID = state.Stage.Source, state.Stage.Context
						private, err := s.source.NewPipelineInput(work, e, task.ID, step.FromSteps, sourceID, contextID)
						if err != nil {
							return err
						}
						validator, err = readexec.NewValidator(private, s.values.Exec)
						if err != nil {
							return err
						}
						withPlan = private.WithPlan
						for _, dep := range step.FromSteps {
							stage, err := s.repo.ReadPipelineStage(work, e, task.ID, dep)
							if err != nil {
								return err
							}
							statement = strings.ReplaceAll(statement, "{{step."+dep+"}}", quotePipelineRelation(stage.Schema, stage.Table))
						}
					}
					if strings.Contains(statement, "{{") || strings.Contains(statement, "}}") {
						return ErrInvalid
					}
					plan, err := validator.Validate(work, e, readexec.Request{Source: sourceID, Context: contextID, SQL: statement})
					if err != nil {
						return err
					}
					if len(step.FromSteps) == 0 {
						actual := plan.Receipt().Dependencies
						expected := append([]string(nil), step.Inputs...)
						sort.Strings(actual)
						sort.Strings(expected)
						if !reflect.DeepEqual(actual, expected) {
							return readexec.ErrBinding
						}
					}
					err = withPlan(work, e, plan, func(held context.Context, sql string, args []readexec.Parameter, b readexec.Binding) error {
						if len(args) != 0 {
							return ErrInvalid
						}
						executionStep := step
						executionStep.Inputs = plan.Receipt().Dependencies
						return pipelineFailure(s.executePipelineStage(held, inv, record, c, state, executionStep, sql, b))
					})
					if err != nil {
						return err
					}
				}
				done[step.ID] = true
				progressed = true
			}
			if !progressed {
				return ErrInvalid
			}
		}
		// One native transaction holds all exact output locks through the single
		// local metadata publication; it does not borrow once per output.
		steps := make([]string, 0, len(record.Definition.Steps))
		for _, step := range record.Definition.Steps {
			steps = append(steps, step.ID)
		}
		return s.source.WithPipelineOutputs(work, e, task.ID, steps, func(held context.Context, records []sources.Record) error {
			return s.repo.CompletePipelineExecution(held, inv, record, records)
		})
	})
	current, err := s.repo.ReadPipelineExecution(ctx, e, task.ID)
	if err != nil {
		return PipelineRun{}, err
	}
	return current.Public(), runErr
}
func quotePipelineRelation(schema, table string) string { return `"` + schema + `"."` + table + `"` }

// AdmitRun returns the durable operation ID before execution so explicit cancel
// is reachable while a separate request performs the bounded attempt.
func (s *PipelineService) AdmitRun(ctx context.Context, e identity.Envelope, id string, version int64, key string) (PipelineRun, error) {
	stop, err := s.begin(ctx, true)
	if err != nil {
		return PipelineRun{}, err
	}
	defer stop()
	record, err := s.repo.ReadPipeline(ctx, e, id, version, "engineering.pipeline.run", "write")
	if err != nil {
		return PipelineRun{}, err
	}
	if record.State != "published" {
		return PipelineRun{}, ErrState
	}
	if err = pipelineAuthority(e, record.Definition, "engineering.pipeline.run", "write"); err != nil {
		return PipelineRun{}, err
	}
	c, err := s.connection(e, record.Definition.Connection)
	if err != nil {
		return PipelineRun{}, err
	}
	if err = s.validateInputLocations(ctx, e, record.Definition); err != nil {
		return PipelineRun{}, err
	}
	schema, _, err := sources.ManagedLocation(c, id)
	if err != nil {
		return PipelineRun{}, ErrOwnership
	}
	task, err := s.runner.Admit(ctx, e, key, jobs.RequestInput{Kind: "pipeline.run", Target: id, InputHash: record.Digest})
	if err != nil {
		return PipelineRun{}, err
	}
	execution, err := s.repo.ReservePipelineExecution(ctx, e, record, task, schema)
	if err != nil {
		return PipelineRun{}, err
	}
	return execution.Public(), nil
}

// Driver diagnostics never cross the managed callback seam. Domain failures are
// closed, content-free sentinels (runner stage wrappers add only fixed labels).
func pipelineFailure(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{ErrInvalid, ErrOwnership, ErrState, ErrUnavailable, ErrLimit, ErrPipelineQuality, ErrPipelineUncertain, readexec.ErrUnsafe, readexec.ErrBinding, readexec.ErrUnsupported, jobs.ErrAuthority, store.ErrConflict, store.ErrUnavailable, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return err
		}
	}
	return ErrUnavailable
}
