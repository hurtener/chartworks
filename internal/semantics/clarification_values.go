package semantics

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func (e *ClarificationFieldError) Error() string { return "semantics: clarification " + e.Code }

func clarificationError(locale, field, code string) *ClarificationFieldError {
	message := "Check this field against the reviewed clarification policy."
	spanish := "Revisá este campo según la política de aclaración revisada."
	switch code {
	case "invalid_union":
		message, spanish = "Supply exactly one typed value.", "Ingresá exactamente un valor tipado."
	case "invalid_number":
		message, spanish = "Use a finite decimal without grouping separators or exponent notation.", "Usá un decimal finito sin separadores de miles ni notación con exponentes."
	case "numeric_precision":
		message, spanish = "The number exceeds the reviewed precision or scale; no rounding was applied.", "El número supera la precisión o escala revisada; no se aplicó redondeo."
	case "invalid_range":
		message, spanish = "The lower boundary must precede the upper boundary.", "El límite inferior debe ser anterior al límite superior."
	case "unit_mismatch":
		message, spanish = "Use the unit declared by this field.", "Usá la unidad declarada para este campo."
	case "invalid_boolean":
		message, spanish = "Use true/false or yes/no.", "Usá verdadero/falso o sí/no."
	case "invalid_date":
		message, spanish = "Use a real Gregorian date (YYYY-MM-DD) or the reviewed month-period input.", "Usá una fecha gregoriana válida (AAAA-MM-DD) o el período mensual revisado."
	case "ambiguous_calendar_boundary":
		message, spanish = "The local midnight is missing or ambiguous in the reviewed timezone. Choose an unambiguous boundary; no offset was guessed.", "La medianoche local no existe o es ambigua en la zona horaria revisada. Elegí un límite inequívoco; no se supuso ningún desplazamiento horario."
	case "unsupported_grain":
		message, spanish = "Choose one of the reviewed grains and align both boundaries to it.", "Elegí una granularidad revisada y alineá ambos límites con ella."
	case "calendar_mismatch":
		message, spanish = "Use the calendar and timezone declared by this field.", "Usá el calendario y la zona horaria declarados para este campo."
	case "invalid_choice":
		message, spanish = "Choose an exact option from the current reviewed question.", "Elegí una opción exacta de la pregunta revisada actual."
	case "unresolved_value":
		message, spanish = "This spelling does not resolve to a governed value. Choose a reviewed value or request review.", "Este texto no corresponde a un valor gobernado. Elegí un valor revisado o solicitá una revisión."
	case "stale_answer":
		message, spanish = "The publication changed. Reevaluate the question and confirm the current field.", "La publicación cambió. Volvé a evaluar la pregunta y confirmá el campo actual."
	case "not_applicable":
		message, spanish = "This field does not apply to the admitted question.", "Este campo no corresponde a la pregunta admitida."
	case "foreign_answer":
		message, spanish = "Use a field belonging to the admitted topic and policy.", "Usá un campo del tema y de la política admitidos."
	case "duplicate_answer":
		message, spanish = "Supply one answer per field.", "Ingresá una sola respuesta por campo."
	case "migration_required":
		message, spanish = "This legacy field needs a reviewed typed effect before it can accept a value.", "Este campo anterior necesita un efecto tipado revisado antes de aceptar un valor."
	case "conflicting_policies":
		message, spanish = "The applicable reviewed constraints are incompatible. Resolve the policy conflict before continuing.", "Las restricciones revisadas aplicables son incompatibles. Resolvé el conflicto entre políticas antes de continuar."
	case "required_answer":
		message, spanish = "Answer this required field before continuing.", "Respondé este campo obligatorio antes de continuar."
	case "null_not_allowed":
		message, spanish = "This field does not permit an explicit null predicate.", "Este campo no permite un predicado nulo explícito."
	}
	if locale == "es" {
		message = spanish
	}
	return &ClarificationFieldError{Field: field, Code: code, Message: message}
}

// ResolveClarificationValue performs pure bounded parsing. The caller must use
// an admitted reviewed slot. The returned fragment has no lifecycle, source,
// question or authority proof; ResolveClarifications supplies those business pins.
func ResolveClarificationValue(slot ClarificationSlot, value ClarificationValue, locale string) (ClarificationResolution, *ClarificationFieldError) {
	if locale != "en" && locale != "es" {
		return ClarificationResolution{}, clarificationError(locale, "locale", "invalid_union")
	}
	members := 0
	for _, present := range []bool{value.OptionID != "", value.Time != nil, value.Number != nil, value.Boolean != nil, value.Text != nil, value.Null} {
		if present {
			members++
		}
	}
	if members != 1 {
		return ClarificationResolution{}, clarificationError(locale, "value", "invalid_union")
	}
	out := ClarificationResolution{SchemaVersion: ClarificationSchemaVersion, ParserVersion: "clarification-values-v1", Locale: locale, Sensitivity: slot.Sensitivity}
	if slot.Kind == SlotChoice {
		if value.OptionID == "" || len(value.OptionID) > 128 {
			return ClarificationResolution{}, clarificationError(locale, "option_id", "invalid_choice")
		}
		for _, option := range slot.Choices {
			if option.ID == value.OptionID && option.Target != nil && option.Target.Valid() {
				ref := *option.Target
				out.Reference = &ref
				return out, nil
			}
		}
		return ClarificationResolution{}, clarificationError(locale, "option_id", "invalid_choice")
	}
	if slot.Effect == nil {
		return ClarificationResolution{}, clarificationError(locale, "value", "migration_required")
	}
	effect := cloneClarificationEffect(slot.Effect)
	out.Effect = effect
	if value.Null {
		if effect.Nulls != "only" {
			return ClarificationResolution{}, clarificationError(locale, "null", "null_not_allowed")
		}
		out.Null = true
		return out, nil
	}
	if effect.Nulls == "only" {
		return ClarificationResolution{}, clarificationError(locale, "null", "null_not_allowed")
	}
	switch slot.Kind {
	case SlotDate:
		if value.Time == nil || effect.Kind != "time_window" {
			return ClarificationResolution{}, clarificationError(locale, "time", "invalid_union")
		}
		window, err := resolveClarificationTime(*value.Time, *effect, locale)
		if err != nil {
			return ClarificationResolution{}, err
		}
		out.Time = &window
	case SlotNumber:
		if value.Number == nil || effect.Kind != "number" {
			return ClarificationResolution{}, clarificationError(locale, "number", "invalid_union")
		}
		if value.Number.Unit != effect.Unit {
			return ClarificationResolution{}, clarificationError(locale, "number.unit", "unit_mismatch")
		}
		canonical, err := canonicalClarificationDecimal(value.Number.Value, locale, effect.Precision, effect.Scale)
		if err != nil {
			return ClarificationResolution{}, err
		}
		out.Value = canonical
		if effect.Operator == "range" {
			upper, err := canonicalClarificationDecimal(value.Number.Upper, locale, effect.Precision, effect.Scale)
			if err != nil {
				err.Field = "number.upper"
				return ClarificationResolution{}, err
			}
			lo, _ := new(big.Rat).SetString(canonical)
			hi, _ := new(big.Rat).SetString(upper)
			if lo.Cmp(hi) > 0 || (lo.Cmp(hi) == 0 && effect.Bounds != "[]") {
				return ClarificationResolution{}, clarificationError(locale, "number.upper", "invalid_range")
			}
			out.Upper = upper
		} else if value.Number.Upper != "" {
			return ClarificationResolution{}, clarificationError(locale, "number.upper", "invalid_union")
		}
	case SlotBoolean:
		if value.Boolean == nil || effect.Kind != "boolean" || len(*value.Boolean) > 16 {
			return ClarificationResolution{}, clarificationError(locale, "boolean", "invalid_boolean")
		}
		switch strings.ToLower(strings.TrimSpace(*value.Boolean)) {
		case "true", "yes":
			out.Value = "true"
		case "false", "no":
			out.Value = "false"
		case "sí", "si", "verdadero":
			if locale != "es" {
				return ClarificationResolution{}, clarificationError(locale, "boolean", "invalid_boolean")
			}
			out.Value = "true"
		case "falso":
			if locale != "es" {
				return ClarificationResolution{}, clarificationError(locale, "boolean", "invalid_boolean")
			}
			out.Value = "false"
		default:
			return ClarificationResolution{}, clarificationError(locale, "boolean", "invalid_boolean")
		}
	case SlotText:
		if value.Text == nil || (effect.Kind != "entity" && effect.Kind != "text") || !validClarificationText(*value.Text, effect.MaxLength) {
			return ClarificationResolution{}, clarificationError(locale, "text", "unresolved_value")
		}
		needle := normalizeClarificationTerm(*value.Text)
		found := false
		for _, governed := range effect.Values {
			spellings := append([]string{governed.Canonical}, governed.Aliases...)
			for _, spelling := range spellings {
				if normalizeClarificationTerm(spelling) != needle {
					continue
				}
				if found && out.Value != governed.Canonical {
					return ClarificationResolution{}, clarificationError(locale, "text", "conflicting_policies")
				}
				out.Value, found = governed.Canonical, true
			}
		}
		if !found {
			return ClarificationResolution{}, clarificationError(locale, "text", "unresolved_value")
		}
	default:
		return ClarificationResolution{}, clarificationError(locale, "value", "invalid_union")
	}
	return out, nil
}

func canonicalClarificationDecimal(value, locale string, precision, scale int) (string, *ClarificationFieldError) {
	if !validClarificationText(value, 160) || precision < 1 || precision > 76 || scale < 0 || scale > precision || scale > 38 {
		return "", clarificationError(locale, "number.value", "invalid_number")
	}
	value = strings.TrimSpace(value)
	if locale == "es" {
		if strings.Contains(value, ",") && strings.Contains(value, ".") {
			return "", clarificationError(locale, "number.value", "invalid_number")
		}
		value = strings.ReplaceAll(value, ",", ".")
	}
	negative := strings.HasPrefix(value, "-")
	if negative || strings.HasPrefix(value, "+") {
		value = value[1:]
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || (len(parts) == 2 && len(parts[1]) == 0) {
		return "", clarificationError(locale, "number.value", "invalid_number")
	}
	for _, part := range parts {
		for _, ch := range part {
			if ch < '0' || ch > '9' {
				return "", clarificationError(locale, "number.value", "invalid_number")
			}
		}
	}
	integer := strings.TrimLeft(parts[0], "0")
	fraction := ""
	if len(parts) == 2 {
		fraction = strings.TrimRight(parts[1], "0")
	}
	if len(integer) > precision-scale || len(fraction) > scale {
		return "", clarificationError(locale, "number.value", "numeric_precision")
	}
	if integer == "" {
		integer = "0"
	}
	result := integer
	if fraction != "" {
		result += "." + fraction
	}
	if negative && result != "0" {
		result = "-" + result
	}
	return result, nil
}

func resolveClarificationTime(value ClarificationTimeInput, effect ClarificationEffect, locale string) (CanonicalClarificationTime, *ClarificationFieldError) {
	if value.Calendar != "gregorian" || value.Calendar != effect.Calendar || value.TimeZone != effect.TimeZone || len(value.TimeZone) > 128 {
		return CanonicalClarificationTime{}, clarificationError(locale, "time.time_zone", "calendar_mismatch")
	}
	if !containsClarificationString(effect.Grains, value.Grain) {
		return CanonicalClarificationTime{}, clarificationError(locale, "time.grain", "unsupported_grain")
	}
	zone, err := time.LoadLocation(effect.TimeZone)
	if err != nil {
		return CanonicalClarificationTime{}, clarificationError(locale, "time.time_zone", "calendar_mismatch")
	}
	var start, end time.Time
	if value.Period != "" {
		if value.Start != "" || value.End != "" || value.Grain != "month" || !validClarificationText(value.Period, 64) {
			return CanonicalClarificationTime{}, clarificationError(locale, "time.period", "invalid_date")
		}
		parts := strings.Fields(strings.ToLower(strings.TrimSpace(value.Period)))
		if len(parts) == 3 && parts[1] == "de" && locale == "es" {
			parts = []string{parts[0], parts[2]}
		}
		if len(parts) != 2 || len(parts[1]) != 4 {
			return CanonicalClarificationTime{}, clarificationError(locale, "time.period", "invalid_date")
		}
		year, parseErr := strconv.Atoi(parts[1])
		month := clarificationMonth(parts[0], locale)
		if parseErr != nil || year < 1 || year > 9998 || month == 0 {
			return CanonicalClarificationTime{}, clarificationError(locale, "time.period", "invalid_date")
		}
		start = time.Date(year, month, 1, 0, 0, 0, 0, zone)
		end = start.AddDate(0, 1, 0)
	} else {
		if len(value.Start) != 10 || len(value.End) != 10 {
			return CanonicalClarificationTime{}, clarificationError(locale, "time.start", "invalid_date")
		}
		start, err = time.ParseInLocation("2006-01-02", value.Start, zone)
		if err != nil || start.Year() < 1 || start.Format("2006-01-02") != value.Start {
			return CanonicalClarificationTime{}, clarificationError(locale, "time.start", "invalid_date")
		}
		end, err = time.ParseInLocation("2006-01-02", value.End, zone)
		if err != nil || end.Year() < 1 || end.Format("2006-01-02") != value.End {
			return CanonicalClarificationTime{}, clarificationError(locale, "time.end", "invalid_date")
		}
	}
	if !start.Before(end) {
		return CanonicalClarificationTime{}, clarificationError(locale, "time.end", "invalid_range")
	}
	if !clarificationGrainBoundary(start, value.Grain) || !clarificationGrainBoundary(end, value.Grain) {
		return CanonicalClarificationTime{}, clarificationError(locale, "time.grain", "unsupported_grain")
	}
	if !uniqueClarificationMidnight(start) || !uniqueClarificationMidnight(end) {
		return CanonicalClarificationTime{}, clarificationError(locale, "time.boundary", "ambiguous_calendar_boundary")
	}
	return CanonicalClarificationTime{BoundaryPolicy: ClarificationTimeBoundaryPolicy, StartUTC: start.UTC().Format(time.RFC3339), EndUTC: end.UTC().Format(time.RFC3339), LocalStart: start.Format("2006-01-02"), LocalEnd: end.Format("2006-01-02"), Calendar: "gregorian", TimeZone: effect.TimeZone, Grain: value.Grain, Bounds: "[)"}, nil
}

func clarificationMonth(value, locale string) time.Month {
	months := strings.Fields("january february march april may june july august september october november december")
	if locale == "es" {
		months = strings.Fields("enero febrero marzo abril mayo junio julio agosto septiembre octubre noviembre diciembre")
	}
	for i, month := range months {
		if month == value {
			return time.Month(i + 1)
		}
	}
	return 0
}

func clarificationGrainBoundary(value time.Time, grain string) bool {
	if value.Hour() != 0 || value.Minute() != 0 || value.Second() != 0 || value.Nanosecond() != 0 {
		return false
	}
	switch grain {
	case "day":
		return true
	case "week":
		return value.Weekday() == time.Monday
	case "month":
		return value.Day() == 1
	case "quarter":
		return value.Day() == 1 && (int(value.Month())-1)%3 == 0
	case "year":
		return value.Day() == 1 && value.Month() == time.January
	}
	return false
}

func validClarificationText(value string, limit int) bool {
	return limit > 0 && len(value) > 0 && len(value) <= limit && utf8.ValidString(value) && strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\x00\r\n")
}

func normalizeClarificationTerm(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(value)), " ")
}

func clarificationTokens(value string) string {
	return " " + strings.Join(strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) }), " ") + " "
}

func containsClarificationString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneClarificationEffect(effect *ClarificationEffect) *ClarificationEffect {
	if effect == nil {
		return nil
	}
	copy := *effect
	copy.Grains = append([]string(nil), effect.Grains...)
	copy.Values = append([]GovernedClarificationValue(nil), effect.Values...)
	for i := range copy.Values {
		copy.Values[i].Aliases = append([]string(nil), effect.Values[i].Aliases...)
	}
	return &copy
}

func cloneClarificationValue(value *ClarificationValue) *ClarificationValue {
	if value == nil {
		return nil
	}
	copy := *value
	if value.Time != nil {
		item := *value.Time
		copy.Time = &item
	}
	if value.Number != nil {
		item := *value.Number
		copy.Number = &item
	}
	if value.Boolean != nil {
		item := *value.Boolean
		copy.Boolean = &item
	}
	if value.Text != nil {
		item := *value.Text
		copy.Text = &item
	}
	return &copy
}

// CloneClarificationAnswers detaches caller-owned unions before retention or reuse.
func CloneClarificationAnswers(values []ClarificationAnswer) []ClarificationAnswer {
	out := append([]ClarificationAnswer(nil), values...)
	for i := range out {
		out[i].Value = cloneClarificationValue(values[i].Value)
	}
	return out
}

// ClarificationEffectSummary is readable reviewed intent, never executable SQL.
func ClarificationEffectSummary(effect *ClarificationEffect) string {
	if effect == nil {
		return "Select the exact reviewed semantic reference."
	}
	return fmt.Sprintf("Apply %s to %s %s; operator %s; nulls %s; unit %s; bounds %s.", effect.Kind, effect.Target.Kind, effect.Target.ID, effect.Operator, effect.Nulls, effect.Unit, effect.Bounds)
}
