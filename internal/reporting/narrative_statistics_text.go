package reporting

import (
	"fmt"
	"math/big"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
)

func validStatisticalEvidence(e NarrativeEvidence, spec NarrativeStatistic, n Narrative) bool {
	d := e.Statistic
	if d == nil || e.Field != spec.Value.Field.Name || e.Type != "statistic" || e.Value != "" || e.Row != -1 ||
		d.ID != spec.ID || d.Kind != spec.Kind || d.Value != spec.Value || digest(d.Time) != digest(spec.Time) || !identity.Identifier(d.Output) || !hashValid(d.Provenance) {
		return false
	}
	p := d.Population
	if p.Scope != "retained_row_prefix" || p.RetainedRows < 1 || p.RetainedRows > 100000 || p.ConsideredRows != min(n.MaxRows, p.RetainedRows) ||
		p.IncludedRows != len(p.SourceRows) || p.IncludedRows < 1 || len(p.NullRows)+p.IncludedRows != p.ConsideredRows ||
		p.RowsReduced != (p.RetainedRows > p.ConsideredRows) || !slices.Contains([]string{"succeeded", "truncated"}, p.QueryOutcome) || !text(p.Truncation, 128) {
		return false
	}
	seen := map[int]bool{}
	for _, rows := range [][]int{p.SourceRows, p.NullRows} {
		for _, row := range rows {
			if row < 0 || row >= p.ConsideredRows || seen[row] {
				return false
			}
			seen[row] = true
		}
	}
	if !slices.IsSorted(p.NullRows) || spec.Time == nil && !slices.IsSorted(p.SourceRows) {
		return false
	}
	if a := d.Amount; a != nil {
		if !identity.Identifier(a.Declaration) || a.Label == "" || !text(a.Label, 256) || a.Scope != p.Scope || !slices.Contains([]string{"complete", "incomplete", "unknown"}, a.Status) ||
			!statisticalValueRef(a.Companion) || a.Companion.Field.Type == "number" || a.Companion.Column == spec.Value.Column ||
			!slices.Contains(n.Fields, a.Companion.Field.Name) || slices.Contains(n.RedactedFields, a.Companion.Field.Name) ||
			(p.QueryOutcome == "truncated" || p.Truncation != "") && a.Status != "unknown" {
			return false
		}
	}
	switch d.Kind {
	case "trend":
		t := d.Trend
		if t == nil || d.Extrema != nil || d.Variance != nil || p.IncludedRows < 2 || t.FirstRow != p.SourceRows[0] || t.LastRow != p.SourceRows[len(p.SourceRows)-1] ||
			!slices.Contains([]string{"increasing", "decreasing", "constant", "nondecreasing", "nonincreasing", "mixed"}, t.Sequence) {
			return false
		}
		first, firstScale, firstOK := narrativeNumber(t.FirstValue)
		last, lastScale, lastOK := narrativeNumber(t.LastValue)
		firstTime, firstTimeOK := statisticalTime(t.FirstTime, d.Time.Meaning)
		lastTime, lastTimeOK := statisticalTime(t.LastTime, d.Time.Meaning)
		if !firstOK || !lastOK || !firstTimeOK || !lastTimeOK || !firstTime.Before(lastTime) {
			return false
		}
		delta := new(big.Rat).Sub(last, first)
		if len(t.Difference) > 4096 || t.Difference != delta.FloatString(max(firstScale, lastScale)) {
			return false
		}
		if t.Sequence == "constant" && delta.Sign() != 0 || slices.Contains([]string{"increasing", "nondecreasing"}, t.Sequence) && delta.Sign() <= 0 || slices.Contains([]string{"decreasing", "nonincreasing"}, t.Sequence) && delta.Sign() >= 0 {
			return false
		}
	case "extrema":
		x := d.Extrema
		if x == nil || d.Trend != nil || d.Variance != nil || x.MinimumTies < 1 || x.MaximumTies < 1 || x.MinimumTies > p.IncludedRows || x.MaximumTies > p.IncludedRows ||
			!slices.Contains(p.SourceRows, x.MinimumRow) || !slices.Contains(p.SourceRows, x.MaximumRow) {
			return false
		}
		minimum, _, minOK := narrativeNumber(x.Minimum)
		maximum, _, maxOK := narrativeNumber(x.Maximum)
		if !minOK || !maxOK || minimum.Cmp(maximum) > 0 {
			return false
		}
		if minimum.Cmp(maximum) == 0 {
			if x.MinimumTies != p.IncludedRows || x.MaximumTies != p.IncludedRows || x.MinimumRow != slices.Min(p.SourceRows) || x.MaximumRow != x.MinimumRow {
				return false
			}
		} else if x.MinimumRow == x.MaximumRow || x.MinimumTies+x.MaximumTies > p.IncludedRows {
			return false
		}
	case "population_variance":
		v := d.Variance
		if v == nil || d.Trend != nil || d.Extrema != nil || p.IncludedRows < 2 || v.Divisor != p.IncludedRows {
			return false
		}
		numerator, ok := statisticalInteger(v.Numerator)
		denominator, denominatorOK := statisticalInteger(v.Denominator)
		if !ok || !denominatorOK || denominator.Sign() <= 0 || new(big.Int).GCD(nil, nil, numerator, denominator).Cmp(big.NewInt(1)) != 0 {
			return false
		}
	default:
		return false
	}
	return true
}

func statisticalInteger(value string) (*big.Int, bool) {
	if len(value) < 1 || len(value) > 4096 {
		return nil, false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return nil, false
		}
	}
	n, ok := new(big.Int).SetString(value, 10)
	return n, ok && n.String() == value
}

func groundedStatisticalText(answer NarrativeAnswer, evidence []NarrativeEvidence, n Narrative) (string, error) {
	if boundedNarrativePolicy(n) != nil {
		return "", ErrNarrativePolicy
	}
	if len(answer.Claims) < 1 || len(answer.Claims) > n.MaxClaims || len(evidence) != len(n.Statistics) {
		return "", gateway.ErrOutput
	}
	byID := map[string]NarrativeEvidence{}
	output := ""
	for i, e := range evidence {
		if e.ID != fmt.Sprintf("e%d", i+1) || !validStatisticalEvidence(e, n.Statistics[i], n) {
			return "", gateway.ErrOutput
		}
		if output != "" && output != e.Statistic.Output {
			return "", gateway.ErrOutput
		}
		output = e.Statistic.Output
		byID[e.ID] = e
	}
	spanish := n.Locale == "es" || strings.HasPrefix(n.Locale, "es-")
	seen := map[string]bool{}
	lines := make([]string, 0, len(answer.Claims))
	for _, claim := range answer.Claims {
		if len(claim.Evidence) != 1 || seen[claim.Evidence[0]] {
			return "", gateway.ErrOutput
		}
		e, ok := byID[claim.Evidence[0]]
		if !ok || claim.Kind != e.Statistic.Kind {
			return "", gateway.ErrOutput
		}
		seen[e.ID] = true
		line := statisticalLine(e, spanish)
		if n.Tone == "technical" {
			if spanish {
				line = "Estadística retenida: " + line
			} else {
				line = "Retained statistic: " + line
			}
		} else if n.Tone == "neutral" {
			if spanish {
				line = "Observación: " + line
			} else {
				line = "Observation: " + line
			}
		}
		lines = append(lines, line)
	}
	if spanish {
		lines = append(lines, "La evidencia es una observación retenida y acotada, no un total completo ni una consulta en vivo.")
	} else {
		lines = append(lines, "Evidence is a bounded retained observation, not a live or full-source total.")
	}
	result := strings.Join(lines, "\n")
	if utf8.RuneCountInString(result) > n.MaxCharacters {
		return "", ErrBudget
	}
	return result, nil
}

func statisticalLine(e NarrativeEvidence, spanish bool) string {
	d, line := e.Statistic, ""
	switch d.Kind {
	case "trend":
		t := d.Trend
		sequence := t.Sequence
		if spanish {
			sequence = map[string]string{"increasing": "creciente", "decreasing": "decreciente", "constant": "constante", "nondecreasing": "no decreciente", "nonincreasing": "no creciente", "mixed": "mixta"}[sequence]
			line = fmt.Sprintf("Cambio entre extremos de %q: %s en %q (fila %d) a %s en %q (fila %d); diferencia %s; secuencia %s.", e.Field, t.FirstValue, t.FirstTime, t.FirstRow, t.LastValue, t.LastTime, t.LastRow, t.Difference, sequence)
		} else {
			line = fmt.Sprintf("Endpoint change for %q: %s at %q (row %d) to %s at %q (row %d); difference %s; sequence %s.", e.Field, t.FirstValue, t.FirstTime, t.FirstRow, t.LastValue, t.LastTime, t.LastRow, t.Difference, sequence)
		}
	case "extrema":
		x := d.Extrema
		if spanish {
			line = fmt.Sprintf("Extremos de %q: mínimo %s (fila %d, %d observaciones empatadas); máximo %s (fila %d, %d observaciones empatadas).", e.Field, x.Minimum, x.MinimumRow, x.MinimumTies, x.Maximum, x.MaximumRow, x.MaximumTies)
		} else {
			line = fmt.Sprintf("Extrema for %q: minimum %s (row %d, %d tied observations); maximum %s (row %d, %d tied observations).", e.Field, x.Minimum, x.MinimumRow, x.MinimumTies, x.Maximum, x.MaximumRow, x.MaximumTies)
		}
	case "population_variance":
		v := d.Variance
		value := v.Numerator
		if v.Denominator != "1" {
			value += "/" + v.Denominator
		}
		if spanish {
			line = fmt.Sprintf("Varianza poblacional de %q: %s exacta, con divisor N=%d (unidades del valor al cuadrado).", e.Field, value, v.Divisor)
		} else {
			line = fmt.Sprintf("Population variance for %q: exactly %s, using divisor N=%d (squared value units).", e.Field, value, v.Divisor)
		}
	}
	p := d.Population
	if spanish {
		line += fmt.Sprintf(" Alcance: primeras %d de %d filas retenidas; %d observaciones numéricas; %d valores NULL excluidos, sin sustituirlos por cero.", p.ConsideredRows, p.RetainedRows, p.IncludedRows, len(p.NullRows))
	} else {
		line += fmt.Sprintf(" Scope: first %d of %d retained rows; %d numeric observations; %d NULL values excluded, without replacing them with zero.", p.ConsideredRows, p.RetainedRows, p.IncludedRows, len(p.NullRows))
	}
	if p.QueryOutcome == "truncated" || p.Truncation != "" {
		if spanish {
			line += " El resultado de origen está truncado."
		} else {
			line += " The source result is truncated."
		}
	}
	if p.RowsReduced {
		if spanish {
			line += " Se aplicó el límite de filas de la narrativa."
		} else {
			line += " The narrative row limit reduced the retained result."
		}
	}
	if a := d.Amount; a != nil {
		if spanish {
			status := map[string]string{"complete": "completa", "incomplete": "incompleta", "unknown": "desconocida"}[a.Status]
			line += fmt.Sprintf(" Importe conocido %q: integridad %s en estas filas.", a.Label, status)
		} else {
			line += fmt.Sprintf(" Known amount %q: completeness %s within these rows.", a.Label, a.Status)
		}
	}
	return line + fmt.Sprintf(" [%s].", e.ID)
}
