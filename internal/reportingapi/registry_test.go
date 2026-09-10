package reportingapi

import "testing"

func TestReportingRegistryFeatureSets(t *testing.T) {
	for _, validation := range []bool{false, true} {
		for _, capture := range []bool{false, true} {
			for _, observe := range []bool{false, true} {
				registry, err := Registry(validation, capture, observe)
				if err != nil || registry == nil {
					t.Fatalf("features validate=%v capture=%v observe=%v: %v", validation, capture, observe, err)
				}
			}
		}
	}
}
