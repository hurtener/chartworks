package rendering

import (
	"math/big"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/charts"
)

// formatCell mirrors the read viewer's closed formatter vocabulary. It never
// interprets a caller-supplied pattern, code fragment or locale implementation.
func formatCell(value charts.Cell, column charts.Column, timezone string) string {
	if value.Null {
		return ""
	}
	raw := value.Value
	f := column.Format
	formatted := raw
	if column.Type == "temporal" && f.DatePattern != "" {
		formatted = formatDate(raw, f.DatePattern, f.Locale, timezone)
	}
	switch f.Percent {
	case "fraction":
		if shifted, ok := shiftPercent(raw); ok {
			formatted = shifted + "%"
		} else {
			formatted = raw + " (fraction)"
		}
	case "whole":
		formatted += "%"
	default:
		if column.Type == "integer" || column.Type == "decimal" || column.Type == "number" {
			formatted = formatDecimal(raw, f.FractionDigits, f.Locale)
		}
	}
	parts := []string{formatted}
	if f.CurrencySymbol != "" {
		parts = append(parts, f.CurrencySymbol)
	} else if f.Currency != "" {
		parts = append(parts, f.Currency)
	}
	if f.Unit != "" {
		parts = append(parts, f.Unit)
	}
	return strings.Join(parts, " ")
}

func shiftPercent(raw string) (string, bool) {
	r, ok := new(big.Rat).SetString(raw)
	if !ok {
		return "", false
	}
	r.Mul(r, big.NewRat(100, 1))
	return strings.TrimRight(strings.TrimRight(r.FloatString(20), "0"), "."), true
}

func formatDecimal(raw string, digits int, locale string) string {
	r, ok := new(big.Rat).SetString(raw)
	if !ok || digits < 0 || digits > 20 {
		return raw
	}
	value := r.FloatString(digits)
	sign := ""
	if strings.HasPrefix(value, "-") || strings.HasPrefix(value, "+") {
		sign, value = value[:1], value[1:]
	}
	parts := strings.SplitN(value, ".", 2)
	whole := parts[0]
	group, decimal := ",", "."
	if strings.HasPrefix(strings.ToLower(locale), "es") {
		group, decimal = ".", ","
	}
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + group + whole[i:]
	}
	if len(parts) == 2 {
		return sign + whole + decimal + parts[1]
	}
	return sign + whole
}

func formatDate(raw, pattern, locale, timezone string) string {
	var parsed time.Time
	var ok bool
	instant := false
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04", "2006-01-02", "2006-01"} {
		if candidate, err := time.Parse(layout, raw); err == nil {
			parsed, ok = candidate, true
			instant = layout == time.RFC3339Nano
			break
		}
	}
	if !ok {
		return raw
	}
	// Offset-bearing timestamps are instants. Apply the report's sealed timezone
	// before extracting calendar fields. Date-only and naive date-times are wall
	// clock values and must not move across a date boundary.
	if instant {
		location, err := time.LoadLocation(timezone)
		if err != nil {
			return raw
		}
		parsed = parsed.In(location)
	}
	spanish := strings.HasPrefix(strings.ToLower(locale), "es")
	months := []string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}
	longMonths := []string{"January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"}
	if spanish {
		months = []string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}
		longMonths = []string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto", "septiembre", "octubre", "noviembre", "diciembre"}
	}
	switch pattern {
	case "year_month":
		if spanish {
			return parsed.Format("01/2006")
		}
		return parsed.Format("2006-01")
	case "date_short":
		if spanish {
			return parsed.Format("02/01/2006")
		}
		return parsed.Format("01/02/2006")
	default:
		selectedMonths := months
		if pattern == "date_long" {
			selectedMonths = longMonths
		}
		date := selectedMonths[int(parsed.Month())-1] + " " + parsed.Format("02, 2006")
		if spanish {
			date = parsed.Format("02") + " " + selectedMonths[int(parsed.Month())-1] + " " + parsed.Format("2006")
		}
		if pattern == "datetime_short" && len(raw) >= 16 {
			return date + " " + parsed.Format("15:04")
		}
		return date
	}
}
