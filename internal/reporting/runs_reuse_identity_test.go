package reporting

import (
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

func TestFrozenReuseIdentityPinsResolvedInputs(t *testing.T) {
	base := RunManifest{
		Version: FrozenVersion, ID: "run-one", Tenant: "tenant", Actor: "actor", Block: "block",
		Revision:     Revision{Digest: strings.Repeat("a", 64)},
		Definitions:  []topics.Definition{{Topic: "sales"}},
		Rules:        []RulePin{{Topic: "sales", RuleVersion: "rules-v1", RuleDigest: strings.Repeat("b", 64)}},
		Dependencies: []Dependency{{Source: "source", Context: "source:v1", SourceRevision: 1, Dataset: "sales"}},
		References:   []ResourceReference{{Kind: "dataset", Permission: "query", ID: "sales"}},
		Binding:      exec.Binding{Tenant: "tenant", Source: "source", Context: "source:v1", Revision: 1, Fingerprint: strings.Repeat("c", 64)},
		Locale:       "en-US", Policy: "published", PartialPolicy: "fail", Model: "model-v1",
	}
	baseline := ReuseIdentity(base)
	if baseline == "" || len(baseline) != 64 {
		t.Fatal("missing canonical reuse identity")
	}
	for name, mutate := range map[string]func(*RunManifest){
		"source":            func(m *RunManifest) { m.Binding.Revision++ },
		"rule":              func(m *RunManifest) { m.Rules[0].RuleDigest = strings.Repeat("d", 64) },
		"context":           func(m *RunManifest) { m.Binding.Context = "source:v2" },
		"topic":             func(m *RunManifest) { m.Revision.Digest = strings.Repeat("e", 64) },
		"runtime selection": func(m *RunManifest) { m.Model = "model-v2" },
		"semantic material": func(m *RunManifest) { m.Definitions[0].Topic = "support" },
		"dependency":        func(m *RunManifest) { m.Dependencies[0].Dataset = "returns" },
		"signed reach":      func(m *RunManifest) { m.References[0].ID = "returns" },
		"resolved window": func(m *RunManifest) {
			m.Resolved.Values = []BoundValue{{Name: "period", Digest: strings.Repeat("f", 64)}}
		},
		"partial policy": func(m *RunManifest) { m.PartialPolicy = "allow_partial" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := clone(base)
			changed.ReuseKey = baseline // deliberate stale-key substitution
			mutate(&changed)
			if changed.ReuseKey == ReuseIdentity(changed) {
				t.Fatal("stale key matched changed resolved input")
			}
		})
	}
	same := clone(base)
	same.ID, same.Actor, same.Session, same.ReuseMaxAge = "run-two", "other-actor", "other-session", 60
	if ReuseIdentity(same) != baseline {
		t.Fatal("public exact-result reuse is tied to an unrelated run ID or actor")
	}
	private := clone(base)
	private.Private = true
	private.Actor = "actor"
	otherPrivate := clone(private)
	otherPrivate.Actor = "other-actor"
	if ReuseIdentity(private) == ReuseIdentity(otherPrivate) || ReuseIdentity(private) == baseline {
		t.Fatal("private preview crossed privacy boundary")
	}
}
