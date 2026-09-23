package onboardingapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/mcpserver"
	"github.com/hurtener/chartworks/internal/onboarding"
	"github.com/hurtener/chartworks/internal/store"
)

type IDRequest struct {
	ID string `json:"id"`
}

func MCPBindings(service *onboarding.Service) ([]mcpserver.Binding, error) {
	if service == nil {
		return nil, nil
	}
	registry, err := Registry()
	if err != nil {
		return nil, err
	}
	mapper := onboardingFault
	var out []mcpserver.Binding
	current := ""
	add := func(b mcpserver.Binding, e error) error {
		if e != nil {
			return fmt.Errorf("bind %s: %w", current, e)
		}
		out = append(out, b)
		return nil
	}
	current = "searchBusinessGoal"
	if err = add(mcpserver.Bind(registry, "searchBusinessGoal", "search_business_goal", "onboarding", "Search authorized current topics and private source/profile evidence for a business goal; results require explicit choice.", service.SearchGoal, mapper)); err != nil {
		return nil, err
	}
	current = "chooseBusinessGoal"
	if err = add(mcpserver.Bind(registry, "chooseBusinessGoal", "choose_business_goal", "onboarding", "Recheck exact current reuse evidence or create an unresolved private topic draft for independent review.", service.ChooseGoal, mapper)); err != nil {
		return nil, err
	}
	current = "startOnboarding"
	if err = add(mcpserver.Bind(registry, "startOnboarding", "start_onboarding", "onboarding", "Start a bounded private setup journey over registered sources and existing domain services. This does not create credentials or approve semantic meaning.", service.Start, mapper)); err != nil {
		return nil, err
	}
	current = "getOnboarding"
	if err = add(mcpserver.Bind(registry, "getOnboarding", "get_onboarding", "onboarding", "Read private progress and the next required human action without source or model work.", func(ctx context.Context, e identity.Envelope, in IDRequest) (onboarding.Run, error) {
		return service.Get(ctx, e, in.ID)
	}, mapper)); err != nil {
		return nil, err
	}
	current = "resumeOnboarding"
	if err = add(mcpserver.Bind(registry, "resumeOnboarding", "resume_onboarding", "onboarding", "Advance one bounded stage. Reuse the returned version and do not infer unresolved business meaning.", func(ctx context.Context, e identity.Envelope, in onboarding.ResumeRequest) (onboarding.Run, error) {
		return service.Resume(ctx, e, in.ID, in)
	}, mapper)); err != nil {
		return nil, err
	}
	current = "answerOnboarding"
	if err = add(mcpserver.Bind(registry, "answerOnboarding", "answer_onboarding", "onboarding", "Supply explicit answers or an exact independent topic review reference.", func(ctx context.Context, e identity.Envelope, in onboarding.AnswerRequest) (onboarding.Run, error) {
		return service.Answer(ctx, e, in.ID, in)
	}, mapper)); err != nil {
		return nil, err
	}
	current = "cancelOnboarding"
	if err = add(mcpserver.Bind(registry, "cancelOnboarding", "cancel_onboarding", "onboarding", "Cancel future setup stages while retaining the private progress receipt.", func(ctx context.Context, e identity.Envelope, in onboarding.CancelRequest) (onboarding.Run, error) {
		return service.Cancel(ctx, e, in.ID, in)
	}, mapper)); err != nil {
		return nil, err
	}
	current = "proposeOnboardingDrift"
	if err = add(mcpserver.Bind(registry, "proposeOnboardingDrift", "propose_onboarding_drift", "onboarding", "Create an affected-only private amendment after source drift; active definitions remain immutable.", func(ctx context.Context, e identity.Envelope, in onboarding.DriftRequest) (onboarding.Amendment, error) {
		return service.Drift(ctx, e, in.ID, in)
	}, mapper)); err != nil {
		return nil, err
	}
	return out, nil
}

func onboardingFault(err error) mcpserver.Fault {
	code := "unavailable"
	switch {
	case errors.Is(err, access.ErrUnauthenticated):
		code = "unauthenticated"
	case errors.Is(err, access.ErrForbidden):
		code = "forbidden"
	case errors.Is(err, store.ErrNotFound):
		code = "not_found"
	case errors.Is(err, store.ErrConflict):
		code = "conflict"
	case errors.Is(err, onboarding.ErrAttention):
		code = "attention_required"
	case errors.Is(err, onboarding.ErrCancelled):
		code = "cancelled"
	case errors.Is(err, onboarding.ErrBudget):
		code = "budget_exhausted"
	case errors.Is(err, onboarding.ErrInvalid), errors.Is(err, store.ErrInvalid):
		code = "invalid_request"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = "cancelled_or_timed_out"
	}
	return mcpserver.Fault{Code: code}
}
