package chartworks_test

import (
	"encoding/json"
	"fmt"
	"github.com/hurtener/chartworks/sdk/chartworks"
	"strings"
	"testing"
)

func TestSQLRecoveryLegacyIntentReviewSDK(t *testing.T) {
	in := chartworks.NLQRefineRequest{QueryID: "legacy", IntentReview: &chartworks.NLQLegacyIntentReview{QueryID: "pending", AnswerContext: "private-pin"}}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out chartworks.NLQRefineRequest
	if json.Unmarshal(b, &out) != nil || out.IntentReview == nil || out.IntentReview.QueryID != "pending" || out.IntentReview.AnswerContext != "private-pin" {
		t.Fatal("review roundtrip")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", *in.IntentReview, *in.IntentReview), "private-pin") {
		t.Fatal("review logs disclose private data")
	}
}
