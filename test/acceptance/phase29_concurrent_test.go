package acceptance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/jobs"
	"github.com/hurtener/chartworks/internal/reporting"
)

type phase29PausedSeal struct {
	reporting.CompositionRepository
	resolved chan struct{}
	release  chan struct{}
	once     sync.Once
}

func (r *phase29PausedSeal) SealComposition(ctx context.Context, e identity.Envelope, task jobs.RequestTask, proof reporting.PreparedComposition) (reporting.CompositionRecord, error) {
	r.once.Do(func() { close(r.resolved) })
	select {
	case <-r.release:
		return r.CompositionRepository.SealComposition(ctx, e, task, proof)
	case <-ctx.Done():
		return reporting.CompositionRecord{}, ctx.Err()
	}
}

func TestReportingCompositionConcurrentPublication(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f.block(t, "concurrent-block", f.base)
	firstDefinition := phase29Text("Original first report")
	firstDefinition.Widgets = append(firstDefinition.Widgets, phase29BlockWidget("frozen", "concurrent-block", 1, "table-main"))
	first := f.report(t, "concurrent-first", firstDefinition, true)
	secondDefinition := phase29Text("Original second report")
	secondDefinition.Widgets = append(secondDefinition.Widgets, phase29BlockWidget("frozen", "concurrent-block", 1, "table-second"))
	second := f.report(t, "concurrent-second", secondDefinition, true)
	dashboardDefinition := phase29Text("Original dashboard")
	dashboardDefinition.Widgets = nil
	dashboardDefinition.Pages = []reporting.DocumentPage{
		{ID: "z-first", Title: "Original first page", Report: first.ID, Revision: 1},
		{ID: "a-second", Title: "Original second page", Report: second.ID, Revision: 1},
	}
	dashboard, err := f.documents.Create(ctx, f.author, "dashboard", "concurrent-dashboard", dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	dashboard = phase29Publish(t, f.documents, f.author, dashboard)
	paused := &phase29PausedSeal{CompositionRepository: f.f.f.db, resolved: make(chan struct{}), release: make(chan struct{})}
	runner, err := jobs.NewRequestRunner(f.f.f.db, jobs.Defaults())
	if err != nil {
		t.Fatal(err)
	}
	compositions, err := reporting.NewCompositions(f.documents, paused, f.runs, nil, runner)
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		view reporting.CompositionView
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		view, err := compositions.Admit(ctx, f.execute, "dashboard", dashboard.ID, reporting.CompositionRequest{Key: "concurrent-seal"})
		done <- outcome{view, err}
	}()
	select {
	case <-paused.resolved:
	case <-ctx.Done():
		t.Fatal("admission did not reach its controlled pre-seal boundary")
	}
	// Publish all three new heads while the old manifest is resolved but not
	// sealed. No metadata transaction locks are held by this test barrier.
	oldBlock, err := f.blocks.Read(ctx, f.blockAuthor, "concurrent-block", reporting.Reference{})
	if err != nil {
		t.Fatal(err)
	}
	changedBlock := phase27Copy(t, f.base)
	changedBlock.SQL = "SELECT id, amount FROM analytics.sales WHERE id=2 ORDER BY id"
	amendedBlock, err := f.blocks.Edit(ctx, f.blockAuthor, "concurrent-block", reporting.EditRequest{ExpectedVersion: oldBlock.State.Version, Definition: changedBlock})
	if err != nil {
		t.Fatal(err)
	}
	phase27ValidatePublish(t, f.blocks, f.blockAuthor, amendedBlock)
	first, err = f.documents.Edit(ctx, f.author, "report", first.ID, first.Version, reporting.DocumentReference{}, phase29Text("Changed first report"))
	if err != nil {
		t.Fatal(err)
	}
	first = phase29Publish(t, f.documents, f.author, first)
	dashboardDefinition.Pages[0], dashboardDefinition.Pages[1] = dashboardDefinition.Pages[1], dashboardDefinition.Pages[0]
	dashboardDefinition.Pages[1].Revision = first.PublishedRevision
	dashboardDefinition.Pages[1].Title = "Changed page title"
	dashboard, err = f.documents.Edit(ctx, f.author, "dashboard", dashboard.ID, dashboard.Version, reporting.DocumentReference{}, dashboardDefinition)
	if err != nil {
		t.Fatal(err)
	}
	phase29Publish(t, f.documents, f.author, dashboard)
	close(paused.release)
	var accepted outcome
	select {
	case accepted = <-done:
	case <-ctx.Done():
		t.Fatal("admission did not complete after concurrent publication")
	}
	v := accepted.view
	if accepted.err != nil || v.Revision != 1 || len(v.Pages) != 2 || v.Pages[0].ID != "z-first" || v.Pages[1].ID != "a-second" || v.Pages[0].Title != "Original first page" || v.Pages[0].Revision != 1 || v.QueryGroups != 1 {
		t.Fatal("concurrent publication changed an already resolved manifest", v, accepted.err)
	}
	beforeQueries := f.attemptCount(t)
	completed, err := f.compositions.Run(ctx, f.execute, v.ID, false)
	if err != nil || !completed.Complete || completed.Manifest != v.Manifest || f.attemptCount(t) != beforeQueries+1 {
		t.Fatal("resolved shared block was repeated or replaced", completed, err)
	}
	record, err := f.f.f.db.ReadComposition(ctx, f.execute, v.ID)
	if err != nil || len(record.Results) != 1 || record.Results[0].Block == nil || record.Results[0].Block.Revision != 1 || len(record.Results[0].Outputs) != 2 {
		t.Fatal("old exact block revision/output union was not executed", record, err)
	}
}
