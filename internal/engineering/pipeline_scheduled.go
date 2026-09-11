package engineering

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/sources"
)

func (s *PipelineService) scheduledPipeline(ctx context.Context, e identity.Envelope, target jobs.PipelineTarget) (PipelineRecord, error) {
	if !target.Valid() {
		return PipelineRecord{}, ErrInvalid
	}
	record, err := s.repo.ReadPipeline(ctx, e, target.ID, target.Version, "engineering.pipeline.run", "write")
	if err != nil {
		return PipelineRecord{}, err
	}
	if record.State != "published" || record.Digest != target.Digest {
		return PipelineRecord{}, ErrState
	}
	if err := pipelineAuthority(e, record.Definition, "engineering.pipeline.run", "write"); err != nil {
		return PipelineRecord{}, err
	}
	if err := s.validateInputLocations(ctx, e, record.Definition); err != nil {
		return PipelineRecord{}, err
	}
	return record, nil
}

// ValidateScheduledPipeline resolves the exact published target and its complete
// external reach before accepting a schedule under the supplied current bearer.
func (s *PipelineService) ValidateScheduledPipeline(ctx context.Context, e identity.Envelope, target jobs.PipelineTarget) error {
	stop, err := s.begin(ctx, true)
	if err != nil {
		return err
	}
	defer stop()
	_, err = s.scheduledPipeline(ctx, e, target)
	return err
}

// ExecuteScheduledPipeline uses the occurrence's existing lease. It never admits
// an inner request task or acquires a second queue slot for the same execution.
func (s *PipelineService) ExecuteScheduledPipeline(ctx context.Context, lease jobs.Lease, proof auth.Execution) error {
	if jobs.AssertExecution(proof, lease.Job) != nil || lease.Job.Kind != jobs.PipelineKind {
		return jobs.ErrAuthority
	}
	stop, err := s.begin(ctx, true)
	if err != nil {
		return err
	}
	defer stop()
	work, cancel := context.WithTimeout(ctx, time.Duration(s.values.Pipelines.Timeout))
	defer cancel()
	joined := context.AfterFunc(s.life, cancel)
	defer joined()
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-work.Done():
		return work.Err()
	}
	e := proof.Envelope()
	record, err := s.scheduledPipeline(work, e, *lease.Job.Pipeline)
	if err != nil {
		return err
	}
	task, err := s.runner.Inspect(work, e, lease.Job.ID)
	if err != nil {
		return err
	}
	inv, err := jobs.PipelineInvocation(proof, lease, task)
	if err != nil {
		return err
	}
	c, err := s.connection(e, record.Definition.Connection)
	if err != nil {
		return err
	}
	schema, _, err := sources.ManagedLocation(c, record.Definition.ID)
	if err != nil {
		return ErrOwnership
	}
	if _, err := s.repo.ReservePipelineExecution(work, e, record, task, schema); err != nil {
		return err
	}
	return s.executePipeline(work, inv, record, c)
}
