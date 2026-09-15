package jobs

import (
	"context"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/identity"
)

// PipelineInvocation carries the existing occurrence lease into domain effects.
// It cannot be created from a user bearer or serialized lease alone. Domain
// persistence still checks this fence against the live database row at commit.
func PipelineInvocation(proof auth.Execution, lease Lease, task RequestTask) (Invocation, error) {
	if AssertExecution(proof, lease.Job) != nil || lease.Job.Kind != PipelineKind || task.Dispatch == nil || !task.Valid() || task.Dispatch.ManifestHash != lease.Job.ManifestHash || task.Attempts != lease.Attempt {
		return Invocation{}, ErrAuthority
	}
	inv := Invocation{authority: proof.Envelope(), owned: true, lease: RequestLease{Task: task, Owner: lease.Owner, Fence: lease.Fence, Attempt: lease.Attempt, Until: lease.Until}}
	if !inv.Valid() {
		return Invocation{}, ErrAuthority
	}
	return inv, nil
}

// PipelineExecutor is the concrete managed-pipeline consumer of queued work.
type PipelineExecutor interface {
	ValidateScheduledPipeline(context.Context, identity.Envelope, PipelineTarget) error
	ExecuteScheduledPipeline(context.Context, Lease, auth.Execution) error
}

// NewWithPipeline installs the pipeline consumer before workers can be started.
func NewWithPipeline(repo Repository, authority Authority, limits Limits, pipeline PipelineExecutor, observer ...func(string)) (*Service, error) {
	if pipeline == nil {
		return nil, ErrInvalid
	}
	s, err := New(repo, authority, limits, observer...)
	if err != nil {
		return nil, err
	}
	s.pipeline = pipeline
	return s, nil
}
