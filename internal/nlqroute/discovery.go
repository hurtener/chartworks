package nlqroute

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
)

const (
	routingPolicyVersion = "evidence-v1"
	routingFloor         = 0.58
	routingMargin        = 0.08
	maxDiscoveredTopics  = 32
)

// TopicEvidence is inspectable, content-free evidence for one current topic.
// Score is a versioned bounded policy over multiple authorized facet signals;
// it is deliberately distinct from vector distance and live quality claims.
type TopicEvidence struct {
	Topic          string   `json:"topic"`
	Version        string   `json:"version"`
	Digest         string   `json:"digest"`
	BestSimilarity float64  `json:"best_similarity"`
	MeanSimilarity float64  `json:"mean_similarity"`
	Kinds          []string `json:"kinds"`
	FacetCount     int      `json:"facet_count"`
	PreRerankScore float64  `json:"pre_rerank_score"`
	RerankPosition *int     `json:"rerank_position,omitempty"`
	Score          float64  `json:"score"`
}

// RoutingDecision records deterministic topic selection policy and evidence.
type RoutingDecision struct {
	Policy        string          `json:"policy"`
	Outcome       nlq.Strategy    `json:"outcome"`
	SelectedTopic string          `json:"selected_topic,omitempty"`
	Confidence    float64         `json:"confidence"`
	Floor         float64         `json:"floor"`
	Margin        float64         `json:"margin"`
	Evidence      []TopicEvidence `json:"evidence"`
	Clarification *Clarification  `json:"clarification,omitempty"`
	Stages        []Stage         `json:"stages,omitempty"`
	RemoteCalls   []gateway.Usage `json:"remote_calls,omitempty"`
}

func (s *Service) discoverTopic(ctx context.Context, e identity.Envelope, in RouteRequest) (string, RoutingDecision, error) {
	decision := RoutingDecision{Policy: routingPolicyVersion, Outcome: nlq.StrategyNoRoute, Floor: routingFloor, Margin: routingMargin}
	catalog, ok := s.topics.(TopicCatalog)
	if !ok {
		return "", decision, ErrInvalid
	}
	started := time.Now()
	summaries, err := catalog.List(ctx, e, topics.ListRequest{Limit: maxDiscoveredTopics + 1})
	if err != nil {
		return "", decision, err
	}
	if len(summaries) > maxDiscoveredTopics {
		return "", decision, readexec.ErrLimit
	}
	var admitted []admittedTopic
	for _, summary := range summaries {
		contract, err := s.topics.Contract(ctx, e, summary.Topic)
		if err != nil {
			return "", decision, err
		}
		p := contract.Publication
		if p.State.Topic != summary.Topic || p.State.Version != summary.Version || p.State.Revision != summary.Revision || p.Digest != summary.Digest || p.Definition.Topic != summary.Topic || p.Definition.Version != summary.Version || !p.State.Active || p.State.Archived || !topics.DigestValid(p.Digest) {
			return "", decision, store.ErrConflict
		}
		if contextMatches([]admittedTopic{{id: summary.Topic, publication: p}}, in.Context) {
			admitted = append(admitted, admittedTopic{id: summary.Topic, publication: p})
		}
	}
	decision.Stages = append(decision.Stages, Stage{Name: "topic_catalog", DurationMS: time.Since(started).Milliseconds()})
	if len(admitted) == 0 {
		return "", decision, nil
	}

	descriptor := s.engine.EmbeddingSpace()
	if descriptor.Key() != s.engine.Space() || !vindex.Space(descriptor).Valid() {
		return "", decision, gateway.ErrSpace
	}
	kinds := append([]string(nil), in.Kinds...)
	if len(kinds) == 0 {
		kinds = []string{"topic", "entity", "measure", "dimension", "kpi", "relationship"}
	}
	limit := in.LimitPerKind
	if limit == 0 {
		limit = 4
	}
	for _, item := range admitted {
		probe := vindex.Query{ID: "discovery-admission-" + vindex.Digest([]string{item.id, in.Context}), Topic: item.id, Context: in.Context, Space: vindex.Space(descriptor), Vector: admissionVector(descriptor.Dimensions), Kinds: append([]string(nil), kinds...), LimitPerKind: limit}
		if _, err := s.index.Explain(ctx, e, probe); err != nil {
			return "", decision, err
		}
	}
	partition := "topic-choice:" + vindex.Digest(struct{ Context, Question string }{in.Context, in.Question})
	call, err := gateway.Authorize(e, "topics.read", partition, routeResources(e, admitted)...)
	if err != nil {
		return "", decision, err
	}
	budget, err := gateway.NewBudget(call, gateway.Limits{Calls: 2, Tokens: 1 << 20, Duration: 30 * time.Second})
	if err != nil {
		return "", decision, err
	}
	started = time.Now()
	embedded, err := s.engine.Embed(ctx, call, budget, descriptor.Key(), []string{in.Question})
	if err != nil {
		return "", decision, err
	}
	decision.Stages = append(decision.Stages, stageFromReceipt("topic_choice_embed", started, embedded.Receipt))
	decision.RemoteCalls = append(decision.RemoteCalls, embedded.Receipt.Calls...)
	if embedded.Space != descriptor.Key() || embedded.Descriptor != descriptor || len(embedded.Vectors) != 1 {
		return "", decision, gateway.ErrSpace
	}
	queries := make([]vindex.Query, len(admitted))
	queryTopics := make(map[string]string, len(admitted))
	versions := make(map[string]topics.Published, len(admitted))
	for i, item := range admitted {
		queries[i] = vindex.Query{ID: "choice-" + vindex.Digest([]string{item.id, in.Context, in.Question}), Topic: item.id, Context: in.Context, Space: vindex.Space(descriptor), Vector: append([]float32(nil), embedded.Vectors[0]...), Kinds: append([]string(nil), kinds...), LimitPerKind: limit}
		queryTopics[queries[i].ID] = item.id
		versions[item.id] = item.publication
	}
	started = time.Now()
	results, err := s.index.Search(ctx, e, queries)
	if err != nil {
		return "", decision, err
	}
	if len(results) != len(queries) {
		return "", decision, gateway.ErrOutput
	}
	decision.Stages = append(decision.Stages, Stage{Name: "topic_choice_retrieval", DurationMS: time.Since(started).Milliseconds()})
	hits, err := flattenHits(queryTopics, results)
	if err != nil {
		return "", decision, err
	}
	if len(hits) == 0 {
		return "", decision, nil
	}
	groups := map[string][]hitWithTopic{}
	for _, hit := range hits {
		groups[hit.topic] = append(groups[hit.topic], hit)
	}
	for topic, group := range groups {
		sort.SliceStable(group, func(i, j int) bool { return group[i].hit.Distance < group[j].hit.Distance })
		count := len(group)
		if count > 3 {
			count = 3
		}
		mean, kindsSeen := 0.0, map[string]bool{}
		for i := 0; i < count; i++ {
			mean += similarity(group[i].hit.Distance)
			kindsSeen[group[i].hit.Kind] = true
		}
		mean /= float64(count)
		kindNames := make([]string, 0, len(kindsSeen))
		for kind := range kindsSeen {
			kindNames = append(kindNames, kind)
		}
		sort.Strings(kindNames)
		best := similarity(group[0].hit.Distance)
		// Multiple complementary facet kinds strengthen a topic; repeated hits
		// of one kind do not manufacture confidence.
		score := clamp01(0.65*best + 0.35*mean + math.Min(float64(len(kindNames)-1)*0.03, 0.09))
		p := versions[topic]
		decision.Evidence = append(decision.Evidence, TopicEvidence{Topic: topic, Version: p.State.Version, Digest: p.Digest, BestSimilarity: best, MeanSimilarity: mean, Kinds: kindNames, FacetCount: len(group), PreRerankScore: score, Score: score})
	}
	sort.Slice(decision.Evidence, func(i, j int) bool {
		if decision.Evidence[i].PreRerankScore != decision.Evidence[j].PreRerankScore {
			return decision.Evidence[i].PreRerankScore > decision.Evidence[j].PreRerankScore
		}
		return decision.Evidence[i].Topic < decision.Evidence[j].Topic
	})
	if in.Rerank && len(decision.Evidence) > 1 {
		candidates := make([]gateway.Candidate, 0, len(decision.Evidence))
		for _, evidence := range decision.Evidence {
			var textParts []string
			for _, hit := range groups[evidence.Topic] {
				if len(textParts) == 3 {
					break
				}
				textParts = append(textParts, hit.hit.Text)
			}
			candidates = append(candidates, gateway.Candidate{ID: evidence.Topic, Text: strings.Join(textParts, "\n"), Resource: access.Resource{Tenant: e.Tenant(), Kind: "topic", Permission: "read", ID: evidence.Topic}})
		}
		admittedCandidates, err := gateway.AdmitCandidates(call, "topics.read", candidates)
		if err != nil {
			return "", decision, err
		}
		started = time.Now()
		ranked, err := s.engine.Rerank(ctx, call, budget, in.Question, admittedCandidates)
		if err != nil {
			return "", decision, err
		}
		decision.Stages = append(decision.Stages, stageFromReceipt("topic_choice_rerank", started, ranked.Receipt))
		decision.RemoteCalls = append(decision.RemoteCalls, ranked.Receipt.Calls...)
		positions := make(map[string]int, len(ranked.Items))
		for position, item := range ranked.Items {
			if _, exists := positions[item.ID]; exists {
				return "", decision, gateway.ErrOutput
			}
			positions[item.ID] = position
		}
		if len(positions) != len(decision.Evidence) {
			return "", decision, gateway.ErrOutput
		}
		denominator := len(decision.Evidence) - 1
		for i := range decision.Evidence {
			position, ok := positions[decision.Evidence[i].Topic]
			if !ok {
				return "", decision, gateway.ErrOutput
			}
			decision.Evidence[i].RerankPosition = new(int)
			*decision.Evidence[i].RerankPosition = position
			rankConfidence := 1.0
			if denominator > 0 {
				rankConfidence = 1 - float64(position)/float64(denominator)
			}
			decision.Evidence[i].Score = clamp01(0.75*decision.Evidence[i].PreRerankScore + 0.25*rankConfidence)
		}
	}
	sort.Slice(decision.Evidence, func(i, j int) bool {
		if decision.Evidence[i].Score != decision.Evidence[j].Score {
			return decision.Evidence[i].Score > decision.Evidence[j].Score
		}
		return decision.Evidence[i].Topic < decision.Evidence[j].Topic
	})
	top := decision.Evidence[0]
	decision.Confidence = top.Score
	if top.Score < routingFloor {
		return "", decision, nil
	}
	if len(decision.Evidence) > 1 && top.Score-decision.Evidence[1].Score < routingMargin {
		decision.Outcome = nlq.StrategyClarify
		decision.Clarification = &Clarification{Reason: "ambiguous_topic", Prompt: "Choose the published topic that should answer this question."}
		for _, candidate := range decision.Evidence {
			if top.Score-candidate.Score >= routingMargin {
				break
			}
			decision.Clarification.Choices = append(decision.Clarification.Choices, ClarificationChoice{ID: candidate.Topic, Label: candidate.Topic})
		}
		return "", decision, nil
	}
	decision.Outcome, decision.SelectedTopic = nlq.StrategySingleTopic, top.Topic
	return top.Topic, decision, nil
}

func similarity(distance float64) float64 { return clamp01(1 - distance/2) }
func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
