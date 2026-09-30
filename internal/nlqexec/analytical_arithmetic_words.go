package nlqexec

import (
	"github.com/hurtener/chartworks/internal/semantics"
	"strings"
)

// reviewedArithmeticWords is a closed v6 operator spelling grammar over a
// reviewed formula, not free-text question interpretation. Input identifiers
// always win over operator-looking words; the ordinary AST compiler still checks
// the complete expression, every declared dependency and exact scalar grammar.
func reviewedArithmeticWords(expression string, inputs map[string]semantics.Reference) string {
	words := strings.Fields(expression)
	for i, word := range words {
		if _, input := inputs[word]; input {
			continue
		}
		switch strings.ToLower(word) {
		case "minus", "menos":
			words[i] = "-"
		case "plus", "más", "mas":
			words[i] = "+"
		case "times", "por":
			words[i] = "*"
		}
	}
	return strings.Join(words, " ")
}
