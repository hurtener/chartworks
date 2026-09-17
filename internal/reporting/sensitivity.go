package reporting

import (
	"slices"

	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// ResultFieldPolicy can tighten the reviewed source classification or require
// redaction. A caller-authored non_sensitive assertion never declassifies data.
// Omitted sensitivity in an authored entry is explicitly unknown, not public.
type ResultFieldPolicy struct {
	Field       string                       `json:"field"`
	Sensitivity semantics.LiteralSensitivity `json:"sensitivity,omitempty" jsonschema:"enum=non_sensitive,enum=sensitive"`
	Redacted    bool                         `json:"redacted"`
}

// EffectiveFieldPolicy is the normalized evidence-egress decision. It is not an
// authorization grant: metadata, SQL and raw values retain their separate scopes.
// The digest binds exact reviewed topics/dependencies and classification evidence.
type EffectiveFieldPolicy struct {
	Field            string `json:"field"`
	Status           string `json:"status" jsonschema:"enum=allowed,enum=sensitive,enum=unknown,enum=conflicting,enum=redacted"`
	Basis            string `json:"basis"`
	ProvenanceDigest string `json:"provenance_digest"`
}

type columnPolicySource struct {
	Topic       string
	Version     string
	Dataset     string
	Column      string
	Sensitivity semantics.LiteralSensitivity
}

// ResolveResultPolicy uses the already verified dependency closure. The current
// reader proves relations and output names, not per-expression source lineage.
// Therefore every result field conservatively inherits the whole query's reviewed
// dependency-column classification (including predicates). Labels or authored
// mapping provenance cannot substitute for a lineage proof. Narrower lineage is
// deliberately unsupported, rather than silently treating derived values as safe.
func ResolveResultPolicy(d Definition, dependencies []Dependency, definitions []topics.Definition) []EffectiveFieldPolicy {
	sources := []columnPolicySource{}
	classifications := map[string][]semantics.LiteralSensitivity{}
	for _, definition := range definitions {
		pinned := false
		for _, pin := range d.Topics {
			pinned = pinned || pin.Topic == definition.Topic && pin.Version == definition.Version
		}
		if !pinned {
			continue
		}
		for _, dataset := range definition.Datasets {
			if dataset.Source.Source != d.Source || dataset.Source.Context != d.Context {
				continue
			}
			for _, column := range dataset.Columns {
				key := dataset.Source.Dataset + "\x00" + column.SourceName
				classifications[key] = append(classifications[key], column.Sensitivity)
				sources = append(sources, columnPolicySource{Topic: definition.Topic, Version: definition.Version, Dataset: dataset.Source.Dataset, Column: column.ID, Sensitivity: column.Sensitivity})
			}
		}
	}
	slices.SortFunc(sources, func(a, b columnPolicySource) int {
		for _, pair := range [][2]string{{a.Topic, b.Topic}, {a.Version, b.Version}, {a.Dataset, b.Dataset}, {a.Column, b.Column}, {string(a.Sensitivity), string(b.Sensitivity)}} {
			if pair[0] < pair[1] {
				return -1
			}
			if pair[0] > pair[1] {
				return 1
			}
		}
		return 0
	})
	provenance := digest([]any{"reviewed-query-evidence-v1", d.Topics, dependencies, sources, d.ResultPolicy})
	status := "allowed"
	unknown, sensitive, conflicting := len(dependencies) == 0, false, false
	for _, dependency := range dependencies {
		if dependency.Source != d.Source || dependency.Context != d.Context || len(dependency.Columns) == 0 {
			unknown = true
			continue
		}
		for _, column := range dependency.Columns {
			values := classifications[dependency.Dataset+"\x00"+column.Name]
			safe, secret := false, false
			if len(values) == 0 {
				unknown = true
			}
			for _, value := range values {
				switch value {
				case semantics.LiteralNonSensitive:
					safe = true
				case semantics.LiteralSensitive:
					secret = true
				default:
					unknown = true
				}
			}
			sensitive = sensitive || secret
			conflicting = conflicting || safe && secret
		}
	}
	if unknown {
		status = "unknown"
	}
	if sensitive {
		status = "sensitive"
	}
	if conflicting {
		status = "conflicting"
	}
	authored := map[string]ResultFieldPolicy{}
	for _, policy := range d.ResultPolicy {
		authored[policy.Field] = policy
	}
	out := make([]EffectiveFieldPolicy, 0, len(d.ExpectedSchema))
	for _, field := range d.ExpectedSchema {
		effective := status
		if policy, found := authored[field.Name]; found {
			switch {
			case policy.Redacted:
				effective = "redacted"
			case policy.Sensitivity == semantics.LiteralSensitive && effective != "conflicting":
				effective = "sensitive"
			case policy.Sensitivity == semantics.LiteralNonSensitive && effective == "sensitive":
				effective = "conflicting"
			case policy.Sensitivity == "" && effective == "allowed":
				effective = "unknown"
			}
		}
		out = append(out, EffectiveFieldPolicy{Field: field.Name, Status: effective, Basis: "reviewed_query_dependencies", ProvenanceDigest: provenance})
	}
	return out
}

func narrativePolicy(policy []EffectiveFieldPolicy, n Narrative) []EffectiveFieldPolicy {
	out := []EffectiveFieldPolicy{}
	for _, field := range policy {
		if !slices.Contains(n.Fields, field.Field) && !slices.Contains(n.RedactedFields, field.Field) {
			continue
		}
		if slices.Contains(n.RedactedFields, field.Field) {
			field.Status = "redacted"
		}
		out = append(out, field)
	}
	return out
}

func restrictNarrativeFields(policy []EffectiveFieldPolicy, n Narrative) Narrative {
	n = clone(n)
	allowed := []string{}
	for _, field := range policy {
		if field.Status == "allowed" && slices.Contains(n.Fields, field.Field) && !slices.Contains(n.RedactedFields, field.Field) {
			allowed = append(allowed, field.Field)
		}
	}
	n.Fields = allowed
	return n
}
