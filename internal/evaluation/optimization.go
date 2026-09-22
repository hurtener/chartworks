package evaluation

import "time"

// CandidateScore summarizes exact heldout report evidence.
type CandidateScore struct {
	ID           string `json:"id"`
	PackDigest   string `json:"pack_digest"`
	Passed       int    `json:"passed"`
	Total        int    `json:"total"`
	EvidenceHash string `json:"evidence_hash"`
}

// OptimizationProposal compares two validated reports without publishing a pack.
type OptimizationProposal struct {
	SchemaVersion int            `json:"schema_version"`
	ID            string         `json:"id"`
	SuiteID       string         `json:"suite_id"`
	SuiteRevision int64          `json:"suite_revision"`
	SuiteDigest   string         `json:"suite_digest"`
	Mode          Mode           `json:"mode"`
	Seed          int64          `json:"seed"`
	Baseline      CandidateScore `json:"baseline"`
	Candidate     CandidateScore `json:"candidate"`
	State         string         `json:"state"`
	CreatedAt     time.Time      `json:"created_at"`
}

// ReviewReceipt records an authenticated human decision.
type ReviewReceipt struct {
	ProposalID     string    `json:"proposal_id"`
	ProposalDigest string    `json:"proposal_digest"`
	Reviewer       string    `json:"reviewer"`
	Decision       string    `json:"decision"`
	ReviewedAt     time.Time `json:"reviewed_at"`
}

// ProposalRequest pins stored reports, suite revision, and pack digests.
type ProposalRequest struct {
	ID            string `json:"id"`
	SuiteID       string `json:"suite_id"`
	SuiteRevision int64  `json:"suite_revision"`
	SuiteDigest   string `json:"suite_digest"`
	BaselineRun   string `json:"baseline_run"`
	CandidateRun  string `json:"candidate_run"`
	BaselinePack  string `json:"baseline_pack"`
	CandidatePack string `json:"candidate_pack"`
}

// ProposalReviewRequest binds a decision to the exact proposal digest.
type ProposalReviewRequest struct {
	Digest   string `json:"digest"`
	Decision string `json:"decision"`
}

// PackSelection is the CAS-controlled active approved pack pointer.
type PackSelection struct {
	Revision   int64     `json:"revision"`
	PackDigest string    `json:"pack_digest"`
	ProposalID string    `json:"proposal_id"`
	Actor      string    `json:"actor"`
	SelectedAt time.Time `json:"selected_at"`
}

// Validate checks immutable proposal identity and evidence references.
func (p OptimizationProposal) Validate() error {
	if p.SchemaVersion != SchemaVersion || !identifier(p.ID) || !identifier(p.SuiteID) || p.SuiteRevision < 1 || !validDigest(p.SuiteDigest) || (p.Mode != Fixture && p.Mode != Live) || p.Seed == 0 || p.State != "candidate" || p.CreatedAt.IsZero() {
		return ErrInvalid
	}
	for _, x := range []CandidateScore{p.Baseline, p.Candidate} {
		if !identifier(x.ID) || !validDigest(x.PackDigest) || !validDigest(x.EvidenceHash) || x.Total < 1 || x.Passed < 0 || x.Passed > x.Total {
			return ErrInvalid
		}
	}
	if p.Baseline.Total != p.Candidate.Total || p.Candidate.Passed <= p.Baseline.Passed {
		return ErrInvalid
	}
	return nil
}

// ProposeOptimization validates complete report evidence and exact heldout provenance.
func ProposeOptimization(id string, s Suite, baseline, candidate Report, baselinePack, candidatePack string, now time.Time) (OptimizationProposal, error) {
	if !identifier(id) || s.Validate() != nil || baseline.Validate() != nil || candidate.Validate() != nil || !validDigest(baselinePack) || !validDigest(candidatePack) || baselinePack == candidatePack {
		return OptimizationProposal{}, ErrInvalid
	}
	for _, r := range []Report{baseline, candidate} {
		if r.SuiteID != s.ID || r.SuiteRevision != s.Revision || r.SuiteDigest == "" || r.Mode != s.Mode || r.Seed != s.Seed {
			return OptimizationProposal{}, ErrInvalid
		}
		d, _ := s.Digest()
		if r.SuiteDigest != d {
			return OptimizationProposal{}, ErrInvalid
		}
	}
	if len(baseline.Cases) != len(candidate.Cases) {
		return OptimizationProposal{}, ErrInvalid
	}
	for i, b := range baseline.Cases {
		c := candidate.Cases[i]
		if b.ID != c.ID || b.Stage != c.Stage || b.HeldOut != c.HeldOut || b.Critical != c.Critical {
			return OptimizationProposal{}, ErrInvalid
		}
	}
	bs, cs := scoreHeldOut(baseline), scoreHeldOut(candidate)
	if bs.Total == 0 || cs.Total != bs.Total || cs.Passed <= bs.Passed {
		return OptimizationProposal{}, ErrInvalid
	}
	d, _ := s.Digest()
	p := OptimizationProposal{SchemaVersion: SchemaVersion, ID: id, SuiteID: s.ID, SuiteRevision: s.Revision, SuiteDigest: d, Mode: s.Mode, Seed: s.Seed, Baseline: CandidateScore{ID: baseline.RunID, PackDigest: baselinePack, Passed: bs.Passed, Total: bs.Total, EvidenceHash: baseline.EvidenceHash}, Candidate: CandidateScore{ID: candidate.RunID, PackDigest: candidatePack, Passed: cs.Passed, Total: cs.Total, EvidenceHash: candidate.EvidenceHash}, State: "candidate", CreatedAt: now.UTC()}
	if p.Validate() != nil {
		return OptimizationProposal{}, ErrInvalid
	}
	return p, nil
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
func proposalReceipt(p OptimizationProposal, reviewer, decision string, now time.Time) (ReviewReceipt, error) {
	d, _ := digest(p)
	if p.State != "candidate" || !identifier(reviewer) || (decision != "approve" && decision != "reject") {
		return ReviewReceipt{}, ErrReview
	}
	return ReviewReceipt{ProposalID: p.ID, ProposalDigest: d, Reviewer: reviewer, Decision: decision, ReviewedAt: now.UTC()}, nil
}
