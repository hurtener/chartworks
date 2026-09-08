package exec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
)

type privatePipelineProofAdapter struct {
	binding Binding
	proof   string
}

func (a *privatePipelineProofAdapter) Binding(context.Context, identity.Envelope, string, string) (Binding, error) {
	return a.binding.Clone(), nil
}
func (a *privatePipelineProofAdapter) Explain(context.Context, identity.Envelope, Candidate) error {
	return nil
}
func (a *privatePipelineProofAdapter) AuthorizePrivatePipeline(_ identity.Envelope, binding Binding, _ []string) (string, error) {
	if Hash(binding) != Hash(a.binding) {
		return "", ErrBinding
	}
	return a.proof, nil
}

func TestSQLBindingAndParameterBoundaries(t *testing.T) {
	b := parserBinding()
	for _, mutate := range []func(*Binding){
		func(b *Binding) { b.Tenant = "" }, func(b *Binding) { b.Source = "bad/" }, func(b *Binding) { b.Context = "" }, func(b *Binding) { b.Catalog = "server.database" }, func(b *Binding) { b.Contract = "" }, func(b *Binding) { b.Revision = 0 }, func(b *Binding) { b.Fingerprint = "short" }, func(b *Binding) { b.Fingerprint = strings.Repeat("z", 64) }, func(b *Binding) { b.Relations = nil }, func(b *Binding) { b.Relations = append(b.Relations, b.Relations[0]) }, func(b *Binding) { b.Relations[0].ID = "bad/" }, func(b *Binding) { b.Relations[0].Schema = "1bad" }, func(b *Binding) { b.Relations[0].Name = "" }, func(b *Binding) { b.Relations[0].Columns = nil }, func(b *Binding) { b.Relations[0].Columns = append(b.Relations[0].Columns, b.Relations[0].Columns[0]) }, func(b *Binding) { b.Relations[0].Columns[0].Name = strings.Repeat("x", 64) }, func(b *Binding) { b.Relations[0].Columns[0].NativeType = "" },
	} {
		changed := b.Clone()
		mutate(&changed)
		if changed.Valid() {
			t.Fatal("malformed resolved binding accepted")
		}
		if !b.Valid() {
			t.Fatal("cloned binding mutated original")
		}
	}
	for _, p := range []Parameter{{"null", ""}, {"text", "a'"}, {"boolean", "true"}, {"boolean", "false"}, {"integer", "-9223372036854775808"}, {"number", "9007199254740993.125"}} {
		if !p.Valid() {
			t.Fatal("valid typed value rejected", p.Kind)
		}
	}
	for _, p := range []Parameter{{"null", "null"}, {"text", "a\x00b"}, {"text", strings.Repeat("x", 4097)}, {"boolean", "TRUE"}, {"integer", "9223372036854775808"}, {"number", "NaN"}, {"number", "1e9999"}, {"number", "+1"}, {"shell", "value"}} {
		if p.Valid() {
			t.Fatal("ambiguous parameter accepted", p.Kind)
		}
	}
	// Internal test payloads establish that every formatting/serialization path
	// redacts sensitive query text, including detailed Go formatting.
	c := Candidate{statement: "PRIVATE_QUERY_CANARY", parameters: []Parameter{{"text", "PRIVATE_VALUE_CANARY"}}}
	p := Plan{candidate: c}
	for _, value := range []any{p, c} {
		encoded, err := json.Marshal(value)
		if err != nil || strings.Contains(string(encoded)+fmt.Sprintf("%v %#v", value, value), "PRIVATE_") {
			t.Fatal("opaque plan or candidate leaked query input", err)
		}
	}
	if _, _, err := p.SQL(p.candidate.owner, b); err == nil {
		t.Fatal("zero native proof admitted")
	}
}

func TestPrivatePipelineProofSurvivesCandidateAndPlanGates(t *testing.T) {
	binding := parserBinding()
	e, err := identity.FromVerified(binding.Tenant, "actor", "session", []string{"engineering.pipeline.run", "cw.source.write:pipeline"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	adapter := &privatePipelineProofAdapter{binding: binding, proof: Hash("private-pipeline-operation")}
	candidate := Candidate{
		binding: binding.Clone(), owner: e, authority: authority(e), private: adapter, privateProof: adapter.proof,
		statement: "SELECT id FROM analytics.sales", dependencies: []string{binding.Relations[0].ID}, checked: true,
	}
	plan := Plan{candidate: candidate, nativeChecked: true}
	if _, _, err = candidate.SQL(e, binding); err != nil {
		t.Fatal("candidate lost private pipeline authority", err)
	}
	if _, _, err = plan.SQL(e, binding); err != nil {
		t.Fatal("plan lost private pipeline authority", err)
	}

	adapter.proof = Hash("other-pipeline-operation")
	if _, _, err = candidate.SQL(e, binding); !errors.Is(err, ErrBinding) {
		t.Fatal("candidate accepted changed private operation proof", err)
	}
	if _, _, err = plan.SQL(e, binding); !errors.Is(err, ErrBinding) {
		t.Fatal("plan accepted changed private operation proof", err)
	}

	ordinary := candidate
	ordinary.private = nil
	ordinary.privateProof = ""
	if _, _, err = ordinary.SQL(e, binding); !errors.Is(err, access.ErrForbidden) {
		t.Fatal("pipeline authority became ordinary source-query authority", err)
	}
}
