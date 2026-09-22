package chartworks

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/api"
)

func TestEXP03InteractionOrderingAndOutcomeRichness(t *testing.T) {
	state, applied, err := ApplyInteraction(QueryInteractionState{}, QueryInteractionEvent{Generation: 1, Sequence: 1, Kind: "start", Status: "accepted"})
	if err != nil || !applied {
		t.Fatal("start", state, applied, err)
	}
	state, applied, err = ApplyInteraction(state, QueryInteractionEvent{Generation: 1, Sequence: 2, QueryID: "query-1", Kind: "progress", Status: "running"})
	if err != nil || !applied || state.QueryID != "query-1" {
		t.Fatal("progress", state, applied, err)
	}
	// A newer user intent wins. Its result cannot later be replaced by the old
	// generation even when the old response arrives with a larger sequence.
	state, applied, err = ApplyInteraction(state, QueryInteractionEvent{Generation: 2, Sequence: 1, Kind: "start", Status: "accepted"})
	if err != nil || !applied {
		t.Fatal("new generation", state, applied, err)
	}
	unchanged, applied, err := ApplyInteraction(state, QueryInteractionEvent{Generation: 1, Sequence: 99, QueryID: "query-1", Kind: "result", Status: "succeeded"})
	if err != nil || applied || !reflect.DeepEqual(unchanged, state) {
		t.Fatal("stale response replaced current state", unchanged, applied, err)
	}
	disconnected, applied, err := ApplyInteraction(state, QueryInteractionEvent{Generation: 2, Sequence: 2, Kind: "disconnect", Status: "transport_disconnected"})
	if err != nil || !applied || disconnected.Status != "accepted" || disconnected.Transport != "transport_disconnected" {
		t.Fatal("disconnect was treated as cancellation", disconnected, err)
	}
	cancelled, applied, err := ApplyInteraction(disconnected, QueryInteractionEvent{Generation: 2, Sequence: 3, Kind: "cancel", Status: "cancel_requested"})
	if err != nil || !applied || cancelled.Status != "cancel_requested" {
		t.Fatal("explicit cancellation lost", cancelled, err)
	}
	for _, status := range []string{"succeeded", "empty", "truncated", "failed", "uncertain", "cancelled", "timed_out", "interrupted"} {
		got, ok, eventErr := ApplyInteraction(cancelled, QueryInteractionEvent{Generation: 3, Sequence: 1, QueryID: "query-" + status, Kind: "result", Status: status})
		if eventErr != nil || !ok || got.Status != status {
			t.Errorf("outcome %s collapsed: %#v %v", status, got, eventErr)
		}
		for index, delayed := range []QueryInteractionEvent{
			{Generation: 3, Sequence: 2, QueryID: "query-" + status, Kind: "progress", Status: "running"},
			{Generation: 3, Sequence: 3, QueryID: "query-" + status, Kind: "disconnect", Status: "transport_disconnected"},
			{Generation: 3, Sequence: 4, QueryID: "query-" + status, Kind: "cancel", Status: "cancel_requested"},
			{Generation: 3, Sequence: 5, QueryID: "query-" + status, Kind: "feedback", Status: "feedback_accepted"},
			{Generation: 3, Sequence: 6, QueryID: "query-" + status, Kind: "refine", Status: "accepted"},
		} {
			before := got
			got, ok, eventErr = ApplyInteraction(got, delayed)
			if eventErr != nil || got.Status != status {
				t.Fatalf("terminal %s overwritten by %#v: %#v %v", status, delayed, got, eventErr)
			}
			if index == 1 || index == 3 {
				if !ok {
					t.Fatalf("terminal side effect %s was not recorded", delayed.Kind)
				}
			} else if ok || !reflect.DeepEqual(got, before) {
				t.Fatalf("terminal transition %s accepted: %#v", delayed.Kind, got)
			}
		}
	}
	for _, event := range []QueryInteractionEvent{
		{Generation: 4, Sequence: 1, Kind: "result", Status: "success"},
		{Generation: 4, Sequence: 1, Kind: "disconnect", Status: "cancelled"},
		{Generation: 4, Sequence: 1, Kind: "view", Status: "cancel_requested", View: "raw_rows"},
	} {
		if _, applied, eventErr := ApplyInteraction(cancelled, event); eventErr == nil || applied {
			t.Error("ambiguous/unknown event accepted", event)
		}
	}
	data, err := json.Marshal(QueryInteractionEvent{Generation: 4, Sequence: 1, QueryID: "query-safe", Kind: "result", Status: "uncertain"})
	if err != nil || strings.Contains(string(data), "sql") || strings.Contains(string(data), "prompt") || strings.Contains(string(data), "rows") {
		t.Fatal("interaction envelope can expose protected content", string(data), err)
	}
}

func TestInteractionMetadataRejectsUnknownRoles(t *testing.T) {
	registry := unitOperationRegistry(t)
	definitions := registry.Definitions()
	definitions[0].Interaction = "invented_success"
	if _, err := api.New(definitions); err == nil {
		t.Fatal("unknown interaction role accepted")
	}
}
