package nlqexec

import (
	"encoding/json"
	"github.com/hurtener/chartworks/internal/exec"
	"math/big"
)

const AmountCompletenessPolicy = "proved-known-amount-result-v1"

// AmountCompleteness describes missing amounts inside the returned query
// population. It is distinct from transport truncation and never certifies the
// coverage of an upstream source outside the reviewed query contract.
type AmountCompleteness struct {
	Policy             string                  `json:"policy"`
	Metric             string                  `json:"metric"`
	ValueColumn        int                     `json:"value_column"`
	UnknownCountMetric string                  `json:"unknown_count_metric"`
	UnknownCountColumn int                     `json:"unknown_count_column"`
	Status             string                  `json:"status"`
	Scope              string                  `json:"scope"`
	Rows               []AmountCompletenessRow `json:"rows,omitempty"`
}
type AmountCompletenessRow struct {
	Row          int    `json:"row"`
	Status       string `json:"status"`
	UnknownCount string `json:"unknown_count,omitempty"`
}

func resultAmountCompleteness(receipt *exec.AnalyticalReceipt, result *exec.Result, success bool) []AmountCompleteness {
	if receipt == nil || receipt.Version != exec.AnalyticalScopedPopulationsVersion || receipt.Completeness == nil || receipt.Completeness.Policy != exec.AnalyticalCompletenessPolicy {
		return nil
	}
	columns := map[string]int{}
	for _, o := range receipt.Outputs {
		if _, exists := columns[o.Metric]; exists {
			return nil
		}
		columns[o.Metric] = o.Column
	}
	out := make([]AmountCompleteness, 0, len(receipt.Completeness.Obligations))
	for _, obligation := range receipt.Completeness.Obligations {
		item := AmountCompleteness{Policy: AmountCompletenessPolicy, Metric: obligation.Metric, ValueColumn: -1, UnknownCountMetric: obligation.UnknownCount, UnknownCountColumn: -1, Status: "unknown", Scope: "returned_query_rows"}
		value, valueOK := columns[obligation.Metric]
		count, countOK := columns[obligation.UnknownCount]
		if valueOK {
			item.ValueColumn = value
		}
		if countOK {
			item.UnknownCountColumn = count
		}
		if !success || result == nil || (result.Outcome != "succeeded" && result.Outcome != "empty") || result.Truncation != "" || !valueOK || !countOK || value < 0 || count < 0 || value >= len(result.Schema) || count >= len(result.Schema) {
			out = append(out, item)
			continue
		}
		if (result.Schema[count].Type != "integer" && result.Schema[count].Type != "decimal") || result.Outcome == "empty" && len(result.Rows) != 0 {
			out = append(out, item)
			continue
		}
		item.Status = "complete"
		for i, row := range result.Rows {
			entry := AmountCompletenessRow{Row: i, Status: "unknown"}
			if value < len(row) && count < len(row) {
				if n, ok := exactUnknownCount(row[count]); ok {
					entry.UnknownCount = n.String()
					entry.Status = "complete"
					if n.Sign() > 0 {
						entry.Status = "incomplete"
					}
				}
			}
			if entry.Status == "unknown" {
				item.Status = "unknown"
			} else if entry.Status == "incomplete" && item.Status != "unknown" {
				item.Status = "incomplete"
			}
			item.Rows = append(item.Rows, entry)
		}
		out = append(out, item)
	}
	return out
}

func exactUnknownCount(raw json.RawMessage) (*big.Int, bool) {
	var value string
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &value) != nil {
			return nil, false
		}
	} else {
		value = string(raw)
	}
	if !exec.Decimal(value) {
		return nil, false
	}
	n, ok := new(big.Rat).SetString(value)
	if !ok || n.Sign() < 0 || !n.IsInt() {
		return nil, false
	}
	return new(big.Int).Set(n.Num()), true
}
