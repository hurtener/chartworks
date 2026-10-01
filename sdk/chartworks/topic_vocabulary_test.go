package chartworks

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTopicVocabularyPublicDTOIsOptionalAndRoundTrips(t *testing.T) {
	request := EnhanceTopicRequest{Expected: 1, Version: "v2", Limit: 2, Change: "Explicit business vocabulary", Vocabulary: []TopicAuthoringValue{{ID: "paid", Kind: "text", Value: "P", Sensitivity: "non_sensitive", Nulls: "exclude", Field: TopicReference{Kind: "column", Dataset: "orders", ID: "status"}, Origin: TopicSourceReference{Source: "source", Context: "context", Dataset: "orders", ProfileVersion: "profile", ProfileDigest: strings.Repeat("a", 64), SourceRevision: 1}}}}
	raw, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded EnhanceTopicRequest
	if json.Unmarshal(raw, &decoded) != nil || len(decoded.Vocabulary) != 1 || decoded.Vocabulary[0].Value != "P" || decoded.Vocabulary[0].Origin.ProfileDigest != request.Vocabulary[0].Origin.ProfileDigest {
		t.Fatal("vocabulary DTO lost exact input")
	}
	request.Vocabulary = nil
	raw, err = json.Marshal(request)
	if err != nil || strings.Contains(string(raw), `"vocabulary":`) {
		t.Fatal("legacy request changed omission semantics")
	}
}
