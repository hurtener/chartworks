package chartworks_test

import (
	"encoding/json"
	"fmt"
	"github.com/hurtener/chartworks/sdk/chartworks"
	"strings"
	"testing"
)

func TestLearningRequalificationSDK(t *testing.T) {
	in := chartworks.NLQExampleRequalificationRequest{ExampleID: "retained", ExpectedVersion: 2, Anchor: chartworks.NLQQuestionRequest{Topic: "topic", Question: "PRIVATE_CURRENT_ANCHOR"}}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out chartworks.NLQExampleRequalificationRequest
	if json.Unmarshal(raw, &out) != nil || out.ExpectedVersion != 2 || out.Anchor.Question != in.Anchor.Question {
		t.Fatal("qualification wire")
	}
	if strings.Contains(fmt.Sprintf("%v %#v", in, in), "PRIVATE_CURRENT_ANCHOR") {
		t.Fatal("qualification log exposed anchor")
	}
}
