package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqroute"
)

type applicabilityKeeper interface {
	WithQueryApplicability(context.Context, identity.Envelope, string, nlqroute.RouteResult, nlqroute.RouteRequest, string, string) (context.Context, error)
	ReissueApplicabilityForCopy(context.Context, identity.Envelope, string, nlqroute.RouteResult) (nlqroute.RouteResult, error)
	ReissueApplicabilityForPending(context.Context, identity.Envelope, string, nlqroute.RouteResult) (nlqroute.RouteResult, error)
}

func (s *Service) withQueryApplicability(ctx context.Context, e identity.Envelope, record QueryRecord, next nlqroute.RouteRequest, action, transition string) (context.Context, error) {
	if record.Route.Applicability == nil {
		return ctx, nil
	}
	keeper, ok := s.router.(applicabilityKeeper)
	if !ok {
		return nil, exec.ErrBinding
	}
	return keeper.WithQueryApplicability(ctx, e, record.ID, record.Route, next, action, transition)
}
