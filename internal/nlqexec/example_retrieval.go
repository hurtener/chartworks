package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
	"log/slog"
	"sort"
	"strings"
	"unicode"
)

// GenerationExampleQuery is server-built from current admitted provenance.
// It is not a public request, an example activation or a source capability.
type GenerationExampleQuery struct {
	Topic               string
	Context             string
	Locale              nlq.Language
	TopicVersion        string
	SourceBindingDigest string
	RuleVersions        []string
	Templates           []rulesets.TemplateSelection
	AllowOwned          bool
	ScopedPolicy        string `json:",omitempty"`
	SearchText          string
	Limit               int
}

func (GenerationExampleQuery) String() string         { return "generation-example-query(redacted)" }
func (q GenerationExampleQuery) GoString() string     { return q.String() }
func (q GenerationExampleQuery) LogValue() slog.Value { return slog.StringValue(q.String()) }

// GenerationExampleReader filters current eligible rows before query-aware
// truncation. Legacy repositories retain their distinct selection-policy marker.
type GenerationExampleReader interface {
	SelectGenerationExamples(context.Context, store.Scope, GenerationExampleQuery) ([]ExampleRecord, error)
}

func generationExampleQuery(a admission, question string) GenerationExampleQuery {
	q := GenerationExampleQuery{Topic: a.route.Topic, Context: a.context, TopicVersion: routeVersion(a.route, a.route.Topic), SourceBindingDigest: exec.Hash(a.binding), RuleVersions: append([]string(nil), a.route.RuleVersions...), Templates: append([]rulesets.TemplateSelection(nil), a.route.Templates...), Limit: maxLearningCandidates}
	if a.route.Context != nil {
		q.Locale = a.route.Context.Locale
	}
	constraints, err := a.route.ResolvedBusinessConstraints()
	q.AllowOwned = hasActiveBusinessEvidence(a.route) && err == nil && len(constraints) > 0 && a.route.SourceBindingDigest == q.SourceBindingDigest
	q.ScopedPolicy = currentScopedLearningPolicy(a)
	if groupedFactLearningUnsupported(a) {
		q.AllowOwned = false
	}
	set := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(question), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }) {
		if len(word) <= 128 {
			set[word] = true
		}
	}
	words := make([]string, 0, len(set))
	for word := range set {
		words = append(words, word)
	}
	sort.Strings(words)
	// Relevance is optional. Oversized query expansion stays explicit as no FTS
	// expansion; current eligibility still precedes the bounded weight fallback.
	if len(words) <= 128 {
		for i := range words {
			words[i] = "\"" + words[i] + "\""
		}
		q.SearchText = strings.Join(words, " OR ")
	}
	return q
}

// ExampleEligibilityEvidence records the pre-limit policy without pretending
// unqueried ineligible rows were inspected or exposing their identities.
type ExampleEligibilityEvidence struct {
	Version                string `json:"version"`
	OriginDigest           string `json:"origin_digest"`
	CurrentOwnedPredicates bool   `json:"current_owned_predicates"`
	CurrentScopedPolicy    string `json:"current_scoped_policy,omitempty"`
}

func exampleEligibilityEvidence(q GenerationExampleQuery) *ExampleEligibilityEvidence {
	q.SearchText = ""
	q.Limit = 0
	version := "current-example-eligibility-v1"
	if q.ScopedPolicy != "" {
		version = "current-example-eligibility-v2"
	}
	return &ExampleEligibilityEvidence{Version: version, OriginDigest: exec.Hash(q), CurrentOwnedPredicates: q.AllowOwned, CurrentScopedPolicy: q.ScopedPolicy}
}
