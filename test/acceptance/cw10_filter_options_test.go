package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/store"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

func TestCW10FilterOptions(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := context.Background()
	block := phase27Copy(t, f.base)
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE amount >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "minimum", Type: "number", Required: true, Default: &reporting.Value{Literal: "0"}}}
	f.block(t, "cw10-options-block", block)
	_, topicService := newPhase18Service(t, f.f)
	published, err := topicService.Read(ctx, f.blockAuthor, block.Topics[0].Topic, block.Topics[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	dataset := published.Definition.Datasets[0]
	column := ""
	for _, candidateDataset := range published.Definition.Datasets {
		for _, candidate := range candidateDataset.Columns {
			if candidate.SourceName == "amount" {
				dataset = candidateDataset
				column = candidate.ID
			}
		}
	}
	if column == "" {
		t.Fatal("reviewed amount column absent")
	}
	if _, err = f.f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,name) VALUES(3,NULL,'three')`); err != nil {
		t.Fatal(err)
	}
	d := phase29Text("Revision-bound choices")
	filter := reporting.ReportFilter{Label: "Amount", Parameter: reporting.Parameter{Name: "amount_filter", Type: "number", Required: true, Default: &reporting.Value{Literal: "0"}}, Options: &reporting.FilterOptionSource{Version: 1, Block: "cw10-options-block", BlockRevision: 1, Topic: published.Definition.Topic, TopicVersion: published.Definition.Version, Dataset: dataset.ID, Column: column}}
	d.Filters = []reporting.ReportFilter{filter}
	widget := phase29BlockWidget("values", "cw10-options-block", 1, "table-main")
	widget.Block.Revision = 1
	widget.Bindings = []reporting.FilterBinding{{Filter: "amount_filter", Parameter: "minimum"}}
	d.Widgets = append(d.Widgets, widget)
	state := f.report(t, "cw10-options-report", d, true)

	request := reporting.FilterOptionsRequest{Revision: state.PublishedRevision, Filter: "amount_filter", Limit: 1, Locale: "es-AR"}
	first, err := f.documents.FilterOptions(ctx, f.execute, state.ID, request)
	if err != nil || first.Complete || len(first.Options) != 1 || first.Next == "" || first.SourceRevision < 1 {
		t.Fatal("first bounded page", first, err)
	}
	seen := map[string]bool{string(first.Options[0].Value): true}
	for !first.Complete {
		request.Cursor = first.Next
		first, err = f.documents.FilterOptions(ctx, f.execute, state.ID, request)
		if err != nil {
			t.Fatal("next deterministic page", err)
		}
		for _, option := range first.Options {
			if seen[string(option.Value)] {
				t.Fatal("duplicate option across keyset pages", string(option.Value))
			}
			seen[string(option.Value)] = true
			if string(option.Value) == "null" && option.Label != "Nulo" {
				t.Fatal("typed null lost localized label", option)
			}
		}
	}
	if len(seen) != 3 || !seen["null"] {
		t.Fatal("distinct typed option set incomplete", seen)
	}

	handler := reportingapi.DocumentsHandler(f.f.f.token.verifier, f.documents, f.compositions, http.NotFoundHandler())
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	bearer := phase27Token(t, f.f, f.execute.User(), f.execute.Session(), phase29RuntimeScopes(f.execute.Tenant()))
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return bearer, nil })
	if err != nil {
		t.Fatal(err)
	}
	wire, err := client.ReportFilterOptions(ctx, state.ID, sdk.DocumentFilterOptionsRequest{Revision: state.PublishedRevision, Filter: "amount_filter", Limit: 1, Locale: "es-AR"})
	if err != nil || len(wire.Options) != 1 || wire.Next == "" || wire.Report != state.ID {
		t.Fatal("HTTP/SDK option transport", wire, err)
	}

	request.Cursor = ""
	page, err := f.documents.FilterOptions(ctx, f.execute, state.ID, request)
	if err != nil || page.Next == "" {
		t.Fatal(err)
	}
	tampered := page.Next[:len(page.Next)-1] + "x"
	if strings.HasSuffix(page.Next, "x") {
		tampered = page.Next[:len(page.Next)-1] + "y"
	}
	before := f.attemptCount(t)
	for _, bad := range []reporting.FilterOptionsRequest{
		{Revision: 2, Filter: request.Filter, Limit: request.Limit, Locale: request.Locale},
		{Revision: request.Revision, Filter: "missing", Limit: request.Limit, Locale: request.Locale},
		{Revision: request.Revision, Filter: request.Filter, Limit: 2, Locale: request.Locale, Cursor: page.Next},
		{Revision: request.Revision, Filter: request.Filter, Search: "changed", Limit: request.Limit, Locale: request.Locale, Cursor: page.Next},
		{Revision: request.Revision, Filter: request.Filter, Limit: 1, Locale: request.Locale, Cursor: tampered},
	} {
		if _, callErr := f.documents.FilterOptions(ctx, f.execute, state.ID, bad); callErr == nil {
			t.Fatal("stale or tampered coordinate executed", bad)
		}
	}
	if f.attemptCount(t) != before {
		t.Fatal("denied option coordinates reached the warehouse")
	}

	limitedScopes := slices.DeleteFunc(phase29RuntimeScopes(f.execute.Tenant()), func(scope string) bool {
		return strings.HasPrefix(scope, "cw.execution_context.use:")
	})
	limited := phase27Actor(t, f.f, "cw10-limited", limitedScopes)
	if _, err := f.documents.FilterOptions(ctx, limited, state.ID, request); !errors.Is(err, access.ErrForbidden) && !errors.Is(err, store.ErrNotFound) {
		t.Fatal("wrong context reach was not denied", err)
	}
	foreign := f.f.f.token.envelope(t, "cw10-foreign-tenant", f.execute.User(), phase29RuntimeScopes("cw10-foreign-tenant")...)
	if _, err := f.documents.FilterOptions(ctx, foreign, state.ID, request); !errors.Is(err, access.ErrNotFound) && !errors.Is(err, store.ErrNotFound) {
		t.Fatal("cross-tenant report coordinates were disclosed", err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("authority denials reached the warehouse")
	}

	request.Cursor = page.Next
	var wg sync.WaitGroup
	results := make(chan reporting.FilterOptionsPage, 4)
	errs := make(chan error, 4)
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, callErr := f.documents.FilterOptions(ctx, f.execute, state.ID, request)
			results <- result
			errs <- callErr
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatal("concurrent cursor reuse", callErr)
		}
	}
	var expected string
	for result := range results {
		if raw := string(result.Options[0].Value); expected == "" {
			expected = raw
		} else if raw != expected {
			t.Fatal("concurrent cursor changed ordering", raw, expected)
		}
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	request.Cursor = ""
	before = f.attemptCount(t)
	if _, err := f.documents.FilterOptions(cancelled, f.execute, state.ID, request); err == nil {
		t.Fatal("cancelled option request admitted")
	}
	if f.attemptCount(t) != before {
		t.Fatal("cancelled option request reached the warehouse")
	}
	amended, err := f.documents.Edit(ctx, f.author, "report", state.ID, state.Version, reporting.DocumentReference{}, d)
	if err != nil {
		t.Fatal(err)
	}
	phase29Publish(t, f.documents, f.author, amended)
	before = f.attemptCount(t)
	request.Cursor = page.Next
	if _, err = f.documents.FilterOptions(ctx, f.execute, state.ID, request); !errors.Is(err, store.ErrConflict) && !errors.Is(err, reporting.ErrStale) {
		t.Fatal("superseded report revision remained executable", err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("superseded report revision reached the warehouse")
	}
}

func TestCW10FilterOptionSearchAndOversize(t *testing.T) {
	f := newPhase27CatalogFixture(t)
	ctx := context.Background()
	block := phase27Definition(t, f.phase17Fixture, f.actor, "SELECT id, amount FROM analytics.sales ORDER BY id")
	block.SQL = "SELECT id, amount FROM analytics.sales WHERE name >= $1 ORDER BY id"
	block.Parameters = []reporting.Parameter{{Name: "category", Type: "dimension_value", Required: true, Default: &reporting.Value{Literal: "one"}, Dimension: &reporting.DimensionReference{Topic: f.pack.Topic, Version: f.pack.Version, Dimension: "category"}}}
	created, err := f.blocks.Create(ctx, f.actor, reporting.CreateRequest{ID: "cw10-search-block", Definition: block})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.blocks, f.actor, created)
	documents, err := reporting.NewDocuments(f.f.db, f.blocks, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	author := phase27Actor(t, f.phase17Fixture, f.actor.User(), phase29AuthorScopes(f.actor.Tenant()))
	execute := phase27Actor(t, f.phase17Fixture, f.actor.User(), phase29RuntimeScopes(f.actor.Tenant()))
	dataset := f.pack.Datasets[0]
	var column, amountColumn string
	for _, candidate := range dataset.Columns {
		if candidate.SourceName == "name" {
			column = candidate.ID
		}
		if candidate.SourceName == "amount" {
			amountColumn = candidate.ID
		}
	}
	if column == "" || amountColumn == "" {
		t.Fatal("reviewed option columns absent")
	}
	d := phase29Text("Searchable choices")
	d.Filters = []reporting.ReportFilter{{Label: "Category", Parameter: block.Parameters[0], Options: &reporting.FilterOptionSource{Version: 1, Block: created.State.ID, BlockRevision: 1, Topic: f.pack.Topic, TopicVersion: f.pack.Version, Dataset: dataset.ID, Column: column}}}
	widget := phase29BlockWidget("categories", created.State.ID, 1, "table-main")
	widget.Block.Revision = 1
	widget.Bindings = []reporting.FilterBinding{{Filter: "category", Parameter: "category"}}
	d.Widgets = append(d.Widgets, widget)
	for _, invalid := range []struct{ id, column string }{{"cw10-missing-option", "missing"}, {"cw10-wrong-option-type", amountColumn}} {
		bad := phase27Copy(t, d)
		bad.Filters[0].Options.Column = invalid.column
		if _, err := documents.Create(ctx, author, "report", invalid.id, bad); !errors.Is(err, reporting.ErrStale) {
			t.Fatal("publication accepted an invalid semantic option chain", invalid.id, err)
		}
	}
	state, err := documents.Create(ctx, author, "report", "cw10-search-report", d)
	if err != nil {
		t.Fatal(err)
	}
	state = phase29Publish(t, documents, author, state)
	page, err := documents.FilterOptions(ctx, execute, state.ID, reporting.FilterOptionsRequest{Revision: state.PublishedRevision, Filter: "category", Search: "TW", Limit: 10, Locale: "en-US"})
	if err != nil || !page.Complete || len(page.Options) != 1 || string(page.Options[0].Value) != `"two"` || page.Options[0].Label != "two" {
		t.Fatal("escaped case-insensitive search", page, err)
	}
	if _, err = f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,name) VALUES(76,7,'rate%')`); err != nil {
		t.Fatal(err)
	}
	page, err = documents.FilterOptions(ctx, execute, state.ID, reporting.FilterOptionsRequest{Revision: state.PublishedRevision, Filter: "category", Search: "%", Limit: 10, Locale: "en-US"})
	if err != nil || len(page.Options) != 1 || string(page.Options[0].Value) != `"rate%"` {
		t.Fatal("LIKE wildcard was not treated literally", page, err)
	}
	oversize := strings.Repeat("z", 1025)
	if _, err = f.f.admin.Exec(ctx, `INSERT INTO analytics.sales(id,amount,name) VALUES(77,7,$1)`, oversize); err != nil {
		t.Fatal(err)
	}
	if _, err = documents.FilterOptions(ctx, execute, state.ID, reporting.FilterOptionsRequest{Revision: state.PublishedRevision, Filter: "category", Search: "zzz", Limit: 10, Locale: "en-US"}); !errors.Is(err, reporting.ErrBudget) {
		t.Fatal("oversized label was not rejected", err)
	}
	drift, err := documents.Create(ctx, author, "report", "cw10-publish-drift", d)
	if err != nil {
		t.Fatal(err)
	}
	drift, err = documents.Transition(ctx, author, "report", drift.ID, drift.Version, drift.LatestRevision, "review", "Ready before source rotation")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.f.s.Rotate(ctx, f.f.e, dataset.Source.Source, dataset.Source.SourceRevision); err != nil {
		t.Fatal(err)
	}
	if _, err = documents.Transition(ctx, author, "report", drift.ID, drift.Version, drift.ReviewRevision, "publish", "Must revalidate exact source revision"); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("publication accepted stale source/semantic option chain", err)
	}
}
