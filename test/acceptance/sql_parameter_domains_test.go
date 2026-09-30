package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/nlq"
	"github.com/hurtener/chartworks/internal/nlq/exampleparams"
	"github.com/hurtener/chartworks/internal/nlqexec"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Real PostgreSQL source EXPLAIN/execution and durable metadata; model answers
// are recorded fixtures, not live language/model quality evidence.
func TestSQLRecoveryParameterDomainsLifecycleAcceptance(t *testing.T) {
	for _, tc := range []struct{ domain, sql, old, next string }{
		{"date", `SELECT id FROM analytics.sales WHERE created_at::date >= $1::date ORDER BY id`, "20260103", "20260102"},
		{"timestamp", `SELECT id FROM analytics.sales WHERE created_at::timestamp >= $1::timestamp ORDER BY id`, "20260103 00:00:00", "20260102 00:00:00"},
		{"timestamptz", `SELECT id FROM analytics.sales WHERE created_at >= $1 ORDER BY id`, "20260103 00:00:00+00", "20260102 00:00:00+00"},
		{"uuid", `SELECT id FROM analytics.sales WHERE $1::uuid = $1::uuid ORDER BY id`, "4b73ce10-a327-4e31-a132-d635b414a117", "c60113cd-c8fa-4a8a-9a61-4758e0914bd9"},
		{"time", `SELECT id FROM analytics.sales WHERE $1::time IS NOT NULL ORDER BY id`, "15:16:17", "04:05:06"},
		{"timetz", `SELECT id FROM analytics.sales WHERE $1::timetz IS NOT NULL ORDER BY id`, "15:16:17+02:00", "04:05:06+00:00"},
		{"interval", `SELECT id FROM analytics.sales WHERE $1::interval IS NOT NULL ORDER BY id`, "2 days 03:04:05", "3 days 04:05:06"},
		{"json", `SELECT id FROM analytics.sales WHERE $1::json IS NOT NULL ORDER BY id`, `{"label":"prior-synthetic"}`, `{"label":"current-synthetic"}`},
		{"jsonb", `SELECT id FROM analytics.sales WHERE $1::jsonb IS NOT NULL ORDER BY id`, `{"label":"prior-synthetic"}`, `{"label":"current-synthetic"}`},
	} {
		t.Run(tc.domain, func(t *testing.T) {
			f := newCW01Fixture(t)
			ctx := context.Background()
			if _, err := f.f.admin.Exec(ctx, `UPDATE analytics.sales SET created_at=CASE id WHEN 1 THEN '2026-01-02T12:00:00Z'::timestamptz ELSE '2026-01-03T12:00:00Z'::timestamptz END`); err != nil {
				t.Fatal(err)
			}
			question := f.question("List sales IDs using "+tc.old, nlq.LanguageEnglish)
			f.model.mode.Store(parameterResponse(t, tc.sql, []readexec.Parameter{{Kind: "text", Value: tc.old}}))
			p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: question})
			if err != nil {
				t.Fatal("original typed source query", err)
			}
			if err = f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
				t.Fatal("domain feedback", err)
			}
			rows, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
			if err != nil || len(rows) != 1 {
				t.Fatal("domain candidate", len(rows), err)
			}
			candidate := rows[0]
			schema := candidate.ParameterSchema
			if schema == nil || schema.Version != exampleparams.DomainVersion || len(schema.Slots) != 1 || schema.Slots[0].Domain != tc.domain || schema.Slots[0].Kind != "text" {
				t.Fatal("native domain not retained", schema)
			}
			raw, _ := json.Marshal(candidate)
			if strings.Contains(string(raw), tc.old) {
				t.Fatal("historical value retained")
			}
			metadata := support.Raw(t, f.f.dsn)
			attempts, calls := count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`), f.model.requests.Load()
			active, err := f.query.ExampleState(ctx, f.e, nlqexec.ExampleStateRequest{ExampleID: candidate.ID, State: "active", ExpectedVersion: candidate.Version, ReviewNote: "Reviewed value-free native domain"})
			if err != nil || active.State != "active" {
				t.Fatal("public-probe native review", err)
			}
			if attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || calls != f.model.requests.Load() {
				t.Fatal("review ran source execution/model")
			}
			bundle, err := f.query.ExportExamples(ctx, f.e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
			if err != nil || bundle.SchemaVersion != 4 || len(bundle.Examples) != 1 || bundle.Examples[0].SchemaVersion != 4 {
				t.Fatal("domain portable contract", err)
			}
			raw, _ = json.Marshal(bundle)
			probes, _ := schema.ProbeValues()
			if strings.Contains(string(raw), tc.old) || strings.Contains(string(raw), probes[0]) || strings.Contains(string(raw), `"value":`) {
				t.Fatal("private/probe/default exported")
			}
			f.query, _ = newPhase18Service(t, f.phase17Fixture)
			f.model.mu.Lock()
			start := len(f.model.requestBodies)
			f.model.mu.Unlock()
			current := f.question("List sales IDs using "+tc.next, nlq.LanguageEnglish)
			f.model.mode.Store(parameterResponse(t, tc.sql, []readexec.Parameter{{Kind: "text", Value: tc.next}}))
			child, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: current})
			if err != nil {
				t.Fatal("domain generation", err)
			}
			sc, _ := store.NewScope(f.e.Tenant(), f.e.User())
			stored, err := f.f.db.ReadQuery(ctx, sc, child.QueryID)
			if err != nil || !reflect.DeepEqual(stored.Parameters, []readexec.Parameter{{Kind: "text", Value: tc.next}}) || stored.ExampleSelection.Usage == nil || len(stored.ExampleSelection.Usage.Used) != 1 || stored.ExampleSelection.Usage.Used[0].ExampleID != active.ID {
				t.Fatal("current-value example use", err)
			}
			result, err := f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil {
				t.Fatal("current typed execution", err)
			}
			requireParameterIDs(t, result, "1", "2")
			f.model.mu.Lock()
			wire := strings.Join(f.model.requestBodies[start:], "\n")
			f.model.mu.Unlock()
			if strings.Contains(wire, tc.old) || strings.Contains(wire, probes[0]) || !strings.Contains(wire, exampleparams.DomainVersion) || !strings.Contains(wire, tc.domain) {
				t.Fatal("domain wire leaked values/lost schema")
			}
			row := bundle.Examples[0]
			row.SQL += " "
			row.Digest = readexec.Hash([]any{"parameterized-example-v2", f.pack.Topic, row.Question, row.SQL, row.ParameterSchema})
			imported, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: row})
			if err != nil || imported.State != "candidate" {
				t.Fatal("domain import", err)
			}
			if _, err := f.query.ExampleState(ctx, f.e, nlqexec.ExampleStateRequest{ExampleID: imported.ID, State: "active", ReviewNote: "Separately reviewed imported native domain"}); err != nil {
				t.Fatal("imported domain review", err)
			}
			// Recomputed digest cannot assert a new domain. Date SQL accepts a timestamp
			// probe natively, so native planning alone is deliberately insufficient.
			bad := row
			bad.ParameterSchema = row.ParameterSchema.Clone()
			bad.ParameterSchema.Slots[0].Domain = "timestamp"
			if tc.domain == "timestamp" {
				bad.ParameterSchema.Slots[0].Domain = "date"
			}
			bad.Digest = readexec.Hash([]any{"parameterized-example-v2", f.pack.Topic, bad.Question, bad.SQL, bad.ParameterSchema})
			if _, err = f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: bad}); err == nil {
				t.Fatal("caller-asserted wrong domain accepted")
			}
			bad = row
			bad.SQL = `SELECT id FROM analytics.sales WHERE name=$1`
			bad.Digest = readexec.Hash([]any{"parameterized-example-v2", f.pack.Topic, bad.Question, bad.SQL, bad.ParameterSchema})
			if _, err = f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: bad}); !errors.Is(err, readexec.ErrBinding) {
				t.Fatal("unproved cross-domain input accepted", err)
			}
			bad = row
			bad.SchemaVersion = 2
			if _, err = f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: bad}); !errors.Is(err, nlqexec.ErrInvalid) {
				t.Fatal("portable downgrade", err)
			}
			for _, mutation := range []string{`parameter_schema=NULL`, `parameter_schema=jsonb_set(parameter_schema,'{slots,0,domain}','"date"')`, `origin=jsonb_set(origin,'{source_binding_digest}','"changed"')`, `parameter_schema=jsonb_set(parameter_schema,'{slots,0,value}','"private"')`} {
				if tc.domain == "date" && strings.Contains(mutation, `'"date"'`) {
					continue
				}
				if _, err = metadata.Exec(ctx, `UPDATE chartworks.nlq_examples SET `+mutation+` WHERE example_id=$1`, active.ID); err == nil {
					t.Fatal("immutable domain/origin changed")
				}
			}
			attempts, calls = count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`), f.model.requests.Load()
			_, err = f.query.Run(ctx, f.e, nlqexec.RunRequest{QueryID: child.QueryID, Operation: child.QueryID + "-run"})
			if err != nil || attempts != count(t, metadata, `SELECT count(*) FROM chartworks.read_attempts`) || calls != f.model.requests.Load() {
				t.Fatal("replay did source/model work", err)
			}
		})
	}
}

func TestSQLRecoveryParameterDomainsOwnedAndExcludedAcceptance(t *testing.T) {
	f := newCW01Fixture(t)
	ctx := context.Background()
	base := `SELECT id FROM analytics.sales WHERE created_at >= $1::timestamptz ORDER BY id`
	q := ownedExampleQuestion(t, f, "List IDs for named sales", nlq.LanguageEnglish, f.answer(t, "customer", cw01Text("primero")))
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "text", Value: "2026-01-01T00:00:00Z"}}))
	p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: q})
	if err != nil {
		t.Fatal(err)
	}
	active := reviewOwnedExample(t, f, p)
	if active.ParameterSchema == nil || len(active.ParameterSchema.Slots) != 1 || active.ParameterSchema.Slots[0].Domain != "timestamptz" {
		t.Fatal("owned slot separation/domain")
	}
	bundle, err := f.query.ExportExamples(ctx, f.e, nlqexec.ExampleExportRequest{Topic: f.pack.Topic, Limit: 8})
	if err != nil || bundle.SchemaVersion != 4 {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(bundle)
	for _, secret := range []string{"2026-01-01", "cw-alpha-731", "primero"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("historical owned/model value exported")
		}
	}
	current := ownedExampleQuestion(t, f, "List IDs for named sales", nlq.LanguageEnglish, f.answer(t, "customer", cw01Text("segundo")))
	f.model.mode.Store(parameterResponse(t, base, []readexec.Parameter{{Kind: "text", Value: "2026-01-02T00:00:00Z"}}))
	child, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: current})
	if err != nil {
		t.Fatal(err)
	}
	record := requireOwnedExampleUse(t, f, child, active.ID)
	if len(record.Parameters) != 2 || record.Parameters[0].Value != "2026-01-02T00:00:00Z" || record.Parameters[1].Value != "cw-beta-731" {
		t.Fatal("historical mixed slots reused")
	}
	row := bundle.Examples[0]
	row.SQL += " "
	row.Digest = readexec.Hash([]any{nlqexec.OwnedExamplePolicy, f.pack.Topic, row.Question, row.SQL, row.ParameterSchema})
	if _, err := f.query.ImportExample(ctx, f.e, nlqexec.ExampleImportRequest{Anchor: current, Example: row}); err != nil {
		t.Fatal("owned domain import", err)
	}
	// These source-valid queries remain valid feedback but are not demonstrations.
	for _, sql := range []string{`SELECT id FROM analytics.sales WHERE $1::date = $1::timestamp`, `SELECT id FROM analytics.sales WHERE created_at::date = $1::date AND created_at::date = '2026-01-02'::date`} {
		f.model.mode.Store(parameterResponse(t, sql, []readexec.Parameter{{Kind: "text", Value: "2026-01-02"}}))
		p, err := f.query.Plan(ctx, f.e, nlqexec.PlanRequest{QuestionRequest: f.question("List records", nlq.LanguageEnglish)})
		if err != nil {
			t.Fatal("native-valid exclusion", err)
		}
		before, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
		if err != nil {
			t.Fatal(err)
		}
		if err = f.query.Feedback(ctx, f.e, nlqexec.FeedbackRequest{QueryID: p.QueryID, Verdict: "positive"}); err != nil {
			t.Fatal("excluded feedback", err)
		}
		after, err := f.query.Examples(ctx, f.e, f.pack.Topic, 8)
		if err != nil || len(after) != len(before) {
			t.Fatal("conflicting/private-value domain learned", err)
		}
	}
	metadata := support.Raw(t, f.f.dsn)
	for _, invalid := range []string{`{"version":"example-parameters-v1","slots":[{"position":1,"kind":"text","domain":"date"}]}`, `{"version":"example-parameters-v2","slots":[{"position":1,"kind":"integer","domain":"date"}]}`, `{"version":"example-parameters-v2","slots":[{"position":1,"kind":"text"}]}`, `{"version":"example-parameters-v2","slots":[{"position":1,"kind":"text","domain":"date","default":"2001-01-01"}]}`} {
		var valid bool
		if err := metadata.QueryRow(ctx, `SELECT chartworks.valid_example_parameters($1::jsonb)`, invalid).Scan(&valid); err != nil || valid {
			t.Fatal("malformed domain schema stored", err)
		}
	}
}
