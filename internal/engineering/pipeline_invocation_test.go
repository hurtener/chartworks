package engineering

import (
	"context"
	"errors"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/jobs"
)

func TestPipelineEffectRequiresOwnedInvocation(t *testing.T) {
	// An unowned claim must fail before touching any store, source or writer.
	// Deliberately leave all service dependencies absent to exercise that order.
	service := &PipelineService{}
	err := service.executePipeline(context.Background(), jobs.Invocation{}, PipelineRecord{}, config.SourceConnection{})
	if !errors.Is(err, jobs.ErrAuthority) {
		t.Fatalf("unowned pipeline effect: %v", err)
	}
}
