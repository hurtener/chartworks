package chartworks

import (
	"net/url"
	"strings"
	"testing"
)

// FuzzOperationCatalog verifies that hostile or version-skewed metadata can
// neither synthesize an external URL nor enable unclassified mutation replay.
func FuzzOperationCatalog(f *testing.F) {
	f.Add([]byte(`{"openapi":"3.1.1","paths":{}}`))
	f.Add([]byte(`{"openapi":"3.1.1","paths":{"/v1/read":{"get":{"operationId":"read","summary":"Synthetic read","x-chartworks-auth":"bearer","x-chartworks-action":"ops.read","x-chartworks-audience":"http","x-chartworks-effect":"read","x-chartworks-audit":"none","x-chartworks-resource-loader":"service.read","responses":{"200":{"content":{"application/json":{"schema":{"type":"object"}}}}}}}}}`))
	f.Add([]byte(`{"openapi":"3.1.1","paths":{"/v1/../outside":{},"/v1/../outside":{}}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			return
		}
		rows, err := ParseOperations(data)
		if err != nil {
			return
		}
		if len(rows) == 0 || len(rows) > 256 {
			t.Fatal("unbounded inventory")
		}
		seen := map[string]bool{}
		for _, row := range rows {
			if seen[row.ID] || !operationPath(row.Path) {
				t.Fatal("ambiguous route")
			}
			seen[row.ID] = true
			u, err := url.ParseRequestURI(strings.ReplaceAll(row.Path, "{id}", "synthetic"))
			if err != nil || u.IsAbs() || u.Host != "" || u.RawQuery != "" {
				t.Fatal("credential origin escape")
			}
			if row.Replay == "read" && row.Method != "GET" && row.Method != "HEAD" {
				t.Fatal("mutating read replay")
			}
			if row.Replay == "keyed" {
				proof := false
				for _, p := range row.Parameters {
					if p.In == "header" && strings.EqualFold(p.Name, "Idempotency-Key") && p.Required {
						proof = true
					}
				}
				if !proof || row.Public || row.Audience != "http" {
					t.Fatal("unproven mutation replay")
				}
			}
		}
	})
}
