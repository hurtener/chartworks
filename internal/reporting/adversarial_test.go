package reporting

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
)

func TestFreshValidationRejectsKnownUnhealthyDependencies(t *testing.T) {
	now := time.Now().UTC()
	h := strings.Repeat("a", 64)
	r := Revision{Number: 1, ID: "revision", Digest: h, ExecutionDigest: h}
	deps := []Dependency{{Dataset: "sales"}}
	v := &ValidationRecord{Dependencies: deps, Evidence: Evidence{ID: "evidence", Revision: 1, RevisionID: r.ID, DefinitionDigest: h, ExecutionDigest: h, DependencyDigest: DependencyDigest(deps, nil), CanonicalizationVersion: CanonicalizationVersion, ExpiresAt: now.Add(time.Hour), Attempt: exec.Attempt{Status: "succeeded"}}}
	baseline := Snapshot{Revision: r, Validation: v, Current: true, Health: Health{Status: "healthy", DependencyDigest: v.Evidence.DependencyDigest}}
	if err := freshValidation(baseline, "evidence", now); err != nil {
		t.Fatal("healthy baseline", err)
	}
	for _, health := range []Health{{Status: "unavailable"}, {Status: "stale"}, {Status: "unknown"}, {Status: "healthy", DependencyDigest: h}} {
		s := baseline
		s.Health = health
		if err := freshValidation(s, "evidence", now); !errors.Is(err, ErrStale) {
			t.Errorf("untrusted health accepted: %#v %v", health, err)
		}
	}
}

func impactBaseline() (ValidationRecord, exec.Binding, sources.CatalogObservation, []topics.Published) {
	binding := exec.Binding{Source: "warehouse", Dialect: "postgres", Relations: []exec.Relation{{ID: "sales", Schema: "analytics", Name: "sales"}}}
	definition := topics.Definition{Topic: "sales-topic", Version: "v1", Datasets: []topics.Dataset{{ID: "sales", Columns: []semantics.Column{{ID: "amount", SourceName: "amount", NativeType: "numeric", Category: "number"}}}}}
	pins := []TopicPin{{Topic: definition.Topic, Version: "v1", Digest: strings.Repeat("d", 64)}}
	catalog := sources.CatalogIdentity{Version: "postgres-catalog-identity-v1", Authority: strings.Repeat("a", 64), Relations: []sources.RelationIdentity{{Dataset: "sales", Object: "table-oid", Columns: []sources.ColumnIdentity{{Name: "amount", Object: "column-attnum"}}}}}
	old := ValidationRecord{Dependencies: []Dependency{{Dataset: "sales", Schema: "analytics", Name: "sales"}}, Binding: binding, BindingDigest: exec.Hash(binding), Topics: pins, Definitions: []topics.Definition{definition}, Catalog: catalog}
	current := []topics.Published{{Definition: clone(definition), Digest: pins[0].Digest}}
	return old, binding, sources.CatalogObservation{BindingDigest: exec.Hash(binding), Identity: clone(catalog)}, current
}

func TestImpactDoesNotMistakeReplacementForContinuity(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*sources.CatalogObservation)
	}{
		{"database or role replacement", func(o *sources.CatalogObservation) { o.Identity.Authority = strings.Repeat("b", 64) }},
		{"table recreated with the same name", func(o *sources.CatalogObservation) { o.Identity.Relations[0].Object = "different-table-oid" }},
		{"column recreated with the same name", func(o *sources.CatalogObservation) { o.Identity.Relations[0].Columns[0].Object = "different-attnum" }},
		{"native proof lost", func(o *sources.CatalogObservation) { o.Identity = sources.CatalogIdentity{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			old, binding, observation, current := impactBaseline()
			if got, _, _ := compareImpact(old, binding, observation, current); got != "unchanged" {
				t.Fatal("invalid baseline", got)
			}
			tc.mutate(&observation)
			if got, reason, renames := compareImpact(old, binding, observation, current); got != "review_required" || len(renames) != 0 {
				t.Fatalf("native replacement classified %s (%s): %v", got, reason, renames)
			}
		})
	}
}

func TestParameterPhysicalSlotBudget(t *testing.T) {
	p := make([]Parameter, 33)
	for i := range p {
		p[i] = Parameter{Name: fmt.Sprintf("window-%d", i), Type: "relative_period"}
	}
	if err := validateDeclarations(p[:32], 64); err != nil {
		t.Fatal("64 physical slots rejected", err)
	}
	if err := validateDeclarations(p, 64); !errors.Is(err, ErrInvalid) {
		t.Fatal("unexecutable 66-slot definition accepted", err)
	}
}
