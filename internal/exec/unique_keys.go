package exec

import "strings"

func cloneUniqueKeys(keys [][]string) [][]string {
	if keys == nil {
		return nil
	}
	out := make([][]string, len(keys))
	for i, key := range keys {
		out[i] = append([]string(nil), key...)
	}
	return out
}

func validUniqueKeys(keys [][]string, columns map[string]bool) bool {
	if len(keys) > 32 {
		return false
	}
	previous := ""
	for _, key := range keys {
		if len(key) == 0 || len(key) > 16 {
			return false
		}
		for i, col := range key {
			if !columns[col] || i > 0 && key[i-1] >= col {
				return false
			}
		}
		canonical := strings.Join(key, "\x00")
		if previous != "" && previous >= canonical {
			return false
		}
		previous = canonical
	}
	return true
}

func restrictUniqueKeys(keys [][]string, columns map[string]bool) [][]string {
	var out [][]string
	for _, key := range keys {
		complete := true
		for _, col := range key {
			complete = complete && columns[col]
		}
		if complete {
			out = append(out, append([]string(nil), key...))
		}
	}
	return out
}

// HasUniqueKey proves that equality on all supplied columns matches at most one
// visible row. Nullable unique keys suffice for ordinary SQL equality: NULL never
// matches NULL. It must not be used for null-safe equality or partial key matches.
func (r Relation) HasUniqueKey(columns []string) bool {
	provided := make(map[string]bool, len(columns))
	for _, col := range columns {
		if provided[col] {
			return false
		}
		provided[col] = true
	}
	for _, key := range r.UniqueKeys {
		complete := len(key) > 0
		for _, col := range key {
			complete = complete && provided[col]
		}
		if complete {
			return true
		}
	}
	return false
}
