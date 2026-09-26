package nlqexec

import (
	"strconv"

	"github.com/hurtener/chartworks/internal/semantics"
)

func referenceKey(ref semantics.Reference) string {
	return string(ref.Kind) + "\x00" + ref.Dataset + "\x00" + ref.ID + "\x00" + strconv.FormatInt(ref.Revision, 10)
}

// retainCatalogSelection makes inferred roots addressable by the existing typed
// edit API. Required/default/answered concepts are reevaluated, not promoted into
// user choices. The route still resolves every coordinate against current pins.
func retainCatalogSelection(old QueryRecord, question *QuestionRequest) {
	if old.Route.Selection == nil {
		return
	}
	metricOwners := map[string]int{}
	for _, topic := range old.Route.Selection.Topics {
		for _, root := range topic.Roots {
			if root.Reference.Kind == semantics.KindMeasure || root.Reference.Kind == semantics.KindKPI {
				metricOwners[root.Reference.ID]++
			}
		}
	}
	for _, topic := range old.Route.Selection.Topics {
		for _, root := range topic.Roots {
			if root.Reason != "catalog_term" {
				continue
			}
			question.References = mergeReferences(question.References, []semantics.Reference{root.Reference})
			if (root.Reference.Kind == semantics.KindMeasure || root.Reference.Kind == semantics.KindKPI) && metricOwners[root.Reference.ID] == 1 {
				question.MetricIDs = mergeStrings(question.MetricIDs, []string{root.Reference.ID})
			}
		}
	}
}

// Omission means "do not infer this root from the utterance again", never
// "ignore a reviewed hard constraint". Explicit add/replacement clears the
// omission for its new target. Refine validates edits before calling this helper.
func retainSelectionOmissions(question *QuestionRequest, edits []ReferenceEdit) {
	for _, edit := range edits {
		if edit.Action == "remove" || edit.Action == "replace" {
			question.OmittedRoots = mergeReferences(question.OmittedRoots, []semantics.Reference{edit.Target})
		}
		var restore *semantics.Reference
		if edit.Action == "add" {
			restore = &edit.Target
		} else if edit.Action == "replace" {
			restore = edit.Replacement
		}
		if restore == nil {
			continue
		}
		kept := make([]semantics.Reference, 0, len(question.OmittedRoots))
		for _, ref := range question.OmittedRoots {
			if ref != *restore {
				kept = append(kept, ref)
			}
		}
		question.OmittedRoots = kept
	}
}
