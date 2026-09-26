package nlq

import (
	"encoding/json"
	"io"
	"strings"
)

// JoinProjectionColumn identifies an already reviewed physical column. It is
// prompt metadata, not a validator capability or proof of join cardinality.
type JoinProjectionColumn struct {
	Dataset string `json:"dataset"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

// ConfirmedJoinProjection names the selected, independently confirmed join of
// one topic. Every topic must supply the same relationship. Legacy packets with
// no such metadata keep the conservative complete-shared-relation rendering.
type ConfirmedJoinProjection struct {
	Version     string               `json:"version"`
	Topic       string               `json:"topic"`
	ID          string               `json:"id"`
	Type        string               `json:"type"`
	Cardinality string               `json:"cardinality"`
	Left        JoinProjectionColumn `json:"left"`
	Right       JoinProjectionColumn `json:"right"`
}

const joinProjectionKind = "confirmed_join_projection"

// JoinProjectionConstraint serializes a source-checked render-only relationship.
// The consumer independently checks the whole multi-topic set before narrowing.
func JoinProjectionConstraint(join ConfirmedJoinProjection, relations []SourceRelation) (MandatoryConstraint, error) {
	if _, err := validateJoinProjection(join, relations); err != nil {
		return MandatoryConstraint{}, err
	}
	raw, err := json.Marshal(join)
	if err != nil {
		return MandatoryConstraint{}, projectionOwnerFailure()
	}
	return MandatoryConstraint{ID: "join-projection-" + projectionIdentity([]string{join.Topic, join.ID}), Kind: joinProjectionKind, Text: string(raw)}, nil
}

func hasJoinProjection(input ContextInput) bool {
	if input.Constraints != nil {
		for _, c := range input.Constraints.Required {
			if c.Kind == joinProjectionKind {
				return true
			}
		}
	}
	return false
}

func joinColumnDependency(column JoinProjectionColumn) MetricDependency {
	// The existing projection resolver is the sole physical-coordinate mapper.
	raw, _ := json.Marshal(struct {
		Dataset string `json:"dataset"`
		Column  struct {
			ID   string `json:"id"`
			Name string `json:"source_name"`
		} `json:"column"`
	}{Dataset: column.Dataset, Column: struct {
		ID   string `json:"id"`
		Name string `json:"source_name"`
	}{column.ID, column.Name}})
	return MetricDependency{Kind: "column", ID: column.Dataset + ":" + column.ID, Text: string(raw)}
}

func validateJoinProjection(join ConfirmedJoinProjection, relations []SourceRelation) ([]MetricDependency, error) {
	if join.Version != "confirmed-joins-v1" || !validID(join.Topic) || !validID(join.ID) || (join.Type != "inner" && join.Type != "left") || join.Cardinality != "one_to_one" || join.Left.Dataset == join.Right.Dataset {
		return nil, projectionOwnerFailure()
	}
	for _, col := range []JoinProjectionColumn{join.Left, join.Right} {
		if !validID(col.Dataset) || !validID(col.ID) || !validText(col.Name, 256) {
			return nil, projectionOwnerFailure()
		}
	}
	deps := []MetricDependency{joinColumnDependency(join.Left), joinColumnDependency(join.Right)}
	if _, err := DependencyRelations(join.Topic, relations, deps); err != nil {
		return nil, err
	}
	return deps, nil
}

// confirmedJoinDependencies validates complete independently-owned coordinates.
// A partial/new malformed marker is an error, never a fallback to guessed keys.
func confirmedJoinDependencies(input ContextInput, topics []string, byTopic map[string][]SourceRelation) (map[string][]MetricDependency, bool, error) {
	if !hasJoinProjection(input) {
		return nil, false, nil
	}
	if input.Strategy != StrategyMultiTopic || len(topics) < 2 || len(topics) > 8 {
		return nil, false, projectionOwnerFailure()
	}
	known := map[string]bool{}
	for _, topic := range topics {
		known[topic] = true
	}
	out := map[string][]MetricDependency{}
	var relationship string
	for _, c := range input.Constraints.Required {
		if c.Kind != joinProjectionKind {
			continue
		}
		var join ConfirmedJoinProjection
		decoder := json.NewDecoder(strings.NewReader(c.Text))
		decoder.DisallowUnknownFields()
		if len(c.Text) > 4096 || decoder.Decode(&join) != nil || decoder.Decode(new(any)) != io.EOF || !known[join.Topic] || out[join.Topic] != nil || c.ID != "join-projection-"+projectionIdentity([]string{join.Topic, join.ID}) {
			return nil, false, projectionOwnerFailure()
		}
		deps, err := validateJoinProjection(join, byTopic[join.Topic])
		if err != nil {
			return nil, false, err
		}
		left, right := join.Left, join.Right
		if join.Type == "inner" && (left.Dataset > right.Dataset || left.Dataset == right.Dataset && left.ID > right.ID) {
			left, right = right, left
		}
		key := projectionIdentity([]any{join.Type, join.Cardinality, left, right})
		if relationship != "" && relationship != key {
			return nil, false, projectionOwnerFailure()
		}
		relationship = key
		out[join.Topic] = deps
	}
	if len(out) != len(known) {
		return nil, false, projectionOwnerFailure()
	}
	return out, true, nil
}
