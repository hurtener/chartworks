package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/engineering"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/internal/topicapi"
	sdk "github.com/hurtener/chartworks/sdk/chartworks"
	"github.com/hurtener/chartworks/test/support"
)

func topicScopes(tenant string) []string {
	return []string{"topics.write", "topics.read", "topics.export", "topics.review", "topics.publish", "sources.read", "engineering.read", "cw.tenant.write:" + tenant, "cw.topic.write:*", "cw.topic.read:*", "cw.topic.export:*", "cw.topic.publish:*", "cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"}
}
func topicFixture(t *testing.T) (*engineeringFixture, *drafts.Service, identity.Envelope, semantics.TopicPack) {
	t.Helper()
	f := newEngineeringFixture(t, nil, nil)
	source := f.create(t, "topic-source")
	profile := f.profile(t, f.profileSpec(t, source, "topic-profile", []string{"id", "amount"}, ""))
	evidence := profile.Profile.Profile
	pack := semantics.TopicPack{SchemaVersion: 1, Topic: "commerce", Version: "v1", Name: "Commerce", Description: "Synthetic draft", Datasets: []semantics.Dataset{{ID: evidence.Dataset, Name: "Sales", Source: semantics.SourceReference{Source: source.ID, Context: source.ContextID, Dataset: evidence.Dataset, SourceRevision: source.Revision, ProfileVersion: evidence.Version, ProfileDigest: evidence.DeterministicHash()}}}}
	for _, c := range evidence.Schema {
		if c.Name == "id" || c.Name == "amount" {
			pack.Datasets[0].Columns = append(pack.Datasets[0].Columns, semantics.Column{ID: c.Name, SourceName: c.Name, Name: c.Name, NativeType: c.NativeType, Category: c.Category, Nullable: c.Nullable})
		}
	}
	pack.Measures = []semantics.Measure{{ID: "revenue", Name: "Revenue", Description: "Total amount", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: evidence.Dataset, ID: "amount"}, Aggregation: semantics.AggregationSum, Unit: "currency"}}
	service, err := drafts.New(f.db, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	return f, service, f.token.envelope(t, f.e.Tenant(), f.e.User(), topicScopes(f.e.Tenant())...), pack
}
func topicClient(t *testing.T, f *engineeringFixture, s *drafts.Service, e identity.Envelope) (*sdk.Client, http.Handler) {
	t.Helper()
	registry, err := topicapi.Registry()
	if err != nil {
		t.Fatal(err)
	}
	h := assertRegisteredWireSchemas(t, registry, topicapi.Handler(f.token.verifier, s, nil, nil, http.NotFoundHandler()))
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	c, err := sdk.New(server.URL, server.Client(), func(context.Context) (string, error) {
		claims := f.token.claims(e.Tenant(), e.User(), e.Scopes())
		claims["session"] = e.Session()
		return f.token.sign(t, claims, nil), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return c, h
}
func cloneTopic(t *testing.T, p semantics.TopicPack) semantics.TopicPack {
	t.Helper()
	m, err := semantics.Compile(p)
	if err != nil {
		t.Fatal(err)
	}
	return m.Pack()
}

// This exercises a partial phase-15 consumer, not the unimplemented full AC set.
func TestTopicDraftAPIAndSDK(t *testing.T) {
	f, s, e, p := topicFixture(t)
	ctx := context.Background()
	c, h := topicClient(t, f, s, e)
	first, err := c.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Pack: p, Change: "Initial review candidate"})
	if err != nil || first.Metadata.Revision != 1 || first.Metadata.Actor != e.User() || first.Metadata.Session != e.Session() {
		t.Fatal("create", first.Metadata, err)
	}
	model, _ := semantics.Compile(p)
	if first.Metadata.Digest != model.Digest() {
		t.Fatal("compiler digest lost")
	}
	current, err := c.TopicDraft(ctx, p.Topic)
	if err != nil || !reflect.DeepEqual(current, first) {
		t.Fatal("retained current", err)
	}
	p.Version = "v2"
	p.Measures[0].Name = "Gross revenue"
	second, err := c.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: 1, Pack: p, Change: "Clarify measure label"})
	if err != nil || second.Metadata.Revision != 2 {
		t.Fatal("CAS edit", err)
	}
	old, err := c.TopicDraftVersion(ctx, p.Topic, 1)
	if err != nil || !reflect.DeepEqual(old, first) {
		t.Fatal("immutable exact revision", err)
	}
	if _, err = c.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: 1, Pack: p, Change: "Stale edit"}); err == nil {
		t.Fatal("stale CAS accepted")
	}
	if _, err = c.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: 2, Pack: p, Change: "Repeated semantic version"}); err == nil {
		t.Fatal("reused semantic version accepted")
	}
	history, err := c.TopicDraftHistory(ctx, p.Topic, 0, 1)
	if err != nil || len(history) != 1 || history[0].Revision != 2 {
		t.Fatal("bounded history", history, err)
	}
	history, err = c.TopicDraftHistory(ctx, p.Topic, 2, 32)
	if err != nil || len(history) != 1 || history[0].Revision != 1 {
		t.Fatal("history cursor", history, err)
	}
	diff, err := c.DiffTopicDraft(ctx, p.Topic, 1, 2)
	if err != nil || len(diff.Changes) != 1 || diff.Changes[0].ID != "revenue" || diff.BeforeDigest != first.Metadata.Digest || diff.AfterDigest != second.Metadata.Digest {
		t.Fatal("exact version diff", diff, err)
	}
	mapping := []semantics.ExportDatasetSlots{{Dataset: p.Datasets[0].ID, Slot: "sales", Columns: []semantics.ExportColumnSlot{{Column: "id", Slot: "key"}, {Column: "amount", Slot: "value"}}}}
	portable, err := c.ExportTopicDraft(ctx, p.Topic, 2, mapping)
	if err != nil {
		t.Fatal("export", err)
	}
	raw, _ := json.Marshal(portable)
	for _, forbidden := range []string{p.Topic, p.Datasets[0].Source.Source, p.Datasets[0].Source.Context, p.Datasets[0].Source.ProfileVersion, p.Datasets[0].Source.ProfileDigest, e.User(), e.Session()} {
		if bytes.Contains(raw, []byte(forbidden)) {
			t.Fatalf("installation coordinate in portable DTO: %q", forbidden)
		}
	}
	binding := semantics.DraftBindings{Topic: "commerce-copy", Version: "copy-v1", Datasets: []semantics.ImportDatasetBinding{{Slot: "sales", Source: p.Datasets[0].Source}}}
	for _, col := range p.Datasets[0].Columns {
		slot := "key"
		if col.ID == "amount" {
			slot = "value"
		}
		binding.Datasets[0].Columns = append(binding.Datasets[0].Columns, semantics.ImportColumnBinding{Slot: slot, ID: col.ID, SourceName: col.SourceName, NativeType: col.NativeType, Category: col.Category, Nullable: col.Nullable})
	}
	imported, err := c.ImportTopicDraft(ctx, sdk.ImportTopicDraftRequest{Portable: portable, Bindings: binding, Change: "Map synthetic portable draft"})
	if err != nil || imported.Metadata.Revision != 1 {
		t.Fatal("import", err)
	}
	again, err := c.ExportTopicDraft(ctx, binding.Topic, 1, mapping)
	if err != nil || !reflect.DeepEqual(again, portable) {
		t.Fatal("neutral service round trip", err)
	}
	// Authentication/closed schema/URL rules exercise the actual registered route.
	token := f.token.sign(t, f.token.claims(e.Tenant(), e.User(), e.Scopes()), nil)
	for _, tc := range []struct {
		method, path, body, token string
		want                      int
	}{{"GET", "/v1/topics/commerce/draft", "", "", 401}, {"POST", "/v1/topic-drafts", `{"tenant":"forged"}`, token, 400}, {"POST", "/v1/topic-drafts", `{"expected_revision":0,"expected_revision":1}`, token, 400}, {"POST", "/v1/topics/commerce/draft-versions/read", `{"revision":0}`, token, 400}, {"GET", "/v1/topics/commerce/draft?revision=1", "", token, 400}, {"GET", "/v1/topics/commerce/draft", "{}", token, 400}} {
		r := callProtected(t, h, tc.method, tc.path, tc.token, tc.body, map[string]string{"Content-Type": "application/json"})
		if r.Code != tc.want {
			t.Fatalf("wire %s: %d %s", tc.path, r.Code, r.Body.String())
		}
	}
	// Metadata reads must remain warehouse/model independent.
	lookups := f.lookups.Load()
	f.mu.Lock()
	f.sourceFixture.values["CHARTWORKS_SOURCE_READ"] = "unavailable"
	f.mu.Unlock()
	if _, err = c.TopicDraft(ctx, p.Topic); err != nil {
		t.Fatal("retained read required warehouse", err)
	}
	if f.lookups.Load() != lookups {
		t.Fatal("retained read resolved source credentials")
	}
	rawdb := support.Raw(t, f.dsn)
	var count int
	if err = rawdb.QueryRow(ctx, `SELECT count(*) FROM chartworks.audit_events WHERE action='topic.drafted'`).Scan(&count); err != nil || count != 3 {
		t.Fatal("atomic draft audit", count, err)
	}
	if _, err = rawdb.Exec(ctx, `UPDATE chartworks.topic_draft_versions SET change_note='rewrite' WHERE topic_id='commerce'`); err == nil {
		t.Fatal("stored immutable version mutated")
	}
	if _, err = rawdb.Exec(ctx, `DELETE FROM chartworks.topic_draft_dependencies WHERE topic_id='commerce'`); err == nil {
		t.Fatal("stored immutable dependencies deleted")
	}
}

func TestTopicDraftOnboardingEntityMutationAndRebind(t *testing.T) {
	f, s, e, seed := topicFixture(t)
	ctx := context.Background()
	c, _ := topicClient(t, f, s, e)

	onboarded, err := c.OnboardTopicProfile(ctx, sdk.OnboardTopicProfileRequest{
		Topic: "onboarded-commerce", Version: "v1", Name: "Onboarded commerce",
		Description: "Unresolved profile scaffold", Profile: seed.Datasets[0].Source.ProfileVersion,
		Change: "Create unresolved scaffold",
	})
	if err != nil || onboarded.Metadata.Revision != 1 || len(onboarded.Pack.Datasets) != 1 || len(onboarded.Pack.Measures) != 0 || len(onboarded.Pack.Dimensions) != 0 || len(onboarded.Pack.KPIs) != 0 || len(onboarded.Pack.Joins) != 0 {
		t.Fatal("deterministic unresolved onboarding", onboarded, err)
	}
	dataset := onboarded.Pack.Datasets[0]
	measure := semantics.Measure{ID: "revenue", Name: "Revenue", Description: "Reviewed amount", Field: semantics.Reference{Kind: semantics.KindColumn, Dataset: dataset.ID, ID: "amount"}, Aggregation: semantics.AggregationSum, Unit: "currency"}
	kpi := semantics.KPI{ID: "revenue_index", Name: "Revenue index", Description: "Reviewed revenue signal", Expression: "reviewed revenue", Inputs: []semantics.Reference{{Kind: semantics.KindMeasure, ID: measure.ID}}}
	mutated, err := c.MutateTopicEntities(ctx, onboarded.Pack.Topic, sdk.MutateTopicEntitiesRequest{
		Expected: 1, Version: "v2", Change: "Add reviewed entities",
		Mutations: []sdk.TopicEntityMutation{{Operation: "put", Kind: semantics.KindMeasure, ID: measure.ID, Measure: &measure}, {Operation: "put", Kind: semantics.KindKPI, ID: kpi.ID, KPI: &kpi}},
	})
	if err != nil || mutated.Metadata.Revision != 2 || len(mutated.Pack.Measures) != 1 || len(mutated.Pack.KPIs) != 1 {
		t.Fatal("atomic entity mutation", mutated, err)
	}
	retained, err := c.TopicDraftVersion(ctx, onboarded.Pack.Topic, 1)
	if err != nil || len(retained.Pack.Measures) != 0 || retained.Metadata.Digest != onboarded.Metadata.Digest {
		t.Fatal("entity mutation changed prior revision", retained, err)
	}
	if _, err = c.MutateTopicEntities(ctx, onboarded.Pack.Topic, sdk.MutateTopicEntitiesRequest{Expected: 2, Version: "v3-invalid", Change: "Strand KPI", Mutations: []sdk.TopicEntityMutation{{Operation: "delete", Kind: semantics.KindMeasure, ID: measure.ID}}}); err == nil {
		t.Fatal("stranded entity reference accepted")
	}
	current, err := c.TopicDraft(ctx, onboarded.Pack.Topic)
	if err != nil || current.Metadata.Revision != 2 {
		t.Fatal("failed mutation moved draft head", current.Metadata, err)
	}
	model, err := semantics.Compile(current.Pack)
	if err != nil {
		t.Fatal("compile enhanced rebind candidate", err)
	}
	enhanced, err := semantics.ApplyEnhancements(model, "v3", []semantics.Enhancement{{Dataset: dataset.ID, Column: "id", Kind: semantics.EnhancementUnresolved, Reason: "Identifier meaning requires review"}})
	if err != nil {
		t.Fatal("create unresolved enhancement", err)
	}
	enhancedDraft, err := c.SaveTopicDraft(ctx, sdk.SaveTopicDraftRequest{Expected: 2, Pack: enhanced.Pack(), Change: "Preserve unresolved authoring gap"})
	if err != nil || enhancedDraft.Metadata.Revision != 3 || len(enhancedDraft.Pack.Unresolved) != 1 {
		t.Fatal("save unresolved enhancement", enhancedDraft, err)
	}
	unresolved := enhancedDraft.Pack.Unresolved[0]

	targetSource := f.create(t, "rebind-source")
	target := f.profile(t, f.profileSpec(t, targetSource, "rebind-profile", []string{"id", "amount"}, "")).Profile.Profile
	mappings := make([]sdk.TopicColumnRebinding, 0, len(dataset.Columns))
	for _, column := range dataset.Columns {
		mappings = append(mappings, sdk.TopicColumnRebinding{Column: column.ID, SourceName: column.SourceName})
	}
	if _, err = c.RebindTopicDataset(ctx, onboarded.Pack.Topic, sdk.RebindTopicDatasetRequest{Expected: 3, Version: "v4-incomplete", Dataset: dataset.ID, Profile: target.Version, Columns: mappings[:len(mappings)-1], Change: "Incomplete move"}); err == nil {
		t.Fatal("incomplete stable column mapping accepted")
	}
	rebind := sdk.RebindTopicDatasetRequest{
		Expected: 3, Version: "v4", Dataset: dataset.ID, Profile: target.Version, Change: "Move to reviewed source profile",
		Columns: mappings,
	}
	rebound, err := c.RebindTopicDataset(ctx, onboarded.Pack.Topic, rebind)
	if err != nil || rebound.Metadata.Revision != 4 || len(rebound.Pack.Datasets) != 1 {
		_, directErr := s.RebindDataset(ctx, e, onboarded.Pack.Topic, rebind)
		t.Fatal("reviewed dataset rebind", rebound, err, directErr)
	}
	moved := rebound.Pack.Datasets[0]
	if moved.ID != target.Dataset || moved.Source.Source != target.Source || moved.Source.Context != target.Context || moved.Source.ProfileVersion != target.Version || moved.Source.SourceRevision != target.SourceRevision || moved.Source.ProfileDigest != target.DeterministicHash() {
		t.Fatal("rebind did not derive target profile evidence", moved.Source)
	}
	if rebound.Pack.Measures[0].Field.Dataset != target.Dataset || rebound.Pack.Measures[0].Field.ID != "amount" {
		t.Fatal("stable semantic reference was not rewritten", rebound.Pack.Measures[0].Field)
	}
	if len(rebound.Pack.Unresolved) != 1 || rebound.Pack.Unresolved[0].ID != unresolved.ID || rebound.Pack.Unresolved[0].Dataset != target.Dataset || rebound.Pack.Unresolved[0].Column != unresolved.Column || rebound.Pack.Unresolved[0].Reason != unresolved.Reason {
		t.Fatal("unresolved semantic reference was not rewritten", rebound.Pack.Unresolved)
	}
	if !reflect.DeepEqual(moved.Columns, dataset.Columns) {
		t.Fatal("stable semantic columns changed during source move", moved.Columns)
	}
}

func TestTopicDraftCASAndScopeFences(t *testing.T) {
	f, s, e, p := topicFixture(t)
	ctx := context.Background()
	if _, err := s.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "Start"}); err != nil {
		t.Fatal(err)
	}
	candidates := []semantics.TopicPack{cloneTopic(t, p), cloneTopic(t, p)}
	candidates[0].Version = "v2-a"
	candidates[1].Version = "v2-b"
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, candidate := range candidates {
		wg.Add(1)
		go func(p semantics.TopicPack) {
			defer wg.Done()
			_, err := s.Save(ctx, e, drafts.SaveRequest{Expected: 1, Pack: p, Change: "Concurrent edit"})
			results <- err
		}(candidate)
	}
	wg.Wait()
	close(results)
	wins, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			wins++
		case errors.Is(err, store.ErrConflict):
			conflicts++
		default:
			t.Fatal("unexpected CAS error", err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal("CAS winners", wins, conflicts)
	}
	for _, tc := range []struct{ name, tenant, user, session, remove, replace string }{{"foreign tenant", "source-b", e.User(), e.Session(), "", ""}, {"different actor", e.Tenant(), "other", e.Session(), "", ""}, {"different session", e.Tenant(), e.User(), "other-session", "", ""}, {"wrong context", e.Tenant(), e.User(), e.Session(), "cw.execution_context.use:*", "cw.execution_context.use:wrong"}, {"wrong dataset", e.Tenant(), e.User(), e.Session(), "cw.dataset.query:*", "cw.dataset.query:wrong"}, {"wrong source", e.Tenant(), e.User(), e.Session(), "cw.source.read:*", "cw.source.read:wrong"}, {"wrong topic", e.Tenant(), e.User(), e.Session(), "cw.topic.read:*", "cw.topic.read:wrong"}, {"missing action", e.Tenant(), e.User(), e.Session(), "topics.read", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			scopes := topicScopes(tc.tenant)
			var selected []string
			for _, scope := range scopes {
				if scope != tc.remove {
					selected = append(selected, scope)
				}
			}
			if tc.replace != "" {
				selected = append(selected, tc.replace)
			}
			claims := f.token.claims(tc.tenant, tc.user, selected)
			claims["session"] = tc.session
			bearer := f.token.sign(t, claims, nil)
			other, err := f.token.verifier.Verify(ctx, bearer, auth.HTTP)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Read(ctx, other, p.Topic, 1); err == nil {
				t.Fatal("inaccessible payload returned")
			}
			history, err := s.History(ctx, other, p.Topic, 0, 32)
			if err == nil && len(history) != 0 {
				t.Fatal("inaccessible history returned")
			}
			c, _ := topicClient(t, f, s, other)
			if _, err = c.TopicDraft(ctx, p.Topic); err == nil {
				t.Fatal("HTTP inaccessible payload returned")
			}
		})
	}
	// Zero admission evidence is not an alternate raw store write path.
	if _, err := f.db.SaveTopicDraft(ctx, e, drafts.Prepared{}, 2, "Forged"); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("zero proof", err)
	}
	current, err := s.Read(ctx, e, p.Topic, 0)
	if err != nil {
		t.Fatal(err)
	}
	// Every attempt to author false source/profile/column evidence fails.
	for _, tc := range []struct {
		name string
		edit func(*semantics.TopicPack)
	}{
		{"profile digest", func(p *semantics.TopicPack) { p.Datasets[0].Source.ProfileDigest = strings.Repeat("a", 64) }},
		{"profile missing", func(p *semantics.TopicPack) { p.Datasets[0].Source.ProfileVersion = "missing" }},
		{"source revision", func(p *semantics.TopicPack) { p.Datasets[0].Source.SourceRevision++ }},
		{"source context", func(p *semantics.TopicPack) { p.Datasets[0].Source.Context = "topic-source:v99" }},
		{"source column", func(p *semantics.TopicPack) { p.Datasets[0].Columns[0].SourceName = "missing" }},
		{"column type", func(p *semantics.TopicPack) { p.Datasets[0].Columns[0].NativeType = "fake" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cloneTopic(t, current.Pack)
			candidate.Version = "v3"
			tc.edit(&candidate)
			if _, err := s.Save(ctx, e, drafts.SaveRequest{Expected: 2, Pack: candidate, Change: "Invalid evidence"}); err == nil {
				t.Fatal("unverified draft stored")
			}
		})
	}
	// Rotating a source invalidates new admission against its old revision; it
	// does not rewrite or pretend to health-check an already retained snapshot.
	if _, err = f.s.Rotate(ctx, f.e, p.Datasets[0].Source.Source, 1); err != nil {
		t.Fatal(err)
	}
	current.Pack.Version = "v3"
	if _, err = s.Save(ctx, e, drafts.SaveRequest{Expected: 2, Pack: current.Pack, Change: "Old source"}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("rotated source admitted", err)
	}
	if _, err = s.Read(ctx, e, p.Topic, 1); err != nil {
		t.Fatal("immutable retained read", err)
	}
}

// Intercept the actual repository commit, after real admission I/O, to prove
// that an observation cannot survive a changed source/profile fence.
type topicCommitBoundary struct {
	drafts.Repository
	before func()
}

func (r *topicCommitBoundary) SaveTopicDraft(ctx context.Context, e identity.Envelope, p drafts.Prepared, expected int64, change string) (drafts.Version, error) {
	r.before()
	return r.Repository.SaveTopicDraft(ctx, e, p, expected, change)
}
func TestTopicDraftCommitFencesAndErasure(t *testing.T) {
	f, s, e, p := topicFixture(t)
	ctx := context.Background()
	if _, err := s.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "Initial"}); err != nil {
		t.Fatal(err)
	}
	boundary := &topicCommitBoundary{Repository: f.db}
	checked, err := drafts.New(boundary, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	p.Version = "v2"
	boundary.before = func() {
		if _, err := f.s.Rotate(ctx, f.e, p.Datasets[0].Source.Source, 1); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = checked.Save(ctx, e, drafts.SaveRequest{Expected: 1, Pack: p, Change: "Rotation race"}); !errors.Is(err, readexec.ErrBinding) {
		t.Fatal("commit did not fence source rotation", err)
	}
	current, err := s.Read(ctx, e, p.Topic, 0)
	if err != nil || current.Metadata.Revision != 1 {
		t.Fatal("failed commit moved head", err)
	}
	// Logical profile erasure gates retained draft access without rewriting an
	// immutable semantic snapshot or consulting a live warehouse.
	raw := support.Raw(t, f.dsn)
	if _, err = raw.Exec(ctx, `UPDATE chartworks.profile_versions SET state='erased',result=NULL,deterministic_hash=NULL WHERE profile_id=$1`, p.Datasets[0].Source.ProfileVersion); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Read(ctx, e, p.Topic, 1); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("erased evidence still readable", err)
	}
	history, err := s.History(ctx, e, p.Topic, 0, 32)
	if err != nil || len(history) != 0 {
		t.Fatal("erased history still reachable", err)
	}
}
func TestTopicDraftProfileHeadAndAuditFences(t *testing.T) {
	f, s, e, p := topicFixture(t)
	ctx := context.Background()
	boundary := &topicCommitBoundary{Repository: f.db}
	checked, err := drafts.New(boundary, f.s, f.service)
	if err != nil {
		t.Fatal(err)
	}
	source, err := f.s.Get(ctx, f.e, p.Datasets[0].Source.Source)
	if err != nil {
		t.Fatal(err)
	}
	var newer semantics.SourceReference
	boundary.before = func() {
		spec := f.profileSpec(t, source, "new-profile", []string{"id", "amount"}, "")
		spec.Previous = p.Datasets[0].Source.ProfileVersion
		profile := f.profile(t, spec)
		newer = p.Datasets[0].Source
		newer.ProfileVersion = profile.Profile.Profile.Version
		newer.ProfileDigest = profile.Profile.Profile.DeterministicHash()
	}
	if _, err = checked.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "Profile replacement race"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("inactive profile crossed commit", err)
	}
	if _, err = s.Read(ctx, e, p.Topic, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("failed creation leaked head", err)
	}
	p.Datasets[0].Source = newer
	raw := support.Raw(t, f.dsn)
	if _, err = raw.Exec(ctx, `CREATE FUNCTION chartworks.test_reject_topic_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.action='topic.drafted' THEN RAISE EXCEPTION 'synthetic audit failure' USING ERRCODE='55000'; END IF; RETURN NEW; END $$; CREATE TRIGGER test_topic_audit BEFORE INSERT ON chartworks.audit_events FOR EACH ROW EXECUTE FUNCTION chartworks.test_reject_topic_audit()`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "Audit failure"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("audit failure classification", err)
	}
	var count int
	if err = raw.QueryRow(ctx, `SELECT count(*) FROM chartworks.topic_draft_versions`).Scan(&count); err != nil || count != 0 {
		t.Fatal("audit failure committed draft", count, err)
	}
	if _, err = raw.Exec(ctx, `DROP TRIGGER test_topic_audit ON chartworks.audit_events`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "After audit recovery"}); err != nil {
		t.Fatal(err)
	}
}

func TestTopicDraftAdmissionLimitsAndIndependentActions(t *testing.T) {
	f, s, e, p := topicFixture(t)
	ctx := context.Background()
	if _, err := drafts.New(nil, f.s, f.service); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("missing dependency", err)
	}
	for _, in := range []drafts.SaveRequest{{Expected: -1, Pack: p, Change: "x"}, {Expected: 128, Pack: p, Change: "x"}, {Pack: p}, {Pack: p, Change: strings.Repeat("x", 1025)}, {Pack: p, Change: string([]byte{255})}, {Pack: semantics.TopicPack{}, Change: "x"}} {
		if _, err := s.Save(ctx, e, in); err == nil {
			t.Fatal("invalid admission accepted")
		}
	}
	//nolint:staticcheck // The public service must reject a nil context before any dependency work.
	if _, err := s.Save(nil, e, drafts.SaveRequest{Pack: p, Change: "x"}); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("nil context", err)
	}
	if _, err := s.Save(ctx, identity.Envelope{}, drafts.SaveRequest{Pack: p, Change: "x"}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal("missing authority", err)
	}
	if _, err := s.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "Start"}); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{"topics.write", "topics.export", "cw.topic.write:*", "cw.topic.export:*", "cw.tenant.write:source-a", "sources.read", "engineering.read"} {
		scopes := topicScopes(e.Tenant())
		var filtered []string
		for _, scope := range scopes {
			if scope != removed {
				filtered = append(filtered, scope)
			}
		}
		other := f.token.envelope(t, e.Tenant(), e.User(), filtered...)
		if strings.Contains(removed, "export") {
			if _, err := s.Export(ctx, other, p.Topic, 1, nil); err == nil || errors.Is(err, semantics.ErrInvalid) {
				t.Fatal("export authority not checked before mapping", removed, err)
			}
		} else {
			candidate := cloneTopic(t, p)
			candidate.Topic = "another"
			if _, err := s.Save(ctx, other, drafts.SaveRequest{Pack: candidate, Change: "Unauthorized creation"}); err == nil {
				t.Fatal("creation authority", removed)
			}
		}
	}
	for _, rev := range []int64{-1, 129} {
		if _, err := s.Read(ctx, e, p.Topic, rev); !errors.Is(err, store.ErrInvalid) {
			t.Fatal("revision limit", err)
		}
	}
	if _, err := s.Read(ctx, e, "bad/path", 1); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("identifier", err)
	}
	if _, err := f.db.ReadTopicDraft(ctx, e, p.Topic, 1, drafts.Access(99)); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("open operation access", err)
	}
	if _, err := s.History(ctx, e, p.Topic, 0, 33); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("history limit", err)
	}
	if _, err := s.Diff(ctx, e, p.Topic, 0, 1); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("diff exact revision", err)
	}
	if _, err := s.Diff(ctx, e, p.Topic, 1, 99); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("diff missing revision", err)
	}
	if _, err := s.Export(ctx, e, p.Topic, 0, nil); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("export exact revision", err)
	}
	if _, err := s.Export(ctx, e, p.Topic, 1, nil); !errors.Is(err, semantics.ErrInvalid) {
		t.Fatal("incomplete mapping", err)
	}
	if _, err := s.Import(ctx, e, drafts.ImportRequest{}); !errors.Is(err, semantics.ErrInvalid) {
		t.Fatal("invalid import", err)
	}
}

func TestTopicDraftMultipleDatasetScopeAndAdmissionBounds(t *testing.T) {
	f, s, e, p := topicFixture(t)
	ctx := context.Background()
	appendDataset := func(sourceID, relationName, profileID string) {
		source, err := f.s.Get(ctx, f.e, sourceID)
		if err != nil {
			t.Fatal(err)
		}
		schema, err := f.s.Discover(ctx, f.e, sourceID)
		if err != nil {
			t.Fatal(err)
		}
		for _, relation := range schema.Relations {
			if relation.Name == relationName {
				run := f.profile(t, engineering.ProfileSpec{ID: profileID, Source: source.ID, Context: source.ContextID, Dataset: relation.ID, Columns: []string{}, SkipLLM: true})
				profile := run.Profile.Profile
				dataset := semantics.Dataset{ID: profile.Dataset, Name: relation.Name, Source: semantics.SourceReference{Source: profile.Source, Context: profile.Context, Dataset: profile.Dataset, SourceRevision: profile.SourceRevision, ProfileVersion: profile.Version, ProfileDigest: profile.DeterministicHash()}}
				for _, column := range profile.Schema {
					if column.Safe {
						dataset.Columns = append(dataset.Columns, semantics.Column{ID: column.Name, SourceName: column.Name, Name: column.Name, NativeType: column.NativeType, Category: column.Category, Nullable: column.Nullable})
					}
				}
				p.Datasets = append(p.Datasets, dataset)
				return
			}
		}
		t.Fatal("missing fixture relation")
	}
	appendDataset("topic-source", "items", "items-profile")
	other := f.create(t, "another-source")
	appendDataset(other.ID, "items", "other-items-profile")
	if _, err := s.Save(ctx, e, drafts.SaveRequest{Pack: p, Change: "Three verified datasets"}); err != nil {
		t.Fatal("multi-dataset admission", err)
	}
	// A proposed edit cannot drop an inaccessible prior dependency to evade the
	// persisted draft's complete pre-query scope restriction.
	narrowed := topicScopes(e.Tenant())
	for i, scope := range narrowed {
		if scope == "cw.dataset.query:*" {
			narrowed[i] = "cw.dataset.query:" + p.Datasets[0].ID
		}
	}
	restricted := f.token.envelope(t, e.Tenant(), e.User(), narrowed...)
	candidate := cloneTopic(t, p)
	candidate.Version = "v2"
	candidate.Datasets = candidate.Datasets[:0]
	candidate.Datasets = append(candidate.Datasets, p.Datasets[0])
	lookups := f.lookups.Load()
	if _, err := s.Save(ctx, restricted, drafts.SaveRequest{Expected: 1, Pack: candidate, Change: "Drop inaccessible dependency"}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("prior dependencies bypassed", err)
	}
	if f.lookups.Load() != lookups {
		t.Fatal("denied edit called source")
	}
	for _, removed := range []string{"cw.source.read:*", "cw.dataset.query:*", "cw.execution_context.use:*"} {
		var scopes []string
		for _, scope := range topicScopes(e.Tenant()) {
			if scope != removed {
				scopes = append(scopes, scope)
			}
		}
		missing := f.token.envelope(t, e.Tenant(), e.User(), scopes...)
		if _, err := s.Read(ctx, missing, p.Topic, 1); !errors.Is(err, access.ErrNotFound) {
			t.Fatal("empty dependency selection", removed, err)
		}
	}
	//nolint:staticcheck // The negative verifies that retained reads reject a nil context.
	if _, err := s.Read(nil, e, p.Topic, 1); err == nil {
		t.Fatal("nil read context")
	}
	//nolint:staticcheck // The negative verifies that history reads reject a nil context.
	if _, err := s.History(nil, e, p.Topic, 0, 1); err == nil {
		t.Fatal("nil history context")
	}
	if _, err := f.db.SaveTopicDraft(ctx, e, drafts.Prepared{}, -1, "invalid"); !errors.Is(err, store.ErrInvalid) {
		t.Fatal("raw store admission bounds", err)
	}
}

func TestTopicProfilePlanningDoesNotPersist(t *testing.T) {
	f, service, actor, pack := topicFixture(t)
	ctx := context.Background()
	request := drafts.OnboardRequest{Topic: "planned-topic", Version: "v1", Name: "Planned topic", Description: "Synthetic reviewed material", Profile: pack.Datasets[0].Source.ProfileVersion, Change: "Plan before save"}
	planned, err := service.PlanProfile(ctx, actor, request)
	if err != nil || planned.Topic != request.Topic || len(planned.Datasets) != 1 || planned.Datasets[0].Source != pack.Datasets[0].Source {
		t.Fatal("profile evidence was not preserved", planned, err)
	}
	if _, err = service.Read(ctx, actor, request.Topic, 0); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("planning persisted a draft", err)
	}
	narrow := f.token.envelope(t, actor.Tenant(), actor.User(), "topics.write", "cw.tenant.write:"+actor.Tenant(), "cw.topic.write:"+request.Topic)
	if _, err = service.PlanProfile(ctx, narrow, request); err == nil {
		t.Fatal("profile planning widened source authority")
	}
	saved, err := service.Save(ctx, actor, drafts.SaveRequest{Pack: planned, Change: request.Change})
	if err != nil || saved.Metadata.Revision != 1 {
		t.Fatal("planned material could not use normal save", saved, err)
	}
}

func TestAutopilotTopicDraftAdapter(t *testing.T) {
	f, service, actor, pack := topicFixture(t)
	ctx := context.Background()
	goal := engineering.AutopilotTopicGoal{Topic: "engineering-topic", Profile: pack.Datasets[0].Source.ProfileVersion, Version: "v1", Name: "Reviewed topic", Description: "Synthetic profile-backed material"}
	planned, err := service.PlanAutopilotTopic(ctx, actor, goal)
	if err != nil {
		t.Fatal(err)
	}
	planned.Name = "Edited proposed meaning"
	if err = service.CheckAutopilotTopic(ctx, actor, goal, planned); err != nil {
		t.Fatal(err)
	}
	invalid := cloneTopic(t, planned)
	invalid.Datasets[0].Source.Context = "unrelated-context"
	if err = service.CheckAutopilotTopic(ctx, actor, goal, invalid); err == nil {
		t.Fatal("changed profile context accepted")
	}
	p := engineering.AutopilotProposal{ID: "topic-proposal", Revision: 1, Digest: strings.Repeat("a", 64), Material: engineering.ProposalMaterial{Request: engineering.AutopilotGoal{Topic: &goal}, Topic: &planned}}
	saved, err := service.ApplyAutopilotTopic(ctx, actor, p)
	if err != nil || saved.Revision != 1 {
		t.Fatal("ordinary topic save failed", saved, err)
	}
	replay, err := service.ApplyAutopilotTopic(ctx, actor, p)
	if err != nil || replay != saved {
		t.Fatal("lost-reply reconciliation failed", replay, err)
	}
	other := p
	other.ID = "different-proposal"
	if _, err = service.ApplyAutopilotTopic(ctx, actor, other); !errors.Is(err, store.ErrConflict) {
		t.Fatal("foreign proposal adopted private effect", err)
	}
	goal.ExpectedRevision = 1
	goal.Version = "v2"
	next, err := service.PlanAutopilotTopic(ctx, actor, goal)
	if err != nil || next.Version != "v2" || next.Datasets[0].Source != planned.Datasets[0].Source {
		t.Fatal("existing draft proposal lost provenance", next, err)
	}
	limited := f.token.envelope(t, actor.Tenant(), actor.User(), "topics.write", "cw.topic.write:"+goal.Topic)
	if _, err = service.PlanAutopilotTopic(ctx, limited, goal); err == nil {
		t.Fatal("topic plan widened authority")
	}
}
