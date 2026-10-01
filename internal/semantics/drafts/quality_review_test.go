package drafts

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
)

type qualityTestEngine struct {
	gateway.Engine
	generate func(string, *gateway.Schema) (gateway.Generated, error)
}

func (e qualityTestEngine) Generate(_ context.Context, _ gateway.Call, _ *gateway.Budget, role, system, input string, schema *gateway.Schema) (gateway.Generated, error) {
	if role != "topic_review" || !strings.Contains(system, "entire candidate") {
		return gateway.Generated{}, gateway.ErrInput
	}
	return e.generate(input, schema)
}

func TestWholeTopicReviewBindsFullCandidateAndCannotMutate(t *testing.T) {
	model, profiles := authoringFixture(t)
	p := model.Pack()
	second := p.Measures[0]
	second.ID = "another_revenue"
	second.Name = "Different revenue"
	p.Measures = append(p.Measures, second)
	model, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	material, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	before := model.Pack()
	engine := qualityTestEngine{generate: func(input string, schema *gateway.Schema) (gateway.Generated, error) {
		var supplied struct {
			Context        authoringContext `json:"context"`
			Coverage       []string         `json:"coverage"`
			CoverageDigest string           `json:"coverage_digest"`
		}
		if err := json.Unmarshal([]byte(input), &supplied); err != nil {
			t.Fatal(err)
		}
		if len(supplied.Context.Candidate.Datasets) != 2 || len(supplied.Context.Relationships) != 36 || len(supplied.Context.Candidate.Measures) != 2 || !reflect.DeepEqual(supplied.Coverage, qualityCoverage(model.Pack())) {
			t.Fatal("review received only a page")
		}
		answer := qualityWire{CandidateDigest: supplied.Context.CandidateDigest, ContextDigest: supplied.Context.Digest, CoverageDigest: supplied.CoverageDigest, Status: "no_findings", Findings: []QualityFinding{}}
		raw, _ := json.Marshal(answer)
		return gateway.Generated{JSON: raw}, nil
	}}
	service := Service{engine: engine}
	q, _, err := service.reviewAuthoringCandidate(context.Background(), gateway.Call{}, nil, model, material)
	if err != nil || q == nil || !q.ValidFor(model) || q.Status != "needs_review" {
		t.Fatal("exact complete advisory missing", q, err)
	}
	if !reflect.DeepEqual(model.Pack(), before) {
		t.Fatal("quality review mutated protected topic")
	}
	found := false
	for _, f := range q.Findings {
		if f.Code == "alias_collision" {
			found = true
		}
	}
	if !found {
		t.Fatal("collision escaped deterministic review")
	}
	changed := model.Pack()
	changed.Description = "Changed business meaning"
	other, err := semantics.Compile(changed)
	if err != nil {
		t.Fatal(err)
	}
	if q.ValidFor(other) {
		t.Fatal("advisory survived candidate drift")
	}
	incomplete := *q
	incomplete.Coverage = append([]string(nil), q.Coverage[1:]...)
	if incomplete.ValidFor(model) {
		t.Fatal("incomplete coverage accepted")
	}
	if _, err := qualityReviewSchema(model.Digest(), material.Digest, qualityCoverage(model.Pack())); err != nil {
		t.Fatal("closed schema invalid", err)
	}
}

func TestWholeTopicReviewRejectsInventedAuthorityDriftAndMutations(t *testing.T) {
	model, profiles := authoringFixture(t)
	material, err := buildAuthoringContext(model, profiles)
	if err != nil {
		t.Fatal(err)
	}
	baseline := map[string]any{"candidate_digest": model.Digest(), "context_digest": material.Digest, "coverage_digest": readexec.Hash(qualityCoverage(model.Pack())), "status": "no_findings", "findings": []QualityFinding{}}
	for name, alter := range map[string]func(map[string]any){
		"approval":        func(v map[string]any) { v["status"] = "approved" },
		"candidate drift": func(v map[string]any) { v["candidate_digest"] = strings.Repeat("f", 64) },
		"context drift":   func(v map[string]any) { v["context_digest"] = strings.Repeat("f", 64) },
		"coverage drift":  func(v map[string]any) { v["coverage_digest"] = strings.Repeat("f", 64) },
		"mutation":        func(v map[string]any) { v["candidate"] = model.Pack() },
		"invented reference": func(v map[string]any) {
			v["status"] = "needs_review"
			v["findings"] = []QualityFinding{{Code: "grain_conflict", Entities: []string{"dataset:private"}, Detail: "invented"}}
		},
		"open code": func(v map[string]any) {
			v["status"] = "needs_review"
			v["findings"] = []QualityFinding{{Code: "publish_now", Entities: []string{"topic:commerce"}, Detail: "approval"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			response := map[string]any{}
			for k, v := range baseline {
				response[k] = v
			}
			alter(response)
			raw, _ := json.Marshal(response)
			service := Service{engine: qualityTestEngine{generate: func(string, *gateway.Schema) (gateway.Generated, error) { return gateway.Generated{JSON: raw}, nil }}}
			if q, _, err := service.reviewAuthoringCandidate(context.Background(), gateway.Call{}, nil, model, material); q != nil || !errors.Is(err, gateway.ErrOutput) {
				t.Fatal("untrusted review accepted", q, err)
			}
		})
	}
}

func TestQualityCoverageUsesUnambiguousColumnCoordinates(t *testing.T) {
	p := semantics.TopicPack{Topic: "topic", Datasets: []semantics.Dataset{{ID: "a:b", Columns: []semantics.Column{{ID: "c"}}}, {ID: "a", Columns: []semantics.Column{{ID: "b:c"}}}}}
	coverage := qualityCoverage(p)
	seen := map[string]bool{}
	for _, key := range coverage {
		if seen[key] {
			t.Fatal("ambiguous coverage coordinates", coverage)
		}
		seen[key] = true
	}
}

func TestEnhanceRequiresExplicitReviewPolicyBeforeAnyIO(t *testing.T) {
	service := Service{engine: qualityTestEngine{generate: func(string, *gateway.Schema) (gateway.Generated, error) {
		t.Fatal("unadmitted model call")
		return gateway.Generated{}, nil
	}}}
	if _, err := service.Enhance(context.Background(), identity.Envelope{}, "topic", EnhanceRequest{Expected: 1, Version: "v2", Limit: 1, Change: "Review policy"}); !errors.Is(err, gateway.ErrDisabled) {
		t.Fatal("unknown review policy admitted", err)
	}
}
