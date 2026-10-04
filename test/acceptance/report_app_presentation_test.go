package acceptance

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/hurtener/chartworks/internal/auth"
	"github.com/hurtener/chartworks/internal/charts"
	"github.com/hurtener/chartworks/internal/config"
	"github.com/hurtener/chartworks/internal/identity"
	"github.com/hurtener/chartworks/internal/reporting"
	"github.com/hurtener/chartworks/internal/store"
	"github.com/hurtener/chartworks/test/support"
)

// Real immutable storage and native authority are exercised with synthetic
// definitions. The authoring service has no source executor or model dependency.
func TestReportAppPresentationAuthoring(t *testing.T) {
	f := newPhase29Execution(t, false)
	ctx := t.Context()
	const sourceID, copyID = "presentation-source", "presentation-copy"
	f.block(t, sourceID, f.base)
	source, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, sourceID, reporting.Reference{}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	if source.Validation == nil {
		t.Fatal("published source lacks fixture evidence")
	}
	if _, err := f.blocks.Certify(ctx, f.blockAuthor, sourceID, reporting.CertifyRequest{ExpectedVersion: source.State.Version, Revision: source.Revision.Number, Evidence: source.Validation.Evidence.ID, Note: "Review synthetic source before display copy"}); err != nil {
		t.Fatal(err)
	}
	source, err = f.f.f.db.ReadBlock(ctx, f.blockAuthor, sourceID, reporting.Reference{}, reporting.Write)
	if err != nil || source.Attestation == nil {
		t.Fatal("certified source fixture", err)
	}
	run, err := f.runs.Admit(ctx, f.execute, sourceID, reporting.RunRequest{Key: "presentation-before-copy", Outputs: []string{"table-main"}, Locale: "en-US"})
	if err != nil {
		t.Fatal(err)
	}
	run, err = f.runs.Run(ctx, f.execute, run.ID, false)
	if err != nil || run.State != "succeeded" {
		t.Fatal("retained setup", run, err)
	}
	retainedBefore, err := f.runs.Output(ctx, f.execute, run.ID, "table-main")
	if err != nil {
		t.Fatal(err)
	}
	_, topics := newPhase18Service(t, f.f)
	blocks, err := reporting.New(f.f.f.db, topics, nil, nil, nil, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	documents, err := reporting.NewDocuments(f.f.f.db, blocks, nil, config.DefaultReporting())
	if err != nil {
		t.Fatal(err)
	}
	service, err := reporting.NewAuthoring(documents, f.compositions)
	if err != nil {
		t.Fatal(err)
	}
	scopes := []string{"reporting.read", "reporting.write", "reporting.preview", "topics.read", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.tenant.write:" + f.author.Tenant(), "cw.block.read:" + sourceID, "cw.block.preview:" + sourceID, "cw.block.read:" + copyID, "cw.block.write:" + copyID, "cw.block.preview:" + copyID, "cw.topic.write:" + source.State.Topic}
	for _, ref := range source.References {
		scope := "cw." + ref.Kind + "." + ref.Permission + ":" + ref.ID
		if !slices.Contains(scopes, scope) {
			scopes = append(scopes, scope)
		}
	}
	actor := func(tenant, user string, grants []string) identity.Envelope {
		t.Helper()
		claims := f.f.f.token.claims(tenant, user, grants)
		claims["session"] = "phase27-session"
		e, err := f.f.f.token.verifier.Verify(ctx, f.f.f.token.sign(t, claims, nil), auth.HTTP)
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	e := actor(f.author.Tenant(), f.author.User(), scopes)
	column := ""
	for _, c := range source.Revision.Definition.Outputs[0].Mapping.Columns {
		if c.Name == "amount" && slices.Contains([]string{"integer", "decimal", "number"}, c.Type) && c.Format.Percent == "" {
			column = c.ID
		}
	}
	if column == "" {
		t.Fatal("fixture lacks canonical numeric amount column")
	}
	label, digits := "Displayed total", 2
	patch := charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: column, Set: &charts.ColumnPresentationSet{DisplayLabel: &label, FractionDigits: &digits}}}}
	request := reporting.AuthoringBlockPresentationCopyRequest{Block: sourceID, NewBlock: copyID, Revision: source.Revision.Number, Digest: source.Revision.Digest, ExpectedVersion: source.State.Version, Output: "table-main", Presentation: patch}
	sourceCalls, modelCalls, attempts := f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)
	assertNoWork := func(t *testing.T) {
		t.Helper()
		if f.f.f.lookups.Load() != sourceCalls || f.f.model.requests.Load() != modelCalls || f.attemptCount(t) != attempts {
			t.Fatal("presentation metadata executed source/model work")
		}
	}
	view, err := service.ReadBlock(ctx, e, reporting.AuthoringBlockReadRequest{Block: sourceID, Revision: 1})
	if err != nil || view.DataValidation != "not_performed" || view.Block.Outputs[0].Presentation == nil {
		t.Fatal("SQL-free capability read", err)
	}
	encoded, err := json.Marshal(view)
	if err != nil || strings.Contains(string(encoded), `"sql"`) || strings.Contains(string(encoded), f.base.SQL) {
		t.Fatal("SQL escaped metadata projection", err)
	}
	missingScopes := []string{"reporting.read", "reporting.write", "reporting.preview", "charts.bind", "cw.tenant.read:" + f.author.Tenant(), "cw.block.read:" + sourceID, "cw.block.preview:" + sourceID, "cw.block.write:" + copyID, "cw.tenant.write:" + f.author.Tenant(), "cw.topic.write:" + source.State.Topic}
	for _, ref := range source.References {
		missingScopes = append(missingScopes, "cw."+ref.Kind+"."+ref.Permission+":"+ref.ID)
	}
	for _, missing := range missingScopes {
		denied := actor(f.author.Tenant(), f.author.User(), slices.DeleteFunc(slices.Clone(scopes), func(scope string) bool { return scope == missing }))
		if _, err := service.CopyBlockPresentation(ctx, denied, request); err == nil {
			t.Fatal("missing copy authority", missing)
		}
	}
	foreign := slices.Clone(scopes)
	for i, s := range foreign {
		foreign[i] = strings.ReplaceAll(s, ":"+f.author.Tenant(), ":foreign")
	}
	if _, err := service.CopyBlockPresentation(ctx, actor("foreign", f.author.User(), foreign), request); err == nil {
		t.Fatal("cross-tenant copy")
	}
	wrongContext := slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == "cw.execution_context.use:"+f.base.Context })
	wrongContext = append(wrongContext, "cw.execution_context.use:other-context")
	if _, err := service.CopyBlockPresentation(ctx, actor(f.author.Tenant(), f.author.User(), wrongContext), request); err == nil {
		t.Fatal("same-tenant other-context copy")
	}
	for _, tc := range []struct {
		name   string
		change func(*reporting.AuthoringBlockPresentationCopyRequest)
	}{
		{"stale version", func(r *reporting.AuthoringBlockPresentationCopyRequest) { r.ExpectedVersion++ }},
		{"wrong revision", func(r *reporting.AuthoringBlockPresentationCopyRequest) { r.Revision++ }},
		{"wrong digest", func(r *reporting.AuthoringBlockPresentationCopyRequest) { r.Digest = strings.Repeat("a", 64) }},
		{"ungranted target", func(r *reporting.AuthoringBlockPresentationCopyRequest) { r.NewBlock = "other-target" }},
		{"same target", func(r *reporting.AuthoringBlockPresentationCopyRequest) { r.NewBlock = sourceID }},
		{"wrong output", func(r *reporting.AuthoringBlockPresentationCopyRequest) { r.Output = "other-output" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := request
			tc.change(&bad)
			if _, err := service.CopyBlockPresentation(ctx, e, bad); err == nil {
				t.Fatal("unqualified copy admitted")
			}
		})
	}
	publishedAmendment := reporting.AuthoringBlockPresentationRequest{Block: sourceID, Revision: source.Revision.Number, Digest: source.Revision.Digest, ExpectedVersion: source.State.Version, Output: "table-main", Presentation: patch}
	if _, err := service.PatchBlockPresentation(ctx, actor(f.author.Tenant(), f.author.User(), append(slices.Clone(scopes), "cw.block.write:"+sourceID)), publishedAmendment); !errors.Is(err, store.ErrConflict) {
		t.Fatal("published revision amended", err)
	}
	copied, err := service.CopyBlockPresentation(ctx, e, request)
	if err != nil {
		t.Fatal("presentation copy", err)
	}
	if !copied.Block.Private || copied.Block.Evidence != nil || copied.Block.State.PublishedRevision != 0 || copied.Block.Trust.Certification != "none" || copied.DataValidation != "not_performed" {
		t.Fatal("copy inherited publication")
	}
	stored, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Revision: 1}, reporting.Write)
	if err != nil {
		t.Fatal(err)
	}
	expected := phase27Copy(t, source.Revision.Definition)
	expected.Outputs[0].Mapping.Presentation = phase27Copy(t, stored.Revision.Definition.Outputs[0].Mapping.Presentation)
	if !reflect.DeepEqual(expected, stored.Revision.Definition) || source.Revision.ExecutionDigest != stored.Revision.ExecutionDigest || source.Revision.Digest == stored.Revision.Digest || stored.Validation != nil || stored.Attestation != nil || stored.PublishedAt != nil {
		t.Fatal("display-only copy changed execution/canonical definition")
	}
	if count(t, support.Raw(t, f.f.f.dsn), `SELECT count(*) FROM chartworks.frozen_runs WHERE tenant_id=$1 AND block_id=$2`, e.Tenant(), copyID) != 0 {
		t.Fatal("copy inherited retained result custody")
	}
	if _, err := service.CopyBlockPresentation(ctx, e, request); !errors.Is(err, store.ErrConflict) {
		t.Fatal("copy collision", err)
	}
	if _, err := f.blocks.Publish(ctx, f.blockAuthor, copyID, reporting.PublishRequest{ExpectedVersion: 1, Evidence: source.Validation.Evidence.ID}); err == nil {
		t.Fatal("source approval reused")
	}
	if _, err := service.ReadBlock(ctx, actor(f.author.Tenant(), "another-actor", scopes), reporting.AuthoringBlockReadRequest{Block: copyID, Revision: 1}); err == nil {
		t.Fatal("cross-actor private read")
	}
	privateCopy := request
	privateCopy.Block, privateCopy.NewBlock = copyID, sourceID
	privateCopy.ExpectedVersion, privateCopy.Revision, privateCopy.Digest = 1, 1, copied.Block.Digest
	if _, err := service.CopyBlockPresentation(ctx, actor(f.author.Tenant(), "another-actor", append(slices.Clone(scopes), "cw.block.write:"+sourceID)), privateCopy); err == nil || errors.Is(err, store.ErrConflict) {
		t.Fatal("other actor reached private-copy mutation", err)
	}
	assertNoWork(t)
	// Explicit validation is the sole source execution in this authoring lane.
	validated, err := f.blocks.Validate(ctx, f.blockAuthor, copyID, reporting.ValidateRequest{ExpectedVersion: 1})
	if err != nil || validated.Evidence.DefinitionDigest != copied.Block.Digest {
		t.Fatal("fresh exact-copy validation", err)
	}
	sourceCalls, modelCalls, attempts = f.f.f.lookups.Load(), f.f.model.requests.Load(), f.attemptCount(t)
	label2 := "Revised display"
	amendment := reporting.AuthoringBlockPresentationRequest{Block: copyID, Revision: 1, Digest: copied.Block.Digest, ExpectedVersion: validated.State.Version, Output: "table-main", Presentation: charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: column, Set: &charts.ColumnPresentationSet{DisplayLabel: &label2}}}}}
	if _, err := service.PatchBlockPresentation(ctx, actor(f.author.Tenant(), "another-actor", scopes), amendment); err == nil {
		t.Fatal("cross-actor private edit")
	}
	for _, missing := range []string{"charts.bind", "cw.block.read:" + copyID, "cw.block.write:" + copyID, "cw.block.preview:" + copyID, "cw.execution_context.use:" + f.base.Context} {
		denied := actor(f.author.Tenant(), f.author.User(), slices.DeleteFunc(slices.Clone(scopes), func(s string) bool { return s == missing }))
		if _, err := service.PatchBlockPresentation(ctx, denied, amendment); err == nil {
			t.Fatal("private edit missing authority", missing)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.PatchBlockPresentation(ctx, e, amendment)
			results <- err
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else if !errors.Is(err, store.ErrConflict) {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatal("CAS winners", winners)
	}
	current, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Draft: true}, reporting.Write)
	if err != nil || current.Revision.Number != 2 || current.State.Version != validated.State.Version+1 || current.Validation != nil || current.Attestation != nil || current.PublishedAt != nil {
		t.Fatal("presentation edit retained approval or wrong CAS", err)
	}
	if current.Revision.ExecutionDigest != stored.Revision.ExecutionDigest || current.Revision.Digest == stored.Revision.Digest {
		t.Fatal("presentation edit changed execution identity or retained definition identity")
	}
	for _, tc := range []struct {
		name   string
		change func(*reporting.AuthoringBlockPresentationRequest)
	}{
		{"stale head", func(r *reporting.AuthoringBlockPresentationRequest) { r.ExpectedVersion-- }},
		{"stale revision", func(r *reporting.AuthoringBlockPresentationRequest) { r.Revision = 1 }},
		{"stale digest", func(r *reporting.AuthoringBlockPresentationRequest) { r.Digest = copied.Block.Digest }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bad := amendment
			bad.ExpectedVersion, bad.Revision, bad.Digest = current.State.Version, current.Revision.Number, current.Revision.Digest
			tc.change(&bad)
			if _, err := service.PatchBlockPresentation(ctx, e, bad); !errors.Is(err, store.ErrConflict) {
				t.Fatal("stale edit admitted", err)
			}
		})
	}
	noop := amendment
	noop.ExpectedVersion, noop.Revision, noop.Digest = current.State.Version, current.Revision.Number, current.Revision.Digest
	if _, err := service.PatchBlockPresentation(ctx, e, noop); !errors.Is(err, reporting.ErrInvalid) {
		t.Fatal("no-op appended a revision", err)
	}
	if _, err := f.blocks.Publish(ctx, f.blockAuthor, copyID, reporting.PublishRequest{ExpectedVersion: current.State.Version, Evidence: validated.Evidence.ID}); err == nil {
		t.Fatal("previous display evidence published an amendment")
	}
	oldCopy, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Revision: 1}, reporting.Write)
	if err != nil || !reflect.DeepEqual(oldCopy.Revision, stored.Revision) {
		t.Fatal("immutable prior copy revision changed", err)
	}
	reset := noop
	reset.Presentation = charts.PresentationPatch{Version: 1, Edits: []charts.ColumnPresentationEdit{{Column: column, Reset: []charts.PresentationField{charts.PresentationDisplayLabel, charts.PresentationFractionDigits}}}}
	resetView, err := service.PatchBlockPresentation(ctx, e, reset)
	if err != nil || resetView.Block.Revision != 3 || resetView.Block.State.Version != current.State.Version+1 || resetView.Block.Digest != source.Revision.Digest || resetView.Block.ExecutionDigest != source.Revision.ExecutionDigest || resetView.Block.Evidence != nil {
		t.Fatal("reset did not restore exact canonical definition without approval", err)
	}
	resetStored, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, copyID, reporting.Reference{Revision: 3}, reporting.Write)
	if err != nil || !reflect.DeepEqual(resetStored.Revision.Definition, source.Revision.Definition) || resetStored.Revision.Definition.Outputs[0].Mapping.Presentation != nil {
		t.Fatal("reset rewrote canonical/sibling bytes or persisted empty overlay", err)
	}
	if _, err := f.blocks.Publish(ctx, f.blockAuthor, copyID, reporting.PublishRequest{ExpectedVersion: resetView.Block.State.Version, Evidence: source.Validation.Evidence.ID}); err == nil {
		t.Fatal("reset inherited matching source definition evidence")
	}
	// Neither state transition permits editing a forbidden current draft.
	for _, state := range []string{"rejected", "archived"} {
		t.Run(state+" draft", func(t *testing.T) {
			id := "presentation-" + state
			created, err := f.blocks.Create(ctx, f.blockAuthor, reporting.CreateRequest{ID: id, Definition: f.base})
			if err != nil {
				t.Fatal(err)
			}
			transition := reporting.TransitionRequest{ExpectedVersion: created.State.Version, Note: "Synthetic lifecycle eligibility check"}
			var changed reporting.State
			if state == "rejected" {
				changed, err = f.blocks.Reject(ctx, f.blockAuthor, id, transition)
			} else {
				changed, err = f.blocks.Archive(ctx, f.blockAuthor, id, transition)
			}
			if err != nil {
				t.Fatal(err)
			}
			grants := append(slices.Clone(scopes), "cw.block.read:"+id, "cw.block.write:"+id, "cw.block.preview:"+id)
			blocked := reporting.AuthoringBlockPresentationRequest{Block: id, ExpectedVersion: changed.Version, Revision: created.Revision, Digest: created.Digest, Output: "table-main", Presentation: patch}
			if _, err := service.PatchBlockPresentation(ctx, actor(f.author.Tenant(), f.author.User(), grants), blocked); err == nil {
				t.Fatal("ineligible draft amended")
			}
		})
	}
	original, err := f.f.f.db.ReadBlock(ctx, f.blockAuthor, sourceID, reporting.Reference{Revision: 1}, reporting.Write)
	if err != nil || !reflect.DeepEqual(original.Revision, source.Revision) || original.State.Version != source.State.Version {
		t.Fatal("immutable source changed", err)
	}
	retainedAfter, err := f.runs.Output(ctx, f.execute, run.ID, "table-main")
	if err != nil || !reflect.DeepEqual(retainedBefore, retainedAfter) {
		t.Fatal("immutable retained source output changed", err)
	}
	assertNoWork(t)
	// Fresh evidence must bind the reset revision even when its definition digest
	// happens to match the immutable published source.
	fresh, err := f.blocks.Validate(ctx, f.blockAuthor, copyID, reporting.ValidateRequest{ExpectedVersion: resetView.Block.State.Version})
	if err != nil || fresh.Evidence.Revision != 3 || fresh.Evidence.DefinitionDigest != resetView.Block.Digest || fresh.Evidence.ID == validated.Evidence.ID {
		t.Fatal("fresh reset validation", err)
	}
	published, err := f.blocks.Publish(ctx, f.blockAuthor, copyID, reporting.PublishRequest{ExpectedVersion: fresh.State.Version, Evidence: fresh.Evidence.ID})
	if err != nil || published.PublishedRevision != 3 {
		t.Fatal("fresh evidence did not permit explicit publication", err)
	}
	if _, err := service.ReadBlock(ctx, actor(f.author.Tenant(), "another-actor", scopes), reporting.AuthoringBlockReadRequest{Block: copyID, Revision: 1}); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("publication exposed historical private revision", err)
	}
}
