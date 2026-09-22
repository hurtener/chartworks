package releaseprofile

import (
	"context"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/evaluation"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway/recorded"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/reporting"
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
	Verifier        bearerVerifier
	Evaluation      *evaluation.Service
	Inputs          evaluation.LiveInputResolver
	Migration       *migration.Service
	Sources         *sources.Service
	Topics          *topics.Service
	Rules           *rulesets.Service
	Validator       *readexec.Validator
	Engine          *recorded.Engine
	Blocks          *reporting.Service
	FrozenStore     reporting.RunRepository
	Requests        *jobs.RequestRunner
	ReportingLimits config.ReportingExecution
	Cohorts         []CaseCohort
	Clock           evaluation.Clock
}

// NewIntegration composes the real published-block and frozen-run path with
// one recorded gateway.Engine. The Phase 25 runtime remains fail-closed until
// the changed reviewed-pack consumer and final_stress evidence are integrated.
func NewIntegration(c Integration) (*evaluation.PerformanceReleaseRuntime, error) {
	if c.Verifier == nil || c.Evaluation == nil || c.Inputs == nil || c.Migration == nil || c.Sources == nil || c.Topics == nil || c.Rules == nil || c.Validator == nil || c.Blocks == nil || c.FrozenStore == nil || c.Requests == nil || c.Engine == nil || c.Engine.PerformanceModelMode() != "recorded" || c.ReportingLimits.Validate() != nil || c.ReportingLimits.ModelVersion == "" {
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
	revisions, err = revisions.WithFrozenBlocks(c.Blocks)
	if err != nil {
		return nil, err
	}
	runs, err := reporting.NewRuns(c.Blocks, c.FrozenStore, c.Requests, c.Engine, c.ReportingLimits.ModelVersion, c.ReportingLimits)
	if err != nil {
		return nil, err
	}
	selector, ok := c.FrozenStore.(reporting.NarrativePackSelector)
	if !ok {
		return nil, evaluation.ErrMode
	}
	runs, err = runs.WithReviewedNarrativePacks(selector)
	if err != nil {
		return nil, err
	}
	factory, err := evaluation.NewFrozenPerformanceReleaseAdapterFactory(c.Inputs, runs, c.FrozenStore, "recorded", c.Clock)
	if err != nil {
		return nil, err
	}
	return &evaluation.PerformanceReleaseRuntime{Verifier: c.Verifier, Service: c.Evaluation, Revisions: revisions, Adapters: factory, Clock: c.Clock}, nil
}
