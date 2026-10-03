package nlqexec

import (
	"context"

	"github.com/hurtener/chartworks/internal/exec"
)

const analyticalGroupedSelectionRecordVersion = 11

// Compile the independently reviewed lanes with their original v8/v10 meaning,
// then attach authenticated selection of complete groups. This does not push a
// shared dimension restriction into a guessed fact population.
func compileAnalyticalGroupedSelection(ctx context.Context, a admission, supplied [][]exec.BusinessConstraint) (*exec.AnalyticalContract, error) {
	if ctx == nil || len(supplied) != 1 || len(supplied[0]) == 0 || !hasActiveBusinessEvidence(a.route) || a.route.SourceBindingDigest != exec.Hash(a.binding) {
		return nil, exec.ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	baseVersion := analyticalRecordVersion
	if selectedMetricPeriods(a) || len(a.metricPeriods) > 0 {
		baseVersion = analyticalGroupedOwnedRecordVersion
	}
	base, err := compileAnalyticalVersion(ctx, a, baseVersion, []exec.BusinessConstraint{})
	if err != nil {
		// The empty-population probe can lack ordinary calendar intent during
		// durable replay. Preserve the previous compiler and its exact errors.
		return compileAnalyticalVersion(ctx, a, baseVersion, supplied[0])
	}
	if base == nil || base.GroupedPopulations == nil {
		// Ordinary scalar/join queries preserve their existing query-population proof.
		return compileAnalyticalVersion(ctx, a, baseVersion, supplied[0])
	}
	if selectedKnownAmountCompleteness(a) || a.binding.Dialect != "postgres" || base.Grain == nil || len(base.Grain.Columns) == 0 {
		return nil, analyticalUnsupported("analytical_group_selection_unsupported")
	}
	var ids []string
	for _, scope := range a.relationScope {
		ids = append(ids, scope.Dataset)
	}
	selection, err := exec.NewAnalyticalQueryPopulationWithin(ctx, a.binding, ids, supplied[0])
	if err != nil {
		return nil, err
	}
	base.Version = exec.AnalyticalGroupedSelectionVersion
	base.GroupSelection = selection
	if err = exec.ValidateAnalyticalGroupedPopulations(*base, a.binding); err != nil {
		return nil, err
	}
	return base, nil
}
