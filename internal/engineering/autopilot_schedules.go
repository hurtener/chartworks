package engineering

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
)

// AutopilotScheduleGoal proposes a real recurrence for the reviewed pipeline.
// ID and ExpectedRevision address an explicit replacement; empty ID creates one.
type AutopilotScheduleGoal struct {
	ID               string    `json:"id,omitempty"`
	ExpectedRevision int64     `json:"expected_revision,omitempty"`
	BindingID        string    `json:"binding_id"`
	Spec             jobs.Spec `json:"spec"`
}

func validScheduleGoal(g *AutopilotScheduleGoal) bool {
	return g == nil || jobs.BindingID(g.BindingID) && g.Spec.Validate() == nil && ((g.ID == "" && g.ExpectedRevision == 0) || (identity.Identifier(g.ID) && g.ExpectedRevision > 0 && g.ExpectedRevision < 1<<62))
}

// NewAutopilotWithSchedules installs the ordinary durable scheduling service.
func NewAutopilotWithSchedules(repo AutopilotRepository, pipelines *PipelineService, limits config.Autopilot, queue *jobs.Service, topics ...AutopilotTopics) (*Autopilot, error) {
	if queue == nil {
		return nil, ErrInvalid
	}
	s, err := NewAutopilot(repo, pipelines, limits, topics...)
	if err != nil {
		return nil, err
	}
	s.schedules = queue
	return s, nil
}

func scheduleReferences(g *AutopilotScheduleGoal) []ProposalReference {
	if g == nil {
		return nil
	}
	refs := []ProposalReference{{Kind: "execution_binding", Permission: "use", ID: g.BindingID}}
	if g.ID != "" {
		refs = append(refs, ProposalReference{Kind: "schedule", Permission: "write", ID: g.ID})
	}
	return refs
}
func scheduleObjectID(g AutopilotGoal) string {
	if g.Schedule.ID != "" {
		return g.Schedule.ID
	}
	return g.ID + ".schedule"
}
func scheduleObjectCount(m ProposalMaterial) int {
	if m.Request.Schedule != nil {
		return 1
	}
	return 0
}

func (s *Autopilot) checkSchedule(ctx context.Context, e identity.Envelope, g AutopilotGoal, apply bool) error {
	if g.Schedule == nil {
		return nil
	}
	if !validScheduleGoal(g.Schedule) || s.schedules == nil || !s.schedules.DispatchEnabled() {
		return ErrUnavailable
	}
	if apply {
		refs := []access.Resource{access.Tenant(e, "write"), {Tenant: e.Tenant(), Kind: "execution_binding", Permission: "use", ID: g.Schedule.BindingID}}
		if g.Schedule.ID != "" {
			refs = append(refs, access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "write", ID: g.Schedule.ID})
		}
		if err := access.Require(e, "scheduling.write", refs...); err != nil {
			return err
		}
	}
	if g.Schedule.ID != "" {
		current, err := s.schedules.GetSchedule(ctx, e, g.Schedule.ID)
		if err != nil {
			return err
		}
		if current.Request.Target.Kind != jobs.PipelineKind || current.Request.Target.Pipeline == nil || current.Request.Target.Pipeline.ID != g.Pipeline {
			return ErrProposalConflict
		}
		// A lost replacement reply may already have advanced this exact revision;
		// the ordinary replacement store will verify the effect key and full content.
		if current.Revision != g.Schedule.ExpectedRevision && (!apply || current.Revision != g.Schedule.ExpectedRevision+1) {
			return ErrProposalConflict
		}
	}
	return nil
}

// ProposalScheduleRequest derives the target from the reviewed pipeline, never
// from a second caller-supplied version or SQL definition.
func ProposalScheduleRequest(p AutopilotProposal) jobs.ScheduleRequest {
	g := p.Material.Request.Schedule
	if g == nil {
		return jobs.ScheduleRequest{}
	}
	spec := g.Spec
	if p.Material.ScheduleSpec != nil {
		spec = *p.Material.ScheduleSpec
	}
	return jobs.ScheduleRequest{Target: jobs.Submission{Kind: jobs.PipelineKind, BindingID: g.BindingID, Pipeline: &jobs.PipelineTarget{ID: p.Material.Pipeline.ID, Version: p.Material.Request.ExpectedPipelineVersion + 1, Digest: readexec.Hash(p.Material.Pipeline)}}, Spec: spec}
}

// ProposalScheduleKey binds retry reconciliation to the exact reviewed revision.
func ProposalScheduleKey(p AutopilotProposal) string {
	return readexec.Hash([]any{"engineering-schedule-v1", p.ID, p.Revision, p.Digest})
}

func (s *Autopilot) applySchedule(ctx context.Context, e identity.Envelope, p AutopilotProposal) (AutopilotProposal, error) {
	if p.Material.Request.Schedule == nil || p.State == "applied" {
		return p, nil
	}
	request := ProposalScheduleRequest(p)
	g := p.Material.Request.Schedule
	var result jobs.Schedule
	var err error
	if g.ID == "" {
		result, err = s.schedules.CreateSchedule(ctx, e, ProposalScheduleKey(p), request)
	} else {
		result, err = s.schedules.ReplaceSchedule(ctx, e, g.ID, g.ExpectedRevision, ProposalScheduleKey(p), request)
	}
	if err != nil {
		return p, err
	}
	proof, err := prepareProposalApply(e, p, "engineering.autopilot.apply", "", nil)
	if err != nil {
		return p, err
	}
	return s.repo.RecordAutopilotSchedule(ctx, e, proof, result)
}
