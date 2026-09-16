package reporting

import (
	"fmt"
	"slices"
	"strings"
)

// NarrativePolicyVersion versions deterministic claim selection and rendering.
// Empty is the retained legacy policy; it must not change historical text hashes.
const NarrativePolicyVersion = "bounded-narrative-v2"

func boundedNarrativePolicy(n Narrative) error {
	if n.PolicyVersion != NarrativePolicyVersion || n.Instructions != "evidence_only" || n.SchemaVersion != "grounded-narrative-v1" || !narrativeLocale(n.Locale) || n.MaxClaims < 1 || n.MaxClaims > 32 || !n.RequireEvidence || !n.RequireCaveats || !slices.Contains([]string{"summary", "comparison", "explanation"}, n.Type) || !slices.Contains([]string{"neutral", "concise", "technical"}, n.Tone) {
		return ErrNarrativePolicy
	}
	// Aggregation retains one sum/count per field, not paired observations of
	// that same field. Do not silently turn an authored comparison into a summary.
	if n.Type == "comparison" && n.Reduction != "first_rows" {
		return ErrNarrativePolicy
	}
	return nil
}

func narrativeClaimKinds(n Narrative) []string {
	if n.PolicyVersion == NarrativePolicyVersion {
		switch n.Type {
		case "summary":
			return []string{"value"}
		case "comparison":
			return []string{"difference"}
		}
	}
	// An explanation is a factual description of retained observations and exact
	// differences. Causation, extrapolation and arbitrary prose remain unsupported.
	return []string{"value", "difference"}
}

func narrativeClaimsAvailable(evidence []NarrativeEvidence, n Narrative) bool {
	if n.PolicyVersion != NarrativePolicyVersion || n.Type != "comparison" {
		return len(evidence) > 0
	}
	for i, a := range evidence {
		if !narrativeNumeric(a.Type) {
			continue
		}
		for _, b := range evidence[i+1:] {
			if a.ID != b.ID && a.Field == b.Field && a.Type == b.Type {
				if _, _, ok := narrativeNumber(a.Value); !ok {
					continue
				}
				if _, _, ok := narrativeNumber(b.Value); ok {
					return true
				}
			}
		}
	}
	return false
}

func narrativeClaimSchema(n Narrative) (string, error) {
	if n.PolicyVersion != "" {
		if err := boundedNarrativePolicy(n); err != nil {
			return "", err
		}
	}
	schema := strings.Replace(narrativeSchema, `"maxItems":32`, fmt.Sprintf(`"maxItems":%d`, maxClaims(n)), 1)
	switch kinds := narrativeClaimKinds(n); len(kinds) {
	case 1:
		schema = strings.Replace(schema, `"enum":["value","difference"]`, fmt.Sprintf(`"enum":[%q]`, kinds[0]), 1)
	}
	return schema, nil
}

// Tone is a deterministic template, not provider-generated prose. Quoted field
// names and values remain inert data; every claim retains evidence citations.
func narrativeTone(line string, evidence NarrativeEvidence, n Narrative) string {
	if n.PolicyVersion != NarrativePolicyVersion || n.Tone == "concise" {
		return line
	}
	spanish := n.Locale == "es" || strings.HasPrefix(n.Locale, "es-")
	if n.Tone == "technical" {
		if spanish {
			return fmt.Sprintf("Dato retenido (tipo=%s, fila=%d): %s", evidence.Type, evidence.Row, line)
		}
		return fmt.Sprintf("Retained evidence (type=%s, row=%d): %s", evidence.Type, evidence.Row, line)
	}
	if spanish {
		return "Observación: " + line
	}
	return "Observation: " + line
}
