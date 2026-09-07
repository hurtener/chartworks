package engineering

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/store"
	"reflect"
	"sort"
	"strings"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
)

func (s *PipelineService) InspectRun(ctx context.Context, e identity.Envelope, id string) (PipelineRun, error) {
	stop, err := s.begin(ctx, false)
	if err != nil {
		return PipelineRun{}, err
	}
	defer stop()
	run, err := s.repo.ReadPipelineExecution(ctx, e, id)
	if err != nil {
		return PipelineRun{}, err
	}
	return run.Public(), nil
}
func (s *PipelineService) Cancel(ctx context.Context, e identity.Envelope, id string) (jobs.RequestTask, error) {
	stop, err := s.begin(ctx, false)
	if err != nil {
		return jobs.RequestTask{}, err
	}
	defer stop()
	if _, err = s.repo.ReadPipelineExecution(ctx, e, id); err != nil {
		return jobs.RequestTask{}, err
	}
	return s.runner.Cancel(ctx, e, id)
}
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
		// Every exact physical output lock remains held through the single local
		// metadata publication transaction. This is not a warehouse transaction.
		records := make([]sources.Record, 0, len(record.Definition.Steps))
		var publish func(context.Context, int) error
		publish = func(held context.Context, index int) error {
			if index == len(record.Definition.Steps) {
				return s.repo.CompletePipelineExecution(held, inv, record, records)
			}
			return s.source.WithPipelineOutput(held, e, task.ID, record.Definition.Steps[index].ID, func(next context.Context, r sources.Record) error {
				records = append(records, r)
				return publish(next, index+1)
			})
		}
		return publish(work, 0)
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
