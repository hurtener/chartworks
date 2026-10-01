package nlqroute

import (
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"regexp"
	"strings"
	"unicode"
)

// unresolvedQuestionRequirements recognizes compositional requirements that
// cannot be satisfied by a scalar metric proof. It does not infer missing values
// from a name or authorize a replacement metric. Quoted data is not intent.
func unresolvedQuestionRequirements(in RouteRequest) *Clarification {
	text := unquotedIntent(in.Question)
	words := strings.Fields(normalizedPhrase(text))
	has := func(terms ...string) bool {
		for _, w := range words {
			for _, term := range terms {
				if w == term {
					return true
				}
			}
		}
		return false
	}
	positive := func(terms ...string) bool {
		for i, w := range words {
			for _, term := range terms {
				if w != term {
					continue
				}
				neg := false
				for j := max(0, i-4); j < i; j++ {
					if words[j] == "not" || words[j] == "no" || words[j] == "ningun" || words[j] == "ningún" {
						neg = true
					}
				}
				if !neg {
					return true
				}
			}
		}
		return false
	}
	unknown := false
	for _, phrase := range []string{"unknown amount", "unknown amounts", "missing amount", "missing amounts", "amount is unknown", "amounts are unknown", "amount is missing", "amounts are missing", "importe desconocido", "importes desconocidos", "importe faltante", "importes faltantes", "monto desconocido", "montos desconocidos"} {
		unknown = unknown || containsPhrase(normalizedPhrase(text), phrase)
	}
	unknown = unknown && positive("unknown", "missing", "desconocido", "desconocidos", "faltante", "faltantes")
	definite := positive("definitive", "definitivo", "definitiva", "definitivos", "definitivas")
	if unknown && definite && has("total", "revenue", "net", "sum", "gross", "importe", "ingresos", "ventas", "neto", "neta", "bruto") {
		prompt := "Unknown amounts cannot establish a definitive total. Choose known-amount results with explicit unknown counts, or resolve the missing values."
		if in.Locale == nlq.LanguageSpanish {
			prompt = "Los importes desconocidos impiden un total definitivo. Elige importes conocidos con recuentos de desconocidos o resuelve los valores faltantes."
		}
		return &Clarification{Reason: "unknown_amount_policy_required", Outcome: semantics.ClarificationConflicting, Prompt: prompt}
	}
	repeated := has("repeated", "repeat", "fold", "repetida", "repetido", "duplicada", "duplicado")
	clock := has("hourly", "hour", "hours", "hora", "horas") || strings.Contains(strings.ToLower(text), "a.m.")
	daylight := has("dst") || strings.Contains(normalizedPhrase(text), "daylight saving") || strings.Contains(normalizedPhrase(text), "horario de verano")
	occurrence := strings.Contains(normalizedPhrase(text), "first occurrence") || strings.Contains(normalizedPhrase(text), "second occurrence") || strings.Contains(normalizedPhrase(text), "primera ocurrencia") || strings.Contains(normalizedPhrase(text), "segunda ocurrencia")
	if repeated && clock && daylight {
		prompt := "Choose which repeated local-clock occurrence or an explicit offset before hourly grouping."
		if in.Locale == nlq.LanguageSpanish {
			prompt = "Elige qué ocurrencia de la hora local repetida o un desplazamiento explícito antes de agrupar por hora."
		}
		offset, _ := regexp.MatchString(`[+-](?:0[0-9]|1[0-4]):[0-5][0-9]`, text)
		if occurrence || offset {
			return &Clarification{Reason: "unsupported_temporal_grain", Outcome: semantics.ClarificationInvalid, Prompt: "Hourly occurrence-aware grouping needs a reviewed supported time-grain contract."}
		}
		return &Clarification{Reason: "ambiguous_local_time", Outcome: semantics.ClarificationConflicting, Prompt: prompt}
	}
	if has("hourly") || containsPhrase(normalizedPhrase(text), "by hour") || containsPhrase(normalizedPhrase(text), "per hour") || containsPhrase(normalizedPhrase(text), "por hora") {
		return &Clarification{Reason: "unsupported_temporal_grain", Outcome: semantics.ClarificationInvalid, Prompt: "Choose a supported reviewed calendar grain before generation."}
	}
	return nil
}

func unquotedIntent(s string) string {
	var out strings.Builder
	var end rune
	runes := []rune(s)
	for i, r := range runes {
		if end != 0 {
			if r == end && (i == 0 || runes[i-1] != '\\') {
				end = 0
				out.WriteRune(' ')
			}
			continue
		}
		switch r {
		case '\'':
			if i == 0 || !unicode.IsLetter(runes[i-1]) && !unicode.IsDigit(runes[i-1]) {
				end = '\''
			} else {
				out.WriteRune(r)
			}
		case '"':
			end = '"'
		case '“':
			end = '”'
		case '‘':
			end = '’'
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func requestedAmountDisclosure(in RouteRequest) bool {
	q := normalizedPhrase(unquotedIntent(in.Question))
	action := false
	for _, p := range []string{"disclose", "report", "acknowledge", "show", "count", "include", "list", "aclara", "informa", "reporta", "indica", "muestra", "cuenta", "incluye", "enumera"} {
		words := strings.Fields(q)
		for i, word := range words {
			if word != p {
				continue
			}
			negated := i > 0 && (words[i-1] == "no" || words[i-1] == "dont" || words[i-1] == "not")
			action = action || !negated
		}
	}
	if !action {
		return false
	}
	for _, p := range []string{"unknown amount", "unknown amounts", "missing amount", "missing amounts", "amount is unknown", "amounts are unknown", "importes desconocidos", "importe desconocido", "montos desconocidos", "monto desconocido"} {
		if containsPhrase(q, p) {
			return true
		}
	}
	return false
}
