package sources

import (
	"context"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"testing"
)

func TestStoredUniqueKeyPolicyNeverDowngradesProof(t *testing.T) {
	legacy := readexec.Binding{Relations: []readexec.Relation{{ID: "plain"}}}
	ctx := withStoredKeyPolicy(context.Background(), legacy)
	if v, _ := ctx.Value(legacyUniqueKeyPolicy{}).(bool); !v {
		t.Fatal("legacy policy missing")
	}
	proven := legacy.Clone()
	proven.Relations[0].UniqueKeys = [][]string{{"id"}}
	ctx = withStoredKeyPolicy(ctx, proven)
	if v, _ := ctx.Value(legacyUniqueKeyPolicy{}).(bool); v {
		t.Fatal("inherited context downgraded physical proof")
	}
	// New omitted metadata preserves the old JSON fingerprint byte-for-byte.
	before := struct {
		OID     int64
		Owner   int64
		ACL     string
		Schema  string
		Name    string
		Columns []columnEvidence
	}{1, 2, "acl", "analytics", "sales", []columnEvidence{{Name: "id"}}}
	after := tableEvidence{OID: 1, Owner: 2, ACL: "acl", Schema: "analytics", Name: "sales", Columns: []columnEvidence{{Name: "id"}}}
	if readexec.Hash(before) != readexec.Hash(after) {
		t.Fatal("legacy source fingerprint changed")
	}
	after.UniqueKeys = [][]string{{"id"}}
	if readexec.Hash(before) == readexec.Hash(after) {
		t.Fatal("new physical key missing from fingerprint")
	}
}
