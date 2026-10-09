package reporting

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAuthoringOptionExactBlockPins(t *testing.T) {
	a := AuthoringOptionBlock{Block: "a", Revision: 1, Digest: strings.Repeat("1", 64)}
	b := AuthoringOptionBlock{Block: "a", Revision: 2, Digest: strings.Repeat("2", 64)}
	in := []AuthoringOptionBlock{a, b, a}
	out, err := canonicalAuthoringOptionBlocks(in)
	if err != nil || !reflect.DeepEqual(out, []AuthoringOptionBlock{a, b}) || len(in) != 3 {
		t.Fatal("mixed-revision compaction", out, err)
	}
	bad := a
	bad.Digest = strings.Repeat("3", 64)
	if _, err := canonicalAuthoringOptionBlocks([]AuthoringOptionBlock{a, b, bad}); err == nil {
		t.Fatal("conflicting immutable pin accepted")
	}
}

func TestAuthoringOptionOperationAge(t *testing.T) {
	now := time.Date(2026, 10, 4, 6, 0, 0, 0, time.UTC)
	key := func(at time.Time) string {
		return "option:" + strconv.FormatInt(at.Unix(), 10) + ":" + strings.Repeat("a", 32)
	}
	for _, delta := range []time.Duration{0, -5 * time.Minute, 30 * time.Second} {
		if !AuthoringOptionOperationValid(key(now.Add(delta)), now, true) {
			t.Fatal(delta)
		}
	}
	for _, operation := range []string{key(now.Add(-5*time.Minute - time.Second)), key(now.Add(31 * time.Second)), "option:0:" + strings.Repeat("a", 32), "option:01:" + strings.Repeat("a", 32), "option:+1:" + strings.Repeat("a", 32), "option:" + strconv.FormatInt(now.Unix(), 10) + ":" + strings.Repeat("A", 32)} {
		if AuthoringOptionOperationValid(operation, now, true) {
			t.Fatal("bad operation accepted", operation)
		}
	}
	if !AuthoringOptionOperationValid(key(now.Add(-48*time.Hour)), now, false) {
		t.Fatal("old status key unavailable")
	}
}

func TestAuthoringOptionNullAndCursorSQL(t *testing.T) {
	_, _, _, binding := preparationCompileFixture()
	relation := binding.Relations[0]
	column := relation.Columns[0]
	for _, search := range []bool{false, true} {
		for _, cursor := range []bool{false, true} {
			statement, err := filterOptionStatementNullPolicy(binding, relation, column, search, cursor, 201, true)
			if err != nil || !strings.Contains(statement, " IS NOT NULL") || strings.Contains(statement, " OR ") || !strings.Contains(statement, "LIMIT 201") {
				t.Fatal(statement, err)
			}
		}
	}
	legacy, err := filterOptionStatement(binding, relation, column, false, true, 20)
	if err != nil || !strings.Contains(legacy, " IS NULL") || strings.Contains(legacy, " IS NOT NULL") {
		t.Fatal("legacy contract changed", legacy, err)
	}
}

func TestAuthoringOptionLostValuesAndNamespace(t *testing.T) {
	r := AuthoringOptionRecord{Status: "completed", ExecutionStatus: "empty", RemoteState: "stopped", Operation: "operation"}
	view := authoringOptionView(r)
	if view.ValuesAvailable || view.Complete || !view.NewOperationAllowed || view.Code != "result_not_retained" || len(view.Options) != 0 {
		t.Fatal("lost empty response confused with no matches", view)
	}
	r.Status = "accepted"
	r.Deadline = time.Now().Add(-time.Second)
	r.RemoteState = "not_issued"
	view = authoringOptionView(r)
	if view.Status != "uncertain" || view.NewOperationAllowed {
		t.Fatal("missing journal is not proof of termination", view)
	}
	target := AuthoringOptionTarget{Dataset: &AuthoringDatasetOptionTarget{NewBlock: "chart", Dataset: "one", Dimension: "region"}}
	namespace := AuthoringOptionNamespace(target)
	target.Dataset.Dimension = "other"
	target.Dataset.Dataset = "two"
	if namespace != AuthoringOptionNamespace(target) {
		t.Fatal("changed field bypassed target liability")
	}
	target = AuthoringOptionTarget{Report: &AuthoringReportOptionTarget{Report: "report", Page: "analysis", Filter: "region", Revision: 1, Policy: "private_preview"}}
	namespace = AuthoringOptionNamespace(target)
	target.Report.Revision = 2
	target.Report.Policy = "published"
	if namespace != AuthoringOptionNamespace(target) {
		t.Fatal("changed revision/policy bypassed target liability")
	}
}

func TestAuthoringOptionCustodyContainsNoQueryInputs(t *testing.T) {
	raw, err := json.Marshal(AuthoringOptionRecord{})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`"sql"`, `"search"`, `"cursor"`, `"options"`, `"token"`, `"parameters"`} {
		if strings.Contains(string(raw), field) {
			t.Fatal("protected operation stores query content", field)
		}
	}
}
