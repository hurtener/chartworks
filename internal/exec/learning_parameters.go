package exec

import (
	"context"
	"strings"
	"unicode/utf8"
)

// CheckLearningParameterContent is a conservative disclosure check after native
// validation, not a SQL-safety proof or a general DLP classifier. Parameterized
// learning must not carry a known binding copied into a literal/comment. Reuse
// the existing bounded scanner; unsupported comment/quote forms are ineligible
// for learning rather than scanned with a permissive fallback. Marker tokens
// are not compared with values: a private integer 1 is not the position "$1".
func CheckLearningParameterContent(ctx context.Context, sql string, parameters []Parameter) error {
	if ctx == nil || len(parameters) < 1 || len(parameters) > 64 || len(sql) > 32768 {
		return ErrBinding
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, p := range parameters {
		if !p.Valid() || !utf8.ValidString(p.Value) {
			return ErrBinding
		}
		if p.Kind == "integer" || p.Kind == "number" {
			if _, ok := analyticalNumber(p.Value); !ok {
				return ErrUnsupported
			}
		}
	}
	tokens, err := businessScan(ctx, sql, false)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrUnsupported
	}
	for i, token := range tokens {
		if err := ctx.Err(); err != nil {
			return err
		}
		switch token.kind {
		case 's', 'q':
			literal := strings.ToLower(strings.Join(strings.Fields(token.text), " "))
			for _, p := range parameters {
				value := strings.ToLower(strings.Join(strings.Fields(p.Value), " "))
				if value != "" && strings.Contains(literal, value) {
					return ErrUnsupported
				}
			}
		case 'n':
			// The reused transformation scanner is not a full numeric lexer.
			// Reject leading-dot/exponent forms rather than interpreting a
			// partial numeric token as proof that a private value is absent.
			if i > 0 && tokens[i-1].text == "." && tokens[i-1].end == token.start || i+1 < len(tokens) && tokens[i+1].start == token.end && tokens[i+1].kind == 'w' {
				return ErrUnsupported
			}
			literal, literalOK := analyticalNumber(token.text)
			for _, p := range parameters {
				if p.Kind != "integer" && p.Kind != "number" {
					continue
				}
				value, valueOK := analyticalNumber(p.Value)
				// A sign can be a preceding operator. Comparing absolute rational values
				// is deliberately conservative; never publish an equivalent signed value.
				if literalOK && valueOK && strings.TrimPrefix(literal, "-") == strings.TrimPrefix(value, "-") {
					return ErrUnsupported
				}
			}
		case 'w':
			for _, p := range parameters {
				if p.Kind == "text" && p.Value != "" && strings.Contains(strings.ToLower(token.text), strings.ToLower(strings.Join(strings.Fields(p.Value), " "))) {
					return ErrUnsupported
				}
			}
			if token.word("true") || token.word("false") {
				for _, p := range parameters {
					if p.Kind == "boolean" && strings.EqualFold(token.text, p.Value) {
						return ErrUnsupported
					}
				}
			}
		}
	}
	return nil
}
