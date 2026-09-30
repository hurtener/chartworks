package semantics

import "github.com/hurtener/chartworks/internal/identity"

// GroupedPopulationPolicy is reviewed business meaning, not source permission.
// It authorizes alignment only for this exact set of independently aggregated
// fact datasets. Missing lane measures stay NULL, including counts.
type GroupedPopulationPolicy struct {
	Policy   string   `json:"policy"`
	Datasets []string `json:"datasets"`
}

const GroupedPopulationUnionPolicy = "union-null-equal-preserve-missing-v1"

func validateGroupedPopulationPolicy(p *GroupedPopulationPolicy) error {
	if p == nil {
		return nil
	}
	if p.Policy != GroupedPopulationUnionPolicy || len(p.Datasets) < 2 || len(p.Datasets) > 4 {
		return invalid(CodeInvalidValue, "grouped_population")
	}
	for i, id := range p.Datasets {
		if !identity.Identifier(id) || i > 0 && p.Datasets[i-1] >= id {
			return invalid(CodeInvalidReference, "grouped_population.datasets")
		}
	}
	return nil
}

func validateGroupedPopulationReferences(p TopicPack) error {
	if err := validateGroupedPopulationPolicy(p.GroupedPopulation); err != nil {
		return err
	}
	if p.GroupedPopulation == nil {
		return nil
	}
	for _, id := range p.GroupedPopulation.Datasets {
		found := false
		for _, d := range p.Datasets {
			found = found || d.ID == id
		}
		if !found {
			return invalid(CodeMissingReference, "grouped_population.datasets")
		}
	}
	return nil
}

func cloneGroupedPopulation(p *GroupedPopulationPolicy) *GroupedPopulationPolicy {
	if p == nil {
		return nil
	}
	out := *p
	out.Datasets = append([]string(nil), p.Datasets...)
	return &out
}
