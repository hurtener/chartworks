package nlqexec

import (
	"context"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/store"
	"strings"
)

// PlanOperationReader preserves the original Plan operation independently of
// later Run operation keys. It never returns an unscoped query.
type PlanOperationReader interface {
	ReadPlanOperation(context.Context, store.Scope, string) (QueryRecord, error)
}

func PlanSubmissionValid(q QueryRecord) bool {
	if q.PlanOperation == "" && q.PlanRequestDigest == "" {
		return true
	}
	return identity.Identifier(q.PlanOperation) && !strings.HasPrefix(q.PlanOperation, "resume:") && len(q.PlanRequestDigest) == 64 && strings.Trim(q.PlanRequestDigest, "0123456789abcdef") == "" && q.SQL != "" && q.GenerationPending == nil
}

func planSubmissionDigest(q QuestionRequest) string {
	canonicalizeQuestion(&q)
	return exec.Hash(q)
}

func (s *Service) readPlanOperation(ctx context.Context, scope store.Scope, operation string) (QueryRecord, error) {
	if reader, ok := s.repo.(PlanOperationReader); ok {
		return reader.ReadPlanOperation(ctx, scope, operation)
	}
	return s.repo.ReadOperation(ctx, scope, operation)
}
