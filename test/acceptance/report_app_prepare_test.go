package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	readexec "github.com/hurtener/chartworks/internal/exec"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/semantics/rulesets"
	"github.com/hurtener/chartworks/internal/semantics/topics"
	"github.com/hurtener/chartworks/internal/store"
)

// Real source and PostgreSQL custody. TestReportPrivateBlockBridge owns
// composition coverage.
func reportDatasetFixture(t *testing.T, target string) (*phase29ExecutionFixture, *reporting.Authoring, identity.Envelope, reporting.AuthoringPrepareRequest, topics.Published, topics.Dataset, []string) {
	t.Helper()
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	s, err := reporting.NewAuthoring(f.documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	_, topicService := newPhase18Service(t, f.f)
	p, err := topicService.Read(ctx, f.blockAuthor, f.base.Topics[0].Topic, f.base.Topics[0].Version)
	if err != nil {
		t.Fatal(err)
	}
	// The publication orders datasets independently from its reviewed measures.
	// Select the exact reviewed revenue fact, never an arbitrary first dataset.
	var datasetID string
	for _, measure := range p.Definition.Measures {
		if measure.ID == "revenue" {
			datasetID = measure.Field.Dataset
		}
	}
	var dataset topics.Dataset
	for _, candidate := range p.Definition.Datasets {
		if candidate.ID == datasetID {
			dataset = candidate
		}
	}
	if dataset.ID == "" {
		t.Fatal("reviewed revenue dataset missing")
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "reporting.validate", "topics.read", "sources.read", "sources.query", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:" + target, "cw.block.write:" + target, "cw.block.preview:" + target, "cw.topic.read:" + p.Definition.Topic, "cw.topic.write:" + p.Definition.Topic, "cw.source.query:" + dataset.Source.Source}
	for _, d := range p.Definition.Datasets {
		for _, scope := range []string{"cw.source.read:" + d.Source.Source, "cw.dataset.query:" + d.ID, "cw.execution_context.use:" + d.Source.Context} {
			if !slices.Contains(scopes, scope) {
				scopes = append(scopes, scope)
			}
		}
	}
	author := phase27Actor(t, f.f, f.author.User(), scopes)
	catalog, err := s.Dataset(ctx, author, reporting.AuthoringDatasetRequest{Topic: f.base.Topics[0], Dataset: dataset.ID})
	if err != nil || !catalog.Supported {
		t.Fatal(catalog, err)
	}
	var measure reporting.AuthoringSemanticField
	for _, candidate := range catalog.Measures {
		if candidate.Supported {
			measure = candidate
			break
		}
	}
	if measure.ID == "" {
		t.Fatal("fixture measure missing")
	}
	request := reporting.AuthoringPrepareRequest{NewBlock: target, Operation: "dataset-prepare-one", Intent: reporting.AuthoringDatasetIntent{Topic: catalog.Topic, Dataset: catalog.Dataset, Dimensions: []string{}, Measure: measure.ID, Mapping: reporting.AuthoringChartMapping{Kind: charts.KPI, Bindings: charts.Bindings{Value: measure.Binding}, Order: []charts.Order{}, Options: charts.DefaultOptions()}}, Metadata: []reporting.Localized{{Locale: "en", Title: "Revenue", Question: "Revenue", Aliases: []string{}}}}
	return f, s, author, request, p, dataset, scopes
}

func TestReportAppDeterministicPreparation(t *testing.T) {
	const target = "dataset-private-chart"
	f, s, author, request, p, dataset, scopes := reportDatasetFixture(t, target)
	ctx := t.Context()
	before, models := f.attemptCount(t), f.f.model.requests.Load()
	var preparations [2]reporting.AuthoringPreparationView
	var failures [2]error
	var wg sync.WaitGroup
	for i := range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); preparations[i], failures[i] = s.PrepareDatasetChart(ctx, author, request) }()
	}
	wg.Wait()
	for _, err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if preparations[0].Preparation != preparations[1].Preparation || f.attemptCount(t) != before+1 || f.f.model.requests.Load() != models {
		t.Fatal("duplicate read/model work", preparations)
	}
	prepared, err := s.Preparation(ctx, author, reporting.AuthoringPreparationRequest{NewBlock: target, Operation: request.Operation})
	if err != nil || prepared.Status != "prepared" || len(prepared.Schema) != 1 {
		t.Fatal(prepared, err)
	}
	if prepared.Schema[0].NativeType == "" || prepared.Digest == "" {
		t.Fatal("no actual source schema")
	}
	for _, missing := range []string{"charts.bind", "reporting.read", "cw.tenant.read:" + author.Tenant(), "cw.block.preview:" + target, "cw.dataset.query:" + dataset.ID} {
		denied := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool { return scope == missing }))
		byID, idErr := f.f.f.db.ReadAuthoringPreparation(ctx, denied, prepared.Preparation)
		byOperation, operationErr := f.f.f.db.ReadAuthoringPreparationOperation(ctx, denied, request.Operation)
		if idErr == nil || operationErr == nil || !reflect.DeepEqual(byID, reporting.AuthoringPreparationRecord{}) || !reflect.DeepEqual(byOperation, reporting.AuthoringPreparationRecord{}) {
			t.Fatal("denied preparation projected protected custody", missing, idErr, operationErr)
		}
	}
	changed := request
	changed.Metadata = append([]reporting.Localized(nil), request.Metadata...)
	changed.Metadata[0].Title = "Changed"
	if _, err := s.PrepareDatasetChart(ctx, author, changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	revoked := phase27Actor(t, f.f, author.User(), slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool { return scope == "sources.query" }))
	create := reporting.AuthoringCreatePreparedRequest{NewBlock: target, Preparation: prepared.Preparation, Digest: prepared.Digest}
	if _, err := s.CreatePreparedChart(ctx, revoked, create); err == nil {
		t.Fatal("revoked source authority consumed custody")
	}
	if _, err := s.CreatePreparedChart(ctx, phase27Actor(t, f.f, "different-actor", scopes), create); err == nil {
		t.Fatal("cross actor")
	}
	wrong := create
	wrong.Digest = strings.Repeat("0", 64)
	if _, err := s.CreatePreparedChart(ctx, author, wrong); err == nil {
		t.Fatal("wrong digest")
	}
	created, err := s.CreatePreparedChart(ctx, author, create)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Block.Private || created.Block.State.DraftState != "draft" || created.Block.Evidence != nil || f.attemptCount(t) != before+1 {
		t.Fatal("create fabricated validation or queried", created)
	}
	replay, err := s.CreatePreparedChart(ctx, author, create)
	if err != nil || replay.Block.Digest != created.Block.Digest || f.attemptCount(t) != before+1 {
		t.Fatal("create retry", replay, err)
	}
	validation, err := f.blocks.Validate(ctx, author, target, reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision})
	if err != nil || validation.State.DraftState != "validated" {
		t.Fatal(validation, err)
	}
	if f.attemptCount(t) != before+2 || f.f.model.requests.Load() != models {
		t.Fatal("expected distinct native validation read")
	}
	// Publication/certification metadata custody keeps the safe absence pins,
	// without granting SQL access or returning the remaining origin metadata.
	lifecycleScopes := append(slices.Clone(scopes), "reporting.publish", "reporting.certify", "cw.block.publish:"+target, "cw.block.certify:"+target)
	lifecycleActor := phase27Actor(t, f.f, author.User(), lifecycleScopes)
	for _, mode := range []reporting.Access{reporting.Publish, reporting.Certify} {
		snapshot, err := f.f.f.db.ReadBlock(ctx, lifecycleActor, target, reporting.Reference{Revision: created.Block.Revision}, mode)
		if err != nil || len(snapshot.Revision.Provenance.RuleAbsence) != 1 || snapshot.Revision.Definition.SQL != "" || snapshot.Revision.Provenance.Kind != "" || snapshot.Revision.Provenance.Query != "" {
			t.Fatal("redacted lifecycle custody stripped absence pins or exposed SQL", mode, err)
		}
	}
	// Complete the same chart's native private report journey, without a
	// publication shortcut or another source read during metadata saving.
	const reportID = "dataset-private-report"
	reportScopes := append(slices.Clone(scopes), "reporting.execute", "cw.block.execute:"+target, "cw.report.read:"+reportID, "cw.report.write:"+reportID, "cw.report.preview:"+reportID, "cw.report.execute:"+reportID)
	reportActor := phase27Actor(t, f.f, author.User(), reportScopes)
	widget := phase29BlockWidget("dataset-chart", target, 0, "chart")
	widget.Block.Policy, widget.Block.Revision, widget.Block.Digest = "private_preview", created.Block.Revision, created.Block.Digest
	document := phase29Text("Dataset chart report")
	document.SchemaVersion, document.Widgets = reporting.PagedDocumentVersion, nil
	document.ReportPages = []reporting.ReportPage{{ID: "analysis", Title: "Analysis", Widgets: []reporting.Widget{widget}}, {ID: "empty", Title: "Empty", Widgets: []reporting.Widget{}}}
	state, err := s.Create(ctx, reportActor, reporting.AuthoringCreateRequest{ID: reportID, Definition: document})
	if err != nil {
		t.Fatal("save dataset chart report", err)
	}
	if _, err := s.Read(ctx, reportActor, reporting.AuthoringReadRequest{Report: reportID}); err != nil {
		t.Fatal(err)
	}
	if f.attemptCount(t) != before+2 {
		t.Fatal("saving metadata executed source")
	}
	preview, err := s.Preview(ctx, reportActor, reporting.AuthoringPreviewRequest{Report: reportID, Revision: state.DraftRevision, Key: "dataset-report-preview"})
	if err != nil || !preview.Private || preview.QueryGroups != 1 || len(preview.Pages) != 2 {
		t.Fatal(preview, err)
	}
	done, err := s.Execute(ctx, reportActor, reporting.AuthoringExecuteRequest{Run: preview.ID})
	if err != nil || !done.Private || !done.Complete {
		t.Fatal(done, err)
	}
	readerScopes := []string{"reporting.read", "reporting.preview", "cw.run.read:" + preview.ID, "cw.report.preview:" + reportID, "cw.execution_context.use:" + dataset.Source.Context}
	reader := phase27Actor(t, f.f, author.User(), readerScopes)
	payload, err := f.compositions.Widget(ctx, reader, preview.ID, "analysis", "dataset-chart")
	if err != nil || len(payload.Outputs) != 1 || payload.Outputs[0].Chart == nil || len(payload.Outputs[0].Chart.Points) != 1 || payload.Outputs[0].Chart.Points[0].Value.Exact != "9007199254740998.625" {
		t.Fatal("retained exact reviewed sum missing", payload, err)
	}
	if state.PublishedRevision != 0 || f.attemptCount(t) != before+3 || f.f.model.requests.Load() != models {
		t.Fatal("private journey published or exceeded its three explicit reads")
	}
	// Optional synthetic fixture export for the two host-adapter browser tests.
	// No bearer, SQL, native control handle or source credential is included.
	if path := os.Getenv("CHARTWORKS_DATASET_FIXTURE_PATH"); path != "" {
		catalog, err := s.Dataset(ctx, author, reporting.AuthoringDatasetRequest{Topic: request.Intent.Topic, Dataset: dataset.ID})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := json.MarshalIndent(map[string]any{"dataset": catalog, "request": request, "preparation": prepared, "created": created, "validation": validation, "report_state": state, "preview": preview, "complete": done, "payload": payload}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, wire, 0600); err != nil {
			t.Fatal(err)
		}
	}
	pending, err := s.Preview(ctx, reportActor, reporting.AuthoringPreviewRequest{Report: reportID, Revision: state.DraftRevision, Key: "dataset-rule-race-preview"})
	if err != nil || pending.QueryGroups != 1 {
		t.Fatal(pending, err)
	}
	// Derived copies must retain the origin fence across their own validation.
	copyTarget := "dataset-private-copy"
	copyScopes := append(slices.Clone(scopes), "cw.block.read:"+copyTarget, "cw.block.write:"+copyTarget, "cw.block.preview:"+copyTarget)
	copyActor := phase27Actor(t, f.f, author.User(), copyScopes)
	copied, err := s.CopyBlockMapping(ctx, copyActor, reporting.AuthoringBlockCopyRequest{Block: target, ExpectedVersion: validation.State.Version, Revision: created.Block.Revision, Digest: created.Block.Digest, Output: "chart", NewBlock: copyTarget, Mapping: request.Intent.Mapping})
	if err != nil {
		t.Fatal(err)
	}
	copySnapshot, err := f.f.f.db.ReadBlock(ctx, copyActor, copyTarget, reporting.Reference{Revision: copied.Block.Revision}, reporting.Write)
	if err != nil || len(copySnapshot.Revision.Provenance.RuleAbsence) != 1 {
		t.Fatal("copy stripped prepared origin", err)
	}
	rules, err := rulesets.New(f.f.f.db, f.f.f.db, f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	phase17PublishRules(t, rules, f.blockAuthor, p)
	before = f.attemptCount(t)
	if _, err := f.blocks.Validate(ctx, copyActor, copyTarget, reporting.ValidateRequest{ExpectedVersion: copied.Block.State.Version, Revision: copied.Block.Revision}); err == nil {
		t.Fatal("rules activated after Create were ignored by copied target")
	}
	if _, err := f.blocks.Preview(ctx, author, target, reporting.PreviewRequest{ValidateRequest: reporting.ValidateRequest{ExpectedVersion: validation.State.Version, Revision: created.Block.Revision}, Outputs: []string{"chart"}}); err == nil {
		t.Fatal("rules activated after validation were ignored by preview")
	}
	if _, err := s.Execute(ctx, reportActor, reporting.AuthoringExecuteRequest{Run: pending.ID}); err == nil {
		t.Fatal("rule activation after private seal allowed execution")
	}
	if _, err := f.compositions.Widget(ctx, reader, preview.ID, "analysis", "dataset-chart"); err != nil {
		t.Fatal("rule activation erased completed private result", err)
	}
	if f.attemptCount(t) != before {
		t.Fatal("stale origin performed source work")
	}
}

// A real native read completes while the owning validation remains paused.
// Publishing a real rule set in that interval must discard its result/evidence.
type pausedPreparedValidation struct {
	delegate reporting.Executor
	ready    chan struct{}
	release  chan struct{}
}

func (p *pausedPreparedValidation) Execute(ctx context.Context, e identity.Envelope, plan readexec.Plan, options readexec.Options) (readexec.ExecutionReport, error) {
	out, err := p.delegate.Execute(ctx, e, plan, options)
	close(p.ready)
	select {
	case <-ctx.Done():
		return readexec.ExecutionReport{}, ctx.Err()
	case <-p.release:
		return out, err
	}
}

func TestReportAppPreparedRuleActivationDuringValidation(t *testing.T) {
	const target = "dataset-validation-rule-race"
	f, s, author, request, publication, _, _ := reportDatasetFixture(t, target)
	ctx := t.Context()
	prepared, err := s.PrepareDatasetChart(ctx, author, request)
	if err != nil || prepared.Status != "prepared" {
		t.Fatal(prepared, err)
	}
	created, err := s.CreatePreparedChart(ctx, author, reporting.AuthoringCreatePreparedRequest{NewBlock: target, Preparation: prepared.Preparation, Digest: prepared.Digest})
	if err != nil {
		t.Fatal(err)
	}
	gate := &pausedPreparedValidation{delegate: f.f.f.executor, ready: make(chan struct{}), release: make(chan struct{})}
	_, topicService := newPhase18Service(t, f.f)
	blocks, err := reporting.New(f.f.f.db, topicService, f.f.f.s, f.f.f.validator, gate, nil, f.limits)
	if err != nil {
		t.Fatal(err)
	}
	finished := make(chan error, 1)
	validationDone := make(chan struct{})
	defer func() {
		select {
		case <-gate.release:
		default:
			close(gate.release)
		}
		<-validationDone
	}()
	go func() {
		defer close(validationDone)
		_, err := blocks.Validate(ctx, author, target, reporting.ValidateRequest{ExpectedVersion: created.Block.State.Version, Revision: created.Block.Revision})
		finished <- err
	}()
	select {
	case <-gate.ready:
	case err := <-finished:
		t.Fatal("validation did not reach native read", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	rules, err := rulesets.New(f.f.f.db, f.f.f.db, f.f.f.db)
	if err != nil {
		t.Fatal(err)
	}
	phase17PublishRules(t, rules, f.blockAuthor, publication)
	close(gate.release)
	if err := <-finished; !errors.Is(err, reporting.ErrStale) {
		t.Fatal("concurrent rule publication blessed validation", err)
	}
	current, err := f.blocks.Read(ctx, author, target, reporting.Reference{Revision: created.Block.Revision})
	if err != nil || current.State.DraftState != "draft" || current.Evidence != nil {
		t.Fatal("concurrent stale read persisted native evidence", current, err)
	}
}
