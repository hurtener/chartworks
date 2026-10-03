package releaseprofile

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
)

type releaseInputFunc func(context.Context, identity.Envelope, evaluation.ProtectedRef) (evaluation.LiveInput, error)

func (f releaseInputFunc) ResolveEvaluationInput(ctx context.Context, e identity.Envelope, ref evaluation.ProtectedRef) (evaluation.LiveInput, error) {
	return f(ctx, e, ref)
}

type releaseMigrationFunc func(context.Context, identity.Envelope, string, string) (migration.ReleaseSource, error)

func (f releaseMigrationFunc) CurrentReleaseSource(ctx context.Context, e identity.Envelope, cohort, source string) (migration.ReleaseSource, error) {
	return f(ctx, e, cohort, source)
}

type releaseSource struct {
	value  sources.Source
	status sources.Status
}

func (f releaseSource) Get(context.Context, identity.Envelope, string) (sources.Source, error) {
	return f.value, nil
}
func (f releaseSource) Test(context.Context, identity.Envelope, string) (sources.Status, error) {
	return f.status, nil
}

type releaseTopicFunc func(context.Context, identity.Envelope, string, string) (topics.Published, error)

func (f releaseTopicFunc) Read(ctx context.Context, e identity.Envelope, id, version string) (topics.Published, error) {
	return f(ctx, e, id, version)
}

type releaseRuleFunc func(context.Context, identity.Envelope, string, string) (rulesets.Published, error)

func (f releaseRuleFunc) Read(ctx context.Context, e identity.Envelope, id, version string) (rulesets.Published, error) {
	return f(ctx, e, id, version)
}

type releaseDatasetFunc func(context.Context, identity.Envelope, sources.Source, []string) (DatasetEvidence, error)

func (f releaseDatasetFunc) ObserveDataset(ctx context.Context, e identity.Envelope, source sources.Source, datasets []string) (DatasetEvidence, error) {
	return f(ctx, e, source, datasets)
}

type releaseBlockFunc func(context.Context, identity.Envelope, string, reporting.Reference) (reporting.View, error)

func (f releaseBlockFunc) Read(ctx context.Context, e identity.Envelope, id string, ref reporting.Reference) (reporting.View, error) {
	return f(ctx, e, id, ref)
}

func TestRevisionResolverAcceptsOnlyCurrentPackNeutralFrozenBlock(t *testing.T) {
	hashA, hashB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	config := gateway.RuntimeConfig{Model: "narrative"}
	config.Digest = gateway.ConfigurationDigest(config)
	pack := evaluation.PackRevision{ID: "reviewed", Revision: 1, Model: "narrative", ConfigurationDigest: config.Digest}
	pack.Digest = pack.CanonicalDigest()
	input := evaluation.LiveInput{ReportID: "block", Frozen: &evaluation.FrozenRunInput{BlockID: "block", Request: reporting.RunRequest{Narrative: true}}}
	proof := evaluation.PerformanceReleaseEvidence{
		Suite: evaluation.SuiteRecord{State: evaluation.Accepted}, Report: evaluation.Report{Status: "passed"},
		Case: evaluation.Case{ID: "case", Stage: evaluation.StageConsumer}, CaseResult: evaluation.CaseResult{ID: "case", Passed: true},
		RuntimePack: evaluation.RuntimePackRecord{Pack: pack, Config: config, State: evaluation.Accepted, Review: &evaluation.RuntimePackReview{Decision: evaluation.Accepted}},
	}
	source := sources.Source{ID: "source", ContextID: "context", Revision: 1, Dialect: "postgres", Status: "registered"}
	topic := topics.Published{State: topics.State{Topic: "topic", Version: "v1", Revision: 1, Active: true}, Digest: hashA, Definition: topics.Definition{Datasets: []topics.Dataset{{ID: "dataset", Source: topics.Binding{Source: "source", Context: "context", Dataset: "dataset", SourceRevision: 1}}}}}
	rules := rulesets.Published{State: rulesets.State{Topic: "topic", Version: "r1", Revision: 1, Active: true}, Digest: hashB, Definition: semantics.RuleSetDefinition{TopicVersion: "v1", PackDigest: hashA}}
	migrated := migration.ReleaseSource{Cohort: "cohort", Batch: "batch", ManifestDigest: hashA, Generation: 1, SourceID: "source", SourceRevision: 1, ContextID: "context", Dialect: "postgres", Snapshot: hashB}
	block := reporting.View{State: reporting.State{ID: "block", PublishedRevision: 1}, Revision: 1, Digest: hashA, Source: "source", Context: "context",
		Topics: []reporting.TopicPin{{Topic: "topic", Version: "v1", Digest: hashA}},
		Rules:  []reporting.RulePin{{Topic: "topic", TopicVersion: "v1", PackDigest: hashA, RuleVersion: "r1", RuleDigest: hashB}},
	}
	resolver, err := NewRevisionResolver(
		releaseInputFunc(func(context.Context, identity.Envelope, evaluation.ProtectedRef) (evaluation.LiveInput, error) {
			return input, nil
		}),
		releaseMigrationFunc(func(context.Context, identity.Envelope, string, string) (migration.ReleaseSource, error) {
			return migrated, nil
		}),
		releaseSource{value: source, status: sources.Status{Available: true, Revision: 1, ContextID: "context"}},
		releaseTopicFunc(func(context.Context, identity.Envelope, string, string) (topics.Published, error) { return topic, nil }),
		releaseRuleFunc(func(context.Context, identity.Envelope, string, string) (rulesets.Published, error) {
			return rules, nil
		}),
		releaseDatasetFunc(func(context.Context, identity.Envelope, sources.Source, []string) (DatasetEvidence, error) {
			return DatasetEvidence{SourceID: "source", ContextID: "context", SourceRevision: 1, Digest: hashA, Rows: 2}, nil
		}),
		[]CaseCohort{{CaseID: "case", Cohort: "cohort"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err = resolver.WithFrozenBlocks(releaseBlockFunc(func(context.Context, identity.Envelope, string, reporting.Reference) (reporting.View, error) {
		return block, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	e, err := identity.FromVerified("tenant", "actor", "session", []string{"reporting.execute", "cw.block.execute:block", "cw.source.query:source", "cw.execution_context.use:context"}, time.Now().Add(time.Hour), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := resolver.ResolvePerformanceRevisions(t.Context(), e, proof)
	if err != nil || got.BlockID != "block" || got.WorkloadReport != "block" || got.BlockDigest != hashA || len(got.TopicPins) != 1 {
		t.Fatal("pack-neutral frozen block did not resolve current owners", err, got)
	}
	input.ReportID = "other-block"
	if _, err := resolver.ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatal("unrelated signed report target could stand in for frozen block", err)
	}
	input.ReportID = "block"
	input.Pack = pack
	input.Pack.Digest = hashB
	if _, err := resolver.ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatal("wrong packed input passed current resolver", err)
	}
	input.Pack = evaluation.PackRevision{}
	block.Topics[0].Digest = hashB
	if _, err := resolver.ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatal("stale block topic pin passed current resolver", err)
	}
}

func TestRevisionResolverRequiresCurrentSelectedOwners(t *testing.T) {
	hashA, hashB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	config := gateway.RuntimeConfig{Model: "model"}
	config.Digest = gateway.ConfigurationDigest(config)
	pack := evaluation.PackRevision{ID: "pack", Revision: 1, Model: "model", ConfigurationDigest: config.Digest}
	pack.Digest = pack.CanonicalDigest()
	input := evaluation.LiveInput{Pack: pack, ReportID: "report", Question: &nlqexec.QuestionRequest{Topic: "topic", Context: "context", Question: "Synthetic question"}, Run: &nlqexec.RunRequest{}}
	proof := evaluation.PerformanceReleaseEvidence{
		Suite: evaluation.SuiteRecord{State: evaluation.Accepted}, Report: evaluation.Report{Status: "passed"},
		Case: evaluation.Case{ID: "case", Stage: evaluation.StageConsumer}, CaseResult: evaluation.CaseResult{ID: "case", Passed: true},
		RuntimePack: evaluation.RuntimePackRecord{Pack: pack, Config: config, State: evaluation.Accepted, Review: &evaluation.RuntimePackReview{Decision: evaluation.Accepted}},
	}
	source := sources.Source{ID: "source", ContextID: "context", Revision: 1, Dialect: "postgres", Status: "registered"}
	topic := topics.Published{State: topics.State{Topic: "topic", Version: "v1", Revision: 1, Active: true}, Digest: hashA, Definition: topics.Definition{Datasets: []topics.Dataset{{ID: "dataset", Source: topics.Binding{Source: "source", Context: "context", Dataset: "dataset", SourceRevision: 1}}}}}
	rules := rulesets.Published{State: rulesets.State{Topic: "topic", Version: "r1", Revision: 1, Active: true}, Digest: hashB, Definition: semantics.RuleSetDefinition{TopicVersion: "v1", PackDigest: hashA}}
	migrated := migration.ReleaseSource{Cohort: "cohort", Batch: "batch", ManifestDigest: hashA, Generation: 1, SourceID: "source", SourceRevision: 1, ContextID: "context", Dialect: "postgres", Snapshot: hashB}
	dataset := DatasetEvidence{SourceID: "source", ContextID: "context", SourceRevision: 1, Digest: hashA, Rows: 2}
	newResolver := func() *RevisionResolver {
		r, err := NewRevisionResolver(
			releaseInputFunc(func(context.Context, identity.Envelope, evaluation.ProtectedRef) (evaluation.LiveInput, error) {
				return input, nil
			}),
			releaseMigrationFunc(func(_ context.Context, _ identity.Envelope, cohort, id string) (migration.ReleaseSource, error) {
				if cohort != "cohort" || id != "source" {
					return migration.ReleaseSource{}, errors.New("wrong selection")
				}
				return migrated, nil
			}),
			releaseSource{value: source, status: sources.Status{Available: true, Revision: 1, ContextID: "context"}},
			releaseTopicFunc(func(context.Context, identity.Envelope, string, string) (topics.Published, error) { return topic, nil }),
			releaseRuleFunc(func(context.Context, identity.Envelope, string, string) (rulesets.Published, error) {
				return rules, nil
			}),
			releaseDatasetFunc(func(_ context.Context, _ identity.Envelope, _ sources.Source, ids []string) (DatasetEvidence, error) {
				if len(ids) != 1 || ids[0] != "dataset" {
					return DatasetEvidence{}, errors.New("wrong dataset")
				}
				return dataset, nil
			}),
			[]CaseCohort{{CaseID: "case", Cohort: "cohort"}},
		)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	actor := func(scopes ...string) identity.Envelope {
		t.Helper()
		e, err := identity.FromVerified("tenant", "actor", "session", scopes, time.Now().Add(time.Hour), time.Now)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	scopes := []string{"reporting.execute", "cw.report.execute:report", "cw.source.query:source", "cw.execution_context.use:context"}
	e := actor(scopes...)
	result, err := newResolver().ResolvePerformanceRevisions(t.Context(), e, proof)
	if err != nil || result.TargetTenant != "tenant" || result.WorkloadReport != "report" || result.SourceRevision == "" || result.RuleRevision == "" || result.TopicRevision == "" || result.DatasetRows != 2 {
		t.Fatalf("current selected evidence: %v, %+v", err, result)
	}
	priorTopic, priorRules := topic, rules
	topic.State.Version, topic.State.Revision, topic.Digest = "v2", 2, strings.Repeat("c", 64)
	rules.State.Version, rules.State.Revision, rules.Digest = "r2", 2, strings.Repeat("d", 64)
	rules.Definition.TopicVersion, rules.Definition.PackDigest = topic.State.Version, topic.Digest
	changed, err := newResolver().ResolvePerformanceRevisions(t.Context(), e, proof)
	if err != nil || changed.TopicRevision == result.TopicRevision || changed.RuleRevision == result.RuleRevision || changed.SourceRevision != result.SourceRevision {
		t.Fatal("current topic publication did not carry its required reviewed-rule revision", err, changed)
	}
	topic, rules = priorTopic, priorRules
	if _, err := newResolver().ResolvePerformanceRevisions(t.Context(), actor(scopes[:len(scopes)-1]...), proof); !errors.Is(err, evaluation.ErrPerformanceAuthority) {
		t.Fatalf("unsigned context: %v", err)
	}
	input.ReportID = "unreachable"
	if _, err := newResolver().ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceAuthority) {
		t.Fatalf("protected report mismatch: %v", err)
	}
	input.ReportID = "report"
	input.Pack.Digest = hashB
	if _, err := newResolver().ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatalf("unreviewed input pack: %v", err)
	}
	input.Pack = pack
	migrated.SourceRevision = 2
	if _, err := newResolver().ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatalf("stale Phase 34 source: %v", err)
	}
	migrated.SourceRevision = 1
	call := 0
	r, err := NewRevisionResolver(
		releaseInputFunc(func(context.Context, identity.Envelope, evaluation.ProtectedRef) (evaluation.LiveInput, error) {
			return input, nil
		}),
		releaseMigrationFunc(func(context.Context, identity.Envelope, string, string) (migration.ReleaseSource, error) {
			return migrated, nil
		}),
		releaseSource{value: source, status: sources.Status{Available: true, Revision: 1, ContextID: "context"}},
		releaseTopicFunc(func(context.Context, identity.Envelope, string, string) (topics.Published, error) {
			call++
			if call == 2 {
				changed := topic
				changed.State.Revision++
				return changed, nil
			}
			return topic, nil
		}),
		releaseRuleFunc(func(context.Context, identity.Envelope, string, string) (rulesets.Published, error) {
			return rules, nil
		}),
		releaseDatasetFunc(func(context.Context, identity.Envelope, sources.Source, []string) (DatasetEvidence, error) {
			return dataset, nil
		}),
		[]CaseCohort{{CaseID: "case", Cohort: "cohort"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.ResolvePerformanceRevisions(t.Context(), e, proof); !errors.Is(err, evaluation.ErrPerformanceEvidence) {
		t.Fatalf("concurrent topic publication: %v", err)
	}
}
