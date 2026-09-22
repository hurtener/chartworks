package chartworks

import (
	"errors"
	"sort"
)

// ErrInvalidInteraction rejects malformed consumer state without echoing any
// caller content. The contract deliberately carries no prompt, SQL or rows.
var ErrInvalidInteraction = errors.New("chartworks: invalid query interaction")

// QueryJourney is generated from installed operation registrations. A slice can
// contain more than one legitimate implementation (for example retained result
// inspection and the original run response); consumers choose the operation
// whose registered action and resource loader match their current authority.
type QueryJourney map[string][]string

// InteractionJourney projects the complete shared consumer journey from actual
// operation metadata. Missing roles fail closed instead of creating a local
// placeholder action.
func InteractionJourney(rows []OperationInfo) (QueryJourney, error) {
	out := QueryJourney{}
	for _, row := range rows {
		if !interactionRole(row.Interaction) {
			return nil, ErrInvalidInteraction
		}
		if row.Interaction != "" {
			out[row.Interaction] = append(out[row.Interaction], row.ID)
		}
	}
	for _, role := range []string{"query_start_or_clarify", "query_progress_or_clarify", "query_cancel", "query_result", "query_view", "query_feedback", "query_refine_or_clarify"} {
		if len(out[role]) == 0 {
			return nil, ErrInvalidInteraction
		}
		sort.Strings(out[role])
	}
	return out, nil
}

// QueryInteractionState is a small consumer-side ordering contract shared by
// network and in-process callers. Generation identifies the newest submitted
// intent and Sequence orders observations within it. It stores no result data.
type QueryInteractionState struct {
	Generation uint64 `json:"generation"`
	Sequence   uint64 `json:"sequence"`
	QueryID    string `json:"query_id,omitempty"`
	Status     string `json:"status"`
	View       string `json:"view,omitempty"`
}

// QueryInteractionEvent carries only bounded lifecycle metadata. Disconnect is
// a transport observation; cancel_requested is accepted only from an explicit
// cancellation response. Outcome statuses remain distinct.
type QueryInteractionEvent struct {
	Generation uint64 `json:"generation"`
	Sequence   uint64 `json:"sequence"`
	QueryID    string `json:"query_id,omitempty"`
	Kind       string `json:"kind"`
	Status     string `json:"status,omitempty"`
	View       string `json:"view,omitempty"`
}

// ApplyInteraction applies a current event or ignores a stale response. It does
// not infer cancellation from disconnection and never converts a missing result
// into an empty successful result.
func ApplyInteraction(current QueryInteractionState, event QueryInteractionEvent) (QueryInteractionState, bool, error) {
	if event.Generation == 0 || event.Sequence == 0 || event.Generation < current.Generation || event.Generation == current.Generation && event.Sequence <= current.Sequence {
		if event.Generation > 0 && (event.Generation < current.Generation || event.Generation == current.Generation && event.Sequence <= current.Sequence) {
			return current, false, nil
		}
		return current, false, ErrInvalidInteraction
	}
	if event.QueryID != "" && !wireID(event.QueryID) || current.QueryID != "" && event.Generation == current.Generation && event.QueryID != "" && event.QueryID != current.QueryID {
		return current, false, ErrInvalidInteraction
	}
	next := QueryInteractionState{Generation: event.Generation, Sequence: event.Sequence, QueryID: event.QueryID, Status: event.Status, View: event.View}
	if next.QueryID == "" && event.Generation == current.Generation {
		next.QueryID = current.QueryID
	}
	switch event.Kind {
	case "start":
		if event.Status != "accepted" || event.View != "" {
			return current, false, ErrInvalidInteraction
		}
	case "progress":
		if event.Status != "routing" && event.Status != "planning" && event.Status != "running" {
			return current, false, ErrInvalidInteraction
		}
	case "clarify":
		if event.Status != "needs_clarification" {
			return current, false, ErrInvalidInteraction
		}
	case "cancel":
		if event.Status != "cancel_requested" && event.Status != "cancelled" {
			return current, false, ErrInvalidInteraction
		}
	case "disconnect":
		if event.Status != "transport_disconnected" {
			return current, false, ErrInvalidInteraction
		}
	case "result":
		if !queryOutcome(event.Status) {
			return current, false, ErrInvalidInteraction
		}
	case "view":
		if event.Status != current.Status || event.View != "table" && event.View != "chart" && event.View != "sql" {
			return current, false, ErrInvalidInteraction
		}
	case "feedback":
		if event.Status != "feedback_accepted" {
			return current, false, ErrInvalidInteraction
		}
	case "refine":
		if event.Status != "accepted" && event.Status != "needs_clarification" {
			return current, false, ErrInvalidInteraction
		}
	default:
		return current, false, ErrInvalidInteraction
	}
	return next, true, nil
}

func queryOutcome(status string) bool {
	switch status {
	case "succeeded", "empty", "truncated", "failed", "uncertain", "cancelled", "timed_out", "interrupted":
		return true
	default:
		return false
	}
}
