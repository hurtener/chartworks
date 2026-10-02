package chartworks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTopicPeriodsAndRelationshipPublicDTO(t *testing.T) {
	kpi := TopicKPI{ID: "net", Name: "Known cohort net", Expression: "gross - refunds", Inputs: []TopicReference{{Kind: "measure", ID: "gross"}, {Kind: "measure", ID: "refunds"}}, Periods: &TopicMetricPeriodBindings{Policy: TopicMetricPeriodBindingsPolicy, Bindings: []TopicMetricPeriodBinding{{Measure: TopicReference{Kind: "measure", ID: "gross"}, Dimension: TopicReference{Kind: "dimension", ID: "order_time"}}, {Measure: TopicReference{Kind: "measure", ID: "refunds"}, Dimension: TopicReference{Kind: "dimension", ID: "order_time"}}}}}
	raw, err := json.Marshal(kpi)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TopicKPI
	if json.Unmarshal(raw, &decoded) != nil || !reflect.DeepEqual(kpi, decoded) {
		t.Fatal("period bindings lost in public DTO")
	}
	filter := TopicSemanticFilter{ID: "paid_parent", Relationship: "refund_order", Field: TopicReference{Kind: "column", Dataset: "orders", ID: "status"}, Operator: "eq", Values: []string{"P"}}
	raw, err = json.Marshal(filter)
	if err != nil {
		t.Fatal(err)
	}
	var got TopicSemanticFilter
	if json.Unmarshal(raw, &got) != nil || !reflect.DeepEqual(filter, got) {
		t.Fatal("exact relationship lost")
	}
	kpi.Periods = nil
	raw, _ = json.Marshal(kpi)
	if strings.Contains(string(raw), `"periods"`) {
		t.Fatal("legacy nil periods changed wire shape")
	}
	filter.Relationship = ""
	raw, _ = json.Marshal(filter)
	if strings.Contains(string(raw), `"relationship"`) {
		t.Fatal("legacy local filter changed wire shape")
	}
}
