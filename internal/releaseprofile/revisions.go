// Package releaseprofile composes the current, signed Phase 34 and domain
// evidence needed by the Phase 25 final-stress release runtime.
package releaseprofile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/evaluation"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/migration"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
)

// CaseCohort chooses an operator-reviewed Phase 34 cohort for one accepted
// Phase 24 case. It supplies a selector, never an authority or revision value.
type CaseCohort struct {
	CaseID string
	Cohort string
}

type releaseSourceReader interface {
	CurrentReleaseSource(context.Context, identity.Envelope, string, string) (migration.ReleaseSource, error)
}

type sourceReader interface {
	Get(context.Context, identity.Envelope, string) (sources.Source, error)
	Test(context.Context, identity.Envelope, string) (sources.Status, error)
}

type topicReader interface {
	Read(context.Context, identity.Envelope, string, string) (topics.Published, error)
}

type ruleReader interface {
	Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error)
}
type blockReader interface {
	Read(context.Context, identity.Envelope, string, reporting.Reference) (reporting.View, error)
}

// DatasetEvidence comes from an actual governed source read. Its digest covers
// all rows of the bounded validator-safe projection, and Rows is observed.
type DatasetEvidence struct {
	SourceID       string
	ContextID      string
	SourceRevision int64
	Digest         string
	Rows           int64
}

type datasetProbe interface {
	ObserveDataset(context.Context, identity.Envelope, sources.Source, []string) (DatasetEvidence, error)
}

type revisionLine struct {
	Topic    string
	Version  string
	Revision int64
	Digest   string
}

// RevisionResolver reads every current binding from its owner. No profile
// field can supply a source/rule/topic/reviewed-pack revision.
type RevisionResolver struct {
	inputs   evaluation.LiveInputResolver
	migrated releaseSourceReader
	sources  sourceReader
	topics   topicReader
	rules    ruleReader
	dataset  datasetProbe
	cohorts  map[string]string
	blocks   blockReader
}

// WithFrozenBlocks enables resolution from an actual selected published block.
// The accepted case still supplies only its protected block selector.
func (r *RevisionResolver) WithFrozenBlocks(blocks blockReader) (*RevisionResolver, error) {
	if r == nil || blocks == nil {
		return nil, evaluation.ErrPerformanceEvidence
	}
	copy := *r
	copy.blocks = blocks
	return &copy, nil
}

func NewRevisionResolver(inputs evaluation.LiveInputResolver, migrated releaseSourceReader, source sourceReader, topic topicReader, rule ruleReader, dataset datasetProbe, selections []CaseCohort) (*RevisionResolver, error) {
	if inputs == nil || migrated == nil || source == nil || topic == nil || rule == nil || dataset == nil || len(selections) == 0 {
		return nil, evaluation.ErrPerformanceEvidence
	}
	cohorts := make(map[string]string, len(selections))
	for _, selection := range selections {
		if !identity.Identifier(selection.CaseID) || !identity.Identifier(selection.Cohort) || cohorts[selection.CaseID] != "" {
			return nil, evaluation.ErrPerformanceEvidence
		}
		cohorts[selection.CaseID] = selection.Cohort
	}
	return &RevisionResolver{inputs: inputs, migrated: migrated, sources: source, topics: topic, rules: rule, dataset: dataset, cohorts: cohorts}, nil
}

var _ evaluation.PerformanceRevisionResolver = (*RevisionResolver)(nil)

func (r *RevisionResolver) ResolvePerformanceRevisions(ctx context.Context, e identity.Envelope, proof evaluation.PerformanceReleaseEvidence) (evaluation.PerformanceRevisionEvidence, error) {
	if r == nil || ctx == nil || !e.Valid() || proof.Case.Stage != evaluation.StageConsumer || proof.Case.ID == "" || proof.Case.ID != proof.CaseResult.ID || proof.Suite.State != evaluation.Accepted || proof.Report.Status != "passed" || proof.RuntimePack.State != evaluation.Accepted || proof.RuntimePack.Review == nil {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	cohort := r.cohorts[proof.Case.ID]
	if cohort == "" {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	input, err := r.inputs.ResolveEvaluationInput(ctx, e, proof.Case.Input)
	if err != nil || !identity.Identifier(input.ReportID) || !evaluation.ProtectedPackMatches(input, proof.RuntimePack.Pack) {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	var topicIDs []string
	var selectedBlock reporting.View
	if input.Frozen != nil {
		if r.blocks == nil || input.Run != nil || input.Question != nil || !identity.Identifier(input.Frozen.BlockID) || input.ReportID != input.Frozen.BlockID {
			return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
		}
		selectedBlock, err = r.blocks.Read(ctx, e, input.Frozen.BlockID, input.Frozen.Request.Reference)
		if err != nil || selectedBlock.State.ID != input.Frozen.BlockID || selectedBlock.Private || selectedBlock.Revision != selectedBlock.State.PublishedRevision || selectedBlock.Source == "" || selectedBlock.Context == "" || !digestValid(selectedBlock.Digest) || len(selectedBlock.Topics) == 0 {
			return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
		}
		for _, pin := range selectedBlock.Topics {
			topicIDs = append(topicIDs, pin.Topic)
		}
		topicIDs = sortedIDs(topicIDs)
	} else {
		if input.Question == nil || input.Run == nil {
			return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
		}
		topicIDs, err = selectedTopics(*input.Question)
		if err != nil {
			return evaluation.PerformanceRevisionEvidence{}, err
		}
	}
	var sourceID, contextID string
	datasets := map[string]bool{}
	sourceRevisions := map[int64]bool{}
	topicRevisions := make([]revisionLine, 0, len(topicIDs))
	ruleRevisions := make([]revisionLine, 0, len(topicIDs))
	for _, topicID := range topicIDs {
		published, readErr := r.topics.Read(ctx, e, topicID, "")
		if readErr != nil || !published.State.Active || published.State.Archived || published.State.Topic != topicID || published.State.Version == "" || published.State.Revision < 1 || !digestValid(published.Digest) || len(published.Definition.Datasets) == 0 {
			return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
		}
		currentRules, readErr := r.rules.Read(ctx, e, topicID, "")
		if readErr != nil || !currentRules.State.Active || currentRules.State.Retired || currentRules.State.Topic != topicID || currentRules.State.Version == "" || currentRules.State.Revision < 1 || !digestValid(currentRules.Digest) || currentRules.Definition.TopicVersion != published.State.Version || currentRules.Definition.PackDigest != published.Digest {
			return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
		}
		if input.Frozen != nil {
			foundTopic, foundRule := false, false
			for _, pin := range selectedBlock.Topics {
				if pin.Topic == topicID && pin.Version == published.State.Version && pin.Digest == published.Digest {
					foundTopic = true
				}
			}
			for _, pin := range selectedBlock.Rules {
				if pin.Topic == topicID && pin.TopicVersion == published.State.Version && pin.PackDigest == published.Digest && pin.RuleVersion == currentRules.State.Version && pin.RuleDigest == currentRules.Digest {
					foundRule = true
				}
			}
			if !foundTopic || len(selectedBlock.Rules) != len(topicIDs) || !foundRule {
				return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
			}
		}
		for _, dataset := range published.Definition.Datasets {
			binding := dataset.Source
			if !identity.Identifier(dataset.ID) || binding.Dataset != dataset.ID || !identity.Identifier(binding.Source) || !identity.Identifier(binding.Context) || binding.SourceRevision < 1 {
				return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
			}
			if sourceID == "" {
				sourceID, contextID = binding.Source, binding.Context
			}
			if binding.Source != sourceID || binding.Context != contextID {
				return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
			}
			datasets[dataset.ID] = true
			sourceRevisions[binding.SourceRevision] = true
		}
		topicRevisions = append(topicRevisions, revisionLine{topicID, published.State.Version, published.State.Revision, published.Digest})
		ruleRevisions = append(ruleRevisions, revisionLine{topicID, currentRules.State.Version, currentRules.State.Revision, currentRules.Digest})
	}
	if sourceID == "" || len(datasets) == 0 || input.Frozen != nil && (sourceID != selectedBlock.Source || contextID != selectedBlock.Context) || input.Frozen == nil && contextID != input.Question.Context {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	targetKind := "report"
	if input.Frozen != nil {
		targetKind = "block"
	}
	if err := access.RequireExecution(e, access.Execution{
		Target:       access.Resource{Tenant: e.Tenant(), Kind: targetKind, Permission: "execute", ID: input.ReportID},
		Dependencies: []access.Resource{{Tenant: e.Tenant(), Kind: "source", Permission: "query", ID: sourceID}},
		Contexts:     []access.Resource{{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: contextID}},
	}); err != nil {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceAuthority
	}
	currentSource, err := r.sources.Get(ctx, e, sourceID)
	if err != nil || currentSource.ID != sourceID || currentSource.ContextID != contextID || currentSource.Revision < 1 || currentSource.Status != "registered" || len(sourceRevisions) != 1 || !sourceRevisions[currentSource.Revision] {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	status, err := r.sources.Test(ctx, e, sourceID)
	if err != nil || !status.Available || status.ContextID != contextID || status.Revision != currentSource.Revision {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	migrated, err := r.migrated.CurrentReleaseSource(ctx, e, cohort, sourceID)
	if err != nil || migrated.SourceID != sourceID || migrated.ContextID != contextID || migrated.SourceRevision != currentSource.Revision || migrated.Dialect != currentSource.Dialect || !digestValid(migrated.Snapshot) || !digestValid(migrated.ManifestDigest) || migrated.Generation < 1 {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	ids := make([]string, 0, len(datasets))
	for id := range datasets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	observed, err := r.dataset.ObserveDataset(ctx, e, currentSource, ids)
	if err != nil || observed.SourceID != sourceID || observed.ContextID != contextID || observed.SourceRevision != currentSource.Revision || !digestValid(observed.Digest) || observed.Rows < 1 {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	// Cross-domain reads cannot share one PostgreSQL transaction. Detect any
	// concurrent owner revision change across this bounded observation window.
	for i, topicID := range topicIDs {
		topic, topicErr := r.topics.Read(ctx, e, topicID, "")
		rule, ruleErr := r.rules.Read(ctx, e, topicID, "")
		if topicErr != nil || ruleErr != nil || !topic.State.Active || topic.State.Archived || !rule.State.Active || rule.State.Retired || (revisionLine{topicID, topic.State.Version, topic.State.Revision, topic.Digest}) != topicRevisions[i] || (revisionLine{topicID, rule.State.Version, rule.State.Revision, rule.Digest}) != ruleRevisions[i] {
			return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
		}
	}
	latestSource, err := r.sources.Get(ctx, e, sourceID)
	if err != nil || latestSource != currentSource {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	latestMigration, err := r.migrated.CurrentReleaseSource(ctx, e, cohort, sourceID)
	if err != nil || latestMigration != migrated {
		return evaluation.PerformanceRevisionEvidence{}, evaluation.ErrPerformanceEvidence
	}
	return evaluation.PerformanceRevisionEvidence{
		TargetTenant:   e.Tenant(),
		WorkloadReport: input.ReportID,
		SourceID:       sourceID,
		ContextID:      contextID,
		SourceRevision: hash(struct {
			Tenant string
			Source migration.ReleaseSource
		}{e.Tenant(), migrated}),
		RuleRevision:  hash(ruleRevisions),
		TopicRevision: hash(topicRevisions),
		DatasetDigest: observed.Digest,
		DatasetRows:   observed.Rows,
		BlockID:       selectedBlock.State.ID,
		BlockRevision: selectedBlock.Revision,
		BlockDigest:   selectedBlock.Digest,
		SourceHead:    currentSource.Revision,
		TopicPins:     append([]reporting.TopicPin(nil), selectedBlock.Topics...),
		RulePins:      append([]reporting.RulePin(nil), selectedBlock.Rules...),
	}, nil
}

func sortedIDs(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func selectedTopics(question nlqexec.QuestionRequest) ([]string, error) {
	values := append([]string(nil), question.Topics...)
	if question.Topic != "" {
		values = append(values, question.Topic)
	}
	if len(values) == 0 || len(values) > 8 {
		return nil, evaluation.ErrPerformanceEvidence
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if !identity.Identifier(value) {
			return nil, evaluation.ErrPerformanceEvidence
		}
		if !seen[value] {
			out = append(out, value)
			seen[value] = true
		}
	}
	sort.Strings(out)
	return out, nil
}

func hash(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func digestValid(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
