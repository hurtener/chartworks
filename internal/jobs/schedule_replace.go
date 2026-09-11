package jobs

import (
	"context"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

// ReplaceSchedule changes a reviewed schedule using CAS while retaining already
// accepted occurrences. The effect key reconciles a lost reply for this revision.
func (s *Service) ReplaceSchedule(ctx context.Context, e identity.Envelope, id string, expected int64, key string, request ScheduleRequest) (Schedule, error) {
	if ctx == nil || !identity.Identifier(id) || !identity.Identifier(key) || expected < 1 || expected >= 1<<62 || request.Validate() != nil {
		return Schedule{}, ErrInvalid
	}
	if err := access.Require(e, "scheduling.write", access.Resource{Tenant: e.Tenant(), Kind: "schedule", Permission: "write", ID: id}); err != nil {
		return Schedule{}, err
	}
	scope, err := s.admission(ctx, e, request.Target)
	if err != nil {
		return Schedule{}, err
	}
	ctx, stop := context.WithDeadline(ctx, e.Deadline())
	defer stop()
	return s.repo.ReplaceSchedule(ctx, scope, e.Session(), id, expected, key, request, s.limits)
}
