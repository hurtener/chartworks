package drafts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/semantics"
)

// QualityFinding is advisory, never an approved semantic mutation.
type QualityFinding struct {
	Code     string   `json:"code"`
	Entities []string `json:"entities"`
	Detail   string   `json:"detail"`
}

// QualityReview binds a whole-candidate advisory to exact immutable content.
// Even no_findings is not publication approval or a proof of business correctness.
type QualityReview struct {
	CandidateDigest string           `json:"candidate_digest"`
	ContextDigest   string           `json:"context_digest"`
	CoverageDigest  string           `json:"coverage_digest"`
	Coverage        []string         `json:"coverage"`
	Status          string           `json:"status"`
	Findings        []QualityFinding `json:"findings"`
}
type qualityWire struct {
	CandidateDigest string           `json:"candidate_digest"`
	ContextDigest   string           `json:"context_digest"`
	CoverageDigest  string           `json:"coverage_digest"`
	Status          string           `json:"status"`
	Findings        []QualityFinding `json:"findings"`
}

const qualitySchemaTemplate = `{"type":"object","additionalProperties":false,"required":["candidate_digest","context_digest","coverage_digest","status","findings"],"properties":{"candidate_digest":{"type":"string","minLength":64,"maxLength":64},"context_digest":{"type":"string","minLength":64,"maxLength":64},"coverage_digest":{"type":"string","minLength":64,"maxLength":64},"status":{"enum":["no_findings","needs_review"]},"findings":{"type":"array","maxItems":128,"items":{"type":"object","additionalProperties":false,"required":["code","entities","detail"],"properties":{"code":{"enum":["alias_collision","ambiguous_meaning","grain_conflict","temporal_conflict","unresolved_relationship","missing_description","incomplete_evidence","unresolved_column"]},"entities":{"type":"array","minItems":1,"maxItems":32,"items":{"type":"string","minLength":1,"maxLength":512}},"detail":{"type":"string","minLength":1,"maxLength":1024}}}}}}`

func qualityCoverage(p semantics.TopicPack) []string {
	out := []string{"topic:" + p.Topic}
	for _, d := range p.Datasets {
		out = append(out, "dataset:"+d.ID)
		for _, c := range d.Columns {
			key, _ := json.Marshal([]string{d.ID, c.ID})
			out = append(out, "column:"+string(key))
		}
	}
	for _, v := range p.Measures {
		out = append(out, "measure:"+v.ID)
		if v.Completeness != nil {
			out = append(out, "amount_completeness:"+v.ID)
		}
	}
	for _, v := range p.Dimensions {
		out = append(out, "dimension:"+v.ID)
	}
	for _, v := range p.KPIs {
		out = append(out, "kpi:"+v.ID)
		if v.Periods != nil {
			out = append(out, "metric_periods:"+v.ID)
		}
	}
	for _, v := range p.Joins {
		out = append(out, "join:"+v.ID)
	}
	for _, v := range p.RelationshipDecisions {
		out = append(out, "relationship:"+v.ID)
	}
	for _, v := range p.CanonicalEntities {
		out = append(out, "canonical_entity:"+v.ID)
	}
	for _, v := range p.Unresolved {
		out = append(out, "unresolved:"+v.ID)
	}
	if p.GroupDomain != nil {
		out = append(out, "group_domain")
	}
	if p.GroupedPopulation != nil {
		out = append(out, "grouped_population")
	}
	sort.Strings(out)
	return out
}

// ValidFor checks content, full coverage and all finding references. It grants no
// authority and deliberately accepts needs_review for a human to adjudicate.
func (q QualityReview) ValidFor(model semantics.Model) bool {
	coverage := qualityCoverage(model.Pack())
	if q.CandidateDigest != model.Digest() || !qualityDigestValid(q.ContextDigest) || q.CoverageDigest != readexec.Hash(coverage) || len(q.Coverage) != len(coverage) || len(q.Findings) > 128 {
		return false
	}
	for i := range coverage {
		if coverage[i] != q.Coverage[i] {
			return false
		}
	}
	if q.Status != "no_findings" && q.Status != "needs_review" || (q.Status == "no_findings") != (len(q.Findings) == 0) {
		return false
	}
	allowed := map[string]bool{}
	for _, v := range coverage {
		allowed[v] = true
	}
	for _, f := range q.Findings {
		switch f.Code {
		case "alias_collision", "ambiguous_meaning", "grain_conflict", "temporal_conflict", "unresolved_relationship", "missing_description", "incomplete_evidence", "unresolved_column":
		default:
			return false
		}
		if len(f.Entities) < 1 || len(f.Entities) > 32 || len(f.Detail) < 1 || len(f.Detail) > 1024 || !utf8.ValidString(f.Detail) {
			return false
		}
		for _, v := range f.Entities {
			if !allowed[v] {
				return false
			}
		}
	}
	return true
}

func deterministicQualityFindings(p semantics.TopicPack, redactions []authoringRedaction) []QualityFinding {
	out := []QualityFinding{}
	terms := map[string][]string{}
	add := func(id, name string, aliases []string) {
		seen := map[string]bool{}
		for _, term := range append([]string{name}, aliases...) {
			key := strings.ToLower(strings.Join(strings.Fields(term), " "))
			if key != "" && !seen[key] {
				terms[key] = append(terms[key], id)
				seen[key] = true
			}
		}
	}
	for _, v := range p.Measures {
		add("measure:"+v.ID, v.Name, v.Aliases)
		if strings.TrimSpace(v.Description) == "" {
			out = append(out, QualityFinding{Code: "missing_description", Entities: []string{"measure:" + v.ID}, Detail: "Business definition is missing."})
		}
	}
	for _, v := range p.Dimensions {
		add("dimension:"+v.ID, v.Name, v.Aliases)
		if strings.TrimSpace(v.Description) == "" {
			out = append(out, QualityFinding{Code: "missing_description", Entities: []string{"dimension:" + v.ID}, Detail: "Business definition is missing."})
		}
	}
	for _, v := range p.KPIs {
		add("kpi:"+v.ID, v.Name, v.Aliases)
		if strings.TrimSpace(v.Description) == "" {
			out = append(out, QualityFinding{Code: "missing_description", Entities: []string{"kpi:" + v.ID}, Detail: "Business definition is missing."})
		}
	}
	keys := []string{}
	for key, entities := range terms {
		if len(entities) > 1 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		entities := terms[key]
		sort.Strings(entities)
		for start := 0; start < len(entities); start += 32 {
			out = append(out, QualityFinding{Code: "alias_collision", Entities: entities[start:min(start+32, len(entities))], Detail: "A normalized business term refers to multiple semantic entities; resolve the intended meaning explicitly."})
		}
	}
	for _, v := range p.Unresolved {
		out = append(out, QualityFinding{Code: "unresolved_column", Entities: []string{"unresolved:" + v.ID}, Detail: "Column semantics remain unresolved and require human review."})
	}
	for _, v := range p.RelationshipDecisions {
		if v.State == "candidate" {
			out = append(out, QualityFinding{Code: "unresolved_relationship", Entities: []string{"relationship:" + v.ID}, Detail: "Relationship remains a proposal; sampled counts cannot prove key uniqueness or authorize a join."})
		}
	}
	for _, v := range redactions {
		out = append(out, QualityFinding{Code: "incomplete_evidence", Entities: []string{v.Entity}, Detail: "Governed values or filter literals were withheld from model review; a human must verify the complete definition."})
	}
	return out
}

func (s *Service) reviewAuthoringCandidate(ctx context.Context, call gateway.Call, budget *gateway.Budget, model semantics.Model, material authoringContext) (*QualityReview, gateway.Receipt, error) {
	coverage := qualityCoverage(model.Pack())
	coverageDigest := readexec.Hash(coverage)
	transport, err := newQualityReviewTransport(model.Digest(), material.Digest, coverage)
	if err != nil {
		return nil, gateway.Receipt{}, err
	}
	input := struct {
		Context            authoringContext         `json:"context"`
		Coverage           []string                 `json:"coverage"`
		CoverageDigest     string                   `json:"coverage_digest"`
		ReferenceEncoding  string                   `json:"review_reference_encoding"`
		DomainSchemaDigest string                   `json:"canonical_review_schema_digest"`
		EntityReferences   []qualityEntityReference `json:"entity_references"`
	}{material, coverage, coverageDigest, "request-local-handles-v1", transport.domainDigest(), transport.references}
	raw, err := json.Marshal(input)
	if err != nil || len(raw) > maxAuthoringContextBytes {
		return nil, gateway.Receipt{}, gateway.ErrBudget
	}
	generated, err := s.engine.Generate(ctx, call, budget, "topic_review", qualityReviewInstructions, string(raw), transport.schema)
	if err != nil {
		return nil, generated.Receipt, err
	}
	wire, decodeErr := transport.decode(generated.JSON)
	if decodeErr != nil || wire.CandidateDigest != model.Digest() || wire.ContextDigest != material.Digest || wire.CoverageDigest != coverageDigest {
		return nil, generated.Receipt, fmt.Errorf("%w: topic review wire or digest binding", gateway.ErrOutput)
	}
	q := &QualityReview{CandidateDigest: wire.CandidateDigest, ContextDigest: wire.ContextDigest, CoverageDigest: wire.CoverageDigest, Coverage: coverage, Status: wire.Status, Findings: wire.Findings}
	if !q.ValidFor(model) {
		return nil, generated.Receipt, fmt.Errorf("%w: topic review provider advisory references or state", gateway.ErrOutput)
	}
	q.Findings = append(q.Findings, deterministicQualityFindings(model.Pack(), material.Redactions)...)
	for _, e := range material.Evidence {
		if len(e.MissingColumns) > 0 {
			q.Findings = append(q.Findings, QualityFinding{Code: "incomplete_evidence", Entities: []string{"dataset:" + e.Origin.Dataset}, Detail: "Some columns have no aggregate profile evidence; their meaning remains dependent on explicit business context."})
		}
	}
	// Do not silently omit findings when a complete review exceeds its bound.
	if len(q.Findings) > 128 {
		return nil, generated.Receipt, gateway.ErrBudget
	}
	if len(q.Findings) > 0 {
		q.Status = "needs_review"
	}
	if !q.ValidFor(model) {
		return nil, generated.Receipt, fmt.Errorf("%w: topic review augmented advisory references or state", gateway.ErrOutput)
	}
	return q, generated.Receipt, nil
}

func qualityDigestValid(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 32 && strings.ToLower(value) == value
}
