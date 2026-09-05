package identity

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestEnvelopeConstruction(t *testing.T) {
	now := time.Now()
	future := now.Add(time.Minute)
	for _, args := range [][3]string{{"", "u", "s"}, {"t", "", "s"}, {"t", "u", ""}, {"t", "Svc:x", "s"}, {"t", "svc:", "s"}} {
		if _, err := FromVerified(args[0], args[1], args[2], nil, future, nil); err == nil {
			t.Fatal("invalid verified coordinates")
		}
	}
	if _, err := FromVerified("t", "u", "s", nil, time.Time{}, nil); err == nil {
		t.Fatal("zero deadline")
	}
	if _, err := FromVerified("t", "u", "s", nil, now.Add(-time.Second), nil); err == nil {
		t.Fatal("expired envelope")
	}
	for _, scopes := range [][]string{{"a", "a"}, {strings.Repeat("x", 257)}, {"cw.x.read:x"}, {"bad\nscope"}, make([]string, 33)} {
		if _, err := FromVerified("t", "u", "s", scopes, future, nil); err == nil {
			t.Fatal("bad scopes")
		}
	}
	e, err := FromVerified("t", "u", "s", []string{"cw.topic.read:x", "query.plan"}, future, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := e.Context(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	copyE, err := FromContext(ctx)
	if err != nil || copyE.Tenant() != "t" || copyE.User() != "u" || copyE.Session() != "s" || copyE.Deadline() != future {
		t.Fatal("identity context changed")
	}
	for _, s := range []string{"", strings.Repeat("a", 129), "a/b", "x%2fy", "a b"} {
		if Identifier(s) {
			t.Fatal("unsafe identifier")
		}
	}
	for _, s := range []string{"cw.source.read:x", "cw.execution_context.use:x:v1", "cw.tenant.read:*"} {
		if _, err := ParseReach(s); err != nil {
			t.Fatal(err)
		}
	}
}
