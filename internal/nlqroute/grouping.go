package nlqroute

import (
	"context"
	"sort"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

// GroupingPolicy identifies explicit logical grouping, never source authority.
const GroupingPolicy = "reviewed-grouping-v1"

// GroupingSelection replaces the complete grouping set. Nil means unspecified;
// a nonnil selection with zero keys explicitly requests a scalar total. Keys
// contain reviewed identities only, never physical SQL, timezone or credentials.
type GroupingSelection struct {
	Policy string        `json:"policy"`
	Keys   []GroupingKey `json:"keys"`
}

// GroupingKey names a direct dimension or one of its reviewed calendar grains.
type GroupingKey struct {
	Topic     string              `json:"topic"`
	Dimension string              `json:"dimension"`
	Grain     semantics.TimeGrain `json:"grain,omitempty"`
}

// CloneGrouping detaches and canonically orders a set without mutating input.
func CloneGrouping(in *GroupingSelection) *GroupingSelection {
	if in == nil {
		return nil
	}
	out := *in
	out.Keys = append([]GroupingKey{}, in.Keys...)
	sort.Slice(out.Keys, func(i, j int) bool {
		a, b := out.Keys[i], out.Keys[j]
		if a.Topic != b.Topic {
			return a.Topic < b.Topic
		}
		if a.Dimension != b.Dimension {
			return a.Dimension < b.Dimension
		}
		return a.Grain < b.Grain
	})
	return &out
}

// ValidateGrouping checks shape only; current authorized publication resolution
// and native/analytical validation are mandatory independent consumers.
func ValidateGrouping(in *GroupingSelection) error {
	if in == nil {
		return nil
	}
	if in.Policy != GroupingPolicy || len(in.Keys) > 16 {
		return ErrInvalid
	}
	seen := map[GroupingKey]bool{}
	for _, key := range in.Keys {
		if !identity.Identifier(key.Topic) || !identity.Identifier(key.Dimension) || seen[key] {
			return ErrInvalid
		}
		switch key.Grain {
		case "", semantics.GrainDay, semantics.GrainMonth, semantics.GrainQuarter, semantics.GrainYear:
		default:
			return ErrInvalid
		}
		seen[key] = true
	}
	return nil
}

func groupingContains(in *GroupingSelection, topic, dimension string) bool {
	if in == nil {
		return false
	}
	for _, k := range in.Keys {
		if k.Topic == topic && k.Dimension == dimension {
			return true
		}
	}
	return false
}

func selectGrouping(ctx context.Context, in RouteRequest, admitted []admittedTopic) error {
	if err := ValidateGrouping(in.Grouping); err != nil {
		return err
	}
	if in.Grouping == nil {
		return nil
	}
	for _, key := range in.Grouping.Keys {
		if err := ctx.Err(); err != nil {
			return err
		}
		found := false
		for i := range admitted {
			item := &admitted[i]
			if item.id != key.Topic {
				continue
			}
			for _, d := range item.publication.Definition.Dimensions {
				if d.ID != key.Dimension {
					continue
				}
				if len(d.Filters) > 0 {
					return ErrInvalid
				}
				if key.Grain == "" {
					if d.Role == semantics.DimensionTemporal {
						return ErrInvalid
					}
				} else {
					if d.Role != semantics.DimensionTemporal || d.Temporal == nil || d.Temporal.Calendar != "gregorian" {
						return ErrInvalid
					}
					allowed := false
					for _, g := range d.Temporal.Grains {
						allowed = allowed || g == key.Grain
					}
					if !allowed {
						return ErrInvalid
					}
				}
				if _, err := addSelectedRoot(item, semantics.Reference{Kind: semantics.KindDimension, ID: d.ID}, "grouping", in.OmittedRoots); err != nil {
					return err
				}
				found = true
			}
		}
		if !found {
			return ErrInvalid
		}
	}
	return nil
}
