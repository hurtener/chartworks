//go:build chartworks_live_fixture

package acceptance

import (
	"slices"
	"testing"

	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/topics"
)

// A representative, entirely synthetic source for the local product journey.
// Profiles and the reviewed topic are created by their real domain services;
// the browser must still prepare, validate, compose and publish its own charts.
func businessDatasetFixture(t *testing.T) (*phase29ExecutionFixture, reporting.AuthoringPrepareRequest, topics.Published, string) {
	t.Helper()
	f, authoring, _, request, _, _, _ := reportDatasetFixture(t, "business-chart")
	ctx := t.Context()
	// Reuse approved columns; fresh source registration records their new native
	// types. IDs encode unique synthetic order counts and cash holds gross profit.
	_, err := f.f.f.admin.Exec(ctx, `ALTER TABLE analytics.sales
 ALTER COLUMN created_at TYPE date USING (created_at AT TIME ZONE 'UTC')::date,
 ALTER COLUMN cash TYPE numeric(30,3) USING cash::numeric;
 TRUNCATE analytics.sales;
 INSERT INTO analytics.sales(id,amount,created_at,name,cash)
 SELECT 400+month*28+region*75,
        (400+month*28+region*75)*(170+region*25)+(month%3)*1000,
        make_date(2026,month,1),
        (ARRAY['North','South','East','West'])[region],
        round(((400+month*28+region*75)*(170+region*25)+(month%3)*1000)*(0.30+region*0.015),3)
 FROM generate_series(4,9) month CROSS JOIN generate_series(1,4) region;`)
	if err != nil {
		t.Fatal("business source", err)
	}
	source := f.f.f.create(t, "business-source")
	fields := []string{"id", "amount", "name", "created_at", "cash"}
	profile := f.f.f.profile(t, f.f.f.profileSpec(t, source, "business-profile", fields, "")).Profile.Profile
	dataset := semantics.Dataset{ID: profile.Dataset, Name: "Regional sales", Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: profile.Dataset, SourceRevision: source.Revision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
	for _, column := range profile.Schema {
		if slices.Contains(fields, column.Name) {
			dataset.Columns = append(dataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable, Sensitivity: semantics.LiteralNonSensitive})
		}
	}
	ref := func(id string) semantics.Reference {
		return semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: id}
	}
	pack := semantics.TopicPack{
		SchemaVersion: semantics.SchemaVersion, Topic: "commercial-performance", Version: "v1",
		Name: "Commercial performance", Description: "Synthetic regional sales, April–September 2026. Monetary values are in USD.",
		Datasets: []semantics.Dataset{dataset},
		Measures: []semantics.Measure{
			{ID: "revenue", Name: "Revenue", Field: ref("amount"), Aggregation: semantics.AggregationSum, Unit: "USD"},
			{ID: "orders", Name: "Orders", Field: ref("id"), Aggregation: semantics.AggregationSum, Unit: "orders"},
			{ID: "gross-profit", Name: "Gross profit", Field: ref("cash"), Aggregation: semantics.AggregationSum, Unit: "USD"},
		},
		Dimensions: []semantics.Dimension{
			{ID: "region", Name: "Region", Role: semantics.DimensionCategorical, Field: ref("name")},
			{ID: "month", Name: "Month", Role: semantics.DimensionTemporal, Field: ref("created_at")},
		},
	}
	draftService, err := drafts.New(f.f.f.db, f.f.f.s, f.f.f.service)
	if err != nil {
		t.Fatal(err)
	}
	_, topicService := newPhase18Service(t, f.f)
	reviewer := f.f.f.token.envelope(t, f.author.Tenant(), f.author.User(), topicScopes(f.author.Tenant())...)
	publication := phase17PublishTopic(t, draftService, topicService, reviewer, pack)
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "topics.read", "sources.read", "sources.query", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:business-chart", "cw.block.write:business-chart", "cw.block.preview:business-chart", "cw.topic.read:" + pack.Topic, "cw.topic.write:" + pack.Topic, "cw.source.read:" + source.ID, "cw.source.query:" + source.ID, "cw.dataset.query:" + dataset.ID, "cw.execution_context.use:" + source.ContextID}
	actor := phase27Actor(t, f.f, f.author.User(), scopes)
	pin := reporting.TopicPin{Topic: pack.Topic, Version: pack.Version, Digest: publication.Digest}
	catalog, err := authoring.Dataset(ctx, actor, reporting.AuthoringDatasetRequest{Topic: pin, Dataset: dataset.ID})
	if err != nil || !catalog.Supported || len(catalog.Measures) != 3 || len(catalog.Dimensions) != 2 {
		t.Fatal("business dataset", err)
	}
	request.Intent.Topic, request.Intent.Dataset, request.Intent.Measure = pin, dataset.ID, "revenue"
	for _, measure := range catalog.Measures {
		if !measure.Supported {
			t.Fatalf("business measure %s unavailable: %s", measure.ID, measure.Reason)
		}
		if measure.ID == "revenue" {
			request.Intent.Mapping.Bindings.Value = measure.Binding
		}
	}
	request.Intent.Mapping.Options.Title = "Total revenue"
	request.Metadata = []reporting.Localized{{Locale: "en-US", Title: "Total revenue", Question: "Revenue for the selected period", Description: "Synthetic commercial performance in USD"}}
	var expected string
	if err := f.f.f.admin.QueryRow(ctx, `SELECT sum(amount)::text FROM analytics.sales`).Scan(&expected); err != nil {
		t.Fatal(err)
	}
	return f, request, publication, expected
}
