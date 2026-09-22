package releaseprofile

import (
	"context"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/evaluation"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway/recorded"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
)

type bearerVerifier interface {
	Verify(context.Context, string, auth.Surface) (identity.Envelope, error)
}

// Integration is an operator-owned composition of existing governed services.
// The recorded engine and cassette are supplied outside manifest/profile input.
// The accepted Phase 24 report and reviewed pack are still loaded by Service
// under freshly verified Pengui authority at measurement time.
type Integration struct {
	Verifier   bearerVerifier
	Evaluation *evaluation.Service
	Inputs     evaluation.LiveInputResolver
	Migration  *migration.Service
	Sources    *sources.Service
	Topics     *topics.Service
	Rules      *rulesets.Service
	Index      nlqroute.IndexReader
	Validator  *readexec.Validator
	Executor   *readexec.Executor
	Repository nlqexec.Repository
	Engine     *recorded.Engine
	Cohorts    []CaseCohort
	Clock      evaluation.Clock
}

// NewIntegration composes the real route→Plan→Run path with one recorded
// gateway.Engine. The Phase 25 runtime remains fail-closed at the product reuse
// check until the frozen-run and current reviewed-pack seams are integrated.
func NewIntegration(c Integration) (*evaluation.PerformanceReleaseRuntime, error) {
	if c.Verifier == nil || c.Evaluation == nil || c.Inputs == nil || c.Migration == nil || c.Sources == nil || c.Topics == nil || c.Rules == nil || c.Index == nil || c.Validator == nil || c.Executor == nil || c.Repository == nil || c.Engine == nil || c.Engine.PerformanceModelMode() != "recorded" {
		return nil, evaluation.ErrMode
	}
	probe, err := NewNativeDatasetProbe(c.Validator, c.Sources)
	if err != nil {
		return nil, err
	}
	revisions, err := NewRevisionResolver(c.Inputs, c.Migration, c.Sources, c.Topics, c.Rules, probe, c.Cohorts)
	if err != nil {
		return nil, err
	}
	routing, err := nlqroute.New(c.Topics, c.Rules, c.Index, c.Engine)
	if err != nil {
		return nil, err
	}
	query, err := nlqexec.New(routing, c.Topics, c.Sources, c.Validator, c.Executor, c.Engine, c.Repository)
	if err != nil {
		return nil, err
	}
	runner := &evaluation.GovernedRunner{Inputs: c.Inputs, Routing: routing, Query: query, Saved: query, Clock: c.Clock}
	factory, err := evaluation.NewGovernedPerformanceReleaseAdapterFactory(runner, evaluation.PerformanceIntegration, "real_postgres", "recorded", c.Clock)
	if err != nil {
		return nil, err
	}
	return &evaluation.PerformanceReleaseRuntime{Verifier: c.Verifier, Service: c.Evaluation, Revisions: revisions, Adapters: factory, Clock: c.Clock}, nil
}
