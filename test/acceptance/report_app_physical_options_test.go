package acceptance

import (
	"encoding/json"
	"net/http"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/hurtener/chartworks/internal/config"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/sources"
)

func TestReportAppPhysicalOptionsNative(t *testing.T) {
	f, s, topicAuthor, prepare, _, dataset, scopes := reportDatasetFixture(t, "physical-options")
	ctx := t.Context()
	if _, err := f.f.f.admin.Exec(ctx, `UPDATE analytics.sales SET active=(id=1), name=CASE id WHEN 1 THEN '' ELSE 'x'' OR TRUE --' END`); err != nil {
		t.Fatal(err)
	}
	rawScopes := slices.DeleteFunc(slices.Clone(scopes), func(v string) bool { return v == "topics.read" || strings.HasPrefix(v, "cw.topic.") })
	rawAuthor := phase27Actor(t, f.f, topicAuthor.User(), rawScopes)
	physical, err := f.f.f.s.DescribeDataset(ctx, rawAuthor, sources.DatasetDescribeRequest{Source: dataset.Source.Source, Context: dataset.Source.Context, Dataset: dataset.ID})
	if err != nil {
		t.Fatal(err)
	}
	pin := &reporting.SourceDatasetPin{Source: physical.Source, Context: physical.Context, Dataset: physical.Relation.ID, SourceRevision: physical.Revision, SchemaDigest: physical.SchemaDigest}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	reads, key := 0, 700
	for _, origin := range []reporting.AuthoringDatasetRequest{{SourceDataset: pin, Dataset: pin.Dataset}, {Topic: prepare.Intent.Topic, Dataset: pin.Dataset}} {
		author := topicAuthor
		actorScopes := scopes
		if origin.SourceDataset != nil {
			author = rawAuthor
			actorScopes = rawScopes
		}
		view, err := s.Dataset(ctx, author, origin)
		if err != nil {
			t.Fatal(err)
		}
		columns := map[string]string{}
		for _, c := range view.Fields.Columns {
			columns[c.SourceName] = c.ID
		}
		makeRequest := func(name, search string) reporting.AuthoringOptionRequest {
			key++
			return reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Dataset: &reporting.AuthoringDatasetOptionTarget{NewBlock: prepare.NewBlock, Topic: origin.Topic, SourceDataset: origin.SourceDataset, Dataset: origin.Dataset, Column: columns[name]}}, Operation: optionKey(key), Limit: 1, Locale: "en-US", Search: search}
		}
		for _, tc := range []struct{ name, first, last string }{{"name", "", "x' OR TRUE --"}, {"amount", "5.500", "9007199254740993.125"}, {"id", "1", "2"}, {"active", "false", "true"}} {
			in := makeRequest(tc.name, "")
			if columns[tc.name] == "" {
				count := f.attemptCount(t)
				in.Target.Dataset.Column = "column-not-in-reviewed-catalog"
				if result, err := s.DatasetOptions(ctx, author, in); err == nil || result.ValuesAvailable || f.attemptCount(t) != count {
					t.Fatal("unreviewed physical column widened topic")
				}
				continue
			}
			count := f.attemptCount(t)
			first, err := s.DatasetOptions(ctx, author, in)
			reads++
			exact, _ := json.Marshal(tc.first)
			if err != nil || !first.ValuesAvailable || first.Complete || first.Next == "" || len(first.Options) != 1 || string(first.Options[0].Value) != string(exact) || first.Options[0].Label != tc.first || f.attemptCount(t) != count+1 {
				t.Fatal("first exact physical page", tc.name, "raw", origin.SourceDataset != nil, columns, first, err)
			}
			replay, err := s.DatasetOptions(ctx, author, in)
			if err != nil || replay.ValuesAvailable || !replay.NewOperationAllowed || f.attemptCount(t) != count+1 {
				t.Fatal("physical replay executed", replay, err)
			}
			changed := in
			changed.Operation = optionKey(key + 10000)
			changed.Target = phase27Copy(t, in.Target)
			changed.Target.Dataset.Column = columns["created_at"]
			if changed.Target.Dataset.Column == "" {
				changed.Target.Dataset.Column = "missing-temporal-column"
			}
			if result, err := s.DatasetOptions(ctx, author, changed); err == nil && result.Status != "unsupported" {
				t.Fatal("temporal discrete lookup accepted", result)
			}
			key++
			in.Operation = optionKey(key)
			in.Cursor = first.Next
			last, err := s.DatasetOptions(ctx, author, in)
			reads++
			exact, _ = json.Marshal(tc.last)
			if err != nil || !last.ValuesAvailable || !last.Complete || len(last.Options) != 1 || string(last.Options[0].Value) != string(exact) {
				t.Fatal("native ordering cursor", tc.name, last, err)
			}
			in = makeRequest(tc.name, tc.last)
			match, err := s.DatasetOptions(ctx, author, in)
			reads++
			if err != nil || !match.ValuesAvailable || !match.Complete || len(match.Options) != 1 || match.Options[0].Label != tc.last {
				t.Fatal("typed exact or text search", tc.name, match, err)
			}
			if origin.SourceDataset != nil && tc.name == "amount" {
				seed := phase27Actor(t, f.f, author.User(), []string{"reporting.discover", "reporting.preview", "cw.source.read:" + pin.Source, "cw.block.read:" + prepare.NewBlock, "cw.block.write:" + prepare.NewBlock, "cw.block.preview:" + prepare.NewBlock})
				manifest, err := s.OptionDependencies(ctx, seed, reporting.OptionDependencyRequest{Mode: "status", Target: in.Target, Operation: in.Operation})
				if err != nil || !manifest.Original || slices.Contains(manifest.Actions, "topics.read") {
					t.Fatal("physical custody gained topic requirement", manifest, err)
				}
				for _, ref := range manifest.References {
					if ref.Kind == "topic" {
						t.Fatal("invented topic dependency")
					}
				}
				// The closed native HTTP schema accepts a raw origin without a topic field.
				registry, err := reportingapi.AuthoringRegistry()
				if err != nil {
					t.Fatal(err)
				}
				handler := assertRegisteredWireSchemas(t, registry, reportingapi.AuthoringHandler(f.f.f.token.verifier, s, http.NotFoundHandler()))
				next := makeRequest("amount", tc.last)
				wire, _ := json.Marshal(next)
				var body map[string]any
				_ = json.Unmarshal(wire, &body)
				delete(body["target"].(map[string]any)["dataset"].(map[string]any), "topic")
				wire, _ = json.Marshal(body)
				bearer := f.f.f.token.sign(t, f.f.f.token.claims(author.Tenant(), author.User(), actorScopes), nil)
				response := callProtected(t, handler, "POST", "/v1/reporting/authoring/v1/dataset_options", bearer, string(wire), map[string]string{"Content-Type": "application/json"})
				reads++
				if response.Code != 200 {
					t.Fatal("physical HTTP options", response.Code, response.Body.String())
				}
			}
		}
		in := makeRequest("id", "1 OR TRUE")
		count := f.attemptCount(t)
		if _, err := s.DatasetOptions(ctx, author, in); err == nil || f.attemptCount(t) != count {
			t.Fatal("invalid typed search reached source")
		}
		for _, missing := range []string{"sources.query", "cw.source.read:" + pin.Source, "cw.dataset.query:" + pin.Dataset, "cw.execution_context.use:" + pin.Context} {
			denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(actorScopes), func(v string) bool { return v == missing }))
			in = makeRequest("name", "")
			result, err := s.DatasetOptions(ctx, denied, in)
			if err == nil || !reflect.DeepEqual(result, reporting.AuthoringOptionView{}) || f.attemptCount(t) != count {
				t.Fatal("revoked physical reach", missing, result, err)
			}
		}
		if origin.SourceDataset != nil {
			in = makeRequest("name", "")
			in.Target.Dataset.SourceDataset = phase27Copy(t, pin)
			in.Target.Dataset.SourceDataset.SchemaDigest = strings.Repeat("0", 64)
			if _, err := s.DatasetOptions(ctx, author, in); err == nil || f.attemptCount(t) != count {
				t.Fatal("changed schema admitted")
			}
		}
	}
	if f.attemptCount(t) != before+reads || f.f.model.requests.Load() != models {
		t.Fatal("implicit or model work", f.attemptCount(t)-before, reads)
	}
}

func checkSourceReportOptions(t *testing.T, f *phase29ExecutionFixture, s *reporting.Authoring, e identity.Envelope, target reporting.AuthoringReportOptionTarget, parameters []reporting.Parameter, key int) {
	t.Helper()
	before := f.attemptCount(t)
	reads := 0
	for _, p := range parameters {
		if p.Column.Type != "text" && p.Column.Type != "boolean" {
			continue
		}
		target.Filter = p.Name
		key++
		in := reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Report: &target}, Operation: optionKey(key), Limit: 199, Locale: "en-US"}
		seedScopes := []string{"reporting.discover", "cw.report.read:" + target.Report}
		if target.Policy == "private_preview" {
			seedScopes = append(seedScopes, "reporting.preview", "cw.report.preview:"+target.Report)
		}
		seed := phase27Actor(t, f.f, e.User(), seedScopes)
		manifest, err := s.OptionDependencies(t.Context(), seed, reporting.OptionDependencyRequest{Mode: "search", Target: in.Target, Operation: in.Operation})
		if err != nil || manifest.Original || slices.Contains(manifest.Actions, "topics.read") {
			t.Fatal("raw report option dependencies", manifest, err)
		}
		result, err := s.ReportOptions(t.Context(), e, in)
		reads++
		want := []string{"alpha", "beta"}
		if p.Column.Type == "boolean" {
			want = []string{"true"}
		}
		got := []string{}
		for _, v := range result.Options {
			var value string
			if json.Unmarshal(v.Value, &value) != nil {
				t.Fatal("non-string typed option")
			}
			got = append(got, value)
		}
		if err != nil || !result.ValuesAvailable || !result.Complete || !reflect.DeepEqual(got, want) {
			t.Fatal("full physical population through report", result, err)
		}
		repeated, err := s.ReportOptions(t.Context(), e, in)
		if err != nil || repeated.ValuesAvailable {
			t.Fatal("report option replay", repeated, err)
		}
	}
	if reads != 2 || f.attemptCount(t) != before+reads {
		t.Fatal("report options implicit work")
	}
}

// Qualify UUID and floating-point codecs against the actual driver and validator,
// not merely the JSON conversion helper. NULL remains absent from selectable values.
func TestReportAppPhysicalOptionScalarTypes(t *testing.T) {
	f, _, _, _, _, _, _ := reportDatasetFixture(t, "scalar-options")
	ctx := t.Context()
	if _, err := f.f.f.admin.Exec(ctx, `ALTER TABLE analytics.sales ADD COLUMN token uuid, ADD COLUMN reading double precision;
 UPDATE analytics.sales SET token=CASE id WHEN 1 THEN '11111111-1111-4111-8111-111111111111'::uuid ELSE '22222222-2222-4222-8222-222222222222'::uuid END,reading=CASE id WHEN 1 THEN 1.25 ELSE 10.5 END;
 INSERT INTO analytics.sales(id) VALUES (3);`); err != nil {
		t.Fatal(err)
	}
	settings := f.f.f.cfg.Clone()
	settings.Connections[0].Relations = []config.SourceRelation{{Schema: "analytics", Name: "sales", Columns: []string{"token", "reading"}}}
	src, err := sources.New(f.f.f.db, settings, f.f.f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(src.Close)
	v, err := readexec.NewValidator(src, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	x, err := readexec.NewExecutor(src, f.f.f.db, f.f.f.values.Exec)
	if err != nil {
		t.Fatal(err)
	}
	_, topics := newPhase18Service(t, f.f)
	blocks, err := reporting.New(f.f.f.db, topics, src, v, x, nil, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	docs, err := reporting.NewDocuments(f.f.f.db, blocks, nil, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	s, err := reporting.NewAuthoring(docs, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	source, err := src.Create(ctx, f.f.f.e, sources.CreateRequest{ID: "scalar-source", Name: "Synthetic scalar fields", Connection: "warehouse"})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := src.Binding(ctx, f.f.f.e, source.ID, source.ContextID)
	if err != nil || len(binding.Relations) != 1 {
		t.Fatal(err)
	}
	datasetID := binding.Relations[0].ID
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "sources.read", "sources.query", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:scalar-options", "cw.block.write:scalar-options", "cw.block.preview:scalar-options", "cw.source.read:" + source.ID, "cw.source.query:" + source.ID, "cw.dataset.query:" + datasetID, "cw.execution_context.use:" + source.ContextID}
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	pin := &reporting.SourceDatasetPin{Source: source.ID, Context: source.ContextID, Dataset: datasetID, SourceRevision: source.Revision, SchemaDigest: readexec.Hash(binding.Relations[0])}
	catalog, err := s.Dataset(ctx, author, reporting.AuthoringDatasetRequest{SourceDataset: pin, Dataset: datasetID})
	if err != nil {
		t.Fatal(err)
	}
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	key := 900
	for _, tc := range []struct{ name, first, last string }{{"token", "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"}, {"reading", "1.25", "10.5"}} {
		column := ""
		for _, c := range catalog.Fields.Columns {
			if c.SourceName == tc.name {
				column = c.ID
			}
		}
		key++
		in := reporting.AuthoringOptionRequest{Target: reporting.AuthoringOptionTarget{Dataset: &reporting.AuthoringDatasetOptionTarget{NewBlock: "scalar-options", SourceDataset: pin, Dataset: datasetID, Column: column}}, Operation: optionKey(key), Limit: 1, Locale: "en-US"}
		first, err := s.DatasetOptions(ctx, author, in)
		if err != nil || !first.ValuesAvailable || len(first.Options) != 1 || first.Options[0].Label != tc.first || first.Next == "" {
			t.Fatal("first native scalar", tc.name, first, err)
		}
		key++
		in.Operation = optionKey(key)
		in.Cursor = first.Next
		last, err := s.DatasetOptions(ctx, author, in)
		if err != nil || !last.ValuesAvailable || !last.Complete || len(last.Options) != 1 || last.Options[0].Label != tc.last {
			t.Fatal("scalar cursor", tc.name, last, err)
		}
		key++
		in.Operation = optionKey(key)
		in.Cursor = ""
		in.Search = tc.last
		exact, err := s.DatasetOptions(ctx, author, in)
		if err != nil || !exact.ValuesAvailable || !exact.Complete || len(exact.Options) != 1 || exact.Options[0].Label != tc.last {
			t.Fatal("scalar exact search", tc.name, exact, err)
		}
	}
	if f.attemptCount(t) != before+6 || f.f.model.requests.Load() != models {
		t.Fatal("unexpected scalar source/model work")
	}
}
