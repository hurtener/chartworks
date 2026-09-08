package acceptance

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	"github.com/hurtener/chartworks/internal/gateway"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqapi"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

type phase17Fixture struct {
	f       *engineeringFixture
	e       identity.Envelope
	pack    semantics.TopicPack
	related semantics.TopicPack
	many    semantics.TopicPack
	other   semantics.TopicPack
	service *nlqroute.Service
	model   *gatewayFixture
	context string
}

// phase17RerankFallbackEngine keeps the published embedding space while using a
// separately configured rerank role for the preserve-candidates acceptance path.
type phase17RerankFallbackEngine struct {
	embedding gateway.Engine
	reranker  gateway.Engine
}

func (e phase17RerankFallbackEngine) Generate(ctx context.Context, call gateway.Call, budget *gateway.Budget, role, instructions, input string, schema *gateway.Schema) (gateway.Generated, error) {
	return e.embedding.Generate(ctx, call, budget, role, instructions, input, schema)
}

func (e phase17RerankFallbackEngine) Embed(ctx context.Context, call gateway.Call, budget *gateway.Budget, space string, texts []string) (gateway.Embedded, error) {
	return e.embedding.Embed(ctx, call, budget, space, texts)
}

func (e phase17RerankFallbackEngine) Rerank(ctx context.Context, call gateway.Call, budget *gateway.Budget, query string, candidates gateway.Candidates) (gateway.Ranked, error) {
	return e.reranker.Rerank(ctx, call, budget, query, candidates)
}

func (e phase17RerankFallbackEngine) VisualRank(ctx context.Context, call gateway.Call, budget *gateway.Budget, query string, candidates gateway.Candidates) (gateway.Ranked, error) {
	return e.reranker.VisualRank(ctx, call, budget, query, candidates)
}

func (e phase17RerankFallbackEngine) Space() string { return e.embedding.Space() }

func (e phase17RerankFallbackEngine) EmbeddingSpace() gateway.EmbeddingSpace {
	return e.embedding.EmbeddingSpace()
}

func (phase17RerankFallbackEngine) Close() {}

func phase17SalesDataset(t *testing.T, f *engineeringFixture, sourceID, contextID, profileID string) semantics.Dataset {
	t.Helper()
	binding, err := f.s.Binding(context.Background(), f.e, sourceID, contextID)
	if err != nil {
		t.Fatal("sales binding", err)
	}
	for _, relation := range binding.Relations {
		if relation.Name != "sales" {
			continue
		}
		run := f.profile(t, engineering.ProfileSpec{ID: profileID, Source: sourceID, Context: contextID, Dataset: relation.ID, Columns: []string{"id", "amount"}, SkipLLM: true})
		evidence := run.Profile.Profile
		dataset := semantics.Dataset{ID: evidence.Dataset, Name: "Sales", Source: semantics.SourceReference{Source: sourceID, Context: contextID, Dataset: evidence.Dataset, SourceRevision: evidence.SourceRevision, ProfileVersion: evidence.Version, ProfileDigest: evidence.DeterministicHash()}}
		for _, column := range evidence.Schema {
			if column.Name == "id" || column.Name == "amount" {
				dataset.Columns = append(dataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
			}
		}
		return dataset
	}
	t.Fatal("sales relation missing")
	return semantics.Dataset{}
}

func phase17ItemsDataset(t *testing.T, f *engineeringFixture, sourceID, contextID, profileID string) semantics.Dataset {
	t.Helper()
	binding, err := f.s.Binding(context.Background(), f.e, sourceID, contextID)
	if err != nil {
		t.Fatal("items binding", err)
	}
	for _, relation := range binding.Relations {
		if relation.Name != "items" {
			continue
		}
		run := f.profile(t, engineering.ProfileSpec{ID: profileID, Source: sourceID, Context: contextID, Dataset: relation.ID, Columns: []string{"sale_id", "quantity"}, SkipLLM: true})
		evidence := run.Profile.Profile
		dataset := semantics.Dataset{ID: evidence.Dataset, Name: "Items", Source: semantics.SourceReference{Source: sourceID, Context: contextID, Dataset: evidence.Dataset, SourceRevision: evidence.SourceRevision, ProfileVersion: evidence.Version, ProfileDigest: evidence.DeterministicHash()}}
		for _, column := range evidence.Schema {
			if column.Name == "sale_id" || column.Name == "quantity" {
				dataset.Columns = append(dataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
			}
		}
		return dataset
	}
	t.Fatal("items relation missing")
	return semantics.Dataset{}
}

func phase17EnrichPack(t *testing.T, f *engineeringFixture, pack semantics.TopicPack) semantics.TopicPack {
	t.Helper()
	items := phase17ItemsDataset(t, f, pack.Datasets[0].Source.Source, pack.Datasets[0].Source.Context, "phase17-items-profile")
	pack.Datasets = append(pack.Datasets, items)
	salesID, itemsID := pack.Datasets[0].ID, items.ID
	pack.Joins = []semantics.Join{
		{ID: "sales-items", Name: "Sales to items", Left: semantics.Reference{Kind: semantics.KindColumn, Dataset: salesID, ID: "id"}, Right: semantics.Reference{Kind: semantics.KindColumn, Dataset: itemsID, ID: "sale_id"}, Type: semantics.JoinInner, Cardinality: semantics.CardinalityOneToOne},
	}
	return cloneTopic(t, pack)
}

func phase17ManyPack(t *testing.T, base semantics.TopicPack) semantics.TopicPack {
	t.Helper()
	out := cloneTopic(t, base)
	out.Topic = "commerce-many"
	out.Name = "Commerce many"
	out.Description = "Synthetic same-source cardinality fixture"
	out.Joins[0].ID = "sales-items-many"
	out.Joins[0].Name = "Sales to items many"
	out.Joins[0].Cardinality = semantics.CardinalityManyToOne
	return out
}

func phase17PublishTopic(t *testing.T, draftsService *drafts.Service, topicsService *topics.Service, e identity.Envelope, pack semantics.TopicPack) topics.Published {
	t.Helper()
	ctx := context.Background()
	draft, err := draftsService.Save(ctx, e, drafts.SaveRequest{Pack: pack, Change: "Phase 17 routing fixture"})
	if err != nil {
		t.Fatal("save topic", err)
	}
	review, err := topicsService.Review(ctx, e, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Phase 17 routing fixture"})
	if err != nil {
		t.Fatal("review topic", err)
	}
	published, err := topicsService.Publish(ctx, e, pack.Topic, topics.PublishRequest{Review: review.ID})
	if err != nil {
		t.Fatal("publish topic", err)
	}
	return published
}

func phase17PublishRules(t *testing.T, service *rulesets.Service, e identity.Envelope, published topics.Published) {
	t.Helper()
	definition := semantics.RuleSetDefinition{
		SchemaVersion: semantics.SchemaVersion,
		ID:            "phase17-rules",
		Version:       "rules-v1",
		Topic:         published.State.Topic,
		TopicVersion:  published.State.Version,
		PackDigest:    published.Digest,
		Rules: []semantics.RuleDefinition{
			{ID: "require-topic-dataset", Version: "v1", Category: semantics.RuleStructural, Class: semantics.RuleExecutionConstraint, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 100, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "phase17-review"}, Constraint: &semantics.Constraint{Kind: semantics.ConstraintRequireReference, Target: semantics.Reference{Kind: semantics.KindDataset, ID: published.Definition.Datasets[0].ID}}},
			{ID: "revenue-advisory", Version: "v1", Category: semantics.RuleSemantic, Class: semantics.RuleAdvisoryContext, Scope: semantics.RuleScope{Kind: semantics.RuleScopeTopic}, Priority: 50, Provenance: semantics.RuleProvenance{Kind: semantics.ProvenanceHuman, Evidence: "phase17-review"}, Guidance: &semantics.AdvisoryGuidance{Text: "Use the reviewed revenue definition.", Sensitivity: semantics.LiteralNonSensitive}},
		},
	}
	draft, err := service.Save(context.Background(), e, rulesets.SaveRequest{Definition: definition, Change: "Phase 17 routing constraints"})
	if err != nil {
		t.Fatal("save rules", err)
	}
	review, err := service.Review(context.Background(), e, published.State.Topic, rulesets.ReviewRequest{DraftRevision: draft.Revision, Digest: draft.Digest, Decision: "approve", Note: "Phase 17 routing constraints"})
	if err != nil {
		t.Fatal("review rules", err)
	}
	if _, err = service.Publish(context.Background(), e, published.State.Topic, rulesets.PublishRequest{Review: review.ID}); err != nil {
		t.Fatal("publish rules", err)
	}
}

func phase17OtherPack(t *testing.T, f *engineeringFixture, base semantics.TopicPack) semantics.TopicPack {
	t.Helper()
	otherSource := f.create(t, "phase17-other-source")
	out := cloneTopic(t, base)
	out.Topic = "commerce-other"
	out.Name = "Commerce other source"
	out.Description = "Synthetic second source routing fixture"
	sales := phase17SalesDataset(t, f, otherSource.ID, otherSource.ContextID, "phase17-other-sales-profile")
	items := phase17ItemsDataset(t, f, otherSource.ID, otherSource.ContextID, "phase17-other-items-profile")
	var salesID, itemsID string
	for _, dataset := range out.Datasets {
		switch dataset.Name {
		case "Sales":
			salesID = dataset.ID
		case "Items":
			itemsID = dataset.ID
		}
	}
	if salesID == "" || itemsID == "" {
		t.Fatal("base topic fixture datasets missing")
	}
	for i := range out.Datasets {
		switch out.Datasets[i].ID {
		case salesID:
			out.Datasets[i] = sales
		case itemsID:
			out.Datasets[i] = items
		}
	}
	remap := func(ref *semantics.Reference) {
		if ref.Kind == semantics.KindColumn {
			switch ref.Dataset {
			case salesID:
				ref.Dataset = sales.ID
			case itemsID:
				ref.Dataset = items.ID
			}
			return
		}
		switch ref.ID {
		case salesID:
			ref.ID = sales.ID
		case itemsID:
			ref.ID = items.ID
		}
	}
	for i := range out.Measures {
		remap(&out.Measures[i].Field)
	}
	for i := range out.Dimensions {
		remap(&out.Dimensions[i].Field)
	}
	for i := range out.KPIs {
		for j := range out.KPIs[i].Inputs {
			remap(&out.KPIs[i].Inputs[j])
		}
	}
	for i := range out.Joins {
		remap(&out.Joins[i].Left)
		remap(&out.Joins[i].Right)
	}
	for i := range out.CanonicalEntities {
		for j := range out.CanonicalEntities[i].Keys {
			remap(&out.CanonicalEntities[i].Keys[j])
		}
	}
	return out
}

func newPhase17Fixture(t *testing.T) *phase17Fixture {
	t.Helper()
	f, draftsService, topicsService, model, pack := publicationFixture(t)
	e := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	pack = phase17EnrichPack(t, f, pack)
	published := phase17PublishTopic(t, draftsService, topicsService, e, pack)
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal("index", err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal("rules", err)
	}
	phase17PublishRules(t, rules, e, published)
	related := cloneTopic(t, pack)
	related.Topic = "commerce-related"
	related.Name = "Commerce related"
	related.Description = "Synthetic same-source routing fixture"
	phase17PublishTopic(t, draftsService, topicsService, e, related)
	many := phase17ManyPack(t, pack)
	phase17PublishTopic(t, draftsService, topicsService, e, many)
	other := phase17OtherPack(t, f, pack)
	phase17PublishTopic(t, draftsService, topicsService, e, other)
	route, err := nlqroute.New(topicsService, rules, index, model.engine)
	if err != nil {
		t.Fatal("route", err)
	}
	return &phase17Fixture{f: f, e: e, pack: pack, related: related, many: many, other: other, service: route, model: model, context: pack.Datasets[0].Source.Context}
}

func phase17Paths(f *gatewayFixture) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.paths...)
}

func phase17PathCount(paths []string, suffix string) int {
	count := 0
	for _, path := range paths {
		if strings.Contains(path, suffix) {
			count++
		}
	}
	return count
}

func phase17MeasureVector(t *testing.T, fixture *phase17Fixture) []float64 {
	t.Helper()
	metadata := support.Raw(t, fixture.f.dsn)
	var literal string
	err := metadata.QueryRow(context.Background(), `SELECT embedding::text FROM chartworks.vector_facets WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3 AND generation_id=(SELECT active_generation FROM chartworks.vector_heads WHERE tenant_id=$1 AND topic_id=$2 AND context_id=$3) AND kind='measure' ORDER BY facet_id LIMIT 1`, fixture.e.Tenant(), fixture.pack.Topic, fixture.context).Scan(&literal)
	if err != nil {
		t.Fatal("measure vector", err)
	}
	parts := strings.Split(strings.Trim(literal, "[]"), ",")
	vector := make([]float64, len(parts))
	for i, part := range parts {
		value, parseErr := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			t.Fatalf("invalid stored measure vector %q: %v", literal, parseErr)
		}
		vector[i] = value
	}
	if len(vector) != 2 {
		t.Fatalf("fixture expected two-dimensional measure vector, got %d", len(vector))
	}
	return vector
}

func phase17VectorAtCosine(t *testing.T, source []float64, cosine float64) []float64 {
	t.Helper()
	if len(source) != 2 || cosine <= -1 || cosine >= 1 {
		t.Fatal("invalid target vector")
	}
	norm := math.Hypot(source[0], source[1])
	if norm == 0 || math.IsNaN(norm) || math.IsInf(norm, 0) {
		t.Fatal("invalid source vector norm")
	}
	u, v := source[0]/norm, source[1]/norm
	perpendicular := math.Sqrt(1 - cosine*cosine)
	return []float64{cosine*u - perpendicular*v, cosine*v + perpendicular*u}
}

func phase17EvidenceKinds(evidence []vindex.Hit) map[string]int {
	counts := map[string]int{}
	for _, hit := range evidence {
		counts[hit.Kind]++
	}
	return counts
}

func TestPhase17(t *testing.T) {
	fixture := newPhase17Fixture(t)
	ctx := context.Background()
	question := nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue?", Kinds: []string{"measure"}, LimitPerKind: 1}

	t.Run("AC01", func(t *testing.T) {
		request := question
		request.Question = "What is revenue for admission?"
		beforePaths := phase17Paths(fixture.model)
		before := fixture.model.requests.Load()
		out, err := fixture.service.Route(ctx, fixture.e, request)
		if err != nil || out.Context == nil || out.Outcome != nlq.StrategySingleTopic || len(out.TopicVersions) != 1 || out.TopicVersions[0] != fixture.pack.Version {
			t.Fatalf("current route admission failed: out=%#v err=%v", out, err)
		}
		if fixture.model.requests.Load() <= before || len(out.RemoteCalls) == 0 || phase17PathCount(phase17Paths(fixture.model)[len(beforePaths):], "embedding") != 1 {
			t.Fatal("real Bifrost embedding was not attributed")
		}
		narrow := fixture.f.token.envelope(t, fixture.e.Tenant(), fixture.e.User(), "topics.read", "cw.topic.read:"+fixture.pack.Topic, "cw.execution_context.use:"+fixture.context)
		before = fixture.model.requests.Load()
		if _, err = fixture.service.Route(ctx, narrow, request); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("missing dependency reach was not denied: %v", err)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("denied dependency reached Bifrost")
		}
	})

	t.Run("AC02", func(t *testing.T) {
		rerankRequest := nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue for rerank integrity?", Kinds: []string{"measure", "entity"}, LimitPerKind: 1, Rerank: true}
		beforePaths := phase17Paths(fixture.model)
		out, err := fixture.service.Route(ctx, fixture.e, rerankRequest)
		if err != nil || out.Context == nil {
			t.Fatalf("rerank route failed: out=%#v err=%v", out, err)
		}
		paths := phase17Paths(fixture.model)[len(beforePaths):]
		if phase17PathCount(paths, "embedding") != 1 || phase17PathCount(paths, "rerank") != 1 {
			t.Fatal("embedding and rerank were not both observed")
		}
		for kind, count := range phase17EvidenceKinds(out.Evidence) {
			if count > 1 {
				t.Fatalf("per-kind limit was widened for %s: %d", kind, count)
			}
		}
		seenEvidence := map[string]bool{}
		for _, item := range out.Context.Evidence {
			if seenEvidence[item.ID] {
				t.Fatalf("rerank duplicated evidence %q", item.ID)
			}
			seenEvidence[item.ID] = true
		}

		fixture.model.rerankMode.Store("duplicate")
		_, err = fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue for duplicate rerank?", Kinds: []string{"measure", "entity"}, LimitPerKind: 1, Rerank: true})
		fixture.model.rerankMode.Store("normal")
		if !errors.Is(err, gateway.ErrOutput) {
			t.Fatalf("malformed rerank was not rejected: %v", err)
		}

		fixture.model.rerankMode.Store("error")
		_, err = fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue for failed rerank?", Kinds: []string{"measure", "entity"}, LimitPerKind: 1, Rerank: true})
		fixture.model.rerankMode.Store("normal")
		if !errors.Is(err, gateway.ErrUnavailable) {
			t.Fatalf("configured rerank failure did not fail closed: %v", err)
		}

		beforePaths = phase17Paths(fixture.model)
		noRerank := rerankRequest
		noRerank.Question = "What is revenue with rerank disabled?"
		noRerank.Kinds = append([]string(nil), question.Kinds...)
		noRerank.Rerank = false
		if disabled, disabledErr := fixture.service.Route(ctx, fixture.e, noRerank); disabledErr != nil || disabled.Context == nil {
			t.Fatalf("disabled rerank route failed: out=%#v err=%v", disabled, disabledErr)
		}
		if phase17PathCount(phase17Paths(fixture.model)[len(beforePaths):], "rerank") != 0 {
			t.Fatal("disabled rerank made a provider call")
		}

		preserveModel := newGatewayFixture(t, func(cfg *config.Gateway) {
			r := cfg.Roles["rerank"]
			r.OnFailure = "preserve_candidates"
			cfg.Roles["rerank"] = r
		})
		preserveModel.rerankMode.Store("error")
		preserveIndex, preserveErr := vindex.New(fixture.f.db)
		if preserveErr != nil {
			t.Fatal(preserveErr)
		}
		preserveTopics, preserveErr := topics.New(fixture.f.db, fixture.f.s, preserveIndex, preserveModel.engine)
		if preserveErr != nil {
			t.Fatal(preserveErr)
		}
		preserveRules, preserveErr := rulesets.New(fixture.f.db, fixture.f.db)
		if preserveErr != nil {
			t.Fatal(preserveErr)
		}
		preserveEngine := phase17RerankFallbackEngine{embedding: fixture.model.engine, reranker: preserveModel.engine}
		preserveRoute, preserveErr := nlqroute.New(preserveTopics, preserveRules, preserveIndex, preserveEngine)
		if preserveErr != nil {
			t.Fatal(preserveErr)
		}
		preserved, preserveErr := preserveRoute.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue with visible fallback?", Kinds: []string{"measure", "entity"}, LimitPerKind: 1, Rerank: true})
		if preserveErr != nil || preserved.Context == nil || len(preserved.Warnings) == 0 || !strings.Contains(strings.Join(preserved.Warnings, " "), "original_order") {
			t.Fatalf("configured rerank preserve policy was not visible: out=%#v err=%v", preserved, preserveErr)
		}
	})

	t.Run("AC03", func(t *testing.T) {
		t.Cleanup(func() { fixture.model.embeddingOverride.Store([]float64{}) })
		examples := make([]nlq.OptionalItem, nlq.MaxExamples)
		for i := range examples {
			examples[i] = nlq.OptionalItem{ID: "example-" + string(rune('a'+i)), Text: "Ejemplo de ingresos", Priority: i}
		}
		stored := phase17MeasureVector(t, fixture)
		for _, tc := range []struct {
			name     string
			cosine   float64
			expected nlq.Tier
		}{{"low", -0.5, nlq.TierLow}, {"medium", 0.5, nlq.TierMedium}, {"high", 0.95, nlq.TierHigh}} {
			t.Run(tc.name, func(t *testing.T) {
				fixture.model.embeddingOverride.Store(phase17VectorAtCosine(t, stored, tc.cosine))
				request := nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageSpanish, Question: "¿Qué ingresos hay para " + tc.name + "?", Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}, Examples: examples}
				out, err := fixture.service.Route(ctx, fixture.e, request)
				if err != nil || out.Context == nil || out.Context.Locale != nlq.LanguageSpanish || len(out.Context.Metrics) != 1 || out.Tier != tc.expected {
					t.Fatalf("tiered Spanish context or pinned metric missing: want=%s out=%#v err=%v", tc.expected, out, err)
				}
				if out.Context.Tokens < 1 || out.Context.Tokens > out.Context.Budget || len(out.Context.Examples) > nlq.MaxExamples || out.Audit.OmittedCount > nlq.MaxOmissions {
					t.Fatalf("invalid tokenizer accounting: context=%#v audit=%#v", out.Context, out.Audit)
				}
				if len(out.Context.Advisory) > 1 {
					t.Fatalf("unbounded advisory lane: %d", len(out.Context.Advisory))
				}
			})
		}

		assembler, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		advisory := make([]nlq.OptionalItem, 8)
		exampleOverflow := make([]nlq.OptionalItem, 8)
		for i := range advisory {
			advisory[i] = nlq.OptionalItem{ID: "advisory-" + strconv.Itoa(i), Text: "Reviewed advisory guidance", Priority: i}
			exampleOverflow[i] = nlq.OptionalItem{ID: "overflow-example-" + strconv.Itoa(i), Text: "Detached example", Priority: i}
		}
		assembled, err := assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "bounded examples", Advisory: advisory, Examples: exampleOverflow}, nlq.TierHigh)
		if err != nil || len(assembled.Examples) > nlq.MaxExamples || assembled.Audit.OmittedCount < 1 || assembled.Audit.OmittedCount > nlq.MaxOmissions || assembled.Tokens > nlq.HighBudget {
			t.Fatalf("example/advisory bounds were not audited: context=%#v err=%v", assembled, err)
		}
	})

	t.Run("AC04", func(t *testing.T) {
		assembler, err := nlq.NewDefaultContextAssembler()
		if err != nil {
			t.Fatal(err)
		}
		constraints := make([]nlq.MandatoryConstraint, 3)
		for i := range constraints {
			constraints[i] = nlq.MandatoryConstraint{ID: "required-" + strconv.Itoa(i), Kind: "required", Text: strings.Repeat("required ", 700)}
		}
		_, err = assembler.Assemble(ctx, nlq.ContextInput{Locale: nlq.LanguageEnglish, Strategy: nlq.StrategySingleTopic, Question: "question", Constraints: &nlq.ConstraintState{Allowed: true, Required: constraints}}, nlq.TierLow)
		if !errors.Is(err, nlq.ErrInsufficient) {
			t.Fatalf("mandatory budget did not produce typed insufficiency: %v", err)
		}
		out, err := fixture.service.Route(ctx, fixture.e, nlqroute.RouteRequest{Topic: fixture.pack.Topic, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "What is revenue with active constraints?", Kinds: []string{"measure"}, LimitPerKind: 1, MetricIDs: []string{"revenue"}})
		if err != nil || out.Context == nil || len(out.Context.Metrics) != 1 || out.Context.Constraints == nil || len(out.Context.Constraints.Required) != 1 || out.Context.Constraints.Required[0].ID != "dataset:"+fixture.pack.Datasets[0].ID {
			t.Fatalf("active rule constraint or pinned metric was lost: out=%#v err=%v", out, err)
		}
		if len(out.Context.Advisory) != 1 || out.Context.Advisory[0].ID != fixture.pack.Topic+":revenue-advisory" {
			t.Fatalf("active advisory rule was not carried into context: %#v", out.Context.Advisory)
		}
	})

	t.Run("AC05", func(t *testing.T) {
		before := fixture.model.requests.Load()
		duplicate := question
		duplicate.Topic = ""
		duplicate.Topics = []string{fixture.pack.Topic, fixture.pack.Topic}
		if _, err := fixture.service.Route(ctx, fixture.e, duplicate); !errors.Is(err, nlqroute.ErrInvalid) {
			t.Fatalf("duplicate multi-topic selection was admitted: %v", err)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("invalid multi-topic selection reached Bifrost")
		}

		valid := nlqroute.RouteRequest{Topics: []string{fixture.pack.Topic, fixture.related.Topic}, Context: fixture.context, Locale: nlq.LanguageEnglish, Question: "Compare revenue across confirmed topics.", Kinds: []string{"measure"}, LimitPerKind: 1, Rerank: true, JoinChoices: []nlqroute.JoinChoice{{Topic: fixture.pack.Topic, JoinID: "sales-items"}, {Topic: fixture.related.Topic, JoinID: "sales-items"}}}
		out, err := fixture.service.Route(ctx, fixture.e, valid)
		if err != nil || out.Context == nil || out.Outcome != nlq.StrategyMultiTopic || out.Context.Strategy != nlq.StrategyMultiTopic || len(out.Topics) != 2 || len(out.TopicVersions) != 2 {
			t.Fatalf("confirmed same-source join was not routed: out=%#v err=%v", out, err)
		}
		seenTopics := map[string]bool{}
		for _, item := range out.Context.Evidence {
			seenTopics[item.Source] = true
		}
		if !seenTopics[fixture.pack.Topic] || !seenTopics[fixture.related.Topic] {
			t.Fatalf("multi-topic evidence lost a topic: %#v", out.Context.Evidence)
		}

		before = fixture.model.requests.Load()
		ambiguous := valid
		ambiguous.Topics = []string{fixture.pack.Topic, fixture.many.Topic}
		ambiguous.Question = "Compare revenue with ambiguous grain."
		ambiguous.Rerank = false
		ambiguous.JoinChoices = []nlqroute.JoinChoice{{Topic: fixture.pack.Topic, JoinID: "sales-items"}, {Topic: fixture.many.Topic, JoinID: "sales-items-many"}}
		ambiguousOut, ambiguousErr := fixture.service.Route(ctx, fixture.e, ambiguous)
		if ambiguousErr != nil || ambiguousOut.Outcome != nlq.StrategyClarify || ambiguousOut.Clarification == nil || ambiguousOut.Clarification.Reason != "ambiguous_cardinality" {
			t.Fatalf("ambiguous cardinality was not clarified: out=%#v err=%v", ambiguousOut, ambiguousErr)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("ambiguous join reached Bifrost")
		}

		before = fixture.model.requests.Load()
		crossSource := valid
		crossSource.Question = "Compare revenue across unconfirmed sources."
		crossSource.Topics = []string{fixture.pack.Topic, fixture.other.Topic}
		crossSource.JoinChoices = []nlqroute.JoinChoice{{Topic: fixture.pack.Topic, JoinID: "sales-items"}, {Topic: fixture.other.Topic, JoinID: "sales-items"}}
		crossOut, crossErr := fixture.service.Route(ctx, fixture.e, crossSource)
		if crossErr != nil || crossOut.Outcome != nlq.StrategyClarify || crossOut.Clarification == nil || crossOut.Clarification.Reason != "unconfirmed_source" {
			t.Fatalf("cross-source join was not rejected: out=%#v err=%v", crossOut, crossErr)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("cross-source join reached Bifrost")
		}

		limitedScopes := []string{"topics.read", "sources.read", "cw.topic.read:" + fixture.pack.Topic, "cw.topic.read:" + fixture.related.Topic, "cw.source.read:" + fixture.pack.Datasets[0].Source.Source, "cw.execution_context.use:" + fixture.context}
		limited := fixture.f.token.envelope(t, fixture.e.Tenant(), fixture.e.User(), limitedScopes...)
		before = fixture.model.requests.Load()
		if _, err = fixture.service.Route(ctx, limited, valid); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, access.ErrNotFound) {
			t.Fatalf("missing joined dataset reach was admitted: %v", err)
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("missing joined dataset reached Bifrost")
		}
	})

	t.Run("AC06", func(t *testing.T) {
		registry, err := nlqapi.Registry()
		if err != nil {
			t.Fatal(err)
		}
		handler := assertRegisteredWireSchemas(t, registry, nlqapi.Handler(fixture.f.token.verifier, fixture.service, http.NotFoundHandler()))
		server := httptest.NewServer(handler)
		defer server.Close()
		client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
			return fixture.f.token.sign(t, fixture.f.token.claims(fixture.e.Tenant(), fixture.e.User(), fixture.e.Scopes()), nil), nil
		})
		if err != nil {
			t.Fatal(err)
		}
		wireRequest := question
		wireRequest.Question = "What is revenue over the HTTP route?"
		if out, callErr := client.RouteNLQ(ctx, wireRequest); callErr != nil || out.Context == nil {
			t.Fatalf("HTTP/SDK route failed: out=%#v err=%v", out, callErr)
		}

		inputCopy := question
		inputCopy.Question = "What is revenue with detached input?"
		inputCopy.Kinds = append([]string(nil), question.Kinds...)
		inputCopy.Examples = []nlq.OptionalItem{{ID: "detached-example", Text: "Original detached example", Priority: 1}}
		out, err := fixture.service.Route(ctx, fixture.e, inputCopy)
		if err != nil || out.Context == nil {
			t.Fatalf("detached-input route failed: out=%#v err=%v", out, err)
		}
		inputCopy.Examples[0].Text = "mutated caller example"
		inputCopy.Kinds[0] = "entity"
		if strings.Contains(out.Context.Prompt, "mutated caller example") || !strings.Contains(out.Context.Prompt, "Original detached example") {
			t.Fatalf("route retained mutable caller input: %q", out.Context.Prompt)
		}

		before := fixture.model.requests.Load()
		other := question
		other.Topic = ""
		other.Topics = []string{fixture.other.Topic}
		other.Context = fixture.other.Datasets[0].Source.Context
		other.Question = question.Question
		otherOut, otherErr := fixture.service.Route(ctx, fixture.e, other)
		if otherErr != nil || otherOut.Context == nil || otherOut.Topic != fixture.other.Topic {
			t.Fatalf("same-tenant different-context route failed: out=%#v err=%v", otherOut, otherErr)
		}
		if fixture.model.requests.Load() <= before {
			t.Fatal("embedding cache crossed topic/context authority boundary")
		}
		before = fixture.model.requests.Load()
		wrongContext := question
		wrongContext.Context = fixture.other.Datasets[0].Source.Context
		wrongContext.Question = "Wrong context must stop before embedding"
		if _, err = fixture.service.Route(ctx, fixture.e, wrongContext); err == nil {
			t.Fatal("topic/context mismatch was admitted")
		}
		if fixture.model.requests.Load() != before {
			t.Fatal("topic/context mismatch reached Bifrost")
		}

		english := question
		english.Question = "Concurrent English revenue"
		spanish := question
		spanish.Locale = nlq.LanguageSpanish
		spanish.Question = "¿Qué ingresos hay?"
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		spanishEnvelope := fixture.f.token.envelope(t, fixture.e.Tenant(), "reader-es", fixture.e.Scopes()...)
		requests := []struct {
			envelope identity.Envelope
			request  nlqroute.RouteRequest
		}{{fixture.e, english}, {spanishEnvelope, spanish}}
		for _, item := range requests {
			item := item
			wg.Add(1)
			go func() {
				defer wg.Done()
				out, err := fixture.service.Route(ctx, item.envelope, item.request)
				if err != nil || out.Context == nil || out.Context.Locale != item.request.Locale || len(out.Stages) < 3 || len(out.RemoteCalls) == 0 {
					errs <- errors.New("concurrent route did not return stage attribution")
				}
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			t.Fatal(err)
		}
	})
}
