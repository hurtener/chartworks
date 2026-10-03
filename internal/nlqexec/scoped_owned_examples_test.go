package nlqexec

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
)

func TestScopedLearningPolicyCustody(t *testing.T) {
	policies := []string{OwnedExamplePolicy, ScopedScalarExamplePolicy, ScopedGroupedExamplePolicy, ScopedSelectionExamplePolicy, ScopedGroupedFactExamplePolicy}
	seen := map[string]bool{}
	for i, policy := range policies {
		if learningPolicyForBinding(i+1) != policy || !ownedLearningPolicy(policy) || !validExampleBindingPolicy(policy) || seen[policy] {
			t.Fatal("binding family was lost")
		}
		seen[policy] = true
		x := ownedExampleFixture(t, false)
		x.Origin.BindingPolicy = policy
		x.Digest = originExampleDigest(x.Topic, x.Question, x.SQL, x.ParameterSchema, x.Origin)
		if !ExampleParametersValid(x) || ownedExampleInstruction(x) == "" {
			t.Fatal("scoped base cannot travel through existing reviewed example surface")
		}
		row := PortableExample{SchemaVersion: portableExampleVersion(x.ParameterSchema, policy), Question: x.Question, SQL: x.SQL, Digest: x.Digest, Origin: x.Origin}
		if row.SchemaVersion != 3 || !portableExampleValid(row) {
			t.Fatal("owned policy lost in portable schema")
		}
		for _, other := range policies {
			if other == policy {
				continue
			}
			bad := x
			bad.Origin.BindingPolicy = other
			if ExampleParametersValid(bad) {
				t.Fatal("binder family substitution preserved digest")
			}
		}
		row.SchemaVersion = 1
		if portableExampleValid(row) {
			t.Fatal("scoped policy downgraded to legacy portability")
		}
	}
	if learningPolicyForBinding(0) != "" || learningPolicyForBinding(6) != "" || validExampleBindingPolicy("unknown-scoped-policy") {
		t.Fatal("unknown binder accepted")
	}
}

func TestScopedLearningCannotAcquireAuthorityFromJSON(t *testing.T) {
	for _, policy := range []string{ScopedScalarExamplePolicy, ScopedGroupedExamplePolicy, ScopedSelectionExamplePolicy, ScopedGroupedFactExamplePolicy} {
		a := groupedPeriodAdmission(t, false)
		// Even genuine historical applications lose their private seal on wire
		// round-trip. A saved policy must not restore current predicates.
		raw, err := json.Marshal(a.route)
		if err != nil {
			t.Fatal(err)
		}
		if err = json.Unmarshal(raw, &a.route); err != nil {
			t.Fatal(err)
		}
		a.metricPeriods = nil
		if ownedExampleApplicable(ExampleRecord{Origin: ExampleOrigin{BindingPolicy: policy}}, a) || scopedExampleApplicable(policy, admission{}) {
			t.Fatal("retained origin/JSON authorized scoped predicates")
		}
	}
	if scopedExampleApplicable("unknown", admission{analytical: &exec.AnalyticalContract{Version: exec.AnalyticalGroupedSelectionVersion}}) {
		t.Fatal("supplied compiled contract bypassed fresh route")
	}
}

func TestScopedLearningNullKeepsDisclosureScanner(t *testing.T) {
	q := QueryRecord{Route: nlqroute.RouteResult{Resolutions: []semantics.ClarificationResolution{{Null: true, Sensitivity: semantics.LiteralSensitive}}}}
	values, complete, err := ownedLearningPrivateValues(t.Context(), q)
	if err != nil || !complete || len(values) != 1 || values[0].Kind != "null" || values[0].Value != "" {
		t.Fatal("resolved NULL lost", err)
	}
	if err := exec.CheckLearningParameterContent(t.Context(), "SELECT amount FROM analytics.sales", values); err != nil {
		t.Fatal("NULL-only scan", err)
	}
	if err := exec.CheckLearningParameterContent(t.Context(), "SELECT amount /* unsupported annotation */ FROM analytics.sales", values); !errors.Is(err, exec.ErrUnsupported) {
		t.Fatal("NULL-only path bypassed syntax guard", err)
	}
}

func TestGroupedFactLearningHasDistinctOwnedBasePolicy(t *testing.T) {
	const want = "current-owned-grouped-fact-predicates-v1"
	if got := learningPolicyForBinding(5); got != want {
		t.Fatalf("authenticated schema-5 base has no distinct learning policy: got %q want %q", got, want)
	}
	if !scopedLearningPolicy(want) || !ownedLearningPolicy(want) || !validExampleBindingPolicy(want) {
		t.Fatal("schema-5 base rejected by closed policy registry")
	}
	if learningPolicyForBinding(6) != "" {
		t.Fatal("schema-6 scalar entailment was enabled")
	}
}
