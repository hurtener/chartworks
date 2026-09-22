package evaluation

import (
	"context"
	"sync"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Repository persists immutable revisions, review receipts, and terminal run evidence.
type Repository interface {
	CreateSuite(context.Context, store.Scope, SuiteRecord) error
	SaveInput(context.Context, store.Scope, ProtectedRef, LiveInput) error
	ReviewSuite(context.Context, store.Scope, SuiteReview) (SuiteRecord, error)
	AcceptedSuite(context.Context, store.Scope, string, int64, string) (SuiteRecord, error)
	BeginRun(context.Context, store.Scope, RunRequest) error
	SaveReport(context.Context, store.Scope, Report) error
	ReadReport(context.Context, store.Scope, string) (Report, error)
	RequestCancel(context.Context, store.Scope, string) error
	RecoverRun(context.Context, store.Scope, string, time.Time) (Report, error)
	SaveFeedbackExport(context.Context, store.Scope, CandidateExport) error
	ReadFeedbackExport(context.Context, store.Scope, string) (CandidateExport, error)
	ReviewFeedbackSplit(context.Context, store.Scope, string, string, FeedbackSplit) error
	ValidateHeldoutCases(context.Context, store.Scope, []Case) error
	ValidateOptimizationHeldout(context.Context, store.Scope, Suite) error
	SaveProposal(context.Context, store.Scope, OptimizationProposal) error
	ReadProposal(context.Context, store.Scope, string) (OptimizationProposal, error)
	ReviewProposal(context.Context, store.Scope, ReviewReceipt) error
	SelectPack(context.Context, store.Scope, PackSelection, int64) (PackSelection, error)
	SelectedPack(context.Context, store.Scope) (PackSelection, error)
}

// RegisterInput stores protected live material and returns its canonical reference.
func (s *Service) RegisterInput(ctx context.Context, e identity.Envelope, retention string, in LiveInput) (ProtectedRef, error) {
	if ctx == nil || !identifier(retention) || !validPack(in.Pack) {
		return ProtectedRef{}, ErrInvalid
	}
	d, err := digest(in)
	if err != nil {
		return ProtectedRef{}, ErrInvalid
	}
	ref := ProtectedRef{Digest: d, Retention: retention}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return ProtectedRef{}, err
	}
	if err = s.repo.SaveInput(ctx, scope, ref, in); err != nil {
		return ProtectedRef{}, err
	}
	return ref, nil
}

// FeedbackSource reads reviewed learning evidence under the original authority.
type FeedbackSource interface {
	ReviewedFeedback(context.Context, identity.Envelope, string, int) ([]FeedbackEvidence, error)
}

// FeedbackEvidence is the content-free projection of reviewed learning state.
type FeedbackEvidence struct {
	ID, Locale, InputDigest, ExpectedDigest, Decision, SourceBindingDigest string
	CreatedAt                                                              time.Time
}

// CandidateExport is an immutable feedback or assigned split ledger.
type CandidateExport struct {
	SchemaVersion int        `json:"schema_version"`
	ID            string     `json:"id"`
	Status        string     `json:"status"`
	Split         string     `json:"split"`
	Cases         []Case     `json:"cases"`
	EvidenceHash  string     `json:"evidence_hash"`
	ParentDigest  string     `json:"parent_digest,omitempty"`
	Author        string     `json:"author"`
	Reviewer      string     `json:"reviewer,omitempty"`
	ReviewedAt    *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (x CandidateExport) validate() error {
	if x.SchemaVersion != SchemaVersion || !identifier(x.ID) || !identifier(x.Author) || !validDigest(x.EvidenceHash) || x.CreatedAt.IsZero() || len(x.Cases) == 0 || (x.Split != "candidate" && x.Split != "training" && x.Split != "heldout") {
		return ErrInvalid
	}
	if x.Split == "candidate" && (x.Status != "pending" || x.ParentDigest != "" || x.Reviewer != "" || x.ReviewedAt != nil) {
		return ErrInvalid
	}
	if (x.Split == "training" || x.Split == "heldout") && (x.Status != "reviewed" || !validDigest(x.ParentDigest) || !identifier(x.Reviewer) || x.ReviewedAt == nil) {
		return ErrInvalid
	}
	for _, c := range x.Cases {
		if !validDigest(c.Input.Digest) || c.HeldOut != (x.Split == "heldout") {
			return ErrInvalid
		}
	}
	return nil
}

// FeedbackSplit is an independently assigned, disjoint and immutable partition.
type FeedbackSplit struct {
	CandidateID      string          `json:"candidate_id"`
	CandidateDigest  string          `json:"candidate_digest"`
	AssignmentDigest string          `json:"assignment_digest"`
	Training         CandidateExport `json:"training"`
	Heldout          CandidateExport `json:"heldout"`
}

func (x FeedbackSplit) validate() error {
	if !identifier(x.CandidateID) || !validDigest(x.CandidateDigest) || !validDigest(x.AssignmentDigest) || x.Training.validate() != nil || x.Heldout.validate() != nil || x.Training.ParentDigest != x.CandidateDigest || x.Heldout.ParentDigest != x.CandidateDigest || x.Training.Reviewer != x.Heldout.Reviewer {
		return ErrInvalid
	}
	seen, digests := map[string]bool{}, map[string]bool{}
	for _, c := range x.Training.Cases {
		seen[c.ID] = true
		digests[c.Input.Digest] = true
	}
	for _, c := range x.Heldout.Cases {
		if seen[c.ID] || digests[c.Input.Digest] {
			return ErrInvalid
		}
		seen[c.ID] = true
	}
	want, _ := digest(struct{ Candidate, Training, Heldout string }{x.CandidateDigest, x.Training.EvidenceHash, x.Heldout.EvidenceHash})
	if want != x.AssignmentDigest {
		return ErrInvalid
	}
	return nil
}

// ValidateSplit checks immutable assignment evidence at persistence boundaries.
func (x FeedbackSplit) ValidateSplit() error { return x.validate() }

// ValidateExport checks immutable feedback split lineage without exposing content.
func (x CandidateExport) ValidateExport() error { return x.validate() }

// Service owns evaluated suite and optimization lifecycles.
type Service struct {
	repo     Repository
	feedback FeedbackSource
	clock    Clock
	mu       sync.Mutex
	running  map[string]context.CancelFunc
}

// New constructs the authority-bound service.
func New(repo Repository, feedback FeedbackSource, clock Clock) (*Service, error) {
	if repo == nil {
		return nil, ErrInvalid
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, feedback: feedback, clock: clock, running: map[string]context.CancelFunc{}}, nil
}

// Author stores an immutable draft. Signed actor attribution is not accepted in input.
func (s *Service) Author(ctx context.Context, e identity.Envelope, suite Suite) (SuiteRecord, error) {
	if ctx == nil || suite.Validate() != nil {
		return SuiteRecord{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return SuiteRecord{}, err
	}
	if err = s.repo.ValidateHeldoutCases(ctx, scope, suite.Cases); err != nil {
		return SuiteRecord{}, err
	}
	d, _ := suite.Digest()
	r := SuiteRecord{Suite: suite, Digest: d, State: Draft, Author: e.User(), CreatedAt: s.clock().UTC()}
	if err = s.repo.CreateSuite(ctx, scope, r); err != nil {
		return SuiteRecord{}, err
	}
	return r, nil
}

// Review records a distinct authenticated decision over an exact draft revision.
func (s *Service) Review(ctx context.Context, e identity.Envelope, id string, in SuiteReviewRequest) (SuiteRecord, error) {
	if ctx == nil || !identifier(id) || in.Revision < 1 || !validDigest(in.Digest) || (in.Decision != Accepted && in.Decision != Rejected) {
		return SuiteRecord{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.audit", "certify")
	if err != nil {
		return SuiteRecord{}, err
	}
	return s.repo.ReviewSuite(ctx, scope, SuiteReview{SuiteID: id, Revision: in.Revision, Digest: in.Digest, Decision: in.Decision, Reviewer: e.User(), ReviewedAt: s.clock().UTC()})
}

// Run loads the exact accepted revision from protected storage before any work.
func (s *Service) Run(ctx context.Context, e identity.Envelope, in RunRequest, runner Runner) (Report, error) {
	if ctx == nil || !identifier(in.RunID) || !identifier(in.SuiteID) || in.SuiteRevision < 1 || !validDigest(in.SuiteDigest) || in.PackDigest != "" && !validDigest(in.PackDigest) {
		return Report{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return Report{}, err
	}
	record, err := s.repo.AcceptedSuite(ctx, scope, in.SuiteID, in.SuiteRevision, in.SuiteDigest)
	if err != nil {
		return Report{}, err
	}
	if record.State != Accepted || record.Review == nil || record.Review.Digest != record.Digest || record.Review.Reviewer == record.Author {
		return Report{}, ErrReview
	}
	if record.Suite.Threshold.QualityMin == nil {
		return Report{}, ErrReview
	}
	if in.PackDigest == "" {
		selected, selectErr := s.repo.SelectedPack(ctx, scope)
		if selectErr != nil {
			return Report{}, selectErr
		}
		in.PackDigest = selected.PackDigest
	}
	pack, ok := record.Suite.pack(in.PackDigest)
	if !ok {
		return Report{}, ErrReview
	}
	if err = s.repo.BeginRun(ctx, scope, in); err != nil {
		return Report{}, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	if _, exists := s.running[in.RunID]; exists {
		s.mu.Unlock()
		cancel()
		return Report{}, store.ErrConflict
	}
	s.running[in.RunID] = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); delete(s.running, in.RunID); s.mu.Unlock() }()
	if runner != nil {
		runner = authorityRunner{Envelope: e, Next: runner}
	}
	r, evalErr := EvaluateWithPack(runCtx, in.RunID, record.Suite, pack, runner, s.clock)
	if r.EvidenceHash != "" {
		if saveErr := s.repo.SaveReport(context.WithoutCancel(ctx), scope, r); saveErr != nil {
			return Report{}, saveErr
		}
	}
	return r, evalErr
}

type authorityRunner struct {
	Envelope identity.Envelope
	Next     Runner
}

func (a authorityRunner) Observe(ctx context.Context, x Execution) (Observation, error) {
	x.Envelope = a.Envelope
	return a.Next.Observe(ctx, x)
}
func (s *Service) Read(ctx context.Context, e identity.Envelope, id string) (Report, error) {
	scope, err := access.StoreScope(e, "ops.read", "read")
	if err != nil {
		return Report{}, err
	}
	return s.repo.ReadReport(ctx, scope, id)
}

// Cancel persists intent before signalling a locally owned run.
func (s *Service) Cancel(ctx context.Context, e identity.Envelope, id string) error {
	if !identifier(id) {
		return ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return err
	}
	if err = s.repo.RequestCancel(ctx, scope, id); err != nil {
		return err
	}
	s.mu.Lock()
	cancel := s.running[id]
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

// Recover marks an abandoned admitted run as a typed terminal dependency failure.
func (s *Service) Recover(ctx context.Context, e identity.Envelope, id string) (Report, error) {
	scope, err := access.StoreScope(e, "ops.maintain", "write")
	if err != nil {
		return Report{}, err
	}
	return s.repo.RecoverRun(ctx, scope, id, s.clock().UTC())
}

// ExportFeedback emits an immutable unassigned candidate ledger. Training and heldout assignment require a later independent split review.
func (s *Service) ExportFeedback(ctx context.Context, e identity.Envelope, id, topic string, limit int) (CandidateExport, error) {
	if s.feedback == nil || !identifier(id) || !identifier(topic) || limit < 1 || limit > 1000 {
		return CandidateExport{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.read", "export")
	if err != nil {
		return CandidateExport{}, err
	}
	rows, err := s.feedback.ReviewedFeedback(ctx, e, topic, limit)
	if err != nil {
		return CandidateExport{}, err
	}
	out := CandidateExport{SchemaVersion: SchemaVersion, ID: id, Status: "pending", Split: "candidate", Cases: []Case{}, Author: e.User(), CreatedAt: s.clock().UTC()}
	for _, r := range rows {
		if !identifier(r.ID) || !validDigest(r.InputDigest) || !validDigest(r.ExpectedDigest) || !validDigest(r.SourceBindingDigest) || (r.Locale != "en" && r.Locale != "es") {
			return CandidateExport{}, ErrInvalid
		}
		out.Cases = append(out.Cases, Case{ID: r.ID, Stage: StageSQL, Locale: r.Locale, HeldOut: false, Input: ProtectedRef{Digest: r.InputDigest, Retention: "protected_feedback"}, BindingDigest: r.SourceBindingDigest, Expected: []Expected{{Decision: r.Decision, SemanticDigest: r.ExpectedDigest}}})
	}
	out.EvidenceHash, _ = digest(struct {
		ID, Split string
		Cases     []Case
	}{out.ID, out.Split, out.Cases})
	if out.validate() != nil {
		return CandidateExport{}, ErrInvalid
	}
	if err = s.repo.SaveFeedbackExport(ctx, scope, out); err != nil {
		return CandidateExport{}, err
	}
	return out, nil
}

// ReviewFeedbackSplit independently partitions a pending candidate before either case set becomes training evidence.
func (s *Service) ReviewFeedbackSplit(ctx context.Context, e identity.Envelope, candidateID, candidateDigest, trainingID, heldoutID string, heldoutCaseIDs []string) (FeedbackSplit, error) {
	if !identifier(candidateID) || !validDigest(candidateDigest) || !identifier(trainingID) || !identifier(heldoutID) || candidateID == trainingID || candidateID == heldoutID || trainingID == heldoutID || len(heldoutCaseIDs) == 0 {
		return FeedbackSplit{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.audit", "certify")
	if err != nil {
		return FeedbackSplit{}, err
	}
	candidate, err := s.repo.ReadFeedbackExport(ctx, scope, candidateID)
	if err != nil {
		return FeedbackSplit{}, err
	}
	if candidate.Split != "candidate" || candidate.Status != "pending" || candidate.EvidenceHash != candidateDigest || candidate.Author == e.User() {
		return FeedbackSplit{}, ErrReview
	}
	heldoutIDs := map[string]bool{}
	for _, id := range heldoutCaseIDs {
		if !identifier(id) || heldoutIDs[id] {
			return FeedbackSplit{}, ErrInvalid
		}
		heldoutIDs[id] = true
	}
	now := s.clock().UTC()
	training := CandidateExport{SchemaVersion: SchemaVersion, ID: trainingID, Status: "reviewed", Split: "training", ParentDigest: candidate.EvidenceHash, Author: candidate.Author, Reviewer: e.User(), ReviewedAt: &now, CreatedAt: now}
	heldout := CandidateExport{SchemaVersion: SchemaVersion, ID: heldoutID, Status: "reviewed", Split: "heldout", ParentDigest: candidate.EvidenceHash, Author: candidate.Author, Reviewer: e.User(), ReviewedAt: &now, CreatedAt: now}
	for _, c := range candidate.Cases {
		if heldoutIDs[c.ID] {
			c.HeldOut = true
			heldout.Cases = append(heldout.Cases, c)
			delete(heldoutIDs, c.ID)
		} else {
			c.HeldOut = false
			training.Cases = append(training.Cases, c)
		}
	}
	if len(heldoutIDs) != 0 || len(training.Cases) == 0 || len(heldout.Cases) == 0 {
		return FeedbackSplit{}, ErrInvalid
	}
	training.EvidenceHash, _ = digest(struct {
		ID, Split, Parent string
		Cases             []Case
		Reviewer          string
	}{training.ID, training.Split, training.ParentDigest, training.Cases, training.Reviewer})
	heldout.EvidenceHash, _ = digest(struct {
		ID, Split, Parent string
		Cases             []Case
		Reviewer          string
	}{heldout.ID, heldout.Split, heldout.ParentDigest, heldout.Cases, heldout.Reviewer})
	out := FeedbackSplit{CandidateID: candidateID, CandidateDigest: candidateDigest, Training: training, Heldout: heldout}
	out.AssignmentDigest, _ = digest(struct{ Candidate, Training, Heldout string }{candidateDigest, training.EvidenceHash, heldout.EvidenceHash})
	if out.validate() != nil {
		return FeedbackSplit{}, ErrInvalid
	}
	if err = s.repo.ReviewFeedbackSplit(ctx, scope, candidateID, candidateDigest, out); err != nil {
		return FeedbackSplit{}, err
	}
	return out, nil
}

// ProposeOptimization loads all evidence from protected storage and persists the proposal.
func (s *Service) ProposeOptimization(ctx context.Context, e identity.Envelope, in ProposalRequest) (OptimizationProposal, error) {
	if !identifier(in.ID) || !identifier(in.BaselineRun) || !identifier(in.CandidateRun) {
		return OptimizationProposal{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return OptimizationProposal{}, err
	}
	suite, err := s.repo.AcceptedSuite(ctx, scope, in.SuiteID, in.SuiteRevision, in.SuiteDigest)
	if err != nil {
		return OptimizationProposal{}, err
	}
	base, err := s.repo.ReadReport(ctx, scope, in.BaselineRun)
	if err != nil {
		return OptimizationProposal{}, err
	}
	candidate, err := s.repo.ReadReport(ctx, scope, in.CandidateRun)
	if err != nil {
		return OptimizationProposal{}, err
	}
	if err = s.repo.ValidateOptimizationHeldout(ctx, scope, suite.Suite); err != nil {
		return OptimizationProposal{}, err
	}
	p, err := ProposeOptimization(in.ID, suite.Suite, base, candidate, s.clock())
	if err != nil {
		return OptimizationProposal{}, err
	}
	if err = s.repo.SaveProposal(ctx, scope, p); err != nil {
		return OptimizationProposal{}, err
	}
	return p, nil
}

// ReviewOptimization derives reviewer identity solely from verified authority.
func (s *Service) ReviewOptimization(ctx context.Context, e identity.Envelope, id string, in ProposalReviewRequest) (ReviewReceipt, error) {
	scope, err := access.StoreScope(e, "ops.audit", "certify")
	if err != nil {
		return ReviewReceipt{}, err
	}
	p, err := s.repo.ReadProposal(ctx, scope, id)
	if err != nil {
		return ReviewReceipt{}, err
	}
	d, _ := digest(p)
	if d != in.Digest {
		return ReviewReceipt{}, store.ErrConflict
	}
	receipt, err := proposalReceipt(p, e.User(), in.Decision, s.clock())
	if err != nil {
		return ReviewReceipt{}, err
	}
	if err = s.repo.ReviewProposal(ctx, scope, receipt); err != nil {
		return ReviewReceipt{}, err
	}
	return receipt, nil
}

// SelectPack advances a CAS pointer only for a persisted approved proposal. Passing an older approved proposal performs an auditable rollback.
func (s *Service) SelectPack(ctx context.Context, e identity.Envelope, proposalID string, expected int64) (PackSelection, error) {
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return PackSelection{}, err
	}
	p, err := s.repo.ReadProposal(ctx, scope, proposalID)
	if err != nil {
		return PackSelection{}, err
	}
	return s.repo.SelectPack(ctx, scope, PackSelection{PackDigest: p.Candidate.PackDigest, ProposalID: p.ID, Actor: e.User(), SelectedAt: s.clock().UTC()}, expected)
}
