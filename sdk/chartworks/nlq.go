package chartworks

import (
	"context"

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

// NLQRouteStage records one bounded routing stage and its remote attribution.
type NLQRouteStage = nlqroute.Stage

// RouteNLQ sends one bounded question to the authorized routing consumer.
func (c *Client) RouteNLQ(ctx context.Context, in NLQRouteRequest) (out NLQRouteResult, err error) {
	err = c.callLimit(ctx, "POST", "/v1/nlq/routes", "", in, &out, 2<<20)
	return
}
