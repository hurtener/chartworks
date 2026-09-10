package reporting

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// normalizeQuestion is deterministic lexical normalization, not an embedding,
// translation or confidence claim. Authorization filters candidates first.
func normalizeQuestion(s string) string {
	s = norm.NFKC.String(strings.ToLower(s))
	var out strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r) {
			if space && out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return out.String()
}

func questionScore(a, b string) float64 {
	x, y := normalizeQuestion(a), normalizeQuestion(b)
	if x == "" || y == "" {
		return 0
	}
	if x == y {
		return 1
	}
	left, right := map[string]bool{}, map[string]bool{}
	for _, word := range strings.Fields(x) {
		left[word] = true
	}
	for _, word := range strings.Fields(y) {
		right[word] = true
	}
	intersection := 0
	for word := range left {
		if right[word] {
			intersection++
		}
	}
	return float64(intersection) / float64(len(left)+len(right)-intersection)
}
