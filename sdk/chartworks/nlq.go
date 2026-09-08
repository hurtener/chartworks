package chartworks

import (
	"context"
	"errors"

	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
)

// NLQRouteRequest mirrors the registered NLQ route request. The SDK forwards
// the current Pengui bearer through Client; it does not derive topic reach or rule state.
type NLQRouteRequest = nlqroute.RouteRequest

// NLQJoinChoice identifies a published relationship selected for a multi-topic route.
type NLQJoinChoice = nlqroute.JoinChoice

// NLQChoiceSelection supplies one reviewed rule clarification choice.
type NLQChoiceSelection = nlqroute.ChoiceSelection

// NLQClarification is a typed route outcome that asks the caller for a bounded choice.
type NLQClarification = nlqroute.Clarification

// NLQClarificationChoice is one reviewed choice exposed by a route clarification.
type NLQClarificationChoice = nlqroute.ClarificationChoice

// NLQRouteResult is the detached routing and context response.
type NLQRouteResult = nlqroute.RouteResult

// NLQContextView is the detached model-context projection in a route result.
type NLQContextView = nlqroute.ContextView

// NLQTopicRevision is one exact topic/version binding in ordered model context.
type NLQTopicRevision = nlq.TopicRevision

// NLQRouteStage records one bounded routing stage and its remote attribution.
type NLQRouteStage = nlqroute.Stage

// NLQPreflightRequest is the bounded question admission request.
type NLQPreflightRequest = nlqexec.PreflightRequest

// NLQPreflightResult is routing evidence returned before generation.
type NLQPreflightResult = nlqexec.PreflightResult

// NLQPlanRequest asks the governed service to generate and validate a plan.
type NLQPlanRequest = nlqexec.PlanRequest

// NLQPlanResult is a validated plan receipt; SQL is present only with the
// separate reporting.sql.read authority.
type NLQPlanResult = nlqexec.PlanResult

// NLQRunRequest executes a previously planned query under a caller operation key.
type NLQRunRequest = nlqexec.RunRequest

// NLQRunResult is the detached execution receipt and bounded result projection.
type NLQRunResult = nlqexec.RunResult

// NLQRefineRequest creates a child plan within the signed session.
type NLQRefineRequest = nlqexec.RefineRequest

// NLQFeedbackRequest records a bounded review of one planned query.
type NLQFeedbackRequest = nlqexec.FeedbackRequest

// NLQExampleStateRequest advances one reviewed example through its lifecycle.
type NLQExampleStateRequest = nlqexec.ExampleStateRequest

// NLQExample is one detached, tenant-scoped learning example.
type NLQExample = nlqexec.ExampleRecord

// NLQExamplesRequest selects bounded examples for one topic.
type NLQExamplesRequest struct {
	Topic string `json:"topic"`
	Limit int    `json:"limit"`
}

// NLQFeedbackResult confirms durable feedback acceptance.
type NLQFeedbackResult struct {
	Accepted bool `json:"accepted"`
}

// RouteNLQ sends one bounded question to the authorized routing consumer.
func (c *Client) RouteNLQ(ctx context.Context, in NLQRouteRequest) (out NLQRouteResult, err error) {
	err = c.callLimit(ctx, "POST", "/v1/nlq/routes", "", in, &out, 2<<20)
	return
}

// PreflightNLQ admits a question and returns current routing evidence.
func (c *Client) PreflightNLQ(ctx context.Context, in NLQPreflightRequest) (out NLQPreflightResult, err error) {
	err = c.callLimit(ctx, "POST", "/v1/nlq/preflight", "", in, &out, 2<<20)
	return
}

// PlanNLQ generates and validates one governed read plan.
func (c *Client) PlanNLQ(ctx context.Context, in NLQPlanRequest) (out NLQPlanResult, err error) {
	if in.Operation != "" && !wireID(in.Operation) {
		return out, errors.New("chartworks: invalid operation")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/plans", "", in, &out, 2<<20)
	return
}

// RunNLQ executes one previously planned query with an explicit operation key.
func (c *Client) RunNLQ(ctx context.Context, in NLQRunRequest) (out NLQRunResult, err error) {
	if !wireID(in.QueryID) || !wireID(in.Operation) {
		return out, errors.New("chartworks: invalid query operation")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/runs", "", in, &out, 2<<20)
	return
}

// RefineNLQ creates a refined child plan within the signed session.
func (c *Client) RefineNLQ(ctx context.Context, in NLQRefineRequest) (out NLQPlanResult, err error) {
	if !wireID(in.QueryID) {
		return out, errors.New("chartworks: invalid query id")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/refinements", "", in, &out, 2<<20)
	return
}

// FeedbackNLQ records one bounded review and returns its acceptance receipt.
func (c *Client) FeedbackNLQ(ctx context.Context, in NLQFeedbackRequest) (out NLQFeedbackResult, err error) {
	if !wireID(in.QueryID) {
		return out, errors.New("chartworks: invalid query id")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/feedback", "", in, &out, 2<<20)
	return
}

// ExampleStateNLQ advances a candidate through the reviewed example lifecycle.
func (c *Client) ExampleStateNLQ(ctx context.Context, in NLQExampleStateRequest) (out NLQExample, err error) {
	if !wireID(in.ExampleID) {
		return out, errors.New("chartworks: invalid example id")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/examples/state", "", in, &out, 2<<20)
	return
}

// ExamplesNLQ reads bounded examples through the same signed tenant boundary.
func (c *Client) ExamplesNLQ(ctx context.Context, in NLQExamplesRequest) (out []NLQExample, err error) {
	if !wireID(in.Topic) || in.Limit < 1 || in.Limit > nlq.MaxExamples+1 {
		return out, errors.New("chartworks: invalid example query")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/examples/read", "", in, &out, 2<<20)
	return
}
