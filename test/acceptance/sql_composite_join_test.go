package acceptance

import (
	"encoding/json"
	"errors"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/vindex"
	"strings"
	"testing"
)

func TestSQLRecoveryCompositeJoinLifecycleAcceptance(t *testing.T) {
	f := newEngineeringFixture(t, nil, nil)
	_, err := f.admin.Exec(t.Context(), `TRUNCATE analytics.sales,analytics.items; ALTER TABLE analytics.sales DROP CONSTRAINT sales_pkey; ALTER TABLE analytics.sales ADD PRIMARY KEY(id,amount); ALTER TABLE analytics.items ALTER COLUMN quantity TYPE numeric(30,3); ALTER TABLE analytics.items ADD UNIQUE(sale_id,quantity); INSERT INTO analytics.sales(id,amount) VALUES(1,10),(1,20),(2,30); INSERT INTO analytics.items VALUES(1,10),(1,20),(1,999),(2,30)`)
	if err != nil {
		t.Fatal(err)
	}
	source := f.create(t, "composite-source")
	binding, err := f.s.Binding(t.Context(), f.e, source.ID, source.ContextID)
	if err != nil {
		t.Fatal(err)
	}
	pack := semantics.TopicPack{SchemaVersion: 1, Topic: "composite-revenue", Version: "v1", Name: "Composite revenue", Description: "Reviewed complete two-column relationship"}
	for _, rel := range binding.Relations {
		fields := []string{"id", "amount"}
		if rel.Name == "items" {
			fields = []string{"sale_id", "quantity"}
		} else if rel.Name != "sales" {
			continue
		}
		profile := f.profile(t, engineering.ProfileSpec{ID: "composite-" + rel.Name, Source: source.ID, Context: source.ContextID, Dataset: rel.ID, Columns: fields, SkipLLM: true}).Profile.Profile
		d := semantics.Dataset{ID: rel.ID, Name: rel.Name, Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: rel.ID, SourceRevision: source.Revision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
		for _, c := range profile.Schema {
			for _, field := range fields {
				if c.Name == field {
					d.Columns = append(d.Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable, Sensitivity: semantics.LiteralNonSensitive})
				}
			}
		}
		pack.Datasets = append(pack.Datasets, d)
	}
	var sales, items string
	for _, d := range pack.Datasets {
		if d.Name == "sales" {
			sales = d.ID
		} else if d.Name == "items" {
			items = d.ID
		}
	}
	ref := func(ds, id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: ds, ID: id}
	}
	pack.Measures = []semantics.Measure{{ID: "revenue", Name: "Revenue", Description: "Exact sales sum", Field: ref(sales, "amount"), Aggregation: semantics.AggregationSum, Unit: "currency"}}
	pack.Dimensions = []semantics.Dimension{{ID: "item_record", Name: "Item record", Description: "Reviewed joined grouping", Role: semantics.DimensionIdentifier, Field: ref(items, "sale_id")}}
	pack.Joins = []semantics.Join{{ID: "sales-items-composite", Name: "Complete sales item key", Left: ref(sales, "id"), Right: ref(items, "sale_id"), AdditionalKeys: []semantics.JoinKeyPair{{Left: ref(sales, "amount"), Right: ref(items, "quantity")}}, Type: semantics.JoinInner, Cardinality: semantics.CardinalityOneToOne}}
	model := newGatewayFixture(t, func(c *config.Gateway) {
		r := c.Roles["embedding"]
		r.MaxBatchItems = 64
		r.MaxBatchBytes = 64 << 10
		c.Roles["embedding"] = r
	})
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := topics.New(f.db, f.s, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	author := f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...)
	phase17PublishTopic(t, draft, topic, author, pack)
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	router, err := nlqroute.New(topic, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	query, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	actor := f.token.envelope(t, f.e.Tenant(), f.e.User(), phase18Scopes(f.e.Tenant(), true)...)
	model.embeddingMode.Store("fixed")
	model.rerankMode.Store("fixed")
	sql := `SELECT i.sale_id,sum(s.amount) AS revenue FROM analytics.sales s JOIN analytics.items i ON s.id=i.sale_id AND s.amount=i.quantity GROUP BY i.sale_id ORDER BY i.sale_id`
	request := nlqexec.QuestionRequest{Topic: pack.Topic, Context: source.ContextID, Question: "Revenue by Item record", Locale: nlq.LanguageEnglish, MetricIDs: []string{"revenue"}, Kinds: []string{"measure", "dimension"}, LimitPerKind: 5}
	model.mode.Store(phase18RawResponse(t, sql))
	p, err := query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request})
	if err != nil {
		t.Fatal("authored composite plan", err)
	}
	out, err := query.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"})
	if err != nil || out.Execution.Result == nil || len(out.Execution.Result.Rows) != 2 {
		t.Fatal("composite result", err)
	}
	for _, row := range out.Execution.Result.Rows {
		var amount string
		if json.Unmarshal(row[1], &amount) != nil || amount != "30.000" {
			t.Fatal("fanout changed result", string(row[1]))
		}
	}
	restarted, err := nlqexec.New(router, topic, f.s, f.validator, f.executor, model.engine, f.db)
	if err != nil {
		t.Fatal(err)
	}
	before := model.requests.Load()
	if _, err = restarted.Run(t.Context(), actor, nlqexec.RunRequest{QueryID: p.QueryID, Operation: p.QueryID + "-run"}); err != nil || model.requests.Load() != before {
		t.Fatal("composite replay", err)
	}
	model.mode.Store(phase18RawResponse(t, strings.Replace(sql, " AND s.amount=i.quantity", "", 1)))
	if _, err = query.Plan(t.Context(), actor, nlqexec.PlanRequest{QuestionRequest: request}); !errors.Is(err, readexec.ErrAnalyticalMismatch) {
		t.Fatal("partial key accepted", err)
	}
	// The omitted-key query really multiplies rows; this is not just a parser fixture.
	var duplicated string
	if err = f.admin.QueryRow(t.Context(), `SELECT sum(s.amount)::text FROM analytics.sales s JOIN analytics.items i ON s.id=i.sale_id`).Scan(&duplicated); err != nil || duplicated != "120.000" {
		t.Fatal("independent fanout baseline", duplicated, err)
	}
}
