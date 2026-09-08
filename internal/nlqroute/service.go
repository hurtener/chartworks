// Package nlqroute owns the first real NLQ routing consumer. It admits current
// topic/source state, evaluates the reviewed rule seam, embeds through Bifrost,
// searches the existing vector index, optionally reranks admitted candidates,
// and assembles one bounded generation context.
package nlqroute

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

const (
	maxTopics       = 4
	maxReferences   = 256
	maxKinds        = 8
	maxExamples     = nlq.MaxExamples
	maxMetricIDs    = 32
	maxQuestionSize = 16 << 10
)

var (
	// ErrInvalid identifies a malformed route request or an impossible
	// caller-supplied combination. It carries no request text.
	ErrInvalid = errors.New("nlqroute: invalid request")
	// ErrNoRoute identifies an authorized request for which no current facet
	// route was found. The result uses StrategyNoRoute for this outcome.
	ErrNoRoute = errors.New("nlqroute: no route")
)

// TopicReader is the live topic contract seam. Contract performs current
// source discovery and publication confirmation before the gateway is called.
type TopicReader interface {
	Contract(context.Context, identity.Envelope, string) (topics.Contract, error)
}

// RuleReader is the reviewed phase-16 lifecycle seam. The routing package does
// not compile or evaluate rules itself.
type RuleReader interface {
	Read(context.Context, identity.Envelope, string, string) (rulesets.Published, error)
	Evaluate(context.Context, identity.Envelope, string, rulesets.EvaluateRequest) (rulesets.Evaluation, error)
}

// IndexReader is the existing authorized pgvector service.
type IndexReader interface {
	Search(context.Context, identity.Envelope, []vindex.Query) ([]vindex.Result, error)
	Explain(context.Context, identity.Envelope, vindex.Query) (json.RawMessage, error)
}

// JoinChoice names one reviewed join in each topic of a multi-topic request.
// The route verifies its source, context, endpoints and cardinality from the
// published topic before any gateway call.
type JoinChoice struct {
	Topic  string `json:"topic"`
	JoinID string `json:"join_id"`
}

// ChoiceSelection supplies one reviewed clarification slot value. A pattern
// may be omitted only when the slot ID is unambiguous in the published rules.
type ChoiceSelection struct {
	Pattern string `json:"pattern,omitempty"`
	Slot    string `json:"slot"`
	Value   string `json:"value"`
}

// RouteRequest contains only bounded question, selection and context inputs.
// Topic reach, source bindings, rule text and facet text are resolved from
// current published state by Service.Route.
type RouteRequest struct {
	Topic        string                `json:"topic,omitempty"`
	Topics       []string              `json:"topics,omitempty"`
	Context      string                `json:"context"`
	Locale       nlq.Language          `json:"locale"`
	Question     string                `json:"question"`
	Kinds        []string              `json:"kinds,omitempty"`
	LimitPerKind int                   `json:"limit_per_kind,omitempty"`
	References   []semantics.Reference `json:"references,omitempty"`
	Choices      []ChoiceSelection     `json:"choices,omitempty"`
	JoinChoices  []JoinChoice          `json:"joins,omitempty"`
	MetricIDs    []string              `json:"metric_ids,omitempty"`
	Examples     []nlq.OptionalItem    `json:"examples,omitempty"`
	Rerank       bool                  `json:"rerank,omitempty"`
}

// ClarificationChoice is a detached presentation choice. It is not authority
// and its target is only admitted after the reviewed ruleset evaluates it.
type ClarificationChoice struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// Clarification is a typed terminal outcome that stops before embedding when
// a required slot, ambiguous join, or ambiguous retrieval needs user input.
type Clarification struct {
	Reason  string                `json:"reason"`
	Pattern string                `json:"pattern,omitempty"`
	Slot    string                `json:"slot,omitempty"`
	Prompt  string                `json:"prompt,omitempty"`
	Choices []ClarificationChoice `json:"choices,omitempty"`
}

func (c *Clarification) Error() string { return "nlqroute: clarification required" }

// Stage records bounded work and remote attribution without exposing prompts,
// vectors, credentials, or provider error bodies.
type Stage struct {
	Name       string `json:"name"`
	DurationMS int64  `json:"duration_ms"`
	Calls      int    `json:"calls"`
	Cached     bool   `json:"cached"`
	Warning    string `json:"warning,omitempty"`
}

// ContextView is the detached response/model-input projection. It omits the
// assembler's private seal so the wire schema cannot accept a forged sealed
// context on a later generation call.
type ContextView struct {
	Tier         nlq.Tier             `json:"tier"`
	Budget       int                  `json:"budget"`
	Tokens       int                  `json:"tokens"`
	Locale       nlq.Language         `json:"locale"`
	Strategy     nlq.Strategy         `json:"strategy"`
	Topic        string               `json:"topic,omitempty"`
	TopicVersion string               `json:"topic_version,omitempty"`
	Topics       []nlq.TopicRevision  `json:"topics,omitempty"`
	Question     string               `json:"question"`
	Prompt       string               `json:"prompt"`
	Evidence     []nlq.Evidence       `json:"evidence"`
	Constraints  *nlq.ConstraintState `json:"constraints,omitempty"`
	Metrics      []nlq.PinnedMetric   `json:"metrics"`
	Advisory     []nlq.OptionalItem   `json:"advisory"`
	Examples     []nlq.OptionalItem   `json:"examples"`
}

// RouteResult is a detached routing and context result. A Clarification or
// StrategyNoRoute result has no Context and therefore cannot reach generation.
type RouteResult struct {
	Outcome       nlq.Strategy      `json:"outcome"`
	Topic         string            `json:"topic"`
	Topics        []string          `json:"topics"`
	TopicVersions []string          `json:"topic_versions"`
	RuleVersions  []string          `json:"rule_versions,omitempty"`
	Confidence    float64           `json:"confidence"`
	Tier          nlq.Tier          `json:"tier,omitempty"`
	Context       *ContextView      `json:"context,omitempty"`
	Audit         nlq.AssemblyAudit `json:"audit,omitempty"`
	Evidence      []vindex.Hit      `json:"evidence,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
	RemoteCalls   []gateway.Usage   `json:"remote_calls,omitempty"`
	Stages        []Stage           `json:"stages"`
	Clarification *Clarification    `json:"clarification,omitempty"`
}

// Service coordinates current topic admission, reviewed constraints, one
// gateway and the existing vector index. It has no cache and no executor.
type Service struct {
	topics    TopicReader
	rules     RuleReader
	index     IndexReader
	engine    gateway.Engine
	assembler *nlq.ContextAssembler
}

// New binds the actual production seams and the pinned cl100k_base assembler.
func New(topicsReader TopicReader, rules RuleReader, index IndexReader, engine gateway.Engine) (*Service, error) {
	if topicsReader == nil || rules == nil || index == nil || engine == nil {
		return nil, store.ErrInvalid
	}
	assembler, err := nlq.NewDefaultContextAssembler()
	if err != nil {
		return nil, err
	}
	return &Service{topics: topicsReader, rules: rules, index: index, engine: engine, assembler: assembler}, nil
}

type admittedTopic struct {
	id          string
	publication topics.Published
	rules       rulesets.Published
	hasRules    bool
	constraints *nlq.ConstraintState
	advisory    []nlq.OptionalItem
}

// Route performs the bounded first routing consumer. Current source and topic
// checks occur before Embed; only authorized vindex hits enter Rerank.
func (s *Service) Route(ctx context.Context, e identity.Envelope, in RouteRequest) (RouteResult, error) {
	if ctx == nil || s == nil || s.topics == nil || s.rules == nil || s.index == nil || s.engine == nil || s.assembler == nil {
		return RouteResult{}, ErrInvalid
	}
	if !e.Valid() {
		return RouteResult{}, access.ErrUnauthenticated
	}
	topicsIDs, err := normalizeRequest(in)
	if err != nil {
		return RouteResult{}, err
	}
	choiceValues, err := choiceMap(in.Choices)
	if err != nil {
		return RouteResult{}, err
	}
	result := RouteResult{Outcome: nlq.StrategySingleTopic, Topic: topicsIDs[0], Topics: append([]string(nil), topicsIDs...), Stages: []Stage{}}
	if len(topicsIDs) > 1 {
		result.Outcome = nlq.StrategyMultiTopic
	}

	admitted := make([]admittedTopic, 0, len(topicsIDs))
	for _, topic := range topicsIDs {
		started := time.Now()
		contract, err := s.topics.Contract(ctx, e, topic)
		if err != nil {
			return RouteResult{}, err
		}
		publication := contract.Publication
		if publication.State.Topic != topic || publication.Definition.Topic != topic || publication.Definition.Version != publication.State.Version || publication.State.Archived || !publication.State.Active || publication.State.Version == "" || !topics.DigestValid(publication.Digest) {
			return RouteResult{}, store.ErrConflict
		}
		item := admittedTopic{id: topic, publication: publication}
		item.rules, item.hasRules, err = s.readRules(ctx, e, topic, publication.State.Version, publication.Digest)
		if err != nil {
			return RouteResult{}, err
		}
		item.constraints, item.advisory, err = s.resolveRules(ctx, e, in, choiceValues, item)
		if err != nil {
			var clarification *Clarification
			if errors.As(err, &clarification) {
				result.Clarification = clarification
				result.Outcome = nlq.StrategyClarify
				result.RuleVersions = ruleVersions(admitted, item)
				result.TopicVersions = topicVersions(admitted, item)
				result.Stages = append(result.Stages, Stage{Name: "admission", DurationMS: time.Since(started).Milliseconds()})
				return result, nil
			}
			return RouteResult{}, err
		}
		admitted = append(admitted, item)
		result.Stages = append(result.Stages, Stage{Name: "admission", DurationMS: time.Since(started).Milliseconds()})
	}
	result.TopicVersions = make([]string, len(admitted))
	result.RuleVersions = make([]string, len(admitted))
	for i := range admitted {
		result.TopicVersions[i] = admitted[i].publication.State.Version
		if admitted[i].hasRules {
			result.RuleVersions[i] = admitted[i].rules.State.Version
		}
	}
	metrics, err := resolveMetrics(admitted, in.MetricIDs)
	if err != nil {
		return RouteResult{}, err
	}
	if len(admitted) > 1 {
		if clarification := confirmJoins(admitted, in.JoinChoices); clarification != nil {
			result.Outcome = nlq.StrategyClarify
			result.Clarification = clarification
			return result, nil
		}
	}
	if !contextMatches(admitted, in.Context) {
		return RouteResult{}, readexec.ErrBinding
	}

	resources := routeResources(e, admitted)
	partition := "route:" + vindex.Digest(struct {
		Topics  []string `json:"topics"`
		Context string   `json:"context"`
	}{topicsIDs, in.Context})
	call, err := gateway.Authorize(e, "topics.read", partition, resources...)
	if err != nil {
		return RouteResult{}, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 4, Tokens: 1 << 20, Duration: 30 * time.Second})
	if err != nil {
		return RouteResult{}, err
	}
	descriptor := s.engine.EmbeddingSpace()
	if descriptor.Key() != s.engine.Space() || !vindex.Space(descriptor).Valid() {
		return RouteResult{}, gateway.ErrSpace
	}
	kinds := append([]string(nil), in.Kinds...)
	if len(kinds) == 0 {
		kinds = []string{"topic", "entity", "measure", "dimension", "kpi", "relationship", "rule", "example"}
	}
	limit := in.LimitPerKind
	if limit == 0 {
		limit = 4
	}
	// Explain is a read-only vector-generation admission check. It validates
	// that every current topic/context has a ready generation in the exact
	// embedding space before question text is sent to Bifrost.
	admissionStarted := time.Now()
	for _, item := range admitted {
		probe := vindex.Query{
			ID:           "admission-" + vindex.Digest([]string{item.id, in.Context}),
			Topic:        item.id,
			Context:      in.Context,
			Space:        vindex.Space(descriptor),
			Vector:       admissionVector(descriptor.Dimensions),
			Kinds:        append([]string(nil), kinds...),
			LimitPerKind: limit,
		}
		if _, err := s.index.Explain(ctx, e, probe); err != nil {
			return RouteResult{}, err
		}
	}
	result.Stages = append(result.Stages, Stage{Name: "facet_admission", DurationMS: time.Since(admissionStarted).Milliseconds()})

	started := time.Now()
	embedded, err := s.engine.Embed(ctx, call, budget, descriptor.Key(), []string{in.Question})
	if err != nil {
		return RouteResult{}, err
	}
	result.RemoteCalls = append(result.RemoteCalls, embedded.Receipt.Calls...)
	embedStage := stageFromReceipt("embed", started, embedded.Receipt)
	result.Stages = append(result.Stages, embedStage)
	if embedStage.Warning != "" {
		result.Warnings = append(result.Warnings, embedStage.Warning)
	}
	if embedded.Space != descriptor.Key() || embedded.Descriptor != descriptor || len(embedded.Vectors) != 1 {
		return RouteResult{}, gateway.ErrSpace
	}

	queries := make([]vindex.Query, len(admitted))
	queryTopics := make(map[string]string, len(admitted))
	for i, item := range admitted {
		queries[i] = vindex.Query{
			ID:           "q-" + vindex.Digest([]string{item.id, in.Context, in.Question}),
			Topic:        item.id,
			Context:      in.Context,
			Space:        vindex.Space(descriptor),
			Vector:       append([]float32(nil), embedded.Vectors[0]...),
			Kinds:        append([]string(nil), kinds...),
			LimitPerKind: limit,
		}
		queryTopics[queries[i].ID] = item.id
	}
	started = time.Now()
	searchResults, err := s.index.Search(ctx, e, queries)
	if err != nil {
		return RouteResult{}, err
	}
	result.Stages = append(result.Stages, Stage{Name: "retrieval", DurationMS: time.Since(started).Milliseconds()})
	hits, err := flattenHits(queryTopics, searchResults)
	if err != nil {
		return RouteResult{}, err
	}
	if len(hits) == 0 {
		result.Outcome = nlq.StrategyNoRoute
		result.Confidence = 0
		return result, nil
	}
	result.Confidence = confidence(hits)
	if ambiguous(hits) && !in.Rerank {
		result.Outcome = nlq.StrategyClarify
		result.Clarification = &Clarification{Reason: "ambiguous_retrieval", Prompt: "Choose which retrieved meaning should answer this question."}
		return result, nil
	}

	candidates := make([]gateway.Candidate, len(hits))
	for i := range hits {
		candidateID := vindex.Digest([]string{hits[i].topic, hits[i].hit.ID, hits[i].hit.Generation})
		candidates[i] = gateway.Candidate{ID: candidateID, Text: hits[i].hit.Text, Resource: access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: hits[i].topic}}
		hits[i].candidateID = candidateID
	}
	admittedCandidates, err := gateway.AdmitCandidates(call, "topics.read", candidates)
	if err != nil {
		return RouteResult{}, err
	}
	if in.Rerank {
		started = time.Now()
		ranked, err := s.engine.Rerank(ctx, call, budget, in.Question, admittedCandidates)
		if err != nil {
			return RouteResult{}, err
		}
		result.RemoteCalls = append(result.RemoteCalls, ranked.Receipt.Calls...)
		result.Stages = append(result.Stages, stageFromReceipt("rerank", started, ranked.Receipt))
		if ranked.Receipt.Warning != "" {
			result.Warnings = append(result.Warnings, ranked.Receipt.Warning)
		}
		if err := reorderHits(hits, ranked.Items); err != nil {
			return RouteResult{}, err
		}
	}

	input := nlq.ContextInput{
		Locale:       in.Locale,
		Strategy:     result.Outcome,
		Topic:        admitted[0].id,
		TopicVersion: admitted[0].publication.State.Version,
		Topics:       topicRevisions(admitted),
		Question:     in.Question,
		Evidence:     makeEvidence(hits),
		Constraints:  mergeConstraints(admitted),
		Metrics:      metrics,
		Advisory:     mergeAdvisory(admitted),
		Examples:     append([]nlq.OptionalItem(nil), in.Examples...),
	}
	assembled, err := s.assembler.AssembleForConfidence(ctx, input, result.Confidence)
	if err != nil {
		return RouteResult{}, err
	}
	result.Context = contextView(assembled)
	result.Audit = assembled.Audit
	result.Tier = assembled.Tier
	result.Evidence = make([]vindex.Hit, len(hits))
	for i := range hits {
		result.Evidence[i] = hits[i].hit
	}
	return result, nil
}

func admissionVector(dimensions int) []float32 {
	vector := make([]float32, dimensions)
	if len(vector) > 0 {
		vector[0] = 1
	}
	return vector
}

func normalizeRequest(in RouteRequest) ([]string, error) {
	if (in.Locale != nlq.LanguageEnglish && in.Locale != nlq.LanguageSpanish) || !validQuestion(in.Question) || !identity.Identifier(in.Context) {
		return nil, ErrInvalid
	}
	topicsIDs := append([]string(nil), in.Topics...)
	if in.Topic != "" {
		if !identity.Identifier(in.Topic) {
			return nil, ErrInvalid
		}
		if len(topicsIDs) == 0 {
			topicsIDs = []string{in.Topic}
		} else if topicsIDs[0] != in.Topic {
			return nil, ErrInvalid
		}
	}
	if len(topicsIDs) < 1 || len(topicsIDs) > maxTopics {
		return nil, ErrInvalid
	}
	seen := map[string]bool{}
	for _, topic := range topicsIDs {
		if !identity.Identifier(topic) || seen[topic] {
			return nil, ErrInvalid
		}
		seen[topic] = true
	}
	if len(in.Kinds) > maxKinds || in.LimitPerKind < 0 || in.LimitPerKind > 10 || len(in.References) > maxReferences || len(in.MetricIDs) > maxMetricIDs || len(in.Examples) > maxExamples || len(in.JoinChoices) > maxTopics || len(in.Choices) > 64 {
		return nil, ErrInvalid
	}
	if len(in.JoinChoices) > 0 {
		if len(topicsIDs) < 2 {
			return nil, ErrInvalid
		}
		seenJoins := map[string]bool{}
		for _, choice := range in.JoinChoices {
			if !identity.Identifier(choice.Topic) || !identity.Identifier(choice.JoinID) {
				return nil, ErrInvalid
			}
			if seenJoins[choice.Topic] {
				return nil, ErrInvalid
			}
			seenJoins[choice.Topic] = true
		}
	}
	if len(in.Kinds) > 0 {
		seen = map[string]bool{}
		for _, kind := range in.Kinds {
			if !vindex.Kind(kind) || seen[kind] {
				return nil, ErrInvalid
			}
			seen[kind] = true
		}
	}
	seenRefs := map[semantics.Reference]bool{}
	for _, ref := range in.References {
		if !ref.Valid() || seenRefs[ref] {
			return nil, ErrInvalid
		}
		seenRefs[ref] = true
	}
	seenChoices := map[string]bool{}
	for _, choice := range in.Choices {
		if (choice.Pattern != "" && !identity.Identifier(choice.Pattern)) || !identity.Identifier(choice.Slot) || !validQuestion(choice.Value) || len(choice.Value) > 1024 {
			return nil, ErrInvalid
		}
		key := choice.Slot
		if choice.Pattern != "" {
			key = choice.Pattern + "." + choice.Slot
		}
		if seenChoices[key] {
			return nil, ErrInvalid
		}
		seenChoices[key] = true
	}
	seenMetrics := map[string]bool{}
	for _, id := range in.MetricIDs {
		if !identity.Identifier(id) {
			return nil, ErrInvalid
		}
		if seenMetrics[id] {
			return nil, ErrInvalid
		}
		seenMetrics[id] = true
	}
	for i := range in.Examples {
		if !identity.Identifier(in.Examples[i].ID) || !validQuestion(in.Examples[i].Text) || len(in.Examples[i].Text) > 16<<10 {
			return nil, ErrInvalid
		}
	}
	return topicsIDs, nil
}

func validQuestion(value string) bool {
	return utf8.ValidString(value) && len(value) > 0 && len(value) <= maxQuestionSize && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\x00\r\n")
}

func (s *Service) readRules(ctx context.Context, e identity.Envelope, topic, version, digest string) (rulesets.Published, bool, error) {
	published, err := s.rules.Read(ctx, e, topic, "")
	if errors.Is(err, store.ErrNotFound) {
		return rulesets.Published{}, false, nil
	}
	if err != nil {
		return rulesets.Published{}, false, err
	}
	if !published.State.Active || published.State.Topic != topic || published.State.Version == "" || published.Definition.Topic != topic || published.Definition.Version != published.State.Version || published.Definition.TopicVersion != version || published.Definition.PackDigest != digest || !topics.DigestValid(published.Digest) {
		return rulesets.Published{}, false, store.ErrConflict
	}
	return published, true, nil
}

func (s *Service) resolveRules(ctx context.Context, e identity.Envelope, in RouteRequest, choices map[string]string, item admittedTopic) (*nlq.ConstraintState, []nlq.OptionalItem, error) {
	if !item.hasRules {
		if len(in.References) > 0 || len(choices) > 0 {
			// Caller selections and references are meaningful only when the
			// current published ruleset can evaluate them. Do not silently drop
			// a constraint on a topic that has no active ruleset.
			return nil, nil, ErrInvalid
		}
		return nil, nil, nil
	}
	definition := item.rules.Definition
	refs := append([]semantics.Reference(nil), in.References...)
	usedChoices := map[string]bool{}
	slotCounts := map[string]int{}
	for _, pattern := range definition.Patterns {
		for _, slot := range pattern.Slots {
			slotCounts[slot.ID]++
		}
	}
	for _, pattern := range definition.Patterns {
		for _, slot := range pattern.Slots {
			key := pattern.ID + "." + slot.ID
			value, ok := choices[key]
			if !ok {
				if slotCounts[slot.ID] > 1 {
					if _, supplied := choices[slot.ID]; supplied {
						return nil, nil, ErrInvalid
					}
				}
				value, ok = choices[slot.ID]
			}
			if !ok {
				if !slot.Required {
					continue
				}
				choices := make([]ClarificationChoice, len(slot.Choices))
				for i := range slot.Choices {
					choices[i] = ClarificationChoice{ID: slot.Choices[i].ID, Label: slot.Choices[i].Label}
				}
				return nil, nil, &Clarification{Reason: "required_slot", Pattern: pattern.ID, Slot: slot.ID, Prompt: slot.Prompt, Choices: choices}
			}
			usedChoices[key], usedChoices[slot.ID] = true, true
			if strings.TrimSpace(value) == "" {
				return nil, nil, &Clarification{Reason: "required_slot", Pattern: pattern.ID, Slot: slot.ID, Prompt: slot.Prompt}
			}
			if slot.Kind == semantics.SlotChoice {
				var selected *semantics.Reference
				for i := range slot.Choices {
					if slot.Choices[i].ID == value {
						selected = slot.Choices[i].Target
						break
					}
				}
				if selected == nil && !choiceExists(slot.Choices, value) {
					return nil, nil, &Clarification{Reason: "invalid_choice", Pattern: pattern.ID, Slot: slot.ID, Prompt: slot.Prompt, Choices: choicesFor(slot.Choices)}
				}
				if selected != nil {
					refs = appendUniqueRef(refs, *selected)
				}
			}
		}
	}
	for key := range choices {
		if !usedChoices[key] {
			return nil, nil, ErrInvalid
		}
	}
	if len(refs) == 0 {
		if len(item.publication.Definition.Datasets) == 0 {
			return nil, nil, store.ErrConflict
		}
		refs = append(refs, semantics.Reference{Kind: semantics.KindDataset, ID: item.publication.Definition.Datasets[0].ID})
	}
	evaluation, err := s.rules.Evaluate(ctx, e, item.id, rulesets.EvaluateRequest{References: refs})
	if err != nil {
		return nil, nil, err
	}
	state := &nlq.ConstraintState{Allowed: evaluation.Result.Allowed}
	for _, ref := range evaluation.Result.Required {
		state.Required = append(state.Required, nlq.MandatoryConstraint{ID: referenceID(ref), Kind: "required", Text: referenceText(ref)})
	}
	for _, ref := range evaluation.Result.Excluded {
		state.Excluded = append(state.Excluded, nlq.MandatoryConstraint{ID: referenceID(ref), Kind: "excluded", Text: referenceText(ref)})
	}
	if !state.Allowed {
		return nil, nil, &Clarification{Reason: "mandatory_constraint", Prompt: "Choose a metric or dimension that satisfies the published rules."}
	}
	var advisory []nlq.OptionalItem
	for _, rule := range definition.Rules {
		if rule.Class != semantics.RuleAdvisoryContext || rule.Guidance == nil {
			continue
		}
		advisory = append(advisory, nlq.OptionalItem{ID: rule.ID, Text: rule.Guidance.Text, Priority: rule.Priority, Source: "rules"})
	}
	sort.Slice(advisory, func(i, j int) bool { return advisory[i].ID < advisory[j].ID })
	return state, advisory, nil
}

func choiceMap(selections []ChoiceSelection) (map[string]string, error) {
	values := make(map[string]string, len(selections))
	for _, selection := range selections {
		key := selection.Slot
		if selection.Pattern != "" {
			key = selection.Pattern + "." + selection.Slot
		}
		if _, exists := values[key]; exists {
			return nil, ErrInvalid
		}
		values[key] = selection.Value
	}
	return values, nil
}

func referenceID(ref semantics.Reference) string {
	parts := []string{string(ref.Kind)}
	if ref.Dataset != "" {
		parts = append(parts, ref.Dataset)
	}
	parts = append(parts, ref.ID)
	if ref.Revision > 0 {
		parts = append(parts, strconv.FormatInt(ref.Revision, 10))
	}
	return strings.Join(parts, ":")
}

func referenceText(ref semantics.Reference) string {
	text := string(ref.Kind) + " " + ref.ID
	if ref.Revision > 0 {
		text += " revision " + strconv.FormatInt(ref.Revision, 10)
	}
	return text
}

func appendUniqueRef(refs []semantics.Reference, ref semantics.Reference) []semantics.Reference {
	for _, existing := range refs {
		if existing == ref {
			return refs
		}
	}
	return append(refs, ref)
}

func choiceExists(choices []semantics.ClarificationChoice, value string) bool {
	for _, choice := range choices {
		if choice.ID == value {
			return true
		}
	}
	return false
}

func choicesFor(choices []semantics.ClarificationChoice) []ClarificationChoice {
	out := make([]ClarificationChoice, len(choices))
	for i := range choices {
		out[i] = ClarificationChoice{ID: choices[i].ID, Label: choices[i].Label}
	}
	return out
}

func confirmJoins(admitted []admittedTopic, choices []JoinChoice) *Clarification {
	if len(choices) != len(admitted) {
		return &Clarification{Reason: "ambiguous_join", Prompt: "Choose one confirmed join for each topic before combining them."}
	}
	selected := map[string]semantics.Join{}
	for _, choice := range choices {
		if _, ok := selected[choice.Topic]; ok {
			return &Clarification{Reason: "ambiguous_join", Prompt: "Choose one join per topic."}
		}
		var item *admittedTopic
		for i := range admitted {
			if admitted[i].id == choice.Topic {
				item = &admitted[i]
				break
			}
		}
		if item == nil {
			return &Clarification{Reason: "ambiguous_join", Prompt: "Choose a join from each resolved topic."}
		}
		var found *semantics.Join
		for i := range item.publication.Definition.Joins {
			if item.publication.Definition.Joins[i].ID == choice.JoinID {
				found = &item.publication.Definition.Joins[i]
				break
			}
		}
		if found == nil {
			return &Clarification{Reason: "ambiguous_join", Prompt: "Choose a published join from each resolved topic."}
		}
		if found.Cardinality != semantics.CardinalityOneToOne {
			return &Clarification{Reason: "ambiguous_cardinality", Prompt: "Choose a one-to-one relationship before combining topics."}
		}
		if !sameJoinSource(item.publication.Definition, *found) {
			return &Clarification{Reason: "unconfirmed_source", Prompt: "The selected relationship does not have one confirmed source and context."}
		}
		selected[choice.Topic] = *found
	}
	if len(selected) != len(admitted) {
		return &Clarification{Reason: "ambiguous_join", Prompt: "Choose one join for each topic."}
	}
	var source, contextID string
	var relationship joinRelationship
	for _, item := range admitted {
		join := selected[item.id]
		left, lok := datasetForReference(item.publication.Definition, join.Left)
		right, rok := datasetForReference(item.publication.Definition, join.Right)
		if !lok || !rok || left.Source.Source != right.Source.Source || left.Source.Context != right.Source.Context {
			return &Clarification{Reason: "unconfirmed_source", Prompt: "The selected relationships must share one source and execution context."}
		}
		if source == "" {
			source, contextID = left.Source.Source, left.Source.Context
		} else if source != left.Source.Source || contextID != left.Source.Context {
			return &Clarification{Reason: "unconfirmed_source", Prompt: "The selected relationships must share one source and execution context."}
		}
		candidate := normalizedRelationship(join)
		if relationship == (joinRelationship{}) {
			relationship = candidate
		} else if relationship != candidate {
			return &Clarification{Reason: "unconfirmed_relationship", Prompt: "Each topic must independently confirm the same relationship before they can be combined."}
		}
	}
	return nil
}

type joinRelationship struct {
	left, right semantics.Reference
	typeName    semantics.JoinType
	cardinality semantics.Cardinality
}

func normalizedRelationship(join semantics.Join) joinRelationship {
	left, right := join.Left, join.Right
	// Inner equality is symmetric. A left join is directional even at one-to-one
	// cardinality because it retains unmatched rows from its left endpoint.
	if join.Type == semantics.JoinInner && referenceID(right) < referenceID(left) {
		left, right = right, left
	}
	return joinRelationship{left: left, right: right, typeName: join.Type, cardinality: join.Cardinality}
}

func sameJoinSource(def topics.Definition, join semantics.Join) bool {
	left, lok := datasetForReference(def, join.Left)
	right, rok := datasetForReference(def, join.Right)
	return lok && rok && left.Source.Source == right.Source.Source && left.Source.Context == right.Source.Context
}

func contextMatches(admitted []admittedTopic, contextID string) bool {
	if !identity.Identifier(contextID) {
		return false
	}
	for _, item := range admitted {
		if len(item.publication.Definition.Datasets) == 0 {
			return false
		}
		for _, dataset := range item.publication.Definition.Datasets {
			if dataset.Source.Context != contextID {
				return false
			}
		}
	}
	return true
}

func datasetForReference(def topics.Definition, ref semantics.Reference) (topics.Dataset, bool) {
	id := ref.Dataset
	if ref.Kind == semantics.KindDataset {
		id = ref.ID
	}
	for _, dataset := range def.Datasets {
		if dataset.ID == id {
			return dataset, true
		}
	}
	return topics.Dataset{}, false
}

func routeResources(e identity.Envelope, admitted []admittedTopic) []access.Resource {
	resources := make([]access.Resource, 0, len(admitted)*4)
	seen := map[string]bool{}
	add := func(r access.Resource) {
		key := r.Kind + "\x00" + r.Permission + "\x00" + r.ID
		if !seen[key] {
			seen[key] = true
			resources = append(resources, r)
		}
	}
	for _, item := range admitted {
		add(access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: item.id})
		for _, dataset := range item.publication.Definition.Datasets {
			add(access.Resource{Tenant: e.Tenant(), Kind: "source", Permission: "read", ID: dataset.Source.Source})
			add(access.Resource{Tenant: e.Tenant(), Kind: "dataset", Permission: "query", ID: dataset.ID})
			add(access.Resource{Tenant: e.Tenant(), Kind: "execution_context", Permission: "use", ID: dataset.Source.Context})
		}
	}
	return resources
}

type hitWithTopic struct {
	topic       string
	hit         vindex.Hit
	candidateID string
}

func flattenHits(queryTopics map[string]string, results []vindex.Result) ([]hitWithTopic, error) {
	out := make([]hitWithTopic, 0)
	seenResults := make(map[string]bool, len(results))
	for _, result := range results {
		topic, ok := queryTopics[result.ID]
		if !ok || seenResults[result.ID] {
			return nil, gateway.ErrOutput
		}
		seenResults[result.ID] = true
		for _, hit := range result.Hits {
			if !identity.Identifier(hit.ID) || !vindex.Kind(hit.Kind) || !identity.Identifier(hit.SourceID) || !identity.Identifier(hit.Generation) || !identity.Identifier(hit.Version) || !identity.Identifier(hit.SourceGeneration) || !utf8.ValidString(hit.Text) || hit.Text == "" || math.IsNaN(hit.Distance) || math.IsInf(hit.Distance, 0) || hit.Distance < 0 || hit.Distance > 2 {
				return nil, gateway.ErrOutput
			}
			out = append(out, hitWithTopic{topic: topic, hit: hit})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].hit.Distance != out[j].hit.Distance {
			return out[i].hit.Distance < out[j].hit.Distance
		}
		if out[i].topic != out[j].topic {
			return out[i].topic < out[j].topic
		}
		return out[i].hit.ID < out[j].hit.ID
	})
	return out, nil
}

func confidence(hits []hitWithTopic) float64 {
	if len(hits) == 0 {
		return 0
	}
	value := 1 - hits[0].hit.Distance/2
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func ambiguous(hits []hitWithTopic) bool {
	return len(hits) > 1 && math.Abs(hits[0].hit.Distance-hits[1].hit.Distance) < 0.05
}

func reorderHits(hits []hitWithTopic, ranked []gateway.RankedItem) error {
	if len(ranked) != len(hits) {
		return gateway.ErrOutput
	}
	byID := make(map[string]hitWithTopic, len(hits))
	for _, hit := range hits {
		if _, ok := byID[hit.candidateID]; ok {
			return gateway.ErrOutput
		}
		byID[hit.candidateID] = hit
	}
	out := make([]hitWithTopic, len(ranked))
	seen := map[string]bool{}
	for i, item := range ranked {
		hit, ok := byID[item.ID]
		if !ok || seen[item.ID] || item.Score != nil && (math.IsNaN(*item.Score) || math.IsInf(*item.Score, 0)) {
			return gateway.ErrOutput
		}
		seen[item.ID] = true
		out[i] = hit
	}
	copy(hits, out)
	return nil
}

func makeEvidence(hits []hitWithTopic) []nlq.Evidence {
	out := make([]nlq.Evidence, len(hits))
	for i, item := range hits {
		confidence := 1 - item.hit.Distance/2
		if confidence < 0 {
			confidence = 0
		}
		if confidence > 1 {
			confidence = 1
		}
		out[i] = nlq.Evidence{ID: item.candidateID, Text: item.hit.Text, Priority: len(hits) - i, Source: item.topic, Confidence: &confidence}
	}
	return out
}

func mergeConstraints(admitted []admittedTopic) *nlq.ConstraintState {
	var out *nlq.ConstraintState
	seenRequired, seenExcluded := map[string]bool{}, map[string]bool{}
	for _, item := range admitted {
		if item.constraints == nil {
			continue
		}
		if out == nil {
			out = &nlq.ConstraintState{Allowed: true}
		}
		for _, constraint := range item.constraints.Required {
			if !seenRequired[constraint.ID] {
				seenRequired[constraint.ID] = true
				out.Required = append(out.Required, constraint)
			}
		}
		for _, constraint := range item.constraints.Excluded {
			if !seenExcluded[constraint.ID] {
				seenExcluded[constraint.ID] = true
				out.Excluded = append(out.Excluded, constraint)
			}
		}
	}
	return out
}

func mergeAdvisory(admitted []admittedTopic) []nlq.OptionalItem {
	var out []nlq.OptionalItem
	seen := map[string]bool{}
	for _, item := range admitted {
		for _, advisory := range item.advisory {
			if !seen[item.id+":"+advisory.ID] {
				seen[item.id+":"+advisory.ID] = true
				advisory.ID = item.id + ":" + advisory.ID
				out = append(out, advisory)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func resolveMetrics(admitted []admittedTopic, ids []string) ([]nlq.PinnedMetric, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]nlq.PinnedMetric, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		var matches []nlq.PinnedMetric
		for _, item := range admitted {
			if metric, ok := findMetric(item.publication.Definition, id); ok {
				matches = append(matches, nlq.PinnedMetric{ID: item.id + ":" + id, Text: metric})
			}
		}
		if len(matches) != 1 {
			return nil, ErrInvalid
		}
		if !seen[matches[0].ID] {
			seen[matches[0].ID] = true
			out = append(out, matches[0])
		}
	}
	return out, nil
}

func findMetric(def topics.Definition, id string) (string, bool) {
	for _, measure := range def.Measures {
		if measure.ID == id {
			return measure.Name + " (" + string(measure.Aggregation) + ")", true
		}
	}
	for _, kpi := range def.KPIs {
		if kpi.ID == id {
			return kpi.Name, true
		}
	}
	return "", false
}

func stageFromReceipt(name string, started time.Time, receipt gateway.Receipt) Stage {
	stage := Stage{Name: name, DurationMS: time.Since(started).Milliseconds(), Calls: len(receipt.Calls), Warning: receipt.Warning}
	for _, call := range receipt.Calls {
		if call.Cached {
			stage.Cached = true
		}
	}
	return stage
}

func contextView(input nlq.AssembledContext) *ContextView {
	out := &ContextView{
		Tier: input.Tier, Budget: input.Budget, Tokens: input.Tokens, Locale: input.Locale,
		Strategy: input.Strategy, Topic: input.Topic, TopicVersion: input.TopicVersion, Topics: append([]nlq.TopicRevision(nil), input.Topics...),
		Question: input.Question, Prompt: input.Prompt, Evidence: append([]nlq.Evidence(nil), input.Evidence...),
		Metrics: append([]nlq.PinnedMetric(nil), input.Metrics...), Advisory: append([]nlq.OptionalItem(nil), input.Advisory...),
		Examples: append([]nlq.OptionalItem(nil), input.Examples...),
	}
	if input.Constraints != nil {
		constraints := &nlq.ConstraintState{Allowed: input.Constraints.Allowed}
		constraints.Required = append([]nlq.MandatoryConstraint(nil), input.Constraints.Required...)
		constraints.Excluded = append([]nlq.MandatoryConstraint(nil), input.Constraints.Excluded...)
		out.Constraints = constraints
	}
	return out
}

func topicRevisions(admitted []admittedTopic) []nlq.TopicRevision {
	out := make([]nlq.TopicRevision, len(admitted))
	for i, item := range admitted {
		out[i] = nlq.TopicRevision{Topic: item.id, Version: item.publication.State.Version}
	}
	return out
}

func topicVersions(admitted []admittedTopic, item admittedTopic) []string {
	out := make([]string, 0, len(admitted)+1)
	for _, existing := range admitted {
		out = append(out, existing.publication.State.Version)
	}
	out = append(out, item.publication.State.Version)
	return out
}

func ruleVersions(admitted []admittedTopic, item admittedTopic) []string {
	out := make([]string, 0, len(admitted)+1)
	for _, existing := range admitted {
		if existing.hasRules {
			out = append(out, existing.rules.State.Version)
		}
	}
	if item.hasRules {
		out = append(out, item.rules.State.Version)
	}
	return out
}
