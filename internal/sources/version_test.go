package sources

import "testing"

func TestQualifiedPostgresMajor(t *testing.T) {
	for _, version := range []int{0, 160000, 169999, 180000, 190001} {
		if supportedPostgresVersion(version) {
			t.Fatalf("unqualified major accepted: %d", version)
		}
	}
	for _, version := range []int{170000, 170010, 179999} {
		if !supportedPostgresVersion(version) {
			t.Fatalf("qualified major rejected: %d", version)
		}
	}
}
