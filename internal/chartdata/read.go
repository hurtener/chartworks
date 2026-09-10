// Package chartdata losslessly adapts common read results to caller-owned chart data.
// It infers no aggregation, units, semantic approval, source scope or authorization.
package chartdata

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/exec"
)

// ErrInvalid reports a read result or conversion request that violates the chart data contract.
var ErrInvalid = errors.New("chartworks: invalid chart result")

// FromReadResult preserves exact scalar encodings and rejects invalid wire data.
func FromReadResult(ctx context.Context, result exec.Result, limits charts.Limits) (charts.Data, error) {
	if ctx == nil || limits.Validate() != nil || len(result.Schema) == 0 || len(result.Schema) > limits.MaxColumns || len(result.Rows) > limits.MaxRows {
		return charts.Data{}, ErrInvalid
	}
	status := "complete_result"
	switch result.Outcome {
	case "empty":
		if len(result.Rows) != 0 {
			return charts.Data{}, ErrInvalid
		}
	case "succeeded":
		if len(result.Rows) == 0 {
			return charts.Data{}, ErrInvalid
		}
	case "truncated":
		status = "truncated"
	default:
		return charts.Data{}, ErrInvalid
	}
	out := charts.Data{Version: 1, Columns: make([]charts.Column, len(result.Schema)), Rows: make([][]charts.Cell, 0, len(result.Rows)), Completeness: charts.Completeness{Status: status, Reason: result.Truncation}}
	for i, field := range result.Schema {
		encoding := "string"
		role := "unknown"
		switch field.Type {
		case "boolean", "number":
			encoding = field.Type
		case "integer", "decimal", "binary", "structured":
		case "text":
			role = "dimension"
		case "temporal":
			role = "time"
		default:
			return charts.Data{}, ErrInvalid
		}
		if field.Encoding != encoding {
			return charts.Data{}, ErrInvalid
		}
		out.Columns[i] = charts.Column{ID: "c" + strconv.Itoa(i), Name: field.Name, Type: field.Type, Role: role, Provenance: charts.Provenance{Version: 1}}
	}
	total := 0
	for _, row := range result.Rows {
		if err := ctx.Err(); err != nil {
			return charts.Data{}, err
		}
		if len(row) != len(result.Schema) {
			return charts.Data{}, ErrInvalid
		}
		values := make([]charts.Cell, len(row))
		for i, raw := range row {
			total += len(raw)
			if len(raw) > limits.MaxCellBytes*6+2 || total > limits.MaxBytes {
				return charts.Data{}, ErrInvalid
			}
			raw = bytes.TrimSpace(raw)
			if bytes.Equal(raw, []byte("null")) {
				values[i].Null = true
				continue
			}
			switch result.Schema[i].Encoding {
			case "string":
				if json.Unmarshal(raw, &values[i].Value) != nil {
					return charts.Data{}, ErrInvalid
				}
			case "boolean":
				if !bytes.Equal(raw, []byte("true")) && !bytes.Equal(raw, []byte("false")) {
					return charts.Data{}, ErrInvalid
				}
				values[i].Value = string(raw)
			case "number":
				// json.Valid alone would accept a quoted number or boolean. Decode through
				// UseNumber and require that exact token kind, never through float64.
				decoder := json.NewDecoder(bytes.NewReader(raw))
				decoder.UseNumber()
				var v any
				if decoder.Decode(&v) != nil || !json.Valid(raw) {
					return charts.Data{}, ErrInvalid
				}
				n, ok := v.(json.Number)
				if !ok {
					return charts.Data{}, ErrInvalid
				}
				values[i].Value = n.String()
			}
		}
		out.Rows = append(out.Rows, values)
	}
	if err := charts.ValidateData(ctx, out, limits); err != nil {
		return charts.Data{}, err
	}
	return out, nil
}
