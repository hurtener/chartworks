package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/drafts"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// This repository double preserves native tenant/read redaction/private/CAS
// semantics. PostgreSQL acceptance separately exercises the actual SQL boundary.
type authoringBlockRepo struct {
	mu                     sync.Mutex
	heads                  map[string]State
	revisions              map[string]map[int64]Snapshot
	reads, copies, commits int
}

func authoringBlockKey(tenant, id string) string { return tenant + ":" + id }

func (r *authoringBlockRepo) ReadBlock(ctx context.Context, e identity.Envelope, id string, ref Reference, a Access) (Snapshot, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if err := Require(e, id, a); err != nil {
		return Snapshot{}, err
	}
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	key := authoringBlockKey(e.Tenant(), id)
	head, ok := r.heads[key]
	if !ok {
		return Snapshot{}, access.ErrNotFound
	}
	revision := ref.Revision
	if ref.Draft {
		revision = head.DraftRevision
	}
	if revision == 0 {
		revision = head.PublishedRevision
	}
	snapshot, ok := r.revisions[key][revision]
	if !ok {
		return Snapshot{}, access.ErrNotFound
	}
	if err := RequireParent(e, head.Topic, a, false); err != nil {
		return Snapshot{}, err
	}
	if err := RequireReferences(e, a, snapshot.References); err != nil {
		return Snapshot{}, err
	}
	if snapshot.PublishedAt == nil {
		if err := RequirePrivate(e, id, snapshot.Revision.Actor, a); err != nil {
			return Snapshot{}, err
		}
	}
	snapshot = clone(snapshot)
	snapshot.State = head
	if a == Read {
		snapshot.Revision.Definition.SQL = ""
		snapshot.Revision.Provenance = Provenance{}
	}
	return snapshot, nil
}

func (r *authoringBlockRepo) ReadBlockForAuthoringCopy(ctx context.Context, e identity.Envelope, id string, ref Reference) (Snapshot, error) {
	for _, a := range []Access{Read, Preview} {
		if err := Require(e, id, a); err != nil {
			return Snapshot{}, err
		}
	}
	r.mu.Lock()
	r.copies++
	r.mu.Unlock()
	return r.ReadBlock(ctx, e, id, ref, Preview)
}

func (r *authoringBlockRepo) CommitBlock(ctx context.Context, e identity.Envelope, p Prepared) (State, error) {
	m, err := p.Checked(e)
	if err != nil {
		return State{}, err
	}
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := authoringBlockKey(e.Tenant(), m.ID)
	head, exists := r.heads[key]
	if m.Kind == "create" {
		if exists {
			return State{}, store.ErrConflict
		}
		head = State{ID: m.ID, Topic: m.Topic, Version: 1, DraftRevision: 1, DraftState: "draft"}
		r.revisions[key] = map[int64]Snapshot{}
	} else {
		if !exists || head.Version != m.ExpectedVersion || m.Revision == nil || m.Revision.Number != head.DraftRevision+1 || r.revisions[key][m.TargetRevision].Revision.Digest != m.TargetDigest {
			return State{}, store.ErrConflict
		}
		head.Version++
		head.DraftRevision = m.Revision.Number
		head.DraftState = "draft"
	}
	r.heads[key] = head
	r.revisions[key][m.Revision.Number] = Snapshot{State: head, Revision: clone(*m.Revision), References: clone(m.References)}
	r.commits++
	return head, nil
}

func (r *authoringBlockRepo) ListBlocks(context.Context, identity.Envelope, ListRequest) (Page, error) {
	return Page{}, ErrUnavailable
}
func (r *authoringBlockRepo) BlockHistory(context.Context, identity.Envelope, string) (History, error) {
	return History{}, ErrUnavailable
}

type authoringTopicReader struct{ publication topics.Published }

func (r *authoringTopicReader) Read(ctx context.Context, e identity.Envelope, id, version string) (topics.Published, error) {
	if err := ctx.Err(); err != nil {
		return topics.Published{}, err
	}
	if err := topics.Require(e, r.publication.Definition, drafts.Read); err != nil {
		return topics.Published{}, err
	}
	return clone(r.publication), nil
}

func authoringBlockScopes() []string {
	return []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.publish", "reporting.validate", "topics.read", "charts.bind", "cw.tenant.read:tenant", "cw.tenant.write:tenant", "cw.block.read:block", "cw.block.write:block", "cw.block.preview:block", "cw.block.publish:block", "cw.block.read:copy", "cw.block.write:copy", "cw.block.preview:copy", "cw.block.publish:copy", "cw.topic.read:sales", "cw.topic.write:sales", "cw.source.read:warehouse", "cw.execution_context.use:readonly", "cw.dataset.query:sales"}
}

func authoringBlockActor(t *testing.T, tenant, user string, scopes []string) identity.Envelope {
	t.Helper()
	now := time.Now()
	e, err := identity.FromVerified(tenant, user, "session", scopes, now.Add(time.Hour), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func authoringBlockFixture(t *testing.T, published bool) (*Authoring, *authoringBlockRepo, identity.Envelope, Definition) {
	t.Helper()
	d := amountDefinition(t)
	d.ExpectedSchema = append(d.ExpectedSchema, exec.Field{Name: "region", Type: "text", NativeType: "text", Encoding: "string"}, exec.Field{Name: "day", Type: "temporal", NativeType: "date", Encoding: "string"})
	other := clone(d.Outputs[0])
	other.ID = "unchanged"
	other.Intent.DisplayOrder = 1
	d.Outputs = append(d.Outputs, other)
	d.Parameters = []Parameter{{Name: "minimum", Type: "integer", Default: &Value{Literal: "0"}}}
	d.SQL = "SELECT n, looks_like_revenue, region, day FROM analytics.sales WHERE n >= :minimum"
	d.QueryLimits = &QueryLimits{MaxRows: 10, MaxBytes: 4096, TimeoutMillis: 1000, QueryAttempts: 1}
	d.ResultPolicy = []ResultFieldPolicy{{Field: "n", Sensitivity: "non_sensitive"}}
	d.Rules = []RulePin{testRulePin()}
	pin := d.Rules[0]
	d.Templates = []TemplateSelection{{ID: "selected-template", Topic: pin.Topic, TopicVersion: pin.TopicVersion, PackDigest: pin.PackDigest, RuleVersion: pin.RuleVersion, RuleDigest: pin.RuleDigest}}
	if err := validateDefinition(t.Context(), d, config.DefaultReporting(), true); err != nil {
		t.Fatal("invalid fixture", err)
	}
	p := topics.Published{State: topics.State{Topic: "sales", Version: "v1", Active: true}, Digest: d.Topics[0].Digest, Definition: topics.Definition{Topic: "sales", Version: "v1", Datasets: []topics.Dataset{{ID: "sales", Source: topics.Binding{Source: "warehouse", Context: "readonly", Dataset: "sales", SourceRevision: 1}}}}}
	refs := definitionReferences(d, []topics.Published{p})
	head := State{ID: "block", Topic: "sales", Version: 4, DraftRevision: 1, DraftState: "validated"}
	snapshot := Snapshot{State: head, Revision: Revision{Number: 1, ID: "original-revision", Definition: clone(d), Digest: DefinitionDigest(d), ExecutionDigest: ExecutionDigest(d), Actor: "author", Provenance: Provenance{Kind: "capture", Template: nil, Templates: clone(d.Templates), CaptureDigest: strings.Repeat("c", 64)}}, References: refs, Validation: &ValidationRecord{Evidence: Evidence{ID: "old-evidence"}}, Attestation: &Attestation{ID: "old-attestation"}}
	if published {
		now := time.Now()
		snapshot.PublishedAt = &now
		head.PublishedRevision = 1
		snapshot.State = head
	}
	key := authoringBlockKey("tenant", "block")
	repo := &authoringBlockRepo{heads: map[string]State{key: head}, revisions: map[string]map[int64]Snapshot{key: {1: snapshot}}}
	rules := snapshotRuleReader{published: rulesets.Published{State: rulesets.State{Topic: pin.Topic, Version: pin.RuleVersion, Active: true}, Digest: pin.RuleDigest, Definition: semantics.RuleSetDefinition{Topic: pin.Topic, TopicVersion: pin.TopicVersion, PackDigest: pin.PackDigest}}}
	blocks, err := New(repo, &authoringTopicReader{publication: p}, nil, nil, nil, nil, config.DefaultReporting(), rules)
	if err != nil {
		t.Fatal(err)
	}
	return &Authoring{documents: &Documents{blocks: blocks}}, repo, authoringBlockActor(t, "tenant", "author", authoringBlockScopes()), d
}

func authoringBarMapping() AuthoringChartMapping {
	return AuthoringChartMapping{Kind: charts.Bar, Bindings: charts.Bindings{Category: "field_2", Value: "c0"}, Order: []charts.Order{}, Options: charts.DefaultOptions()}
}
func authoringPatch(d Definition) AuthoringBlockMappingRequest {
	return AuthoringBlockMappingRequest{Block: "block", ExpectedVersion: 4, Revision: 1, Digest: DefinitionDigest(d), Output: "table", Mapping: authoringBarMapping()}
}
func authoringCopy(d Definition) AuthoringBlockCopyRequest {
	p := authoringPatch(d)
	return AuthoringBlockCopyRequest{Block: p.Block, ExpectedVersion: p.ExpectedVersion, Revision: p.Revision, Digest: p.Digest, Output: p.Output, NewBlock: "copy", Mapping: p.Mapping}
}

func TestAuthoringBlockReadSQLFreeExactAndDetached(t *testing.T) {
	s, repo, e, d := authoringBlockFixture(t, false)
	out, err := s.ReadBlock(t.Context(), e, AuthoringBlockReadRequest{Block: "block", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	if out.Block.State.Version != 4 || out.Block.Digest != DefinitionDigest(d) || out.Block.Revision != 1 || out.DataValidation != "not_performed" || len(out.OutputColumns) != 2 {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), `"sql"`) || strings.Contains(string(raw), d.SQL) || strings.Contains(string(raw), "selected-template") {
		t.Fatal("hidden SQL/provenance escaped")
	}
	if got := out.OutputColumns[0].Columns; len(got) != 4 || got[2].ID != "field_2" || got[2].Role != "unknown" || got[2].Provenance.Source != "" || !reflect.DeepEqual(got[:2], d.Outputs[0].Mapping.Columns) {
		t.Fatal(got)
	}
	out.Block.Outputs[0].Mapping.Columns[0].Name = "tampered"
	out.OutputColumns[0].Columns[0].Name = "tampered"
	if repo.revisions["tenant:block"][1].Revision.Definition.Outputs[0].Mapping.Columns[0].Name != "n" {
		t.Fatal("projection aliases retained bytes")
	}
	for _, revision := range []int64{0, -1, 257} {
		if _, err := s.ReadBlock(t.Context(), e, AuthoringBlockReadRequest{Block: "block", Revision: revision}); !errors.Is(err, ErrInvalid) {
			t.Fatal(err)
		}
	}
}

func TestAuthoringMappingDenialBeforeRepositoryAccess(t *testing.T) {
	s, repo, _, d := authoringBlockFixture(t, false)
	for _, missing := range []string{"reporting.read", "reporting.write", "charts.bind", "cw.tenant.read:tenant", "cw.block.read:block", "cw.block.write:block"} {
		t.Run(missing, func(t *testing.T) {
			scopes := slices.DeleteFunc(authoringBlockScopes(), func(scope string) bool { return scope == missing })
			before := repo.reads
			if _, err := s.PatchBlockMapping(t.Context(), authoringBlockActor(t, "tenant", "author", scopes), authoringPatch(d)); err == nil {
				t.Fatal("missing authority admitted")
			}
			if repo.reads != before || repo.commits != 0 {
				t.Fatal("denied access reached repository")
			}
		})
	}
	for _, missing := range []string{"reporting.preview", "cw.block.preview:block", "cw.block.write:copy", "cw.tenant.write:tenant"} {
		t.Run("copy/"+missing, func(t *testing.T) {
			before := repo.reads
			scopes := slices.DeleteFunc(authoringBlockScopes(), func(s string) bool { return s == missing })
			if _, err := s.CopyBlockMapping(t.Context(), authoringBlockActor(t, "tenant", "author", scopes), authoringCopy(d)); err == nil {
				t.Fatal("copy widened authority")
			}
			if repo.reads != before {
				t.Fatal("copy denial after source access")
			}
		})
	}
	broad := append(authoringBlockScopes(), "cw.block.read:*")
	if _, err := s.ReadBlock(t.Context(), authoringBlockActor(t, "tenant", "author", broad), AuthoringBlockReadRequest{Block: "block", Revision: 1}); !errors.Is(err, access.ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := s.ReadBlock(t.Context(), identity.Envelope{}, AuthoringBlockReadRequest{Block: "block", Revision: 1}); !errors.Is(err, access.ErrUnauthenticated) {
		t.Fatal(err)
	}
}

func TestAuthoringMappingPrivateTenantAndDependencyEligibility(t *testing.T) {
	for _, tc := range []struct{ name, tenant, user, missing string }{
		{"other actor", "tenant", "other", ""}, {"other tenant", "foreign", "author", ""}, {"private preview", "tenant", "author", "cw.block.preview:block"}, {"topic", "tenant", "author", "cw.topic.read:sales"}, {"source", "tenant", "author", "cw.source.read:warehouse"}, {"context", "tenant", "author", "cw.execution_context.use:readonly"}, {"dataset", "tenant", "author", "cw.dataset.query:sales"}, {"parent write", "tenant", "author", "cw.topic.write:sales"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, _, d := authoringBlockFixture(t, false)
			scopes := slices.DeleteFunc(authoringBlockScopes(), func(scope string) bool { return scope == tc.missing })
			if tc.tenant != "tenant" {
				for i, v := range scopes {
					scopes[i] = strings.ReplaceAll(v, ":tenant", ":"+tc.tenant)
				}
			}
			e := authoringBlockActor(t, tc.tenant, tc.user, scopes)
			if _, err := s.PatchBlockMapping(t.Context(), e, authoringPatch(d)); err == nil {
				t.Fatal("unauthorized amendment")
			}
			if _, err := s.CopyBlockMapping(t.Context(), e, authoringCopy(d)); err == nil {
				t.Fatal("unauthorized copy")
			}
			if r.commits != 0 {
				t.Fatal("denial mutated")
			}
		})
	}
}

func TestAuthoringMappingPreservesFrozenDefinitionAndInvalidatesEvidence(t *testing.T) {
	s, r, e, d := authoringBlockFixture(t, false)
	before := clone(r.revisions["tenant:block"][1])
	out, err := s.PatchBlockMapping(t.Context(), e, authoringPatch(d))
	if err != nil {
		t.Fatal(err)
	}
	if out.Block.Revision != 2 || out.Block.State.Version != 5 || !out.Block.Private || out.Block.Evidence != nil || out.Block.Trust.Certification != "none" || out.DataValidation != "not_performed" {
		t.Fatal(out)
	}
	stored := r.revisions["tenant:block"][2]
	expected := clone(d)
	expected.Outputs[0] = stored.Revision.Definition.Outputs[0]
	if !reflect.DeepEqual(expected, stored.Revision.Definition) || !reflect.DeepEqual(before, r.revisions["tenant:block"][1]) {
		t.Fatal("unselected definition or retained revision changed")
	}
	if stored.Revision.Definition.Outputs[0].Kind != "chart" || len(stored.Revision.Definition.Outputs[0].AmountCompleteness) != 1 || stored.Revision.Definition.Outputs[0].AmountCompleteness[0].Role != "amount" {
		t.Fatal("disclosure not derived from selected fields")
	}
	if !reflect.DeepEqual(stored.Revision.Provenance.Templates, before.Revision.Provenance.Templates) || stored.Revision.Provenance.Kind != "amendment" {
		t.Fatal("capture provenance lost")
	}
	if _, err := s.documents.blocks.Publish(t.Context(), e, "block", PublishRequest{ExpectedVersion: 5, Evidence: "old-evidence"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("schema-only edit published", err)
	}
	if _, err := s.documents.blocks.Validate(t.Context(), e, "block", ValidateRequest{ExpectedVersion: 5}); !errors.Is(err, ErrUnavailable) {
		t.Fatal("no execution dependencies yet validation succeeded", err)
	}
}

func TestAuthoringMappingConflictsAndClosedStructuralValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*AuthoringBlockMappingRequest)
	}{
		{"old version", func(p *AuthoringBlockMappingRequest) { p.ExpectedVersion = 3 }},
		{"wrong digest", func(p *AuthoringBlockMappingRequest) { p.Digest = strings.Repeat("0", 64) }},
		{"unknown output", func(p *AuthoringBlockMappingRequest) { p.Output = "absent" }},
		{"unknown column", func(p *AuthoringBlockMappingRequest) { p.Mapping.Bindings.Value = "client_column" }},
		{"wrong value type", func(p *AuthoringBlockMappingRequest) {
			p.Mapping.Bindings.Value = "field_2"
			p.Mapping.Bindings.Category = "field_3"
		}},
		{"unsupported kind", func(p *AuthoringBlockMappingRequest) { p.Mapping.Kind = "waterfall" }},
		{"invalid chart slots", func(p *AuthoringBlockMappingRequest) { p.Mapping.Bindings.X = "field_2" }},
		{"order unknown", func(p *AuthoringBlockMappingRequest) {
			p.Mapping.Order = []charts.Order{{Column: "counter", Direction: "asc"}}
		}},
		{"invalid options", func(p *AuthoringBlockMappingRequest) { p.Mapping.Options.Legend.Position = "external" }},
		{"display incompatible", func(p *AuthoringBlockMappingRequest) {
			p.Mapping.KPI = &charts.KPIOptions{ValueRow: "first", ComparisonMode: "none"}
		}},
		{"intent collision", func(p *AuthoringBlockMappingRequest) {
			p.Mapping.Intent = &OutputIntent{Enabled: true, DisplayOrder: 1, Metadata: []OutputMetadata{{Locale: "en", DisplayName: "Conflict"}}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, e, d := authoringBlockFixture(t, false)
			p := authoringPatch(d)
			tc.change(&p)
			if _, err := s.PatchBlockMapping(t.Context(), e, p); err == nil {
				t.Fatal("invalid amendment admitted")
			}
			if r.commits != 0 {
				t.Fatal("invalid amendment wrote")
			}
		})
	}
	s, r, e, d := authoringBlockFixture(t, true)
	if _, err := s.PatchBlockMapping(t.Context(), e, authoringPatch(d)); !errors.Is(err, store.ErrConflict) {
		t.Fatal("published baseline amended in place", err)
	}
	if r.commits != 0 {
		t.Fatal("publication changed")
	}
	s, r, e, d = authoringBlockFixture(t, false)
	p := authoringPatch(d)
	if _, err := s.PatchBlockMapping(t.Context(), e, p); err != nil {
		t.Fatal(err)
	}
	p.ExpectedVersion = 5
	if _, err := s.PatchBlockMapping(t.Context(), e, p); !errors.Is(err, store.ErrConflict) {
		t.Fatal("fresh head allowed stale baseline rollback", err)
	}
}

func TestAuthoringMappingCopyPrivatePreservesPublishedSource(t *testing.T) {
	s, r, e, d := authoringBlockFixture(t, true)
	before := clone(r.revisions["tenant:block"][1])
	beforeRaw, err := json.Marshal(r.revisions["tenant:block"][1])
	if err != nil {
		t.Fatal(err)
	}
	scopes := slices.DeleteFunc(e.Scopes(), func(v string) bool { return v == "cw.block.write:block" })
	e = authoringBlockActor(t, "tenant", "author", scopes)
	out, err := s.CopyBlockMapping(t.Context(), e, authoringCopy(d))
	if err != nil {
		t.Fatal(err)
	}
	if out.Block.State.ID != "copy" || out.Block.Revision != 1 || out.Block.State.Version != 1 || out.Block.State.PublishedRevision != 0 || !out.Block.Private || out.Block.Evidence != nil || out.Block.Trust.Certification != "none" || r.copies != 1 {
		t.Fatal(out)
	}
	after := r.revisions["tenant:copy"][1]
	expected := clone(d)
	expected.Outputs[0] = after.Revision.Definition.Outputs[0]
	afterRaw, err := json.Marshal(r.revisions["tenant:block"][1])
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, after.Revision.Definition) {
		t.Fatal("copy altered untargeted definition fields")
	}
	// Compare the exact persisted bytes, not time.Time's process-local monotonic
	// component (which is deliberately absent from the stored JSON contract).
	if string(beforeRaw) != string(afterRaw) || r.heads["tenant:block"].Version != 4 {
		t.Fatal("copy altered immutable source bytes or head")
	}
	if after.Validation != nil || after.Attestation != nil || after.PublishedAt != nil || after.Revision.Provenance.Kind != "copy" || !reflect.DeepEqual(after.Revision.Provenance.Templates, before.Revision.Provenance.Templates) {
		t.Fatal("copied approval or lost custody")
	}
	if _, err := s.CopyBlockMapping(t.Context(), e, authoringCopy(d)); !errors.Is(err, store.ErrConflict) {
		t.Fatal("duplicate target not create-CAS fenced", err)
	}
	if _, err := s.documents.blocks.Publish(t.Context(), e, "copy", PublishRequest{ExpectedVersion: 1, Evidence: "old-evidence"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("copy reused source approval", err)
	}
}

func TestAuthoringMappingConcurrentCASHasOneWinner(t *testing.T) {
	s, r, e, d := authoringBlockFixture(t, false)
	p := authoringPatch(d)
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() { <-start; _, err := s.PatchBlockMapping(context.Background(), e, p); results <- err }()
	}
	close(start)
	wins := 0
	for range 2 {
		if err := <-results; err == nil {
			wins++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 || r.commits != 1 || len(r.revisions["tenant:block"]) != 2 {
		t.Fatal("CAS lost", wins, r.commits)
	}
}

func TestAuthoringReadRedactsNarrativeAndPrivateExecutionProvenance(t *testing.T) {
	s, r, e, _ := authoringBlockFixture(t, false)
	snapshot := r.revisions["tenant:block"][1]
	narrative := contractNarrative()
	narrative.Instructions = "PRIVATE_PROMPT_CANARY"
	narrative.PromptVersion = "private-prompt-version"
	snapshot.Revision.Definition.Outputs = append(snapshot.Revision.Definition.Outputs, Output{ID: "narrative", Kind: "narrative", Narrative: &narrative})
	snapshot.Revision.Provenance.OriginalQuestion = "PRIVATE_CAPTURE_CANARY"
	snapshot.Revision.Provenance.Query = "private-query-id"
	snapshot.Validation.Evidence.Attempt.Manifest.Session = "PRIVATE_SESSION_CANARY"
	snapshot.Validation.Evidence.Attempt.Remote = &exec.RemoteQuery{Driver: "postgres", Tag: "PRIVATE_REMOTE_CANARY"}
	r.revisions["tenant:block"][1] = snapshot
	view, err := s.ReadBlock(t.Context(), e, AuthoringBlockReadRequest{Block: "block", Revision: 1})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(view)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"PRIVATE_PROMPT_CANARY", "private-prompt-version", "PRIVATE_CAPTURE_CANARY", "private-query-id", "PRIVATE_SESSION_CANARY", "PRIVATE_REMOTE_CANARY", `"instructions"`, `"narrative":`, `"attempt":`} {
		if strings.Contains(string(raw), marker) {
			t.Fatal("private authoring payload escaped", marker)
		}
	}
	last := view.Block.Outputs[len(view.Block.Outputs)-1]
	if last.ID != "narrative" || last.Kind != "narrative" || last.Editable || last.Mapping != nil {
		t.Fatal("noneditable output identity disappeared", last)
	}
	if r.revisions["tenant:block"][1].Revision.Definition.Outputs[2].Narrative.Instructions != "PRIVATE_PROMPT_CANARY" {
		t.Fatal("projection changed retained narrative")
	}
}

func TestAuthoringMappingDisplayAndIntentControls(t *testing.T) {
	cases := []struct {
		name    string
		mapping AuthoringChartMapping
		kind    string
	}{
		{"kpi sparkline", AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Category: "field_3", Value: "c0"}, Order: []charts.Order{{Column: "field_3", Direction: "asc"}}, Options: charts.DefaultOptions(), KPI: &charts.KPIOptions{ValueRow: "last", ComparisonMode: "none", Sparkline: true, Thresholds: []charts.KPIThreshold{{Operator: "gte", Value: "1", State: "good", Label: "Target reached"}}}}, "kpi"},
		{"table visible fields", AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"c0", "counter", "field_2"}}, Order: []charts.Order{{Column: "field_2", Direction: "asc"}}, Options: charts.DefaultOptions(), Table: &charts.TableOptions{PageSize: 25, Columns: []charts.TableColumnIntent{{Column: "c0", Visible: true}, {Column: "counter", Visible: true}, {Column: "field_2", Visible: false}}}}, "table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, r, e, d := authoringBlockFixture(t, false)
			p := authoringPatch(d)
			p.Mapping = tc.mapping
			p.Mapping.Intent = &OutputIntent{Enabled: true, DefaultSelected: false, DisplayOrder: 4, Metadata: []OutputMetadata{{Locale: "en-US", DisplayName: "Selected output"}}}
			out, err := s.PatchBlockMapping(t.Context(), e, p)
			if err != nil {
				t.Fatal(err)
			}
			stored := r.revisions["tenant:block"][2].Revision.Definition
			if out.Block.Outputs[0].Kind != tc.kind || out.Block.Outputs[0].Mapping.Version != charts.DisplayVersion || !reflect.DeepEqual(stored.Outputs[0].Intent, p.Mapping.Intent) || !reflect.DeepEqual(stored.Outputs[1], d.Outputs[1]) || !reflect.DeepEqual(stored.AmountCompleteness, d.AmountCompleteness) {
				t.Fatal("display/intent control changed unintended data")
			}
		})
	}
}

func TestAuthoringMappingLegacyAndUnavailableFailClosed(t *testing.T) {
	s, _, e, _ := authoringBlockFixture(t, false)
	legacy := contractDefinition()
	mapping := AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Value: "c0"}, Order: []charts.Order{}, Options: charts.DefaultOptions()}
	updated, err := amendAuthoringMapping(t.Context(), legacy, "table", mapping, s.documents.blocks)
	if err != nil || updated.SchemaVersion != SchemaVersion || updated.Outputs[0].Intent != nil {
		t.Fatal("legacy mapping implicitly migrated", err)
	}
	mapping.Intent = &OutputIntent{Enabled: true, DefaultSelected: true, Metadata: []OutputMetadata{{Locale: "en", DisplayName: "Legacy"}}}
	if _, err := amendAuthoringMapping(t.Context(), legacy, "table", mapping, s.documents.blocks); !errors.Is(err, ErrInvalid) {
		t.Fatal("legacy mutation changed untargeted migration semantics", err)
	}
	empty := &Authoring{}
	if _, err := empty.ReadBlock(t.Context(), e, AuthoringBlockReadRequest{Block: "block", Revision: 1}); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.ReadBlock(ctx, e, AuthoringBlockReadRequest{Block: "block", Revision: 1}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestAuthoringBlockValidationIsExplicitExactAndFailClosed(t *testing.T) {
	s, r, e, d := authoringBlockFixture(t, false)
	request := AuthoringBlockValidateRequest{Block: "block", ExpectedVersion: 4, Revision: 1, Digest: DefinitionDigest(d)}
	for _, missing := range []string{"reporting.read", "reporting.validate", "cw.block.read:block", "cw.block.write:block"} {
		before := r.reads
		scopes := slices.DeleteFunc(authoringBlockScopes(), func(scope string) bool { return scope == missing })
		if _, err := s.ValidateBlock(t.Context(), authoringBlockActor(t, "tenant", "author", scopes), request); err == nil {
			t.Fatal("validation missing authority", missing)
		}
		if r.reads != before {
			t.Fatal("validation scope denial after access")
		}
	}
	if _, err := s.ValidateBlock(t.Context(), e, request); !errors.Is(err, ErrUnavailable) {
		t.Fatal("schema binding created data validation evidence", err)
	}
	for _, change := range []func(*AuthoringBlockValidateRequest){func(r *AuthoringBlockValidateRequest) { r.ExpectedVersion++ }, func(r *AuthoringBlockValidateRequest) { r.Digest = strings.Repeat("f", 64) }} {
		bad := request
		change(&bad)
		if _, err := s.ValidateBlock(t.Context(), e, bad); !errors.Is(err, store.ErrConflict) {
			t.Fatal("stale validation coordinate", err)
		}
	}
	s, _, e, d = authoringBlockFixture(t, true)
	request.Digest = DefinitionDigest(d)
	if _, err := s.ValidateBlock(t.Context(), e, request); !errors.Is(err, store.ErrConflict) {
		t.Fatal("manual validation accepted published source", err)
	}
	bad := request
	bad.Revision = 0
	if _, err := s.ValidateBlock(t.Context(), e, bad); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	_, err := (&Authoring{}).ValidateBlock(t.Context(), e, request)
	if !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}

func TestAuthoringBlockValidationProjectionIsDetached(t *testing.T) {
	in := Evidence{ID: "evidence", Revision: 2, DefinitionDigest: strings.Repeat("a", 64), SchemaDigest: strings.Repeat("b", 64), Schema: []exec.Field{{Name: "amount", Type: "integer", Encoding: "string", NativeType: "int8"}}, Attempt: exec.Attempt{Manifest: exec.Manifest{Session: "PRIVATE_VALIDATION_SESSION"}}}
	out := authoringBlockEvidence(in)
	out.Schema[0].Name = "changed"
	if in.Schema[0].Name != "amount" {
		t.Fatal("evidence projection aliases retained schema")
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "PRIVATE_VALIDATION_SESSION") || strings.Contains(string(raw), `"attempt"`) {
		t.Fatal("execution custody leaked")
	}
}
