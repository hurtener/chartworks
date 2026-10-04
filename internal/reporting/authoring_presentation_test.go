package reporting

import (
	"errors"
	"reflect"
	"slices"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/store"
)

func authoringPresentation(d Definition) AuthoringBlockPresentationRequest {
	label, digits := "Displayed total", 2
	return AuthoringBlockPresentationRequest{Block: "block", ExpectedVersion: 4, Revision: 1, Digest: DefinitionDigest(d), Output: "table", Presentation: charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: "c0", Set: &charts.ColumnPresentationSet{DisplayLabel: &label, FractionDigits: &digits}}}}}
}

func TestAuthoringPresentationOnlyPreservesExecutionAndCanonicalDefinition(t *testing.T) {
	s, repo, e, d := authoringBlockFixture(t, false)
	before := clone(repo.revisions["tenant:block"][1])
	out, err := s.PatchBlockPresentation(t.Context(), e, authoringPresentation(d))
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.revisions["tenant:block"][2]
	if out.Block.Revision != 2 || out.Block.State.Version != 5 || out.Block.Evidence != nil || !out.Block.Private || out.DataValidation != "not_performed" {
		t.Fatal("unexpected presentation revision lifecycle", out)
	}
	want := clone(d)
	want.Outputs[0].Mapping.Presentation = clone(stored.Revision.Definition.Outputs[0].Mapping.Presentation)
	if want.Outputs[0].Mapping.Presentation == nil || !reflect.DeepEqual(want, stored.Revision.Definition) || !reflect.DeepEqual(before, repo.revisions["tenant:block"][1]) {
		t.Fatal("display change altered canonical or unrelated definition fields")
	}
	if ExecutionDigest(d) != stored.Revision.ExecutionDigest || DefinitionDigest(d) == stored.Revision.Digest {
		t.Fatal("presentation must change definition identity, not execution identity")
	}
	if _, err := s.documents.blocks.Publish(t.Context(), e, "block", PublishRequest{ExpectedVersion: 5, Evidence: "old-evidence"}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("display amendment reused source validation", err)
	}
}

func TestAuthoringPresentationAdmissionAndNoop(t *testing.T) {
	for _, missing := range []string{"charts.bind", "reporting.write", "cw.block.write:block", "cw.block.preview:block", "cw.tenant.read:tenant"} {
		t.Run(missing, func(t *testing.T) {
			s, repo, e, d := authoringBlockFixture(t, false)
			scopes := slices.DeleteFunc(e.Scopes(), func(scope string) bool { return scope == missing })
			e = authoringBlockActor(t, "tenant", "author", scopes)
			if _, err := s.PatchBlockPresentation(t.Context(), e, authoringPresentation(d)); err == nil || repo.commits != 0 {
				t.Fatal("missing authority admitted a display write", err)
			}
		})
	}
	s, repo, e, d := authoringBlockFixture(t, false)
	request := authoringPresentation(d)
	out, err := s.PatchBlockPresentation(t.Context(), e, request)
	if err != nil {
		t.Fatal(err)
	}
	request.ExpectedVersion, request.Revision, request.Digest = out.Block.State.Version, out.Block.Revision, out.Block.Digest
	if _, err = s.PatchBlockPresentation(t.Context(), e, request); !errors.Is(err, ErrInvalid) || repo.commits != 1 {
		t.Fatal("no-op appended a revision", err, repo.commits)
	}
}

func TestAuthoringPresentationCopyAndConcurrentCAS(t *testing.T) {
	s, repo, e, d := authoringBlockFixture(t, true)
	before := clone(repo.revisions["tenant:block"][1])
	p := authoringPresentation(d)
	copyRequest := AuthoringBlockPresentationCopyRequest{Block: p.Block, NewBlock: "copy", ExpectedVersion: p.ExpectedVersion, Revision: p.Revision, Digest: p.Digest, Output: p.Output, Presentation: p.Presentation}
	out, err := s.CopyBlockPresentation(t.Context(), e, copyRequest)
	if err != nil {
		t.Fatal(err)
	}
	if !out.Block.Private || out.Block.Evidence != nil || out.Block.State.PublishedRevision != 0 || digest(before) != digest(repo.revisions["tenant:block"][1]) {
		t.Fatal("copy mutated source or transferred approval")
	}
	if _, err = s.CopyBlockPresentation(t.Context(), e, copyRequest); !errors.Is(err, store.ErrConflict) {
		t.Fatal("copy collision", err)
	}
	s, repo, e, d = authoringBlockFixture(t, false)
	p = authoringPresentation(d)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := s.PatchBlockPresentation(t.Context(), e, p)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	wins := 0
	for err := range results {
		if err == nil {
			wins++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if wins != 1 || repo.commits != 1 {
		t.Fatal("presentation CAS lost", wins, repo.commits)
	}
}

func TestAuthoringMappingPreservesOrRejectsPresentation(t *testing.T) {
	s, repo, e, d := authoringBlockFixture(t, false)
	out, err := s.PatchBlockPresentation(t.Context(), e, authoringPresentation(d))
	if err != nil {
		t.Fatal(err)
	}
	current := repo.revisions["tenant:block"][2].Revision.Definition
	m := current.Outputs[0].Mapping
	request := AuthoringBlockMappingRequest{Block: "block", ExpectedVersion: out.Block.State.Version, Revision: out.Block.Revision, Digest: out.Block.Digest, Output: "table", Mapping: AuthoringChartMapping{Kind: m.Kind, Bindings: clone(m.Bindings), Order: clone(m.Order), Options: clone(m.Options), KPI: clone(m.KPI), Table: clone(m.Table)}}
	request.Mapping.Options.Title = "New title"
	next, err := s.PatchBlockMapping(t.Context(), e, request)
	if err != nil {
		t.Fatal("legacy mapping request dropped overlay", err)
	}
	saved := repo.revisions["tenant:block"][3].Revision.Definition.Outputs[0].Mapping
	if !reflect.DeepEqual(saved.Presentation, m.Presentation) {
		t.Fatal("overlay erased by old mapping request")
	}
	request.ExpectedVersion, request.Revision, request.Digest = next.Block.State.Version, next.Block.Revision, next.Block.Digest
	request.Mapping = authoringBarMapping()
	if _, err := s.PatchBlockMapping(t.Context(), e, request); !errors.Is(err, ErrInvalid) || repo.commits != 2 {
		t.Fatal("table header silently retargeted to plot", err, repo.commits)
	}
}

func TestAuthoringPresentationPrivateRevisionPins(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*AuthoringBlockPresentationRequest)
	}{
		{"stale head", func(p *AuthoringBlockPresentationRequest) { p.ExpectedVersion++ }},
		{"stale digest", func(p *AuthoringBlockPresentationRequest) {
			p.Digest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}},
		{"foreign output", func(p *AuthoringBlockPresentationRequest) { p.Output = "absent" }},
		{"foreign column", func(p *AuthoringBlockPresentationRequest) { p.Presentation.Edits[0].Column = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, repo, e, d := authoringBlockFixture(t, false)
			p := authoringPresentation(d)
			tc.mutate(&p)
			if _, err := s.PatchBlockPresentation(t.Context(), e, p); err == nil || repo.commits != 0 {
				t.Fatal("invalid display edit admitted", err)
			}
		})
	}
	s, repo, e, d := authoringBlockFixture(t, true)
	if _, err := s.PatchBlockPresentation(t.Context(), e, authoringPresentation(d)); !errors.Is(err, store.ErrConflict) || repo.commits != 0 {
		t.Fatal("published source edited", err)
	}
}

func TestAuthoringMappingReordersPresentationByStableColumnIdentity(t *testing.T) {
	s, repo, e, d := authoringBlockFixture(t, false)
	p := authoringPresentation(d)
	other := "Count of unknown amounts"
	p.Presentation.Edits = append(p.Presentation.Edits, charts.ColumnPresentationEdit{Column: "counter", Set: &charts.ColumnPresentationSet{DisplayLabel: &other}})
	out, err := s.PatchBlockPresentation(t.Context(), e, p)
	if err != nil {
		t.Fatal(err)
	}
	current := repo.revisions["tenant:block"][2].Revision.Definition.Outputs[0].Mapping
	request := AuthoringBlockMappingRequest{Block: "block", ExpectedVersion: out.Block.State.Version, Revision: out.Block.Revision, Digest: out.Block.Digest, Output: "table", Mapping: AuthoringChartMapping{Kind: charts.Table, Bindings: charts.Bindings{Columns: []string{"counter", "c0"}}, Order: clone(current.Order), Options: clone(current.Options)}}
	if _, err := s.PatchBlockMapping(t.Context(), e, request); err != nil {
		t.Fatal("valid reorder rejected", err)
	}
	after := repo.revisions["tenant:block"][3].Revision.Definition.Outputs[0].Mapping
	if len(after.Presentation.Columns) != 2 || after.Presentation.Columns[0].Column != "counter" || after.Presentation.Columns[1].Column != "c0" || !reflect.DeepEqual(after.Presentation.Columns[0], current.Presentation.Columns[1]) || !reflect.DeepEqual(after.Presentation.Columns[1], current.Presentation.Columns[0]) {
		t.Fatal("presentation did not follow stable field identity")
	}
}
