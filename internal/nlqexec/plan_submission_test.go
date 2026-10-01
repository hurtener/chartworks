package nlqexec

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanSubmissionPreservesOriginalIdentity(t *testing.T) {
	q := QuestionRequest{Question: "monthly known revenue", Context: "ctx"}
	r := QueryRecord{SQL: "SELECT 1", Operation: "run-new", PlanOperation: "plan-original", PlanRequestDigest: planSubmissionDigest(q)}
	if !PlanSubmissionValid(r) {
		t.Fatal("valid original identity rejected")
	}
	before := QueryLineageDigest(r)
	r.PlanRequestDigest = strings.Repeat("b", 64)
	if QueryLineageDigest(r) == before {
		t.Fatal("private submission not bound to lineage")
	}
	for _, bad := range []QueryRecord{{PlanOperation: "x"}, {PlanRequestDigest: strings.Repeat("a", 64)}, {SQL: "SELECT 1", PlanOperation: "resume:x", PlanRequestDigest: strings.Repeat("a", 64)}} {
		if PlanSubmissionValid(bad) {
			t.Fatal("malformed identity accepted")
		}
	}
	if !PlanSubmissionValid(QueryRecord{}) {
		t.Fatal("legacy row rejected")
	}
	raw, _ := json.Marshal(r)
	if strings.Contains(string(raw), "plan-original") || strings.Contains(string(raw), strings.Repeat("b", 64)) {
		t.Fatal("private plan identity exposed")
	}
	altered := q
	altered.Question = "another question"
	if planSubmissionDigest(q) == planSubmissionDigest(altered) {
		t.Fatal("changed request accepted")
	}
}
