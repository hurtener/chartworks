package reporting

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/access"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/semantics"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

type preparationMemory struct {
	*authoringBlockRepo
	mu          sync.Mutex
	records     map[string]AuthoringPreparationRecord
	activeRules bool
}

func (r *preparationMemory) AuthoringRulesActive(context.Context, identity.Envelope, TopicPin) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeRules, nil
}
func (r *preparationMemory) ReserveAuthoringPreparation(_ context.Context, e identity.Envelope, in AuthoringPreparationRecord) (AuthoringPreparationRecord, bool, error) {
	if err := RequireAuthoringPreparation(e, in, true); err != nil {
		return AuthoringPreparationRecord{}, false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, prior := range r.records {
		if prior.Actor == e.User() && prior.Session == e.Session() && prior.Operation == in.Operation {
			if prior.InputDigest != in.InputDigest {
				return AuthoringPreparationRecord{}, false, store.ErrConflict
			}
			return clone(prior), false, nil
		}
	}
	if r.activeRules {
		return AuthoringPreparationRecord{}, false, ErrStale
	}
	r.records[in.ID] = clone(in)
	return clone(in), true, nil
}
func (r *preparationMemory) ReadAuthoringPreparation(_ context.Context, e identity.Envelope, id string) (AuthoringPreparationRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	record, ok := r.records[id]
	if !ok {
		return record, store.ErrNotFound
	}
	if err := RequireAuthoringPreparation(e, record, false); err != nil {
		return AuthoringPreparationRecord{}, err
	}
	return clone(record), nil
}
func (r *preparationMemory) ReadAuthoringPreparationOperation(ctx context.Context, e identity.Envelope, op string) (AuthoringPreparationRecord, error) {
	r.mu.Lock()
	var id string
	for key, r := range r.records {
		if r.Operation == op && r.Actor == e.User() && r.Session == e.Session() {
			id = key
		}
	}
	r.mu.Unlock()
	return r.ReadAuthoringPreparation(ctx, e, id)
}
func (r *preparationMemory) SealAuthoringPreparation(context.Context, identity.Envelope, AuthoringPreparationRecord, exec.Receipt) error {
	return nil
}
func (r *preparationMemory) SettleAuthoringPreparation(ctx context.Context, e identity.Envelope, id string, _ bool) (AuthoringPreparationRecord, error) {
	return r.ReadAuthoringPreparation(ctx, e, id)
}
func (r *preparationMemory) FinishAuthoringPreparation(_ context.Context, _ identity.Envelope, in AuthoringPreparationRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.activeRules {
		return ErrStale
	}
	if r.records[in.ID].Status != "accepted" {
		return store.ErrConflict
	}
	r.records[in.ID] = clone(in)
	return nil
}
func (r *preparationMemory) CommitBlock(ctx context.Context, e identity.Envelope, p Prepared) (State, error) {
	m, err := p.Checked(e)
	if err != nil {
		return State{}, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.Preparation == nil {
		return r.authoringBlockRepo.CommitBlock(ctx, e, p)
	}
	record := r.records[m.Preparation.ID]
	if record.Status != "prepared" || record.Digest != m.Preparation.Digest || record.Target != m.ID || record.Revision == nil || digest(record.Revision) != digest(m.Revision) || r.activeRules {
		return State{}, store.ErrConflict
	}
	state, err := r.authoringBlockRepo.CommitBlock(ctx, e, p)
	if err == nil {
		record.Status = "consumed"
		r.records[record.ID] = record
	}
	return state, err
}

type preparationBoundary struct {
	binding     exec.Binding
	publication topics.Published
	physical    atomic.Int64
	planning    atomic.Int64
	truncate    bool
	failure     bool
	onRead      func()
}

func (b *preparationBoundary) Read(context.Context, identity.Envelope, string, string) (topics.Published, error) {
	return clone(b.publication), nil
}
func (b *preparationBoundary) ContextBinding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	return b.binding.Clone(), nil
}
func (b *preparationBoundary) Binding(context.Context, identity.Envelope, string, string) (exec.Binding, error) {
	return b.binding.Clone(), nil
}
func (b *preparationBoundary) Explain(_ context.Context, e identity.Envelope, c exec.Candidate) error {
	b.planning.Add(1)
	_, _, err := c.SQL(e, b.binding)
	return err
}
func (b *preparationBoundary) Execute(_ context.Context, e identity.Envelope, p exec.Plan, o exec.Options) (exec.ExecutionReport, error) {
	b.physical.Add(1)
	if b.onRead != nil {
		b.onRead()
	}
	if b.failure {
		return exec.ExecutionReport{}, exec.ErrUncertain
	}
	if _, _, err := p.SQL(e, b.binding); err != nil {
		return exec.ExecutionReport{}, err
	}
	now := time.Now()
	status, truncation := "succeeded", ""
	if b.truncate {
		status, truncation = "truncated", "row_limit"
	}
	schema := []exec.Field{{Name: semanticBinding("d", "region"), Type: "text", NativeType: "text", Encoding: "string"}, {Name: semanticBinding("m", "revenue"), Type: "integer", NativeType: "int8", Encoding: "string"}}
	result := exec.Result{Schema: schema, Rows: [][]json.RawMessage{{json.RawMessage(`"West"`), json.RawMessage(`"23"`)}}, Outcome: status, Truncation: truncation, Bytes: 200}
	a := exec.Attempt{ID: strings.Repeat("b", 32), Number: 1, Manifest: exec.Manifest{Operation: o.Operation, Session: e.Session(), Receipt: p.Receipt(), Preview: true}, Status: status, RemoteState: "stopped", Created: now, Deadline: now.Add(time.Second), Finished: &now, Rows: 1, Bytes: 200}
	return exec.ExecutionReport{Attempt: a, Result: &result}, nil
}
func preparationServiceFixture(t *testing.T) (*Authoring, *preparationMemory, *preparationBoundary, identity.Envelope, AuthoringPrepareRequest) {
	t.Helper()
	in, p, _, b := preparationCompileFixture()
	boundary := &preparationBoundary{binding: b, publication: p}
	validator, err := exec.NewValidator(boundary, config.DefaultReadValidation())
	if err != nil {
		t.Fatal(err)
	}
	repo := &preparationMemory{authoringBlockRepo: &authoringBlockRepo{heads: map[string]State{}, revisions: map[string]map[int64]Snapshot{}}, records: map[string]AuthoringPreparationRecord{}}
	blocks, err := New(repo, boundary, boundary, validator, boundary, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	scopes := append(authoringBlockScopes(), "sources.read", "sources.query", "cw.source.query:warehouse")
	e := authoringBlockActor(t, "tenant", "author", scopes)
	inRequest := AuthoringPrepareRequest{NewBlock: "copy", Operation: "prepare:" + strconv.FormatInt(time.Now().Unix(), 10) + ":" + strings.Repeat("1", 32), OperationVersion: AuthoringPreparationOperationVersion, Intent: in, Metadata: []Localized{{Locale: "en", Title: "Revenue by region", Question: "Revenue by region", Aliases: []string{}}}}
	return &Authoring{documents: &Documents{blocks: blocks}}, repo, boundary, e, inRequest
}
func TestAuthoringPreparationActualSchemaAndNoRepeatedRead(t *testing.T) {
	s, _, b, e, in := preparationServiceFixture(t)
	catalog, err := s.Dataset(t.Context(), e, AuthoringDatasetRequest{Topic: in.Intent.Topic, Dataset: in.Intent.Dataset})
	if err != nil || !catalog.Supported || b.physical.Load() != 0 || b.planning.Load() != 0 {
		t.Fatal(catalog, err)
	}
	prepared, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal(prepared, err)
	}
	if prepared.Schema[1].NativeType != "int8" || prepared.Mapping.Columns[1].Type != "integer" {
		t.Fatal("guessed aggregate output type", prepared)
	}
	for range 3 {
		replay, err := s.PrepareDatasetChart(t.Context(), e, in)
		if err != nil || replay.Preparation != prepared.Preparation || replay.Digest != prepared.Digest {
			t.Fatal(replay, err)
		}
	}
	if b.physical.Load() != 1 || b.planning.Load() != 1 {
		t.Fatal("replay repeated source work")
	}
	view, err := s.CreatePreparedChart(t.Context(), e, AuthoringCreatePreparedRequest{NewBlock: "copy", Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	if !view.Block.Private || view.Block.State.DraftState != "draft" || view.Block.Evidence != nil || view.Block.State.PublishedRevision != 0 || b.physical.Load() != 1 {
		t.Fatal("created validation/publication or read", view)
	}
	raw, _ := json.Marshal(prepared)
	if strings.Contains(string(raw), "SELECT") || strings.Contains(string(raw), "West") || strings.Contains(string(raw), `"sql"`) {
		t.Fatal("private custody escaped")
	}
}
func TestAuthoringPreparationRejectsOverflowAndUnknownOutcome(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		s, _, b, e, in := preparationServiceFixture(t)
		b.truncate = !unknown
		b.failure = unknown
		first, err := s.PrepareDatasetChart(t.Context(), e, in)
		if err != nil {
			t.Fatal(err)
		}
		if first.Status == "prepared" || first.Digest != "" {
			t.Fatal(first)
		}
		replay, err := s.PrepareDatasetChart(t.Context(), e, in)
		if err != nil || replay.Status != first.Status || b.physical.Load() != 1 {
			t.Fatal(replay, err)
		}
	}
}
func TestAuthoringPreparationRulesAndActorFences(t *testing.T) {
	s, repo, b, e, in := preparationServiceFixture(t)
	repo.activeRules = true
	out, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil || out.Status != "unsupported" || b.planning.Load() != 0 || b.physical.Load() != 0 {
		t.Fatal(out, err)
	}
	repo.activeRules = false
	prepared, err := s.PrepareDatasetChart(t.Context(), e, in)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal(prepared, err)
	}
	foreign := authoringBlockActor(t, "tenant", "other", append(authoringBlockScopes(), "sources.read", "sources.query", "cw.source.query:warehouse"))
	if _, err := s.Preparation(t.Context(), foreign, AuthoringPreparationRequest{NewBlock: "copy", Preparation: prepared.Preparation}); !errors.Is(err, access.ErrNotFound) {
		t.Fatal("cross actor", err)
	}
	repo.activeRules = true
	if _, err := s.CreatePreparedChart(t.Context(), e, AuthoringCreatePreparedRequest{NewBlock: "copy", Preparation: prepared.Preparation, Digest: prepared.Digest}); !errors.Is(err, ErrStale) {
		t.Fatal("rules activated after preparation", err)
	}
}
func TestAuthoringPreparationRuleActivationDuringRead(t *testing.T) {
	s, repo, b, e, in := preparationServiceFixture(t)
	b.onRead = func() { repo.mu.Lock(); repo.activeRules = true; repo.mu.Unlock() }
	if _, err := s.PrepareDatasetChart(t.Context(), e, in); !errors.Is(err, ErrStale) {
		t.Fatal("changed rule head blessed", err)
	}
	if len(repo.heads) != 0 {
		t.Fatal("failed preparation created block")
	}
}
func TestAuthoringPreparationChangedOperationInputRejected(t *testing.T) {
	s, _, b, e, in := preparationServiceFixture(t)
	if _, err := s.PrepareDatasetChart(t.Context(), e, in); err != nil {
		t.Fatal(err)
	}
	in.Metadata[0].Title = "Different"
	if _, err := s.PrepareDatasetChart(t.Context(), e, in); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	if b.physical.Load() != 1 {
		t.Fatal("repeated source work")
	}
}

func TestAuthoringDatasetExplainsNoSupportedMeasure(t *testing.T) {
	s, _, b, e, in := preparationServiceFixture(t)
	b.publication.Definition.Measures[0].Filters = []semantics.SemanticFilter{{ID: "mandatory-filter"}}
	catalog, err := s.Dataset(t.Context(), e, AuthoringDatasetRequest{Topic: in.Intent.Topic, Dataset: in.Intent.Dataset})
	if err != nil || catalog.Supported || catalog.Reason != "no_supported_measure" || len(catalog.Measures) != 1 || catalog.Measures[0].Supported || catalog.Measures[0].Reason != "measure_filters_unsupported" {
		t.Fatal(catalog, err)
	}
	if b.physical.Load() != 0 || b.planning.Load() != 0 {
		t.Fatal("metadata discovery executed source work")
	}
}
