package nlqexec

import (
	"strings"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
)

const maxGenerationExplanationBytes = 1024

// retainGenerationExplanations is called only after a candidate has passed the
// applicable native, analytical and correction-equivalence checks. These notes
// describe that candidate, not failed proposals or a correctness certificate.
// Existing query fields are used so retained records need no reinterpretation.
func retainGenerationExplanations(record *QueryRecord, candidate generatedCandidate, answers []semantics.ClarificationAnswer, parent *QueryRecord) {
	resolutions := semantics.CloneClarificationResolutions(record.Route.Resolutions)
	redactionAnswers := semantics.CloneClarificationAnswers(record.Route.Request.Answers)
	redactionAnswers = append(redactionAnswers, semantics.CloneClarificationAnswers(answers)...)
	parameters := append([]exec.Parameter(nil), candidate.Parameters...)
	parameters = append(parameters, record.Parameters...)
	if parent != nil {
		resolutions = append(resolutions, semantics.CloneClarificationResolutions(parent.Route.Resolutions)...)
		redactionAnswers = append(redactionAnswers, semantics.CloneClarificationAnswers(parent.Route.Request.Answers)...)
		parameters = append(parameters, parent.Parameters...)
	}
	// Scalar parameters have no public sensitivity contract. Conservatively
	// remove their exact known spellings; never publish bindings via prose.
	for _, parameter := range parameters {
		if parameter.Value != "" {
			resolutions = append(resolutions, semantics.ClarificationResolution{Value: parameter.Value, Sensitivity: semantics.LiteralSensitive})
		}
	}
	// Canonical persistence no longer retains the user's original alias. Keep
	// redaction reproducible for active sensitive values on execution repair by
	// consulting only that value's reviewed aliases, not unrelated vocabulary.
	var aliases []semantics.ClarificationResolution
	for _, resolution := range resolutions {
		if resolution.Sensitivity != semantics.LiteralSensitive || resolution.Effect == nil {
			continue
		}
		for _, value := range resolution.Effect.Values {
			if strings.Join(strings.Fields(value.Canonical), " ") != resolution.Value {
				continue
			}
			for _, alias := range value.Aliases {
				aliases = append(aliases, semantics.ClarificationResolution{Value: alias, Sensitivity: semantics.LiteralSensitive})
			}
		}
	}
	resolutions = append(resolutions, aliases...)
	redact := func(input []string) []string {
		output := make([]string, len(input))
		for i, text := range input {
			text = semantics.RedactClarificationText(text, redactionAnswers, resolutions)
			// A short private literal can expand into many markers. Do not
			// violate response/storage limits or cut through a Unicode string.
			if len(text) > maxGenerationExplanationBytes {
				text = "[explanation withheld: redaction exceeds limit]"
			}
			output[i] = text
		}
		return output
	}
	record.Assumptions = redact(candidate.Assumptions)
	record.Ambiguities = redact(candidate.Ambiguities)
}
