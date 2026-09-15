package postgres

import (
	"context"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

var _ reporting.ScheduledRunRepository = (*DB)(nil)
var _ reporting.ScheduledCompositionRepository = (*DB)(nil)

func scheduledSealAuthority(inv jobs.Invocation, kind string) (identity.Envelope, jobs.RequestTask, error) {
	l := inv.Lease()
	if _, nested := inv.Parent(); nested || l.Task.Dispatch == nil || l.Task.Dispatch.Kind != jobs.ReportingKind ||
		!l.Task.Valid() || l.Task.Input.Kind != kind {
		return identity.Envelope{}, jobs.RequestTask{}, jobs.ErrAuthority
	}
	e, err := inv.Current(l.Task.Input.Kind, l.Task.Input.Target, l.Task.Input.InputHash)
	return e, l.Task, err
}

// SealScheduledFrozenRun requires both the prepared definition proof and the
// existing queue owner's opaque invocation. No caller may seal a different pin
// into an accepted occurrence, even if it has broader current resource reach.
func (d *DB) SealScheduledFrozenRun(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedRun) (reporting.RunRecord, error) {
	e, task, err := scheduledSealAuthority(inv, "reporting.run")
	if err != nil {
		return reporting.RunRecord{}, err
	}
	m, err := proof.Checked(e)
	if err != nil {
		return reporting.RunRecord{}, err
	}
	if err := reporting.CheckScheduledFrozen(*task.Dispatch, m); err != nil {
		return reporting.RunRecord{}, err
	}
	return d.sealFrozenRun(ctx, e, task, proof, &inv)
}

// SealScheduledComposition uses the ordinary composition store and indexes;
// the immutable root publication and selected widget pins are independently
// verified before any report/query result can be retained under this job.
func (d *DB) SealScheduledComposition(ctx context.Context, inv jobs.Invocation, proof reporting.PreparedComposition) (reporting.CompositionRecord, error) {
	e, task, err := scheduledSealAuthority(inv, "report.run")
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	m, err := proof.Checked(e)
	if err != nil {
		return reporting.CompositionRecord{}, err
	}
	if err := reporting.CheckScheduledComposition(*task.Dispatch, m); err != nil {
		return reporting.CompositionRecord{}, err
	}
	return d.sealComposition(ctx, e, task, proof, &inv)
}
