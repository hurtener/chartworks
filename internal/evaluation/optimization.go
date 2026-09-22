package evaluation

import "time"

// CandidateScore summarizes held-out evidence for one pack.
type CandidateScore struct {
	ID           string `json:"id"`
	PackDigest   string `json:"pack_digest"`
	Passed       int    `json:"passed"`
	Total        int    `json:"total"`
	EvidenceHash string `json:"evidence_hash"`
}

// OptimizationProposal is evidence, never a publication command.
type OptimizationProposal struct {
	SchemaVersion int            `json:"schema_version"`
	SuiteID       string         `json:"suite_id"`
	Baseline      CandidateScore `json:"baseline"`
	Candidate     CandidateScore `json:"candidate"`
	State         string         `json:"state"`
	CreatedAt     time.Time      `json:"created_at"`
}

// ReviewReceipt records an explicit human decision.
type ReviewReceipt struct {
	ProposalDigest string    `json:"proposal_digest"`
	Reviewer       string    `json:"reviewer"`
	Decision       string    `json:"decision"`
	ReviewedAt     time.Time `json:"reviewed_at"`
}

// ProposeOptimization compares only held-out results and never publishes a pack.
func ProposeOptimization(s Suite, baseline, candidate Report, baselinePack, candidatePack string, now time.Time) (OptimizationProposal, error) {
	if s.Validate() != nil || !validDigest(baselinePack) || !validDigest(candidatePack) || baselinePack == candidatePack || baseline.SuiteID != s.ID || candidate.SuiteID != s.ID {
		return OptimizationProposal{}, ErrInvalid
	}
	bs, cs := scoreHeldOut(baseline), scoreHeldOut(candidate)
	if bs.Total == 0 || cs.Total != bs.Total || cs.Passed <= bs.Passed {
		return OptimizationProposal{}, ErrInvalid
	}
	return OptimizationProposal{SchemaVersion: SchemaVersion, SuiteID: s.ID, Baseline: CandidateScore{ID: "baseline", PackDigest: baselinePack, Passed: bs.Passed, Total: bs.Total, EvidenceHash: baseline.EvidenceHash}, Candidate: CandidateScore{ID: "candidate", PackDigest: candidatePack, Passed: cs.Passed, Total: cs.Total, EvidenceHash: candidate.EvidenceHash}, State: "candidate", CreatedAt: now.UTC()}, nil
}
func scoreHeldOut(r Report) CandidateScore {
	x := CandidateScore{}
	for _, c := range r.Cases {
		if c.HeldOut && !c.Critical {
			x.Total++
			if c.Passed {
				x.Passed++
			}
		}
	}
	return x
}

// ReviewPromotion returns a reviewed immutable decision. No evaluator or score can self-promote.
func ReviewPromotion(p OptimizationProposal, reviewer, decision string, now time.Time) (ReviewReceipt, error) {
	d, _ := digest(p)
	if p.State != "candidate" || !identifier(reviewer) || (decision != "approve" && decision != "reject") {
		return ReviewReceipt{}, ErrReview
	}
	return ReviewReceipt{ProposalDigest: d, Reviewer: reviewer, Decision: decision, ReviewedAt: now.UTC()}, nil
}
