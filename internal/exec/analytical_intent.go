package exec

import "strconv"

// AnalyticalIntentVersion retains the interpretation and conformance policy.
const AnalyticalIntentVersion = "analytical-metrics-v6"
const AnalyticalIntentPolicy = "reviewed-order-limit-v1"

// AnalyticalOrder names a selected metric or a reviewed physical grouping field.
// SQL output names are never authority. Nulls is explicitly first or last.
type AnalyticalOrder struct {
	Bucket     *AnalyticalBucket `json:"bucket,omitempty"`
	Metric     string            `json:"metric,omitempty"`
	Column     string            `json:"column,omitempty"`
	Descending bool              `json:"descending,omitempty"`
	Nulls      string            `json:"nulls"`
}

type AnalyticalIntent struct {
	Policy string            `json:"policy"`
	Order  []AnalyticalOrder `json:"order"`
	Limit  int               `json:"limit"`
}

func validateAnalyticalIntent(c AnalyticalContract) error {
	if c.Intent == nil {
		return nil
	}
	p := c.Intent
	if c.Version != AnalyticalIntentVersion || p.Policy != AnalyticalIntentPolicy || len(p.Order) > 16 || p.Limit < 0 || p.Limit > 100000 {
		return ErrBinding
	}
	seen := map[string]bool{}
	for _, o := range p.Order {
		targets := 0
		if o.Metric != "" {
			targets++
		}
		if o.Column != "" {
			targets++
		}
		if o.Bucket != nil {
			targets++
		}
		if targets != 1 || o.Nulls != "first" && o.Nulls != "last" {
			return ErrBinding
		}
		key := Hash([]any{o.Metric, o.Column, o.Bucket})
		if seen[key] {
			return ErrBinding
		}
		seen[key] = true
		found := false
		if o.Metric != "" {
			for _, m := range c.Metrics {
				if m.ID == o.Metric {
					found = true
				}
			}
		} else if o.Bucket != nil {
			if c.Grain != nil {
				for _, bucket := range c.Grain.Buckets {
					if Hash(bucket) == Hash(*o.Bucket) {
						found = true
					}
				}
			}
		} else {
			found = o.Column != ""
		}
		if !found {
			return ErrBinding
		}
	}
	return nil
}

func (a *analyticalChecker) checkIntent(q map[string]any, terms []analyticalTerm, aliases map[string]analyticalTerm) error {
	if a.intent == nil {
		return nil
	}
	failOrder := func() error { return analyticalFailure("analytical_order_mismatch", false) }
	sorts := array(q["sortClause"])
	if a.intent.Order != nil && len(sorts) != len(a.intent.Order) {
		return failOrder()
	}
	for i := range a.intent.Order {
		node := sorts[i]
		s := fieldObject(node, "SortBy")
		if !only(s, "node", "sortby_dir", "sortby_nulls", "location") {
			return failOrder()
		}
		direction := text(s["sortby_dir"])
		if direction != "SORTBY_DEFAULT" && direction != "SORTBY_ASC" && direction != "SORTBY_DESC" {
			return failOrder()
		}
		want := a.intent.Order[i]
		desc := direction == "SORTBY_DESC"
		if desc != want.Descending {
			return failOrder()
		}
		nulls := text(s["sortby_nulls"])
		switch nulls {
		case "SORTBY_NULLS_DEFAULT", "":
			if desc {
				nulls = "first"
			} else {
				nulls = "last"
			}
		case "SORTBY_NULLS_FIRST":
			nulls = "first"
		case "SORTBY_NULLS_LAST":
			nulls = "last"
		default:
			return failOrder()
		}
		if nulls != want.Nulls {
			return failOrder()
		}
		var term analyticalTerm
		// ORDER BY resolves bare output aliases before input column names.
		if n, ok := analyticalIntegerConstant(s["node"]); ok {
			if n < 1 || n > len(terms) {
				return failOrder()
			}
			term = terms[n-1]
		} else {
			parts, valid := names(fieldObject(s["node"], "ColumnRef")["fields"])
			var found bool
			if valid && len(parts) == 1 {
				term, found = aliases[parts[0]]
			}
			if !found {
				var err error
				term, err = a.term(s["node"], 0)
				if err != nil {
					return err
				}
			}
		}
		if term.guarded {
			return failOrder()
		}
		if want.Bucket != nil {
			if term.bucket != analyticalBucketKey(*want.Bucket) || term.aggregate {
				return failOrder()
			}
		} else if want.Column != "" {
			if term.column != want.Column || term.aggregate {
				return failOrder()
			}
		} else if term.key != a.intentMetricKeys[want.Metric] || !term.aggregate {
			return failOrder()
		}
	}
	failLimit := func() error { return analyticalFailure("analytical_limit_mismatch", false) }
	if q["limitOffset"] != nil {
		n, ok := analyticalIntegerConstant(q["limitOffset"])
		if !ok || n != 0 {
			return failLimit()
		}
	}
	if option := text(q["limitOption"]); option != "" && option != "LIMIT_OPTION_DEFAULT" && option != "LIMIT_OPTION_COUNT" {
		return failLimit()
	}
	if a.intent.Limit == 0 {
		if q["limitCount"] != nil {
			return failLimit()
		}
		return nil
	}
	n, ok := analyticalIntegerConstant(q["limitCount"])
	if !ok {
		ref := fieldObject(q["limitCount"], "ParamRef")
		index, _ := ref["number"].(float64)
		if index < 1 || index > float64(len(a.parameters)) || index != float64(int(index)) {
			return failLimit()
		}
		p := a.parameters[int(index)-1]
		if p.Kind != "integer" {
			return failLimit()
		}
		var err error
		n, err = strconv.Atoi(p.Value)
		ok = err == nil
	}
	if !ok || n != a.intent.Limit {
		return failLimit()
	}
	return nil
}
