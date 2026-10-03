package chartworks

import (
	"context"
	"errors"
	"github.com/hurtener/chartworks/internal/nlqexec"
)

type NLQExampleRequalificationRequest = nlqexec.ExampleRequalificationRequest
type NLQExampleRequalification = nlqexec.ExampleRequalification
type NLQQuestionRequest = nlqexec.QuestionRequest

// RequalifyExampleNLQ establishes current proof and creates a separate candidate.
// It does not replace the original example or grant activation approval.
func (c *Client) RequalifyExampleNLQ(ctx context.Context, in NLQExampleRequalificationRequest) (out NLQExample, err error) {
	if !wireID(in.ExampleID) || in.ExpectedVersion < 1 {
		return out, errors.New("chartworks: invalid example qualification")
	}
	err = c.callLimit(ctx, "POST", "/v1/nlq/examples/requalify", "", in, &out, 2<<20)
	return
}
