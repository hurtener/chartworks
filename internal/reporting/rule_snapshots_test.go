package reporting

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/store"
)

type snapshotRuleReader struct {
	published rulesets.Published
	err       error
}

func (r snapshotRuleReader) Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error) {
	return r.published, r.err
}

func testRulePin() RulePin {
	return RulePin{Topic: "sales", TopicVersion: "v1", PackDigest: strings.Repeat("a", 64), RuleVersion: "rules-v1", RuleDigest: strings.Repeat("b", 64)}
}

func TestRulePinsAreVersionedExecutionDependencies(t *testing.T) {
	d := contractDefinition()
	var err error
	d, err = MigrateDefinition(d)
	if err != nil {
		t.Fatal(err)
	}
	d.Rules = []RulePin{testRulePin()}
	if err := validateDefinition(context.Background(), d, config.DefaultReporting(), false); err != nil {
		t.Fatal("valid exact rule pin rejected", err)
	}
	base := ExecutionDigest(d)
	changed := clone(d)
	changed.Rules[0].RuleVersion = "rules-v2"
	changed.Rules[0].RuleDigest = strings.Repeat("c", 64)
	if base == ExecutionDigest(changed) || DependencyDigest(nil, d.Topics, d.Rules) == DependencyDigest(nil, changed.Topics, changed.Rules) {
		t.Fatal("rule publication identity was omitted from execution or dependency digest")
	}

	for name, mutate := range map[string]func(*Definition){
		"legacy_implicit_mapping": func(d *Definition) { d.SchemaVersion = SchemaVersion },
		"missing_topic_snapshot":  func(d *Definition) { d.Rules = nil },
		"topic_mismatch":          func(d *Definition) { d.Rules[0].TopicVersion = "v2" },
		"pack_mismatch":           func(d *Definition) { d.Rules[0].PackDigest = strings.Repeat("d", 64) },
		"invalid_rule_version":    func(d *Definition) { d.Rules[0].RuleVersion = "../rule" },
		"invalid_rule_digest":     func(d *Definition) { d.Rules[0].RuleDigest = "not-a-digest" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := clone(d)
			mutate(&bad)
			if name == "missing_topic_snapshot" {
				// Empty rules remain the explicit compatibility disposition for
				// existing v2 blocks; migration never invents a rule snapshot.
				if err := validateDefinition(context.Background(), bad, config.DefaultReporting(), false); err != nil {
					t.Fatal("compatible no-rule definition rejected", err)
				}
				return
			}
			if validateDefinition(context.Background(), bad, config.DefaultReporting(), false) == nil {
				t.Fatal("unsupported rule mapping admitted")
			}
		})
	}
}

func TestResolveRulesRejectsStaleOrUnavailablePublications(t *testing.T) {
	pin := testRulePin()
	published := rulesets.Published{
		State:      rulesets.State{Topic: pin.Topic, Version: pin.RuleVersion, Active: true},
		Definition: semantics.RuleSetDefinition{Topic: pin.Topic, TopicVersion: pin.TopicVersion, PackDigest: pin.PackDigest},
		Digest:     pin.RuleDigest,
	}
	d := Definition{Rules: []RulePin{pin}, Topics: []TopicPin{{Topic: pin.Topic, Version: pin.TopicVersion, Digest: pin.PackDigest}}}
	s := &Service{rules: snapshotRuleReader{published: published}}
	resolved, err := s.resolveRules(context.Background(), identity.Envelope{}, d, true)
	if err != nil || len(resolved) != 1 || resolved[0] != pin {
		t.Fatalf("exact active snapshot did not resolve: %#v %v", resolved, err)
	}

	retired := published
	retired.State.Active = false
	s.rules = snapshotRuleReader{published: retired}
	if _, err := s.resolveRules(context.Background(), identity.Envelope{}, d, true); !errors.Is(err, ErrStale) {
		t.Fatalf("retired current snapshot accepted: %v", err)
	}
	if _, err := s.resolveRules(context.Background(), identity.Envelope{}, d, false); err != nil {
		t.Fatalf("exact retained snapshot could not be inspected: %v", err)
	}
	s.rules = snapshotRuleReader{err: store.ErrNotFound}
	if _, err := s.resolveRules(context.Background(), identity.Envelope{}, d, true); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("missing retained snapshot was widened: %v", err)
	}
	s.rules = nil
	if _, err := s.resolveRules(context.Background(), identity.Envelope{}, d, true); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("missing rule reader did not fail closed: %v", err)
	}
}

func TestFrozenEligibilityRefusesRuleSnapshotDrift(t *testing.T) {
	pin := testRulePin()
	h := strings.Repeat("c", 64)
	r := Revision{Number: 1, ID: "revision", Digest: h, Definition: Definition{Topics: []TopicPin{{Topic: pin.Topic, Version: pin.TopicVersion, Digest: pin.PackDigest}}, Rules: []RulePin{pin}}}
	v := &ValidationRecord{Rules: []RulePin{pin}, Evidence: Evidence{DependencyDigest: DependencyDigest(nil, r.Definition.Topics, []RulePin{pin})}}
	snapshot := Snapshot{State: State{ID: "block"}, Revision: r, Validation: v}
	m := RunManifest{Block: "block", Revision: r, Rules: []RulePin{pin}, Dependencies: nil}
	changed := pin
	changed.RuleVersion = "rules-v2"
	changed.RuleDigest = strings.Repeat("d", 64)
	m.Rules = []RulePin{changed}
	// The authority guard runs first for complete manifests; the direct digest
	// relationship is asserted independently so no test fabricates a bearer.
	if DependencyDigest(snapshot.Validation.Dependencies, r.Definition.Topics, snapshot.Validation.Rules) == DependencyDigest(m.Dependencies, r.Definition.Topics, m.Rules) {
		t.Fatal("frozen reuse identity ignored rule snapshot drift")
	}
	group := CompositionGroup{Kind: "block", Block: snapshot.State.ID, Revision: r.Number, Definition: r.Digest, Execution: r.ExecutionDigest, Rules: []RulePin{changed}}
	if err := CheckCompositionBlock(identity.Envelope{}, group, snapshot); !errors.Is(err, ErrStale) {
		t.Fatalf("composition admitted a different rule snapshot: %v", err)
	}
}
