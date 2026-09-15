package engineering

import (
	"testing"
	"time"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/jobs"
)

func TestReviewedSchedulePreservesOriginalRequest(t *testing.T) {
	original := jobs.Spec{Type: "manual", Timezone: "UTC", Missed: "skip", Overlap: "queue"}
	edited := jobs.Spec{Type: "interval", Timezone: "UTC", Missed: "skip", Overlap: "queue", IntervalSeconds: 60, Anchor: time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)}
	p := AutopilotProposal{Material: ProposalMaterial{Request: AutopilotGoal{ExpectedPipelineVersion: 3, Schedule: &AutopilotScheduleGoal{BindingID: "binding", Spec: original}}, Pipeline: PipelineDefinition{ID: "pipeline"}, ScheduleSpec: &edited}}
	got := ProposalScheduleRequest(p)
	if got.Target.Pipeline.Version != 4 || got.Target.Pipeline.Digest != readexec.Hash(p.Material.Pipeline) || got.Spec.Type != "interval" {
		t.Fatal("reviewed schedule not pinned", got)
	}
	if p.Material.Request.Schedule.Spec.Type != "manual" {
		t.Fatal("edit rewrote original goal")
	}
	bad := *p.Material.Request.Schedule
	bad.Spec.Type = "event"
	if validScheduleGoal(&bad) {
		t.Fatal("unsupported trigger accepted")
	}
	bad = *p.Material.Request.Schedule
	bad.ID = "existing"
	if validScheduleGoal(&bad) {
		t.Fatal("replacement without revision accepted")
	}
}
