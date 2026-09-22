package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
)

// Repository provides CAS-fenced, tenant/actor/session-partitioned persistence.
type Repository interface {
	CreateOnboarding(context.Context, identity.Envelope, Run, string) (Run, error)
	ReadOnboarding(context.Context, identity.Envelope, string) (Run, error)
	SaveOnboarding(context.Context, identity.Envelope, Run, int64) (Run, error)
}

// Adapter delegates each effect to its existing domain owner. Every operation key
// is stable across retries and must reconcile an already committed effect.
type Adapter interface {
	Connect(context.Context, identity.Envelope, StartRequest, string) (StepResult, error)
	Inspect(context.Context, identity.Envelope, Run, string) (StepResult, error)
	Profile(context.Context, identity.Envelope, Run, string) (StepResult, error)
	DraftSemantics(context.Context, identity.Envelope, Run, string) (StepResult, error)
	PublishReviewed(context.Context, identity.Envelope, Run, ReviewReference, string) (StepResult, error)
	ProposeQueriesBlocksReports(context.Context, identity.Envelope, Run, string) (StepResult, error)
	ProposeDriftAmendment(context.Context, identity.Envelope, Run, DriftRequest, string) (Amendment, error)
}

type Service struct {
	repo    Repository
	adapter Adapter
	limits  Limits
	now     func() time.Time
}

func New(repo Repository, adapter Adapter, limits Limits) (*Service, error) {
	if repo == nil || adapter == nil || !limits.valid() {
		return nil, ErrInvalid
	}
	return &Service{repo: repo, adapter: adapter, limits: limits, now: time.Now}, nil
}

func require(e identity.Envelope, action, id, permission string) error {
	if !e.Valid() {
		return access.ErrUnauthenticated
	}
	if id == "" {
		return access.Require(e, action, access.Tenant(e, permission))
	}
	return access.Require(e, action, access.Resource{Tenant: e.Tenant(), Kind: "onboarding", Permission: permission, ID: id})
}

func requireDependencies(e identity.Envelope, action string, r Run) error {
	return access.Require(e, action,
		access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: r.Input.Source},
		access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: r.Input.Context},
	)
}

func validText(v string, max int) bool {
	if strings.TrimSpace(v) == "" || len(v) > max {
		return false
	}
	for _, r := range v {
		if r < 32 && r != '\n' && r != '\t' || r == 127 {
			return false
		}
	}
	return true
}

func validateStart(in StartRequest) bool {
	if !identity.Identifier(in.ID) || !identity.Identifier(in.Key) || (in.Mode != ModeConnect && in.Mode != ModeUpload) || (in.Locale != "en" && in.Locale != "es") {
		return false
	}
	for _, v := range []string{in.Source, in.Context, in.Dataset, in.Profile, in.Topic, in.TopicVersion, in.Block, in.Report} {
		if !identity.Identifier(v) {
			return false
		}
	}
	if in.Mode == ModeUpload && !identity.Identifier(in.Upload) {
		return false
	}
	return !in.Transformation && in.TransformationProposal == "" || in.Transformation && identity.Identifier(in.TransformationProposal)
}

func requestDigest(in StartRequest) string {
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func valueDigest(in any) string {
	raw, _ := json.Marshal(in)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func operationKey(r Run, stage Stage) string { return r.ID + "-" + string(stage) + "-v1" }

func closedDecision(v Answer) bool {
	if !identity.Identifier(v.ID) || (v.Decision != "confirmed_external" && v.Decision != "unresolved" && v.Decision != "not_applicable") {
		return false
	}
	return v.Reference == "" || identity.Identifier(v.Reference)
}

func cancelReason(v string) bool {
	return v == "user_requested" || v == "superseded" || v == "incorrect_source" || v == "budget"
}

func leaseReservation(r Run, stage Stage) (int, int, error) {
	if stage != StageReview {
		return 0, 0, nil
	}
	calls, tokens := r.Limits.MaxModelCalls-r.Usage.ModelCalls, r.Limits.MaxTokens-r.Usage.Tokens
	if calls < 1 || tokens < 1 {
		return 0, 0, ErrBudget
	}
	// Reserve the remaining token ceiling for the one publication embedding
	// attempt. A crash retains this non-refundable charge and permits
	// reconciliation only, so the same allowance can never fund a blind replay.
	return 1, tokens, nil
}

func newLease(r Run, inputDigest string) (*Lease, error) {
	calls, tokens, err := leaseReservation(r, r.Stage)
	if err != nil {
		return nil, err
	}
	operation := operationKey(r, r.Stage)
	sum := sha256.Sum256([]byte(operation + "\x00" + inputDigest + "\x00" + strconv.FormatInt(r.Version, 10)))
	return &Lease{Stage: r.Stage, Operation: operation, Fence: hex.EncodeToString(sum[:16]), InputDigest: inputDigest, ReservedCalls: calls, ReservedTokens: tokens, Charged: stageIsModel(r.Stage)}, nil
}

func stageIsModel(stage Stage) bool { return stage == StageReview }

func delegatedUsage(step StepResult, lease *Lease) (Usage, *DelegatedReceipt, error) {
	usage := Usage{Entities: len(step.References) + len(step.Evidence)}
	receipt := DelegatedReceipt{}
	if lease != nil {
		receipt.Operation = lease.Operation
	}
	// A review lease is a non-refundable pessimistic reservation. Account the
	// reservation rather than trusting a delegated adapter or provider's token
	// self-report; this also closes crash/retry accounting when only an uncertain
	// or recovered immutable publication receipt is available.
	if lease != nil && lease.Stage == StageReview {
		receipt.Calls = lease.ReservedCalls
		receipt.Tokens = lease.ReservedTokens
		receipt.UnknownTokens = true
		receipt.Reconciled = true
		return usage, &receipt, nil
	}
	for _, call := range step.Receipt.Calls {
		attempts := call.Attempts
		if attempts < 1 {
			attempts = 1
		}
		usage.ModelCalls += attempts
		if call.InputTokens == nil || call.OutputTokens == nil {
			receipt.UnknownTokens = true
			continue
		}
		if *call.InputTokens < 0 || *call.OutputTokens < 0 {
			return Usage{}, nil, ErrInvalid
		}
		usage.Tokens += *call.InputTokens + *call.OutputTokens
	}
	if receipt.UnknownTokens {
		if lease == nil || lease.ReservedTokens < usage.Tokens {
			return Usage{}, nil, ErrBudget
		}
		usage.Tokens = lease.ReservedTokens
	}
	if lease == nil && usage.ModelCalls > 0 || lease != nil && (usage.ModelCalls > lease.ReservedCalls || usage.Tokens > lease.ReservedTokens) {
		return Usage{}, nil, ErrBudget
	}
	receipt.Calls, receipt.Tokens = usage.ModelCalls, usage.Tokens
	if receipt.Calls == 0 && receipt.Tokens == 0 && !receipt.UnknownTokens {
		return usage, nil, nil
	}
	return usage, &receipt, nil
}

func applyStep(r *Run, step StepResult, usage Usage, receipt *DelegatedReceipt) {
	r.References = appendUnique(r.References, step.References...)
	r.Evidence = appendEvidence(r.Evidence, step.Evidence...)
	for _, ref := range step.References {
		if (ref.Kind == "dataset" || ref.Kind == "profile") && ref.Revision > r.SourceRevision {
			r.SourceRevision = ref.Revision
		}
	}
	r.Questions = step.Questions
	r.Usage = addUsage(r.Usage, usage)
	r.Usage.Stages++
	if receipt != nil {
		setDelegatedReceipt(r, *receipt)
	}
}

func setDelegatedReceipt(r *Run, receipt DelegatedReceipt) {
	for i := range r.Receipts {
		if r.Receipts[i].Operation == receipt.Operation {
			r.Receipts[i] = receipt
			return
		}
	}
	r.Receipts = append(r.Receipts, receipt)
}

func (s *Service) Start(ctx context.Context, e identity.Envelope, in StartRequest) (Run, error) {
	if ctx == nil || !validateStart(in) {
		return Run{}, ErrInvalid
	}
	if err := require(e, "onboarding.write", "", "write"); err != nil {
		return Run{}, err
	}
	if err := requireDependencies(e, "onboarding.write", Run{Input: in}); err != nil {
		return Run{}, err
	}
	now := s.now().UTC()
	run := Run{ID: in.ID, Key: in.Key, Version: 1, Stage: StageConnect, Status: StatusReady, Locale: in.Locale, Message: message(in.Locale, StageConnect), Input: in, References: []Reference{}, Evidence: []Evidence{}, Questions: []Question{}, Answers: []Answer{}, Receipts: []DelegatedReceipt{}, Amendments: []Amendment{}, Limits: s.limits, Progress: progress(StageConnect), CreatedAt: now, UpdatedAt: now, Deadline: now.Add(s.limits.MaxDuration)}
	return s.repo.CreateOnboarding(ctx, e, run, requestDigest(in))
}

func (s *Service) Get(ctx context.Context, e identity.Envelope, id string) (Run, error) {
	if ctx == nil || !identity.Identifier(id) {
		return Run{}, ErrInvalid
	}
	if err := require(e, "onboarding.read", id, "read"); err != nil {
		return Run{}, err
	}
	return s.repo.ReadOnboarding(ctx, e, id)
}

func (s *Service) Resume(ctx context.Context, e identity.Envelope, id string, in ResumeRequest) (Run, error) {
	if ctx == nil || !identity.Identifier(id) || in.ID != "" && in.ID != id || in.ExpectedVersion < 1 {
		return Run{}, ErrInvalid
	}
	if err := require(e, "onboarding.write", id, "write"); err != nil {
		return Run{}, err
	}
	r, err := s.repo.ReadOnboarding(ctx, e, id)
	if err != nil {
		return Run{}, err
	}
	if err = requireDependencies(e, "onboarding.write", r); err != nil {
		return Run{}, err
	}
	if r.Version != in.ExpectedVersion {
		return Run{}, store.ErrConflict
	}
	if r.Status == StatusCancelled {
		return Run{}, ErrCancelled
	}
	if r.Status == StatusComplete {
		return r, nil
	}
	if !s.now().Before(r.Deadline) {
		return Run{}, ErrBudget
	}
	if r.Usage.Stages >= r.Limits.MaxStages {
		return Run{}, ErrBudget
	}
	if len(r.Questions) > 0 && r.Status == StatusAttention {
		return r, ErrAttention
	}
	if r.Stage == StageReview {
		return attention(r, "review_topic", s.now()), ErrAttention
	}
	if r.Lease == nil {
		r.Lease, err = newLease(r, "")
		if err != nil {
			return Run{}, err
		}
		r.Status = StatusRunning
		r.RequiredAction = ""
		r.UpdatedAt = s.now().UTC()
		r, err = s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
		if err != nil {
			return Run{}, err
		}
	} else if r.Lease.Stage != r.Stage || r.Lease.Operation != operationKey(r, r.Stage) || r.Lease.InputDigest != "" {
		return Run{}, store.ErrConflict
	}
	expected := r.Version
	var step StepResult
	switch r.Stage {
	case StageConnect:
		step, err = s.adapter.Connect(ctx, e, r.Input, r.Lease.Operation)
	case StageInspect:
		step, err = s.adapter.Inspect(ctx, e, r, r.Lease.Operation)
	case StageProfile:
		step, err = s.adapter.Profile(ctx, e, r, r.Lease.Operation)
	case StageSemantic:
		step, err = s.adapter.DraftSemantics(ctx, e, r, r.Lease.Operation)
	case StageProposals:
		step, err = s.adapter.ProposeQueriesBlocksReports(ctx, e, r, r.Lease.Operation)
	default:
		return Run{}, ErrInvalid
	}
	if err != nil {
		r.Status = StatusFailed
		r.Message = statusMessage(r.Locale, "failed")
		r.UpdatedAt = s.now().UTC()
		saved, saveErr := s.repo.SaveOnboarding(ctx, e, r, expected)
		if saveErr != nil {
			return Run{}, saveErr
		}
		return saved, err
	}
	usage, receipt, err := delegatedUsage(step, r.Lease)
	if err != nil {
		return Run{}, err
	}
	if err = validateStep(step, s.limits, r.Usage, usage); err != nil {
		return Run{}, err
	}
	applyStep(&r, step, usage, receipt)
	r.Lease = nil
	if r.CancelRequested {
		r.Status = StatusCancelled
		r.RequiredAction = ""
		r.Questions = []Question{}
		r.Message = statusMessage(r.Locale, "cancelled")
		r.UpdatedAt = s.now().UTC()
		return s.repo.SaveOnboarding(ctx, e, r, expected)
	}
	if r.Stage == StageProfile && hasQuestion(step.Questions, "approve_transformation") {
		r.RequiredAction = "review_transformation"
		r.Status = StatusAttention
	} else if len(r.Questions) > 0 {
		r.RequiredAction = "answer_questions"
		r.Status = StatusAttention
	} else {
		r.Stage = next(r.Stage)
		r.Status = StatusReady
		if r.Stage == StageReview {
			r.RequiredAction = "review_topic"
			r.Status = StatusAttention
		}
		if r.Stage == StageComplete {
			r.RequiredAction = "review_blocks"
			r.Status = StatusComplete
		}
	}
	r.Message = message(r.Locale, r.Stage)
	r.Progress = progress(r.Stage)
	r.UpdatedAt = s.now().UTC()
	return s.repo.SaveOnboarding(ctx, e, r, expected)
}

func (s *Service) Answer(ctx context.Context, e identity.Envelope, id string, in AnswerRequest) (Run, error) {
	if ctx == nil || !identity.Identifier(id) || in.ID != "" && in.ID != id || in.ExpectedVersion < 1 || len(in.Answers) > 64 {
		return Run{}, ErrInvalid
	}
	if err := require(e, "onboarding.write", id, "write"); err != nil {
		return Run{}, err
	}
	r, err := s.repo.ReadOnboarding(ctx, e, id)
	if err != nil {
		return Run{}, err
	}
	if err = requireDependencies(e, "onboarding.write", r); err != nil {
		return Run{}, err
	}
	if r.Version != in.ExpectedVersion {
		return Run{}, store.ErrConflict
	}
	if r.Status == StatusCancelled {
		return Run{}, ErrCancelled
	}
	if !s.now().Before(r.Deadline) {
		return Run{}, ErrBudget
	}
	if r.Stage == StageReview {
		if in.Review == nil || !identity.Identifier(in.Review.ID) || in.Review.Revision < 1 || len(in.Review.Digest) != 64 {
			return Run{}, ErrInvalid
		}
		digest := valueDigest(in.Review)
		reconcileOnly := r.Lease != nil
		if r.Lease == nil {
			r.Lease, err = newLease(r, digest)
			if err != nil {
				return Run{}, err
			}
			if r.Lease.Charged {
				r.Usage.ModelCalls += r.Lease.ReservedCalls
				r.Usage.Tokens += r.Lease.ReservedTokens
				setDelegatedReceipt(&r, DelegatedReceipt{Operation: r.Lease.Operation, Calls: r.Lease.ReservedCalls, Tokens: r.Lease.ReservedTokens, UnknownTokens: true, Reconciled: false})
			}
			r.Status = StatusRunning
			r.RequiredAction = ""
			r.UpdatedAt = s.now().UTC()
			r, err = s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
			if err != nil {
				return Run{}, err
			}
		} else if r.Lease.Stage != StageReview || r.Lease.Operation != operationKey(r, StageReview) || r.Lease.InputDigest != digest {
			return Run{}, store.ErrConflict
		}
		expected := r.Version
		if reconcileOnly {
			ctx = context.WithValue(ctx, reconciliationKey{}, true)
		}
		step, e2 := s.adapter.PublishReviewed(ctx, e, r, *in.Review, r.Lease.Operation)
		if e2 != nil {
			setDelegatedReceipt(&r, DelegatedReceipt{Operation: r.Lease.Operation, Calls: r.Lease.ReservedCalls, Tokens: r.Lease.ReservedTokens, UnknownTokens: true, Reconciled: false})
			r.Status = StatusFailed
			r.Message = statusMessage(r.Locale, "failed")
			r.UpdatedAt = s.now().UTC()
			saved, saveErr := s.repo.SaveOnboarding(ctx, e, r, expected)
			if saveErr != nil {
				return Run{}, saveErr
			}
			return saved, e2
		}
		usage, receipt, e2 := delegatedUsage(step, r.Lease)
		if e2 != nil {
			return Run{}, e2
		}
		if e2 = validateStep(step, s.limits, r.Usage, usage); e2 != nil {
			return Run{}, e2
		}
		applyStep(&r, step, usage, receipt)
		r.Lease = nil
		if r.CancelRequested {
			r.Status = StatusCancelled
			r.RequiredAction = ""
			r.Questions = []Question{}
			r.Message = statusMessage(r.Locale, "cancelled")
			r.UpdatedAt = s.now().UTC()
			return s.repo.SaveOnboarding(ctx, e, r, expected)
		}
		r.Stage = StageProposals
		r.Status = StatusReady
		r.RequiredAction = ""
		r.Message = message(r.Locale, r.Stage)
		r.Progress = progress(r.Stage)
		r.UpdatedAt = s.now().UTC()
		return s.repo.SaveOnboarding(ctx, e, r, expected)
	} else {
		if in.Review != nil || r.Lease != nil {
			return Run{}, store.ErrConflict
		}
		if len(r.Questions) == 0 {
			return Run{}, store.ErrConflict
		}
		allowed := make(map[string]struct{}, len(r.Questions))
		for _, question := range r.Questions {
			allowed[question.ID] = struct{}{}
		}
		submitted := map[string]Answer{}
		for _, answer := range in.Answers {
			_, expected := allowed[answer.ID]
			if !expected || !closedDecision(answer) || submitted[answer.ID].ID != "" {
				return Run{}, ErrInvalid
			}
			submitted[answer.ID] = answer
		}
		for _, q := range r.Questions {
			_, ok := submitted[q.ID]
			if q.Required && !ok {
				return Run{}, ErrInvalid
			}
		}
		for _, q := range r.Questions {
			if _, ok := submitted[q.ID]; !ok && !q.Required {
				r.Answers = setAnswer(r.Answers, Answer{ID: q.ID, Decision: "unresolved"})
			}
		}
		for _, answer := range in.Answers {
			r.Answers = setAnswer(r.Answers, answer)
		}
		r.Questions = []Question{}
		r.RequiredAction = ""
		r.Status = StatusReady
	}
	r.Message = message(r.Locale, r.Stage)
	r.Progress = progress(r.Stage)
	r.UpdatedAt = s.now().UTC()
	return s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
}

func (s *Service) Cancel(ctx context.Context, e identity.Envelope, id string, in CancelRequest) (Run, error) {
	if ctx == nil || !identity.Identifier(id) || in.ID != "" && in.ID != id || in.ExpectedVersion < 1 || !cancelReason(in.Reason) {
		return Run{}, ErrInvalid
	}
	if err := require(e, "onboarding.cancel", id, "cancel"); err != nil {
		return Run{}, err
	}
	r, err := s.repo.ReadOnboarding(ctx, e, id)
	if err != nil {
		return Run{}, err
	}
	if r.Version != in.ExpectedVersion {
		return Run{}, store.ErrConflict
	}
	if r.Status == StatusComplete {
		return Run{}, store.ErrConflict
	}
	if r.Status == StatusCancelled {
		if r.CancellationReason == in.Reason {
			return r, nil
		}
		return Run{}, store.ErrConflict
	}
	r.CancellationReason = in.Reason
	r.Questions = []Question{}
	if r.Lease != nil {
		r.CancelRequested = true
		r.Status = StatusRunning
		r.RequiredAction = "reconcile_cancellation"
		r.Message = statusMessage(r.Locale, "cancelling")
	} else {
		r.Status = StatusCancelled
		r.RequiredAction = ""
		r.Message = statusMessage(r.Locale, "cancelled")
	}
	r.UpdatedAt = s.now().UTC()
	return s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
}

func (s *Service) Drift(ctx context.Context, e identity.Envelope, id string, in DriftRequest) (Amendment, error) {
	if ctx == nil || !identity.Identifier(id) || in.ID != "" && in.ID != id || in.ExpectedVersion < 1 {
		return Amendment{}, ErrInvalid
	}
	if err := require(e, "onboarding.write", id, "write"); err != nil {
		return Amendment{}, err
	}
	r, err := s.repo.ReadOnboarding(ctx, e, id)
	if err != nil {
		return Amendment{}, err
	}
	if err = requireDependencies(e, "onboarding.write", r); err != nil {
		return Amendment{}, err
	}
	if r.Version != in.ExpectedVersion {
		if r.Version == in.ExpectedVersion+1 && len(r.Amendments) > 0 {
			last := r.Amendments[len(r.Amendments)-1]
			if last.RunVersion == r.Version {
				if err = access.Require(e, "onboarding.write", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: last.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: last.Context}); err != nil {
					return Amendment{}, err
				}
				return last, nil
			}
		}
		return Amendment{}, store.ErrConflict
	}
	if r.Status != StatusComplete {
		return Amendment{}, store.ErrConflict
	}
	if len(r.Amendments) >= 64 {
		return Amendment{}, store.ErrConflict
	}
	out, err := s.adapter.ProposeDriftAmendment(ctx, e, r, in, r.ID+"-drift-v1")
	if err != nil {
		return Amendment{}, err
	}
	if out.Run != r.ID || out.Source != r.Input.Source || !identity.Identifier(out.Context) || out.SourceRevision <= r.SourceRevision || (out.Observation != "schema_changed" && out.Observation != "binding_changed") || !out.ExistingIntact || out.RequiredAction != "review_amendment" || !identity.Identifier(out.Proposal.ID) || len(out.Changes) == 0 || len(out.Changes) > r.Limits.MaxEntities || len(out.Affected) == 0 || len(out.Affected) > r.Limits.MaxEntities {
		return Amendment{}, ErrInvalid
	}
	if err = access.Require(e, "onboarding.write", access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: out.Source}, access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: out.Context}); err != nil {
		return Amendment{}, err
	}
	for _, amendment := range r.Amendments {
		if amendment.SourceRevision == out.SourceRevision && amendment.Observation == out.Observation {
			return amendment, nil
		}
	}
	out.RunVersion = in.ExpectedVersion + 1
	r.Amendments = append(r.Amendments, out)
	r.UpdatedAt = s.now().UTC()
	_, err = s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
	if err != nil {
		return Amendment{}, err
	}
	return out, nil
}

func validateStep(v StepResult, l Limits, used, delta Usage) error {
	if len(v.References) > l.MaxEntities || len(v.Evidence) > l.MaxEntities || len(v.Questions) > 64 {
		return ErrBudget
	}
	for _, r := range v.References {
		if !identity.Identifier(r.ID) || r.Kind == "" || len(r.Digest) > 64 {
			return ErrInvalid
		}
	}
	for _, e := range v.Evidence {
		if !identity.Identifier(e.Entity) || len(e.Basis) == 0 || len(e.Basis) > 16 || (e.Confidence != "observed" && e.Confidence != "inferred" && e.Confidence != "unresolved") {
			return ErrInvalid
		}
	}
	for _, q := range v.Questions {
		if !identity.Identifier(q.ID) || !validText(q.Prompt, 512) || len(q.Evidence) > 16 {
			return ErrInvalid
		}
	}
	n := addUsage(used, delta)
	// A successful adapter invocation consumes one stage even when the adapter
	// itself reports no nested stage work.
	if n.Stages+1 > l.MaxStages || n.ModelCalls > l.MaxModelCalls || n.Tokens > l.MaxTokens || n.Entities > l.MaxEntities {
		return ErrBudget
	}
	return nil
}
func addUsage(a, b Usage) Usage {
	return Usage{a.Stages + b.Stages, a.ModelCalls + b.ModelCalls, a.Tokens + b.Tokens, a.Entities + b.Entities}
}
func appendUnique(base []Reference, in ...Reference) []Reference {
	seen := map[string]bool{}
	for _, r := range base {
		seen[r.Kind+"\x00"+r.ID] = true
	}
	for _, r := range in {
		k := r.Kind + "\x00" + r.ID
		if !seen[k] {
			base = append(base, r)
			seen[k] = true
		}
	}
	return base
}
func appendEvidence(base []Evidence, in ...Evidence) []Evidence { return append(base, in...) }
func hasQuestion(questions []Question, id string) bool {
	for _, question := range questions {
		if question.ID == id {
			return true
		}
	}
	return false
}
func answerValue(answers []Answer, id string) string {
	for _, answer := range answers {
		if answer.ID == id {
			return answer.Decision
		}
	}
	return ""
}
func setAnswer(answers []Answer, value Answer) []Answer {
	for i := range answers {
		if answers[i].ID == value.ID {
			answers[i] = value
			return answers
		}
	}
	return append(answers, value)
}
func next(s Stage) Stage {
	switch s {
	case StageConnect:
		return StageInspect
	case StageInspect:
		return StageProfile
	case StageProfile:
		return StageSemantic
	case StageSemantic:
		return StageReview
	case StageProposals:
		return StageComplete
	}
	return s
}
func progress(s Stage) Progress {
	n := map[Stage]int{StageConnect: 0, StageInspect: 1, StageProfile: 2, StageSemantic: 3, StageReview: 4, StageProposals: 5, StageComplete: 6}[s]
	return Progress{Completed: n, Total: 6, Percent: n * 100 / 6}
}
func message(locale string, s Stage) string {
	es := map[Stage]string{StageConnect: "Conectar o cargar la fuente", StageInspect: "Inspeccionar la fuente autorizada", StageProfile: "Crear evidencia de perfil", StageSemantic: "Preparar un borrador semántico", StageReview: "Revisar y publicar el tema", StageProposals: "Preparar propuestas privadas", StageComplete: "Configuración completa; revise las propuestas"}
	en := map[Stage]string{StageConnect: "Connect or upload the source", StageInspect: "Inspect the authorized source", StageProfile: "Create profile evidence", StageSemantic: "Prepare a semantic draft", StageReview: "Review and publish the topic", StageProposals: "Prepare private proposals", StageComplete: "Setup complete; review the proposals"}
	if locale == "es" {
		return es[s]
	}
	return en[s]
}
func statusMessage(locale, kind string) string {
	if locale == "es" {
		if kind == "cancelled" {
			return "Configuración cancelada"
		}
		if kind == "cancelling" {
			return "Cancelación solicitada; conciliando la etapa en curso"
		}
		return "La etapa falló; puede reanudarse con la misma referencia"
	}
	if kind == "cancelled" {
		return "Setup cancelled"
	}
	if kind == "cancelling" {
		return "Cancellation requested; reconciling the in-flight stage"
	}
	return "Stage failed; resume with the same reference"
}
func attention(r Run, action string, now time.Time) Run {
	r.Status = StatusAttention
	r.RequiredAction = action
	r.Message = message(r.Locale, r.Stage)
	r.UpdatedAt = now.UTC()
	return r
}
