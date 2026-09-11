package jobs

import (
	"strings"
	"testing"
)

func TestPipelineTargetPinsExactVersion(t *testing.T) {
	p := PipelineTarget{ID: "pipeline", Version: 1, Digest: strings.Repeat("a", 64)}
	if !p.Valid() {
		t.Fatal("valid target rejected")
	}
	for _, change := range []func(*PipelineTarget){
		func(p *PipelineTarget) { p.ID = "*" },
		func(p *PipelineTarget) { p.Version = 0 },
		func(p *PipelineTarget) { p.Digest = strings.Repeat("A", 64) },
	} {
		bad := p
		change(&bad)
		if bad.Valid() {
			t.Fatal("invalid target accepted")
		}
	}
	// Extending target types must not turn a maintenance request into pipeline
	// authority or advertise execution before the handler is installed.
	if (Submission{Kind: MaintenanceKind, BindingID: "binding", Pipeline: &p}).Validate() == nil {
		t.Fatal("maintenance target widened")
	}
	j := Job{Pipeline: &p}
	before := j.Digest()
	p.Version++
	if j.Digest() == before {
		t.Fatal("pipeline version absent from manifest")
	}
	before = j.Digest()
	p.Digest = strings.Repeat("b", 64)
	if j.Digest() == before {
		t.Fatal("pipeline definition absent from manifest")
	}
}
