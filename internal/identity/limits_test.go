package identity

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestExpandedExactScopeEnvelope(t *testing.T) {
	scopes := []string{"reporting.read", "mcp.use"}
	for i := 0; len(scopes) < MaxScopes; i++ {
		scopes = append(scopes, fmt.Sprintf("cw.block.read:chart-%03d", i))
	}
	e, err := FromVerified("tenant-a", "reader", "session-a", scopes, time.Now().Add(time.Minute), nil)
	if err != nil || len(e.Reach()) != MaxScopes-2 || !e.Has(scopes[len(scopes)-1]) {
		t.Fatal("exact reach was lost at the expanded ceiling", err)
	}
	scopes[2] = "cw.block.read:*"
	if e.Has(scopes[2]) || e.Has("reporting.execute") || e.Has("cw.block.read:ungranted") {
		t.Fatal("expanded count or caller mutation widened authority")
	}
	if _, err := FromVerified("tenant-a", "reader", "session-a", append(e.Scopes(), "extra"), e.Deadline(), nil); err == nil {
		t.Fatal("accepted one scope beyond the ceiling")
	}
	wide := make([]string, 64)
	for i := range wide {
		wide[i] = fmt.Sprintf("%03d", i) + strings.Repeat("x", MaxScopeBytes-3)
	}
	if _, err := FromVerified("tenant-a", "reader", "session-a", wide, e.Deadline(), nil); err != nil {
		t.Fatal("rejected exact aggregate byte ceiling", err)
	}
	if _, err := FromVerified("tenant-a", "reader", "session-a", append(wide, "x"), e.Deadline(), nil); err == nil {
		t.Fatal("accepted aggregate byte overflow")
	}
}
