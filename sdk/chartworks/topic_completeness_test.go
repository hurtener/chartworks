package chartworks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTopicCompletenessPublicDTO(t *testing.T) {
	measure := TopicMeasure{ID: "gross", Name: "Known gross", Aggregation: "sum", Completeness: &TopicKnownAmountCompleteness{Policy: TopicKnownAmountCompletenessPolicy, UnknownCount: TopicReference{Kind: "kpi", ID: "unknown_amounts"}}}
	raw, err := json.Marshal(measure)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TopicMeasure
	if json.Unmarshal(raw, &decoded) != nil || !reflect.DeepEqual(measure, decoded) {
		t.Fatal("completeness link lost")
	}
	measure.Completeness = nil
	raw, _ = json.Marshal(measure)
	if strings.Contains(string(raw), `"completeness"`) {
		t.Fatal("legacy omission changed")
	}
}
