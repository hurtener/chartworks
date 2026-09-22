package evaluation

import (
	"context"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Repository persists protected immutable evaluation evidence.
type Repository interface {
	SaveSuite(context.Context, store.Scope, Suite, string) error
	SaveReport(context.Context, store.Scope, Report) error
	ReadReport(context.Context, store.Scope, string) (Report, error)
}

// FeedbackSource reads reviewed feedback under store scope.
type FeedbackSource interface {
	ReviewedFeedback(context.Context, store.Scope, int) ([]FeedbackEvidence, error)
}

// FeedbackEvidence is content-addressed reviewed learning evidence.
type FeedbackEvidence struct {
	ID, Locale, InputDigest, ExpectedDigest, Decision, SourceBindingDigest string
	CreatedAt                                                              time.Time
}

// CandidateExport is never an active gated suite.
type CandidateExport struct {
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Cases         []Case `json:"cases"`
	EvidenceHash  string `json:"evidence_hash"`
}

// Service applies signed authority before persistence or export.
type Service struct {
	repo     Repository
	feedback FeedbackSource
	clock    Clock
}

// New constructs an authority-bound evaluation service.
func New(repo Repository, feedback FeedbackSource, clock Clock) (*Service, error) {
	if repo == nil {
		return nil, ErrInvalid
	}
	if clock == nil {
		clock = time.Now
	}
	return &Service{repo: repo, feedback: feedback, clock: clock}, nil
}

// Run persists a suite and its completed reproducible report.
func (s *Service) Run(ctx context.Context, e identity.Envelope, runID string, suite Suite, runner Runner) (Report, error) {
	if suite.Validate() != nil {
		return Report{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.write", "write")
	if err != nil {
		return Report{}, err
	}
	d, _ := digest(suite)
	if err = s.repo.SaveSuite(ctx, scope, suite, d); err != nil {
		return Report{}, err
	}
	r, evalErr := Evaluate(ctx, runID, suite, runner, s.clock)
	if r.EvidenceHash == "" {
		return r, evalErr
	}
	if err = s.repo.SaveReport(ctx, scope, r); err != nil {
		return Report{}, err
	}
	return r, evalErr
}

// Read returns one exact actor-scoped evaluation report.
func (s *Service) Read(ctx context.Context, e identity.Envelope, runID string) (Report, error) {
	scope, err := access.StoreScope(e, "ops.read", "read")
	if err != nil {
		return Report{}, err
	}
	return s.repo.ReadReport(ctx, scope, runID)
}

// ExportFeedback emits content-addressed candidate cases. It cannot activate a suite.
func (s *Service) ExportFeedback(ctx context.Context, e identity.Envelope, limit int) (CandidateExport, error) {
	if s.feedback == nil || limit < 1 || limit > 1000 {
		return CandidateExport{}, ErrInvalid
	}
	scope, err := access.StoreScope(e, "ops.read", "export")
	if err != nil {
		return CandidateExport{}, err
	}
	rows, err := s.feedback.ReviewedFeedback(ctx, scope, limit)
	if err != nil {
		return CandidateExport{}, err
	}
	out := CandidateExport{SchemaVersion: SchemaVersion, Status: "candidate", Cases: []Case{}}
	for _, r := range rows {
		if !identifier(r.ID) || !validDigest(r.InputDigest) || !validDigest(r.ExpectedDigest) || !validDigest(r.SourceBindingDigest) || (r.Locale != "en" && r.Locale != "es") {
			return CandidateExport{}, ErrInvalid
		}
		out.Cases = append(out.Cases, Case{ID: r.ID, Stage: StageSQL, Locale: r.Locale, HeldOut: true, Input: ProtectedRef{Digest: r.InputDigest, Retention: "protected_feedback"}, Expected: []Expected{{Decision: r.Decision, SemanticDigest: r.ExpectedDigest}}})
		out.Cases[len(out.Cases)-1].BindingDigest = r.SourceBindingDigest
	}
	out.EvidenceHash, _ = digest(out.Cases)
	return out, nil
}
