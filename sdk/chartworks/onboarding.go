package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/onboarding"
)

type OnboardingStart = onboarding.StartRequest
type OnboardingRun = onboarding.Run
type OnboardingAnswer = onboarding.AnswerRequest
type OnboardingAmendment = onboarding.Amendment

func (c *Client) StartOnboarding(ctx context.Context, in OnboardingStart) (out OnboardingRun, err error) {
	err = c.call(ctx, "POST", "/v1/onboarding", "", in, &out)
	return
}
func (c *Client) Onboarding(ctx context.Context, id string) (out OnboardingRun, err error) {
	if !wireID(id) {
		return out, errors.New("chartworks: invalid onboarding identifier")
	}
	err = c.call(ctx, "GET", "/v1/onboarding/"+id, "", nil, &out)
	return
}
func (c *Client) ResumeOnboarding(ctx context.Context, id string, expected int64) (out OnboardingRun, err error) {
	if !wireID(id) || expected < 1 {
		return out, errors.New("chartworks: invalid onboarding resume")
	}
	err = c.call(ctx, "POST", "/v1/onboarding/resume", "", onboarding.ResumeRequest{ID: id, ExpectedVersion: expected}, &out)
	return
}
func (c *Client) AnswerOnboarding(ctx context.Context, id string, in OnboardingAnswer) (out OnboardingRun, err error) {
	if !wireID(id) || in.ExpectedVersion < 1 {
		return out, errors.New("chartworks: invalid onboarding answer")
	}
	in.ID = id
	err = c.call(ctx, "POST", "/v1/onboarding/answers", "", in, &out)
	return
}
func (c *Client) CancelOnboarding(ctx context.Context, id string, expected int64, reason string) (out OnboardingRun, err error) {
	if !wireID(id) || expected < 1 {
		return out, errors.New("chartworks: invalid onboarding cancellation")
	}
	err = c.call(ctx, "POST", "/v1/onboarding/cancel", "", onboarding.CancelRequest{ID: id, ExpectedVersion: expected, Reason: reason}, &out)
	return
}
func (c *Client) ProposeOnboardingDrift(ctx context.Context, id string, in onboarding.DriftRequest) (out OnboardingAmendment, err error) {
	if !wireID(id) || in.ExpectedVersion < 1 {
		return out, errors.New("chartworks: invalid onboarding drift")
	}
	in.ID = id
	err = c.call(ctx, "POST", "/v1/onboarding/drift", "", in, &out)
	return
}
