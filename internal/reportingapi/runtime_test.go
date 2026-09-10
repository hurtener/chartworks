package reportingapi

import "testing"

func TestRuntimeRegistry(t *testing.T) {
	for _, execution := range []bool{false, true} {
		for _, planning := range []bool{false, true} {
			r, err := RuntimeRegistry(execution, planning)
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range r.Definitions() {
				if d.Response == nil || d.Method != "GET" && d.Request == nil {
					t.Fatal("missing wire schema", d.ID)
				}
			}
		}
	}
}
