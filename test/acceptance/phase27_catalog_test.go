package acceptance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/nlqroute"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/reportingapi"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/sources"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/vindex"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
)

// The catalog fixture publishes actual profile-backed columns. SQL assistance
// and native rename tests never replace the validator or execution result.
type phase27CatalogFixture struct {
	*phase17Fixture
	drafts *drafts.Service
	topics *topics.Service
	blocks *reporting.Service
	actor  identity.Envelope
}

func newPhase27CatalogFixture(t *testing.T) *phase27CatalogFixture {
	t.Helper()
	f, draftService, topicService, model, pack := publicationFixture(t)
	source := pack.Datasets[0].Source
	pack.Datasets[0] = phase27ProfileDataset(t, f, sources.Source{ID: source.Source, ContextID: source.Context, Revision: source.SourceRevision}, "phase27-rich-profile", "amount", source.ProfileVersion)
	pack.Dimensions = []semantics.Dimension{{ID: "category", Name: "Category", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: pack.Datasets[0].ID, ID: "name"}, Role: semantics.DimensionCategorical}}
	legacy := &phase17Fixture{f: f, e: f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...), pack: pack, model: model, context: source.Context}
	index, err := vindex.New(f.db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := rulesets.New(f.db, f.db)
	if err != nil {
		t.Fatal(err)
	}
	legacy.service, err = nlqroute.New(topicService, rules, index, model.engine)
	if err != nil {
		t.Fatal(err)
	}
	out := &phase27CatalogFixture{phase17Fixture: legacy, drafts: draftService, topics: topicService}
	out.actor = phase27Actor(t, legacy, f.e.User(), phase27Scopes(f.e.Tenant()))
	out.publish(t, pack)
	out.blocks, err = reporting.New(f.db, topicService, f.s, f.validator, f.executor, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func phase27ProfileDataset(t *testing.T, f *engineeringFixture, source sources.Source, profileID, amountName, previous string) semantics.Dataset {
	t.Helper()
	names := []string{"id", amountName, "created_at", "name", "active"}
	spec := f.profileSpec(t, source, profileID, names, "")
	spec.Previous = previous
	run := f.profile(t, spec)
	p := run.Profile.Profile
	d := semantics.Dataset{ID: p.Dataset, Name: "Sales", Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: p.Dataset, SourceRevision: source.Revision, ProfileVersion: p.Version, ProfileDigest: p.DeterministicHash()}}
	for _, c := range p.Schema {
		if slices.Contains(names, c.Name) {
			id := c.Name
			if id == amountName {
				id = "amount"
			}
			d.Columns = append(d.Columns, semantics.Column{ID: id, SourceName: c.Name, Name: id, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable})
		}
	}
	return d
}

func (f *phase27CatalogFixture) publish(t *testing.T, pack semantics.TopicPack) topics.Published {
	t.Helper()
	ctx := context.Background()
	var draftVersion, publishedVersion int64
	previous, err := f.drafts.Read(ctx, f.e, pack.Topic, 0)
	if err == nil {
		draftVersion = previous.Metadata.Revision
	} else if !errors.Is(err, store.ErrNotFound) {
		t.Fatal("read draft before publication", err)
	}
	current, err := f.topics.Read(ctx, f.e, pack.Topic, "")
	if err == nil {
		publishedVersion = current.State.Revision
	} else if !errors.Is(err, store.ErrNotFound) {
		t.Fatal("read topic before publication", err)
	}
	draft, err := f.drafts.Save(ctx, f.e, drafts.SaveRequest{Expected: draftVersion, Pack: pack, Change: "Phase 27 reviewed catalog transition"})
	if err != nil {
		t.Fatal("save exact topic", err)
	}
	review, err := f.topics.Review(ctx, f.e, pack.Topic, topics.ReviewRequest{DraftRevision: draft.Metadata.Revision, Digest: draft.Metadata.Digest, Decision: "approve", Note: "Review exact catalog transition"})
	if err != nil {
		t.Fatal("review topic", err)
	}
	published, err := f.topics.Publish(ctx, f.e, pack.Topic, topics.PublishRequest{Review: review.ID, Expected: publishedVersion})
	if err != nil {
		t.Fatal("publish topic", err)
	}
	f.pack = pack
	f.context = pack.Datasets[0].Source.Context
	return published
}

func phase27Literal(s string) *reporting.Value { return &reporting.Value{Literal: s} }
func phase27Period(start, end string) *reporting.Value {
	return &reporting.Value{Period: &reporting.Period{Mode: "explicit", Start: start, End: end, DSTPolicy: "reject", MonthPolicy: "clamp"}}
}

func testPhase27Parameters(t *testing.T) {
	f := newPhase27CatalogFixture(t)
	ctx := context.Background()
	e := f.actor
	base := phase27Definition(t, f.phase17Fixture, e, "SELECT id, amount FROM analytics.sales ORDER BY id")
	d := phase27Copy(t, base)
	d.Parameters = []reporting.Parameter{
		{Name: "start", Type: "date", Required: true, Default: phase27Literal("2026-01-01"), Min: "2025-01-01", Max: "2027-01-01"},
		{Name: "end", Type: "datetime", Required: true, Default: phase27Literal("2026-02-01T00:00:00Z")},
		{Name: "window", Type: "relative_period", Required: true, Default: phase27Period("2026-01-01", "2026-02-01")},
		{Name: "category", Type: "dimension_value", Required: true, Default: phase27Literal("one"), Dimension: &reporting.DimensionReference{Topic: f.pack.Topic, Version: f.pack.Version, Dimension: "category"}},
		{Name: "minimum", Type: "number", Required: true, Default: phase27Literal("0"), Min: "0", Max: "9007199254740994"},
		{Name: "first_id", Type: "integer", Required: true, Default: phase27Literal("1"), Min: "1", Max: "2"},
		{Name: "enabled", Type: "boolean", Required: true, Default: phase27Literal("true")},
		{Name: "grain", Type: "grain", Required: true, Default: phase27Literal("day"), Enum: []string{"day", "month"}},
		{Name: "top", Type: "top_n", Required: true, Default: phase27Literal("10"), Max: "20"},
	}
	d.SQL = "SELECT id, amount FROM analytics.sales WHERE created_at >= $1::date AND created_at < $2::timestamptz AND created_at >= $3::timestamptz AND created_at < $4::timestamptz AND name = $5 AND amount >= $6 AND id >= $7 AND active = $8 AND $9::text = 'day' ORDER BY id LIMIT $10"
	created, err := f.blocks.Create(ctx, e, reporting.CreateRequest{ID: "p27-typed", Definition: d})
	if err != nil {
		t.Fatal("typed create", err)
	}
	at := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	before := f.f.lookups.Load()
	resolved, err := phase27Client(t, f).ResolveBlockParameters(ctx, created.State.ID, reporting.ResolveRequest{Reference: reporting.Reference{Draft: true}, Resolution: reporting.Resolution{At: at, Timezone: "UTC"}})
	if err != nil || len(resolved.Resolved.Values) != 9 || len(resolved.Resolved.Parameters) != 10 || f.f.lookups.Load() != before {
		t.Fatal("typed defaults opened a source or lost bind slots", resolved, err)
	}
	for i, value := range resolved.Resolved.Values {
		if value.Type != d.Parameters[i].Type || value.Provenance != "block_default" {
			t.Fatal("typed provenance", value)
		}
	}
	validation, err := f.blocks.Validate(ctx, e, created.State.ID, reporting.ValidateRequest{ExpectedVersion: created.State.Version, Resolution: reporting.Resolution{At: at, Timezone: "UTC"}})
	if err != nil {
		t.Fatal("real typed execution", err)
	}
	preview, err := f.blocks.Preview(ctx, e, created.State.ID, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: validation.State.Version, Arguments: []reporting.Argument{{Name: "category", Value: reporting.Value{Literal: "two"}}, {Name: "enabled", Value: reporting.Value{Literal: "false"}}}}, Outputs: []string{"table-main"}})
	if err != nil || len(preview.Result.Rows) != 1 {
		t.Fatal("typed invocation override", preview, err)
	}
	for _, arg := range []reporting.Argument{{Name: "first_id", Value: reporting.Value{Literal: "0"}}, {Name: "minimum", Value: reporting.Value{Literal: "NaN"}}, {Name: "grain", Value: reporting.Value{Literal: "year"}}, {Name: "unknown", Value: reporting.Value{Literal: "x"}}} {
		if _, err := f.blocks.Resolve(ctx, e, created.State.ID, reporting.ResolveRequest{Reference: reporting.Reference{Draft: true}, Arguments: []reporting.Argument{arg}}); !errors.Is(err, reporting.ErrInvalid) {
			t.Fatal("invalid typed override accepted", arg.Name, err)
		}
	}
	bad := phase27Copy(t, d)
	bad.Parameters[3].Dimension.Dimension = "absent-dimension"
	if _, err := f.blocks.Create(ctx, e, reporting.CreateRequest{ID: "bad-dimension", Definition: bad}); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("invented dimension accepted", err)
	}
	// SQL-looking category content is a value, never a rewritten predicate.
	injected, err := f.blocks.Preview(ctx, e, created.State.ID, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: validation.State.Version, Arguments: []reporting.Argument{{Name: "category", Value: reporting.Value{Literal: "one' OR true --"}}}}, Outputs: []string{"table-main"}})
	if err != nil || len(injected.Result.Rows) != 0 {
		t.Fatal("dimension became executable SQL", injected, err)
	}

	original := phase27Copy(t, base)
	original.SQL = "SELECT id, amount FROM analytics.sales WHERE created_at >= '2026-01-01' AND created_at < '2026-02-01' AND name = 'one' ORDER BY id /* preserve 2026-01-01 */"
	authored, err := f.blocks.Create(ctx, e, reporting.CreateRequest{ID: "p27-period-assist", Definition: original})
	if err != nil {
		t.Fatal(err)
	}
	state, _ := phase27ValidatePublish(t, f.blocks, e, authored)
	request := reporting.ParameterizeRequest{ExpectedVersion: state.Version, DefinitionDigest: authored.Digest, Column: []string{"created_at"}, Parameter: reporting.Parameter{Name: "period", Type: "relative_period", Required: true, Default: phase27Period("2026-01-01", "2026-02-01")}, Note: "Explicitly approve selected period proposal"}
	changed, err := phase27Client(t, f).ParameterizeBlock(ctx, authored.State.ID, request)
	if err != nil {
		t.Fatal("period proposal", err)
	}
	approved, err := f.blocks.SQL(ctx, e, authored.State.ID, reporting.Reference{})
	if err != nil || approved.SQL != original.SQL {
		t.Fatal("assistance rewrote publication", approved, err)
	}
	amended, err := f.blocks.SQL(ctx, e, authored.State.ID, reporting.Reference{Draft: true})
	want := strings.Replace(strings.Replace(original.SQL, "'2026-01-01'", "$1", 1), "'2026-02-01'", "$2", 1)
	if err != nil || amended.SQL != want || !changed.Private || changed.Evidence != nil || !reflect.DeepEqual(changed.Outputs, authored.Outputs) {
		t.Fatal("period assistance altered unrelated content", amended, err)
	}
	if _, err := f.blocks.Parameterize(ctx, e, authored.State.ID, request); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale proposal replay accepted", err)
	}
	if _, err := f.blocks.Validate(ctx, e, authored.State.ID, reporting.ValidateRequest{ExpectedVersion: changed.State.Version}); err != nil {
		t.Fatal("assisted SQL did not execute", err)
	}
}

func (f *phase27CatalogFixture) reloadSource(t *testing.T, amountName string) {
	t.Helper()
	cfg := f.f.cfg.Clone()
	for i := range cfg.Connections {
		for j := range cfg.Connections[i].Relations {
			for k, column := range cfg.Connections[i].Relations[j].Columns {
				if column == "amount" {
					cfg.Connections[i].Relations[j].Columns[k] = amountName
				}
			}
		}
	}
	svc, err := sources.New(f.f.db, cfg, f.f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	validator, err := readexec.NewValidator(svc, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	executor, err := readexec.NewExecutor(svc, f.f.db, f.f.values.Exec)
	if err != nil {
		t.Fatal(err)
	}
	values := f.f.values
	values.Sources = cfg
	profiles, err := engineering.New(f.f.db, svc, validator, executor, nil, values, f.f.lookup)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(profiles.Close)
	f.f.s = svc
	f.f.validator = validator
	f.f.executor = executor
	f.f.service = profiles
	f.f.cfg = cfg
	f.drafts, err = drafts.New(f.f.db, svc, profiles)
	if err != nil {
		t.Fatal(err)
	}
	index, err := vindex.New(f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.topics, err = topics.New(f.f.db, svc, index, f.model.engine)
	if err != nil {
		t.Fatal(err)
	}
	f.blocks, err = reporting.New(f.f.db, f.topics, svc, validator, executor, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
}

func testPhase27DependencyImpact(t *testing.T) {
	f := newPhase27CatalogFixture(t)
	ctx := context.Background()
	e := f.actor
	d := phase27Definition(t, f.phase17Fixture, e, "SELECT id, amount FROM analytics.sales WHERE amount > 0 ORDER BY id /* amount stays in comment */")
	created, err := f.blocks.Create(ctx, e, reporting.CreateRequest{ID: "p27-impact", Definition: d})
	if err != nil {
		t.Fatal(err)
	}
	state, evidence := phase27ValidatePublish(t, f.blocks, e, created)
	att, err := f.blocks.Certify(ctx, e, created.State.ID, reporting.CertifyRequest{ExpectedVersion: state.Version, Revision: 1, Evidence: evidence.ID, Note: "Initial reviewed evidence"})
	if err != nil {
		t.Fatal(err)
	}
	state.Version++
	impact, err := phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: state.Version})
	if err != nil || impact.Classification != "unchanged" {
		t.Fatal("exact catalog", impact, err)
	}
	// Same-name replacement is not continuity: registered names and types stay
	// unchanged, but the new physical column has a different native attnum.
	if _, err := f.f.admin.Exec(ctx, "BEGIN; ALTER TABLE analytics.sales RENAME COLUMN amount TO amount_retained; ALTER TABLE analytics.sales ADD COLUMN amount numeric(30,3); UPDATE analytics.sales SET amount=amount_retained; COMMIT"); err != nil {
		t.Fatal(err)
	}
	replaced, err := phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: impact.Version})
	if err != nil || replaced.Classification != "review_required" || replaced.ProposalDigest != "" || len(replaced.Renames) != 0 {
		t.Fatal("same-name native replacement", replaced, err)
	}
	if _, err := f.f.admin.Exec(ctx, "BEGIN; ALTER TABLE analytics.sales DROP COLUMN amount; ALTER TABLE analytics.sales RENAME COLUMN amount_retained TO amount; COMMIT"); err != nil {
		t.Fatal(err)
	}
	impact, err = phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: replaced.Version})
	if err != nil || impact.Classification != "unchanged" {
		t.Fatal("restored native identity", impact, err)
	}
	// Current unavailability does not destroy history and cannot issue an
	// attestation from an otherwise unexpired old validation record.
	f.f.mu.Lock()
	saved := f.f.sourceFixture.values["CHARTWORKS_SOURCE_READ"]
	f.f.sourceFixture.values["CHARTWORKS_SOURCE_READ"] = "postgres://synthetic:synthetic@127.0.0.1:1/unavailable?sslmode=disable"
	f.f.mu.Unlock()
	unavailable, err := phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: impact.Version})
	f.f.mu.Lock()
	f.f.sourceFixture.values["CHARTWORKS_SOURCE_READ"] = saved
	f.f.mu.Unlock()
	if err != nil || unavailable.Classification != "unavailable" {
		t.Fatal("unavailable source classification", unavailable, err)
	}
	view, err := f.blocks.Read(ctx, e, created.State.ID, reporting.Reference{})
	if err != nil || view.Trust.Certification != "unavailable" || view.Trust.HistoricalAttestation == nil || view.Trust.HistoricalAttestation.ID != att.ID {
		t.Fatal("historical attestation overwritten", view, err)
	}
	if _, err := f.blocks.Certify(ctx, e, created.State.ID, reporting.CertifyRequest{ExpectedVersion: unavailable.Version, Revision: 1, Evidence: evidence.ID, Note: "Must not certify known bad health"}); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("known-unhealthy certification admitted", err)
	}
	recovered, err := phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: unavailable.Version})
	if err != nil || recovered.Classification != "unchanged" {
		t.Fatal(recovered, err)
	}

	cosmetic := phase27Copy(t, f.pack)
	cosmetic.Version = "v2"
	cosmetic.Description = "Updated presentation text only"
	f.publish(t, cosmetic)
	impact, err = phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: recovered.Version})
	if err != nil || impact.Classification != "cosmetic" || impact.ProposalDigest == "" {
		t.Fatal("cosmetic source-backed proposal", impact, err)
	}
	refreshed, err := phase27Client(t, f).ApplyBlockImpact(ctx, created.State.ID, reporting.ApplyImpactRequest{ExpectedVersion: impact.Version, Revision: 1, ProposalDigest: impact.ProposalDigest, Note: "Accept presentation-only semantic revision"})
	if err != nil || !refreshed.Private || refreshed.Evidence != nil {
		t.Fatal("cosmetic amendment", refreshed, err)
	}
	state, _ = phase27ValidatePublish(t, f.blocks, e, refreshed)

	if _, err := f.f.admin.Exec(ctx, "ALTER TABLE analytics.sales RENAME COLUMN amount TO net_amount"); err != nil {
		t.Fatal(err)
	}
	f.reloadSource(t, "net_amount")
	source := f.pack.Datasets[0].Source
	rotated, err := f.f.s.Rotate(ctx, f.f.e, source.Source, source.SourceRevision)
	if err != nil {
		t.Fatal("register renamed source", err)
	}
	renamed := phase27Copy(t, f.pack)
	renamed.Version = "v3"
	renamed.Datasets[0] = phase27ProfileDataset(t, f.f, rotated, "phase27-renamed-profile", "net_amount", source.ProfileVersion)
	f.publish(t, renamed)
	impact, err = phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: state.Version})
	if err != nil || impact.Classification != "rename" || len(impact.Renames) != 1 || impact.Renames[0].From != "amount" || impact.Renames[0].To != "net_amount" {
		t.Fatal("native rename not proved", impact, err)
	}
	wrong := reporting.ApplyImpactRequest{ExpectedVersion: impact.Version, Revision: refreshed.Revision, ProposalDigest: strings.Repeat("0", 64), Note: "Incorrect proposal must fail"}
	if _, err := f.blocks.ApplyImpact(ctx, e, created.State.ID, wrong); !errors.Is(err, reporting.ErrStale) {
		t.Fatal("forged proposal applied", err)
	}
	request := wrong
	request.ProposalDigest = impact.ProposalDigest
	request.Note = "Accept exact native identity rename"
	amendment, err := phase27Client(t, f).ApplyBlockImpact(ctx, created.State.ID, request)
	if err != nil {
		t.Fatal("apply native rename", err)
	}
	frozen, err := f.blocks.SQL(ctx, e, created.State.ID, reporting.Reference{})
	if err != nil || frozen.SQL != d.SQL {
		t.Fatal("source drift rewrote approved SQL", frozen, err)
	}
	pending, err := f.blocks.SQL(ctx, e, created.State.ID, reporting.Reference{Draft: true})
	want := `SELECT id, "net_amount" AS "amount" FROM analytics.sales WHERE "net_amount" > 0 ORDER BY id /* amount stays in comment */`
	if err != nil || pending.SQL != want || amendment.Evidence != nil || !amendment.Private || !reflect.DeepEqual(amendment.Outputs, created.Outputs) {
		t.Fatal("native amendment broke output contract", pending, err)
	}
	state, _ = phase27ValidatePublish(t, f.blocks, e, amendment)

	changed := phase27Copy(t, f.pack)
	changed.Version = "v4"
	changed.Measures[0].Aggregation = semantics.AggregationAverage
	f.publish(t, changed)
	impact, err = phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: state.Version})
	if err != nil || impact.Classification != "review_required" || impact.ProposalDigest != "" {
		t.Fatal("semantic change silently accepted", impact, err)
	}
	// Missing registered columns must not be labelled cosmetic or renamed.
	if _, err := f.f.admin.Exec(ctx, "ALTER TABLE analytics.sales DROP COLUMN net_amount"); err != nil {
		t.Fatal(err)
	}
	impact, err = phase27Client(t, f).RecheckBlockImpact(ctx, created.State.ID, reporting.ImpactRequest{ExpectedVersion: impact.Version})
	if err != nil || impact.Classification != "review_required" {
		t.Fatal("missing source dependency", impact, err)
	}
}

func phase27Client(t *testing.T, f *phase27CatalogFixture) *sdk.Client {
	t.Helper()
	registry, err := reportingapi.Registry(f.blocks.CanValidate(), f.blocks.CanCapture(), f.blocks.CanObserve())
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(assertRegisteredWireSchemas(t, registry, reportingapi.Handler(f.f.token.verifier, f.blocks, http.NotFoundHandler())))
	t.Cleanup(server.Close)
	token := phase27Token(t, f.phase17Fixture, f.actor.User(), "phase27-session", phase27Scopes(f.actor.Tenant()))
	client, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) { return token, nil })
	if err != nil {
		t.Fatal(err)
	}
	return client
}
