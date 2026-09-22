package onboarding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
func operationKey(r Run, stage Stage) string { return r.ID + "-" + string(stage) + "-v1" }

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
	run := Run{ID: in.ID, Key: in.Key, Version: 1, Stage: StageConnect, Status: StatusReady, Locale: in.Locale, Message: message(in.Locale, StageConnect), Input: in, References: []Reference{}, Evidence: []Evidence{}, Questions: []Question{}, Answers: []Answer{}, Amendments: []Amendment{}, Limits: s.limits, Progress: progress(StageConnect), CreatedAt: now, UpdatedAt: now, Deadline: now.Add(s.limits.MaxDuration)}
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
	r.Status = StatusRunning
	var step StepResult
	switch r.Stage {
	case StageConnect:
		step, err = s.adapter.Connect(ctx, e, r.Input, operationKey(r, r.Stage))
	case StageInspect:
		step, err = s.adapter.Inspect(ctx, e, r, operationKey(r, r.Stage))
	case StageProfile:
		step, err = s.adapter.Profile(ctx, e, r, operationKey(r, r.Stage))
	case StageSemantic:
		step, err = s.adapter.DraftSemantics(ctx, e, r, operationKey(r, r.Stage))
	case StageReview:
		return attention(r, "review_topic", s.now()), ErrAttention
	case StageProposals:
		step, err = s.adapter.ProposeQueriesBlocksReports(ctx, e, r, operationKey(r, r.Stage))
	default:
		return Run{}, ErrInvalid
	}
	if err != nil {
		r.Status = StatusFailed
		r.Message = statusMessage(r.Locale, "failed")
		r.UpdatedAt = s.now().UTC()
		saved, saveErr := s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
		if saveErr != nil {
			return Run{}, saveErr
		}
		return saved, err
	}
	if err = validateStep(step, s.limits, r.Usage); err != nil {
		return Run{}, err
	}
	r.References = appendUnique(r.References, step.References...)
	r.Evidence = appendEvidence(r.Evidence, step.Evidence...)
	for _, ref := range step.References {
		if (ref.Kind == "dataset" || ref.Kind == "profile") && ref.Revision > r.SourceRevision {
			r.SourceRevision = ref.Revision
		}
	}
	r.Questions = step.Questions
	r.Usage = addUsage(r.Usage, step.Usage)
	r.Usage.Stages++
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
	return s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
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
	if !s.now().Before(r.Deadline) {
		return Run{}, ErrBudget
	}
	if r.Stage == StageReview {
		if in.Review == nil || !identity.Identifier(in.Review.ID) || in.Review.Revision < 1 || len(in.Review.Digest) != 64 {
			return Run{}, ErrInvalid
		}
		step, e2 := s.adapter.PublishReviewed(ctx, e, r, *in.Review, operationKey(r, r.Stage))
		if e2 != nil {
			return Run{}, e2
		}
		if e2 = validateStep(step, s.limits, r.Usage); e2 != nil {
			return Run{}, e2
		}
		r.References = appendUnique(r.References, step.References...)
		r.Evidence = appendEvidence(r.Evidence, step.Evidence...)
		r.Usage = addUsage(r.Usage, step.Usage)
		r.Usage.Stages++
		r.Stage = StageProposals
		r.Status = StatusReady
		r.RequiredAction = ""
	} else {
		if len(r.Questions) == 0 {
			return Run{}, store.ErrConflict
		}
		allowed := make(map[string]struct{}, len(r.Questions))
		for _, question := range r.Questions {
			allowed[question.ID] = struct{}{}
		}
		submitted := map[string]string{}
		for _, answer := range in.Answers {
			_, expected := allowed[answer.ID]
			if !expected || !identity.Identifier(answer.ID) || !validText(answer.Value, 2048) || submitted[answer.ID] != "" {
				return Run{}, ErrInvalid
			}
			submitted[answer.ID] = answer.Value
		}
		for _, q := range r.Questions {
			v, ok := submitted[q.ID]
			if q.Required && (!ok || !validText(v, 2048)) {
				return Run{}, ErrInvalid
			}
		}
		for _, q := range r.Questions {
			if _, ok := submitted[q.ID]; !ok && !q.Required {
				r.Answers = setAnswer(r.Answers, Answer{ID: q.ID, Value: "unresolved"})
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
	if ctx == nil || !identity.Identifier(id) || in.ID != "" && in.ID != id || in.ExpectedVersion < 1 || !validText(in.Reason, 512) {
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
	r.Status = StatusCancelled
	r.CancellationReason = in.Reason
	r.RequiredAction = ""
	r.Questions = []Question{}
	r.Message = statusMessage(r.Locale, "cancelled")
	r.UpdatedAt = s.now().UTC()
	return s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
}

func (s *Service) Drift(ctx context.Context, e identity.Envelope, id string, in DriftRequest) (Amendment, error) {
	if ctx == nil || !identity.Identifier(id) || in.ID != "" && in.ID != id || in.ExpectedVersion < 1 || in.SourceRevision < 1 || !identity.Identifier(in.Observation) {
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
	for _, amendment := range r.Amendments {
		if amendment.Observation == in.Observation && amendment.SourceRevision == in.SourceRevision {
			return amendment, nil
		}
	}
	if r.Version != in.ExpectedVersion || r.Status != StatusComplete {
		return Amendment{}, store.ErrConflict
	}
	if in.SourceRevision <= r.SourceRevision || len(r.Amendments) >= 64 {
		return Amendment{}, store.ErrConflict
	}
	out, err := s.adapter.ProposeDriftAmendment(ctx, e, r, in, r.ID+"-drift-"+in.Observation)
	if err != nil {
		return Amendment{}, err
	}
	if out.Run != r.ID || out.SourceRevision != in.SourceRevision || !out.ExistingIntact || out.RequiredAction != "review_amendment" || !identity.Identifier(out.Proposal.ID) || len(out.Affected) > r.Limits.MaxEntities {
		return Amendment{}, ErrInvalid
	}
	out.Observation = in.Observation
	out.RunVersion = in.ExpectedVersion + 1
	r.Amendments = append(r.Amendments, out)
	r.UpdatedAt = s.now().UTC()
	_, err = s.repo.SaveOnboarding(ctx, e, r, in.ExpectedVersion)
	if err != nil {
		return Amendment{}, err
	}
	return out, nil
}

func validateStep(v StepResult, l Limits, used Usage) error {
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
	n := addUsage(used, v.Usage)
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
			return answer.Value
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
		return "La etapa falló; puede reanudarse con la misma referencia"
	}
	if kind == "cancelled" {
		return "Setup cancelled"
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
