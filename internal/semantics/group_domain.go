package semantics

// GroupDomainPolicy is reviewed group-existence meaning for ordinary aggregate
// queries. It grants no source reach and does not change independent fact lanes.
type GroupDomainPolicy struct {
	Policy string `json:"policy"`
	Domain string `json:"domain"`
}

const MetricGroupDomainPolicy = "metric-group-domain-v1"

func validateGroupDomain(p *GroupDomainPolicy) error {
	if p != nil && (p.Policy != MetricGroupDomainPolicy || p.Domain != "raw_source_groups" && p.Domain != "qualifying_population") {
		return invalid(CodeInvalidValue, "group_domain")
	}
	return nil
}

func cloneGroupDomain(p *GroupDomainPolicy) *GroupDomainPolicy {
	if p == nil {
		return nil
	}
	out := *p
	return &out
}
