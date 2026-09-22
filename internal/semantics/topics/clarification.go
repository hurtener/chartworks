package topics

import (
	"context"

	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
)

// ClarificationBinding supplies current native metadata to the conditional
// clarification consumer. The ordinary source service rechecks current signed
// reach and the actual execution partition; no replay token is retained.
func (s *Service) ClarificationBinding(ctx context.Context, e identity.Envelope, source, executionContext string) (exec.Binding, error) {
	return s.source.Binding(ctx, e, source, executionContext)
}
