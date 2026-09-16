package reporting

import (
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"slices"
	"strings"
)

// FieldPolicy is reviewed result classification, not permission to read values.
// Empty sensitivity is unknown and cannot reach a narrative provider.
type FieldPolicy struct {
	Name        string                       `json:"name"`
	Sensitivity semantics.LiteralSensitivity `json:"sensitivity,omitempty" jsonschema:"enum=non_sensitive,enum=sensitive"`
	Reason      string                       `json:"reason" jsonschema:"enum=reviewed_non_sensitive,enum=reviewed_sensitive,enum=unknown_lineage,enum=unknown_column,enum=unknown_sensitivity,enum=conflicting_metadata,enum=authored_sensitive"`
}

func physicalSchema(in []exec.Field) []exec.Field {
	out := clone(in)
	for i := range out {
		out[i].Sensitivity = ""
	}
	return out
}
func annotatedSchema(in []exec.Field, policy []FieldPolicy) []exec.Field {
	out := physicalSchema(in)
	if len(out) != len(policy) {
		return out
	}
	for i := range out {
		if out[i].Name == policy[i].Name {
			out[i].Sensitivity = policy[i].Sensitivity
		}
	}
	return out
}
func originSensitivity(origin exec.ColumnOrigin, definitions []topics.Definition) (semantics.LiteralSensitivity, string) {
	seen := map[semantics.LiteralSensitivity]bool{}
	for _, definition := range definitions {
		for _, dataset := range definition.Datasets {
			if dataset.ID != origin.Dataset {
				continue
			}
			for _, column := range dataset.Columns {
				if column.SourceName == origin.Column {
					seen[column.Sensitivity] = true
				}
			}
		}
	}
	if len(seen) == 0 {
		return "", "unknown_column"
	}
	if seen[semantics.LiteralSensitive] {
		if len(seen) > 1 {
			return semantics.LiteralSensitive, "conflicting_metadata"
		}
		return semantics.LiteralSensitive, "reviewed_sensitive"
	}
	if len(seen) > 1 {
		return "", "conflicting_metadata"
	}
	if seen[semantics.LiteralNonSensitive] {
		return semantics.LiteralNonSensitive, "reviewed_non_sensitive"
	}
	return "", "unknown_sensitivity"
}

// inheritedPolicy uses exact reviewed topics and validator-owned lineage. A
// caller's non_sensitive annotation or matching alias cannot declassify values.
func inheritedPolicy(d Definition, definitions []topics.Definition, lineage []exec.OutputLineage) []FieldPolicy {
	byName := map[string]exec.OutputLineage{}
	duplicate := map[string]bool{}
	for _, item := range lineage {
		if _, exists := byName[item.Output]; exists {
			duplicate[item.Output] = true
		}
		byName[item.Output] = item
	}
	out := make([]FieldPolicy, 0, len(d.ExpectedSchema))
	for _, field := range d.ExpectedSchema {
		policy := FieldPolicy{Name: field.Name, Reason: "unknown_lineage"}
		item, exists := byName[field.Name]
		if exists && !duplicate[field.Name] && item.Complete && len(item.Columns) > 0 {
			policy.Sensitivity, policy.Reason = semantics.LiteralNonSensitive, "reviewed_non_sensitive"
			for _, origin := range item.Columns {
				classification, reason := originSensitivity(origin, definitions)
				switch {
				case reason == "conflicting_metadata":
					if classification == semantics.LiteralSensitive || policy.Sensitivity != semantics.LiteralSensitive {
						policy.Sensitivity = classification
					}
					policy.Reason = reason
				case classification == semantics.LiteralSensitive:
					policy.Sensitivity = classification
					if policy.Reason != "conflicting_metadata" {
						policy.Reason = reason
					}
				case classification == "" && policy.Sensitivity != semantics.LiteralSensitive:
					policy.Sensitivity = ""
					if policy.Reason != "conflicting_metadata" {
						policy.Reason = reason
					}
				}
			}
		}
		if field.Sensitivity == semantics.LiteralSensitive {
			policy.Sensitivity, policy.Reason = semantics.LiteralSensitive, "authored_sensitive"
		} else if field.Sensitivity == semantics.LiteralNonSensitive && policy.Sensitivity != semantics.LiteralNonSensitive {
			policy.Reason = "conflicting_metadata"
		}
		out = append(out, policy)
	}
	return out
}
func supportedNarrativeLocale(locale string) bool {
	return locale == "en" || strings.HasPrefix(locale, "en-") || locale == "es" || strings.HasPrefix(locale, "es-")
}
func narrativeClaims(n Narrative) int {
	if n.MaxClaims == 0 {
		return 32
	}
	return n.MaxClaims
}
func narrativeResult(m RunManifest, result exec.Result) exec.Result {
	out := result
	policy := m.ResultPolicy
	if len(policy) == 0 {
		policy = inheritedPolicy(m.Revision.Definition, m.Definitions, m.ResultLineage)
	}
	out.Schema = annotatedSchema(result.Schema, policy)
	return out
}
func validResultPolicy(m RunManifest) bool {
	if m.Version == LegacyFrozenVersion {
		return true
	}
	want := inheritedPolicy(m.Revision.Definition, m.Definitions, m.ResultLineage)
	return len(want) > 0 && digest(want) == digest(m.ResultPolicy)
}
func validNarrativeEvidence(m RunManifest, n Narrative, evidence []NarrativeEvidence) bool {
	if m.Version == LegacyFrozenVersion && len(m.ResultPolicy) == 0 {
		return true
	}
	allowed := map[string]bool{}
	for _, field := range m.ResultPolicy {
		if field.Sensitivity == semantics.LiteralNonSensitive {
			allowed[field.Name] = true
		}
	}
	for _, redacted := range n.RedactedFields {
		delete(allowed, redacted)
	}
	for _, item := range evidence {
		name := item.Field
		if n.Reduction == "aggregate_evidence" {
			for _, prefix := range []string{"retained_sum(", "retained_count("} {
				if strings.HasPrefix(name, prefix) && strings.HasSuffix(name, ")") {
					name = name[len(prefix) : len(name)-1]
					break
				}
			}
		}
		if !allowed[name] || !slices.Contains(n.Fields, name) {
			return false
		}
	}
	return true
}
