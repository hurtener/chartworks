package nlqexec

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
)

// Reference choices select catalog meaning, not a row predicate. This predicate
// is used only after sealed route validation or authenticated rule replay.
func referenceOnlyResolutions(route nlqroute.RouteResult) bool {
	if len(route.Resolutions) == 0 || route.Interpretation != nil && len(route.Interpretation.Values)+len(route.Interpretation.Temporal) != 0 {
		return false
	}
	for _, resolution := range route.Resolutions {
		if resolution.Effect != nil || resolution.Reference == nil || !resolution.Reference.Valid() {
			return false
		}
	}
	return true
}

func referenceOnlyBindingMatches(route nlqroute.RouteResult, binding exec.Binding) bool {
	return referenceOnlyResolutions(route) && binding.Valid() &&
		(route.SourceBindingDigest == "" || route.SourceBindingDigest == exec.Hash(binding))
}

// Reference-only children may retain value-free change receipts, but never a
// fabricated SQL predicate binding. Every current change must match replayed
// catalog evidence. The normal source validator still owns executable plans.
func validateReferenceOnlyEvidence(record QueryRecord, binding exec.Binding) error {
	if !referenceOnlyBindingMatches(record.Route, binding) {
		return exec.ErrBinding
	}
	evidence := record.Clarification
	if evidence == nil {
		return nil
	}
	if evidence.SchemaVersion != 1 || evidence.BaseSQL != "" || len(evidence.BaseParameters) != 0 ||
		exec.Hash(evidence.Binding) != exec.Hash(exec.BusinessBindingReceipt{}) {
		return exec.ErrBinding
	}
	type key struct{ topic, pattern, slot string }
	current := map[key]string{}
	for _, resolution := range record.Route.Resolutions {
		k := key{resolution.Topic, resolution.Pattern, resolution.Slot}
		if resolution.ID == "" || current[k] != "" {
			return exec.ErrBinding
		}
		current[k] = resolution.ID
	}
	seen := map[key]bool{}
	for _, change := range evidence.Changes {
		k := key{change.Topic, change.Pattern, change.Slot}
		if seen[k] {
			return exec.ErrBinding
		}
		seen[k] = true
		switch change.Action {
		case "resolved":
			if change.Previous != "" || change.Current == "" || current[k] != change.Current {
				return exec.ErrBinding
			}
		case "superseded":
			if change.Previous == "" || change.Current == "" || current[k] != change.Current {
				return exec.ErrBinding
			}
		case "removed":
			if change.Previous == "" || change.Current != "" || current[k] != "" {
				return exec.ErrBinding
			}
		default:
			return exec.ErrBinding
		}
	}
	for k := range current {
		if !seen[k] {
			return exec.ErrBinding
		}
	}
	return nil
}
