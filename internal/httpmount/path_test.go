package httpmount

import (
	"strings"
	"testing"
)

func TestCanonicalMount(t *testing.T) {
	for _, path := range []string{"/", "/api", "/api/v1", "/a_B-1.2:x", "/" + strings.Repeat("a", 63)} {
		if !Valid(path) {
			t.Fatalf("canonical mount rejected: %q", path)
		}
	}
	for _, path := range []string{"", "api", "/a/", "//a", "/a//b", "/.", "/..", "/a/../b", "/a/./b", "/%61", "/a?b", "/a#b", "/a\\b", "/a\n", "/é", "/" + strings.Repeat("a", 64)} {
		if Valid(path) {
			t.Fatalf("ambiguous mount accepted: %q", path)
		}
	}
}
func FuzzCanonicalMount(f *testing.F) {
	for _, path := range []string{"/", "/api/v1", "/../", "/%2f"} {
		f.Add(path)
	}
	f.Fuzz(func(t *testing.T, path string) {
		if !Valid(path) {
			return
		}
		if len(path) > 64 || !strings.HasPrefix(path, "/") || strings.ContainsAny(path, "%?#\\") {
			t.Fatal("noncanonical mount")
		}
	})
}
